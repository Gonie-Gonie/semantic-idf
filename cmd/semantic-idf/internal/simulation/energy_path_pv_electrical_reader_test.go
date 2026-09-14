package simulation

// Literal SQLite tests, never an EnergyPlus/candidate oracle.
import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type pvElectricalReaderLiteral struct {
	name, key string
	meter     bool
	value     float64
}

// Literal registry independent of the production definition loop. The toy
// observed axis has two Monthly cells and three Hourly cells, not a claim that
// a missing annual calendar is complete or that a battery circuit balances.
func pvElectricalReaderLiterals() []pvElectricalReaderLiteral {
	return []pvElectricalReaderLiteral{
		{"Generator Produced DC Electricity Energy", "PV1", false, 1},
		{"Generator Produced DC Electricity Energy", "PV2", false, 2},
		{"Generator Produced DC Electricity Energy", "PV3", false, 3},
		{"Generator Produced DC Electricity Energy", "PV4", false, 4},
		{"Generator Produced DC Electricity Energy", "PV5", false, 5},
		{"Inverter DC Input Electricity Energy", "INVERTER", false, 11},
		{"Inverter AC Output Electricity Energy", "INVERTER", false, 10},
		{"Inverter Conversion Loss Energy", "INVERTER", false, .0034},
		{"Inverter Conversion Loss Decrement Energy", "INVERTER", false, -.0034},
		{"Inverter Ancillary AC Electricity Energy", "INVERTER", false, 0},
		{"Electric Storage Charge Energy", "BATTERY", false, 7},
		{"Electric Storage Production Decrement Energy", "BATTERY", false, -7},
		{"Electric Storage Discharge Energy", "BATTERY", false, 6},
		{"Electric Storage Thermal Loss Energy", "BATTERY", false, -2.5},
		{"Electric Load Center Produced Electricity Energy", "CENTER", false, 8},
		{"Electric Load Center Produced Thermal Energy", "CENTER", false, -1.5},
		{"Electricity:Facility", "", true, 9},
		{"ElectricityProduced:Facility", "", true, -.25},
		{"ElectricityPurchased:Facility", "", true, 9.25},
		{"ElectricitySurplusSold:Facility", "", true, 0},
	}
}

func pvElectricalReaderDocument(t *testing.T) idf.Document {
	t.Helper()
	doc, err := idf.Parse(`Version,25.1;
ElectricLoadCenter:Distribution,Center,MissingList,TrackElectrical,,,,DirectCurrentWithInverterDCStorage,Inverter,Battery;
ElectricLoadCenter:Storage:Battery,Battery;
ElectricLoadCenter:Inverter:LookUpTable,Inverter;
Generator:Photovoltaic,PV1,MissingSurface;
Generator:Photovoltaic,PV2,MissingSurface;
Generator:Photovoltaic,PV3,MissingSurface;
Generator:Photovoltaic,PV4,MissingSurface;
Generator:Photovoltaic,PV5,MissingSurface;`)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func pvElectricalReaderPlan(doc idf.Document) PurposeRunPlan {
	builder := newPurposePlanBuilder(doc, NormalizeSimulationPurposeRequest(&SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}))
	builder.addEnergyPathPVElectricalOutputs()
	return builder.plan()
}

func pvElectricalReaderCreateSQL(t *testing.T, mutations ...string) string {
	t.Helper()
	return pvElectricalReaderCreateSQLWithLiterals(t, pvElectricalReaderLiterals(), mutations...)
}

