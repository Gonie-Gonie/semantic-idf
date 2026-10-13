package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Gross directions retain native interval evidence separately from the signed
// net used in heat-balance equations. Clipping an already summed month, or
// subtracting clipped reconciliation terms, cannot reconstruct this evidence.
type energyDriverDirectionalValues struct {
	gain, loss map[int]float64
	observed   map[int]bool
	sourceIDs  []string
	effective  bool
}

type energyNativeReportTime struct {
	id                       int64
	month, day, hour, minute int
	hours                    float64
	end                      time.Time
	valid                    bool
	intervalType             string
}

// Time.IntervalType is not a reliable domain marker: native EnergyPlus can
// write Zone Timestep as EachCall (-1). Source dictionary frequency selects
// the zone axis; unrelated Detailed/HVAC System Timestep rows never join it.
func energyNativeIntervalMonthComplete(times []energyNativeReportTime, references ...[]energyNativeReportTime) bool {
	if len(times) == 0 {
		return false
	}
	for index, item := range times {
		if !item.valid || index > 0 && item.end.Sub(times[index-1].end) != time.Duration(item.hours*float64(time.Hour)) {
			return false
		}
	}
	start, end := times[0].end.Add(-time.Duration(times[0].hours*float64(time.Hour))), times[len(times)-1].end
	if start.Hour() != 0 || start.Minute() != 0 || end.Hour() != 0 || end.Minute() != 0 || !end.After(start) {
		return false
	}
	for _, reference := range references {
		if len(reference) > 0 && (!energyNativeIntervalMonthComplete(reference) || !start.Equal(reference[0].end.Add(-time.Duration(reference[0].hours*float64(time.Hour)))) || !end.Equal(reference[len(reference)-1].end)) {
			return false
		}
	}
	return true
}

type energyDriverDirectionalSource struct {
	values  energyDriverDirectionalValues
	seen    map[int]int
	last    int64
	hasLast bool
}

type energyDriverDirectionalCollector struct {
	times             map[int64]energyNativeReportTime
	counts            map[int]map[int]int
	valid             map[int]map[int]bool
	series            map[int]*energyDriverDirectionalSource
	zoneTimeIDs       map[int64]bool
	zoneAxisFinalized bool
	months            map[int]bool
}

func energyHeatDirectionFrequencyRank(frequency string) int {
	switch strings.ToLower(strings.TrimSpace(frequency)) {
	case "timestep", "zone timestep":
		return 0
	case "hourly":
		return 1
	default:
		return -1
	}
}

