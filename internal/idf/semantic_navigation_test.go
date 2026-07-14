package idf

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSemanticNavigationStableEntityIDsAcrossRenderAndObjectOrder(t *testing.T) {
	doc := mustParseSemanticNavigationFixture(t)
	first := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	second := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})

	for _, objectIndex := range []int{0, 1, 2} {
		firstID := semanticDefinitionEntityID(t, first, objectIndex)
		secondID := semanticDefinitionEntityID(t, second, objectIndex)
		if firstID != secondID {
			t.Fatalf("object %d entity changed across render: %q != %q", objectIndex, firstID, secondID)
		}
	}

	reordered := doc.clone()
	for left, right := 0, len(reordered.Objects)-1; left < right; left, right = left+1, right-1 {
		reordered.Objects[left], reordered.Objects[right] = reordered.Objects[right], reordered.Objects[left]
	}
	reorderedProjection := BuildSemanticYAMLProjection(reordered, SemanticYAMLMetadata{})
	for _, objectIndex := range []int{0, 1, 2} {
		if got, want := semanticDefinitionEntityID(t, reorderedProjection, objectIndex), semanticDefinitionEntityID(t, first, objectIndex); got != want {
			t.Fatalf("object %d entity changed after object reorder: got %q want %q", objectIndex, got, want)
		}
	}

	edited := doc.clone()
	edited.Objects[2].Fields[len(edited.Objects[2].Fields)-1].Value = "7"
	editedProjection := BuildSemanticYAMLProjection(edited, SemanticYAMLMetadata{})
	if got, want := semanticDefinitionEntityID(t, editedProjection, 2), semanticDefinitionEntityID(t, first, 2); got != want {
		t.Fatalf("named profile source entity changed after non-name edit: got %q want %q", got, want)
	}
}

func TestSemanticNavigationDuplicateNamesDoNotCollide(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office;

Zone,
  Office;
`)
	if err != nil {
		t.Fatal(err)
	}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	first := semanticDefinitionEntityID(t, projection, 0)
	second := semanticDefinitionEntityID(t, projection, 1)
	if first == second {
		t.Fatalf("duplicate Zone names collided at %q", first)
	}
	if !strings.HasPrefix(first, "zone:office:source:") || !strings.HasPrefix(second, "zone:office:source:") {
		t.Fatalf("duplicate IDs must retain the canonical zone prefix and source disambiguator: %q / %q", first, second)
	}
}

func TestSemanticNavigationZoneListProfileTargetsRemainZoneSpecific(t *testing.T) {
	doc, err := Parse(`
Zone,
  Zone A;                   !- Name

Zone,
  Zone B;                   !- Name

ZoneList,
  Both Zones,               !- Name
  Zone A,                   !- Zone 1 Name
  Zone B;                   !- Zone 2 Name

People,
  Shared People,            !- Name
  Both Zones,               !- Zone or ZoneList Name
  ,                         !- Number of People Schedule Name
  People,                   !- Number of People Calculation Method
  3;                        !- Number of People
`)
	if err != nil {
		t.Fatal(err)
	}

	expectedTargets := map[string]string{}
	for _, zone := range AnalyzeProfile(doc).ZoneProfiles {
		for _, item := range zone.Items {
			if item.ObjectIndex == 3 && item.Dimension == ProfileDimensionOccupancy {
				expectedTargets[normalizeName(item.ZoneName)] = item.ID
			}
		}
	}
	if len(expectedTargets) != 2 {
		t.Fatalf("ZoneList People profile targets = %#v, want one occupancy item per zone", expectedTargets)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	gotTargets := map[string]string{}
	for _, occurrence := range projection.Navigation.Occurrences {
		if occurrence.SourceAnchor == nil || occurrence.SourceAnchor.ObjectIndex == nil || *occurrence.SourceAnchor.ObjectIndex != 3 {
			continue
		}
		if occurrence.ContextKind != "zone_profile" || !strings.HasPrefix(occurrence.EntityID, "profile-item:occupancy:") {
			continue
		}
		zoneName := semanticZoneName(occurrence.Path)
		zoneKey := normalizeName(zoneName)
		wantTarget, ok := expectedTargets[zoneKey]
		if !ok {
			t.Fatalf("profile occurrence path %q resolved unexpected zone %q", occurrence.Path, zoneName)
		}
		if wantEntityID := semanticProfileItemEntityID(ProfileDimensionOccupancy, occurrence.SourceAnchor.ObjectID); occurrence.EntityID != wantEntityID {
			t.Fatalf("%s profile entity = %q, want %q", zoneName, occurrence.EntityID, wantEntityID)
		}
		gotTarget, ok := semanticViewTargetID(occurrence.ViewTargets, "profile", "profile-item")
		if !ok {
			t.Fatalf("%s profile occurrence has no ProfileItem target: %#v", zoneName, occurrence.ViewTargets)
		}
		if gotTarget != wantTarget {
			t.Fatalf("%s ProfileItem target = %q, want zone-specific %q", zoneName, gotTarget, wantTarget)
		}
		gotTargets[zoneKey] = gotTarget
	}
	if len(gotTargets) != len(expectedTargets) {
		t.Fatalf("navigable ZoneList People targets = %#v, want %#v", gotTargets, expectedTargets)
	}
	if gotTargets[normalizeName("Zone A")] == gotTargets[normalizeName("Zone B")] {
		t.Fatalf("ZoneList People targets collided: %#v", gotTargets)
	}
}

func TestSemanticNavigationDuplicateZonesUseExactGeometryTargets(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office;

Zone,
  Office;
`)
	if err != nil {
		t.Fatal(err)
	}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})

	zoneEntities := map[string]bool{}
	for _, entity := range projection.Navigation.Entities {
		if entity.Kind != "zone" {
			continue
		}
		if entity.ID == "zone:office" {
			t.Fatalf("duplicate zones produced phantom unsuffixed entity %#v", entity)
		}
		zoneEntities[entity.ID] = true
	}
	if len(zoneEntities) != 2 {
		t.Fatalf("zone entities = %#v, want exactly the two source-disambiguated zones", zoneEntities)
	}

	for objectIndex := 0; objectIndex < 2; objectIndex++ {
		entityID := semanticDefinitionEntityID(t, projection, objectIndex)
		if !zoneEntities[entityID] {
			t.Fatalf("object %d definition entity %q is not one of the two zone entities", objectIndex, entityID)
		}
		wantTarget := fmt.Sprintf("zone-%d", objectIndex)

		definitionFound := false
		for _, line := range projection.Lines {
			if line.ObjectIndex == nil || *line.ObjectIndex != objectIndex || line.FieldIndex == nil || *line.FieldIndex != 0 || strings.TrimSpace(line.Key) != "name" {
				continue
			}
			definitionFound = true
			if gotTarget, ok := semanticViewTargetID(line.ViewTargets, "geometry", "zone"); !ok || gotTarget != wantTarget {
				t.Fatalf("object %d definition Geometry target = %q (present=%v), want %q", objectIndex, gotTarget, ok, wantTarget)
			}
			break
		}
		if !definitionFound {
			t.Fatalf("object %d source-backed zone definition line not found", objectIndex)
		}

		geometryFound := false
		for _, line := range projection.Lines {
			if line.EntityID != entityID || line.SourceAnchor != nil || semanticContextKind(line.SemanticPath, line.EntityKind) != "zone_geometry" {
				continue
			}
			gotTarget, ok := semanticViewTargetID(line.ViewTargets, "geometry", "zone")
			if !ok {
				continue
			}
			geometryFound = true
			if gotTarget != wantTarget {
				t.Fatalf("object %d source-less zone_geometry target = %q, want %q (path %q)", objectIndex, gotTarget, wantTarget, line.SemanticPath)
			}
			break
		}
		if !geometryFound {
			t.Fatalf("object %d source-less zone_geometry line for entity %q not found", objectIndex, entityID)
		}
	}
}

