package simulation

// Independent acceptance support. Hand native observations; no engine, candidate or expected file.
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

func epathSQLPoolSourceHandWeather() epathRealSQLWeather {
	return epathRealSQLWeather{Year: 2017, EnvironmentIndex: 3, Months: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, CoverageBasis: "monthly_intervals_and_cumulative_simulation_days"}
}

func epathSQLPoolHandDictionary(source epathRealSQLSource) epathSQLPoolNativeDictionary {
	dictionary := epathSQLPoolNativeDictionary{DictionaryIndex: source.DictionaryIndex, Type: "Sum", TimestepType: "HVAC System"}
	if source.SourceUnit == "W" {
		dictionary.Type = "Avg"
	}
	if source.IsMeter {
		dictionary.TimestepType = "Zone"
	}
	return dictionary
}

// A pure source-math fixture, not positive original topology evidence. The
// full compiler independently recomputes A's proof from original text.
func epathSQLPoolSourceHandOriginal() epathSQLPoolOriginalProof {
	d := epathRealSQLPoolSystem{ID: "hand-pool", PoolName: "Test Pool", SurfaceName: "Pool Floor", ZoneName: "Office", HotWaterLoopName: "HW", ChilledWaterLoopName: "CW", BoilerName: "Boiler", HotWaterPumpName: "HW Pump", ChilledWaterPumpName: "CW Pump"}
	return epathSQLPoolOriginalProof{Declaration: d, OriginalSHA256: strings.Repeat("a", 64), Owners: []epathSQLPoolOriginalOwner{
		{ObjectType: "SwimmingPool:Indoor", ObjectName: d.PoolName, ObjectIndex: 1, PlantLoopName: "HW", ZoneName: "Office"},
		{ObjectType: "Boiler:HotWater", ObjectName: d.BoilerName, ObjectIndex: 2, FuelType: "NaturalGas", PlantLoopName: "HW"},
		{ObjectType: "Pump:VariableSpeed", ObjectName: d.HotWaterPumpName, ObjectIndex: 3, PlantLoopName: "HW"},
		{ObjectType: "Pump:VariableSpeed", ObjectName: d.ChilledWaterPumpName, ObjectIndex: 4, PlantLoopName: "CW"},
	}}
}

// Each month has a constant hand power in kW. Native Monthly W is an average,
// whereas Hourly W RawSum is a sum of reported averages; neither is the kWh
// scalar. E/R/month/hour agree without taking any candidate quantity as input.
func epathSQLPoolSourceHandObservation(index int, name, key, unit, frequency string, meter bool, powerKW float64) (epathRealSQLSource, []epathSQLPoolNativeRow) {
	source := epathRealSQLSource{DictionaryIndex: index, Name: name, KeyValue: key, SourceUnit: unit, ReportingFrequency: frequency, IsMeter: meter, IndexGroup: "HVAC", RawSum: epathOracleNumber(0), EnergyKWh: epathOracleNumber(0)}
	var rows []epathSQLPoolNativeRow
	for month := 1; month <= 12; month++ {
		last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC)
		hours, count := last.Day()*24, 1
		if frequency == "Hourly" {
			count = hours
		}
		bucket := epathRealSQLMonth{Month: month, Rows: count, RawSum: epathOracleNumber(0), EnergyKWh: epathOracleNumber(0)}
		for n := 0; n < count; n++ {
			row := epathSQLPoolNativeRow{TimeIndex: month, EnvironmentIndex: 3, Year: 2017, Month: month, Day: last.Day(), Hour: 24, IntervalType: 3, IntervalMinutes: float64(hours * 60), SimulationDays: last.YearDay()}
			if frequency == "Hourly" {
				date := time.Date(2017, time.Month(month), 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Hour)
				row.TimeIndex, row.Day, row.Hour = 100+(date.YearDay()-1)*24+date.Hour(), date.Day(), date.Hour()+1
				row.IntervalType, row.IntervalMinutes, row.SimulationDays = 1, 60, date.YearDay()
			}
			row.EnergyKWh = powerKW * row.IntervalMinutes / 60
			row.NativeValue = row.EnergyKWh * 3600000
			if unit == "W" {
				row.NativeValue = powerKW * 1000
			}
			rows = append(rows, row)
			*bucket.RawSum += row.NativeValue
			*bucket.EnergyKWh += row.EnergyKWh
		}
		source.Rows += count
		*source.RawSum += *bucket.RawSum
		*source.EnergyKWh += *bucket.EnergyKWh
		source.Months = append(source.Months, bucket)
	}
	return source, rows
}

