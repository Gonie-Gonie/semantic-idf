package idf

import (
	"fmt"
	"testing"
)

const (
	phaseHLargeObjectCount      = 10_240
	phaseHLargeZoneCount        = 500
	phaseHLargeSurfaceCount     = 1_000
	phaseHLargeServicePathCount = 100
	phaseHLargeDiagnosticCount  = 1_000
)

// TestSemanticNavigationLargeIndexIntegrity exercises the navigation builder
// at the acceptance-test sizes from LINK-184 without making the test depend on
// the much more expensive report renderers. The semantic context contains the
// same panel records that a completed analysis supplies, while the model has a
// source-backed node for every object.
func TestSemanticNavigationLargeIndexIntegrity(t *testing.T) {
	doc, context := phaseHLargeNavigationFixture(t)
	first := phaseHBuildLargeNavigation(doc, context)
	second := phaseHBuildLargeNavigation(doc, context)

	if got := len(doc.Objects); got < 10_000 {
		t.Fatalf("large fixture object count = %d, want at least 10,000", got)
	}
	if got := len(context.geometry.Zones); got != phaseHLargeZoneCount {
		t.Fatalf("large fixture zone count = %d, want %d", got, phaseHLargeZoneCount)
	}
	if got := len(context.geometry.Surfaces); got != phaseHLargeSurfaceCount {
		t.Fatalf("large fixture surface count = %d, want %d", got, phaseHLargeSurfaceCount)
	}
	if got := phaseHServicePathCount(context.hvac.ServiceModel.ZoneServices); got != phaseHLargeServicePathCount {
		t.Fatalf("large fixture service path count = %d, want %d", got, phaseHLargeServicePathCount)
	}
	if got := len(context.diagnostics); got != phaseHLargeDiagnosticCount {
		t.Fatalf("large fixture diagnostic count = %d, want %d", got, phaseHLargeDiagnosticCount)
	}

	phaseHAssertNavigationIndexIntegrity(t, first)
	phaseHAssertNavigationIndexIntegrity(t, second)

	for _, objectIndex := range []int{0, phaseHLargeZoneCount - 1, phaseHLargeZoneCount, phaseHLargeZoneCount + phaseHLargeSurfaceCount - 1, phaseHLargeObjectCount - 1} {
		firstID := phaseHDefinitionEntityIDForObject(t, first, objectIndex)
		secondID := phaseHDefinitionEntityIDForObject(t, second, objectIndex)
		if firstID != secondID {
			t.Fatalf("object %d entity ID changed across rebuild: %q != %q", objectIndex, firstID, secondID)
		}
	}

	lastObjectIndex := phaseHLargeObjectCount - 1
	lastOccurrence := phaseHDefinitionOccurrenceForObject(t, first, lastObjectIndex)
	if len(first.ByEntityID[lastOccurrence.EntityID]) == 0 {
		t.Fatalf("last entity %q has no direct entity reverse lookup", lastOccurrence.EntityID)
	}
	if lastOccurrence.SourceAnchor == nil || len(first.ByObjectID[lastOccurrence.SourceAnchor.ObjectID]) == 0 {
		t.Fatalf("last object %d has no stable source-object reverse lookup", lastObjectIndex)
	}
	if got := lastOccurrence.LineIndexes; len(got) != 1 || got[0] <= 250 {
		t.Fatalf("last object direct reveal line indexes = %#v, want an exact line after 250", got)
	}

	lastSurfaceIndex := phaseHLargeZoneCount + phaseHLargeSurfaceCount - 1
	lastSurfaceTarget := fmt.Sprintf("surface-%d", lastSurfaceIndex)
	if got := first.ByViewTarget[semanticViewTargetIndexKey("geometry", lastSurfaceTarget)]; len(got) == 0 {
		t.Fatalf("last of 1,000 surfaces has no Geometry reverse target %q", lastSurfaceTarget)
	}

	for _, summary := range context.hvac.ServiceModel.ZoneServices {
		for _, path := range summary.Paths {
			occurrenceIDs := first.ByViewTarget[semanticViewTargetIndexKey("hvac", path.ID)]
			if len(occurrenceIDs) == 0 {
				t.Fatalf("HVAC service path %q has no semantic reverse target", path.ID)
			}
			foundZoneService := false
			for _, occurrenceID := range occurrenceIDs {
				if phaseHOccurrenceByID(t, first, occurrenceID).ContextKind == "zone_service" {
					foundZoneService = true
					break
				}
			}
			if !foundZoneService {
				t.Fatalf("HVAC service path %q has no zone_service occurrence in %#v", path.ID, occurrenceIDs)
			}
		}
	}

	for _, diagnostic := range context.diagnostics {
		diagnosticID := semanticDiagnosticEntityID(diagnostic)
		occurrenceIDs := first.ByViewTarget[semanticViewTargetIndexKey("diagnose", diagnosticID)]
		if len(occurrenceIDs) == 0 {
			t.Fatalf("diagnostic %q has no Diagnose reverse target", diagnosticID)
		}
		if got := phaseHOccurrenceByID(t, first, occurrenceIDs[0]).EntityID; got != diagnosticID {
			t.Fatalf("diagnostic reverse target %q resolves entity %q", diagnosticID, got)
		}
	}

	for _, summary := range context.hvac.ServiceModel.ZoneServices[:3] {
		path := summary.Paths[0]
		firstID := phaseHHVACPathEntityIDForTarget(t, first, path.ID)
		secondID := phaseHHVACPathEntityIDForTarget(t, second, path.ID)
		if firstID != secondID {
			t.Fatalf("HVAC path %q stable entity changed across rebuild: %q != %q", path.ID, firstID, secondID)
		}
	}
}

