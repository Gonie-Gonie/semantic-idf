package idf

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

func enrichProfileGraphDataset(report *ProfileReport) {
	report.GraphDataset = buildProfileGraphDataset(*report)
	report.ScheduleClusters = buildProfileScheduleClusters(*report, report.GraphDataset.Series)
	report.Outliers = buildProfileOutlierHints(*report, report.GraphDataset.Series, report.ScheduleClusters)
	report.ParameterCandidates = buildProfileParameterCandidates(*report, report.GraphDataset.Series, report.Outliers)
	report.GraphDataset.ScheduleClusters = report.ScheduleClusters
	report.GraphDataset.Outliers = report.Outliers
	report.GraphDataset.ParameterCandidates = report.ParameterCandidates
}

func buildProfileGraphDataset(report ProfileReport) ProfileGraphDataset {
	itemMap := profileReportItemMap(report)
	schedules := profileScheduleLookup(report.Schedules)
	groupByZone := profileGroupByZone(report.Groups)
	series := make([]ProfileGraphSeries, 0, len(report.ZoneProfiles)*len(report.Dimensions)+len(report.Groups)*len(report.Dimensions))

	for _, zone := range report.ZoneProfiles {
		group := groupByZone[normalizeName(zone.ZoneName)]
		for _, dimension := range zone.Dimensions {
			series = append(series, profileGraphSeriesForDimension("zone", group, zone.ZoneName, dimension, itemMap, schedules))
		}
	}
	for _, group := range report.Groups {
		for _, dimension := range group.Dimensions {
			series = append(series, profileGraphSeriesForDimension("group", group, "", dimension, itemMap, schedules))
		}
	}
	sort.Slice(series, func(i, j int) bool {
		if series[i].ScopeType != series[j].ScopeType {
			return series[i].ScopeType < series[j].ScopeType
		}
		if series[i].GroupID != series[j].GroupID {
			return series[i].GroupID < series[j].GroupID
		}
		if series[i].ZoneName != series[j].ZoneName {
			return series[i].ZoneName < series[j].ZoneName
		}
		return series[i].Dimension < series[j].Dimension
	})

	return ProfileGraphDataset{
		Series: series,
	}
}

func profileGraphSeriesForDimension(scopeType string, group ProfileGroup, zoneName string, dimension ProfileDimensionSummary, itemMap map[string]ProfileItem, schedules profileScheduleIndex) ProfileGraphSeries {
	schedule := schedules.forDimension(dimension)
	annualMultiplier := annualMultiplierProfile(schedule)
	if len(annualMultiplier) == 0 {
		annualMultiplier = annualProfileFromDayProfiles(filledProfile(1), filledProfile(1), filledProfile(1))
	}
	dayMultiplier := profileGraphDayMultiplier(schedule)
	weekMultiplier := profileGraphWeekMultiplier(schedule)
	monthMultiplier := monthlyAverages(annualMultiplier)
	durationMultiplier := append([]float64(nil), annualMultiplier...)
	sort.Sort(sort.Reverse(sort.Float64Slice(durationMultiplier)))
	ruleMultiplier := profileRuleMultiplier(schedule)
	actualAnnual := multiplyProfile(annualMultiplier, dimension.Value)
	if values, ok := profileDimensionActualValues(dimension, itemMap, schedules, 8760); ok {
		actualAnnual = values
		annualMultiplier = divideProfile(actualAnnual, dimension.Value)
		dayMultiplier = representativeDaysFromAnnual(annualMultiplier)
		weekMultiplier = representativeWeekFromAnnual(annualMultiplier)
		monthMultiplier = monthlyAverages(annualMultiplier)
		durationMultiplier = append([]float64(nil), annualMultiplier...)
		sort.Sort(sort.Reverse(sort.Float64Slice(durationMultiplier)))
		ruleMultiplier = profileRuleMultiplierFromAnnual(annualMultiplier)
	}
	peak := maxFloat64(actualAnnual)
	sourceIndexes := make([]int, 0, len(dimension.ItemIDs))
	var warnings []ProfileWarning
	graphStatus := dimension.Status
	for _, id := range dimension.ItemIDs {
		item := itemMap[id]
		if item.ID != "" && item.ObjectIndex >= 0 {
			sourceIndexes = append(sourceIndexes, item.ObjectIndex)
		}
		warnings = append(warnings, item.Warnings...)
		for _, warning := range item.Warnings {
			if (warning.Code == "schedule_profile_fallback" || warning.Code == "missing_schedule_summary" || warning.Code == "nominal_outdoor_air_profile") && graphStatus == metricStatusOK {
				graphStatus = metricStatusPartial
			}
		}
	}
	scopeID := safeID(zoneName)
	label := zoneName
	if scopeType == "group" {
		scopeID = group.ID
		label = group.Name
	}
	if label == "" {
		label = group.Name
	}
	scheduleName := dimension.ScheduleName
	scheduleHash := dimension.ScheduleHash
	schedulePattern := dimension.SchedulePattern
	if schedule.ScheduleName != "" && !profileDimensionHasMultipleSchedules(dimension) {
		scheduleName = schedule.ScheduleName
		scheduleHash = schedule.ContentHash
		schedulePattern = schedule.DetectedPattern
	}
	return ProfileGraphSeries{
		ID:                        fmt.Sprintf("profile-series-%s-%s-%s", scopeType, safeID(scopeID), safeID(dimension.Dimension)),
		Label:                     fmt.Sprintf("%s / %s", label, dimension.Label),
		ScopeType:                 scopeType,
		GroupID:                   group.ID,
		GroupName:                 group.Name,
		ZoneName:                  zoneName,
		Dimension:                 dimension.Dimension,
		DimensionLabel:            dimension.Label,
		MetricID:                  dimension.MetricID,
		MetricLabel:               dimension.MetricLabel,
		Unit:                      dimension.Unit,
		DesignValue:               dimension.Value,
		DisplayValue:              dimension.DisplayValue,
		ScheduleName:              scheduleName,
		ScheduleHash:              scheduleHash,
		SchedulePattern:           schedulePattern,
		Values:                    append([]float64(nil), actualAnnual...),
		DayMultiplierProfile:      append([]float64(nil), dayMultiplier...),
		WeekMultiplierProfile:     append([]float64(nil), weekMultiplier...),
		MonthMultiplierProfile:    append([]float64(nil), monthMultiplier...),
		AnnualMultiplierProfile:   append([]float64(nil), annualMultiplier...),
		DurationMultiplierProfile: append([]float64(nil), durationMultiplier...),
		RuleMultiplierProfile:     append([]float64(nil), ruleMultiplier...),
		SourceItemIDs:             append([]string(nil), dimension.ItemIDs...),
		SourceObjectIndexes:       uniqueInts(sourceIndexes),
		OperatingHours:            profileOperatingHours(annualMultiplier),
		EquivalentFullHours:       roundedNumber(sumFloat64(annualMultiplier), 1),
		AnnualContribution:        sumFloat64(actualAnnual),
		Peak:                      peak,
		Status:                    graphStatus,
		Warnings:                  warnings,
	}
}

