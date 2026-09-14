package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Only the SQLite calendar fixture is shared. Neither the native Monthly
// values nor the source identity/owner proof use a production classifier.
func epathSQLHourlyCompanionUnitFixture(t *testing.T, unit string, values func(int) float64) (string, epathRealSQLModel, []epathRealSQLSource, epathSQLFrames) {
	t.Helper()
	fixtureName, name, key := "Baseboard Total Heating Energy", "Surface Inside Face Convection Heat Gain Energy", "Floor"
	if unit == "W" {
		fixtureName, name, key = "Baseboard Total Heating Rate", "Zone Air Heat Balance Air Energy Storage Rate", "Office"
	}
	path, _, _, _ := epathSQLBaseboardContextUnit(t, fixtureName, "Hourly", values)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{
		`CREATE TABLE Zones(ZoneIndex INTEGER,ZoneName TEXT,Multiplier REAL,ListMultiplier REAL)`,
		`INSERT INTO Zones VALUES(1,'Office',7,1)`,
		`CREATE TABLE Surfaces(SurfaceIndex INTEGER,SurfaceName TEXT,ZoneIndex INTEGER,HeatTransferSurf INTEGER)`,
		`INSERT INTO Surfaces VALUES(1,'Floor',1,1)`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE ReportDataDictionary SET Name=?,KeyValue=? WHERE ReportDataDictionaryIndex=40`, name, key); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(41,?,?,?,0,'Monthly','Zone')`, key, name, unit); err != nil {
		t.Fatal(err)
	}
	var sums [12]float64
	rows, err := db.Query(`SELECT t.Month,SUM(r.Value) FROM ReportData r JOIN Time t USING(TimeIndex) WHERE r.ReportDataDictionaryIndex=40 GROUP BY t.Month ORDER BY t.Month`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var month int
		var value float64
		if err := rows.Scan(&month, &value); err != nil {
			t.Fatal(err)
		}
		sums[month-1] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	for month := 1; month <= 12; month++ {
		stamp := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC)
		value := sums[month-1]
		if unit == "W" {
			value /= float64(stamp.Day() * 24)
		}
		if _, err := db.Exec(`INSERT INTO Time VALUES(?,?,?,24,0,2017,?,3,3,0,?)`, 8760+month, month, stamp.Day(), stamp.Day()*1440, stamp.YearDay()); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,41,?)`, 8760+month, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{"office": {Name: "Office", Multiplier: 7}}, SourceIdentities: map[int]epathRealSQLSource{}, SourceZone: map[int]string{41: "office"}, Cells: map[string]*epathSQLCell{}, SourceRaw: map[int][]epathSQLQuantity{}, SourceEffective: map[int][]epathSQLQuantity{}}
	for _, source := range observed.Sources {
		if source.DictionaryIndex == 41 {
			frames.SourceIdentities[41] = source
		}
	}
	for month := 1; month <= 12; month++ {
		frames.Cells[epathSQLKey("office", "surface.balance", month)] = &epathSQLCell{Zone: "Office", Family: "surface.balance", Category: "balance.storage_other", Month: month, SourceIDs: []int{41}, Raw: epathSQLQuantity{Value: 3}, Effective: epathSQLQuantity{Value: 21}}
	}
	model := epathRealSQLModel{HourlyCompanions: []epathRealSQLHourlyCompanion{{Name: name, Unit: unit, Keys: []string{key}}}}
	return path, model, observed.Sources, frames
}

func epathSQLHourlyCompanionUnitValues(index int) float64 {
	values := []float64{.00049, .00051, -.00049, -.00051, 0, 1.23456}
	if index < len(values) {
		return values[index]
	}
	return 0
}

func epathSQLHourlyCompanionUnitSource(p epathSQLHourlyCompanionIdentity) EnergyDataSource {
	method := "sum_report_data"
	if p.Source.SourceUnit == "W" {
		method = "integrate_rate_by_time_interval"
	}
	return EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", p.Source.DictionaryIndex), SourceType: "sql_report_data", Name: p.Source.Name, KeyValue: p.Source.KeyValue, ZoneName: "Office", Units: p.Source.SourceUnit, SourceUnit: p.Source.SourceUnit, NormalizedUnit: "kWh", ReportingFrequency: "Hourly", AggregationMethod: method, AggregationBasis: "model_total", MultiplierApplication: "requires_zone_multiplier", EffectiveMultiplier: 7, RawValue: p.ReportedScalar, EffectiveValue: math.Round(p.ReportedScalar*7*1000) / 1000, inspectorDecodedFromJSON: true, inspectorValuePresence: 3, HourlyEnergy: &EnergySourceHourlyEnergy{Unit: "kWh", Basis: "reported_source", Values: append([]float64(nil), p.HourlyValues...)}}
}

func TestEnergyPathRealSQLHourlyCompanionNativeSignsZerosAndCellOnlyBinding(t *testing.T) {
	for _, unit := range []string{"J", "W"} {
		t.Run(unit, func(t *testing.T) {
			path, model, observed, frames := epathSQLHourlyCompanionUnitFixture(t, unit, epathSQLHourlyCompanionUnitValues)
			before := map[string]epathSQLCell{}
			for key, cell := range frames.Cells {
				before[key] = *cell
			}
			if err := epathSQLBindHourlyCompanions(path, observed, model, &frames); err != nil {
				t.Fatal(err)
			}
			p := frames.TraceSourceIdentities[40].NativeCompanion
			if p == nil {
				t.Fatal("native companion identity missing")
			}
			if p.ReportedScalar != 1.235 || math.Abs(*p.Source.EnergyKWh-1.23456) > 1e-12 || !reflect.DeepEqual(p.HourlyValues[:6], []float64{0, .001, 0, -.001, 0, 1.235}) || p.HourlyLabels[0] != "01-01 01:00" || p.HourlyLabels[8759] != "12-31 24:00" {
				t.Fatalf("lost independently known signed native/quantized transport: scalar=%g native=%g first=%v", p.ReportedScalar, *p.Source.EnergyKWh, p.HourlyValues[:6])
			}
			if len(frames.SourceIdentities) != 2 || len(frames.TraceSourceIdentities) != 1 || len(frames.CellTraceSourceIDs) != 12 || len(frames.SourceRaw) != 0 || len(frames.SourceEffective) != 0 || len(frames.SourceZone) != 1 || len(frames.Loads) != 0 {
				t.Fatal("companion became an additive quantity/Zone authority or lost exact cell coverage")
			}
			for key, cell := range frames.Cells {
				if !reflect.DeepEqual(*cell, before[key]) || !reflect.DeepEqual(frames.CellTraceSourceIDs[key], []int{40}) {
					t.Fatal("original Monthly cell/denominator changed")
				}
			}
			source := epathSQLHourlyCompanionUnitSource(*p)
			if source.EffectiveValue != 8.645 || !epathSQLTemporalTraceSourceMatches(source, frames.TraceSourceIdentities[40]) {
				t.Fatal("exact source failed signed once-only multiplier proof")
			}
			if _, err := epathSQLTemporalTraceAnnualQuantity(frames.TraceSourceIdentities[40], "rawValue"); err == nil {
				t.Fatal("cell-local trace became a numeric annual frame")
			}
			for _, tc := range []struct {
				name   string
				mutate func(*EnergyDataSource)
			}{
				{"wrong native key", func(s *EnergyDataSource) { s.KeyValue = "Other" }},
				{"wrong Zone", func(s *EnergyDataSource) { s.ZoneName = "Other" }},
				{"wrong frequency", func(s *EnergyDataSource) { s.ReportingFrequency = "Monthly" }},
				{"wrong unit", func(s *EnergyDataSource) { s.SourceUnit = "kWh" }},
				{"multiplier twice", func(s *EnergyDataSource) { s.EffectiveValue *= 7 }},
				{"raw multiplied", func(s *EnergyDataSource) { s.RawValue *= 7 }},
				{"unknown scalar", func(s *EnergyDataSource) { s.inspectorValuePresence = 1 }},
				{"unscaled basis", func(s *EnergyDataSource) { s.MultiplierApplication = "already_model_total" }},
				{"derived disguised as original", func(s *EnergyDataSource) { s.InputSourceIDs = []string{"sql-rdd-41"} }},
				{"chart rounded zero invented", func(s *EnergyDataSource) { s.HourlyEnergy.Values[1] = 0 }},
				{"chart native zero invented", func(s *EnergyDataSource) { s.HourlyEnergy.Values[4] = .001 }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					bad := epathSQLHourlyCompanionUnitSource(*p)
					tc.mutate(&bad)
					if epathSQLTemporalTraceSourceMatches(bad, frames.TraceSourceIdentities[40]) {
						t.Fatal("accepted altered original source")
					}
				})
			}
		})
	}
	path, model, observed, frames := epathSQLHourlyCompanionUnitFixture(t, "J", func(int) float64 { return 0 })
	proofs, _, err := epathCompileSQLHourlyCompanions(path, observed, model, frames)
	if err != nil {
		t.Fatal(err)
	}
	p := proofs[40]
	source := epathSQLHourlyCompanionUnitSource(p)
	if source.RawValue != 0 || source.EffectiveValue != 0 || epathSQLMatchHourlyCompanionSource(source, p) != nil {
		t.Fatal("native known zero disappeared")
	}
	source.inspectorValuePresence = 0
	if epathSQLMatchHourlyCompanionSource(source, p) == nil {
		t.Fatal("native known zero became missing")
	}
	if epathSQLHourlyCompanionNativeNear(1, 1.00001, 1, 744) {
		t.Fatal("presentation tolerance contaminated native H/M equivalence")
	}
}

func TestEnergyPathRealSQLHourlyCompanionRejectsOriginalIdentityCalendarAndOwnershipMutations(t *testing.T) {
	path, model, observed, frames := epathSQLHourlyCompanionUnitFixture(t, "J", epathSQLHourlyCompanionUnitValues)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, query string }{
		{"missing hour", `DELETE FROM ReportData WHERE TimeIndex=1`},
		{"NULL hour", `UPDATE ReportData SET Value=NULL WHERE TimeIndex=1`},
		{"duplicate hour", `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(1,40,1764)`},
		{"dictionary-only duplicate", `INSERT INTO ReportDataDictionary VALUES(99,'Floor','Surface Inside Face Convection Heat Gain Energy','J',0,'Hourly','Zone')`},
		{"wrong dictionary unit", `UPDATE ReportDataDictionary SET Units='W' WHERE ReportDataDictionaryIndex=40`},
		{"shifted hour", `UPDATE Time SET Hour=2 WHERE TimeIndex=1`},
		{"wrong interval", `UPDATE Time SET "Interval"=30 WHERE TimeIndex=1`},
		{"warmup row", `UPDATE Time SET WarmupFlag=1 WHERE TimeIndex=1`},
		{"non-weather row", `UPDATE Time SET EnvironmentPeriodIndex=9 WHERE TimeIndex=1`},
		{"Monthly non-equivalence", `UPDATE ReportData SET Value=Value+36 WHERE TimeIndex=8761`},
		{"missing Monthly", `DELETE FROM ReportData WHERE TimeIndex=8761`},
		{"wrong Monthly calendar", `UPDATE Time SET "Interval"=60 WHERE TimeIndex=8761`},
		{"foreign surface owner", `UPDATE Surfaces SET ZoneIndex=2`},
		{"duplicate surface owner", `INSERT INTO Surfaces VALUES(2,'Floor',1,1)`},
		{"changed multiplier", `UPDATE Zones SET Multiplier=49`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copyPath := filepath.Join(t.TempDir(), "mutated.sql")
			if err := os.WriteFile(copyPath, original, 0600); err != nil {
				t.Fatal(err)
			}
			epathOracleEditSQL(t, copyPath, tc.query)
			if _, _, err := epathCompileSQLHourlyCompanions(copyPath, observed, model, frames); err == nil {
				t.Fatal("accepted changed original SQL authority")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		mutate func(*epathRealSQLModel, *epathSQLFrames)
	}{
		{"wildcard", func(m *epathRealSQLModel, f *epathSQLFrames) { m.HourlyCompanions[0].Keys = []string{"*"} }},
		{"duplicate explicit key", func(m *epathRealSQLModel, f *epathSQLFrames) { m.HourlyCompanions[0].Keys = []string{"Floor", "FLOOR"} }},
		{"unreviewed native name", func(m *epathRealSQLModel, f *epathSQLFrames) {
			m.HourlyCompanions[0].Name = "Surface Unsupported Heat Energy"
		}},
		{"unselected Monthly", func(m *epathRealSQLModel, f *epathSQLFrames) { f.SourceIdentities = map[int]epathRealSQLSource{} }},
		{"no Monthly cell", func(m *epathRealSQLModel, f *epathSQLFrames) { f.Cells = map[string]*epathSQLCell{} }},
		{"foreign cell", func(m *epathRealSQLModel, f *epathSQLFrames) {
			f.Cells = map[string]*epathSQLCell{"foreign": {Zone: "Other", SourceIDs: []int{41}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, f := model, frames
			m.HourlyCompanions = append([]epathRealSQLHourlyCompanion(nil), model.HourlyCompanions...)
			tc.mutate(&m, &f)
			if _, _, err := epathCompileSQLHourlyCompanions(path, observed, m, f); err == nil {
				t.Fatal("accepted non-cell-local declaration")
			}
		})
	}
	frames.SourceRaw[40] = []epathSQLQuantity{{Value: 1}}
	if err := epathSQLBindHourlyCompanions(path, observed, model, &frames); err == nil || len(frames.SourceIdentities) != 1 || frames.TraceSourceIdentities != nil || frames.CellTraceSourceIDs != nil {
		t.Fatal("failed overlap validation published partial trace permission")
	}
	if p, c, err := epathCompileSQLHourlyCompanions("does-not-exist.sql", nil, epathRealSQLModel{}, epathSQLFrames{}); err != nil || len(p) != 0 || len(c) != 0 {
		t.Fatal("prior fixtures acquired an undeclared native source dependency")
	}
}

func TestEnergyPathRealSQLHourlyCompanionConsumedDriverBranchCannotBroadenAuthority(t *testing.T) {
	path, model, observed, frames := epathSQLHourlyCompanionUnitFixture(t, "J", epathSQLHourlyCompanionUnitValues)
	if err := epathSQLBindHourlyCompanions(path, observed, model, &frames); err != nil {
		t.Fatal(err)
	}
	alias := frames.TraceSourceIdentities[40]
	load := epathRealSQLSource{DictionaryIndex: 60, Name: "Zone Air System Sensible Cooling Energy", KeyValue: "Office", SourceUnit: "J", ReportingFrequency: "Monthly"}
	frames.SourceIdentities[60] = load
	monthly := EnergyDataSource{ID: "sql-rdd-41", SourceType: "sql_report_data", Name: alias.Authority.Name, KeyValue: alias.Authority.KeyValue, SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly"}
	cooling := EnergyDataSource{ID: "sql-rdd-60", SourceType: "sql_report_data", Name: load.Name, KeyValue: "Office", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly"}
	sources := map[string]EnergyDataSource{"sql-rdd-40": epathSQLHourlyCompanionUnitSource(*alias.NativeCompanion), "sql-rdd-41": monthly, "sql-rdd-60": cooling, "derived-driver": {ID: "derived-driver", SourceType: "derived_formula", Formula: "signed original balance", InputSourceIDs: []string{"sql-rdd-41", "sql-rdd-40"}}}
	from := EnergyExplanationNode{ID: "driver", ZoneName: "Office", DriverCategory: "balance.storage_other", SourceIDs: []string{"derived-driver"}}
	to := EnergyExplanationNode{ID: "load", ZoneName: "Office", SourceIDs: []string{"sql-rdd-60"}}
	link := EnergyPathLink{ID: "pair", ZoneName: "Office", SourceIDs: []string{"derived-driver", "sql-rdd-60"}}
	proof := epathSQLDriverLinkProof{Service: "cooling", SourceIdentities: frames.SourceIdentities, DriverSources: []int{41}, LoadSources: []int{60}, TemporalTraceSources: map[int]epathSQLTraceSourceIdentity{40: alias}, Branches: map[string]map[string]epathSQLDriverBranchProof{"balance.storage_other": {"office": {DriverSources: []int{41}, LoadSources: []int{60}, TraceSources: []int{40}}}}}
	leaves, err := epathSQLDriverLinkTrace(link, from, to, sources, &proof)
	if err != nil || !leaves.Original[40] || leaves.Driver[40] || !reflect.DeepEqual(leaves.Driver, map[int]bool{41: true}) {
		t.Fatalf("companion lost derived-vector provenance or became additive: %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*epathSQLDriverLinkProof)
	}{
		{"Hourly promoted to pressure", func(p *epathSQLDriverLinkProof) { p.DriverSources = []int{41, 40} }},
		{"authority not a contributing driver", func(p *epathSQLDriverLinkProof) { p.DriverSources = []int{60} }},
		{"unbound Hourly authority", func(p *epathSQLDriverLinkProof) { p.TemporalTraceSources = nil }},
		{"companion from another monthly branch", func(p *epathSQLDriverLinkProof) {
			p.Branches = map[string]map[string]epathSQLDriverBranchProof{"balance.storage_other": {"office": {DriverSources: []int{41}, LoadSources: []int{60}}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := proof
			tc.mutate(&bad)
			if _, err := epathSQLDriverLinkTrace(link, from, to, sources, &bad); err == nil {
				t.Fatal("source-local trace broadened driver/period authority")
			}
		})
	}
	wrongZone := to
	wrongZone.ZoneName = "Other"
	if _, err := epathSQLDriverLinkTrace(link, from, wrongZone, sources, &proof); err == nil {
		t.Fatal("companion crossed the delivered-load Zone boundary")
	}
}
