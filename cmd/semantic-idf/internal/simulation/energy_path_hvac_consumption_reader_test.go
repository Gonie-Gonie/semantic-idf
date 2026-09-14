package simulation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func mixedHeatingConsumptionSQLFixture(t *testing.T) (string, string, idf.Document, PurposeRunPlan, energyDriverBuildContext) {
	t.Helper()
	inputPath := filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "5ZoneElectricBaseboard.idf")
	data, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	doc := parsePurposePlanFixture(t, string(data))
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare})
	path := directHVACSQLFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{`DELETE FROM ReportData`, `DELETE FROM ReportDataDictionary`} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	add := func(id int, key, name, unit string, meter int, first float64) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,?,?,'Monthly','HVAC')`, id, key, name, unit, meter); err != nil {
			t.Fatal(err)
		}
		for month := 1; month <= 2; month++ {
			value := first * float64(month)
			if unit == "J" {
				value *= 3600000
			}
			if _, err := db.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,?,?)`, month, id, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	add(1, "", "Electricity:Facility", "J", 1, 50)
	add(2, "", "Heating:Electricity", "J", 1, 50)
	add(3, "", "NaturalGas:Facility", "J", 1, 100)
	add(4, "", "Heating:NaturalGas", "J", 1, 100)
	add(50, "SPACE2-1 BASEBOARD", "Baseboard Electricity Energy", "J", 0, 10)
	add(51, "SPACE4-1 BASEBOARD", "Baseboard Electricity Energy", "J", 0, 30)
	add(52, "CENTRAL BOILER", energyPathBoilerAncillaryElectricityEnergy, "J", 0, 10)
	add(60, "SPACE2-1 BASEBOARD", "Baseboard Total Heating Energy", "J", 0, 200)
	add(61, "SPACE4-1 BASEBOARD", "Baseboard Total Heating Energy", "J", 0, 300)
	add(62, "SPACE2-1 BASEBOARD", "Baseboard Electricity Rate", "W", 0, 4000)
	add(63, "SPACE2-1 BASEBOARD", "Baseboard Total Heating Rate", "W", 0, 5000)
	add(64, "CENTRAL BOILER", energyPathBoilerAncillaryElectricityRate, "W", 0, 6000)
	for i := 1; i <= 5; i++ {
		add(100+i, fmt.Sprintf("SPACE%d-1", i), "Zone Air System Sensible Heating Energy", "J", 0, 100)
	}
	add(106, "PLENUM-1", "Zone Air System Sensible Heating Energy", "J", 0, 99)
	return path, inputPath, doc, plan, context
}

