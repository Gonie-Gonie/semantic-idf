package simulation

// Only actual positive source-local reservations authorize carrier-ribbon
// provenance. A declared pool, context source, missing month or measured zero
// alone is not proof of consumption allocated to a visible branch.
func energyPathHVACConsumptionSourceIndex(plan energyPathZoneHVACAllocationPlan) map[string]bool {
	out := map[string]bool{}
	for _, row := range plan.ConsumptionSourceAllocations {
		if (row.ServiceKind != "cooling" && row.ServiceKind != "heating") || row.Carrier == "" || row.ZoneName == "" ||
			!energyPathFinite(row.AllocatedValue) || row.AllocatedValue <= 0 || len(row.SourceIDs) != 1 || row.SourceIDs[0] == "" {
			continue
		}
		out[energyPathZoneHVACAllocationGroupKey(row.ServiceKind, row.Carrier)+"\x00"+row.SourceIDs[0]] = true
	}
	return out
}

func appendEnergyPathHVACConsumptionSources(ids []string, endUse EnergyExplanationNode, observed map[string]bool) []string {
	if !endUse.hvacConsumptionPoolBound || !endUse.AllocationApplied || canonicalEnergyPathBasis(endUse.Basis, "") != "service_path_allocation" {
		return ids
	}
	key := energyPathZoneHVACAllocationGroupKey(canonicalEnergyPathEndUse(endUse.EndUse), canonicalEnergyPathCarrier(endUse.Carrier)) + "\x00"
	// Use the still carrier-qualified, selected-Zone endpoint, not the merged
	// canonical node. Its allocation trace also includes denominator/load
	// sources; the native reservation index excludes those and sibling pools.
	for _, id := range endUse.allocationSourceIDs {
		if observed[key+id] {
			ids = appendUniqueStrings(ids, id)
		}
	}
	return ids
}

// Carry exact source-local ownership through all legacy graph, source, ledger
// and contribution projections. This private marker is deliberately not a wire
// field or a fabricated zero-valued allocation edge. The latter would allow an
// older firstNonZero fallback to substitute a thermal load for zero site energy.
func qualifyEnergyPathHVACConsumptionAllocationNodes(nodes []EnergyExplanationNode, plan energyPathZoneHVACAllocationPlan) []EnergyExplanationNode {
	if len(plan.ConsumptionPoolGroups) == 0 {
		return nodes
	}
	out := append([]EnergyExplanationNode(nil), nodes...)
	for index := range out {
		node := &out[index]
		if !plan.CentralEndUseNodeIDs[node.ID] {
			continue
		}
		service := canonicalEnergyPathEndUse(firstNonEmpty(node.EndUse, energyExplanationKindSuffix(node.Kind)))
		group := energyPathZoneHVACAllocationGroupKey(service, canonicalEnergyPathPart(node.Carrier))
		node.hvacConsumptionPoolBound = plan.ConsumptionPoolGroups[group]
	}
	return out
}
