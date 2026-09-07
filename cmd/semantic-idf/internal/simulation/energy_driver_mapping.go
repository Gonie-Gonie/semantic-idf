package simulation

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type energyAirCouplingMatch struct {
	PairID           string
	ZoneName         string
	Pairwise         bool
	RelatedEntityIDs []string
}

type energyAirCouplingIndex struct {
	byExactKey map[string][]energyAirCouplingMatch
	byZoneKey  map[string][]energyAirCouplingMatch
}

func buildEnergyAirCouplingIndex(topology idf.ThermalTopologyReport) energyAirCouplingIndex {
	index := energyAirCouplingIndex{
		byExactKey: map[string][]energyAirCouplingMatch{},
		byZoneKey:  map[string][]energyAirCouplingMatch{},
	}
	zoneByNodeID := map[string]string{}
	for _, node := range topology.Nodes {
		zone := firstNonEmpty(node.ZoneName, node.Label, node.ObjectName)
		if zone == "" {
			continue
		}
		zoneByNodeID[normalizeEnergySurfaceKey(node.ID)] = zone
		zoneByNodeID[normalizeEnergySurfaceKey(node.EntityID)] = zone
	}
	for _, coupling := range topology.AirCouplings {
		fromZone := zoneByNodeID[normalizeEnergySurfaceKey(coupling.FromNodeID)]
		toZone := zoneByNodeID[normalizeEnergySurfaceKey(coupling.ToNodeID)]
		related := appendUniqueStrings(nil, coupling.ID, coupling.EntityID, coupling.FromNodeID, coupling.ToNodeID, coupling.SurfaceID)
		// AFN linkage reports flow, not direct pair heat energy. Never infer a
		// pairwise heat contribution merely because a linkage can be navigated.
		explicitPairwise := !strings.HasPrefix(normalizeEnergyOutputName(coupling.ObjectType), "airflownetwork") &&
			(strings.EqualFold(coupling.Direction, "directed") || strings.EqualFold(coupling.Direction, "bidirectional"))
		exact := energyAirCouplingMatch{
			PairID:           energyAirCouplingPairID(coupling.FromNodeID, coupling.ToNodeID),
			ZoneName:         toZone,
			Pairwise:         explicitPairwise,
			RelatedEntityIDs: related,
		}
		for _, key := range []string{coupling.ID, coupling.EntityID, coupling.ObjectName} {
			key = normalizeEnergySurfaceKey(key)
			if key != "" {
				index.byExactKey[key] = append(index.byExactKey[key], exact)
			}
		}
		for _, zone := range []string{fromZone, toZone} {
			key := normalizeEnergySurfaceKey(zone)
			if key == "" {
				continue
			}
			zoneMatch := exact
			zoneMatch.ZoneName = zone
			zoneMatch.Pairwise = false
			zoneMatch.PairID = ""
			index.byZoneKey[key] = append(index.byZoneKey[key], zoneMatch)
		}
	}
	return index
}

func energyAirCouplingPairID(fromNodeID string, toNodeID string) string {
	left := normalizeEnergySurfaceKey(fromNodeID)
	right := normalizeEnergySurfaceKey(toNodeID)
	if left == "" || right == "" {
		return ""
	}
	if right < left {
		left, right = right, left
	}
	return "air-pair:" + left + "|" + right
}

func (index energyAirCouplingIndex) resolve(sourceKey string) energyAirCouplingMatch {
	key := normalizeEnergySurfaceKey(sourceKey)
	if key == "" {
		return energyAirCouplingMatch{}
	}
	if matches := index.byExactKey[key]; len(matches) == 1 {
		return matches[0]
	}
	matches := index.byZoneKey[key]
	result := energyAirCouplingMatch{ZoneName: strings.TrimSpace(sourceKey)}
	for _, match := range matches {
		result.RelatedEntityIDs = appendUniqueStrings(result.RelatedEntityIDs, match.RelatedEntityIDs...)
		if result.ZoneName == "" {
			result.ZoneName = match.ZoneName
		}
	}
	return result
}

func energyDriverComponentForSeries(item energyExplanationSeries) string {
	name := normalizeEnergyOutputName(firstNonEmpty(item.SourceName, item.sourceName, item.Kind))
	component := firstNonEmpty(strings.ToLower(strings.TrimSpace(item.ThermalComponent)), "combined")
	direction := energyDriverHeatDirection(item)
	suffix := component
	if direction == "gain" || direction == "loss" {
		suffix += "." + direction
	}
	switch {
	case strings.Contains(name, "zone air heat balance surface convection"):
		return "surface.reconciliation"
	case strings.Contains(name, "surface inside face convection") || item.parseCategoryAggregate:
		return "surface." + suffix
	case strings.Contains(name, "zone total internal convective") || strings.Contains(name, "zone air heat balance internal convective"):
		return "internal.reconciliation.convective"
	case strings.Contains(name, "zone total internal latent"):
		return "internal.reconciliation.latent"
	case strings.Contains(name, "zone combined outdoor air") || strings.Contains(name, "zone air heat balance outdoor air"):
		return "outdoor_air.reconciliation." + suffix
	case strings.Contains(name, "zone ideal loads outdoor air"):
		return "mechanical_ventilation.ideal_loads_context." + suffix
	case strings.Contains(name, "air system outdoor air"):
		return "mechanical_ventilation.system_context." + suffix
	case strings.Contains(name, "heat exchanger") || strings.Contains(name, "heat recovery"):
		return "mechanical_ventilation.heat_recovery_context." + suffix
	case strings.Contains(name, "infiltration"):
		return "infiltration." + suffix
	case strings.Contains(name, "ventilation"):
		return "mechanical_ventilation." + suffix
	case strings.Contains(name, "mixing") || strings.Contains(name, "interzone air"):
		return "interzone_air." + suffix
	case strings.Contains(name, "air energy storage"):
		return "zone_air_storage"
	case strings.Contains(name, "heat balance deviation"):
		return "heat_balance_deviation"
	case item.DriverCategory == energyDriverCategoryPeople:
		return "internal.people." + suffix
	case item.DriverCategory == energyDriverCategoryLighting:
		return "internal.lighting." + suffix
	case item.DriverCategory == energyDriverCategoryEquipment:
		return "internal.equipment." + suffix
	case item.DriverCategory == energyDriverCategoryInternalOther:
		return "internal.other." + suffix
	case item.DriverCategory == energyDriverCategoryStorageOther:
		return "storage_other." + suffix
	default:
		return suffix
	}
}

