package simulation

// Independent finite native-source proof. Only lexical IDF parsing, original
// test-only proof, read-only SQLite and test-only weather primitives are shared.
// No production PV inventory/reader/classifier/series or candidate value is used.
import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathSQLPVNativeSpec struct {
	ID, Name, Key, Role, Sign, MetricGroup string
	IsMeter                                bool
	Owner                                  *epathSQLPVOriginalObject // nil means an unkeyed Facility meter.
}

type epathSQLPVNativeDictionary struct {
	Index                                                      int
	Name, Key, Frequency, Unit, Type, TimestepType, IndexGroup string
	IsMeter                                                    bool
	KeyWasNULL                                                 bool
}

type epathSQLPVNativeRow struct {
	TimeIndex, EnvironmentIndex, Year, Month, Day, Hour, Minute, IntervalType, SimulationDays int
	IntervalMinutes, NativeJ                                                                  float64
}

type epathSQLPVSourceIdentity struct {
	Spec                                   epathSQLPVNativeSpec
	Source                                 epathRealSQLSource
	Dictionary                             epathSQLPVNativeDictionary
	Rows                                   []epathSQLPVNativeRow
	NativeMonthlyJ, NativeMonthlyKWh       [12]float64
	NativeAnnualJ, NativeAnnualKWh         float64
	HasNegativeValue                       bool
	OriginalOwnerIndex, ExecutedOwnerIndex *int
	OutputObjectIndex                      *int // Unique ORIGINAL Output request, never equipment/run index.
	RequestBound                           bool
	AggregationBasis                       string
	EffectiveMultiplier                    float64
}

type epathSQLPVSourceFrames struct {
	Original                  epathSQLPVOriginalProof
	ExecutedOwners            []epathSQLPVOriginalObject
	ExecutedSHA256, SQLSHA256 string
	Weather                   epathRealSQLWeather
	Precision                 epathRealSQLPrecision
	DictionaryCensusComplete  bool
	Sources                   map[int]epathSQLPVSourceIdentity
}

