package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const (
	simulationResultPartsMediaType = "application/vnd.semantic-idf.simulation-parts+json"
	simulationResultPartsSchema    = "semantic-idf.simulation-parts/v1"
	simulationPartTargetBytes      = 2 << 20
	simulationPartMaximumBytes     = 4 << 20
	simulationPartInlineBytes      = 4 << 10
	simulationCatalogInlineBytes   = 64 << 10
	simulationLegacyFallbackBytes  = 448 << 20
)

// Only representation size limitations qualify for the explicitly negotiated
// legacy fallback. Invalid data or internal errors must not be hidden by it.
type simulationResultPartsSizeError struct {
	kind  string
	bytes int
}

func (e *simulationResultPartsSizeError) Error() string {
	return fmt.Sprintf("simulation result %s is %d bytes and exceeds the %d-byte resource limit", e.kind, e.bytes, simulationPartMaximumBytes)
}

// Parts are projections of an already serialized immutable snapshot. Their
// slices retain its exact number tokens and custom JSON marshal semantics;
// producing or fetching a part never reparses SQL or rebuilds a result.
type simulationResultParts struct {
	runID, textHash, snapshot string
	source                    simulationPartsSource
	manifest                  []byte
	resources                 []simulationResultPart
}

type simulationPartsSource struct {
	first           *byte
	length          int
	runID, textHash string
}
type simulationPartsBuild struct {
	done   chan struct{}
	result *simulationResultParts
	err    error
}

type simulationPartBinding struct {
	Path   []any `json:"path"`
	Offset *int  `json:"offset,omitempty"`
	Count  int   `json:"count,omitempty"`
	Length int   `json:"length,omitempty"`
}

type simulationPartDescriptor struct {
	ID         string                  `json:"id"`
	ByteLength int                     `json:"byteLength"`
	Group      string                  `json:"group"`
	Bindings   []simulationPartBinding `json:"bindings"`
}

type simulationResultPart struct {
	descriptor simulationPartDescriptor
	segments   [][]byte
}

type simulationPartsBuilder struct {
	payload   []byte
	resources []simulationResultPart
	open      map[string]int
	protected *simulationPartPathNode
	catalog   bool
	idOffset  int
}

type simulationPartPathNode struct {
	children map[any]*simulationPartPathNode
}

func (n *simulationPartPathNode) add(path []any) {
	for _, key := range path {
		if index, ok := key.(float64); ok {
			key = int(index)
		}
		if n.children == nil {
			n.children = map[any]*simulationPartPathNode{}
		}
		if n.children[key] == nil {
			n.children[key] = &simulationPartPathNode{}
		}
		n = n.children[key]
	}
}

func (n *simulationPartPathNode) contains(path []any) bool {
	for _, key := range path {
		if n == nil {
			return false
		}
		n = n.children[key]
	}
	return n != nil
}

// Read only the compact envelope's small binding catalogs. Their owners must
// remain structural metadata even when an array of null columns is large.
func simulationPartEnvelope(payload []byte) (bool, *simulationPartPathNode, error) {
	protected := &simulationPartPathNode{}
	compact := false
	position := jsonSkipSpace(payload, 0)
	if position >= len(payload) || payload[position] != '{' {
		return false, protected, fmt.Errorf("invalid simulation result object")
	}
	position = jsonSkipSpace(payload, position+1)
	for position < len(payload) && payload[position] != '}' {
		keyEnd, err := jsonStringEnd(payload, position)
		if err != nil {
			return false, nil, err
		}
		var key string
		if err := json.Unmarshal(payload[position:keyEnd], &key); err != nil {
			return false, nil, err
		}
		start := jsonSkipSpace(payload, keyEnd)
		if start >= len(payload) || payload[start] != ':' {
			return false, nil, fmt.Errorf("invalid simulation result object")
		}
		start = jsonSkipSpace(payload, start+1)
		end, err := jsonValueEnd(payload, start)
		if err != nil {
			return false, nil, err
		}
		if key == "schema" {
			var schema string
			if err := json.Unmarshal(payload[start:end], &schema); err != nil {
				return false, nil, err
			}
			compact = schema == "semantic-idf.simulation-transfer/v1"
		}
		if key == "pointBindings" || key == "columnBindings" {
			var bindings []struct {
				Path []any `json:"path"`
			}
			if err := json.Unmarshal(payload[start:end], &bindings); err != nil {
				return false, nil, err
			}
			for _, binding := range bindings {
				protected.add(append([]any{"result"}, binding.Path...))
			}
		}
		position = jsonSkipSpace(payload, end)
		if position < len(payload) && payload[position] == ',' {
			position = jsonSkipSpace(payload, position+1)
		}
	}
	return compact, protected, nil
}

