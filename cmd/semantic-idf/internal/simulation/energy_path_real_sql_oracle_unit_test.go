package simulation

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func epathOracleUnitSQL(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "independent-oracle.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentName TEXT,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(1,'FULL WEATHER 2017',3),(2,'SUMMER DESIGN DAY',1)`,
		`CREATE TABLE "Time"(TimeIndex INTEGER PRIMARY KEY,EnvironmentPeriodIndex INTEGER,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,"Interval" REAL,WarmupFlag INTEGER,IntervalType INTEGER,SimulationDays INTEGER)`,
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,Name TEXT,KeyValue TEXT,IsMeter INTEGER,ReportingFrequency TEXT,Units TEXT,IndexGroup TEXT)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,ReportDataDictionaryIndex INTEGER,TimeIndex INTEGER,Value REAL)`,
		`INSERT INTO ReportDataDictionary VALUES(1,'Cooling:Electricity','',1,'Monthly','J','Facility'),(2,'Observed Rate','Office',0,'Timestep','W','Zone'),(3,'Missing Observation','Office',0,'Monthly','J','Zone'),(4,'Cooling:Electricity','',1,'Run Period','J','Facility'),(5,'Rate With Unknown Interval','Office',0,'Timestep','W','Zone'),(6,'Yearly energy','',1,'Annual','J','Facility')`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	for month := 1; month <= 12; month++ {
		last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC)
		if _, err := db.Exec(`INSERT INTO "Time" VALUES(?,1,2017,?,?,24,0,?,NULL,3,?)`, month, month, last.Day(), last.Day()*24*60, last.YearDay()); err != nil {
			t.Fatal(err)
		}
		value := float64(month) * 3600000
		if month == 2 {
			value = 0
		}
		if _, err := db.Exec(`INSERT INTO ReportData VALUES(?,1,?,?),(?,3,?,NULL)`, month, month, value, 100+month, month); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		`INSERT INTO "Time" VALUES(1001,1,2017,1,1,1,30,30,0,0,1),(1002,1,2017,1,1,2,0,30,0,0,1),(1003,1,2017,1,1,2,30,NULL,0,0,1),(2001,2,2017,7,21,24,0,1440,NULL,2,1),(2002,1,2017,1,1,24,0,1440,1,0,1),(4001,1,NULL,NULL,NULL,NULL,NULL,525600,NULL,4,365),(5001,1,2017,NULL,NULL,NULL,NULL,NULL,NULL,5,NULL),(6001,1,2017,1,1,3,0,30,NULL,0,1)`,
		`INSERT INTO ReportData VALUES(301,2,1001,1000),(302,2,1002,1000),(303,5,1001,1000),(304,5,1003,1000),(401,4,4001,273600000),(501,1,2001,999999999999),(502,1,2002,999999999999),(601,6,5001,273600000),(701,2,6001,999999999999)`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func epathOracleEditSQL(t *testing.T, path, query string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(query); err != nil {
		t.Fatal(err)
	}
}

func TestEnergyPathRealSQLOracleIndependentWeatherUnitsAndUnknown(t *testing.T) {
	path := epathOracleUnitSQL(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	if observed.Acceptance || observed.Status != "metadata_only" || len(observed.CheckedGroups) != 0 || len(observed.Weather.Months) != 12 {
		t.Fatalf("raw capture claimed acceptance or lost actual weather coverage: %#v", observed)
	}
	byID := map[int]epathRealSQLSource{}
	for _, source := range observed.Sources {
		byID[source.DictionaryIndex] = source
	}
	energy := byID[1]
	if energy.Rows != 12 || energy.EnergyKWh == nil || *energy.EnergyKWh != 76 || len(energy.Months) != 12 {
		t.Fatalf("weather-only monthly energy should be76, not design/warmup/other-frequency sums: %#v", energy)
	}
	if energy.Months[1].EnergyKWh == nil || *energy.Months[1].EnergyKWh != 0 {
		t.Fatal("reported zero became unknown")
	}
	if byID[2].EnergyKWh == nil || *byID[2].EnergyKWh != 1 {
		t.Fatalf("1000W over two actual30min intervals must be1kWh: %#v", byID[2])
	}
	if byID[3].RawSum != nil || byID[3].EnergyKWh != nil || byID[3].MissingRows != 12 {
		t.Fatalf("NULL observations became0: %#v", byID[3])
	}
	if len(byID[4].Months) != 0 || byID[4].EnergyKWh == nil || *byID[4].EnergyKWh != 76 || epathOracleSourceValue(byID[4], "M12") != nil {
		t.Fatalf("annual source was assigned to December: %#v", byID[4])
	}
	if len(byID[6].Months) != 0 || byID[6].EnergyKWh == nil || *byID[6].EnergyKWh != 76 {
		t.Fatal("actual Year scalar with NULL Month/Day was rejected or promoted to a month")
	}
	if byID[5].EnergyKWh != nil || byID[5].Months[0].EnergyKWh != nil {
		t.Fatal("partly missing rate interval produced false known energy")
	}
	encoded, _ := json.Marshal(observed)
	if !strings.Contains(string(encoded), `"energyKWh":0`) || !strings.Contains(string(encoded), `"energyKWh":null`) {
		t.Fatal("wire output lost known0/unknown distinction")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("read-only oracle mutated SQL")
	}
}

func TestEnergyPathRealSQLOracleRejectsNonAnnualAndDuplicateEvidence(t *testing.T) {
	for _, test := range []struct{ name, query, message string }{
		{"warmup cannot fill missing month", `UPDATE "Time" SET WarmupFlag=1 WHERE Month=6 AND EnvironmentPeriodIndex=1`, "all 12 months"},
		{"design day cannot fill missing month", `UPDATE "Time" SET EnvironmentPeriodIndex=2 WHERE Month=8 AND EnvironmentPeriodIndex=1`, "all 12 months"},
		{"multiple weather periods", `INSERT INTO EnvironmentPeriods VALUES(3,'SECOND RUN',3); INSERT INTO "Time" VALUES(3001,3,2017,1,1,24,0,1440,0,2,1)`, "multiple weather"},
		{"duplicate observation", `INSERT INTO ReportData VALUES(999,1,1,3600000)`, "duplicate ReportData"},
		{"wrong calendar year", `UPDATE "Time" SET Year=2018 WHERE EnvironmentPeriodIndex=1 AND WarmupFlag=0`, "calendar year"},
		{"partial January despite twelve months", `UPDATE "Time" SET Day=2 WHERE EnvironmentPeriodIndex=1 AND Month=1 AND Day=1; UPDATE "Time" SET SimulationDays=30 WHERE TimeIndex=1`, "full monthly calendar"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := epathOracleUnitSQL(t)
			epathOracleEditSQL(t, path, test.query)
			if _, err := epathReadRealSQLOracle(path); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %s rejection, got %v", test.message, err)
			}
		})
	}
}

func TestEnergyPathRealSQLOracleExactIdentityAndTwelveMonthSource(t *testing.T) {
	path := epathOracleUnitSQL(t)
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	binding := epathRealOracleSourceRecipe{ID: "cooling", Names: []string{"Cooling:Electricity"}, Keys: []string{""}, IsMeter: true, Frequency: "Monthly", SourceUnit: "J"}
	value, err := epathResolveOracleSource(observed.Sources, binding, "annual")
	if err != nil || value == nil || *value != 76 {
		t.Fatalf("exactMonthly selection doubled annual counterpart: %v %v", value, err)
	}
	epathOracleEditSQL(t, path, `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=1 AND TimeIndex=6`)
	observed, err = epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	value, err = epathResolveOracleSource(observed.Sources, binding, "annual")
	if err != nil || value != nil {
		t.Fatalf("11-month series acquired annual value: %v %v", value, err)
	}
	epathOracleEditSQL(t, path, `INSERT INTO ReportDataDictionary VALUES(7,'Cooling:Electricity','',1,'Monthly','J','Facility'); INSERT INTO ReportData VALUES(999,7,1,3600000)`)
	observed, err = epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := epathResolveOracleSource(observed.Sources, binding, "M1"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("duplicate exact identity was silently selected: %v", err)
	}
}

func TestEnergyPathRealSQLOracleMonthlyOnlyAnnualCalendar(t *testing.T) {
	for _, test := range []struct{ name, query, message string }{
		{"complete monthly-only year", "", ""},
		{"full nominal interval cannot hide partial January", `UPDATE "Time" SET SimulationDays=17 WHERE TimeIndex=1`, "full monthly calendar"},
		{"short monthly interval", `UPDATE "Time" SET "Interval"=24480 WHERE TimeIndex=1`, "full monthly calendar"},
		{"missing middle month", `DELETE FROM "Time" WHERE TimeIndex=6`, "all 12 months"},
		{"inconsistent middle cumulative day", `UPDATE "Time" SET SimulationDays=180 WHERE TimeIndex=6`, "full monthly calendar"},
		{"unknown cumulative day", `UPDATE "Time" SET SimulationDays=NULL WHERE TimeIndex=1`, "full monthly calendar"},
		{"duplicated monthly timestamp", `INSERT INTO "Time" SELECT 9001,EnvironmentPeriodIndex,Year,Month,Day,Hour,Minute,"Interval",WarmupFlag,IntervalType,SimulationDays FROM "Time" WHERE TimeIndex=1`, "full monthly calendar"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := epathOracleUnitSQL(t)
			epathOracleEditSQL(t, path, `DELETE FROM "Time" WHERE IntervalType NOT IN(3,4,5)`)
			if test.query != "" {
				epathOracleEditSQL(t, path, test.query)
			}
			observed, err := epathReadRealSQLOracle(path)
			if test.message != "" {
				if err == nil || !strings.Contains(err.Error(), test.message) {
					t.Fatalf("want %s rejection, got %v", test.message, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if observed.Weather.FirstDay != 31 || observed.Weather.LastDay != 31 || observed.Weather.TimeRows != 12 || observed.Weather.CoverageBasis != "monthly_intervals_and_cumulative_simulation_days" {
				t.Fatalf("monthly endpoint observations lost or annual coverage fabricated: %#v", observed.Weather)
			}
		})
	}
	// Valid endpoint records must not conceal invalid monthly coverage.
	path := epathOracleUnitSQL(t)
	epathOracleEditSQL(t, path, `UPDATE "Time" SET SimulationDays=17 WHERE TimeIndex=1`)
	if _, err := epathReadRealSQLOracle(path); err == nil {
		t.Fatal("January1 timestep allowed a partial annual calendar to pass")
	}
}

func TestEnergyPathRealSQLOracleTerminalMonthlyRunPeriod(t *testing.T) {
	path := epathOracleUnitSQL(t)
	epathOracleEditSQL(t, path, `UPDATE ReportData SET TimeIndex=12 WHERE ReportDataDictionaryIndex=4;
