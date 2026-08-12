package idf

import (
	"reflect"
	"testing"
)

func TestThermalFixture250ExteriorEnclosure(t *testing.T) {
	document, err := Parse(metricsFixtureIDF)
	if err != nil {
		t.Fatal(err)
	}
	for index := range document.Objects {
		object := &document.Objects[index]
		switch {
		case stringsEqualFold(object.Type, "Zone"):
			object.Fields[6].Value = "2"
		case isBuildingSurfaceType(object.Type):
			object.Fields[2].Value = "Opaque A"
		case isFenestrationType(object.Type):
			object.Fields[2].Value = "Window Construction"
		}
	}
	document.Objects = append(document.Objects, thermalConstructionTestDocument().Objects...)
	reindexThermalFixture(&document)
	geometry := AnalyzeGeometry(document)
	topology := geometry.Topology
	if geometry.ZoneCount != 1 || geometry.SurfaceCount != 6 || geometry.WindowCount == 0 {
		t.Fatalf("exterior fixture geometry counts = zones %d surfaces %d windows %d", geometry.ZoneCount, geometry.SurfaceCount, geometry.WindowCount)
	}
	exterior := findThermalConnection(t, topology, "exterior")
	ground := findThermalConnection(t, topology, "ground")
	if exterior.PhysicalGrossArea <= 0 || exterior.EffectiveGrossArea != exterior.PhysicalGrossArea*2 || !exterior.HasUA || !ground.HasUA {
		t.Fatalf("exterior/ground areas or UA missing: exterior %#v ground %#v", exterior, ground)
	}
	if len(topology.ZoneEnclosures) != 1 || !topology.ZoneEnclosures[0].ClosedShell {
		t.Fatalf("exterior enclosure should be closed: %#v", topology.ZoneEnclosures)
	}
	signature := findZoneThermalSignature(t, topology, "Zone 1")
	if signature.ExteriorArea <= 0 || signature.GroundArea <= 0 || !signature.HasTotalUA || len(topology.Matrix) == 0 {
		t.Fatalf("exterior signature/matrix missing: signature %#v matrix %d", signature, len(topology.Matrix))
	}
}

func TestThermalFixture251ReciprocalInterzone(t *testing.T) {
	topology := AnalyzeGeometry(thermalGeometryPairTestDocument(0, true)).Topology
	connection := findThermalConnection(t, topology, "interzone_explicit_surface")
	if connection.SurfaceCount != 1 || len(connection.BoundaryIDs) != 2 || connection.OpeningCount != 1 || len(connection.OpeningIDs) != 1 {
		t.Fatalf("reciprocal surfaces/openings were not compacted once: %#v", connection)
	}
	for _, boundaryName := range []string{"Pair A", "Pair B"} {
		if boundary := findThermalBoundary(t, topology, boundaryName); boundary.ConstructionStatus != "reverse_layer_equivalent" {
			t.Fatalf("%s construction status = %q", boundaryName, boundary.ConstructionStatus)
		}
	}
}

func TestThermalFixture252MissingCounterpart(t *testing.T) {
	document := thermalGeometryPairTestDocument(0, true)
	thermalFixtureObject(t, &document, "BuildingSurface:Detailed", "Pair A").Fields[6].Value = "Missing Pair"
	assertThermalFixtureIssue(t, document, thermalDiagnosticSurfaceCounterpartMissing)
}

func TestThermalFixture252OneWayCounterpart(t *testing.T) {
	document := thermalGeometryPairTestDocument(0, true)
	pairB := thermalFixtureObject(t, &document, "BuildingSurface:Detailed", "Pair B")
	pairB.Fields[5].Value = "Outdoors"
	pairB.Fields[6].Value = ""
	assertThermalFixtureIssue(t, document, thermalDiagnosticSurfaceCounterpartOneWay)
}

func TestThermalFixture252DuplicateCounterpart(t *testing.T) {
	document := thermalGeometryPairTestDocument(0, true)
	duplicate := cloneThermalFixtureObject(*thermalFixtureObject(t, &document, "BuildingSurface:Detailed", "Pair B"))
	document.Objects = append(document.Objects, duplicate)
	reindexThermalFixture(&document)
	assertThermalFixtureIssue(t, document, thermalDiagnosticSurfaceCounterpartDuplicate)
}