func acceptsSimulationResultParts(r *http.Request) bool {
	return acceptsSimulationResultMediaType(r, simulationResultPartsMediaType)
}

func acceptsSimulationResultMediaType(r *http.Request, mediaType string) bool {
	for _, value := range strings.Split(r.Header.Get("Accept"), ",") {
		if strings.TrimSpace(strings.SplitN(value, ";", 2)[0]) == mediaType {
			return true
		}
	}
	return false
}

func buildSimulationResultParts(payload []byte, runID, textHash, snapshot string) (*simulationResultParts, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("simulation result snapshot is empty")
	}
	compact, protected, err := simulationPartEnvelope(payload)
	if err != nil {
		return nil, err
	}
	builder := simulationPartsBuilder{payload: payload, open: map[string]int{}, protected: protected}
	var skeleton bytes.Buffer
	var path []any
	if !compact {
		skeleton.WriteString(`{"schema":"semantic-idf.simulation-transfer/v1","timelines":[],"pointSets":[],"pointBindings":[],"columnSets":[],"columnBindings":[],"result":`)
		path = []any{"result"}
	}
	end, err := builder.value(&skeleton, 0, path)
	if err != nil {
		return nil, err
	}
	if !compact {
		skeleton.WriteByte('}')
	}
	if jsonSkipSpace(payload, end) != len(payload) {
		return nil, fmt.Errorf("unexpected data after simulation result")
	}
	descriptors := make([]simulationPartDescriptor, len(builder.resources))
	for i := range builder.resources {
		descriptors[i] = builder.resources[i].descriptor
	}
	// Catalogs are a separate, required stage. They may contain the structural
	// ancestors of lazy data, so finish hydrating them before installing data
	// bindings. This also bounds large selector catalogs and descriptor lists.
	catalog, err := json.Marshal(struct {
		Root  json.RawMessage            `json:"root"`
		Parts []simulationPartDescriptor `json:"parts"`
	}{skeleton.Bytes(), descriptors})
	if err != nil {
		return nil, err
	}
	catalogBuilder := simulationPartsBuilder{payload: catalog, open: map[string]int{}, catalog: true, idOffset: len(builder.resources)}
	var catalogSkeleton bytes.Buffer
	if _, err = catalogBuilder.value(&catalogSkeleton, 0, nil); err != nil {
		return nil, err
	}
	var catalogRoot struct {
		Root  json.RawMessage `json:"root"`
		Parts json.RawMessage `json:"parts"`
	}
	if err = json.Unmarshal(catalogSkeleton.Bytes(), &catalogRoot); err != nil {
		return nil, err
	}
	catalogDescriptors := make([]simulationPartDescriptor, len(catalogBuilder.resources))
	for i := range catalogBuilder.resources {
		catalogDescriptors[i] = catalogBuilder.resources[i].descriptor
	}
	manifest, err := json.Marshal(struct {
		Schema       string                     `json:"schema"`
		RunID        string                     `json:"runId"`
		TextHash     string                     `json:"textHash"`
		Snapshot     string                     `json:"snapshot"`
		Root         json.RawMessage            `json:"root"`
		Parts        json.RawMessage            `json:"parts"`
		CatalogParts []simulationPartDescriptor `json:"catalogParts"`
	}{simulationResultPartsSchema, runID, textHash, snapshot, catalogRoot.Root, catalogRoot.Parts, catalogDescriptors})
	if err != nil {
		return nil, err
	}
	if len(manifest) > simulationPartMaximumBytes {
		return nil, &simulationResultPartsSizeError{"catalog", len(manifest)}
	}
	return &simulationResultParts{runID: runID, textHash: textHash, snapshot: snapshot,
		source: simulationPartsSource{&payload[0], len(payload), runID, textHash}, manifest: manifest, resources: append(builder.resources, catalogBuilder.resources...)}, nil
}

