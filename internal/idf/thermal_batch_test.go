package idf

import "testing"

func TestSummarizeThermalTopologyForBatchUsesRequestedAreaBasis(t *testing.T) {
	report := AnalyzeGeometry(thermalOpeningTestDocument()).Topology
	effective := SummarizeThermalTopologyForBatch(report, "effective")
	physical := SummarizeThermalTopologyForBatch(report, "physical")
	if got := effective.Metrics["topology_zone_count"].Value; got != 2 {
		t.Fatalf("zone count = %v, want 2", got)
	}
	if got := effective.Metrics["topology_interzone_area"].Value; got != 8 {
		t.Fatalf("effective interzone area = %v, want 8", got)
	}
	if got := physical.Metrics["topology_interzone_area"].Value; got != 4 {
		t.Fatalf("physical interzone area = %v, want 4", got)
	}
	ua := effective.Metrics["topology_interzone_ua"]
	if ua.Value != 14.8572 || ua.Status != "ok" || !ua.HasCoverage || ua.Coverage != 1 {
		t.Fatalf("interzone UA metric = %#v", ua)
	}
	if got := effective.Metrics["topology_exterior_opening_area"].Value; got != 0 {
		t.Fatalf("exterior opening area = %v, want 0", got)
	}
}

func TestSummarizeThermalTopologyForBatchMarksMissingUValues(t *testing.T) {
	report := AnalyzeGeometry(thermalTopologyTestDocument()).Topology
	summary := SummarizeThermalTopologyForBatch(report, "effective")
	metric := summary.Metrics["topology_exterior_ua"]
	if metric.Status != "missing" || !metric.HasCoverage || metric.Coverage != 0 {
		t.Fatalf("missing exterior UA metric = %#v", metric)
	}
	if len(ThermalTopologyBatchMetricDefinitions()) < 16 {
		t.Fatalf("topology metric definition count is too small")
	}
}
