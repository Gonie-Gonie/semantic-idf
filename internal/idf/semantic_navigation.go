package idf

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// SemanticSourceAnchor locates the IDF source represented by a semantic line.
// ObjectID is stable for the lifetime of the logical source object; indexes are
// deliberately retained only as source-location fallbacks.
type SemanticSourceAnchor struct {
	ObjectID    string `json:"objectId,omitempty"`
	ObjectIndex *int   `json:"objectIndex,omitempty"`
	ObjectType  string `json:"objectType,omitempty"`
	ObjectName  string `json:"objectName,omitempty"`
	FieldIndex  *int   `json:"fieldIndex,omitempty"`
	FieldName   string `json:"fieldName,omitempty"`
}

type SemanticViewTarget struct {
	View       string `json:"view"`
	TargetKind string `json:"targetKind"`
	TargetID   string `json:"targetId"`
	Label      string `json:"label,omitempty"`
	Priority   int    `json:"priority,omitempty"`
}

type SemanticNavigationEntity struct {
	ID               string                 `json:"id"`
	Kind             string                 `json:"kind"`
	Label            string                 `json:"label"`
	SourceAnchors    []SemanticSourceAnchor `json:"sourceAnchors,omitempty"`
	OccurrenceIDs    []string               `json:"occurrenceIds,omitempty"`
	ViewTargets      []SemanticViewTarget   `json:"viewTargets,omitempty"`
	RelatedEntityIDs []string               `json:"relatedEntityIds,omitempty"`
}

type SemanticNavigationOccurrence = SemanticOccurrence

type SemanticNavigationIndex struct {
	Entities      []SemanticNavigationEntity     `json:"entities,omitempty"`
	Occurrences   []SemanticNavigationOccurrence `json:"occurrences,omitempty"`
	ByEntityID    map[string][]string            `json:"byEntityId,omitempty"`
	ByObjectID    map[string][]string            `json:"byObjectId,omitempty"`
	ByObjectIndex map[int][]string               `json:"byObjectIndex,omitempty"`
	ByViewTarget  map[string][]string            `json:"byViewTarget,omitempty"`
}

type semanticSourceRegistry struct {
	byObjectIndex map[int]string
	duplicateName map[int]bool
}

type semanticEntityDescriptor struct {
	ID    string
	Kind  string
	Label string
}

type semanticPathEntry struct {
	indent int
	label  string
}

type semanticNavigationBuildState struct {
	doc                        Document
	ctx                        *semanticContext
	registry                   semanticSourceRegistry
	projectKey                 string
	objectsByName              map[string][]Object
	hvacByObject               map[int]HVACNavigationEntity
	serviceByZone              map[string][]ZoneServicePath
	profileBySource            map[string]string
	geometryByObject           map[int]string
	geometryByEntity           map[string]string
	geometryByKindName         map[string][]string
	hvacBySemanticID           map[string]string
	hvacPathEntityByTarget     map[string]string
	hvacCouplingEntityByTarget map[string]string
	outputByObject             map[int]string
	entityIndex                map[string]int
	occurrenceIndex            map[string]int
	entities                   []SemanticNavigationEntity
	occurrences                []SemanticOccurrence
	byEntityID                 map[string][]string
	byObjectID                 map[string][]string
	byObjectIndex              map[int][]string
	byViewTarget               map[string][]string
	activeZone                 semanticEntityDescriptor
}

func buildSemanticNavigation(doc Document, ctx *semanticContext, model *SemanticModel) {
	registry := newSemanticSourceRegistry(doc)
	state := &semanticNavigationBuildState{
		doc:             doc,
		ctx:             ctx,
		registry:        registry,
		projectKey:      semanticProjectKey(doc, registry),
		objectsByName:   semanticObjectsByName(doc),
		hvacByObject:    semanticHVACEntitiesByObject(ctx),
		serviceByZone:   semanticServicePathsByZone(ctx),
		profileBySource: semanticProfileItemsBySource(ctx),
		entityIndex:     map[string]int{},
		occurrenceIndex: map[string]int{},
		byEntityID:      map[string][]string{},
		byObjectID:      map[string][]string{},
		byObjectIndex:   map[int][]string{},
		byViewTarget:    map[string][]string{},
	}
	state.indexPanelTargets()

	paths := semanticNodePaths(model.Nodes)
	for lineIndex := range model.Nodes {
		node := &model.Nodes[lineIndex]
		path := paths[lineIndex]
		state.prepareZoneContext(*node, path)
		node.SemanticPath = path
		node.SourceAnchor = state.sourceAnchorForNode(*node)

		descriptor := state.entityForNode(*node, path)
		if descriptor.ID == "" {
			continue
		}
		node.EntityID = descriptor.ID
		node.EntityKind = descriptor.Kind
		node.ViewTargets = state.viewTargetsFor(descriptor, *node, path)
		node.PreferredView, node.PreferredTargetID = semanticPreferredTarget(path, descriptor, node.ViewTargets)

		rootPath := semanticOccurrenceRootPath(path, descriptor, node.SourceAnchor)
		contextKind := semanticContextKind(path, descriptor.Kind)
		sourceObjectID := ""
		if node.SourceAnchor != nil {
			sourceObjectID = node.SourceAnchor.ObjectID
		}
		occurrenceKey := strings.Join([]string{descriptor.ID, contextKind, rootPath, sourceObjectID}, "\x00")
		occurrenceID := "occ-" + semanticStableHash(occurrenceKey, 20)
		node.OccurrenceID = occurrenceID

		occurrence := SemanticOccurrence{
			OccurrenceID:      occurrenceID,
			EntityID:          descriptor.ID,
			SourceObjectID:    sourceObjectID,
			Path:              rootPath,
			RoleHere:          semanticRoleForCanonicalPath(path),
			ContextKind:       contextKind,
			PreferredView:     node.PreferredView,
			PreferredTargetID: node.PreferredTargetID,
			Class:             node.ObjectType,
			Name:              descriptor.Label,
			SourceAnchor:      cloneSemanticSourceAnchor(node.SourceAnchor),
			ViewTargets:       append([]SemanticViewTarget(nil), node.ViewTargets...),
			LineIndexes:       []int{lineIndex},
		}
		state.addOccurrence(occurrence)
		state.addEntityLine(descriptor, occurrence, *node, path)
	}

	// Synthetic occurrences carry explicit relationships. Do not let the last
	// zone visited by the linear renderer leak into those records.
	state.activeZone = semanticEntityDescriptor{}
	state.addProfileGroupEntities()
	state.addHVACDerivedEntities()
	state.addDiagnosticEntities()
	state.finalize(model)
}

func (state *semanticNavigationBuildState) indexPanelTargets() {
	state.geometryByObject = map[int]string{}
	state.geometryByEntity = map[string]string{}
	state.geometryByKindName = map[string][]string{}
	state.hvacBySemanticID = map[string]string{}
	state.hvacPathEntityByTarget = map[string]string{}
	state.hvacCouplingEntityByTarget = map[string]string{}
	state.outputByObject = map[int]string{}
	if state.ctx == nil {
		return
	}
	addGeometry := func(kind string, objectIndex int, name string, targetID string) {
		if targetID == "" {
			return
		}
		state.geometryByObject[objectIndex] = targetID
		state.geometryByKindName[kind+"|"+normalizeName(name)] = appendUniqueString(state.geometryByKindName[kind+"|"+normalizeName(name)], targetID)
		if object, ok := state.objectByIndex(objectIndex); ok {
			state.geometryByEntity[state.objectEntity(object).ID] = targetID
		}
	}
	for _, zone := range state.ctx.geometry.Zones {
		addGeometry("zone", zone.ObjectIndex, zone.Name, zone.ID)
	}
	for _, space := range state.ctx.geometry.Spaces {
		addGeometry("space", space.ObjectIndex, space.Name, space.ID)
	}
	for _, surface := range state.ctx.geometry.Surfaces {
		addGeometry("surface", surface.ObjectIndex, surface.Name, surface.ID)
	}
	for _, window := range state.ctx.geometry.Windows {
		addGeometry("fenestration", window.ObjectIndex, window.Name, window.ID)
	}
	for _, loop := range state.ctx.hvac.Loops {
		state.hvacBySemanticID[semanticHVACLoopEntityID(loop.Type, loop.Name)] = navigationLoopID(loopRefFromLoop(loop))
	}
	for _, entity := range state.ctx.hvac.ServiceModel.Navigation.Entities {
		switch entity.Kind {
		case "loop":
			state.hvacBySemanticID[semanticHVACLoopEntityID(entity.LoopType, entity.LoopName)] = entity.ID
		case "component", "system":
			state.hvacBySemanticID[semanticHVACComponentEntityID(entity.ObjectType, entity.ObjectName)] = entity.ID
		}
	}
	for _, summary := range state.ctx.hvac.ServiceModel.ZoneServices {
		for _, servicePath := range summary.Paths {
			state.hvacPathEntityByTarget[servicePath.ID] = state.stableHVACPathEntityID(servicePath)
		}
	}
	for _, coupling := range state.ctx.hvac.ServiceModel.Couplings {
		state.hvacCouplingEntityByTarget[coupling.ID] = state.stableHVACCouplingEntityID(coupling)
	}
	for _, output := range state.ctx.output.Existing {
		if output.Signature != "" {
			state.outputByObject[output.ObjectIndex] = output.Signature
		}
	}
}