func pvElectricalReaderCreateSQLWithLiterals(t *testing.T, literals []pvElectricalReaderLiteral, mutations ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pv-electrical-native.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY,IsMeter INTEGER,Type TEXT,IndexGroup TEXT,TimestepType TEXT,KeyValue TEXT,Name TEXT,ReportingFrequency TEXT,ScheduleName TEXT,Units TEXT)`,
		`CREATE TABLE EnvironmentPeriods (EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentType INTEGER)`,
		`CREATE TABLE Time (TimeIndex INTEGER PRIMARY KEY,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,"Interval" INTEGER,IntervalType INTEGER,EnvironmentPeriodIndex INTEGER,WarmupFlag INTEGER)`,
		`CREATE TABLE ReportData (ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
		`INSERT INTO EnvironmentPeriods VALUES (1,1),(2,3)`,
		`INSERT INTO Time VALUES (1,2017,1,1,1,0,60,1,1,0),(2,2017,1,1,1,0,60,1,2,1),(101,2017,1,31,24,0,44640,3,2,NULL),(102,2017,2,28,24,0,40320,3,2,NULL),(201,2017,1,1,1,0,60,1,2,0),(202,2017,1,1,2,0,60,1,2,0),(203,2017,1,1,3,0,60,1,2,0)`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for index, literal := range literals {
		for frequencyIndex, frequency := range []string{"Monthly", "Hourly"} {
			id := 900 + 2*index + frequencyIndex
			isMeter := 0
			step, group := "HVAC System", "Plant"
			var key any = literal.key
			if literal.meter {
				isMeter = 1
				step, group = "Zone", "Meter"
				key = nil
			}
			if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES (?,?, 'Sum',?,?,?,?,?,NULL,'J')`, id, isMeter, group, step, key, literal.name, frequency); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(`INSERT INTO ReportData (TimeIndex,ReportDataDictionaryIndex,Value) VALUES (1,?,999999999999),(2,?,999999999999)`, id, id); err != nil {
				t.Fatal(err)
			}
			if frequency == "Monthly" {
				if _, err := tx.Exec(`INSERT INTO ReportData (TimeIndex,ReportDataDictionaryIndex,Value) VALUES (101,?,?),(102,?,0)`, id, literal.value*3600000, id); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := tx.Exec(`INSERT INTO ReportData (TimeIndex,ReportDataDictionaryIndex,Value) VALUES (201,?,?),(202,?,0),(203,?,?)`, id, literal.value*1800000, id, id, literal.value*1800000); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	for _, statement := range mutations {
		if _, err := tx.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path
}

func pvElectricalReaderRead(t *testing.T, path string, doc idf.Document, existing []EnergyDataSource) (energyPathPVElectricalEvidence, []EnergyDataSource, []string) {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	db, err := openSimulationSQLiteReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1) // An unclosed outer cursor must not be hidden by a new connection.
	plan := pvElectricalReaderPlan(doc)
	evidence, sources, labels, err := readEnergyPathPVElectricalObservations(db, filepath.Base(path), &plan, energyPathBuildPVElectricalInventory(doc), existing)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		t.Fatal("reader closed its caller-owned database")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("read-only native SQL changed")
	}
	return evidence, sources, labels
}

func pvElectricalReaderSource(t *testing.T, sources []EnergyDataSource, id int) EnergyDataSource {
	t.Helper()
	want := fmt.Sprintf("sql-rdd-%d", id)
	count := 0
	var out EnergyDataSource
	for _, source := range sources {
		if source.ID == want {
			out = source
			count++
		}
	}
	if count != 1 {
		t.Fatalf("actual RDD %s occurs %d times", want, count)
	}
	return out
}

func pvElectricalReaderObservation(t *testing.T, evidence energyPathPVElectricalEvidence, name, frequency string) energyPathPVElectricalObservation {
	t.Helper()
	for _, observation := range evidence.Observations {
		if observation.Intent.Target.Definition.Name == name && observation.Intent.Frequency == frequency {
			return observation
		}
	}
	t.Fatalf("observation absent %s/%s", name, frequency)
	return energyPathPVElectricalObservation{}
}

func TestPVElectricalReaderDraftRetainsFortyNativeSignedObservations(t *testing.T) {
	doc := pvElectricalReaderDocument(t)
	original := doc.String()
	path := pvElectricalReaderCreateSQL(t)
	evidence, sources, labels := pvElectricalReaderRead(t, path, doc, nil)
	if len(evidence.Observations) != 40 || len(sources) != 40 || len(evidence.SourceSnapshots) != 40 || len(evidence.UnmatchedDictionaries) != 0 || len(evidence.Issues) != 0 {
		t.Fatalf("20 x M/H native roster lost: %+v / %d sources", evidence, len(sources))
	}
	if !reflect.DeepEqual(labels, []string{"01-01 01:00", "01-01 02:00", "01-01 03:00"}) {
		t.Fatalf("wrong native end-of-hour axis: %v", labels)
	}
	for index, literal := range pvElectricalReaderLiterals() {
		for frequencyIndex, frequency := range []string{"Monthly", "Hourly"} {
			source := pvElectricalReaderSource(t, sources, 900+2*index+frequencyIndex)
			if source.Name != literal.name || source.KeyValue != literal.key || source.ReportingFrequency != frequency || source.SourceUnit != "J" || source.Units != "J" || source.IsMeter != literal.meter || source.NormalizedUnit != "kWh" || source.AggregationMethod != "sum_report_data" || source.SourceType != "sql_report_data" {
				t.Fatalf("actual native dictionary changed: %+v", source)
			}
			if !energyDataSourceValueKnown(source, energySourceObservedRaw) || !energyDataSourceValueKnown(source, energySourceObservedEffective) || math.Abs(source.RawValue-literal.value) > 1e-12 || source.EffectiveValue != source.RawValue {
				t.Fatalf("native signed/zero total changed: %+v want %v", source, literal.value)
			}
			if source.AggregationBasis != "model_total" || source.EffectiveMultiplier != 1 || source.MultiplierApplication != "already_model_total" || source.ObjectIndex != nil || source.ZoneName != "" || source.AllocationApplied || source.AllocatedValue != 0 || len(source.ScopeDetails) != 0 || len(source.RelatedEntityIDs) != 0 {
				t.Fatalf("source invented physical/executed owner, Zone or allocation: %+v", source)
			}
			if frequency == "Monthly" {
				if source.HourlyEnergy != nil {
					t.Fatal("Monthly source got a fabricated hourly curve")
				}
			} else {
				if source.HourlyEnergy == nil || !reflect.DeepEqual(source.HourlyEnergy.Values, []float64{roundedEnergyNumber(literal.value / 2), 0, roundedEnergyNumber(literal.value / 2)}) {
					t.Fatalf("signed/zero chart policy changed: %+v", source)
				}
			}
		}
	}
	for _, observation := range evidence.Observations {
		if observation.Status != "observed" || len(observation.Dictionaries) != 1 || len(observation.SourceIDs) != 1 || observation.ExcludedDesignOrWarmupRows != 2 || len(observation.MissingTimes)+len(observation.NullTimes)+len(observation.InvalidTimes)+len(observation.DuplicateTimes) != 0 {
			t.Fatalf("native knownness/axis changed: %+v", observation)
		}
		if observation.Intent.Frequency == "Monthly" {
			if len(observation.MonthlyValues) != 2 || observation.MonthlyValues[2] != 0 {
				t.Fatal("Monthly native zero was lost")
			}
		} else if observation.MonthlyValues != nil {
			t.Fatal("Hourly was promoted into Monthly authority")
		}
	}
	decrement := pvElectricalReaderSource(t, sources, 917)
	if decrement.RawValue == decrement.HourlyEnergy.Values[0]+decrement.HourlyEnergy.Values[2] {
		t.Fatal("tiny native total was rebuilt from rounded chart values")
	}
	if doc.String() != original {
		t.Fatal("source query mutated original")
	}
}

func TestPVElectricalReaderDraftMissingNullInvalidAndDuplicateRemainUnknown(t *testing.T) {
	for _, trial := range []struct {
		name, mutation string
		id             int
		status         string
	}{
		{"monthly_null", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=922 AND TimeIndex=102`, 922, "incomplete_or_invalid_observation"},
		{"monthly_missing", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=922 AND TimeIndex=102`, 922, "incomplete_or_invalid_observation"},
		{"monthly_duplicate", `INSERT INTO ReportData (TimeIndex,ReportDataDictionaryIndex,Value) VALUES (102,922,0)`, 922, "incomplete_or_invalid_observation"},
		{"monthly_positive_decrement", `UPDATE ReportData SET Value=3600000 WHERE ReportDataDictionaryIndex=922 AND TimeIndex=102`, 922, "incomplete_or_invalid_observation"},
		{"monthly_nonfinite", `UPDATE ReportData SET Value=1e999 WHERE ReportDataDictionaryIndex=922 AND TimeIndex=102`, 922, "incomplete_or_invalid_observation"},
		{"hourly_null", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=923 AND TimeIndex=202`, 923, "incomplete_or_invalid_observation"},
		{"hourly_missing", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=923 AND TimeIndex=202`, 923, "incomplete_or_invalid_observation"},
		{"hourly_duplicate", `INSERT INTO ReportData (TimeIndex,ReportDataDictionaryIndex,Value) VALUES (202,923,0)`, 923, "incomplete_or_invalid_observation"},
		{"hourly_positive_decrement", `UPDATE ReportData SET Value=3600000 WHERE ReportDataDictionaryIndex=923 AND TimeIndex=202`, 923, "incomplete_or_invalid_observation"},
		{"hourly_negative_purchased", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=919 AND TimeIndex=202`, 919, "incomplete_or_invalid_observation"},
		{"hourly_nonfinite", `UPDATE ReportData SET Value=1e999 WHERE ReportDataDictionaryIndex=923 AND TimeIndex=202`, 923, "incomplete_or_invalid_observation"},
		{"off_axis_weather_row", `INSERT INTO ReportData (TimeIndex,ReportDataDictionaryIndex,Value) VALUES (101,923,-1)`, 923, "incomplete_or_invalid_observation"},
		{"orphan_time", `INSERT INTO ReportData (TimeIndex,ReportDataDictionaryIndex,Value) VALUES (999999,923,-1)`, 923, "incomplete_or_invalid_observation"},
	} {
		t.Run(trial.name, func(t *testing.T) {
			path := pvElectricalReaderCreateSQL(t, trial.mutation)
			doc := pvElectricalReaderDocument(t)
			// A prior generic reader's known positive scalar must not survive native
			// source-local validation, including stale scoped/allocation metadata.
			prior := EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", trial.id), Name: "stale", RawValue: 999, EffectiveValue: 999, observedValuePresence: 3, ZoneName: "wrong", AllocationApplied: true, AllocatedValue: 999, ScopeDetails: []EnergyDataSourceScopeDetail{{RawValue: 999}}}
			evidence, sources, _ := pvElectricalReaderRead(t, path, doc, []EnergyDataSource{prior})
			source := pvElectricalReaderSource(t, sources, trial.id)
			if energyDataSourceValueKnown(source, energySourceObservedRaw) || energyDataSourceValueKnown(source, energySourceObservedEffective) || source.RawValue != 0 || source.EffectiveValue != 0 || source.HourlyEnergy != nil || source.ZoneName != "" || len(source.ScopeDetails) != 0 || source.AllocationApplied {
				t.Fatalf("invalid observation retained a scalar/chart/scope: %+v", source)
			}
			observation := pvElectricalReaderObservation(t, evidence, source.Name, source.ReportingFrequency)
			if observation.Status != trial.status {
				t.Fatalf("status=%s want %s", observation.Status, trial.status)
			}
			if strings.HasPrefix(trial.name, "monthly_") {
				if value, known := observation.MonthlyValues[1]; !known || value != -7 {
					t.Fatal("unaffected native month was erased")
				}
				if _, known := observation.MonthlyValues[2]; known {
					t.Fatal("missing/invalid month fabricated zero")
				}
			}
			good := pvElectricalReaderSource(t, sources, 900)
			if !energyDataSourceValueKnown(good, energySourceObservedRaw) || good.RawValue != 1 {
				t.Fatal("one source failure erased independent PV observation")
			}
		})
	}
}

