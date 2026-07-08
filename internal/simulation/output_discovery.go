package simulation

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
)

type OutputDiscoveryRequest struct {
	Text            string                    `json:"text,omitempty"`
	OutputDirectory string                    `json:"outputDirectory,omitempty"`
	SQLPath         string                    `json:"sqlPath,omitempty"`
	RDDPath         string                    `json:"rddPath,omitempty"`
	MDDPath         string                    `json:"mddPath,omitempty"`
	PurposeRequest  *SimulationPurposeRequest `json:"purposeRequest,omitempty"`
}

type OutputDiscoveryResult struct {
	Items   []OutputDiscoveryItem `json:"items"`
	Sources []string              `json:"sources,omitempty"`
	Counts  map[string]int        `json:"counts,omitempty"`
}

type OutputDiscoveryItem struct {
	ObjectType         string                `json:"objectType"`
	KeyValue           string                `json:"keyValue,omitempty"`
	Name               string                `json:"name"`
	Units              string                `json:"units,omitempty"`
	ResourceType       string                `json:"resourceType,omitempty"`
	EndUseCategory     string                `json:"endUseCategory,omitempty"`
	MeterGroup         string                `json:"meterGroup,omitempty"`
	ReportingFrequency string                `json:"reportingFrequency,omitempty"`
	Source             string                `json:"source"`
	Status             string                `json:"status"`
	AliasOf            string                `json:"aliasOf,omitempty"`
	AliasReason        string                `json:"aliasReason,omitempty"`
	PurposeIDs         []SimulationPurposeID `json:"purposeIds,omitempty"`
}

type outputDiscoveryCacheEntry struct {
	modTimeUnixNano int64
	size            int64
	items           []OutputDiscoveryItem
}

var outputDiscoveryCache = struct {
	sync.Mutex
	items map[string]outputDiscoveryCacheEntry
}{items: map[string]outputDiscoveryCacheEntry{}}

func DiscoverAvailableOutputs(request OutputDiscoveryRequest) (OutputDiscoveryResult, error) {
	result := OutputDiscoveryResult{Counts: map[string]int{}}
	collector := outputDiscoveryCollector{items: map[string]OutputDiscoveryItem{}}
	for _, path := range discoverySQLPaths(request) {
		items, err := discoverOutputsCached("sql", path, discoverOutputsFromSQL)
		if err != nil {
			continue
		}
		for _, item := range items {
			collector.add(item)
		}
		if len(items) > 0 {
			result.Sources = append(result.Sources, filepath.Base(path))
		}
	}
	for _, path := range discoveryRDDPaths(request) {
		items, err := discoverOutputsCached("rdd", path, discoverOutputsFromRDD)
		if err != nil {
			continue
		}
		for _, item := range items {
			collector.add(item)
		}
		if len(items) > 0 {
			result.Sources = append(result.Sources, filepath.Base(path))
		}
	}
	for _, path := range discoveryMDDPaths(request) {
		items, err := discoverOutputsCached("mdd", path, discoverOutputsFromMDD)
		if err != nil {
			continue
		}
		for _, item := range items {
			collector.add(item)
		}
		if len(items) > 0 {
			result.Sources = append(result.Sources, filepath.Base(path))
		}
	}
	if strings.TrimSpace(request.Text) != "" && request.PurposeRequest != nil {
		doc, err := idf.Parse(request.Text)
		if err == nil {
			plan := BuildPurposeRunPlan(doc, NormalizeSimulationPurposeRequest(request.PurposeRequest))
			for _, object := range plan.OutputObjects {
				if !purposeObjectIsSeries(object.ObjectType) {
					continue
				}
				name := purposeOutputDiscoveryName(object)
				status := "fallback"
				aliasOf := ""
				aliasReason := ""
				units := ""
				source := "purpose_plan"
				if collector.has(object.ObjectType, object.KeyValue, name) {
					status = "available"
				} else if alias, ok := collector.aliasFor(object.ObjectType, object.KeyValue, name); ok {
					status = "alias"
					aliasOf = alias.Name
					aliasReason = "A discovered output can be used as an alias for this purpose output."
					units = alias.Units
					source = mergeDiscoveryToken(source, alias.Source)
				} else if purposeIDsContain(object.PurposeIDs, SimulationPurposeCustomOutputs) {
					status = "missing"
				}
				collector.add(OutputDiscoveryItem{
					ObjectType:         object.ObjectType,
					KeyValue:           object.KeyValue,
					Name:               name,
					Units:              units,
					ReportingFrequency: object.ReportingFrequency,
					Source:             source,
					Status:             status,
					AliasOf:            aliasOf,
					AliasReason:        aliasReason,
					PurposeIDs:         object.PurposeIDs,
				})
			}
		}
	}
	result.Items = collector.sorted()
	for _, item := range result.Items {
		result.Counts[item.Status]++
		if item.Source != "" {
			result.Counts["source:"+item.Source]++
		}
	}
	sort.Strings(result.Sources)
	result.Sources = normalizePurposeStrings(result.Sources)
	return result, nil
}

