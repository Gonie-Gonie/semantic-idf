package simulation

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