func TestSemanticNavigationDuplicateSpaceNamesUseTheirOwnZones(t *testing.T) {
	doc, err := Parse(`
Zone,
  Zone A;       !- Name
Zone,
  Zone B;       !- Name
Space,
  Shared,       !- Name
  Zone A;       !- Zone Name
Space,
  Shared,       !- Name
  Zone B;       !- Zone Name
`)
	if err != nil {
		t.Fatal(err)
	}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	seen := map[string]bool{}
	for objectIndex, wantPrefix := range map[int]string{2: "space:zone%20a:shared", 3: "space:zone%20b:shared"} {
		found := false
		for _, line := range projection.Lines {
			if line.ObjectIndex == nil || *line.ObjectIndex != objectIndex || line.EntityKind != "space" || !strings.HasSuffix(line.SemanticPath, "/spaces/Shared") {
				continue
			}
			found = true
			if !strings.HasPrefix(line.EntityID, wantPrefix) {
				t.Fatalf("Space object %d entity = %q, want prefix %q", objectIndex, line.EntityID, wantPrefix)
			}
			if seen[line.EntityID] {
				t.Fatalf("duplicate cross-zone Space entity collided at %q", line.EntityID)
			}
			seen[line.EntityID] = true
			if targetID, ok := semanticViewTargetID(line.ViewTargets, "geometry", "space"); !ok || targetID != fmt.Sprintf("space-%d", objectIndex) {
				t.Fatalf("Space object %d Geometry target = %q (present=%v), want space-%d", objectIndex, targetID, ok, objectIndex)
			}
			break
		}
		if !found {
			t.Fatalf("Space object %d occurrence not found", objectIndex)
		}
	}
}

func TestSemanticNavigationOccurrencesPreserveContext(t *testing.T) {
	projection := BuildSemanticYAMLProjection(mustParseSemanticNavigationFixture(t), SemanticYAMLMetadata{})
	schedule := semanticNavigationEntityByKindLabel(t, projection.Navigation, "schedule", "AlwaysOn")
	occurrenceIDs := projection.Navigation.ByEntityID[schedule.ID]
	if len(occurrenceIDs) < 2 {
		t.Fatalf("schedule should have definition and use occurrences, got %#v", occurrenceIDs)
	}
	contexts := map[string]bool{}
	for _, occurrenceID := range occurrenceIDs {
		occurrence := semanticNavigationOccurrenceByID(t, projection.Navigation, occurrenceID)
		contexts[occurrence.ContextKind] = true
	}
	if !contexts["definition"] || !contexts["zone_profile"] {
		t.Fatalf("schedule occurrence contexts = %#v, want definition and zone_profile", contexts)
	}

	zone := semanticNavigationEntityByKindLabel(t, projection.Navigation, "zone", "Office")
	zoneContexts := map[string]bool{}
	for _, occurrenceID := range projection.Navigation.ByEntityID[zone.ID] {
		zoneContexts[semanticNavigationOccurrenceByID(t, projection.Navigation, occurrenceID).ContextKind] = true
	}
	if !zoneContexts["definition"] || !zoneContexts["zone_geometry"] || !zoneContexts["zone_profile"] {
		t.Fatalf("zone occurrence contexts = %#v, want definition/geometry/profile", zoneContexts)
	}
}

func TestSemanticNavigationSourceAnchorsAndPanelTargets(t *testing.T) {
	projection := BuildSemanticYAMLProjection(mustParseSemanticNavigationFixture(t), SemanticYAMLMetadata{})
	for index, line := range projection.Lines {
		if line.ObjectIndex == nil || *line.ObjectIndex < 0 {
			continue
		}
		if line.SourceAnchor == nil || line.SourceAnchor.ObjectID == "" {
			t.Fatalf("source-backed line %d lacks a stable anchor: %#v", index, line)
		}
		if line.SourceAnchor.ObjectIndex == nil || *line.SourceAnchor.ObjectIndex != *line.ObjectIndex {
			t.Fatalf("line %d anchor index mismatch: %#v", index, line.SourceAnchor)
		}
		if line.FieldIndex != nil && (line.SourceAnchor.FieldIndex == nil || *line.SourceAnchor.FieldIndex != *line.FieldIndex) {
			t.Fatalf("line %d anchor field mismatch: line=%#v anchor=%#v", index, line.FieldIndex, line.SourceAnchor.FieldIndex)
		}
	}

	assertSemanticPreferredContext(t, projection, "zone_geometry", "geometry")
	assertSemanticPreferredContext(t, projection, "zone_profile", "profile")
	assertSemanticPreferredKind(t, projection, "schedule", "profile")
	assertSemanticPreferredKind(t, projection, "output", "output")

	zone := semanticNavigationEntityByKindLabel(t, projection.Navigation, "zone", "Office")
	views := map[string]bool{}
	for _, target := range zone.ViewTargets {
		views[target.View] = true
	}
	if !views["geometry"] || !views["profile"] {
		t.Fatalf("zone view targets = %#v, want Geometry and Profile", zone.ViewTargets)
	}
}

