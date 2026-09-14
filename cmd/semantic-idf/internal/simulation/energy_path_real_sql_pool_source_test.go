package simulation

// Independent acceptance support. Independent native source compiler; no production classifier,
// reader, inventory, multiplier, allocation or candidate quantity is authority.
import (
	"database/sql"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathSQLPoolNativeSpec struct {
	ID, EnergyName, RateName           string
	Role, EndUse, Carrier, SourceGroup string
	Owner                              epathSQLPoolOriginalOwner
}

type epathSQLPoolNativeRow struct {
	TimeIndex, EnvironmentIndex, Year, Month, Day, Hour, Minute, IntervalType, SimulationDays int
	IntervalMinutes, NativeValue, EnergyKWh                                                   float64
}

type epathSQLPoolNativeDictionary struct {
	DictionaryIndex    int
	Type, TimestepType string
}

type epathSQLPoolSourceIdentity struct {
	Spec                                     epathSQLPoolNativeSpec
	Source                                   epathRealSQLSource // RawSum retains J or sum-of-reported-W, NOT kWh.
	Dictionary                               epathSQLPoolNativeDictionary
	Precision                                epathRealSQLPrecision
	Canonical, BudgetAuthority, RequestBound bool
	ExecutedOwnerIndex                       int
	OutputObjectIndex                        *int // Separate output request, never confused with owner.
	Rows                                     []epathSQLPoolNativeRow
	NativeRawMonthly, NativeEnergyMonthly    [12]float64
}

type epathSQLPoolSourceFamily struct {
	Spec         epathSQLPoolNativeSpec
	CanonicalID  int
	CompanionIDs []int
	Monthly      []epathSQLQuantity // Monthly Energy only; transport interval bounds.
	Annual       epathSQLQuantity   // Native integral, not sum of rounded chart samples.
}

type epathSQLPoolParentClosure struct {
	EndUse, Carrier      string
	Source               epathRealSQLSource
	Dictionary           epathSQLPoolNativeDictionary
	Rows                 []epathSQLPoolNativeRow
	NativeMonthly        [12]float64
	ConstituentFamilyIDs []string
	ConstituentSourceIDs []int
	Monthly              []epathSQLQuantity
}

type epathSQLPoolSourceFrames struct {
	Original  epathSQLPoolOriginalProof
	Weather   epathRealSQLWeather
	Precision epathRealSQLPrecision
	Families  map[string]epathSQLPoolSourceFamily
	Sources   map[int]epathSQLPoolSourceIdentity
	Parents   map[string]epathSQLPoolParentClosure
}

func epathSQLPoolSourceSpecs(original epathSQLPoolOriginalProof) ([]epathSQLPoolNativeSpec, error) {
	declaration := original.Declaration
	one := func(typ, name string) (epathSQLPoolOriginalOwner, error) {
		var found epathSQLPoolOriginalOwner
		count := 0
		for _, owner := range original.Owners {
			if strings.EqualFold(owner.ObjectType, typ) && strings.EqualFold(owner.ObjectName, name) {
				found, count = owner, count+1
			}
		}
		if count != 1 || name == "" || found.ObjectIndex < 0 {
			return found, fmt.Errorf("Pool native source needs one independently proved typed original owner: %s/%s", typ, name)
		}
		return found, nil
	}
	pool, err := one("SwimmingPool:Indoor", declaration.PoolName)
	if err != nil {
		return nil, err
	}
	boiler, err := one("Boiler:HotWater", declaration.BoilerName)
	if err != nil {
		return nil, err
	}
	hw, err := one("Pump:VariableSpeed", declaration.HotWaterPumpName)
	if err != nil {
		return nil, err
	}
	cw, err := one("Pump:VariableSpeed", declaration.ChilledWaterPumpName)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(boiler.FuelType, "NaturalGas") || !strings.EqualFold(pool.ZoneName, declaration.ZoneName) || !strings.EqualFold(pool.PlantLoopName, declaration.HotWaterLoopName) || !strings.EqualFold(boiler.PlantLoopName, declaration.HotWaterLoopName) || !strings.EqualFold(hw.PlantLoopName, declaration.HotWaterLoopName) || !strings.EqualFold(cw.PlantLoopName, declaration.ChilledWaterLoopName) || strings.EqualFold(hw.ObjectName, cw.ObjectName) {
		return nil, fmt.Errorf("Pool native family crossed its original fuel, surface owner or separate plant boundary")
	}
	return []epathSQLPoolNativeSpec{
		{ID: "pool.water_heating", EnergyName: "Indoor Pool Water Heating Energy", RateName: "Indoor Pool Water Heating Rate", Role: "thermal_context", SourceGroup: "loads", Owner: pool},
		{ID: "boiler.heating_output", EnergyName: "Boiler Heating Energy", RateName: "Boiler Heating Rate", Role: "thermal_context", SourceGroup: "loads", Owner: boiler},
		{ID: "boiler.natural_gas", EnergyName: "Boiler NaturalGas Energy", RateName: "Boiler NaturalGas Rate", Role: "purchased_constituent", EndUse: "heating", Carrier: "natural_gas", SourceGroup: "endUses", Owner: boiler},
		{ID: "boiler.ancillary_natural_gas", EnergyName: "Boiler Ancillary NaturalGas Energy", RateName: "Boiler Ancillary NaturalGas Rate", Role: "purchased_constituent", EndUse: "heating", Carrier: "natural_gas", SourceGroup: "endUses", Owner: boiler},
		{ID: "boiler.ancillary_electricity", EnergyName: "Boiler Ancillary Electricity Energy", RateName: "Boiler Ancillary Electricity Rate", Role: "purchased_constituent", EndUse: "heating", Carrier: "electricity", SourceGroup: "endUses", Owner: boiler},
		{ID: "pump.hw.electricity", EnergyName: "Pump Electricity Energy", RateName: "Pump Electricity Rate", Role: "purchased_constituent", EndUse: "pumps", Carrier: "electricity", SourceGroup: "endUses", Owner: hw},
		{ID: "pump.cw.electricity", EnergyName: "Pump Electricity Energy", RateName: "Pump Electricity Rate", Role: "purchased_constituent", EndUse: "pumps", Carrier: "electricity", SourceGroup: "endUses", Owner: cw},
	}, nil
}

func epathSQLPoolSourcePrecision(p epathRealSQLPrecision) error {
	if p.DecimalPlaces != 3 || p.SourceStages < 1 || p.SourceStages > 3 || p.ContributionStages < 1 || p.ContributionStages > 3 {
		return fmt.Errorf("Pool source precision must retain the existing independent interval contract")
	}
	return nil
}

func epathSQLPoolSameNative(a, b float64) bool {
	return epathOracleFinite(a) && epathOracleFinite(b) && (a == 0) == (b == 0) && epathSQLFanPoolNear(a, b)
}

func epathSQLPoolSameMonths(a, b [12]float64) bool {
	for month := range a {
		if !epathSQLPoolSameNative(a[month], b[month]) {
			return false
		}
	}
	return true
}

func epathSQLValidatePoolNativeDictionary(source epathRealSQLSource, dictionary epathSQLPoolNativeDictionary) error {
	wantType, wantStep := "Sum", "HVAC System"
	if source.SourceUnit == "W" {
		wantType = "Avg"
	}
	if source.IsMeter {
		wantStep = "Zone"
	}
	if source.DictionaryIndex <= 0 || dictionary.DictionaryIndex != source.DictionaryIndex || source.SourceUnit != "J" && source.SourceUnit != "W" || source.IsMeter && source.SourceUnit != "J" || dictionary.Type != wantType || dictionary.TimestepType != wantStep {
		return fmt.Errorf("Pool dictionary must prove native E Sum/J or R Avg/W; component HVAC System and unkeyed parent Zone are distinct")
	}
	return nil
}

func epathSQLReadPoolNativeDictionary(db *sql.DB, source epathRealSQLSource) (epathSQLPoolNativeDictionary, error) {
	var dictionary epathSQLPoolNativeDictionary
	if source.DictionaryIndex <= 0 || source.Name == "" || source.ReportingFrequency != "Monthly" && source.ReportingFrequency != "Hourly" || source.SourceUnit != "J" && source.SourceUnit != "W" || source.IsMeter && (source.SourceUnit != "J" || source.ReportingFrequency != "Monthly" || source.KeyValue != "") {
		return dictionary, fmt.Errorf("Pool native source has an unsupported exact dictionary contract")
	}
	// Native meters have no reporting owner: EnergyPlus stores KeyValue NULL.
	// The existing independent evidence reader represents only that absence as
	// an empty key. Component keys remain exact nonempty original owner names.
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,Name,COALESCE(KeyValue,''),IsMeter,ReportingFrequency,Units,Type,TimestepType FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=? OR (Name=? COLLATE NOCASE AND COALESCE(KeyValue,'')=? COLLATE NOCASE AND ReportingFrequency=? COLLATE NOCASE)`, source.DictionaryIndex, source.Name, source.KeyValue, source.ReportingFrequency)
	if err != nil {
		return dictionary, err
	}
	count := 0
	for rows.Next() {
		var index, meter int
		var name, key, frequency, unit string
		if err := rows.Scan(&index, &name, &key, &meter, &frequency, &unit, &dictionary.Type, &dictionary.TimestepType); err != nil {
			rows.Close()
			return dictionary, err
		}
		dictionary.DictionaryIndex = index
		count++
		if index != source.DictionaryIndex || name != source.Name || !strings.EqualFold(key, source.KeyValue) || meter != 0 && meter != 1 || (meter == 1) != source.IsMeter || frequency != source.ReportingFrequency || unit != source.SourceUnit {
			rows.Close()
			return dictionary, fmt.Errorf("duplicate or conflicting native Pool dictionary")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return dictionary, err
	}
	if count != 1 {
		return dictionary, fmt.Errorf("Pool source lacks one exact native dictionary")
	}
	if err := epathSQLValidatePoolNativeDictionary(source, dictionary); err != nil {
		return dictionary, err
	}
	return dictionary, nil
}

// This row audit also audits the three exact native parent meters. It does not
// derive a component from the parent or repair a source summary supplied by SQL.
func epathSQLAuditPoolNative(db *sql.DB, source epathRealSQLSource, weather epathRealSQLWeather) ([]epathSQLPoolNativeRow, error) {
	if _, err := epathSQLReadPoolNativeDictionary(db, source); err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT t.TimeIndex,t.EnvironmentPeriodIndex,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,t.SimulationDays,r.Value FROM ReportData r JOIN Time t USING(TimeIndex) JOIN EnvironmentPeriods e USING(EnvironmentPeriodIndex) WHERE r.ReportDataDictionaryIndex=? AND e.EnvironmentType=3 AND `+epathOracleNonWarmupSQL+` ORDER BY t.TimeIndex`, source.DictionaryIndex)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []epathSQLPoolNativeRow
	for rows.Next() {
		var row epathSQLPoolNativeRow
		var minutes, value sql.NullFloat64
		var days sql.NullInt64
		if err := rows.Scan(&row.TimeIndex, &row.EnvironmentIndex, &row.Year, &row.Month, &row.Day, &row.Hour, &row.Minute, &minutes, &row.IntervalType, &days, &value); err != nil {
			return nil, err
		}
		if !minutes.Valid || !value.Valid || !days.Valid {
			return nil, fmt.Errorf("Pool native interval/observation is absent, not zero")
		}
		row.IntervalMinutes, row.NativeValue, row.SimulationDays = minutes.Float64, value.Float64, int(days.Int64)
		row.EnergyKWh = row.NativeValue / 3600000
		if source.SourceUnit == "W" {
			row.EnergyKWh = row.NativeValue * (row.IntervalMinutes / 60) / 1000
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, _, err := epathSQLValidatePoolNativeRows(source, weather, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Pure counterpart revalidates retained evidence before consumer arithmetic.
// Exact native zeros have zero tolerance; NULL, a missing month and a missing
// Hourly calendar slot never acquire a value through aggregate coincidence.
func epathSQLValidatePoolNativeRows(source epathRealSQLSource, weather epathRealSQLWeather, rows []epathSQLPoolNativeRow) (raw, energy [12]float64, err error) {
	fail := func(message string) ([12]float64, [12]float64, error) {
		return raw, energy, fmt.Errorf("Pool native source: %s", message)
	}
	if weather.Year != 2017 || weather.EnvironmentIndex <= 0 || len(weather.Months) != 12 || weather.CoverageBasis == "" || source.DictionaryIndex <= 0 || source.MissingRows != 0 || len(source.Months) != 12 || source.RawSum == nil || source.EnergyKWh == nil || source.SourceUnit != "J" && source.SourceUnit != "W" || source.ReportingFrequency != "Monthly" && source.ReportingFrequency != "Hourly" {
		return fail("incomplete controlled annual/source identity")
	}
	hourly := source.ReportingFrequency == "Hourly"
	wantRows := 12
	if hourly {
		wantRows = 8760
	}
	if len(rows) != wantRows || source.Rows != wantRows {
		return fail("missing or repeated full-calendar observation")
	}
	seen, seenTimes, perMonth := map[int]bool{}, map[int]bool{}, [12]int{}
	for _, row := range rows {
		date := time.Date(row.Year, time.Month(row.Month), row.Day, 0, 0, 0, 0, time.UTC)
		if row.TimeIndex <= 0 || seenTimes[row.TimeIndex] || row.EnvironmentIndex != weather.EnvironmentIndex || row.Year != weather.Year || row.Month < 1 || row.Month > 12 || row.Day < 1 || date.Month() != time.Month(row.Month) || date.Day() != row.Day || row.Minute != 0 || row.SimulationDays != date.YearDay() || !epathOracleFinite(row.NativeValue) || row.NativeValue < 0 || !epathOracleFinite(row.IntervalMinutes) || row.IntervalMinutes <= 0 || !epathOracleFinite(row.EnergyKWh) || row.EnergyKWh < 0 {
			return fail("unknown, negative, duplicated or invalid native interval")
		}
		seenTimes[row.TimeIndex] = true
		slot := row.Month - 1
		last := time.Date(weather.Year, time.Month(row.Month+1), 0, 0, 0, 0, 0, time.UTC).Day()
		if hourly {
			slot = (date.YearDay()-1)*24 + row.Hour - 1
			if row.IntervalType != 1 || row.Hour < 1 || row.Hour > 24 || row.IntervalMinutes != 60 || slot < 0 || slot >= 8760 {
				return fail("Hourly row escaped its actual hourly interval")
			}
		} else if row.IntervalType != 3 || row.Hour != 24 || row.Day != last || row.IntervalMinutes != float64(last*1440) {
			return fail("Monthly row is not one complete actual month")
		}
		if seen[slot] {
			return fail("duplicate calendar slot")
		}
		seen[slot] = true
		value := row.NativeValue / 3600000
		if source.SourceUnit == "W" {
			value = row.NativeValue * (row.IntervalMinutes / 60) / 1000
		}
		if !epathSQLPoolSameNative(value, row.EnergyKWh) {
			return fail("native units/duration were changed during normalization")
		}
		raw[row.Month-1] += row.NativeValue
		energy[row.Month-1] += value
		perMonth[row.Month-1]++
	}
	totalRaw, totalEnergy := 0.0, 0.0
	for month, bucket := range source.Months {
		if weather.Months[month] != month+1 || bucket.Month != month+1 || bucket.Rows != perMonth[month] || bucket.MissingRows != 0 || bucket.RawSum == nil || bucket.EnergyKWh == nil || !epathSQLPoolSameNative(raw[month], *bucket.RawSum) || !epathSQLPoolSameNative(energy[month], *bucket.EnergyKWh) {
			return fail("reported monthly summary differs from native rows or knownness")
		}
		totalRaw += raw[month]
		totalEnergy += energy[month]
	}
	if !epathSQLPoolSameNative(totalRaw, *source.RawSum) || !epathSQLPoolSameNative(totalEnergy, *source.EnergyKWh) {
		return fail("annual summary differs from native monthly sums")
	}
	return raw, energy, nil
}

func epathSQLPoolOutputCensus(doc idf.Document, name, key, frequency string, meter bool) (exact, wildcard map[int]bool) {
	exact, wildcard = map[int]bool{}, map[int]bool{}
	for _, object := range doc.Objects {
		if meter {
			if strings.EqualFold(object.Type, "Output:Meter") && strings.EqualFold(epathSQLSharedField(object, 0), name) && strings.EqualFold(epathSQLSharedField(object, 1), frequency) {
				exact[object.Index] = true
			}
			continue
		}
		if !strings.EqualFold(object.Type, "Output:Variable") || !strings.EqualFold(epathSQLSharedField(object, 1), name) || !strings.EqualFold(epathSQLSharedField(object, 2), frequency) {
			continue
		}
		owner := epathSQLSharedField(object, 0)
		if strings.EqualFold(owner, key) {
			exact[object.Index] = true
		} else if owner == "" || owner == "*" {
			wildcard[object.Index] = true
		}
	}
	return exact, wildcard
}

func epathSQLPoolSourceRequest(original, executed idf.Document, plan *PurposeRunPlan, source epathRealSQLSource, scopeZone string) (*int, error) {
	if plan == nil || plan.BasicEnergyDetail != "energy_path" {
		return nil, fmt.Errorf("Pool proof needs its actual Energy Path request plan")
	}
	oldExact, oldWild := epathSQLPoolOutputCensus(original, source.Name, source.KeyValue, source.ReportingFrequency, source.IsMeter)
	runExact, runWild := epathSQLPoolOutputCensus(executed, source.Name, source.KeyValue, source.ReportingFrequency, source.IsMeter)
	if len(runExact) < len(oldExact) || len(runWild) < len(oldWild) {
		return nil, fmt.Errorf("executed Pool request erased an original output identity")
	}
	count := 0
	for _, output := range plan.OutputObjects {
		requestName := output.VariableName
		if source.IsMeter {
			requestName = output.KeyValue
		}
		if !strings.EqualFold(requestName, source.Name) || output.ReportingFrequency != source.ReportingFrequency {
			continue
		}
		if !source.IsMeter && !strings.EqualFold(output.KeyValue, source.KeyValue) {
			continue
		}
		count++
		typ := "Output:Variable"
		if source.IsMeter {
			typ = "Output:Meter"
		}
		basic := false
		for _, purpose := range output.PurposeIDs {
			basic = basic || purpose == SimulationPurposeBasicEnergy
		}
		if output.ObjectType != typ || !strings.EqualFold(output.ScopeZoneName, scopeZone) || !basic || output.State != "temporary" && output.State != "existing" {
			return nil, fmt.Errorf("Pool source request changed native type, owner scope, purpose or state")
		}
		wantFields := []string{source.KeyValue, source.Name, source.ReportingFrequency}
		if source.IsMeter {
			if output.VariableName != "" {
				return nil, fmt.Errorf("native meter request borrowed a variable identity")
			}
			wantFields = []string{source.Name, source.ReportingFrequency}
		}
		if len(output.Fields) != len(wantFields) {
			return nil, fmt.Errorf("Pool source request lacks exact native output fields")
		}
		for n, want := range wantFields {
			if !strings.EqualFold(strings.TrimSpace(output.Fields[n].Value), want) {
				return nil, fmt.Errorf("Pool request metadata disagrees with its native field positions")
			}
		}
		if output.ObjectIndex != nil && !runExact[*output.ObjectIndex] && !oldExact[*output.ObjectIndex] {
			return nil, fmt.Errorf("Pool plan borrowed an unrelated output opener")
		}
		if output.State == "temporary" && output.ObjectIndex != nil || output.State == "existing" && len(oldExact) == 0 {
			return nil, fmt.Errorf("Pool request state has no original identity proof")
		}
	}
	if count != 1 || len(runExact) == 0 {
		return nil, fmt.Errorf("Pool source requires one exact executed key/name/frequency request")
	}
	if len(runExact) != 1 {
		return nil, nil
	} // An ambiguous opener is not guessed.
	for index := range runExact {
		copy := index
		return &copy, nil
	}
	return nil, fmt.Errorf("unreachable Pool request census")
}

func epathSQLPoolExecutedOwner(original, executed idf.Document, owner epathSQLPoolOriginalOwner) (int, error) {
	old, err := (epathSQLSharedOriginal{doc: original}).one(owner.ObjectType, owner.ObjectName)
	if err != nil {
		return 0, err
	}
	run, err := (epathSQLSharedOriginal{doc: executed}).one(owner.ObjectType, owner.ObjectName)
	if err != nil {
		return 0, err
	}
	if old.Index != owner.ObjectIndex || len(old.Fields) != len(run.Fields) {
		return 0, fmt.Errorf("Pool original/executed owner shape changed")
	}
	for n := range old.Fields {
		if !strings.EqualFold(strings.TrimSpace(old.Fields[n].Value), strings.TrimSpace(run.Fields[n].Value)) {
			return 0, fmt.Errorf("Pool executed source owner changed an original physical field")
		}
	}
	return run.Index, nil
}

func epathSQLPoolSelectNative(observed []epathRealSQLSource, name, key, frequency, unit string, meter bool) (epathRealSQLSource, error) {
	var found epathRealSQLSource
	count := 0
	for _, source := range observed {
		if strings.EqualFold(source.Name, name) && strings.EqualFold(source.KeyValue, key) && strings.EqualFold(source.ReportingFrequency, frequency) {
			found, count = source, count+1
		}
	}
	if count != 1 || found.Name != name || found.ReportingFrequency != frequency || found.SourceUnit != unit || found.IsMeter != meter {
		return found, fmt.Errorf("Pool requires one observed native %s/%s/%s/%s, absent is not zero", key, name, frequency, unit)
	}
	return found, nil
}

func epathCompileSQLPoolSourceFrames(observed epathRealOracleEvidence, original epathSQLPoolOriginalProof, precision epathRealSQLPrecision) (epathSQLPoolSourceFrames, error) {
	out := epathSQLPoolSourceFrames{Original: original, Precision: precision, Families: map[string]epathSQLPoolSourceFamily{}, Sources: map[int]epathSQLPoolSourceIdentity{}, Parents: map[string]epathSQLPoolParentClosure{}}
	if err := epathSQLPoolSourcePrecision(precision); err != nil {
		return out, err
	}
	proofs, err := epathSQLValidatePoolOriginal(observed.originalText, []epathRealSQLPoolSystem{original.Declaration})
	if err != nil {
		return out, err
	}
	if len(proofs) != 1 || !reflect.DeepEqual(proofs[0], original) {
		return out, fmt.Errorf("Pool source compiler lost its independently validated original proof")
	}
	specs, err := epathSQLPoolSourceSpecs(original)
	if err != nil {
		return out, err
	}
	doc, err := idf.Parse(observed.originalText)
	if err != nil {
		return out, err
	}
	run, err := idf.Parse(observed.executedText)
	if err != nil || strings.TrimSpace(observed.executedText) == "" {
		return out, fmt.Errorf("Pool native proof requires the separately bound executed IDF: %v", err)
	}
	db, err := epathOpenOracleSQL(observed.sqlPath)
	if err != nil {
		return out, err
	}
	defer db.Close()
	out.Weather, err = epathReadOracleWeather(db)
	if err != nil {
		return out, err
	}
	for _, spec := range specs {
		family := epathSQLPoolSourceFamily{Spec: spec}
		executedOwner, err := epathSQLPoolExecutedOwner(doc, run, spec.Owner)
		if err != nil {
			return out, err
		}
		for _, selection := range []struct{ name, unit, frequency string }{{spec.EnergyName, "J", "Monthly"}, {spec.RateName, "W", "Monthly"}, {spec.EnergyName, "J", "Hourly"}, {spec.RateName, "W", "Hourly"}} {
			source, err := epathSQLPoolSelectNative(observed.Sources, selection.name, spec.Owner.ObjectName, selection.frequency, selection.unit, false)
			if err != nil {
				return out, err
			}
			if _, exists := out.Sources[source.DictionaryIndex]; exists {
				return out, fmt.Errorf("Pool family reused a native source identity")
			}
			zone := ""
			if spec.Owner.ObjectType == "SwimmingPool:Indoor" {
				zone = spec.Owner.ZoneName
			}
			opener, err := epathSQLPoolSourceRequest(doc, run, observed.outputPlan, source, zone)
			if err != nil {
				return out, err
			}
			dictionary, err := epathSQLReadPoolNativeDictionary(db, source)
			if err != nil {
				return out, err
			}
			rows, err := epathSQLAuditPoolNative(db, source, out.Weather)
			if err != nil {
				return out, err
			}
			raw, energy, err := epathSQLValidatePoolNativeRows(source, out.Weather, rows)
			if err != nil {
				return out, err
			}
			canonical := selection.name == spec.EnergyName && selection.frequency == "Monthly"
			identity := epathSQLPoolSourceIdentity{Spec: spec, Source: source, Dictionary: dictionary, Precision: precision, Canonical: canonical, BudgetAuthority: canonical && spec.Role == "purchased_constituent", RequestBound: true, ExecutedOwnerIndex: executedOwner, OutputObjectIndex: opener, Rows: rows, NativeRawMonthly: raw, NativeEnergyMonthly: energy}
			out.Sources[source.DictionaryIndex] = identity
			if canonical {
				family.CanonicalID = source.DictionaryIndex
				family.Monthly, err = epathSQLMonthly(source, precision)
				if err != nil {
					return out, err
				}
				family.Annual, err = epathSQLPoolSourceQuantity(identity, out.Weather)
				if err != nil {
					return out, err
				}
			} else {
				family.CompanionIDs = append(family.CompanionIDs, source.DictionaryIndex)
			}
		}
		out.Families[spec.ID] = family
	}
	for _, parent := range []struct {
		name, endUse, carrier string
		families              []string
	}{
		{"Heating:NaturalGas", "heating", "natural_gas", []string{"boiler.natural_gas", "boiler.ancillary_natural_gas"}},
		{"Heating:Electricity", "heating", "electricity", []string{"boiler.ancillary_electricity"}},
		{"Pumps:Electricity", "pumps", "electricity", []string{"pump.hw.electricity", "pump.cw.electricity"}},
	} {
		source, err := epathSQLPoolSelectNative(observed.Sources, parent.name, "", "Monthly", "J", true)
		if err != nil {
			return out, err
		}
		if _, err := epathSQLPoolSourceRequest(doc, run, observed.outputPlan, source, ""); err != nil {
			return out, err
		}
		dictionary, err := epathSQLReadPoolNativeDictionary(db, source)
		if err != nil {
			return out, err
		}
		rows, err := epathSQLAuditPoolNative(db, source, out.Weather)
		if err != nil {
			return out, err
		}
		_, native, err := epathSQLValidatePoolNativeRows(source, out.Weather, rows)
		if err != nil {
			return out, err
		}
		monthly, err := epathSQLMonthly(source, precision)
		if err != nil {
			return out, err
		}
		closure := epathSQLPoolParentClosure{EndUse: parent.endUse, Carrier: parent.carrier, Source: source, Dictionary: dictionary, Rows: rows, NativeMonthly: native, ConstituentFamilyIDs: append([]string(nil), parent.families...), Monthly: monthly}
		for _, id := range parent.families {
			closure.ConstituentSourceIDs = append(closure.ConstituentSourceIDs, out.Families[id].CanonicalID)
		}
		out.Parents[parent.name] = closure
	}
	if err := epathSQLValidatePoolSourceFrames(out); err != nil {
		return out, err
	}
	return out, nil
}

func epathSQLPoolSourceQuantity(identity epathSQLPoolSourceIdentity, weather epathRealSQLWeather) (epathSQLQuantity, error) {
	if err := epathSQLPoolSourcePrecision(identity.Precision); err != nil {
		return epathSQLQuantity{}, err
	}
	if err := epathSQLValidatePoolNativeDictionary(identity.Source, identity.Dictionary); err != nil {
		return epathSQLQuantity{}, err
	}
	raw, energy, err := epathSQLValidatePoolNativeRows(identity.Source, weather, identity.Rows)
	if err != nil {
		return epathSQLQuantity{}, err
	}
	canonical := identity.Source.Name == identity.Spec.EnergyName && identity.Source.ReportingFrequency == "Monthly"
	if !identity.RequestBound || identity.ExecutedOwnerIndex < 0 || identity.Source.IsMeter || !strings.EqualFold(identity.Source.KeyValue, identity.Spec.Owner.ObjectName) || identity.Source.Name != identity.Spec.EnergyName && identity.Source.Name != identity.Spec.RateName || identity.Source.Name == identity.Spec.EnergyName && identity.Source.SourceUnit != "J" || identity.Source.Name == identity.Spec.RateName && identity.Source.SourceUnit != "W" || identity.Canonical != canonical || identity.BudgetAuthority != (canonical && identity.Spec.Role == "purchased_constituent") || !epathSQLPoolSameMonths(raw, identity.NativeRawMonthly) || !epathSQLPoolSameMonths(energy, identity.NativeEnergyMonthly) {
		return epathSQLQuantity{}, fmt.Errorf("Pool source changed native owner, request, units or nonadditive authority")
	}
	value := 0.0
	for _, month := range energy {
		value += month
	}
	q := epathSQLQuantity{Value: value}
	if value != 0 {
		q.Error = 1e-10 * math.Max(1, math.Abs(value))
	}
	return q, nil
}

func epathSQLValidatePoolSourceFrames(frames epathSQLPoolSourceFrames) error {
	specs, err := epathSQLPoolSourceSpecs(frames.Original)
	if err != nil {
		return err
	}
	if len(frames.Families) != len(specs) || len(frames.Sources) != 4*len(specs) || len(frames.Parents) != 3 {
		return fmt.Errorf("Pool source frame is missing or duplicating a declared family/companion/parent")
	}
	seen := map[int]bool{}
	for _, spec := range specs {
		family, exists := frames.Families[spec.ID]
		if !exists || !reflect.DeepEqual(family.Spec, spec) || len(family.CompanionIDs) != 3 {
			return fmt.Errorf("Pool family has no exact original owner and three companion observations")
		}
		canonical, exists := frames.Sources[family.CanonicalID]
		if !exists || !canonical.Canonical {
			return fmt.Errorf("Pool family has no measured Monthly Energy authority")
		}
		quantities, err := epathSQLMonthly(canonical.Source, frames.Precision)
		if err != nil || !reflect.DeepEqual(quantities, family.Monthly) {
			return fmt.Errorf("Pool family changed Monthly native source bounds")
		}
		annual, err := epathSQLPoolSourceQuantity(canonical, frames.Weather)
		if err != nil || !reflect.DeepEqual(annual, family.Annual) {
			return fmt.Errorf("Pool family changed native annual source integral: %v", err)
		}
		combinations := map[string]bool{}
		for _, id := range append([]int{family.CanonicalID}, family.CompanionIDs...) {
			identity, exists := frames.Sources[id]
			if !exists || seen[id] || identity.Source.DictionaryIndex != id || !reflect.DeepEqual(identity.Spec, spec) || !reflect.DeepEqual(identity.Precision, frames.Precision) {
				return fmt.Errorf("Pool source proof overlaps or crosses original families")
			}
			seen[id] = true
			combination := identity.Source.Name + "|" + identity.Source.ReportingFrequency
			if combinations[combination] {
				return fmt.Errorf("Pool source repeats one E/R-frequency combination")
			}
			combinations[combination] = true
			if _, err := epathSQLPoolSourceQuantity(identity, frames.Weather); err != nil {
				return err
			}
			for month, value := range identity.NativeEnergyMonthly {
				if !epathSQLPoolSameNative(value, canonical.NativeEnergyMonthly[month]) {
					return fmt.Errorf("Pool native companion differs from Monthly Energy at M%d", month+1)
				}
			}
		}
	}
	return epathSQLValidatePoolParentClosure(frames)
}

func epathSQLValidatePoolParentClosure(frames epathSQLPoolSourceFrames) error {
	parents := []struct {
		name, endUse, carrier string
		families              []string
	}{
		{"Heating:NaturalGas", "heating", "natural_gas", []string{"boiler.natural_gas", "boiler.ancillary_natural_gas"}},
		{"Heating:Electricity", "heating", "electricity", []string{"boiler.ancillary_electricity"}},
		{"Pumps:Electricity", "pumps", "electricity", []string{"pump.hw.electricity", "pump.cw.electricity"}},
	}
	seen := map[int]bool{}
	for _, want := range parents {
		parent, exists := frames.Parents[want.name]
		if !exists || parent.EndUse != want.endUse || parent.Carrier != want.carrier || parent.Source.Name != want.name || !parent.Source.IsMeter || parent.Source.KeyValue != "" || parent.Source.ReportingFrequency != "Monthly" || parent.Source.SourceUnit != "J" || !reflect.DeepEqual(parent.ConstituentFamilyIDs, want.families) || len(parent.ConstituentSourceIDs) != len(want.families) {
			return fmt.Errorf("Pool parent ledger lost its exact finite native resource roster")
		}
		if seen[parent.Source.DictionaryIndex] || frames.Sources[parent.Source.DictionaryIndex].Source.DictionaryIndex != 0 {
			return fmt.Errorf("Pool parent/source identity overlaps")
		}
		if err := epathSQLValidatePoolNativeDictionary(parent.Source, parent.Dictionary); err != nil {
			return err
		}
		seen[parent.Source.DictionaryIndex] = true
		_, native, err := epathSQLValidatePoolNativeRows(parent.Source, frames.Weather, parent.Rows)
		if err != nil || !epathSQLPoolSameMonths(native, parent.NativeMonthly) {
			return fmt.Errorf("Pool parent changed actual native rows: %v", err)
		}
		monthly, err := epathSQLMonthly(parent.Source, frames.Precision)
		if err != nil || !reflect.DeepEqual(monthly, parent.Monthly) {
			return fmt.Errorf("Pool parent changed independent source bounds")
		}
		sums := [12]float64{}
		for n, familyID := range want.families {
			family, exists := frames.Families[familyID]
			identity := frames.Sources[family.CanonicalID]
			if !exists || family.CanonicalID <= 0 || parent.ConstituentSourceIDs[n] != family.CanonicalID || !identity.Canonical || !identity.BudgetAuthority || identity.Spec.EndUse != want.endUse || identity.Spec.Carrier != want.carrier {
				return fmt.Errorf("Pool parent substituted a thermal/rate/foreign constituent")
			}
			for month, value := range identity.NativeEnergyMonthly {
				sums[month] += value
			}
		}
		for month, value := range sums {
			if !epathSQLPoolSameNative(value, native[month]) {
				return fmt.Errorf("Pool native parent/constituents do not close at %s/M%d; no normalization is permitted", want.name, month+1)
			}
		}
	}
	return nil
}

// Stable source order for downstream checks; thermal context and companions
// stay visible here, but only Monthly purchased Energy can own a budget.
func epathSQLPoolSourceIDs(frames epathSQLPoolSourceFrames) []int {
	ids := make([]int, 0, len(frames.Sources))
	for id := range frames.Sources {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}
