package simulation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathVRFOracleUnitModel() epathRealSQLModel {
	keys := []string{"TU1", "TU2", "TU3", "TU4", "TU5"}
	selector := func(name string, keys []string, meter bool) epathRealSQLSelector {
		return epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: name, Unit: "J"}}, Keys: append([]string(nil), keys...), IsMeter: meter}
	}
	system := epathRealSQLVRFSystem{OutdoorUnit: epathRealSQLVRFObject{"AirConditioner:VariableRefrigerantFlow", "VRF Heat Pump"}, TerminalUnitList: epathRealSQLVRFObject{"ZoneTerminalUnitList", "VRF Heat Pump TU List"}, CoolingSiteID: "cooling.electricity", HeatingSiteID: "heating.electricity", Frequency: "Monthly", AggregationBasis: "model_total", AllocationPolicy: "per_constituent_millikwh_largest_remainder_v1"}
	for i, key := range keys {
		system.Terminals = append(system.Terminals, epathRealSQLVRFTerminal{epathRealSQLVRFObject{"ZoneHVAC:TerminalUnit:VariableRefrigerantFlow", key}, fmt.Sprintf("SPACE%d-1", i+1)})
	}
	system.Sources = epathRealSQLVRFSelectors{
		LocalCooling: selector("Zone VRF Air Terminal Cooling Electricity Energy", keys, false), LocalHeating: selector("Zone VRF Air Terminal Heating Electricity Energy", keys, false),
		SharedCooling: selector("VRF Heat Pump Cooling Electricity Energy", []string{"VRF Heat Pump"}, false), SharedCrankcase: selector("VRF Heat Pump Crankcase Heater Electricity Energy", []string{"VRF Heat Pump"}, false),
		SharedHeating: selector("VRF Heat Pump Heating Electricity Energy", []string{"VRF Heat Pump"}, false), SharedDefrost: selector("VRF Heat Pump Defrost Electricity Energy", []string{"VRF Heat Pump"}, false),
	}
	return epathRealSQLModel{NativeVRFSystems: []epathRealSQLVRFSystem{system}, Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}, Site: []epathRealSQLSite{
		{ID: "cooling.electricity", EndUse: "cooling", Carrier: "electricity", Source: selector("Cooling:Electricity", []string{""}, true)},
		{ID: "heating.electricity", EndUse: "heating", Carrier: "electricity", Source: selector("Heating:Electricity", []string{""}, true)},
	}}
}

