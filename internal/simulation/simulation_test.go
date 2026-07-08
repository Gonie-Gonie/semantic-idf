package simulation

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
)

func TestParseERRFileCountsIssues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.err")
	content := `Program Version,EnergyPlus
   ** Warning ** Missing schedule fallback
   ** Severe  ** Node connection problem
   ** Fatal  ** Simulation stopped
EnergyPlus Terminated--Fatal Error Detected. 1 Warning; 1 Severe Errors; Elapsed Time=00hr 00min 01.00sec
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	summary := parseERRFile(path)
	if summary.Warnings != 1 || summary.Severe != 1 || summary.Fatal != 1 {
		t.Fatalf("counts = warnings %d severe %d fatal %d", summary.Warnings, summary.Severe, summary.Fatal)
	}
	if len(summary.Issues) != 3 {
		t.Fatalf("issue count = %d, want 3", len(summary.Issues))
	}
	if summary.Completed {
		t.Fatalf("fatal run should not be marked completed")
	}
}

func TestParseSimulationCSVBuildsSummariesAndSeries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.csv")
	content := `Date/Time,ZONE ONE:Zone Air Temperature [C](Hourly),Electricity:Facility [J](Hourly)
 01/01  01:00:00,20.0,100
 01/01  02:00:00,21.5,150
 01/01  03:00:00,19.5,125
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	summary, series, err := parseSimulationCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if summary.RowCount != 3 {
		t.Fatalf("row count = %d, want 3", summary.RowCount)
	}
	if len(summary.ColumnInfo) != 2 {
		t.Fatalf("column summary count = %d, want 2", len(summary.ColumnInfo))
	}
	first := summary.ColumnInfo[0]
	if first.Min != 19.5 || first.Max != 21.5 || first.Last != 19.5 {
		t.Fatalf("temperature summary = %+v", first)
	}
	if len(series) != 2 || len(series[0].Points) != 3 {
		t.Fatalf("series = %+v", series)
	}
}

func TestParseSimulationSQLSeriesBuildsTimedSeries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergyPlusSQL(t, path)

	series, err := parseSimulationSQLSeries(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 4 {
		t.Fatalf("series count = %d, want 4: %#v", len(series), series)
	}
	first := series[0]
	if first.File != "eplusout.sql" || first.Column != "ZONE ONE:Zone Mean Air Temperature [C]" {
		t.Fatalf("first series identity = %+v", first)
	}
	if first.Min != 20 || first.Max != 21.5 || first.Average != 20.75 || first.RowCount != 2 {
		t.Fatalf("first series stats = %+v", first)
	}
	if len(first.Points) != 2 || first.Points[0].Label != "01-01 01:00" || first.Points[1].Label != "01-01 02:00" {
		t.Fatalf("first series points = %#v", first.Points)
	}
}

func TestParseSimulationSQLGracefullyHandlesMissingReportTables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE Metadata (Name TEXT, Value TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	series, err := parseSimulationSQLSeries(path)
	if err != nil || len(series) != 0 {
		t.Fatalf("series parser = len %d err %v, want empty nil", len(series), err)
	}
	energy, err := parseSimulationEnergySQL(path)
	if err != nil {
		t.Fatalf("energy parser err = %v", err)
	}
	if len(energy.FacilityMonthly)+len(energy.EndUseMonthly)+len(energy.ZoneMonthly) != 0 {
		t.Fatalf("energy parser should return empty result: %#v", energy)
	}
	heatFlow, err := parseSimulationHeatFlowSQL(path)
	if err != nil {
		t.Fatalf("heat-flow parser err = %v", err)
	}
	if len(heatFlow.Zones) != 0 || heatFlow.FrameCount != 0 {
		t.Fatalf("heat-flow parser should return empty result: %#v", heatFlow)
	}
}

func TestParseSimulationSQLEntrypointBuildsCombinedResult(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergyPlusSQL(t, path)

	result, err := parseSimulationSQL(path, PurposeRunPlan{Purposes: []SimulationPurposeID{SimulationPurposeZoneHeatFlow}})
	if err != nil {
		t.Fatal(err)
	}
	if result.SourceFile != "eplusout.sql" || len(result.Purposes) != 1 || result.Purposes[0] != SimulationPurposeZoneHeatFlow {
		t.Fatalf("entrypoint metadata = %#v", result)
	}
	if len(result.Series) == 0 || len(result.HeatFlow.Zones) == 0 {
		t.Fatalf("entrypoint parsed result = %#v", result)
	}
}

func TestParseSimulationSQLWithContextHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := parseSimulationSQLWithContext(ctx, filepath.Join(t.TempDir(), "eplusout.sql"), PurposeRunPlan{})
	if err == nil {
		t.Fatalf("expected cancellation error, got result %#v", result)
	}
}

func TestParseSimulationHeatFlowCSVBuildsDataset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.csv")
	content := `Date/Time,ZONE ONE:Zone Mean Air Temperature [C](Hourly),ZONE ONE:Zone Air Heat Balance Internal Convective Heat Gain Rate [W](Hourly),ZONE ONE:Zone Air Heat Balance Surface Convection Rate [W](Hourly),ZONE TWO:Zone Air Heat Balance Outdoor Air Transfer Rate [W](Hourly)
 01/01  01:00:00,20.0,100,-30,-12
 01/01  02:00:00,21.5,150,-45,-18
 01/01  03:00:00,19.5,125,-20,-8
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	dataset, err := parseSimulationHeatFlowCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if dataset.FrameCount != 3 || dataset.OriginalFrameCount != 3 {
		t.Fatalf("frame counts = %d/%d, want 3/3", dataset.FrameCount, dataset.OriginalFrameCount)
	}
	if len(dataset.Categories) != 3 {
		t.Fatalf("category count = %d, want 3: %#v", len(dataset.Categories), dataset.Categories)
	}
	if len(dataset.Zones) != 2 {
		t.Fatalf("zone count = %d, want 2: %#v", len(dataset.Zones), dataset.Zones)
	}
	if dataset.Zones[0].Name != "ZONE ONE" || dataset.Zones[0].Temperature[1] != 21.5 {
		t.Fatalf("first zone = %#v", dataset.Zones[0])
	}
	if dataset.MaxAbs != 150 {
		t.Fatalf("max abs = %v, want 150", dataset.MaxAbs)
	}
}

func TestParseSimulationHeatFlowESOBuildsDataset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.eso")
	content := `Program Version,EnergyPlus
1,5,Environment Title[],Latitude[deg],Longitude[deg],Time Zone[],Elevation[m]
2,8,Day of Simulation[],Month[],Day of Month[],DST Indicator[1=yes 0=no],Hour[],StartMinute[],EndMinute[],DayType
10,1,ZONE ONE,Zone Mean Air Temperature [C] !Hourly
11,1,ZONE ONE,Zone Air Heat Balance Internal Convective Heat Gain Rate [W] !Hourly
12,1,ZONE ONE,Zone Air Heat Balance Surface Convection Rate [W] !Hourly
13,1,ZONE TWO,Zone Air Heat Balance Outdoor Air Transfer Rate [W] !Hourly
End of Data Dictionary
1,RUN PERIOD,  0.00, 0.00,   0.00,  0.00
2,1, 1, 1, 0, 1, 0.00,60.00,Monday
10,20.0
11,100.0
12,-30.0
13,-12.0
2,1, 1, 1, 0, 2, 0.00,60.00,Monday
10,21.5
11,150.0
12,-45.0
13,-18.0
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	dataset, err := parseSimulationHeatFlowESO(path)
	if err != nil {
		t.Fatal(err)
	}
	if dataset.FrameCount != 2 || dataset.OriginalFrameCount != 2 {
		t.Fatalf("frame counts = %d/%d, want 2/2", dataset.FrameCount, dataset.OriginalFrameCount)
	}
	if len(dataset.Categories) != 3 {
		t.Fatalf("category count = %d, want 3: %#v", len(dataset.Categories), dataset.Categories)
	}
	if len(dataset.Zones) != 2 {
		t.Fatalf("zone count = %d, want 2: %#v", len(dataset.Zones), dataset.Zones)
	}
	if dataset.Zones[0].Name != "ZONE ONE" || dataset.Zones[0].Temperature[1] != 21.5 {
		t.Fatalf("first zone = %#v", dataset.Zones[0])
	}
	if dataset.Labels[1] != "01-01 02:00" {
		t.Fatalf("labels = %#v", dataset.Labels)
	}
}

func TestParseSimulationHeatFlowSQLBuildsDataset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergyPlusSQL(t, path)

	dataset, err := parseSimulationHeatFlowSQL(path)
	if err != nil {
		t.Fatal(err)
	}
	if dataset.FrameCount != 2 || dataset.OriginalFrameCount != 2 {
		t.Fatalf("frame counts = %d/%d, want 2/2", dataset.FrameCount, dataset.OriginalFrameCount)
	}
	if len(dataset.Categories) != 3 {
		t.Fatalf("category count = %d, want 3: %#v", len(dataset.Categories), dataset.Categories)
	}
	if len(dataset.Zones) != 2 {
		t.Fatalf("zone count = %d, want 2: %#v", len(dataset.Zones), dataset.Zones)
	}
	if dataset.Zones[0].Name != "ZONE ONE" || dataset.Zones[0].Temperature[1] != 21.5 {
		t.Fatalf("first zone = %#v", dataset.Zones[0])
	}
	if dataset.Labels[1] != "01-01 02:00" {
		t.Fatalf("labels = %#v", dataset.Labels)
	}
	if dataset.SourceFile != "eplusout.sql" {
		t.Fatalf("source file = %q, want eplusout.sql", dataset.SourceFile)
	}
}

func TestParseSimulationEnergySQLBuildsDashboard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)

	result, err := parseSimulationEnergySQL(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.FacilityMonthly) != 1 {
		t.Fatalf("facility series count = %d, want 1: %#v", len(result.FacilityMonthly), result.FacilityMonthly)
	}
	if len(result.EndUseMonthly) != 1 {
		t.Fatalf("end-use series count = %d, want 1: %#v", len(result.EndUseMonthly), result.EndUseMonthly)
	}
	if len(result.ZoneMonthly) != 1 {
		t.Fatalf("zone series count = %d, want 1: %#v", len(result.ZoneMonthly), result.ZoneMonthly)
	}
	if result.FacilityMonthly[0].Unit != "kWh" || result.FacilityMonthly[0].Total != 3 {
		t.Fatalf("facility energy = %+v, want 3 kWh", result.FacilityMonthly[0])
	}
	if len(result.FacilityMonthly[0].Points) != 2 || result.FacilityMonthly[0].Points[0].Label != "M1" || result.FacilityMonthly[0].Points[1].Label != "M2" {
		t.Fatalf("facility monthly points = %#v", result.FacilityMonthly[0].Points)
	}
	if result.ZoneMonthly[0].ZoneName != "ZONE ONE" || result.ZoneMonthly[0].Total != 0.5 {
		t.Fatalf("zone energy = %+v, want ZONE ONE 0.5 kWh", result.ZoneMonthly[0])
	}
}

func TestParseSimulationEnergySQLClassifiesExtendedMeters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, '', 'FuelOilNo1:Facility', 'J'),
		(24, '', 'Steam:Facility', 'J'),
		(25, '', 'ElectricityProduced:Facility', 'J'),
		(26, '', 'DistrictCooling:Cooling', 'J'),
		(27, '', 'DistrictHeating:Heating', 'J'),
		(28, '', 'Cooling:Electricity', 'J'),
		(29, '', 'Heating:Electricity', 'J'),
		(30, '', 'InteriorLights:Electricity', 'J'),
		(31, '', 'Heating:Propane', 'J'),
		(32, '', 'WaterSystems:Steam', 'J')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 1, 23, 3600000.0),
		(8, 1, 24, 7200000.0),
		(9, 1, 25, 1800000.0),
		(10, 1, 26, 900000.0),
		(11, 1, 27, 450000.0),
		(12, 1, 28, 360000.0),
		(13, 1, 29, 720000.0),
		(14, 1, 30, 1080000.0),
		(15, 1, 31, 1440000.0),
		(16, 1, 32, 2160000.0)`); err != nil {
		t.Fatal(err)
	}

	result, err := parseSimulationEnergySQL(path)
	if err != nil {
		t.Fatal(err)
	}
	if item := energySeriesByName(result.FacilityMonthly, "FuelOilNo1:Facility"); item == nil || item.Total != 1 {
		t.Fatalf("fuel oil facility series = %#v", result.FacilityMonthly)
	}
	if item := energySeriesByName(result.FacilityMonthly, "Steam:Facility"); item == nil || item.Total != 2 {
		t.Fatalf("steam facility series = %#v", result.FacilityMonthly)
	}
	if item := energySeriesByName(result.EndUseMonthly, "ElectricityProduced:Facility"); item == nil || item.Total != 0.5 {
		t.Fatalf("onsite production end-use series = %#v", result.EndUseMonthly)
	}
	if item := energySeriesByName(result.EndUseMonthly, "DistrictCooling:Cooling"); item == nil || item.Total != 0.25 {
		t.Fatalf("district cooling end-use series = %#v", result.EndUseMonthly)
	}
	if item := energySeriesByName(result.EndUseMonthly, "DistrictHeating:Heating"); item == nil || item.Total != 0.125 {
		t.Fatalf("district heating end-use series = %#v", result.EndUseMonthly)
	}
	if item := energySeriesByName(result.EndUseMonthly, "Cooling:Electricity"); item == nil || item.Total != 0.1 {
		t.Fatalf("cooling electricity end-use alias series = %#v", result.EndUseMonthly)
	}
	if item := energySeriesByName(result.EndUseMonthly, "Heating:Electricity"); item == nil || item.Total != 0.2 {
		t.Fatalf("heating electricity end-use alias series = %#v", result.EndUseMonthly)
	}
	if item := energySeriesByName(result.EndUseMonthly, "InteriorLights:Electricity"); item == nil || item.Total != 0.3 {
		t.Fatalf("lighting electricity end-use alias series = %#v", result.EndUseMonthly)
	}
	if item := energySeriesByName(result.EndUseMonthly, "Heating:Propane"); item == nil || item.Total != 0.4 {
		t.Fatalf("propane heating end-use series = %#v", result.EndUseMonthly)
	}
	if item := energySeriesByName(result.EndUseMonthly, "WaterSystems:Steam"); item == nil || item.Total != 0.6 {
		t.Fatalf("steam water systems end-use series = %#v", result.EndUseMonthly)
	}
}

func TestParseSimulationEnergySQLAggregatesRowsByMonth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO "Time" VALUES (3, 1, 15, 1, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES (7, 3, 20, 3600000.0)`); err != nil {
		t.Fatal(err)
	}

	result, err := parseSimulationEnergySQL(path)
	if err != nil {
		t.Fatal(err)
	}
	facility := result.FacilityMonthly[0]
	if facility.Total != 4 {
		t.Fatalf("facility total = %+v, want 4 kWh", facility)
	}
	if len(facility.Points) != 2 || facility.Points[0].Label != "M1" || facility.Points[0].Value != 2 || facility.Points[1].Value != 2 {
		t.Fatalf("aggregated monthly points = %#v", facility.Points)
	}
}

func TestQueryReportDataFiltersAndPreservesMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE ReportDataDictionary (
			ReportDataDictionaryIndex INTEGER PRIMARY KEY,
			KeyValue TEXT,
			Name TEXT,
			Units TEXT,
			IsMeter INTEGER,
			ReportingFrequency TEXT,
			IndexGroup TEXT
		)`,
		`CREATE TABLE "Time" (
			TimeIndex INTEGER PRIMARY KEY,
			Month INTEGER,
			Day INTEGER,
			Hour INTEGER,
			Minute INTEGER,
			IntervalType TEXT
		)`,
		`CREATE TABLE ReportData (
			ReportDataIndex INTEGER PRIMARY KEY,
			TimeIndex INTEGER,
			ReportDataDictionaryIndex INTEGER,
			Value REAL
		)`,
		`INSERT INTO ReportDataDictionary VALUES
			(20, '', 'Electricity:Facility', 'J', 1, 'Hourly', 'Meters'),
			(21, 'ZONE ONE', 'Zone Lights Electricity Energy', 'J', 0, 'Monthly', 'Zone')`,
		`INSERT INTO "Time" VALUES
			(1, 1, 1, 1, 0, 'Zone Timestep'),
			(2, 1, 31, 24, 0, 'Month')`,
		`INSERT INTO ReportData VALUES
			(1, 1, 20, 3600000.0),
			(2, 2, 21, 900000.0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("sql fixture statement failed: %v\n%s", err, statement)
		}
	}

	isMeter := true
	rows, err := QueryReportData(db, SQLSeriesQuery{
		Names:     []string{"Electricity:Facility"},
		IsMeter:   &isMeter,
		Frequency: []string{"Hourly"},
		Units:     []string{"J"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("meter row count = %d, want 1: %#v", len(rows), rows)
	}
	row := rows[0]
	if row.DictionaryIndex != 20 || row.Name != "Electricity:Facility" || row.KeyValue != "" || !row.IsMeter {
		t.Fatalf("meter dictionary metadata = %#v", row)
	}
	if row.ReportingFrequency != "Hourly" || row.IndexGroup != "Meters" || row.Units != "J" {
		t.Fatalf("meter report metadata = %#v", row)
	}
	if !row.Month.Valid || row.Month.Int64 != 1 || !row.Day.Valid || row.Day.Int64 != 1 || !row.Hour.Valid || row.Hour.Int64 != 1 || row.IntervalType != "Zone Timestep" {
		t.Fatalf("meter time metadata = %#v", row)
	}
	if !row.Value.Valid || row.Value.Float64 != 3600000 {
		t.Fatalf("meter value = %#v", row.Value)
	}

	notMeter := false
	rows, err = QueryReportData(db, SQLSeriesQuery{
		KeyValues: []string{"ZONE ONE"},
		IsMeter:   &notMeter,
		Frequency: []string{"Monthly"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].DictionaryIndex != 21 || rows[0].IntervalType != "Month" {
		t.Fatalf("variable row = %#v, want monthly ZONE ONE row", rows)
	}

	rows, err = QueryReportData(db, SQLSeriesQuery{
		IndexGroups: []string{"Zone"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].DictionaryIndex != 21 || rows[0].IndexGroup != "Zone" {
		t.Fatalf("index-group filtered row = %#v, want Zone dictionary row", rows)
	}

	rows, err = QueryReportData(db, SQLSeriesQuery{
		Names:   []string{"Electricity:Facility"},
		IsMeter: &notMeter,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("non-meter filter returned facility meter rows: %#v", rows)
	}
}

func TestConvertEnergySQLValueUnits(t *testing.T) {
	tests := []struct {
		name      string
		value     float64
		unit      string
		wantValue float64
		wantUnit  string
	}{
		{name: "joules", value: 3600000, unit: "J", wantValue: 1, wantUnit: "kWh"},
		{name: "kilojoules", value: 3600, unit: "kJ", wantValue: 1, wantUnit: "kWh"},
		{name: "megajoules", value: 3.6, unit: "MJ", wantValue: 1, wantUnit: "kWh"},
		{name: "gigajoules", value: 0.0036, unit: "GJ", wantValue: 1, wantUnit: "kWh"},
		{name: "watt hours", value: 1000, unit: "Wh", wantValue: 1, wantUnit: "kWh"},
		{name: "kilowatt hours", value: 2.5, unit: "kWh", wantValue: 2.5, wantUnit: "kWh"},
		{name: "watts", value: 1500, unit: "W", wantValue: 1.5, wantUnit: "kW"},
		{name: "unknown unit", value: 7, unit: " kg ", wantValue: 7, wantUnit: "kg"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotValue, gotUnit := convertEnergySQLValue(test.value, test.unit)
			if math.Abs(gotValue-test.wantValue) > 0.000001 || gotUnit != test.wantUnit {
				t.Fatalf("convertEnergySQLValue(%v, %q) = %v %s, want %v %s", test.value, test.unit, gotValue, gotUnit, test.wantValue, test.wantUnit)
			}
		})
	}
}

func TestNormalizeSimulationSeriesDisplayUnits(t *testing.T) {
	energy := normalizeSimulationSeriesDisplay(SimulationSeries{
		File:    "eplusout.sql",
		Column:  "Electricity:Facility [J]",
		Min:     0,
		Max:     7200000,
		Average: 3600000,
		Points: []SimulationPoint{
			{X: 1, Label: "M1", Value: 3600000},
			{X: 2, Label: "M2", Value: 7200000},
		},
	})
	if energy.DisplayColumn != "Electricity:Facility [kWh]" || energy.DisplayUnit != "kWh" {
		t.Fatalf("energy display identity = %q %q", energy.DisplayColumn, energy.DisplayUnit)
	}
	if energy.DisplayMax != 2 || energy.DisplayAverage != 1 || len(energy.DisplayPoints) != 2 || energy.DisplayPoints[1].Value != 2 {
		t.Fatalf("energy display values = %#v", energy)
	}

	power := normalizeSimulationSeriesDisplay(SimulationSeries{
		File:    "eplusout.sql",
		Column:  "Supply Fan:Fan Electricity Rate [W]",
		Min:     500,
		Max:     1500,
		Average: 1000,
		Points: []SimulationPoint{
			{X: 1, Value: 500},
			{X: 2, Value: 1500},
		},
	})
	if power.DisplayUnit != "kW" || power.DisplayMin != 0.5 || power.DisplayMax != 1.5 || power.DisplayAverage != 1 {
		t.Fatalf("power display values = %#v", power)
	}

	humidity := normalizeSimulationSeriesDisplay(SimulationSeries{
		File:    "eplusout.sql",
		Column:  "Node:System Node Humidity Ratio [kgWater/kgDryAir]",
		Min:     0.004,
		Max:     0.009,
		Average: 0.006,
		Points:  []SimulationPoint{{X: 1, Value: 0.006}},
	})
	if humidity.DisplayColumn != "Node:System Node Humidity Ratio [kg/kg]" || humidity.DisplayUnit != "kg/kg" || len(humidity.DisplayPoints) != 0 {
		t.Fatalf("humidity display = %#v", humidity)
	}
}

func TestParseSimulationEnergySQLTotalsConvertedMonthlyPoints(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE ReportDataDictionary SET Units = 'W' WHERE ReportDataDictionaryIndex = 21`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE ReportData SET Value = 1000.0 WHERE ReportDataIndex = 3`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE ReportData SET Value = 2500.0 WHERE ReportDataIndex = 4`); err != nil {
		t.Fatal(err)
	}

	result, err := parseSimulationEnergySQL(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.EndUseMonthly) != 1 {
		t.Fatalf("end-use series count = %d, want 1: %#v", len(result.EndUseMonthly), result.EndUseMonthly)
	}
	endUse := result.EndUseMonthly[0]
	if endUse.Unit != "kW" || endUse.Total != 3.5 {
		t.Fatalf("converted end-use total = %+v, want 3.5 kW", endUse)
	}
	if len(endUse.Points) != 2 || endUse.Points[0].Value != 1 || endUse.Points[1].Value != 2.5 {
		t.Fatalf("converted monthly points = %#v", endUse.Points)
	}
}

func TestParseSimulationEnergyExplanationSQLBuildsAccountingGraph(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, 'ZONE ONE', 'Zone Air System Sensible Cooling Energy', 'J'),
		(24, 'ZONE ONE', 'Zone Air Heat Balance Internal Convective Heat Gain Rate', 'W'),
		(25, 'ZONE ONE', 'Zone Air Heat Balance Surface Convection Rate', 'W')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO "Time" VALUES
		(3, 1, 1, 1, 0),
		(4, 1, 1, 2, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 1, 23, 900000.0),
		(8, 2, 23, 900000.0),
		(9, 3, 24, 250.0),
		(10, 4, 24, 250.0),
		(11, 3, 25, -100.0),
		(12, 4, 25, -100.0)`); err != nil {
		t.Fatal(err)
	}

	facilityObjectIndex := 0
	coolingObjectIndex := 1
	loadObjectIndex := 2
	internalHeatObjectIndex := 3
	surfaceHeatObjectIndex := 4
	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
		{ObjectType: "Output:Meter", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "Electricity:Facility", ObjectIndex: &facilityObjectIndex},
		{ObjectType: "Output:Meter", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "Cooling:Electricity", ObjectIndex: &coolingObjectIndex},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "*", VariableName: "Zone Air System Sensible Cooling Energy", ObjectIndex: &loadObjectIndex},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "*", VariableName: "Zone Air Heat Balance Internal Convective Heat Gain Rate", ObjectIndex: &internalHeatObjectIndex},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "*", VariableName: "Zone Air Heat Balance Surface Convection Rate", ObjectIndex: &surfaceHeatObjectIndex},
	}}
	result, err := parseSimulationEnergyExplanationSQL(path, plan)
	if err != nil {
		t.Fatal(err)
	}
	if result.Schema != energyExplanationSchema || result.Purpose != string(SimulationPurposeBasicEnergy) {
		t.Fatalf("identity = %q/%q", result.Schema, result.Purpose)
	}
	if result.AllocationPolicy != PurposeAllocationPolicyDirectOnly {
		t.Fatalf("allocation policy = %q", result.AllocationPolicy)
	}
	if len(result.RelationshipRules) < 5 || result.RelationshipRules[0].ID == "" {
		t.Fatalf("relationship rules = %#v", result.RelationshipRules)
	}
	encoded, err := json.Marshal(result.RelationshipRules[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"fromLevel"`) || strings.Contains(string(encoded), `"FromLevel"`) {
		t.Fatalf("relationship rule json = %s", encoded)
	}
	if len(result.Periods) != 3 || result.Periods[0].ID != "annual" || result.Periods[1].ID != "M1" || result.Periods[2].ID != "M2" {
		t.Fatalf("periods = %#v", result.Periods)
	}
	facility := energyExplanationNodeByID(result.Nodes, "energy.carrier.electricity")
	if facility == nil || facility.Value != 3 || facility.Unit != "kWh" || facility.MeterHierarchyLevel != "facility_total" || !stringSliceContains(facility.SourceIDs, "sql-rdd-20") {
		t.Fatalf("facility node = %#v", facility)
	}
	cooling := energyExplanationNodeByID(result.Nodes, "energy.end_use.cooling.electricity")
	if cooling == nil || cooling.Value != 1 || cooling.Carrier != "electricity" || cooling.EndUse != "cooling" || cooling.MeterHierarchyLevel != "broad_end_use" {
		t.Fatalf("cooling node = %#v", cooling)
	}
	load := energyExplanationNodeByID(result.Nodes, "load.cooling.zone_one")
	if load == nil || load.Level != "load" || load.Value != 0.5 || load.ServiceKind != "cooling" || load.PathType != "zone" || load.ZoneName != "ZONE ONE" || load.Basis != "measured_energy_variable" || !stringSliceContains(load.SourceIDs, "sql-rdd-23") {
		t.Fatalf("load node = %#v", load)
	}
	heat := energyExplanationNodeByID(result.Nodes, "heat.internal_convective.zone_one")
	if heat == nil || heat.Level != "heat" || heat.Value != 0.5 || heat.SignedValue != 0.5 || heat.Unit != "kWh" || heat.HeatCategory != "internal_gains" || heat.Basis != "derived_balance" || !stringSliceContains(heat.SourceIDs, "sql-rdd-24") {
		t.Fatalf("heat node = %#v", heat)
	}
	surface := energyExplanationNodeByID(result.Nodes, "heat.surface_convection.zone_one")
	if surface == nil || surface.Value != 0.2 || surface.SignedValue != -0.2 || surface.ServiceKind != "heating" || surface.Sign != "negative" {
		t.Fatalf("surface heat node = %#v", surface)
	}
	residual := energyExplanationNodeByID(result.Nodes, "residual.energy.electricity")
	if residual == nil || residual.Value != 2 || residual.Level != "residual" {
		t.Fatalf("residual node = %#v", residual)
	}
	if !energyExplanationHasEdge(result.Edges, "meter_enduse", "measured_meter", "energy.carrier.electricity", "energy.end_use.cooling.electricity") {
		t.Fatalf("missing measured meter edge: %#v", result.Edges)
	}
	if !energyExplanationHasEdge(result.Edges, "delivered_load", "measured_variable", "energy.end_use.cooling.electricity", "load.cooling.zone_one") {
		t.Fatalf("missing delivered-load edge: %#v", result.Edges)
	}
	if !energyExplanationHasEdge(result.Edges, "heat_driver", "derived_balance", "load.cooling.zone_one", "heat.internal_convective.zone_one") {
		t.Fatalf("missing heat-driver edge: %#v", result.Edges)
	}
	if !energyExplanationHasEdge(result.Edges, "residual", "residual", "energy.carrier.electricity", "residual.energy.electricity") {
		t.Fatalf("missing residual edge: %#v", result.Edges)
	}
	if edge := energyExplanationEdgeByIDs(result.Edges, "energy.carrier.electricity", "energy.end_use.cooling.electricity"); edge == nil || edge.RuleID != energyRelationshipRuleMeterEndUse {
		t.Fatalf("meter end-use rule edge = %#v", edge)
	}
	if edge := energyExplanationEdgeByIDs(result.Edges, "energy.end_use.cooling.electricity", "load.cooling.zone_one"); edge == nil || edge.RuleID != energyRelationshipRuleMeasuredLoad {
		t.Fatalf("measured load rule edge = %#v", edge)
	}
	if edge := energyExplanationEdgeByIDs(result.Edges, "load.cooling.zone_one", "heat.internal_convective.zone_one"); edge == nil || edge.RuleID != energyRelationshipRuleHeatDriverBalance {
		t.Fatalf("heat driver rule edge = %#v", edge)
	}
	if edge := energyExplanationEdgeByIDs(result.Edges, "energy.carrier.electricity", "residual.energy.electricity"); edge == nil || edge.RuleID != energyRelationshipRuleEnergyResidual {
		t.Fatalf("energy residual rule edge = %#v", edge)
	}
	energyReconciliation := energyExplanationReconciliationByID(result.Reconciliation, "reconcile.energy.electricity.annual")
	heatReconciliation := energyExplanationReconciliationByID(result.Reconciliation, "reconcile.heat.cooling.annual")
	if energyReconciliation == nil || energyReconciliation.Status != "residual" || energyReconciliation.ResidualValue != 2 || heatReconciliation == nil || heatReconciliation.Status != "balanced" || heatReconciliation.ResidualValue != 0 || heatReconciliation.ExplainedValue != 0.5 || result.Completeness.MappedPercent != 33.333 {
		t.Fatalf("reconciliation/completeness = %#v / %#v", result.Reconciliation, result.Completeness)
	}
	if !stringSliceContains(energyReconciliation.SourceIDs, "sql-rdd-20") || !stringSliceContains(energyReconciliation.SourceIDs, "sql-rdd-21") {
		t.Fatalf("energy reconciliation sources = %#v", energyReconciliation.SourceIDs)
	}
	zoneHeatReconciliation := energyExplanationReconciliationByID(result.Reconciliation, "reconcile.heat.cooling.zone_one.annual")
	if zoneHeatReconciliation == nil || zoneHeatReconciliation.Status != "balanced" || zoneHeatReconciliation.ZoneName != "ZONE ONE" || zoneHeatReconciliation.ServiceKind != "cooling" || zoneHeatReconciliation.ExpectedValue != 0.5 || zoneHeatReconciliation.ExplainedValue != 0.5 || zoneHeatReconciliation.ResidualValue != 0 {
		t.Fatalf("zone heat reconciliation = %#v", zoneHeatReconciliation)
	}
	if !stringSliceContains(zoneHeatReconciliation.SourceIDs, "sql-rdd-23") || !stringSliceContains(zoneHeatReconciliation.SourceIDs, "sql-rdd-24") {
		t.Fatalf("zone heat reconciliation sources = %#v", zoneHeatReconciliation.SourceIDs)
	}
	monthlyHeatReconciliation := energyExplanationReconciliationByID(result.Periods[1].Reconciliation, "reconcile.heat.cooling.M1")
	if monthlyHeatReconciliation == nil || monthlyHeatReconciliation.Status != "overmapped" || monthlyHeatReconciliation.ExpectedValue != 0.25 || monthlyHeatReconciliation.ExplainedValue != 0.5 || monthlyHeatReconciliation.ResidualValue != -0.25 {
		t.Fatalf("monthly heat reconciliation = %#v", result.Periods[1].Reconciliation)
	}
	monthlyZoneHeatReconciliation := energyExplanationReconciliationByID(result.Periods[1].Reconciliation, "reconcile.heat.cooling.zone_one.M1")
	if monthlyZoneHeatReconciliation == nil || monthlyZoneHeatReconciliation.Status != "overmapped" || monthlyZoneHeatReconciliation.ZoneName != "ZONE ONE" || monthlyZoneHeatReconciliation.ExpectedValue != 0.25 || monthlyZoneHeatReconciliation.ExplainedValue != 0.5 || monthlyZoneHeatReconciliation.ResidualValue != -0.25 {
		t.Fatalf("monthly zone heat reconciliation = %#v", result.Periods[1].Reconciliation)
	}
	if warning := energyExplanationWarningByCode(result.Periods[1].Warnings, "zone_heat_residual_gap"); warning == nil || warning.Period != "M1" || !strings.Contains(warning.Message, "ZONE ONE") {
		t.Fatalf("monthly zone heat residual warning = %#v", result.Periods[1].Warnings)
	}
	if result.Completeness.HeatDrivers.Found != 2 || result.Completeness.HeatDrivers.Total != 2 || result.Completeness.DeliveredLoad.Found != 1 || result.Completeness.DeliveredLoad.Total != 1 {
		t.Fatalf("explanation completeness = %#v", result.Completeness)
	}
	if availability := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, "Zone Air Heat Balance Surface Convection Rate"); availability == nil || availability.Status != "found" || availability.Level != "heat" || !stringSliceContains(availability.SourceIDs, "sql-rdd-25") {
		t.Fatalf("source availability = %#v", result.Completeness.SourceAvailability)
	}
	if availability := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, "Cooling:Electricity"); availability == nil || availability.Status != "found" || availability.Level != "energy" || !stringSliceContains(availability.SourceIDs, "sql-rdd-21") {
		t.Fatalf("aliased source availability = %#v", result.Completeness.SourceAvailability)
	}
	if len(result.Sources) != 5 || !energyExplanationHasSource(result.Sources, "sql-rdd-20", true, "Electricity:Facility") || !energyExplanationHasSource(result.Sources, "sql-rdd-24", false, "Zone Air Heat Balance Internal Convective Heat Gain Rate") {
		t.Fatalf("sources = %#v", result.Sources)
	}
	if source := energyExplanationSourceByID(result.Sources, "sql-rdd-20"); source == nil || source.ObjectIndex == nil || *source.ObjectIndex != facilityObjectIndex || source.AggregationMethod != "sum_report_data" {
		t.Fatalf("facility source object index = %#v", source)
	}
	if source := energyExplanationSourceByID(result.Sources, "sql-rdd-21"); source == nil || source.ObjectIndex == nil || *source.ObjectIndex != coolingObjectIndex {
		t.Fatalf("aliased cooling source object index = %#v", source)
	}
	if source := energyExplanationSourceByID(result.Sources, "sql-rdd-20"); source == nil || source.Units != "J" || source.SourceUnit != "J" || source.NormalizedUnit != "kWh" {
		t.Fatalf("facility source units = %#v", source)
	}
	if source := energyExplanationSourceByID(result.Sources, "sql-rdd-20"); source == nil || source.TableName != "ReportData" || source.RowName != "Electricity:Facility" || source.ColumnName != "Value [J]" {
		t.Fatalf("facility source table metadata = %#v", source)
	}
	if source := energyExplanationSourceByID(result.Sources, "sql-rdd-24"); source == nil || source.ObjectIndex == nil || *source.ObjectIndex != internalHeatObjectIndex || source.AggregationMethod != "integrate_rate_by_time_interval" {
		t.Fatalf("heat source object index = %#v", source)
	}
	if source := energyExplanationSourceByID(result.Sources, "sql-rdd-24"); source == nil || source.Units != "W" || source.SourceUnit != "W" || source.NormalizedUnit != "kWh" {
		t.Fatalf("heat source units = %#v", source)
	}
	if source := energyExplanationSourceByID(result.Sources, "sql-rdd-24"); source == nil || source.TableName != "ReportData" || source.RowName != "ZONE ONE / Zone Air Heat Balance Internal Convective Heat Gain Rate" || source.ColumnName != "Value [W]" {
		t.Fatalf("heat source table metadata = %#v", source)
	}
	summary := buildEnergyExplanationSummary(result)
	if summary.Schema != energyExplanationSummarySchema || summary.AllocationPolicy != PurposeAllocationPolicyDirectOnly || len(summary.EnergyByCarrier) != 1 || summary.EnergyByCarrier[0].Value != 3 {
		t.Fatalf("summary energy = %#v", summary)
	}
	if item := energyExplanationSummaryItemByID(summary.DeliveredLoadByService, "load.zone_cooling"); item == nil || item.Value != 0.5 || item.PathType != "zone" {
		t.Fatalf("summary delivered load = %#v", summary.DeliveredLoadByService)
	}
	if item := energyExplanationSummaryItemByID(summary.DerivedKPIs, "kpi.cooling_cop"); item == nil || item.Value != 0.5 || item.ServiceKind != "cooling" || item.PathType != "zone" || item.Basis != "derived_kpi" || item.Formula == "" || item.NumeratorValue != 0.5 || item.NumeratorUnit != "kWh" || item.DenominatorValue != 1 || item.DenominatorUnit != "kWh" || !stringSliceContains(item.SourceIDs, "sql-rdd-21") || !stringSliceContains(item.SourceIDs, "sql-rdd-23") {
		t.Fatalf("summary derived KPIs = %#v", summary.DerivedKPIs)
	}
	if item := energyExplanationSummaryItemByID(summary.TopHeatDrivers, "heat.internal_convective"); item == nil || item.Value != 0.5 {
		t.Fatalf("summary top heat drivers = %#v", summary.TopHeatDrivers)
	}
	if item := energyExplanationSummaryItemByID(summary.TopZones, "zone.zone_one"); item == nil || item.Value != 0.7 {
		t.Fatalf("summary top zones = %#v", summary.TopZones)
	}
}

func TestParseSimulationEnergyExplanationSQLPreservesHeatGainLossSigns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, 'ZONE ONE', 'Zone Infiltration Sensible Heat Gain Energy', 'J'),
		(24, 'ZONE ONE', 'Zone Infiltration Sensible Heat Loss Energy', 'J')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 1, 23, 900000.0),
		(8, 2, 23, 900000.0),
		(9, 1, 24, 450000.0),
		(10, 2, 24, 450000.0)`); err != nil {
		t.Fatal(err)
	}

	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Infiltration Sensible Heat Gain Energy"},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Infiltration Sensible Heat Loss Energy"},
	}}
	result, err := parseSimulationEnergyExplanationSQL(path, plan)
	if err != nil {
		t.Fatal(err)
	}
	gain := energyExplanationNodeByID(result.Nodes, "heat.infiltration.positive.zone_one")
	if gain == nil || gain.Label != "Infiltration heat gain" || gain.Value != 0.5 || gain.SignedValue != 0.5 || gain.ServiceKind != "cooling" || gain.Sign != "positive" || !stringSliceContains(gain.SourceIDs, "sql-rdd-23") {
		t.Fatalf("infiltration gain node = %#v", gain)
	}
	loss := energyExplanationNodeByID(result.Nodes, "heat.infiltration.negative.zone_one")
	if loss == nil || loss.Label != "Infiltration heat loss" || loss.Value != 0.25 || loss.SignedValue != -0.25 || loss.ServiceKind != "heating" || loss.Sign != "negative" || !stringSliceContains(loss.SourceIDs, "sql-rdd-24") {
		t.Fatalf("infiltration loss node = %#v", loss)
	}
	if result.Completeness.HeatDrivers.Found != 1 || result.Completeness.HeatDrivers.Total != 1 {
		t.Fatalf("heat completeness = %#v", result.Completeness.HeatDrivers)
	}
	summary := buildEnergyExplanationSummary(result)
	if item := energyExplanationSummaryItemByID(summary.HeatDrivers, "heat.infiltration.positive"); item == nil || item.Label != "Infiltration heat gain" || item.Value != 0.5 || item.HeatCategory != "air_exchange" || item.Sign != "positive" {
		t.Fatalf("summary infiltration gain = %#v", summary.HeatDrivers)
	}
	if item := energyExplanationSummaryItemByID(summary.HeatDrivers, "heat.infiltration.negative"); item == nil || item.Label != "Infiltration heat loss" || item.Value != 0.25 || item.HeatCategory != "air_exchange" || item.Sign != "negative" {
		t.Fatalf("summary infiltration loss = %#v", summary.HeatDrivers)
	}
}

