package simulation

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	energyLoadAuthorityZoneAirSystem = iota
	energyLoadAuthorityIdealZone
	energyLoadAuthorityDirectZoneEquipment
	energyLoadAuthoritySystem
	energyLoadAuthorityPlant
)

type energyLoadCandidate struct {
	series         energyExplanationSeries
	authority      int
	component      string
	predicted      bool
	idealZoneTotal bool
}

type energyLoadBreakdownSeries struct {
	Component              string
	Unit                   string
	SourceIDs              []string
	AnnualSourceIDs        []string
	MonthlySourceIDs       []string
	DailySourceIDs         []string
	HourlySourceIDs        []string
	SelectedRangeSourceIDs []string
	Total                  float64
	Monthly                map[int]float64
	Daily                  map[int]float64
	Hourly                 map[int]float64
	SelectedRange          float64
	HasSelectedRange       bool
}

// selectCanonicalEnergyExplanationLoads collapses competing delivered-load
// levels before the legacy graph builder can add them into the same service
// node. Selection is fixed for the analysis period: a lower-authority family
// may be used only when the higher family has no period-capable source, and it
// is never spliced into uncovered months of the selected family.
func selectCanonicalEnergyExplanationLoads(input []energyExplanationSeries) []energyExplanationSeries {
	out := make([]energyExplanationSeries, 0, len(input))
	byZoneService := map[string][]energyLoadCandidate{}
	detailByZoneService := map[string][]energyExplanationSeries{}
	zoneOrder := []string{}
	fallbackByService := map[string][]energyLoadCandidate{}
	fallbackOrder := []string{}

	for _, original := range input {
		item := canonicalEnergyExplanationSeries(original)
		service, thermalService := energyLoadCanonicalSelectionService(item.ServiceKind)
		if item.Stage != "load" {
			out = append(out, item)
			continue
		}
		if !thermalService {
			if detailService, ok := energyLoadHumidityDetailService(item.ServiceKind); ok && strings.TrimSpace(item.ZoneName) != "" {
				key := normalizeEnergySurfaceKey(item.ZoneName) + "|" + detailService
				detailByZoneService[key] = append(detailByZoneService[key], item)
			}
			// Humidity and unmet-demand series remain inspector provenance. They
			// are not additional primary thermal-load nodes.
			continue
		}
		candidate, ok := canonicalEnergyLoadCandidate(item)
		if !ok || candidate.predicted {
			// Predicted thermostat/system loads are controls context. Their raw
			// sources are retained separately, but they are never additive load
			// series, even when no delivered-load source exists.
			continue
		}
		if candidate.authority <= energyLoadAuthorityDirectZoneEquipment && strings.TrimSpace(item.ZoneName) != "" {
			key := normalizeEnergySurfaceKey(item.ZoneName) + "|" + service
			if _, exists := byZoneService[key]; !exists {
				zoneOrder = append(zoneOrder, key)
			}
			byZoneService[key] = append(byZoneService[key], candidate)
			continue
		}
		if _, exists := fallbackByService[service]; !exists {
			fallbackOrder = append(fallbackOrder, service)
		}
		fallbackByService[service] = append(fallbackByService[service], candidate)
	}

	servicesWithZoneLoad := map[string]bool{}
	for _, key := range zoneOrder {
		selected, ok := selectCanonicalEnergyLoadTarget(byZoneService[key])
		if !ok {
			continue
		}
		service, _ := energyLoadCanonicalSelectionService(selected.ServiceKind)
		for _, detail := range detailByZoneService[key] {
			selected = appendEnergyLoadDetailProvenance(selected, detail)
		}
		selected.DriverCategory = "load." + service
		servicesWithZoneLoad[service] = true
		out = append(out, selected)
	}
	for _, service := range fallbackOrder {
		if servicesWithZoneLoad[service] {
			continue
		}
		selected, ok := selectCanonicalEnergyLoadTarget(fallbackByService[service])
		if !ok {
			continue
		}
		// A system/plant fallback has no defensible zone owner. Collapse all
		// selected equipment at that authority level into one service load;
		// later zone aggregation must not count a plant level again.
		selected.ZoneName = ""
		selected.LoopName = ""
		selected.PathType = energyLoadPathTypeForAuthority(energyLoadAuthority(selected))
		selected = canonicalEnergyExplanationSeries(selected)
		selected.DriverCategory = "load." + service
		out = append(out, selected)
	}
	return out
}