func TestSemanticNavigationSiteDefinitionPrefersSummary(t *testing.T) {
	doc, err := Parse(`Site:Location, Seoul, 37.5, 127.0, 9.0, 38.0;`)
	if err != nil {
		t.Fatal(err)
	}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, line := range projection.Lines {
		if line.ObjectIndex == nil || *line.ObjectIndex != 0 || line.FieldIndex == nil || *line.FieldIndex != 0 {
			continue
		}
		if line.PreferredView != "summary" || line.PreferredTargetID != "model_inventory" {
			t.Fatalf("site definition preference = %q/%q, want summary/model_inventory", line.PreferredView, line.PreferredTargetID)
		}
		if targetID, ok := semanticViewTargetID(line.ViewTargets, "summary", "category"); !ok || targetID != "model_inventory" {
			t.Fatalf("site Summary target = %q (present=%v), want model_inventory", targetID, ok)
		}
		return
	}
	t.Fatal("Site:Location definition line not found")
}

func TestSemanticNavigationDoesNotRewriteSpaceNamesAsZoneSections(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office;     !- Name
Space,
  HVAC,       !- Name
  Office;     !- Zone Name
Space,
  Controls,   !- Name
  Office;     !- Zone Name
Space,
  services,   !- Name
  Office;     !- Zone Name
Space,
  profiles,   !- Name
  Office;     !- Zone Name
Space,
  geometry,   !- Name
  Office;     !- Zone Name
`)
	if err != nil {
		t.Fatal(err)
	}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for objectIndex, name := range map[int]string{1: "HVAC", 2: "Controls", 3: "services", 4: "profiles", 5: "geometry"} {
		found := false
		for _, line := range projection.Lines {
			if line.ObjectIndex == nil || *line.ObjectIndex != objectIndex || line.EntityKind != "space" || line.SemanticPath != "zones/Office/spaces/"+name {
				continue
			}
			found = true
			wantPath := "zones/Office/spaces/" + name
			if line.SemanticPath != wantPath || semanticContextKind(line.SemanticPath, line.EntityKind) != "definition" {
				t.Fatalf("Space %q path/context = %q/%q, want %q/definition", name, line.SemanticPath, semanticContextKind(line.SemanticPath, line.EntityKind), wantPath)
			}
			if line.PreferredView != "geometry" {
				t.Fatalf("Space %q preferred view = %q, want geometry", name, line.PreferredView)
			}
			if targetID, ok := semanticViewTargetID(line.ViewTargets, "geometry", "space"); !ok || targetID != fmt.Sprintf("space-%d", objectIndex) {
				t.Fatalf("Space %q Geometry target = %q (present=%v), want space-%d", name, targetID, ok, objectIndex)
			}
			break
		}
		if !found {
			t.Fatalf("Space %q definition not found:\n%s", name, projection.Text)
		}
	}
}

func TestSemanticNavigationHVACPathAmbiguityIsExplicit(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office;

ZoneHVAC:EquipmentConnections,
  Office,
  Office Equipment,
  Office Supply Inlet,
  ,
  Office Zone Air Node,
  ;

ZoneHVAC:EquipmentList,
  Office Equipment,
  SequentialLoad,
  ZoneHVAC:FourPipeFanCoil,
  Office FPFC,
  1,
  1;

ZoneHVAC:FourPipeFanCoil,
  Office FPFC,
  ,
  Autosize,
  Office Supply Inlet,
  Office Zone Air Node,
  HW Inlet,
  HW Outlet,
  CHW Inlet,
  CHW Outlet;
`)
	if err != nil {
		t.Fatal(err)
	}

	var servicePaths []ZoneServicePath
	for _, summary := range AnalyzeHVAC(doc).ServiceModel.ZoneServices {
		if strings.EqualFold(summary.ZoneName, "Office") {
			servicePaths = append(servicePaths, summary.Paths...)
		}
	}
	if len(servicePaths) < 2 {
		t.Fatalf("fixture produced %d Office HVAC paths, want an ambiguous multi-path zone: %#v", len(servicePaths), servicePaths)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	expectedPathIDs := map[string]bool{}
	pathOccurrenceIDs := map[string]bool{}
	for _, servicePath := range servicePaths {
		expectedPathIDs[servicePath.ID] = true
		entity := semanticNavigationEntityForViewTarget(t, projection.Navigation, "hvac-path", "hvac", "service-path", servicePath.ID)
		entityID := entity.ID
		if entity.Kind != "hvac-path" {
			t.Fatalf("HVAC path entity %q kind = %q, want hvac-path", entityID, entity.Kind)
		}

		matched := false
		for _, occurrenceID := range projection.Navigation.ByEntityID[entityID] {
			occurrence := semanticNavigationOccurrenceByID(t, projection.Navigation, occurrenceID)
			gotTarget, ok := semanticViewTargetID(occurrence.ViewTargets, "hvac", "service-path")
			if occurrence.ContextKind != "zone_service" || !ok || gotTarget != servicePath.ID {
				continue
			}
			matched = true
			if occurrence.EntityID != entityID {
				t.Fatalf("path %q occurrence points at entity %q, want %q", servicePath.ID, occurrence.EntityID, entityID)
			}
			if occurrence.PreferredView != "hvac" || occurrence.PreferredTargetID != servicePath.ID {
				t.Fatalf("explicit path occurrence preference = %q/%q, want hvac/%q", occurrence.PreferredView, occurrence.PreferredTargetID, servicePath.ID)
			}
			if !stringSliceContains(projection.Navigation.ByViewTarget[semanticViewTargetIndexKey("hvac", servicePath.ID)], occurrenceID) {
				t.Fatalf("ByViewTarget missing path %q occurrence %q", servicePath.ID, occurrenceID)
			}
			pathOccurrenceIDs[occurrenceID] = true
			break
		}
		if !matched {
			t.Fatalf("HVAC path %q has no exact zone_service occurrence", servicePath.ID)
		}
	}
	if len(pathOccurrenceIDs) != len(expectedPathIDs) {
		t.Fatalf("distinct HVAC path occurrences = %d, want %d for %#v", len(pathOccurrenceIDs), len(expectedPathIDs), expectedPathIDs)
	}

	zoneEntityID := semanticDefinitionEntityID(t, projection, 0)
	ambiguousFound := false
	for _, occurrenceID := range projection.Navigation.ByEntityID[zoneEntityID] {
		occurrence := semanticNavigationOccurrenceByID(t, projection.Navigation, occurrenceID)
		if occurrence.ContextKind != "zone_service" {
			continue
		}
		matchedTargets := map[string]bool{}
		for _, target := range occurrence.ViewTargets {
			if target.View == "hvac" && target.TargetKind == "service-path" && expectedPathIDs[target.TargetID] {
				matchedTargets[target.TargetID] = true
			}
		}
		if len(matchedTargets) < 2 {
			continue
		}
		ambiguousFound = true
		if occurrence.PreferredView != "hvac" || occurrence.PreferredTargetID != "" {
			t.Fatalf("ambiguous zone_service preference = %q/%q, want hvac with an empty target chooser", occurrence.PreferredView, occurrence.PreferredTargetID)
		}
		break
	}
	if !ambiguousFound {
		t.Fatalf("Office zone has no occurrence exposing multiple HVAC path choices: %#v", expectedPathIDs)
	}
}