func TestThermalFixture252SameZoneSurfacePair(t *testing.T) {
	document := thermalGeometryPairTestDocument(0, true)
	pairB := thermalFixtureObject(t, &document, "BuildingSurface:Detailed", "Pair B")
	pairB.Fields[3].Value = "Zone A"
	pairB.Fields[4].Value = "Space A"
	assertThermalFixtureIssue(t, document, thermalDiagnosticSurfacePairZoneMismatch)
}

func TestThermalFixture252AreaMismatch(t *testing.T) {
	document := thermalGeometryPairTestDocument(0, true)
	pairB := thermalFixtureObject(t, &document, "BuildingSurface:Detailed", "Pair B")
	for fieldIndex := 11; fieldIndex < len(pairB.Fields); fieldIndex += 3 {
		if pairB.Fields[fieldIndex].Value == "2" {
			pairB.Fields[fieldIndex].Value = "3"
		}
	}
	assertThermalFixtureIssue(t, document, thermalDiagnosticSurfacePairAreaMismatch)
}

func TestThermalFixture252PlaneMismatch(t *testing.T) {
	document := thermalGeometryPairTestDocument(0, true)
	pairB := thermalFixtureObject(t, &document, "BuildingSurface:Detailed", "Pair B")
	for fieldIndex := 12; fieldIndex < len(pairB.Fields); fieldIndex += 3 {
		pairB.Fields[fieldIndex].Value = "0.1"
	}
	assertThermalFixtureIssue(t, document, thermalDiagnosticSurfacePairPlaneMismatch)
}

func TestThermalFixture252NormalMismatch(t *testing.T) {
	document := thermalGeometryPairTestDocument(0, true)
	reverseThermalSurfaceVertices(thermalFixtureObject(t, &document, "BuildingSurface:Detailed", "Pair B"), 0)
	assertThermalFixtureIssue(t, document, thermalDiagnosticSurfacePairNormalMismatch)
}

func TestThermalFixture252OverlapMismatch(t *testing.T) {
	assertThermalFixtureIssue(t, thermalGeometryPairTestDocument(0.5, true), thermalDiagnosticSurfacePairOverlapMismatch)
}

func TestThermalFixture252ConstructionMismatch(t *testing.T) {
	document := thermalGeometryPairTestDocument(0, true)
	thermalFixtureObject(t, &document, "BuildingSurface:Detailed", "Pair B").Fields[2].Value = "C Factor Construction"
	assertThermalFixtureIssue(t, document, thermalDiagnosticSurfacePairConstructionMismatch)
}

func TestThermalFixture252OpeningCounterpartMismatch(t *testing.T) {
	document := thermalGeometryPairTestDocument(0, true)
	thermalFixtureObject(t, &document, "FenestrationSurface:Detailed", "Window B").Fields[4].Value = "Missing Window"
	topology := AnalyzeGeometry(document).Topology
	if !hasTopologyIssueCode(topology, thermalDiagnosticFenestrationCounterpartMissing) || !hasTopologyIssueCode(topology, thermalDiagnosticFenestrationCounterpartOneWay) {
		t.Fatalf("opening counterpart diagnostics missing: %#v", topology.IssueLinks)
	}
}

func TestThermalFixture253ImplicitZoneAndSpaceBoundaries(t *testing.T) {
	topology := AnalyzeGeometry(thermalTopologyTestDocument()).Topology
	for _, test := range []struct {
		name, relation, target string
	}{
		{name: "Implicit Zone", relation: "interzone_implicit_zone", target: "zone"},
		{name: "Implicit Space", relation: "interspace_implicit", target: "space"},
	} {
		boundary := findThermalBoundary(t, topology, test.name)
		if boundary.RelationKind != test.relation || boundary.TargetKind != test.target || boundary.TargetID == "" || !boundary.VirtualCounterpart {
			t.Errorf("implicit boundary %q = %#v", test.name, boundary)
		}
		for _, diagnosticID := range boundary.DiagnosticIDs {
			if issue := thermalFixtureIssueByID(topology, diagnosticID); issue.Code == thermalDiagnosticSurfaceCounterpartMissing {
				t.Errorf("implicit boundary %q produced false missing-surface issue", test.name)
			}
		}
	}
}