func TestParseSimulationEnergyExplanationSQLMapsWindowSolarHeatDriver(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, 'ZONE ONE', 'Zone Windows Total Transmitted Solar Radiation Energy', 'J')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 1, 23, 1800000.0),
		(8, 2, 23, 1800000.0)`); err != nil {
		t.Fatal(err)
	}

	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Windows Total Transmitted Solar Radiation Energy"},
	}}
	result, err := parseSimulationEnergyExplanationSQL(path, plan)
	if err != nil {
		t.Fatal(err)
	}
	solar := energyExplanationNodeByID(result.Nodes, "heat.solar_window.zone_one")
	if solar == nil || solar.Value != 1 || solar.SignedValue != 1 || solar.ServiceKind != "cooling" || solar.HeatCategory != "surface_envelope" || !stringSliceContains(solar.SourceIDs, "sql-rdd-23") {
		t.Fatalf("window solar heat driver = %#v", solar)
	}
	if result.Completeness.HeatDrivers.Found != 1 || result.Completeness.HeatDrivers.Total != 1 {
		t.Fatalf("solar heat completeness = %#v", result.Completeness.HeatDrivers)
	}
	if availability := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, "Zone Windows Total Transmitted Solar Radiation Energy"); availability == nil || availability.Status != "found" || availability.Level != "heat" {
		t.Fatalf("solar source availability = %#v", result.Completeness.SourceAvailability)
	}
}

func TestParseSimulationEnergyExplanationSQLMapsWindowHeatGainLossDrivers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, 'ZONE ONE', 'Zone Windows Total Heat Gain Energy', 'J'),
		(24, 'ZONE ONE', 'Zone Windows Total Heat Loss Energy', 'J')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 1, 23, 1800000.0),
		(8, 2, 23, 1800000.0),
		(9, 1, 24, 900000.0),
		(10, 2, 24, 900000.0)`); err != nil {
		t.Fatal(err)
	}

	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Windows Total Heat Gain Energy"},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Windows Total Heat Loss Energy"},
	}}
	result, err := parseSimulationEnergyExplanationSQL(path, plan)
	if err != nil {
		t.Fatal(err)
	}
	gain := energyExplanationNodeByID(result.Nodes, "heat.window_heat_transfer.positive.zone_one")
	if gain == nil || gain.Label != "Window heat gain" || gain.Value != 1 || gain.SignedValue != 1 || gain.ServiceKind != "cooling" || gain.HeatCategory != "surface_envelope" || !stringSliceContains(gain.SourceIDs, "sql-rdd-23") {
		t.Fatalf("window gain heat driver = %#v", gain)
	}
	loss := energyExplanationNodeByID(result.Nodes, "heat.window_heat_transfer.negative.zone_one")
	if loss == nil || loss.Label != "Window heat loss" || loss.Value != 0.5 || loss.SignedValue != -0.5 || loss.ServiceKind != "heating" || loss.HeatCategory != "surface_envelope" || !stringSliceContains(loss.SourceIDs, "sql-rdd-24") {
		t.Fatalf("window loss heat driver = %#v", loss)
	}
	if result.Completeness.HeatDrivers.Found != 1 || result.Completeness.HeatDrivers.Total != 1 {
		t.Fatalf("window heat completeness = %#v", result.Completeness.HeatDrivers)
	}
	summary := buildEnergyExplanationSummary(result)
	if item := energyExplanationSummaryItemByID(summary.HeatDrivers, "heat.window_heat_transfer.positive"); item == nil || item.Label != "Window heat gain" || item.Value != 1 {
		t.Fatalf("summary window gain = %#v", summary.HeatDrivers)
	}
	if item := energyExplanationSummaryItemByID(summary.HeatDrivers, "heat.window_heat_transfer.negative"); item == nil || item.Label != "Window heat loss" || item.Value != 0.5 {
		t.Fatalf("summary window loss = %#v", summary.HeatDrivers)
	}
}

func TestParseSimulationEnergyExplanationSQLMapsLatentAndVentilationLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, 'ZONE ONE', 'Zone Ideal Loads Supply Air Latent Heating Energy', 'J'),
		(24, 'ZONE ONE', 'Zone Ideal Loads Supply Air Latent Cooling Energy', 'J'),
		(25, 'ZONE ONE', 'Zone Ideal Loads Outdoor Air Total Heating Energy', 'J')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 1, 23, 900000.0),
		(8, 2, 23, 900000.0),
		(9, 1, 24, 1800000.0),
		(10, 2, 24, 1800000.0),
		(11, 1, 25, 2700000.0),
		(12, 2, 25, 2700000.0)`); err != nil {
		t.Fatal(err)
	}

	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Ideal Loads Supply Air Latent Heating Energy"},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Ideal Loads Supply Air Latent Cooling Energy"},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Ideal Loads Outdoor Air Total Heating Energy"},
	}}
	result, err := parseSimulationEnergyExplanationSQL(path, plan)
	if err != nil {
		t.Fatal(err)
	}
	humidification := energyExplanationNodeByID(result.Nodes, "load.humidification.zone_one")
	if humidification == nil || humidification.Value != 0.5 || humidification.ServiceKind != "humidification" || humidification.Kind != "load.zone_humidification" || !stringSliceContains(humidification.SourceIDs, "sql-rdd-23") {
		t.Fatalf("humidification load node = %#v", humidification)
	}
	dehumidification := energyExplanationNodeByID(result.Nodes, "load.dehumidification.zone_one")
	if dehumidification == nil || dehumidification.Value != 1 || dehumidification.ServiceKind != "dehumidification" || dehumidification.Kind != "load.zone_dehumidification" || !stringSliceContains(dehumidification.SourceIDs, "sql-rdd-24") {
		t.Fatalf("dehumidification load node = %#v", dehumidification)
	}
	ventilation := energyExplanationNodeByID(result.Nodes, "load.ventilation.zone_one")
	if ventilation == nil || ventilation.Value != 1.5 || ventilation.ServiceKind != "ventilation" || ventilation.Kind != "load.ventilation_conditioning" || !stringSliceContains(ventilation.SourceIDs, "sql-rdd-25") {
		t.Fatalf("ventilation conditioning load node = %#v", ventilation)
	}
	if result.Completeness.DeliveredLoad.Found != 3 || result.Completeness.DeliveredLoad.Total != 3 {
		t.Fatalf("latent/ventilation load completeness = %#v", result.Completeness.DeliveredLoad)
	}
	summary := buildEnergyExplanationSummary(result)
	if item := energyExplanationSummaryItemByID(summary.DeliveredLoadByService, "load.zone_humidification"); item == nil || item.Value != 0.5 || item.ServiceKind != "humidification" {
		t.Fatalf("summary humidification load = %#v", summary.DeliveredLoadByService)
	}
	if item := energyExplanationSummaryItemByID(summary.DeliveredLoadByService, "load.zone_dehumidification"); item == nil || item.Value != 1 || item.ServiceKind != "dehumidification" {
		t.Fatalf("summary dehumidification load = %#v", summary.DeliveredLoadByService)
	}
	if item := energyExplanationSummaryItemByID(summary.DeliveredLoadByService, "load.ventilation_conditioning"); item == nil || item.Value != 1.5 || item.ServiceKind != "ventilation" {
		t.Fatalf("summary ventilation load = %#v", summary.DeliveredLoadByService)
	}
}

func TestParseSimulationEnergyExplanationSQLPrefersEnergyOverRateFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, 'ZONE ONE', 'Zone Air System Sensible Cooling Energy', 'J'),
		(24, 'ZONE ONE', 'Zone Air System Sensible Cooling Rate', 'W'),
		(25, 'Supply Fan', 'Fan Air Heat Gain Energy', 'J'),
		(26, 'Supply Fan', 'Fan Air Heat Gain Rate', 'W')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 1, 23, 900000.0),
		(8, 2, 23, 900000.0),
		(9, 1, 24, 1000.0),
		(10, 2, 24, 1000.0),
		(11, 1, 25, 360000.0),
		(12, 2, 25, 360000.0),
		(13, 1, 26, 1000.0),
		(14, 2, 26, 1000.0)`); err != nil {
		t.Fatal(err)
	}

	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Air System Sensible Cooling Energy"},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Air System Sensible Cooling Rate"},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Fan Air Heat Gain Energy"},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Fan Air Heat Gain Rate"},
	}}
	result, err := parseSimulationEnergyExplanationSQL(path, plan)
	if err != nil {
		t.Fatal(err)
	}

	load := energyExplanationNodeByID(result.Nodes, "load.cooling.zone_one")
	if load == nil || load.Value != 0.5 || load.Basis != "measured_energy_variable" || !stringSliceContains(load.SourceIDs, "sql-rdd-23") || stringSliceContains(load.SourceIDs, "sql-rdd-24") {
		t.Fatalf("load source priority node = %#v", load)
	}
	heat := energyExplanationNodeByID(result.Nodes, "heat.fan_to_air")
	if heat == nil || heat.Value != 0.2 || !stringSliceContains(heat.SourceIDs, "sql-rdd-25") || stringSliceContains(heat.SourceIDs, "sql-rdd-26") {
		t.Fatalf("heat source priority node = %#v", heat)
	}
	if result.Completeness.DeliveredLoad.Found != 1 || result.Completeness.DeliveredLoad.Total != 1 ||
		result.Completeness.HeatDrivers.Found != 1 || result.Completeness.HeatDrivers.Total != 1 {
		t.Fatalf("fallback completeness should count canonical groups: %#v", result.Completeness)
	}
	if !energyExplanationHasSource(result.Sources, "sql-rdd-24", false, "Zone Air System Sensible Cooling Rate") ||
		!energyExplanationHasSource(result.Sources, "sql-rdd-26", false, "Fan Air Heat Gain Rate") {
		t.Fatalf("fallback sources should stay traceable: %#v", result.Sources)
	}
}

func TestParseSimulationEnergyExplanationSQLSeparatesFanElectricityAndHeat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, '', 'Electricity:Fans', 'J'),
		(24, 'Supply Fan', 'Fan Air Heat Gain Energy', 'J')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 1, 23, 360000.0),
		(8, 2, 23, 720000.0),
		(9, 1, 24, 180000.0),
		(10, 2, 24, 180000.0)`); err != nil {
		t.Fatal(err)
	}

	fanElectricityObjectIndex := 5
	fanHeatObjectIndex := 6
	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
		{ObjectType: "Output:Meter", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "Electricity:Fans", ObjectIndex: &fanElectricityObjectIndex},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "*", VariableName: "Fan Air Heat Gain Energy", ObjectIndex: &fanHeatObjectIndex},
	}}
	result, err := parseSimulationEnergyExplanationSQL(path, plan)
	if err != nil {
		t.Fatal(err)
	}

	fanEnergy := energyExplanationNodeByID(result.Nodes, "energy.end_use.fans.electricity")
	if fanEnergy == nil || fanEnergy.Level != "energy" || fanEnergy.Value != 0.3 || fanEnergy.EndUse != "fans" || fanEnergy.Carrier != "electricity" ||
		!stringSliceContains(fanEnergy.SourceIDs, "sql-rdd-23") || stringSliceContains(fanEnergy.SourceIDs, "sql-rdd-24") {
		t.Fatalf("fan electricity node = %#v", fanEnergy)
	}
	fanHeat := energyExplanationNodeByID(result.Nodes, "heat.fan_to_air")
	if fanHeat == nil || fanHeat.Level != "heat" || fanHeat.Value != 0.1 || fanHeat.HeatCategory != "hvac_system" ||
		!stringSliceContains(fanHeat.SourceIDs, "sql-rdd-24") || stringSliceContains(fanHeat.SourceIDs, "sql-rdd-23") {
		t.Fatalf("fan heat node = %#v", fanHeat)
	}
	if result.Completeness.EnergyUse.Status != "complete" ||
		result.Completeness.HeatDrivers.Found != 1 || result.Completeness.HeatDrivers.Total != 1 {
		t.Fatalf("fan energy/heat completeness = %#v", result.Completeness)
	}
	if availability := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, "Electricity:Fans"); availability == nil || availability.Status != "found" || availability.Level != "energy" || !stringSliceContains(availability.SourceIDs, "sql-rdd-23") {
		t.Fatalf("fan electricity availability = %#v", result.Completeness.SourceAvailability)
	}
	if availability := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, "Fan Air Heat Gain Energy"); availability == nil || availability.Status != "found" || availability.Level != "heat" || !stringSliceContains(availability.SourceIDs, "sql-rdd-24") {
		t.Fatalf("fan heat availability = %#v", result.Completeness.SourceAvailability)
	}
	if source := energyExplanationSourceByID(result.Sources, "sql-rdd-23"); source == nil || !source.IsMeter || source.ObjectIndex == nil || *source.ObjectIndex != fanElectricityObjectIndex {
		t.Fatalf("fan electricity source = %#v", source)
	}
	if source := energyExplanationSourceByID(result.Sources, "sql-rdd-24"); source == nil || source.IsMeter || source.ObjectIndex == nil || *source.ObjectIndex != fanHeatObjectIndex {
		t.Fatalf("fan heat source = %#v", source)
	}

	summary := buildEnergyExplanationSummary(result)
	if item := energyExplanationSummaryItemByID(summary.EnergyByEndUse, "fans.electricity"); item == nil || item.Value != 0.3 || !stringSliceContains(item.SourceIDs, "sql-rdd-23") || stringSliceContains(item.SourceIDs, "sql-rdd-24") {
		t.Fatalf("summary fan electricity = %#v", summary.EnergyByEndUse)
	}
	if item := energyExplanationSummaryItemByID(summary.HeatDrivers, "heat.fan_to_air"); item == nil || item.Value != 0.1 || !stringSliceContains(item.SourceIDs, "sql-rdd-24") || stringSliceContains(item.SourceIDs, "sql-rdd-23") {
		t.Fatalf("summary fan heat = %#v", summary.HeatDrivers)
	}
}

func TestParseSimulationEnergyExplanationSQLIntegratesRateOnlyOutputs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, 'ZONE ONE', 'Zone Air System Sensible Cooling Rate', 'W'),
		(24, 'ZONE ONE', 'Zone Air Heat Balance Internal Convective Heat Gain Rate', 'W')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO "Time" VALUES
		(3, 1, 1, 1, 0),
		(4, 1, 1, 2, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 3, 23, 1000.0),
		(8, 4, 23, 1000.0),
		(9, 3, 24, 250.0),
		(10, 4, 24, 250.0)`); err != nil {
		t.Fatal(err)
	}

	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Air System Sensible Cooling Rate"},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Air Heat Balance Internal Convective Heat Gain Rate"},
	}}
	result, err := parseSimulationEnergyExplanationSQL(path, plan)
	if err != nil {
		t.Fatal(err)
	}

	load := energyExplanationNodeByID(result.Nodes, "load.cooling.zone_one")
	if load == nil || load.Value != 2 || load.Unit != "kWh" || load.Basis != "integrated_rate_variable" || !stringSliceContains(load.SourceIDs, "sql-rdd-23") {
		t.Fatalf("rate-integrated load node = %#v", load)
	}
	heat := energyExplanationNodeByID(result.Nodes, "heat.internal_convective.zone_one")
	if heat == nil || heat.Value != 0.5 || heat.Unit != "kWh" || heat.SignedValue != 0.5 || !stringSliceContains(heat.SourceIDs, "sql-rdd-24") {
		t.Fatalf("rate-integrated heat node = %#v", heat)
	}
	monthly := result.Periods[1]
	if monthly.ID != "M1" {
		t.Fatalf("first monthly period = %#v", monthly)
	}
	if monthLoad := energyExplanationNodeByID(monthly.Nodes, "load.cooling.zone_one"); monthLoad == nil || monthLoad.Value != 2 {
		t.Fatalf("monthly rate-integrated load = %#v", monthLoad)
	}
	if source := energyExplanationSourceByID(result.Sources, "sql-rdd-23"); source == nil || source.AggregationMethod != "integrate_rate_by_time_interval" || source.NormalizedUnit != "kWh" {
		t.Fatalf("rate load source metadata = %#v", source)
	}
	if source := energyExplanationSourceByID(result.Sources, "sql-rdd-24"); source == nil || source.AggregationMethod != "integrate_rate_by_time_interval" || source.NormalizedUnit != "kWh" {
		t.Fatalf("rate heat source metadata = %#v", source)
	}
}

func TestParseSimulationEnergyExplanationSQLWarnsOnLargeHeatBalanceDeviation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, 'ZONE ONE', 'Zone Air System Sensible Cooling Rate', 'W'),
		(24, 'ZONE ONE', 'Zone Air Heat Balance Deviation Rate', 'W')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO "Time" VALUES
		(3, 1, 1, 1, 0),
		(4, 1, 1, 2, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 3, 23, 1000.0),
		(8, 4, 23, 1000.0),
		(9, 3, 24, 500.0),
		(10, 4, 24, 500.0)`); err != nil {
		t.Fatal(err)
	}

	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Air System Sensible Cooling Rate"},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Air Heat Balance Deviation Rate"},
	}}
	result, err := parseSimulationEnergyExplanationSQL(path, plan)
	if err != nil {
		t.Fatal(err)
	}

	warning := energyExplanationWarningByCode(result.Warnings, "heat_balance_deviation_large")
	if warning == nil || warning.Period != "annual" || !strings.Contains(warning.Message, "ZONE ONE") || !strings.Contains(warning.Message, "1 kWh") {
		t.Fatalf("annual heat-balance deviation warning = %#v", result.Warnings)
	}
	monthly := energyExplanationPeriodByID(result.Periods, "M1")
	if monthly == nil {
		t.Fatalf("monthly period missing from %#v", result.Periods)
	}
	if warning := energyExplanationWarningByCode(monthly.Warnings, "heat_balance_deviation_large"); warning == nil || warning.Period != "M1" || !strings.Contains(warning.Message, "ZONE ONE") {
		t.Fatalf("monthly heat-balance deviation warning = %#v", monthly.Warnings)
	}
	deviation := energyExplanationNodeByID(result.Nodes, "heat.zone_balance_residual.zone_one")
	if deviation == nil || deviation.Value != 1 || deviation.HeatCategory != "storage_residual" || deviation.Sign != "positive" {
		t.Fatalf("heat-balance deviation node = %#v", deviation)
	}
}

func TestParseSimulationEnergyExplanationSQLScopesDeliveredLoadNodes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, 'ZONE ONE', 'Zone Air System Sensible Cooling Energy', 'J'),
		(24, 'Cooling Coil', 'Cooling Coil Total Cooling Energy', 'J'),
		(25, 'CHW Loop', 'Plant Loop Cooling Demand Energy', 'J')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 1, 23, 900000.0),
		(8, 2, 23, 900000.0),
		(9, 1, 24, 1800000.0),
		(10, 2, 24, 1800000.0),
		(11, 1, 25, 3600000.0),
		(12, 2, 25, 3600000.0)`); err != nil {
		t.Fatal(err)
	}

	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Air System Sensible Cooling Energy"},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Cooling Coil Total Cooling Energy"},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Plant Loop Cooling Demand Energy"},
	}}
	result, err := parseSimulationEnergyExplanationSQL(path, plan)
	if err != nil {
		t.Fatal(err)
	}

	zoneLoad := energyExplanationNodeByID(result.Nodes, "load.cooling.zone_one")
	if zoneLoad == nil || zoneLoad.ZoneName != "ZONE ONE" || zoneLoad.LoopName != "" || zoneLoad.Value != 0.5 {
		t.Fatalf("zone load node = %#v", zoneLoad)
	}
	systemLoad := energyExplanationNodeByID(result.Nodes, "load.cooling")
	if systemLoad == nil || systemLoad.ZoneName != "" || systemLoad.LoopName != "" || systemLoad.Value != 1 {
		t.Fatalf("system load node = %#v", systemLoad)
	}
	plantLoad := energyExplanationNodeByID(result.Nodes, "load.cooling.chw_loop")
	if plantLoad == nil || plantLoad.ZoneName != "" || plantLoad.LoopName != "CHW Loop" || plantLoad.Value != 2 {
		t.Fatalf("plant load node = %#v", plantLoad)
	}
	heatReconciliation := energyExplanationReconciliationByID(result.Reconciliation, "reconcile.heat.cooling.annual")
	if heatReconciliation == nil || heatReconciliation.ExpectedValue != 0.5 {
		t.Fatalf("heat reconciliation should use zone load basis when available: %#v", heatReconciliation)
	}
}