func epathSQLPVSourceSpecs(original epathSQLPVOriginalProof) ([]epathSQLPVNativeSpec, error) {
	if len(original.Owners) != 8 || len(original.PhysicalObjects) != 23 || len(original.OriginalSHA256) != 64 || len(original.Declaration.Generators) != 5 {
		return nil, fmt.Errorf("PV source roster needs finite original 8-owner/23-object proof")
	}
	one := func(typ, name string) (*epathSQLPVOriginalObject, error) {
		var found *epathSQLPVOriginalObject
		for _, owner := range original.Owners {
			if strings.EqualFold(owner.ObjectType, typ) && strings.EqualFold(owner.ObjectName, name) {
				if found != nil || owner.ObjectIndex < 0 || len(owner.Fields) == 0 || owner.Fields[0] != owner.ObjectName {
					return nil, fmt.Errorf("PV original source owner ambiguous/invalid")
				}
				copy := owner
				copy.Fields = append([]string(nil), owner.Fields...)
				found = &copy
			}
		}
		if found == nil {
			return nil, fmt.Errorf("PV original source owner missing %s/%s", typ, name)
		}
		return found, nil
	}
	d := original.Declaration
	inv, err := one("ElectricLoadCenter:Inverter:LookUpTable", d.InverterName)
	if err != nil {
		return nil, err
	}
	bat, err := one("ElectricLoadCenter:Storage:Battery", d.StorageName)
	if err != nil {
		return nil, err
	}
	lc, err := one("ElectricLoadCenter:Distribution", d.LoadCenterName)
	if err != nil {
		return nil, err
	}
	var out []epathSQLPVNativeSpec
	add := func(id, name, role, sign, group string, owner *epathSQLPVOriginalObject) {
		key := ""
		if owner != nil {
			key = owner.ObjectName
		}
		out = append(out, epathSQLPVNativeSpec{ID: id, Name: name, Key: key, Role: role, Sign: sign, MetricGroup: group, IsMeter: owner == nil, Owner: owner})
	}
	for n, g := range d.Generators {
		pv, err := one("Generator:Photovoltaic", g.Name)
		if err != nil {
			return nil, err
		}
		add(fmt.Sprintf("pv.dc.%d", n+1), "Generator Produced DC Electricity Energy", "produced_constituent", "nonnegative", "carriers", pv)
	}
	for _, row := range []struct{ id, name, role, sign, group string }{
		{"inverter.dc_input", "Inverter DC Input Electricity Energy", "transfer_context", "nonnegative", "carriers"},
		{"inverter.ac_output", "Inverter AC Output Electricity Energy", "transfer_context", "nonnegative", "carriers"},
		{"inverter.loss", "Inverter Conversion Loss Energy", "loss_context", "nonnegative", "carriers"},
		{"inverter.loss_decrement", "Inverter Conversion Loss Decrement Energy", "produced_decrement", "nonpositive", "carriers"},
		{"inverter.ancillary", "Inverter Ancillary AC Electricity Energy", "purchased_constituent", "nonnegative", "endUses"},
	} {
		add(row.id, row.name, row.role, row.sign, row.group, inv)
	}
	for _, row := range []struct{ id, name, role, sign, group string }{
		{"storage.charge", "Electric Storage Charge Energy", "transfer_context", "nonnegative", "carriers"},
		{"storage.decrement", "Electric Storage Production Decrement Energy", "produced_decrement", "nonpositive", "carriers"},
		{"storage.discharge", "Electric Storage Discharge Energy", "produced_constituent", "nonnegative", "carriers"},
		{"storage.thermal", "Electric Storage Thermal Loss Energy", "thermal_context", "signed", "loads"},
	} {
		add(row.id, row.name, row.role, row.sign, row.group, bat)
	}
	add("distribution.electricity", "Electric Load Center Produced Electricity Energy", "generation_subtotal_context", "nonnegative", "carriers", lc)
	add("distribution.thermal", "Electric Load Center Produced Thermal Energy", "thermal_context", "signed", "loads", lc)
	for _, row := range []struct{ id, name, sign string }{
		{"facility.demand", "Electricity:Facility", "nonnegative"}, {"facility.produced", "ElectricityProduced:Facility", "signed"},
		{"facility.purchased", "ElectricityPurchased:Facility", "nonnegative"}, {"facility.sold", "ElectricitySurplusSold:Facility", "nonnegative"},
	} {
		add(row.id, row.name, "facility_observation", row.sign, "carriers", nil)
	}
	return out, nil
}