func newEnergyDriverDirectionalCollector(db *sql.DB, enabled bool) *energyDriverDirectionalCollector {
	if !enabled {
		return nil
	}
	var count, environment int
	if err := db.QueryRow(`SELECT COUNT(*),COALESCE(MIN(EnvironmentPeriodIndex),0) FROM EnvironmentPeriods WHERE EnvironmentType=3`).Scan(&count, &environment); err != nil || count != 1 {
		return nil
	}
	rows, err := db.Query(`SELECT TimeIndex,Year,Month,Day,Hour,Minute,"Interval",IntervalType FROM "Time"
WHERE EnvironmentPeriodIndex=? AND (WarmupFlag=0 OR WarmupFlag IS NULL) AND IntervalType IN (-1,0,1,2) ORDER BY TimeIndex`, environment)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := &energyDriverDirectionalCollector{times: map[int64]energyNativeReportTime{}, counts: map[int]map[int]int{-1: {}, 0: {}, 1: {}, 2: {}}, valid: map[int]map[int]bool{-1: {}, 0: {}, 1: {}, 2: {}}, series: map[int]*energyDriverDirectionalSource{}, zoneTimeIDs: map[int64]bool{}, months: map[int]bool{}}
	previous := map[int]time.Time{}
	starts, ends := map[int]map[int]time.Time{-1: {}, 0: {}, 1: {}, 2: {}}, map[int]map[int]time.Time{-1: {}, 0: {}, 1: {}, 2: {}}
	durations := map[int]map[int]float64{-1: {}, 0: {}, 1: {}, 2: {}}
	for rows.Next() {
		var id int64
		var year, month, day, hour, minute, intervalType int
		var interval float64
		if rows.Scan(&id, &year, &month, &day, &hour, &minute, &interval, &intervalType) != nil {
			return nil
		}
		rank := intervalType
		valid := year > 0 && month >= 1 && month <= 12 && day >= 1 && day <= time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day() && hour >= 0 && hour <= 24 && minute >= 0 && minute < 60 && (hour < 24 || minute == 0) && energyPathPositiveFinite(interval) && (interval <= 60 || rank == 2 && interval == 1440)
		if rank == 1 {
			valid = valid && hour >= 1 && minute == 0 && interval == 60
		}
		if rank == 2 {
			valid = valid && hour == 24 && minute == 0 && interval == 1440
		}
		end := time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.UTC)
		nativeValid := valid
		if last, exists := previous[rank]; exists && end.Sub(last) != time.Duration(interval*float64(time.Minute)) {
			valid = false
		}
		previous[rank] = end
		if out.counts[rank][month] == 0 {
			out.valid[rank][month] = true
			starts[rank][month] = end.Add(-time.Duration(interval * float64(time.Minute)))
		}
		ends[rank][month] = end
		durations[rank][month] += interval
		out.valid[rank][month] = out.valid[rank][month] && valid
		out.counts[rank][month]++
		if _, duplicate := out.times[id]; duplicate {
			return nil
		}
		out.times[id] = energyNativeReportTime{id: id, month: month, day: day, hour: hour, minute: minute, hours: interval / 60, end: end, valid: nativeValid, intervalType: fmt.Sprint(intervalType)}
		out.months[month] = true
	}
	if rows.Err() != nil {
		return nil
	}
	for rank, months := range out.counts {
		for month := range months {
			start, end := starts[rank][month], ends[rank][month]
			out.valid[rank][month] = out.valid[rank][month] && start.Hour() == 0 && start.Minute() == 0 && end.Hour() == 0 && end.Minute() == 0 && end.After(start) && math.Abs(end.Sub(start).Minutes()-durations[rank][month]) < 1e-8
			for _, other := range []int{1, 2} {
				if other != rank && out.counts[other][month] > 0 {
					out.valid[rank][month] = out.valid[rank][month] && start.Equal(starts[other][month]) && end.Equal(ends[other][month])
				}
			}
		}
	}
	return out
}

func (collector *energyDriverDirectionalCollector) observe(row SQLSeriesRow, dictionary energyExplanationDictionary) {
	if collector == nil || dictionary.heat == nil || energyHeatAliasExplicitSign(dictionary.row.name) != "" {
		return
	}
	rank := energyHeatDirectionFrequencyRank(dictionary.reportingFrequency)
	if rank < 0 {
		return
	}
	t, included := collector.times[row.TimeIndex]
	if !included {
		return
	}
	if rank == 0 && t.intervalType != "0" && t.intervalType != "-1" || rank == 1 && t.intervalType != "1" {
		return
	}
	state := collector.series[row.DictionaryIndex]
	if state == nil {
		state = &energyDriverDirectionalSource{values: energyDriverDirectionalValues{gain: map[int]float64{}, loss: map[int]float64{}, observed: map[int]bool{}, sourceIDs: []string{fmt.Sprintf("sql-rdd-%d", row.DictionaryIndex)}}, seen: map[int]int{}}
		for month := range collector.months {
			state.values.observed[month] = rank == 0 || collector.valid[rank][month]
		}
		collector.series[row.DictionaryIndex] = state
	}
	duplicate := state.hasLast && state.last == row.TimeIndex
	state.hasLast, state.last = true, row.TimeIndex
	if duplicate || !row.Value.Valid || !energyPathFinite(row.Value.Float64) || !row.Month.Valid || !row.Day.Valid || !row.Hour.Valid || !row.Minute.Valid || int(row.Month.Int64) != t.month || int(row.Day.Int64) != t.day || int(row.Hour.Int64) != t.hour || int(row.Minute.Int64) != t.minute || row.IntervalType != t.intervalType {
		state.values.observed[t.month] = false
		return
	}
	if rank == 0 {
		collector.zoneTimeIDs[row.TimeIndex] = true
	}
	value, unit := convertEnergySQLValue(row.Value.Float64, dictionary.row.units)
	if energyExplanationIntegratesRate(dictionary) {
		switch normalizeUnitToken(dictionary.row.units) {
		case "w":
			value, unit = row.Value.Float64*t.hours/1000, "kWh"
		case "kw":
			value, unit = row.Value.Float64*t.hours, "kWh"
		default:
			unit = ""
		}
	}
	if unit != "kWh" || !energyPathFinite(value) {
		state.values.observed[t.month] = false
		return
	}
	item := energyExplanationSeriesForBuilder(&energyExplanationSeriesBuilder{dictionary: dictionary}, "")
	value *= energyExplanationHeatSeriesSignMultiplier(item)
	state.values.gain[t.month] += math.Max(value, 0)
	state.values.loss[t.month] += math.Max(-value, 0)
	state.seen[t.month]++
}