func canonicalEnergyLoadCandidate(item energyExplanationSeries) (energyLoadCandidate, bool) {
	name := normalizeEnergyOutputName(firstNonEmpty(item.SourceName, item.sourceName, item.Label))
	component := energyExplanationLoadThermalComponent(firstNonEmpty(item.SourceName, item.sourceName), item.Kind)
	if strings.TrimSpace(item.ThermalComponent) != "" {
		component = strings.ToLower(strings.TrimSpace(item.ThermalComponent))
	}
	candidate := energyLoadCandidate{series: item, component: component}
	switch {
	case strings.Contains(name, "predicted sensible load"):
		candidate.predicted = true
		return candidate, true
	case strings.Contains(name, "zone air system"):
		candidate.authority = energyLoadAuthorityZoneAirSystem
	case strings.Contains(name, "zone ideal loads zone total"):
		candidate.authority = energyLoadAuthorityIdealZone
		candidate.idealZoneTotal = true
	case strings.Contains(name, "zone ideal loads zone sensible"), strings.Contains(name, "zone ideal loads zone latent"):
		candidate.authority = energyLoadAuthorityIdealZone
	case strings.Contains(name, "zone radiant hvac"), strings.Contains(name, "zone baseboard total"):
		candidate.authority = energyLoadAuthorityDirectZoneEquipment
	case strings.Contains(name, "zone ideal loads supply air"):
		// Supply-air load includes outdoor-air conditioning and is not the
		// authoritative heat delivered to the zone. It is a system fallback.
		candidate.authority = energyLoadAuthoritySystem
	case strings.HasPrefix(item.Kind, "load.zone_radiant_"), strings.HasPrefix(item.Kind, "load.zone_equipment_"):
		candidate.authority = energyLoadAuthorityDirectZoneEquipment
	case item.PathType == "system", strings.HasPrefix(item.Kind, "load.system_"):
		candidate.authority = energyLoadAuthoritySystem
	case item.PathType == "plant", strings.HasPrefix(item.Kind, "load.plant_"):
		candidate.authority = energyLoadAuthorityPlant
	case strings.TrimSpace(item.ZoneName) != "" && strings.HasPrefix(item.Kind, "load.zone_"):
		// Canonical/synthetic zone loads with no raw alias remain authoritative.
		// Explicit radiant and Ideal Loads aliases were classified above.
		candidate.authority = energyLoadAuthorityZoneAirSystem
	default:
		return energyLoadCandidate{}, false
	}
	return candidate, true
}

func selectCanonicalEnergyLoadTarget(candidates []energyLoadCandidate) (energyExplanationSeries, bool) {
	if len(candidates) == 0 {
		return energyExplanationSeries{}, false
	}
	byAuthority := map[int][]energyLoadCandidate{}
	allByAuthority := map[int][]energyLoadCandidate{}
	anyPeriodCapable := false
	for _, candidate := range candidates {
		allByAuthority[candidate.authority] = append(allByAuthority[candidate.authority], candidate)
		if !energyLoadSeriesHasMaterialValue(candidate.series) {
			continue
		}
		byAuthority[candidate.authority] = append(byAuthority[candidate.authority], candidate)
		if energyLoadSeriesHasPeriodValues(candidate.series) {
			anyPeriodCapable = true
		}
	}
	selectedAuthority := -1
	for authority := energyLoadAuthorityZoneAirSystem; authority <= energyLoadAuthorityPlant; authority++ {
		items := byAuthority[authority]
		if len(items) == 0 {
			continue
		}
		if anyPeriodCapable && !energyLoadCandidatesHavePeriodValues(items) {
			continue
		}
		selectedAuthority = authority
		break
	}
	if selectedAuthority < 0 {
		// Keep an all-zero selected family as a canonical allocation sentinel.
		// It does not create a load node, but it ensures a period containing raw
		// driver pressure and zero actual load records an explicit zero allocation
		// instead of falling back to the historical raw ribbon width.
		for authority := energyLoadAuthorityZoneAirSystem; authority <= energyLoadAuthorityPlant; authority++ {
			if len(allByAuthority[authority]) > 0 {
				selectedAuthority = authority
				break
			}
		}
		if selectedAuthority < 0 {
			return energyExplanationSeries{}, false
		}
		byAuthority[selectedAuthority] = allByAuthority[selectedAuthority]
	}
	items := byAuthority[selectedAuthority]
	if anyPeriodCapable {
		periodItems := items[:0]
		for _, item := range items {
			if energyLoadSeriesHasPeriodValues(item.series) {
				periodItems = append(periodItems, item)
			}
		}
		items = periodItems
	}
	selected, ok := selectCanonicalEnergyLoadAuthority(items)
	if !ok {
		return energyExplanationSeries{}, false
	}
	selected.PathType = energyLoadPathTypeForAuthority(selectedAuthority)
	if strings.TrimSpace(selected.ZoneName) != "" {
		service, _ := energyLoadCanonicalSelectionService(selected.ServiceKind)
		selected.Kind = "load.zone_" + service
		selected.CanonicalKind = selected.Kind
		selected.Label = "Zone " + service + " load"
		selected.PathType = "zone"
		selected.LoopName = ""
	}
	selected = canonicalEnergyExplanationSeries(selected)
	return selected, true
}

