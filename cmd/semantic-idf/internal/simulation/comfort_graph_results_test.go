package simulation

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestComfortGraphsResolvePeopleAndBuildingScopes(t *testing.T) {
	doc := parsePurposePlanFixture(t, `Zone, Office; Zone, Lab;
Space, Desk A, Office; Space, Desk B, Office;
ZoneList, Workspaces, Office, Lab;
People, Office Staff, Office;
People, Lab Staff, Lab;
People, Shared Staff, Workspaces;`)
	series := []SimulationSeries{}
	for _, item := range [][2]string{
		{"OFFICE", "Zone Mean Air Temperature"},
		{"Desk A Office Staff", "Zone Thermal Comfort Fanger Model PMV"},
		{"Desk B Office Staff", "Zone Thermal Comfort Fanger Model PMV"},
		{"Lab Staff", "Zone Thermal Comfort Fanger Model PPD"},
		{"Lab Shared Staff", "Zone Thermal Comfort Fanger Model PMV"},
		{"Unknown Staff", "Zone Thermal Comfort Fanger Model PMV"},
		{"Environment", "Facility Heating Setpoint Not Met Time"},
		{"OFFICE", "Zone Air System Sensible Cooling Rate"},
	} {
		series = append(series, SimulationSeries{Column: item[0] + ":" + item[1] + " [](Hourly)", Points: []SimulationPoint{{X: 1, Value: 1}}})
	}
	result := buildComfortResult(series, SimulationPurposeScope{}, &doc)
	if len(result.Zones) != 2 || len(result.BuildingMetrics) != 1 || len(result.Zones[0].Metrics) != 2 || len(result.Zones[1].Metrics) != 3 {
		t.Fatalf("People/facility outputs were misclassified: %+v", result)
	}
	if result.Zones[1].ZoneName != "Office" || result.Zones[1].Metrics[1].KeyValue != "Desk A Office Staff" {
		t.Fatalf("executed-zone ownership or separate People keys lost: %+v", result.Zones)
	}
	scoped := buildComfortResult(series, SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"Lab"}}, &doc)
	if len(scoped.Zones) != 1 || scoped.Zones[0].ZoneName != "Lab" || len(scoped.BuildingMetrics) != 1 {
		t.Fatalf("selected zone scope is not authoritative: %+v", scoped)
	}
	withoutModel := buildComfortResult(series, SimulationPurposeScope{})
	if len(withoutModel.Zones) != 1 || withoutModel.Zones[0].ZoneName != "OFFICE" {
		t.Fatalf("unknown People must not become fake zones: %+v", withoutModel.Zones)
	}
	for _, test := range []struct {
		name, file, frequency, columnFrequency, want string
		included                                     bool
	}{
		{"native Hourly overrides column", "eplusout.sql", "Hourly", "Timestep", "Hourly", true},
		{"native Hourly spelling", "eplusout.sql", " hOuRlY ", "Hourly", " hOuRlY ", true},
		{"native zone cadence remains excluded", "eplusout.sql", "Zone Timestep", "Hourly", "", false},
		{"native system cadence remains excluded", "eplusout.sql", "HVAC System Timestep", "Hourly", "", false},
		{"unknown SQL cadence", "eplusout.sql", "", "Hourly", "", true},
		{"CSV column cadence", "EPLUSOUT.CSV", "", "Hourly", "Hourly", true},
		{"explicit CSV cadence remains excluded", "eplusout.csv", "Daily", "Hourly", "", false},
		{"unknown source cadence", "", "", "Hourly", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := SimulationSeries{File: test.file, ReportingFrequency: test.frequency, Column: "Desk A Office Staff:Zone Thermal Comfort Fanger Model PMV [](" + test.columnFrequency + ")", Min: -.5, Max: 0, Average: -.25, Points: []SimulationPoint{{X: 3, Label: "01-01 01:00", Value: 0}, {X: 8, Label: "01-02 01:00", Value: -.5}}}
			before, _ := json.Marshal(item)
			result := buildComfortResult([]SimulationSeries{item}, SimulationPurposeScope{}, &doc)
			if !test.included {
				if len(result.Zones) != 0 || len(result.Series) != 0 {
					t.Fatal("metadata broadened existing Hourly metric selection", result)
				}
				return
			}
			if len(result.Zones) != 1 || len(result.Zones[0].Metrics) != 1 {
				t.Fatal("cadence metadata changed zone ownership or metric membership", result)
			}
			metric := result.Zones[0].Metrics[0]
			if metric.ReportingFrequency != test.want || !reflect.DeepEqual(metric.Points, item.Points) || metric.Min != item.Min || metric.Max != item.Max || metric.Average != item.Average || metric.Source != item.File {
				t.Fatal("cadence metadata changed observations or source summaries", metric)
			}
			wire, _ := json.Marshal(metric)
			var fields map[string]any
			if err := json.Unmarshal(wire, &fields); err != nil {
				t.Fatal(err)
			}
			if _, present := fields["reportingFrequency"]; present != (test.want != "") {
				t.Fatal("unknown cadence acquired a reporting-frequency claim", string(wire))
			}
			scoped := buildComfortResult([]SimulationSeries{item}, SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"Office"}, PeriodMode: "custom", PeriodStart: "01-02", PeriodEnd: "01-02"}, &doc)
			if len(scoped.Zones) != 1 || len(scoped.Zones[0].Metrics) != 1 {
				t.Fatal("metadata changed selected period or zone", scoped)
			}
			selected := scoped.Zones[0].Metrics[0]
			if selected.ReportingFrequency != test.want || !reflect.DeepEqual(selected.Points, item.Points[1:]) || selected.Min != -.5 || selected.Max != -.5 || selected.Average != -.5 {
				t.Fatal("cadence was lost or scoping/summary values changed", selected)
			}
			after, _ := json.Marshal(item)
			if string(before) != string(after) {
				t.Fatal("comfort metadata mutated its source series")
			}
		})
	}
}