func (collector *energyDriverDirectionalCollector) values(dictionary energyExplanationDictionary) *energyDriverDirectionalValues {
	if collector == nil {
		return nil
	}
	state := collector.series[dictionary.row.index]
	if state == nil {
		return nil
	}
	rank := energyHeatDirectionFrequencyRank(dictionary.reportingFrequency)
	collector.finalizeZoneAxis()
	for month, valid := range state.values.observed {
		state.values.observed[month] = valid && collector.valid[rank][month] && state.seen[month] == collector.counts[rank][month] && collector.counts[rank][month] > 0 && energyPathFinite(state.values.gain[month]) && energyPathFinite(state.values.loss[month])
	}
	return &state.values
}

func (collector *energyDriverDirectionalCollector) finalizeZoneAxis() {
	if collector.zoneAxisFinalized {
		return
	}
	collector.zoneAxisFinalized = true
	axes := map[string]map[int][]energyNativeReportTime{"zone": {}, "hourly": {}, "daily": {}}
	ids := make([]int64, 0, len(collector.times))
	for id := range collector.times {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		item := collector.times[id]
		axis := ""
		if collector.zoneTimeIDs[id] {
			axis = "zone"
		} else if item.intervalType == "1" {
			axis = "hourly"
		} else if item.intervalType == "2" {
			axis = "daily"
		}
		if axis != "" {
			axes[axis][item.month] = append(axes[axis][item.month], item)
		}
	}
	collector.counts[0], collector.valid[0] = map[int]int{}, map[int]bool{}
	for month, times := range axes["zone"] {
		collector.counts[0][month] = len(times)
		collector.valid[0][month] = (len(axes["hourly"][month]) > 0 || len(axes["daily"][month]) > 0) && energyNativeIntervalMonthComplete(times, axes["hourly"][month], axes["daily"][month])
	}
}

func preferredEnergyDriverDirections(items []energyExplanationSeries) *energyDriverDirectionalValues {
	var selected energyExplanationSeries
	found := false
	for _, item := range items {
		if item.driverDirections == nil {
			continue
		}
		if !found || energyHeatDirectionFrequencyRank(item.sourceFrequency) < energyHeatDirectionFrequencyRank(selected.sourceFrequency) || energyHeatDirectionFrequencyRank(item.sourceFrequency) == energyHeatDirectionFrequencyRank(selected.sourceFrequency) && energyExplanationSeriesSourcePreferred(item, selected) {
			selected, found = item, true
		}
	}
	if !found {
		return nil
	}
	return selected.driverDirections
}

