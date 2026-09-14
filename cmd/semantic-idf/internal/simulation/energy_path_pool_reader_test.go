package simulation

import (
	"database/sql"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Hand observations on the untouched original topology, not acceptance data.
// Native pool heat is 10 kWh; the pool's original Zone multiplier is 3. Native
// component/rate companions intentionally disagree with Monthly Energy so a
// fallback or E+R sum cannot accidentally pass the authority checks.
func energyPathPoolSQLHand(t *testing.T) (string, PurposeRunPlan, energyDriverBuildContext) {
	t.Helper()
	doc := energyPathPoolOriginal(t)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	path := directHVACSQLFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Seed one isolated hand fixture atomically. Per-row filesystem journal
	// commits add no coverage and made the full Windows regression suite hit
	// its timeout; native captures and every observation/assertion are unchanged.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, query := range []string{`DELETE FROM ReportData`, `DELETE FROM ReportDataDictionary`,
		`INSERT INTO "Time" (TimeIndex,Month,Day,Hour,Minute,Year,"Interval",IntervalType,EnvironmentPeriodIndex,WarmupFlag) VALUES (1001,1,1,1,0,2017,60,1,3,0),(1002,1,1,2,0,2017,60,1,3,0)`} {
		if _, err := tx.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	add := func(id int, key, name, unit, frequency string, meter int, value float64) {
		t.Helper()
		if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,?,?,?, 'HVAC')`, id, key, name, unit, meter, frequency); err != nil {
			t.Fatal(err)
		}
		first, second := 1, 2
		if frequency == "Hourly" {
			first, second = 1001, 1002
		}
		if unit == "J" {
			value *= 3600000
		}
		for _, row := range []struct {
			time  int
			value float64
		}{{first, value}, {second, 0}} {
			if _, err := tx.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,?,?)`, row.time, id, row.value); err != nil {
				t.Fatal(err)
			}
		}
	}
	add(1, "", "Electricity:Facility", "J", "Monthly", 1, 122)
	add(2, "", "Pumps:Electricity", "J", "Monthly", 1, 120)
	add(3, "", "Heating:NaturalGas", "J", "Monthly", 1, 45)
	add(4, "", "Heating:Electricity", "J", "Monthly", 1, 2)
	add(5, "", "NaturalGas:Facility", "J", "Monthly", 1, 45)
	add(51, "TEST POOL", "Indoor Pool Water Heating Energy", "J", "Monthly", 0, 10)
	add(52, "TEST POOL", "Indoor Pool Water Heating Rate", "W", "Monthly", 0, 2000)
	add(53, "TEST POOL", "Indoor Pool Water Heating Energy", "J", "Hourly", 0, 1)
	add(54, "TEST POOL", "Indoor Pool Water Heating Rate", "W", "Hourly", 0, 1000)
	add(61, "CENTRAL BOILER", "Boiler NaturalGas Energy", "J", "Monthly", 0, 40)
	add(62, "CENTRAL BOILER", "Boiler Ancillary NaturalGas Energy", "J", "Monthly", 0, 5)
	add(63, "CENTRAL BOILER", "Boiler Ancillary Electricity Energy", "J", "Monthly", 0, 2)
	add(64, "CENTRAL BOILER", "Boiler Heating Energy", "J", "Monthly", 0, 30)
	add(71, "HW CIRC PUMP", "Pump Electricity Energy", "J", "Monthly", 0, 100)
	add(72, "CW CIRC PUMP", "Pump Electricity Energy", "J", "Monthly", 0, 20)
	for i, zone := range []string{"PLENUM-1", "SPACE1-1", "SPACE2-1", "SPACE3-1", "SPACE4-1", "SPACE5-1"} {
		add(101+i, zone, "Zone Air System Sensible Heating Energy", "J", "Monthly", 0, 10)
		add(111+i, zone, "Zone Air System Sensible Cooling Energy", "J", "Monthly", 0, 10)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path, plan, context
}

func energyPathPoolSQLRead(t *testing.T, path string, plan PurposeRunPlan, context energyDriverBuildContext) (energyPathPoolEvidence, []EnergyDataSource, []string) {
	t.Helper()
	db, err := openSimulationSQLiteReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	return readEnergyPathPoolEvidence(db, "eplusout.sql", &plan, context)
}