func energyDriverHeatDirection(item energyExplanationSeries) string {
	switch strings.ToLower(firstNonEmpty(item.HeatSign, item.Sign)) {
	case "positive":
		return "gain"
	case "negative":
		return "loss"
	}
	return "signed"
}

type energyDriverVector struct {
	total            float64
	monthly          map[int]float64
	daily            map[int]float64
	hourly           map[int]float64
	selectedRange    float64
	hasSelectedRange bool
}

func energyDriverSignedVector(item energyExplanationSeries) energyDriverVector {
	multiplier := energyExplanationHeatSeriesSignMultiplier(item)
	return energyDriverScaledVector(item, multiplier)
}

func energyDriverLoadVector(item energyExplanationSeries) energyDriverVector {
	multiplier := 1.0
	if energyCanonicalServiceKind(item.ServiceKind) == "heating" {
		multiplier = -1
	}
	return energyDriverScaledVector(item, multiplier)
}

func energyDriverScaledVector(item energyExplanationSeries, multiplier float64) energyDriverVector {
	vector := energyDriverVector{
		total:            item.Total * multiplier,
		monthly:          scaledEnergyExplanationPeriodValues(item.Monthly, multiplier),
		daily:            scaledEnergyExplanationPeriodValues(item.Daily, multiplier),
		hourly:           scaledEnergyExplanationPeriodValues(item.Hourly, multiplier),
		selectedRange:    item.SelectedRange * multiplier,
		hasSelectedRange: item.HasSelectedRange,
	}
	return vector
}

func (vector *energyDriverVector) add(next energyDriverVector) {
	vector.total += next.total
	addEnergyDriverPeriodMap(&vector.monthly, next.monthly, 1)
	addEnergyDriverPeriodMap(&vector.daily, next.daily, 1)
	addEnergyDriverPeriodMap(&vector.hourly, next.hourly, 1)
	if next.hasSelectedRange {
		vector.selectedRange += next.selectedRange
		vector.hasSelectedRange = true
	}
}

func (vector *energyDriverVector) subtract(next energyDriverVector) {
	vector.total -= next.total
	addEnergyDriverPeriodMap(&vector.monthly, next.monthly, -1)
	addEnergyDriverPeriodMap(&vector.daily, next.daily, -1)
	addEnergyDriverPeriodMap(&vector.hourly, next.hourly, -1)
	if next.hasSelectedRange {
		vector.selectedRange -= next.selectedRange
		vector.hasSelectedRange = true
	}
}

func addEnergyDriverPeriodMap(target *map[int]float64, values map[int]float64, multiplier float64) {
	if len(values) == 0 {
		return
	}
	if *target == nil {
		*target = map[int]float64{}
	}
	for period, value := range values {
		(*target)[period] += value * multiplier
	}
}

func energyDriverVectorHasValue(vector energyDriverVector) bool {
	if math.Abs(vector.total) > 1e-9 || vector.hasSelectedRange && math.Abs(vector.selectedRange) > 1e-9 {
		return true
	}
	for _, values := range []map[int]float64{vector.monthly, vector.daily, vector.hourly} {
		for _, value := range values {
			if math.Abs(value) > 1e-9 {
				return true
			}
		}
	}
	return false
}

func finalizeEnergyDriverMappings(series []energyExplanationSeries, sources []EnergyDataSource, context energyDriverBuildContext) ([]energyExplanationSeries, []EnergyDataSource, []EnergyWarning) {
	out := make([]energyExplanationSeries, 0, len(series)+8)
	for _, original := range series {
		item := canonicalEnergyExplanationSeries(original)
		if item.Stage == "driver" && item.DriverComponent == "" {
			item.DriverComponent = energyDriverComponentForSeries(item)
		}
		out = append(out, item)
	}
	warnings := []EnergyWarning{}
	out, sources = appendEnergyDriverInterzoneBuildingResiduals(out, sources)
	out, sources, warnings = appendEnergyDriverInterzoneZoneFallbacks(out, sources, warnings)
	out, sources, warnings = appendEnergyDriverVentilationFallbacks(out, sources, warnings)
	out, sources = appendEnergyDriverReconciliationComponents(out, sources)
	out, sources = appendEnergyDriverUnmappedBalanceComponents(out, sources)
	out, sources = retainEnergyDriverDerivedRawZoneEquivalent(out, sources, context)
	return out, sources, warnings
}

func appendEnergyDriverInterzoneBuildingResiduals(series []energyExplanationSeries, sources []EnergyDataSource) ([]energyExplanationSeries, []EnergyDataSource) {
	pairs := map[string]energyDriverAggregate{}
	loadTotal := 0.0
	for _, item := range series {
		if energyDriverIsActualCanonicalZoneLoad(item) {
			loadTotal += math.Abs(item.Total)
		}
		if item.Stage != "driver" || item.DriverSourceRole != energyDriverSourceRoleMainFlow || item.DriverCategory != energyDriverCategoryInterzoneAir || item.interzonePairID == "" {
			continue
		}
		pair := pairs[item.interzonePairID]
		pair.vector.add(energyDriverSignedVector(item))
		pair.sourceIDs = appendUniqueStrings(pair.sourceIDs, append(item.SourceIDs, item.DriverInputSourceIDs...)...)
		pair.relatedIDs = appendUniqueStrings(pair.relatedIDs, item.RelatedEntityIDs...)
		pairs[item.interzonePairID] = pair
	}
	for _, pairID := range sortedEnergyDriverAggregateKeys(pairs) {
		pair := pairs[pairID]
		if !energyDriverVectorHasValue(pair.vector) {
			continue
		}
		category := energyDriverCategoryInterzoneAir
		component := "interzone_air.building_residual"
		label := energyDriverCategoryLabel(category)
		explanation := "Only the signed residual of an explicitly sourced interzone pair is retained at Building scope."
		section := energyDriverInspectorSectionBreakdown
		if loadTotal <= 0 || math.Abs(pair.vector.total) < loadTotal*energyDriverInterzoneThreshold {
			category = energyDriverCategoryStorageOther
			component = "interzone_air.small_building_residual"
			label = energyDriverCategoryLabel(category)
			explanation = "The signed Building net of an explicitly sourced interzone pair is small and is retained in Other / storage."
			section = energyDriverInspectorSectionBalance
		}
		item, nextSources := appendDerivedEnergyDriverSeries(
			sources,
			"interzone-building-"+pairID,
			"",
			category,
			"heat.interzone_building_residual",
			component,
			label,
			explanation,
			"signed pairwise interzone receiving-zone contributions summed at Building scope",
			pair.sourceIDs,
			pair.relatedIDs,
			pair.vector,
			false,
			section,
		)
		item.driverBuildingOnly = true
		series = append(series, item)
		sources = nextSources
	}
	return series, sources
}

