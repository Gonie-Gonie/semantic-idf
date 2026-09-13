package simulation

import (
	"database/sql"
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func createEnergySourceHourlySQL(t *testing.T, hours int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY, KeyValue TEXT, Name TEXT, Units TEXT, IsMeter INTEGER, ReportingFrequency TEXT, IndexGroup TEXT)`,
		`CREATE TABLE Time (TimeIndex INTEGER PRIMARY KEY, Year INTEGER, Month INTEGER, Day INTEGER, Hour INTEGER, Minute INTEGER, "Interval" INTEGER, IntervalType INTEGER, EnvironmentPeriodIndex INTEGER, WarmupFlag INTEGER)`,
		`CREATE TABLE EnvironmentPeriods (EnvironmentPeriodIndex INTEGER PRIMARY KEY, EnvironmentType INTEGER)`,
		`CREATE TABLE ReportData (ReportDataIndex INTEGER PRIMARY KEY, TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)`,
		`INSERT INTO EnvironmentPeriods VALUES (1,1), (2,3)`,
		`INSERT INTO ReportDataDictionary VALUES (300,'','Electricity:Facility','J',1,'Hourly','Meter'), (301,'Office','Zone Air System Sensible Cooling Rate','W',0,'Hourly','Zone'), (302,'','Electricity:Facility','J',1,'Monthly','Meter')`,
		`INSERT INTO Time VALUES (1,2017,1,1,1,0,60,1,1,0), (2,2017,1,1,1,0,60,1,2,1), (3,2017,1,31,24,0,44640,3,2,NULL)`,
		`INSERT INTO ReportData VALUES (1,1,300,999999999), (2,2,300,999999999), (3,3,302,3600000)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	transaction, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < hours; index++ {
		date := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(index/24) * 24 * time.Hour)
		timeID := index + 10
		if _, err := transaction.Exec(`INSERT INTO Time VALUES (?,2017,?,?,?,0,60,1,2,0)`, timeID, int(date.Month()), date.Day(), index%24+1); err != nil {
			t.Fatal(err)
		}
		value := 3600000.0
		if index == 1 {
			value = 0
		}
		if _, err := transaction.Exec(`INSERT INTO ReportData (TimeIndex,ReportDataDictionaryIndex,Value) VALUES (?,300,?),(?,301,500)`, timeID, value, timeID); err != nil {
			t.Fatal(err)
		}
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	return path
}