func TestThermalFixture254AdiabaticDoesNotCreateInterzoneEdge(t *testing.T) {
	topology := AnalyzeGeometry(thermalGeometryPairTestDocument(0, false)).Topology
	for _, boundaryName := range []string{"Pair A", "Pair B"} {
		if boundary := findThermalBoundary(t, topology, boundaryName); boundary.RelationKind != "adiabatic_explicit" {
			t.Fatalf("adiabatic boundary %q became %q", boundaryName, boundary.RelationKind)
		}
	}
	if !hasAdjacencyObservation(topology, "geometrically_adjacent_but_thermally_disconnected") {
		t.Fatal("adiabatic geometric adjacency observation missing")
	}
	for _, connection := range topology.Connections {
		if connection.RelationKind == "interzone_explicit_surface" {
			t.Fatalf("adiabatic fixture created false interzone edge: %#v", connection)
		}
	}
}

func TestThermalFixture255GroundFoundationAndOtherSideFamilies(t *testing.T) {
	document := thermalTopologyTestDocument()
	document.Objects = append(document.Objects,
		thermalSurfaceObject("Ground Slab", "Zone A", "Space A", "GroundSlabPreprocessorCore", "", 36),
		thermalSurfaceObject("Ground Basement", "Zone A", "Space A", "GroundBasementPreprocessorAverageWall", "", 39),
		thermalSurfaceObject("Other Side Model", "Zone A", "Space A", "OtherSideConditionsModel", "OSCM", 42),
		Object{Type: "SurfaceProperty:OtherSideConditionsModel", Fields: []Field{{Value: "OSCM"}}},
		thermalSurfaceObject("Missing Foundation", "Zone A", "Space A", "Foundation", "Missing Kiva", 45),
	)
	reindexThermalFixture(&document)
	topology := AnalyzeGeometry(document).Topology
	for _, test := range []struct {
		name, relation string
	}{
		{name: "Ground", relation: "ground"},
		{name: "Ground Preprocessor", relation: "ground_preprocessor"},
		{name: "Ground Slab", relation: "ground_preprocessor"},
		{name: "Ground Basement", relation: "ground_preprocessor"},
		{name: "Foundation", relation: "foundation"},
		{name: "Other Side", relation: "other_side_coefficients"},
		{name: "Other Side Model", relation: "other_side_conditions_model"},
	} {
		if boundary := findThermalBoundary(t, topology, test.name); boundary.RelationKind != test.relation || boundary.TargetID == "" {
			t.Errorf("boundary family %q = %#v", test.name, boundary)
		}
	}
	missing := findThermalBoundary(t, topology, "Missing Foundation")
	if missing.RelationKind != "invalid" || !hasTopologyIssueCode(topology, thermalDiagnosticMissingBoundaryTarget) {
		t.Fatalf("missing named boundary target was not diagnosed: %#v", missing)
	}
}

