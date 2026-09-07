package simulation

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// The ordinary site meter has the same identity/units/frequency as actual
// Large Office dictionary 4105. No graph supplies the observation proof.
func epathRealZeroSourceSQL(t *testing.T, mode string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "observed-zero.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	queries := []string{
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentName TEXT,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(1,'ANNUAL WEATHER',3)`,
		`CREATE TABLE "Time"(TimeIndex INTEGER PRIMARY KEY,EnvironmentPeriodIndex INTEGER,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,"Interval" REAL,WarmupFlag INTEGER,IntervalType INTEGER,SimulationDays INTEGER)`,
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,Name TEXT,KeyValue TEXT,IsMeter INTEGER,ReportingFrequency TEXT,Units TEXT,IndexGroup TEXT)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,ReportDataDictionaryIndex INTEGER,TimeIndex INTEGER,Value REAL)`,
	}
	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if mode != "absent" {
		name, key, frequency, unit, isMeter := "Heating:Electricity", "", "Monthly", "J", 1
		if mode == "rate_without_interval" {
			name, key, frequency, unit, isMeter = "Zone Air System Sensible Heating Rate", "Office", "Timestep", "W", 0
		}
		if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(4105,?,?,?,?,?,'Facility')`, name, key, isMeter, frequency, unit); err != nil {
			t.Fatal(err)
		}
	}
	for month := 1; month <= 12; month++ {
		last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC)
		var interval any = last.Day() * 24 * 60
		intervalType := 3
		if mode == "rate_without_interval" {
			interval, intervalType = nil, 0
		}
		if _, err := db.Exec(`INSERT INTO "Time" VALUES(?,1,2017,?,?,24,0,?,NULL,?,?)`, month, month, last.Day(), interval, intervalType, last.YearDay()); err != nil {
			t.Fatal(err)
		}
		if mode == "absent" || mode == "no_rows" {
			continue
		}
		var value any = 0.0
		if mode == "all_null" || mode == "one_null" && month == 6 {
			value = nil
		}
		if _, err := db.Exec(`INSERT INTO ReportData VALUES(?,4105,?,?)`, month, month, value); err != nil {
			t.Fatal(err)
		}
	}
	if mode == "duplicate" {
		if _, err := db.Exec(`INSERT INTO ReportData VALUES(100,4105,1,0)`); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func epathRealSourceWireFields(t *testing.T, source EnergyDataSource) map[string]json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(energyPathSourceWire{source})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

func epathRealAssertSourceScalar(t *testing.T, fields map[string]json.RawMessage, name string, present bool) {
	t.Helper()
	value, exists := fields[name]
	if exists != present || present && string(value) != "0" {
		t.Fatalf("%s = %s (present %v), want known zero = %v", name, value, exists, present)
	}
}

func TestEnergyPathRealSourceObservedZeroSQLAndRepeatedV2Wire(t *testing.T) {
	path := epathRealZeroSourceSQL(t, "zero")
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, nil, energyDriverBuildContext{})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Sources) != 1 || len(parsed.Series) != 1 {
		t.Fatalf("actual zero rows disappeared: %+v", parsed)
	}
	if !energyDataSourceValueKnown(parsed.Sources[0], energySourceObservedRaw) || energyDataSourceValueKnown(parsed.Sources[0], energySourceObservedEffective) {
		t.Fatal("SQL read must prove raw only, before multiplier resolution")
	}
	if len(parsed.Series[0].Monthly) != 12 {
		t.Fatal("zero months were pruned from observed series")
	}
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, nil, energyDriverBuildContext{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(legacy)
	for pass := 0; pass < 3; pass++ {
		if len(result.Sources) != 1 {
			t.Fatalf("pass %d source missing", pass)
		}
		source := result.Sources[0]
		if source.ID != "sql-rdd-4105" || source.DriverRole != "" || source.SourceUnit != "J" || source.NormalizedUnit != "kWh" || source.EffectiveMultiplier != 1 {
			t.Fatalf("ordinary zero meter metadata changed: %+v", source)
		}
		fields := epathRealSourceWireFields(t, source)
		epathRealAssertSourceScalar(t, fields, "rawValue", true)
		epathRealAssertSourceScalar(t, fields, "effectiveValue", true)
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encoded, &result); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEnergyPathRealSourceUnknownSQLNeverAcquiresZeroProof(t *testing.T) {
	for _, mode := range []string{"absent", "no_rows", "all_null", "one_null", "duplicate", "rate_without_interval"} {
		t.Run(mode, func(t *testing.T) {
			parsed, err := parseSimulationEnergyExplanationCanonicalSQL(epathRealZeroSourceSQL(t, mode), nil, energyDriverBuildContext{})
			if err != nil {
				t.Fatal(err)
			}
			_, sources, _ := applyEnergyExplanationMultipliers(parsed.Series, parsed.Sources, energyEffectiveMultiplierIndex{})
			wantSource := mode == "one_null" || mode == "duplicate" || mode == "rate_without_interval"
			if (len(sources) == 1) != wantSource {
				t.Fatalf("%s source count = %d", mode, len(sources))
			}
			for _, source := range sources {
				if energyDataSourceValueKnown(source, energySourceObservedRaw) || energyDataSourceValueKnown(source, energySourceObservedEffective) {
					t.Fatalf("%s acquired false fully observed proof: %+v", mode, source)
				}
				fields := epathRealSourceWireFields(t, source)
				epathRealAssertSourceScalar(t, fields, "rawValue", false)
				epathRealAssertSourceScalar(t, fields, "effectiveValue", false)
			}
		})
	}
}

func TestEnergyPathRealSourceMonthlyProofUsesActualShortReportingAxis(t *testing.T) {
	for _, test := range []struct {
		name, edit string
		known      bool
	}{
		{"short_complete", `DELETE FROM ReportData WHERE TimeIndex>3; DELETE FROM "Time" WHERE TimeIndex>3`, true},
		{"short_missing_one", `DELETE FROM ReportData WHERE TimeIndex>3 OR TimeIndex=2; DELETE FROM "Time" WHERE TimeIndex>3`, false},
		{"annual_missing_one", `DELETE FROM ReportData WHERE TimeIndex=6`, false},
		{"unknown_environment", `DROP TABLE EnvironmentPeriods`, false},
		{"unknown_interval", `UPDATE "Time" SET "Interval"=NULL WHERE TimeIndex=6`, false},
		{"wrong_environment", `UPDATE EnvironmentPeriods SET EnvironmentType=1`, false},
		{"warmup_not_weather", `UPDATE "Time" SET WarmupFlag=1 WHERE TimeIndex=6`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := epathRealZeroSourceSQL(t, "zero")
			epathOracleEditSQL(t, path, test.edit)
			parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, nil, energyDriverBuildContext{})
			if err != nil {
				t.Fatal(err)
			}
			_, sources, _ := applyEnergyExplanationMultipliers(parsed.Series, parsed.Sources, energyEffectiveMultiplierIndex{})
			if len(sources) != 1 {
				t.Fatalf("source count = %d", len(sources))
			}
			for _, bit := range []uint8{energySourceObservedRaw, energySourceObservedEffective} {
				if energyDataSourceValueKnown(sources[0], bit) != test.known {
					t.Fatalf("bit %d known = %v, want %v", bit, energyDataSourceValueKnown(sources[0], bit), test.known)
				}
			}
			fields := epathRealSourceWireFields(t, sources[0])
			epathRealAssertSourceScalar(t, fields, "rawValue", test.known)
			epathRealAssertSourceScalar(t, fields, "effectiveValue", test.known)
		})
	}
}

func TestEnergyPathRealSourceScalarProofDoesNotLeakIntoOtherScalarOrScope(t *testing.T) {
	source := EnergyDataSource{ID: "observed", observedValuePresence: energySourceObservedRaw,
		ScopeDetails: []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}, EffectiveMultiplier: 1, MultiplierApplication: energyMultiplierAlreadyModelTotal}}}
	fields := epathRealSourceWireFields(t, source)
	epathRealAssertSourceScalar(t, fields, "rawValue", true)
	epathRealAssertSourceScalar(t, fields, "effectiveValue", false)
	var details []map[string]json.RawMessage
	if err := json.Unmarshal(fields["scopeDetails"], &details); err != nil {
		t.Fatal(err)
	}
	epathRealAssertSourceScalar(t, details[0], "rawValue", false)
	epathRealAssertSourceScalar(t, details[0], "effectiveValue", false)
	plain, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]json.RawMessage
	if err := json.Unmarshal(plain, &legacy); err != nil {
		t.Fatal(err)
	}
	epathRealAssertSourceScalar(t, legacy, "rawValue", false) // frozen v1 plain writer
	for _, raw := range []string{`{"id":"s"}`, `{"id":"s","rawValue":null,"effectiveValue":null}`, `{"id":"s","rawValue":0}`, `{"id":"s","effectiveValue":0}`} {
		var decoded EnergyDataSource
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			t.Fatal(err)
		}
		fields := epathRealSourceWireFields(t, decoded)
		epathRealAssertSourceScalar(t, fields, "rawValue", raw == `{"id":"s","rawValue":0}`)
		epathRealAssertSourceScalar(t, fields, "effectiveValue", raw == `{"id":"s","effectiveValue":0}`)
	}
}

func TestEnergyPathRealSourceKnownZeroNotReconstructedFromGraph(t *testing.T) {
	source := EnergyDataSource{ID: "reported-zero", observedValuePresence: energySourceObservedRaw | energySourceObservedEffective}
	node := EnergyExplanationNode{ID: "unrelated-allocation", Level: "energy", Value: 20, RawValue: 20, EffectiveValue: 20, SourceIDs: []string{source.ID}}
	sources := filterEnergyDataSourcesForV2([]EnergyDataSource{source}, []EnergyExplanationNode{node}, nil, nil, nil, nil, EnergyExplanationScope{Kind: "building"}, "")
	if len(sources) != 1 || sources[0].RawValue != 0 || sources[0].EffectiveValue != 0 {
		t.Fatalf("known source zero reconstructed from graph: %+v", sources)
	}
}

func TestEnergyPathRealSourceScopedZeroRequiresOwnMatchingZoneProof(t *testing.T) {
	for _, test := range []struct {
		name, zone string
		proof      uint8
		decoded    bool
		want       uint8
	}{
		{"own_both", "Office", 3, false, 3},
		{"own_raw_only", "Office", 1, false, 1},
		{"own_effective_only", " office ", 2, false, 2},
		{"own_stored_zero", "Office", 3, true, 3},
		{"other_zone", "Elsewhere", 3, false, 0},
		{"building_pool", "", 3, false, 0},
		{"scope_unknown", "Office", 0, false, 0},
		{"stored_scope_unknown", "Office", 0, true, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Parent is intentionally known. Its proof must never repair the
			// absent or differently owned scoped source's scalar presence.
			parent := []EnergyDataSource{{ID: "context", observedValuePresence: 3,
				DriverRole: energyDriverSourceRoleMainFlow, DriverCategory: energyDriverCategoryExteriorWalls,
				MultiplierApplication: energyMultiplierRequiresZone, EffectiveMultiplier: 1}}
			scoped := EnergyDataSource{ID: "context", ZoneName: test.zone, DriverRole: energyDriverSourceRoleContext, EffectiveMultiplier: 1, MultiplierApplication: energyMultiplierRequiresZone}
			if test.decoded {
				scoped.inspectorDecodedFromJSON = true
				scoped.inspectorValuePresence = test.proof
			} else {
				scoped.observedValuePresence = test.proof
			}
			appendEnergyDataSourceScopeDetails(parent, []EnergyDataSource{scoped}, EnergyExplanationScope{Kind: "zone", ZoneName: "Office"})
			for pass := 0; pass < 3; pass++ {
				fields := epathRealSourceWireFields(t, parent[0])
				var details []map[string]json.RawMessage
				if err := json.Unmarshal(fields["scopeDetails"], &details); err != nil || len(details) != 1 {
					t.Fatalf("scope details: %v", err)
				}
				epathRealAssertSourceScalar(t, details[0], "rawValue", test.want&1 != 0)
				epathRealAssertSourceScalar(t, details[0], "effectiveValue", test.want&2 != 0)
				encoded, err := json.Marshal(energyPathSourceWire{parent[0]})
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(encoded, &parent[0]); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestEnergyPathRealSourceScopedContextZeroFromActualMonthlySQL(t *testing.T) {
	path := epathRealZeroSourceSQL(t, "zero")
	epathOracleEditSQL(t, path, `UPDATE ReportDataDictionary SET Name='Zone Air Heat Balance Interzone Air Transfer Rate',KeyValue='Office',IsMeter=0,Units='W',IndexGroup='Zone' WHERE ReportDataDictionaryIndex=4105;
		INSERT INTO ReportDataDictionary VALUES(4106,'Zone Air System Sensible Cooling Energy','Office',0,'Monthly','J','Zone');
		INSERT INTO ReportData SELECT TimeIndex+100,4106,TimeIndex,3600000 FROM "Time"`)
	context := energyDriverBuildContext{Enabled: true, Multipliers: energyEffectiveMultiplierIndex{Enabled: true, Zones: map[string]energyZoneMultiplierRecord{"office": {ZoneName: "Office", ZoneMultiplier: 10, GroupMultiplier: 1}}}}
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, nil, context)
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(legacy)
	var source *EnergyDataSource
	for index := range result.Sources {
		if result.Sources[index].ID == "sql-rdd-4105" {
			source = &result.Sources[index]
		}
	}
	if source == nil || source.ZoneName != "Office" || source.DriverRole != energyDriverSourceRoleReconciliation || source.InspectorSection != energyDriverInspectorSectionContext {
		t.Fatalf("actual scoped context source absent: %+v", source)
	}
	fields := epathRealSourceWireFields(t, *source)
	var details []map[string]json.RawMessage
	if err := json.Unmarshal(fields["scopeDetails"], &details); err != nil || len(details) != 1 {
		t.Fatalf("context detail: %v, %s", err, fields["scopeDetails"])
	}
	epathRealAssertSourceScalar(t, details[0], "rawValue", true)
	epathRealAssertSourceScalar(t, details[0], "effectiveValue", true)
}