func selectCanonicalEnergyLoadAuthority(candidates []energyLoadCandidate) (energyExplanationSeries, bool) {
	if len(candidates) == 0 {
		return energyExplanationSeries{}, false
	}
	authority := candidates[0].authority
	if authority == energyLoadAuthorityIdealZone {
		totals := []energyLoadCandidate{}
		components := []energyLoadCandidate{}
		for _, candidate := range candidates {
			if candidate.idealZoneTotal {
				totals = append(totals, candidate)
			} else {
				components = append(components, candidate)
			}
		}
		if len(totals) > 0 {
			total, ok := preferredCanonicalEnergyLoadSource(totals)
			if !ok {
				return energyExplanationSeries{}, false
			}
			breakdown := selectCanonicalEnergyLoadComponents(components, false)
			for _, item := range breakdown {
				total = appendEnergyLoadBreakdownProvenance(total, item)
			}
			total.ThermalComponent = "combined"
			total.DriverFormula = "reported Ideal Loads zone total; sensible and latent sources are breakdown only"
			return total, true
		}
		selected := selectCanonicalEnergyLoadComponents(components, false)
		return combineCanonicalEnergyLoadSeries(selected)
	}

	// Zone Air System sources are additive only across sensible and latent.
	if authority == energyLoadAuthorityZoneAirSystem {
		selected := selectCanonicalEnergyLoadComponents(candidates, false)
		return combineCanonicalEnergyLoadSeries(selected)
	}
	// Direct/system/plant levels may contain multiple physical pieces of
	// equipment. Within each piece, a reported total is authoritative over its
	// sensible/latent breakdown; only independent pieces are additive.
	selected := selectCanonicalEnergyLoadPhysicalFamilies(candidates)
	return combineCanonicalEnergyLoadSeries(selected)
}

func selectCanonicalEnergyLoadPhysicalFamilies(candidates []energyLoadCandidate) []energyExplanationSeries {
	grouped := map[string][]energyLoadCandidate{}
	order := []string{}
	for _, candidate := range candidates {
		key := energyLoadPhysicalFamily(candidate.series)
		if _, exists := grouped[key]; !exists {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], candidate)
	}
	out := make([]energyExplanationSeries, 0, len(order))
	for _, key := range order {
		family := grouped[key]
		totals := []energyLoadCandidate{}
		components := []energyLoadCandidate{}
		for _, candidate := range family {
			if candidate.component == "combined" {
				totals = append(totals, candidate)
			} else {
				components = append(components, candidate)
			}
		}
		if len(totals) > 0 {
			total, ok := preferredCanonicalEnergyLoadSource(totals)
			if !ok {
				continue
			}
			total.ThermalComponent = "combined"
			out = append(out, total)
			continue
		}
		selectedComponents := selectCanonicalEnergyLoadComponents(components, false)
		if combined, ok := combineCanonicalEnergyLoadSeries(selectedComponents); ok {
			out = append(out, combined)
		}
	}
	return out
}

func selectCanonicalEnergyLoadComponents(candidates []energyLoadCandidate, preservePhysicalFamily bool) []energyExplanationSeries {
	grouped := map[string][]energyLoadCandidate{}
	order := []string{}
	for _, candidate := range candidates {
		key := candidate.component
		if preservePhysicalFamily {
			key += "|" + energyLoadPhysicalFamily(candidate.series)
		}
		if _, exists := grouped[key]; !exists {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], candidate)
	}
	out := make([]energyExplanationSeries, 0, len(order))
	for _, key := range order {
		if selected, ok := preferredCanonicalEnergyLoadSource(grouped[key]); ok {
			out = append(out, selected)
		}
	}
	return out
}

func preferredCanonicalEnergyLoadSource(candidates []energyLoadCandidate) (energyExplanationSeries, bool) {
	if len(candidates) == 0 {
		return energyExplanationSeries{}, false
	}
	items := candidates
	hasPeriod := energyLoadCandidatesHavePeriodValues(items)
	var selected energyExplanationSeries
	found := false
	for _, candidate := range items {
		item := canonicalEnergyExplanationSeries(candidate.series)
		if hasPeriod && !energyLoadSeriesHasPeriodValues(item) {
			continue
		}
		if !found || energyExplanationSeriesSourcePreferred(item, selected) {
			selected = item
			found = true
		}
	}
	if !found {
		return energyExplanationSeries{}, false
	}
	selected.ThermalComponent = candidates[0].component
	selected.DriverInputSourceIDs = nil
	selected.loadBreakdown = energyLoadBreakdownForSelectedSeries(selected, candidates[0].component)
	return selected, true
}

