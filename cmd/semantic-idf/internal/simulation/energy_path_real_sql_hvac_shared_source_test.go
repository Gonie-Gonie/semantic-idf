package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"
)

const epathSQLSharedBoilerEnergy = "Boiler Ancillary Electricity Energy"
const epathSQLSharedBoilerRate = "Boiler Ancillary Electricity Rate"

type epathSQLHVACSharedSourceProof struct {
	Member    epathRealSQLHVACSharedMember
	Source    epathRealSQLSource
	Precision epathRealSQLPrecision
	Hourly    []float64 // Original weather-only values; not a production chart.
	Canonical bool
}

func epathSQLValidateHVACSharedDeclaration(m epathRealSQLHVACSharedMember) error {
	if strings.TrimSpace(m.ID) == "" || m.ObjectType != "Boiler:HotWater" || strings.TrimSpace(m.ObjectName) == "" || strings.TrimSpace(m.PlantLoopName) == "" || len(m.ServedZones) == 0 || m.Source.IsMeter || m.Source.AllowAbsent || len(m.Source.Keys) != 1 || !strings.EqualFold(m.Source.Keys[0], m.ObjectName) || len(m.Source.Alternatives) != 1 || m.Source.Alternatives[0] != (epathRealSQLAlternative{Name: epathSQLSharedBoilerEnergy, Unit: "J"}) {
		return fmt.Errorf("shared constituent requires exact original hot-water boiler and native ancillary Monthly/J selector")
	}
	seen := map[string]bool{}
	for _, zone := range m.ServedZones {
		key := epathSQLSharedKey(zone)
		if key == "" || key == "*" || seen[key] {
			return fmt.Errorf("shared boiler requires unique exact served Zones")
		}
		seen[key] = true
	}
	return nil
}

func epathSQLValidateHVACSharedObservation(source epathRealSQLSource, member epathRealSQLHVACSharedMember, precision epathRealSQLPrecision) error {
	if err := epathSQLValidateHVACSharedDeclaration(member); err != nil {
		return err
	}
	unit := "J"
	if source.Name == epathSQLSharedBoilerRate {
		unit = "W"
	} else if source.Name != epathSQLSharedBoilerEnergy {
		return fmt.Errorf("shared source is not a reviewed native boiler quantity")
	}
	hourly := source.ReportingFrequency == "Hourly"
	rows := 12
	if hourly {
		rows = 8760
	} else if source.ReportingFrequency != "Monthly" {
		return fmt.Errorf("unsupported shared native frequency")
	}
	if source.DictionaryIndex <= 0 || source.IsMeter || source.SourceUnit != unit || !strings.EqualFold(source.KeyValue, member.ObjectName) || source.Rows != rows || source.MissingRows != 0 || len(source.Months) != 12 || source.RawSum == nil || !epathOracleFinite(*source.RawSum) || *source.RawSum < 0 || source.EnergyKWh == nil || !epathOracleFinite(*source.EnergyKWh) || *source.EnergyKWh < 0 {
		return fmt.Errorf("shared source lacks complete finite native identity/observations")
	}
	if precision.DecimalPlaces != 3 || precision.SourceStages < 1 || precision.SourceStages > 3 || precision.ContributionStages < 1 || precision.ContributionStages > 3 {
		return fmt.Errorf("invalid shared source precision")
	}
	rawTotal, energyTotal := 0.0, 0.0
	for n, b := range source.Months {
		hours := time.Date(2017, time.Month(n+2), 0, 0, 0, 0, 0, time.UTC).Day() * 24
		wantRows := 1
		if hourly {
			wantRows = hours
		}
		if b.Month != n+1 || b.Rows != wantRows || b.MissingRows != 0 || b.RawSum == nil || !epathOracleFinite(*b.RawSum) || *b.RawSum < 0 || b.EnergyKWh == nil || !epathOracleFinite(*b.EnergyKWh) || *b.EnergyKWh < 0 {
			return fmt.Errorf("shared source has missing/negative/nonfinite native M%d", n+1)
		}
		want := *b.RawSum / 3600000
		if unit == "W" {
			want = *b.RawSum / 1000
			if !hourly {
				want *= float64(hours)
			}
		}
		if !epathSQLFanPoolNear(want, *b.EnergyKWh) || (want == 0) != (*b.EnergyKWh == 0) {
			return fmt.Errorf("shared source M%d violates native J or W-duration normalization", n+1)
		}
		rawTotal += *b.RawSum
		energyTotal += *b.EnergyKWh
	}
	if !epathSQLFanPoolNear(rawTotal, *source.RawSum) || !epathSQLFanPoolNear(energyTotal, *source.EnergyKWh) || (energyTotal == 0) != (*source.EnergyKWh == 0) {
		return fmt.Errorf("shared source annual total differs from its native months")
	}
	return nil
}

