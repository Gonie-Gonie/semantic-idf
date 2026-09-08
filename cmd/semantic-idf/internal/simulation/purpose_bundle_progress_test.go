package simulation

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Reuse the existing small model/monthly meter fixture; append only this
// test's surface drilldown rows. Energy and ThermalTopology deliberately see
// different preferred reporting frequencies in the same immutable database.
func purposeBundleProgressFixture(t *testing.T) (*SimulationRunResult, SimulationPurposeRequest) {
	t.Helper()
	directory := t.TempDir()
	input := filepath.Join(directory, "model.idf")
	if err := os.WriteFile(input, []byte(thermalTopologySimulationFixture), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`ALTER TABLE ReportDataDictionary ADD COLUMN ReportingFrequency TEXT`,
		`ALTER TABLE ReportDataDictionary ADD COLUMN IsMeter INTEGER`,
		`ALTER TABLE ReportDataDictionary ADD COLUMN IndexGroup TEXT`,
		`UPDATE ReportDataDictionary SET ReportingFrequency='Monthly',IsMeter=CASE WHEN KeyValue='' THEN 1 ELSE 0 END,IndexGroup='Energy'`,
		`UPDATE ReportDataDictionary SET KeyValue='Office' WHERE KeyValue='ZONE ONE'`,
		`INSERT INTO "Time" VALUES(3,1,1,1,0),(4,1,1,2,0)`,
		`INSERT INTO ReportDataDictionary VALUES
		(30,'South Wall','Surface Average Face Conduction Heat Transfer Energy','J','Monthly',0,'Surface'),
		(31,'South Wall','Surface Average Face Conduction Heat Transfer Energy','J','Hourly',0,'Surface'),
		(32,'South Wall','Surface Average Face Conduction Heat Transfer Rate','W','Hourly',0,'Surface'),
		(33,'South Wall','Surface Inside Face Conduction Heat Transfer Energy','J','Hourly',0,'Surface'),
		(34,'South Wall','Surface Outside Face Conduction Heat Transfer Energy','J','Hourly',0,'Surface'),
		(35,'South Window','Surface Window Heat Gain Energy','J','Hourly',0,'Surface'),
		(36,'South Window','Surface Window Heat Loss Energy','J','Hourly',0,'Surface'),
		(40,'Office','Zone Air Heat Balance Surface Convection Rate','W','Hourly',0,'Zone')`,
		`INSERT INTO ReportData VALUES
		(30,1,30,356400000),
		(31,3,31,3600000),(32,4,31,-1800000),
		(33,3,32,999000),(34,4,32,999000),
		(35,3,33,4320000),(36,4,33,-1440000),
		(37,3,34,-2880000),(38,4,34,2160000),
		(39,3,35,720000),(40,4,35,0),
		(41,3,36,360000),(42,4,36,360000),
		(43,3,40,100),(44,4,40,-50)`,
	} {
		if _, err := db.Exec(query); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	heat, err := parseSimulationHeatFlowSQL(path)
	if err != nil {
		t.Fatal(err)
	}
	request := SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy, SimulationPurposeZoneHeatFlow}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, ZoneHeatFlowDetail: PurposeZoneHeatFlowDetailSurface, Scope: SimulationPurposeScope{ZoneMode: "all", PeriodMode: "full"}}
	plan := BuildPurposeRunPlan(parsePurposePlanFixture(t, thermalTopologySimulationFixture), request)
	result := &SimulationRunResult{InputPath: input, OutputDirectory: directory, PurposeRunPlan: &plan, Files: []SimulationFileInfo{{Name: "eplusout.sql", Path: path, Kind: "sqlite"}}, HeatFlow: heat,
		// Wrong fallback values make an accidental SQL→Series switch visible.
		Series: []SimulationSeries{{File: "fallback.csv", Column: "South Wall:Surface Average Face Conduction Heat Transfer Energy [J]", Points: []SimulationPoint{{X: 1, Label: "01/01 01:00", Value: 999000000}, {X: 2, Label: "01/01 02:00", Value: 999000000}}}}}
	return result, request
}

func purposeBundleProgressJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func purposeBundleProgressCapture(t *testing.T, result *SimulationRunResult, request SimulationPurposeRequest) (PurposeResultBundle, []string) {
	t.Helper()
	phases := []string{}
	bundle := buildPurposeResultBundleWithProgress(result, request, func(phase, message string) {
		if strings.TrimSpace(phase) == "" || strings.TrimSpace(message) == "" {
			t.Errorf("empty progress stage/message: %q %q", phase, message)
		}
		phases = append(phases, phase)
	})
	return bundle, phases
}

