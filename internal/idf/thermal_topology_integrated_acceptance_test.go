package idf

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTOPO280ExteriorZoneIntegratedFlow(t *testing.T) {
	document := readThermalTopologyFlowFixture(t, "small_office.idf")
	geometry := AnalyzeGeometry(document)
	topology := geometry.Topology
	boundary := findThermalBoundary(t, topology, "South Wall")
	connection := thermalConnectionContainingBoundary(t, topology, boundary.ID)
	if connection.RelationKind != "exterior" || (connection.FromNodeID != "thermal-environment:outdoors" && connection.ToNodeID != "thermal-environment:outdoors") {
		t.Fatalf("South Wall connection = %#v", connection)
	}
	if boundary.PhysicalGrossArea <= 0 || boundary.EffectiveGrossArea <= 0 || !boundary.HasUA || connection.TotalUA <= 0 {
		t.Fatalf("exterior Area/UA is incomplete: boundary %#v connection %#v", boundary, connection)
	}
	if len(boundary.OpeningIDs) != 1 || boundary.PhysicalOpeningArea <= 0 || boundary.OpeningUA <= 0 {
		t.Fatalf("exterior opening breakdown is incomplete: %#v", boundary)
	}
	opening := findThermalOpening(t, topology, "South Window")
	if opening.BaseSurfaceID != boundary.SurfaceID || opening.UA <= 0 {
		t.Fatalf("opening does not trace to the selected exterior wall: %#v", opening)
	}
	if !thermalAnchorsContainOBC(boundary.SourceAnchors) {
		t.Fatalf("boundary source anchors do not include exact OBC fields: %#v", boundary.SourceAnchors)
	}

	projection := BuildSemanticYAMLProjection(document, SemanticYAMLMetadata{})
	if len(projection.Navigation.ByViewTarget["geometry|"+boundary.ID]) == 0 || len(projection.Navigation.ByViewTarget["geometry|"+connection.ID]) == 0 {
		t.Fatalf("Semantic navigation cannot restore boundary/connection selection: %#v", projection.Navigation.ByViewTarget)
	}
}

func TestTOPO281InterzonePairIntegratedFlow(t *testing.T) {
	document := thermalGeometryPairTestDocument(0, true)
	topology := AnalyzeGeometry(document).Topology
	connection := findThermalConnection(t, topology, "interzone_explicit_surface")
	if connection.SurfaceCount != 1 || len(connection.BoundaryIDs) != 2 || connection.OpeningCount != 1 || len(connection.OpeningIDs) != 1 {
		t.Fatalf("compact interzone pair was double-counted: %#v", connection)
	}
	for _, name := range []string{"Pair A", "Pair B"} {
		boundary := findThermalBoundary(t, topology, name)
		if boundary.CounterpartSurfaceID == "" || boundary.PairID == "" || boundary.ConstructionStatus != "reverse_layer_equivalent" || (boundary.GeometryCheck.Status != "valid" && boundary.GeometryCheck.Status != "not_applicable") {
			t.Fatalf("%s reciprocal/construction/geometry validation = %#v", name, boundary)
		}
	}
	for _, cell := range thermalMatrixCellsForConnection(topology, connection.ID) {
		if cell.Area != connection.EffectiveGrossArea || cell.UA != connection.TotalUA || cell.HasUA != connection.HasUA {
			t.Fatalf("Graph/Matrix Area or UA differs: cell %#v connection %#v", cell, connection)
		}
	}

	projection := BuildSemanticYAMLProjection(document, SemanticYAMLMetadata{})
	sourceObjects := map[int]bool{}
	for _, occurrenceID := range projection.Navigation.ByViewTarget["geometry|"+connection.ID] {
		for _, occurrence := range projection.Navigation.Occurrences {
			if occurrence.OccurrenceID == occurrenceID && occurrence.ContextKind == "surface_boundary_context" && occurrence.SourceAnchor.ObjectIndex != nil {
				sourceObjects[*occurrence.SourceAnchor.ObjectIndex] = true
			}
		}
	}
	if len(sourceObjects) != 2 {
		t.Fatalf("interzone connection does not navigate to both source surfaces: %#v", sourceObjects)
	}
}

func TestTOPO282ModelingDecisionAndQAIntegratedFlow(t *testing.T) {
	disconnected := AnalyzeGeometry(thermalGeometryPairTestDocument(0, false)).Topology
	if !hasAdjacencyObservation(disconnected, "geometrically_adjacent_but_thermally_disconnected") {
		t.Fatal("adiabatic adjacency QA observation is missing")
	}
	for _, connection := range disconnected.Connections {
		if connection.RelationKind == "interzone_explicit_surface" {
			t.Fatalf("QA adjacency created an authoritative thermal relation: %#v", connection)
		}
	}

	brokenDocument := thermalGeometryPairTestDocument(0, true)
	thermalFixtureObject(t, &brokenDocument, "BuildingSurface:Detailed", "Pair A").Fields[6].Value = "Missing Pair"
	brokenTopology := AnalyzeGeometry(brokenDocument).Topology
	brokenBoundary := findThermalBoundary(t, brokenTopology, "Pair A")
	issue := thermalIssueByCode(t, brokenTopology, thermalDiagnosticSurfaceCounterpartMissing)
	if issue.BoundaryID != brokenBoundary.ID || issue.EntityID != brokenBoundary.SurfaceEntityID || len(issue.SourceAnchors) == 0 {
		t.Fatalf("invalid counterpart issue is not linked to its exact boundary/source: issue %#v boundary %#v", issue, brokenBoundary)
	}
	foundDiagnoseIssue := false
	for _, diagnostic := range AnalyzeDiagnostics(brokenDocument) {
		foundDiagnoseIssue = foundDiagnoseIssue || diagnostic.ID == issue.ID
	}
	if !foundDiagnoseIssue {
		t.Fatalf("Diagnose did not retain topology issue ID %q", issue.ID)
	}

	fixedDocument := brokenDocument.clone()
	thermalFixtureObject(t, &fixedDocument, "BuildingSurface:Detailed", "Pair A").Fields[6].Value = "Pair B"
	fixedBoundary := findThermalBoundary(t, AnalyzeGeometry(fixedDocument).Topology, "Pair A")
	if fixedBoundary.ID != brokenBoundary.ID || fixedBoundary.SurfaceEntityID != brokenBoundary.SurfaceEntityID || fixedBoundary.RelationKind != "interzone_explicit_surface" {
		t.Fatalf("source edit did not restore selection by stable entity: before %#v after %#v", brokenBoundary, fixedBoundary)
	}
}

