package simulation

import (
	"database/sql"
	"fmt"
	"math"
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

type energyDriverDirectionTime struct {
	month, day, hour, minute int
	hours                    float64
	intervalType             string
}

type energyDriverDirectionalSource struct {
	values  energyDriverDirectionalValues
	seen    map[int]int
	last    int64
	hasLast bool
}

type energyDriverDirectionalCollector struct {
	times  map[int64]energyDriverDirectionTime
	counts map[int]map[int]int
	valid  map[int]map[int]bool
	series map[int]*energyDriverDirectionalSource
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
WHERE EnvironmentPeriodIndex=? AND (WarmupFlag=0 OR WarmupFlag IS NULL) AND IntervalType IN (0,1,2) ORDER BY TimeIndex`, environment)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := &energyDriverDirectionalCollector{times: map[int64]energyDriverDirectionTime{}, counts: map[int]map[int]int{0: {}, 1: {}, 2: {}}, valid: map[int]map[int]bool{0: {}, 1: {}, 2: {}}, series: map[int]*energyDriverDirectionalSource{}}
	previous := map[int]time.Time{}
	starts, ends := map[int]map[int]time.Time{0: {}, 1: {}, 2: {}}, map[int]map[int]time.Time{0: {}, 1: {}, 2: {}}
	durations := map[int]map[int]float64{0: {}, 1: {}, 2: {}}
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
		out.times[id] = energyDriverDirectionTime{month: month, day: day, hour: hour, minute: minute, hours: interval / 60, intervalType: fmt.Sprint(intervalType)}
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
	if t.intervalType != fmt.Sprint(rank) {
		return
	}
	state := collector.series[row.DictionaryIndex]
	if state == nil {
		state = &energyDriverDirectionalSource{values: energyDriverDirectionalValues{gain: map[int]float64{}, loss: map[int]float64{}, observed: map[int]bool{}, sourceIDs: []string{fmt.Sprintf("sql-rdd-%d", row.DictionaryIndex)}}, seen: map[int]int{}}
		for month, valid := range collector.valid[rank] {
			state.values.observed[month] = valid
		}
		collector.series[row.DictionaryIndex] = state
	}
	duplicate := state.hasLast && state.last == row.TimeIndex
	state.hasLast, state.last = true, row.TimeIndex
	if duplicate || !row.Value.Valid || !energyPathFinite(row.Value.Float64) || !row.Month.Valid || !row.Day.Valid || !row.Hour.Valid || !row.Minute.Valid || int(row.Month.Int64) != t.month || int(row.Day.Int64) != t.day || int(row.Hour.Int64) != t.hour || int(row.Minute.Int64) != t.minute || row.IntervalType != t.intervalType {
		state.values.observed[t.month] = false
		return
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
	for month, valid := range state.values.observed {
		state.values.observed[month] = valid && state.seen[month] == collector.counts[rank][month] && collector.counts[rank][month] > 0 && energyPathFinite(state.values.gain[month]) && energyPathFinite(state.values.loss[month])
	}
	return &state.values
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
