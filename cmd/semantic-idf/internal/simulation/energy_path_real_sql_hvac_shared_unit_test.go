package simulation

import (
	"database/sql"
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

func epathSQLHVACSharedUnitMember() epathRealSQLHVACSharedMember {
	return epathRealSQLHVACSharedMember{ID: "boiler.ancillary.electricity:central_boiler", ObjectType: "Boiler:HotWater", ObjectName: "Central Boiler", PlantLoopName: "Hot Water Loop", ServedZones: []string{"SPACE1-1", "SPACE2-1", "SPACE3-1", "SPACE4-1", "SPACE5-1"}, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: epathSQLSharedBoilerEnergy, Unit: "J"}}, Keys: []string{"Central Boiler"}}}
}

// Pure mathematical companion fixture. Values are hand-specified native
// monthly kWh, never read from a production/candidate result. Each Hourly rate
// is constant within its calendar month, so the four source integrals agree.
func epathSQLTestHVACSharedIdentity(values [12]float64) epathSQLHVACSharedSourceIdentity {
	identity := epathSQLHVACSharedSourceIdentity{Member: epathSQLHVACSharedUnitMember(), Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1}}
	for n, spec := range []struct{ name, unit, frequency string }{{epathSQLSharedBoilerEnergy, "J", "Monthly"}, {epathSQLSharedBoilerRate, "W", "Monthly"}, {epathSQLSharedBoilerEnergy, "J", "Hourly"}, {epathSQLSharedBoilerRate, "W", "Hourly"}} {
		source := epathRealSQLSource{DictionaryIndex: 1000 + n, Name: spec.name, KeyValue: identity.Member.ObjectName, SourceUnit: spec.unit, ReportingFrequency: spec.frequency, IndexGroup: "HVAC", RawSum: epathOracleNumber(0), EnergyKWh: epathOracleNumber(0)}
		for m, energy := range values {
			hours := time.Date(2017, time.Month(m+2), 0, 0, 0, 0, 0, time.UTC).Day() * 24
			rows := 1
			raw := energy * 3600000
			if spec.frequency == "Hourly" {
				rows = hours
			}
			if spec.unit == "W" {
				raw = energy * 1000
				if spec.frequency == "Monthly" {
					raw /= float64(hours)
				}
			}
			source.Rows += rows
			*source.RawSum += raw
			*source.EnergyKWh += energy
			source.Months = append(source.Months, epathRealSQLMonth{Month: m + 1, Rows: rows, RawSum: epathOracleNumber(raw), EnergyKWh: epathOracleNumber(energy)})
		}
		if n == 0 {
			identity.Source = source
		} else {
			identity.Companions = append(identity.Companions, source)
		}
	}
	return identity
}