func TestPVElectricalReaderDraftNativeDictionaryAndCalendarBoundaries(t *testing.T) {
	for _, trial := range []struct {
		name, mutation string
		id             int
		wantCount      int
		status         string
	}{
		{"duplicate_native_dictionary", `INSERT INTO ReportDataDictionary SELECT 1999,IsMeter,Type,IndexGroup,TimestepType,KeyValue,Name,ReportingFrequency,ScheduleName,Units FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=922`, 922, 41, "duplicate_dictionary"},
		{"wrong_unit", `UPDATE ReportDataDictionary SET Units='W' WHERE ReportDataDictionaryIndex=922`, 922, 40, "invalid_native_dictionary_or_owner"},
		{"wrong_aggregation", `UPDATE ReportDataDictionary SET Type='Avg' WHERE ReportDataDictionaryIndex=922`, 922, 40, "invalid_native_dictionary_or_owner"},
		{"wrong_component_timestep", `UPDATE ReportDataDictionary SET TimestepType='Zone' WHERE ReportDataDictionaryIndex=922`, 922, 40, "invalid_native_dictionary_or_owner"},
		{"wrong_meter_timestep", `UPDATE ReportDataDictionary SET TimestepType='HVAC System' WHERE ReportDataDictionaryIndex=932`, 932, 40, "invalid_native_dictionary_or_owner"},
		{"null_meter_kind", `UPDATE ReportDataDictionary SET IsMeter=NULL WHERE ReportDataDictionaryIndex=922`, 922, 40, "invalid_native_dictionary_or_owner"},
		{"wrong_meter_kind", `UPDATE ReportDataDictionary SET IsMeter=2 WHERE ReportDataDictionaryIndex=922`, 922, 40, "invalid_native_dictionary_or_owner"},
		{"scheduled_dictionary", `UPDATE ReportDataDictionary SET ScheduleName='LimitedSchedule' WHERE ReportDataDictionaryIndex=922`, 922, 40, "invalid_native_dictionary_or_owner"},
		{"ambiguous_weather", `INSERT INTO EnvironmentPeriods VALUES (3,3)`, 922, 40, "weather_axis_unavailable"},
		{"wrong_hourly_interval", `UPDATE Time SET "Interval"=30 WHERE TimeIndex=202`, 923, 40, "weather_axis_unavailable"},
		{"duplicate_hourly_calendar", `UPDATE Time SET Hour=1 WHERE TimeIndex=202`, 923, 40, "weather_axis_unavailable"},
		{"duplicate_monthly_calendar", `UPDATE Time SET Month=1 WHERE TimeIndex=102`, 922, 40, "weather_axis_unavailable"},
	} {
		t.Run(trial.name, func(t *testing.T) {
			path := pvElectricalReaderCreateSQL(t, trial.mutation)
			evidence, sources, _ := pvElectricalReaderRead(t, path, pvElectricalReaderDocument(t), nil)
			if len(sources) != trial.wantCount {
				t.Fatalf("dictionary identity discarded/duplicated: %d want %d", len(sources), trial.wantCount)
			}
			source := pvElectricalReaderSource(t, sources, trial.id)
			if energyDataSourceValueKnown(source, energySourceObservedRaw) || source.HourlyEnergy != nil {
				t.Fatalf("unproved native metadata/calendar became known: %+v", source)
			}
			observation := pvElectricalReaderObservation(t, evidence, source.Name, source.ReportingFrequency)
			if observation.Status != trial.status {
				t.Fatalf("wrong evidence boundary: %+v", observation)
			}
			if trial.name == "duplicate_native_dictionary" {
				duplicate := pvElectricalReaderSource(t, sources, 1999)
				if energyDataSourceValueKnown(duplicate, energySourceObservedRaw) || len(observation.Dictionaries) != 2 || len(observation.SourceIDs) != 2 {
					t.Fatal("duplicate dictionary was selected or silently merged")
				}
			}
		})
	}
}