func combineCanonicalEnergyLoadSeries(items []energyExplanationSeries) (energyExplanationSeries, bool) {
	if len(items) == 0 {
		return energyExplanationSeries{}, false
	}
	result := canonicalEnergyExplanationSeries(items[0])
	components := map[string]bool{strings.ToLower(strings.TrimSpace(result.ThermalComponent)): true}
	for _, original := range items[1:] {
		item := canonicalEnergyExplanationSeries(original)
		result.Total = roundedEnergyNumber(result.Total + item.Total)
		result.RawTotal = roundedEnergyNumber(result.RawTotal + item.RawTotal)
		addEnergyLoadPeriodValues(&result.Monthly, item.Monthly)
		addEnergyLoadPeriodValues(&result.RawMonthly, item.RawMonthly)
		addEnergyLoadPeriodValues(&result.Daily, item.Daily)
		addEnergyLoadPeriodValues(&result.RawDaily, item.RawDaily)
		addEnergyLoadPeriodValues(&result.Hourly, item.Hourly)
		addEnergyLoadPeriodValues(&result.RawHourly, item.RawHourly)
		if item.HasSelectedRange {
			result.SelectedRange = roundedEnergyNumber(result.SelectedRange + item.SelectedRange)
			result.RawSelectedRange = roundedEnergyNumber(result.RawSelectedRange + item.RawSelectedRange)
			result.HasSelectedRange = true
		}
		result.SourceIDs = appendUniqueStrings(result.SourceIDs, item.SourceIDs...)
		result.AnnualSourceIDs = appendUniqueStrings(result.AnnualSourceIDs, item.AnnualSourceIDs...)
		result.MonthlySourceIDs = appendUniqueStrings(result.MonthlySourceIDs, item.MonthlySourceIDs...)
		result.DailySourceIDs = appendUniqueStrings(result.DailySourceIDs, item.DailySourceIDs...)
		result.HourlySourceIDs = appendUniqueStrings(result.HourlySourceIDs, item.HourlySourceIDs...)
		result.SelectedRangeSourceIDs = appendUniqueStrings(result.SelectedRangeSourceIDs, item.SelectedRangeSourceIDs...)
		result.loadBreakdown = mergeEnergyLoadBreakdownSeries(result.loadBreakdown, item.loadBreakdown)
		components[strings.ToLower(strings.TrimSpace(item.ThermalComponent))] = true
	}
	delete(components, "")
	if len(components) > 1 || components["combined"] {
		result.ThermalComponent = "combined"
	}
	result.SourceName = firstNonEmpty(result.SourceName, result.sourceName)
	result = canonicalEnergyExplanationSeries(result)
	return result, true
}

func appendEnergyLoadBreakdownProvenance(total energyExplanationSeries, component energyExplanationSeries) energyExplanationSeries {
	total.SourceIDs = appendUniqueStrings(total.SourceIDs, component.SourceIDs...)
	total.AnnualSourceIDs = appendUniqueStrings(total.AnnualSourceIDs, component.AnnualSourceIDs...)
	total.MonthlySourceIDs = appendUniqueStrings(total.MonthlySourceIDs, component.MonthlySourceIDs...)
	total.DailySourceIDs = appendUniqueStrings(total.DailySourceIDs, component.DailySourceIDs...)
	total.HourlySourceIDs = appendUniqueStrings(total.HourlySourceIDs, component.HourlySourceIDs...)
	total.SelectedRangeSourceIDs = appendUniqueStrings(total.SelectedRangeSourceIDs, component.SelectedRangeSourceIDs...)
	total.DriverInputSourceIDs = appendUniqueStrings(total.DriverInputSourceIDs, component.SourceIDs...)
	total.loadBreakdown = mergeEnergyLoadBreakdownSeries(total.loadBreakdown, component.loadBreakdown)
	return total
}

func appendEnergyLoadDetailProvenance(total energyExplanationSeries, detail energyExplanationSeries) energyExplanationSeries {
	total.SourceIDs = appendUniqueStrings(total.SourceIDs, detail.SourceIDs...)
	total.AnnualSourceIDs = appendUniqueStrings(total.AnnualSourceIDs, detail.AnnualSourceIDs...)
	total.MonthlySourceIDs = appendUniqueStrings(total.MonthlySourceIDs, detail.MonthlySourceIDs...)
	total.DailySourceIDs = appendUniqueStrings(total.DailySourceIDs, detail.DailySourceIDs...)
	total.HourlySourceIDs = appendUniqueStrings(total.HourlySourceIDs, detail.HourlySourceIDs...)
	total.SelectedRangeSourceIDs = appendUniqueStrings(total.SelectedRangeSourceIDs, detail.SelectedRangeSourceIDs...)
	total.DriverInputSourceIDs = appendUniqueStrings(total.DriverInputSourceIDs, detail.SourceIDs...)
	return total
}

func energyLoadBreakdownForSelectedSeries(item energyExplanationSeries, component string) []energyLoadBreakdownSeries {
	component = strings.ToLower(strings.TrimSpace(component))
	if component != "sensible" && component != "latent" {
		return nil
	}
	return []energyLoadBreakdownSeries{{
		Component:              component,
		Unit:                   item.Unit,
		SourceIDs:              appendUniqueStrings(nil, item.SourceIDs...),
		AnnualSourceIDs:        appendUniqueStrings(nil, item.AnnualSourceIDs...),
		MonthlySourceIDs:       appendUniqueStrings(nil, item.MonthlySourceIDs...),
		DailySourceIDs:         appendUniqueStrings(nil, item.DailySourceIDs...),
		HourlySourceIDs:        appendUniqueStrings(nil, item.HourlySourceIDs...),
		SelectedRangeSourceIDs: appendUniqueStrings(nil, item.SelectedRangeSourceIDs...),
		Total:                  item.Total,
		Monthly:                cloneEnergyExplanationPeriodValues(item.Monthly),
		Daily:                  cloneEnergyExplanationPeriodValues(item.Daily),
		Hourly:                 cloneEnergyExplanationPeriodValues(item.Hourly),
		SelectedRange:          item.SelectedRange,
		HasSelectedRange:       item.HasSelectedRange,
	}}
}