func TestSemanticNavigationHVACPathIDsSurviveReparseAndReindex(t *testing.T) {
	firstSource := `
Zone, Office;
ZoneHVAC:EquipmentConnections, Office, Office Equipment, Office Supply Inlet, , Office Zone Air Node, ;
ZoneHVAC:EquipmentList, Office Equipment, SequentialLoad, ZoneHVAC:PackagedTerminalHeatPump, Office PTHP, 1, 1;
ZoneHVAC:PackagedTerminalHeatPump, Office PTHP, , Autosize, Office Supply Inlet, Office Zone Air Node;
`
	secondSource := `
ZoneHVAC:PackagedTerminalHeatPump, Office PTHP, , Autosize, Office Supply Inlet, Office Zone Air Node;
ZoneHVAC:EquipmentList, Office Equipment, SequentialLoad, ZoneHVAC:PackagedTerminalHeatPump, Office PTHP, 1, 1;
ZoneHVAC:EquipmentConnections, Office, Office Equipment, Office Supply Inlet, , Office Zone Air Node, ;
Zone, Office;
`
	firstDoc, err := Parse(firstSource)
	if err != nil {
		t.Fatal(err)
	}
	secondDoc, err := Parse(secondSource)
	if err != nil {
		t.Fatal(err)
	}

	firstReport := AnalyzeHVAC(firstDoc)
	secondReport := AnalyzeHVAC(secondDoc)
	firstRawTargets := hvacServicePathIDSet(firstReport)
	secondRawTargets := hvacServicePathIDSet(secondReport)
	if len(firstRawTargets) < 2 || len(secondRawTargets) < 2 {
		t.Fatalf("reordered PTHP fixture did not produce heating/cooling paths: first=%#v second=%#v", firstRawTargets, secondRawTargets)
	}
	if strings.Join(firstRawTargets, "|") == strings.Join(secondRawTargets, "|") {
		t.Fatalf("fixture did not reindex the legacy raw service targets: %#v", firstRawTargets)
	}

	firstProjection := BuildSemanticYAMLProjection(firstDoc, SemanticYAMLMetadata{})
	secondProjection := BuildSemanticYAMLProjection(secondDoc, SemanticYAMLMetadata{})
	firstStableIDs := semanticEntityIDsByKind(firstProjection.Navigation, "hvac-path")
	secondStableIDs := semanticEntityIDsByKind(secondProjection.Navigation, "hvac-path")
	if strings.Join(firstStableIDs, "|") != strings.Join(secondStableIDs, "|") {
		t.Fatalf("stable HVAC path IDs changed after reparse/reindex:\nfirst  %#v\nsecond %#v", firstStableIDs, secondStableIDs)
	}
	assertSemanticHVACPathTargetsResolve(t, firstProjection.Navigation, firstRawTargets)
	assertSemanticHVACPathTargetsResolve(t, secondProjection.Navigation, secondRawTargets)
}

func TestSemanticNavigationHVACCouplingIDsSurviveReparseAndReindex(t *testing.T) {
	firstDoc := semanticNavigationCouplingFixture()
	secondDoc := firstDoc.clone()
	for left, right := 0, len(secondDoc.Objects)-1; left < right; left, right = left+1, right-1 {
		secondDoc.Objects[left], secondDoc.Objects[right] = secondDoc.Objects[right], secondDoc.Objects[left]
	}
	for index := range secondDoc.Objects {
		secondDoc.Objects[index].Index = index
	}

	firstReport := AnalyzeHVAC(firstDoc)
	secondReport := AnalyzeHVAC(secondDoc)
	firstCoupling := findSystemCouplingByObject(firstReport.ServiceModel.Couplings, "ThermalStorage:Ice:Simple", "Ice Tank")
	secondCoupling := findSystemCouplingByObject(secondReport.ServiceModel.Couplings, "ThermalStorage:Ice:Simple", "Ice Tank")
	if firstCoupling == nil || secondCoupling == nil {
		t.Fatalf("Ice Tank coupling missing after reorder: first=%#v second=%#v", firstCoupling, secondCoupling)
	}
	if firstCoupling.ID == secondCoupling.ID {
		t.Fatalf("fixture did not reindex the legacy raw coupling target: %q", firstCoupling.ID)
	}

	firstProjection := BuildSemanticYAMLProjection(firstDoc, SemanticYAMLMetadata{})
	secondProjection := BuildSemanticYAMLProjection(secondDoc, SemanticYAMLMetadata{})
	firstCouplingEntity := semanticNavigationEntityForViewTarget(t, firstProjection.Navigation, "hvac-coupling", "hvac", "hvac-coupling", navigationCouplingID(firstCoupling.ID))
	secondCouplingEntity := semanticNavigationEntityForViewTarget(t, secondProjection.Navigation, "hvac-coupling", "hvac", "hvac-coupling", navigationCouplingID(secondCoupling.ID))
	if firstCouplingEntity.ID != secondCouplingEntity.ID {
		t.Fatalf("stable HVAC coupling ID changed after reparse/reindex: %q != %q", firstCouplingEntity.ID, secondCouplingEntity.ID)
	}

	assertStableCouplingPathRelationship(t, firstProjection.Navigation, firstReport, firstCouplingEntity)
	assertStableCouplingPathRelationship(t, secondProjection.Navigation, secondReport, secondCouplingEntity)
}