func profileDimensionHasMultipleSchedules(dimension ProfileDimensionSummary) bool {
	return strings.Contains(dimension.ScheduleName, " + ") || strings.Contains(dimension.ScheduleHash, "+")
}

func profileDimensionActualValues(dimension ProfileDimensionSummary, itemMap map[string]ProfileItem, schedules profileScheduleIndex, length int) ([]float64, bool) {
	if length <= 0 || len(dimension.ItemIDs) == 0 {
		return nil, false
	}

	// Ratio metrics must be scheduled through an additive engineering quantity.
	// Summing per-Space ratios (for example W/m2) gives the wrong Zone result
	// whenever the Spaces have different areas. Count, total power, and volume
	// flow remain additive across targets, so apply every object's own schedule to
	// that quantity first and normalize the combined result afterwards.
	numeratorID, inverse := profileGraphNumeratorMetric(dimension.MetricID)
	contributions, designTotal := profileGraphContributions(dimension, numeratorID, itemMap, schedules, length)
	if len(contributions) < maxInt(1, dimension.ResolvedItemCount) && numeratorID != dimension.MetricID {
		// Source/fallback metrics do not always have a derivable extensive
		// counterpart. Preserve them using their own contributions while still
		// combining each item's schedule rather than borrowing the first schedule.
		inverse = false
		contributions, designTotal = profileGraphContributions(dimension, dimension.MetricID, itemMap, schedules, length)
	}
	if len(contributions) == 0 {
		return nil, false
	}
	if designTotal == 0 {
		if dimension.Value != 0 {
			return nil, false
		}
		return make([]float64, length), true
	}

	values := make([]float64, length)
	if inverse {
		basis := dimension.Value * designTotal
		for index := range values {
			actualDenominator := 0.0
			for _, contribution := range contributions {
				actualDenominator += contribution.value * contribution.multiplier[index]
			}
			if actualDenominator > 0 {
				values[index] = basis / actualDenominator
			}
		}
		return values, true
	}

	// Scaling by the authoritative aggregated design value also makes fallback
	// source metrics stable: with all schedules at 1, the graph peak is exactly
	// the value displayed in the Profile table.
	scale := dimension.Value / designTotal
	for index := range values {
		for _, contribution := range contributions {
			values[index] += contribution.value * contribution.multiplier[index] * scale
		}
	}
	return values, true
}