func (state *semanticNavigationBuildState) stableHVACPathEntityID(servicePath ZoneServicePath) string {
	parts := []string{
		state.stableHVACSubjectEntityID(servicePath.ServedSubject),
		normalizeName(servicePath.ServiceKind),
		normalizeName(servicePath.PathType),
		state.semanticComponentEntityForRef(servicePath.Delivery).ID,
	}
	// Match the existing ZoneServicePath identity tuple exactly, replacing only
	// its index-derived component/system segments with stable semantic ones.
	for _, loop := range []*LoopRef{servicePath.PlantLoop, servicePath.AirLoop} {
		if loop != nil {
			parts = append(parts, state.semanticLoopEntityForRef(*loop).ID)
		}
	}
	parts = append(parts, state.stableHVACSystemEntityID(servicePath.RefrigerantSystem))
	stableKey := strings.Join(nonEmptyStrings(parts), "|")
	label := firstNonEmpty(servicePath.ServiceKind, "service") + "-" + semanticStableHash(stableKey, 20)
	return semanticHVACPathEntityID(label)
}

func (state *semanticNavigationBuildState) stableHVACCouplingEntityID(coupling SystemCoupling) string {
	parts := []string{
		normalizeName(coupling.CouplingType),
		state.semanticComponentEntityForRef(coupling.Object).ID,
	}
	stableKey := strings.Join(nonEmptyStrings(parts), "|")
	label := firstNonEmpty(coupling.CouplingType, "coupling") + "-" + semanticStableHash(stableKey, 20)
	return semanticHVACCouplingEntityID(label)
}

func (state *semanticNavigationBuildState) stableHVACSubjectEntityID(subject ServedSubjectRef) string {
	if subject.ObjectIndex >= 0 {
		if object, ok := state.objectByIndex(subject.ObjectIndex); ok {
			matchesKind := (subject.Kind == "space" && strings.EqualFold(object.Type, "Space")) || (subject.Kind != "space" && strings.EqualFold(object.Type, "Zone"))
			if matchesKind && (subject.Name == "" || strings.EqualFold(objectName(object), subject.Name)) {
				return state.objectEntity(object).ID
			}
		}
	}
	if subject.Kind == "space" {
		return "space:" + semanticIDToken(subject.ZoneName) + ":" + semanticIDToken(firstNonEmpty(subject.SpaceName, subject.Name))
	}
	return "zone:" + semanticIDToken(firstNonEmpty(subject.ZoneName, subject.Name))
}

func (state *semanticNavigationBuildState) stableHVACSystemEntityID(system *SystemRef) string {
	if system == nil {
		return ""
	}
	objectType := firstNonEmpty(system.ObjectType, system.Type)
	objectNameValue := firstNonEmpty(system.ObjectName, system.Name)
	if system.ObjectIndex >= 0 && strings.TrimSpace(objectType) != "" {
		if object, ok := state.objectByIndex(system.ObjectIndex); ok && strings.EqualFold(object.Type, objectType) && (objectNameValue == "" || strings.EqualFold(objectName(object), objectNameValue)) {
			return state.objectEntity(object).ID
		}
	}
	return "hvac-system:" + semanticIDToken(objectType) + ":" + semanticIDToken(objectNameValue)
}

func (state *semanticNavigationBuildState) prepareZoneContext(node SemanticYAMLNode, path string) {
	segments := semanticPathSegments(path)
	if len(segments) < 2 || segments[0] != "zones" {
		state.activeZone = semanticEntityDescriptor{}
		return
	}
	if len(segments) != 2 || node.ObjectIndex == nil || node.FieldIndex == nil || *node.FieldIndex != 0 {
		return
	}
	object, ok := state.objectByIndex(*node.ObjectIndex)
	if !ok || !strings.EqualFold(object.Type, "Zone") {
		return
	}
	state.activeZone = state.objectEntity(object)
}

func newSemanticSourceRegistry(doc Document) semanticSourceRegistry {
	type candidate struct {
		object      Object
		baseKey     string
		fingerprint string
	}
	candidates := make([]candidate, 0, len(doc.Objects))
	baseCount := map[string]int{}
	fingerprintCount := map[string]int{}
	for _, object := range doc.Objects {
		typeKey := semanticIDToken(object.Type)
		nameKey := semanticIDToken(objectName(object))
		fingerprint := semanticObjectFingerprint(object)
		baseKey := "named|" + typeKey + "|" + nameKey
		if nameKey == "" {
			baseKey = "anonymous|" + typeKey + "|" + fingerprint
		}
		candidates = append(candidates, candidate{object: object, baseKey: baseKey, fingerprint: fingerprint})
		baseCount[baseKey]++
		fingerprintCount[baseKey+"|"+fingerprint]++
	}

	registry := semanticSourceRegistry{
		byObjectIndex: map[int]string{},
		duplicateName: map[int]bool{},
	}
	for _, candidate := range candidates {
		key := candidate.baseKey
		name := objectName(candidate.object)
		if baseCount[candidate.baseKey] > 1 {
			registry.duplicateName[candidate.object.Index] = strings.TrimSpace(name) != ""
			key += "|content|" + candidate.fingerprint
			if fingerprintCount[candidate.baseKey+"|"+candidate.fingerprint] > 1 {
				// Byte-identical duplicates are semantically indistinguishable. The
				// source index is the explicitly documented final fallback.
				key += fmt.Sprintf("|index|%d", candidate.object.Index)
			}
		}
		registry.byObjectIndex[candidate.object.Index] = "obj-" + semanticStableHash(key, 20)
	}
	return registry
}

func semanticObjectFingerprint(object Object) string {
	var builder strings.Builder
	builder.WriteString(strings.ToLower(strings.TrimSpace(object.Type)))
	for _, field := range object.Fields {
		builder.WriteByte('\x00')
		builder.WriteString(strings.TrimSpace(field.Value))
		builder.WriteByte('\x01')
		builder.WriteString(strings.TrimSpace(field.Comment))
	}
	return semanticStableHash(builder.String(), 20)
}

func semanticProjectKey(doc Document, registry semanticSourceRegistry) string {
	for _, object := range doc.Objects {
		if strings.EqualFold(object.Type, "Building") && strings.TrimSpace(objectName(object)) != "" {
			key := semanticIDToken(objectName(object))
			if registry.duplicateName[object.Index] {
				key += ":source:" + strings.TrimPrefix(registry.byObjectIndex[object.Index], "obj-")
			}
			return key
		}
	}
	objectIDs := make([]string, 0, len(registry.byObjectIndex))
	for _, objectID := range registry.byObjectIndex {
		objectIDs = append(objectIDs, objectID)
	}
	sort.Strings(objectIDs)
	return "model-" + semanticStableHash(strings.Join(objectIDs, "|"), 16)
}

func semanticNodePaths(nodes []SemanticYAMLNode) []string {
	paths := make([]string, len(nodes))
	stack := []semanticPathEntry{}
	for index, node := range nodes {
		label := semanticNodePathLabel(node)
		if label != "" {
			for len(stack) > 0 && stack[len(stack)-1].indent >= node.Indent {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, semanticPathEntry{indent: node.Indent, label: label})
		}
		labels := make([]string, 0, len(stack))
		for _, entry := range stack {
			labels = append(labels, entry.label)
		}
		paths[index] = canonicalSemanticPath(labels)
	}
	return paths
}

func canonicalSemanticPath(labels []string) string {
	if len(labels) > 0 && labels[0] == "semantic_energyplus_model" {
		labels = labels[1:]
	}
	canonical := append([]string(nil), labels...)
	insideZone := len(canonical) >= 2 && canonical[0] == "zones"
	if insideZone {
		canonicalizeZoneSection := func(index int) {
			if index >= len(canonical) {
				return
			}
			switch canonical[index] {
			case "loads", "thermal_mass", "behavior", "air_exchange", "controls":
				canonical[index] = "profiles"
			case "hvac":
				canonical[index] = "services"
			}
		}
		canonicalizeZoneSection(2)
		if len(canonical) >= 4 && canonical[2] == "spaces" {
			canonicalizeZoneSection(4)
		}
	}
	return strings.Join(canonical, "/")
}

func (state *semanticNavigationBuildState) sourceAnchorForNode(node SemanticYAMLNode) *SemanticSourceAnchor {
	if node.ObjectIndex == nil || *node.ObjectIndex < 0 {
		return nil
	}
	object, ok := state.objectByIndex(*node.ObjectIndex)
	if !ok {
		return nil
	}
	fieldName := ""
	if node.FieldIndex != nil && *node.FieldIndex >= 0 && *node.FieldIndex < len(object.Fields) {
		fieldName = strings.TrimSpace(object.Fields[*node.FieldIndex].Comment)
	}
	if fieldName == "" && node.FieldIndex != nil {
		fieldName = strings.TrimSpace(node.Key)
	}
	return &SemanticSourceAnchor{
		ObjectID:    state.registry.byObjectIndex[object.Index],
		ObjectIndex: intPtr(object.Index),
		ObjectType:  object.Type,
		ObjectName:  objectName(object),
		FieldIndex:  cloneIntPtr(node.FieldIndex),
		FieldName:   fieldName,
	}
}

func (state *semanticNavigationBuildState) objectByIndex(index int) (Object, bool) {
	if state.ctx != nil {
		if object, ok := state.ctx.objectByIndex[index]; ok {
			return object, true
		}
	}
	for _, object := range state.doc.Objects {
		if object.Index == index {
			return object, true
		}
	}
	return Object{}, false
}