func TestEnergyPathRealSQLHVACSharedOriginalPlantAndServedZones(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "5ZoneElectricBaseboard.idf"))
	if err != nil {
		t.Fatal(err)
	}
	member := epathSQLHVACSharedUnitMember()
	pools := []epathRealSQLHVACConsumptionPool{{SiteID: "heating.electricity", Shared: []epathRealSQLHVACSharedMember{member}}}
	if err := epathSQLValidateHVACConsumptionOriginalModel(string(data), pools); err != nil {
		t.Fatal(err)
	}
	doc, err := idf.Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	zones, err := epathSQLHVACOriginalAirLoopZones(doc, "VAV Sys 1")
	if err != nil || !reflect.DeepEqual(zones, member.ServedZones) {
		t.Fatalf("original AirLoop served roster: %v / %v", zones, err)
	}
	for _, tc := range []struct {
		name, typ, object string
		field             int
		value             string
	}{
		{"boiler outlet", "Boiler:HotWater", "Central Boiler", 11, "Disconnected"},
		{"boiler branch type", "Branch", "Central Boiler Branch", 2, "Boiler:Steam"},
		{"plant supply list", "PlantLoop", "Hot Water Loop", 12, "Cooling Supply Side Branches"},
		{"plant outer node", "PlantLoop", "Hot Water Loop", 10, "Disconnected"},
		{"demand splitter branch", "Connector:Splitter", "Heating Demand Splitter", 5, "Missing"},
		{"demand mixer branch", "Connector:Mixer", "Heating Demand Mixer", 5, "Missing"},
		{"demand coil water inlet", "Coil:Heating:Water", "Main Heating Coil 1", 4, "Disconnected"},
		{"central coil air outlet", "Coil:Heating:Water", "Main Heating Coil 1", 7, "Disconnected"},
		{"fan inlet chain", "Fan:VariableVolume", "Supply Fan 1", 15, "Disconnected"},
		{"airloop outlet", "AirLoopHVAC", "VAV Sys 1", 9, "Disconnected"},
		{"splitter inlet", "AirLoopHVAC:ZoneSplitter", "Zone Supply Air Splitter 1", 1, "Disconnected"},
		{"splitter terminal inlet", "AirLoopHVAC:ZoneSplitter", "Zone Supply Air Splitter 1", 3, "Missing"},
		{"terminal reheat inlet", "AirTerminal:SingleDuct:VAV:Reheat", "SPACE1-1 VAV Reheat", 2, "Disconnected"},
		{"ADU outlet", "ZoneHVAC:AirDistributionUnit", "SPACE2-1 ATU", 1, "Disconnected"},
		{"equipment owner", "ZoneHVAC:EquipmentList", "SPACE2-1 Eq", 3, "Missing"},
		{"Zone inlet", "NodeList", "SPACE2-1 In Nodes", 1, "Disconnected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed, err := idf.Parse(string(data))
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for n := range changed.Objects {
				if strings.EqualFold(changed.Objects[n].Type, tc.typ) && strings.EqualFold(epathSQLSharedField(changed.Objects[n], 0), tc.object) {
					changed.Objects[n].Fields[tc.field].Value = tc.value
					count++
				}
			}
			if count != 1 {
				t.Fatal("mutation did not target one exact original object")
			}
			if err := epathSQLValidateHVACConsumptionOriginalModel(changed.String(), pools); err == nil {
				t.Fatal("disconnected/shared source ownership was accepted")
			}
		})
	}
	for _, typ := range []string{"Boiler:HotWater", "Boiler:Steam", "PlantLoop", "AirLoopHVAC:ZoneSplitter"} {
		t.Run("duplicate "+typ, func(t *testing.T) {
			changed, err := idf.Parse(string(data))
			if err != nil {
				t.Fatal(err)
			}
			if typ == "Boiler:Steam" {
				changed.Objects = append(changed.Objects, idf.Object{Type: typ, Fields: []idf.Field{{Value: member.ObjectName}}})
			} else {
				for _, o := range changed.Objects {
					if strings.EqualFold(o.Type, typ) {
						changed.Objects = append(changed.Objects, o)
						break
					}
				}
			}
			if err := epathSQLValidateHVACConsumptionOriginalModel(changed.String(), pools); err == nil {
				t.Fatal("ambiguous typed original ownership accepted")
			}
		})
	}
	member.ServedZones = member.ServedZones[:4]
	pools[0].Shared[0] = member
	if err := epathSQLValidateHVACConsumptionOriginalModel(string(data), pools); err == nil {
		t.Fatal("partial served roster accepted")
	}
}

func TestEnergyPathRealSQLHVACSharedAirPathAmbiguousOwnersAndOriginalRequests(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "5ZoneElectricBaseboard.idf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"AirLoopHVAC", "AirLoopHVAC:SupplyPath"} {
		t.Run(typ, func(t *testing.T) {
			doc, err := idf.Parse(string(data))
			if err != nil {
				t.Fatal(err)
			}
			for _, o := range doc.Objects {
				if strings.EqualFold(o.Type, typ) {
					o.Fields = append([]idf.Field(nil), o.Fields...)
					o.Fields[0].Value = "Other owner"
					doc.Objects = append(doc.Objects, o)
					break
				}
			}
			if _, err := epathSQLHVACOriginalAirLoopZones(doc, "VAV Sys 1"); err == nil {
				t.Fatal("shared demand/splitter accepted more than one original owner")
			}
		})
	}
	doc, err := idf.Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	doc.Objects = append(doc.Objects, idf.Object{Type: "Output:Variable", Fields: []idf.Field{{Value: "*"}, {Value: epathSQLSharedBoilerEnergy}, {Value: "Hourly"}}})
	if err := epathSQLValidateHVACConsumptionOriginalModel(doc.String(), []epathRealSQLHVACConsumptionPool{{SiteID: "heating.electricity", Shared: []epathRealSQLHVACSharedMember{epathSQLHVACSharedUnitMember()}}}); err == nil {
		t.Fatal("unreviewed original boiler opener silently accepted as temporary")
	}
}

