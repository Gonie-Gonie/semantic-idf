package idf

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
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
	outdoorBoundary := findThermalBoundary(t, topology, "Outdoors")
	outdoors := findThermalNode(t, topology, outdoorBoundary.TargetID)
	if outdoors.EntityID != "" || len(outdoors.SourceAnchors) == 0 {
		t.Fatalf("virtual outdoors node must expose source anchors without a fake entity: %#v", outdoors)
	}
	if outdoorBoundary.TargetName != outdoors.Label || !strings.HasPrefix(outdoors.Kind, "outdoors_") {
		t.Fatalf("surface outdoors target must preserve its exposure: boundary %#v node %#v", outdoorBoundary, outdoors)
	}
}

func TestThermalOutdoorsExposureTargetClassifiesDirectionAndSurfaceFamily(t *testing.T) {
	tests := []struct {
		name      string
		boundary  ThermalBoundaryRecord
		wantKind  string
		wantLabel string
	}{
		{name: "north", boundary: ThermalBoundaryRecord{SurfaceType: "Wall", Orientation: " North "}, wantKind: "outdoors_north", wantLabel: "Outdoor N"},
		{name: "east", boundary: ThermalBoundaryRecord{SurfaceType: "Wall", Orientation: "East"}, wantKind: "outdoors_east", wantLabel: "Outdoor E"},
		{name: "south", boundary: ThermalBoundaryRecord{SurfaceType: "Wall", Orientation: "South"}, wantKind: "outdoors_south", wantLabel: "Outdoor S"},
		{name: "west", boundary: ThermalBoundaryRecord{SurfaceType: "Wall", Orientation: "West"}, wantKind: "outdoors_west", wantLabel: "Outdoor W"},
		{name: "roof", boundary: ThermalBoundaryRecord{SurfaceType: "RoofCeiling", Orientation: "North"}, wantKind: "outdoors_roof", wantLabel: "Outdoor Roof"},
		{name: "ceiling", boundary: ThermalBoundaryRecord{SurfaceType: "Ceiling", Orientation: "South"}, wantKind: "outdoors_roof", wantLabel: "Outdoor Roof"},
		{name: "floor", boundary: ThermalBoundaryRecord{SurfaceType: "Floor", Orientation: "North"}, wantKind: "outdoors_floor", wantLabel: "Outdoor Floor"},
		{name: "unknown", boundary: ThermalBoundaryRecord{SurfaceType: "Wall"}, wantKind: "outdoors", wantLabel: "Outdoors"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind, label := thermalOutdoorsExposureTarget(test.boundary)
			if kind != test.wantKind || label != test.wantLabel {
				t.Fatalf("thermalOutdoorsExposureTarget(%#v) = %q/%q, want %q/%q", test.boundary, kind, label, test.wantKind, test.wantLabel)
			}
		})
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
	invalidConnection := findThermalConnection(t, topology, "invalid")
	if !invalidConnection.QAOnly || invalidConnection.HasUA || len(invalidConnection.BoundaryIDs) != 1 {
		t.Fatalf("invalid boundary QA-only connection = %#v", invalidConnection)
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
	document := thermalGeometryPairTestDocument(0, false)
	topology := AnalyzeGeometry(document).Topology
	boundary := findThermalBoundary(t, topology, "Pair A")
	if boundary.RelationKind != "adiabatic_explicit" {
		t.Fatalf("geometric adjacency changed authoritative relation to %q", boundary.RelationKind)
	}
	if !hasAdjacencyObservation(topology, "geometrically_adjacent_but_thermally_disconnected") {
		t.Fatal("disconnected geometric adjacency observation missing")
	}
	for _, diagnostic := range AnalyzeDiagnostics(document) {
		if strings.HasPrefix(diagnostic.Code, "surface_pair_") {
			t.Fatalf("geometric adjacency observation became diagnostic %#v", diagnostic)
		}
	}
}

func TestTopologyDiagnosticsReuseIssueIdentityAndEvidence(t *testing.T) {
	document := thermalGeometryPairTestDocument(0.5, true)
	topology := AnalyzeGeometry(document).Topology
	var issue ThermalTopologyIssueLink
	for _, candidate := range topology.IssueLinks {
		if candidate.Code == thermalDiagnosticSurfacePairOverlapMismatch {
			issue = candidate
			break
		}
	}
	if issue.ID == "" {
		t.Fatal("surface pair overlap issue missing")
	}
	for _, diagnostic := range AnalyzeDiagnostics(document) {
		if diagnostic.ID != issue.ID {
			continue
		}
		if diagnostic.Code != issue.Code || diagnostic.Source != "energyplus_rule" || diagnostic.Field == "" || diagnostic.Evidence == "" || len(diagnostic.RelatedEntityIDs) == 0 {
			t.Fatalf("topology diagnostic lost shared identity or evidence: issue %#v diagnostic %#v", issue, diagnostic)
		}
		return
	}
	t.Fatalf("Diagnose result does not contain topology issue ID %q", issue.ID)
}

func TestAnalysisSessionBuildsThermalTopologyOnce(t *testing.T) {
	document := thermalTopologyTestDocument()
	session := newAnalysisSession(NewDocumentIndex(document))
	wantCacheKey := newThermalGeometryCacheKey(NewDocumentIndex(document)).String()
	if session.cacheKey == "" || session.cacheKey != wantCacheKey {
		t.Fatalf("analysis cache key = %q, want %q", session.cacheKey, wantCacheKey)
	}

	var waitGroup sync.WaitGroup
	for _, work := range []func(){
		func() { _ = session.Geometry() },
		func() { _ = session.Profile() },
		func() { _ = session.Diagnostics() },
		func() { _ = session.HVAC() },
		func() { _ = session.Geometry() },
	} {
		waitGroup.Add(1)
		go func(work func()) {
			defer waitGroup.Done()
			work()
		}(work)
	}
	waitGroup.Wait()
	if session.geometryBuildCount != 1 {
		t.Fatalf("thermal topology build count = %d, want 1", session.geometryBuildCount)
	}
}

func TestThermalGeometryCacheSharesReportsArtifactsAndFlights(t *testing.T) {
	document := thermalTopologyTestDocument()
	index := NewDocumentIndex(document)
	key := newThermalGeometryCacheKey(index)
	if key.DocumentTextHash == "" || key.SchemaAdapterVersion == "" || key.TopologySchemaVersion != thermalTopologySchema || key.GeometryTransformVersion != thermalGeometryTransformVersion {
		t.Fatalf("incomplete geometry cache key: %#v", key)
	}

	cache := newThermalGeometryAnalysisCache(4)
	const callers = 8
	entries := make(chan thermalGeometryCacheEntry, callers)
	var waitGroup sync.WaitGroup
	for caller := 0; caller < callers; caller++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			entry, _ := cache.getOrCompute(index)
			entries <- entry
		}()
	}
	waitGroup.Wait()
	close(entries)
	if cache.buildCount != 1 {
		t.Fatalf("concurrent geometry cache build count = %d, want 1", cache.buildCount)
	}
	for entry := range entries {
		if entry.report.Topology.SourceModelHash == "" || entry.report.Topology.SourceModelHash != entry.key.DocumentTextHash {
			t.Fatalf("cached geometry and topology were not restored together: %#v", entry.key)
		}
		if entry.artifacts.WorldGeometryDescriptor.SurfaceCount != entry.report.SurfaceCount || len(entry.artifacts.SpatialAdjacencyIndex) == 0 {
			t.Fatalf("shared geometry artifacts were not cached: %#v", entry.artifacts.WorldGeometryDescriptor)
		}
	}

	changed, err := Parse(document.String())
	if err != nil {
		t.Fatalf("Parse(document.String()) error = %v", err)
	}
	changed.Objects[0].Fields[0].Value += " edited"
	changedIndex := NewDocumentIndex(changed)
	if newThermalGeometryCacheKey(changedIndex).DocumentTextHash == key.DocumentTextHash {
		t.Fatal("field edit did not change geometry cache document hash")
	}
	if _, hit := cache.getOrCompute(changedIndex); hit || cache.buildCount != 2 {
		t.Fatalf("field edit should populate a new cache entry: hit=%v builds=%d", hit, cache.buildCount)
	}
}