func TestPurposeBundleProgressCombinedParityAndImmutableInputs(t *testing.T) {
	result, request := purposeBundleProgressFixture(t)
	before, requestBefore := purposeBundleProgressJSON(t, result), purposeBundleProgressJSON(t, request)
	sqlBefore, err := os.ReadFile(result.Files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	inputBefore, err := os.ReadFile(result.InputPath)
	if err != nil {
		t.Fatal(err)
	}
	basicRequest := request
	basicRequest.Purposes = []SimulationPurposeID{SimulationPurposeBasicEnergy}
	basic, basicPhases := purposeBundleProgressCapture(t, result, basicRequest)
	combined, phases := purposeBundleProgressCapture(t, result, request)
	public := BuildPurposeResultBundle(result, request)
	for name, pair := range map[string][2]any{"energy": {basic.Energy, combined.Energy}, "explanation": {basic.EnergyExplanation, combined.EnergyExplanation}, "summary": {basic.EnergyExplanationSummary, combined.EnergyExplanationSummary}, "public wrapper": {combined, public}} {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Errorf("progress/combined-purpose processing changed %s", name)
		}
	}
	wantBasic := []string{"energy_geometry", "energy_dashboard", "energy_drivers", "energy_service_paths", "energy_path"}
	wantCombined := append(append([]string{}, wantBasic...), "zone_heat_flow", "thermal_topology")
	if !reflect.DeepEqual(basicPhases, wantBasic) || !reflect.DeepEqual(phases, wantCombined) {
		t.Fatalf("selected-purpose stage order: basic=%v combined=%v", basicPhases, phases)
	}
	if len(combined.EnergyExplanation.Nodes) == 0 || len(combined.EnergyExplanationSummary.Drivers)+len(combined.EnergyExplanationSummary.Carriers) == 0 {
		t.Fatal("parity compared empty Energy results")
	}
	if len(combined.Energy.FacilityMonthly) != 1 || combined.Energy.FacilityMonthly[0].Total != 3 {
		t.Fatalf("monthly meter changed: %+v", combined.Energy.FacilityMonthly)
	}
	if combined.ZoneHeatFlow.FrameCount != 2 || len(combined.ZoneHeatFlow.Zones) != 1 || combined.ZoneHeatFlow.Unit != "W" {
		t.Fatalf("hourly zone heat flow lost: %+v", combined.ZoneHeatFlow)
	}
	overlay := combined.ThermalTopology
	if !overlay.Available || len(overlay.Periods) != 4 || len(overlay.Sources) != 5 {
		t.Fatalf("surface periods/sources unavailable: %+v", overlay)
	}
	for _, period := range []string{"annual", "monthly", "daily", "hourly"} {
		_ = findThermalTopologySimulationPeriod(t, overlay.Periods, period)
	}
	annual := findThermalTopologySimulationPeriod(t, overlay.Periods, "annual")
	if len(annual.BoundaryFlows) != 1 || math.Abs(annual.BoundaryFlows[0].Value-.5) > 1e-9 || len(annual.BoundaryFlows[0].Traces) < 3 || len(annual.BoundaryFlows[0].SourceIDs) != 3 {
		t.Fatalf("surface selection/traces changed: %+v", annual)
	}
	for _, source := range overlay.Sources {
		if source.SourceType != "sql_report_data" || source.ReportingFrequency != "Hourly" || source.SourceUnit != "J" || source.NormalizedUnit != "kWh" {
			t.Fatalf("surface SQL source authority changed: %+v", source)
		}
	}
	if !bytes.Equal(before, purposeBundleProgressJSON(t, result)) || !bytes.Equal(requestBefore, purposeBundleProgressJSON(t, request)) {
		t.Fatal("bundle building mutated input result/request")
	}
	sqlAfter, err := os.ReadFile(result.Files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sqlBefore, sqlAfter) {
		t.Fatal("bundle building changed SQL input")
	}
	inputAfter, err := os.ReadFile(result.InputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(inputBefore, inputAfter) {
		t.Fatal("bundle building changed model input")
	}
}

func TestPurposeBundleProgressOnlySelectedStagesAndGeometryFallback(t *testing.T) {
	for _, test := range []struct {
		name                     string
		purposes                 []SimulationPurposeID
		surface, missingGeometry bool
		want                     []string
	}{
		{"surface_only", []SimulationPurposeID{SimulationPurposeZoneHeatFlow}, true, false, []string{"energy_geometry", "zone_heat_flow", "thermal_topology"}},
		{"zone_only", []SimulationPurposeID{SimulationPurposeZoneHeatFlow}, false, false, []string{"zone_heat_flow"}},
		{"comfort_only", []SimulationPurposeID{SimulationPurposeComfort}, false, false, []string{}},
		{"geometry_failure", []SimulationPurposeID{SimulationPurposeBasicEnergy, SimulationPurposeZoneHeatFlow}, true, true, []string{"energy_geometry", "energy_dashboard", "energy_drivers", "energy_service_paths", "energy_path", "zone_heat_flow", "thermal_topology"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, request := purposeBundleProgressFixture(t)
			request.Purposes = test.purposes
			request.ZoneHeatFlowDetail = ""
			if test.surface {
				request.ZoneHeatFlowDetail = PurposeZoneHeatFlowDetailSurface
			}
			if test.missingGeometry {
				result.InputPath = filepath.Join(t.TempDir(), "missing.idf")
			}
			before := purposeBundleProgressJSON(t, result)
			bundle, phases := purposeBundleProgressCapture(t, result, request)
			if !reflect.DeepEqual(phases, test.want) {
				t.Fatalf("actual selected stages=%v, want%v", phases, test.want)
			}
			if test.missingGeometry {
				if bundle.ThermalTopology.Available || !strings.Contains(bundle.ThermalTopology.UnavailableReason, "could not be mapped") {
					t.Fatalf("geometry failure masked: %+v", bundle.ThermalTopology)
				}
				if len(bundle.Energy.FacilityMonthly) != 1 || bundle.Energy.FacilityMonthly[0].Total != 3 || len(bundle.EnergyExplanation.Sources) == 0 {
					t.Fatal("geometry failure discarded valid SQL energy")
				}
				found := false
				for _, warning := range bundle.EnergyExplanation.Warnings {
					if warning.Code == "energy_driver_geometry_unavailable" {
						found = true
					}
				}
				if !found {
					t.Fatal("geometry fallback warning missing")
				}
			}
			if !bytes.Equal(before, purposeBundleProgressJSON(t, result)) {
				t.Fatal("stage reporting mutated result")
			}
		})
	}
}