func TestEnergyPathHVACConsumptionSQLMixedHeatingSourceBoundaries(t *testing.T) {
	path, inputPath, _, plan, context := mixedHeatingConsumptionSQLFixture(t)
	if len(context.BaseboardTargets) != 2 || len(context.SharedHeatingElectricTargets) != 1 || len(context.SharedHeatingElectricTargets[0].RelatedPathIDs) != 5 {
		t.Fatalf("original typed local/shared roster unresolved: baseboard=%d shared=%+v", len(context.BaseboardTargets), context.SharedHeatingElectricTargets)
	}
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.hvacConsumptionPools) != 1 || len(legacy.hvacConsumptionPools[0].Members) != 3 {
		t.Fatalf("native three-constituent roster missing: %+v", legacy.hvacConsumptionPools)
	}
	legacy = enrichEnergyExplanationWithServicePaths(legacy, inputPath)
	result := UpgradeEnergyExplanationV1(legacy)
	for _, zone := range result.ZoneResults {
		if zone.Scope.ZoneName == "PLENUM-1" {
			for _, node := range zone.Nodes {
				if node.Level == "carrier" && node.Value > 0 {
					t.Errorf("unserved return plenum acquired consumption: %+v", node)
				}
			}
			continue
		}
		owner := 0
		fmt.Sscanf(zone.Scope.ZoneName, "SPACE%d-1", &owner)
		if owner < 1 || owner > 5 {
			t.Fatalf("equipment key became a fictitious Zone: %s", zone.Scope.ZoneName)
		}
		for _, period := range zone.Periods {
			if period.ID != "M1" && period.ID != "M2" {
				continue
			}
			factor := 1.0
			if period.ID == "M2" {
				factor = 2
			}
			wantElectricity := []float64{2, 12, 2, 32, 2}[owner-1] * factor
			assertMixedHeatingNode(t, period.Nodes, "carrier", "electricity", wantElectricity)
			assertMixedHeatingNode(t, period.Nodes, "carrier", "natural_gas", 20*factor)
			assertMixedHeatingNode(t, period.Nodes, "load", "heating", 100*factor)
		}
		assertMixedHeatingNode(t, zone.Nodes, "carrier", "electricity", []float64{6, 36, 6, 96, 6}[owner-1])
		assertMixedHeatingNode(t, zone.Nodes, "load", "heating", 300)
	}
	assertMixedHeatingNode(t, result.Nodes, "carrier", "electricity", 150)
	assertMixedHeatingNode(t, result.Nodes, "end_use", "heating", 450)
	for _, source := range result.Sources {
		if source.ID == "sql-rdd-52" {
			if !energyDataSourceValueKnown(source, energySourceObservedRaw) || source.RawValue != 30 {
				t.Errorf("boiler observed budget changed: %+v", source)
			}
			seen := 0
			for _, detail := range source.ScopeDetails {
				if detail.Scope.Kind == "zone" && strings.HasPrefix(detail.Scope.ZoneName, "SPACE") {
					seen++
					if detail.inspectorValuePresence&3 != 0 || !detail.inspectorScopedValuePresence || detail.AllocatedValue != 6 || !detail.AllocationApplied {
						t.Errorf("constituent raw/allocation boundary lost: %+v", detail)
					}
				}
			}
			if seen != 5 {
				t.Errorf("boiler source lacks five exact scoped allocations: %d", seen)
			}
		}
		if source.ID == "sql-rdd-60" || source.ID == "sql-rdd-61" || source.ID == "sql-rdd-62" || source.ID == "sql-rdd-63" || source.ID == "sql-rdd-64" {
			if source.DriverRole != energyDriverSourceRoleContext {
				t.Errorf("native component/rate context became additive: %+v", source)
			}
		}
	}
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var reopened EnergyExplanationResult
	if err := json.Unmarshal(wire, &reopened); err != nil {
		t.Fatal(err)
	}
	assertMixedHeatingNode(t, reopened.Nodes, "carrier", "electricity", 150)
	for _, zone := range reopened.ZoneResults {
		if zone.Scope.ZoneName == "SPACE2-1" {
			assertMixedHeatingNode(t, zone.Nodes, "carrier", "electricity", 36)
		}
	}
}

func assertMixedHeatingNode(t *testing.T, nodes []EnergyExplanationNode, level, category string, want float64) {
	t.Helper()
	count := 0
	for _, node := range nodes {
		value := node.EndUse
		if level == "carrier" {
			value = node.Carrier
		}
		if level == "load" {
			value = node.ServiceKind
		}
		if node.Level == level && value == category {
			count++
			if math.Abs(node.Value-want) > 1e-9 {
				t.Errorf("%s/%s=%g want %g: %+v", level, category, node.Value, want, node)
			}
		}
	}
	if count != 1 {
		t.Errorf("%s/%s node count=%d want1", level, category, count)
	}
}