func TestComfortPlanRequestsPeopleKeysForSelectedZone(t *testing.T) {
	doc := parsePurposePlanFixture(t, `Zone, Office; Zone, Lab;
People, Office Staff, Office; People, Lab Staff, Lab;
ZoneList, Workspaces, Office, Lab; People, Shared Staff, Workspaces;`)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeComfort}, Scope: SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"Office"}}})
	for _, key := range []string{"office staff", "office shared staff"} {
		if findPurposeOutput(plan, "Output:Variable", key, "Zone Thermal Comfort Fanger Model PMV") == nil {
			t.Fatalf("missing selected People key %q: %+v", key, plan.OutputObjects)
		}
	}
	for _, key := range []string{"Office", "lab staff", "lab shared staff", "*"} {
		if findPurposeOutput(plan, "Output:Variable", key, "Zone Thermal Comfort Fanger Model PMV") != nil {
			t.Fatalf("incorrect PMV scope key %q", key)
		}
	}
	if findPurposeOutput(plan, "Output:Variable", "*", "Facility Heating Setpoint Not Met Time") == nil {
		t.Fatal("reported building comfort hours missing")
	}
}

func TestComfortSQLGraphsKeepEveryWeatherHour(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY, KeyValue TEXT, Name TEXT, Units TEXT, ReportingFrequency TEXT, IsMeter INTEGER)`,
		`CREATE TABLE "Time" (TimeIndex INTEGER PRIMARY KEY, Month INTEGER, Day INTEGER, Hour INTEGER, Minute INTEGER, EnvironmentPeriodIndex INTEGER, IntervalType INTEGER, WarmupFlag INTEGER)`,
		`CREATE TABLE EnvironmentPeriods (EnvironmentPeriodIndex INTEGER, EnvironmentType INTEGER)`,
		`CREATE TABLE ReportData (TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`,
		`INSERT INTO EnvironmentPeriods VALUES (1,1),(2,3)`,
		`INSERT INTO ReportDataDictionary VALUES (1,'Office','Zone Mean Air Temperature','C','Hourly',0),(2,'Office','Zone Thermostat Cooling Setpoint Temperature','C','Hourly',0),(3,'Other','Unrelated','W','Hourly',0)`,
		`INSERT INTO "Time" VALUES (1,1,1,1,0,1,1,0),(2,1,1,1,0,2,1,1)`,
		`INSERT INTO ReportData VALUES (1,1,999),(2,1,999)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	const frames = 1301
	for frame := 1; frame <= frames; frame++ {
		if _, err := tx.Exec(`INSERT INTO "Time" VALUES (?,1,?,?,0,2,1,0)`, frame+2, (frame-1)/24+1, (frame-1)%24+1); err != nil {
			t.Fatal(err)
		}
		for column := 1; column <= 3; column++ {
			if column == 2 && frame == 250 {
				continue
			}
			if _, err := tx.Exec(`INSERT INTO ReportData VALUES (?,?,?)`, frame+2, column, frame); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	series, err := parseSimulationSQLSeriesForPlan(path, PurposeRunPlan{Purposes: []SimulationPurposeID{SimulationPurposeComfort}})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 3 || len(series[0].Points) != frames || len(series[1].Points) != frames-1 || len(series[2].Points) != maxCSVSeriesPoints {
		t.Fatalf("comfort hourly retention / generic preview bounds: %v", hvacTestPointCounts(series))
	}
	if series[0].Points[0].Value != 1 || series[1].Points[249].X != series[0].Points[250].X {
		t.Fatal("design/warmup observations or missing-hour alignment corrupted comfort plots")
	}
	result := buildComfortResult(series, SimulationPurposeScope{})
	if len(result.Zones) != 1 || len(result.Zones[0].Metrics) != 2 {
		t.Fatal("native SQL comfort metrics changed membership")
	}
	for _, metric := range result.Zones[0].Metrics {
		if metric.ReportingFrequency != "Hourly" {
			t.Fatal("native SQL cadence was dropped from comfort metric", metric.ReportingFrequency)
		}
	}
}

func TestComfortCSVGraphsKeepEveryHour(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.csv")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"Date/Time", "Office:Zone Mean Air Temperature [C](Hourly)", "Other:Unrelated [W](Hourly)"}); err != nil {
		t.Fatal(err)
	}
	for frame := 1; frame <= 1301; frame++ {
		if err := writer.Write([]string{fmt.Sprintf("01/%02d %02d:00:00", (frame-1)/24+1, (frame-1)%24+1), strconv.Itoa(frame), "1"}); err != nil {
			t.Fatal(err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	_, series, err := parseSimulationCSVForPlan(path, PurposeRunPlan{Purposes: []SimulationPurposeID{SimulationPurposeComfort}})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 2 || len(series[0].Points) != 1301 || len(series[1].Points) != maxCSVSeriesPoints {
		t.Fatalf("comfort CSV hourly series lost: %v", hvacTestPointCounts(series))
	}
	result := buildComfortResult(series, SimulationPurposeScope{})
	if len(result.Zones) != 1 || len(result.Zones[0].Metrics) != 1 || result.Zones[0].Metrics[0].ReportingFrequency != "Hourly" || len(result.Zones[0].Metrics[0].Points) != 1301 {
		t.Fatal("CSV cadence fallback changed complete comfort trace", result.Zones)
	}
}

func TestComfortUnmetSummariesFindLateBuildingReports(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	createTestComfortSQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 300; index++ {
		if _, err := db.Exec(`INSERT INTO TabularDataWithStrings VALUES ('AAA','Entire Facility','Energy','Energy','Power','W',?,1,'999')`, index); err != nil {
			t.Fatal(err)
		}
	}
	for _, statement := range []string{
		`INSERT INTO TabularDataWithStrings VALUES ('AnnualBuildingUtilityPerformanceSummary','Entire Facility','Comfort and Setpoint Not Met Summary','Time Setpoint Not Met During Occupied Cooling','Facility','hr',3,1,'18.5')`,
		`INSERT INTO TabularDataWithStrings VALUES ('AnnualBuildingUtilityPerformanceSummary','Entire Facility','Setpoint Not Met Criteria','Tolerance for Zone Heating Setpoint Not Met Time','Tolerance','C',4,1,'0.2')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	items, err := parseComfortUnmetSQL(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].Scope != "building" || items[0].Value != 18.5 || items[0].Metric != "Time Setpoint Not Met During Occupied Cooling" {
		t.Fatalf("building comfort report missing or criteria included: %+v", items)
	}
	doc := parsePurposePlanFixture(t, `Zone, Office; Zone, Lab;`)
	scoped := scopeComfortUnmetSummaries(items, &doc, SimulationPurposeScope{ZoneMode: "selected", ZoneNames: []string{"Office"}})
	if len(scoped) != 2 || scoped[1].ZoneName != "Office" || scoped[1].Scope != "zone" {
		t.Fatalf("unmet summary ownership incorrect: %+v", scoped)
	}
}