func TestSemanticNavigationHVACLoopSubtreeUsesCanonicalEntityAndPanelTarget(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "PlantLoop", Fields: []Field{
			{Value: "Chilled Water Loop"}, {Value: "Water"}, {Value: ""}, {Value: ""}, {Value: "CHW Setpoint"}, {Value: "15"}, {Value: "5"}, {Value: "Autosize"}, {Value: "0"}, {Value: "Autosize"},
			{Value: "Plant Supply Inlet"}, {Value: "Plant Supply Outlet"}, {Value: "Plant Supply Branches"}, {Value: ""}, {Value: "Plant Demand Inlet"}, {Value: "Plant Demand Outlet"}, {Value: ""}, {Value: ""},
		}},
		{Index: 1, Type: "BranchList", Fields: []Field{{Value: "Plant Supply Branches"}, {Value: "Plant Supply Branch"}}},
		{Index: 2, Type: "Branch", Fields: []Field{{Value: "Plant Supply Branch"}}},
	}}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	wantEntityID := semanticHVACLoopEntityID("PlantLoop", "Chilled Water Loop")
	wantTargetID := navigationLoopID(LoopRef{Type: "PlantLoop", Name: "Chilled Water Loop"})
	for _, line := range projection.Lines {
		if line.SourceAnchor != nil || line.SemanticPath != "hvac/plant_loops/Chilled Water Loop/supply_side" {
			continue
		}
		if line.EntityID != wantEntityID {
			t.Fatalf("source-less loop subtree entity = %q, want %q", line.EntityID, wantEntityID)
		}
		if got, ok := semanticViewTargetID(line.ViewTargets, "hvac", "hvac-loop"); !ok || got != wantTargetID {
			t.Fatalf("source-less loop subtree target = %q (present=%v), want %q", got, ok, wantTargetID)
		}
		return
	}
	t.Fatalf("source-less plant loop supply_side line not found")
}

func TestSemanticNavigationByEntityIndexesOnlyOwnOccurrences(t *testing.T) {
	projection := BuildSemanticYAMLProjection(mustParseSemanticNavigationFixture(t), SemanticYAMLMetadata{})
	for entityID, occurrenceIDs := range projection.Navigation.ByEntityID {
		for _, occurrenceID := range occurrenceIDs {
			occurrence := semanticNavigationOccurrenceByID(t, projection.Navigation, occurrenceID)
			if occurrence.EntityID != entityID {
				t.Fatalf("ByEntityID[%q] contains occurrence %q owned by %q", entityID, occurrenceID, occurrence.EntityID)
			}
		}
	}
	for _, entity := range projection.Navigation.Entities {
		for _, occurrenceID := range entity.OccurrenceIDs {
			occurrence := semanticNavigationOccurrenceByID(t, projection.Navigation, occurrenceID)
			if occurrence.EntityID != entity.ID {
				t.Fatalf("entity %q contains occurrence %q owned by %q", entity.ID, occurrenceID, occurrence.EntityID)
			}
		}
	}
}

func TestSemanticNavigationReverseIndexesAndJSONSchema(t *testing.T) {
	projection := BuildSemanticYAMLProjection(mustParseSemanticNavigationFixture(t), SemanticYAMLMetadata{})
	var checked bool
	for _, line := range projection.Lines {
		if line.EntityID == "" || line.OccurrenceID == "" || line.SourceAnchor == nil || len(line.ViewTargets) == 0 {
			continue
		}
		if !stringSliceContains(projection.Navigation.ByEntityID[line.EntityID], line.OccurrenceID) {
			t.Fatalf("ByEntityID missing %q -> %q", line.EntityID, line.OccurrenceID)
		}
		if !stringSliceContains(projection.Navigation.ByObjectID[line.SourceAnchor.ObjectID], line.OccurrenceID) {
			t.Fatalf("ByObjectID missing %q -> %q", line.SourceAnchor.ObjectID, line.OccurrenceID)
		}
		if line.SourceAnchor.ObjectIndex != nil && !stringSliceContains(projection.Navigation.ByObjectIndex[*line.SourceAnchor.ObjectIndex], line.OccurrenceID) {
			t.Fatalf("ByObjectIndex missing %d -> %q", *line.SourceAnchor.ObjectIndex, line.OccurrenceID)
		}
		target := line.ViewTargets[0]
		if !stringSliceContains(projection.Navigation.ByViewTarget[semanticViewTargetIndexKey(target.View, target.TargetID)], line.OccurrenceID) {
			t.Fatalf("ByViewTarget missing %s/%s -> %q", target.View, target.TargetID, line.OccurrenceID)
		}
		checked = true
		break
	}
	if !checked {
		t.Fatal("fixture did not produce a source-backed navigable line")
	}

	payload, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, key := range []string{`"navigation"`, `"entityId"`, `"occurrenceId"`, `"semanticPath"`, `"sourceAnchor"`, `"viewTargets"`, `"byEntityId"`, `"byObjectId"`, `"byObjectIndex"`, `"byViewTarget"`} {
		if !strings.Contains(text, key) {
			t.Fatalf("projection JSON missing schema key %s", key)
		}
	}
}