func epathSQLValidateHVACSharedSourceIdentity(identity epathSQLHVACSharedSourceIdentity) error {
	if identity.Source.Name != epathSQLSharedBoilerEnergy || identity.Source.ReportingFrequency != "Monthly" {
		return fmt.Errorf("shared additive authority must be the native Monthly energy, not an E/R alias")
	}
	if err := epathSQLValidateHVACSharedObservation(identity.Source, identity.Member, identity.Precision); err != nil {
		return err
	}
	if len(identity.Companions) != 3 {
		return fmt.Errorf("shared source proof requires the three separately observed native companions")
	}
	seen := map[string]bool{identity.Source.Name + "|" + identity.Source.ReportingFrequency: true}
	ids := map[int]bool{identity.Source.DictionaryIndex: true}
	for _, source := range identity.Companions {
		key := source.Name + "|" + source.ReportingFrequency
		if seen[key] || ids[source.DictionaryIndex] {
			return fmt.Errorf("shared source proof duplicates a physical observation")
		}
		seen[key] = true
		ids[source.DictionaryIndex] = true
		if err := epathSQLValidateHVACSharedObservation(source, identity.Member, identity.Precision); err != nil {
			return err
		}
		for n, b := range source.Months {
			if !epathSQLFanPoolNear(*b.EnergyKWh, *identity.Source.Months[n].EnergyKWh) || (*b.EnergyKWh == 0) != (*identity.Source.Months[n].EnergyKWh == 0) {
				return fmt.Errorf("shared companion differs from independent native Monthly energy")
			}
		}
	}
	return nil
}

func epathSQLValidateHVACConsumptionPoolFrame(frame epathSQLHVACConsumptionPoolFrame) error {
	if frame.Declaration.SiteID == "" || len(frame.Declaration.Shared) == 0 || len(frame.Shared) != len(frame.Declaration.Shared) {
		return fmt.Errorf("shared source frame lacks its literal nonempty pool declaration")
	}
	seenIDs, seenSources := map[string]bool{}, map[int]bool{}
	for n, identity := range frame.Shared {
		if !reflect.DeepEqual(identity.Member, frame.Declaration.Shared[n]) || seenIDs[identity.Member.ID] {
			return fmt.Errorf("shared frame changed/duplicated its declared member")
		}
		seenIDs[identity.Member.ID] = true
		if err := epathSQLValidateHVACSharedSourceIdentity(identity); err != nil {
			return err
		}
		for _, source := range append([]epathRealSQLSource{identity.Source}, identity.Companions...) {
			if seenSources[source.DictionaryIndex] {
				return fmt.Errorf("shared frame reused an observed source")
			}
			seenSources[source.DictionaryIndex] = true
		}
	}
	return nil
}