func energyPathPoolSQLObservation(t *testing.T, evidence energyPathPoolEvidence, definition, name string) energyPathPoolObservation {
	t.Helper()
	for _, observation := range evidence.Observations {
		if observation.Definition.ID == definition && strings.EqualFold(observation.Component.ObjectName, name) {
			return observation
		}
	}
	t.Fatalf("missing original component observation %s/%s", definition, name)
	return energyPathPoolObservation{}
}

func energyPathPoolSQLSource(t *testing.T, sources []EnergyDataSource, id string) EnergyDataSource {
	t.Helper()
	for _, source := range sources {
		if source.ID == id {
			return source
		}
	}
	t.Fatalf("missing source %s", id)
	return EnergyDataSource{}
}

func TestEnergyPathPoolSQLMonthlyAuthorityAndNativeFactorOne(t *testing.T) {
	path, plan, context := energyPathPoolSQLHand(t)
	evidence, sources, labels := energyPathPoolSQLRead(t, path, plan, context)
	if len(evidence.Observations) != 7 || len(sources) != 10 || !reflect.DeepEqual(labels, []string{"01-01 01:00", "01-01 02:00"}) {
		t.Fatalf("native context roster/axis: observations=%d sources=%d labels=%v", len(evidence.Observations), len(sources), labels)
	}
	pool := energyPathPoolSQLObservation(t, evidence, "pool.water_heating", "Test Pool")
	value, raw, ids, known := energyPathHVACConsumptionPeriodValue(pool.Series, "M1", "monthly", true)
	if !pool.Valid || !known || value != 10 || raw != 10 || !reflect.DeepEqual(ids, []string{"sql-rdd-51"}) || pool.Series.Stage != "context" || pool.Series.Level != "context" || pool.Series.ZoneName != "" || pool.Series.EffectiveMultiplier != 1 {
		t.Fatalf("pool transfer borrowed Zone3, rate, or Hourly authority: %+v / %g,%g,%v,%v", pool, value, raw, ids, known)
	}
	if value, _, _, known := energyPathHVACConsumptionPeriodValue(pool.Series, "M2", "monthly", true); !known || value != 0 {
		t.Fatal("native measured zero became absent")
	}
	for _, tc := range []struct {
		id    string
		value float64
	}{{"sql-rdd-51", 10}, {"sql-rdd-52", 1488}, {"sql-rdd-53", 1}, {"sql-rdd-54", 1}, {"sql-rdd-71", 100}, {"sql-rdd-72", 20}} {
		source := energyPathPoolSQLSource(t, sources, tc.id)
		if !energyDataSourceValueKnown(source, energySourceObservedRaw) || !energyDataSourceValueKnown(source, energySourceObservedEffective) || source.RawValue != tc.value || source.EffectiveValue != tc.value || source.EffectiveMultiplier != 1 || source.MultiplierApplication != "already_model_total" || source.DriverRole != energyDriverSourceRoleContext {
			t.Fatalf("native observation must remain factor1 context: %+v want%g", source, tc.value)
		}
		if tc.id == "sql-rdd-53" || tc.id == "sql-rdd-54" {
			if source.HourlyEnergy == nil || !reflect.DeepEqual(source.HourlyEnergy.Values, []float64{1, 0}) || source.HourlyEnergy.Basis != "reported_source" {
				t.Fatalf("native Hourly zero or physical axis lost: %+v", source)
			}
		}
	}
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	loads := 0
	for _, item := range legacy.poolEvidence.LoadSeries {
		if item.ZoneName == "SPACE1-1" && item.Stage == "load" && energyCanonicalServiceKind(item.ServiceKind) == "heating" {
			loads++
			if item.Monthly[1] != 30 {
				t.Fatalf("representative Zone load must apply its original factor3 once, unlike pool: %+v", item)
			}
		}
	}
	if loads != 1 {
		t.Fatalf("pool glue must retain one selected canonical SPACE1 heating authority, got%d", loads)
	}
	for _, node := range legacy.Nodes {
		if stringSliceContains(node.SourceIDs, "sql-rdd-51") || stringSliceContains(node.SourceIDs, "sql-rdd-52") || stringSliceContains(node.SourceIDs, "sql-rdd-53") || stringSliceContains(node.SourceIDs, "sql-rdd-54") || stringSliceContains(node.SourceIDs, "sql-rdd-64") {
			t.Fatalf("component thermal context became an additive graph node: %+v", node)
		}
	}
}

