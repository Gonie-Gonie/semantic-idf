package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

type energyPathV1BatchXLSXContract struct {
	XLSX struct {
		DefaultSheets []string `json:"defaultSheets"`
		Sections      []string `json:"sections"`
	} `json:"xlsx"`
}

func TestEnergyPathV1BatchXLSXGoldenMatchesCurrentExporter(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("internal", "simulation", "testdata", "energy_path", "v1_batch_export.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden energyPathV1BatchXLSXContract
	if err := json.Unmarshal(payload, &golden); err != nil {
		t.Fatal(err)
	}

	request := representativeEnergyPathV1BatchExportRequest()
	sheets := batchSimulationWorkbookSheets(request)
	gotSheets := make([]string, 0, len(sheets))
	gotSections := make([]string, 0, len(sheets))
	for _, sheet := range sheets {
		gotSheets = append(gotSheets, sheet.Name)
		if len(sheet.Sections) != 1 {
			t.Fatalf("v1 Batch XLSX sheet %q has %d sections, want 1", sheet.Name, len(sheet.Sections))
		}
		if len(sheet.Sections[0].Rows) == 0 {
			t.Fatalf("representative v1 Batch XLSX sheet %q has no rows", sheet.Name)
		}
		gotSections = append(gotSections, sheet.Sections[0].Title)
	}
	if !reflect.DeepEqual(gotSheets, golden.XLSX.DefaultSheets) {
		t.Fatalf("v1 Batch XLSX sheets = %#v, golden = %#v", gotSheets, golden.XLSX.DefaultSheets)
	}
	if !reflect.DeepEqual(gotSections, golden.XLSX.Sections) {
		t.Fatalf("v1 Batch XLSX sections = %#v, golden = %#v", gotSections, golden.XLSX.Sections)
	}
}

func representativeEnergyPathV1BatchExportRequest() BatchSimulationXLSXExportRequest {
	baseline := representativeEnergyPathV1BatchRun("baseline-run", "baseline.idf", 1, 33.333, "complete")
	target := representativeEnergyPathV1BatchRun("target-run", "target.idf", 1.5, 50, "partial")
	return BatchSimulationXLSXExportRequest{
		Result: simulation.MultiSimulationResult{
			RunID:     "v1-batch-export",
			Total:     2,
			Completed: 2,
			Succeeded: 2,
			Results:   []simulation.SimulationRunResult{baseline, target},
		},
		Context: BatchSimulationXLSXExportContext{
			SelectedPaths: []string{"baseline.idf", "target.idf"},
			ViewMode:      "purpose",
		},
		Comparison: BatchSimulationComparisonXLSXContext{
			BaselineRowID: baseline.RunID,
			TargetRowID:   target.RunID,
		},
	}
}

func representativeEnergyPathV1BatchRun(runID string, filename string, coolingValue float64, mappedPercent float64, completenessStatus string) simulation.SimulationRunResult {
	objectIndex := 1
	sourceID := "sql-rdd-21"
	nodeID := "energy.end_use.cooling.electricity"
	edgeID := "edge.energy_carrier_electricity.energy_end_use_cooling_electricity"
	completeness := simulation.EnergyCompleteness{
		Status:        completenessStatus,
		MappedPercent: mappedPercent,
		SourceAvailability: []simulation.EnergySourceAvailabilityEntry{{
			Name:      "Cooling:Electricity",
			Level:     "energy",
			Status:    "found",
			SourceIDs: []string{sourceID},
		}},
	}
	period := simulation.EnergyPeriod{
		ID:    "annual",
		Label: "Annual",
		Kind:  "annual",
		Nodes: []simulation.EnergyExplanationNode{{
			ID:                  nodeID,
			Level:               "energy",
			Kind:                "energy.cooling",
			Label:               "Cooling energy",
			Value:               coolingValue,
			Unit:                "kWh",
			Period:              "annual",
			Carrier:             "electricity",
			EndUse:              "cooling",
			MeterHierarchyLevel: "broad_end_use",
			Basis:               "measured_meter",
			SourceIDs:           []string{sourceID},
		}},
		Edges: []simulation.EnergyExplanationEdge{{
			ID:        edgeID,
			FromID:    "energy.carrier.electricity",
			ToID:      nodeID,
			Value:     coolingValue,
			Unit:      "kWh",
			Period:    "annual",
			Relation:  "meter_enduse",
			Basis:     "measured_meter",
			RuleID:    "meter.end_use",
			SourceIDs: []string{sourceID},
		}},
		Reconciliation: []simulation.EnergyReconciliation{{
			ID:             "reconcile.energy.electricity.annual",
			Level:          "energy",
			Period:         "annual",
			Label:          "Electricity total basis",
			Status:         "residual",
			ExpectedValue:  coolingValue + 2,
			ExplainedValue: coolingValue,
			ResidualValue:  2,
			Unit:           "kWh",
			Basis:          "residual",
			SourceIDs:      []string{sourceID},
		}},
		Warnings: []simulation.EnergyWarning{{
			Severity: "warning",
			Code:     "v1_representative_warning",
			Message:  "Representative v1 export warning.",
			Period:   "annual",
		}},
	}
	return simulation.SimulationRunResult{
		RunID:    runID,
		Filename: filename,
		Status:   "succeeded",
		PurposeMetrics: []simulation.PurposeMetric{{
			ID:           "energy_explanation.energy_by_end_use.cooling.electricity",
			Label:        "Cooling energy",
			PurposeID:    simulation.SimulationPurposeBasicEnergy,
			Value:        coolingValue,
			Unit:         "kWh",
			DisplayValue: "representative",
			Status:       "ok",
		}},
		PurposeResults: &simulation.PurposeResultBundle{
			EnergyExplanation: simulation.EnergyExplanationResult{
				Schema:         "semantic-idf.energy-explanation/v1",
				Purpose:        "basic_energy",
				Frequency:      "monthly",
				Periods:        []simulation.EnergyPeriod{period},
				Nodes:          period.Nodes,
				Edges:          period.Edges,
				Reconciliation: period.Reconciliation,
				Sources: []simulation.EnergyDataSource{{
					ID:                 sourceID,
					SourceType:         "sql_report_data",
					IsMeter:            true,
					Name:               "Cooling:Electricity",
					Units:              "J",
					SourceUnit:         "J",
					NormalizedUnit:     "kWh",
					ReportingFrequency: "Monthly",
					AggregationMethod:  "sum_report_data",
					TableName:          "ReportData",
					RowName:            "Cooling:Electricity",
					ColumnName:         "Value [J]",
					ObjectIndex:        &objectIndex,
				}},
				Completeness: completeness,
				Warnings:     period.Warnings,
			},
			EnergyExplanationSummary: simulation.EnergyExplanationSummary{
				Schema:       "semantic-idf.energy-explanation-summary/v1",
				Completeness: completeness,
				EnergyByEndUse: []simulation.EnergyExplanationSummaryItem{{
					ID:        "cooling.electricity",
					Level:     "energy",
					Label:     "Cooling electricity",
					Value:     coolingValue,
					Unit:      "kWh",
					Carrier:   "electricity",
					EndUse:    "cooling",
					Basis:     "measured_meter",
					SourceIDs: []string{sourceID},
				}},
			},
		},
	}
}