func appendEnergyDriverInterzoneZoneFallbacks(series []energyExplanationSeries, sources []EnergyDataSource, warnings []EnergyWarning) ([]energyExplanationSeries, []EnergyDataSource, []EnergyWarning) {
	mainByZone := map[string]bool{}
	aggregates := map[string]energyDriverVector{}
	inputIDs := map[string][]string{}
	relatedIDs := map[string][]string{}
	zoneNames := map[string]string{}
	for _, item := range series {
		if item.Stage != "driver" || item.DriverCategory != energyDriverCategoryInterzoneAir || strings.TrimSpace(item.ZoneName) == "" {
			continue
		}
		key := normalizeEnergySurfaceKey(item.ZoneName)
		zoneNames[key] = item.ZoneName
		if item.DriverSourceRole == energyDriverSourceRoleMainFlow {
			mainByZone[key] = true
			continue
		}
		if item.Kind != "heat.interzone_air" || item.DriverSourceRole != energyDriverSourceRoleReconciliation {
			continue
		}
		vector := aggregates[key]
		vector.add(energyDriverSignedVector(item))
		aggregates[key] = vector
		inputIDs[key] = appendUniqueStrings(inputIDs[key], append(item.SourceIDs, item.DriverInputSourceIDs...)...)
		relatedIDs[key] = appendUniqueStrings(relatedIDs[key], item.RelatedEntityIDs...)
	}
	keys := sortedEnergyDriverKeys(aggregates)
	for _, key := range keys {
		if mainByZone[key] || !energyDriverVectorHasValue(aggregates[key]) {
			continue
		}
		var item energyExplanationSeries
		item, sources = appendDerivedEnergyDriverSeries(sources, "interzone-zone-aggregate", zoneNames[key], energyDriverCategoryInterzoneAir, "heat.interzone_zone_aggregate", "interzone_air.zone_aggregate", "Zone interzone-air aggregate", "Zone Air Heat Balance Interzone Air Transfer is retained only for Zone scope because no pairwise heat source was available.", "reported zone interzone-air aggregate; excluded from Building main flow", inputIDs[key], relatedIDs[key], aggregates[key], true, energyDriverInspectorSectionBreakdown)
		series = append(series, item)
		warnings = appendEnergyDriverWarning(warnings, EnergyWarning{
			Severity: "info",
			Code:     "energy_driver_interzone_zone_aggregate",
			Message:  fmt.Sprintf("%s interzone-air heat is available only as a receiving-zone aggregate; it is excluded from the Building main flow.", zoneNames[key]),
		})
	}
	return series, sources, warnings
}

type energyDriverAggregate struct {
	vector     energyDriverVector
	sourceIDs  []string
	relatedIDs []string
	present    bool
}