func (state *semanticNavigationBuildState) entityForNode(node SemanticYAMLNode, path string) semanticEntityDescriptor {
	if referenced, ok := state.referencedEntityForNode(node); ok {
		return referenced
	}
	if node.ObjectIndex != nil {
		if object, ok := state.objectByIndex(*node.ObjectIndex); ok {
			descriptor := state.objectEntity(object)
			context := semanticContextKind(path, descriptor.Kind)
			if descriptor.Kind == "source-object" && context == "zone_profile" {
				dimension := profileDimensionForObject(object.Type)
				if dimension != "" {
					descriptor = semanticEntityDescriptor{
						ID:    semanticProfileItemEntityID(dimension, state.registry.byObjectIndex[object.Index]),
						Kind:  "profile-item",
						Label: firstNonEmpty(objectName(object), object.Type),
					}
				}
			}
			if descriptor.Kind == "source-object" && (context == "zone_service" || strings.HasPrefix(path, "hvac/")) {
				descriptor = state.hvacComponentEntity(object)
				if state.registry.duplicateName[object.Index] {
					descriptor.ID += ":source:" + strings.TrimPrefix(state.registry.byObjectIndex[object.Index], "obj-")
				}
			}
			return descriptor
		}
	}

	segments := semanticPathSegments(path)
	if len(segments) >= 2 && segments[0] == "zones" {
		if zone := state.zoneEntityForPath(path); zone.ID != "" {
			return zone
		}
		// Never merge duplicated zone names into a third, unsuffixed entity.
		return semanticEntityDescriptor{ID: "project:" + state.projectKey + ":section:zones", Kind: "semantic-section", Label: "Zones"}
	}
	if len(segments) >= 2 && segments[0] == "hvac" {
		switch segments[1] {
		case "air_loops", "plant_loops", "condenser_loops":
			if len(segments) >= 3 {
				loopTypes := map[string]string{
					"air_loops":       "AirLoopHVAC",
					"plant_loops":     "PlantLoop",
					"condenser_loops": "CondenserLoop",
				}
				return semanticEntityDescriptor{ID: semanticHVACLoopEntityID(loopTypes[segments[1]], segments[2]), Kind: "hvac-loop", Label: segments[2]}
			}
		}
	}
	if len(segments) > 0 {
		section := semanticIDToken(segments[0])
		return semanticEntityDescriptor{
			ID:    "project:" + state.projectKey + ":section:" + section,
			Kind:  "semantic-section",
			Label: semanticSectionLabel(segments[0]),
		}
	}
	return semanticEntityDescriptor{ID: "project:" + state.projectKey, Kind: "project", Label: "Project"}
}

func (state *semanticNavigationBuildState) referencedEntityForNode(node SemanticYAMLNode) (semanticEntityDescriptor, bool) {
	if node.FieldIndex == nil || strings.TrimSpace(node.DisplayValue) == "" {
		return semanticEntityDescriptor{}, false
	}
	key := strings.ToLower(strings.TrimSpace(node.Key))
	value := strings.TrimSpace(node.DisplayValue)
	var matchesType func(string) bool
	switch key {
	case "schedule", "activity_schedule":
		matchesType = isScheduleType
	case "construction":
		matchesType = func(objectType string) bool {
			return strings.HasPrefix(strings.ToLower(strings.TrimSpace(objectType)), "construction")
		}
	case "zone", "zone_name":
		matchesType = func(objectType string) bool { return strings.EqualFold(objectType, "Zone") }
	case "space", "space_name":
		matchesType = func(objectType string) bool { return strings.EqualFold(objectType, "Space") }
	case "base_surface", "surface":
		matchesType = isBuildingSurfaceType
	case "air_loop":
		matchesType = func(objectType string) bool { return strings.EqualFold(objectType, "AirLoopHVAC") }
	case "plant_loop":
		matchesType = func(objectType string) bool { return strings.EqualFold(objectType, "PlantLoop") }
	default:
		return semanticEntityDescriptor{}, false
	}
	var matches []Object
	for _, object := range state.objectsByName[normalizeName(value)] {
		if matchesType(object.Type) {
			matches = append(matches, object)
		}
	}
	if len(matches) != 1 {
		return semanticEntityDescriptor{}, false
	}
	return state.objectEntity(matches[0]), true
}

func (state *semanticNavigationBuildState) objectEntity(object Object) semanticEntityDescriptor {
	objectType := strings.TrimSpace(object.Type)
	typeKey := semanticIDToken(objectType)
	name := strings.TrimSpace(objectName(object))
	nameKey := semanticIDToken(name)
	objectID := state.registry.byObjectIndex[object.Index]
	descriptor := semanticEntityDescriptor{
		ID:    semanticSourceObjectEntityID(objectID),
		Kind:  "source-object",
		Label: firstNonEmpty(name, objectType),
	}
	lowerType := strings.ToLower(objectType)
	switch {
	case strings.EqualFold(objectType, "Building"):
		descriptor = semanticEntityDescriptor{ID: "building:" + firstNonEmpty(nameKey, objectID), Kind: "building", Label: firstNonEmpty(name, "Building")}
	case strings.EqualFold(objectType, "Zone"):
		descriptor = semanticEntityDescriptor{ID: "zone:" + firstNonEmpty(nameKey, objectID), Kind: "zone", Label: firstNonEmpty(name, "Zone")}
	case strings.EqualFold(objectType, "Space"):
		zoneName := semanticSpaceFromObject(object).ZoneName
		descriptor = semanticEntityDescriptor{ID: "space:" + semanticIDToken(zoneName) + ":" + firstNonEmpty(nameKey, objectID), Kind: "space", Label: firstNonEmpty(name, "Space")}
	case isFenestrationType(objectType):
		descriptor = semanticEntityDescriptor{ID: "fenestration:" + typeKey + ":" + firstNonEmpty(nameKey, objectID), Kind: "fenestration", Label: firstNonEmpty(name, objectType)}
	case isBuildingSurfaceType(objectType):
		descriptor = semanticEntityDescriptor{ID: "surface:" + typeKey + ":" + firstNonEmpty(nameKey, objectID), Kind: "surface", Label: firstNonEmpty(name, objectType)}
	case isScheduleType(objectType):
		descriptor = semanticEntityDescriptor{ID: "schedule:" + typeKey + ":" + firstNonEmpty(nameKey, objectID), Kind: "schedule", Label: firstNonEmpty(name, objectType)}
	case strings.HasPrefix(lowerType, "construction"):
		descriptor = semanticEntityDescriptor{ID: "construction:" + typeKey + ":" + firstNonEmpty(nameKey, objectID), Kind: "construction", Label: firstNonEmpty(name, objectType)}
	case isMaterialType(objectType):
		descriptor = semanticEntityDescriptor{ID: "material:" + typeKey + ":" + firstNonEmpty(nameKey, objectID), Kind: "material", Label: firstNonEmpty(name, objectType)}
	case strings.EqualFold(objectType, "AirLoopHVAC") || strings.EqualFold(objectType, "PlantLoop") || strings.EqualFold(objectType, "CondenserLoop"):
		loopID := semanticHVACLoopEntityID(objectType, name)
		if nameKey == "" {
			loopID = "hvac-loop:" + typeKey + ":" + objectID
		}
		descriptor = semanticEntityDescriptor{ID: loopID, Kind: "hvac-loop", Label: firstNonEmpty(name, objectType)}
	case isOutputManagementType(objectType):
		signature := outputObjectSignature(object.Type, outputFieldValues(object))
		target := semanticIDToken(signature)
		if target == "" {
			target = objectID
		}
		descriptor = semanticEntityDescriptor{ID: "output:" + target, Kind: "output", Label: firstNonEmpty(name, objectLabel(object))}
	case state.isHVACNavigationObject(object):
		descriptor = state.hvacComponentEntity(object)
	}
	if state.registry.duplicateName[object.Index] && descriptor.Kind != "source-object" {
		descriptor.ID += ":source:" + strings.TrimPrefix(objectID, "obj-")
	}
	return descriptor
}

func (state *semanticNavigationBuildState) isHVACNavigationObject(object Object) bool {
	_, ok := state.hvacByObject[object.Index]
	return ok
}

func (state *semanticNavigationBuildState) hvacComponentEntity(object Object) semanticEntityDescriptor {
	name := firstNonEmpty(objectName(object), object.Type)
	entityID := semanticHVACComponentEntityID(object.Type, objectName(object))
	if strings.TrimSpace(objectName(object)) == "" {
		entityID = "hvac-component:" + semanticIDToken(object.Type) + ":" + state.registry.byObjectIndex[object.Index]
	}
	return semanticEntityDescriptor{
		ID:    entityID,
		Kind:  "hvac-component",
		Label: name,
	}
}