func mergeEnergyLoadBreakdownSeries(current []energyLoadBreakdownSeries, next []energyLoadBreakdownSeries) []energyLoadBreakdownSeries {
	for _, item := range next {
		index := -1
		for candidate := range current {
			if strings.EqualFold(current[candidate].Component, item.Component) {
				index = candidate
				break
			}
		}
		if index < 0 {
			current = append(current, item)
			continue
		}
		target := &current[index]
		target.Total = roundedEnergyNumber(target.Total + item.Total)
		addEnergyLoadPeriodValues(&target.Monthly, item.Monthly)
		addEnergyLoadPeriodValues(&target.Daily, item.Daily)
		addEnergyLoadPeriodValues(&target.Hourly, item.Hourly)
		if item.HasSelectedRange {
			target.SelectedRange = roundedEnergyNumber(target.SelectedRange + item.SelectedRange)
			target.HasSelectedRange = true
		}
		target.SourceIDs = appendUniqueStrings(target.SourceIDs, item.SourceIDs...)
		target.AnnualSourceIDs = appendUniqueStrings(target.AnnualSourceIDs, item.AnnualSourceIDs...)
		target.MonthlySourceIDs = appendUniqueStrings(target.MonthlySourceIDs, item.MonthlySourceIDs...)
		target.DailySourceIDs = appendUniqueStrings(target.DailySourceIDs, item.DailySourceIDs...)
		target.HourlySourceIDs = appendUniqueStrings(target.HourlySourceIDs, item.HourlySourceIDs...)
		target.SelectedRangeSourceIDs = appendUniqueStrings(target.SelectedRangeSourceIDs, item.SelectedRangeSourceIDs...)
	}
	return current
}

func energyExplanationLoadComponentsForPeriod(item energyExplanationSeries, period string, total float64) []EnergyExplanationLoadComponent {
	out := make([]EnergyExplanationLoadComponent, 0, len(item.loadBreakdown))
	for _, component := range item.loadBreakdown {
		value := component.Total
		sourceIDs := energyExplanationPeriodSourceIDs(component.AnnualSourceIDs, component.SourceIDs)
		switch {
		case strings.EqualFold(period, "selected_range"):
			if !component.HasSelectedRange {
				continue
			}
			value = component.SelectedRange
			sourceIDs = energyExplanationPeriodSourceIDs(component.SelectedRangeSourceIDs, component.SourceIDs)
		case strings.HasPrefix(strings.ToUpper(period), "M"):
			index, _ := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(period), "M"))
			value = component.Monthly[index]
			sourceIDs = energyExplanationPeriodSourceIDs(component.MonthlySourceIDs, component.SourceIDs)
		case strings.HasPrefix(strings.ToUpper(period), "D"):
			index, _ := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(period), "D"))
			value = component.Daily[index]
			sourceIDs = energyExplanationPeriodSourceIDs(component.DailySourceIDs, component.SourceIDs)
		case strings.HasPrefix(strings.ToUpper(period), "H"):
			index, _ := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(period), "H"))
			value = component.Hourly[index]
			sourceIDs = energyExplanationPeriodSourceIDs(component.HourlySourceIDs, component.SourceIDs)
		}
		value = roundedEnergyNumber(math.Abs(value))
		if value == 0 {
			continue
		}
		share := 0.0
		if math.Abs(total) > 1e-9 {
			share = value / math.Abs(total)
		}
		out = append(out, EnergyExplanationLoadComponent{
			Component: strings.ToLower(strings.TrimSpace(component.Component)),
			Value:     value,
			Share:     share,
			Unit:      firstNonEmpty(component.Unit, item.Unit),
			SourceIDs: appendUniqueStrings(nil, sourceIDs...),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		order := func(component string) int {
			switch component {
			case "sensible":
				return 0
			case "latent":
				return 1
			default:
				return 2
			}
		}
		if order(out[i].Component) != order(out[j].Component) {
			return order(out[i].Component) < order(out[j].Component)
		}
		return out[i].Component < out[j].Component
	})
	return out
}

func mergeEnergyExplanationLoadComponents(current []EnergyExplanationLoadComponent, next []EnergyExplanationLoadComponent) []EnergyExplanationLoadComponent {
	current = cloneEnergyExplanationLoadComponents(current)
	for _, component := range next {
		index := -1
		for candidate := range current {
			if strings.EqualFold(current[candidate].Component, component.Component) {
				index = candidate
				break
			}
		}
		if index < 0 {
			component.SourceIDs = appendUniqueStrings(nil, component.SourceIDs...)
			current = append(current, component)
			continue
		}
		current[index].Value = roundedEnergyNumber(current[index].Value + component.Value)
		current[index].SourceIDs = appendUniqueStrings(current[index].SourceIDs, component.SourceIDs...)
		current[index].Unit = firstNonEmpty(current[index].Unit, component.Unit)
	}
	return current
}

