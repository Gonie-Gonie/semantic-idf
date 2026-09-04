package simulation

import (
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestPrepareBatchPurposeSimulationRequestUsesNormalizedOutputApplyMode(t *testing.T) {
	request := SimulationRunRequest{Text: purposePlanFixtureIDF + `
Output:Variable,
  *,
  Zone Mean Air Temperature,
  Monthly;
`}
	prepared, err := prepareBatchPurposeSimulationRequest(request, SimulationPurposeRequest{
		Purposes:        []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
		OutputApplyMode: PurposeOutputApplyModeReplaceConflicts,
	})
	if err != nil {
		t.Fatalf("prepare batch purpose request: %v", err)
	}
	if prepared.PurposeRequest == nil || prepared.PurposeRequest.OutputApplyMode != PurposeOutputApplyModeReplaceConflicts {
		t.Fatalf("prepared purpose request = %#v", prepared.PurposeRequest)
	}

	doc, err := idf.Parse(prepared.Text)
	if err != nil {
		t.Fatalf("parse prepared batch text: %v", err)
	}
	matching := 0
	frequency := ""
	for _, object := range doc.Objects {
		if !strings.EqualFold(object.Type, "Output:Variable") || len(object.Fields) < 3 ||
			!strings.EqualFold(object.Fields[0].Value, "*") ||
			!strings.EqualFold(object.Fields[1].Value, "Zone Mean Air Temperature") {
			continue
		}
		matching++
		frequency = object.Fields[2].Value
	}
	if matching != 1 || !strings.EqualFold(frequency, "Hourly") {
		t.Fatalf("prepared conflict output count/frequency = %d/%q, want 1/Hourly", matching, frequency)
	}
}

func TestSummarizePurposeMetricsIncludesEnergyExplanationLevels(t *testing.T) {
	bundle := &PurposeResultBundle{
		EnergyExplanation: EnergyExplanationResult{
			Schema: energyExplanationSchema,
			Scope:  EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
			Nodes: []EnergyExplanationNode{
				{ID: "carrier.electricity.building", Level: "carrier", Kind: "carrier.electricity", Label: "Electricity", Value: 10, Unit: "kWh", Carrier: "electricity"},
				{ID: "end_use.cooling.building", Level: "end_use", Kind: "end_use.cooling", Label: "Cooling", Value: 4, Unit: "kWh", Carrier: "electricity", EndUse: "cooling"},
				{ID: "load.cooling.building", Level: "load", Kind: "load.cooling", Label: "Cooling load", Value: 8, Unit: "kWh", ServiceKind: "cooling", PathType: "zone"},
				{ID: "driver.internal_convective.cooling.building", Level: "driver", Kind: "driver.internal_convective", Label: "Internal gains", Value: 3, DisplayValue: 3, Unit: "kWh"},
				{ID: "residual.site_electricity.building", Level: "residual", Kind: "residual.site_energy", Label: "Residual", Value: 2, Unit: "kWh"},
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
	if metric := purposeMetricByID(metrics, "energy_explanation.heat.driver_internal_convective"); metric == nil || metric.Label != "Heat driver: Internal gains" {
		t.Fatalf("heat driver metric = %#v", metric)
	}
	if metric := purposeMetricByID(metrics, "energy_explanation.mapped_percent"); metric == nil || metric.Value != 40 || metric.Status != "partial" {
		t.Fatalf("mapped percent metric = %#v", metric)
	}
	if metric := purposeMetricByID(metrics, "energy_explanation.kpi.cooling_cop"); metric == nil || metric.Value != 2 || metric.DisplayValue != "2" || metric.Label != "Derived KPI: Cooling COP" {
		t.Fatalf("derived KPI metric = %#v", metric)
	}
}

func TestCompactPurposeResultBundleForBatchOmitsMonthlyEnergyGraphs(t *testing.T) {
	bundle := &PurposeResultBundle{
		EnergyExplanation: EnergyExplanationResult{
			Schema: energyExplanationSchema,
			Scope:  EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
			Nodes:  []EnergyExplanationNode{{ID: "carrier.electricity.building", Level: "carrier", Value: 12, Unit: "kWh"}},
			Links:  []EnergyPathLink{{ID: "annual-link", FromID: "end_use.cooling.building", ToID: "carrier.electricity.building", Relation: "end_use_to_carrier", Basis: "reported_meter", FromValue: 12, ToValue: 12, FromUnit: "kWh", ToUnit: "kWh"}},
			Periods: []EnergyPeriod{
				{ID: "annual", Nodes: []EnergyExplanationNode{{ID: "annual-node"}}, Links: []EnergyPathLink{{ID: "annual-period-link"}}},
				{ID: "M1", Nodes: []EnergyExplanationNode{{ID: "month-node"}}, Links: []EnergyPathLink{{ID: "month-link"}}},
			},
			AvailableZones: []string{"Office"},
			ZoneResults:    []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"}}},
			Sources:        []EnergyDataSource{{ID: "facility", SourceType: "sql_meter"}},
		},
		EnergyExplanationSummary: EnergyExplanationSummary{
			Schema:   energyExplanationSummarySchema,
			Period:   "annual",
			Scope:    EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
			Carriers: []EnergyExplanationSummaryItem{{ID: "carrier.electricity.building", Value: 12}},
		},
	}
	compactPurposeResultBundleForBatch(bundle)
	if len(bundle.EnergyExplanation.Periods) != 0 || len(bundle.EnergyExplanation.ZoneResults) != 0 {
		t.Fatalf("batch nested graphs = periods %#v zones %#v", bundle.EnergyExplanation.Periods, bundle.EnergyExplanation.ZoneResults)
	}
	if len(bundle.EnergyExplanation.Nodes) != 1 || len(bundle.EnergyExplanation.Links) != 1 || len(bundle.EnergyExplanation.Sources) != 1 || len(bundle.EnergyExplanation.AvailableZones) != 1 || len(bundle.EnergyExplanationSummary.Carriers) != 1 {
		t.Fatalf("batch annual summary/trace was removed: %#v", bundle)
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
