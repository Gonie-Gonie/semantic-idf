package simulation

import (
	"fmt"
	"sort"
	"strings"
)

// Native quantities remain untouched. Each observed source gets its own fixed
// 3dp monthly display budget; Annual sums these completed monthly quantities.
func energyPathVRFDisplayPlan(native energyPathVRFAllocationPlan) energyPathVRFAllocationPlan {
	out := native
	out.Zones = append([]energyPathVRFZoneAllocation(nil), native.Zones...)
	out.Sources = append([]energyPathVRFSourceAllocation(nil), native.Sources...)
	out.Services = append([]energyPathVRFServiceAllocation(nil), native.Services...)
	loadWeights := map[string]float64{}
	for _, zone := range native.Zones {
		if zone.Month > 0 && zone.LoadKnown {
			key := zone.SystemID + "|" + zone.ServiceKind + "|" + zone.ZoneName + "|" + zone.PeriodID
			loadWeights[key] = zone.LoadValue
		}
	}
	groups := map[string][]int{}
	for i, source := range out.Sources {
		if source.Month > 0 && source.Shared && source.AllocatedKnown {
			key := fmt.Sprintf("%s|%s|%d", source.SystemID, source.Source.ID, source.Month)
			groups[key] = append(groups[key], i)
		}
	}
	for _, indices := range groups {
		targets := make([]energyPathZoneAuxiliaryTarget, len(indices))
		for i, index := range indices {
			item := out.Sources[index]
			key := item.SystemID + "|" + item.ServiceKind + "|" + item.ZoneName + "|" + item.PeriodID
			// The independent display policy first rounds the source budget,
			// then multiplies by original load/sum(load). Reusing a precomputed
			// source quota as the weight would introduce a second normalization.
			targets[i] = energyPathZoneAuxiliaryTarget{NodeID: item.ZoneName, ZoneName: item.ZoneName, Value: loadWeights[key]}
		}
		shares := energyPathFanPoolShares(out.Sources[indices[0]].ObservedValue, targets)
		for i, index := range indices {
			out.Sources[index].AllocatedValue = 0
			if i < len(shares) {
				out.Sources[index].AllocatedValue = shares[i]
			}
		}
	}
	sourceAnnual := map[string]float64{}
	zoneShared := map[string]float64{}
	for _, source := range out.Sources {
		if source.Month == 0 || !source.Shared {
			continue
		}
		key := source.SystemID + "|" + vrfAllocationTargetKey(source.Target) + "|" + source.ZoneName
		sourceAnnual[key] = roundedEnergyNumber(sourceAnnual[key] + source.AllocatedValue)
		key = source.SystemID + "|" + source.ServiceKind + "|" + source.ZoneName + "|" + source.PeriodID
		zoneShared[key] = roundedEnergyNumber(zoneShared[key] + source.AllocatedValue)
	}
	for i := range out.Sources {
		source := &out.Sources[i]
		if source.Month == 0 && source.Shared {
			source.AllocatedValue = sourceAnnual[source.SystemID+"|"+vrfAllocationTargetKey(source.Target)+"|"+source.ZoneName]
		}
	}
	annualZones := map[string]energyPathVRFZoneAllocation{}
	for i := range out.Zones {
		zone := &out.Zones[i]
		if zone.Month == 0 {
			continue
		}
		key := zone.SystemID + "|" + zone.ServiceKind + "|" + zone.ZoneName
		zone.DirectValue = roundedEnergyNumber(zone.DirectValue)
		zone.AllocatedValue = zoneShared[key+"|"+zone.PeriodID]
		zone.TotalValue = roundedEnergyNumber(zone.DirectValue + zone.AllocatedValue)
		current := annualZones[key]
		current.DirectValue = roundedEnergyNumber(current.DirectValue + zone.DirectValue)
		current.AllocatedValue = roundedEnergyNumber(current.AllocatedValue + zone.AllocatedValue)
		current.TotalValue = roundedEnergyNumber(current.TotalValue + zone.TotalValue)
		annualZones[key] = current
	}
	for i := range out.Zones {
		zone := &out.Zones[i]
		if zone.Month == 0 {
			current := annualZones[zone.SystemID+"|"+zone.ServiceKind+"|"+zone.ZoneName]
			zone.DirectValue, zone.AllocatedValue, zone.TotalValue = current.DirectValue, current.AllocatedValue, current.TotalValue
		}
	}
	serviceValues := map[string]energyPathVRFServiceAllocation{}
	for _, zone := range out.Zones {
		if zone.Month == 0 {
			continue
		}
		key := zone.SystemID + "|" + zone.ServiceKind + "|" + zone.PeriodID
		value := serviceValues[key]
		value.DirectValue = roundedEnergyNumber(value.DirectValue + zone.DirectValue)
		value.AllocatedValue = roundedEnergyNumber(value.AllocatedValue + zone.AllocatedValue)
		serviceValues[key] = value
	}
	seenSourceBudgets := map[string]bool{}
	for _, source := range out.Sources {
		if source.Month == 0 || !source.Shared || !source.ObservedKnown {
			continue
		}
		key := source.SystemID + "|" + source.ServiceKind + "|" + source.PeriodID
		sourceKey := key + "|" + source.Source.ID
		if seenSourceBudgets[sourceKey] {
			continue
		}
		seenSourceBudgets[sourceKey] = true
		value := serviceValues[key]
		value.SharedValue = roundedEnergyNumber(value.SharedValue + roundedEnergyNumber(source.ObservedValue))
		serviceValues[key] = value
	}
	annualServices := map[string]energyPathVRFServiceAllocation{}
	for i := range out.Services {
		row := &out.Services[i]
		if row.Month == 0 {
			continue
		}
		key := row.SystemID + "|" + row.ServiceKind
		value := serviceValues[key+"|"+row.PeriodID]
		row.DirectValue, row.SharedValue, row.AllocatedValue = value.DirectValue, value.SharedValue, value.AllocatedValue
		row.UnassignedValue = roundedEnergyNumber(row.SharedValue - row.AllocatedValue)
		annual := annualServices[key]
		annual.DirectValue = roundedEnergyNumber(annual.DirectValue + row.DirectValue)
		annual.SharedValue = roundedEnergyNumber(annual.SharedValue + row.SharedValue)
		annual.AllocatedValue = roundedEnergyNumber(annual.AllocatedValue + row.AllocatedValue)
		annual.UnassignedValue = roundedEnergyNumber(annual.UnassignedValue + row.UnassignedValue)
		annualServices[key] = annual
	}
	for i := range out.Services {
		row := &out.Services[i]
		if row.Month == 0 {
			value := annualServices[row.SystemID+"|"+row.ServiceKind]
			row.DirectValue, row.SharedValue, row.AllocatedValue, row.UnassignedValue = value.DirectValue, value.SharedValue, value.AllocatedValue, value.UnassignedValue
		}
	}
	return out
}