func (state *semanticNavigationBuildState) viewTargetsFor(descriptor semanticEntityDescriptor, node SemanticYAMLNode, path string) []SemanticViewTarget {
	var targets []SemanticViewTarget
	add := func(target SemanticViewTarget) {
		if target.View == "" || target.TargetID == "" {
			return
		}
		for _, existing := range targets {
			if existing.View == target.View && existing.TargetID == target.TargetID {
				return
			}
		}
		targets = append(targets, target)
	}

	switch descriptor.Kind {
	case "project", "building", "semantic-section":
		section := firstNonEmpty(semanticTopLevelSection(path), "project")
		add(SemanticViewTarget{View: "summary", TargetKind: "category", TargetID: semanticSummaryCategory(section), Label: descriptor.Label, Priority: 80})
		if section == "hvac" {
			add(SemanticViewTarget{View: "hvac", TargetKind: "section", TargetID: "hvac", Label: "HVAC", Priority: 70})
		}
		if section == "outputs" {
			add(SemanticViewTarget{View: "output", TargetKind: "section", TargetID: "outputs", Label: "Output", Priority: 70})
		}
		if section == "source_name_conflicts" {
			add(SemanticViewTarget{View: "diagnose", TargetKind: "group", TargetID: "source-name-conflicts", Label: "Diagnostics", Priority: 70})
		}
	case "zone":
		add(SemanticViewTarget{View: "geometry", TargetKind: "zone", TargetID: state.geometryPanelTarget("zone", node, descriptor), Label: descriptor.Label, Priority: 80})
		add(SemanticViewTarget{View: "profile", TargetKind: "zone", TargetID: descriptor.Label, Label: descriptor.Label, Priority: 75})
		if semanticContextKind(path, descriptor.Kind) == "zone_profile" {
			if dimension := semanticProfileDimension(path); dimension != "" {
				add(SemanticViewTarget{View: "profile", TargetKind: "zone-dimension", TargetID: semanticProfileZoneDimensionTargetID(descriptor.Label, dimension), Label: profileDimensionLabel(dimension), Priority: 95})
			}
		}
		for _, servicePath := range state.servicePathsForZone(descriptor.Label) {
			add(SemanticViewTarget{View: "hvac", TargetKind: "service-path", TargetID: servicePath.ID, Label: firstNonEmpty(servicePath.ServiceKind, descriptor.Label), Priority: 90})
		}
		if state.zoneHasOutputs(descriptor.Label) {
			add(SemanticViewTarget{View: "output", TargetKind: "zone", TargetID: descriptor.Label, Label: descriptor.Label, Priority: 60})
		}
	case "space":
		add(SemanticViewTarget{View: "geometry", TargetKind: "space", TargetID: state.geometryPanelTarget("space", node, descriptor), Label: descriptor.Label, Priority: 80})
	case "surface", "fenestration":
		add(SemanticViewTarget{View: "geometry", TargetKind: descriptor.Kind, TargetID: state.geometryPanelTarget(descriptor.Kind, node, descriptor), Label: descriptor.Label, Priority: 100})
	case "schedule":
		add(SemanticViewTarget{View: "profile", TargetKind: descriptor.Kind, TargetID: descriptor.Label, Label: descriptor.Label, Priority: 100})
	case "profile-item":
		add(SemanticViewTarget{View: "profile", TargetKind: descriptor.Kind, TargetID: state.profilePanelTarget(node, path, descriptor), Label: descriptor.Label, Priority: 100})
	case "profile-group":
		add(SemanticViewTarget{View: "profile", TargetKind: descriptor.Kind, TargetID: descriptor.ID, Label: descriptor.Label, Priority: 100})
	case "hvac-path", "hvac-loop", "hvac-component", "hvac-coupling", "hvac-network":
		targetID := state.hvacPanelTarget(node, descriptor)
		add(SemanticViewTarget{View: "hvac", TargetKind: descriptor.Kind, TargetID: targetID, Label: descriptor.Label, Priority: 100})
	case "output":
		add(SemanticViewTarget{View: "output", TargetKind: "request", TargetID: state.outputPanelTarget(node, descriptor), Label: descriptor.Label, Priority: 100})
	case "diagnostic":
		add(SemanticViewTarget{View: "diagnose", TargetKind: "diagnostic", TargetID: descriptor.ID, Label: descriptor.Label, Priority: 100})
	case "construction", "material":
		add(SemanticViewTarget{View: "geometry", TargetKind: descriptor.Kind, TargetID: descriptor.Label, Label: descriptor.Label, Priority: 70})
	case "source-object":
		if section := semanticTopLevelSection(path); section == "site" {
			add(SemanticViewTarget{View: "summary", TargetKind: "category", TargetID: semanticSummaryCategory(section), Label: descriptor.Label, Priority: 70})
		}
	}
	if node.SourceAnchor != nil {
		add(SemanticViewTarget{View: "input-text", TargetKind: "source", TargetID: node.SourceAnchor.ObjectID, Label: "Source", Priority: 20})
	}
	return targets
}

func (state *semanticNavigationBuildState) servicePathsForZone(zoneName string) []ZoneServicePath {
	return state.serviceByZone[normalizeName(zoneName)]
}

func (state *semanticNavigationBuildState) zoneHasOutputs(zoneName string) bool {
	if state.ctx == nil {
		return false
	}
	return len(state.ctx.outputsByTarget[normalizeName(zoneName)]) > 0 || len(state.ctx.wildcardOutputs) > 0
}

func (state *semanticNavigationBuildState) hvacPanelTarget(node SemanticYAMLNode, descriptor semanticEntityDescriptor) string {
	if node.ObjectIndex != nil {
		if entity, ok := state.hvacByObject[*node.ObjectIndex]; ok && entity.ID != "" {
			return entity.ID
		}
	}
	if targetID := state.hvacBySemanticID[descriptor.ID]; targetID != "" {
		return targetID
	}
	return ""
}

func (state *semanticNavigationBuildState) geometryPanelTarget(kind string, node SemanticYAMLNode, descriptor semanticEntityDescriptor) string {
	if state.ctx == nil {
		return descriptor.ID
	}
	if targetID := state.geometryByEntity[descriptor.ID]; targetID != "" {
		return targetID
	}
	if node.SourceAnchor != nil && node.SourceAnchor.ObjectIndex != nil && semanticAnchorMatchesGeometryKind(node.SourceAnchor.ObjectType, kind) {
		if targetID := state.geometryByObject[*node.SourceAnchor.ObjectIndex]; targetID != "" {
			return targetID
		}
	}
	if targetIDs := state.geometryByKindName[kind+"|"+normalizeName(descriptor.Label)]; len(targetIDs) == 1 {
		return targetIDs[0]
	}
	return descriptor.ID
}

func semanticAnchorMatchesGeometryKind(objectType string, kind string) bool {
	switch kind {
	case "zone":
		return strings.EqualFold(objectType, "Zone")
	case "space":
		return strings.EqualFold(objectType, "Space")
	case "surface":
		return isBuildingSurfaceType(objectType)
	case "fenestration":
		return isFenestrationType(objectType)
	default:
		return false
	}
}

func (state *semanticNavigationBuildState) profilePanelTarget(node SemanticYAMLNode, path string, descriptor semanticEntityDescriptor) string {
	if node.SourceAnchor != nil && node.SourceAnchor.ObjectIndex != nil {
		dimension := profileDimensionForObject(node.SourceAnchor.ObjectType)
		key := semanticProfileSourceKey(*node.SourceAnchor.ObjectIndex, semanticZoneName(path), dimension)
		if targetID := state.profileBySource[key]; targetID != "" {
			return targetID
		}
	}
	return descriptor.ID
}

func (state *semanticNavigationBuildState) outputPanelTarget(node SemanticYAMLNode, descriptor semanticEntityDescriptor) string {
	if node.SourceAnchor != nil && node.SourceAnchor.ObjectIndex != nil {
		if targetID := state.outputByObject[*node.SourceAnchor.ObjectIndex]; targetID != "" {
			return targetID
		}
	}
	return descriptor.ID
}

func semanticPreferredTarget(path string, descriptor semanticEntityDescriptor, targets []SemanticViewTarget) (string, string) {
	context := semanticContextKind(path, descriptor.Kind)
	preferredView := ""
	switch context {
	case "zone_geometry":
		preferredView = "geometry"
	case "zone_profile":
		preferredView = "profile"
	case "zone_service", "system_definition", "loop_occurrence", "component_occurrence", "coupling_occurrence":
		preferredView = "hvac"
	case "zone_output", "output_request":
		preferredView = "output"
	case "zone_diagnostic", "diagnostic_occurrence":
		preferredView = "diagnose"
	case "source_only":
		preferredView = "input-text"
	}
	if preferredView == "" {
		if descriptor.Kind == "source-object" && semanticTopLevelSection(path) == "site" {
			preferredView = "summary"
		}
	}
	if preferredView == "" {
		switch descriptor.Kind {
		case "surface", "fenestration", "space":
			preferredView = "geometry"
		case "schedule", "profile-item", "profile-group":
			preferredView = "profile"
		case "hvac-path", "hvac-loop", "hvac-component", "hvac-coupling", "hvac-network":
			preferredView = "hvac"
		case "output":
			preferredView = "output"
		case "diagnostic":
			preferredView = "diagnose"
		case "project", "building", "semantic-section":
			preferredView = "summary"
		}
	}
	// A zone definition intentionally advertises several lenses without
	// forcing one of them as its preferred panel.
	if descriptor.Kind == "zone" && context == "definition" {
		preferredView = ""
	}
	bestID := ""
	bestPriority := -1
	bestCount := 0
	for _, target := range targets {
		if target.View != preferredView {
			continue
		}
		if target.Priority > bestPriority {
			bestPriority = target.Priority
			bestID = target.TargetID
			bestCount = 1
		} else if target.Priority == bestPriority {
			bestCount++
		}
	}
	// Equal-priority targets represent an intentionally ambiguous occurrence
	// (for example, a zone served by both heating and cooling paths). The
	// controller opens a chooser instead of silently following list order.
	if bestCount > 1 {
		bestID = ""
	}
	return preferredView, bestID
}