func epathSQLHVACSharedSourceUnit(t *testing.T, zero bool) (string, epathRealSQLModel, []epathRealSQLSource, epathSQLFrames) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "boiler.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER,EnvironmentName TEXT,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(3,'Annual',3)`,
		`CREATE TABLE Time(TimeIndex INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,Year INTEGER,"Interval" REAL,IntervalType INTEGER,EnvironmentPeriodIndex INTEGER,WarmupFlag INTEGER,SimulationDays INTEGER)`,
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER,KeyValue TEXT,Name TEXT,Units TEXT,IsMeter INTEGER,ReportingFrequency TEXT,IndexGroup TEXT)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
	} {
		if _, err := db.Exec(query); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 8760; n++ {
		date := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Hour)
		if _, err := tx.Exec(`INSERT INTO Time VALUES(?,?,?,?,0,2017,60,1,3,NULL,?)`, n+100, int(date.Month()), date.Day(), date.Hour()+1, date.YearDay()); err != nil {
			t.Fatal(err)
		}
	}
	for m := 1; m <= 12; m++ {
		date := time.Date(2017, time.Month(m+1), 0, 0, 0, 0, 0, time.UTC)
		if _, err := tx.Exec(`INSERT INTO Time VALUES(?,?,?,24,0,2017,?,3,3,NULL,?)`, m, m, date.Day(), date.Day()*1440, date.YearDay()); err != nil {
			t.Fatal(err)
		}
	}
	for n, spec := range []struct{ name, unit, frequency string }{{epathSQLSharedBoilerEnergy, "J", "Monthly"}, {epathSQLSharedBoilerRate, "W", "Monthly"}, {epathSQLSharedBoilerEnergy, "J", "Hourly"}, {epathSQLSharedBoilerRate, "W", "Hourly"}} {
		if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES(?,'Central Boiler',?,?,0,?,'HVAC')`, n+1, spec.name, spec.unit, spec.frequency); err != nil {
			t.Fatal(err)
		}
		expression := "0"
		if !zero {
			if spec.unit == "J" {
				expression = `0.00049*3600000*("Interval"/60)`
			} else {
				expression = "0.49"
			}
		}
		kind := 3
		if spec.frequency == "Hourly" {
			kind = 1
		}
		if _, err := tx.Exec(fmt.Sprintf(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,%d,%s FROM Time WHERE IntervalType=%d`, n+1, expression, kind)); err != nil {
			t.Fatal(err)
		}
	}
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
	member := epathSQLHVACSharedUnitMember()
	member.ServedZones = []string{"Office"}
	model := epathRealSQLModel{Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1}, Site: []epathRealSQLSite{{ID: "heating.electricity", EndUse: "heating", Carrier: "electricity"}}, HVACConsumptionPools: []epathRealSQLHVACConsumptionPool{{SiteID: "heating.electricity", Shared: []epathRealSQLHVACSharedMember{member}}}}
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{"office": {Name: "Office", Multiplier: 8}}, Site: map[string][]*epathSQLQuantity{"heating.electricity": make([]*epathSQLQuantity, 12)}, SourceIdentities: map[int]epathRealSQLSource{}}
	return path, model, observed.Sources, frames
}