func TestSemanticNavigationProjectionMetadataGolden(t *testing.T) {
	projection := BuildSemanticYAMLProjection(mustParseSemanticNavigationFixture(t), SemanticYAMLMetadata{})
	zone := semanticNavigationEntityByKindLabel(t, projection.Navigation, "zone", "Office")
	type goldenOccurrence struct {
		Context           string   `json:"context"`
		Path              string   `json:"path"`
		PreferredView     string   `json:"preferredView,omitempty"`
		PreferredTargetID string   `json:"preferredTargetId,omitempty"`
		Targets           []string `json:"targets"`
	}
	var occurrences []goldenOccurrence
	for _, occurrenceID := range projection.Navigation.ByEntityID[zone.ID] {
		occurrence := semanticNavigationOccurrenceByID(t, projection.Navigation, occurrenceID)
		occurrences = append(occurrences, goldenOccurrence{
			Context:           occurrence.ContextKind,
			Path:              occurrence.Path,
			PreferredView:     occurrence.PreferredView,
			PreferredTargetID: occurrence.PreferredTargetID,
			Targets:           semanticTargetGoldenKeys(occurrence.ViewTargets),
		})
	}
	var definition SemanticYAMLLine
	for _, line := range projection.Lines {
		if line.EntityID == zone.ID && line.SourceAnchor != nil && line.SourceAnchor.ObjectIndex != nil && *line.SourceAnchor.ObjectIndex == 0 && line.FieldIndex != nil && *line.FieldIndex == 0 {
			definition = line
			break
		}
	}
	snapshot := struct {
		Definition struct {
			EntityID       string   `json:"entityId"`
			OccurrenceID   string   `json:"occurrenceId"`
			Path           string   `json:"path"`
			SourceObjectID string   `json:"sourceObjectId"`
			Targets        []string `json:"targets"`
		} `json:"definition"`
		Entity struct {
			ID      string   `json:"id"`
			Kind    string   `json:"kind"`
			Targets []string `json:"targets"`
		} `json:"entity"`
		Occurrences []goldenOccurrence `json:"occurrences"`
		ByEntity    int                `json:"byEntityCount"`
	}{
		Occurrences: occurrences,
		ByEntity:    len(projection.Navigation.ByEntityID[zone.ID]),
	}
	snapshot.Definition.EntityID = definition.EntityID
	snapshot.Definition.OccurrenceID = definition.OccurrenceID
	snapshot.Definition.Path = definition.SemanticPath
	snapshot.Definition.SourceObjectID = definition.SourceAnchor.ObjectID
	snapshot.Definition.Targets = semanticTargetGoldenKeys(definition.ViewTargets)
	snapshot.Entity.ID = zone.ID
	snapshot.Entity.Kind = zone.Kind
	snapshot.Entity.Targets = semanticTargetGoldenKeys(zone.ViewTargets)
	payload, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	const want = `{
  "definition": {
    "entityId": "zone:office",
    "occurrenceId": "occ-0a18b9cc6239fc9e7baf",
    "path": "zones/Office",
    "sourceObjectId": "obj-1fa10db56c3fbabc5392",
    "targets": [
      "geometry|zone|zone-0|80",
      "profile|zone|Office|75",
      "output|zone|Office|60",
      "input-text|source|obj-1fa10db56c3fbabc5392|20"
    ]
  },
  "entity": {
    "id": "zone:office",
    "kind": "zone",
    "targets": [
      "profile|zone-dimension|profile-zone-dimension:office:occupancy|95",
      "geometry|zone|zone-0|80",
      "profile|zone|Office|75",
      "output|zone|Office|60",
      "input-text|source|obj-1da6b4d2796ca95b0025|20",
      "input-text|source|obj-1fa10db56c3fbabc5392|20"
    ]
  },
  "occurrences": [
    {
      "context": "definition",
      "path": "zones/Office",
      "targets": [
        "geometry|zone|zone-0|80",
        "profile|zone|Office|75",
        "output|zone|Office|60",
        "input-text|source|obj-1fa10db56c3fbabc5392|20"
      ]
    },
    {
      "context": "zone_output",
      "path": "zones/Office/outputs",
      "preferredView": "output",
      "preferredTargetId": "Office",
      "targets": [
        "geometry|zone|zone-0|80",
        "profile|zone|Office|75",
        "output|zone|Office|60"
      ]
    },
    {
      "context": "zone_geometry",
      "path": "zones/Office/geometry",
      "preferredView": "geometry",
      "preferredTargetId": "zone-0",
      "targets": [
        "geometry|zone|zone-0|80",
        "profile|zone|Office|75",
        "output|zone|Office|60"
      ]
    },
    {
      "context": "zone_profile",
      "path": "zones/Office/profiles",
      "preferredView": "profile",
      "preferredTargetId": "profile-zone-dimension:office:occupancy",
      "targets": [
        "geometry|zone|zone-0|80",
        "profile|zone|Office|75",
        "profile|zone-dimension|profile-zone-dimension:office:occupancy|95",
        "output|zone|Office|60",
        "input-text|source|obj-1da6b4d2796ca95b0025|20"
      ]
    },
    {
      "context": "definition",
      "path": "zones/Office/spaces/Office",
      "targets": [
        "geometry|zone|zone-0|80",
        "profile|zone|Office|75",
        "output|zone|Office|60",
        "input-text|source|obj-1fa10db56c3fbabc5392|20"
      ]
    },
    {
      "context": "zone_profile",
      "path": "zones/Office/profiles",
      "preferredView": "profile",
      "preferredTargetId": "Office",
      "targets": [
        "geometry|zone|zone-0|80",
        "profile|zone|Office|75",
        "output|zone|Office|60",
        "profile|zone-dimension|profile-zone-dimension:office:occupancy|95"
      ]
    },
    {
      "context": "zone_geometry",
      "path": "zones/Office/geometry",
      "preferredView": "geometry",
      "preferredTargetId": "zone-0",
      "targets": [
        "geometry|zone|zone-0|80",
        "profile|zone|Office|75",
        "output|zone|Office|60",
        "input-text|source|obj-1fa10db56c3fbabc5392|20"
      ]
    },
    {
      "context": "definition",
      "path": "zones/Office/spaces",
      "targets": [
        "geometry|zone|zone-0|80",
        "profile|zone|Office|75",
        "output|zone|Office|60"
      ]
    }
  ],
  "byEntityCount": 8
}`
	if string(payload) != want {
		t.Fatalf("semantic navigation metadata golden mismatch:\n%s", payload)
	}
}

func TestSemanticNavigationBasicModeIndexesLastEntityPastLegacyBudget(t *testing.T) {
	var source strings.Builder
	for index := 0; index < 520; index++ {
		fmt.Fprintf(&source, "Zone,\n  Zone %03d;\n\n", index)
	}
	doc, err := Parse(source.String())
	if err != nil {
		t.Fatal(err)
	}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	if projection.BasicVisibleLineCount <= 250 {
		t.Fatalf("fixture should prove the hard budget is removed, basic lines = %d", projection.BasicVisibleLineCount)
	}
	lastIndex := 519
	lastID := semanticDefinitionEntityID(t, projection, lastIndex)
	occurrences := projection.Navigation.ByObjectIndex[lastIndex]
	if len(occurrences) == 0 {
		t.Fatalf("last object %d has no reverse-indexed occurrence", lastIndex)
	}
	if len(projection.Navigation.ByEntityID[lastID]) == 0 {
		t.Fatalf("last entity %q has no O(1) occurrence lookup", lastID)
	}
}