type profileGraphContribution struct {
	value      float64
	multiplier []float64
}

func profileGraphContributions(dimension ProfileDimensionSummary, metricID string, itemMap map[string]ProfileItem, schedules profileScheduleIndex, length int) ([]profileGraphContribution, float64) {
	if metricID == "" {
		return nil, 0
	}
	contributions := make([]profileGraphContribution, 0, len(dimension.ItemIDs))
	designTotal := 0.0
	for _, itemID := range dimension.ItemIDs {
		item := itemMap[itemID]
		if item.ID == "" {
			continue
		}
		metric := selectProfileMetric(item.Normalized, metricID)
		if metric.Status == metricStatusMissing || math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) {
			continue
		}
		multiplier := annualMultiplierProfile(schedules.forItem(item))
		if len(multiplier) != length {
			continue
		}
		contributions = append(contributions, profileGraphContribution{value: metric.Value, multiplier: multiplier})
		designTotal += metric.Value
	}
	return contributions, designTotal
}

func profileGraphNumeratorMetric(metricID string) (string, bool) {
	switch metricID {
	case "people_per_area":
		return "count", false
	case "area_per_person":
		return "count", true
	case "power_per_area", "power_per_person":
		return "total_power", false
	case "flow_per_area", "flow_per_exterior_area", "flow_per_exterior_wall_area", "flow_per_person", "ach":
		return "flow", false
	default:
		return metricID, false
	}
}

func divideProfile(values []float64, denominator float64) []float64 {
	if denominator == 0 {
		return make([]float64, len(values))
	}
	out := make([]float64, len(values))
	for index, value := range values {
		out[index] = value / denominator
	}
	return out
}

func representativeDaysFromAnnual(values []float64) []float64 {
	if len(values) < 24 {
		return values
	}
	weekday := append([]float64(nil), values[:24]...)
	saturdayStart := 5 * 24
	sundayStart := 6 * 24
	if len(values) < sundayStart+24 {
		return weekday
	}
	return append(append(weekday, values[saturdayStart:saturdayStart+24]...), values[sundayStart:sundayStart+24]...)
}

func representativeWeekFromAnnual(values []float64) []float64 {
	if len(values) < 168 {
		return values
	}
	return append([]float64(nil), values[:168]...)
}

func profileRuleMultiplierFromAnnual(values []float64) []float64 {
	if len(values) == 0 {
		return nil
	}
	out := []float64{values[0]}
	for _, value := range values[1:] {
		if math.Abs(value-out[len(out)-1]) > 1e-12 {
			out = append(out, value)
		}
	}
	return out
}

func buildProfileScheduleClusters(report ProfileReport, series []ProfileGraphSeries) []ProfileScheduleCluster {
	type clusterState struct {
		cluster    ProfileScheduleCluster
		names      map[string]bool
		zones      map[string]bool
		dimensions map[string]bool
		seriesIDs  map[string]bool
	}
	nameHashes := map[string]map[string]bool{}
	for _, schedule := range report.Schedules {
		name := strings.TrimSpace(schedule.ScheduleName)
		if name == "" {
			continue
		}
		key := normalizeName(name)
		if nameHashes[key] == nil {
			nameHashes[key] = map[string]bool{}
		}
		nameHashes[key][schedule.ContentHash] = true
	}
	states := map[string]*clusterState{}
	for _, schedule := range report.Schedules {
		if schedule.ContentHash == "" {
			continue
		}
		state := states[schedule.ContentHash]
		if state == nil {
			state = &clusterState{
				cluster: ProfileScheduleCluster{
					ID:            "schedule-cluster-" + safeID(schedule.ContentHash),
					ScheduleHash:  schedule.ContentHash,
					Pattern:       schedule.DetectedPattern,
					FeatureVector: profileScheduleFeatureVector(schedule),
					CentroidX:     schedule.AnnualStats.Average,
					CentroidY:     schedule.AnnualStats.OperatingHours,
				},
				names:      map[string]bool{},
				zones:      map[string]bool{},
				dimensions: map[string]bool{},
				seriesIDs:  map[string]bool{},
			}
			states[schedule.ContentHash] = state
		}
		state.names[schedule.ScheduleName] = true
		if len(nameHashes[normalizeName(schedule.ScheduleName)]) > 1 {
			state.cluster.SameNameDifferentContent = true
		}
	}
	for _, item := range series {
		if item.ScheduleHash == "" {
			continue
		}
		for _, hash := range strings.Split(item.ScheduleHash, "+") {
			hash = strings.TrimSpace(hash)
			state := states[hash]
			if state == nil {
				state = &clusterState{
					cluster:    ProfileScheduleCluster{ID: "schedule-cluster-" + safeID(hash), ScheduleHash: hash},
					names:      map[string]bool{},
					zones:      map[string]bool{},
					dimensions: map[string]bool{},
					seriesIDs:  map[string]bool{},
				}
				states[hash] = state
			}
			if item.ScheduleName != "" {
				for _, name := range strings.Split(item.ScheduleName, "+") {
					name = strings.TrimSpace(name)
					if name != "" {
						state.names[name] = true
					}
				}
			}
			if item.ZoneName != "" {
				state.zones[item.ZoneName] = true
			}
			state.dimensions[item.Dimension] = true
			state.seriesIDs[item.ID] = true
		}
	}
	clusters := make([]ProfileScheduleCluster, 0, len(states))
	for _, state := range states {
		state.cluster.ScheduleNames = sortedStringSet(state.names)
		state.cluster.ZoneNames = sortedStringSet(state.zones)
		state.cluster.Dimensions = sortedStringSet(state.dimensions)
		state.cluster.SeriesIDs = sortedStringSet(state.seriesIDs)
		if state.cluster.Pattern == "" {
			state.cluster.Pattern = "unresolved"
		}
		if len(state.cluster.ScheduleNames) > 1 {
			state.cluster.SameContentDifferentNames = true
		}
		if len(state.cluster.ScheduleNames) > 0 {
			state.cluster.Label = fmt.Sprintf("%s (%d)", state.cluster.Pattern, len(state.cluster.ScheduleNames))
		} else {
			state.cluster.Label = state.cluster.Pattern
		}
		clusters = append(clusters, state.cluster)
	}
	sort.Slice(clusters, func(i, j int) bool {
		if len(clusters[i].ZoneNames) != len(clusters[j].ZoneNames) {
			return len(clusters[i].ZoneNames) > len(clusters[j].ZoneNames)
		}
		return clusters[i].ScheduleHash < clusters[j].ScheduleHash
	})
	return clusters
}