func phaseHLargeNavigationFixture(t *testing.T) (Document, *semanticContext) {
	t.Helper()
	doc := Document{Objects: make([]Object, 0, phaseHLargeObjectCount)}
	geometry := GeometryReport{
		Zones:    make([]GeometryZone, 0, phaseHLargeZoneCount),
		Surfaces: make([]GeometrySurface, 0, phaseHLargeSurfaceCount),
	}

	addObject := func(objectType, name string, fields ...Field) int {
		index := len(doc.Objects)
		objectFields := []Field{{Value: name, Comment: "Name"}}
		objectFields = append(objectFields, fields...)
		doc.Objects = append(doc.Objects, Object{Index: index, Type: objectType, Fields: objectFields})
		return index
	}

	for index := 0; index < phaseHLargeZoneCount; index++ {
		name := fmt.Sprintf("Large Zone %03d", index)
		objectIndex := addObject("Zone", name)
		geometry.Zones = append(geometry.Zones, GeometryZone{ID: fmt.Sprintf("zone-%d", objectIndex), ObjectIndex: objectIndex, Name: name})
	}
	for index := 0; index < phaseHLargeSurfaceCount; index++ {
		name := fmt.Sprintf("Large Surface %04d", index)
		zoneName := fmt.Sprintf("Large Zone %03d", index%phaseHLargeZoneCount)
		objectIndex := addObject(
			"BuildingSurface:Detailed",
			name,
			Field{Value: "Wall", Comment: "Surface Type"},
			Field{Value: "Synthetic Construction", Comment: "Construction Name"},
			Field{Value: zoneName, Comment: "Zone Name"},
		)
		geometry.Surfaces = append(geometry.Surfaces, GeometrySurface{
			ID:          fmt.Sprintf("surface-%d", objectIndex),
			ObjectIndex: objectIndex,
			Name:        name,
			Type:        "BuildingSurface:Detailed",
			ZoneName:    zoneName,
		})
	}

	componentIndexes := make([]int, 0, phaseHLargeServicePathCount)
	for index := 0; index < phaseHLargeServicePathCount; index++ {
		componentIndexes = append(componentIndexes, addObject("ZoneHVAC:IdealLoadsAirSystem", fmt.Sprintf("Large Ideal Loads %03d", index)))
	}
	for len(doc.Objects) < phaseHLargeObjectCount {
		index := len(doc.Objects)
		addObject("Synthetic:Object", fmt.Sprintf("Synthetic Object %05d", index))
	}

	zoneServices := make([]ZoneServiceSummary, 0, phaseHLargeServicePathCount)
	for index, componentIndex := range componentIndexes {
		zoneName := fmt.Sprintf("Large Zone %03d", index)
		componentName := fmt.Sprintf("Large Ideal Loads %03d", index)
		subject := ServedSubjectRef{Kind: "zone", Name: zoneName, ZoneName: zoneName, ObjectIndex: index}
		delivery := ComponentRef{
			ID:          fmt.Sprintf("component-%03d", index),
			ObjectType:  "ZoneHVAC:IdealLoadsAirSystem",
			ObjectName:  componentName,
			ObjectIndex: componentIndex,
			DisplayName: componentName,
		}
		path := ZoneServicePath{
			ID:            fmt.Sprintf("large-service-path-%03d", index),
			ZoneName:      zoneName,
			ServiceKind:   "cooling",
			PathType:      "direct-zone-equipment",
			Delivery:      delivery,
			ServedSubject: subject,
		}
		zoneServices = append(zoneServices, ZoneServiceSummary{
			ID:            fmt.Sprintf("large-zone-service-%03d", index),
			ZoneName:      zoneName,
			ServedSubject: subject,
			Paths:         []ZoneServicePath{path},
		})
	}

	diagnostics := make([]Diagnostic, 0, phaseHLargeDiagnosticCount)
	for index := 0; index < phaseHLargeDiagnosticCount; index++ {
		objectIndex := 2_000 + index
		object := doc.Objects[objectIndex]
		diagnostics = append(diagnostics, Diagnostic{
			Severity:    DiagnosticWarning,
			Category:    "Large model acceptance",
			Code:        "large_model_check",
			Message:     fmt.Sprintf("Synthetic large-model diagnostic %04d", index),
			ObjectIndex: objectIndex,
			ObjectType:  object.Type,
			ObjectName:  objectName(object),
			FieldIndex:  0,
			Field:       "Name",
		})
	}

	objectByIndex := make(map[int]Object, len(doc.Objects))
	for _, object := range doc.Objects {
		objectByIndex[object.Index] = object
	}
	context := &semanticContext{
		doc:           doc,
		objectByIndex: objectByIndex,
		geometry:      geometry,
		hvac: HVACReport{ServiceModel: HVACServiceModel{
			ZoneServices: zoneServices,
		}},
		diagnostics: diagnostics,
	}
	return doc, context
}