func parseEnergyHourlySourceFixture(t *testing.T, path string) EnergyExplanationV1 {
	t.Helper()
	doc := parsePurposePlanFixture(t, `Version,24.2; Zone,Office,0,0,0,0,1,2,3,300,100; ZoneList,Repeated,Office; ZoneGroup,Offices,Repeated,3;`)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	result, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestEnergySourceHourlyPreservesCompleteReportedResolutionAndZeroes(t *testing.T) {
	const hours = 1305
	path := createEnergySourceHourlySQL(t, hours)
	result := UpgradeEnergyExplanationV1(parseEnergyHourlySourceFixture(t, path))
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded EnergyExplanationResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"hourlyLabels":`) != 1 || strings.Contains(string(data), `"points":`) {
		t.Fatal("hourly source serialization must share one label axis and store only numeric value arrays")
	}
	for _, explanation := range []EnergyExplanationResult{result, decoded} {
		if len(explanation.HourlyLabels) != hours || explanation.HourlyLabels[0] != "01-01 01:00" || explanation.HourlyLabels[23] != "01-01 24:00" {
			t.Fatal("shared hourly axis lost complete end-of-hour labels during upgrade or serialization")
		}
		found := 0
		for _, source := range explanation.Sources {
			switch source.ID {
			case "sql-rdd-300", "sql-rdd-301":
				found++
				trace := source.HourlyEnergy
				if trace == nil || trace.Unit != "kWh" || trace.Basis != "reported_source" || len(trace.Values) != len(explanation.HourlyLabels) {
					t.Fatalf("source %s lost full reported hourly trace: %#v", source.ID, trace)
				}
				if source.ID == "sql-rdd-300" && (trace.Values[0] != 1 || trace.Values[1] != 0 || trace.Values[hours-1] != 1) {
					t.Fatalf("energy conversion, observed zero, or design/warmup exclusion failed: %#v", trace.Values[:2])
				}
				if source.ID == "sql-rdd-301" && (math.Abs(trace.Values[0]-.5) > 1e-12 || source.EffectiveMultiplier != 6) {
					t.Fatalf("reported500 W for60 minutes must stay0.5 kWh with separate Zone2/List3 multiplier metadata; got value %v / multiplier %v", trace.Values[0], source.EffectiveMultiplier)
				}
			case "sql-rdd-302":
				if source.HourlyEnergy != nil {
					t.Fatal("Monthly observations were distributed into synthetic hourly energy")
				}
			}
		}
		if found != 2 {
			t.Fatalf("only %d complete hourly sources survived canonical parsing", found)
		}
	}
}

func TestEnergySourceHourlyRejectsMissingInvalidAndAmbiguousObservations(t *testing.T) {
	for _, test := range []struct {
		name, mutation string
		allUnavailable bool
	}{
		{"missing point", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=300 AND TimeIndex=11`, false},
		{"null point", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=300 AND TimeIndex=11`, false},
		{"duplicate point", `INSERT INTO ReportData (TimeIndex,ReportDataDictionaryIndex,Value) VALUES (11,300,3600000)`, false},
		{"nonhourly interval", `UPDATE Time SET "Interval"=30 WHERE TimeIndex=11`, true},
		{"duplicate calendar slot", `UPDATE Time SET Hour=1 WHERE TimeIndex=11`, true},
		{"ambiguous weather run", `INSERT INTO EnvironmentPeriods VALUES (3,3)`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := createEnergySourceHourlySQL(t, 3)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(test.mutation); err != nil {
				t.Fatal(err)
			}
			db.Close()
			result := parseEnergyHourlySourceFixture(t, path)
			if test.allUnavailable && len(result.HourlyLabels) != 0 {
				t.Fatalf("%s exported an hourly axis without any valid source", test.name)
			}
			for _, source := range result.Sources {
				if (source.ID == "sql-rdd-300" || test.allUnavailable) && source.HourlyEnergy != nil {
					t.Fatalf("%s exported an unproven hourly curve for %s", test.name, source.ID)
				}
			}
		})
	}
}

func TestEnergyPathHourlyRequestsRetainEveryMonthlyInputAndExistingOutputs(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyPathScopeFixtureIDF+"\nOutput:Meter,Electricity:Facility,Daily;\n")
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, FrequencyPolicy: PurposeFrequencyPolicyPreserve})
	monthly, hourly := map[string]PurposeOutputObject{}, map[string]PurposeOutputObject{}
	for _, output := range plan.OutputObjects {
		if !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) || !purposeObjectIsSeries(output.ObjectType) {
			continue
		}
		key := strings.Join([]string{output.ObjectType, output.KeyValue, output.VariableName}, "|")
		if output.ReportingFrequency == "Monthly" {
			monthly[key] = output
		} else if output.ReportingFrequency == "Hourly" {
			hourly[key] = output
		} else {
			t.Fatalf("unexpected Energy Path frequency %s", output.ReportingFrequency)
		}
	}
	if len(monthly) == 0 || len(monthly) != len(hourly) {
		t.Fatalf("monthly/hourly source rosters differ: %d / %d", len(monthly), len(hourly))
	}
	for key, month := range monthly {
		hour, exists := hourly[key]
		if !exists || month.ScopeZoneName != hour.ScopeZoneName || hour.State == PurposeOutputStateConflict || hour.ObjectIndex != nil {
			t.Fatalf("hourly companion missing or rewrites existing output for %s: %#v", key, hour)
		}
	}
}

func TestEnergySourceHourlyDirectHVACIsSourceOnlyAndKeepsTypedOwner(t *testing.T) {
	path := createEnergySourceHourlySQL(t, 3)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO ReportDataDictionary VALUES (303,'SPACE1-1 PTAC CCoil','Cooling Coil Electricity Energy','J',0,'Hourly','HVAC')`,
		`INSERT INTO ReportData (TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,303,7200000 FROM Time WHERE EnvironmentPeriodIndex=2 AND IntervalType=1 AND WarmupFlag=0`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	doc, _, _ := directHVACSQLDocument(t)
	plan := directHVACSQLPlan(doc)
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, source := range parsed.Sources {
		if source.ID == "sql-rdd-303" {
			found = true
			if source.ZoneName != "SPACE1-1" || source.HourlyEnergy == nil || len(parsed.HourlyLabels) != 3 || len(source.HourlyEnergy.Values) != 3 || source.HourlyEnergy.Values[0] != 2 {
				t.Fatalf("native equipment source lost its observed curve or typed owner: %#v", source)
			}
		}
	}
	if !found {
		t.Fatal("native equipment Hourly source was omitted")
	}
	for _, series := range parsed.Series {
		if energyExplanationSourcesIntersect(series.SourceIDs, map[string]bool{"sql-rdd-303": true}) {
			t.Fatal("source-only Hourly equipment entered canonical Monthly allocation series")
		}
	}
}