func buildProfileOutlierHints(report ProfileReport, series []ProfileGraphSeries, clusters []ProfileScheduleCluster) []ProfileOutlierHint {
	var hints []ProfileOutlierHint
	hints = append(hints, robustProfileValueOutliers(report)...)
	hints = append(hints, profileGroupConsistencyHints(report)...)
	hints = append(hints, profileScheduleConsistencyHints(series, clusters)...)
	hints = append(hints, profileOperationHints(series)...)
	for index := range hints {
		if hints[index].ID == "" {
			hints[index].ID = fmt.Sprintf("profile-outlier-%03d", index+1)
		}
	}
	sort.SliceStable(hints, func(i, j int) bool {
		if profileSeverityRank(hints[i].Severity) != profileSeverityRank(hints[j].Severity) {
			return profileSeverityRank(hints[i].Severity) > profileSeverityRank(hints[j].Severity)
		}
		return hints[i].ID < hints[j].ID
	})
	return hints
}

func robustProfileValueOutliers(report ProfileReport) []ProfileOutlierHint {
	type valueRow struct {
		zone      ZoneProfile
		dimension ProfileDimensionSummary
	}
	byDimension := map[string][]valueRow{}
	for _, zone := range report.ZoneProfiles {
		for _, dimension := range zone.Dimensions {
			if dimension.Status == metricStatusMissing {
				continue
			}
			byDimension[dimension.Dimension] = append(byDimension[dimension.Dimension], valueRow{zone: zone, dimension: dimension})
		}
	}
	var hints []ProfileOutlierHint
	for dimension, rows := range byDimension {
		if len(rows) < 3 {
			continue
		}
		values := make([]float64, 0, len(rows))
		for _, row := range rows {
			values = append(values, row.dimension.Value)
		}
		median := medianFloat64(values)
		mad := medianAbsoluteDeviation(values, median)
		threshold := math.Max(mad*3.5, math.Max(math.Abs(median)*0.35, 0.001))
		for _, row := range rows {
			delta := math.Abs(row.dimension.Value - median)
			if delta <= threshold {
				continue
			}
			score := delta
			if mad > 0 {
				score = delta / mad
			}
			hints = append(hints, ProfileOutlierHint{
				Severity:      "warning",
				RuleID:        "robust_value_outlier",
				Message:       fmt.Sprintf("%s %s differs from the model median %s.", row.zone.ZoneName, profileDimensionLabel(dimension), profileMetricDisplay(median, row.dimension.Unit, metricStatusOK, profileMetricPrecision(row.dimension.MetricID))),
				ZoneName:      row.zone.ZoneName,
				Dimension:     dimension,
				ScheduleName:  row.dimension.ScheduleName,
				ScheduleHash:  row.dimension.ScheduleHash,
				Value:         row.dimension.Value,
				Median:        roundedNumber(median, profileMetricPrecision(row.dimension.MetricID)),
				MAD:           roundedNumber(mad, profileMetricPrecision(row.dimension.MetricID)),
				Score:         roundedNumber(score, 2),
				SourceItemIDs: append([]string(nil), row.dimension.ItemIDs...),
			})
		}
	}
	return hints
}

