package idf

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestAnalyzeGeometryIncludesThermalTopologySchema(t *testing.T) {
	document := thermalTopologyTestDocument()
	geometry := AnalyzeGeometry(document)
	if geometry.Topology.Schema != thermalTopologySchema {
		t.Fatalf("topology schema = %q, want %q", geometry.Topology.Schema, thermalTopologySchema)
	}
	if len(geometry.Topology.Nodes) == 0 || len(geometry.Topology.Boundaries) == 0 {
		t.Fatalf("topology was not populated: %#v", geometry.Topology.Stats)
	}
	payload, err := json.Marshal(geometry)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(payload), `"topology":{"schema":"semantic-idf.thermal-topology/v1"`) {
		t.Fatalf("geometry JSON does not contain topology schema: %s", payload)
	}
}

func TestThermalTopologyNodesUseSemanticEntityIDs(t *testing.T) {
	topology := AnalyzeGeometry(thermalTopologyTestDocument()).Topology
	zone := findThermalNode(t, topology, "zone:zone%20a")
	if zone.EntityID != zone.ID || zone.Kind != "zone" || zone.ObjectIndex == nil {
		t.Fatalf("zone topology node does not use semantic entity identity: %#v", zone)
	}
	space := findThermalNode(t, topology, "space:zone%20a:space%20a")
	if space.EntityID != space.ID || space.Kind != "space" || space.ZoneName != "Zone A" {
		t.Fatalf("space topology node does not use semantic entity identity: %#v", space)
	}
	outdoors := findThermalNode(t, topology, "thermal-environment:outdoors")
	if outdoors.EntityID != "" || len(outdoors.SourceAnchors) == 0 {
		t.Fatalf("virtual outdoors node must expose source anchors without a fake entity: %#v", outdoors)
	}
}

func TestThermalTopologyBuildsOneAuthoritativeBoundaryPerHeatTransferSurface(t *testing.T) {
	geometry := AnalyzeGeometry(thermalTopologyTestDocument())
	if got, want := len(geometry.Topology.Boundaries), geometry.SurfaceCount; got != want {
		t.Fatalf("boundary count = %d, want heat-transfer surface count %d", got, want)
	}
	for _, boundary := range geometry.Topology.Boundaries {
		if boundary.SurfaceID == "" || boundary.SurfaceEntityID == "" || boundary.OwnerZoneID == "" {
			t.Fatalf("boundary lost source ownership: %#v", boundary)
		}
		if len(boundary.SourceAnchors) < 3 && boundary.SurfaceName == "Pair A" {
			t.Fatalf("boundary source anchors do not include OBC and OBC object fields: %#v", boundary.SourceAnchors)
		}
	}
}

func TestThermalTopologyResolvesEnergyPlusBoundaryFamilies(t *testing.T) {
	topology := AnalyzeGeometry(thermalTopologyTestDocument()).Topology
	tests := []struct {
		name            string
		relationKind    string
		targetKind      string
		virtual         bool
		counterpartName string
	}{
		{name: "Outdoors", relationKind: "exterior", targetKind: "outdoors"},
		{name: "Ground", relationKind: "ground", targetKind: "ground"},
		{name: "Adiabatic", relationKind: "adiabatic_explicit", targetKind: "adiabatic"},
		{name: "Self Surface", relationKind: "adiabatic_self_reference", targetKind: "adiabatic", virtual: true},
		{name: "Implicit Zone", relationKind: "interzone_implicit_zone", targetKind: "zone", virtual: true},
		{name: "Implicit Space", relationKind: "interspace_implicit", targetKind: "space", virtual: true},
		{name: "Foundation", relationKind: "foundation", targetKind: "foundation"},
		{name: "Ground Preprocessor", relationKind: "ground_preprocessor", targetKind: "ground_preprocessor"},
		{name: "Other Side", relationKind: "other_side_coefficients", targetKind: "other_side_coefficients"},
		{name: "Pair A", relationKind: "interzone_explicit_surface", targetKind: "space", counterpartName: "Pair B"},
		{name: "Pair B", relationKind: "interzone_explicit_surface", targetKind: "space", counterpartName: "Pair A"},
	}
	for _, test := range tests {
		boundary := findThermalBoundary(t, topology, test.name)
		if boundary.RelationKind != test.relationKind || boundary.TargetKind != test.targetKind || boundary.VirtualCounterpart != test.virtual {
			t.Errorf("boundary %q = relation %q target %q virtual %v", test.name, boundary.RelationKind, boundary.TargetKind, boundary.VirtualCounterpart)
		}
		if test.counterpartName != "" {
			counterpart := findThermalBoundary(t, topology, test.counterpartName)
			if boundary.CounterpartSurfaceID != counterpart.SurfaceID || boundary.PairID == "" || boundary.PairID != counterpart.PairID {
				t.Errorf("boundary %q reciprocal link = counterpart %q pair %q", test.name, boundary.CounterpartSurfaceID, boundary.PairID)
			}
		}
	}

	invalid := findThermalBoundary(t, topology, "Invalid")
	if invalid.RelationKind != "invalid" || invalid.TargetKind != "unresolved_target" || len(invalid.DiagnosticIDs) == 0 {
		t.Fatalf("invalid boundary did not survive with a linked issue: %#v", invalid)
	}
	if topology.Stats.InvalidBoundaryCount != 1 || topology.Stats.DiagnosticCount == 0 {
		t.Fatalf("topology stats do not reconcile invalid boundary: %#v", topology.Stats)
	}
}