func phaseHBuildLargeNavigation(doc Document, context *semanticContext) SemanticNavigationIndex {
	nodes := make([]SemanticYAMLNode, 0, len(doc.Objects)+2)
	nodes = append(nodes,
		SemanticYAMLNode{Indent: 0, Raw: "semantic_energyplus_model:"},
		SemanticYAMLNode{Indent: 1, Raw: "objects:"},
	)
	for _, object := range doc.Objects {
		nodes = append(nodes, SemanticYAMLNode{
			Indent:      2,
			Raw:         "- name: " + objectName(object),
			ObjectIndex: intPtr(object.Index),
			ObjectType:  object.Type,
			ObjectName:  objectName(object),
			SourceKind:  "derived",
			EditKind:    "readonly",
			Role:        "object",
		})
	}
	model := SemanticModel{Nodes: nodes, Source: SemanticModelSource{ObjectCount: len(doc.Objects)}}
	buildSemanticNavigation(doc, context, &model)
	return model.Navigation
}

func phaseHAssertNavigationIndexIntegrity(t *testing.T, navigation SemanticNavigationIndex) {
	t.Helper()
	entities := make(map[string]SemanticNavigationEntity, len(navigation.Entities))
	for _, entity := range navigation.Entities {
		if entity.ID == "" {
			t.Fatal("navigation contains an entity with an empty ID")
		}
		if _, exists := entities[entity.ID]; exists {
			t.Fatalf("navigation contains duplicate entity ID %q", entity.ID)
		}
		entities[entity.ID] = entity
	}
	occurrences := make(map[string]SemanticOccurrence, len(navigation.Occurrences))
	for _, occurrence := range navigation.Occurrences {
		if occurrence.OccurrenceID == "" || occurrence.EntityID == "" {
			t.Fatalf("navigation contains incomplete occurrence %#v", occurrence)
		}
		if _, exists := occurrences[occurrence.OccurrenceID]; exists {
			t.Fatalf("navigation contains duplicate occurrence ID %q", occurrence.OccurrenceID)
		}
		if _, exists := entities[occurrence.EntityID]; !exists {
			t.Fatalf("occurrence %q references missing entity %q", occurrence.OccurrenceID, occurrence.EntityID)
		}
		occurrences[occurrence.OccurrenceID] = occurrence
	}
	if len(navigation.ByObjectIndex) < phaseHLargeObjectCount {
		t.Fatalf("object-index reverse map entries = %d, want at least %d", len(navigation.ByObjectIndex), phaseHLargeObjectCount)
	}

	assertIDs := func(label string, values map[string][]string, validate func(string, SemanticOccurrence) bool) {
		t.Helper()
		for key, occurrenceIDs := range values {
			if len(occurrenceIDs) == 0 {
				t.Fatalf("%s[%q] has an empty occurrence list", label, key)
			}
			for _, occurrenceID := range occurrenceIDs {
				occurrence, exists := occurrences[occurrenceID]
				if !exists {
					t.Fatalf("%s[%q] references missing occurrence %q", label, key, occurrenceID)
				}
				if validate != nil && !validate(key, occurrence) {
					t.Fatalf("%s[%q] contains incompatible occurrence %#v", label, key, occurrence)
				}
			}
		}
	}
	assertIDs("ByEntityID", navigation.ByEntityID, func(key string, occurrence SemanticOccurrence) bool {
		return occurrence.EntityID == key
	})
	assertIDs("ByObjectID", navigation.ByObjectID, func(key string, occurrence SemanticOccurrence) bool {
		return occurrence.SourceAnchor != nil && occurrence.SourceAnchor.ObjectID == key
	})
	assertIDs("ByViewTarget", navigation.ByViewTarget, nil)
	for objectIndex, occurrenceIDs := range navigation.ByObjectIndex {
		if len(occurrenceIDs) == 0 {
			t.Fatalf("ByObjectIndex[%d] has an empty occurrence list", objectIndex)
		}
		for _, occurrenceID := range occurrenceIDs {
			occurrence, exists := occurrences[occurrenceID]
			if !exists {
				t.Fatalf("ByObjectIndex[%d] references missing occurrence %q", objectIndex, occurrenceID)
			}
			if occurrence.SourceAnchor == nil || occurrence.SourceAnchor.ObjectIndex == nil || *occurrence.SourceAnchor.ObjectIndex != objectIndex {
				t.Fatalf("ByObjectIndex[%d] contains incompatible occurrence %#v", objectIndex, occurrence)
			}
		}
	}
}