func TestParseSimulationEnergyExplanationSQLMapsPlantUnmetResidualLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, 'CHW Loop', 'Plant Supply Side Unmet Demand Rate', 'W'),
		(24, 'Condenser Loop', 'Cond Loop Demand Not Distributed', 'W')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO "Time" VALUES
		(3, 1, 1, 1, 0),
		(4, 1, 1, 2, 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 3, 23, 1000.0),
		(8, 4, 23, 500.0),
		(9, 3, 24, 250.0),
		(10, 4, 24, 250.0)`); err != nil {
		t.Fatal(err)
	}

	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Plant Supply Side Unmet Demand Rate"},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Cond Loop Demand Not Distributed"},
	}}
	result, err := parseSimulationEnergyExplanationSQL(path, plan)
	if err != nil {
		t.Fatal(err)
	}

	unmet := energyExplanationNodeByID(result.Nodes, "load.unmet_or_residual.chw_loop")
	if unmet == nil || unmet.Value != 1.5 || unmet.Unit != "kWh" || unmet.Basis != "integrated_rate_variable" || unmet.Kind != "load.plant_unmet_or_residual" || unmet.LoopName != "CHW Loop" || !stringSliceContains(unmet.SourceIDs, "sql-rdd-23") {
		t.Fatalf("plant unmet load node = %#v", unmet)
	}
	notDistributed := energyExplanationNodeByID(result.Nodes, "load.unmet_or_residual.condenser_loop")
	if notDistributed == nil || notDistributed.Value != 0.5 || notDistributed.ServiceKind != "unmet_or_residual" || notDistributed.LoopName != "Condenser Loop" || !stringSliceContains(notDistributed.SourceIDs, "sql-rdd-24") {
		t.Fatalf("plant residual load node = %#v", notDistributed)
	}
	if result.Completeness.DeliveredLoad.Found != 1 || result.Completeness.DeliveredLoad.Total != 1 {
		t.Fatalf("plant unmet load completeness = %#v", result.Completeness.DeliveredLoad)
	}
	if availability := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, "Plant Supply Side Unmet Demand Rate"); availability == nil || availability.Status != "found" || availability.Level != "load" {
		t.Fatalf("plant unmet source availability = %#v", result.Completeness.SourceAvailability)
	}
	if availability := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, "Cond Loop Demand Not Distributed"); availability == nil || availability.Status != "found" || availability.Level != "load" {
		t.Fatalf("cond loop not-distributed source availability = %#v", result.Completeness.SourceAvailability)
	}
	summary := buildEnergyExplanationSummary(result)
	if item := energyExplanationSummaryItemByID(summary.DeliveredLoadByService, "load.plant_unmet_or_residual"); item == nil || item.Value != 2 || item.ServiceKind != "unmet_or_residual" || item.PathType != "plant" {
		t.Fatalf("summary plant unmet/residual load = %#v", summary.DeliveredLoadByService)
	}
}

func TestSQLTimeIntervalHoursUsesElapsedFirstPeriod(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	intervals, err := sqlTimeIntervalHours(db)
	if err != nil {
		t.Fatal(err)
	}
	if intervals[1] != 744 {
		t.Fatalf("first monthly interval = %v, want 744 hours", intervals[1])
	}
	if intervals[2] != 672 {
		t.Fatalf("second monthly interval = %v, want 672 hours", intervals[2])
	}
}

func TestEnergyMeterAliasCatalogHandlesCarrierAndEndUseOrder(t *testing.T) {
	coolingA, ok := energyMeterAliasDefinitionForName("Cooling:Electricity")
	if !ok || coolingA.Carrier != "electricity" || coolingA.EndUse != "cooling" || coolingA.HierarchyLevel != "broad_end_use" {
		t.Fatalf("Cooling:Electricity alias = %#v ok=%t", coolingA, ok)
	}
	coolingB, ok := energyMeterAliasDefinitionForName("Electricity:Cooling")
	if !ok || coolingB.Carrier != "electricity" || coolingB.EndUse != "cooling" {
		t.Fatalf("Electricity:Cooling alias = %#v ok=%t", coolingB, ok)
	}
	facility, ok := energyMeterAliasDefinitionForName("NaturalGas:Facility")
	if !ok || !facility.FacilityTotal || facility.Carrier != "natural_gas" {
		t.Fatalf("NaturalGas:Facility alias = %#v ok=%t", facility, ok)
	}
	refrigeration, ok := energyMeterAliasDefinitionForName("Electricity:Refrigeration")
	if !ok || refrigeration.Kind != "energy.refrigeration" || refrigeration.EndUse != "refrigeration" {
		t.Fatalf("Electricity:Refrigeration alias = %#v ok=%t", refrigeration, ok)
	}
	heatRecovery, ok := energyMeterAliasDefinitionForName("HeatRecovery:Electricity")
	if !ok || heatRecovery.Kind != "energy.heat_recovery" || heatRecovery.EndUse != "heat_recovery" {
		t.Fatalf("HeatRecovery:Electricity alias = %#v ok=%t", heatRecovery, ok)
	}
	gasEquipment, ok := energyMeterAliasDefinitionForName("NaturalGas:InteriorEquipment")
	if !ok || gasEquipment.Carrier != "natural_gas" || gasEquipment.EndUse != "interior_equipment" {
		t.Fatalf("NaturalGas:InteriorEquipment alias = %#v ok=%t", gasEquipment, ok)
	}
	storageCharge, ok := energyVariableAliasDefinitionForName("Electric Storage Charge Energy")
	if !ok || storageCharge.Kind != "energy.storage_charge" || storageCharge.EndUse != "storage_charge" {
		t.Fatalf("Electric Storage Charge Energy alias = %#v ok=%t", storageCharge, ok)
	}
	storageDischarge, ok := energyVariableAliasDefinitionForName("Electric Storage Discharge Energy")
	if !ok || storageDischarge.Kind != "energy.storage_discharge" || storageDischarge.EndUse != "storage_discharge" {
		t.Fatalf("Electric Storage Discharge Energy alias = %#v ok=%t", storageDischarge, ok)
	}
	if _, ok := energyMeterAliasDefinitionForName("SomeCustomMeter:Electricity"); ok {
		t.Fatalf("custom meter should not be classified as a known alias")
	}
	propaneHeating, ok := energyMeterAliasOrOtherDefinitionForName("Heating:Propane")
	if !ok || propaneHeating.Kind != "energy.heating" || propaneHeating.Carrier != "propane" || propaneHeating.EndUse != "heating" || propaneHeating.HierarchyLevel != "broad_end_use" {
		t.Fatalf("propane heating meter alias = %#v ok=%t", propaneHeating, ok)
	}
	steamWater, ok := energyMeterAliasOrOtherDefinitionForName("WaterSystems:Steam")
	if !ok || steamWater.Kind != "energy.water_systems" || steamWater.Carrier != "steam" || steamWater.EndUse != "water_systems" {
		t.Fatalf("steam water systems meter alias = %#v ok=%t", steamWater, ok)
	}
	dhw, ok := energyMeterAliasOrOtherDefinitionForName("DHW:FuelOilNo2")
	if !ok || dhw.Kind != "energy.water_systems" || dhw.Carrier != "fuel_oil_2" || dhw.EndUse != "water_systems" {
		t.Fatalf("DHW fuel oil meter alias = %#v ok=%t", dhw, ok)
	}
	exteriorEquipment, ok := energyMeterAliasOrOtherDefinitionForName("ExteriorEquipment:Electricity")
	if !ok || exteriorEquipment.Kind != "energy.exterior_equipment" || exteriorEquipment.Carrier != "electricity" || exteriorEquipment.EndUse != "exterior_equipment" {
		t.Fatalf("exterior equipment meter alias = %#v ok=%t", exteriorEquipment, ok)
	}
	humidifier, ok := energyMeterAliasOrOtherDefinitionForName("Humidifier:NaturalGas")
	if !ok || humidifier.Kind != "energy.humidification" || humidifier.Carrier != "natural_gas" || humidifier.EndUse != "humidification" {
		t.Fatalf("humidifier meter alias = %#v ok=%t", humidifier, ok)
	}
	cogeneration, ok := energyMeterAliasOrOtherDefinitionForName("Cogeneration:Electricity")
	if !ok || cogeneration.Kind != "energy.generators" || cogeneration.Carrier != "electricity" || cogeneration.EndUse != "generators" {
		t.Fatalf("cogeneration meter alias = %#v ok=%t", cogeneration, ok)
	}
	miscellaneous, ok := energyMeterAliasOrOtherDefinitionForName("Miscellaneous:Propane")
	if !ok || miscellaneous.Kind != "energy.other" || miscellaneous.Carrier != "propane" || miscellaneous.EndUse != "other" {
		t.Fatalf("miscellaneous meter alias = %#v ok=%t", miscellaneous, ok)
	}
	other, ok := energyMeterAliasOrOtherDefinitionForName("SomeCustomMeter:Electricity")
	if !ok || other.Carrier != "electricity" || other.EndUse != "other" || other.HierarchyLevel != "broad_end_use" {
		t.Fatalf("custom meter fallback = %#v ok=%t", other, ok)
	}
}

func TestParseSimulationEnergyExplanationSQLMapsUnknownCarrierMeterToOther(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES (30, '', 'SomeCustomMeter:Electricity', 'J')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES (30, 1, 30, 3600000.0)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := parseSimulationEnergyExplanationSQL(path, &PurposeRunPlan{})
	if err != nil {
		t.Fatal(err)
	}
	other := energyExplanationNodeByID(result.Nodes, "energy.end_use.other.electricity")
	if other == nil || other.Value != 1 || other.EndUse != "other" || other.Carrier != "electricity" || other.MeterHierarchyLevel != "broad_end_use" || !stringSliceContains(other.SourceIDs, "sql-rdd-30") {
		t.Fatalf("other electricity node = %#v", other)
	}
	source := energyExplanationSourceByID(result.Sources, "sql-rdd-30")
	if source == nil || source.Name != "SomeCustomMeter:Electricity" || source.KeyValue != "" {
		t.Fatalf("custom source = %#v", source)
	}
	reconciliation := energyExplanationReconciliationByID(result.Reconciliation, "reconcile.energy.electricity.annual")
	if reconciliation == nil || reconciliation.ExpectedValue != 3 || reconciliation.ExplainedValue != 2 || reconciliation.ResidualValue != 1 {
		t.Fatalf("custom meter reconciliation = %#v", reconciliation)
	}
}

func TestParseSimulationEnergyExplanationSQLMapsGenericFuelEndUseMeters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(30, '', 'Propane:Facility', 'J'),
		(31, '', 'Heating:Propane', 'J'),
		(32, '', 'Steam:Facility', 'J'),
		(33, '', 'WaterSystems:Steam', 'J')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(30, 1, 30, 7200000.0),
		(31, 1, 31, 3600000.0),
		(32, 1, 32, 3600000.0),
		(33, 1, 33, 1800000.0)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := parseSimulationEnergyExplanationSQL(path, &PurposeRunPlan{})
	if err != nil {
		t.Fatal(err)
	}
	propaneHeating := energyExplanationNodeByID(result.Nodes, "energy.end_use.heating.propane")
	if propaneHeating == nil || propaneHeating.Value != 1 || propaneHeating.Kind != "energy.heating" || propaneHeating.Carrier != "propane" || propaneHeating.EndUse != "heating" {
		t.Fatalf("propane heating end-use node = %#v", propaneHeating)
	}
	steamWater := energyExplanationNodeByID(result.Nodes, "energy.end_use.water_systems.steam")
	if steamWater == nil || steamWater.Value != 0.5 || steamWater.Kind != "energy.water_systems" || steamWater.Carrier != "steam" || steamWater.EndUse != "water_systems" {
		t.Fatalf("steam water systems end-use node = %#v", steamWater)
	}
	if edge := energyExplanationEdgeByIDs(result.Edges, "energy.carrier.propane", "energy.end_use.heating.propane"); edge == nil || edge.Relation != "meter_enduse" || edge.Value != 1 {
		t.Fatalf("propane heating edge = %#v; all edges = %#v", edge, result.Edges)
	}
	summary := buildEnergyExplanationSummary(result)
	if item := energyExplanationSummaryItemByID(summary.EnergyByEndUse, "heating.propane"); item == nil || item.Value != 1 || item.Carrier != "propane" {
		t.Fatalf("propane heating summary = %#v", summary.EnergyByEndUse)
	}
}

func TestParseSimulationEnergyExplanationSQLMapsElectricStorageVariables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(30, 'Battery', 'Electric Storage Charge Energy', 'J'),
		(31, 'Battery', 'Electric Storage Discharge Energy', 'J')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(30, 1, 30, 1800000.0),
		(31, 2, 30, 1800000.0),
		(32, 1, 31, 900000.0),
		(33, 2, 31, 900000.0)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	chargeObjectIndex := 5
	dischargeObjectIndex := 6
	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "*", VariableName: "Electric Storage Charge Energy", ObjectIndex: &chargeObjectIndex},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "*", VariableName: "Electric Storage Discharge Energy", ObjectIndex: &dischargeObjectIndex},
	}}
	result, err := parseSimulationEnergyExplanationSQL(path, plan)
	if err != nil {
		t.Fatal(err)
	}
	charge := energyExplanationNodeByID(result.Nodes, "energy.end_use.storage_charge.electricity")
	if charge == nil || charge.Value != 1 || charge.Basis != "measured_energy_variable" || charge.EndUse != "storage_charge" || !stringSliceContains(charge.SourceIDs, "sql-rdd-30") {
		t.Fatalf("storage charge node = %#v", charge)
	}
	discharge := energyExplanationNodeByID(result.Nodes, "energy.end_use.storage_discharge.electricity")
	if discharge == nil || discharge.Value != 0.5 || discharge.Basis != "measured_energy_variable" || discharge.EndUse != "storage_discharge" || !stringSliceContains(discharge.SourceIDs, "sql-rdd-31") {
		t.Fatalf("storage discharge node = %#v", discharge)
	}
	chargeEdge := energyExplanationEdgeByIDs(result.Edges, "energy.carrier.electricity", "energy.end_use.storage_charge.electricity")
	if chargeEdge == nil || chargeEdge.Relation != "energy_variable" || chargeEdge.Basis != "measured_energy_variable" || chargeEdge.RuleID != energyRelationshipRuleMeasuredEnergyVariable {
		t.Fatalf("storage charge edge = %#v", chargeEdge)
	}
	dischargeEdge := energyExplanationEdgeByIDs(result.Edges, "energy.carrier.electricity", "energy.end_use.storage_discharge.electricity")
	if dischargeEdge == nil || dischargeEdge.Relation != "storage_discharge" || dischargeEdge.Basis != "measured_energy_variable" || dischargeEdge.RuleID != energyRelationshipRuleStorageDischarge {
		t.Fatalf("storage discharge edge = %#v", dischargeEdge)
	}
	reconciliation := energyExplanationReconciliationByID(result.Reconciliation, "reconcile.energy.electricity.annual")
	if reconciliation == nil || reconciliation.ExpectedValue != 3 || reconciliation.ExplainedValue != 2 || reconciliation.ResidualValue != 1 {
		t.Fatalf("storage reconciliation should count charge but not discharge as consumption: %#v", reconciliation)
	}
	if source := energyExplanationSourceByID(result.Sources, "sql-rdd-30"); source == nil || source.ObjectIndex == nil || *source.ObjectIndex != chargeObjectIndex {
		t.Fatalf("storage charge source = %#v", source)
	}
	if availability := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, "Electric Storage Discharge Energy"); availability == nil || availability.Status != "found" || availability.Level != "energy" {
		t.Fatalf("storage source availability = %#v", result.Completeness.SourceAvailability)
	}
}

func TestEnergyRelationshipRuleCatalogProvidesEdgeBasis(t *testing.T) {
	rules := energyRelationshipRuleCatalog()
	if len(rules) < 5 {
		t.Fatalf("rule catalog = %#v", rules)
	}
	meterRule := energyRelationshipRuleByID(energyRelationshipRuleMeterEndUse)
	if meterRule.Basis != "measured_meter" || meterRule.FromLevel != "energy" || meterRule.ToLevel != "energy" || len(meterRule.RequiredSource) == 0 {
		t.Fatalf("meter rule = %#v", meterRule)
	}
	energyVariableRule := energyRelationshipRuleByID(energyRelationshipRuleMeasuredEnergyVariable)
	if energyVariableRule.Basis != "measured_energy_variable" || energyVariableRule.FromLevel != "energy" || energyVariableRule.ToLevel != "energy" {
		t.Fatalf("energy variable rule = %#v", energyVariableRule)
	}
	loadRule := energyRelationshipRuleByID(energyRelationshipRuleMeasuredLoad)
	if loadRule.Basis != "measured_variable" || !strings.Contains(loadRule.Formula, "not COP-converted") {
		t.Fatalf("load rule = %#v", loadRule)
	}
	heatRule := energyRelationshipRuleByID(energyRelationshipRuleHeatDriverBalance)
	if heatRule.Basis != "derived_balance" || heatRule.FromLevel != "load" || heatRule.ToLevel != "heat" {
		t.Fatalf("heat rule = %#v", heatRule)
	}
	internalGainRule := energyRelationshipRuleByID(energyRelationshipRuleInternalGainHeat)
	if internalGainRule.Basis != "measured_meter_plus_zone_gain_variable" || internalGainRule.FromLevel != "energy" || internalGainRule.ToLevel != "heat" {
		t.Fatalf("internal gain rule = %#v", internalGainRule)
	}
	allocationRule := energyRelationshipRuleByID(energyRelationshipRuleAllocatedZoneLoad)
	if allocationRule.Basis != "allocated" || allocationRule.FromLevel != "energy" || allocationRule.ToLevel != "load" {
		t.Fatalf("allocation rule = %#v", allocationRule)
	}
	servicePathAllocationRule := energyRelationshipRuleByID(energyRelationshipRuleAllocatedServicePathLoad)
	if servicePathAllocationRule.Basis != "allocated" || !strings.Contains(servicePathAllocationRule.Formula, "service path") {
		t.Fatalf("service path allocation rule = %#v", servicePathAllocationRule)
	}
	productionRule := energyRelationshipRuleByID(energyRelationshipRuleOnsiteProduction)
	if productionRule.Basis != "measured_meter" || !strings.Contains(productionRule.Formula, "separately") {
		t.Fatalf("onsite production rule = %#v", productionRule)
	}
	storageDischargeRule := energyRelationshipRuleByID(energyRelationshipRuleStorageDischarge)
	if storageDischargeRule.Basis != "measured_energy_variable" || !strings.Contains(storageDischargeRule.Formula, "storage discharge") {
		t.Fatalf("storage discharge rule = %#v", storageDischargeRule)
	}
	if request := NormalizeSimulationPurposeRequest(&SimulationPurposeRequest{AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare}); request.AllocationPolicy != PurposeAllocationPolicyByServicePathLoadShare {
		t.Fatalf("service path allocation policy normalized to %q", request.AllocationPolicy)
	}
}

func TestBuildEnergyExplanationLinksInteriorLightingEnergyToLightingHeat(t *testing.T) {
	result := buildEnergyExplanationResult([]energyExplanationSeries{
		{
			Level:               "energy",
			Kind:                "energy.interior_lighting",
			Label:               "Interior lighting",
			Unit:                "kWh",
			Carrier:             "electricity",
			EndUse:              "interior_lighting",
			MeterHierarchyLevel: "broad_end_use",
			SourceIDs:           []string{"lighting-energy-source"},
			Total:               1.5,
			Monthly:             map[int]float64{1: 0.75},
		},
		{
			Level:              "heat",
			Kind:               "heat.lighting",
			Label:              "Lighting heat",
			Unit:               "kWh",
			ZoneName:           "Office",
			HeatCategory:       "internal_gains",
			SourceIDs:          []string{"lighting-heat-source"},
			Total:              1.2,
			Monthly:            map[int]float64{1: 0.6},
			sourceName:         "Zone Lights Total Heating Energy",
			heatSignMultiplier: 1,
		},
	}, nil, &PurposeRunPlan{})

	fromID := "energy.end_use.interior_lighting.electricity"
	toID := "heat.lighting.office"
	edge := energyExplanationEdgeByIDs(result.Edges, fromID, toID)
	if edge == nil || edge.Relation != "internal_gain_heat" || edge.Basis != "measured_meter_plus_zone_gain_variable" || edge.RuleID != energyRelationshipRuleInternalGainHeat || edge.Value != 1.2 ||
		!stringSliceContains(edge.SourceIDs, "lighting-energy-source") || !stringSliceContains(edge.SourceIDs, "lighting-heat-source") {
		t.Fatalf("lighting energy heat edge = %#v", edge)
	}
	monthly := energyExplanationPeriodByID(result.Periods, "M1")
	if monthly == nil {
		t.Fatalf("monthly period missing from %#v", result.Periods)
	}
	monthlyEdge := energyExplanationEdgeByIDs(monthly.Edges, fromID, toID)
	if monthlyEdge == nil || monthlyEdge.Value != 0.6 || monthlyEdge.RuleID != energyRelationshipRuleInternalGainHeat {
		t.Fatalf("monthly lighting energy heat edge = %#v", monthlyEdge)
	}
}

func TestBuildEnergyExplanationSankeyGraphContracts(t *testing.T) {
	result := buildEnergyExplanationResult([]energyExplanationSeries{
		{
			Level:               "energy",
			Kind:                "energy.total",
			Label:               "Electricity total",
			Unit:                "kWh",
			Carrier:             "electricity",
			MeterHierarchyLevel: "facility_total",
			SourceIDs:           []string{"facility-source"},
			Total:               10,
			Monthly:             map[int]float64{1: 5},
		},
		{
			Level:               "energy",
			Kind:                "energy.cooling",
			Label:               "Cooling energy",
			Unit:                "kWh",
			Carrier:             "electricity",
			EndUse:              "cooling",
			MeterHierarchyLevel: "broad_end_use",
			SourceIDs:           []string{"cooling-source"},
			Total:               4,
			Monthly:             map[int]float64{1: 2},
		},
		{
			Level:       "load",
			Kind:        "load.zone_cooling",
			Label:       "Zone cooling load",
			Unit:        "kWh",
			ServiceKind: "cooling",
			PathType:    "zone",
			ZoneName:    "Office",
			SourceIDs:   []string{"load-source"},
			Total:       3,
			Monthly:     map[int]float64{1: 1.5},
		},
		{
			Level:              "heat",
			Kind:               "heat.internal_convective",
			Label:              "Internal convection",
			Unit:               "kWh",
			ZoneName:           "Office",
			HeatCategory:       "internal_gains",
			SourceIDs:          []string{"heat-source"},
			Total:              1,
			Monthly:            map[int]float64{1: 0.5},
			heatSignMultiplier: 1,
		},
	}, nil, &PurposeRunPlan{})

	expectedNodeIDs := []string{
		"energy.carrier.electricity",
		"energy.end_use.cooling.electricity",
		"load.cooling.office",
		"heat.internal_convective.office",
		"residual.energy.electricity",
	}
	for _, id := range expectedNodeIDs {
		if energyExplanationNodeByID(result.Nodes, id) == nil {
			t.Fatalf("annual node %q missing from %#v", id, result.Nodes)
		}
	}
	monthly := energyExplanationPeriodByID(result.Periods, "M1")
	if monthly == nil {
		t.Fatalf("monthly period missing from %#v", result.Periods)
	}
	for _, id := range expectedNodeIDs {
		if energyExplanationNodeByID(monthly.Nodes, id) == nil {
			t.Fatalf("monthly node %q missing from %#v", id, monthly.Nodes)
		}
	}

	annualEdges := []struct {
		from     string
		to       string
		relation string
		basis    string
		ruleID   string
	}{
		{"energy.carrier.electricity", "energy.end_use.cooling.electricity", "meter_enduse", "measured_meter", energyRelationshipRuleMeterEndUse},
		{"energy.end_use.cooling.electricity", "load.cooling.office", "delivered_load", "measured_variable", energyRelationshipRuleMeasuredLoad},
		{"load.cooling.office", "heat.internal_convective.office", "heat_driver", "derived_balance", energyRelationshipRuleHeatDriverBalance},
		{"energy.carrier.electricity", "residual.energy.electricity", "residual", "residual", energyRelationshipRuleEnergyResidual},
	}
	for _, expected := range annualEdges {
		edge := energyExplanationEdgeByIDs(result.Edges, expected.from, expected.to)
		if edge == nil || edge.Relation != expected.relation || edge.Basis != expected.basis || edge.RuleID != expected.ruleID {
			t.Fatalf("annual edge %s -> %s = %#v", expected.from, expected.to, edge)
		}
		monthlyEdge := energyExplanationEdgeByIDs(monthly.Edges, expected.from, expected.to)
		if monthlyEdge == nil || monthlyEdge.Relation != expected.relation || monthlyEdge.Basis != expected.basis || monthlyEdge.RuleID != expected.ruleID {
			t.Fatalf("monthly edge %s -> %s = %#v", expected.from, expected.to, monthlyEdge)
		}
		if edge.ID != monthlyEdge.ID {
			t.Fatalf("edge ID should be period-stable for %s -> %s: annual=%q monthly=%q", expected.from, expected.to, edge.ID, monthlyEdge.ID)
		}
		if strings.Contains(edge.ID, ".annual.") || strings.Contains(monthlyEdge.ID, ".M1.") {
			t.Fatalf("edge ID should not embed period: annual=%q monthly=%q", edge.ID, monthlyEdge.ID)
		}
	}
}

func TestBuildEnergyExplanationAllocatedZoneLoadShare(t *testing.T) {
	result := buildEnergyExplanationResult([]energyExplanationSeries{
		{
			Level:     "energy",
			Kind:      "energy.cooling",
			Label:     "Cooling energy",
			Unit:      "kWh",
			Carrier:   "electricity",
			EndUse:    "cooling",
			SourceIDs: []string{"energy-source"},
			Total:     4,
		},
		{
			Level:       "load",
			Kind:        "load.zone_cooling",
			Label:       "Zone cooling load",
			Unit:        "kWh",
			ServiceKind: "cooling",
			ZoneName:    "ZONE A",
			SourceIDs:   []string{"load-a"},
			Total:       1,
		},
		{
			Level:       "load",
			Kind:        "load.zone_cooling",
			Label:       "Zone cooling load",
			Unit:        "kWh",
			ServiceKind: "cooling",
			ZoneName:    "ZONE B",
			SourceIDs:   []string{"load-b"},
			Total:       3,
		},
	}, nil, &PurposeRunPlan{AllocationPolicy: PurposeAllocationPolicyByZoneLoadShare})

	if result.AllocationPolicy != PurposeAllocationPolicyByZoneLoadShare {
		t.Fatalf("allocation policy = %q", result.AllocationPolicy)
	}
	edgeA := energyExplanationEdgeByIDs(result.Edges, "energy.end_use.cooling.electricity", "load.cooling.zone_a")
	if edgeA == nil || edgeA.Relation != "allocation" || edgeA.Basis != "allocated" || edgeA.RuleID != energyRelationshipRuleAllocatedZoneLoad || edgeA.Value != 1 {
		t.Fatalf("allocated zone A edge = %#v; all edges = %#v", edgeA, result.Edges)
	}
	edgeB := energyExplanationEdgeByIDs(result.Edges, "energy.end_use.cooling.electricity", "load.cooling.zone_b")
	if edgeB == nil || edgeB.Relation != "allocation" || edgeB.Value != 3 {
		t.Fatalf("allocated zone B edge = %#v; all edges = %#v", edgeB, result.Edges)
	}
	if energyExplanationHasEdge(result.Edges, "delivered_load", "measured_variable", "energy.end_use.cooling.electricity", "load.cooling.zone_a") {
		t.Fatalf("allocated view should not also emit measured delivered-load edges: %#v", result.Edges)
	}
}

func TestApplyEnergyExplanationServicePathLoadShareAllocation(t *testing.T) {
	explanation := EnergyExplanationResult{
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes: []EnergyExplanationNode{
			{
				ID:          "energy.end_use.cooling.electricity",
				Level:       "energy",
				Kind:        "energy.cooling",
				Label:       "Cooling energy",
				Value:       12,
				Unit:        "kWh",
				Carrier:     "electricity",
				EndUse:      "cooling",
				ServiceKind: "cooling",
				SourceIDs:   []string{"energy-source"},
			},
			{
				ID:             "load.cooling.office",
				Level:          "load",
				Kind:           "load.zone_cooling",
				Label:          "Office cooling",
				Value:          4,
				Unit:           "kWh",
				ServiceKind:    "cooling",
				ZoneName:       "OFFICE",
				SourceIDs:      []string{"load-office"},
				RelatedPathIDs: []string{"path.office.cooling"},
			},
			{
				ID:             "load.cooling.lab",
				Level:          "load",
				Kind:           "load.zone_cooling",
				Label:          "Lab cooling",
				Value:          8,
				Unit:           "kWh",
				ServiceKind:    "cooling",
				ZoneName:       "LAB",
				SourceIDs:      []string{"load-lab"},
				RelatedPathIDs: []string{"path.lab.cooling"},
			},
		},
		Edges: []EnergyExplanationEdge{
			{
				ID:          "edge.office",
				FromID:      "energy.end_use.cooling.electricity",
				ToID:        "load.cooling.office",
				Value:       4,
				Unit:        "kWh",
				Relation:    "delivered_load",
				Basis:       "measured_variable",
				RuleID:      energyRelationshipRuleMeasuredLoad,
				ServiceKind: "cooling",
				SourceIDs:   []string{"load-office"},
			},
			{
				ID:          "edge.lab",
				FromID:      "energy.end_use.cooling.electricity",
				ToID:        "load.cooling.lab",
				Value:       8,
				Unit:        "kWh",
				Relation:    "delivered_load",
				Basis:       "measured_variable",
				RuleID:      energyRelationshipRuleMeasuredLoad,
				ServiceKind: "cooling",
				SourceIDs:   []string{"load-lab"},
			},
		},
	}
	result := applyEnergyExplanationServicePathLoadShareAllocation(explanation)
	office := energyExplanationEdgeByIDs(result.Edges, "energy.end_use.cooling.electricity", "load.cooling.office")
	if office == nil || office.Relation != "allocation" || office.Basis != "allocated" || office.RuleID != energyRelationshipRuleAllocatedServicePathLoad || office.Value != 4 || len(office.RelatedPathIDs) != 1 {
		t.Fatalf("office service-path allocation edge = %#v; all edges = %#v", office, result.Edges)
	}
	lab := energyExplanationEdgeByIDs(result.Edges, "energy.end_use.cooling.electricity", "load.cooling.lab")
	if lab == nil || lab.Relation != "allocation" || lab.Value != 8 || !strings.Contains(lab.Formula, "path.lab.cooling") {
		t.Fatalf("lab service-path allocation edge = %#v; all edges = %#v", lab, result.Edges)
	}
	if energyExplanationHasEdge(result.Edges, "delivered_load", "measured_variable", "energy.end_use.cooling.electricity", "load.cooling.office") {
		t.Fatalf("service-path allocated view should not retain measured delivered-load edge: %#v", result.Edges)
	}
}

func TestBuildEnergyExplanationKeepsProductionOutOfConsumptionResidual(t *testing.T) {
	result := buildEnergyExplanationResult([]energyExplanationSeries{
		{
			Level:     "energy",
			Kind:      "energy.electricity.total",
			Label:     "Electricity total",
			Unit:      "kWh",
			Carrier:   "electricity",
			EndUse:    "total",
			SourceIDs: []string{"facility"},
			Total:     10,
		},
		{
			Level:     "energy",
			Kind:      "energy.cooling",
			Label:     "Cooling energy",
			Unit:      "kWh",
			Carrier:   "electricity",
			EndUse:    "cooling",
			SourceIDs: []string{"cooling"},
			Total:     6,
		},
		{
			Level:     "energy",
			Kind:      "energy.generators",
			Label:     "Generators / onsite production",
			Unit:      "kWh",
			Carrier:   "electricity",
			EndUse:    "generators",
			SourceIDs: []string{"pv"},
			Total:     2,
		},
	}, nil, &PurposeRunPlan{})

	production := energyExplanationNodeByID(result.Nodes, "energy.end_use.generators.electricity")
	if production == nil || production.Value != 2 {
		t.Fatalf("production node = %#v", production)
	}
	if energyExplanationHasEdge(result.Edges, "meter_enduse", "measured_meter", "energy.carrier.electricity", "energy.end_use.generators.electricity") {
		t.Fatalf("production should not be treated as facility consumption edge: %#v", result.Edges)
	}
	if edge := energyExplanationEdgeByIDs(result.Edges, "energy.carrier.electricity", "energy.end_use.generators.electricity"); edge == nil || edge.Relation != "onsite_production" || edge.RuleID != energyRelationshipRuleOnsiteProduction || edge.Value != 2 {
		t.Fatalf("production support edge = %#v; all edges = %#v", edge, result.Edges)
	}
	reconciliation := energyExplanationReconciliationByID(result.Reconciliation, "reconcile.energy.electricity.annual")
	if reconciliation == nil || reconciliation.ExpectedValue != 10 || reconciliation.ExplainedValue != 6 || reconciliation.ResidualValue != 4 || result.Completeness.MappedPercent != 60 {
		t.Fatalf("production-aware reconciliation = %#v completeness=%#v", reconciliation, result.Completeness)
	}
	if !stringSliceContains(reconciliation.SourceIDs, "facility") || !stringSliceContains(reconciliation.SourceIDs, "cooling") || stringSliceContains(reconciliation.SourceIDs, "pv") {
		t.Fatalf("production-aware reconciliation sources = %#v", reconciliation.SourceIDs)
	}
	summary := buildEnergyExplanationSummary(result)
	if item := energyExplanationSummaryItemByID(summary.EnergyByEndUse, "generators.electricity"); item == nil || item.Value != 2 {
		t.Fatalf("production summary item = %#v", summary.EnergyByEndUse)
	}
}

func TestEnergyHeatAliasCatalogHandlesObjectScopedFanHeat(t *testing.T) {
	def, ok := energyHeatAliasDefinitionForName("Fan Air Heat Gain Rate")
	if !ok || def.Kind != "heat.fan_to_air" || def.HeatCategory != "hvac_system" || !def.ObjectScoped {
		t.Fatalf("fan heat alias = %#v ok=%t", def, ok)
	}
	series := energyExplanationSeriesForBuilder(&energyExplanationSeriesBuilder{
		dictionary: energyExplanationDictionary{
			row: sqlOutputDictionaryRow{
				keyValue: "Supply Fan",
				name:     "Fan Air Heat Gain Rate",
			},
			heat: &def,
		},
		unit:  "kWh",
		total: 0.4,
	}, "sql-rdd-99")
	if series.Level != "heat" || series.Kind != "heat.fan_to_air" || series.ZoneName != "" || series.Total != 0.4 {
		t.Fatalf("fan heat series = %#v", series)
	}
}

func TestPurposeResultBundleUsesSQLEnergyDashboard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)

	result := &SimulationRunResult{
		Status: "succeeded",
		PurposeRunPlan: &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
			{ObjectType: "Output:Meter", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "Electricity:Facility"},
			{ObjectType: "Output:Meter", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "NaturalGas:Facility"},
			{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Lights Electricity Energy"},
		}},
		Files: []SimulationFileInfo{{
			Name: "eplusout.sql",
			Path: path,
			Kind: "sqlite",
		}},
	}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy},
	})

	if len(bundle.Energy.FacilityMonthly) != 1 || bundle.Energy.FacilityMonthly[0].Source != "eplusout.sql" {
		t.Fatalf("bundle energy = %#v", bundle.Energy)
	}
	if bundle.EnergyExplanation.Schema != energyExplanationSchema || len(bundle.EnergyExplanation.Nodes) == 0 {
		t.Fatalf("bundle energy explanation = %#v", bundle.EnergyExplanation)
	}
	if bundle.EnergyExplanationSummary.Schema != energyExplanationSummarySchema || bundle.EnergyExplanationSummary.AllocationPolicy != PurposeAllocationPolicyDirectOnly || len(bundle.EnergyExplanationSummary.EnergyByCarrier) == 0 {
		t.Fatalf("bundle energy explanation summary = %#v", bundle.EnergyExplanationSummary)
	}
	if availability := energyExplanationSourceAvailabilityByName(bundle.EnergyExplanation.Completeness.SourceAvailability, "NaturalGas:Facility"); availability == nil || availability.Status != "missing" || availability.Level != "energy" {
		t.Fatalf("natural gas source availability = %#v", bundle.EnergyExplanation.Completeness.SourceAvailability)
	}
	if !stringSliceContains(bundle.EnergyExplanation.Completeness.MissingCategories, "energy: NaturalGas:Facility") {
		t.Fatalf("missing categories = %#v", bundle.EnergyExplanation.Completeness.MissingCategories)
	}
	if len(bundle.Completeness) != 3 ||
		!purposeCompletenessFound(bundle.Completeness, "Electricity:Facility") ||
		!purposeCompletenessFound(bundle.Completeness, "Zone Lights Electricity Energy") ||
		purposeCompletenessFound(bundle.Completeness, "NaturalGas:Facility") {
		t.Fatalf("bundle completeness = %#v", bundle.Completeness)
	}
}

func TestEnergyExplanationCompletenessMarksUnrequestedLightDetailsNotApplicable(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes:          []SimulationPurposeID{SimulationPurposeBasicEnergy},
		BasicEnergyDetail: PurposeBasicEnergyDetailLight,
	})

	completeness := buildEnergyExplanationCompleteness(nil, nil, &plan, 0)
	if completeness.DeliveredLoad.Status != "not_applicable" || !strings.Contains(completeness.DeliveredLoad.Message, "not requested") {
		t.Fatalf("light-tier delivered-load completeness = %#v", completeness.DeliveredLoad)
	}
	if completeness.HeatDrivers.Status != "not_applicable" || !strings.Contains(completeness.HeatDrivers.Message, "not requested") {
		t.Fatalf("light-tier heat-driver completeness = %#v", completeness.HeatDrivers)
	}
	if availability := energyExplanationSourceAvailabilityByLevelStatus(completeness.SourceAvailability, "load", "not_applicable"); availability == nil || availability.Name != "not requested by current output plan" {
		t.Fatalf("light-tier load source availability = %#v", completeness.SourceAvailability)
	}
	if availability := energyExplanationSourceAvailabilityByLevelStatus(completeness.SourceAvailability, "heat", "not_applicable"); availability == nil || availability.Name != "not requested by current output plan" {
		t.Fatalf("light-tier heat source availability = %#v", completeness.SourceAvailability)
	}
	for _, category := range completeness.MissingCategories {
		if strings.HasPrefix(category, "load:") || strings.HasPrefix(category, "heat:") {
			t.Fatalf("light tier should not report unrequested detail source shortage: %#v", completeness.MissingCategories)
		}
	}
}

func TestEnergyExplanationCompletenessTreatsUnrequestedEnergyAsNotApplicable(t *testing.T) {
	doc := parsePurposePlanFixture(t, `