func TestPVElectricalReaderDraftNativeNullKeyAndMissingDictionaryAreNotZero(t *testing.T) {
	for _, mutation := range []string{
		`UPDATE ReportDataDictionary SET KeyValue=NULL WHERE ReportDataDictionaryIndex=922`,
		`UPDATE ReportDataDictionary SET KeyValue='FOREIGN' WHERE ReportDataDictionaryIndex=922`,
		`DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=922`,
	} {
		path := pvElectricalReaderCreateSQL(t, mutation)
		prior := EnergyDataSource{ID: "sql-rdd-922", Name: "Electric Storage Production Decrement Energy", KeyValue: "BATTERY", SourceUnit: "J", ReportingFrequency: "Monthly", RawValue: -99, EffectiveValue: -99, observedValuePresence: 3}
		evidence, sources, _ := pvElectricalReaderRead(t, path, pvElectricalReaderDocument(t), []EnergyDataSource{prior})
		source := pvElectricalReaderSource(t, sources, 922)
		if energyDataSourceValueKnown(source, energySourceObservedRaw) || source.RawValue != 0 {
			t.Fatal("missing/foreign native dictionary retained stale known scalar")
		}
		observation := pvElectricalReaderObservation(t, evidence, "Electric Storage Production Decrement Energy", "Monthly")
		if observation.Status != "missing_dictionary" || len(observation.SourceIDs) != 0 {
			t.Fatal("foreign key borrowed another source identity")
		}
		meter := pvElectricalReaderSource(t, sources, 932)
		if meter.KeyValue != "" || !meter.IsMeter || !energyDataSourceValueKnown(meter, energySourceObservedRaw) {
			t.Fatal("native meter NULL key incorrectly required an equipment owner")
		}
	}
}