func energyPathVRFZoneLegacyInputs(nodes []EnergyExplanationNode, edges []EnergyExplanationEdge, plan energyPathVRFAllocationPlan, scope EnergyExplanationScope) ([]EnergyExplanationNode, []EnergyExplanationEdge) {
	if scope.Kind != "zone" || len(plan.Zones) == 0 {
		return nodes, edges
	}
	owned := map[string]bool{}
	for _, row := range plan.Zones {
		if strings.EqualFold(row.ZoneName, scope.ZoneName) {
			owned[row.ServiceKind] = true
		}
	}
	if len(owned) == 0 {
		return nodes, edges
	}
	removed := map[string]bool{}
	outNodes := make([]EnergyExplanationNode, 0, len(nodes))
	for _, node := range nodes {
		service := energyCanonicalServiceKind(firstNonEmpty(node.ServiceKind, node.EndUse, energyExplanationKindSuffix(node.Kind)))
		if strings.TrimSpace(node.ZoneName) == "" && strings.TrimSpace(node.LoopName) == "" && (node.Level == "energy" || node.Level == "end_use") &&
			!legacyEnergyNodeIsCarrier(node) && !legacyEnergyNodeIsSupport(node) && owned[service] {
			// A generic sibling-carrier/service fallback is not a VRF observation.
			// Exact nonblank-Zone direct energy is deliberately retained.
			removed[node.ID] = true
			continue
		}
		outNodes = append(outNodes, node)
	}
	outEdges := make([]EnergyExplanationEdge, 0, len(edges))
	for _, edge := range edges {
		if !removed[edge.FromID] && !removed[edge.ToID] {
			outEdges = append(outEdges, edge)
		}
	}
	return outNodes, outEdges
}