func semanticContextKind(path string, entityKind string) string {
	segments := semanticPathSegments(path)
	if len(segments) >= 2 && segments[0] == "zones" {
		sectionIndex := 2
		if len(segments) > 2 && segments[2] == "spaces" {
			sectionIndex = 4
		}
		if sectionIndex < len(segments) {
			switch segments[sectionIndex] {
			case "geometry":
				return "zone_geometry"
			case "profiles":
				return "zone_profile"
			case "services":
				return "zone_service"
			case "outputs", "inherited_outputs":
				return "zone_output"
			case "diagnostics":
				return "zone_diagnostic"
			}
		}
		return "definition"
	}
	if len(segments) > 0 && segments[0] == "hvac" {
		switch entityKind {
		case "hvac-loop":
			return "loop_occurrence"
		case "hvac-component":
			return "component_occurrence"
		case "hvac-coupling":
			return "coupling_occurrence"
		default:
			return "system_definition"
		}
	}
	if len(segments) > 0 && segments[0] == "outputs" {
		return "output_request"
	}
	if len(segments) > 0 && (segments[0] == "diagnostics" || segments[0] == "source_name_conflicts") {
		return "diagnostic_occurrence"
	}
	if len(segments) > 0 && segments[0] == "source_preservation" {
		return "source_only"
	}
	return "definition"
}

func semanticOccurrenceRootPath(path string, descriptor semanticEntityDescriptor, anchor *SemanticSourceAnchor) string {
	segments := semanticPathSegments(path)
	if len(segments) == 0 {
		return path
	}
	if len(segments) >= 2 && segments[0] == "zones" {
		contextIndex := 2
		if len(segments) > 2 && segments[2] == "spaces" {
			contextIndex = 4
		}
		if contextIndex < len(segments) {
			segment := segments[contextIndex]
			if segment == "geometry" || segment == "profiles" || segment == "services" || segment == "outputs" || segment == "inherited_outputs" || segment == "diagnostics" {
				if anchor == nil || descriptor.Kind == "zone" || descriptor.Kind == "space" {
					return strings.Join(segments[:contextIndex+1], "/")
				}
			}
		}
	}
	if anchor != nil {
		objectName := strings.TrimSpace(anchor.ObjectName)
		if objectName != "" {
			for index := len(segments) - 1; index >= 0; index-- {
				if strings.EqualFold(strings.Trim(segments[index], `"`), objectName) {
					return strings.Join(segments[:index+1], "/")
				}
			}
		}
	}
	return path
}

func semanticProfileDimension(path string) string {
	segments := semanticPathSegments(path)
	sectionIndex := 2
	if len(segments) > 2 && segments[0] == "zones" && segments[2] == "spaces" {
		sectionIndex = 4
	}
	if sectionIndex < len(segments) && segments[sectionIndex] == "profiles" && sectionIndex+1 < len(segments) {
		switch segments[sectionIndex+1] {
		case "people", ProfileDimensionOccupancy:
			return ProfileDimensionOccupancy
		case "lights", ProfileDimensionLighting:
			return ProfileDimensionLighting
		case "electric_equipment", "gas_equipment", "other_equipment", ProfileDimensionEquipment:
			return ProfileDimensionEquipment
		case ProfileDimensionInfiltration:
			return ProfileDimensionInfiltration
		case ProfileDimensionVentilation:
			return ProfileDimensionVentilation
		case "outdoor_air_specs", ProfileDimensionOutdoorAir:
			return ProfileDimensionOutdoorAir
		}
	}
	return ""
}

func semanticZoneName(path string) string {
	segments := semanticPathSegments(path)
	if len(segments) >= 2 && segments[0] == "zones" {
		return segments[1]
	}
	return ""
}

func semanticRoleForCanonicalPath(path string) string {
	switch semanticContextKind(path, "") {
	case "zone_geometry":
		return "zone_geometry"
	case "zone_profile":
		return "zone_profile"
	case "zone_service":
		return "zone_service"
	case "zone_output":
		return "zone_output"
	case "zone_diagnostic":
		return "zone_diagnostic"
	case "output_request":
		return "output_request"
	case "diagnostic_occurrence":
		return "diagnostic"
	case "source_only":
		return "source_only"
	default:
		return "definition"
	}
}

func (state *semanticNavigationBuildState) addProfileGroupEntities() {
	if state.ctx == nil {
		return
	}
	itemsByID := map[string]ProfileItem{}
	for _, zone := range state.ctx.profile.ZoneProfiles {
		for _, item := range zone.Items {
			itemsByID[item.ID] = item
		}
	}
	for _, group := range state.ctx.profile.Groups {
		stableKey := firstNonEmpty(group.Key, group.Name, group.ID)
		descriptor := semanticEntityDescriptor{
			ID:    semanticProfileGroupEntityID(stableKey),
			Kind:  "profile-group",
			Label: firstNonEmpty(group.Name, "Profile group"),
		}
		target := SemanticViewTarget{View: "profile", TargetKind: "profile-group", TargetID: group.ID, Label: descriptor.Label, Priority: 100}
		added := false
		for _, zoneName := range group.ZoneNames {
			var anchor *SemanticSourceAnchor
			for _, itemID := range group.ItemIDs {
				item, ok := itemsByID[itemID]
				if !ok || !strings.EqualFold(item.ZoneName, zoneName) {
					continue
				}
				anchor = state.sourceAnchorForObjectField(item.ObjectIndex, nil)
				break
			}
			zone := state.zoneEntityForPath("zones/" + zoneName)
			base := state.bestOccurrenceForEntity(zone.ID, "zone_profile")
			lineIndexes := []int(nil)
			if base != nil {
				lineIndexes = append(lineIndexes, base.LineIndexes...)
			}
			path := "zones/" + zoneName + "/profiles/groups/" + semanticIDToken(stableKey)
			related := []string(nil)
			if zone.ID != "" {
				related = append(related, zone.ID)
			}
			state.addSyntheticOccurrence(descriptor, path, "zone_profile", anchor, []SemanticViewTarget{target}, lineIndexes, related, "profile")
			added = true
		}
		if !added {
			state.addSyntheticOccurrence(descriptor, "profiles/groups/"+semanticIDToken(stableKey), "definition", nil, []SemanticViewTarget{target}, nil, nil, "profile")
		}
		entity := state.ensureEntity(descriptor)
		for _, itemID := range group.ItemIDs {
			item, ok := itemsByID[itemID]
			if !ok {
				continue
			}
			if anchor := state.sourceAnchorForObjectField(item.ObjectIndex, nil); anchor != nil {
				entity.SourceAnchors = appendUniqueSemanticSourceAnchor(entity.SourceAnchors, *anchor)
			}
		}
	}
}

func (state *semanticNavigationBuildState) addHVACDerivedEntities() {
	if state.ctx == nil {
		return
	}
	state.addHVACPathEntities()
	navigation := state.ctx.hvac.ServiceModel.Navigation
	for _, coupling := range state.ctx.hvac.ServiceModel.Couplings {
		entityID := state.hvacCouplingEntityByTarget[coupling.ID]
		if entityID == "" {
			entityID = semanticHVACCouplingEntityID(coupling.ID)
		}
		descriptor := semanticEntityDescriptor{
			ID:    entityID,
			Kind:  "hvac-coupling",
			Label: firstNonEmpty(coupling.Object.DisplayName, coupling.Object.ObjectName, coupling.Role, coupling.ID),
		}
		panelTargetID := navigationCouplingID(coupling.ID)
		target := SemanticViewTarget{View: "hvac", TargetKind: "hvac-coupling", TargetID: panelTargetID, Label: descriptor.Label, Priority: 100}
		anchor := state.sourceAnchorForHVACComponent(coupling.Object)
		var pathIDs []string
		for _, entity := range navigation.Entities {
			if entity.ID == panelTargetID {
				pathIDs = append(pathIDs, entity.RelatedPathIDs...)
			}
		}
		if len(pathIDs) == 0 {
			state.addSyntheticOccurrence(descriptor, "hvac/couplings/"+semanticIDToken(coupling.ID), "coupling_occurrence", anchor, []SemanticViewTarget{target}, state.lineIndexesForAnchor(anchor), nil, "hvac")
			continue
		}
		for _, pathID := range pathIDs {
			pathEntityID := state.hvacPathEntityByTarget[pathID]
			if pathEntityID == "" {
				pathEntityID = semanticHVACPathEntityID(pathID)
			}
			lineIndexes := state.lineIndexesForViewTarget("hvac", pathID)
			state.addSyntheticOccurrence(
				descriptor,
				"hvac/service-paths/"+semanticIDToken(pathID)+"/couplings/"+semanticIDToken(coupling.ID),
				"coupling_occurrence",
				anchor,
				[]SemanticViewTarget{target},
				lineIndexes,
				[]string{pathEntityID},
				"hvac",
			)
		}
	}

	for _, network := range state.ctx.hvac.ServiceModel.Networks {
		descriptor := semanticEntityDescriptor{
			ID:    semanticHVACNetworkEntityID(network.NetworkType, network.Name),
			Kind:  "hvac-network",
			Label: firstNonEmpty(network.Name, network.NetworkType),
		}
		target := SemanticViewTarget{View: "hvac", TargetKind: "hvac-network", TargetID: navigationNetworkID(network), Label: descriptor.Label, Priority: 100}
		var anchor *SemanticSourceAnchor
		var related []string
		for _, component := range network.Components {
			if anchor == nil {
				anchor = state.sourceAnchorForHVACComponent(component)
			}
			if component.ObjectIndex >= 0 {
				if object, ok := state.objectByIndex(component.ObjectIndex); ok {
					related = appendUniqueString(related, state.objectEntity(object).ID)
				}
			}
		}
		for _, couplingID := range network.CouplingIDs {
			couplingEntityID := state.hvacCouplingEntityByTarget[couplingID]
			if couplingEntityID == "" {
				couplingEntityID = semanticHVACCouplingEntityID(couplingID)
			}
			related = appendUniqueString(related, couplingEntityID)
		}
		state.addSyntheticOccurrence(
			descriptor,
			"hvac/networks/"+semanticIDToken(network.NetworkType)+"/"+semanticIDToken(network.Name),
			"system_definition",
			anchor,
			[]SemanticViewTarget{target},
			state.lineIndexesForAnchor(anchor),
			related,
			"hvac",
		)
	}
}