// Structural catalogs retain the ancestry needed for selectors, source
// identity and lazy transport bindings. Other collections are independent
// resources. This declarative rule also handles newly added result fields.
func simulationPartCatalog(path []any) bool {
	if len(path) == 0 {
		return false
	}
	key, _ := path[len(path)-1].(string)
	switch key {
	case "series", "hvacLoops", "components", "zones", "zoneResults", "periods", "sources", "pointSets", "columnSets", "timelines", "pointBindings", "columnBindings":
		return true
	}
	return false
}

func simulationPartMetadataLeaf(path []any) bool {
	if len(path) == 0 {
		return false
	}
	key, _ := path[len(path)-1].(string)
	switch key {
	case "summary", "quality", "completeness", "energyExplanationSummary":
		return true
	}
	return false
}

func simulationPartGroup(path []any) string {
	for _, key := range path {
		switch key {
		case "energy", "energyExplanation", "energyExplanationSummary":
			return "energy"
		case "heatFlow", "zoneHeatFlow", "thermalTopology":
			return "heat-flow"
		case "hvacLoops":
			return "hvac"
		case "comfort":
			return "comfort"
		case "pointSets", "columnSets", "timelines", "pointBindings", "columnBindings":
			return "shared"
		}
	}
	if len(path) > 0 && path[0] == "series" || len(path) > 1 && path[0] == "result" && path[1] == "series" {
		return "series"
	}
	return "metadata"
}

func (b *simulationPartsBuilder) value(out *bytes.Buffer, start int, path []any) (int, error) {
	start = jsonSkipSpace(b.payload, start)
	end, err := jsonValueEnd(b.payload, start)
	if err != nil {
		return start, err
	}
	size := end - start
	inlineBytes := simulationPartInlineBytes
	if b.catalog {
		inlineBytes = simulationCatalogInlineBytes
	}
	switch b.payload[start] {
	case '[':
		if size > inlineBytes && (b.catalog || !simulationPartCatalog(path)) && !b.protected.contains(path) {
			if ok, err := b.arrayResource(start, end, path); err != nil {
				return start, err
			} else if ok {
				out.WriteString("[]")
				return end, nil
			}
		}
		out.WriteByte('[')
		position, index := jsonSkipSpace(b.payload, start+1), 0
		for position < end-1 {
			if index > 0 {
				out.WriteByte(',')
			}
			position, err = b.value(out, position, simulationPartPath(path, index))
			if err != nil {
				return start, err
			}
			index++
			position = jsonSkipSpace(b.payload, position)
			if position < end-1 {
				position = jsonSkipSpace(b.payload, position+1)
			}
		}
		out.WriteByte(']')
	case '{':
		if len(path) > 0 && size > inlineBytes && size+2 <= simulationPartTargetBytes && (b.catalog || simulationPartMetadataLeaf(path)) && !b.protected.contains(path) {
			b.addResource(path, simulationPartBinding{Path: path}, [][]byte{b.payload[start:end]}, size)
			out.WriteString("null")
			return end, nil
		}
		out.WriteByte('{')
		position, count := jsonSkipSpace(b.payload, start+1), 0
		for position < end-1 {
			if count > 0 {
				out.WriteByte(',')
			}
			keyEnd, err := jsonStringEnd(b.payload, position)
			if err != nil {
				return start, err
			}
			var key string
			if err := json.Unmarshal(b.payload[position:keyEnd], &key); err != nil {
				return start, err
			}
			out.Write(b.payload[position:keyEnd])
			out.WriteByte(':')
			position = jsonSkipSpace(b.payload, keyEnd)
			if position >= end || b.payload[position] != ':' {
				return start, fmt.Errorf("invalid simulation result object")
			}
			position, err = b.value(out, position+1, simulationPartPath(path, key))
			if err != nil {
				return start, err
			}
			count++
			position = jsonSkipSpace(b.payload, position)
			if position < end-1 {
				position = jsonSkipSpace(b.payload, position+1)
			}
		}
		out.WriteByte('}')
	default:
		if size+2 > simulationPartMaximumBytes {
			return start, &simulationResultPartsSizeError{"scalar", size + 2}
		}
		if size > inlineBytes {
			b.addResource(path, simulationPartBinding{Path: path}, [][]byte{b.payload[start:end]}, size)
			out.WriteString("null")
		} else {
			out.Write(b.payload[start:end])
		}
	}
	return end, nil
}