func profileGroupConsistencyHints(report ProfileReport) []ProfileOutlierHint {
	var hints []ProfileOutlierHint
	zoneByName := map[string]ZoneProfile{}
	for _, zone := range report.ZoneProfiles {
		zoneByName[normalizeName(zone.ZoneName)] = zone
	}
	for _, group := range report.Groups {
		if len(group.ZoneNames) < 2 {
			continue
		}
		for _, option := range report.Dimensions {
			values := map[string]bool{}
			schedules := map[string]bool{}
			var sourceIDs []string
			for _, zoneName := range group.ZoneNames {
				for _, dimension := range zoneByName[normalizeName(zoneName)].Dimensions {
					if dimension.Dimension != option.ID {
						continue
					}
					values[fmt.Sprintf("%.6f", dimension.Value)] = true
					if dimension.ScheduleHash != "" {
						schedules[dimension.ScheduleHash] = true
					}
					sourceIDs = append(sourceIDs, dimension.ItemIDs...)
				}
			}
			if len(values) > 1 {
				hints = append(hints, ProfileOutlierHint{
					Severity:      "info",
					RuleID:        "same_group_different_value",
					Message:       fmt.Sprintf("%s has multiple %s values inside one profile group.", group.Name, profileDimensionLabel(option.ID)),
					GroupID:       group.ID,
					Dimension:     option.ID,
					SourceItemIDs: cleanProfileNames(sourceIDs),
				})
			}
			if len(schedules) > 1 {
				hints = append(hints, ProfileOutlierHint{
					Severity:      "info",
					RuleID:        "same_group_different_schedule",
					Message:       fmt.Sprintf("%s has multiple %s schedules inside one profile group.", group.Name, profileDimensionLabel(option.ID)),
					GroupID:       group.ID,
					Dimension:     option.ID,
					SourceItemIDs: cleanProfileNames(sourceIDs),
				})
			}
		}
	}
	return hints
}

func profileScheduleConsistencyHints(series []ProfileGraphSeries, clusters []ProfileScheduleCluster) []ProfileOutlierHint {
	var hints []ProfileOutlierHint
	for _, cluster := range clusters {
		if cluster.SameContentDifferentNames {
			hints = append(hints, ProfileOutlierHint{
				Severity:     "info",
				RuleID:       "different_name_same_schedule_hash",
				Message:      fmt.Sprintf("%d schedule names share the same resolved profile.", len(cluster.ScheduleNames)),
				ScheduleHash: cluster.ScheduleHash,
				SeriesIDs:    append([]string(nil), cluster.SeriesIDs...),
			})
		}
		if cluster.SameNameDifferentContent {
			hints = append(hints, ProfileOutlierHint{
				Severity:     "warning",
				RuleID:       "same_name_different_schedule_hash",
				Message:      "A schedule name resolves to multiple schedule profiles.",
				ScheduleHash: cluster.ScheduleHash,
				SeriesIDs:    append([]string(nil), cluster.SeriesIDs...),
			})
		}
	}
	type bucket struct {
		values []float64
		series []ProfileGraphSeries
	}
	buckets := map[string]*bucket{}
	for _, item := range series {
		if item.ScopeType != "zone" || item.ScheduleHash == "" {
			continue
		}
		key := item.Dimension + "|" + item.ScheduleHash
		if buckets[key] == nil {
			buckets[key] = &bucket{}
		}
		buckets[key].values = append(buckets[key].values, item.DesignValue)
		buckets[key].series = append(buckets[key].series, item)
	}
	for _, bucket := range buckets {
		if len(bucket.series) < 2 {
			continue
		}
		minValue, maxValue := minMaxFloat64(bucket.values)
		if maxValue-minValue <= math.Max(math.Abs(maxValue)*0.2, 0.001) {
			continue
		}
		ids := make([]string, 0, len(bucket.series))
		for _, item := range bucket.series {
			ids = append(ids, item.ID)
		}
		first := bucket.series[0]
		hints = append(hints, ProfileOutlierHint{
			Severity:     "info",
			RuleID:       "same_schedule_different_value",
			Message:      fmt.Sprintf("%s uses one schedule profile with different design values.", profileDimensionLabel(first.Dimension)),
			Dimension:    first.Dimension,
			ScheduleHash: first.ScheduleHash,
			Value:        roundedNumber(maxValue, profileMetricPrecision(first.MetricID)),
			Median:       roundedNumber((minValue+maxValue)/2, profileMetricPrecision(first.MetricID)),
			SeriesIDs:    ids,
		})
	}
	return hints
}