func cloneEnergyExplanationLoadComponents(input []EnergyExplanationLoadComponent) []EnergyExplanationLoadComponent {
	if len(input) == 0 {
		return nil
	}
	out := make([]EnergyExplanationLoadComponent, len(input))
	for index, component := range input {
		out[index] = component
		out[index].SourceIDs = appendUniqueStrings(nil, component.SourceIDs...)
	}
	return out
}

func finalizeEnergyExplanationLoadNode(node *EnergyExplanationNode) {
	if node == nil || !strings.EqualFold(node.Level, "load") {
		return
	}
	node.LoadBreakdown = cloneEnergyExplanationLoadComponents(node.LoadBreakdown)
	node.Badges = appendUniqueStrings(nil, node.Badges...)
	latent := 0.0
	for index := range node.LoadBreakdown {
		component := &node.LoadBreakdown[index]
		component.Value = roundedEnergyNumber(component.Value)
		component.Share = 0
		if math.Abs(node.Value) > 1e-9 {
			component.Share = math.Abs(component.Value) / math.Abs(node.Value)
		}
		if strings.EqualFold(component.Component, "latent") {
			latent += math.Abs(component.Value)
		}
	}
	node.LatentShare = 0
	if math.Abs(node.Value) > 1e-9 {
		node.LatentShare = latent / math.Abs(node.Value)
	}
	filteredBadges := node.Badges[:0]
	for _, badge := range node.Badges {
		if badge != "dehumidification_significant" && badge != "humidification_significant" {
			filteredBadges = append(filteredBadges, badge)
		}
	}
	node.Badges = filteredBadges
	if node.LatentShare+1e-9 >= 0.10 {
		badge := "dehumidification_significant"
		if energyCanonicalServiceKind(node.ServiceKind) == "heating" {
			badge = "humidification_significant"
		}
		node.Badges = appendUniqueStrings(node.Badges, badge)
	}
}

func addEnergyLoadPeriodValues(target *map[int]float64, values map[int]float64) {
	if len(values) == 0 {
		return
	}
	if *target == nil {
		*target = map[int]float64{}
	}
	for period, value := range values {
		(*target)[period] = roundedEnergyNumber((*target)[period] + value)
	}
}

func energyLoadSeriesHasPeriodValues(item energyExplanationSeries) bool {
	return len(item.Monthly) > 0 || len(item.Daily) > 0 || len(item.Hourly) > 0 || item.HasSelectedRange
}

func energyLoadSeriesHasMaterialValue(item energyExplanationSeries) bool {
	if math.Abs(item.Total) > 1e-9 || item.HasSelectedRange && math.Abs(item.SelectedRange) > 1e-9 {
		return true
	}
	for _, values := range []map[int]float64{item.Monthly, item.Daily, item.Hourly} {
		for _, value := range values {
			if math.Abs(value) > 1e-9 {
				return true
			}
		}
	}
	return false
}

func energyLoadCandidatesHavePeriodValues(items []energyLoadCandidate) bool {
	for _, item := range items {
		if energyLoadSeriesHasPeriodValues(item.series) {
			return true
		}
	}
	return false
}

func energyLoadPhysicalFamily(item energyExplanationSeries) string {
	key := normalizeEnergyOutputName(firstNonEmpty(item.SourceKey, item.sourceKeyValue))
	if key == "" {
		key = normalizeEnergyOutputName(firstNonEmpty(item.SourceName, item.sourceName, item.SourceClass, item.Kind))
		for _, token := range []string{" total ", " sensible ", " latent ", " energy", " rate"} {
			key = strings.ReplaceAll(" "+key+" ", token, " ")
			key = strings.TrimSpace(key)
		}
	}
	return key
}

func energyLoadPathTypeForAuthority(authority int) string {
	switch authority {
	case energyLoadAuthorityZoneAirSystem, energyLoadAuthorityIdealZone, energyLoadAuthorityDirectZoneEquipment:
		return "zone"
	case energyLoadAuthoritySystem:
		return "system"
	case energyLoadAuthorityPlant:
		return "plant"
	default:
		return ""
	}
}

func energyLoadAuthority(item energyExplanationSeries) int {
	candidate, ok := canonicalEnergyLoadCandidate(item)
	if !ok {
		return energyLoadAuthoritySystem
	}
	return candidate.authority
}

func energyExplanationLoadThermalComponent(name string, kind string) string {
	key := normalizeEnergyOutputName(firstNonEmpty(name, kind))
	switch {
	case strings.Contains(key, "latent"), strings.Contains(normalizeEnergyOutputName(kind), "latent"):
		return "latent"
	case strings.Contains(key, "sensible"):
		return "sensible"
	default:
		return "combined"
	}
}