func epathSQLPoolSourceHandFrames(t *testing.T) epathSQLPoolSourceFrames {
	return epathSQLPoolSourceHandFramesForOriginal(t, epathSQLPoolSourceHandOriginal())
}

func epathSQLPoolSourceHandFramesForOriginal(t *testing.T, original epathSQLPoolOriginalProof) epathSQLPoolSourceFrames {
	t.Helper()
	frame := epathSQLPoolSourceFrames{Original: original, Weather: epathSQLPoolSourceHandWeather(), Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1}, Families: map[string]epathSQLPoolSourceFamily{}, Sources: map[int]epathSQLPoolSourceIdentity{}, Parents: map[string]epathSQLPoolParentClosure{}}
	specs, err := epathSQLPoolSourceSpecs(frame.Original)
	if err != nil {
		t.Fatal(err)
	}
	// Distinct thermal/purchased values make thermal fuel subtraction and
	// duplicate source substitution visible. Boiler ancillary E is observed 0.
	powers := []float64{8, 12, 3, 1, 0, 2, 5}
	for n, spec := range specs {
		family := epathSQLPoolSourceFamily{Spec: spec}
		for p, selection := range []struct{ name, unit, frequency string }{{spec.EnergyName, "J", "Monthly"}, {spec.RateName, "W", "Monthly"}, {spec.EnergyName, "J", "Hourly"}, {spec.RateName, "W", "Hourly"}} {
			source, rows := epathSQLPoolSourceHandObservation(100+n*4+p, selection.name, spec.Owner.ObjectName, selection.unit, selection.frequency, false, powers[n])
			identity := epathSQLPoolSourceIdentity{Spec: spec, Source: source, Dictionary: epathSQLPoolHandDictionary(source), Rows: rows, Precision: frame.Precision, Canonical: p == 0, BudgetAuthority: p == 0 && spec.Role == "purchased_constituent", RequestBound: true, ExecutedOwnerIndex: spec.Owner.ObjectIndex}
			for m, bucket := range source.Months {
				identity.NativeRawMonthly[m], identity.NativeEnergyMonthly[m] = *bucket.RawSum, *bucket.EnergyKWh
			}
			frame.Sources[source.DictionaryIndex] = identity
			if p == 0 {
				family.CanonicalID = source.DictionaryIndex
				family.Monthly, err = epathSQLMonthly(source, frame.Precision)
				if err != nil {
					t.Fatal(err)
				}
				family.Annual, err = epathSQLPoolSourceQuantity(identity, frame.Weather)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				family.CompanionIDs = append(family.CompanionIDs, source.DictionaryIndex)
			}
		}
		frame.Families[spec.ID] = family
	}
	for n, spec := range []struct {
		name, endUse, carrier string
		power                 float64
		members               []string
	}{
		{"Heating:NaturalGas", "heating", "natural_gas", 4, []string{"boiler.natural_gas", "boiler.ancillary_natural_gas"}},
		{"Heating:Electricity", "heating", "electricity", 0, []string{"boiler.ancillary_electricity"}},
		{"Pumps:Electricity", "pumps", "electricity", 7, []string{"pump.hw.electricity", "pump.cw.electricity"}},
	} {
		source, rows := epathSQLPoolSourceHandObservation(200+n, spec.name, "", "J", "Monthly", true, spec.power)
		parent := epathSQLPoolParentClosure{Source: source, Dictionary: epathSQLPoolHandDictionary(source), Rows: rows, EndUse: spec.endUse, Carrier: spec.carrier, ConstituentFamilyIDs: spec.members}
		parent.Monthly, err = epathSQLMonthly(source, frame.Precision)
		if err != nil {
			t.Fatal(err)
		}
		for m, bucket := range source.Months {
			parent.NativeMonthly[m] = *bucket.EnergyKWh
		}
		for _, member := range spec.members {
			parent.ConstituentSourceIDs = append(parent.ConstituentSourceIDs, frame.Families[member].CanonicalID)
		}
		frame.Parents[spec.name] = parent
	}
	return frame
}