func profileOperationHints(series []ProfileGraphSeries) []ProfileOutlierHint {
	var hints []ProfileOutlierHint
	for _, item := range series {
		if item.ScopeType != "zone" {
			continue
		}
		if item.ScheduleName == "" && item.DesignValue > 0 {
			hints = append(hints, ProfileOutlierHint{
				Severity:      "info",
				RuleID:        "missing_schedule_reference",
				Message:       fmt.Sprintf("%s %s has no schedule reference and is treated as always on.", item.ZoneName, profileDimensionLabel(item.Dimension)),
				ZoneName:      item.ZoneName,
				Dimension:     item.Dimension,
				SourceItemIDs: append([]string(nil), item.SourceItemIDs...),
				SeriesIDs:     []string{item.ID},
			})
			continue
		}
		if item.OperatingHours < 8750 || item.DesignValue <= 0 {
			continue
		}
		switch item.Dimension {
		case ProfileDimensionLighting, ProfileDimensionEquipment, ProfileDimensionOccupancy, ProfileDimensionVentilation, ProfileDimensionOutdoorAir:
			hints = append(hints, ProfileOutlierHint{
				Severity:      "warning",
				RuleID:        "always_on_operation",
				Message:       fmt.Sprintf("%s %s is scheduled for nearly all annual hours.", item.ZoneName, profileDimensionLabel(item.Dimension)),
				ZoneName:      item.ZoneName,
				Dimension:     item.Dimension,
				ScheduleName:  item.ScheduleName,
				ScheduleHash:  item.ScheduleHash,
				Value:         item.OperatingHours,
				SourceItemIDs: append([]string(nil), item.SourceItemIDs...),
				SeriesIDs:     []string{item.ID},
			})
		}
	}
	return hints
}

func buildProfileParameterCandidates(report ProfileReport, series []ProfileGraphSeries, hints []ProfileOutlierHint) []ProfileParameterCandidate {
	type state struct {
		candidate ProfileParameterCandidate
		zones     map[string]bool
		sources   map[string]bool
		rules     map[string]bool
		values    []float64
	}
	states := map[string]*state{}
	for _, item := range series {
		if item.ScopeType != "zone" || item.Dimension == "" || item.DesignValue <= 0 {
			continue
		}
		key := item.Dimension + "|" + item.MetricID
		if states[key] == nil {
			states[key] = &state{
				candidate: ProfileParameterCandidate{
					ID:        "profile-param-" + safeID(item.Dimension+"-"+item.MetricID),
					Label:     fmt.Sprintf("%s %s", profileDimensionLabel(item.Dimension), item.MetricLabel),
					Dimension: item.Dimension,
					MetricID:  item.MetricID,
					Reason:    "Profile values vary across zones and can be promoted to a parameter.",
					Severity:  "info",
				},
				zones:   map[string]bool{},
				sources: map[string]bool{},
				rules:   map[string]bool{},
			}
		}
		state := states[key]
		state.zones[item.ZoneName] = true
		for _, source := range item.SourceItemIDs {
			state.sources[source] = true
		}
		state.values = append(state.values, item.DesignValue)
		state.candidate.ImpactScore += item.AnnualContribution
	}
	for _, hint := range hints {
		if hint.Dimension == "" {
			continue
		}
		for key, state := range states {
			if !strings.HasPrefix(key, hint.Dimension+"|") {
				continue
			}
			state.rules[hint.RuleID] = true
			for _, source := range hint.SourceItemIDs {
				state.sources[source] = true
			}
			if hint.Severity == "warning" {
				state.candidate.Severity = "warning"
				state.candidate.Reason = "QA rules found profile outliers that are good parameterization candidates."
			}
		}
	}
	candidates := make([]ProfileParameterCandidate, 0, len(states))
	for _, state := range states {
		if len(state.values) < 2 {
			continue
		}
		minValue, maxValue := minMaxFloat64(state.values)
		if maxValue-minValue <= math.Max(math.Abs(maxValue)*0.05, 0.001) && state.candidate.Severity != "warning" {
			continue
		}
		state.candidate.ZoneNames = sortedStringSet(state.zones)
		state.candidate.SourceItemIDs = sortedStringSet(state.sources)
		state.candidate.RuleIDs = sortedStringSet(state.rules)
		state.candidate.CurrentMin = roundedNumber(minValue, profileMetricPrecision(state.candidate.MetricID))
		state.candidate.CurrentMedian = roundedNumber(medianFloat64(state.values), profileMetricPrecision(state.candidate.MetricID))
		state.candidate.CurrentMax = roundedNumber(maxValue, profileMetricPrecision(state.candidate.MetricID))
		state.candidate.ImpactScore = roundedNumber(state.candidate.ImpactScore, profileMetricPrecision(state.candidate.MetricID))
		state.candidate.ApplyRequest = profileCandidateApplyRequest(report, state.candidate)
		candidates = append(candidates, state.candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Severity != candidates[j].Severity {
			return profileSeverityRank(candidates[i].Severity) > profileSeverityRank(candidates[j].Severity)
		}
		return candidates[i].ImpactScore > candidates[j].ImpactScore
	})
	return candidates
}