func phaseHDefinitionEntityIDForObject(t *testing.T, navigation SemanticNavigationIndex, objectIndex int) string {
	t.Helper()
	return phaseHDefinitionOccurrenceForObject(t, navigation, objectIndex).EntityID
}

func phaseHDefinitionOccurrenceForObject(t *testing.T, navigation SemanticNavigationIndex, objectIndex int) SemanticOccurrence {
	t.Helper()
	for _, occurrenceID := range navigation.ByObjectIndex[objectIndex] {
		occurrence := phaseHOccurrenceByID(t, navigation, occurrenceID)
		if occurrence.ContextKind == "definition" {
			return occurrence
		}
	}
	t.Fatalf("object %d has no definition occurrence", objectIndex)
	return SemanticOccurrence{}
}

func phaseHOccurrenceByID(t *testing.T, navigation SemanticNavigationIndex, occurrenceID string) SemanticOccurrence {
	t.Helper()
	for _, occurrence := range navigation.Occurrences {
		if occurrence.OccurrenceID == occurrenceID {
			return occurrence
		}
	}
	t.Fatalf("occurrence %q not found", occurrenceID)
	return SemanticOccurrence{}
}

func phaseHHVACPathEntityIDForTarget(t *testing.T, navigation SemanticNavigationIndex, targetID string) string {
	t.Helper()
	entityKinds := make(map[string]string, len(navigation.Entities))
	for _, entity := range navigation.Entities {
		entityKinds[entity.ID] = entity.Kind
	}
	for _, occurrenceID := range navigation.ByViewTarget[semanticViewTargetIndexKey("hvac", targetID)] {
		occurrence := phaseHOccurrenceByID(t, navigation, occurrenceID)
		if occurrence.ContextKind == "zone_service" && entityKinds[occurrence.EntityID] == "hvac-path" {
			return occurrence.EntityID
		}
	}
	t.Fatalf("HVAC target %q has no hvac-path entity occurrence", targetID)
	return ""
}

func phaseHServicePathCount(summaries []ZoneServiceSummary) int {
	count := 0
	for _, summary := range summaries {
		count += len(summary.Paths)
	}
	return count
}