func TestPVElectricalReaderDraftReusesSourceIDAndIndependentRequestNavigation(t *testing.T) {
	doc := pvElectricalReaderDocument(t)
	requestDocument, err := idf.Parse("Output:Variable,BATTERY,Electric Storage Charge Energy,Monthly;")
	if err != nil {
		t.Fatal(err)
	}
	requestIndex := len(doc.Objects)
	requestDocument.Objects[0].Index = requestIndex
	doc.Objects = append(doc.Objects, requestDocument.Objects[0])
	plan := pvElectricalReaderPlan(doc)
	path := pvElectricalReaderCreateSQL(t)
	db, err := openSimulationSQLiteReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := EnergyDataSource{ID: "sql-rdd-920", Name: "Electric Storage Charge Energy", KeyValue: "BATTERY", SourceUnit: "J", ReportingFrequency: "Monthly", RawValue: 999, EffectiveValue: 999, observedValuePresence: 3}
	unrelated := EnergyDataSource{ID: "unrelated", Name: "Unrelated", RawValue: 41, RelatedEntityIDs: []string{"keep"}}
	existing := []EnergyDataSource{old, unrelated, old}
	before, _ := json.Marshal(existing)
	evidence, sources, _, err := readEnergyPathPVElectricalObservations(db, filepath.Base(path), &plan, energyPathBuildPVElectricalInventory(doc), existing)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 41 {
		t.Fatalf("one RDD was appended twice: %d", len(sources))
	}
	monthly, hourly := pvElectricalReaderSource(t, sources, 920), pvElectricalReaderSource(t, sources, 921)
	if monthly.RawValue != 7 || monthly.ObjectIndex == nil || *monthly.ObjectIndex != requestIndex || hourly.ObjectIndex != nil {
		t.Fatalf("actual-frequency request/physical/execution indices conflated: %+v / %+v", monthly, hourly)
	}
	if sources[1].ID != "unrelated" || !reflect.DeepEqual(sources[1], unrelated) {
		t.Fatal("unrelated source changed")
	}
	after, _ := json.Marshal(existing)
	if string(before) != string(after) {
		t.Fatal("merge mutated prior caller sources")
	}
	observation := pvElectricalReaderObservation(t, evidence, "Electric Storage Charge Energy", "Monthly")
	if len(observation.Intent.Target.OriginalOwners) != 1 || observation.Intent.Target.OriginalOwners[0].ObjectIndex == requestIndex {
		t.Fatal("physical original provenance overwritten with request index")
	}
	if energyPathPVElectricalOutputRequestIndex(observation.Intent, &plan, nil) != nil {
		t.Fatal("deduplicated plan alone became original opener proof")
	}
	// A second different original request opener is ambiguous, not first-wins.
	for _, output := range append([]PurposeOutputObject(nil), plan.OutputObjects...) {
		if output.VariableName == "Electric Storage Charge Energy" && output.ReportingFrequency == "Monthly" {
			copy := output
			index := 124
			copy.ObjectIndex = &index
			plan.OutputObjects = append(plan.OutputObjects, copy)
			break
		}
	}
	if energyPathPVElectricalOutputRequestIndex(observation.Intent, &plan, evidence.Inventory.OriginalOutputs) != nil {
		t.Fatal("duplicate request opener chose arbitrary index")
	}
	wrongFrequencyPlan := plan
	wrongFrequencyPlan.OutputObjects = append([]PurposeOutputObject(nil), plan.OutputObjects...)
	for i := range wrongFrequencyPlan.OutputObjects {
		if wrongFrequencyPlan.OutputObjects[i].VariableName == "Electric Storage Charge Energy" {
			wrongFrequencyPlan.OutputObjects[i].ReportingFrequency = "Hourly"
		}
	}
	if energyPathPVElectricalOutputRequestIndex(observation.Intent, &wrongFrequencyPlan, evidence.Inventory.OriginalOutputs) != nil {
		t.Fatal("inconsistent/other-frequency output metadata borrowed Monthly opener")
	}
	if got := applyEnergyPathPVElectricalSourceObservations(sources, evidence); !reflect.DeepEqual(got, sources) {
		t.Fatal("authoritative-source reapply is not idempotent")
	}
	for i := range sources {
		if sources[i].ID == "sql-rdd-923" {
			sources[i].HourlyEnergy.Values[0] = 999
		}
	}
	restored := applyEnergyPathPVElectricalSourceObservations(sources, evidence)
	if pvElectricalReaderSource(t, restored, 923).HourlyEnergy.Values[0] != -3.5 {
		t.Fatal("downstream chart mutation altered the independent native snapshot")
	}
}

