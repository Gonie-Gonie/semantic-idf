package simulation

import "testing"

func TestSummarizePurposeMetricsIncludesEnergyExplanationLevels(t *testing.T) {
	bundle := &PurposeResultBundle{
		EnergyExplanation: EnergyExplanationResult{
			Schema: energyExplanationSchema,
			Nodes: []EnergyExplanationNode{
				{ID: "energy.carrier.electricity", Level: "energy", Label: "Electricity", Value: 10, Unit: "kWh"},
				{ID: "energy.end_use.cooling.electricity", Level: "energy", Label: "Cooling", Value: 4, Unit: "kWh"},
				{ID: "load.cooling", Level: "load", Label: "Cooling load", Value: 8, Unit: "kWh"},
				{ID: "heat.internal_convective", Level: "heat", Label: "Internal gains", Value: 3, DisplayValue: 3, Unit: "kWh"},
				{ID: "residual.energy.electricity", Level: "residual", Label: "Residual", Value: 2, Unit: "kWh"},
			},
			Completeness: EnergyCompleteness{
				Status:        "partial",
				MappedPercent: 40,
			},
		},
	}

	metrics := SummarizePurposeMetrics(bundle)
	if purposeMetricByID(metrics, "energy_explanation.energy_use") == nil {
		t.Fatalf("missing energy explanation total metric: %#v", metrics)
	}
	if metric := purposeMetricByID(metrics, "energy_explanation.delivered_load"); metric == nil || metric.Value != 8 || metric.DisplayValue != "8 kWh" {
		t.Fatalf("delivered load metric = %#v", metric)
	}
	if metric := purposeMetricByID(metrics, "energy_explanation.heat.heat_internal_convective"); metric == nil || metric.Label != "Heat driver: Internal gains" {
		t.Fatalf("heat driver metric = %#v", metric)
	}
	if metric := purposeMetricByID(metrics, "energy_explanation.mapped_percent"); metric == nil || metric.Value != 40 || metric.Status != "partial" {
		t.Fatalf("mapped percent metric = %#v", metric)
	}
}

func purposeMetricByID(metrics []PurposeMetric, id string) *PurposeMetric {
	for index := range metrics {
		if metrics[index].ID == id {
			return &metrics[index]
		}
	}
	return nil
}