func epathSQLHVACSharedUnitBundle(p epathSQLHVACSharedSourceProof) PurposeResultBundle {
	s := p.Source
	method := "sum_report_data"
	if s.SourceUnit == "W" {
		method = "integrate_rate_by_time_interval"
	}
	actual := EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", s.DictionaryIndex), SourceType: "sql_report_data", Name: s.Name, KeyValue: s.KeyValue, Units: s.SourceUnit, SourceUnit: s.SourceUnit, NormalizedUnit: "kWh", ReportingFrequency: s.ReportingFrequency, AggregationMethod: method, AggregationBasis: "model_total", EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", RawValue: *s.EnergyKWh, EffectiveValue: *s.EnergyKWh, DriverRole: "context", InspectorSection: "Context", inspectorDecodedFromJSON: true, inspectorValuePresence: 3}
	labels := []string{}
	if s.ReportingFrequency == "Hourly" {
		actual.HourlyEnergy = &EnergySourceHourlyEnergy{Unit: "kWh", Basis: "reported_source"}
		for n, value := range p.Hourly {
			actual.HourlyEnergy.Values = append(actual.HourlyEnergy.Values, math.Round(value*1000)/1000)
			date := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Hour)
			labels = append(labels, fmt.Sprintf("%02d-%02d %02d:00", date.Month(), date.Day(), date.Hour()+1))
		}
	}
	return PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Sources: []EnergyDataSource{actual}, HourlyLabels: labels}}
}

func epathSQLHVACSharedUnitPlan() *PurposeRunPlan {
	plan := &PurposeRunPlan{}
	for _, name := range []string{epathSQLSharedBoilerEnergy, epathSQLSharedBoilerRate} {
		for _, frequency := range []string{"Monthly", "Hourly"} {
			plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", VariableName: name, KeyValue: "Central Boiler", ReportingFrequency: frequency, State: "temporary", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
		}
	}
	return plan
}