// Standalone independent SQLite/plan fixture: no production VRF catalog,
// purpose builder, SQL parser, allocator or candidate quantities are called.
func epathVRFOracleUnitFixture(t *testing.T) (string, string, PurposeRunPlan, epathRealSQLModel, epathRealOracleEvidence, epathSQLFrames) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "DOAToVRF.idf"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "vrf.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER,Name TEXT,KeyValue TEXT,Units TEXT,IsMeter INTEGER,ReportingFrequency TEXT,IndexGroup TEXT)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
		`CREATE TABLE "Time"(TimeIndex INTEGER,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,"Interval" REAL,IntervalType INTEGER,SimulationDays INTEGER,EnvironmentPeriodIndex INTEGER,WarmupFlag INTEGER)`,
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER,EnvironmentName TEXT,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(3,'Controlled Weather',3)`,
	} {
		exec(query)
	}
	for month := 1; month <= 12; month++ {
		last := time.Date(2017, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC)
		exec(`INSERT INTO "Time" VALUES(?,2017,?,?,24,0,?,3,?,3,NULL)`, month, month, last.Day(), last.Day()*1440, last.YearDay())
	}
	model := epathVRFOracleUnitModel()
	plan := PurposeRunPlan{}
	add := func(id int, name, key, zone string, coefficient float64, meter bool) {
		flag := 0
		if meter {
			flag = 1
		}
		exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,'J',?,'Monthly','HVAC')`, id, name, key, flag)
		for month := 1; month <= 12; month++ {
			exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,?,?)`, month, id, coefficient*float64(month)*3600000)
		}
		if !meter {
			index := 5000 + id
			plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", KeyValue: key, VariableName: name, ReportingFrequency: "Monthly", ScopeZoneName: zone, PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, ObjectIndex: &index})
		}
	}
	for owner := 1; owner <= 5; owner++ {
		cooling, heating := float64(owner), float64(owner)
		if owner == 1 {
			cooling = 0
		}
		if owner == 2 {
			heating = 0
		}
		key, zone := fmt.Sprintf("TU%d", owner), fmt.Sprintf("SPACE%d-1", owner)
		add(100+2*owner, "Zone VRF Air Terminal Cooling Electricity Energy", key, zone, cooling, false)
		add(101+2*owner, "Zone VRF Air Terminal Heating Electricity Energy", key, zone, heating, false)
	}
	add(201, "VRF Heat Pump Cooling Electricity Energy", "VRF Heat Pump", "", 100, false)
	add(202, "VRF Heat Pump Crankcase Heater Electricity Energy", "VRF Heat Pump", "", 10, false)
	add(203, "VRF Heat Pump Heating Electricity Energy", "VRF Heat Pump", "", 200, false)
	add(204, "VRF Heat Pump Defrost Electricity Energy", "VRF Heat Pump", "", 20, false)
	add(1, "Cooling:Electricity", "", "", 124, true)
	add(2, "Heating:Electricity", "", "", 233, true)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{}, Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{}, SourceIdentities: map[int]epathRealSQLSource{}}
	for owner := 1; owner <= 5; owner++ {
		zone := fmt.Sprintf("SPACE%d-1", owner)
		frames.Zones[strings.ToLower(zone)] = epathSQLZone{Name: zone, Multiplier: 1}
	}
	for _, site := range model.Site {
		sources, err := epathSQLSelect(observed.Sources, site.Source)
		if err != nil {
			t.Fatal(err)
		}
		values, err := epathSQLMonthly(sources[0], model.Precision)
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range values {
			value := value
			frames.Site[site.ID] = append(frames.Site[site.ID], &value)
		}
		frames.SiteSources[site.ID] = []int{sources[0].DictionaryIndex}
		frames.SourceIdentities[sources[0].DictionaryIndex] = sources[0]
	}
	return path, string(raw), plan, model, observed, frames
}

func epathVRFOracleUnitCopy[T any](t *testing.T, value T) T {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestEnergyPathRealSQLVRFSourceFramesIndependent(t *testing.T) {
	path, text, plan, model, observed, frames := epathVRFOracleUnitFixture(t)
	before, _ := json.Marshal(frames)
	got, err := epathCompileSQLVRFSystems(path, text, &plan, observed.Sources, model, frames)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Sources) != 14 {
		t.Fatal("lost original 5 local pairs + 4 shared constituents")
	}
	knownZero := 0
	for _, source := range got[0].Sources {
		if source.RequestObjectIndex == nil || *source.RequestObjectIndex != 5000+source.Source.DictionaryIndex || source.EquipmentObjectIndex == *source.RequestObjectIndex {
			t.Fatal("request/physical/source indices conflated")
		}
		for month, value := range source.Months {
			if value.Value == 0 {
				knownZero++
				if value.Error != 0 {
					t.Fatal("original zero widened")
				}
			} else if value.Error != .001 {
				t.Fatal("source precision budget changed")
			}
			if value.Value != *source.Source.Months[month].EnergyKWh {
				t.Fatal("native quantity center changed")
			}
		}
		if source.Source.DictionaryIndex == 201 && (source.Months[0].Value != 100 || *source.Source.EnergyKWh != 7800) {
			t.Fatal("hand original outdoor arithmetic changed")
		}
	}
	if knownZero != 24 {
		t.Fatalf("known zero months=%d, want 24", knownZero)
	}
	after, _ := json.Marshal(frames)
	if string(before) != string(after) {
		t.Fatal("standalone VRF frame mutated existing numeric authority")
	}
	// Native system consumption is already model-total; load multipliers do
	// not multiply any terminal/outdoor J observation a second time.
	z := frames.Zones["space1-1"]
	z.Multiplier = 7
	frames.Zones["space1-1"] = z
	other, err := epathCompileSQLVRFSystems(path, text, &plan, observed.Sources, model, frames)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, other) {
		t.Fatal("native VRF consumption changed under load multiplier")
	}
	if empty, err := epathCompileSQLVRFSystems("does-not-exist", "invalid", nil, nil, epathRealSQLModel{}, epathSQLFrames{}); err != nil || empty != nil {
		t.Fatal("optional VRF changed preexisting recipe behavior")
	}
}

func TestEnergyPathRealSQLVRFSourceRejectsUnreviewedInputs(t *testing.T) {
	path, text, plan, model, observed, frames := epathVRFOracleUnitFixture(t)
	for _, tc := range []struct {
		name string
		edit func(*epathRealSQLModel)
	}{
		{"missing crankcase", func(m *epathRealSQLModel) { m.NativeVRFSystems[0].Sources.SharedCrankcase = epathRealSQLSelector{} }},
		{"wrong crankcase service name", func(m *epathRealSQLModel) {
			m.NativeVRFSystems[0].Sources.SharedCrankcase.Alternatives[0].Name = "VRF Heat Pump Defrost Electricity Energy"
		}},
		{"missing local owner", func(m *epathRealSQLModel) { m.NativeVRFSystems[0].Terminals = m.NativeVRFSystems[0].Terminals[:4] }},
		{"wildcard owner", func(m *epathRealSQLModel) { m.NativeVRFSystems[0].Sources.LocalCooling.Keys = []string{"*"} }},
		{"allow absent", func(m *epathRealSQLModel) { m.NativeVRFSystems[0].Sources.SharedDefrost.AllowAbsent = true }},
		{"wrong basis", func(m *epathRealSQLModel) { m.NativeVRFSystems[0].AggregationBasis = "per_zone" }},
		{"wrong policy", func(m *epathRealSQLModel) { m.NativeVRFSystems[0].AllocationPolicy = "annual_load_share" }},
		{"wrong broad carrier", func(m *epathRealSQLModel) { m.Site[0].Carrier = "natural_gas" }},
		{"duplicate system", func(m *epathRealSQLModel) { m.NativeVRFSystems = append(m.NativeVRFSystems, m.NativeVRFSystems[0]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := epathVRFOracleUnitCopy(t, model)
			tc.edit(&bad)
			if _, err := epathCompileSQLVRFSystems(path, text, &plan, observed.Sources, bad, frames); err == nil {
				t.Fatal("invalid declaration accepted")
			}
		})
	}
	for _, kind := range []string{"missing", "wrong scope", "wrong purpose", "only Timestep", "duplicate"} {
		t.Run("request "+kind, func(t *testing.T) {
			bad := epathVRFOracleUnitCopy(t, plan)
			switch kind {
			case "missing":
				bad.OutputObjects = bad.OutputObjects[1:]
			case "wrong scope":
				bad.OutputObjects[0].ScopeZoneName = "SPACE2-1"
			case "wrong purpose":
				bad.OutputObjects[0].PurposeIDs = []SimulationPurposeID{SimulationPurposeZoneHeatFlow}
			case "only Timestep":
				bad.OutputObjects[0].ReportingFrequency = "Timestep"
			case "duplicate":
				bad.OutputObjects = append(bad.OutputObjects, bad.OutputObjects[0])
			}
			if _, err := epathCompileSQLVRFSystems(path, text, &bad, observed.Sources, model, frames); err == nil {
				t.Fatal("invalid original request accepted")
			}
		})
	}
	for _, kind := range []string{"missing main", "missing month", "null", "negative", "nonfinite", "converted center", "duplicate source"} {
		t.Run("observation "+kind, func(t *testing.T) {
			bad := epathVRFOracleUnitCopy(t, observed.Sources)
			target := 0
			for i, s := range bad {
				if s.DictionaryIndex == 201 {
					target = i
				}
			}
			switch kind {
			case "missing main":
				bad = append(bad[:target], bad[target+1:]...)
			case "missing month":
				bad[target].Months = bad[target].Months[:11]
			case "null":
				bad[target].Months[0].RawSum = nil
			case "negative":
				*bad[target].Months[0].RawSum = -1
			case "nonfinite":
				*bad[target].Months[0].EnergyKWh = math.Inf(1)
			case "converted center":
				*bad[target].Months[0].EnergyKWh += 1
			case "duplicate source":
				bad = append(bad, bad[target])
			}
			if _, err := epathCompileSQLVRFSystems(path, text, &plan, bad, model, frames); err == nil {
				t.Fatal("invalid native source accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLVRFOriginalOwnershipRejectsAmbiguity(t *testing.T) {
	_, text, _, model, _, _ := epathVRFOracleUnitFixture(t)
	for _, kind := range []string{"list duplicate", "foreign list", "shared list", "duplicate TU", "wrong zone", "shared cooling coil", "disconnected mixer", "wrong fuel", "supplemental coil"} {
		t.Run(kind, func(t *testing.T) {
			doc, err := idf.Parse(text)
			if err != nil {
				t.Fatal(err)
			}
			find := func(objectType, name string) *idf.Object {
				for i := range doc.Objects {
					if strings.EqualFold(doc.Objects[i].Type, objectType) && strings.EqualFold(epathSQLVRFField(doc.Objects[i], 0), name) {
						return &doc.Objects[i]
					}
				}
				t.Fatalf("missing literal fixture object %s/%s", objectType, name)
				return nil
			}
			switch kind {
			case "list duplicate":
				find("ZoneTerminalUnitList", "VRF Heat Pump TU List").Fields[1].Value = "TU1"
			case "foreign list":
				find("ZoneTerminalUnitList", "VRF Heat Pump TU List").Fields[1].Value = "FOREIGN"
			case "shared list":
				copy := *find("AirConditioner:VariableRefrigerantFlow", "VRF Heat Pump")
				copy.Fields = append([]idf.Field(nil), copy.Fields...)
				copy.Fields[0].Value = "Other Outdoor"
				doc.Objects = append(doc.Objects, copy)
			case "duplicate TU":
				copy := *find("ZoneHVAC:TerminalUnit:VariableRefrigerantFlow", "TU1")
				doc.Objects = append(doc.Objects, copy)
			case "wrong zone":
				find("ZoneHVAC:EquipmentConnections", "SPACE1-1").Fields[1].Value = "SPACE2-1 Eq"
			case "shared cooling coil":
				find("ZoneHVAC:TerminalUnit:VariableRefrigerantFlow", "TU2").Fields[18].Value = "TU1 VRF DX Cooling Coil"
			case "disconnected mixer":
				find("AirTerminal:SingleDuct:Mixer", "SPACE1-1 DOAS Air Terminal").Fields[3].Value = "OTHER INLET"
			case "wrong fuel":
				find("AirConditioner:VariableRefrigerantFlow", "VRF Heat Pump").Fields[66].Value = "NaturalGas"
			case "supplemental coil":
				find("ZoneHVAC:TerminalUnit:VariableRefrigerantFlow", "TU1").Fields[26].Value = "Coil:Heating:Electric"
			}
			if _, err := epathSQLVRFOriginalOwners(doc.String(), model.NativeVRFSystems); err == nil {
				t.Fatal("invalid original typed ownership accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLVRFOriginalSQLAndSourceTraceBinding(t *testing.T) {
	for _, query := range []string{
		`INSERT INTO ReportDataDictionary SELECT 999,Name,KeyValue,Units,IsMeter,ReportingFrequency,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=201`,
		`UPDATE ReportData SET Value=Value+1 WHERE ReportDataDictionaryIndex=201 AND TimeIndex=1`,
		`UPDATE "Time" SET SimulationDays=SimulationDays-1 WHERE TimeIndex=1`,
		`INSERT INTO "Time" SELECT 999,Year,Month,Day,Hour,Minute,"Interval",IntervalType,SimulationDays,EnvironmentPeriodIndex,WarmupFlag FROM "Time" WHERE TimeIndex=1`,
	} {
		t.Run(query, func(t *testing.T) {
			path, text, plan, model, observed, frames := epathVRFOracleUnitFixture(t)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(query)
			db.Close()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := epathCompileSQLVRFSystems(path, text, &plan, observed.Sources, model, frames); err == nil {
				t.Fatal("stale/duplicate original SQL accepted")
			}
		})
	}
	path, text, plan, model, observed, frames := epathVRFOracleUnitFixture(t)
	systems, err := epathCompileSQLVRFSystems(path, text, &plan, observed.Sources, model, frames)
	if err != nil {
		t.Fatal(err)
	}
	proof := systems[0].Sources[0]
	actual := EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", proof.Source.DictionaryIndex), SourceType: "sql_report_data", Name: proof.Source.Name, KeyValue: proof.Source.KeyValue, ZoneName: proof.ZoneName, Units: "J", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", AggregationMethod: "sum", AggregationBasis: "model_total", EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", ObjectIndex: proof.RequestObjectIndex}
	if err := epathSQLMatchVRFSource(actual, proof); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"physical index", "wrong source", "wrong zone", "wrong frequency", "wrong multiplier", "derived wrapper"} {
		t.Run(kind, func(t *testing.T) {
			bad := actual
			switch kind {
			case "physical index":
				index := proof.EquipmentObjectIndex
				bad.ObjectIndex = &index
			case "wrong source":
				bad.ID = "sql-rdd-999"
			case "wrong zone":
				bad.ZoneName = "PLENUM-1"
			case "wrong frequency":
				bad.ReportingFrequency = "Timestep"
			case "wrong multiplier":
				bad.EffectiveMultiplier = 7
			case "derived wrapper":
				bad.InputSourceIDs = []string{actual.ID}
			}
			if err := epathSQLMatchVRFSource(bad, proof); err == nil {
				t.Fatal("spoofed source provenance accepted")
			}
		})
	}
	mutant := epathVRFOracleUnitCopy(t, systems[0])
	mutant.Sources[0].Months[0].Value = 1e-9
	if err := epathSQLValidateVRFSystemFrame(mutant); err == nil {
		t.Fatal("source-independent frame center was replaced")
	}
}