func (state *semanticNavigationBuildState) addHVACPathEntities() {
	for _, summary := range state.ctx.hvac.ServiceModel.ZoneServices {
		zoneName := firstNonEmpty(summary.ZoneName, summary.ServedSubject.ZoneName)
		for _, servicePath := range summary.Paths {
			entityID := state.hvacPathEntityByTarget[servicePath.ID]
			if entityID == "" {
				entityID = semanticHVACPathEntityID(servicePath.ID)
			}
			descriptor := semanticEntityDescriptor{
				ID:    entityID,
				Kind:  "hvac-path",
				Label: firstNonEmpty(servicePath.ServiceKind, servicePath.PathType, zoneName, "HVAC service path"),
			}
			target := SemanticViewTarget{View: "hvac", TargetKind: "service-path", TargetID: servicePath.ID, Label: descriptor.Label, Priority: 100}
			anchor := state.sourceAnchorForHVACComponent(servicePath.Delivery)
			if anchor == nil && servicePath.SourceSystem != nil {
				anchor = state.sourceAnchorForHVACComponent(ComponentRef{ObjectIndex: servicePath.SourceSystem.ObjectIndex, ObjectType: firstNonEmpty(servicePath.SourceSystem.ObjectType, servicePath.SourceSystem.Type), ObjectName: firstNonEmpty(servicePath.SourceSystem.ObjectName, servicePath.SourceSystem.Name)})
			}
			var related []string
			zone := semanticEntityDescriptor{}
			if servicePath.ServedSubject.ObjectIndex >= 0 {
				if object, ok := state.objectByIndex(servicePath.ServedSubject.ObjectIndex); ok {
					zone = state.objectEntity(object)
				}
			}
			if zone.ID == "" {
				zone = state.zoneEntityForPath("zones/" + zoneName)
			}
			if zone.ID != "" {
				related = append(related, zone.ID)
			}
			for _, loop := range []*LoopRef{servicePath.AirLoop, servicePath.PlantLoop, servicePath.CondenserLoop} {
				if loop != nil && strings.TrimSpace(loop.Name) != "" {
					related = appendUniqueString(related, state.semanticLoopEntityForRef(*loop).ID)
				}
			}
			components := append([]ComponentRef{servicePath.Delivery}, servicePath.Conditioning...)
			if servicePath.DeliveryWrapper != nil {
				components = append(components, *servicePath.DeliveryWrapper)
			}
			for _, component := range components {
				if componentEntity := state.semanticComponentEntityForRef(component); componentEntity.ID != "" {
					related = appendUniqueString(related, componentEntity.ID)
				}
			}
			for _, couplingID := range servicePath.SupportingCouplings {
				couplingEntityID := state.hvacCouplingEntityByTarget[couplingID]
				if couplingEntityID == "" {
					couplingEntityID = semanticHVACCouplingEntityID(couplingID)
				}
				related = appendUniqueString(related, couplingEntityID)
			}
			path := "zones/" + zoneName + "/services/" + firstNonEmpty(servicePath.ServiceKind, "unknown") + "/" + semanticIDToken(servicePath.ID)
			lineIndexes := state.lineIndexesForAnchor(anchor)
			if len(lineIndexes) == 0 {
				lineIndexes = state.lineIndexesForViewTarget("hvac", servicePath.ID)
			}
			state.addSyntheticOccurrence(descriptor, path, "zone_service", anchor, []SemanticViewTarget{target}, lineIndexes, related, "hvac")
		}
	}
}

func (state *semanticNavigationBuildState) semanticLoopEntityForRef(loop LoopRef) semanticEntityDescriptor {
	if loop.ObjectIndex >= 0 {
		if object, ok := state.objectByIndex(loop.ObjectIndex); ok && strings.EqualFold(object.Type, loop.Type) {
			return state.objectEntity(object)
		}
	}
	return semanticEntityDescriptor{ID: semanticHVACLoopEntityID(loop.Type, loop.Name), Kind: "hvac-loop", Label: firstNonEmpty(loop.Name, loop.Type)}
}

func (state *semanticNavigationBuildState) semanticComponentEntityForRef(component ComponentRef) semanticEntityDescriptor {
	if component.ObjectIndex >= 0 && strings.TrimSpace(component.ObjectType) != "" {
		if object, ok := state.objectByIndex(component.ObjectIndex); ok && strings.EqualFold(object.Type, component.ObjectType) && (component.ObjectName == "" || strings.EqualFold(objectName(object), component.ObjectName)) {
			return state.objectEntity(object)
		}
	}
	if strings.TrimSpace(component.ObjectType) == "" {
		return semanticEntityDescriptor{}
	}
	return semanticEntityDescriptor{
		ID:    semanticHVACComponentEntityID(component.ObjectType, component.ObjectName),
		Kind:  "hvac-component",
		Label: firstNonEmpty(component.DisplayName, component.ObjectName, component.ObjectType),
	}
}

func (state *semanticNavigationBuildState) addDiagnosticEntities() {
	if state.ctx == nil {
		return
	}
	for _, diagnostic := range state.ctx.diagnostics {
		diagnosticID := firstNonEmpty(diagnostic.ID, semanticDiagnosticEntityID(diagnostic))
		descriptor := semanticEntityDescriptor{
			ID:    diagnosticID,
			Kind:  "diagnostic",
			Label: firstNonEmpty(diagnostic.Category, diagnostic.Code, "Diagnostic"),
		}
		anchor := state.sourceAnchorForDiagnostic(diagnostic)
		var base *SemanticOccurrence
		if anchor != nil {
			base = state.bestOccurrenceForObject(diagnostic.ObjectIndex)
		}
		path := "diagnostics/" + semanticIDToken(diagnostic.Code)
		lineIndexes := []int(nil)
		var related []string
		if base != nil {
			path = base.Path + "/diagnostics/" + semanticIDToken(diagnostic.Code)
			lineIndexes = append(lineIndexes, base.LineIndexes...)
			related = append(related, base.EntityID)
		}
		target := SemanticViewTarget{View: "diagnose", TargetKind: "diagnostic", TargetID: diagnosticID, Label: descriptor.Label, Priority: 100}
		state.addSyntheticOccurrence(descriptor, path, "diagnostic_occurrence", anchor, []SemanticViewTarget{target}, lineIndexes, related, "diagnose")
	}
}

func (state *semanticNavigationBuildState) addSyntheticOccurrence(
	descriptor semanticEntityDescriptor,
	path string,
	contextKind string,
	anchor *SemanticSourceAnchor,
	targets []SemanticViewTarget,
	lineIndexes []int,
	relatedEntityIDs []string,
	preferredView string,
) {
	sourceObjectID := ""
	if anchor != nil {
		sourceObjectID = anchor.ObjectID
	}
	occurrenceID := "occ-" + semanticStableHash(strings.Join([]string{descriptor.ID, contextKind, path, sourceObjectID}, "\x00"), 20)
	preferredTargetID := ""
	bestPriority := -1
	for _, target := range targets {
		if target.View == preferredView && target.Priority > bestPriority {
			preferredTargetID = target.TargetID
			bestPriority = target.Priority
		}
	}
	occurrence := SemanticOccurrence{
		OccurrenceID:      occurrenceID,
		EntityID:          descriptor.ID,
		SourceObjectID:    sourceObjectID,
		Path:              path,
		RoleHere:          semanticRoleForCanonicalPath(path),
		ContextKind:       contextKind,
		PreferredView:     preferredView,
		PreferredTargetID: preferredTargetID,
		Name:              descriptor.Label,
		SourceAnchor:      cloneSemanticSourceAnchor(anchor),
		ViewTargets:       append([]SemanticViewTarget(nil), targets...),
		LineIndexes:       append([]int(nil), lineIndexes...),
	}
	state.addOccurrence(occurrence)
	state.addEntityLine(descriptor, occurrence, SemanticYAMLNode{SourceAnchor: anchor, ViewTargets: targets}, path)
	entity := state.ensureEntity(descriptor)
	entity.RelatedEntityIDs = appendUniqueStrings(entity.RelatedEntityIDs, relatedEntityIDs...)
	for _, relatedEntityID := range relatedEntityIDs {
		if relatedIndex, ok := state.entityIndex[relatedEntityID]; ok {
			state.entities[relatedIndex].RelatedEntityIDs = appendUniqueString(state.entities[relatedIndex].RelatedEntityIDs, descriptor.ID)
		}
	}
}