func epathSQLPVNativeNear(a, b float64) bool {
	return epathOracleFinite(a) && epathOracleFinite(b) && (a == 0) == (b == 0) && (a < 0) == (b < 0) && math.Abs(a-b) <= 1e-10*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

func epathSQLPVFileSHA(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func epathSQLPVSignValid(sign string, value float64) bool {
	if !epathOracleFinite(value) {
		return false
	}
	switch sign {
	case "nonnegative":
		return value >= 0
	case "nonpositive":
		return value <= 0
	case "signed":
		return true
	}
	return false
}

func epathSQLPVReadDictionary(db *sql.DB, spec epathSQLPVNativeSpec, frequency string) (epathSQLPVNativeDictionary, error) {
	var out epathSQLPVNativeDictionary
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,Name,KeyValue,IsMeter,ReportingFrequency,Units,Type,TimestepType,IndexGroup,ScheduleName FROM ReportDataDictionary WHERE Name=? COLLATE NOCASE AND COALESCE(KeyValue,'')=? COLLATE NOCASE AND ReportingFrequency=? COLLATE NOCASE`, spec.Name, spec.Key, frequency)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, meter sql.NullInt64
		var name, key, freq, unit, typ, step, group, schedule sql.NullString
		if err := rows.Scan(&id, &name, &key, &meter, &freq, &unit, &typ, &step, &group, &schedule); err != nil {
			return out, err
		}
		count++
		wantStep := "HVAC System"
		if spec.IsMeter {
			wantStep = "Zone"
		}
		if !id.Valid || id.Int64 <= 0 || !name.Valid || name.String != spec.Name || !freq.Valid || freq.String != frequency || !unit.Valid || unit.String != "J" || !meter.Valid || meter.Int64 != 0 && meter.Int64 != 1 || (meter.Int64 == 1) != spec.IsMeter || !typ.Valid || typ.String != "Sum" || !step.Valid || step.String != wantStep || strings.TrimSpace(schedule.String) != "" || spec.IsMeter && key.String != "" || !spec.IsMeter && (!key.Valid || strings.TrimSpace(key.String) == "" || !strings.EqualFold(key.String, spec.Key)) {
			return out, fmt.Errorf("PV invalid exact native dictionary %s/%s/%s", spec.Name, spec.Key, frequency)
		}
		out = epathSQLPVNativeDictionary{Index: int(id.Int64), Name: name.String, Key: key.String, Frequency: freq.String, Unit: unit.String, Type: typ.String, TimestepType: step.String, IndexGroup: group.String, IsMeter: meter.Int64 == 1, KeyWasNULL: !key.Valid}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if count != 1 {
		return out, fmt.Errorf("PV native dictionary count=%d for %s/%s/%s; absence/duplicate is not zero", count, spec.Name, spec.Key, frequency)
	}
	return out, nil
}

func epathSQLPVReadRows(db *sql.DB, d epathSQLPVNativeDictionary) ([]epathSQLPVNativeRow, error) {
	rows, err := db.Query(`SELECT r.TimeIndex,r.Value,t.EnvironmentPeriodIndex,e.EnvironmentType,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,t.SimulationDays,t.WarmupFlag FROM ReportData r LEFT JOIN Time t ON t.TimeIndex=r.TimeIndex LEFT JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex WHERE r.ReportDataDictionaryIndex=? ORDER BY r.TimeIndex,r.ReportDataIndex`, d.Index)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []epathSQLPVNativeRow
	for rows.Next() {
		var id, env, envType, year, month, day, hour, minute, intervalType, days, warmup sql.NullInt64
		var value, interval sql.NullFloat64
		if err := rows.Scan(&id, &value, &env, &envType, &year, &month, &day, &hour, &minute, &interval, &intervalType, &days, &warmup); err != nil {
			return nil, err
		}
		if !id.Valid || !env.Valid || !envType.Valid {
			return nil, fmt.Errorf("PV orphan/unknown Time environment")
		}
		if envType.Int64 != 3 || warmup.Valid && warmup.Int64 != 0 {
			continue
		}
		if !year.Valid || !month.Valid || !day.Valid || !hour.Valid || !minute.Valid || !intervalType.Valid || !days.Valid || !interval.Valid || !value.Valid || !warmup.Valid && intervalType.Int64 != 1 && intervalType.Int64 != 3 {
			return nil, fmt.Errorf("PV unknown native calendar/value is not zero")
		}
		out = append(out, epathSQLPVNativeRow{TimeIndex: int(id.Int64), EnvironmentIndex: int(env.Int64), Year: int(year.Int64), Month: int(month.Int64), Day: int(day.Int64), Hour: int(hour.Int64), Minute: int(minute.Int64), IntervalType: int(intervalType.Int64), SimulationDays: int(days.Int64), IntervalMinutes: interval.Float64, NativeJ: value.Float64})
	}
	return out, rows.Err()
}

// Complete native observation proof. This is intentionally not a production
// partial-data display reader: NULL/missing/duplicate returns an error, never a
// known-zero source metric or a silent allow-absent acceptance exception.
func epathSQLPVNativeSummary(spec epathSQLPVNativeSpec, d epathSQLPVNativeDictionary, weather epathRealSQLWeather, rows []epathSQLPVNativeRow) (epathRealSQLSource, [12]float64, [12]float64, bool, error) {
	var raw, energy [12]float64
	negative := false
	source := epathRealSQLSource{DictionaryIndex: d.Index, Name: d.Name, KeyValue: d.Key, IsMeter: d.IsMeter, ReportingFrequency: d.Frequency, SourceUnit: d.Unit, IndexGroup: d.IndexGroup}
	fail := func(message string) (epathRealSQLSource, [12]float64, [12]float64, bool, error) {
		return source, raw, energy, negative, fmt.Errorf("PV native source %s: %s", spec.ID, message)
	}
	step := "HVAC System"
	if spec.IsMeter {
		step = "Zone"
	}
	if weather.Year != 2017 || weather.EnvironmentIndex <= 0 || len(weather.Months) != 12 || weather.CoverageBasis == "" || d.Index <= 0 || d.Name != spec.Name || !strings.EqualFold(d.Key, spec.Key) || d.IsMeter != spec.IsMeter || d.Unit != "J" || d.Type != "Sum" || d.TimestepType != step || !spec.IsMeter && d.KeyWasNULL || d.Frequency != "Monthly" && d.Frequency != "Hourly" {
		return fail("wrong original/native identity or annual weather")
	}
	want := 12
	if d.Frequency == "Hourly" {
		want = 8760
	}
	if len(rows) != want {
		return fail("incomplete or duplicate calendar row count")
	}
	seenTimes, seenSlots := map[int]bool{}, map[int]bool{}
	perMonth := [12]int{}
	for _, row := range rows {
		date := time.Date(row.Year, time.Month(row.Month), row.Day, 0, 0, 0, 0, time.UTC)
		if row.TimeIndex <= 0 || seenTimes[row.TimeIndex] || row.EnvironmentIndex != weather.EnvironmentIndex || row.Year != 2017 || row.Month < 1 || row.Month > 12 || row.Day < 1 || date.Month() != time.Month(row.Month) || date.Day() != row.Day || row.Minute != 0 || row.SimulationDays != date.YearDay() || !epathSQLPVSignValid(spec.Sign, row.NativeJ) || !epathOracleFinite(row.IntervalMinutes) {
			return fail("invalid sign, exact zero/knownness or native date")
		}
		last := time.Date(2017, time.Month(row.Month+1), 0, 0, 0, 0, 0, time.UTC)
		slot := row.Month - 1
		if d.Frequency == "Hourly" {
			slot = (date.YearDay()-1)*24 + row.Hour - 1
			if row.IntervalType != 1 || row.IntervalMinutes != 60 || row.Hour < 1 || row.Hour > 24 {
				return fail("Hourly native interval mismatch")
			}
		} else if row.IntervalType != 3 || row.IntervalMinutes != float64(last.Day()*1440) || row.Day != last.Day() || row.Hour != 24 {
			return fail("Monthly native interval mismatch")
		}
		if seenSlots[slot] {
			return fail("duplicate native calendar slot")
		}
		seenSlots[slot], seenTimes[row.TimeIndex] = true, true
		raw[row.Month-1] += row.NativeJ
		energy[row.Month-1] += row.NativeJ / 3600000
		perMonth[row.Month-1]++
		negative = negative || row.NativeJ < 0
	}
	totalRaw, totalEnergy := 0.0, 0.0
	for n := 0; n < 12; n++ {
		if weather.Months[n] != n+1 || !epathOracleFinite(raw[n]) || !epathOracleFinite(energy[n]) {
			return fail("invalid native annual accumulation")
		}
		totalRaw += raw[n]
		totalEnergy += energy[n]
		source.Months = append(source.Months, epathRealSQLMonth{Month: n + 1, Rows: perMonth[n], RawSum: epathOracleNumber(raw[n]), EnergyKWh: epathOracleNumber(energy[n])})
	}
	if !epathOracleFinite(totalRaw) || !epathOracleFinite(totalEnergy) {
		return fail("nonfinite native total")
	}
	source.Rows = len(rows)
	source.RawSum = epathOracleNumber(totalRaw)
	source.EnergyKWh = epathOracleNumber(totalEnergy)
	return source, raw, energy, negative, nil
}

func epathSQLPVRequestCovers(typ string, fields []string, spec epathSQLPVNativeSpec, frequency string) bool {
	if spec.IsMeter {
		if !strings.EqualFold(typ, "Output:Meter") || len(fields) != 2 || !strings.EqualFold(fields[1], frequency) {
			return false
		}
		selector := strings.ToLower(strings.TrimSpace(fields[0]))
		name := strings.ToLower(spec.Name)
		return selector == name || strings.Count(selector, "*") == 1 && strings.HasSuffix(selector, "*") && strings.HasPrefix(name, strings.TrimSuffix(selector, "*"))
	}
	if !strings.EqualFold(typ, "Output:Variable") || len(fields) < 3 || len(fields) > 4 || !strings.EqualFold(fields[1], spec.Name) || !strings.EqualFold(fields[2], frequency) || len(fields) == 4 && strings.TrimSpace(fields[3]) != "" {
		return false
	}
	key := strings.TrimSpace(fields[0])
	return key == "" || key == "*" || strings.EqualFold(key, spec.Key)
}

func epathSQLPVRequestBinding(original, executed idf.Document, plan *PurposeRunPlan, spec epathSQLPVNativeSpec, frequency string, scopedZone ...string) (*int, error) {
	// PV and equipment requests remain unscoped by default. Only a caller with
	// an independently proved native Zone key may authorize matching metadata.
	zone := ""
	if len(scopedZone) > 1 {
		return nil, fmt.Errorf("native output request has ambiguous Zone scope")
	}
	if len(scopedZone) == 1 {
		zone = scopedZone[0]
	}
	if zone != "" && (spec.IsMeter || !strings.EqualFold(zone, spec.Key)) {
		return nil, fmt.Errorf("native output Zone scope differs from exact native key")
	}
	if plan == nil || plan.BasicEnergyDetail != "energy_path" {
		return nil, fmt.Errorf("PV source requires actual Energy Path output plan")
	}
	old, run := map[int]bool{}, map[int]bool{}
	oldFields := map[string]int{}
	runFields := map[string]int{}
	for index, doc := range []idf.Document{original, executed} {
		for _, object := range doc.Objects {
			fields := []string{}
			for _, f := range object.Fields {
				fields = append(fields, strings.TrimSpace(f.Value))
			}
			if !epathSQLPVRequestCovers(object.Type, fields, spec, frequency) {
				continue
			}
			signature := strings.ToLower(object.Type + "\x00" + strings.Join(fields, "\x00"))
			if index == 0 {
				old[object.Index] = true
				oldFields[signature]++
			} else {
				run[object.Index] = true
				runFields[signature]++
			}
		}
	}
	if len(run) == 0 {
		return nil, fmt.Errorf("PV native source lacks executed output request")
	}
	for signature, count := range oldFields {
		if runFields[signature] < count {
			return nil, fmt.Errorf("PV executed output removed an original declaration")
		}
	}
	cover := 0
	for _, output := range plan.OutputObjects {
		fields := []string{}
		for _, f := range output.Fields {
			fields = append(fields, strings.TrimSpace(f.Value))
		}
		if !epathSQLPVRequestCovers(output.ObjectType, fields, spec, frequency) {
			continue
		}
		cover++
		basic := false
		for _, purpose := range output.PurposeIDs {
			basic = basic || purpose == SimulationPurposeBasicEnergy
		}
		scopeOK := output.ScopeZoneName == ""
		if output.ScopeZoneName != "" && zone != "" && strings.EqualFold(output.ScopeZoneName, zone) && strings.EqualFold(fields[0], zone) {
			// A scoped plan cannot borrow an executed wildcard request: its
			// exact actual key/variable/frequency/schedule tuple must be present.
			signature := strings.ToLower(output.ObjectType + "\x00" + strings.Join(fields, "\x00"))
			scopeOK = runFields[signature] > 0
		}
		if !basic || !scopeOK || output.ReportingFrequency != frequency || output.State != "existing" && output.State != "temporary" || output.State == "temporary" && output.ObjectIndex != nil || output.State == "existing" && (output.ObjectIndex == nil || !old[*output.ObjectIndex]) {
			return nil, fmt.Errorf("PV output plan metadata borrowed purpose, scope or opener")
		}
		if spec.IsMeter {
			if output.VariableName != "" || !strings.EqualFold(output.KeyValue, fields[0]) {
				return nil, fmt.Errorf("PV meter request metadata differs")
			}
		} else if !strings.EqualFold(output.VariableName, fields[1]) || !strings.EqualFold(output.KeyValue, fields[0]) {
			return nil, fmt.Errorf("PV variable request metadata differs")
		}
	}
	if cover == 0 {
		return nil, fmt.Errorf("PV native source absent from actual output plan")
	}
	if len(old) != 1 || cover != 1 {
		return nil, nil
	}
	for index := range old {
		for _, output := range plan.OutputObjects {
			if output.State == "existing" && output.ObjectIndex != nil && *output.ObjectIndex == index {
				copy := index
				return &copy, nil
			}
		}
	}
	return nil, nil
}

func epathCompileSQLPVSourceFrames(observed epathRealOracleEvidence, original epathSQLPVOriginalProof, precision epathRealSQLPrecision) (epathSQLPVSourceFrames, error) {
	out := epathSQLPVSourceFrames{Original: original, Precision: precision, Sources: map[int]epathSQLPVSourceIdentity{}}
	if precision.DecimalPlaces != 3 || precision.SourceStages < 1 || precision.SourceStages > 3 || precision.ContributionStages < 1 || precision.ContributionStages > 3 {
		return out, fmt.Errorf("PV source transport precision differs from independent contract")
	}
	executedOwners, err := epathSQLBindPVExecuted(observed.originalText, observed.executedText, original)
	if err != nil {
		return out, err
	}
	out.ExecutedOwners = executedOwners
	out.ExecutedSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(observed.executedText)))
	specs, err := epathSQLPVSourceSpecs(original)
	if err != nil {
		return out, err
	}
	doc, err := idf.Parse(observed.originalText)
	if err != nil {
		return out, err
	}
	run, err := idf.Parse(observed.executedText)
	if err != nil {
		return out, err
	}
	db, err := epathOpenOracleSQL(observed.sqlPath)
	if err != nil {
		return out, err
	}
	defer db.Close()
	out.SQLSHA256, err = epathSQLPVFileSHA(observed.sqlPath)
	if err != nil {
		return out, err
	}
	out.Weather, err = epathReadOracleWeather(db)
	if err != nil {
		return out, err
	}
	if !reflect.DeepEqual(out.Weather, observed.Weather) {
		return out, fmt.Errorf("PV weather proof changed after source collection")
	}
	for _, spec := range specs {
		for _, frequency := range []string{"Monthly", "Hourly"} {
			dictionary, err := epathSQLPVReadDictionary(db, spec, frequency)
			if err != nil {
				return out, err
			}
			if _, duplicate := out.Sources[dictionary.Index]; duplicate {
				return out, fmt.Errorf("PV native RDD belongs to two identities")
			}
			rows, err := epathSQLPVReadRows(db, dictionary)
			if err != nil {
				return out, err
			}
			summary, raw, energy, negative, err := epathSQLPVNativeSummary(spec, dictionary, out.Weather, rows)
			if err != nil {
				return out, err
			}
			found, count := epathRealSQLSource{}, 0
			for _, source := range observed.Sources {
				if source.DictionaryIndex == dictionary.Index {
					found = source
					count++
				}
			}
			if count != 1 || !epathSQLPVSourceSummaryEqual(summary, found) {
				return out, fmt.Errorf("PV exact native rows do not reproduce independently collected RDD %d", dictionary.Index)
			}
			opener, err := epathSQLPVRequestBinding(doc, run, observed.outputPlan, spec, frequency)
			if err != nil {
				return out, err
			}
			identity := epathSQLPVSourceIdentity{Spec: spec, Source: summary, Dictionary: dictionary, Rows: rows, NativeMonthlyJ: raw, NativeMonthlyKWh: energy, NativeAnnualJ: *summary.RawSum, NativeAnnualKWh: *summary.EnergyKWh, HasNegativeValue: negative, OutputObjectIndex: opener, RequestBound: true, AggregationBasis: "model_total", EffectiveMultiplier: 1}
			if spec.Owner != nil {
				old := spec.Owner.ObjectIndex
				identity.OriginalOwnerIndex = &old
				for _, owner := range executedOwners {
					if strings.EqualFold(owner.ObjectType, spec.Owner.ObjectType) && strings.EqualFold(owner.ObjectName, spec.Owner.ObjectName) {
						copy := owner.ObjectIndex
						identity.ExecutedOwnerIndex = &copy
					}
				}
				if identity.ExecutedOwnerIndex == nil {
					return out, fmt.Errorf("PV missing independently parsed executed owner")
				}
			}
			out.Sources[dictionary.Index] = identity
		}
	}
	// Closed finite Energy M/H roster: extra foreign keys cannot evade the
	// original proof merely because all expected source keys were also found.
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,Name,ReportingFrequency FROM ReportDataDictionary`)
	if err != nil {
		return out, err
	}
	names := map[string]bool{}
	for _, spec := range specs {
		names[strings.ToLower(spec.Name)] = true
	}
	for rows.Next() {
		var index int
		var name, frequency sql.NullString
		if err := rows.Scan(&index, &name, &frequency); err != nil {
			rows.Close()
			return out, err
		}
		if names[strings.ToLower(name.String)] && (strings.EqualFold(frequency.String, "Monthly") || strings.EqualFold(frequency.String, "Hourly")) {
			if _, found := out.Sources[index]; !found {
				rows.Close()
				return out, fmt.Errorf("PV extra/foreign M/H native dictionary %d", index)
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	out.DictionaryCensusComplete = true
	if err := epathSQLValidatePVSourceFrames(out); err != nil {
		return out, err
	}
	return out, nil
}

func epathSQLPVSourceSummaryEqual(a, b epathRealSQLSource) bool {
	if a.DictionaryIndex != b.DictionaryIndex || a.Name != b.Name || a.KeyValue != b.KeyValue || a.IsMeter != b.IsMeter || a.ReportingFrequency != b.ReportingFrequency || a.SourceUnit != b.SourceUnit || a.IndexGroup != b.IndexGroup || a.Rows != b.Rows || b.MissingRows != 0 || a.RawSum == nil || b.RawSum == nil || a.EnergyKWh == nil || b.EnergyKWh == nil || !epathSQLPVNativeNear(*a.RawSum, *b.RawSum) || !epathSQLPVNativeNear(*a.EnergyKWh, *b.EnergyKWh) || len(b.Months) != 12 {
		return false
	}
	for n, want := range a.Months {
		got := b.Months[n]
		if got.Month != want.Month || got.Rows != want.Rows || got.MissingRows != 0 || got.RawSum == nil || got.EnergyKWh == nil || !epathSQLPVNativeNear(*want.RawSum, *got.RawSum) || !epathSQLPVNativeNear(*want.EnergyKWh, *got.EnergyKWh) {
			return false
		}
	}
	return true
}

func epathSQLValidatePVSourceFrames(frame epathSQLPVSourceFrames) error {
	specs, err := epathSQLPVSourceSpecs(frame.Original)
	if err != nil {
		return err
	}
	if !frame.DictionaryCensusComplete || len(frame.Sources) != 40 || len(frame.ExecutedOwners) != 8 || len(frame.ExecutedSHA256) != 64 || len(frame.SQLSHA256) != 64 {
		return fmt.Errorf("PV native frame lost closed source/executed/hash roster")
	}
	if frame.Precision.DecimalPlaces != 3 || frame.Precision.SourceStages < 1 || frame.Precision.SourceStages > 3 || frame.Precision.ContributionStages < 1 || frame.Precision.ContributionStages > 3 {
		return fmt.Errorf("PV native source precision contract changed")
	}
	seen := map[string]bool{}
	for id, identity := range frame.Sources {
		var spec *epathSQLPVNativeSpec
		for n := range specs {
			if specs[n].ID == identity.Spec.ID {
				spec = &specs[n]
			}
		}
		if spec == nil || !reflect.DeepEqual(*spec, identity.Spec) || id != identity.Dictionary.Index || id != identity.Source.DictionaryIndex || !identity.RequestBound || identity.AggregationBasis != "model_total" || identity.EffectiveMultiplier != 1 {
			return fmt.Errorf("PV source substituted owner/role/identity/multiplier")
		}
		key := spec.ID + "/" + identity.Dictionary.Frequency
		if seen[key] {
			return fmt.Errorf("PV duplicated family frequency")
		}
		seen[key] = true
		if spec.Owner == nil {
			if identity.OriginalOwnerIndex != nil || identity.ExecutedOwnerIndex != nil {
				return fmt.Errorf("PV Facility acquired physical owner")
			}
		} else {
			if identity.OriginalOwnerIndex == nil || *identity.OriginalOwnerIndex != spec.Owner.ObjectIndex || identity.ExecutedOwnerIndex == nil {
				return fmt.Errorf("PV source lost separate original/executed owner")
			}
			count := 0
			for _, owner := range frame.ExecutedOwners {
				if owner.ObjectIndex == *identity.ExecutedOwnerIndex && strings.EqualFold(owner.ObjectType, spec.Owner.ObjectType) && strings.EqualFold(owner.ObjectName, spec.Owner.ObjectName) && reflect.DeepEqual(owner.Fields, spec.Owner.Fields) {
					count++
				}
			}
			if count != 1 {
				return fmt.Errorf("PV source borrowed execution owner index")
			}
		}
		source, raw, energy, negative, err := epathSQLPVNativeSummary(*spec, identity.Dictionary, frame.Weather, identity.Rows)
		if err != nil {
			return err
		}
		if !epathSQLPVSourceSummaryEqual(source, identity.Source) || negative != identity.HasNegativeValue || !epathSQLPVNativeNear(*source.RawSum, identity.NativeAnnualJ) || !epathSQLPVNativeNear(*source.EnergyKWh, identity.NativeAnnualKWh) {
			return fmt.Errorf("PV native source summary/sign changed")
		}
		for month := 0; month < 12; month++ {
			if !epathSQLPVNativeNear(raw[month], identity.NativeMonthlyJ[month]) || !epathSQLPVNativeNear(energy[month], identity.NativeMonthlyKWh[month]) {
				return fmt.Errorf("PV monthly native source quantity changed")
			}
		}
	}
	for _, spec := range specs {
		var monthly, hourly *epathSQLPVSourceIdentity
		for _, identity := range frame.Sources {
			if identity.Spec.ID != spec.ID {
				continue
			}
			copy := identity
			if identity.Dictionary.Frequency == "Monthly" {
				monthly = &copy
			} else if identity.Dictionary.Frequency == "Hourly" {
				hourly = &copy
			}
		}
		if !seen[spec.ID+"/Monthly"] || !seen[spec.ID+"/Hourly"] || monthly == nil || hourly == nil {
			return fmt.Errorf("PV source frequency missing")
		}
		for month := 0; month < 12; month++ {
			if !epathSQLPVNativeNear(monthly.NativeMonthlyKWh[month], hourly.NativeMonthlyKWh[month]) {
				return fmt.Errorf("PV native M/H same-output sum differs for %s/M%d", spec.ID, month+1)
			}
		}
	}
	return nil
}

func epathSQLPVSourceIDs(frame epathSQLPVSourceFrames) []int {
	ids := make([]int, 0, len(frame.Sources))
	for id := range frame.Sources {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}