func TestEnergyPathRealSQLPoolSourceNativeMatrixAndParentClosure(t *testing.T) {
	frame := epathSQLPoolSourceHandFrames(t)
	if err := epathSQLValidatePoolSourceFrames(frame); err != nil {
		t.Fatal(err)
	}
	if len(frame.Sources) != 28 || len(frame.Families) != 7 || len(frame.Parents) != 3 {
		t.Fatal("native source/context roster changed")
	}
	budgets := 0
	for _, identity := range frame.Sources {
		if identity.BudgetAuthority {
			budgets++
		}
		q, err := epathSQLPoolSourceQuantity(identity, frame.Weather)
		if err != nil {
			t.Fatal(err)
		}
		if identity.Spec.ID == "pool.water_heating" && !epathSQLPoolSameNative(q.Value, 8*8760) {
			t.Fatal("native Pool thermal source applied representative Zone multiplier or E/R double count")
		}
		if identity.Spec.ID == "boiler.ancillary_electricity" && (q.Value != 0 || q.Error != 0) {
			t.Fatal("observed zero lost exact presence/bounds")
		}
		if identity.Spec.ID == "pool.water_heating" && identity.Source.SourceUnit == "W" && identity.Source.ReportingFrequency == "Monthly" && (*identity.Source.RawSum != 8000*12 || !epathSQLPoolSameNative(q.Value, 8*8760)) {
			t.Fatal("native W raw sum was conflated with integrated kWh")
		}
	}
	if budgets != 5 {
		t.Fatalf("only five purchased Monthly Energy sources own budgets, got %d", budgets)
	}
}