Version, 24.1;

Zone,
  Office;
`)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy},
	})
	if output := findPurposeOutput(plan, "Output:Meter", "Electricity:Facility", ""); output != nil {
		t.Fatalf("minimal model should not request facility meters: %+v", output)
	}

	completeness := buildEnergyExplanationCompleteness(nil, nil, &plan, 0)
	if completeness.EnergyUse.Status != "not_applicable" || !strings.Contains(completeness.EnergyUse.Message, "not requested") {
		t.Fatalf("unrequested energy completeness = %#v", completeness.EnergyUse)
	}
	if availability := energyExplanationSourceAvailabilityByLevelStatus(completeness.SourceAvailability, "energy", "not_applicable"); availability == nil || availability.Name != "not requested by current output plan" {
		t.Fatalf("unrequested energy source availability = %#v", completeness.SourceAvailability)
	}
	for _, category := range completeness.MissingCategories {
		if strings.HasPrefix(category, "energy:") {
			t.Fatalf("unrequested energy should not report source shortage: %#v", completeness.MissingCategories)
		}
	}
}

func TestEnergyExplanationSourceAvailabilityMatchesLoadAndSignedHeatAliases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, 'ZONE ONE', 'Zone Ideal Loads Zone Sensible Cooling Energy', 'J'),
		(24, 'ZONE ONE', 'Zone Infiltration Sensible Heat Gain Rate', 'W')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 1, 23, 900000.0),
		(8, 2, 23, 900000.0),
		(9, 1, 24, 100.0),
		(10, 2, 24, 100.0)`); err != nil {
		t.Fatal(err)
	}

	loadObjectIndex := 31
	heatGainObjectIndex := 32
	heatLossObjectIndex := 33
	plan := &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "*", VariableName: "Zone Air System Sensible Cooling Energy", ObjectIndex: &loadObjectIndex},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "*", VariableName: "Zone Infiltration Sensible Heat Gain Energy", ObjectIndex: &heatGainObjectIndex},
		{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "*", VariableName: "Zone Infiltration Sensible Heat Loss Energy", ObjectIndex: &heatLossObjectIndex},
	}}
	result, err := parseSimulationEnergyExplanationSQL(path, plan)
	if err != nil {
		t.Fatal(err)
	}

	load := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, "Zone Air System Sensible Cooling Energy")
	if load == nil || load.Status != "found" || !stringSliceContains(load.SourceIDs, "sql-rdd-23") {
		t.Fatalf("load alias availability = %#v", result.Completeness.SourceAvailability)
	}
	heatGain := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, "Zone Infiltration Sensible Heat Gain Energy")
	if heatGain == nil || heatGain.Status != "found" || !stringSliceContains(heatGain.SourceIDs, "sql-rdd-24") {
		t.Fatalf("heat gain alias availability = %#v", result.Completeness.SourceAvailability)
	}
	heatLoss := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, "Zone Infiltration Sensible Heat Loss Energy")
	if heatLoss == nil || heatLoss.Status != "missing" || len(heatLoss.SourceIDs) != 0 {
		t.Fatalf("heat loss availability should not match opposite sign aliases: %#v", result.Completeness.SourceAvailability)
	}
	if source := energyExplanationSourceByID(result.Sources, "sql-rdd-23"); source == nil || source.ObjectIndex == nil || *source.ObjectIndex != loadObjectIndex {
		t.Fatalf("load alias source object index = %#v", source)
	}
	if source := energyExplanationSourceByID(result.Sources, "sql-rdd-24"); source == nil || source.ObjectIndex == nil || *source.ObjectIndex != heatGainObjectIndex || *source.ObjectIndex == heatLossObjectIndex {
		t.Fatalf("signed heat alias source object index = %#v", source)
	}
	for _, category := range result.Completeness.MissingCategories {
		if strings.HasPrefix(category, "load:") || category == "heat: Zone Infiltration Sensible Heat Gain Energy" {
			t.Fatalf("alias-matched sources should not be reported missing: %#v", result.Completeness.MissingCategories)
		}
	}
}