func TestPVElectricalReaderDraftWireRetainsSignedZeroAndUnknownThroughTwoLoads(t *testing.T) {
	path := pvElectricalReaderCreateSQL(t, `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=922 AND TimeIndex=102`)
	_, sources, _ := pvElectricalReaderRead(t, path, pvElectricalReaderDocument(t), nil)
	for round := 0; round < 2; round++ {
		raw, err := json.Marshal(energyPathSourcesForWire(sources))
		if err != nil {
			t.Fatal(err)
		}
		var decoded []EnergyDataSource
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		sources = decoded
		zero := pvElectricalReaderSource(t, sources, 918)
		unknown := pvElectricalReaderSource(t, sources, 922)
		signed := pvElectricalReaderSource(t, sources, 923)
		if !energyDataSourceValueKnown(zero, energySourceObservedRaw) || zero.RawValue != 0 || energyDataSourceValueKnown(unknown, energySourceObservedRaw) || unknown.RawValue != 0 || signed.RawValue != -7 || signed.HourlyEnergy == nil || signed.HourlyEnergy.Values[0] != -3.5 {
			t.Fatalf("wire round %d changed sign/knownness", round)
		}
		for _, source := range sources {
			if source.ZoneName != "" || source.AllocationApplied || len(source.ScopeDetails) != 0 {
				t.Fatal("source-only wire created Zone allocation")
			}
		}
	}
}

func TestPVElectricalReaderDraftAmbiguousOriginalOwnerRetainsUnknownNativeIdentities(t *testing.T) {
	doc := pvElectricalReaderDocument(t)
	peer, err := idf.Parse("ElectricLoadCenter:Storage:Simple,Battery;")
	if err != nil {
		t.Fatal(err)
	}
	peer.Objects[0].Index = len(doc.Objects)
	doc.Objects = append(doc.Objects, peer.Objects[0])
	evidence, sources, _ := pvElectricalReaderRead(t, pvElectricalReaderCreateSQL(t), doc, nil)
	if len(sources) != 40 || len(evidence.Observations) != 40 {
		t.Fatal("ambiguous original erased actual native source identities")
	}
	for id := 920; id <= 927; id++ {
		source := pvElectricalReaderSource(t, sources, id)
		if energyDataSourceValueKnown(source, energySourceObservedRaw) || source.HourlyEnergy != nil {
			t.Fatal("ambiguous original reporting owner gained quantity authority")
		}
		observation := pvElectricalReaderObservation(t, evidence, source.Name, source.ReportingFrequency)
		if observation.Status != "invalid_native_dictionary_or_owner" || len(observation.Intent.Target.OriginalOwners) != 2 {
			t.Fatal("ambiguous original owner trace was discarded")
		}
	}
	if source := pvElectricalReaderSource(t, sources, 900); !energyDataSourceValueKnown(source, energySourceObservedRaw) {
		t.Fatal("independent PV source was erased by a storage ambiguity")
	}
}

func TestPVElectricalReaderDraftSchemaFailureRevokesOnlyNativePriorValues(t *testing.T) {
	path := pvElectricalReaderCreateSQL(t, `ALTER TABLE ReportDataDictionary RENAME COLUMN Type TO UnprovedType`)
	db, err := openSimulationSQLiteReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	doc := pvElectricalReaderDocument(t)
	plan := pvElectricalReaderPlan(doc)
	prior := EnergyDataSource{ID: "sql-rdd-920", Name: "Electric Storage Charge Energy", KeyValue: "BATTERY", SourceUnit: "J", ReportingFrequency: "Monthly", RawValue: 99, EffectiveValue: 99, observedValuePresence: 3}
	unrelated := EnergyDataSource{ID: "unrelated", Name: "Other Source", RawValue: 4}
	evidence, sources, labels, err := readEnergyPathPVElectricalObservations(db, filepath.Base(path), &plan, energyPathBuildPVElectricalInventory(doc), []EnergyDataSource{prior, unrelated})
	if err == nil || len(evidence.Issues) != 1 || len(labels) != 0 || len(sources) != 2 || len(evidence.Observations) != 40 {
		t.Fatal("native schema failure did not remain explicit")
	}
	if source := pvElectricalReaderSource(t, sources, 920); energyDataSourceValueKnown(source, energySourceObservedRaw) || source.RawValue != 0 {
		t.Fatal("schema failure left generic native scalar known")
	}
	if !reflect.DeepEqual(sources[1], unrelated) {
		t.Fatal("schema failure changed unrelated source")
	}
	for _, observation := range evidence.Observations {
		if observation.Status != "native_schema_unavailable" || len(observation.SourceIDs) != 0 {
			t.Fatal("missing schema created source identity proof")
		}
	}
}