func purposeOutputDiscoveryName(object PurposeOutputObject) string {
	if strings.TrimSpace(object.VariableName) != "" {
		return strings.TrimSpace(object.VariableName)
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(object.ObjectType)), "output:meter") {
		return strings.TrimSpace(object.KeyValue)
	}
	return strings.TrimSpace(object.KeyValue)
}

func discoverOutputsCached(kind string, path string, loader func(string) ([]OutputDiscoveryItem, error)) ([]OutputDiscoveryItem, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, err
	}
	key := strings.ToLower(kind + ":" + cleanPath)
	modTime := info.ModTime().UnixNano()
	size := info.Size()
	outputDiscoveryCache.Lock()
	if entry, ok := outputDiscoveryCache.items[key]; ok && entry.modTimeUnixNano == modTime && entry.size == size {
		items := cloneOutputDiscoveryItems(entry.items)
		outputDiscoveryCache.Unlock()
		return items, nil
	}
	outputDiscoveryCache.Unlock()

	items, err := loader(cleanPath)
	if err != nil {
		return nil, err
	}
	outputDiscoveryCache.Lock()
	outputDiscoveryCache.items[key] = outputDiscoveryCacheEntry{
		modTimeUnixNano: modTime,
		size:            size,
		items:           cloneOutputDiscoveryItems(items),
	}
	outputDiscoveryCache.Unlock()
	return cloneOutputDiscoveryItems(items), nil
}

func cloneOutputDiscoveryItems(items []OutputDiscoveryItem) []OutputDiscoveryItem {
	out := make([]OutputDiscoveryItem, len(items))
	for index, item := range items {
		out[index] = item
		out[index].PurposeIDs = append([]SimulationPurposeID(nil), item.PurposeIDs...)
	}
	return out
}

type outputDiscoveryCollector struct {
	items map[string]OutputDiscoveryItem
}

func (collector *outputDiscoveryCollector) add(item OutputDiscoveryItem) {
	item.ObjectType = strings.TrimSpace(item.ObjectType)
	item.KeyValue = strings.TrimSpace(item.KeyValue)
	item.Name = strings.TrimSpace(item.Name)
	item.Units = strings.TrimSpace(item.Units)
	item.ResourceType = strings.TrimSpace(item.ResourceType)
	item.EndUseCategory = strings.TrimSpace(item.EndUseCategory)
	item.MeterGroup = strings.TrimSpace(item.MeterGroup)
	item.ReportingFrequency = canonicalPurposeFrequency(item.ReportingFrequency)
	item.Source = strings.TrimSpace(item.Source)
	item.Status = strings.TrimSpace(item.Status)
	item.AliasOf = strings.TrimSpace(item.AliasOf)
	item.AliasReason = strings.TrimSpace(item.AliasReason)
	enrichOutputDiscoveryMeterMetadata(&item)
	if item.Status == "" {
		item.Status = "available"
	}
	if item.ObjectType == "" || item.Name == "" {
		return
	}
	key := outputDiscoveryKey(item.ObjectType, item.KeyValue, item.Name)
	existing, ok := collector.items[key]
	if ok {
		existing.Source = mergeDiscoveryToken(existing.Source, item.Source)
		existing.Status = mergeDiscoveryStatus(existing.Status, item.Status)
		existing.PurposeIDs = normalizePurposeIDs(append(existing.PurposeIDs, item.PurposeIDs...))
		if existing.Units == "" {
			existing.Units = item.Units
		}
		if existing.ResourceType == "" {
			existing.ResourceType = item.ResourceType
		}
		if existing.EndUseCategory == "" {
			existing.EndUseCategory = item.EndUseCategory
		}
		if existing.MeterGroup == "" {
			existing.MeterGroup = item.MeterGroup
		}
		if existing.ReportingFrequency == "" {
			existing.ReportingFrequency = item.ReportingFrequency
		}
		if existing.AliasOf == "" {
			existing.AliasOf = item.AliasOf
		}
		if existing.AliasReason == "" {
			existing.AliasReason = item.AliasReason
		}
		collector.items[key] = existing
		return
	}
	collector.items[key] = item
}