func TestEnergyPathRealSQLPoolSourceFrameRejectsBoundarySubstitution(t *testing.T) {
	for _, mutation := range []string{"missing family", "missing zero", "duplicate companion", "rate budget", "thermal budget", "wrong owner", "foreign carrier", "wrong plant", "parent normalization", "parent physical mismatch", "parent thermal", "parent duplicate", "source precision", "annual from rounded chart", "independently different companion", "energy aggregation", "rate aggregation", "component timestep", "parent aggregation", "parent timestep", "dictionary identity"} {
		t.Run(mutation, func(t *testing.T) {
			frame := epathSQLPoolSourceHandFrames(t)
			family := frame.Families["boiler.ancillary_electricity"]
			identity := frame.Sources[family.CanonicalID]
			switch mutation {
			case "missing family":
				delete(frame.Families, "pool.water_heating")
			case "missing zero":
				delete(frame.Sources, family.CanonicalID)
			case "duplicate companion":
				family.CompanionIDs[1] = family.CompanionIDs[0]
				frame.Families[family.Spec.ID] = family
			case "rate budget":
				item := frame.Sources[family.CompanionIDs[0]]
				item.BudgetAuthority = true
				frame.Sources[item.Source.DictionaryIndex] = item
			case "thermal budget":
				f := frame.Families["pool.water_heating"]
				item := frame.Sources[f.CanonicalID]
				item.BudgetAuthority = true
				frame.Sources[f.CanonicalID] = item
			case "wrong owner":
				identity.Source.KeyValue = "Other Boiler"
				frame.Sources[identity.Source.DictionaryIndex] = identity
			case "foreign carrier":
				identity.Spec.Carrier = "natural_gas"
				frame.Sources[identity.Source.DictionaryIndex] = identity
			case "wrong plant":
				identity.Spec.Owner.PlantLoopName = "CW"
				frame.Sources[identity.Source.DictionaryIndex] = identity
			case "parent normalization":
				p := frame.Parents["Pumps:Electricity"]
				p.NativeMonthly[0] *= 2
				frame.Parents[p.Source.Name] = p
			case "parent physical mismatch":
				p := frame.Parents["Pumps:Electricity"]
				p.Source, p.Rows = epathSQLPoolSourceHandObservation(p.Source.DictionaryIndex, p.Source.Name, "", "J", "Monthly", true, 14)
				p.Monthly, _ = epathSQLMonthly(p.Source, frame.Precision)
				for m, b := range p.Source.Months {
					p.NativeMonthly[m] = *b.EnergyKWh
				}
				frame.Parents[p.Source.Name] = p
			case "parent thermal":
				p := frame.Parents["Heating:NaturalGas"]
				p.ConstituentSourceIDs[0] = frame.Families["boiler.heating_output"].CanonicalID
				frame.Parents[p.Source.Name] = p
			case "parent duplicate":
				p := frame.Parents["Pumps:Electricity"]
				p.ConstituentSourceIDs[1] = p.ConstituentSourceIDs[0]
				frame.Parents[p.Source.Name] = p
			case "source precision":
				identity.Precision.SourceStages = 99
				frame.Sources[identity.Source.DictionaryIndex] = identity
			case "annual from rounded chart":
				f := frame.Families["pump.cw.electricity"]
				f.Annual.Value += .001
				frame.Families[f.Spec.ID] = f
			case "independently different companion":
				f := frame.Families["pump.cw.electricity"]
				item := frame.Sources[f.CompanionIDs[0]]
				item.Source, item.Rows = epathSQLPoolSourceHandObservation(item.Source.DictionaryIndex, item.Source.Name, item.Source.KeyValue, "W", "Monthly", false, 6)
				for m, b := range item.Source.Months {
					item.NativeRawMonthly[m], item.NativeEnergyMonthly[m] = *b.RawSum, *b.EnergyKWh
				}
				frame.Sources[item.Source.DictionaryIndex] = item
			case "energy aggregation":
				identity.Dictionary.Type = "Avg"
				frame.Sources[identity.Source.DictionaryIndex] = identity
			case "rate aggregation":
				item := frame.Sources[family.CompanionIDs[0]]
				item.Dictionary.Type = "Sum"
				frame.Sources[item.Source.DictionaryIndex] = item
			case "component timestep":
				identity.Dictionary.TimestepType = "Zone"
				frame.Sources[identity.Source.DictionaryIndex] = identity
			case "parent aggregation":
				p := frame.Parents["Heating:Electricity"]
				p.Dictionary.Type = "Avg"
				frame.Parents[p.Source.Name] = p
			case "parent timestep":
				p := frame.Parents["Pumps:Electricity"]
				p.Dictionary.TimestepType = "HVAC System"
				frame.Parents[p.Source.Name] = p
			case "dictionary identity":
				identity.Dictionary.DictionaryIndex++
				frame.Sources[identity.Source.DictionaryIndex] = identity
			}
			if err := epathSQLValidatePoolSourceFrames(frame); err == nil {
				t.Fatal("unproved native source/parent boundary was accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLPoolSourceRowsRejectUnknownAndCalendarRepair(t *testing.T) {
	for _, frequency := range []string{"Monthly", "Hourly"} {
		for _, mutation := range []string{"NULL summary", "absent month", "absent row", "duplicate slot", "negative row", "nonfinite row", "wrong interval", "wrong days", "wrong environment", "native zero changed", "wrong units", "offsetting rows"} {
			t.Run(frequency+"/"+mutation, func(t *testing.T) {
				source, rows := epathSQLPoolSourceHandObservation(1, "Indoor Pool Water Heating Rate", "Test Pool", "W", frequency, false, 0)
				switch mutation {
				case "NULL summary":
					source.Months[0].EnergyKWh = nil
				case "absent month":
					source.Months = source.Months[1:]
				case "absent row":
					rows = rows[1:]
				case "duplicate slot":
					rows[1] = rows[0]
				case "negative row":
					rows[0].NativeValue = -1
				case "nonfinite row":
					rows[0].NativeValue = math.Inf(1)
				case "wrong interval":
					rows[0].IntervalMinutes /= 2
				case "wrong days":
					rows[0].SimulationDays++
				case "wrong environment":
					rows[0].EnvironmentIndex++
				case "native zero changed":
					*source.Months[0].EnergyKWh = 1e-13
				case "wrong units":
					source.SourceUnit = "kWh"
				case "offsetting rows":
					rows[0].NativeValue, rows[1].NativeValue = -1, 1
				}
				if _, _, err := epathSQLValidatePoolNativeRows(source, epathSQLPoolSourceHandWeather(), rows); err == nil {
					t.Fatal("unknown/invalid native row was repaired into observed zero")
				}
			})
		}
	}
}

func epathSQLPoolNativeAuditDB(t *testing.T, source epathRealSQLSource, rows []epathSQLPoolNativeRow) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "native.sql"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, query := range []string{
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER,EnvironmentName TEXT,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(3,'Annual',3)`,
		`CREATE TABLE Time(TimeIndex INTEGER,EnvironmentPeriodIndex INTEGER,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,"Interval" REAL,IntervalType INTEGER,SimulationDays INTEGER,WarmupFlag INTEGER)`,
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER,Name TEXT,KeyValue TEXT,IsMeter INTEGER,ReportingFrequency TEXT,Units TEXT,Type TEXT,TimestepType TEXT)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	dictionary := epathSQLPoolHandDictionary(source)
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,?,?,?,?,?)`, source.DictionaryIndex, source.Name, source.KeyValue, source.IsMeter, source.ReportingFrequency, source.SourceUnit, dictionary.Type, dictionary.TimestepType); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if _, err := tx.Exec(`INSERT INTO Time VALUES(?,?,?,?,?,?,?,?,?,?,NULL)`, row.TimeIndex, row.EnvironmentIndex, row.Year, row.Month, row.Day, row.Hour, row.Minute, row.IntervalMinutes, row.IntervalType, row.SimulationDays); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,?,?)`, row.TimeIndex, source.DictionaryIndex, row.NativeValue); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestEnergyPathRealSQLPoolNativeAuditQueriesActualRows(t *testing.T) {
	for _, frequency := range []string{"Monthly", "Hourly"} {
		source, rows := epathSQLPoolSourceHandObservation(1, "Indoor Pool Water Heating Rate", "Test Pool", "W", frequency, false, 0)
		db := epathSQLPoolNativeAuditDB(t, source, rows)
		actual, err := epathSQLAuditPoolNative(db, source, epathSQLPoolSourceHandWeather())
		if err != nil || !reflect.DeepEqual(actual, rows) {
			t.Fatalf("native zero rows rejected: %v", err)
		}
		dictionary, err := epathSQLReadPoolNativeDictionary(db, source)
		if err != nil || dictionary != epathSQLPoolHandDictionary(source) {
			t.Fatalf("native aggregation/timestep metadata not retained: %v", err)
		}
		for _, query := range []string{
			`UPDATE ReportData SET Value=NULL WHERE ReportDataIndex=1`,
			`UPDATE ReportData SET Value=-1 WHERE ReportDataIndex=1`,
			`UPDATE Time SET "Interval"=NULL WHERE TimeIndex=(SELECT TimeIndex FROM ReportData WHERE ReportDataIndex=1)`,
			`INSERT INTO ReportDataDictionary SELECT 999,Name,KeyValue,IsMeter,ReportingFrequency,Units,Type,TimestepType FROM ReportDataDictionary LIMIT 1`,
			`UPDATE ReportDataDictionary SET Type='Sum'`,
			`UPDATE ReportDataDictionary SET Type=NULL`,
			`UPDATE ReportDataDictionary SET TimestepType='Zone'`,
			`UPDATE ReportDataDictionary SET TimestepType=NULL`,
			`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,ReportDataDictionaryIndex,Value FROM ReportData LIMIT 1`,
			`DELETE FROM ReportData WHERE ReportDataIndex=1`,
		} {
			if _, err := db.Exec(`SAVEPOINT mutation`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(query); err != nil {
				t.Fatal(err)
			}
			if _, err := epathSQLAuditPoolNative(db, source, epathSQLPoolSourceHandWeather()); err == nil {
				t.Fatalf("aggregate-compatible corruption accepted: %s", query)
			}
			if _, err := db.Exec(`ROLLBACK TO mutation`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`RELEASE mutation`); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestEnergyPathRealSQLPoolNativeMeterHasNoReportingOwner(t *testing.T) {
	source, rows := epathSQLPoolSourceHandObservation(1, "Pumps:Electricity", "", "J", "Monthly", true, 7)
	db := epathSQLPoolNativeAuditDB(t, source, rows)
	if _, err := db.Exec(`UPDATE ReportDataDictionary SET KeyValue=NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err := epathSQLAuditPoolNative(db, source, epathSQLPoolSourceHandWeather()); err != nil {
		t.Fatalf("native unkeyed meter NULL was mistaken for missing energy: %v", err)
	}
	for _, query := range []string{`UPDATE ReportDataDictionary SET Type='Avg'`, `UPDATE ReportDataDictionary SET TimestepType='HVAC System'`} {
		if _, err := db.Exec(`SAVEPOINT dictionary_mutation`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
		if _, err := epathSQLAuditPoolNative(db, source, epathSQLPoolSourceHandWeather()); err == nil {
			t.Fatal("parent meter dictionary borrowed component aggregation/timestep semantics")
		}
		if _, err := db.Exec(`ROLLBACK TO dictionary_mutation`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`RELEASE dictionary_mutation`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE ReportDataDictionary SET KeyValue='CW Pump'`); err != nil {
		t.Fatal(err)
	}
	if _, err := epathSQLAuditPoolNative(db, source, epathSQLPoolSourceHandWeather()); err == nil {
		t.Fatal("broad meter acquired an invented component owner")
	}
	component, componentRows := epathSQLPoolSourceHandObservation(2, "Pump Electricity Energy", "CW Pump", "J", "Monthly", false, 5)
	componentDB := epathSQLPoolNativeAuditDB(t, component, componentRows)
	if _, err := componentDB.Exec(`UPDATE ReportDataDictionary SET KeyValue=NULL`); err != nil {
		t.Fatal(err)
	}
	if _, err := epathSQLAuditPoolNative(componentDB, component, epathSQLPoolSourceHandWeather()); err == nil {
		t.Fatal("unkeyed component borrowed broad-meter key semantics")
	}
}

func epathSQLPoolHandRequest(source epathRealSQLSource, zone string) PurposeOutputObject {
	output := PurposeOutputObject{ObjectType: "Output:Variable", VariableName: source.Name, KeyValue: source.KeyValue, ScopeZoneName: zone, ReportingFrequency: source.ReportingFrequency, State: "temporary", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Fields: []idf.OutputFieldValue{{Value: source.KeyValue}, {Value: source.Name}, {Value: source.ReportingFrequency}}}
	if source.IsMeter {
		output.ObjectType, output.VariableName, output.KeyValue = "Output:Meter", "", source.Name
		output.Fields = []idf.OutputFieldValue{{Value: source.Name}, {Value: source.ReportingFrequency}}
	}
	return output
}

func TestEnergyPathRealSQLPoolSourceRequestExactOwnerAndFrequency(t *testing.T) {
	for _, meter := range []bool{false, true} {
		name, key, zone := "Indoor Pool Water Heating Energy", "Test Pool", "Office"
		statement := fmt.Sprintf("Output:Variable,%s,%s,Monthly;", key, name)
		if meter {
			name, key, zone = "Pumps:Electricity", "", ""
			statement = "Output:Meter,Pumps:Electricity,Monthly;"
		}
		original, _ := idf.Parse("")
		executed, err := idf.Parse(statement)
		if err != nil {
			t.Fatal(err)
		}
		source := epathRealSQLSource{Name: name, KeyValue: key, ReportingFrequency: "Monthly", IsMeter: meter}
		plan := PurposeRunPlan{BasicEnergyDetail: "energy_path", OutputObjects: []PurposeOutputObject{epathSQLPoolHandRequest(source, zone)}}
		index, err := epathSQLPoolSourceRequest(original, executed, &plan, source, zone)
		if err != nil || index == nil || *index != executed.Objects[0].Index {
			t.Fatalf("exact native request rejected: %v", err)
		}
		before, _ := json.Marshal(plan)
		for _, mutation := range []string{"scope", "frequency", "purpose", "wildcard", "field mismatch", "index", "state", "duplicate", "no executed"} {
			var changed PurposeRunPlan
			if err := json.Unmarshal(before, &changed); err != nil {
				t.Fatal(err)
			}
			run := executed
			switch mutation {
			case "scope":
				changed.OutputObjects[0].ScopeZoneName = "Other"
			case "frequency":
				changed.OutputObjects[0].ReportingFrequency = "Hourly"
			case "purpose":
				changed.OutputObjects[0].PurposeIDs = nil
			case "wildcard":
				if meter {
					changed.OutputObjects[0].KeyValue = "Electricity:Facility"
				} else {
					changed.OutputObjects[0].KeyValue = "*"
				}
			case "field mismatch":
				changed.OutputObjects[0].Fields[0].Value = "Foreign"
			case "index":
				bad := 999
				changed.OutputObjects[0].ObjectIndex = &bad
			case "state":
				changed.OutputObjects[0].State = "existing"
			case "duplicate":
				changed.OutputObjects = append(changed.OutputObjects, changed.OutputObjects[0])
			case "no executed":
				run = original
			}
			if _, err := epathSQLPoolSourceRequest(original, run, &changed, source, zone); err == nil {
				t.Fatalf("request proof borrowed %s", mutation)
			}
		}
		after, _ := json.Marshal(plan)
		if string(before) != string(after) {
			t.Fatal("request audit mutated source evidence")
		}
	}
}

func TestEnergyPathRealSQLPoolSourceRequestPreservesOriginalWildcardAndAmbiguity(t *testing.T) {
	original, err := idf.Parse("Output:Variable,*,Indoor Pool Water Heating Rate,Hourly;")
	if err != nil {
		t.Fatal(err)
	}
	exact := "Output:Variable,Test Pool,Indoor Pool Water Heating Rate,Hourly;"
	run, err := idf.Parse(original.String() + "\n" + exact)
	if err != nil {
		t.Fatal(err)
	}
	source := epathRealSQLSource{Name: "Indoor Pool Water Heating Rate", KeyValue: "Test Pool", ReportingFrequency: "Hourly"}
	plan := PurposeRunPlan{BasicEnergyDetail: "energy_path", OutputObjects: []PurposeOutputObject{epathSQLPoolHandRequest(source, "Office")}}
	index, err := epathSQLPoolSourceRequest(original, run, &plan, source, "Office")
	if err != nil || index == nil || *index != run.Objects[1].Index {
		t.Fatalf("native exact request borrowed original wildcard opener: %v", err)
	}
	ambiguous, err := idf.Parse(run.String() + "\n" + exact)
	if err != nil {
		t.Fatal(err)
	}
	index, err = epathSQLPoolSourceRequest(original, ambiguous, &plan, source, "Office")
	if err != nil || index != nil {
		t.Fatal("duplicate executed exact opener was guessed")
	}
	deleted, err := idf.Parse(exact)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := epathSQLPoolSourceRequest(original, deleted, &plan, source, "Office"); err == nil {
		t.Fatal("request proof accepted removal of the original wildcard")
	}
}

func TestEnergyPathRealSQLPoolSourceExecutedOwnerIndependentParsedIndices(t *testing.T) {
	originalText := epathSQLPoolOriginalFixture(t)
	proofs, err := epathSQLValidatePoolOriginal(originalText, []epathRealSQLPoolSystem{epathSQLPoolOriginalDeclaration()})
	if err != nil || len(proofs) != 1 {
		t.Fatalf("untouched original proof: %v", err)
	}
	original, err := idf.Parse(originalText)
	if err != nil {
		t.Fatal(err)
	}
	// Two unrelated, real output objects precede the unchanged physical model.
	// The expected displacement comes from this hand document construction,
	// never an observed capture's apparent original/executed index offset.
	executedText := "Output:Variable,*,Zone Mean Air Temperature,Monthly;\nOutput:Variable,*,Zone Air Relative Humidity,Hourly;\n" + originalText
	for _, owner := range proofs[0].Owners {
		run, err := idf.Parse(executedText)
		if err != nil {
			t.Fatal(err)
		}
		index, err := epathSQLPoolExecutedOwner(original, run, owner)
		if err != nil || index != owner.ObjectIndex+2 {
			t.Fatalf("separately parsed %s/%s owner did not preserve exact fields across two real prepended objects: index=%d original=%d err=%v", owner.ObjectType, owner.ObjectName, index, owner.ObjectIndex, err)
		}
		for _, mutation := range []string{"physical field", "duplicate owner"} {
			changed, err := idf.Parse(executedText)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for n, object := range changed.Objects {
				if object.Index != index {
					continue
				}
				found = true
				if mutation == "physical field" {
					if len(object.Fields) < 2 {
						t.Fatal("native owner fixture lacks a physical field")
					}
					changed.Objects[n].Fields[1].Value = "different-native-physical-field"
				} else {
					changed.Objects = append(changed.Objects, object)
				}
				break
			}
			if !found {
				t.Fatal("newly parsed owner index is absent")
			}
			// Parse the changed text again: this exercises real lexical object
			// identity and fields, not a forged ExecutedOwnerIndex proof member.
			reparsed, err := idf.Parse(changed.String())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := epathSQLPoolExecutedOwner(original, reparsed, owner); err == nil {
				t.Fatalf("accepted %s for %s/%s", mutation, owner.ObjectType, owner.ObjectName)
			}
		}
	}
}

// Full compiler uses the untouched original topology and a newly created hand
// SQL database. The real saved capture is not needed to test its binding path.
func TestEnergyPathRealSQLPoolSourceCompilerOriginalAndExecutedBinding(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "5ZoneSwimmingPoolZoneMultipliers.idf"))
	if err != nil {
		t.Fatal(err)
	}
	proofs, err := epathSQLValidatePoolOriginal(string(data), []epathRealSQLPoolSystem{epathSQLPoolOriginalDeclaration()})
	if err != nil || len(proofs) != 1 {
		t.Fatalf("untouched original proof: %v", err)
	}
	hand := epathSQLPoolSourceHandFramesForOriginal(t, proofs[0])
	original, err := idf.Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	executedText := string(data)
	plan := PurposeRunPlan{BasicEnergyDetail: "energy_path"}
	var sources []epathRealSQLSource
	for _, id := range epathSQLPoolSourceIDs(hand) {
		sources = append(sources, hand.Sources[id].Source)
	}
	for _, name := range []string{"Heating:NaturalGas", "Heating:Electricity", "Pumps:Electricity"} {
		sources = append(sources, hand.Parents[name].Source)
	}
	for _, source := range sources {
		exact, _ := epathSQLPoolOutputCensus(original, source.Name, source.KeyValue, source.ReportingFrequency, source.IsMeter)
		output := epathSQLPoolHandRequest(source, "")
		if strings.EqualFold(source.KeyValue, proofs[0].Declaration.PoolName) {
			output.ScopeZoneName = proofs[0].Declaration.ZoneName
		}
		if len(exact) > 0 {
			output.State = "existing"
			for index := range exact {
				copy := index
				output.ObjectIndex = &copy
				break
			}
		} else if source.IsMeter {
			executedText += fmt.Sprintf("\nOutput:Meter,%s,%s;\n", source.Name, source.ReportingFrequency)
		} else {
			executedText += fmt.Sprintf("\nOutput:Variable,%s,%s,%s;\n", source.KeyValue, source.Name, source.ReportingFrequency)
		}
		plan.OutputObjects = append(plan.OutputObjects, output)
	}
	path := filepath.Join(t.TempDir(), "all-native.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER,EnvironmentName TEXT,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(3,'Annual',3)`,
		`CREATE TABLE Time(TimeIndex INTEGER,EnvironmentPeriodIndex INTEGER,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,"Interval" REAL,IntervalType INTEGER,SimulationDays INTEGER,WarmupFlag INTEGER)`,
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER,Name TEXT,KeyValue TEXT,IsMeter INTEGER,ReportingFrequency TEXT,Units TEXT,IndexGroup TEXT,Type TEXT,TimestepType TEXT)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
	} {
		if _, err := db.Exec(query); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	_, monthlyRows := epathSQLPoolSourceHandObservation(1, "", "", "J", "Monthly", false, 0)
	_, hourlyRows := epathSQLPoolSourceHandObservation(1, "", "", "J", "Hourly", false, 0)
	for _, row := range append(monthlyRows, hourlyRows...) {
		if _, err := tx.Exec(`INSERT INTO Time VALUES(?,?,?,?,?,?,?,?,?,?,NULL)`, row.TimeIndex, row.EnvironmentIndex, row.Year, row.Month, row.Day, row.Hour, row.Minute, row.IntervalMinutes, row.IntervalType, row.SimulationDays); err != nil {
			tx.Rollback()
			db.Close()
			t.Fatal(err)
		}
	}
	for _, source := range sources {
		dictionary := epathSQLPoolHandDictionary(source)
		if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,?,?,?,'HVAC',?,?)`, source.DictionaryIndex, source.Name, source.KeyValue, source.IsMeter, source.ReportingFrequency, source.SourceUnit, dictionary.Type, dictionary.TimestepType); err != nil {
			tx.Rollback()
			db.Close()
			t.Fatal(err)
		}
		rows := hand.Sources[source.DictionaryIndex].Rows
		if source.IsMeter {
			rows = hand.Parents[source.Name].Rows
		}
		power := rows[0].EnergyKWh / (rows[0].IntervalMinutes / 60)
		kind, expression := 3, `(? * ("Interval"/60) * 3600000)`
		if source.ReportingFrequency == "Hourly" {
			kind = 1
		}
		if source.SourceUnit == "W" {
			expression = `(? * 1000)`
		}
		query := `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,?,` + expression + ` FROM Time WHERE IntervalType=?`
		if _, err := tx.Exec(query, source.DictionaryIndex, power, kind); err != nil {
			tx.Rollback()
			db.Close()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	observed.originalText, observed.executedText, observed.outputPlan = string(data), executedText, &plan
	compiled, err := epathCompileSQLPoolSourceFrames(observed, proofs[0], hand.Precision)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Sources) != 28 || len(compiled.Parents) != 3 {
		t.Fatal("full native original/request/SQL compiler lost a required source")
	}
	for _, identity := range compiled.Sources {
		if !identity.RequestBound || identity.OutputObjectIndex == nil || identity.ExecutedOwnerIndex != identity.Spec.Owner.ObjectIndex {
			t.Fatal("native proof confused component identity with exact executed output")
		}
	}
	changed := proofs[0]
	changed.Owners = append([]epathSQLPoolOriginalOwner(nil), changed.Owners...)
	changed.Owners[0].ObjectIndex++
	if _, err := epathCompileSQLPoolSourceFrames(observed, changed, hand.Precision); err == nil {
		t.Fatal("caller supplied a fabricated original owner proof")
	}
	wrongPlan := plan
	wrongPlan.OutputObjects = append([]PurposeOutputObject(nil), plan.OutputObjects...)
	wrongPlan.OutputObjects = wrongPlan.OutputObjects[1:]
	observed.outputPlan = &wrongPlan
	if _, err := epathCompileSQLPoolSourceFrames(observed, proofs[0], hand.Precision); err == nil {
		t.Fatal("missing exact native request accepted from existing SQL alone")
	}
}