func TestSemanticDiagnosticEntityIDIsStableAndMessageSensitive(t *testing.T) {
	diagnostic := Diagnostic{Code: "missing_schedule_reference", ObjectIndex: 7, FieldIndex: 2, Message: "Missing schedule AlwaysOn"}
	first := semanticDiagnosticEntityID(diagnostic)
	if second := semanticDiagnosticEntityID(diagnostic); first != second {
		t.Fatalf("diagnostic ID changed: %q != %q", first, second)
	}
	diagnostic.Message = "Missing schedule Occupancy"
	if changed := semanticDiagnosticEntityID(diagnostic); changed == first {
		t.Fatalf("diagnostic message hash did not disambiguate %q", first)
	}
}

func TestSemanticNavigationCanonicalIDRules(t *testing.T) {
	objectID := "obj-0123456789abcdef"
	tests := map[string]string{
		"profile item":   semanticProfileItemEntityID("Lighting", objectID),
		"profile group":  semanticProfileGroupEntityID("Office Loads"),
		"hvac path":      semanticHVACPathEntityID("service-path:zone:office:cooling"),
		"hvac loop":      semanticHVACLoopEntityID("PlantLoop", "CHW Loop"),
		"hvac component": semanticHVACComponentEntityID("Coil:Cooling:Water", "Main Coil"),
		"hvac coupling":  semanticHVACCouplingEntityID("coil-water-side:Main Coil"),
		"hvac network":   semanticHVACNetworkEntityID("chilled-water", "Primary CHW"),
		"source object":  semanticSourceObjectEntityID(objectID),
		"source field":   semanticSourceFieldEntityID(objectID, 7),
	}
	prefixes := map[string]string{
		"profile item":   "profile-item:lighting:" + objectID,
		"profile group":  "profile-group:office%20loads",
		"hvac path":      "hvac-path:service-path%3azone%3aoffice%3acooling",
		"hvac loop":      "hvac-loop:plantloop:chw%20loop",
		"hvac component": "hvac-component:coil%3acooling%3awater:main%20coil",
		"hvac coupling":  "hvac-coupling:coil-water-side%3amain%20coil",
		"hvac network":   "hvac-network:chilled-water:primary%20chw",
		"source object":  "source-object:" + objectID,
		"source field":   "source-field:" + objectID + ":field-7",
	}
	for label, got := range tests {
		if want := prefixes[label]; got != want {
			t.Errorf("%s ID = %q, want %q", label, got, want)
		}
	}
}

func mustParseSemanticNavigationFixture(t *testing.T) Document {
	t.Helper()
	doc, err := Parse(`
Zone,
  Office;

Schedule:Compact,
  AlwaysOn,
  Fraction,
  Through: 12/31,
  For: AllDays,
  Until: 24:00,
  1;

People,
  Office People,
  Office,
  AlwaysOn,
  People,
  3;

Output:Variable,
  Office,
  People Occupant Count,
  Hourly;
`)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func semanticNavigationCouplingFixture() Document {
	return Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"}, {Value: "Office Equipment"}, {Value: ""}, {Value: ""}, {Value: "Office Zone Air Node"}, {Value: ""},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"}, {Value: "ZoneHVAC:LowTemperatureRadiant:VariableFlow"}, {Value: "Office Radiant"}, {Value: "1"}, {Value: "1"},
		}},
		{Index: 3, Type: "ZoneHVAC:LowTemperatureRadiant:VariableFlow", Fields: []Field{
			{Value: "Office Radiant"}, {Value: "Radiant Design"}, {Value: ""}, {Value: "Office"}, {Value: "Office Radiant Surface"}, {Value: "100"}, {Value: "Autosize"}, {Value: "Autosize"}, {Value: "HW Demand Inlet"}, {Value: "HW Demand Outlet"},
		}},
		{Index: 4, Type: "PlantLoop", Fields: []Field{
			{Value: "Heating Water Loop"}, {Value: "Water"}, {Value: ""}, {Value: "TES Operation"}, {Value: "HW Setpoint"}, {Value: "80"}, {Value: "20"}, {Value: "Autosize"}, {Value: "0"}, {Value: "Autosize"},
			{Value: "HW Supply Inlet"}, {Value: "HW Supply Outlet"}, {Value: "HW Supply Branches"}, {Value: ""}, {Value: "HW Demand Inlet"}, {Value: "HW Demand Outlet"}, {Value: "HW Demand Branches"}, {Value: ""},
		}},
		{Index: 5, Type: "BranchList", Fields: []Field{{Value: "HW Supply Branches"}, {Value: "Storage Branch"}}},
		{Index: 6, Type: "Branch", Fields: []Field{{Value: "Storage Branch"}, {Value: ""}, {Value: "ThermalStorage:Ice:Simple"}, {Value: "Ice Tank"}, {Value: "TES Inlet"}, {Value: "TES Outlet"}}},
		{Index: 7, Type: "BranchList", Fields: []Field{{Value: "HW Demand Branches"}, {Value: "Radiant Branch"}}},
		{Index: 8, Type: "Branch", Fields: []Field{{Value: "Radiant Branch"}, {Value: ""}, {Value: "ZoneHVAC:LowTemperatureRadiant:VariableFlow"}, {Value: "Office Radiant"}, {Value: "HW Demand Inlet"}, {Value: "HW Demand Outlet"}}},
		{Index: 9, Type: "ThermalStorage:Ice:Simple", Fields: []Field{{Value: "Ice Tank"}}},
		{Index: 10, Type: "PlantEquipmentOperation:ThermalEnergyStorage", Fields: []Field{{Value: "TES Operation"}}},
	}}
}

func semanticDefinitionEntityID(t *testing.T, projection SemanticYAMLProjection, objectIndex int) string {
	t.Helper()
	for _, line := range projection.Lines {
		if line.ObjectIndex == nil || *line.ObjectIndex != objectIndex || line.FieldIndex == nil || *line.FieldIndex != 0 {
			continue
		}
		if strings.TrimSpace(line.Key) == "name" && line.EntityID != "" {
			return line.EntityID
		}
	}
	t.Fatalf("definition entity for object %d not found", objectIndex)
	return ""
}