func appendEnergyDriverVentilationFallbacks(series []energyExplanationSeries, sources []EnergyDataSource, warnings []EnergyWarning) ([]energyExplanationSeries, []EnergyDataSource, []EnergyWarning) {
	combinedAggregateComponents := map[string]bool{}
	for _, item := range series {
		if item.Stage == "driver" && item.DriverSourceRole == energyDriverSourceRoleReconciliation && item.Kind == "heat.combined_outdoor_air" {
			component := firstNonEmpty(strings.ToLower(strings.TrimSpace(item.ThermalComponent)), "combined")
			combinedAggregateComponents[normalizeEnergySurfaceKey(item.ZoneName)+"|"+component] = true
		}
	}
	aggregates := map[string]energyDriverAggregate{}
	infiltration := map[string]energyDriverAggregate{}
	ventilation := map[string]energyDriverAggregate{}
	zoneNames := map[string]string{}
	for _, item := range series {
		if item.Stage != "driver" || strings.TrimSpace(item.ZoneName) == "" {
			continue
		}
		zoneKey := normalizeEnergySurfaceKey(item.ZoneName)
		zoneNames[zoneKey] = item.ZoneName
		component := firstNonEmpty(strings.ToLower(strings.TrimSpace(item.ThermalComponent)), "combined")
		direction := energyDriverHeatDirection(item)
		key := energyDriverAggregateDimensionKey(zoneKey, component, direction)
		switch {
		case item.DriverSourceRole == energyDriverSourceRoleReconciliation && (item.Kind == "heat.combined_outdoor_air" || item.Kind == "heat.ventilation_outdoor_air"):
			if item.Kind == "heat.ventilation_outdoor_air" && combinedAggregateComponents[zoneKey+"|"+component] {
				continue
			}
			aggregate := aggregates[key]
			aggregate.present = true
			aggregate.vector.add(energyDriverSignedVector(item))
			aggregate.sourceIDs = appendUniqueStrings(aggregate.sourceIDs, append(item.SourceIDs, item.DriverInputSourceIDs...)...)
			aggregate.relatedIDs = appendUniqueStrings(aggregate.relatedIDs, item.RelatedEntityIDs...)
			aggregates[key] = aggregate
		case item.DriverSourceRole == energyDriverSourceRoleMainFlow && item.DriverCategory == energyDriverCategoryInfiltration:
			aggregate := infiltration[key]
			aggregate.present = true
			aggregate.vector.add(energyDriverSignedVector(item))
			aggregate.sourceIDs = appendUniqueStrings(aggregate.sourceIDs, append(item.SourceIDs, item.DriverInputSourceIDs...)...)
			aggregate.relatedIDs = appendUniqueStrings(aggregate.relatedIDs, item.RelatedEntityIDs...)
			infiltration[key] = aggregate
		case item.DriverSourceRole == energyDriverSourceRoleMainFlow && item.DriverCategory == energyDriverCategoryMechanicalVentilation:
			aggregate := ventilation[key]
			aggregate.present = true
			aggregate.vector.add(energyDriverSignedVector(item))
			aggregate.sourceIDs = appendUniqueStrings(aggregate.sourceIDs, append(item.SourceIDs, item.DriverInputSourceIDs...)...)
			aggregate.relatedIDs = appendUniqueStrings(aggregate.relatedIDs, item.RelatedEntityIDs...)
			ventilation[key] = aggregate
		}
	}
	for _, key := range sortedEnergyDriverAggregateKeys(aggregates) {
		aggregate := aggregates[key]
		parts := strings.SplitN(key, "|", 3)
		zoneKey, component, direction := parts[0], parts[1], parts[2]
		infil := energyDriverAggregateForDimension(infiltration, zoneKey, component, direction)
		vent := energyDriverAggregateForDimension(ventilation, zoneKey, component, direction)
		if vent.present {
			continue
		}
		componentDimension := energyDriverComponentDimension(component, direction)
		if !infil.present {
			var item energyExplanationSeries
			item, sources = appendDerivedEnergyDriverSeries(sources, "outdoor-air-unsplit-"+componentDimension, zoneNames[zoneKey], energyDriverCategoryStorageOther, "heat.outdoor_air_unsplit", "outdoor_air.unsplit."+componentDimension, "Unsplit outdoor-air aggregate", "Combined outdoor-air heat could not be separated into infiltration and mechanical ventilation.", "signed outdoor-air aggregate retained in Other / storage because component sources are unavailable", aggregate.sourceIDs, aggregate.relatedIDs, aggregate.vector, false, energyDriverInspectorSectionBalance)
			item, sources = applyEnergyDriverDerivedDirection(item, sources, direction)
			series = append(series, item)
			warnings = appendEnergyDriverWarning(warnings, EnergyWarning{
				Severity: "warning",
				Code:     "energy_driver_outdoor_air_unsplit",
				Message:  fmt.Sprintf("%s outdoor-air aggregate could not be split because no compatible infiltration source was available; it remains in Other / storage.", zoneNames[zoneKey]),
			})
			continue
		}
		residual := aggregate.vector
		residual.subtract(infil.vector)
		if !energyDriverVectorHasValue(residual) {
			continue
		}
		inputs := appendUniqueStrings(aggregate.sourceIDs, infil.sourceIDs...)
		related := appendUniqueStrings(aggregate.relatedIDs, infil.relatedIDs...)
		var item energyExplanationSeries
		item, sources = appendDerivedEnergyDriverSeries(sources, "mechanical-ventilation-fallback-"+componentDimension, zoneNames[zoneKey], energyDriverCategoryMechanicalVentilation, "heat.ventilation_fallback", "mechanical_ventilation.fallback_residual."+componentDimension, "Mechanical ventilation", "Mechanical ventilation is derived without consuming the outdoor-air aggregate twice.", "signed outdoor-air aggregate - signed infiltration", inputs, related, residual, false, energyDriverInspectorSectionBreakdown)
		item, sources = applyEnergyDriverDerivedDirection(item, sources, direction)
		series = append(series, item)
		warnings = appendEnergyDriverWarning(warnings, EnergyWarning{
			Severity: "info",
			Code:     "energy_driver_ventilation_fallback",
			Message:  fmt.Sprintf("%s mechanical ventilation uses the outdoor-air-minus-infiltration fallback.", zoneNames[zoneKey]),
		})
	}
	return series, sources, warnings
}

func energyDriverAggregateDimensionKey(zoneKey string, component string, direction string) string {
	return zoneKey + "|" + component + "|" + direction
}

func energyDriverAggregateForDimension(values map[string]energyDriverAggregate, zoneKey string, component string, direction string) energyDriverAggregate {
	if component != "combined" && direction != "signed" {
		return values[energyDriverAggregateDimensionKey(zoneKey, component, direction)]
	}
	result := energyDriverAggregate{}
	for key, value := range values {
		parts := strings.SplitN(key, "|", 3)
		if len(parts) != 3 || parts[0] != zoneKey || component != "combined" && parts[1] != component || direction != "signed" && parts[2] != direction {
			continue
		}
		result.vector.add(value.vector)
		result.sourceIDs = appendUniqueStrings(result.sourceIDs, value.sourceIDs...)
		result.relatedIDs = appendUniqueStrings(result.relatedIDs, value.relatedIDs...)
		result.present = result.present || value.present
	}
	return result
}

func energyDriverComponentDimension(component string, direction string) string {
	if direction == "gain" || direction == "loss" {
		return component + "." + direction
	}
	return component
}

