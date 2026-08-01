package simulation

import (
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
)

const thermalTopologySimulationFixture = `
Version, 24.1;

Zone,
  Office;

BuildingSurface:Detailed,
  South Wall,
  Wall,
  ,
  Office,
  ,
  Outdoors,
  ,
  SunExposed,
  WindExposed,
  ,
  4,
  0, 0, 0,
  5, 0, 0,
  5, 0, 3,
  0, 0, 3;

FenestrationSurface:Detailed,
  South Window,
  Window,
  ,
  South Wall,
  ,
  ,
  ,
  1,
  4,
  1, 0, 1,
  2, 0, 1,
  2, 0, 2,
  1, 0, 2;
`

func TestPurposeRunPlanSurfaceHeatFlowIsModelAware(t *testing.T) {
	doc, err := idf.Parse(thermalTopologySimulationFixture)
	if err != nil {
		t.Fatal(err)
	}
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes:           []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
		ZoneHeatFlowDetail: PurposeZoneHeatFlowDetailSurface,
	})
	if plan.ZoneHeatFlowDetail != PurposeZoneHeatFlowDetailSurface {
		t.Fatalf("zone heat-flow detail = %q", plan.ZoneHeatFlowDetail)
	}
	for _, output := range []struct{ key, name string }{
		{"South Wall", "Surface Average Face Conduction Heat Transfer Energy"},
		{"South Wall", "Surface Inside Face Conduction Heat Transfer Energy"},
		{"South Wall", "Surface Outside Face Conduction Heat Transfer Energy"},
		{"South Window", "Surface Window Heat Gain Energy"},
		{"South Window", "Surface Window Heat Loss Energy"},
	} {
		if findPurposeOutput(plan, "Output:Variable", output.key, output.name) == nil {
			t.Fatalf("missing model-aware output %q / %q", output.key, output.name)
		}
	}
	if findPurposeOutput(plan, "Output:Diagnostics", "", "") == nil {
		t.Fatalf("surface conduction plan should enable advanced report variables")
	}
	if plan.EstimatedWeight == "Light" || plan.EstimatedFrames == 0 {
		t.Fatalf("surface plan weight/frames = %q/%d", plan.EstimatedWeight, plan.EstimatedFrames)
	}
}

func TestBuildThermalTopologySimulationResultMapsBoundaryAndNormalizesEnergy(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "model.idf")
	if err := os.WriteFile(inputPath, []byte(thermalTopologySimulationFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	points := func(first float64, second float64) []SimulationPoint {
		return []SimulationPoint{{X: 1, Label: "01/01 01:00", Value: first}, {X: 2, Label: "01/01 02:00", Value: second}}
	}
	result := &SimulationRunResult{
		InputPath: inputPath,
		Series: []SimulationSeries{
			{File: "eplusout.csv", Column: "South Wall:Surface Average Face Conduction Heat Transfer Energy [J]", Points: points(3_600_000, -1_800_000)},
			{File: "eplusout.csv", Column: "South Wall:Surface Inside Face Conduction Heat Transfer Energy [J]", Points: points(4_320_000, -1_440_000)},
			{File: "eplusout.csv", Column: "South Wall:Surface Outside Face Conduction Heat Transfer Energy [J]", Points: points(-2_880_000, 2_160_000)},
			{File: "eplusout.csv", Column: "South Window:Surface Window Heat Gain Energy [J]", Points: points(720_000, 0)},
			{File: "eplusout.csv", Column: "South Window:Surface Window Heat Loss Energy [J]", Points: points(360_000, 360_000)},
		},
	}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes:           []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
		ZoneHeatFlowDetail: PurposeZoneHeatFlowDetailSurface,
	})
	overlay := bundle.ThermalTopology
	if !overlay.Available || overlay.State != "simulation_overlay" {
		t.Fatalf("overlay unavailable: %+v", overlay)
	}
	if len(overlay.Periods) != 4 || len(overlay.Sources) != 5 {
		t.Fatalf("period/source counts = %d/%d", len(overlay.Periods), len(overlay.Sources))
	}
	annual := findThermalTopologySimulationPeriod(t, overlay.Periods, "annual")
	if len(annual.BoundaryFlows) != 1 {
		t.Fatalf("annual boundary flows = %#v", annual.BoundaryFlows)
	}
	flow := annual.BoundaryFlows[0]
	if math.Abs(flow.Value-0.5) > 1e-9 || flow.Unit != "kWh" {
		t.Fatalf("annual flow = %v %s, want 0.5 kWh", flow.Value, flow.Unit)
	}
	if flow.BoundaryID == "" || flow.ConnectionID == "" || flow.OwnerNodeID == "" || len(flow.SourceIDs) != 3 {
		t.Fatalf("boundary topology/provenance links = %+v", flow)
	}
	if len(overlay.Reconciliation) != 1 || overlay.Reconciliation[0].Status != "ok" {
		t.Fatalf("reconciliation = %#v", overlay.Reconciliation)
	}
	if overlay.OutputWeight.Series != 5 || overlay.OutputWeight.Frames != 2 || overlay.OutputWeight.Values != 10 {
		t.Fatalf("output weight = %+v", overlay.OutputWeight)
	}
}

func TestThermalTopologySimulationIntegratesRateFallback(t *testing.T) {
	raw := thermalTopologyRawSeries{
		unit:               "W",
		reportingFrequency: "Hourly",
		rate:               true,
		points: []SimulationPoint{
			{X: 1, Label: "01/01 01:00", Value: 1000},
			{X: 2, Label: "01/01 02:00", Value: -500},
		},
	}
	values, _ := normalizeThermalTopologySeries(raw)
	if len(values) != 2 || values[0] != 1 || values[1] != -0.5 {
		t.Fatalf("integrated rate values = %#v", values)
	}
}

func TestLoadThermalTopologyRawSeriesReadsAllSurfaceSQLRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	createTestEnergyPlusSQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES (20, 'South Wall', 'Surface Average Face Conduction Heat Transfer Energy', 'J')`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES (20, 1, 20, 3600000), (21, 2, 20, -1800000)`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	series, err := loadThermalTopologyRawSeriesFromSQL(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || series[0].source.SourceType != "sql_report_data" || series[0].source.SourceUnit != "J" || series[0].source.NormalizedUnit != "kWh" {
		t.Fatalf("surface SQL series = %#v", series)
	}
	values, labels := normalizeThermalTopologySeries(series[0])
	if len(values) != 2 || values[0] != 1 || values[1] != -0.5 || labels[0] != "01-01 01:00" {
		t.Fatalf("surface SQL normalized points = %#v / %#v", values, labels)
	}
}

func findThermalTopologySimulationPeriod(t *testing.T, periods []ThermalTopologySimulationPeriod, id string) ThermalTopologySimulationPeriod {
	t.Helper()
	for _, period := range periods {
		if period.ID == id {
			return period
		}
	}
	t.Fatalf("missing period %q in %#v", id, periods)
	return ThermalTopologySimulationPeriod{}
}