func (b *simulationPartsBuilder) arrayResource(start, end int, path []any) (bool, error) {
	type span struct{ start, end, offset, count int }
	var chunks []span
	position, length := jsonSkipSpace(b.payload, start+1), 0
	current := span{start: position, offset: 0}
	for position < end-1 {
		next, err := jsonValueEnd(b.payload, position)
		if err != nil {
			return false, err
		}
		if next-position+4 > simulationPartMaximumBytes {
			return false, nil
		}
		if current.count > 0 && next-current.start+4 > simulationPartTargetBytes {
			chunks = append(chunks, current)
			current = span{start: position, offset: length}
		}
		current.end, current.count = next, current.count+1
		length++
		position = jsonSkipSpace(b.payload, next)
		if position < end-1 {
			position = jsonSkipSpace(b.payload, position+1)
		}
	}
	if current.count > 0 {
		chunks = append(chunks, current)
	}
	for _, chunk := range chunks {
		offset := chunk.offset
		binding := simulationPartBinding{Path: path, Offset: &offset, Count: chunk.count, Length: length}
		b.addResource(path, binding, [][]byte{[]byte("["), b.payload[chunk.start:chunk.end], []byte("]")}, chunk.end-chunk.start+2)
	}
	return true, nil
}

func (b *simulationPartsBuilder) addResource(path []any, binding simulationPartBinding, segments [][]byte, size int) {
	group := simulationPartGroup(path)
	if b.catalog {
		group = "metadata"
	}
	index, exists := b.open[group]
	if !exists || b.resources[index].descriptor.ByteLength+size+1 > simulationPartTargetBytes {
		index = len(b.resources)
		b.resources = append(b.resources, simulationResultPart{descriptor: simulationPartDescriptor{ID: "p" + strconv.Itoa(b.idOffset+index), Group: group, ByteLength: 2}})
		b.open[group] = index
	}
	part := &b.resources[index]
	if len(part.descriptor.Bindings) > 0 {
		part.segments = append(part.segments, []byte(","))
		part.descriptor.ByteLength++
	}
	part.descriptor.Bindings = append(part.descriptor.Bindings, binding)
	part.segments = append(part.segments, segments...)
	part.descriptor.ByteLength += size
}

func simulationPartPath(path []any, key any) []any {
	next := make([]any, len(path)+1)
	copy(next, path)
	next[len(path)] = key
	return next
}

func jsonSkipSpace(data []byte, offset int) int {
	for offset < len(data) {
		switch data[offset] {
		case ' ', '\n', '\r', '\t':
			offset++
		default:
			return offset
		}
	}
	return offset
}

func jsonStringEnd(data []byte, start int) (int, error) {
	if start >= len(data) || data[start] != '"' {
		return start, fmt.Errorf("invalid simulation result string")
	}
	for i := start + 1; i < len(data); i++ {
		if data[i] == '\\' {
			i++
			continue
		}
		if data[i] == '"' {
			return i + 1, nil
		}
	}
	return start, fmt.Errorf("unterminated simulation result string")
}

func jsonValueEnd(data []byte, start int) (int, error) {
	start = jsonSkipSpace(data, start)
	if start >= len(data) {
		return start, io.ErrUnexpectedEOF
	}
	if data[start] == '"' {
		return jsonStringEnd(data, start)
	}
	if data[start] == '{' || data[start] == '[' {
		depth := 1
		for i := start + 1; i < len(data); i++ {
			if data[i] == '"' {
				end, err := jsonStringEnd(data, i)
				if err != nil {
					return start, err
				}
				i = end - 1
				continue
			}
			switch data[i] {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return i + 1, nil
				}
			}
		}
		return start, io.ErrUnexpectedEOF
	}
	end := start
	for end < len(data) {
		switch data[end] {
		case ',', '}', ']', ' ', '\n', '\r', '\t':
			if end == start {
				return start, fmt.Errorf("invalid simulation result value")
			}
			return end, nil
		}
		end++
	}
	return end, nil
}

