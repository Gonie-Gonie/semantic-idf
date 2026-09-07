package simulation

import (
	"math"
	"sort"
	"strings"
)

// refreshEnergyPathAllocatedDriverLinks restores the presentation of an already
// calculated allocation. It never allocates annual raw pressure again and does
// not alter conversion links, whose values may cover only overlapping periods.
func refreshEnergyPathAllocatedDriverLinks(nodes []EnergyExplanationNode, links []EnergyPathLink, period string) []EnergyPathLink {
	nodeByID := make(map[string]EnergyExplanationNode, len(nodes))
	for _, node := range nodes {
		nodeByID[node.ID] = node
	}
	validCounts := map[string]int{}
	for _, link := range links {
		if link.Relation == "driver_to_load" && energyPathDriverLinkEndpointsMatch(nodeByID[link.FromID], nodeByID[link.ToID], link.ServiceKind) {
			validCounts[link.FromID]++
		}
	}
	out := make([]EnergyPathLink, 0, len(links))
	for _, original := range links {
		if original.Relation != "driver_to_load" {
			out = append(out, original)
			continue
		}
		driver, load := nodeByID[original.FromID], nodeByID[original.ToID]
		if !energyPathDriverLinkEndpointsMatch(driver, load, original.ServiceKind) {
			continue
		}
		link := original
		link.SourceIDs = appendUniqueStrings(nil, original.SourceIDs...)
		link.ServiceKind = energyCanonicalServiceKind(load.ServiceKind)
		link.FromUnit, link.ToUnit = driver.Unit, load.Unit
		if driver.AllocationApplied && validCounts[driver.ID] == 1 && energyPathFinite(driver.AllocatedValue) {
			allocated := math.Abs(driver.AllocatedValue)
			link.FromValue, link.ToValue = allocated, allocated
			link.SourceIDs = appendUniqueStrings(link.SourceIDs, driver.SourceIDs...)
			link.SourceIDs = appendUniqueStrings(link.SourceIDs, load.SourceIDs...)
			link.Explanation = firstNonEmpty(link.Explanation, driver.AllocationExplanation)
		}
		sort.Strings(link.SourceIDs)
		out = append(out, link)
	}
	for _, driver := range nodes {
		if driver.Level != "driver" || !driver.AllocationApplied || validCounts[driver.ID] != 0 || !energyPathPositiveFinite(driver.AllocatedValue) {
			continue
		}
		var target *EnergyExplanationNode
		for index := range nodes {
			if !energyPathDriverLinkEndpointsMatch(driver, nodes[index], "") {
				continue
			}
			if target != nil {
				target = nil // More than one matching load gives no safe allocation.
				break
			}
			target = &nodes[index]
		}
		if target == nil {
			continue
		}
		link := EnergyPathLink{
			FromID: driver.ID, ToID: target.ID,
			Relation: "driver_to_load", Basis: "heat_balance_share",
			RuleID:      energyRelationshipRuleHeatDriverBalance,
			Explanation: firstNonEmpty(driver.AllocationExplanation, energyDriverAllocationExplanation),
			FromValue:   driver.AllocatedValue, ToValue: driver.AllocatedValue,
			FromUnit: driver.Unit, ToUnit: target.Unit,
			Period: period, ZoneName: driver.ZoneName,
			ServiceKind:    energyCanonicalServiceKind(target.ServiceKind),
			SourceIDs:      appendUniqueStrings(appendUniqueStrings(nil, driver.SourceIDs...), target.SourceIDs...),
			RelatedPathIDs: appendUniqueStrings(nil, driver.RelatedPathIDs...),
		}
		link.ID = energyPathLinkID(link)
		sort.Strings(link.SourceIDs)
		out = append(out, link)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func energyPathDriverLinkEndpointsMatch(driver EnergyExplanationNode, load EnergyExplanationNode, linkService string) bool {
	if driver.Level != "driver" || load.Level != "load" ||
		!strings.EqualFold(driver.ScaleDomain, "thermal") || !strings.EqualFold(load.ScaleDomain, "thermal") {
		return false
	}
	service := energyCanonicalServiceKind(driver.ServiceKind)
	if (service != "cooling" && service != "heating") || service != energyCanonicalServiceKind(load.ServiceKind) ||
		(linkService != "" && energyCanonicalServiceKind(linkService) != service) ||
		!strings.EqualFold(strings.TrimSpace(driver.ZoneName), strings.TrimSpace(load.ZoneName)) {
		return false
	}
	unit := energyPathConversionUnitBase(driver.Unit)
	return unit != "" && unit == energyPathConversionUnitBase(load.Unit)
}
