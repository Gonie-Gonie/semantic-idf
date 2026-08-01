package idf

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExportThermalTopologyJSONUsesCanonicalReport(t *testing.T) {
	report := AnalyzeGeometry(thermalTopologyTestDocument()).Topology
	payload, err := ExportThermalTopologyJSON(report, ThermalTopologyProjectionOptions{AreaBasis: "physical"})
	if err != nil {
		t.Fatalf("ExportThermalTopologyJSON() error = %v", err)
	}
	var exported ThermalTopologyReport
	if err := json.Unmarshal(payload, &exported); err != nil {
		t.Fatalf("exported topology is not valid JSON: %v", err)
	}
	if exported.Schema != thermalTopologySchema || exported.SourceModelHash == "" || exported.AreaBasis != "physical" {
		t.Fatalf("export metadata = schema %q hash %q area %q", exported.Schema, exported.SourceModelHash, exported.AreaBasis)
	}
	if len(exported.Nodes) != len(report.Nodes) || len(exported.Boundaries) != len(report.Boundaries) || len(exported.Matrix) != len(report.Matrix) {
		t.Fatalf("canonical export changed report cardinality: %#v", exported.Stats)
	}
	if len(exported.Boundaries[0].SourceAnchors) == 0 {
		t.Fatal("canonical export dropped source anchors")
	}
	text := string(payload)
	for _, forbidden := range []string{"panX", "panY", "zoom", "layoutCache"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("canonical export contains UI-only key %q", forbidden)
		}
	}
}

func TestThermalTopologyGraphExportsShareStableProjection(t *testing.T) {
	report := AnalyzeGeometry(thermalTopologyTestDocument()).Topology
	options := ThermalTopologyProjectionOptions{Level: "zone", Metric: "area", Scope: "building", AreaBasis: "effective"}
	projection, err := ProjectThermalTopologyGraph(report, options)
	if err != nil {
		t.Fatalf("ProjectThermalTopologyGraph() error = %v", err)
	}
	if projection.SourceSchema != report.Schema || projection.SourceModelHash != report.SourceModelHash || len(projection.Edges) == 0 {
		t.Fatalf("graph projection lost source identity or edges: %#v", projection)
	}
	nodes := map[string]bool{}
	for _, node := range projection.Nodes {
		nodes[node.ID] = true
	}
	for _, edge := range projection.Edges {
		if !nodes[edge.FromNodeID] || !nodes[edge.ToNodeID] {
			t.Fatalf("edge %q references missing endpoint %q -> %q", edge.ID, edge.FromNodeID, edge.ToNodeID)
		}
	}
	graphML, err := ExportThermalTopologyGraphML(report, options)
	if err != nil || !strings.Contains(string(graphML), "<graphml") || !strings.Contains(string(graphML), projection.Edges[0].ID) {
		t.Fatalf("GraphML export error = %v\n%s", err, graphML)
	}
	dot, err := ExportThermalTopologyDOT(report, options)
	if err != nil || !strings.Contains(string(dot), "graph thermal_topology") || !strings.Contains(string(dot), projection.Edges[0].ID) {
		t.Fatalf("DOT export error = %v\n%s", err, dot)
	}
}

func TestThermalTopologyProjectionOptionsValidateScopeAndEnums(t *testing.T) {
	if _, err := NormalizeThermalTopologyProjectionOptions(ThermalTopologyProjectionOptions{Level: "mesh"}); err == nil {
		t.Fatal("invalid graph level was accepted")
	}
	if _, err := NormalizeThermalTopologyProjectionOptions(ThermalTopologyProjectionOptions{Scope: "neighbors"}); err == nil {
		t.Fatal("neighbors scope without selection was accepted")
	}
	normalized, err := NormalizeThermalTopologyProjectionOptions(ThermalTopologyProjectionOptions{})
	if err != nil || normalized.Level != "zone" || normalized.Metric != "topology" || normalized.Scope != "building" || normalized.AreaBasis != "effective" {
		t.Fatalf("default projection options = %#v, error = %v", normalized, err)
	}
}