func (a *App) simulationPartsForPayload(payload []byte, runID, textHash string) (*simulationResultParts, error) {
	if len(payload) == 0 || bytes.Equal(payload, []byte("null")) {
		return nil, nil
	}
	source := simulationPartsSource{&payload[0], len(payload), runID, textHash}
	cache := &a.simulationWorkspaceCache
	cache.mu.Lock()
	for _, existing := range cache.parts {
		if existing.source == source {
			cache.mu.Unlock()
			return existing, nil
		}
	}
	if pending := cache.partsBuilding[source]; pending != nil {
		cache.mu.Unlock()
		<-pending.done
		return pending.result, pending.err
	}
	if cache.partsBuilding == nil {
		cache.partsBuilding = map[simulationPartsSource]*simulationPartsBuild{}
	}
	pending := &simulationPartsBuild{done: make(chan struct{})}
	cache.partsBuilding[source] = pending
	cache.mu.Unlock()
	var token [16]byte
	_, err := rand.Read(token[:])
	var parts *simulationResultParts
	if err == nil {
		parts, err = buildSimulationResultParts(payload, runID, textHash, hex.EncodeToString(token[:]))
	}
	cache.mu.Lock()
	if err == nil {
		if cache.parts == nil {
			cache.parts = map[string]*simulationResultParts{}
		}
		cache.parts[parts.snapshot] = parts
		cache.partsOrder = append(cache.partsOrder, parts.snapshot)
		// Retain the current workspace and one preceding response, so a late
		// older completion cannot replace the active run's resource identities.
		for len(cache.partsOrder) > 2 {
			remove := 0
			for remove < len(cache.partsOrder)-1 {
				candidate := cache.parts[cache.partsOrder[remove]]
				if len(cache.payload) == 0 || candidate.source.first != &cache.payload[0] {
					break
				}
				remove++
			}
			delete(cache.parts, cache.partsOrder[remove])
			cache.partsOrder = append(cache.partsOrder[:remove], cache.partsOrder[remove+1:]...)
		}
	}
	pending.result, pending.err = parts, err
	delete(cache.partsBuilding, source)
	close(pending.done)
	cache.mu.Unlock()
	return parts, err
}

func (a *App) writeSimulationPartsManifest(w http.ResponseWriter, r *http.Request, payload []byte, runID, textHash string) error {
	parts, err := a.simulationPartsForPayload(payload, runID, textHash)
	if err != nil {
		var sizeError *simulationResultPartsSizeError
		if errors.As(err, &sizeError) && len(payload) < simulationLegacyFallbackBytes && acceptsSimulationResultMediaType(r, compactSimulationResultMediaType) {
			// Legacy snapshots can include long unbounded stdout/stderr strings.
			// Preserve that working response within a conservative native string
			// ceiling, without repeating the simulation POST or rebuilding data.
			w.Header().Set("Content-Type", "application/json")
			return writeSimulationResultBytes(w, payload)
		}
		return err
	}
	if parts == nil {
		return writeSimulationResultBytes(w, nil)
	}
	w.Header().Set("Content-Type", simulationResultPartsMediaType)
	return writeSimulationResultBytes(w, parts.manifest)
}

func (a *App) serveSimulationResultPart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if len(r.URL.RawQuery) > 4096 {
		http.Error(w, "invalid result resource identity", http.StatusBadRequest)
		return
	}
	query := r.URL.Query()
	runID, textHash, snapshot, partID := query.Get("runId"), query.Get("textHash"), query.Get("snapshot"), query.Get("partId")
	if runID == "" || textHash == "" || len(snapshot) != 32 || len(partID) < 2 || partID[0] != 'p' {
		http.Error(w, "invalid result resource identity", http.StatusBadRequest)
		return
	}
	index, err := strconv.Atoi(partID[1:])
	if err != nil || index < 0 || partID != "p"+strconv.Itoa(index) {
		http.Error(w, "invalid result resource identity", http.StatusBadRequest)
		return
	}
	cache := &a.simulationWorkspaceCache
	cache.mu.RLock()
	parts := cache.parts[snapshot]
	if parts == nil || parts.runID != runID || parts.textHash != textHash || index >= len(parts.resources) {
		cache.mu.RUnlock()
		http.Error(w, "simulation result resource is no longer available", http.StatusNotFound)
		return
	}
	part := parts.resources[index]
	cache.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(part.descriptor.ByteLength))
	segments := make([][]byte, 0, len(part.segments)+2)
	segments = append(segments, []byte("["))
	segments = append(segments, part.segments...)
	segments = append(segments, []byte("]"))
	for _, segment := range segments {
		if n, err := w.Write(segment); err != nil || n != len(segment) {
			return
		}
	}
}
