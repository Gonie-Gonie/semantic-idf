package simulation

import (
	"sort"
	"strings"
)

func energyPathLinkIsCarrierSplit(link EnergyPathLink) bool {
	return link.Relation == "end_use_to_carrier" || link.Relation == "direct_end_use_to_carrier"
}

func energyPathStoredCarrierSplitValid(link EnergyPathLink, endUse EnergyExplanationNode, carrier EnergyExplanationNode) bool {
	if endUse.Level != "end_use" || carrier.Level != "carrier" ||
		!strings.EqualFold(endUse.ScaleDomain, "site") || !strings.EqualFold(carrier.ScaleDomain, "site") ||
		!energyPathFinite(link.ToValue) || link.ToValue < 0 {
		return false
	}
	fromUnit, toUnit := energyPathConversionUnitBase(link.FromUnit), energyPathConversionUnitBase(link.ToUnit)
	return fromUnit != "" && fromUnit == toUnit &&
		fromUnit == energyPathConversionUnitBase(endUse.Unit) && toUnit == energyPathConversionUnitBase(carrier.Unit)
}

// refreshEnergyPathEndUseCarrierSplits uses the carrier-qualified, period-local
// ToValue already used by facility reconciliation. A source's annual effective
// value is not a replacement for a monthly or allocated-zone ribbon value.
func refreshEnergyPathEndUseCarrierSplits(nodes []EnergyExplanationNode, links []EnergyPathLink, pruned map[string]bool) ([]EnergyExplanationNode, []EnergyPathLink) {
	totals := map[string]float64{}
	outLinks := make([]EnergyPathLink, 0, len(links))
	for _, original := range links {
		link := original
		if energyPathLinkIsCarrierSplit(link) {
			link.ToValue = roundedEnergyNumber(link.ToValue)
			link.FromValue = link.ToValue
			// A carrier-neutral node owns other carriers' source IDs too. Keep
			// the exact branch provenance rather than copying that union.
			link.SourceIDs = appendUniqueStrings(nil, original.SourceIDs...)
			sort.Strings(link.SourceIDs)
			totals[link.FromID] = roundedEnergyNumber(totals[link.FromID] + link.ToValue)
		}
		outLinks = append(outLinks, link)
	}
	removed := map[string]bool{}
	outNodes := make([]EnergyExplanationNode, 0, len(nodes))
	for _, original := range nodes {
		node := original
		if node.Level == "end_use" {
			total, hasSplits := totals[node.ID]
			if hasSplits || pruned[node.ID] {
				if total == 0 {
					// Keep original sources in the result, but do not let a zero
					// graph total fall back to its older nonzero raw/effective value.
					removed[node.ID] = true
					continue
				}
				if pruned[node.ID] && total != node.Value {
					node.Badges = appendUniqueStrings(appendUniqueStrings(nil, node.Badges...), "filtered_carrier_splits")
					const note = "Graph energy includes only valid retained carrier splits; raw and effective values retain the original source context."
					if !strings.Contains(node.AllocationExplanation, note) {
						node.AllocationExplanation = strings.TrimSpace(node.AllocationExplanation + " " + note)
					}
				}
				node.Value, node.DisplayValue, node.AllocatedValue = total, total, total
			}
		}
		outNodes = append(outNodes, node)
	}
	if len(removed) > 0 {
		filtered := make([]EnergyPathLink, 0, len(outLinks))
		for _, link := range outLinks {
			if !removed[link.FromID] && !removed[link.ToID] {
				filtered = append(filtered, link)
			}
		}
		outLinks = filtered
	}
	return outNodes, outLinks
}
