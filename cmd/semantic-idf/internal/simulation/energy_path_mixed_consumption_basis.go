package simulation

import (
	"sort"
	"strings"
)

const energyRelationshipRuleMixedHVACConsumptionBasis = "aggregation.observed_and_allocated_zone_energy"

// A native local heater and an independently allocated central service can
// share one bounded end-use node. Preserve each site branch's evidence, but do
// not label their combined node/conversion as entirely directly measured.
// This qualification is restricted to the native constituent contract; the
// legacy packaged-equipment projection remains a separate compatibility path.
func qualifyEnergyPathMixedConsumptionBasis(nodes []EnergyExplanationNode, links []EnergyPathLink, sources []EnergyDataSource, scope EnergyExplanationScope) []EnergyPathLink {
	if scope.Kind != "zone" || strings.TrimSpace(scope.ZoneName) == "" {
		return links
	}
	local := energyPathNativeConsumptionSources(sources, scope)
	if len(local) == 0 {
		return links
	}
	type evidence struct {
		direct    bool
		allocated string
	}
	byEndUse := map[string]evidence{}
	for _, link := range links {
		if (link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier") || !energyPathPositiveFinite(link.ToValue) || !energyPathPositiveFinite(link.FromValue) {
			continue
		}
		current := byEndUse[link.FromID]
		switch canonicalEnergyPathBasis(link.Basis, "") {
		case "direct_zone_energy":
			for _, id := range link.SourceIDs {
				if local[id] {
					current.direct = true
				}
			}
		case "zone_load_allocation":
			current.allocated = "zone_load_allocation"
		case "service_path_allocation":
			if current.allocated == "" {
				current.allocated = "service_path_allocation"
			}
		}
		byEndUse[link.FromID] = current
	}
	qualified := map[string]string{}
	const explanation = "Mixed Zone subtotal: directly observed local heater consumption plus separately allocated central HVAC energy. The combined total is not a fully measured Zone observation; carrier branches retain their own direct or allocated basis."
	for index := range nodes {
		node := &nodes[index]
		e := byEndUse[node.ID]
		if node.Level != "end_use" || node.EndUse != "heating" || !strings.EqualFold(node.ZoneName, scope.ZoneName) || !e.direct || e.allocated == "" {
			continue
		}
		node.Basis = e.allocated
		node.AllocationApplied = true
		node.AllocationExplanation = explanation
		qualified[node.ID] = e.allocated
	}
	if len(qualified) == 0 {
		return links
	}
	out := make([]EnergyPathLink, 0, len(links))
	merged := map[string]int{}
	for _, original := range links {
		link := original
		basis := qualified[link.ToID]
		if link.Relation != "load_to_end_use" || basis == "" {
			out = append(out, link)
			continue
		}
		link.Basis = basis
		link.RuleID = energyRelationshipRuleMixedHVACConsumptionBasis
		link.Explanation = explanation
		link.SourceIDs = appendUniqueStrings(nil, link.SourceIDs...)
		link.RelatedPathIDs = appendUniqueStrings(nil, link.RelatedPathIDs...)
		link.ID = energyPathLinkID(link)
		key := energyPathV2LinkAggregationKey(link)
		if index, exists := merged[key]; exists {
			current := &out[index]
			current.FromValue = roundedEnergyNumber(current.FromValue + link.FromValue)
			current.ToValue = roundedEnergyNumber(current.ToValue + link.ToValue)
			current.SourceIDs = appendUniqueStrings(current.SourceIDs, link.SourceIDs...)
			current.RelatedPathIDs = appendUniqueStrings(current.RelatedPathIDs, link.RelatedPathIDs...)
			finalizeEnergyPathLinkRatio(current)
			continue
		}
		merged[key] = len(out)
		out = append(out, link)
	}
	for index := range out {
		if out[index].RuleID == energyRelationshipRuleMixedHVACConsumptionBasis {
			sort.Strings(out[index].SourceIDs)
			sort.Strings(out[index].RelatedPathIDs)
		}
	}
	qualifyEnergyPathLinkCollisions(out)
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func energyPathNativeConsumptionSources(sources []EnergyDataSource, scope EnergyExplanationScope) map[string]bool {
	out := map[string]bool{}
	if scope.Kind != "zone" || strings.TrimSpace(scope.ZoneName) == "" {
		return out
	}
	for _, source := range sources {
		if source.SourceType == "sql_report_data" && !source.IsMeter && strings.EqualFold(source.Name, "Baseboard Electricity Energy") && strings.EqualFold(source.ReportingFrequency, "Monthly") && strings.EqualFold(source.ZoneName, scope.ZoneName) && source.SourceUnit == "J" && source.NormalizedUnit == "kWh" {
			out[source.ID] = true
		}
	}
	return out
}