func TestEnergyPathHVACConsumptionSQLMissingIsNotMeasuredZero(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		known       bool
		valid       bool
		value       float64
	}{
		{"reported zero", `UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex=52`, true, true, 0},
		{"NULL", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=52`, false, true, 0},
		{"missing", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=52`, false, true, 0},
		{"negative", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=52`, false, true, 0},
		{"wrong unit", `UPDATE ReportDataDictionary SET Units='kJ' WHERE ReportDataDictionaryIndex=52`, false, true, 0},
		{"duplicate identity", `INSERT INTO ReportDataDictionary SELECT 152,KeyValue,Name,Units,IsMeter,ReportingFrequency,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=52`, false, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, _, _, plan, context := mixedHeatingConsumptionSQLFixture(t)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.query); err != nil {
				db.Close()
				t.Fatal(err)
			}
			db.Close()
			parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, context)
			if err != nil {
				t.Fatal(err)
			}
			if len(parsed.HVACConsumptionPools) != 1 {
				t.Fatalf("missing pool gate: %+v", parsed.HVACConsumptionPools)
			}
			pool := parsed.HVACConsumptionPools[0]
			if pool.Valid != tc.valid {
				t.Errorf("pool validity=%v want%v", pool.Valid, tc.valid)
			}
			for _, member := range pool.Members {
				if !strings.EqualFold(member.ObjectName, "Central Boiler") {
					continue
				}
				value, _, _, known := energyPathHVACConsumptionPeriodValue(member.Series, "M1", "monthly", true)
				if known != tc.known || known && value != tc.value {
					t.Errorf("observed member=%g/%v want%g/%v", value, known, tc.value, tc.known)
				}
			}
			if tc.name != "duplicate identity" {
				for _, source := range parsed.Sources {
					if source.ID == "sql-rdd-52" && energyDataSourceValueKnown(source, energySourceObservedRaw) != tc.known {
						t.Errorf("raw source presence fabricated: %+v", source)
					}
				}
			}
		})
	}
}

func TestEnergyPathBaseboardSQLContextCannotReplaceMissingZoneLoad(t *testing.T) {
	path, _, _, plan, context := mixedHeatingConsumptionSQLFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM ReportData WHERE ReportDataDictionaryIndex BETWEEN 101 AND 106`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(legacy)
	for _, node := range result.Nodes {
		if node.Level == "load" {
			t.Errorf("component context fabricated an aggregate Zone load: %+v", node)
		}
	}
	for _, zone := range result.ZoneResults {
		for _, node := range zone.Nodes {
			if node.Level == "load" {
				t.Errorf("component context became a Zone fallback: %+v", node)
			}
		}
	}
	seen := false
	for _, source := range result.Sources {
		if source.ID == "sql-rdd-60" {
			seen = true
			if source.RawValue != 600 || source.EffectiveValue != 600 || source.DriverRole != energyDriverSourceRoleContext || source.ZoneName != "SPACE2-1" {
				t.Errorf("original non-additive context not retained: %+v", source)
			}
		}
	}
	if !seen {
		t.Error("native component context was silently dropped")
	}
}

func TestEnergyPathBaseboardSQLNativeModelTotalIsNotMultipliedAgain(t *testing.T) {
	path, _, doc, plan, _ := mixedHeatingConsumptionSQLFixture(t)
	for i := range doc.Objects {
		object := &doc.Objects[i]
		if strings.EqualFold(object.Type, "Zone") && strings.EqualFold(object.Fields[0].Value, "SPACE2-1") {
			// The original Zone field 6 is Multiplier; no equipment output
			// changes when the independently supplied aggregate Zone load scales.
			object.Fields[6].Value = "6"
		}
	}
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(legacy)
	seen := map[string]bool{}
	for _, source := range result.Sources {
		if source.ID != "sql-rdd-50" && source.ID != "sql-rdd-60" {
			continue
		}
		seen[source.ID] = true
		want := 30.0
		if source.ID == "sql-rdd-60" {
			want = 600
		}
		if source.RawValue != want || source.EffectiveValue != want || source.EffectiveMultiplier != 1 || source.MultiplierApplication != energyMultiplierAlreadyModelTotal {
			t.Errorf("native model-total multiplied again: %+v", source)
		}
	}
	if !seen["sql-rdd-50"] || !seen["sql-rdd-60"] {
		t.Errorf("missing native consumption/context sources: %v", seen)
	}
	for _, zone := range result.ZoneResults {
		if zone.Scope.ZoneName == "SPACE2-1" {
			assertMixedHeatingNode(t, zone.Nodes, "load", "heating", 1800)
		}
	}
}

func TestEnergyPathHVACConsumptionSQLDirectOnlyHasNoMeasuredSharedZoneValues(t *testing.T) {
	path, inputPath, _, plan, context := mixedHeatingConsumptionSQLFixture(t)
	plan.AllocationPolicy = PurposeAllocationPolicyDirectOnly
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(enrichEnergyExplanationWithServicePaths(legacy, inputPath))
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var reopened EnergyExplanationResult
	if err := json.Unmarshal(wire, &reopened); err != nil {
		t.Fatal(err)
	}
	for _, view := range []EnergyExplanationResult{result, reopened} {
		seen := 0
		for _, source := range view.Sources {
			if source.ID != "sql-rdd-52" && source.ID != "sql-rdd-64" {
				continue
			}
			seen++
			if source.AllocationApplied || source.AllocatedValue != 0 {
				t.Errorf("DirectOnly allocated a shared component: %+v", source)
			}
			for _, detail := range source.ScopeDetails {
				if detail.Scope.Kind == "zone" {
					t.Errorf("native whole-component value became a Zone measurement: %+v", detail)
				}
			}
			if source.ID == "sql-rdd-52" && (!energyDataSourceValueKnown(source, energySourceObservedRaw) || source.RawValue != 30) {
				t.Errorf("native whole-component observation lost: %+v", source)
			}
		}
		if seen != 2 {
			t.Errorf("lost shared native source/context: %d", seen)
		}
	}
}

func TestEnergyPathHVACConsumptionSQLPartialMonthRetainsOnlyActualObservations(t *testing.T) {
	for _, query := range []string{
		`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=52 AND TimeIndex=2`,
		`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(2,52,72000000)`,
		`UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=52 AND TimeIndex=2`,
	} {
		t.Run(query, func(t *testing.T) {
			path, _, _, plan, context := mixedHeatingConsumptionSQLFixture(t)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(query); err != nil {
				db.Close()
				t.Fatal(err)
			}
			db.Close()
			parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, context)
			if err != nil {
				t.Fatal(err)
			}
			for _, member := range parsed.HVACConsumptionPools[0].Members {
				if !strings.EqualFold(member.ObjectName, "Central Boiler") {
					continue
				}
				m1, _, _, known1 := energyPathHVACConsumptionPeriodValue(member.Series, "M1", "monthly", true)
				_, _, _, known2 := energyPathHVACConsumptionPeriodValue(member.Series, "M2", "monthly", true)
				if !known1 || m1 != 10 || known2 {
					t.Errorf("partial month state changed: %+v", member.Series)
				}
			}
			for _, source := range parsed.Sources {
				if source.ID == "sql-rdd-52" && energyDataSourceValueKnown(source, energySourceObservedRaw) {
					t.Errorf("partial sum became annual native total: %+v", source)
				}
			}
		})
	}
}