func TestAnalyzeGeometryTimedReportsTransformAndTopologyStages(t *testing.T) {
	stages := map[string]bool{}
	report := AnalyzeGeometryFromIndexTimed(NewDocumentIndex(thermalTopologyTestDocument()), func(stage string, _ time.Duration) {
		stages[stage] = true
	})
	if report.Topology.Schema != thermalTopologySchema {
		t.Fatalf("timed geometry omitted topology: %#v", report.Topology.Stats)
	}
	for _, stage := range []string{"geometry_transform", "topology"} {
		if !stages[stage] {
			t.Fatalf("timed geometry omitted %q stage", stage)
		}
	}
}

func TestThermalTopologyStableIDsSurviveReparse(t *testing.T) {
	document := thermalAirCouplingTestDocument()
	first := AnalyzeGeometry(document).Topology
	reparsed, err := Parse(document.String())
	if err != nil {
		t.Fatalf("Parse(document.String()) error = %v", err)
	}
	second := AnalyzeGeometry(reparsed).Topology
	if got, want := thermalTopologyStableIDs(second), thermalTopologyStableIDs(first); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("stable topology IDs changed after reparse:\nfirst:  %q\nsecond: %q", want, got)
	}
	for _, boundary := range first.Boundaries {
		if !strings.HasPrefix(boundary.ID, "thermal-boundary:"+boundary.SurfaceEntityID) {
			t.Errorf("boundary ID does not follow contract: %#v", boundary)
		}
		if boundary.PairID != "" && !strings.HasPrefix(boundary.PairID, "thermal-interface:") {
			t.Errorf("interface ID does not follow contract: %q", boundary.PairID)
		}
	}
	for _, connection := range first.Connections {
		want := thermalConnectionID(connection.FromNodeID, connection.ToNodeID, connection.RelationKind)
		if connection.ID != want {
			t.Errorf("connection ID = %q, want %q", connection.ID, want)
		}
	}
	for _, coupling := range first.AirCouplings {
		if !strings.HasPrefix(coupling.ID, "thermal-air-coupling:source-object:") {
			t.Errorf("air coupling ID does not follow contract: %q", coupling.ID)
		}
	}
	for _, cell := range first.Matrix {
		if cell.ID != "thermal-matrix-cell:"+cell.RowNodeID+":"+cell.ColumnNodeID {
			t.Errorf("matrix cell ID does not follow contract: %#v", cell)
		}
	}
	seenMatrixIDs := map[string]bool{}
	for _, cell := range first.Matrix {
		if seenMatrixIDs[cell.ID] {
			t.Errorf("matrix cell ID is not unique: %q", cell.ID)
		}
		seenMatrixIDs[cell.ID] = true
		if connection := thermalConnectionByID(first, cell.ConnectionID); connection.RelationKind == "air_coupling" {
			t.Errorf("air coupling leaked into conductive boundary matrix: %#v", cell)
		}
	}
}