func applyEnergyDriverDerivedDirection(item energyExplanationSeries, sources []EnergyDataSource, direction string) (energyExplanationSeries, []EnergyDataSource) {
	switch direction {
	case "gain":
		item.HeatSign = "positive"
		item.Sign = "positive"
		item.heatSignMultiplier = 1
	case "loss":
		item.HeatSign = "negative"
		item.Sign = "negative"
		item.heatSignMultiplier = -1
		item.Total = math.Abs(item.Total)
		item.RawTotal = math.Abs(item.RawTotal)
		item.Monthly = absoluteEnergyDriverPeriodValues(item.Monthly)
		item.RawMonthly = absoluteEnergyDriverPeriodValues(item.RawMonthly)
		item.Daily = absoluteEnergyDriverPeriodValues(item.Daily)
		item.RawDaily = absoluteEnergyDriverPeriodValues(item.RawDaily)
		item.Hourly = absoluteEnergyDriverPeriodValues(item.Hourly)
		item.RawHourly = absoluteEnergyDriverPeriodValues(item.RawHourly)
		item.SelectedRange = math.Abs(item.SelectedRange)
		item.RawSelectedRange = math.Abs(item.RawSelectedRange)
	default:
		return item, sources
	}
	for index := range sources {
		matched := false
		for _, sourceID := range item.SourceIDs {
			if sourceID == sources[index].ID {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		sources[index].HeatDirection = direction
		if direction == "loss" {
			sources[index].RawValue = math.Abs(sources[index].RawValue)
			sources[index].EffectiveValue = math.Abs(sources[index].EffectiveValue)
			sources[index].AllocatedValue = math.Abs(sources[index].AllocatedValue)
		}
	}
	return item, sources
}

func absoluteEnergyDriverPeriodValues(values map[int]float64) map[int]float64 {
	if len(values) == 0 {
		return values
	}
	out := make(map[int]float64, len(values))
	for key, value := range values {
		out[key] = math.Abs(value)
	}
	return out
}

func appendEnergyDriverReconciliationComponents(series []energyExplanationSeries, sources []EnergyDataSource) ([]energyExplanationSeries, []EnergyDataSource) {
	type family struct {
		detail    energyDriverAggregate
		aggregate energyDriverAggregate
		zoneName  string
		component string
	}
	families := map[string]*family{}
	combinedOutdoorAirComponents := map[string]bool{}
	for _, item := range series {
		if item.Stage == "driver" && item.DriverSourceRole == energyDriverSourceRoleReconciliation && item.Kind == "heat.combined_outdoor_air" {
			component := firstNonEmpty(strings.ToLower(strings.TrimSpace(item.ThermalComponent)), "combined")
			combinedOutdoorAirComponents[normalizeEnergySurfaceKey(item.ZoneName)+"|"+component] = true
		}
	}
	for _, item := range series {
		if item.Stage != "driver" || strings.TrimSpace(item.ZoneName) == "" {
			continue
		}
		zoneKey := normalizeEnergySurfaceKey(item.ZoneName)
		component := firstNonEmpty(strings.ToLower(strings.TrimSpace(item.ThermalComponent)), "combined")
		familyName := ""
		isAggregate := false
		switch {
		case item.Kind == "heat.surface_convection" && item.DriverSourceRole == energyDriverSourceRoleReconciliation:
			familyName, isAggregate = "surface", true
		case item.Kind == "heat.surface_inside_face_convection" && item.DriverSourceRole == energyDriverSourceRoleMainFlow:
			familyName = "surface"
		case item.Kind == "heat.internal_convective" && item.DriverSourceRole == energyDriverSourceRoleReconciliation:
			familyName, isAggregate = "internal", true
		case item.DriverSourceRole == energyDriverSourceRoleMainFlow && strings.HasPrefix(item.DriverCategory, "internal.") && !strings.Contains(item.DriverComponent, "reconciliation_gap"):
			familyName = "internal"
		case item.DriverSourceRole == energyDriverSourceRoleReconciliation && item.Kind == "heat.combined_outdoor_air":
			familyName, isAggregate = "outdoor_air", true
		case item.DriverSourceRole == energyDriverSourceRoleReconciliation && item.Kind == "heat.ventilation_outdoor_air":
			if combinedOutdoorAirComponents[zoneKey+"|"+component] {
				continue
			}
			familyName, isAggregate = "outdoor_air", true
		case item.DriverSourceRole == energyDriverSourceRoleMainFlow && (item.DriverCategory == energyDriverCategoryInfiltration || item.DriverCategory == energyDriverCategoryMechanicalVentilation || strings.HasPrefix(item.DriverComponent, "outdoor_air.unsplit.")):
			familyName = "outdoor_air"
		default:
			continue
		}
		key := familyName + "|" + zoneKey + "|" + component
		group := families[key]
		if group == nil {
			group = &family{zoneName: item.ZoneName, component: component}
			families[key] = group
		}
		target := &group.detail
		if isAggregate {
			target = &group.aggregate
		}
		target.vector.add(energyDriverSignedVector(item))
		target.sourceIDs = appendUniqueStrings(target.sourceIDs, append(item.SourceIDs, item.DriverInputSourceIDs...)...)
		target.relatedIDs = appendUniqueStrings(target.relatedIDs, item.RelatedEntityIDs...)
	}
	familyKeys := make([]string, 0, len(families))
	for key := range families {
		familyKeys = append(familyKeys, key)
	}
	sort.Strings(familyKeys)
	for _, key := range familyKeys {
		group := families[key]
		if !energyDriverVectorHasValue(group.aggregate.vector) || !energyDriverVectorHasValue(group.detail.vector) {
			continue
		}
		difference := group.aggregate.vector
		difference.subtract(group.detail.vector)
		if !energyDriverVectorHasValue(difference) {
			continue
		}
		familyName := strings.SplitN(key, "|", 2)[0]
		category := energyDriverCategoryStorageOther
		kind := "heat.surface_reconciliation"
		componentName := "surface.reconciliation_difference"
		label := "Surface reconciliation difference"
		explanation := "Surface category values remain source-level; this separate balance term records the aggregate mismatch."
		formula := "signed zone surface-convection aggregate - signed selected surface-source sum"
		section := energyDriverInspectorSectionBalance
		switch familyName {
		case "internal":
			category = energyDriverCategoryInternalOther
			kind = "heat.internal_other_reconciliation"
			componentName = "internal.other.reconciliation_gap." + group.component
			label = "Other internal heat"
			explanation = "Unmapped internal convective or latent gain is retained separately from explicit People, Lighting, and Equipment families."
			formula = "signed total internal aggregate - signed mapped internal source families"
			section = energyDriverInspectorSectionBreakdown
		case "outdoor_air":
			kind = "heat.outdoor_air_reconciliation"
			componentName = "outdoor_air.reconciliation_difference." + group.component
			label = "Outdoor-air reconciliation difference"
			explanation = "The outdoor-air aggregate mismatch is retained as a named Other / storage balance component without consuming the aggregate twice."
			formula = "signed outdoor-air aggregate - signed infiltration and mechanical-ventilation drivers"
		}
		inputs := appendUniqueStrings(group.aggregate.sourceIDs, group.detail.sourceIDs...)
		related := appendUniqueStrings(group.aggregate.relatedIDs, group.detail.relatedIDs...)
		var item energyExplanationSeries
		item, sources = appendDerivedEnergyDriverSeries(sources, familyName+"-reconciliation-"+group.component, group.zoneName, category, kind, componentName, label, explanation, formula, inputs, related, difference, false, section)
		series = append(series, item)
	}
	return series, sources
}

func appendEnergyDriverUnmappedBalanceComponents(series []energyExplanationSeries, sources []EnergyDataSource) ([]energyExplanationSeries, []EnergyDataSource) {
	loads := map[string]energyDriverAggregate{}
	drivers := map[string]energyDriverAggregate{}
	zoneNames := map[string]string{}
	hasUnscopedMainDriver := false
	for _, item := range series {
		if strings.TrimSpace(item.ZoneName) == "" {
			if item.Stage == "driver" && item.DriverSourceRole == energyDriverSourceRoleMainFlow && !item.driverBuildingOnly && energyDriverVectorHasValue(energyDriverSignedVector(item)) {
				hasUnscopedMainDriver = true
			}
			continue
		}
		key := normalizeEnergySurfaceKey(item.ZoneName)
		zoneNames[key] = item.ZoneName
		switch {
		case energyDriverIsActualCanonicalZoneLoad(item):
			group := loads[key]
			group.vector.add(energyDriverLoadVector(item))
			group.sourceIDs = appendUniqueStrings(group.sourceIDs, item.SourceIDs...)
			group.relatedIDs = appendUniqueStrings(group.relatedIDs, item.RelatedEntityIDs...)
			loads[key] = group
		case item.Stage == "driver" && item.DriverSourceRole == energyDriverSourceRoleMainFlow:
			group := drivers[key]
			group.vector.add(energyDriverSignedVector(item))
			group.sourceIDs = appendUniqueStrings(group.sourceIDs, append(item.SourceIDs, item.DriverInputSourceIDs...)...)
			group.relatedIDs = appendUniqueStrings(group.relatedIDs, item.RelatedEntityIDs...)
			drivers[key] = group
		}
	}
	// A source-level driver without zone provenance cannot be allocated safely to
	// a zone before EPATH-070. Retain that named source and avoid manufacturing an
	// additional zone balance term that would double count it.
	if hasUnscopedMainDriver {
		return series, sources
	}
	for _, key := range sortedEnergyDriverAggregateKeys(loads) {
		remaining := loads[key].vector
		remaining.subtract(drivers[key].vector)
		if !energyDriverVectorHasValue(remaining) {
			continue
		}
		inputs := appendUniqueStrings(loads[key].sourceIDs, drivers[key].sourceIDs...)
		related := appendUniqueStrings(loads[key].relatedIDs, drivers[key].relatedIDs...)
		var item energyExplanationSeries
		item, sources = appendDerivedEnergyDriverSeries(sources, "unmapped-zone-balance", zoneNames[key], energyDriverCategoryStorageOther, "heat.unmapped_zone_balance", "unmapped_zone_heat_balance", "Other / storage", "The remaining signed zone heat-balance term is preserved as a named component, not treated as an analyzer error.", "signed zone cooling load - signed zone heating load - signed mapped driver contributions", inputs, related, remaining, false, energyDriverInspectorSectionBalance)
		series = append(series, item)
	}
	return series, sources
}

func energyDriverIsActualCanonicalZoneLoad(item energyExplanationSeries) bool {
	if item.Stage != "load" || strings.TrimSpace(item.ZoneName) == "" {
		return false
	}
	name := normalizeEnergyOutputName(firstNonEmpty(item.SourceName, item.sourceName, item.Label))
	if strings.Contains(name, "predicted") {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(item.Kind)) {
	case "load.zone_cooling", "load.zone_heating":
		return true
	default:
		return false
	}
}

func appendDerivedEnergyDriverSeries(sources []EnergyDataSource, idSuffix string, zoneName string, category string, kind string, component string, label string, explanation string, formula string, inputSourceIDs []string, relatedEntityIDs []string, vector energyDriverVector, zoneOnly bool, inspectorSection string) (energyExplanationSeries, []EnergyDataSource) {
	sourceID := "derived-driver-" + metricID(idSuffix) + "-" + metricID(zoneName)
	item := energyExplanationSeries{
		Stage:                  "driver",
		CanonicalKind:          kind,
		Level:                  "heat",
		Kind:                   kind,
		Label:                  label,
		Unit:                   "kWh",
		ZoneName:               zoneName,
		ThermalComponent:       energyDriverComponentThermalKind(component),
		DriverCategory:         category,
		DriverSourceRole:       energyDriverSourceRoleMainFlow,
		DriverExplanation:      explanation,
		DriverComponent:        component,
		DriverFormula:          formula,
		DriverInputSourceIDs:   appendUniqueStrings(nil, inputSourceIDs...),
		Sign:                   "signed",
		Basis:                  "derived_balance",
		SourceIDs:              []string{sourceID},
		AnnualSourceIDs:        []string{sourceID},
		MonthlySourceIDs:       []string{sourceID},
		DailySourceIDs:         []string{sourceID},
		HourlySourceIDs:        []string{sourceID},
		SelectedRangeSourceIDs: []string{sourceID},
		RelatedEntityIDs:       appendUniqueStrings(nil, relatedEntityIDs...),
		RawTotal:               roundedEnergyNumber(vector.total),
		RawMonthly:             roundedEnergyExplanationMonthly(vector.monthly),
		RawDaily:               roundedEnergyExplanationDaily(vector.daily),
		RawHourly:              roundedEnergyExplanationHourly(vector.hourly),
		RawSelectedRange:       roundedEnergyNumber(vector.selectedRange),
		Total:                  roundedEnergyNumber(vector.total),
		Monthly:                roundedEnergyExplanationMonthly(vector.monthly),
		Daily:                  roundedEnergyExplanationDaily(vector.daily),
		Hourly:                 roundedEnergyExplanationHourly(vector.hourly),
		SelectedRange:          roundedEnergyNumber(vector.selectedRange),
		HasSelectedRange:       vector.hasSelectedRange,
		EffectiveMultiplier:    1,
		MultiplierApplication:  energyMultiplierAlreadyModelTotal,
		multiplierApplied:      true,
		sourceKeyValue:         zoneName,
		sourceName:             label,
		sourceFrequency:        "Monthly",
		heatSignMultiplier:     1,
		driverZoneOnly:         zoneOnly,
	}
	item = canonicalEnergyExplanationSeries(item)
	source := EnergyDataSource{
		ID:                    sourceID,
		SourceType:            "derived_formula",
		KeyValue:              zoneName,
		Name:                  label,
		Units:                 "kWh",
		SourceUnit:            "kWh",
		NormalizedUnit:        "kWh",
		ReportingFrequency:    "Monthly",
		AggregationMethod:     "signed_formula",
		ZoneName:              zoneName,
		RawValue:              roundedEnergyNumber(vector.total),
		EffectiveValue:        roundedEnergyNumber(vector.total),
		EffectiveMultiplier:   1,
		MultiplierApplication: energyMultiplierAlreadyModelTotal,
		DriverRole:            energyDriverSourceRoleMainFlow,
		DriverCategory:        category,
		DriverComponent:       component,
		HeatDirection:         "signed",
		InspectorSection:      inspectorSection,
		Explanation:           explanation,
		Formula:               formula,
		InputSourceIDs:        appendUniqueStrings(nil, inputSourceIDs...),
		RelatedEntityIDs:      appendUniqueStrings(nil, relatedEntityIDs...),
	}
	return item, appendOrReplaceEnergyDataSource(sources, source)
}

func appendOrReplaceEnergyDataSource(sources []EnergyDataSource, source EnergyDataSource) []EnergyDataSource {
	for index := range sources {
		if sources[index].ID == source.ID {
			sources[index] = source
			return sources
		}
	}
	return append(sources, source)
}

func energyDriverComponentThermalKind(component string) string {
	if strings.Contains(component, "latent") {
		return "latent"
	}
	if strings.Contains(component, "sensible") || strings.Contains(component, "convective") || strings.HasPrefix(component, "surface.") {
		return "sensible"
	}
	return "combined"
}

type energyDriverPeriodValue struct {
	value     float64
	gross     float64
	sourceIDs []string
}

func (value *energyDriverPeriodValue) add(next float64, sourceIDs ...string) {
	value.value += next
	value.gross += math.Abs(next)
	value.sourceIDs = appendUniqueStrings(value.sourceIDs, sourceIDs...)
}

func appendEnergyDriverPeriodAccounting(period string, series []energyExplanationSeries, valueFor func(energyExplanationSeries) float64, reconciliation []EnergyReconciliation, warnings []EnergyWarning) ([]EnergyReconciliation, []EnergyWarning) {
	surfaceDetail := map[string]*energyDriverPeriodValue{}
	surfaceAggregate := map[string]*energyDriverPeriodValue{}
	internalDetail := map[string]*energyDriverPeriodValue{}
	internalAggregate := map[string]*energyDriverPeriodValue{}
	outdoorDetail := map[string]*energyDriverPeriodValue{}
	outdoorAggregate := map[string]*energyDriverPeriodValue{}
	interzonePairs := map[string]*energyDriverPeriodValue{}
	storage := map[string]*energyDriverPeriodValue{}
	loads := map[string]*energyDriverPeriodValue{}
	zoneNames := map[string]string{}
	combinedOutdoorAirComponents := map[string]bool{}
	for _, item := range series {
		if item.Stage == "driver" && item.DriverSourceRole == energyDriverSourceRoleReconciliation && item.Kind == "heat.combined_outdoor_air" {
			component := firstNonEmpty(strings.ToLower(strings.TrimSpace(item.ThermalComponent)), "combined")
			combinedOutdoorAirComponents[normalizeEnergySurfaceKey(item.ZoneName)+"|"+component] = true
		}
	}
	for _, original := range series {
		item := canonicalEnergyExplanationSeries(original)
		value := valueFor(item)
		if value == 0 {
			continue
		}
		zoneKey := normalizeEnergySurfaceKey(item.ZoneName)
		if zoneKey != "" {
			zoneNames[zoneKey] = item.ZoneName
		}
		sourceIDs := appendUniqueStrings(item.SourceIDs, item.DriverInputSourceIDs...)
		if energyDriverIsActualCanonicalZoneLoad(item) {
			periodValue(loads, zoneKey).add(math.Abs(value), sourceIDs...)
			continue
		}
		if item.Stage != "driver" || zoneKey == "" {
			continue
		}
		signed := value * energyExplanationHeatSeriesSignMultiplier(item)
		component := firstNonEmpty(strings.ToLower(strings.TrimSpace(item.ThermalComponent)), "combined")
		componentKey := zoneKey + "|" + component
		switch {
		case item.Kind == "heat.surface_inside_face_convection" && item.DriverSourceRole == energyDriverSourceRoleMainFlow:
			periodValue(surfaceDetail, zoneKey).add(signed, sourceIDs...)
		case item.Kind == "heat.surface_convection" && item.DriverSourceRole == energyDriverSourceRoleReconciliation:
			periodValue(surfaceAggregate, zoneKey).add(signed, sourceIDs...)
		case item.Kind == "heat.internal_convective" && item.DriverSourceRole == energyDriverSourceRoleReconciliation:
			periodValue(internalAggregate, componentKey).add(signed, sourceIDs...)
		case item.DriverSourceRole == energyDriverSourceRoleMainFlow && strings.HasPrefix(item.DriverCategory, "internal.") && !strings.Contains(item.DriverComponent, "reconciliation_gap"):
			periodValue(internalDetail, componentKey).add(signed, sourceIDs...)
		case item.DriverSourceRole == energyDriverSourceRoleReconciliation && item.Kind == "heat.combined_outdoor_air":
			periodValue(outdoorAggregate, componentKey).add(signed, sourceIDs...)
		case item.DriverSourceRole == energyDriverSourceRoleReconciliation && item.Kind == "heat.ventilation_outdoor_air":
			if !combinedOutdoorAirComponents[componentKey] {
				periodValue(outdoorAggregate, componentKey).add(signed, sourceIDs...)
			}
		case item.DriverSourceRole == energyDriverSourceRoleMainFlow && (item.DriverCategory == energyDriverCategoryInfiltration || item.DriverCategory == energyDriverCategoryMechanicalVentilation || strings.HasPrefix(item.DriverComponent, "outdoor_air.unsplit.")):
			periodValue(outdoorDetail, componentKey).add(signed, sourceIDs...)
		}
		if item.DriverSourceRole == energyDriverSourceRoleMainFlow && item.interzonePairID != "" {
			periodValue(interzonePairs, item.interzonePairID).add(signed, sourceIDs...)
		}
		if item.DriverSourceRole == energyDriverSourceRoleMainFlow && item.DriverCategory == energyDriverCategoryStorageOther {
			periodValue(storage, zoneKey).add(signed, sourceIDs...)
		}
	}
	for _, family := range []struct {
		name      string
		detail    map[string]*energyDriverPeriodValue
		aggregate map[string]*energyDriverPeriodValue
		formula   string
		code      string
	}{
		{name: "surface", detail: surfaceDetail, aggregate: surfaceAggregate, formula: "signed zone surface-convection aggregate - signed selected surface-source sum", code: "energy_driver_surface_reconciliation_gap"},
		{name: "internal", detail: internalDetail, aggregate: internalAggregate, formula: "signed total internal aggregate - signed mapped internal source families", code: "energy_driver_internal_reconciliation_gap"},
		{name: "outdoor_air", detail: outdoorDetail, aggregate: outdoorAggregate, formula: "signed outdoor-air aggregate - signed infiltration and mechanical-ventilation drivers", code: "energy_driver_outdoor_air_reconciliation_gap"},
	} {
		for _, key := range sortedEnergyDriverPeriodKeys(family.aggregate) {
			aggregate := family.aggregate[key]
			detail := family.detail[key]
			if aggregate == nil || detail == nil {
				continue
			}
			residual := roundedEnergyNumber(aggregate.value - detail.value)
			reference := math.Max(math.Abs(aggregate.value), detail.gross)
			identity := strings.SplitN(key, "|", 2)
			zoneKey := identity[0]
			component := ""
			if len(identity) == 2 {
				component = identity[1]
			}
			sourceIDs := appendUniqueStrings(aggregate.sourceIDs, detail.sourceIDs...)
			reconciliation = append(reconciliation, EnergyReconciliation{
				ID:                        "reconcile.driver." + family.name + "." + metricID(zoneNames[zoneKey]) + "." + period,
				Level:                     "driver",
				Period:                    period,
				Label:                     energyDriverReconciliationLabel(family.name) + " - " + zoneNames[zoneKey],
				Status:                    energyDriverReconciliationStatus(reference, residual),
				ZoneName:                  zoneNames[zoneKey],
				ExpectedValue:             roundedEnergyNumber(aggregate.value),
				ExplainedValue:            roundedEnergyNumber(detail.value),
				ResidualValue:             residual,
				Unit:                      "kWh",
				Basis:                     "residual",
				Formula:                   family.formula,
				SourceIDs:                 sourceIDs,
				driverAccountingComponent: component,
			})
			if math.Abs(residual) > energyDriverReconciliationTolerance(reference) {
				warnings = appendEnergyDriverWarning(warnings, EnergyWarning{
					Severity: "warning",
					Code:     family.code,
					Message:  fmt.Sprintf("%s %s mismatch is %g kWh for this period; source-level graph values are retained.", zoneNames[zoneKey], strings.ReplaceAll(family.name, "_", " "), residual),
					Period:   period,
				})
			}
		}
	}
	for _, pairID := range sortedEnergyDriverPeriodKeys(interzonePairs) {
		pair := interzonePairs[pairID]
		residual := roundedEnergyNumber(-pair.value)
		reconciliation = append(reconciliation, EnergyReconciliation{
			ID:             "reconcile.driver.interzone_pair." + metricID(pairID) + "." + period,
			Level:          "driver",
			Period:         period,
			Label:          "Interzone pairwise balance",
			Status:         energyDriverReconciliationStatus(pair.gross, residual),
			ExpectedValue:  0,
			ExplainedValue: roundedEnergyNumber(pair.value),
			ResidualValue:  residual,
			Unit:           "kWh",
			Basis:          "residual",
			Formula:        "signed receiving-zone pair contributions sum to zero at Building scope",
			SourceIDs:      pair.sourceIDs,
		})
	}
	for _, zoneKey := range sortedEnergyDriverPeriodKeys(storage) {
		load := loads[zoneKey]
		if load == nil || load.gross == 0 {
			continue
		}
		value := storage[zoneKey]
		if math.Abs(value.value) <= math.Max(0.001, load.gross*energyDriverBalanceWarningRatio) {
			continue
		}
		warnings = appendEnergyDriverWarning(warnings, EnergyWarning{
			Severity: "warning",
			Code:     "energy_driver_storage_other_large",
			Message:  fmt.Sprintf("%s Other / storage is %g kWh against %g kWh of zone load; inspect its named balance components and source gaps.", zoneNames[zoneKey], value.value, load.gross),
			Period:   period,
		})
	}
	return reconciliation, warnings
}

func periodValue(values map[string]*energyDriverPeriodValue, key string) *energyDriverPeriodValue {
	value := values[key]
	if value == nil {
		value = &energyDriverPeriodValue{}
		values[key] = value
	}
	return value
}

func energyDriverReconciliationTolerance(reference float64) float64 {
	return math.Max(0.001, math.Abs(reference)*0.01)
}

func energyDriverReconciliationStatus(reference float64, residual float64) string {
	if math.Abs(residual) <= energyDriverReconciliationTolerance(reference) {
		return "balanced"
	}
	if residual < 0 {
		return "overmapped"
	}
	return "residual"
}

func energyDriverReconciliationLabel(family string) string {
	switch family {
	case "surface":
		return "Surface reconciliation"
	case "internal":
		return "Internal gain reconciliation"
	case "outdoor_air":
		return "Outdoor-air reconciliation"
	default:
		return "Driver reconciliation"
	}
}

func sortedEnergyDriverKeys(values map[string]energyDriverVector) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedEnergyDriverAggregateKeys(values map[string]energyDriverAggregate) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedEnergyDriverPeriodKeys(values map[string]*energyDriverPeriodValue) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