func TestThermalFixture256SimpleAndTransformedGeometry(t *testing.T) {
	simpleDocument, err := Parse(rectangularGeometryFixture)
	if err != nil {
		t.Fatal(err)
	}
	simpleGeometry := AnalyzeGeometry(simpleDocument)
	simpleBoundary := findThermalBoundary(t, simpleGeometry.Topology, "Simple Wall")
	if simpleBoundary.RelationKind != "exterior" || simpleBoundary.PhysicalGrossArea != 12 {
		t.Fatalf("simple surface topology = %#v", simpleBoundary)
	}

	relativeDocument, err := Parse(geometryCoordinateFixture("Relative", "0,0,0, 2,0,0, 2,0,2, 0,0,2", "0.5,0,0.5, 1.5,0,0.5, 1.5,0,1.5, 0.5,0,1.5"))
	if err != nil {
		t.Fatal(err)
	}
	worldDocument, err := Parse(geometryCoordinateFixture("World", "-20,10,3, -20,12,3, -20,12,5, -20,10,5", "-20,10.5,3.5, -20,11.5,3.5, -20,11.5,4.5, -20,10.5,4.5"))
	if err != nil {
		t.Fatal(err)
	}
	thermalFixtureObject(t, &relativeDocument, "Zone", "Zone A").Fields[6].Value = "3"
	thermalFixtureObject(t, &worldDocument, "Zone", "Zone A").Fields[6].Value = "3"
	relative := AnalyzeGeometry(relativeDocument)
	world := AnalyzeGeometry(worldDocument)
	if !reflect.DeepEqual(findGeometrySurface(t, relative, "Space Wall").WorldVertices, findGeometrySurface(t, world, "Space Wall").WorldVertices) {
		t.Fatal("relative building/zone transforms did not match equivalent world geometry")
	}
	boundary := findThermalBoundary(t, relative.Topology, "Space Wall")
	if boundary.EffectiveGrossArea != boundary.PhysicalGrossArea*3 {
		t.Fatalf("physical/effective transformed area split = %v/%v", boundary.PhysicalGrossArea, boundary.EffectiveGrossArea)
	}
}

func TestThermalFixture257AirCouplingFamilies(t *testing.T) {
	topology := AnalyzeGeometry(thermalAirCouplingTestDocument()).Topology
	for _, kind := range []string{"zone_mixing", "zone_cross_mixing", "refrigeration_door_mixing", "airflow_network"} {
		if coupling := findThermalAirCoupling(t, topology, kind); coupling.FromNodeID == "" || coupling.ToNodeID == "" {
			t.Errorf("air coupling %q unresolved: %#v", kind, coupling)
		}
	}
	if findThermalAirCoupling(t, topology, "zone_mixing").Direction != "directed" || findThermalAirCoupling(t, topology, "zone_cross_mixing").Direction != "bidirectional" {
		t.Fatal("air coupling directions do not preserve source semantics")
	}
	if coupling := findThermalAirCoupling(t, AnalyzeGeometry(thermalAirBoundaryTestDocument()).Topology, "construction_air_boundary"); coupling.Direction != "bidirectional" {
		t.Fatalf("Construction:AirBoundary coupling = %#v", coupling)
	}

	broken := thermalAirCouplingTestDocument()
	broken.Objects = append(broken.Objects,
		thermalSurfaceObject("AFN Outdoors", "Zone A", "Space A", "Outdoors", "", 50),
		Object{Type: "AirflowNetwork:MultiZone:Surface", Fields: []Field{{Value: "AFN Outdoors"}, {Value: "Outdoor Crack"}, {Value: ""}}},
		Object{Type: "AirflowNetwork:MultiZone:Surface:Crack", Fields: []Field{{Value: "Outdoor Crack"}, {Value: "0.001"}, {Value: "0.65"}}},
		Object{Type: "AirflowNetwork:MultiZone:Surface", Fields: []Field{{Value: "Missing Surface"}, {Value: "Missing Component"}, {Value: ""}}},
		Object{Type: "ZoneMixing", Fields: []Field{
			{Value: "Missing Zone Mixing"}, {Value: "Zone A"}, {Value: "Always On"}, {Value: "Flow/Zone"}, {Value: "0.1"}, {Value: ""}, {Value: ""}, {Value: ""}, {Value: "Missing Zone"},
		}},
	)
	reindexThermalFixture(&broken)
	brokenTopology := AnalyzeGeometry(broken).Topology
	outdoorBoundary := findThermalBoundary(t, brokenTopology, "AFN Outdoors")
	hasAFNOutdoors := false
	for _, coupling := range brokenTopology.AirCouplings {
		if coupling.CouplingKind == "airflow_network" && coupling.ToNodeID == outdoorBoundary.TargetID {
			hasAFNOutdoors = true
		}
	}
	if !hasAFNOutdoors || !hasTopologyIssueCode(brokenTopology, thermalDiagnosticAirCouplingTargetMissing) || !hasTopologyIssueCode(brokenTopology, thermalDiagnosticAirflowNetworkSurfaceMissing) || !hasTopologyIssueCode(brokenTopology, thermalDiagnosticAirflowNetworkComponentMissing) {
		t.Fatalf("AFN outdoors/missing target/component coverage = outdoors %v issues %#v", hasAFNOutdoors, brokenTopology.IssueLinks)
	}
}