func TestEnergyPathHVACConsumptionHourlyInvalidAndOverflowScalarsStayUnknown(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		hours int
		chart bool
	}{
		{"native zero", 0.0, 2, true}, {"negative", -1.0, 2, false}, {"NULL", nil, 2, false}, {"finite hourly values but overflowing sum", 1e308, 1800, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, _, _, plan, _ := mixedHeatingConsumptionSQLFixture(t)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(910,'CENTRAL BOILER','Boiler Ancillary Electricity Rate','W',0,'Hourly','HVAC')`); err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < tc.hours; i++ {
				stamp := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour)
				if _, err := tx.Exec(`INSERT INTO "Time" (TimeIndex,Month,Day,Hour,Minute,Year,"Interval",IntervalType,EnvironmentPeriodIndex,WarmupFlag) VALUES(?,?,?,?,0,2017,60,1,3,0)`, i+3, int(stamp.Month()), stamp.Day(), stamp.Hour()+1); err != nil {
					tx.Rollback()
					t.Fatal(err)
				}
				if _, err := tx.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,910,?)`, i+3, tc.value); err != nil {
					tx.Rollback()
					t.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			dictionaries, err := energyPathBoilerConsumptionDictionaries(db, "eplusout.sql", "CENTRAL BOILER")
			if err != nil {
				t.Fatal(err)
			}
			for _, dictionary := range dictionaries {
				if dictionary.row.index != 910 {
					continue
				}
				source, _ := energyPathReadBoilerConsumptionObservation(db, dictionary, &plan, energyPathHVACConsumptionMonthlyAxis(db), newEnergySourceHourlyCollector(db))
				if (source.HourlyEnergy != nil) != tc.chart {
					t.Errorf("chart knownness=%v want%v", source.HourlyEnergy != nil, tc.chart)
				}
				if energyDataSourceValueKnown(source, energySourceObservedRaw) != (tc.name == "native zero") {
					t.Errorf("invalid aggregate became known scalar: %+v", source)
				}
				if _, err := json.Marshal(source); err != nil {
					t.Errorf("invalid scalar escaped to JSON: %v", err)
				}
			}
		})
	}
}