func (state *semanticNavigationBuildState) sourceAnchorForObjectField(objectIndex int, fieldIndex *int) *SemanticSourceAnchor {
	return state.sourceAnchorForNode(SemanticYAMLNode{ObjectIndex: intPtr(objectIndex), FieldIndex: cloneIntPtr(fieldIndex)})
}

func (state *semanticNavigationBuildState) sourceAnchorForHVACComponent(component ComponentRef) *SemanticSourceAnchor {
	if component.ObjectIndex < 0 {
		return nil
	}
	object, ok := state.objectByIndex(component.ObjectIndex)
	if !ok || (component.ObjectType != "" && !strings.EqualFold(component.ObjectType, object.Type)) {
		return nil
	}
	return state.sourceAnchorForObjectField(component.ObjectIndex, nil)
}

func (state *semanticNavigationBuildState) sourceAnchorForDiagnostic(diagnostic Diagnostic) *SemanticSourceAnchor {
	if diagnostic.ObjectType == "" && diagnostic.ObjectName == "" && diagnostic.Field == "" {
		return nil
	}
	object, ok := state.objectByIndex(diagnostic.ObjectIndex)
	if !ok {
		return nil
	}
	var fieldIndex *int
	if diagnostic.FieldIndex >= 0 && diagnostic.FieldIndex < len(object.Fields) && (diagnostic.Field != "" || diagnostic.FieldIndex > 0) {
		fieldIndex = intPtr(diagnostic.FieldIndex)
	}
	return state.sourceAnchorForObjectField(diagnostic.ObjectIndex, fieldIndex)
}

func (state *semanticNavigationBuildState) bestOccurrenceForEntity(entityID string, preferredContext string) *SemanticOccurrence {
	var fallback *SemanticOccurrence
	for _, occurrenceID := range state.byEntityID[entityID] {
		index, ok := state.occurrenceIndex[occurrenceID]
		if !ok {
			continue
		}
		occurrence := &state.occurrences[index]
		if occurrence.ContextKind == preferredContext {
			return occurrence
		}
		if fallback == nil {
			fallback = occurrence
		}
	}
	return fallback
}

func (state *semanticNavigationBuildState) bestOccurrenceForObject(objectIndex int) *SemanticOccurrence {
	priorities := map[string]int{
		"zone_service":  6,
		"zone_profile":  5,
		"zone_geometry": 4,
		"zone_output":   3,
		"definition":    2,
		"source_only":   1,
	}
	var best *SemanticOccurrence
	bestPriority := -1
	for _, occurrenceID := range state.byObjectIndex[objectIndex] {
		index, ok := state.occurrenceIndex[occurrenceID]
		if !ok {
			continue
		}
		occurrence := &state.occurrences[index]
		priority := priorities[occurrence.ContextKind]
		if best == nil || priority > bestPriority {
			best = occurrence
			bestPriority = priority
		}
	}
	return best
}

func (state *semanticNavigationBuildState) lineIndexesForAnchor(anchor *SemanticSourceAnchor) []int {
	if anchor == nil || anchor.ObjectIndex == nil {
		return nil
	}
	base := state.bestOccurrenceForObject(*anchor.ObjectIndex)
	if base == nil {
		return nil
	}
	return append([]int(nil), base.LineIndexes...)
}

func (state *semanticNavigationBuildState) lineIndexesForViewTarget(view string, targetID string) []int {
	var indexes []int
	for _, occurrenceID := range state.byViewTarget[semanticViewTargetIndexKey(view, targetID)] {
		index, ok := state.occurrenceIndex[occurrenceID]
		if ok {
			indexes = appendUniqueInt(indexes, state.occurrences[index].LineIndexes...)
		}
	}
	return indexes
}

func (state *semanticNavigationBuildState) addOccurrence(candidate SemanticOccurrence) {
	if index, ok := state.occurrenceIndex[candidate.OccurrenceID]; ok {
		occurrence := &state.occurrences[index]
		occurrence.LineIndexes = appendUniqueInt(occurrence.LineIndexes, candidate.LineIndexes...)
		occurrence.ViewTargets = appendUniqueSemanticViewTargets(occurrence.ViewTargets, candidate.ViewTargets...)
		if occurrence.SourceAnchor == nil && candidate.SourceAnchor != nil {
			occurrence.SourceAnchor = cloneSemanticSourceAnchor(candidate.SourceAnchor)
		}
		return
	}
	state.occurrenceIndex[candidate.OccurrenceID] = len(state.occurrences)
	state.occurrences = append(state.occurrences, candidate)
}

func (state *semanticNavigationBuildState) addEntityLine(descriptor semanticEntityDescriptor, occurrence SemanticOccurrence, node SemanticYAMLNode, path string) {
	entity := state.ensureEntity(descriptor)
	entity.OccurrenceIDs = appendUniqueString(entity.OccurrenceIDs, occurrence.OccurrenceID)
	entity.ViewTargets = appendUniqueSemanticViewTargets(entity.ViewTargets, node.ViewTargets...)
	if node.SourceAnchor != nil {
		entity.SourceAnchors = appendUniqueSemanticSourceAnchor(entity.SourceAnchors, *node.SourceAnchor)
		state.byObjectID[node.SourceAnchor.ObjectID] = appendUniqueString(state.byObjectID[node.SourceAnchor.ObjectID], occurrence.OccurrenceID)
		if node.SourceAnchor.ObjectIndex != nil {
			state.byObjectIndex[*node.SourceAnchor.ObjectIndex] = appendUniqueString(state.byObjectIndex[*node.SourceAnchor.ObjectIndex], occurrence.OccurrenceID)
		}
		sourceEntityID := semanticSourceObjectEntityID(node.SourceAnchor.ObjectID)
		if sourceEntityID != descriptor.ID {
			entity.RelatedEntityIDs = appendUniqueString(entity.RelatedEntityIDs, sourceEntityID)
			sourceEntity := state.ensureEntity(semanticEntityDescriptor{ID: sourceEntityID, Kind: "source-object", Label: firstNonEmpty(node.SourceAnchor.ObjectName, node.SourceAnchor.ObjectType)})
			sourceEntity.SourceAnchors = appendUniqueSemanticSourceAnchor(sourceEntity.SourceAnchors, *node.SourceAnchor)
			sourceEntity.RelatedEntityIDs = appendUniqueString(sourceEntity.RelatedEntityIDs, descriptor.ID)
			sourceEntity.ViewTargets = appendUniqueSemanticViewTargets(sourceEntity.ViewTargets, SemanticViewTarget{View: "input-text", TargetKind: "source", TargetID: node.SourceAnchor.ObjectID, Label: "Source", Priority: 100})
		}
	}
	if zone := state.zoneEntityForPath(path); zone.ID != "" && zone.ID != descriptor.ID {
		// ensureEntity(source) above may have grown the backing slice.
		entity = state.ensureEntity(descriptor)
		entity.RelatedEntityIDs = appendUniqueString(entity.RelatedEntityIDs, zone.ID)
		zoneEntity := state.ensureEntity(zone)
		zoneEntity.RelatedEntityIDs = appendUniqueString(zoneEntity.RelatedEntityIDs, descriptor.ID)
	}
	state.byEntityID[descriptor.ID] = appendUniqueString(state.byEntityID[descriptor.ID], occurrence.OccurrenceID)
	for _, target := range node.ViewTargets {
		key := semanticViewTargetIndexKey(target.View, target.TargetID)
		state.byViewTarget[key] = appendUniqueString(state.byViewTarget[key], occurrence.OccurrenceID)
	}
}

func (state *semanticNavigationBuildState) ensureEntity(descriptor semanticEntityDescriptor) *SemanticNavigationEntity {
	if index, ok := state.entityIndex[descriptor.ID]; ok {
		return &state.entities[index]
	}
	state.entityIndex[descriptor.ID] = len(state.entities)
	state.entities = append(state.entities, SemanticNavigationEntity{ID: descriptor.ID, Kind: descriptor.Kind, Label: descriptor.Label})
	return &state.entities[len(state.entities)-1]
}