func profileCandidateApplyRequest(report ProfileReport, candidate ProfileParameterCandidate) *ProfileApplyRequest {
	if len(candidate.SourceItemIDs) == 0 || len(candidate.ZoneNames) == 0 {
		return nil
	}
	itemMap := profileReportItemMap(report)
	sourceIndexes := make([]int, 0, len(candidate.SourceItemIDs))
	for _, id := range candidate.SourceItemIDs {
		item := itemMap[id]
		if item.ID != "" && item.ObjectIndex >= 0 {
			sourceIndexes = append(sourceIndexes, item.ObjectIndex)
		}
	}
	if len(sourceIndexes) == 0 {
		return nil
	}
	return &ProfileApplyRequest{
		SourceObjectIndexes:   uniqueInts(sourceIndexes),
		TargetZoneNames:       append([]string(nil), candidate.ZoneNames...),
		Dimensions:            []string{candidate.Dimension},
		Mode:                  "clone",
		ReplaceExistingPolicy: "replace",
		NameSuffix:            " Parameter Candidate",
	}
}

func annualMultiplierProfile(schedule ScheduleSummary) []float64 {
	if schedule.ScheduleName == "" && schedule.ContentHash == "" {
		return annualProfileFromDayProfiles(filledProfile(1), filledProfile(1), filledProfile(1))
	}
	if len(schedule.Rules) > 0 {
		values := make([]float64, 0, 8760)
		for day := 1; day <= 365; day++ {
			var selected *ScheduleRule
			for index := range schedule.Rules {
				rule := &schedule.Rules[index]
				if day >= rule.StartDay && day <= rule.EndDay && dayMatchesSelector(day, rule.Selector) {
					selected = rule
					break
				}
			}
			values = append(values, scheduleRuleHourlyProfile(selected)...)
		}
		if len(values) == 8760 {
			return roundedProfile(values)
		}
	}
	return annualProfileFromDayProfiles(schedule.WeekdayProfile, schedule.SaturdayProfile, schedule.SundayProfile)
}

func annualProfileFromDayProfiles(weekday, saturday, sunday []float64) []float64 {
	weekday = normalizeDayProfile(weekday, 1)
	saturday = normalizeDayProfile(saturday, 1)
	sunday = normalizeDayProfile(sunday, 1)
	values := make([]float64, 0, 8760)
	for day := 1; day <= 365; day++ {
		switch (day - 1) % 7 {
		case 5:
			values = append(values, saturday...)
		case 6:
			values = append(values, sunday...)
		default:
			values = append(values, weekday...)
		}
	}
	return roundedProfile(values)
}

func scheduleRuleHourlyProfile(rule *ScheduleRule) []float64 {
	profile := make([]float64, 24)
	if rule == nil {
		return profile
	}
	for _, interval := range rule.Intervals {
		start := int(math.Floor(interval.StartHour))
		end := int(math.Ceil(interval.EndHour))
		if start < 0 {
			start = 0
		}
		if end > 24 {
			end = 24
		}
		for hour := start; hour < end; hour++ {
			profile[hour] = interval.Value
		}
	}
	return roundedProfile(profile)
}

func profileGraphDayMultiplier(schedule ScheduleSummary) []float64 {
	if len(schedule.WeekdayProfile) > 0 || len(schedule.SaturdayProfile) > 0 || len(schedule.SundayProfile) > 0 {
		return append(append(normalizeDayProfile(schedule.WeekdayProfile, 1), normalizeDayProfile(schedule.SaturdayProfile, 1)...), normalizeDayProfile(schedule.SundayProfile, 1)...)
	}
	return append(append(filledProfile(1), filledProfile(1)...), filledProfile(1)...)
}

func profileGraphWeekMultiplier(schedule ScheduleSummary) []float64 {
	if len(schedule.WeeklyProfile) > 0 {
		return schedule.WeeklyProfile
	}
	return weeklyProfileFromDayProfiles(normalizeDayProfile(schedule.WeekdayProfile, 1), normalizeDayProfile(schedule.SaturdayProfile, 1), normalizeDayProfile(schedule.SundayProfile, 1))
}