func projectEnergyPathVRFZoneGraph(nodes []EnergyExplanationNode, links []EnergyPathLink, plan energyPathVRFAllocationPlan, scope EnergyExplanationScope, period string, enabled bool) ([]EnergyExplanationNode, []EnergyPathLink) {
	if scope.Kind != "zone" || len(plan.Zones) == 0 {
		return nodes, links
	}
	rows := map[string][]energyPathVRFZoneAllocation{}
	for _, row := range plan.Zones {
		if strings.EqualFold(row.ZoneName, scope.ZoneName) && strings.EqualFold(row.PeriodID, period) {
			rows[row.ServiceKind] = append(rows[row.ServiceKind], row)
		}
	}
	if len(rows) == 0 {
		return nodes, links
	}
	outNodes := append([]EnergyExplanationNode(nil), nodes...)
	outLinks := append([]EnergyPathLink(nil), links...)
	scopeToken := energyExplanationScopeToken(scope)
	for _, service := range []string{"cooling", "heating"} {
		selected := rows[service]
		if len(selected) == 0 {
			continue
		}
		value, complete := 0.0, enabled
		ids, weights, entities := []string{}, []string{}, []string{}
		for _, row := range selected {
			value += row.DirectValue
			if enabled {
				value += row.AllocatedValue
			}
			complete = complete && row.TotalKnown && row.LoadKnown
			weights = appendUniqueStrings(weights, row.WeightSourceIDs...)
		}
		for _, source := range plan.Sources {
			if source.ServiceKind != service || !strings.EqualFold(source.ZoneName, scope.ZoneName) || !strings.EqualFold(source.PeriodID, period) {
				continue
			}
			if !source.Shared && (source.ObservedKnown || source.ObservedValue > 0) || enabled && source.Shared && (source.AllocatedKnown || source.AllocatedValue > 0) {
				ids = appendUniqueStrings(ids, source.Source.ID)
				entities = appendUniqueStrings(entities, energyPathVRFSourceEntities(source)...)
			}
		}
		value = roundedEnergyNumber(value)
		endID, carrierID := "end_use."+service+"."+scopeToken, "carrier.electricity."+scopeToken
		loadID := "load." + service + "." + scopeToken
		basis, explanation := "direct_zone_energy", "Observed VRF terminal electricity only; shared outdoor consumption is not allocated under this policy."
		if enabled {
			basis = "service_path_allocation"
			explanation = "Observed local VRF terminal electricity plus independently allocated original outdoor components; already model-total."
		}
		if !complete {
			explanation += " Partial observed/allocated subtotal; complete service consumption is unavailable."
		}
		endIndex, carrierIndex, loadIndex := -1, -1, -1
		for i, node := range outNodes {
			switch node.ID {
			case endID:
				endIndex = i
			case carrierID:
				carrierIndex = i
			case loadID:
				loadIndex = i
			}
		}
		paths := []string{}
		if loadIndex >= 0 {
			paths = appendUniqueStrings(paths, outNodes[loadIndex].RelatedPathIDs...)
		}
		// Replace any existing service conversion, not the measured load. The
		// service is paired once after all exact direct and VRF shares are merged.
		kept := outLinks[:0]
		for _, link := range outLinks {
			if link.Relation == "load_to_end_use" && link.ToID == endID {
				continue
			}
			kept = append(kept, link)
		}
		outLinks = kept
		if value > 0 {
			addition := EnergyExplanationNode{ID: endID, Level: "end_use", Kind: "energy." + service, Label: canonicalEnergyPathEndUseLabel(service),
				Value: value, EffectiveValue: value, AllocatedValue: value, AllocationApplied: enabled, AllocationExplanation: explanation,
				DisplayValue: value, Unit: "kWh", ScaleDomain: "site", Period: period, ZoneName: scope.ZoneName, ServiceKind: service, EndUse: service,
				Basis: basis, AggregationBasis: scope.AggregationBasis, Multiplier: 1, SourceIDs: append([]string(nil), ids...), endUseCarriers: []string{"electricity"}}
			addition.RelatedPathIDs, addition.RelatedEntityIDs = append([]string(nil), paths...), append([]string(nil), entities...)
			if !complete {
				addition.Badges = []string{"partial"}
			}
			if endIndex < 0 {
				outNodes, endIndex = append(outNodes, addition), len(outNodes)
			} else {
				current := outNodes[endIndex]
				current.Value = roundedEnergyNumber(current.Value + value)
				current.EffectiveValue, current.AllocatedValue, current.DisplayValue = current.Value, current.Value, current.Value
				current.RawValue = 0 // The combined shared subtotal is not measured Zone raw energy.
				current.AllocationApplied = current.AllocationApplied || enabled
				current.AllocationExplanation, current.Basis = explanation, basis
				current.SourceIDs = appendUniqueStrings(append([]string(nil), current.SourceIDs...), ids...)
				current.RelatedPathIDs = appendUniqueStrings(append([]string(nil), current.RelatedPathIDs...), paths...)
				current.RelatedEntityIDs = appendUniqueStrings(append([]string(nil), current.RelatedEntityIDs...), entities...)
				current.endUseCarriers = appendUniqueStrings(append([]string(nil), current.endUseCarriers...), "electricity")
				current.Badges = appendUniqueStrings(append([]string(nil), current.Badges...), addition.Badges...)
				outNodes[endIndex] = current
			}
			carrier := EnergyExplanationNode{ID: carrierID, Level: "carrier", Kind: "energy.electricity", Label: energyCarrierLabel("electricity"),
				Value: value, EffectiveValue: value, AllocatedValue: value, AllocationApplied: enabled, AllocationExplanation: explanation, DisplayValue: value,
				Unit: "kWh", ScaleDomain: "site", Period: period, ZoneName: scope.ZoneName, Carrier: "electricity", EndUse: "total", Basis: basis,
				AggregationBasis: scope.AggregationBasis, Multiplier: 1, SourceIDs: append([]string(nil), ids...), Badges: []string{"partial"}}
			carrier.RelatedPathIDs, carrier.RelatedEntityIDs = append([]string(nil), paths...), append([]string(nil), entities...)
			if carrierIndex < 0 {
				outNodes = append(outNodes, carrier)
			} else {
				current := outNodes[carrierIndex]
				current.Value = roundedEnergyNumber(current.Value + value)
				current.EffectiveValue, current.AllocatedValue, current.DisplayValue = current.Value, current.Value, current.Value
				current.RawValue = 0
				current.AllocationApplied = current.AllocationApplied || enabled
				// Existing measured lighting/equipment (or another exact direct
				// branch) keeps the carrier's established evidence precedence.
				// The new VRF service node itself remains a combined allocation.
				if current.Basis != "direct_zone_energy" {
					current.Basis = basis
				}
				current.AllocationExplanation = explanation
				current.SourceIDs = appendUniqueStrings(append([]string(nil), current.SourceIDs...), ids...)
				current.RelatedPathIDs = appendUniqueStrings(append([]string(nil), current.RelatedPathIDs...), paths...)
				current.RelatedEntityIDs = appendUniqueStrings(append([]string(nil), current.RelatedEntityIDs...), entities...)
				current.Badges = appendUniqueStrings(append([]string(nil), current.Badges...), "partial")
				outNodes[carrierIndex] = current
			}
			consumption := EnergyPathLink{FromID: endID, ToID: carrierID, Relation: "direct_end_use_to_carrier", Basis: basis,
				FromValue: value, ToValue: value, FromUnit: "kWh", ToUnit: "kWh", Period: period, ZoneName: scope.ZoneName, ServiceKind: service,
				SourceIDs: append([]string(nil), ids...), RelatedPathIDs: append([]string(nil), paths...), Explanation: explanation}
			if complete && loadIndex >= 0 && energyExplanationEffectiveNodeValue(outNodes[loadIndex]) > 0 {
				consumption.Relation = "end_use_to_carrier"
			}
			consumption.ID = energyPathLinkID(consumption)
			merged := map[string]*EnergyPathLink{}
			for _, link := range outLinks {
				mergeEnergyPathLink(merged, link)
			}
			mergeEnergyPathLink(merged, consumption)
			outLinks = outLinks[:0]
			for _, link := range merged {
				outLinks = append(outLinks, *link)
			}
		}
		if complete && endIndex >= 0 && loadIndex >= 0 {
			load, endUse := outNodes[loadIndex], outNodes[endIndex]
			if energyExplanationEffectiveNodeValue(load) > 0 && endUse.Value > 0 {
				conversion := EnergyPathLink{FromID: loadID, ToID: endID, Relation: "load_to_end_use", Basis: basis,
					FromValue: energyExplanationEffectiveNodeValue(load), ToValue: endUse.Value, FromUnit: load.Unit, ToUnit: endUse.Unit,
					Period: period, ZoneName: scope.ZoneName, ServiceKind: service, Explanation: explanation,
					SourceIDs:      appendUniqueStrings(appendUniqueStrings(append([]string(nil), endUse.SourceIDs...), load.SourceIDs...), weights...),
					RelatedPathIDs: appendUniqueStrings(append([]string(nil), endUse.RelatedPathIDs...), load.RelatedPathIDs...)}
				conversion.ID = energyPathLinkID(conversion)
				setEnergyPathConversionRatioKind(&conversion, &load, &endUse)
				finalizeEnergyPathLinkRatio(&conversion)
				outLinks = append(outLinks, conversion)
			}
		}
	}
	sortEnergyExplanationNodes(outNodes)
	sort.Slice(outLinks, func(i, j int) bool { return outLinks[i].ID < outLinks[j].ID })
	return outNodes, outLinks
}