func TestPurposeResultBundleAppliesEnergyExplanationPeriodScope(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, path)

	result := &SimulationRunResult{
		Status:         "succeeded",
		PurposeRunPlan: &PurposeRunPlan{},
		Files: []SimulationFileInfo{{
			Name: "eplusout.sql",
			Path: path,
			Kind: "sqlite",
		}},
	}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy},
		Scope: SimulationPurposeScope{
			PeriodMode:  "custom",
			PeriodStart: "01-01",
			PeriodEnd:   "01-31",
		},
	})

	period := energyExplanationPeriodByID(bundle.EnergyExplanation.Periods, "selected_range")
	if period == nil || period.Label != "01-01 to 01-31" || period.Kind != "selected_range" {
		t.Fatalf("selected period = %#v periods=%#v", period, bundle.EnergyExplanation.Periods)
	}
	facility := energyExplanationNodeByID(period.Nodes, "energy.carrier.electricity")
	if facility == nil || facility.Value != 1 || facility.Unit != "kWh" {
		t.Fatalf("selected facility node = %#v", facility)
	}
	cooling := energyExplanationNodeByID(period.Nodes, "energy.end_use.cooling.electricity")
	if cooling == nil || cooling.Value != 0.5 {
		t.Fatalf("selected cooling node = %#v", cooling)
	}
	reconciliation := energyExplanationReconciliationByID(period.Reconciliation, "reconcile.energy.electricity.selected_range")
	if reconciliation == nil || reconciliation.ExpectedValue != 1 || reconciliation.ExplainedValue != 0.5 || reconciliation.ResidualValue != 0.5 {
		t.Fatalf("selected reconciliation = %#v", period.Reconciliation)
	}
	if len(bundle.EnergyExplanation.Periods) != 4 || bundle.EnergyExplanation.Periods[0].ID != "annual" || bundle.EnergyExplanation.Periods[1].ID != "selected_range" {
		t.Fatalf("period order = %#v", bundle.EnergyExplanation.Periods)
	}
}

func TestParseSimulationEnergyExplanationSQLBuildsDailyPeriods(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergyDailySQL(t, path)

	result, err := parseSimulationEnergyExplanationSQL(path, &PurposeRunPlan{})
	if err != nil {
		t.Fatal(err)
	}
	dayOne := energyExplanationPeriodByID(result.Periods, "D1")
	dayTwo := energyExplanationPeriodByID(result.Periods, "D2")
	if dayOne == nil || dayOne.Kind != "daily" || dayOne.Label != "Day 1" || dayTwo == nil || dayTwo.Kind != "daily" {
		t.Fatalf("daily periods = %#v", result.Periods)
	}
	monthly := energyExplanationPeriodByID(result.Periods, "M1")
	if monthly == nil || monthly.Kind != "monthly" {
		t.Fatalf("monthly period missing from daily source = %#v", result.Periods)
	}
	facilityDayOne := energyExplanationNodeByID(dayOne.Nodes, "energy.carrier.electricity")
	if facilityDayOne == nil || facilityDayOne.Value != 1 {
		t.Fatalf("day 1 facility node = %#v", facilityDayOne)
	}
	coolingDayTwo := energyExplanationNodeByID(dayTwo.Nodes, "energy.end_use.cooling.electricity")
	if coolingDayTwo == nil || coolingDayTwo.Value != 0.5 {
		t.Fatalf("day 2 cooling node = %#v", coolingDayTwo)
	}
	reconciliation := energyExplanationReconciliationByID(dayTwo.Reconciliation, "reconcile.energy.electricity.D2")
	if reconciliation == nil || reconciliation.ExpectedValue != 2 || reconciliation.ExplainedValue != 0.5 || reconciliation.ResidualValue != 1.5 {
		t.Fatalf("day 2 reconciliation = %#v", dayTwo.Reconciliation)
	}
	if len(result.Periods) != 4 || result.Periods[0].ID != "annual" || result.Periods[1].ID != "M1" || result.Periods[2].ID != "D1" || result.Periods[3].ID != "D2" {
		t.Fatalf("daily period order = %#v", result.Periods)
	}
}

func TestParseSimulationEnergyExplanationSQLBuildsHourlyPeriods(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergyHourlySQL(t, path)

	result, err := parseSimulationEnergyExplanationSQL(path, &PurposeRunPlan{})
	if err != nil {
		t.Fatal(err)
	}
	hourOne := energyExplanationPeriodByID(result.Periods, "H1")
	hourTwo := energyExplanationPeriodByID(result.Periods, "H2")
	if hourOne == nil || hourOne.Kind != "hourly" || hourOne.Label != "Hour 1" || hourTwo == nil || hourTwo.Kind != "hourly" {
		t.Fatalf("hourly periods = %#v", result.Periods)
	}
	dayOne := energyExplanationPeriodByID(result.Periods, "D1")
	if dayOne == nil || dayOne.Kind != "daily" {
		t.Fatalf("daily period missing from hourly source = %#v", result.Periods)
	}
	facilityHourOne := energyExplanationNodeByID(hourOne.Nodes, "energy.carrier.electricity")
	if facilityHourOne == nil || facilityHourOne.Value != 1 {
		t.Fatalf("hour 1 facility node = %#v", facilityHourOne)
	}
	facilityHourTwo := energyExplanationNodeByID(hourTwo.Nodes, "energy.carrier.electricity")
	if facilityHourTwo == nil || facilityHourTwo.Value != 2 {
		t.Fatalf("hour 2 facility node = %#v", facilityHourTwo)
	}
	reconciliation := energyExplanationReconciliationByID(hourTwo.Reconciliation, "reconcile.energy.electricity.H2")
	if reconciliation == nil || reconciliation.ExpectedValue != 2 || reconciliation.ExplainedValue != 0.5 || reconciliation.ResidualValue != 1.5 {
		t.Fatalf("hour 2 reconciliation = %#v", hourTwo.Reconciliation)
	}
	if len(result.Periods) != 5 || result.Periods[0].ID != "annual" || result.Periods[1].ID != "M1" || result.Periods[2].ID != "D1" || result.Periods[3].ID != "H1" || result.Periods[4].ID != "H2" {
		t.Fatalf("hourly period order = %#v", result.Periods)
	}
}

func TestParseSimulationEnergyExplanationSQLUsesTabularAnnualFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestEnergyTabularSQL(t, path)

	result, err := parseSimulationEnergyExplanationSQL(path, &PurposeRunPlan{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Schema != energyExplanationSchema || len(result.Periods) != 1 || result.Periods[0].ID != "annual" {
		t.Fatalf("tabular explanation identity/periods = %q %#v", result.Schema, result.Periods)
	}
	facility := energyExplanationNodeByID(result.Nodes, "energy.carrier.electricity")
	if facility == nil || facility.Value != 3 || facility.Unit != "kWh" || facility.MeterHierarchyLevel != "facility_total" {
		t.Fatalf("tabular facility node = %#v", facility)
	}
	cooling := energyExplanationNodeByID(result.Nodes, "energy.end_use.cooling.electricity")
	if cooling == nil || cooling.Value != 1 || cooling.MeterHierarchyLevel != "broad_end_use" {
		t.Fatalf("tabular cooling node = %#v", cooling)
	}
	lighting := energyExplanationNodeByID(result.Nodes, "energy.end_use.interior_lighting.electricity")
	if lighting == nil || lighting.Value != 0.5 {
		t.Fatalf("tabular lighting node = %#v", lighting)
	}
	reconciliation := energyExplanationReconciliationByID(result.Reconciliation, "reconcile.energy.electricity.annual")
	if reconciliation == nil || reconciliation.ExpectedValue != 3 || reconciliation.ExplainedValue != 1.5 || reconciliation.ResidualValue != 1.5 {
		t.Fatalf("tabular reconciliation = %#v", result.Reconciliation)
	}
	source := energyExplanationSourceByName(result.Sources, "Cooling:Electricity")
	if source == nil || source.SourceType != "sql_tabular" || source.AggregationMethod != "tabular_annual_value" || source.TableName != "End Uses" || source.RowName != "Cooling" || source.ColumnName != "Electricity [GJ]" {
		t.Fatalf("tabular source = %#v", source)
	}
	if availability := energyExplanationSourceAvailabilityByName(result.Completeness.SourceAvailability, "Electricity:Facility"); availability == nil || availability.Status != "found" {
		t.Fatalf("tabular source availability = %#v", result.Completeness.SourceAvailability)
	}
}

func TestPurposeResultBundleLinksEnergyExplanationToHVACServicePaths(t *testing.T) {
	dir := t.TempDir()
	sqlPath := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, sqlPath)
	db, err := sql.Open("sqlite", sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, 'Office', 'Zone Air System Sensible Cooling Energy', 'J')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 1, 23, 900000.0),
		(8, 2, 23, 900000.0)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(dir, "model.idf")
	if err := os.WriteFile(inputPath, []byte(`
Version, 24.1;

Zone,
  Office;

ZoneHVAC:EquipmentConnections,
  Office,
  Office Equipment,
  Office Supply Inlet,
  ,
  Office Zone Air Node,
  ;

ZoneHVAC:EquipmentList,
  Office Equipment,
  SequentialLoad,
  ZoneHVAC:IdealLoadsAirSystem,
  Office Ideal Loads,
  1,
  1,
  ,
  ;

ZoneHVAC:IdealLoadsAirSystem,
  Office Ideal Loads,
  Always On,
  Office Supply Inlet,
  ,
  ,
  50,
  13,
  0.015,
  0.009,
  NoLimit,
  Autosize,
  Autosize,
  NoLimit,
  Autosize,
  Autosize,
  ,
  ,
  None,
  0.7;
`), 0o644); err != nil {
		t.Fatal(err)
	}

	result := &SimulationRunResult{
		Status:    "succeeded",
		InputPath: inputPath,
		PurposeRunPlan: &PurposeRunPlan{OutputObjects: []PurposeOutputObject{
			{ObjectType: "Output:Meter", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "Electricity:Cooling"},
			{ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, VariableName: "Zone Air System Sensible Cooling Energy"},
		}},
		Files: []SimulationFileInfo{{
			Name: "eplusout.sql",
			Path: sqlPath,
			Kind: "sqlite",
		}},
	}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy},
	})

	load := energyExplanationNodeByID(bundle.EnergyExplanation.Nodes, "load.cooling.office")
	if load == nil || len(load.RelatedPathIDs) == 0 {
		t.Fatalf("load node related paths = %#v", load)
	}
	edge := energyExplanationEdgeByIDs(bundle.EnergyExplanation.Edges, "energy.end_use.cooling.electricity", "load.cooling.office")
	if edge == nil || len(edge.RelatedPathIDs) == 0 || !stringSlicesEqual(edge.RelatedPathIDs, load.RelatedPathIDs) {
		t.Fatalf("delivered-load edge related paths = %#v load=%#v", edge, load)
	}
	residual := energyExplanationNodeByID(bundle.EnergyExplanation.Nodes, "residual.heat.cooling")
	if residual == nil || len(residual.RelatedPathIDs) == 0 || !stringSlicesEqual(residual.RelatedPathIDs, load.RelatedPathIDs) {
		t.Fatalf("heat residual related paths = %#v load=%#v", residual, load)
	}
	periodLoad := energyExplanationNodeByID(bundle.EnergyExplanation.Periods[1].Nodes, "load.cooling.office")
	if periodLoad == nil || !stringSlicesEqual(periodLoad.RelatedPathIDs, load.RelatedPathIDs) {
		t.Fatalf("period load related paths = %#v annual=%#v", periodLoad, load)
	}
	periodResidual := energyExplanationNodeByID(bundle.EnergyExplanation.Periods[1].Nodes, "residual.heat.cooling")
	if periodResidual == nil || !stringSlicesEqual(periodResidual.RelatedPathIDs, load.RelatedPathIDs) {
		t.Fatalf("period heat residual related paths = %#v annual=%#v", periodResidual, residual)
	}
}

func energySeriesByName(items []EnergySeries, name string) *EnergySeries {
	for index := range items {
		if items[index].Name == name {
			return &items[index]
		}
	}
	return nil
}

func energyExplanationNodeByID(nodes []EnergyExplanationNode, id string) *EnergyExplanationNode {
	for index := range nodes {
		if nodes[index].ID == id {
			return &nodes[index]
		}
	}
	return nil
}

func energyExplanationPeriodByID(periods []EnergyPeriod, id string) *EnergyPeriod {
	for index := range periods {
		if periods[index].ID == id {
			return &periods[index]
		}
	}
	return nil
}

func energyExplanationHasEdge(edges []EnergyExplanationEdge, relation string, basis string, fromID string, toID string) bool {
	for _, edge := range edges {
		if edge.Relation == relation && edge.Basis == basis && edge.FromID == fromID && edge.ToID == toID {
			return true
		}
	}
	return false
}

func energyExplanationEdgeByIDs(edges []EnergyExplanationEdge, fromID string, toID string) *EnergyExplanationEdge {
	for index := range edges {
		if edges[index].FromID == fromID && edges[index].ToID == toID {
			return &edges[index]
		}
	}
	return nil
}

func energyExplanationHasSource(sources []EnergyDataSource, id string, isMeter bool, name string) bool {
	for _, source := range sources {
		if source.ID == id && source.IsMeter == isMeter && source.Name == name {
			return true
		}
	}
	return false
}

func energyExplanationSourceByID(sources []EnergyDataSource, id string) *EnergyDataSource {
	for index := range sources {
		if sources[index].ID == id {
			return &sources[index]
		}
	}
	return nil
}

func energyExplanationSourceByName(sources []EnergyDataSource, name string) *EnergyDataSource {
	for index := range sources {
		if sources[index].Name == name {
			return &sources[index]
		}
	}
	return nil
}

func energyExplanationReconciliationByID(items []EnergyReconciliation, id string) *EnergyReconciliation {
	for index := range items {
		if items[index].ID == id {
			return &items[index]
		}
	}
	return nil
}

func energyExplanationWarningByCode(items []EnergyWarning, code string) *EnergyWarning {
	for index := range items {
		if items[index].Code == code {
			return &items[index]
		}
	}
	return nil
}

func energyExplanationSourceAvailabilityByName(items []EnergySourceAvailabilityEntry, name string) *EnergySourceAvailabilityEntry {
	for index := range items {
		if items[index].Name == name {
			return &items[index]
		}
	}
	return nil
}

func energyExplanationSourceAvailabilityByLevelStatus(items []EnergySourceAvailabilityEntry, level string, status string) *EnergySourceAvailabilityEntry {
	for index := range items {
		if items[index].Level == level && items[index].Status == status {
			return &items[index]
		}
	}
	return nil
}

func energyExplanationSummaryItemByID(items []EnergyExplanationSummaryItem, id string) *EnergyExplanationSummaryItem {
	for index := range items {
		if items[index].ID == id {
			return &items[index]
		}
	}
	return nil
}

func TestPurposeResultBundleBuildsZoneHeatFlowCompleteness(t *testing.T) {
	result := &SimulationRunResult{
		Status: "succeeded",
		HeatFlow: HeatFlowDataset{
			SourceFile: "eplusout.sql",
			FrameCount: 1,
			Categories: []HeatFlowCategory{{
				ID:           "internalConvective",
				Label:        "Internal convective gains",
				VariableName: "Zone Air Heat Balance Internal Convective Heat Gain Rate",
				Unit:         "W",
			}},
			Zones: []HeatFlowZoneSeries{{
				Name:        "Office",
				Temperature: []float64{21},
				Values:      [][]float64{{120}},
			}},
		},
	}

	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
	})

	wantCount := len(zoneHeatFlowVariableNames()) + 1
	if len(bundle.ZoneHeatFlow.Completeness) != wantCount || len(bundle.Completeness) != wantCount {
		t.Fatalf("zone heat-flow completeness count = %d/%d, want %d", len(bundle.ZoneHeatFlow.Completeness), len(bundle.Completeness), wantCount)
	}
	if !purposeCompletenessFound(bundle.ZoneHeatFlow.Completeness, "Zone Mean Air Temperature") {
		t.Fatalf("temperature completeness missing: %#v", bundle.ZoneHeatFlow.Completeness)
	}
	if !purposeCompletenessFound(bundle.ZoneHeatFlow.Completeness, "Zone Air Heat Balance Internal Convective Heat Gain Rate") {
		t.Fatalf("found category completeness missing: %#v", bundle.ZoneHeatFlow.Completeness)
	}
	if purposeCompletenessFound(bundle.ZoneHeatFlow.Completeness, "Zone Air Heat Balance Surface Convection Rate") {
		t.Fatalf("missing category should not be marked found: %#v", bundle.ZoneHeatFlow.Completeness)
	}
}