// Re-query every physical row rather than trusting aggregate summaries. This
// rejects a duplicate zero dictionary, negative offsetting rows, NULLs, absent
// months and wrong intervals even when an apparent annual sum would match.
func epathSQLAuditHVACSharedObservation(db *sql.DB, source epathRealSQLSource, weather epathRealSQLWeather) ([]float64, error) {
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,Name,KeyValue,IsMeter,ReportingFrequency,Units FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=? OR (Name=? COLLATE NOCASE AND KeyValue=? COLLATE NOCASE AND ReportingFrequency=? COLLATE NOCASE)`, source.DictionaryIndex, source.Name, source.KeyValue, source.ReportingFrequency)
	if err != nil {
		return nil, err
	}
	count := 0
	for rows.Next() {
		var id, meter int
		var name, key, frequency, unit string
		if err := rows.Scan(&id, &name, &key, &meter, &frequency, &unit); err != nil {
			rows.Close()
			return nil, err
		}
		count++
		if id != source.DictionaryIndex || meter != 0 || name != source.Name || !strings.EqualFold(key, source.KeyValue) || frequency != source.ReportingFrequency || unit != source.SourceUnit {
			rows.Close()
			return nil, fmt.Errorf("shared source has duplicate/conflicting exact native dictionary")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, fmt.Errorf("shared source needs one exact original dictionary")
	}
	rows, err = db.Query(`SELECT t.TimeIndex,t.EnvironmentPeriodIndex,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,r.Value FROM ReportData r JOIN Time t USING(TimeIndex) JOIN EnvironmentPeriods e USING(EnvironmentPeriodIndex) WHERE r.ReportDataDictionaryIndex=? AND e.EnvironmentType=3 AND `+epathOracleNonWarmupSQL+` ORDER BY t.TimeIndex`, source.DictionaryIndex)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hourly := source.ReportingFrequency == "Hourly"
	seen := map[int]bool{}
	sums, rawSums := [12]float64{}, [12]float64{}
	var hours []float64
	if hourly {
		hours = make([]float64, 8760)
	}
	for rows.Next() {
		var index, environment, year, month, day, hour, minute, kind int
		var interval, value sql.NullFloat64
		if err := rows.Scan(&index, &environment, &year, &month, &day, &hour, &minute, &interval, &kind, &value); err != nil {
			return nil, err
		}
		date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
		if environment != weather.EnvironmentIndex || year != 2017 || month < 1 || month > 12 || day < 1 || date.Month() != time.Month(month) || date.Day() != day || minute != 0 || !interval.Valid || !value.Valid || !epathOracleFinite(value.Float64) || value.Float64 < 0 {
			return nil, fmt.Errorf("shared native source has an invalid/unknown weather interval at %d", index)
		}
		slot := month - 1
		if hourly {
			slot = (date.YearDay()-1)*24 + hour - 1
			if kind != 1 || hour < 1 || hour > 24 || interval.Float64 != 60 || slot < 0 || slot >= 8760 {
				return nil, fmt.Errorf("shared native Hourly source has an invalid exact calendar slot")
			}
		} else if kind != 3 || hour != 24 || day != last || interval.Float64 != float64(last*24*60) {
			return nil, fmt.Errorf("shared native Monthly source lacks an exact full-month interval")
		}
		if seen[slot] {
			return nil, fmt.Errorf("shared native source repeats a calendar slot")
		}
		seen[slot] = true
		energy := value.Float64 / 3600000
		if source.SourceUnit == "W" {
			energy = value.Float64 * (interval.Float64 / 60) / 1000
		}
		if !epathOracleFinite(energy) {
			return nil, fmt.Errorf("shared normalized source is nonfinite")
		}
		sums[month-1] += energy
		rawSums[month-1] += value.Float64
		if hourly {
			hours[slot] = energy
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(seen) != source.Rows {
		return nil, fmt.Errorf("shared native source lacks its complete actual calendar")
	}
	for n, sum := range sums {
		if !epathSQLFanPoolNear(sum, *source.Months[n].EnergyKWh) || !epathSQLFanPoolNear(rawSums[n], *source.Months[n].RawSum) {
			return nil, fmt.Errorf("shared source summary changed its original SQL rows")
		}
	}
	return hours, nil
}

func epathCompileSQLHVACConsumptionPoolFrames(sqlPath string, observed []epathRealSQLSource, model epathRealSQLModel, frames *epathSQLFrames, plan *PurposeRunPlan) error {
	if len(model.HVACConsumptionPools) == 0 {
		return nil
	}
	if frames == nil || len(frames.HVACConsumptionPools) != 0 || len(frames.HVACSharedSourceIdentities) != 0 {
		return fmt.Errorf("shared compiler requires fresh independent frames")
	}
	db, err := epathOpenOracleSQL(sqlPath)
	if err != nil {
		return err
	}
	defer db.Close()
	weather, err := epathReadOracleWeather(db)
	if err != nil {
		return err
	}
	pools := []epathSQLHVACConsumptionPoolFrame{}
	proofs := map[int]epathSQLHVACSharedSourceProof{}
	sites := map[string]bool{}
	for _, declaration := range model.HVACConsumptionPools {
		if sites[declaration.SiteID] {
			return fmt.Errorf("shared source pool site is repeated")
		}
		sites[declaration.SiteID] = true
		pool := epathSQLHVACConsumptionPoolFrame{Declaration: declaration}
		siteCount := 0
		for _, site := range model.Site {
			if site.ID == declaration.SiteID {
				siteCount++
				if site.Facility || site.EndUse != "heating" || site.Carrier != "electricity" || site.Tabular != nil || len(frames.Site[site.ID]) != 12 {
					return fmt.Errorf("shared boiler source requires the actual Monthly Heating electricity site")
				}
			}
		}
		if siteCount != 1 {
			return fmt.Errorf("shared boiler lacks one exact parent site")
		}
		for _, member := range declaration.Shared {
			if err := epathSQLValidateHVACSharedDeclaration(member); err != nil {
				return err
			}
			for _, zone := range member.ServedZones {
				if !strings.EqualFold(frames.Zones[epathSQLSharedKey(zone)].Name, zone) {
					return fmt.Errorf("shared original served Zone is absent from SQL")
				}
			}
			identity := epathSQLHVACSharedSourceIdentity{Member: member, Precision: model.Precision}
			for _, spec := range []struct{ name, frequency string }{{epathSQLSharedBoilerEnergy, "Monthly"}, {epathSQLSharedBoilerRate, "Monthly"}, {epathSQLSharedBoilerEnergy, "Hourly"}, {epathSQLSharedBoilerRate, "Hourly"}} {
				if err := epathSQLHVACSharedTemporaryRequest(plan, member.ObjectName, spec.name, spec.frequency); err != nil {
					return err
				}
				var source epathRealSQLSource
				count := 0
				for _, candidate := range observed {
					if strings.EqualFold(candidate.Name, spec.name) && strings.EqualFold(candidate.KeyValue, member.ObjectName) && strings.EqualFold(candidate.ReportingFrequency, spec.frequency) {
						source = candidate
						count++
					}
				}
				if count != 1 {
					return fmt.Errorf("shared boiler needs one observed native %s/%s", spec.name, spec.frequency)
				}
				if err := epathSQLValidateHVACSharedObservation(source, member, model.Precision); err != nil {
					return err
				}
				hourly, err := epathSQLAuditHVACSharedObservation(db, source, weather)
				if err != nil {
					return err
				}
				if proofs[source.DictionaryIndex].Source.DictionaryIndex != 0 || frames.SourceIdentities[source.DictionaryIndex].DictionaryIndex != 0 {
					return fmt.Errorf("shared boiler observation was already assigned another independent role")
				}
				canonical := spec.name == epathSQLSharedBoilerEnergy && spec.frequency == "Monthly"
				if canonical {
					identity.Source = source
				} else {
					identity.Companions = append(identity.Companions, source)
				}
				proofs[source.DictionaryIndex] = epathSQLHVACSharedSourceProof{Member: member, Source: source, Precision: model.Precision, Hourly: hourly, Canonical: canonical}
			}
			pool.Shared = append(pool.Shared, identity)
		}
		if err := epathSQLValidateHVACConsumptionPoolFrame(pool); err != nil {
			return err
		}
		pools = append(pools, pool)
	}
	frames.HVACConsumptionPools, frames.HVACSharedSourceIdentities = pools, proofs
	if frames.SourceIdentities == nil {
		frames.SourceIdentities = map[int]epathRealSQLSource{}
	}
	for id, proof := range proofs {
		frames.SourceIdentities[id] = proof.Source
	}
	return nil
}

func epathSQLHVACSharedTemporaryRequest(plan *PurposeRunPlan, key, name, frequency string) error {
	if plan == nil {
		return fmt.Errorf("shared boiler proof needs the actual output plan")
	}
	count := 0
	for _, output := range plan.OutputObjects {
		if !strings.EqualFold(output.VariableName, name) || !strings.EqualFold(output.ReportingFrequency, frequency) || !(strings.EqualFold(output.KeyValue, key) || output.KeyValue == "*") {
			continue
		}
		count++
		basic := false
		for _, purpose := range output.PurposeIDs {
			if purpose == SimulationPurposeBasicEnergy {
				basic = true
			}
		}
		if output.ObjectType != "Output:Variable" || !strings.EqualFold(output.KeyValue, key) || output.ScopeZoneName != "" || output.State != "temporary" || output.ObjectIndex != nil || !basic {
			return fmt.Errorf("shared boiler source requires an exact temporary request for its actual frequency, not a borrowed original opener")
		}
	}
	if count != 1 {
		return fmt.Errorf("shared boiler needs exactly one actual key/name/frequency request")
	}
	return nil
}

func epathSQLHVACSharedSourceQuantity(p epathSQLHVACSharedSourceProof) (epathSQLQuantity, error) {
	if err := epathSQLValidateHVACSharedObservation(p.Source, p.Member, p.Precision); err != nil {
		return epathSQLQuantity{}, err
	}
	if p.Canonical != (p.Source.Name == epathSQLSharedBoilerEnergy && p.Source.ReportingFrequency == "Monthly") {
		return epathSQLQuantity{}, fmt.Errorf("shared proof changed its Monthly authority/companion role")
	}
	if p.Source.ReportingFrequency == "Hourly" {
		if len(p.Hourly) != 8760 {
			return epathSQLQuantity{}, fmt.Errorf("shared Hourly proof lacks actual row values")
		}
		sums := [12]float64{}
		for n, value := range p.Hourly {
			if !epathOracleFinite(value) || value < 0 {
				return epathSQLQuantity{}, fmt.Errorf("invalid shared Hourly proof value")
			}
			month := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Hour).Month()
			sums[int(month)-1] += value
		}
		for n, sum := range sums {
			if !epathSQLFanPoolNear(sum, *p.Source.Months[n].EnergyKWh) || (sum == 0) != (*p.Source.Months[n].EnergyKWh == 0) {
				return epathSQLQuantity{}, fmt.Errorf("shared Hourly proof changed original month sums")
			}
		}
	} else if len(p.Hourly) != 0 {
		return epathSQLQuantity{}, fmt.Errorf("shared Monthly proof borrowed a companion chart")
	}
	// Native annual consumption is not the sum of transport-rounded hourly
	// chart samples or rounded monthly display amounts. Only floating-point
	// integration roundoff is admitted; a reported zero remains exactly zero.
	q := epathSQLQuantity{Value: *p.Source.EnergyKWh}
	if q.Value != 0 {
		q.Error = 1e-10 * math.Max(1, math.Abs(q.Value))
	}
	return q, nil
}

func epathSQLModelHVACSharedSourceChecks(frames epathSQLFrames, checks *epathSQLModelChecks) error {
	ids := []int{}
	for id := range frames.HVACSharedSourceIdentities {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		proof := frames.HVACSharedSourceIdentities[id]
		if id != proof.Source.DictionaryIndex || !reflect.DeepEqual(frames.SourceIdentities[id], proof.Source) || frames.SourceZone[id] != "" {
			return fmt.Errorf("shared source lost its Building-only original source registration")
		}
		q, err := epathSQLHVACSharedSourceQuantity(proof)
		if err != nil {
			return err
		}
		for _, field := range []string{"rawValue", "effectiveValue"} {
			s := proof.Source
			target := epathRealOracleTarget{Collection: "sources", Field: field, SourceName: s.Name, SourceKey: s.KeyValue, SourceUnit: s.SourceUnit, Frequency: s.ReportingFrequency, Unit: "kWh"}
			if err := checks.add("endUses", "building", "", "annual", fmt.Sprintf("shared_hvac_source/%s/%s/%s/%s", proof.Member.ID, s.Name, s.ReportingFrequency, field), "kWh", &q, target, "", nil, nil); err != nil {
				return err
			}
			copy := proof
			checks.Rows[len(checks.Rows)-1].HVACSharedSource = &copy
		}
	}
	return nil
}

func epathCheckSQLHVACSharedSource(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	p, target := check.HVACSharedSource, check.Item.Target
	if p == nil {
		return fmt.Errorf("missing independent shared source proof")
	}
	q, err := epathSQLHVACSharedSourceQuantity(*p)
	if err != nil {
		return err
	}
	s := p.Source
	if check.Item.Group != "endUses" || check.Item.Scope != "building" || check.Item.Zone != "" || check.Item.Period != "annual" || check.Item.Unit != "kWh" || check.Quantity == nil || !reflect.DeepEqual(*check.Quantity, q) || target.Collection != "sources" || target.Field != "rawValue" && target.Field != "effectiveValue" || target.SourceName != s.Name || target.SourceKey != s.KeyValue || target.SourceUnit != s.SourceUnit || target.Frequency != s.ReportingFrequency || target.Unit != "kWh" {
		return fmt.Errorf("shared source check escaped its exact source/quantity/context")
	}
	count := 0
	for _, actual := range bundle.EnergyExplanation.Sources {
		if actual.ID != fmt.Sprintf("sql-rdd-%d", s.DictionaryIndex) && !(strings.EqualFold(actual.Name, s.Name) && strings.EqualFold(actual.KeyValue, s.KeyValue) && actual.ReportingFrequency == s.ReportingFrequency) {
			continue
		}
		count++
		method := "sum_report_data"
		if s.SourceUnit == "W" {
			method = "integrate_rate_by_time_interval"
		}
		if actual.ID != fmt.Sprintf("sql-rdd-%d", s.DictionaryIndex) || actual.SourceType != "sql_report_data" || actual.IsMeter || actual.Name != s.Name || !strings.EqualFold(actual.KeyValue, s.KeyValue) || actual.SourceUnit != s.SourceUnit || actual.Units != s.SourceUnit || actual.NormalizedUnit != "kWh" || actual.ReportingFrequency != s.ReportingFrequency || actual.ZoneName != "" || actual.AggregationMethod != method || actual.AggregationBasis != "model_total" || actual.EffectiveMultiplier != 1 || actual.MultiplierApplication != "already_model_total" || actual.ObjectIndex != nil || actual.DriverRole != "context" || actual.InspectorSection != "Context" || len(actual.InputSourceIDs) != 0 {
			return fmt.Errorf("shared source lost its native Building identity/temporary opener; a served Zone is not its measured owner")
		}
		value, bit := actual.RawValue, uint8(1)
		if target.Field == "effectiveValue" {
			value, bit = actual.EffectiveValue, 2
		}
		if actual.inspectorDecodedFromJSON && actual.inspectorValuePresence&bit == 0 || !actual.inspectorDecodedFromJSON && actual.observedValuePresence&bit == 0 {
			return fmt.Errorf("shared source scalar is absent, not observed zero")
		}
		if err := epathCheckSQLModelQuantity(&value, &q); err != nil {
			return err
		}
		if !p.Canonical {
			if actual.AllocationApplied || actual.AllocatedValue != 0 || actual.AllocationFactor != 0 {
				return fmt.Errorf("shared native companion was incorrectly allocated as a second consuming source")
			}
			for _, detail := range actual.ScopeDetails {
				if detail.Scope.Kind == "zone" || detail.AllocationApplied || detail.AllocatedValue != 0 {
					return fmt.Errorf("shared native companion leaked a Zone allocation")
				}
			}
		}
		if s.ReportingFrequency == "Hourly" {
			if len(p.Hourly) != 8760 || actual.HourlyEnergy == nil || actual.HourlyEnergy.Unit != "kWh" || actual.HourlyEnergy.Basis != "reported_source" || len(actual.HourlyEnergy.Values) != 8760 || len(bundle.EnergyExplanation.HourlyLabels) != 8760 {
				return fmt.Errorf("shared hourly source lost its original complete chart")
			}
			for n, want := range p.Hourly {
				stamp := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Hour)
				label := fmt.Sprintf("%02d-%02d %02d:00", stamp.Month(), stamp.Day(), stamp.Hour()+1)
				if !epathOracleFinite(want) || want < 0 || actual.HourlyEnergy.Values[n] != math.Round(want*1000)/1000 || bundle.EnergyExplanation.HourlyLabels[n] != label {
					return fmt.Errorf("shared hourly native value/axis changed at %d", n)
				}
			}
		} else if actual.HourlyEnergy != nil {
			return fmt.Errorf("shared Monthly source borrowed an Hourly companion chart")
		}
	}
	if count != 1 {
		return fmt.Errorf("shared source requires one exact candidate observation")
	}
	if !p.Canonical {
		id := fmt.Sprintf("sql-rdd-%d", s.DictionaryIndex)
		sources := map[string]EnergyDataSource{}
		for _, source := range bundle.EnergyExplanation.Sources {
			if _, duplicate := sources[source.ID]; duplicate {
				return fmt.Errorf("duplicate shared candidate source identity")
			}
			sources[source.ID] = source
		}
		var reaches func(string, map[string]bool) bool
		reaches = func(current string, seen map[string]bool) bool {
			if current == id {
				return true
			}
			if seen[current] {
				return false
			}
			seen[current] = true
			for _, parent := range sources[current].InputSourceIDs {
				if reaches(parent, seen) {
					return true
				}
			}
			return false
		}
		coverage := epathSQLModelCoverageReport{}
		for _, context := range epathSQLCoverageContexts(bundle, &coverage) {
			for _, node := range context.nodes {
				for _, sourceID := range node.SourceIDs {
					if reaches(sourceID, map[string]bool{}) {
						return fmt.Errorf("shared companion became an additive graph node source")
					}
				}
			}
			for _, link := range context.links {
				for _, sourceID := range link.SourceIDs {
					if reaches(sourceID, map[string]bool{}) {
						return fmt.Errorf("shared companion became an additive graph link source")
					}
				}
			}
		}
	}
	return nil
}