func (collector outputDiscoveryCollector) has(objectType string, keyValue string, name string) bool {
	_, ok := collector.find(objectType, keyValue, name)
	return ok
}

func (collector outputDiscoveryCollector) find(objectType string, keyValue string, name string) (OutputDiscoveryItem, bool) {
	if item := collector.items[outputDiscoveryKey(objectType, keyValue, name)]; item.Name != "" {
		return item, true
	}
	if item := collector.items[outputDiscoveryKey(objectType, "*", name)]; item.Name != "" {
		return item, true
	}
	keys := make([]string, 0, len(collector.items))
	for key, item := range collector.items {
		if strings.EqualFold(item.ObjectType, objectType) && strings.EqualFold(item.Name, name) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		return collector.items[keys[0]], true
	}
	return OutputDiscoveryItem{}, false
}

func (collector outputDiscoveryCollector) aliasFor(objectType string, keyValue string, name string) (OutputDiscoveryItem, bool) {
	for _, alias := range outputDiscoveryAliases(objectType, name) {
		if item, ok := collector.find(objectType, keyValue, alias); ok {
			return item, true
		}
	}
	return OutputDiscoveryItem{}, false
}

func (collector outputDiscoveryCollector) sorted() []OutputDiscoveryItem {
	out := make([]OutputDiscoveryItem, 0, len(collector.items))
	for _, item := range collector.items {
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ObjectType != out[j].ObjectType {
			return strings.ToLower(out[i].ObjectType) < strings.ToLower(out[j].ObjectType)
		}
		if out[i].Name != out[j].Name {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		return strings.ToLower(out[i].KeyValue) < strings.ToLower(out[j].KeyValue)
	})
	return out
}

func discoverySQLPaths(request OutputDiscoveryRequest) []string {
	return discoveryPaths(request.SQLPath, request.OutputDirectory, "eplusout.sql", ".sql")
}

func discoveryRDDPaths(request OutputDiscoveryRequest) []string {
	return discoveryPaths(request.RDDPath, request.OutputDirectory, "eplusout.rdd", ".rdd")
}

func discoveryMDDPaths(request OutputDiscoveryRequest) []string {
	return discoveryPaths(request.MDDPath, request.OutputDirectory, "eplusout.mdd", ".mdd")
}

func discoveryPaths(explicit string, outputDirectory string, defaultName string, extension string) []string {
	paths := []string{}
	if strings.TrimSpace(explicit) != "" {
		paths = append(paths, explicit)
	}
	if strings.TrimSpace(outputDirectory) != "" {
		paths = append(paths, filepath.Join(outputDirectory, defaultName))
		entries, err := os.ReadDir(outputDirectory)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), extension) {
					continue
				}
				paths = append(paths, filepath.Join(outputDirectory, entry.Name()))
			}
		}
	}
	return normalizePurposeStrings(paths)
}