func TestParseSimulationIntegritySQLBuildsDiagnosticsAndTabular(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestIntegritySQL(t, path)

	result, err := parseSimulationIntegritySQL(path)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasErrorsTable || len(result.Issues) != 2 {
		t.Fatalf("sql issues = %#v", result)
	}
	if result.Issues[1].Severity != "severe" || result.Issues[1].Count != 1 {
		t.Fatalf("severe issue = %#v", result.Issues[1])
	}
	if !result.HasTabularData || len(result.TabularReports) != 1 {
		t.Fatalf("tabular reports = %#v", result.TabularReports)
	}
	report := result.TabularReports[0]
	if report.ReportName != "AnnualBuildingUtilityPerformanceSummary" || report.TableName != "Site and Source Energy" {
		t.Fatalf("report identity = %#v", report)
	}
	if len(report.Columns) != 2 || report.Rows[0].Values["Total Energy [GJ]"] != "12.5" {
		t.Fatalf("report values = %#v", report)
	}
}

func TestPurposeResultBundleUsesSQLIntegrityResult(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestIntegritySQL(t, path)
	inputPath := filepath.Join(dir, "run.idf")
	if err := os.WriteFile(inputPath, []byte("Version, 9.6;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := &SimulationRunResult{
		Status:    "succeeded",
		InputPath: inputPath,
		Files: []SimulationFileInfo{{
			Name: "eplusout.sql",
			Path: path,
			Kind: "sqlite",
		}},
	}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeIntegrity},
	})

	if len(bundle.Integrity.SQLIssues) != 2 || len(bundle.Integrity.TabularReports) != 1 {
		t.Fatalf("integrity bundle = %#v", bundle.Integrity)
	}
	if !integrityHasStaticDiagnostic(bundle.Integrity.StaticDiagnostics, "missing_required_object") {
		t.Fatalf("static diagnostics = %#v, want missing required object", bundle.Integrity.StaticDiagnostics)
	}
	if len(bundle.Completeness) != 3 || !bundle.Completeness[1].Found || !bundle.Completeness[2].Found {
		t.Fatalf("integrity completeness = %#v", bundle.Completeness)
	}
}

func TestPurposeResultBundleCrossChecksIntegrityTabularNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestIntegrityCrossCheckSQL(t, path)
	inputPath := filepath.Join(dir, "run.idf")
	input := `Version, 9.6;

Zone,
  Office;

Zone,
  Lab;

Material,
  Gypsum,
  Smooth,
  0.012,
  0.16,
  800,
  1090;

Construction,
  Wall Cons,
  Gypsum;

BuildingSurface:Detailed,
  South Wall,
  Wall,
  Wall Cons,
  Office,
  Outdoors,
  ,
  SunExposed,
  WindExposed,
  0.5,
  4,
  0, 0, 3,
  4, 0, 3,
  4, 0, 0,
  0, 0, 0;
`
	if err := os.WriteFile(inputPath, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	result := &SimulationRunResult{
		Status:    "succeeded",
		InputPath: inputPath,
		Files: []SimulationFileInfo{{
			Name: "eplusout.sql",
			Path: path,
			Kind: "sqlite",
		}},
	}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeIntegrity},
	})

	assertIntegrityCrossCheck(t, bundle.Integrity.CrossChecks, "zone", "Office", "exact")
	assertIntegrityCrossCheck(t, bundle.Integrity.CrossChecks, "zone", "Lab", "static_only")
	assertIntegrityCrossCheck(t, bundle.Integrity.CrossChecks, "zone", "SQL Only Zone", "sql_only")
	assertIntegrityCrossCheck(t, bundle.Integrity.CrossChecks, "construction", "Wall Cons", "normalized")
	assertIntegrityCrossCheck(t, bundle.Integrity.CrossChecks, "surface", "South Wall", "alias")
	assertIntegrityCrossCheck(t, bundle.Integrity.CrossChecks, "nominal_load", "Office", "info")
}

func TestPurposeResultBundleBuildsHVACLoopSeries(t *testing.T) {
	result := &SimulationRunResult{
		Status: "succeeded",
		Series: []SimulationSeries{
			{File: "eplusout.sql", Column: "Air Supply Inlet:System Node Temperature [C]", Min: 20, Max: 22, Average: 21, Points: []SimulationPoint{{X: 1, Value: 20}, {X: 2, Value: 22}}},
			{File: "eplusout.sql", Column: "Air Supply Inlet:System Node Setpoint Temperature [C]", Min: 21, Max: 21, Average: 21, Points: []SimulationPoint{{X: 1, Value: 21}, {X: 2, Value: 21}}},
			{File: "eplusout.sql", Column: "Air Supply Inlet:System Node Mass Flow Rate [kg/s]", Min: 0.2, Max: 0.4, Average: 0.3, Points: []SimulationPoint{{X: 1, Value: 0.2}, {X: 2, Value: 0.4}}},
			{File: "eplusout.sql", Column: "Fan Outlet:System Node Temperature [C]", Min: 15, Max: 17, Average: 16, Points: []SimulationPoint{{X: 1, Value: 15}, {X: 2, Value: 17}}},
			{File: "eplusout.sql", Column: "Fan Outlet:System Node Mass Flow Rate [kg/s]", Min: 0, Max: 0, Average: 0, Points: []SimulationPoint{{X: 1, Value: 0}, {X: 2, Value: 0}}},
			{File: "eplusout.sql", Column: "Supply Fan:Fan Electricity Rate [W]", Min: 120, Max: 260, Average: 190, Points: []SimulationPoint{{X: 1, Value: 120}, {X: 2, Value: 260}}},
			{File: "eplusout.sql", Column: "Supply Fan:Fan Electricity Energy [J]", Min: 432000, Max: 936000, Average: 684000, Points: []SimulationPoint{{X: 1, Value: 432000}, {X: 2, Value: 936000}}},
			{File: "eplusout.sql", Column: "Office:Zone Mean Air Temperature [C]", Min: 21, Max: 21, Average: 21, Points: []SimulationPoint{{X: 1, Value: 21}}},
		},
	}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck},
		Scope: SimulationPurposeScope{
			AirLoopNames: []string{"Main Air Loop"},
		},
	})

	if len(bundle.HVACLoops) != 1 || bundle.HVACLoops[0].Name != "Main Air Loop" {
		t.Fatalf("hvac loop result = %#v", bundle.HVACLoops)
	}
	if len(bundle.HVACLoops[0].Series) != 5 {
		t.Fatalf("hvac series = %#v", bundle.HVACLoops[0].Series)
	}
	if len(bundle.HVACLoops[0].NodeSummaries) != 2 {
		t.Fatalf("node summaries = %#v", bundle.HVACLoops[0].NodeSummaries)
	}
	if len(bundle.HVACLoops[0].Components) != 1 || bundle.HVACLoops[0].Components[0].ComponentName != "Supply Fan" {
		t.Fatalf("component summaries = %#v", bundle.HVACLoops[0].Components)
	}
	if len(bundle.HVACLoops[0].Components[0].Metrics) != 2 {
		t.Fatalf("component metrics = %#v", bundle.HVACLoops[0].Components[0].Metrics)
	}
	if len(bundle.HVACLoops[0].DerivedMetrics) == 0 {
		t.Fatalf("derived metrics missing: %#v", bundle.HVACLoops[0])
	}
	if bundle.HVACLoops[0].Status != "setpoint_tracking" {
		t.Fatalf("hvac status = %q (%s)", bundle.HVACLoops[0].Status, bundle.HVACLoops[0].StatusMessage)
	}
	if !hvacLoopDerivedMetricExists(bundle.HVACLoops[0].DerivedMetrics, "Estimated air-side heat transfer", "derived_from_node_state") {
		t.Fatalf("expected derived air-side heat transfer: %#v", bundle.HVACLoops[0].DerivedMetrics)
	}
	if !hvacLoopAlertExists(bundle.HVACLoops[0].Alerts, "no_detected_mass_flow") {
		t.Fatalf("expected zero-flow alert: %#v", bundle.HVACLoops[0].Alerts)
	}
	if len(bundle.Completeness) != 2 || !bundle.Completeness[0].Found || !bundle.Completeness[1].Found || bundle.Completeness[0].Source != "sql" {
		t.Fatalf("hvac completeness = %#v", bundle.Completeness)
	}
	if len(bundle.HVACLoops[0].Completeness) != len(hvacLoopCheckNodeVariables())+1 {
		t.Fatalf("node completeness = %#v", bundle.HVACLoops[0].Completeness)
	}
}

func TestClassifyHVACLoopStatusStates(t *testing.T) {
	tests := []struct {
		name    string
		nodes   []HVACNodeRunSummary
		metrics []HVACLoopDerivedMetric
		alerts  []HVACLoopAlert
		want    string
	}{
		{
			name: "unknown without flow",
			nodes: []HVACNodeRunSummary{
				hvacStatusNode("Supply", 21, false, 0, 0, false),
			},
			want: "unknown",
		},
		{
			name: "off at zero flow",
			nodes: []HVACNodeRunSummary{
				hvacStatusNode("Supply", 21, true, 0, 0, false),
			},
			want: "off",
		},
		{
			name: "flow without load",
			nodes: []HVACNodeRunSummary{
				hvacStatusNode("Supply", 21, true, 0.4, 0, false),
				hvacStatusNode("Return", 21.1, true, 0.3, 0, false),
			},
			want: "flow_no_load",
		},
		{
			name: "heating role from node names",
			nodes: []HVACNodeRunSummary{
				hvacStatusNode("Hot Water Supply", 50, true, 0.8, 0, false),
				hvacStatusNode("HW Return", 45, true, 0.6, 0, false),
			},
			want: "active_heating",
		},
		{
			name: "cooling role from node names",
			nodes: []HVACNodeRunSummary{
				hvacStatusNode("CHW Supply", 7, true, 0.8, 0, false),
				hvacStatusNode("Chilled Water Return", 12, true, 0.7, 0, false),
			},
			want: "active_cooling",
		},
		{
			name: "setpoint tracking",
			nodes: []HVACNodeRunSummary{
				hvacStatusNode("Supply", 20, true, 0.5, 1.2, true),
				hvacStatusNode("Return", 22, true, 0.4, 1.4, true),
			},
			want: "setpoint_tracking",
		},
		{
			name: "setpoint not met",
			nodes: []HVACNodeRunSummary{
				hvacStatusNode("Supply", 20, true, 0.5, 6.1, true),
				hvacStatusNode("Return", 24, true, 0.4, 6.4, true),
			},
			want: "setpoint_not_met",
		},
		{
			name: "suspicious generic role",
			nodes: []HVACNodeRunSummary{
				hvacStatusNode("Node A", 20, true, 0.5, 0, false),
				hvacStatusNode("Node B", 24, true, 0.4, 0, false),
			},
			want: "suspicious",
		},
		{
			name: "derived metric fallback",
			nodes: []HVACNodeRunSummary{
				hvacStatusNode("Supply", 20, true, 0.5, 0, false),
			},
			metrics: []HVACLoopDerivedMetric{{Name: "Estimated air-side heat transfer", Source: "derived_from_node_state"}},
			want:    "setpoint_tracking",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, message := classifyHVACLoopStatus(test.nodes, test.metrics, test.alerts)
			if got != test.want {
				t.Fatalf("classifyHVACLoopStatus() = %q (%s), want %q", got, message, test.want)
			}
		})
	}
}

func TestBuildHVACLoopDerivedAlertsFlowWithoutTemperatureSpread(t *testing.T) {
	nodes := []HVACNodeRunSummary{
		hvacStatusNode("Supply", 21, true, 0.4, 0, false),
		hvacStatusNode("Return", 21.1, true, 0.3, 0, false),
	}

	alerts := buildHVACLoopDerivedAlerts(nodes)
	if len(alerts) != 1 || alerts[0].Code != "flow_without_temperature_spread" || alerts[0].Severity != "warning" {
		t.Fatalf("derived alerts = %#v", alerts)
	}
}

func TestPurposeResultBundleBuildsComfortResult(t *testing.T) {
	result := &SimulationRunResult{
		Status: "succeeded",
		Series: []SimulationSeries{
			{File: "eplusout.sql", Column: "Office:Zone Mean Air Temperature [C]", Min: 18, Max: 28, Average: 23, Points: []SimulationPoint{{X: 1, Label: "01-01 01:00", Value: 18}, {X: 2, Label: "01-01 02:00", Value: 28}}},
			{File: "eplusout.sql", Column: "Office:Zone Air Relative Humidity [%]", Min: 40, Max: 50, Average: 45, Points: []SimulationPoint{{X: 1, Value: 45}}},
			{File: "eplusout.sql", Column: "Office:Zone Thermostat Heating Setpoint Temperature [C]", Min: 20, Max: 20, Average: 20, Points: []SimulationPoint{{X: 1, Value: 20}, {X: 2, Value: 20}}},
			{File: "eplusout.sql", Column: "Office:Zone Thermostat Cooling Setpoint Temperature [C]", Min: 26, Max: 26, Average: 26, Points: []SimulationPoint{{X: 1, Value: 26}, {X: 2, Value: 26}}},
			{File: "eplusout.sql", Column: "Office:Zone Air System Sensible Heating Rate [W]", Min: 0, Max: 120, Average: 60, Points: []SimulationPoint{{X: 1, Value: 60}}},
			{File: "eplusout.sql", Column: "Air Supply Inlet:System Node Temperature [C]", Min: 12, Max: 14, Average: 13, Points: []SimulationPoint{{X: 1, Value: 13}}},
		},
	}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeComfort},
	})

	if len(bundle.Comfort.Zones) != 1 || bundle.Comfort.Zones[0].ZoneName != "Office" {
		t.Fatalf("comfort zones = %#v", bundle.Comfort.Zones)
	}
	if len(bundle.Comfort.Series) != 5 || len(bundle.Comfort.Zones[0].Metrics) != 5 {
		t.Fatalf("comfort series = %#v", bundle.Comfort)
	}
	if len(bundle.Comfort.Issues) != 1 || bundle.Comfort.Issues[0].UnmetSamples != 2 || bundle.Comfort.Issues[0].HeatingSamples != 1 || bundle.Comfort.Issues[0].CoolingSamples != 1 {
		t.Fatalf("comfort issue ranking = %#v", bundle.Comfort.Issues)
	}
	if len(bundle.Completeness) != len(comfortCheckVariables()) ||
		!purposeCompletenessFound(bundle.Completeness, "Zone Air Relative Humidity") ||
		!purposeCompletenessFound(bundle.Completeness, "Zone Air System Sensible Heating Rate") ||
		purposeCompletenessFound(bundle.Completeness, "Zone Air System Sensible Cooling Rate") {
		t.Fatalf("comfort completeness = %#v", bundle.Completeness)
	}
}

func hvacStatusNode(name string, temperature float64, hasFlow bool, massFlow float64, setpointDelta float64, hasSetpoint bool) HVACNodeRunSummary {
	node := HVACNodeRunSummary{
		NodeName:           name,
		HasTemperature:     true,
		TemperatureAverage: temperature,
		TemperatureUnit:    "C",
	}
	if hasFlow {
		node.HasMassFlow = true
		node.MassFlowMax = massFlow
		node.MassFlowUnit = "kg/s"
	}
	if hasSetpoint {
		node.HasSetpoint = true
		node.TemperatureSetpointDelta = setpointDelta
		node.TemperatureSetpointSamples = 1
	}
	return node
}

func TestPurposeResultBundleAppliesComfortPeriodScope(t *testing.T) {
	result := &SimulationRunResult{
		Status: "succeeded",
		Series: []SimulationSeries{
			{File: "eplusout.sql", Column: "Office:Zone Mean Air Temperature [C]", Min: 18, Max: 28, Average: 23, Points: []SimulationPoint{{X: 1, Label: "01-01 01:00", Value: 18}, {X: 2, Label: "02-15 02:00", Value: 28}}},
			{File: "eplusout.sql", Column: "Office:Zone Thermostat Heating Setpoint Temperature [C]", Min: 20, Max: 20, Average: 20, Points: []SimulationPoint{{X: 1, Label: "01-01 01:00", Value: 20}, {X: 2, Label: "02-15 02:00", Value: 20}}},
			{File: "eplusout.sql", Column: "Office:Zone Thermostat Cooling Setpoint Temperature [C]", Min: 26, Max: 26, Average: 26, Points: []SimulationPoint{{X: 1, Label: "01-01 01:00", Value: 26}, {X: 2, Label: "02-15 02:00", Value: 26}}},
		},
	}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeComfort},
		Scope: SimulationPurposeScope{
			PeriodMode:  "custom",
			PeriodStart: "02-01",
			PeriodEnd:   "02-28",
		},
	})

	if bundle.Comfort.PeriodScope != "02-01 to 02-28" {
		t.Fatalf("period scope = %q", bundle.Comfort.PeriodScope)
	}
	metrics := comfortMetricMap(bundle.Comfort.Zones[0].Metrics)
	temperature := metrics[normalizePurposeToken("Zone Mean Air Temperature")]
	if temperature == nil || len(temperature.Points) != 1 || temperature.Points[0].Value != 28 || temperature.Min != 28 || temperature.Max != 28 {
		t.Fatalf("filtered temperature metric = %#v", temperature)
	}
	if len(bundle.Comfort.Issues) != 1 || bundle.Comfort.Issues[0].UnmetSamples != 1 || bundle.Comfort.Issues[0].HeatingSamples != 0 || bundle.Comfort.Issues[0].CoolingSamples != 1 {
		t.Fatalf("period-scoped comfort issues = %#v", bundle.Comfort.Issues)
	}
}

func TestPurposeResultBundleBuildsComfortUnmetSummaries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createTestComfortSQL(t, path)

	result := &SimulationRunResult{
		Status: "succeeded",
		Files: []SimulationFileInfo{{
			Name: "eplusout.sql",
			Path: path,
			Kind: "sqlite",
		}},
	}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeComfort},
	})

	if len(bundle.Comfort.UnmetHours) != 2 {
		t.Fatalf("comfort unmet summaries = %#v", bundle.Comfort.UnmetHours)
	}
	if bundle.Comfort.UnmetHours[0].ZoneName != "Office" || bundle.Comfort.UnmetHours[0].Value != 12.5 || bundle.Comfort.UnmetHours[0].Source != "eplusout.sql" {
		t.Fatalf("top unmet summary = %#v", bundle.Comfort.UnmetHours[0])
	}
}

func hvacLoopAlertExists(alerts []HVACLoopAlert, code string) bool {
	for _, alert := range alerts {
		if alert.Code == code {
			return true
		}
	}
	return false
}

func hvacLoopDerivedMetricExists(metrics []HVACLoopDerivedMetric, name string, source string) bool {
	for _, metric := range metrics {
		if metric.Name == name && metric.Source == source {
			return true
		}
	}
	return false
}

func TestWriteSimulationRunManifest(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "in.idf")
	if err := os.WriteFile(inputPath, []byte("Version, 24.1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := &SimulationRunResult{
		RunID:             "sim-test",
		Status:            "succeeded",
		InputPath:         inputPath,
		Filename:          "in.idf",
		EnergyPlusVersion: "24.2",
		OutputDirectory:   dir,
		StartedAt:         "2026-06-10T00:00:00Z",
		FinishedAt:        "2026-06-10T00:00:01Z",
		ResultSourcePriority: []string{
			"sql",
			"csv",
			"eso",
		},
		ResultSources: []string{"sql"},
		Files: []SimulationFileInfo{{
			Name: "eplusout.sql",
			Path: filepath.Join(dir, "eplusout.sql"),
			Kind: "sqlite",
			Size: 120,
		}},
	}
	plan := &PurposeRunPlan{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, EstimatedWeight: "Light"}
	request := SimulationRunRequest{
		PurposeRequest: &SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}},
		PurposeRunPlan: plan,
		ResultMode:     "sql_first",
	}

	writeSimulationRunManifest(result, request)

	path := filepath.Join(dir, "semantic-idf-run.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest SimulationRunManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.RunID != "sim-test" || manifest.Status != "succeeded" || manifest.InputHash == "" {
		t.Fatalf("manifest core fields = %+v", manifest)
	}
	if manifest.EnergyPlusVersion != "24.2" {
		t.Fatalf("manifest EnergyPlus version = %q, want 24.2", manifest.EnergyPlusVersion)
	}
	if len(manifest.Purposes) != 1 || manifest.Purposes[0] != SimulationPurposeBasicEnergy {
		t.Fatalf("manifest purposes = %#v", manifest.Purposes)
	}
	if manifest.OutputPlan == nil || manifest.OutputPlan.EstimatedWeight != "Light" {
		t.Fatalf("manifest output plan = %#v", manifest.OutputPlan)
	}
	if !stringSlicesEqual(manifest.ResultSourcePriority, []string{"sql", "csv", "eso"}) || !stringSlicesEqual(manifest.ResultSources, []string{"sql"}) {
		t.Fatalf("manifest source metadata = priority %#v sources %#v", manifest.ResultSourcePriority, manifest.ResultSources)
	}
	if len(result.Files) != 2 || !simulationResultHasFileKind(result.Files, "manifest") {
		t.Fatalf("result files = %#v", result.Files)
	}
}