func TestPVElectricalReaderDraftDoesNotOptInNoOriginalOrUnreviewedVersion(t *testing.T) {
	path := pvElectricalReaderCreateSQL(t)
	db, err := openSimulationSQLiteReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	doc := pvElectricalReaderDocument(t)
	plan := pvElectricalReaderPlan(doc)
	unreviewed := pvElectricalReaderDocument(t)
	unreviewed.Objects[0].Fields[0].Value = "22.1"
	prior := []EnergyDataSource{{ID: "legacy", Name: "Electric Storage Charge Energy", RawValue: 7}}
	for _, inventory := range []energyPathPVElectricalInventory{{}, energyPathBuildPVElectricalInventory(idf.Document{}), energyPathBuildPVElectricalInventory(unreviewed)} {
		evidence, sources, labels, err := readEnergyPathPVElectricalObservations(db, filepath.Base(path), &plan, inventory, prior)
		if err != nil || len(evidence.Observations) != 0 || len(labels) != 0 || !reflect.DeepEqual(sources, prior) {
			t.Fatal("reader changed no-original/unreviewed legacy outside explicit native scope")
		}
	}
}

func pvElectricalReaderAppendOriginal(t *testing.T, doc *idf.Document, text string) {
	t.Helper()
	extra, err := idf.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	for _, object := range extra.Objects {
		object.Index = len(doc.Objects)
		doc.Objects = append(doc.Objects, object)
	}
}

func TestPVElectricalReaderDraftRawOriginalOutputDuplicatesDoNotGainUniqueNavigation(t *testing.T) {
	for _, trial := range []struct{ name, firstKey, secondKey string }{
		{"identical_exact", "BATTERY", "BATTERY"},
		{"identical_wildcards", "*", "*"},
		{"identical_default_wildcards", "", ""},
		{"equivalent_default_and_explicit_wildcards", "", "*"},
		{"overlapping_exact_and_wildcard", "BATTERY", "*"},
	} {
		t.Run(trial.name, func(t *testing.T) {
			doc := pvElectricalReaderDocument(t)
			for _, key := range []string{trial.firstKey, trial.secondKey} {
				pvElectricalReaderAppendOriginal(t, &doc, fmt.Sprintf("Output:Variable,%s,Electric Storage Charge Energy,Monthly;", key))
			}
			before := doc.String()
			inventory := energyPathBuildPVElectricalInventory(doc)
			plan := pvElectricalReaderPlan(doc)
			if len(inventory.OriginalOutputs) != 2 {
				t.Fatalf("raw original output census deduplicated: %+v", inventory.OriginalOutputs)
			}
			if inventory.OriginalOutputs[0].ObjectIndex == inventory.OriginalOutputs[1].ObjectIndex || !inventory.OriginalOutputs[0].NativeShapeValid || !inventory.OriginalOutputs[1].NativeShapeValid {
				t.Fatal("raw duplicate object indices/shape lost")
			}
			// This is the actual production deduplication which hid the ambiguity:
			// two identical raw declarations become one indexed plan request.
			indexedPlanCount := 0
			for _, output := range plan.OutputObjects {
				if output.VariableName == "Electric Storage Charge Energy" && output.ReportingFrequency == "Monthly" && output.ObjectIndex != nil {
					indexedPlanCount++
				}
			}
			if indexedPlanCount != 1 {
				t.Fatalf("fixture did not exercise real deduplicated/reused plan: %d", indexedPlanCount)
			}
			evidence, sources, _ := pvElectricalReaderRead(t, pvElectricalReaderCreateSQL(t), doc, nil)
			source := pvElectricalReaderSource(t, sources, 920)
			if source.ObjectIndex != nil || source.RawValue != 7 || !energyDataSourceValueKnown(source, energySourceObservedRaw) || !energyDataSourceValueKnown(source, energySourceObservedEffective) {
				t.Fatalf("original request ambiguity changed native quantity or chose opener: %+v", source)
			}
			observation := pvElectricalReaderObservation(t, evidence, "Electric Storage Charge Energy", "Monthly")
			if observation.Status != "observed" || len(observation.SourceIDs) != 1 {
				t.Fatal("request ambiguity became a dictionary/source ambiguity")
			}
			apply := PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeAddMissingOnly)
			if len(apply.Updates) != 0 || len(apply.RemoveObjectIndexes) != 0 {
				t.Fatal("duplicate originals were rewritten or removed")
			}
			applied, preview := idf.ApplyOutput(doc, apply)
			if !preview.CanApply {
				t.Fatalf("add-only output plan failed: %+v", preview)
			}
			for i, original := range doc.Objects {
				if !reflect.DeepEqual(original, applied.Objects[i]) {
					t.Fatal("add-only run changed an original duplicate")
				}
			}
			if doc.String() != before {
				t.Fatal("raw census or source read changed original")
			}
		})
	}
}