func TestEnergyPathRealSQLHVACSharedRequestCannotBorrowAnotherFrequency(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*PurposeRunPlan)
	}{
		{"Monthly opener", func(p *PurposeRunPlan) { index := 11; p.OutputObjects[0].ObjectIndex = &index }},
		{"Hourly opener", func(p *PurposeRunPlan) { index := 11; p.OutputObjects[1].ObjectIndex = &index }},
		{"existing instead of temporary", func(p *PurposeRunPlan) { p.OutputObjects[0].State = "existing" }},
		{"wrong Zone owner", func(p *PurposeRunPlan) { p.OutputObjects[0].ScopeZoneName = "Office" }},
		{"missing Hourly", func(p *PurposeRunPlan) { p.OutputObjects = append(p.OutputObjects[:1], p.OutputObjects[2:]...) }},
		{"duplicate request", func(p *PurposeRunPlan) { p.OutputObjects = append(p.OutputObjects, p.OutputObjects[0]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := epathSQLHVACSharedUnitPlan()
			tc.mutate(plan)
			bad := false
			for _, name := range []string{epathSQLSharedBoilerEnergy, epathSQLSharedBoilerRate} {
				for _, frequency := range []string{"Monthly", "Hourly"} {
					bad = bad || epathSQLHVACSharedTemporaryRequest(plan, "Central Boiler", name, frequency) != nil
				}
			}
			if !bad {
				t.Fatal("wrong actual-frequency temporary request accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLHVACSharedNativeSourcesKnownZeroAndCompanions(t *testing.T) {
	for _, zero := range []bool{true, false} {
		t.Run(fmt.Sprint(zero), func(t *testing.T) {
			path, model, observed, frames := epathSQLHVACSharedSourceUnit(t, zero)
			if err := epathCompileSQLHVACConsumptionPoolFrames(path, observed, model, &frames, epathSQLHVACSharedUnitPlan()); err != nil {
				t.Fatal(err)
			}
			if len(frames.HVACConsumptionPools) != 1 || len(frames.HVACSharedSourceIdentities) != 4 || len(frames.SourceZone) != 0 || len(frames.DirectHVAC) != 0 || len(frames.Cells) != 0 {
				t.Fatal("shared source became a Zone-owned/additive observation")
			}
			checks := epathSQLModelChecks{}
			if err := epathSQLModelHVACSharedSourceChecks(frames, &checks); err != nil {
				t.Fatal(err)
			}
			if len(checks.Rows) != 8 {
				t.Fatal("missing exact four-source raw/effective checks")
			}
			for _, check := range checks.Rows {
				bundle := epathSQLHVACSharedUnitBundle(*check.HVACSharedSource)
				if err := epathCheckSQLHVACSharedSource(bundle, check); err != nil {
					t.Fatal(err)
				}
			}
			if !zero {
				for _, check := range checks.Rows {
					if check.Item.Target.Field != "rawValue" {
						continue
					}
					bundle := epathSQLHVACSharedUnitBundle(*check.HVACSharedSource)
					source := &bundle.EnergyExplanation.Sources[0]
					source.RawValue = math.Round(source.RawValue*1000) / 1000
					if err := epathCheckSQLHVACSharedSource(bundle, check); err == nil {
						t.Fatal("native source scalar was incorrectly replaced by a display-rounded annual amount")
					}
				}
			}
			for _, tc := range []struct {
				name   string
				id     int
				mutate func(*EnergyDataSource)
			}{
				{"unknown observed zero", 1, func(s *EnergyDataSource) { s.inspectorValuePresence = 0 }},
				{"native raw scalar changed", 1, func(s *EnergyDataSource) { s.RawValue += .001 }},
				{"false Zone measured owner", 1, func(s *EnergyDataSource) { s.ZoneName = "Office" }},
				{"double multiplier", 1, func(s *EnergyDataSource) { s.EffectiveMultiplier = 8 }},
				{"borrowed derived identity", 1, func(s *EnergyDataSource) { s.InputSourceIDs = []string{"other"} }},
				{"Rate duplicated allocation", 2, func(s *EnergyDataSource) { s.AllocationApplied = true; s.AllocatedValue = 1 }},
				{"Hourly duplicated Zone allocation", 3, func(s *EnergyDataSource) {
					s.ScopeDetails = []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}}}
				}},
				{"native Hourly chart changed", 3, func(s *EnergyDataSource) { s.HourlyEnergy.Values[0] = .002 }},
				{"Monthly borrowed Hourly chart", 1, func(s *EnergyDataSource) { s.HourlyEnergy = &EnergySourceHourlyEnergy{} }},
				{"borrowed original opener", 3, func(s *EnergyDataSource) { index := 17; s.ObjectIndex = &index }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					for _, check := range checks.Rows {
						if check.HVACSharedSource.Source.DictionaryIndex != tc.id {
							continue
						}
						bundle := epathSQLHVACSharedUnitBundle(*check.HVACSharedSource)
						tc.mutate(&bundle.EnergyExplanation.Sources[0])
						if err := epathCheckSQLHVACSharedSource(bundle, check); err == nil {
							t.Fatal("mutated shared source contract accepted")
						}
						break
					}
				})
			}
		})
	}
}

func TestEnergyPathRealSQLHVACSharedRejectsNativeMissingNegativeAndAmbiguity(t *testing.T) {
	for _, tc := range []struct{ name, query string }{
		{"missing Month", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=1 AND TimeIndex=1`},
		{"NULL Monthly", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=1 AND TimeIndex=1`},
		{"negative Monthly", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=1 AND TimeIndex=1`},
		{"negative Hourly", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=3 AND TimeIndex=100`},
		{"NULL Hourly", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=4 AND TimeIndex=100`},
		{"duplicate zero dictionary", `INSERT INTO ReportDataDictionary VALUES(7,'Central Boiler','Boiler Ancillary Electricity Energy','J',0,'Monthly','HVAC')`},
		{"missing Hourly", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=3 AND TimeIndex=100`},
		{"wrong monthly duration", `UPDATE Time SET "Interval"=60 WHERE TimeIndex=1`},
		{"duplicate zero row", `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(1,1,0)`},
		{"wrong source units", `UPDATE ReportDataDictionary SET Units='W' WHERE ReportDataDictionaryIndex=1`},
		{"warmup Month", `UPDATE Time SET WarmupFlag=1 WHERE TimeIndex=1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, model, observed, frames := epathSQLHVACSharedSourceUnit(t, true)
			epathOracleEditSQL(t, path, tc.query)
			if err := epathCompileSQLHVACConsumptionPoolFrames(path, observed, model, &frames, epathSQLHVACSharedUnitPlan()); err == nil || len(frames.HVACConsumptionPools) != 0 || len(frames.HVACSharedSourceIdentities) != 0 {
				t.Fatal("corrupt native source accepted or partial frame committed")
			}
		})
	}
}