func thermalConnectionByID(topology ThermalTopologyReport, id string) ThermalConnectionAggregate {
	for _, connection := range topology.Connections {
		if connection.ID == id {
			return connection
		}
	}
	return ThermalConnectionAggregate{}
}

func thermalTopologyStableIDs(topology ThermalTopologyReport) []string {
	values := []string{}
	for _, node := range topology.Nodes {
		values = append(values, node.ID)
	}
	for _, boundary := range topology.Boundaries {
		values = append(values, boundary.ID, boundary.PairID)
	}
	for _, connection := range topology.Connections {
		values = append(values, connection.ID)
	}
	for _, opening := range topology.Openings {
		values = append(values, opening.ID, opening.PairID)
	}
	for _, coupling := range topology.AirCouplings {
		values = append(values, coupling.ID)
	}
	for _, cell := range topology.Matrix {
		values = append(values, cell.ID)
	}
	return sortedUniqueStrings(values)
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
	if outdoors := findThermalNode(t, topology, ventilation.FromNodeID); outdoors.Kind != "outdoors" || outdoors.Label != "Outdoors" {
		t.Fatalf("non-surface outdoor-air target must remain generic: %#v", outdoors)
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

func TestCompactConnectionsDeduplicateReciprocalInterfaces(t *testing.T) {
	topology := AnalyzeGeometry(thermalOpeningTestDocument()).Topology
	connection := findThermalConnection(t, topology, "interzone_explicit_surface")
	if connection.SurfaceCount != 1 || len(connection.BoundaryIDs) != 2 {
		t.Fatalf("paired interface aggregation = surface count %d boundary IDs %#v", connection.SurfaceCount, connection.BoundaryIDs)
	}
	if connection.OpeningCount != 1 || len(connection.OpeningIDs) != 1 {
		t.Fatalf("paired opening aggregation = opening count %d IDs %#v", connection.OpeningCount, connection.OpeningIDs)
	}
	if connection.PhysicalGrossArea != 4 || connection.EffectiveGrossArea != 8 || connection.EffectiveOpeningArea != 6 {
		t.Fatalf("connection areas = physical gross %v effective gross %v opening %v", connection.PhysicalGrossArea, connection.EffectiveGrossArea, connection.EffectiveOpeningArea)
	}
	if !connection.HasUA || connection.TotalUA != 14.8572 || !connection.HasPhysicalUA || connection.PhysicalTotalUA != 7.4286 {
		t.Fatalf("connection UA = effective %v/%v physical %v/%v", connection.TotalUA, connection.HasUA, connection.PhysicalTotalUA, connection.HasPhysicalUA)
	}
	if connection.FromNodeID > connection.ToNodeID {
		t.Fatalf("connection node order is not deterministic: %q > %q", connection.FromNodeID, connection.ToNodeID)
	}
}

func TestZoneSignatureAndMatrixReconcileWithConnections(t *testing.T) {
	topology := AnalyzeGeometry(thermalOpeningTestDocument()).Topology
	zoneA := findZoneThermalSignature(t, topology, "Zone A")
	if zoneA.AreaBasis != "effective" || zoneA.InterzoneArea != 8 || zoneA.WindowArea != 6 || zoneA.InterzoneUA != 14.8572 || zoneA.TotalUA != 14.8572 || !zoneA.HasTotalUA || zoneA.UACoverage != 1 {
		t.Fatalf("zone A thermal signature = %#v", zoneA)
	}
	if len(zoneA.AdjacentZoneIDs) != 1 || zoneA.AdjacentZoneIDs[0] != "zone:zone%20b" {
		t.Fatalf("zone A adjacency = %#v", zoneA.AdjacentZoneIDs)
	}
	connection := findThermalConnection(t, topology, "interzone_explicit_surface")
	cells := thermalMatrixCellsForConnection(topology, connection.ID)
	if len(cells) != 2 {
		t.Fatalf("matrix cells for connection = %d, want symmetric pair", len(cells))
	}
	for _, cell := range cells {
		if cell.Area != connection.EffectiveGrossArea || cell.UA != connection.TotalUA || cell.HasUA != connection.HasUA || cell.SurfaceCount != connection.SurfaceCount {
			t.Errorf("matrix cell does not reconcile with connection: cell %#v connection %#v", cell, connection)
		}
	}
}

func TestConnectionTotalsReconcileWithoutDoubleCounting(t *testing.T) {
	document, err := Parse(summaryFixtureIDF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	topology := AnalyzeGeometry(document).Topology
	boundaryGrossArea := 0.0
	for _, boundary := range topology.Boundaries {
		if boundary.RelationKind != "invalid" {
			boundaryGrossArea += boundary.EffectiveGrossArea
		}
	}
	connectionGrossArea := 0.0
	for _, connection := range topology.Connections {
		if connection.RelationKind != "air_coupling" {
			connectionGrossArea += connection.EffectiveGrossArea
		}
	}
	if connectionGrossArea != boundaryGrossArea {
		t.Fatalf("connection/boundary gross area = %v/%v", connectionGrossArea, boundaryGrossArea)
	}
	zone := findZoneThermalSignature(t, topology, "Zone 1")
	if zone.ExteriorArea != 380 || zone.GroundArea != 200 || zone.WindowArea != 6 || zone.ExteriorWWR != 0.0333 {
		t.Fatalf("summary fixture thermal signature = %#v", zone)
	}
	if zone.HasTotalUA || zone.TotalUA != 0 || zone.UACoverage != 0 {
		t.Fatalf("missing U-values were exposed as a partial total: %#v", zone)
	}
}

func TestAirCouplingsUseSeparateCompactConnections(t *testing.T) {
	topology := AnalyzeGeometry(thermalAirCouplingTestDocument()).Topology
	airConnection := findThermalConnection(t, topology, "air_coupling")
	if len(airConnection.AirCouplingIDs) == 0 || airConnection.SurfaceCount != 0 || airConnection.EffectiveGrossArea != 0 || airConnection.HasUA {
		t.Fatalf("air coupling connection was mixed with conduction: %#v", airConnection)
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

func findThermalConnection(t *testing.T, topology ThermalTopologyReport, relationKind string) ThermalConnectionAggregate {
	t.Helper()
	for _, connection := range topology.Connections {
		if connection.RelationKind == relationKind {
			return connection
		}
	}
	t.Fatalf("thermal connection relation %q not found", relationKind)
	return ThermalConnectionAggregate{}
}

func findZoneThermalSignature(t *testing.T, topology ThermalTopologyReport, zoneName string) ZoneThermalSignature {
	t.Helper()
	for _, signature := range topology.ZoneSignatures {
		if signature.ZoneName == zoneName {
			return signature
		}
	}
	t.Fatalf("zone thermal signature %q not found", zoneName)
	return ZoneThermalSignature{}
}

func thermalMatrixCellsForConnection(topology ThermalTopologyReport, connectionID string) []ThermalMatrixCell {
	var cells []ThermalMatrixCell
	for _, cell := range topology.Matrix {
		if cell.ConnectionID == connectionID {
			cells = append(cells, cell)
		}
	}
	return cells
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