func attachEnergyDriverCategoryDirections(dictionaries []energyExplanationDictionary, selected map[int]energySurfacePeriodSelection, categories map[string]*energyExplanationCategorySeriesBuilder, collector *energyDriverDirectionalCollector) {
	if collector == nil {
		return
	}
	expected, matched := map[string]map[string]bool{}, map[string]map[string]bool{}
	for _, dictionary := range dictionaries {
		selection, exists := selected[dictionary.row.index]
		if !exists || !selection.monthly {
			continue
		}
		key := energyExplanationCategoryBuilderKey(dictionary, selection.category)
		if expected[key] == nil {
			expected[key] = map[string]bool{}
		}
		expected[key][normalizeEnergySurfaceKey(dictionary.row.keyValue)] = true
	}
	for _, dictionary := range dictionaries {
		selection, exists := selected[dictionary.row.index]
		if !exists || !selection.directional {
			continue
		}
		key := energyExplanationCategoryBuilderKey(dictionary, selection.category)
		category := categories[key]
		if category == nil {
			continue
		}
		gross := collector.values(dictionary)
		if matched[key] == nil {
			matched[key] = map[string]bool{}
		}
		matched[key][normalizeEnergySurfaceKey(dictionary.row.keyValue)] = gross != nil
		if gross == nil {
			continue
		}
		aggregate := category.seriesBuilder.driverDirections
		if aggregate == nil {
			aggregate = &energyDriverDirectionalValues{gain: map[int]float64{}, loss: map[int]float64{}, observed: map[int]bool{}}
			for month := range category.seriesBuilder.monthly {
				aggregate.observed[month] = true
			}
			category.seriesBuilder.driverDirections = aggregate
		}
		for month := range aggregate.observed {
			aggregate.observed[month] = aggregate.observed[month] && gross.observed[month]
			aggregate.gain[month] += gross.gain[month]
			aggregate.loss[month] += gross.loss[month]
		}
		aggregate.sourceIDs = appendUniqueStrings(aggregate.sourceIDs, gross.sourceIDs...)
	}
	for key, category := range categories {
		gross := category.seriesBuilder.driverDirections
		if gross == nil {
			continue
		}
		for physical := range expected[key] {
			if !matched[key][physical] {
				for month := range gross.observed {
					gross.observed[month] = false
				}
			}
		}
	}
}

func splitEnergyDriverDirectionalSeries(series []energyExplanationSeries, sources []EnergyDataSource) []energyExplanationSeries {
	sourceByID := make(map[string]EnergyDataSource, len(sources))
	for _, source := range sources {
		sourceByID[source.ID] = source
	}
	out := make([]energyExplanationSeries, 0, len(series)*2)
	for _, item := range series {
		if item.Stage != "driver" || item.DriverSourceRole != energyDriverSourceRoleMainFlow || item.driverBuildingOnly {
			out = append(out, item)
			continue
		}
		if item.HeatSign != "" {
			direction := energyDriverHeatDirection(item)
			proven := len(item.SourceIDs) > 0 && item.Total >= 0
			for _, id := range item.SourceIDs {
				proven = proven && energyDriverNativeSourceDirection(sourceByID[id]) == direction
			}
			for _, value := range item.Monthly {
				proven = proven && energyPathFinite(value) && value >= 0
			}
			if proven {
				item.heatDirection, item.heatDirectionBasis = direction, "native_direction"
			} else {
				item.heatDirectionBasis = "net_only"
			}
			out = append(out, item)
			continue
		}
		gross := item.driverDirections
		complete := gross != nil && len(item.Monthly) > 0
		for month := range item.Monthly {
			complete = complete && gross != nil && gross.observed[month]
		}
		if !complete {
			item.heatDirectionBasis = "net_only"
			out = append(out, item)
			continue
		}
		for _, direction := range []string{"gain", "loss"} {
			part := item
			part.Monthly, part.RawMonthly = map[int]float64{}, map[int]float64{}
			part.Total, part.RawTotal = 0, 0
			part.Daily, part.RawDaily, part.Hourly, part.RawHourly = nil, nil, nil, nil
			part.HasSelectedRange = false
			part.driverMonthlyShadow = nil
			part.heatDirection, part.heatDirectionBasis = direction, "native_interval"
			part.HeatSign, part.Sign, part.heatSignMultiplier = "positive", "positive", 1
			values := gross.gain
			if direction == "loss" {
				values = gross.loss
				part.HeatSign, part.Sign, part.heatSignMultiplier = "negative", "negative", -1
			}
			for month := range item.Monthly {
				raw := values[month]
				if gross.effective {
					raw /= item.EffectiveMultiplier
				}
				part.RawMonthly[month] = roundedEnergyNumber(raw)
				part.Monthly[month] = roundedEnergyNumber(raw * item.EffectiveMultiplier)
				part.RawTotal += part.RawMonthly[month]
				part.Total += part.Monthly[month]
			}
			part.Total, part.RawTotal = roundedEnergyNumber(part.Total), roundedEnergyNumber(part.RawTotal)
			if part.Total == 0 {
				continue
			}
			part.SourceIDs = appendUniqueStrings(nil, gross.sourceIDs...)
			if strings.HasPrefix(item.Kind, "heat.interzone_zone_") {
				part.SourceIDs = appendUniqueStrings(item.SourceIDs, part.SourceIDs...)
			}
			part.AnnualSourceIDs, part.MonthlySourceIDs = part.SourceIDs, part.SourceIDs
			out = append(out, part)
		}
	}
	return out
}