// annotateEnergyExplanationLoadSources makes non-additive load levels and
// predicted control demand inspectable without allowing them into the graph.
// This is enabled only for the canonical Energy Path build so frozen v1 JSON
// remains byte-for-byte compatible.
func annotateEnergyExplanationLoadSources(candidates []energyExplanationSeries, selected []energyExplanationSeries, sources []EnergyDataSource) []EnergyDataSource {
	selectedMain := map[string]energyExplanationSeries{}
	selectedBreakdown := map[string]energyExplanationSeries{}
	for _, item := range selected {
		item = canonicalEnergyExplanationSeries(item)
		_, thermalService := energyLoadCanonicalSelectionService(item.ServiceKind)
		if item.Stage != "load" || !thermalService {
			continue
		}
		breakdown := map[string]bool{}
		for _, id := range item.DriverInputSourceIDs {
			breakdown[id] = true
			selectedBreakdown[id] = item
		}
		for _, id := range item.SourceIDs {
			if !breakdown[id] {
				selectedMain[id] = item
			}
		}
	}
	candidateBySource := map[string]energyExplanationSeries{}
	for _, item := range candidates {
		item = canonicalEnergyExplanationSeries(item)
		if item.Stage != "load" {
			continue
		}
		for _, id := range item.SourceIDs {
			if existing, exists := candidateBySource[id]; exists {
				_, existingPrimary := energyLoadCanonicalSelectionService(existing.ServiceKind)
				_, nextPrimary := energyLoadCanonicalSelectionService(item.ServiceKind)
				if existingPrimary && !nextPrimary {
					continue
				}
			}
			candidateBySource[id] = item
		}
	}
	for index := range sources {
		source := &sources[index]
		candidate, ok := candidateBySource[source.ID]
		if !ok {
			continue
		}
		service, thermalService := energyLoadCanonicalSelectionService(candidate.ServiceKind)
		humidityService, humidityDetail := energyLoadHumidityDetailService(candidate.ServiceKind)
		if !thermalService && !humidityDetail {
			continue
		}
		if humidityDetail {
			service = humidityService
		}
		component := energyExplanationLoadThermalComponent(firstNonEmpty(candidate.SourceName, candidate.sourceName, source.Name), candidate.Kind)
		source.DriverCategory = "load." + service
		source.HeatDirection = service
		source.DriverComponent = "load.delivered." + component
		source.InspectorSection = energyDriverInspectorSectionContext
		source.DriverRole = energyDriverSourceRoleContext
		source.Explanation = "Lower-authority delivered load retained as non-additive context."
		// Formula/InputSourceIDs are reserved for actual derived records. Raw
		// EnergyPlus reports carry selection semantics in Explanation only.
		source.Formula = ""
		source.InputSourceIDs = nil
		if humidityDetail {
			source.DriverComponent = "load." + strings.ToLower(strings.TrimSpace(candidate.ServiceKind))
			source.DriverRole = energyDriverSourceRoleContext
			source.InspectorSection = energyDriverInspectorSectionBreakdown
			source.Explanation = "Humidity delivery is retained as latent load breakdown and is not a separate primary load."
			continue
		}
		if strings.Contains(normalizeEnergyOutputName(firstNonEmpty(candidate.SourceName, candidate.sourceName, source.Name)), "predicted sensible load") {
			source.DriverComponent = "load.predicted.sensible"
			source.Explanation = "Predicted thermostat load is controls/unmet-load context and is not included in delivered load."
			continue
		}
		if _, ok := selectedBreakdown[source.ID]; ok {
			source.InspectorSection = energyDriverInspectorSectionBreakdown
			source.Explanation = "Raw sensible/latent breakdown of the authoritative reported total; not added again."
			continue
		}
		if _, ok := selectedMain[source.ID]; ok {
			source.DriverRole = energyDriverSourceRoleMainFlow
			source.InspectorSection = energyDriverInspectorSectionBreakdown
			source.Explanation = "Selected actual delivered-load contribution."
			continue
		}
	}
	return appendEnergyLoadPredictionComparisonSources(candidates, selectedMain, selectedBreakdown, candidateBySource, sources)
}