func TestThermalFixture258EnclosureIntegrityVariants(t *testing.T) {
	document, err := Parse(metricsFixtureIDF)
	if err != nil {
		t.Fatal(err)
	}
	closed := AnalyzeGeometry(document).Topology.ZoneEnclosures[0]
	if !closed.ClosedShell || closed.ComputedVolume != closed.DeclaredVolume {
		t.Fatalf("closed enclosure = %#v", closed)
	}

	missingWall := cloneThermalFixtureDocument(document)
	filtered := missingWall.Objects[:0]
	for _, object := range missingWall.Objects {
		if isBuildingSurfaceType(object.Type) && objectName(object) == "West Wall" {
			continue
		}
		filtered = append(filtered, object)
	}
	missingWall.Objects = filtered
	if enclosure := AnalyzeGeometry(missingWall).Topology.ZoneEnclosures[0]; enclosure.ClosedShell || enclosure.OpenEdgeCount == 0 {
		t.Fatalf("missing-wall enclosure = %#v", enclosure)
	}

	nonManifold := cloneThermalFixtureDocument(document)
	duplicate := cloneThermalFixtureObject(*thermalFixtureObject(t, &nonManifold, "BuildingSurface:Detailed", "South Wall"))
	duplicate.Fields[0].Value = "Duplicate South Wall"
	nonManifold.Objects = append(nonManifold.Objects, duplicate)
	reindexThermalFixture(&nonManifold)
	if enclosure := AnalyzeGeometry(nonManifold).Topology.ZoneEnclosures[0]; enclosure.NonManifoldEdgeCount == 0 || !hasTopologyIssueCode(AnalyzeGeometry(nonManifold).Topology, thermalDiagnosticZoneShellNonManifold) {
		t.Fatalf("non-manifold enclosure = %#v", enclosure)
	}

	volumeMismatch := cloneThermalFixtureDocument(document)
	thermalFixtureObject(t, &volumeMismatch, "Zone", "Zone 1").Fields[8].Value = "900"
	if enclosure := AnalyzeGeometry(volumeMismatch).Topology.ZoneEnclosures[0]; enclosure.VolumeDifferencePct == 0 || !hasTopologyIssueCode(AnalyzeGeometry(volumeMismatch).Topology, thermalDiagnosticZoneVolumeMismatch) {
		t.Fatalf("volume mismatch enclosure = %#v", enclosure)
	}
}

func assertThermalFixtureIssue(t *testing.T, document Document, code string) {
	t.Helper()
	topology := AnalyzeGeometry(document).Topology
	if !hasTopologyIssueCode(topology, code) {
		t.Fatalf("fixture did not produce diagnostic %q: %#v", code, topology.IssueLinks)
	}
}

func thermalFixtureIssueByID(topology ThermalTopologyReport, id string) ThermalTopologyIssueLink {
	for _, issue := range topology.IssueLinks {
		if issue.ID == id {
			return issue
		}
	}
	return ThermalTopologyIssueLink{}
}

func thermalFixtureObject(t *testing.T, document *Document, objectType string, name string) *Object {
	t.Helper()
	for index := range document.Objects {
		object := &document.Objects[index]
		if stringsEqualFold(object.Type, objectType) && stringsEqualFold(objectName(*object), name) {
			return object
		}
	}
	t.Fatalf("fixture object %s/%s not found", objectType, name)
	return nil
}

func stringsEqualFold(left string, right string) bool {
	return normalizeName(left) == normalizeName(right)
}

func cloneThermalFixtureObject(object Object) Object {
	object.Fields = append([]Field(nil), object.Fields...)
	return object
}

func cloneThermalFixtureDocument(document Document) Document {
	clone := Document{Objects: make([]Object, len(document.Objects))}
	for index, object := range document.Objects {
		clone.Objects[index] = cloneThermalFixtureObject(object)
	}
	return clone
}

func reindexThermalFixture(document *Document) {
	for index := range document.Objects {
		document.Objects[index].Index = index
	}
}