func profileRuleMultiplier(schedule ScheduleSummary) []float64 {
	if len(schedule.Rules) == 0 {
		return []float64{1}
	}
	var out []float64
	for _, rule := range schedule.Rules {
		for _, interval := range rule.Intervals {
			out = append(out, interval.Value)
		}
	}
	return out
}

func normalizeDayProfile(values []float64, fallback float64) []float64 {
	if len(values) == 24 {
		return roundedProfile(values)
	}
	out := make([]float64, 24)
	for i := range out {
		out[i] = fallback
	}
	return out
}

func monthlyAverages(values []float64) []float64 {
	monthDays := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	out := make([]float64, 0, 12)
	offset := 0
	for _, days := range monthDays {
		hours := days * 24
		chunk := values[offset:minInt(offset+hours, len(values))]
		out = append(out, averageFloat64(chunk))
		offset += hours
		if offset >= len(values) {
			for len(out) < 12 {
				out = append(out, 0)
			}
			break
		}
	}
	return out
}

func profileScheduleFeatureVector(schedule ScheduleSummary) []float64 {
	return []float64{
		roundedNumber(schedule.AnnualStats.Average, 4),
		roundedNumber(schedule.AnnualStats.Max, 4),
		roundedNumber(schedule.AnnualStats.P95, 4),
		roundedNumber(divide(schedule.AnnualStats.OperatingHours, 8760), 4),
		roundedNumber(divide(schedule.AnnualStats.EquivalentFullHours, 8760), 4),
	}
}

type profileScheduleIndex struct {
	byName map[string]ScheduleSummary
	byHash map[string]ScheduleSummary
}

func profileScheduleLookup(schedules []ScheduleSummary) profileScheduleIndex {
	index := profileScheduleIndex{byName: map[string]ScheduleSummary{}, byHash: map[string]ScheduleSummary{}}
	for _, schedule := range schedules {
		if schedule.ScheduleName != "" {
			index.byName[normalizeName(schedule.ScheduleName)] = schedule
		}
		if schedule.ContentHash != "" {
			index.byHash[schedule.ContentHash] = schedule
		}
	}
	return index
}

func (index profileScheduleIndex) forDimension(dimension ProfileDimensionSummary) ScheduleSummary {
	for _, name := range strings.Split(dimension.ScheduleName, "+") {
		if schedule, ok := index.byName[normalizeName(name)]; ok {
			return schedule
		}
	}
	for _, hash := range strings.Split(dimension.ScheduleHash, "+") {
		hash = strings.TrimSpace(hash)
		if schedule, ok := index.byHash[hash]; ok {
			return schedule
		}
	}
	return ScheduleSummary{}
}

func (index profileScheduleIndex) forItem(item ProfileItem) ScheduleSummary {
	if schedule, ok := index.byName[normalizeName(item.ScheduleName)]; ok {
		return schedule
	}
	if schedule, ok := index.byHash[strings.TrimSpace(item.ScheduleHash)]; ok {
		return schedule
	}
	return ScheduleSummary{}
}

func profileReportItemMap(report ProfileReport) map[string]ProfileItem {
	items := map[string]ProfileItem{}
	for _, zone := range report.ZoneProfiles {
		for _, item := range zone.Items {
			items[item.ID] = item
		}
	}
	return items
}

func profileGroupByZone(groups []ProfileGroup) map[string]ProfileGroup {
	out := map[string]ProfileGroup{}
	for _, group := range groups {
		for _, zoneName := range group.ZoneNames {
			out[normalizeName(zoneName)] = group
		}
	}
	return out
}

func profileSeverityRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "error":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func multiplyProfile(values []float64, factor float64) []float64 {
	out := make([]float64, len(values))
	for index, value := range values {
		out[index] = value * factor
	}
	return out
}

func profileOperatingHours(values []float64) float64 {
	var hours float64
	for _, value := range values {
		if value > 0 {
			hours++
		}
	}
	return roundedNumber(hours, 1)
}

func sumFloat64(values []float64) float64 {
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum
}

func averageFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return sumFloat64(values) / float64(len(values))
}

func maxFloat64(values []float64) float64 {
	maxValue := 0.0
	for _, value := range values {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

func minMaxFloat64(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	minValue := values[0]
	maxValue := values[0]
	for _, value := range values[1:] {
		if value < minValue {
			minValue = value
		}
		if value > maxValue {
			maxValue = value
		}
	}
	return minValue, maxValue
}

func medianFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func medianAbsoluteDeviation(values []float64, median float64) float64 {
	deviations := make([]float64, 0, len(values))
	for _, value := range values {
		deviations = append(deviations, math.Abs(value-median))
	}
	return medianFloat64(deviations)
}