func semanticNavigationEntityByKindLabel(t *testing.T, index SemanticNavigationIndex, kind string, label string) SemanticNavigationEntity {
	t.Helper()
	for _, entity := range index.Entities {
		if entity.Kind == kind && strings.EqualFold(entity.Label, label) {
			return entity
		}
	}
	t.Fatalf("navigation entity kind=%q label=%q not found", kind, label)
	return SemanticNavigationEntity{}
}

func semanticNavigationEntityByID(t *testing.T, index SemanticNavigationIndex, entityID string) SemanticNavigationEntity {
	t.Helper()
	for _, entity := range index.Entities {
		if entity.ID == entityID {
			return entity
		}
	}
	t.Fatalf("navigation entity %q not found", entityID)
	return SemanticNavigationEntity{}
}

func semanticNavigationEntityForViewTarget(t *testing.T, index SemanticNavigationIndex, entityKind string, view string, targetKind string, targetID string) SemanticNavigationEntity {
	t.Helper()
	for _, entity := range index.Entities {
		if entity.Kind != entityKind {
			continue
		}
		for _, target := range entity.ViewTargets {
			if target.View == view && target.TargetKind == targetKind && target.TargetID == targetID {
				return entity
			}
		}
	}
	t.Fatalf("navigation entity kind=%q for %s/%s/%s not found", entityKind, view, targetKind, targetID)
	return SemanticNavigationEntity{}
}

func semanticNavigationOccurrenceByID(t *testing.T, index SemanticNavigationIndex, occurrenceID string) SemanticOccurrence {
	t.Helper()
	for _, occurrence := range index.Occurrences {
		if occurrence.OccurrenceID == occurrenceID {
			return occurrence
		}
	}
	t.Fatalf("navigation occurrence %q not found", occurrenceID)
	return SemanticOccurrence{}
}

func semanticViewTargetID(targets []SemanticViewTarget, view string, targetKind string) (string, bool) {
	for _, target := range targets {
		if target.View == view && target.TargetKind == targetKind {
			return target.TargetID, true
		}
	}
	return "", false
}

func semanticTargetGoldenKeys(targets []SemanticViewTarget) []string {
	keys := make([]string, 0, len(targets))
	for _, target := range targets {
		keys = append(keys, fmt.Sprintf("%s|%s|%s|%d", target.View, target.TargetKind, target.TargetID, target.Priority))
	}
	return keys
}

func hvacServicePathIDSet(report HVACReport) []string {
	ids := map[string]bool{}
	for _, summary := range report.ServiceModel.ZoneServices {
		for _, servicePath := range summary.Paths {
			ids[servicePath.ID] = true
		}
	}
	return sortedStringSet(ids)
}

func semanticEntityIDsByKind(index SemanticNavigationIndex, kind string) []string {
	var ids []string
	for _, entity := range index.Entities {
		if entity.Kind == kind {
			ids = append(ids, entity.ID)
		}
	}
	return ids
}

func assertSemanticHVACPathTargetsResolve(t *testing.T, index SemanticNavigationIndex, validTargetIDs []string) {
	t.Helper()
	valid := map[string]bool{}
	for _, targetID := range validTargetIDs {
		valid[targetID] = true
	}
	for _, entity := range index.Entities {
		if entity.Kind != "hvac-path" {
			continue
		}
		targetID, ok := semanticViewTargetID(entity.ViewTargets, "hvac", "service-path")
		if !ok || !valid[targetID] {
			t.Fatalf("HVAC path entity %q target = %q (present=%v), valid targets %#v", entity.ID, targetID, ok, validTargetIDs)
		}
		if len(index.ByViewTarget[semanticViewTargetIndexKey("hvac", targetID)]) == 0 {
			t.Fatalf("HVAC path target %q has no reverse index", targetID)
		}
	}
}

func assertStableCouplingPathRelationship(t *testing.T, index SemanticNavigationIndex, report HVACReport, couplingEntity SemanticNavigationEntity) {
	t.Helper()
	couplingTargetID, ok := semanticViewTargetID(couplingEntity.ViewTargets, "hvac", "hvac-coupling")
	if !ok {
		t.Fatalf("coupling entity %q has no HVAC target", couplingEntity.ID)
	}
	for _, summary := range report.ServiceModel.ZoneServices {
		for _, servicePath := range summary.Paths {
			for _, couplingID := range servicePath.SupportingCouplings {
				if navigationCouplingID(couplingID) != couplingTargetID {
					continue
				}
				pathEntity := semanticNavigationEntityForViewTarget(t, index, "hvac-path", "hvac", "service-path", servicePath.ID)
				if !stringSliceContains(couplingEntity.RelatedEntityIDs, pathEntity.ID) || !stringSliceContains(pathEntity.RelatedEntityIDs, couplingEntity.ID) {
					t.Fatalf("stable path/coupling relationship missing: path=%#v coupling=%#v", pathEntity.RelatedEntityIDs, couplingEntity.RelatedEntityIDs)
				}
				return
			}
		}
	}
	t.Fatalf("no service path references coupling entity %q", couplingEntity.ID)
}

func assertSemanticPreferredContext(t *testing.T, projection SemanticYAMLProjection, context string, view string) {
	t.Helper()
	for _, occurrence := range projection.Navigation.Occurrences {
		if occurrence.ContextKind == context && occurrence.PreferredView == view {
			return
		}
	}
	t.Fatalf("no occurrence with context %q and preferred view %q", context, view)
}

func assertSemanticPreferredKind(t *testing.T, projection SemanticYAMLProjection, kind string, view string) {
	t.Helper()
	entity := SemanticNavigationEntity{}
	for _, candidate := range projection.Navigation.Entities {
		if candidate.Kind == kind {
			entity = candidate
			break
		}
	}
	if entity.ID == "" {
		t.Fatalf("no entity of kind %q", kind)
	}
	for _, occurrenceID := range projection.Navigation.ByEntityID[entity.ID] {
		if semanticNavigationOccurrenceByID(t, projection.Navigation, occurrenceID).PreferredView == view {
			return
		}
	}
	t.Fatalf("entity kind %q has no preferred %q occurrence", kind, view)
}