func TestTOPO283AirCouplingIntegratedFlow(t *testing.T) {
	topology := AnalyzeGeometry(thermalAirCouplingTestDocument()).Topology
	mixing := findThermalAirCoupling(t, topology, "zone_mixing")
	if mixing.Direction != "directed" || mixing.ScheduleName != "Always On" || mixing.DesignFlowRate != 0.1 || len(mixing.SourceAnchors) == 0 {
		t.Fatalf("ZoneMixing source/schedule/flow is incomplete: %#v", mixing)
	}
	afn := findThermalAirCoupling(t, topology, "airflow_network")
	if afn.SurfaceID == "" || afn.ComponentName != "Pair Crack" || len(afn.SourceAnchors) < 3 {
		t.Fatalf("AFN base surface/component/source context is incomplete: %#v", afn)
	}
	airConnection := findThermalConnection(t, topology, "air_coupling")
	if len(airConnection.AirCouplingIDs) == 0 || len(airConnection.BoundaryIDs) != 0 || airConnection.HasUA || airConnection.EffectiveGrossArea != 0 {
		t.Fatalf("air coupling was mixed into the conductive layer: %#v", airConnection)
	}
	conductive := findThermalConnection(t, topology, "interzone_explicit_surface")
	if conductive.ID == airConnection.ID || len(conductive.AirCouplingIDs) != 0 {
		t.Fatalf("conductive and air relations are not independent: conductive %#v air %#v", conductive, airConnection)
	}
}

func TestTOPO285SettingsAndBatchRoundTripContract(t *testing.T) {
	document := thermalOpeningTestDocument()
	session := newAnalysisSession(NewDocumentIndex(document))
	topology := session.Geometry().Topology
	_ = session.Profile()
	_ = session.Diagnostics()
	if session.geometryBuildCount != 1 {
		t.Fatalf("settings-style result navigation rebuilt topology %d times", session.geometryBuildCount)
	}

	baseline := SummarizeThermalTopologyForBatch(topology, "effective")
	compareDocument := document.clone()
	for index := range compareDocument.Objects {
		if strings.EqualFold(compareDocument.Objects[index].Type, "Zone") {
			compareDocument.Objects[index].Fields[6].Value = "3"
		}
	}
	compare := SummarizeThermalTopologyForBatch(AnalyzeGeometry(compareDocument).Topology, "effective")
	baselineValue := baseline.Metrics["topology_interzone_area"].Value
	compareValue := compare.Metrics["topology_interzone_area"].Value
	delta := compareValue - baselineValue
	percent := delta / baselineValue * 100
	if baselineValue != 8 || compareValue != 12 || delta != 4 || math.Abs(percent-50) > 1e-9 {
		t.Fatalf("Batch Topology A/B delta = %v/%v delta %v percent %v", baselineValue, compareValue, delta, percent)
	}
	physicalBaseline := SummarizeThermalTopologyForBatch(topology, "physical")
	physicalCompare := SummarizeThermalTopologyForBatch(AnalyzeGeometry(compareDocument).Topology, "physical")
	if physicalBaseline.Metrics["topology_interzone_area"].Value != physicalCompare.Metrics["topology_interzone_area"].Value {
		t.Fatal("physical basis changed when only the zone multiplier changed")
	}
}

func readThermalTopologyFlowFixture(t *testing.T, name string) Document {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", "thermal_topology", name))
	if err != nil {
		t.Fatal(err)
	}
	document, err := Parse(string(content))
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func thermalConnectionContainingBoundary(t *testing.T, topology ThermalTopologyReport, boundaryID string) ThermalConnectionAggregate {
	t.Helper()
	for _, connection := range topology.Connections {
		for _, candidate := range connection.BoundaryIDs {
			if candidate == boundaryID {
				return connection
			}
		}
	}
	t.Fatalf("connection containing boundary %q not found", boundaryID)
	return ThermalConnectionAggregate{}
}

func thermalAnchorsContainOBC(anchors []SemanticSourceAnchor) bool {
	condition, target := false, false
	for _, anchor := range anchors {
		field := strings.ToLower(anchor.FieldName)
		condition = condition || strings.Contains(field, "outside boundary condition") && !strings.Contains(field, "object")
		target = target || strings.Contains(field, "outside boundary condition object")
	}
	return condition && target
}

func thermalIssueByCode(t *testing.T, topology ThermalTopologyReport, code string) ThermalTopologyIssueLink {
	t.Helper()
	for _, issue := range topology.IssueLinks {
		if issue.Code == code {
			return issue
		}
	}
	t.Fatalf("topology issue %q not found", code)
	return ThermalTopologyIssueLink{}
}