func TestEnergyPathPoolSQLZeroUnknownIdentityAndAxisBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, query  string
		known, valid bool
		value        float64
	}{
		{"native zero", `UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex=51`, true, true, 0},
		{"NULL", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=51`, false, true, 0},
		{"missing rows", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=51`, false, true, 0},
		{"negative", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=51`, false, true, 0},
		{"wrong unit", `UPDATE ReportDataDictionary SET Units='kJ' WHERE ReportDataDictionaryIndex=51`, false, true, 0},
		{"wrong key", `UPDATE ReportDataDictionary SET KeyValue='SPACE1-1' WHERE ReportDataDictionaryIndex=51`, false, true, 0},
		{"Hourly only cannot replace Monthly", `DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=51`, false, true, 0},
		{"duplicate native identity", `INSERT INTO ReportDataDictionary SELECT 151,KeyValue,Name,Units,IsMeter,ReportingFrequency,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=51`, false, false, 0},
		{"duplicate month observation", `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(1,51,36000000)`, false, true, 0},
		{"duplicate weather environment", `INSERT INTO EnvironmentPeriods VALUES(4,3)`, false, true, 0},
		{"invalid Monthly interval", `UPDATE "Time" SET "Interval"=0 WHERE TimeIndex=1`, false, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, plan, context := energyPathPoolSQLHand(t)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.query); err != nil {
				db.Close()
				t.Fatal(err)
			}
			db.Close()
			evidence, sources, _ := energyPathPoolSQLRead(t, path, plan, context)
			pool := energyPathPoolSQLObservation(t, evidence, "pool.water_heating", "Test Pool")
			value, _, _, known := energyPathHVACConsumptionPeriodValue(pool.Series, "M1", "monthly", true)
			if pool.Valid != tc.valid || known != tc.known || known && value != tc.value {
				t.Fatalf("Monthly identity/knownness must not fall back to existing Rate/Hourly: %+v value=%g known=%v", pool, value, known)
			}
			for _, source := range sources {
				if source.ID != "sql-rdd-51" || tc.name == "duplicate native identity" {
					continue
				}
				for pass := 0; pass < 3; pass++ {
					if pass > 0 {
						// Match the source wire used by EnergyExplanationResult,
						// not the legacy raw struct's omitempty representation.
						wire, err := json.Marshal(energyPathSourceWire{source})
						if err != nil {
							t.Fatal(err)
						}
						var decoded EnergyDataSource
						if err := json.Unmarshal(wire, &decoded); err != nil {
							t.Fatal(err)
						}
						source = decoded
					}
					if energyDataSourceValueKnown(source, energySourceObservedRaw) != tc.known || energyDataSourceValueKnown(source, energySourceObservedEffective) != tc.known || tc.known && (source.RawValue != 0 || source.EffectiveValue != 0) {
						t.Fatalf("zero/unknown scalar contract changed on JSON pass%d: %+v", pass, source)
					}
				}
			}
		})
	}
}

func TestEnergyPathPoolSQLHourlyCompanionRejectsIncompleteNativeAxis(t *testing.T) {
	for _, query := range []string{
		`UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=53 AND TimeIndex=1002`,
		`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=53 AND TimeIndex=1002`,
		`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(1001,53,3600000)`,
		`UPDATE "Time" SET "Interval"=30 WHERE TimeIndex=1002`,
		`UPDATE "Time" SET Hour=1 WHERE TimeIndex=1002`,
		`UPDATE ReportDataDictionary SET Units='W' WHERE ReportDataDictionaryIndex=53`,
	} {
		t.Run(query, func(t *testing.T) {
			path, plan, context := energyPathPoolSQLHand(t)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(query); err != nil {
				db.Close()
				t.Fatal(err)
			}
			db.Close()
			evidence, sources, _ := energyPathPoolSQLRead(t, path, plan, context)
			source := energyPathPoolSQLSource(t, sources, "sql-rdd-53")
			if source.HourlyEnergy != nil || energyDataSourceValueKnown(source, energySourceObservedRaw) || energyDataSourceValueKnown(source, energySourceObservedEffective) {
				t.Fatalf("incomplete Hourly series fabricated chart/scalar: %+v", source)
			}
			pool := energyPathPoolSQLObservation(t, evidence, "pool.water_heating", "Test Pool")
			if value, _, _, known := energyPathHVACConsumptionPeriodValue(pool.Series, "M1", "monthly", true); !pool.Valid || !known || math.Abs(value-10) > 1e-12 {
				t.Fatalf("independent Monthly authority corrupted: %g/%v", value, known)
			}
		})
	}
}
