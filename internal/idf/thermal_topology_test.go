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