func energyDriverDirectionHasObservedZero(item energyExplanationSeries, period string) bool {
	if item.heatDirection == "" || (item.heatDirectionBasis != "native_interval" && item.heatDirectionBasis != "native_direction") {
		return false
	}
	var month int
	if _, err := fmt.Sscanf(period, "M%d", &month); err != nil || month < 1 || month > 12 {
		return false
	}
	if _, observed := item.Monthly[month]; !observed {
		return false
	}
	for _, value := range item.Monthly {
		if value != 0 {
			return true
		}
	}
	return false
}

func normalizeEnergyDriverDirectionMetadata(node *EnergyExplanationNode) {
	if node == nil || (node.Level != "driver" && node.Level != "heat") {
		return
	}
	if node.HeatDirection == "" && node.HeatDirectionBasis == "" {
		return
	}
	valid := (node.HeatDirection == "gain" || node.HeatDirection == "loss") && (node.HeatDirectionBasis == "native_interval" || node.HeatDirectionBasis == "native_direction") && energyPathFinite(node.SignedValue) && energyPathFinite(node.EffectiveValue) && node.EffectiveValue >= 0 && math.Abs(math.Abs(node.SignedValue)-node.EffectiveValue) <= 1e-8
	valid = valid && (!node.inspectorDecodedFromJSON || node.inspectorValuePresence&11 == 11)
	valid = valid && (node.HeatDirection == "gain" && node.SignedValue >= 0 || node.HeatDirection == "loss" && node.SignedValue <= 0)
	if !valid {
		node.HeatDirection = ""
		if node.HeatDirectionBasis != "" {
			node.HeatDirectionBasis = "net_only"
		}
	}
}

func qualifyStoredEnergyDriverDirectionSources(nodes []EnergyExplanationNode, sources []EnergyDataSource) {
	sourceByID := map[string]EnergyDataSource{}
	for _, source := range sources {
		sourceByID[source.ID] = source
	}
	for index := range nodes {
		node := &nodes[index]
		if node.Level != "driver" || node.HeatDirection == "" {
			continue
		}
		proven := false
		switch node.HeatDirectionBasis {
		case "native_interval":
			for _, id := range node.SourceIDs {
				if energyHeatDirectionFrequencyRank(sourceByID[id].ReportingFrequency) >= 0 {
					proven = true
				}
			}
		case "native_direction":
			proven = len(node.SourceIDs) > 0
			for _, id := range node.SourceIDs {
				proven = proven && energyDriverNativeSourceDirection(sourceByID[id]) == node.HeatDirection
			}
		}
		if !proven {
			node.HeatDirection, node.HeatDirectionBasis = "", "net_only"
		}
	}
}

func energyDriverNativeSourceDirection(source EnergyDataSource) string {
	if source.SourceType != "sql_report_data" {
		return ""
	}
	if _, supported := energyHeatAliasDefinitionForName(source.Name); !supported {
		return ""
	}
	switch energyHeatAliasExplicitSign(source.Name) {
	case "positive":
		return "gain"
	case "negative":
		return "loss"
	}
	return ""
}