func appendEnergyLoadPredictionComparisonSources(candidates []energyExplanationSeries, selectedMain map[string]energyExplanationSeries, selectedBreakdown map[string]energyExplanationSeries, candidateBySource map[string]energyExplanationSeries, sources []EnergyDataSource) []EnergyDataSource {
	predictedByTarget := map[string][]energyExplanationSeries{}
	actualSensibleByTarget := map[string]energyExplanationSeries{}
	for sourceID := range selectedMain {
		candidate, ok := candidateBySource[sourceID]
		if !ok || energyExplanationLoadThermalComponent(firstNonEmpty(candidate.SourceName, candidate.sourceName), candidate.Kind) != "sensible" {
			continue
		}
		service, ok := energyLoadCanonicalSelectionService(candidate.ServiceKind)
		if !ok {
			continue
		}
		actualSensibleByTarget[normalizeEnergySurfaceKey(candidate.ZoneName)+"|"+service] = selectedLoadSourceSeries(candidate, sourceID)
	}
	// Ideal Loads Zone Total remains authoritative, but its reported sensible
	// component is still the correct comparison basis for thermostat demand.
	for sourceID := range selectedBreakdown {
		candidate, ok := candidateBySource[sourceID]
		if !ok || energyExplanationLoadThermalComponent(firstNonEmpty(candidate.SourceName, candidate.sourceName), candidate.Kind) != "sensible" {
			continue
		}
		service, ok := energyLoadCanonicalSelectionService(candidate.ServiceKind)
		if !ok {
			continue
		}
		key := normalizeEnergySurfaceKey(candidate.ZoneName) + "|" + service
		if _, exists := actualSensibleByTarget[key]; !exists {
			actualSensibleByTarget[key] = selectedLoadSourceSeries(candidate, sourceID)
		}
	}
	for _, original := range candidates {
		item := canonicalEnergyExplanationSeries(original)
		service, ok := energyLoadCanonicalSelectionService(item.ServiceKind)
		if !ok || !strings.Contains(normalizeEnergyOutputName(firstNonEmpty(item.SourceName, item.sourceName, item.Label)), "predicted sensible load") || !energyLoadSeriesHasMaterialValue(item) {
			continue
		}
		key := normalizeEnergySurfaceKey(item.ZoneName) + "|" + service
		predictedByTarget[key] = append(predictedByTarget[key], item)
	}
	keys := make([]string, 0, len(predictedByTarget))
	for key := range predictedByTarget {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		actual, ok := actualSensibleByTarget[key]
		if !ok {
			continue
		}
		predicted, ok := preferredEnergyLoadPrediction(predictedByTarget[key])
		if !ok {
			continue
		}
		service, _ := energyLoadCanonicalSelectionService(actual.ServiceKind)
		zoneName := firstNonEmpty(actual.ZoneName, predicted.ZoneName)
		delta := roundedEnergyNumber(math.Abs(predicted.Total) - math.Abs(actual.Total))
		sourceID := "derived-load-predicted-vs-delivered-" + service + "-" + metricID(zoneName)
		inputIDs := appendUniqueStrings(nil, actual.SourceIDs...)
		inputIDs = appendUniqueStrings(inputIDs, predicted.SourceIDs...)
		sources = append(sources, EnergyDataSource{
			ID:                    sourceID,
			SourceType:            "derived_formula",
			KeyValue:              zoneName,
			Name:                  "Predicted vs delivered sensible " + service,
			Units:                 "kWh",
			SourceUnit:            "kWh",
			NormalizedUnit:        "kWh",
			ReportingFrequency:    "Monthly",
			AggregationMethod:     "signed_formula",
			ZoneName:              zoneName,
			RawValue:              delta,
			EffectiveValue:        delta,
			EffectiveMultiplier:   1,
			MultiplierApplication: energyMultiplierAlreadyModelTotal,
			AllocationFactor:      1,
			AllocatedValue:        delta,
			DriverRole:            energyDriverSourceRoleContext,
			DriverCategory:        "load." + service,
			DriverComponent:       "load.predicted_vs_delivered.sensible",
			HeatDirection:         service,
			InspectorSection:      energyDriverInspectorSectionBalance,
			Explanation:           "Thermostat prediction is compared with actual sensible delivery for controls/unmet-load inspection only.",
			Formula:               "absolute selected predicted sensible load - absolute actual delivered sensible load",
			InputSourceIDs:        inputIDs,
		})
	}
	return sources
}

func selectedLoadSourceSeries(item energyExplanationSeries, sourceID string) energyExplanationSeries {
	item.SourceIDs = []string{sourceID}
	item.AnnualSourceIDs = []string{sourceID}
	item.MonthlySourceIDs = []string{sourceID}
	item.DailySourceIDs = []string{sourceID}
	item.HourlySourceIDs = []string{sourceID}
	item.SelectedRangeSourceIDs = []string{sourceID}
	return item
}

func preferredEnergyLoadPrediction(items []energyExplanationSeries) (energyExplanationSeries, bool) {
	var selected energyExplanationSeries
	found := false
	for _, item := range items {
		if !found || energyLoadPredictionPreferred(item, selected) {
			selected = item
			found = true
		}
	}
	return selected, found
}

func energyLoadPredictionPreferred(candidate energyExplanationSeries, current energyExplanationSeries) bool {
	candidateClass := energyExplanationPhysicalSourceClass(candidate)
	currentClass := energyExplanationPhysicalSourceClass(current)
	candidateRank := energyExplanationPhysicalSourceClassRank(candidateClass)
	currentRank := energyExplanationPhysicalSourceClassRank(currentClass)
	if candidateRank != currentRank {
		return candidateRank < currentRank
	}
	return energyExplanationSeriesSourcePreferred(candidate, current)
}

func energyLoadCanonicalSelectionService(serviceKind string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(serviceKind)) {
	case "cooling":
		return "cooling", true
	case "heating":
		return "heating", true
	default:
		return "", false
	}
}

func energyLoadHumidityDetailService(serviceKind string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(serviceKind)) {
	case "dehumidification":
		return "cooling", true
	case "humidification":
		return "heating", true
	default:
		return "", false
	}
}

func sortedEnergyLoadKeys(values map[string][]energyLoadCandidate) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