func appendEnergyPathVRFSources(sources []EnergyDataSource, plan energyPathVRFAllocationPlan, scope EnergyExplanationScope, enabled bool) []EnergyDataSource {
	if len(plan.Sources) == 0 {
		return sources
	}
	out := append([]EnergyDataSource(nil), sources...)
	index := map[string]int{}
	for i, source := range out {
		index[source.ID] = i
	}
	seen := map[string]bool{}
	for _, trace := range plan.Sources {
		if trace.Month != 0 || trace.Source.ID == "" || seen[trace.Source.ID] ||
			scope.Kind == "zone" && !strings.EqualFold(trace.ZoneName, scope.ZoneName) {
			continue
		}
		if scope.Kind == "zone" && trace.Shared && !enabled {
			continue
		}
		source := trace.Source
		source.ScopeDetails = nil
		source.RelatedEntityIDs = energyPathVRFSourceEntities(trace)
		source.AllocatedValue, source.AllocationFactor, source.AllocationApplied = 0, 0, false
		if scope.Kind == "zone" && trace.Shared {
			// A measured outdoor pool is not a measured individual Zone scalar.
			source.RawValue, source.EffectiveValue, source.EffectiveMultiplier = 0, 0, 0
			source.observedValuePresence &^= energySourceObservedRaw | energySourceObservedEffective
			source.inspectorValuePresence &^= energySourceObservedRaw | energySourceObservedEffective
			source.MultiplierApplication = ""
			source.AllocatedValue = trace.AllocatedValue
			source.AllocationApplied = trace.AllocatedKnown || trace.AllocatedValue > 0
			if trace.ObservedKnown && trace.ObservedValue > 0 {
				source.AllocationFactor = trace.AllocatedValue / trace.ObservedValue
			}
			source.AllocationExplanation = "Allocated from this exact original VRF outdoor component; individual-Zone raw/effective consumption is unavailable."
			if !trace.AllocatedKnown {
				source.AllocationExplanation += " Incomplete monthly allocation coverage."
			}
			source.AllocationFormula = "sum of completed monthly source-specific VRF service-load-share display allocations"
		} else if trace.ObservedKnown {
			source.RawValue, source.EffectiveValue = roundedEnergyNumber(trace.ObservedValue), roundedEnergyNumber(trace.ObservedValue)
			source.observedValuePresence |= energySourceObservedRaw | energySourceObservedEffective
			source.EffectiveMultiplier, source.MultiplierApplication = 1, energyMultiplierAlreadyModelTotal
			source.AllocatedValue = source.EffectiveValue
		} else {
			source.RawValue, source.EffectiveValue, source.EffectiveMultiplier = 0, 0, 0
			source.observedValuePresence &^= energySourceObservedRaw | energySourceObservedEffective
			source.inspectorValuePresence &^= energySourceObservedRaw | energySourceObservedEffective
			source.MultiplierApplication = ""
		}
		source.AggregationBasis = scope.AggregationBasis
		if i, exists := index[source.ID]; exists {
			out[i] = source
		} else {
			index[source.ID] = len(out)
			out = append(out, source)
		}
		seen[source.ID] = true
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func energyPathVRFSourceEntities(trace energyPathVRFSourceAllocation) []string {
	ids := appendUniqueStrings(append([]string(nil), trace.Source.RelatedEntityIDs...), trace.SystemID)
	// The context's native ComponentRef uses this original object-index ID.
	// Source.ObjectIndex is instead an Output:Variable navigation index and is
	// never a substitute for this validated physical ownership identity.
	if trace.Target.ObjectIndex >= 0 {
		ids = appendUniqueStrings(ids, fmt.Sprintf("component:%d", trace.Target.ObjectIndex))
	}
	sort.Strings(ids)
	return ids
}