func TestPVElectricalReaderDraftThreeStorageTypesShareOnlyFourNativeEnergyObservations(t *testing.T) {
	for _, kind := range []string{"ElectricLoadCenter:Storage:Simple", "ElectricLoadCenter:Storage:Battery", "ElectricLoadCenter:Storage:LiIonNMCBattery"} {
		t.Run(kind, func(t *testing.T) {
			doc := pvElectricalReaderDocument(t)
			for i := range doc.Objects {
				if strings.EqualFold(doc.Objects[i].Type, "ElectricLoadCenter:Storage:Battery") {
					doc.Objects[i].Type = kind
				}
			}
			evidence, sources, _ := pvElectricalReaderRead(t, pvElectricalReaderCreateSQL(t), doc, nil)
			if len(evidence.Observations) != 40 || len(sources) != 40 || len(evidence.UnmatchedDictionaries) != 0 {
				t.Fatal("common reporting support changed the twenty-identity roster")
			}
			for id := 920; id <= 927; id++ {
				source := pvElectricalReaderSource(t, sources, id)
				if !energyDataSourceValueKnown(source, energySourceObservedRaw) || source.EffectiveValue != source.RawValue || source.EffectiveMultiplier != 1 || source.MultiplierApplication != "already_model_total" {
					t.Fatalf("common native storage observation incorrectly unknown: %+v", source)
				}
				observation := pvElectricalReaderObservation(t, evidence, source.Name, source.ReportingFrequency)
				if observation.Status != "observed" || len(observation.Intent.Target.OriginalOwners) != 1 || observation.Intent.Target.OriginalOwners[0].ObjectType != kind {
					t.Fatal("actual storage type was relabeled as Battery physics")
				}
			}
		})
	}
}

func TestPVElectricalReaderDraftMixedDistinctStorageKeysRemainKnownAndCollisionIsLocal(t *testing.T) {
	doc := pvElectricalReaderDocument(t)
	pvElectricalReaderAppendOriginal(t, &doc, "ElectricLoadCenter:Storage:Simple,SimpleOne; ElectricLoadCenter:Storage:LiIonNMCBattery,LiIonOne;")
	literals := append(pvElectricalReaderLiterals(), []pvElectricalReaderLiteral{
		{"Electric Storage Charge Energy", "SIMPLEONE", false, 3},
		{"Electric Storage Production Decrement Energy", "SIMPLEONE", false, -3},
		{"Electric Storage Discharge Energy", "SIMPLEONE", false, 2},
		{"Electric Storage Thermal Loss Energy", "SIMPLEONE", false, -.2},
		{"Electric Storage Charge Energy", "LIIONONE", false, 5},
		{"Electric Storage Production Decrement Energy", "LIIONONE", false, -5},
		{"Electric Storage Discharge Energy", "LIIONONE", false, 4},
		{"Electric Storage Thermal Loss Energy", "LIIONONE", false, -.5},
	}...)
	path := pvElectricalReaderCreateSQLWithLiterals(t, literals)
	evidence, sources, _ := pvElectricalReaderRead(t, path, doc, nil)
	if len(evidence.Observations) != 56 || len(sources) != 56 || len(evidence.UnmatchedDictionaries) != 0 || len(evidence.Inventory.UnreviewedOwners) != 0 {
		t.Fatalf("mixed common storage keys were degraded: observations=%d sources=%d unmatched=%d", len(evidence.Observations), len(sources), len(evidence.UnmatchedDictionaries))
	}
	for index, literal := range literals {
		for offset := 0; offset < 2; offset++ {
			source := pvElectricalReaderSource(t, sources, 900+2*index+offset)
			if source.KeyValue != literal.key || math.Abs(source.RawValue-literal.value) > 1e-12 || !energyDataSourceValueKnown(source, energySourceObservedRaw) {
				t.Fatalf("distinct native reporting key/value lost: %+v", source)
			}
		}
	}
	// Native same-name cross-type collision still invalidates all four common
	// energies for that key, while the third distinct storage remains known.
	for i := range doc.Objects {
		if doc.Objects[i].Type == "ElectricLoadCenter:Storage:Simple" {
			doc.Objects[i].Fields[0].Value = "Battery"
		}
	}
	_, collided, _ := pvElectricalReaderRead(t, path, doc, nil)
	for id := 920; id <= 927; id++ {
		if source := pvElectricalReaderSource(t, collided, id); energyDataSourceValueKnown(source, energySourceObservedRaw) || source.HourlyEnergy != nil {
			t.Fatal("same-key cross-type collision selected a storage owner")
		}
	}
	for id := 948; id <= 955; id++ {
		if source := pvElectricalReaderSource(t, collided, id); !energyDataSourceValueKnown(source, energySourceObservedRaw) {
			t.Fatal("one key collision erased the independent LiIon storage")
		}
	}
}