INSERT INTO ReportDataDictionary VALUES(8,'Annual rate without annual duration','Office',0,'Run Period','W','Zone');
INSERT INTO ReportData VALUES(801,8,12,1000)`)
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range observed.Sources {
		if source.DictionaryIndex == 4 && (source.Rows != 1 || source.EnergyKWh == nil || *source.EnergyKWh != 76 || len(source.Months) != 0 || epathOracleSourceValue(source, "M12") != nil) {
			t.Fatalf("terminal RunPeriod energy was lost or assigned to December: %#v", source)
		}
		if source.DictionaryIndex == 8 && (source.RawSum == nil || *source.RawSum != 1000 || source.EnergyKWh != nil || len(source.Months) != 0) {
			t.Fatalf("annual rate integrated final-month interval as annual duration: %#v", source)
		}
	}
	for _, test := range []struct{ name, query, message string }{
		{"nonterminal monthly timestamp", `UPDATE ReportData SET TimeIndex=11 WHERE ReportDataDictionaryIndex=4`, "contradicts"},
		{"two annual observations", `INSERT INTO "Time" VALUES(4002,1,NULL,NULL,NULL,NULL,NULL,525600,NULL,4,365); INSERT INTO ReportData VALUES(802,4,4002,273600000)`, "single annual observation"},
		{"wrong annual year", `UPDATE "Time" SET Year=2018 WHERE TimeIndex=5001`, "wrong calendar year"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := epathOracleUnitSQL(t)
			epathOracleEditSQL(t, path, test.query)
			if _, err := epathReadRealSQLOracle(path); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("want %s rejection, got %v", test.message, err)
			}
		})
	}
}

func TestEnergyPathRealSQLOracleDailyCalendarNeedsEveryObservedDay(t *testing.T) {
	path := epathOracleUnitSQL(t)
	epathOracleEditSQL(t, path, `DELETE FROM "Time"`)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for day := 1; day <= 365; day++ {
		date := time.Date(2017, 1, day, 0, 0, 0, 0, time.UTC)
		if _, err := tx.Exec(`INSERT INTO "Time" VALUES(?,1,2017,?,?,24,0,1440,NULL,2,?)`, day, int(date.Month()), date.Day(), day); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	weather, err := epathReadOracleWeather(db)
	if err != nil {
		t.Fatal(err)
	}
	if weather.CoverageBasis != "365_observed_calendar_and_simulation_days" || weather.TimeRows != 365 {
		t.Fatalf("daily calendar proof missing: %#v", weather)
	}
	if _, err := db.Exec(`DELETE FROM "Time" WHERE SimulationDays=32`); err != nil {
		t.Fatal(err)
	}
	if _, err := epathReadOracleWeather(db); err == nil || !strings.Contains(err.Error(), "day 32") {
		t.Fatalf("twelve month labels and endpoints concealed missing observed day: %v", err)
	}
}

func TestEnergyPathRealOracleComparisonCannotBlessUnknownOrMissingGroups(t *testing.T) {
	zero := epathOracleNumber(0)
	if epathCompareOracleNumber(zero, nil, 0, 0) == nil || epathCompareOracleNumber(nil, zero, 0, 0) == nil {
		t.Fatal("unknown and0 compare equal")
	}
	if err := epathCompareOracleNumber(zero, zero, 0, 0); err != nil {
		t.Fatal(err)
	}
	for _, inputs := range [][]*float64{{zero, zero}, {nil, epathOracleNumber(1)}} {
		if value, err := epathOracleArithmetic("ratio", inputs); err != nil || value != nil {
			t.Fatalf("unavailable ratio became0: %v %v", value, err)
		}
	}
	if value, err := epathOracleArithmetic("ratio", []*float64{zero, epathOracleNumber(2)}); err != nil || value == nil || *value != 0 {
		t.Fatal("known zero ratio lost")
	}
	if _, err := epathOracleArithmetic("constant", []*float64{epathOracleNumber(123)}); err == nil {
		t.Fatal("constants can masquerade as SQL evidence")
	}
	if err := epathValidateOracleMetricGroups([]epathRealOracleMetric{{Key: "only", Group: "carriers", Value: zero}}); err == nil {
		t.Fatal("partial groups accepted as all eight")
	}
	observed := epathRealOracleEvidence{Sources: []epathRealSQLSource{{Name: "Cooling:Electricity", IsMeter: true, ReportingFrequency: "Annual", SourceUnit: "J", EnergyKWh: epathOracleNumber(12)}}}
	recipe := epathRealOracleRecipe{Sources: []epathRealOracleSourceRecipe{{ID: "meter", Names: []string{"Cooling:Electricity"}, Keys: []string{""}, IsMeter: true, Frequency: "Annual", SourceUnit: "J"}}, Metrics: []epathRealOracleMetricRecipe{{Key: "cooling", Group: "endUses", Scope: "building", Period: "annual", Unit: "kWh", Operation: "source", Inputs: []string{"meter"}, Target: epathRealOracleTarget{Collection: "nodes", Field: "value", Level: "end_use", Category: "cooling", Unit: "kWh", ScaleDomain: "site"}}}}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}, Nodes: []EnergyExplanationNode{{ID: "cooling", Level: "end_use", EndUse: "cooling", Value: 13, Unit: "kWh", ScaleDomain: "site"}}}}
	if err := epathEvaluateRealOracle(&observed, recipe, bundle); err == nil || !strings.Contains(err.Error(), "independent SQL vs candidate") {
		t.Fatalf("candidate-derived number was blessed instead of checked: %v", err)
	}
	bundle.EnergyExplanation.Nodes[0].Value = 12
	observed.Metrics = nil
	observed.CheckedGroups = nil
	if err := epathEvaluateRealOracle(&observed, recipe, bundle); err != nil {
		t.Fatal(err)
	}
	if len(observed.CheckedGroups) != 1 || observed.CheckedGroups[0] != "endUses" {
		t.Fatal("checked-group evidence missing")
	}
	recipe.Metrics[0].Target.Collection = ""
	observed.Metrics = nil
	observed.CheckedGroups = nil
	if err := epathEvaluateRealOracle(&observed, recipe, bundle); err != nil || len(observed.CheckedGroups) != 0 {
		t.Fatal("SQL-only intermediate claimed a candidate comparison")
	}
}

func TestEnergyPathRealOracleStrictJSON(t *testing.T) {
	for _, test := range []struct {
		name, text string
		valid      bool
	}{
		{"EOF whitespace", `{"id":"reviewed"} `, true},
		{"malformed trailing bytes", `{"id":"reviewed"} invalid`, false},
		{"second JSON object", `{"id":"reviewed"}{}`, false},
		{"unknown recipe property", `{"id":"reviewed","autoBless":true}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "recipe.json")
			if err := os.WriteFile(path, []byte(test.text), 0600); err != nil {
				t.Fatal(err)
			}
			var value epathRealOracleSourceRecipe
			err := epathDecodeOracleFile(path, &value)
			if (err == nil) != test.valid {
				t.Fatalf("strict JSON valid=%v error=%v", test.valid, err)
			}
		})
	}
}
