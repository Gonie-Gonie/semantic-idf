package simulation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

func epathSQLTemporalTraceUnitFixture(t *testing.T) (string, epathRealSQLModel) {
	t.Helper()
	path, model := epathSQLModelUnitFixture(t)
	epathOracleEditSQL(t, path, `INSERT INTO ReportDataDictionary VALUES
(80,'Zone Air Heat Balance Surface Convection Rate','Office',0,'Hourly','W','System'),
(81,'Zone Air Heat Balance Internal Convective Heat Gain Rate','Office',0,'Hourly','W','System'),
(82,'Zone Air Heat Balance Surface Convection Rate','Office',0,'Monthly','W','System'),
(83,'Zone Total Internal Convective Heating Energy','Office',0,'Monthly','J','Zone');
WITH RECURSIVE hours(n) AS(SELECT 0 UNION ALL SELECT n+1 FROM hours WHERE n<8759)
INSERT INTO Time SELECT 10000+n,1,2017,CAST(strftime('%m',date('2017-01-01','+'||(n/24)||' days')) AS INTEGER),CAST(strftime('%d',date('2017-01-01','+'||(n/24)||' days')) AS INTEGER),n%24+1,0,60,NULL,1,n/24+1 FROM hours;
INSERT INTO ReportData SELECT 100000+TimeIndex*2,80,TimeIndex,CASE WHEN Month=1 THEN 0 WHEN Month%2=0 THEN 100.49 ELSE -50.49 END FROM Time WHERE IntervalType=1;
INSERT INTO ReportData SELECT 100001+TimeIndex*2,81,TimeIndex,CASE WHEN Month=1 THEN 0 ELSE 200.49 END FROM Time WHERE IntervalType=1;
INSERT INTO ReportData SELECT 500000+TimeIndex*2,82,TimeIndex,CASE WHEN Month=1 THEN 0 WHEN Month%2=0 THEN 100.49 ELSE -50.49 END FROM Time WHERE IntervalType=3;
INSERT INTO ReportData SELECT 500001+TimeIndex*2,83,TimeIndex,(CASE WHEN Month=1 THEN 0 ELSE 200.49 END)*"Interval"*60 FROM Time WHERE IntervalType=3;`)
	for _, kind := range []string{"surface", "internal"} {
		id, category, original, unit, alias := "surface.balance", "balance.storage_other", "Zone Air Heat Balance Surface Convection Rate", "W", "Zone Air Heat Balance Surface Convection Rate"
		subtract := []string{"surface.total"}
		if kind == "internal" {
			id, category, original, unit, alias = "internal.other.sensible", "internal.other", "Zone Total Internal Convective Heating Energy", "J", "Zone Air Heat Balance Internal Convective Heat Gain Rate"
			subtract = []string{"people"}
		}
		model.Families = append(model.Families, epathRealSQLFamily{ID: id, Category: category, Component: "sensible", Role: "pressure", BuildingVisible: true, Keys: []string{"Office"}, Terms: []epathRealSQLTerm{{Sign: 1, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: original, Unit: unit}}}}}, Subtract: subtract, TraceSources: []epathRealSQLTraceSource{{Frequency: "Hourly", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: alias, Unit: "W"}}, Keys: []string{"Office"}}}}})
	}
	return path, model
}

func epathSQLTemporalTraceUnitRead(t *testing.T) (epathSQLFrames, epathRealSQLModel, epathRealOracleEvidence) {
	t.Helper()
	path, model := epathSQLTemporalTraceUnitFixture(t)
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	return frames, model, observed
}

func epathSQLTemporalTraceUnitSource(proof epathSQLTraceSourceIdentity) EnergyDataSource {
	category, component, direction := "balance.storage_other", "surface.reconciliation", "signed"
	if proof.Family == "internal.other.sensible" {
		category, component, direction = "internal.other", "internal.reconciliation.convective", "gain"
	}
	// Each month is a constant original rate in this hand fixture. Apply only
	// ordinary decimal3 rounding to each independently known hourly energy.
	value := 0.0
	for _, month := range proof.Source.Months {
		value += math.Round(*month.EnergyKWh/float64(month.Rows)*1000) / 1000 * float64(month.Rows)
	}
	value = math.Round(value*1000) / 1000
	return EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", proof.Source.DictionaryIndex), SourceType: "sql_report_data", Name: proof.Source.Name, KeyValue: proof.Source.KeyValue, Units: "W", SourceUnit: "W", NormalizedUnit: "kWh", ReportingFrequency: "Hourly", AggregationMethod: "integrate_rate_by_time_interval", AggregationBasis: "model_total", ZoneName: proof.ZoneName, DriverRole: "reconciliation", DriverCategory: category, DriverComponent: component, HeatDirection: direction, InspectorSection: "Context", MultiplierApplication: "requires_zone_multiplier", EffectiveMultiplier: proof.Multiplier, RawValue: value, EffectiveValue: math.Round(value*proof.Multiplier*1000) / 1000, inspectorDecodedFromJSON: true, inspectorValuePresence: 3}
}