func (state *semanticNavigationBuildState) finalize(model *SemanticModel) {
	for index := range state.entities {
		entity := &state.entities[index]
		sort.Strings(entity.OccurrenceIDs)
		sort.Strings(entity.RelatedEntityIDs)
		sort.SliceStable(entity.ViewTargets, func(i, j int) bool {
			if entity.ViewTargets[i].Priority != entity.ViewTargets[j].Priority {
				return entity.ViewTargets[i].Priority > entity.ViewTargets[j].Priority
			}
			if entity.ViewTargets[i].View != entity.ViewTargets[j].View {
				return entity.ViewTargets[i].View < entity.ViewTargets[j].View
			}
			return entity.ViewTargets[i].TargetID < entity.ViewTargets[j].TargetID
		})
	}
	sort.SliceStable(state.entities, func(i, j int) bool { return state.entities[i].ID < state.entities[j].ID })
	sort.SliceStable(state.occurrences, func(i, j int) bool { return state.occurrences[i].OccurrenceID < state.occurrences[j].OccurrenceID })
	sortSemanticIndexMap(state.byEntityID)
	sortSemanticIndexMap(state.byObjectID)
	sortSemanticIntIndexMap(state.byObjectIndex)
	sortSemanticIndexMap(state.byViewTarget)

	model.Navigation = SemanticNavigationIndex{
		Entities:      state.entities,
		Occurrences:   state.occurrences,
		ByEntityID:    state.byEntityID,
		ByObjectID:    state.byObjectID,
		ByObjectIndex: state.byObjectIndex,
		ByViewTarget:  state.byViewTarget,
	}
	model.Source.OccurrenceIndex = map[int][]SemanticOccurrence{}
	for _, occurrence := range state.occurrences {
		if occurrence.SourceAnchor == nil || occurrence.SourceAnchor.ObjectIndex == nil {
			continue
		}
		index := *occurrence.SourceAnchor.ObjectIndex
		model.Source.OccurrenceIndex[index] = append(model.Source.OccurrenceIndex[index], occurrence)
	}
}

func (state *semanticNavigationBuildState) zoneEntityForPath(path string) semanticEntityDescriptor {
	segments := semanticPathSegments(path)
	if len(segments) < 2 || segments[0] != "zones" {
		return semanticEntityDescriptor{}
	}
	zoneName := segments[1]
	if state.activeZone.ID != "" && strings.EqualFold(state.activeZone.Label, zoneName) {
		return state.activeZone
	}
	var zones []Object
	for _, object := range state.objectsByName[normalizeName(zoneName)] {
		if strings.EqualFold(object.Type, "Zone") {
			zones = append(zones, object)
		}
	}
	if len(zones) == 1 {
		return state.objectEntity(zones[0])
	}
	return semanticEntityDescriptor{}
}

func semanticObjectsByName(doc Document) map[string][]Object {
	objects := map[string][]Object{}
	for _, object := range doc.Objects {
		name := normalizeName(objectName(object))
		if name != "" {
			objects[name] = append(objects[name], object)
		}
	}
	return objects
}

func semanticHVACEntitiesByObject(ctx *semanticContext) map[int]HVACNavigationEntity {
	entities := map[int]HVACNavigationEntity{}
	if ctx == nil {
		return entities
	}
	for _, entity := range ctx.hvac.ServiceModel.Navigation.Entities {
		if entity.ID == "" || entity.ObjectType == "" {
			continue
		}
		object, ok := ctx.objectByIndex[entity.ObjectIndex]
		if !ok || !strings.EqualFold(object.Type, entity.ObjectType) {
			continue
		}
		if entity.ObjectName != "" && !strings.EqualFold(objectName(object), entity.ObjectName) {
			continue
		}
		current, exists := entities[entity.ObjectIndex]
		if !exists || (current.Kind != "component" && entity.Kind == "component") {
			entities[entity.ObjectIndex] = entity
		}
	}
	return entities
}

func semanticServicePathsByZone(ctx *semanticContext) map[string][]ZoneServicePath {
	paths := map[string][]ZoneServicePath{}
	if ctx == nil {
		return paths
	}
	for _, summary := range ctx.hvac.ServiceModel.ZoneServices {
		zoneName := firstNonEmpty(summary.ZoneName, summary.ServedSubject.ZoneName)
		key := normalizeName(zoneName)
		if key != "" {
			paths[key] = append(paths[key], summary.Paths...)
		}
	}
	return paths
}

func semanticProfileItemsBySource(ctx *semanticContext) map[string]string {
	items := map[string]string{}
	if ctx == nil {
		return items
	}
	for _, zone := range ctx.profile.ZoneProfiles {
		for _, item := range zone.Items {
			items[semanticProfileSourceKey(item.ObjectIndex, item.ZoneName, item.Dimension)] = item.ID
		}
	}
	return items
}

func semanticProfileSourceKey(objectIndex int, zoneName string, dimension string) string {
	return fmt.Sprintf("%d|%s|%s", objectIndex, normalizeName(zoneName), normalizeName(dimension))
}

func semanticTopLevelSection(path string) string {
	segments := semanticPathSegments(path)
	if len(segments) == 0 {
		return ""
	}
	return segments[0]
}

func semanticSummaryCategory(section string) string {
	switch semanticIDToken(section) {
	case "geometry", "zones", "spaces", "site_geometry":
		return "geometry_areas"
	case "envelope", "constructions", "materials", "fenestration":
		return "envelope_fenestration"
	case "loads", "profiles", "internal_loads":
		return "internal_loads"
	case "schedules", "operation":
		return "schedules_operation"
	case "hvac", "services":
		return "hvac_conditioning"
	default:
		return "model_inventory"
	}
}

func semanticPathSegments(path string) []string {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func semanticSectionLabel(value string) string {
	parts := strings.Fields(strings.ReplaceAll(value, "_", " "))
	for index := range parts {
		if parts[index] != "" {
			parts[index] = strings.ToUpper(parts[index][:1]) + parts[index][1:]
		}
	}
	return strings.Join(parts, " ")
}

func semanticViewTargetIndexKey(view string, targetID string) string {
	return strings.TrimSpace(view) + "|" + strings.TrimSpace(targetID)
}

func semanticDiagnosticEntityID(diagnostic Diagnostic) string {
	messageHash := semanticStableHash(strings.TrimSpace(diagnostic.Message), 12)
	return fmt.Sprintf("diagnostic:%s:%d:%d:%s", semanticIDToken(diagnostic.Code), diagnostic.ObjectIndex, diagnostic.FieldIndex, messageHash)
}

func semanticProfileItemEntityID(dimension string, sourceObjectID string) string {
	return "profile-item:" + semanticIDToken(dimension) + ":" + strings.TrimSpace(sourceObjectID)
}

func semanticProfileGroupEntityID(stableGroupID string) string {
	return "profile-group:" + semanticIDToken(stableGroupID)
}

func semanticProfileZoneDimensionTargetID(zoneName string, dimension string) string {
	return "profile-zone-dimension:" + semanticIDToken(zoneName) + ":" + semanticIDToken(dimension)
}

func semanticHVACPathEntityID(pathID string) string {
	return "hvac-path:" + semanticIDToken(pathID)
}

func semanticHVACLoopEntityID(loopType string, loopName string) string {
	return "hvac-loop:" + semanticIDToken(loopType) + ":" + semanticIDToken(loopName)
}

func semanticHVACComponentEntityID(objectType string, objectName string) string {
	return "hvac-component:" + semanticIDToken(objectType) + ":" + semanticIDToken(objectName)
}

func semanticHVACCouplingEntityID(couplingID string) string {
	return "hvac-coupling:" + semanticIDToken(couplingID)
}

func semanticHVACNetworkEntityID(networkType string, networkName string) string {
	return "hvac-network:" + semanticIDToken(networkType) + ":" + semanticIDToken(networkName)
}

func semanticSourceObjectEntityID(objectID string) string {
	return "source-object:" + strings.TrimSpace(objectID)
}

func semanticSourceFieldEntityID(objectID string, fieldIndex int) string {
	return fmt.Sprintf("source-field:%s:field-%d", strings.TrimSpace(objectID), fieldIndex)
}

func semanticIDToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, character := range []byte(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			builder.WriteByte(character)
			continue
		}
		fmt.Fprintf(&builder, "%%%02x", character)
	}
	return builder.String()
}

func semanticStableHash(value string, length int) string {
	sum := sha256.Sum256([]byte(value))
	encoded := hex.EncodeToString(sum[:])
	if length > 0 && length < len(encoded) {
		return encoded[:length]
	}
	return encoded
}

func cloneSemanticSourceAnchor(anchor *SemanticSourceAnchor) *SemanticSourceAnchor {
	if anchor == nil {
		return nil
	}
	clone := *anchor
	clone.ObjectIndex = cloneIntPtr(anchor.ObjectIndex)
	clone.FieldIndex = cloneIntPtr(anchor.FieldIndex)
	return &clone
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func appendUniqueSemanticViewTargets(values []SemanticViewTarget, candidates ...SemanticViewTarget) []SemanticViewTarget {
	for _, candidate := range candidates {
		found := false
		for _, value := range values {
			if value.View == candidate.View && value.TargetID == candidate.TargetID {
				found = true
				break
			}
		}
		if !found {
			values = append(values, candidate)
		}
	}
	return values
}

func appendUniqueSemanticSourceAnchor(values []SemanticSourceAnchor, candidate SemanticSourceAnchor) []SemanticSourceAnchor {
	for _, value := range values {
		if value.ObjectID == candidate.ObjectID && intPtrValue(value.FieldIndex) == intPtrValue(candidate.FieldIndex) {
			return values
		}
	}
	return append(values, candidate)
}

func intPtrValue(value *int) int {
	if value == nil {
		return -1
	}
	return *value
}

func appendUniqueInt(values []int, candidates ...int) []int {
	for _, candidate := range candidates {
		found := false
		for _, value := range values {
			if value == candidate {
				found = true
				break
			}
		}
		if !found {
			values = append(values, candidate)
		}
	}
	return values
}

func sortSemanticIndexMap(values map[string][]string) {
	for key := range values {
		sort.Strings(values[key])
	}
}

func sortSemanticIntIndexMap(values map[int][]string) {
	for key := range values {
		sort.Strings(values[key])
	}
}