func TestWritePurposeRunArtifacts(t *testing.T) {
	dir := t.TempDir()
	request := SimulationRunRequest{
		PurposeRunPlan:      &PurposeRunPlan{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, EstimatedWeight: "Light"},
		TemporaryOutputDiff: "--- original.idf\n+++ purpose-run-copy.idf\n+Output:SQLite,\n+  SimpleAndTabular;  !- Option Type\n",
	}

	writePurposeRunArtifacts(dir, request)

	if _, err := os.Stat(filepath.Join(dir, "semantic-idf-run-plan.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "temporary_outputs.diff")); err != nil {
		t.Fatal(err)
	}
	files := collectSimulationFiles(dir)
	if !simulationResultHasFileKind(files, "run_plan") || !simulationResultHasFileKind(files, "temporary_output_diff") {
		t.Fatalf("artifact file kinds = %#v", files)
	}
}

func TestSimulationUsesReadVarsESOPolicy(t *testing.T) {
	cases := []struct {
		name string
		req  SimulationRunRequest
		want bool
	}{
		{
			name: "sql first disables readvarseso by default",
			req:  SimulationRunRequest{ResultMode: "sql_first"},
			want: false,
		},
		{
			name: "sql first allows explicit readvarseso fallback",
			req:  SimulationRunRequest{ResultMode: "sql_first", UseReadVarsESO: true},
			want: true,
		},
		{
			name: "csv fallback always uses readvarseso",
			req:  SimulationRunRequest{ResultMode: "csv_fallback"},
			want: true,
		},
		{
			name: "legacy empty mode keeps readvarseso behavior",
			req:  SimulationRunRequest{},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := simulationUsesReadVarsESO(tc.req); got != tc.want {
				t.Fatalf("simulationUsesReadVarsESO() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEnergyPlusVersionForExecutableUsesInstallationMetadata(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "energyplus.exe")

	version := energyPlusVersionForExecutable(executable, []EnergyPlusInstallSetting{{
		ExecutablePath: executable,
		Version:        "25.1",
	}})

	if version != "25.1" {
		t.Fatalf("version = %q, want 25.1", version)
	}
	if empty := energyPlusVersionForExecutable("", nil); empty != "" {
		t.Fatalf("empty executable version = %q, want empty", empty)
	}
}

func TestReadSimulationOutputsEmitsDetailedProgressPhases(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "eplusout.err"), []byte("EnergyPlus Completed Successfully"), 0o644); err != nil {
		t.Fatal(err)
	}
	var phases []string
	result := &SimulationRunResult{OutputDirectory: dir}

	readSimulationOutputsWithProgress(result, "sim-progress", func(progress SimulationProgress) {
		phases = append(phases, progress.Phase)
	}, "")

	if !stringSliceContains(phases, "parse_sql") || !stringSliceContains(phases, "parse_fallback") {
		t.Fatalf("progress phases = %#v", phases)
	}
}

func simulationResultHasFileKind(files []SimulationFileInfo, kind string) bool {
	for _, file := range files {
		if file.Kind == kind {
			return true
		}
	}
	return false
}

func purposeCompletenessFound(items []PurposeCompletenessItem, requiredOutput string) bool {
	for _, item := range items {
		if item.RequiredOutput == requiredOutput {
			return item.Found
		}
	}
	return false
}

func integrityHasStaticDiagnostic(items []idf.Diagnostic, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}

func assertIntegrityCrossCheck(t *testing.T, items []IntegrityCrossCheck, category string, name string, status string) {
	t.Helper()
	for _, item := range items {
		if item.Category == category && item.Name == name && item.Status == status {
			return
		}
	}
	t.Fatalf("missing integrity cross check category=%q name=%q status=%q in %#v", category, name, status, items)
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestReadSimulationOutputsPrefersSQLHeatFlowAndSeries(t *testing.T) {
	dir := t.TempDir()
	createTestEnergyPlusSQL(t, filepath.Join(dir, "eplusout.sql"))
	content := `Program Version,EnergyPlus
2,8,Day of Simulation[],Month[],Day of Month[],DST Indicator[1=yes 0=no],Hour[],StartMinute[],EndMinute[],DayType
10,1,ZONE ONE,Zone Mean Air Temperature [C] !Hourly
11,1,ZONE ONE,Zone Air Heat Balance Internal Convective Heat Gain Rate [W] !Hourly
End of Data Dictionary
2,1, 1, 1, 0, 1, 0.00,60.00,Monday
10,20.0
11,100.0
`
	if err := os.WriteFile(filepath.Join(dir, "eplusout.eso"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "eplusout.err"), []byte("EnergyPlus Completed Successfully"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := &SimulationRunResult{OutputDirectory: dir}
	readSimulationOutputs(result)
	if len(result.HeatFlow.Zones) != 2 {
		t.Fatalf("heat-flow zones = %d, want 2; files = %#v", len(result.HeatFlow.Zones), result.Files)
	}
	if result.HeatFlow.SourceFile != "eplusout.sql" {
		t.Fatalf("source file = %q, want eplusout.sql", result.HeatFlow.SourceFile)
	}
	if len(result.Series) == 0 || result.Series[0].File != "eplusout.sql" {
		t.Fatalf("sql series were not parsed first: %#v", result.Series)
	}
	if !stringSlicesEqual(result.ResultSourcePriority, []string{"sql", "csv", "eso"}) || !stringSlicesEqual(result.ResultSources, []string{"sql"}) {
		t.Fatalf("result source metadata = priority %#v sources %#v", result.ResultSourcePriority, result.ResultSources)
	}
}

func TestReadSimulationOutputsUsesESOHeatFlowFallback(t *testing.T) {
	dir := t.TempDir()
	content := `Program Version,EnergyPlus
2,8,Day of Simulation[],Month[],Day of Month[],DST Indicator[1=yes 0=no],Hour[],StartMinute[],EndMinute[],DayType
10,1,ZONE ONE,Zone Mean Air Temperature [C] !Hourly
11,1,ZONE ONE,Zone Air Heat Balance Internal Convective Heat Gain Rate [W] !Hourly
End of Data Dictionary
2,1, 1, 1, 0, 1, 0.00,60.00,Monday
10,20.0
11,100.0
`
	if err := os.WriteFile(filepath.Join(dir, "eplusout.eso"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "eplusout.err"), []byte("EnergyPlus Completed Successfully"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := &SimulationRunResult{OutputDirectory: dir}
	readSimulationOutputs(result)
	if len(result.HeatFlow.Zones) != 1 {
		t.Fatalf("heat-flow fallback zones = %d, want 1; files = %#v", len(result.HeatFlow.Zones), result.Files)
	}
	if result.HeatFlow.SourceFile != "eplusout.eso" {
		t.Fatalf("source file = %q, want eplusout.eso", result.HeatFlow.SourceFile)
	}
	if !stringSlicesEqual(result.ResultSourcePriority, []string{"sql", "csv", "eso"}) || !stringSlicesEqual(result.ResultSources, []string{"eso"}) {
		t.Fatalf("result source metadata = priority %#v sources %#v", result.ResultSourcePriority, result.ResultSources)
	}
}

func createTestEnergyPlusSQL(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE ReportDataDictionary (
			ReportDataDictionaryIndex INTEGER PRIMARY KEY,
			KeyValue TEXT,
			Name TEXT,
			Units TEXT
		)`,
		`CREATE TABLE "Time" (
			TimeIndex INTEGER PRIMARY KEY,
			Month INTEGER,
			Day INTEGER,
			Hour INTEGER,
			Minute INTEGER
		)`,
		`CREATE TABLE ReportData (
			ReportDataIndex INTEGER PRIMARY KEY,
			TimeIndex INTEGER,
			ReportDataDictionaryIndex INTEGER,
			Value REAL
		)`,
		`INSERT INTO ReportDataDictionary VALUES
			(10, 'ZONE ONE', 'Zone Mean Air Temperature', 'C'),
			(11, 'ZONE ONE', 'Zone Air Heat Balance Internal Convective Heat Gain Rate', 'W'),
			(12, 'ZONE ONE', 'Zone Air Heat Balance Surface Convection Rate', 'W'),
			(13, 'ZONE TWO', 'Zone Air Heat Balance Outdoor Air Transfer Rate', 'W')`,
		`INSERT INTO "Time" VALUES
			(1, 1, 1, 1, 0),
			(2, 1, 1, 2, 0)`,
		`INSERT INTO ReportData VALUES
			(1, 1, 10, 20.0),
			(2, 1, 11, 100.0),
			(3, 1, 12, -30.0),
			(4, 1, 13, -12.0),
			(5, 2, 10, 21.5),
			(6, 2, 11, 150.0),
			(7, 2, 12, -45.0),
			(8, 2, 13, -18.0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("sql fixture statement failed: %v\n%s", err, statement)
		}
	}
}

func createTestEnergySQL(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE ReportDataDictionary (
			ReportDataDictionaryIndex INTEGER PRIMARY KEY,
			KeyValue TEXT,
			Name TEXT,
			Units TEXT
		)`,
		`CREATE TABLE "Time" (
			TimeIndex INTEGER PRIMARY KEY,
			Month INTEGER,
			Day INTEGER,
			Hour INTEGER,
			Minute INTEGER
		)`,
		`CREATE TABLE ReportData (
			ReportDataIndex INTEGER PRIMARY KEY,
			TimeIndex INTEGER,
			ReportDataDictionaryIndex INTEGER,
			Value REAL
		)`,
		`INSERT INTO ReportDataDictionary VALUES
			(20, '', 'Electricity:Facility', 'J'),
			(21, '', 'Electricity:Cooling', 'J'),
			(22, 'ZONE ONE', 'Zone Lights Electricity Energy', 'J')`,
		`INSERT INTO "Time" VALUES
			(1, 1, 31, 24, 0),
			(2, 2, 28, 24, 0)`,
		`INSERT INTO ReportData VALUES
			(1, 1, 20, 3600000.0),
			(2, 2, 20, 7200000.0),
			(3, 1, 21, 1800000.0),
			(4, 2, 21, 1800000.0),
			(5, 1, 22, 900000.0),
			(6, 2, 22, 900000.0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("sql fixture statement failed: %v\n%s", err, statement)
		}
	}
}

func createTestEnergyDailySQL(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE ReportDataDictionary (
			ReportDataDictionaryIndex INTEGER PRIMARY KEY,
			KeyValue TEXT,
			Name TEXT,
			Units TEXT,
			IsMeter INTEGER,
			ReportingFrequency TEXT
		)`,
		`CREATE TABLE "Time" (
			TimeIndex INTEGER PRIMARY KEY,
			Month INTEGER,
			Day INTEGER,
			Hour INTEGER,
			Minute INTEGER
		)`,
		`CREATE TABLE ReportData (
			ReportDataIndex INTEGER PRIMARY KEY,
			TimeIndex INTEGER,
			ReportDataDictionaryIndex INTEGER,
			Value REAL
		)`,
		`INSERT INTO ReportDataDictionary VALUES
			(20, '', 'Electricity:Facility', 'J', 1, 'Daily'),
			(21, '', 'Electricity:Cooling', 'J', 1, 'Daily')`,
		`INSERT INTO "Time" VALUES
			(1, 1, 1, 24, 0),
			(2, 1, 2, 24, 0)`,
		`INSERT INTO ReportData VALUES
			(1, 1, 20, 3600000.0),
			(2, 2, 20, 7200000.0),
			(3, 1, 21, 1800000.0),
			(4, 2, 21, 1800000.0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("sql fixture statement failed: %v\n%s", err, statement)
		}
	}
}

func createTestEnergyHourlySQL(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE ReportDataDictionary (
			ReportDataDictionaryIndex INTEGER PRIMARY KEY,
			KeyValue TEXT,
			Name TEXT,
			Units TEXT,
			IsMeter INTEGER,
			ReportingFrequency TEXT
		)`,
		`CREATE TABLE "Time" (
			TimeIndex INTEGER PRIMARY KEY,
			Month INTEGER,
			Day INTEGER,
			Hour INTEGER,
			Minute INTEGER
		)`,
		`CREATE TABLE ReportData (
			ReportDataIndex INTEGER PRIMARY KEY,
			TimeIndex INTEGER,
			ReportDataDictionaryIndex INTEGER,
			Value REAL
		)`,
		`INSERT INTO ReportDataDictionary VALUES
			(20, '', 'Electricity:Facility', 'J', 1, 'Hourly'),
			(21, '', 'Electricity:Cooling', 'J', 1, 'Hourly')`,
		`INSERT INTO "Time" VALUES
			(1, 1, 1, 1, 0),
			(2, 1, 1, 2, 0)`,
		`INSERT INTO ReportData VALUES
			(1, 1, 20, 3600000.0),
			(2, 2, 20, 7200000.0),
			(3, 1, 21, 1800000.0),
			(4, 2, 21, 1800000.0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("sql fixture statement failed: %v\n%s", err, statement)
		}
	}
}

func createTestEnergyTabularSQL(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE TabularDataWithStrings (
			ReportName TEXT,
			ReportForString TEXT,
			TableName TEXT,
			RowName TEXT,
			ColumnName TEXT,
			Units TEXT,
			RowId INTEGER,
			ColumnId INTEGER,
			Value TEXT
		)`,
		`INSERT INTO TabularDataWithStrings VALUES
			('AnnualBuildingUtilityPerformanceSummary', 'Entire Facility', 'End Uses', 'Total End Uses', 'Electricity', 'GJ', 1, 1, '0.0108'),
			('AnnualBuildingUtilityPerformanceSummary', 'Entire Facility', 'End Uses', 'Cooling', 'Electricity', 'GJ', 2, 1, '0.0036'),
			('AnnualBuildingUtilityPerformanceSummary', 'Entire Facility', 'End Uses', 'Interior Lighting', 'Electricity', 'GJ', 3, 1, '0.0018'),
			('AnnualBuildingUtilityPerformanceSummary', 'Entire Facility', 'End Uses', 'Humidification', 'Electricity', 'GJ', 4, 1, '0.0001')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("sql fixture statement failed: %v\n%s", err, statement)
		}
	}
}

func createTestIntegritySQL(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE Errors (
			ErrorIndex INTEGER PRIMARY KEY,
			ErrorType TEXT,
			ErrorMessage TEXT,
			Count INTEGER
		)`,
		`CREATE TABLE TabularDataWithStrings (
			ReportName TEXT,
			ReportForString TEXT,
			TableName TEXT,
			RowName TEXT,
			ColumnName TEXT,
			Units TEXT,
			RowId INTEGER,
			ColumnId INTEGER,
			Value TEXT
		)`,
		`INSERT INTO Errors VALUES
			(1, 'Warning', 'Calculated design day warning', 2),
			(2, 'Severe', 'Node connection problem', 1)`,
		`INSERT INTO TabularDataWithStrings VALUES
			('AnnualBuildingUtilityPerformanceSummary', 'Entire Facility', 'Site and Source Energy', 'Total Site Energy', 'Total Energy', 'GJ', 1, 1, '12.5'),
			('AnnualBuildingUtilityPerformanceSummary', 'Entire Facility', 'Site and Source Energy', 'Total Site Energy', 'Energy Per Total Building Area', 'MJ/m2', 1, 2, '85.1')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("sql fixture statement failed: %v\n%s", err, statement)
		}
	}
}

func createTestIntegrityCrossCheckSQL(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE TabularDataWithStrings (
			ReportName TEXT,
			ReportForString TEXT,
			TableName TEXT,
			RowName TEXT,
			ColumnName TEXT,
			Units TEXT,
			RowId INTEGER,
			ColumnId INTEGER,
			Value TEXT
		)`,
		`INSERT INTO TabularDataWithStrings VALUES
			('InputVerificationandResultsSummary', 'Entire Facility', 'Zone Summary', 'Office', 'Conditioned', '', 1, 1, 'Yes'),
			('InputVerificationandResultsSummary', 'Entire Facility', 'Zone Summary', 'SQL Only Zone', 'Conditioned', '', 2, 1, 'Yes'),
			('EnvelopeSummary', 'Entire Facility', 'Constructions', 'wall   cons', 'U-Factor', 'W/m2-K', 3, 1, '0.4'),
			('EnvelopeSummary', 'Entire Facility', 'Opaque Exterior', 'South-Wall', 'Area', 'm2', 4, 1, '12.0'),
			('ComponentSizingSummary', 'Entire Facility', 'Zone Sensible Cooling Nominal Loads', 'Office', 'Nominal Total Capacity', 'W', 5, 1, '4200')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("sql fixture statement failed: %v\n%s", err, statement)
		}
	}
}

func createTestComfortSQL(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE TabularDataWithStrings (
			ReportName TEXT,
			ReportForString TEXT,
			TableName TEXT,
			RowName TEXT,
			ColumnName TEXT,
			Units TEXT,
			RowId INTEGER,
			ColumnId INTEGER,
			Value TEXT
		)`,
		`INSERT INTO TabularDataWithStrings VALUES
			('AnnualBuildingUtilityPerformanceSummary', 'Entire Facility', 'Comfort and Setpoint Not Met Summary', 'Office', 'Time Setpoint Not Met During Occupied Heating', 'hr', 1, 1, '12.5'),
			('AnnualBuildingUtilityPerformanceSummary', 'Entire Facility', 'Comfort and Setpoint Not Met Summary', 'Lab', 'Time Setpoint Not Met During Occupied Cooling', 'hr', 2, 1, '3.0'),
			('AnnualBuildingUtilityPerformanceSummary', 'Entire Facility', 'Site and Source Energy', 'Total Site Energy', 'Total Energy', 'GJ', 3, 1, '99.0')`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("sql fixture statement failed: %v\n%s", err, statement)
		}
	}
}

func TestCollectWeatherFoldersGroupsEPWFiles(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "USA")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "Chicago.epw"), []byte("weather"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte("skip"), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings := []string{}
	folders := collectWeatherFolders(nil, []string{dir}, &warnings)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(folders) != 1 {
		t.Fatalf("folder count = %d, want 1", len(folders))
	}
	if folders[0].Label != "USA" || len(folders[0].Files) != 1 {
		t.Fatalf("folder = %+v", folders[0])
	}
}

func TestDefaultSimulationWorkerCountUsesFractionAndMax(t *testing.T) {
	workers := DefaultWorkerCount(SimulationSettings{WorkerFraction: 0.5, MaxWorkers: 1})
	if workers != 1 {
		t.Fatalf("workers = %d, want 1", workers)
	}
}

func TestEnergyPlusInstallationsSortNewestFirst(t *testing.T) {
	installations := []EnergyPlusInstallSetting{
		{Name: "EnergyPlus 23.2", Version: "23.2", AutoDetected: true},
		{Name: "EnergyPlus 9.6", Version: "9.6", AutoDetected: true},
		{Name: "EnergyPlus 25.1", Version: "25.1", AutoDetected: true},
		{Name: "EnergyPlus 24.2", Version: "24.2", AutoDetected: true},
	}

	sortEnergyPlusInstallations(installations)

	got := []string{installations[0].Version, installations[1].Version, installations[2].Version, installations[3].Version}
	want := []string{"25.1", "24.2", "23.2", "9.6"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("versions = %v, want %v", got, want)
		}
	}
}