func discoverOutputsFromSQL(path string) ([]OutputDiscoveryItem, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	exists, err := sqlTableExists(db, "ReportDataDictionary")
	if err != nil || !exists {
		return nil, err
	}
	rows, err := db.Query(`
SELECT DISTINCT COALESCE(KeyValue, ''), COALESCE(Name, ''), COALESCE(Units, '')
FROM ReportDataDictionary
WHERE TRIM(COALESCE(Name, '')) <> ''
ORDER BY Name, KeyValue`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []OutputDiscoveryItem
	for rows.Next() {
		var keyValue, name, units string
		if err := rows.Scan(&keyValue, &name, &units); err != nil {
			continue
		}
		items = append(items, OutputDiscoveryItem{
			ObjectType: "Output:Variable",
			KeyValue:   keyValue,
			Name:       name,
			Units:      units,
			Source:     "sql",
			Status:     "available",
		})
	}
	return items, rows.Err()
}

func discoverOutputsFromRDD(path string) ([]OutputDiscoveryItem, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	items := []OutputDiscoveryItem{}
	for _, line := range strings.Split(string(content), "\n") {
		parts := dictionaryLineParts(line)
		if len(parts) < 2 {
			continue
		}
		name, units := splitDictionaryNameUnits(parts[len(parts)-1])
		keyValue := "*"
		if len(parts) >= 3 {
			keyValue = parts[0]
		}
		items = append(items, OutputDiscoveryItem{
			ObjectType: "Output:Variable",
			KeyValue:   keyValue,
			Name:       name,
			Units:      units,
			Source:     "rdd",
			Status:     "available",
		})
	}
	return items, nil
}

func discoverOutputsFromMDD(path string) ([]OutputDiscoveryItem, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	items := []OutputDiscoveryItem{}
	for _, line := range strings.Split(string(content), "\n") {
		parts := dictionaryLineParts(line)
		if len(parts) == 0 {
			continue
		}
		name, units := splitDictionaryNameUnits(parts[len(parts)-1])
		item := OutputDiscoveryItem{
			ObjectType: "Output:Meter",
			Name:       name,
			Units:      units,
			Source:     "mdd",
			Status:     "available",
		}
		enrichOutputDiscoveryMeterMetadata(&item)
		items = append(items, item)
	}
	return items, nil
}

func enrichOutputDiscoveryMeterMetadata(item *OutputDiscoveryItem) {
	if item == nil || !outputDiscoveryIsMeter(item.ObjectType) {
		return
	}
	name := item.Name
	if strings.TrimSpace(name) == "" {
		name = item.KeyValue
	}
	resourceType, endUseCategory, meterGroup := parseMeterDiscoveryName(name)
	if item.ResourceType == "" {
		item.ResourceType = resourceType
	}
	if item.EndUseCategory == "" {
		item.EndUseCategory = endUseCategory
	}
	if item.MeterGroup == "" {
		item.MeterGroup = meterGroup
	}
}

func parseMeterDiscoveryName(name string) (string, string, string) {
	raw := strings.Split(strings.TrimSpace(name), ":")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "", "", ""
	}
	resourceType := parts[0]
	endUseCategory := ""
	if len(parts) > 1 {
		endUseCategory = parts[1]
	}
	meterGroup := ""
	if len(parts) > 2 {
		meterGroup = strings.Join(parts[2:], ":")
	}
	return resourceType, endUseCategory, meterGroup
}

func outputDiscoveryIsMeter(objectType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(objectType)), "output:meter")
}

func dictionaryLineParts(line string) []string {
	line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
	if line == "" || strings.HasPrefix(line, "!") || strings.HasPrefix(strings.ToLower(line), "program version") {
		return nil
	}
	raw := strings.Split(line, ",")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func splitDictionaryNameUnits(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	start := strings.LastIndex(value, "[")
	end := strings.LastIndex(value, "]")
	if start >= 0 && end > start {
		return strings.TrimSpace(value[:start]), strings.TrimSpace(value[start+1 : end])
	}
	return value, ""
}

func outputDiscoveryKey(objectType string, keyValue string, name string) string {
	return normalizePurposeToken(objectType) + "|" + normalizePurposeToken(keyValue) + "|" + normalizePurposeToken(name)
}

func outputDiscoveryAliases(objectType string, name string) []string {
	switch strings.ToLower(strings.TrimSpace(objectType)) {
	case "output:variable":
		switch normalizePurposeToken(name) {
		case "zone mean air temperature":
			return []string{"Zone Air Temperature", "Space Mean Air Temperature"}
		default:
			return nil
		}
	case "output:meter", "output:meter:meterfileonly", "output:meter:cumulative", "output:meter:cumulativemeterfileonly":
		switch normalizePurposeToken(name) {
		case "naturalgas:facility":
			return []string{"Gas:Facility"}
		case "naturalgas:heating":
			return []string{"Gas:Heating"}
		case "naturalgas:watersystems":
			return []string{"Gas:WaterSystems"}
		case "gas:facility":
			return []string{"NaturalGas:Facility"}
		case "gas:heating":
			return []string{"NaturalGas:Heating"}
		case "gas:watersystems":
			return []string{"NaturalGas:WaterSystems"}
		default:
			return nil
		}
	}
	return nil
}

func mergeDiscoveryToken(left string, right string) string {
	if strings.TrimSpace(left) == "" {
		return strings.TrimSpace(right)
	}
	if strings.TrimSpace(right) == "" || strings.Contains("|"+left+"|", "|"+right+"|") {
		return left
	}
	return left + "|" + right
}

func mergeDiscoveryStatus(left string, right string) string {
	if left == "available" || right == "available" {
		return "available"
	}
	if left == "alias" || right == "alias" {
		return "alias"
	}
	if left == "fallback" || right == "fallback" {
		return "fallback"
	}
	if left == "missing" || right == "missing" {
		return "missing"
	}
	return strings.TrimSpace(left + " " + right)
}