func TestCanonicalOutsideBoundaryConditionNormalizesEnergyPlusFamilies(t *testing.T) {
	tests := map[string]string{
		" outdoors ":                                "Outdoors",
		"Other Zone":                                "Zone",
		"other-side-coefficients":                   "OtherSideCoefficients",
		"Other Side Conditions Model":               "OtherSideConditionsModel",
		"Ground FCfactor Method":                    "GroundFCfactorMethod",
		"Ground Slab Preprocessor Core":             "GroundSlabPreprocessorCore",
		"Ground Basement Preprocessor Average Wall": "GroundBasementPreprocessorAverageWall",
		"not-an-energyplus-choice":                  "",
	}
	for raw, want := range tests {
		if got := canonicalOutsideBoundaryCondition(raw); got != want {
			t.Errorf("canonicalOutsideBoundaryCondition(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestThermalTopologyExcludesShadingSurfaces(t *testing.T) {
	doc, err := Parse(rectangularGeometryFixture)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	geometry := AnalyzeGeometry(doc)
	if geometry.SurfaceCount != 4 {
		t.Fatalf("spatial surface count = %d, want 4 including shading", geometry.SurfaceCount)
	}
	if len(geometry.Topology.Boundaries) != 2 {
		t.Fatalf("thermal boundary count = %d, want 2 excluding shading", len(geometry.Topology.Boundaries))
	}
}

func TestThermalTopologyBuildsOpeningRecordsAndUA(t *testing.T) {
	geometry := AnalyzeGeometry(thermalOpeningTestDocument())
	if len(geometry.Topology.Openings) != 2 {
		t.Fatalf("opening count = %d, want 2", len(geometry.Topology.Openings))
	}
	openingA := findThermalOpening(t, geometry.Topology, "Window A")
	openingB := findThermalOpening(t, geometry.Topology, "Window B")
	if openingA.CounterpartOpeningID != openingB.ID || openingA.PairID == "" || openingA.PairID != openingB.PairID {
		t.Fatalf("opening pair was not resolved: A %#v B %#v", openingA, openingB)
	}
	if openingA.PhysicalArea != 3 || openingA.EffectiveArea != 6 || openingA.UValue != 2 || openingA.UA != 12 || !openingA.HasUA {
		t.Fatalf("opening area/UA = physical %v effective %v U %v UA %v has %v", openingA.PhysicalArea, openingA.EffectiveArea, openingA.UValue, openingA.UA, openingA.HasUA)
	}

	boundaryA := findThermalBoundary(t, geometry.Topology, "Pair A")
	if boundaryA.PhysicalGrossArea != 4 || boundaryA.PhysicalOpeningArea != 3 || boundaryA.PhysicalOpaqueArea != 1 {
		t.Fatalf("physical boundary areas = gross %v opening %v opaque %v", boundaryA.PhysicalGrossArea, boundaryA.PhysicalOpeningArea, boundaryA.PhysicalOpaqueArea)
	}
	if boundaryA.EffectiveGrossArea != 8 || boundaryA.EffectiveOpeningArea != 6 || boundaryA.EffectiveOpaqueArea != 2 {
		t.Fatalf("effective boundary areas = gross %v opening %v opaque %v", boundaryA.EffectiveGrossArea, boundaryA.EffectiveOpeningArea, boundaryA.EffectiveOpaqueArea)
	}
	if boundaryA.ConstructionStatus != "reverse_layer_equivalent" || !boundaryA.HasUA {
		t.Fatalf("boundary construction/UA status = %q / %v", boundaryA.ConstructionStatus, boundaryA.HasUA)
	}
	if boundaryA.OpaqueUA != 2.8572 || boundaryA.OpeningUA != 12 || boundaryA.TotalUA != 14.8572 {
		t.Fatalf("effective boundary UA = opaque %v opening %v total %v", boundaryA.OpaqueUA, boundaryA.OpeningUA, boundaryA.TotalUA)
	}

	openingsByID := map[string]ThermalOpeningRecord{openingA.ID: openingA}
	opaqueUA, openingUA, totalUA, hasUA := thermalBoundaryUAForAreaBasis(boundaryA, openingsByID, "physical")
	if !hasUA || opaqueUA != 1.4286 || openingUA != 6 || totalUA != 7.4286 {
		t.Fatalf("physical boundary UA = opaque %v opening %v total %v has %v", opaqueUA, openingUA, totalUA, hasUA)
	}
}

func TestGeometryConstructionIndexCoversStaticPerformanceFamilies(t *testing.T) {
	document := thermalConstructionTestDocument()
	constructions := geometryConstructionsFromDocument(document)
	index := thermalConstructionIndex(constructions)
	tests := []struct {
		name string
		kind string
		u    float64
		hasU bool
	}{
		{name: "Opaque A", kind: "layer_based_opaque", u: 1.4286, hasU: true},
		{name: "Window Construction", kind: "layer_based_window", u: 2, hasU: true},
		{name: "C Factor Construction", kind: "c_factor", u: 0.5, hasU: true},
		{name: "F Factor Construction", kind: "f_factor", u: 0.2, hasU: true},
		{name: "Complex State", kind: "complex_fenestration", hasU: false},
	}
	for _, test := range tests {
		construction, ok := index[normalizeName(test.name)]
		if !ok {
			t.Errorf("construction %q missing from shared index", test.name)
			continue
		}
		if construction.Kind != test.kind || construction.HasThermalPerformance != test.hasU || construction.UValue != test.u {
			t.Errorf("construction %q = kind %q U %v has %v", test.name, construction.Kind, construction.UValue, construction.HasThermalPerformance)
		}
	}
}

func TestDeclaredSurfaceGeometryValidationUsesWorldPolygons(t *testing.T) {
	validTopology := AnalyzeGeometry(thermalGeometryPairTestDocument(0, true)).Topology
	valid := findThermalBoundary(t, validTopology, "Pair A")
	if valid.GeometryCheck.Status != "valid" || valid.GeometryCheck.OverlapRatio != 1 || valid.GeometryCheck.NormalDot != -1 {
		t.Fatalf("valid declared pair geometry = %#v", valid.GeometryCheck)
	}
	if !hasAdjacencyObservation(validTopology, "declared_and_geometrically_matched") {
		t.Fatal("valid declared pair observation missing")
	}

	invalidTopology := AnalyzeGeometry(thermalGeometryPairTestDocument(0.5, true)).Topology
	invalid := findThermalBoundary(t, invalidTopology, "Pair A")
	if invalid.GeometryCheck.Status != "invalid" || invalid.GeometryCheck.OverlapRatio >= 0.99 {
		t.Fatalf("shifted declared pair geometry = %#v", invalid.GeometryCheck)
	}
	if !hasTopologyIssueCode(invalidTopology, "surface_pair_overlap_mismatch") {
		t.Fatalf("shifted declared pair did not link overlap diagnostic: %#v", invalid.DiagnosticIDs)
	}
	if !strings.Contains(invalid.GeometryCheck.Message, "tolerance") {
		t.Fatalf("geometry check does not expose tolerance evidence: %q", invalid.GeometryCheck.Message)
	}
}

func TestGeometricAdjacencyRemainsQAOnly(t *testing.T) {
	topology := AnalyzeGeometry(thermalGeometryPairTestDocument(0, false)).Topology
	boundary := findThermalBoundary(t, topology, "Pair A")
	if boundary.RelationKind != "adiabatic_explicit" {
		t.Fatalf("geometric adjacency changed authoritative relation to %q", boundary.RelationKind)
	}
	if !hasAdjacencyObservation(topology, "geometrically_adjacent_but_thermally_disconnected") {
		t.Fatal("disconnected geometric adjacency observation missing")
	}
}

func TestZoneEnclosureIntegrityFindsClosedAndOpenShells(t *testing.T) {
	document, err := Parse(summaryFixtureIDF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	closed := AnalyzeGeometry(document).Topology.ZoneEnclosures
	if len(closed) != 1 || !closed[0].ClosedShell || closed[0].OpenEdgeCount != 0 || closed[0].NonManifoldEdgeCount != 0 {
		t.Fatalf("closed box enclosure = %#v", closed)
	}
	if closed[0].ComputedVolume != 600 || closed[0].DeclaredVolume != 600 || closed[0].VolumeDifferencePct != 0 {
		t.Fatalf("closed box volume = computed %v declared %v difference %v", closed[0].ComputedVolume, closed[0].DeclaredVolume, closed[0].VolumeDifferencePct)
	}

	openDocument := document
	openDocument.Objects = append([]Object(nil), document.Objects...)
	filtered := openDocument.Objects[:0]
	for _, object := range openDocument.Objects {
		if isBuildingSurfaceType(object.Type) && objectName(object) == "West Wall" {
			continue
		}
		filtered = append(filtered, object)
	}
	openDocument.Objects = filtered
	open := AnalyzeGeometry(openDocument).Topology.ZoneEnclosures
	if len(open) != 1 || open[0].ClosedShell || open[0].OpenEdgeCount != 4 || len(open[0].OpenEdges) != 4 {
		t.Fatalf("open box enclosure = %#v", open)
	}
	if !hasTopologyIssueCode(AnalyzeGeometry(openDocument).Topology, "zone_shell_open") {
		t.Fatal("open shell diagnostic link missing")
	}
}

func TestThermalTopologyBuildsExplicitAndAirflowNetworkCouplings(t *testing.T) {
	topology := AnalyzeGeometry(thermalAirCouplingTestDocument()).Topology
	if len(topology.AirCouplings) != 5 {
		t.Fatalf("air coupling count = %d, want 5: %#v", len(topology.AirCouplings), topology.AirCouplings)
	}
	mixing := findThermalAirCoupling(t, topology, "zone_mixing")
	if mixing.Direction != "directed" || mixing.DesignFlowRate != 0.1 || mixing.Unit != "m3/s" || mixing.FromNodeID != "zone:zone%20b" || mixing.ToNodeID != "zone:zone%20a" {
		t.Fatalf("zone mixing coupling = %#v", mixing)
	}
	cross := findThermalAirCoupling(t, topology, "zone_cross_mixing")
	if cross.Direction != "bidirectional" || cross.DesignFlowRate != 0.2 {
		t.Fatalf("cross mixing coupling = %#v", cross)
	}
	door := findThermalAirCoupling(t, topology, "refrigeration_door_mixing")
	if door.Direction != "bidirectional" || door.FromNodeID == "" || door.ToNodeID == "" {
		t.Fatalf("door mixing coupling = %#v", door)
	}
	ventilation := findThermalAirCoupling(t, topology, "outdoor_ventilation")
	if ventilation.FromNodeID != "thermal-environment:outdoors" || ventilation.DesignFlowRate != 0.3 {
		t.Fatalf("outdoor ventilation coupling = %#v", ventilation)
	}
	afn := findThermalAirCoupling(t, topology, "airflow_network")
	if afn.SurfaceID == "" || afn.ComponentName != "Pair Crack" || afn.ToNodeID != "space:zone%20b:space%20b" || len(afn.SourceAnchors) < 3 {
		t.Fatalf("AFN coupling = %#v", afn)
	}
	if hasTopologyIssueCode(topology, "air_coupling_target_missing") {
		t.Fatal("valid air coupling fixture produced missing-target diagnostic")
	}
}

func TestConstructionAirBoundaryCreatesSingleBidirectionalCoupling(t *testing.T) {
	topology := AnalyzeGeometry(thermalAirBoundaryTestDocument()).Topology
	var airBoundaries []ThermalAirCoupling
	for _, coupling := range topology.AirCouplings {
		if coupling.CouplingKind == "construction_air_boundary" {
			airBoundaries = append(airBoundaries, coupling)
		}
	}
	if len(airBoundaries) != 1 {
		t.Fatalf("air boundary coupling count = %d, want 1: %#v", len(airBoundaries), airBoundaries)
	}
	coupling := airBoundaries[0]
	if coupling.Direction != "bidirectional" || coupling.ScheduleName != "Always On" || coupling.DesignFlowRate != 0.1 || coupling.SurfaceID == "" {
		t.Fatalf("air boundary coupling = %#v", coupling)
	}
}

func TestAirCouplingMissingTargetSurvivesWithDiagnostic(t *testing.T) {
	document := thermalAirCouplingTestDocument()
	document.Objects = append(document.Objects, Object{Type: "ZoneMixing", Fields: []Field{
		{Value: "Broken Mixing"}, {Value: "Zone A"}, {Value: "Always On"}, {Value: "Flow/Zone"}, {Value: "0.1"}, {Value: ""}, {Value: ""}, {Value: ""}, {Value: "Missing Zone"},
	}})
	document.Objects[len(document.Objects)-1].Index = len(document.Objects) - 1
	topology := AnalyzeGeometry(document).Topology
	broken := findThermalAirCouplingByName(t, topology, "Broken Mixing")
	if len(broken.DiagnosticIDs) == 0 || broken.FromNodeID == "" || !hasTopologyIssueCode(topology, "air_coupling_target_missing") {
		t.Fatalf("broken air coupling was not retained with diagnostic: %#v", broken)
	}
}

func hasAdjacencyObservation(topology ThermalTopologyReport, kind string) bool {
	for _, observation := range topology.AdjacencyObservations {
		if observation.ObservationKind == kind {
			return true
		}
	}
	return false
}

func hasTopologyIssueCode(topology ThermalTopologyReport, code string) bool {
	for _, issue := range topology.IssueLinks {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func findThermalNode(t *testing.T, topology ThermalTopologyReport, id string) ThermalTopologyNode {
	t.Helper()
	for _, node := range topology.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("thermal node %q not found", id)
	return ThermalTopologyNode{}
}

func findThermalBoundary(t *testing.T, topology ThermalTopologyReport, name string) ThermalBoundaryRecord {
	t.Helper()
	for _, boundary := range topology.Boundaries {
		if boundary.SurfaceName == name {
			return boundary
		}
	}
	t.Fatalf("thermal boundary %q not found", name)
	return ThermalBoundaryRecord{}
}

func findThermalOpening(t *testing.T, topology ThermalTopologyReport, name string) ThermalOpeningRecord {
	t.Helper()
	for _, opening := range topology.Openings {
		if opening.Name == name {
			return opening
		}
	}
	t.Fatalf("thermal opening %q not found", name)
	return ThermalOpeningRecord{}
}

func findThermalAirCoupling(t *testing.T, topology ThermalTopologyReport, kind string) ThermalAirCoupling {
	t.Helper()
	for _, coupling := range topology.AirCouplings {
		if coupling.CouplingKind == kind {
			return coupling
		}
	}
	t.Fatalf("thermal air coupling kind %q not found", kind)
	return ThermalAirCoupling{}
}

func findThermalAirCouplingByName(t *testing.T, topology ThermalTopologyReport, name string) ThermalAirCoupling {
	t.Helper()
	for _, coupling := range topology.AirCouplings {
		if coupling.ObjectName == name {
			return coupling
		}
	}
	t.Fatalf("thermal air coupling %q not found", name)
	return ThermalAirCoupling{}
}

func thermalTopologyTestDocument() Document {
	objects := []Object{
		{Type: "Version", Fields: []Field{{Value: "22.1"}}},
		{Type: "GlobalGeometryRules", Fields: []Field{{Value: "UpperLeftCorner"}, {Value: "CounterClockWise"}, {Value: "World"}, {Value: "World"}, {Value: "World"}}},
		thermalZoneObject("Zone A"),
		thermalZoneObject("Zone B"),
		{Type: "Space", Fields: []Field{{Value: "Space A"}, {Value: "Zone A"}}},
		{Type: "Space", Fields: []Field{{Value: "Space B"}, {Value: "Zone B"}}},
		thermalSurfaceObject("Pair A", "Zone A", "Space A", "Surface", "Pair B", 0),
		thermalSurfaceObject("Pair B", "Zone B", "Space B", "Surface", "Pair A", 3),
		thermalSurfaceObject("Outdoors", "Zone A", "Space A", "Outdoors", "", 6),
		thermalSurfaceObject("Ground", "Zone A", "Space A", "Ground", "", 9),
		thermalSurfaceObject("Adiabatic", "Zone A", "Space A", "Adiabatic", "", 12),
		thermalSurfaceObject("Self Surface", "Zone A", "Space A", "Surface", "Self Surface", 15),
		thermalSurfaceObject("Implicit Zone", "Zone A", "Space A", "Zone", "Zone B", 18),
		thermalSurfaceObject("Implicit Space", "Zone A", "Space A", "Space", "Space B", 21),
		thermalSurfaceObject("Foundation", "Zone A", "Space A", "Foundation", "Kiva Foundation", 24),
		thermalSurfaceObject("Ground Preprocessor", "Zone A", "Space A", "GroundFCfactorMethod", "", 27),
		thermalSurfaceObject("Other Side", "Zone A", "Space A", "OtherSideCoefficients", "OSC", 30),
		thermalSurfaceObject("Invalid", "Zone A", "Space A", "Mystery", "", 33),
		{Type: "Foundation:Kiva", Fields: []Field{{Value: "Kiva Foundation"}}},
		{Type: "SurfaceProperty:OtherSideCoefficients", Fields: []Field{{Value: "OSC"}}},
	}
	for index := range objects {
		objects[index].Index = index
	}
	return Document{Objects: objects}
}

func thermalOpeningTestDocument() Document {
	document := Document{Objects: []Object{
		{Type: "Version", Fields: []Field{{Value: "22.1"}}},
		{Type: "GlobalGeometryRules", Fields: []Field{{Value: "UpperLeftCorner"}, {Value: "CounterClockWise"}, {Value: "World"}, {Value: "World"}, {Value: "World"}}},
		thermalZoneObjectWithMultiplier("Zone A", 2),
		thermalZoneObjectWithMultiplier("Zone B", 2),
		{Type: "Space", Fields: []Field{{Value: "Space A"}, {Value: "Zone A"}}},
		{Type: "Space", Fields: []Field{{Value: "Space B"}, {Value: "Zone B"}}},
		thermalSurfaceObjectWithConstruction("Pair A", "Zone A", "Space A", "Opaque A", "Surface", "Pair B", 0),
		thermalSurfaceObjectWithConstruction("Pair B", "Zone B", "Space B", "Opaque Reverse", "Surface", "Pair A", 0),
		thermalOpeningObject("Window A", "Pair A", "Window B", 0),
		thermalOpeningObject("Window B", "Pair B", "Window A", 0),
	}}
	document.Objects = append(document.Objects, thermalConstructionTestDocument().Objects...)
	for index := range document.Objects {
		document.Objects[index].Index = index
	}
	return document
}

func thermalGeometryPairTestDocument(shift float64, declared bool) Document {
	document := thermalOpeningTestDocument()
	for objectIndex := range document.Objects {
		object := &document.Objects[objectIndex]
		if !isBuildingSurfaceType(object.Type) {
			continue
		}
		switch objectName(*object) {
		case "Pair A":
			if !declared {
				object.Fields[5].Value = "Adiabatic"
				object.Fields[6].Value = ""
			}
		case "Pair B":
			reverseThermalSurfaceVertices(object, shift)
			if !declared {
				object.Fields[5].Value = "Adiabatic"
				object.Fields[6].Value = ""
			}
		}
	}
	return document
}

func thermalAirCouplingTestDocument() Document {
	document := thermalGeometryPairTestDocument(0, true)
	document.Objects = append(document.Objects,
		Object{Type: "ZoneMixing", Fields: []Field{
			{Value: "Mix B to A"}, {Value: "Zone A"}, {Value: "Always On"}, {Value: "Flow/Zone"}, {Value: "0.1"}, {Value: ""}, {Value: ""}, {Value: ""}, {Value: "Zone B"},
		}},
		Object{Type: "ZoneCrossMixing", Fields: []Field{
			{Value: "Cross A B"}, {Value: "Zone A"}, {Value: "Always On"}, {Value: "Flow/Zone"}, {Value: "0.2"}, {Value: ""}, {Value: ""}, {Value: ""}, {Value: "Zone B"},
		}},
		Object{Type: "ZoneRefrigerationDoorMixing", Fields: []Field{
			{Value: "Door A B"}, {Value: "Zone A"}, {Value: "Zone B"}, {Value: "Always On"}, {Value: "2"}, {Value: "2"}, {Value: "None"},
		}},
		Object{Type: "ZoneVentilation:DesignFlowRate", Fields: []Field{
			{Value: "Vent A"}, {Value: "Zone A"}, {Value: "Always On"}, {Value: "Flow/Zone"}, {Value: "0.3"}, {Value: ""}, {Value: ""}, {Value: ""},
		}},
		Object{Type: "AirflowNetwork:MultiZone:Zone", Fields: []Field{{Value: "Zone A"}}},
		Object{Type: "AirflowNetwork:MultiZone:Zone", Fields: []Field{{Value: "Zone B"}}},
		Object{Type: "AirflowNetwork:MultiZone:Surface", Fields: []Field{{Value: "Pair A"}, {Value: "Pair Crack"}, {Value: ""}}},
		Object{Type: "AirflowNetwork:MultiZone:Surface:Crack", Fields: []Field{{Value: "Pair Crack"}, {Value: "0.001"}, {Value: "0.65"}}},
	)
	for index := range document.Objects {
		document.Objects[index].Index = index
	}
	return document
}

func thermalAirBoundaryTestDocument() Document {
	document := thermalGeometryPairTestDocument(0, true)
	for index := range document.Objects {
		object := &document.Objects[index]
		if isBuildingSurfaceType(object.Type) && (objectName(*object) == "Pair A" || objectName(*object) == "Pair B") {
			object.Fields[2].Value = "Open Partition"
		}
		if strings.EqualFold(object.Type, "Zone") {
			for len(object.Fields) <= 8 {
				object.Fields = append(object.Fields, Field{})
			}
			object.Fields[8].Value = "360"
		}
	}
	document.Objects = append(document.Objects, Object{Type: "Construction:AirBoundary", Fields: []Field{
		{Value: "Open Partition"}, {Value: "SimpleMixing"}, {Value: "1"}, {Value: "Always On"},
	}})
	for index := range document.Objects {
		document.Objects[index].Index = index
	}
	return document
}

func reverseThermalSurfaceVertices(object *Object, shift float64) {
	const vertexStart = 11
	const vertexCount = 4
	vertices := make([][]Field, vertexCount)
	for index := 0; index < vertexCount; index++ {
		offset := vertexStart + index*3
		vertices[index] = append([]Field(nil), object.Fields[offset:offset+3]...)
	}
	for index := 0; index < vertexCount; index++ {
		offset := vertexStart + index*3
		vertex := vertices[vertexCount-1-index]
		x, _ := strconv.ParseFloat(vertex[0].Value, 64)
		vertex[0].Value = formatNumber(x + shift)
		copy(object.Fields[offset:offset+3], vertex)
	}
}

func thermalConstructionTestDocument() Document {
	objects := []Object{
		{Type: "Material:NoMass", Fields: []Field{{Value: "Insulation"}, {Value: "Rough"}, {Value: "0.5"}}},
		{Type: "Material:AirGap", Fields: []Field{{Value: "Air Gap"}, {Value: "0.2"}}},
		{Type: "WindowMaterial:SimpleGlazingSystem", Fields: []Field{{Value: "Simple Glass"}, {Value: "2"}, {Value: "0.4"}, {Value: "0.6"}}},
		{Type: "Construction", Fields: []Field{{Value: "Opaque A"}, {Value: "Insulation"}, {Value: "Air Gap"}}},
		{Type: "Construction", Fields: []Field{{Value: "Opaque Reverse"}, {Value: "Air Gap"}, {Value: "Insulation"}}},
		{Type: "Construction", Fields: []Field{{Value: "Window Construction"}, {Value: "Simple Glass"}}},
		{Type: "Construction:CfactorUndergroundWall", Fields: []Field{{Value: "C Factor Construction"}, {Value: "0.5"}, {Value: "3"}}},
		{Type: "Construction:FfactorGroundFloor", Fields: []Field{{Value: "F Factor Construction"}, {Value: "0.8"}, {Value: "40"}, {Value: "10"}}},
		{Type: "Construction:ComplexFenestrationState", Fields: []Field{{Value: "Complex State"}}},
	}
	for index := range objects {
		objects[index].Index = index
	}
	return Document{Objects: objects}
}

func thermalZoneObjectWithMultiplier(name string, multiplier float64) Object {
	zone := thermalZoneObject(name)
	zone.Fields[6].Value = formatNumber(multiplier)
	return zone
}

func thermalSurfaceObjectWithConstruction(name string, zoneName string, spaceName string, constructionName string, boundary string, boundaryObject string, x float64) Object {
	surface := thermalSurfaceObject(name, zoneName, spaceName, boundary, boundaryObject, x)
	surface.Fields[2].Value = constructionName
	return surface
}

func thermalOpeningObject(name string, baseSurfaceName string, counterpartName string, x float64) Object {
	return Object{Type: "FenestrationSurface:Detailed", Fields: []Field{
		{Value: name},
		{Value: "Window"},
		{Value: "Window Construction"},
		{Value: baseSurfaceName},
		{Value: counterpartName},
		{Value: "0.5"},
		{Value: ""},
		{Value: "3"},
		{Value: "4"},
		{Value: formatNumber(x + 0.5)}, {Value: "0"}, {Value: "0.5"},
		{Value: formatNumber(x + 0.5)}, {Value: "0"}, {Value: "1.5"},
		{Value: formatNumber(x + 1.5)}, {Value: "0"}, {Value: "1.5"},
		{Value: formatNumber(x + 1.5)}, {Value: "0"}, {Value: "0.5"},
	}}
}

func thermalZoneObject(name string) Object {
	return Object{Type: "Zone", Fields: []Field{
		{Value: name},
		{Value: "0"},
		{Value: "0"},
		{Value: "0"},
		{Value: "0"},
		{Value: "1"},
		{Value: "1"},
	}}
}

func thermalSurfaceObject(name string, zoneName string, spaceName string, boundary string, boundaryObject string, x float64) Object {
	return Object{Type: "BuildingSurface:Detailed", Fields: []Field{
		{Value: name},
		{Value: "Wall"},
		{Value: ""},
		{Value: zoneName},
		{Value: spaceName},
		{Value: boundary},
		{Value: boundaryObject},
		{Value: "NoSun"},
		{Value: "NoWind"},
		{Value: "0.5"},
		{Value: "4"},
		{Value: formatNumber(x)}, {Value: "0"}, {Value: "0"},
		{Value: formatNumber(x)}, {Value: "0"}, {Value: "2"},
		{Value: formatNumber(x + 2)}, {Value: "0"}, {Value: "2"},
		{Value: formatNumber(x + 2)}, {Value: "0"}, {Value: "0"},
	}}
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