func TestEnergyPathRealSQLTemporalTraceIndependentCellsAndPrecision(t *testing.T) {
	path, model := epathSQLTemporalTraceUnitFixture(t)
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	without := model
	without.Families = append([]epathRealSQLFamily(nil), model.Families...)
	for i := range without.Families {
		without.Families[i].TraceSources = nil
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, without)
	if err != nil {
		t.Fatal(err)
	}
	// Snapshot one completed numeric frame before and after the trace-only
	// phase. Recompiling unrelated map-ordered floating sums would not test
	// this helper's immutability contract.
	snapshot := func() []byte {
		data, err := json.Marshal(struct {
			Cells         map[string]*epathSQLCell
			Loads         map[string]epathSQLQuantity
			LoadSourceIDs map[string][]int
		}{frames.Cells, frames.Loads, frames.LoadSourceIDs})
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	before := snapshot()
	if err := epathCompileSQLTemporalTraceSources(path, observed.Sources, model, &frames); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, snapshot()) {
		t.Fatal("trace alias became numeric authority, changed budget, or selected source IDs")
	}
	if len(frames.TraceSourceIdentities) != 2 || len(frames.CellTraceSourceIDs) != 24 {
		t.Fatal("missing exact per-family original temporal coverage")
	}
	for id, proof := range frames.TraceSourceIdentities {
		if proof.NonzeroRows[0] != 0 || frames.SourceRaw[id][0].Value != 0 || frames.SourceRaw[id][0].Error != 0 {
			t.Fatal("actual zero acquired an interval or unknown status")
		}
		if proof.NonzeroRows[1] != 28*24 || frames.SourceRaw[id][1].Error != 28*24*.0005 {
			t.Fatal("source error not bounded by actual nonzero original hourly rows")
		}
		if frames.SourceRaw[id][1].Value != *proof.Source.Months[1].EnergyKWh {
			t.Fatal("source numeric center changed to a candidate/display value")
		}
		q, err := epathSQLTemporalTraceAnnualQuantity(proof, "rawValue")
		if err != nil {
			t.Fatal(err)
		}
		source := epathSQLTemporalTraceUnitSource(proof)
		if !epathSQLTemporalTraceSourceMatches(source, proof) || epathCheckSQLModelQuantity(&source.RawValue, &q) != nil {
			t.Fatal("valid exact hourly normalization not covered by its independently counted bound")
		}
		bad := q.Value + q.Error + .01
		if epathCheckSQLModelQuantity(&bad, &q) == nil {
			t.Fatal("hourly source count bound became unrestricted tolerance")
		}
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelSourceChecks(frames, observed.Sources, model, &checks); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, check := range checks.Rows {
		if check.TraceSource != nil {
			count++
			if !strings.Contains(check.Want.Key, "/Hourly/") {
				t.Fatal("new temporal source key collides with existing Monthly identity")
			}
		}
	}
	if count != 8 {
		t.Fatalf("expected two observed Hourly sources x two fields x two scopes: %d", count)
	}
}

func TestEnergyPathRealSQLTemporalTraceRejectsFalseEquivalenceAndCoverage(t *testing.T) {
	for _, test := range []struct{ name, query string }{
		{"wrong monthly physical total", `UPDATE ReportData SET Value=Value+100 WHERE ReportDataDictionaryIndex=82 AND TimeIndex=2`},
		{"missing hour", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=80 AND TimeIndex=10000`},
		{"duplicate hour", `INSERT INTO ReportData SELECT 900000,80,10000,Value FROM ReportData WHERE ReportDataDictionaryIndex=80 AND TimeIndex=10000`},
		{"null hour", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=80 AND TimeIndex=10000`},
		{"partial interval", `UPDATE Time SET "Interval"=30 WHERE TimeIndex=10000`},
		{"warmup", `UPDATE Time SET WarmupFlag=1 WHERE TimeIndex=10000`},
		{"design environment", `UPDATE Time SET EnvironmentPeriodIndex=2 WHERE TimeIndex=10000`},
		{"duplicate dictionary", `INSERT INTO ReportDataDictionary SELECT 180,Name,KeyValue,IsMeter,ReportingFrequency,Units,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=80`},
		{"case-only duplicate dictionary", `INSERT INTO ReportDataDictionary SELECT 180,lower(Name),lower(KeyValue),IsMeter,lower(ReportingFrequency),lower(Units),IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=80`},
		{"duplicate Monthly authority dictionary", `INSERT INTO ReportDataDictionary SELECT 182,Name,KeyValue,IsMeter,ReportingFrequency,Units,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=82`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, model := epathSQLTemporalTraceUnitFixture(t)
			epathOracleEditSQL(t, path, test.query)
			observed, err := epathReadRealSQLOracle(path)
			if err != nil {
				return
			}
			if _, err := epathCompileSQLModelFrames(path, observed.Sources, model); err == nil {
				t.Fatal("unobserved/non-equivalent temporal alias accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLTemporalTraceRejectsUnreviewedDeclaration(t *testing.T) {
	for _, mutation := range []string{"different physics", "wildcard", "unknown owner", "duplicate owner", "wrong frequency", "alternate list", "optional missing", "wrong canonical unit"} {
		t.Run(mutation, func(t *testing.T) {
			path, model := epathSQLTemporalTraceUnitFixture(t)
			f := &model.Families[1]
			trace := &f.TraceSources[0]
			switch mutation {
			case "different physics":
				trace.Source.Alternatives[0].Name = "Zone Air Heat Balance Internal Convective Heat Gain Rate"
			case "wildcard":
				trace.Source.Keys = []string{"*"}
			case "unknown owner":
				trace.Source.Keys = []string{"Plenum"}
			case "duplicate owner":
				trace.Source.Keys = append(trace.Source.Keys, "office")
			case "wrong frequency":
				trace.Frequency = "Monthly"
			case "alternate list":
				trace.Source.Alternatives = append(trace.Source.Alternatives, trace.Source.Alternatives[0])
			case "optional missing":
				trace.Source.AllowAbsent = true
			case "wrong canonical unit":
				f.Terms[0].Source.Alternatives[0].Unit = "J"
			}
			observed, err := epathReadRealSQLOracle(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := epathCompileSQLModelFrames(path, observed.Sources, model); err == nil {
				t.Fatal("undeclared physical/temporal equivalence accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLTemporalTraceStrictCandidateMetadata(t *testing.T) {
	frames, _, _ := epathSQLTemporalTraceUnitRead(t)
	proof := frames.TraceSourceIdentities[80]
	for _, mutation := range []string{"", "wrong Zone", "wrong family", "main flow", "Monthly disguise", "derived wrapper", "wrong unit", "wrong multiplier"} {
		source := epathSQLTemporalTraceUnitSource(proof)
		switch mutation {
		case "wrong Zone":
			source.ZoneName = "Plenum"
		case "wrong family":
			source.DriverCategory = "internal.other"
		case "main flow":
			source.DriverRole = "main_flow"
		case "Monthly disguise":
			source.ReportingFrequency = "Monthly"
		case "derived wrapper":
			source.InputSourceIDs = []string{"sql-rdd-82"}
		case "wrong unit":
			source.SourceUnit = "J"
		case "wrong multiplier":
			source.EffectiveMultiplier = 1
		}
		if epathSQLTemporalTraceSourceMatches(source, proof) != (mutation == "") {
			t.Fatalf("strict temporal source metadata %q", mutation)
		}
	}
}

func TestEnergyPathRealSQLTemporalTraceKnownZeroHasNoRoundingAllowance(t *testing.T) {
	path, model := epathSQLTemporalTraceUnitFixture(t)
	epathOracleEditSQL(t, path, `UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex IN (80,81,82,83)`)
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	for id, proof := range frames.TraceSourceIdentities {
		if proof.NonzeroRows != [12]int{} {
			t.Fatal("zero original observations acquired nonzero rounding counts")
		}
		for _, field := range []string{"rawValue", "effectiveValue"} {
			q, err := epathSQLTemporalTraceAnnualQuantity(proof, field)
			low, high := q.bounds()
			if err != nil || q.Value != 0 || q.Error != 0 || low != 0 || high != 0 {
				t.Fatalf("%d/%s known zero is not exact: %+v, %v", id, field, q, err)
			}
			nonzero := .0001
			if epathCheckSQLModelQuantity(&nonzero, &q) == nil {
				t.Fatal("known zero source was given a nonzero presentation allowance")
			}
		}
	}
}
