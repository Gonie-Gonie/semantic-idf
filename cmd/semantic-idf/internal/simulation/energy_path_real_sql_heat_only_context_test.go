package simulation

// Finite Furnace context extension. The original/executed HeatOnly binding is
// the owner authority; native SQL rows are the only numerical authority.
import (
	"database/sql"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathSQLHeatOnlyContextSpec struct{ Name, Key, Unit, Frequency, Zone, Service string }
type epathSQLHeatOnlyContext struct {
	Spec              epathSQLHeatOnlyContextSpec
	DictionaryIndex   int
	OutputIndex       *int
	ServedZones       []string
	Monthly, Absolute [12]float64 // Unrounded, signed, independently integrated kWh.
	WireMonthly       [12]float64
	Scalar            float64 // Sum of separately serialized row values, then serialized once.
	Hourly            []float64
}

func epathSQLHeatOnlyContextSpecs(b *epathSQLHeatOnlyBinding) ([]epathSQLHeatOnlyContextSpec, error) {
	if b == nil || len(b.Inputs.Furnaces) != 1 || len(b.Original.ServedZones) != 3 {
		return nil, fmt.Errorf("HeatOnly contexts need the finite original binding")
	}
	d := b.Inputs.Furnaces[0]
	if !reflect.DeepEqual(b.Original.Declaration, d) || !reflect.DeepEqual(b.Executed.Declaration, d) {
		return nil, fmt.Errorf("HeatOnly context ownership lost original/executed agreement")
	}
	var out []epathSQLHeatOnlyContextSpec
	for _, frequency := range []string{"Monthly", "Hourly"} {
		for _, zone := range b.Original.ServedZones {
			for _, service := range []string{"Heating", "Cooling"} {
				for _, prefix := range []string{"Zone Predicted Sensible Load to ", "Zone System Predicted Sensible Load to "} {
					out = append(out, epathSQLHeatOnlyContextSpec{prefix + service + " Setpoint Heat Transfer Rate", zone, "W", frequency, zone, strings.ToLower(service)})
				}
			}
		}
		out = append(out, epathSQLHeatOnlyContextSpec{"Heating Coil Heating Energy", d.FuelCoilName, "J", frequency, "", "heating"}, epathSQLHeatOnlyContextSpec{"Heating Coil Heating Rate", d.FuelCoilName, "W", frequency, "", "heating"})
	}
	return out, nil
}
func epathSQLHeatOnlyContextKey(s epathSQLHeatOnlyContextSpec) string {
	return s.Name + "|" + strings.ToLower(s.Key) + "|" + s.Frequency
}

// One bounded native stream for 28 actual identities. LEFT JOIN intentionally
// exposes orphan Time references. No weather filter can hide extra bad rows.
func epathSQLReadHeatOnlyContexts(b *epathSQLHeatOnlyBinding) ([]epathSQLHeatOnlyContext, error) {
	specs, err := epathSQLHeatOnlyContextSpecs(b)
	if err != nil {
		return nil, err
	}
	original, err := idf.Parse(b.OriginalText)
	if err != nil {
		return nil, err
	}
	executed, err := idf.Parse(b.ExecutedText)
	if err != nil {
		return nil, err
	}
	db, err := epathOpenOracleSQL(b.SQLPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	weather, err := epathReadOracleWeather(db)
	if err != nil {
		return nil, err
	}
	out := make([]epathSQLHeatOnlyContext, len(specs))
	byKey := map[string]int{}
	byID := map[int]int{}
	names := []string{}
	for i, s := range specs {
		byKey[epathSQLHeatOnlyContextKey(s)] = i
		out[i].Spec = s
		out[i].ServedZones = append([]string(nil), b.Original.ServedZones...)
		found := false
		for _, n := range names {
			found = found || n == s.Name
		}
		if !found {
			names = append(names, s.Name)
		}
		out[i].OutputIndex, err = epathSQLPVRequestBinding(original, executed, &b.OutputPlan, epathSQLPVNativeSpec{Name: s.Name, Key: s.Key}, s.Frequency, s.Zone)
		if err != nil {
			return nil, err
		}
	}
	args := []any{}
	marks := []string{}
	for _, n := range names {
		args = append(args, n)
		marks = append(marks, "?")
	}
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,Name,COALESCE(KeyValue,''),ReportingFrequency,Units,IsMeter,Type,TimestepType,IndexGroup,ScheduleName FROM ReportDataDictionary WHERE Name COLLATE NOCASE IN (`+strings.Join(marks, ",")+`) AND ReportingFrequency COLLATE NOCASE IN ('Monthly','Hourly')`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, meter int
		var name, key, frequency, unit, kind, step, group string
		var schedule sql.NullString
		if err = rows.Scan(&id, &name, &key, &frequency, &unit, &meter, &kind, &step, &group, &schedule); err != nil {
			rows.Close()
			return nil, err
		}
		i, ok := byKey[name+"|"+strings.ToLower(key)+"|"+frequency]
		wantKind := "Avg"
		if unit == "J" {
			wantKind = "Sum"
		}
		if !ok || id <= 0 || out[i].DictionaryIndex != 0 || meter != 0 || unit != out[i].Spec.Unit || kind != wantKind || step != "HVAC System" || group != "System" || schedule.Valid && schedule.String != "" {
			rows.Close()
			return nil, fmt.Errorf("HeatOnly context native dictionary is missing/duplicate/foreign")
		}
		if _, duplicate := byID[id]; duplicate {
			rows.Close()
			return nil, fmt.Errorf("HeatOnly context dictionary ID reused")
		}
		out[i].DictionaryIndex = id
		byID[id] = i
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(byID) != 28 {
		return nil, fmt.Errorf("HeatOnly requires all 28 actual Monthly/Hourly context dictionaries")
	}
	args = nil
	marks = nil
	seen := make([]map[int]bool, len(out))
	for i, p := range out {
		args = append(args, p.DictionaryIndex)
		marks = append(marks, "?")
		seen[i] = map[int]bool{}
		if p.Spec.Frequency == "Hourly" {
			out[i].Hourly = make([]float64, 8760)
		}
	}
	rows, err = db.Query(`SELECT r.ReportDataDictionaryIndex,t.EnvironmentPeriodIndex,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,t.WarmupFlag,r.Value FROM ReportData r LEFT JOIN "Time" t USING(TimeIndex) WHERE r.ReportDataDictionaryIndex IN (`+strings.Join(marks, ",")+`) ORDER BY r.ReportDataIndex`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, env, year, month, day, hour, minute, kind int
		var interval, value sql.NullFloat64
		var warmup sql.NullInt64
		if err = rows.Scan(&id, &env, &year, &month, &day, &hour, &minute, &interval, &kind, &warmup, &value); err != nil {
			return nil, err
		}
		i, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("foreign HeatOnly source row")
		}
		p := &out[i]
		date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		if env != weather.EnvironmentIndex || year != 2017 || month < 1 || month > 12 || day < 1 || date.Month() != time.Month(month) || date.Day() != day || minute != 0 || !interval.Valid || !value.Valid || !epathOracleFinite(value.Float64) || warmup.Valid && warmup.Int64 != 0 || p.Spec.Zone == "" && value.Float64 < 0 {
			return nil, fmt.Errorf("HeatOnly context has invalid/foreign native row")
		}
		slot := month - 1
		last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
		if p.Spec.Frequency == "Hourly" {
			slot = (date.YearDay()-1)*24 + hour - 1
			if kind != 1 || hour < 1 || hour > 24 || interval.Float64 != 60 || slot < 0 || slot >= 8760 {
				return nil, fmt.Errorf("HeatOnly context lacks exact native hourly interval")
			}
		} else if kind != 3 || day != last || hour != 24 || interval.Float64 != float64(last*1440) {
			return nil, fmt.Errorf("HeatOnly context lacks exact native full month")
		}
		if seen[i][slot] {
			return nil, fmt.Errorf("HeatOnly context repeats native calendar slot")
		}
		seen[i][slot] = true
		energy := value.Float64 * (1.0 / 3600000)
		if p.Spec.Unit == "W" {
			energy = value.Float64 * (interval.Float64 / 60) / 1000
		}
		if !epathOracleFinite(energy) {
			return nil, fmt.Errorf("HeatOnly context integration is nonfinite")
		}
		wire := math.Round(energy*1000) / 1000
		p.Monthly[month-1] += energy
		p.Absolute[month-1] += math.Abs(energy)
		p.WireMonthly[month-1] += wire
		p.Scalar += wire
		if p.Spec.Frequency == "Hourly" {
			p.Hourly[slot] = wire
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		p := &out[i]
		want := 12
		if p.Spec.Frequency == "Hourly" {
			want = 8760
		}
		if len(seen[i]) != want {
			return nil, fmt.Errorf("HeatOnly context is incomplete; absence is never zero")
		}
		p.Scalar = math.Round(p.Scalar*1000) / 1000
		for m := range p.WireMonthly {
			p.WireMonthly[m] = math.Round(p.WireMonthly[m]*1000) / 1000
		}
	}
	for _, h := range out {
		if h.Spec.Frequency != "Hourly" {
			continue
		}
		s := h.Spec
		s.Frequency = "Monthly"
		m := out[byKey[epathSQLHeatOnlyContextKey(s)]]
		for n := 0; n < 12; n++ {
			hours := time.Date(2017, time.Month(n+2), 0, 0, 0, 0, 0, time.UTC).Day() * 24
			if h.Spec.Zone == "" && (h.Absolute[n] == 0) != (m.Monthly[n] == 0) || !epathSQLHourlyCompanionNativeNear(h.Monthly[n], m.Monthly[n], h.Absolute[n], hours) {
				return nil, fmt.Errorf("HeatOnly context native Hourly/Monthly closure failed")
			}
		}
	}
	// These aliases describe one coil thermal boundary; they are not additional
	// delivery or fuel amounts. Closure is native arithmetic, never ratio 0.8.
	for _, frequency := range []string{"Monthly", "Hourly"} {
		var energy, rate *epathSQLHeatOnlyContext
		for i := range out {
			p := &out[i]
			if p.Spec.Frequency == frequency && p.Spec.Zone == "" {
				if p.Spec.Unit == "J" {
					energy = p
				} else {
					rate = p
				}
			}
		}
		for m := 0; m < 12; m++ {
			count := 1
			if frequency == "Hourly" {
				count = time.Date(2017, time.Month(m+2), 0, 0, 0, 0, 0, time.UTC).Day() * 24
			}
			if energy == nil || rate == nil || !epathSQLHourlyCompanionNativeNear(energy.Monthly[m], rate.Monthly[m], energy.Absolute[m]+rate.Absolute[m], count) {
				return nil, fmt.Errorf("HeatOnly native coil E/R boundary differs")
			}
		}
	}
	return out, nil
}

func epathSQLModelHeatOnlyContextChecks(frames epathSQLFrames, checks *epathSQLModelChecks) error {
	if checks.HeatOnly == nil {
		return nil
	}
	proofs, err := epathSQLReadHeatOnlyContexts(checks.HeatOnly)
	if err != nil {
		return err
	}
	for _, p := range proofs {
		id := p.DictionaryIndex
		if _, ok := frames.SourceRaw[id]; ok {
			return fmt.Errorf("HeatOnly context was declared as an additive/other source authority")
		}
		for _, ids := range frames.LoadSourceIDs {
			for _, v := range ids {
				if v == id {
					return fmt.Errorf("HeatOnly context became delivered load")
				}
			}
		}
		for _, ids := range frames.SiteSources {
			for _, v := range ids {
				if v == id {
					return fmt.Errorf("HeatOnly context became site consumption")
				}
			}
		}
		for _, cell := range frames.Cells {
			for _, v := range cell.SourceIDs {
				if v == id {
					return fmt.Errorf("HeatOnly context became driver pressure")
				}
			}
		}
		for _, zone := range []string{"", p.Spec.Zone} {
			scope := "building"
			if zone != "" {
				scope = "zone"
			}
			for _, field := range []string{"rawValue", "effectiveValue"} {
				target := epathRealOracleTarget{Collection: "sources", Field: field, SourceName: p.Spec.Name, SourceKey: strings.ToUpper(p.Spec.Key), SourceUnit: p.Spec.Unit, Frequency: p.Spec.Frequency, Unit: "kWh"}
				q := epathSQLQuantity{Value: p.Scalar}
				if err = checks.add("loads", scope, zone, "annual", "heat_only_context/"+epathSQLHeatOnlyContextKey(p.Spec)+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
					return err
				}
				copy := p
				checks.Rows[len(checks.Rows)-1].HeatOnlyContext = &copy
			}
			if p.Spec.Zone == "" {
				break
			}
		}
	}
	return nil
}

// Mandatory finite source census supplements (and is sealed by) the existing
// HeatOnly row registry. Rebuild native proof once per validation boundary,
// not once per target field. No candidate metadata authorizes a source role.
func epathSQLHeatOnlyContextCensus(checks epathSQLModelChecks) error {
	if checks.HeatOnly == nil {
		return nil
	}
	native, err := epathSQLReadHeatOnlyContexts(checks.HeatOnly)
	if err != nil {
		return err
	}
	expected := map[string]epathSQLHeatOnlyContext{}
	for _, p := range native {
		expected[epathSQLHeatOnlyContextKey(p.Spec)] = p
	}
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, row := range checks.Rows {
		p := row.HeatOnlyContext
		if p == nil {
			continue
		}
		key := epathSQLHeatOnlyContextKey(p.Spec)
		want, ok := expected[key]
		if !ok || !reflect.DeepEqual(want, *p) || row.Quantity == nil || *row.Quantity != (epathSQLQuantity{Value: p.Scalar}) {
			return fmt.Errorf("HeatOnly context retained native proof changed")
		}
		selector := key + "|" + row.Item.Scope + "|" + strings.ToLower(row.Item.Zone) + "|" + row.Item.Target.Field
		if seen[selector] {
			return fmt.Errorf("HeatOnly context selector duplicated")
		}
		seen[selector] = true
		counts[key]++
		if err = epathSQLHeatOnlyContextTarget(row); err != nil {
			return err
		}
	}
	for key, p := range expected {
		want := 4
		if p.Spec.Zone == "" {
			want = 2
		}
		if counts[key] != want {
			return fmt.Errorf("HeatOnly mandatory source context deleted: %s", key)
		}
	}
	return nil
}

func epathSQLHeatOnlyContextTarget(c epathSQLModelCheck) error {
	p := c.HeatOnlyContext
	if p == nil {
		return fmt.Errorf("missing HeatOnly source proof")
	}
	s, t := p.Spec, c.Item.Target
	if c.Item.Group != "loads" || c.Item.Period != "annual" || c.Item.Unit != "kWh" || c.Item.Scope != "building" && c.Item.Scope != "zone" || c.Item.Scope == "building" && c.Item.Zone != "" || c.Item.Scope == "zone" && (s.Zone == "" || !strings.EqualFold(c.Item.Zone, s.Zone)) || c.Quantity == nil || *c.Quantity != (epathSQLQuantity{Value: p.Scalar}) || t.Collection != "sources" || t.Field != "rawValue" && t.Field != "effectiveValue" || t.SourceName != s.Name || !strings.EqualFold(t.SourceKey, s.Key) || t.SourceUnit != s.Unit || t.Frequency != s.Frequency || t.Unit != "kWh" {
		return fmt.Errorf("HeatOnly context escaped exact native scalar target")
	}
	return nil
}

func epathCheckSQLHeatOnlyContextSource(bundle PurposeResultBundle, c epathSQLModelCheck) error {
	if err := epathSQLHeatOnlyContextTarget(c); err != nil {
		return err
	}
	p := c.HeatOnlyContext
	s := p.Spec
	id := fmt.Sprintf("sql-rdd-%d", p.DictionaryIndex)
	count := 0
	for _, a := range bundle.EnergyExplanation.Sources {
		if a.ID != id && !(strings.EqualFold(a.Name, s.Name) && strings.EqualFold(a.KeyValue, s.Key) && a.ReportingFrequency == s.Frequency) {
			continue
		}
		count++
		method, application, component := "integrate_rate_by_time_interval", "already_model_total", "load.delivered.combined"
		if s.Unit == "J" {
			method = "sum_report_data"
		}
		if s.Zone != "" {
			component = "load.predicted.sensible"
			if strings.HasPrefix(s.Name, "Zone Predicted ") {
				application = "requires_zone_multiplier"
			}
		}
		if a.ID != id || a.SourceType != "sql_report_data" || a.IsMeter || a.Name != s.Name || !strings.EqualFold(a.KeyValue, s.Key) || !strings.EqualFold(a.ZoneName, s.Zone) || a.Units != s.Unit || a.SourceUnit != s.Unit || a.NormalizedUnit != "kWh" || a.ReportingFrequency != s.Frequency || a.IndexGroup != "System" || a.AggregationMethod != method || a.AggregationBasis != "model_total" || a.EffectiveMultiplier != 1 || a.MultiplierApplication != application || !reflect.DeepEqual(a.ObjectIndex, p.OutputIndex) || a.DriverRole != "context" || a.InspectorSection != "Context" || a.DriverCategory != "load."+s.Service || a.DriverComponent != component || a.HeatDirection != s.Service || a.Formula != "" || len(a.InputSourceIDs) != 0 {
			return fmt.Errorf("HeatOnly context identity/owner/request/multiplier/role changed")
		}
		if !a.inspectorDecodedFromJSON || a.inspectorValuePresence&3 != 3 || a.RawValue != p.Scalar || a.EffectiveValue != p.Scalar {
			return fmt.Errorf("HeatOnly signed/known-zero source scalar lost native transport")
		}
		// Ordinary context serialization echoes effectiveValue as allocatedValue
		// even without allocation. It is not an allocated graph contribution.
		if a.AllocationApplied || a.AllocationFormula != "" || a.AllocationExplanation != "" || a.AllocationFactor != 0 && a.AllocationFactor != 1 || a.AllocatedValue != p.Scalar {
			return fmt.Errorf("HeatOnly context acquired an allocation or changed its native echo")
		}
		detailKeys := map[string]bool{}
		for _, d := range a.ScopeDetails {
			key := d.Scope.Kind + "|" + strings.ToLower(d.Scope.ZoneName)
			allowed := d.Scope.Kind == "building" && d.Scope.ZoneName == ""
			if d.Scope.Kind == "zone" {
				for _, zone := range p.ServedZones {
					allowed = allowed || strings.EqualFold(zone, d.Scope.ZoneName)
				}
			}
			if !allowed || detailKeys[key] || d.Scope.Kind == "zone" && s.Zone != "" && !strings.EqualFold(d.Scope.ZoneName, s.Zone) || !d.inspectorDecodedFromJSON || d.inspectorValuePresence&3 != 3 || d.RawValue != p.Scalar || d.EffectiveValue != p.Scalar || d.EffectiveMultiplier != 1 || d.MultiplierApplication != application || d.AggregationBasis != "model_total" || d.AllocationApplied || d.AllocatedValue != p.Scalar || d.AllocationFactor != 0 && d.AllocationFactor != 1 {
				return fmt.Errorf("HeatOnly context cache changed signed observation or claimed allocation")
			}
			detailKeys[key] = true
		}
		if c.Item.Scope == "zone" && !detailKeys["zone|"+strings.ToLower(c.Item.Zone)] {
			return fmt.Errorf("HeatOnly requested Zone context cache missing")
		}
		if s.Frequency == "Monthly" {
			if a.HourlyEnergy != nil {
				return fmt.Errorf("HeatOnly Monthly source fabricated hourly values")
			}
		} else {
			if len(p.Hourly) != 8760 || a.HourlyEnergy == nil || a.HourlyEnergy.Unit != "kWh" || a.HourlyEnergy.Basis != "reported_source" || !reflect.DeepEqual(a.HourlyEnergy.Values, p.Hourly) || len(bundle.EnergyExplanation.HourlyLabels) != 8760 {
				return fmt.Errorf("HeatOnly context lost actual native hourly values")
			}
			for n, label := range bundle.EnergyExplanation.HourlyLabels {
				date := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Hour)
				if label != fmt.Sprintf("%02d-%02d %02d:00", date.Month(), date.Day(), date.Hour()+1) {
					return fmt.Errorf("HeatOnly context hourly axis changed")
				}
			}
		}
	}
	if count != 1 {
		return fmt.Errorf("HeatOnly context missing/duplicate original source")
	}
	return nil
}

// Reuse original graph-wrapper coverage and source dependencies. A metadata
// context label never excuses any node/link input, including a zero edge.
func epathSQLCheckHeatOnlyContextGraph(bundle PurposeResultBundle, checks epathSQLModelChecks) error {
	if checks.HeatOnly == nil {
		return nil
	}
	denied := map[string]bool{}
	for _, c := range checks.Rows {
		if c.HeatOnlyContext != nil {
			denied[fmt.Sprintf("sql-rdd-%d", c.HeatOnlyContext.DictionaryIndex)] = true
		}
	}
	if len(denied) != 28 {
		return fmt.Errorf("HeatOnly graph lost mandatory context identities")
	}
	sources, err := epathSQLDirectHVACSourceMap(bundle)
	if err != nil {
		return err
	}
	var reaches func(string, map[string]bool) bool
	reaches = func(id string, seen map[string]bool) bool {
		if denied[id] {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		for _, parent := range sources[id].InputSourceIDs {
			if reaches(parent, seen) {
				return true
			}
		}
		return false
	}
	var coverage epathSQLModelCoverageReport
	for _, context := range epathSQLCoverageContexts(bundle, &coverage) {
		for _, node := range context.nodes {
			for _, id := range node.SourceIDs {
				if reaches(id, map[string]bool{}) {
					return fmt.Errorf("HeatOnly source-only context became a graph node input")
				}
			}
		}
		for _, link := range context.links {
			for _, id := range link.SourceIDs {
				if reaches(id, map[string]bool{}) {
					return fmt.Errorf("HeatOnly source-only context became a graph flow input")
				}
			}
		}
	}
	if len(coverage.Failures) > 0 {
		return fmt.Errorf("HeatOnly context original graph wrappers invalid")
	}
	return nil
}
