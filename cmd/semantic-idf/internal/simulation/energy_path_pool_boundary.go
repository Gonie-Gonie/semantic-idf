package simulation

import "strings"

func mergeEnergyPathPoolSources(existing, native []EnergyDataSource) []EnergyDataSource {
	if len(native) == 0 {
		return existing
	}
	out := append([]EnergyDataSource(nil), existing...)
	indices := map[string]int{}
	for i, source := range out {
		indices[source.ID] = i
	}
	for _, source := range native {
		if i, exists := indices[source.ID]; exists {
			out[i] = source
		} else {
			indices[source.ID] = len(out)
			out = append(out, source)
		}
	}
	return out
}

// A stored v1 graph retains restrictions but cannot serialize positive private
// source/roster authority. Missing runtime pools must therefore disable, not
// restore, generic broad-meter allocation on read. Existing measured cohorts
// retain their own direct/source-local allocation contracts.
func energyPathBoundaryFallbackHVACConsumptionPools(pools []energyPathHVACConsumptionPool, nodes []EnergyExplanationNode) []energyPathHVACConsumptionPool {
	out := pools
	for _, node := range nodes {
		if !energyPathServiceBoundaryDeniesConversion(node) || node.ZoneName != "" {
			continue
		}
		endUse, carrier := energyPathServiceBoundaryEndUse(node), canonicalEnergyPathPart(node.Carrier)
		bound := false
		for _, pool := range out {
			if pool.ServiceKind == endUse && pool.Carrier == carrier && len(energyPathZoneHVACIntersectPaths(pool.MeterSourceIDs, node.SourceIDs)) > 0 {
				bound = true
				break
			}
		}
		if bound {
			continue
		}
		if len(out) == len(pools) {
			out = append([]energyPathHVACConsumptionPool(nil), pools...)
		}
		out = append(out, energyPathHVACConsumptionPool{ID: "restricted.v1:" + node.ID, ServiceKind: endUse, Carrier: carrier, MeterSourceIDs: appendUniqueStrings(nil, node.SourceIDs...), Valid: false})
	}
	return out
}

// Apply to all original Building contributors before scope projection. The
// scope's exclusion of the pool's surface Zone cannot erase a shared-source
// boundary. All values, source observations and accounting remain unchanged.
func applyEnergyPathPoolResultRestrictions(result *EnergyExplanationV1) {
	if result == nil || !result.poolEvidence.Inventory.HasNativePool {
		return
	}
	result.Nodes = energyPathPoolRestrictedNodes(result.Nodes, result.poolEvidence)
	for i := range result.Periods {
		result.Periods[i].Nodes = energyPathPoolRestrictedNodes(result.Periods[i].Nodes, result.poolEvidence)
	}
}

func energyPathPoolRestrictedNodes(nodes []EnergyExplanationNode, evidence energyPathPoolEvidence) []EnergyExplanationNode {
	if !evidence.Inventory.HasNativePool {
		return nodes
	}
	out := append([]EnergyExplanationNode(nil), nodes...)
	for i := range out {
		node := &out[i]
		endUse := energyPathServiceBoundaryEndUse(*node)
		carrier := canonicalEnergyPathPart(node.Carrier)
		if node.ZoneName != "" || endUse != "heating" && endUse != "pumps" || carrier == "" || endUse == "pumps" && carrier != "electricity" {
			continue
		}
		reviewedCarrier := carrier == "electricity" || carrier == "natural_gas"
		var meterIDs []string
		for _, sourceID := range node.SourceIDs {
			if energyPathBoundarySQLSourceIDValid(sourceID) {
				meterIDs = appendUniqueStrings(meterIDs, sourceID)
			}
		}
		// An original Pool with no native meter reference still denies a false
		// service conversion. Record restriction-only incomplete applicability;
		// never fabricate a source ID, observed scalar, or measured zero.
		var restrictions []energyPathServiceBoundaryRestriction
		for _, pool := range evidence.Inventory.Pools {
			reason := energyPathBoundaryTopologyIncomplete
			loopName := ""
			if reviewedCarrier && len(meterIDs) > 0 && pool.resolved() && len(evidence.Inventory.UnresolvedPoolIndices) == 0 {
				reason, loopName = energyPathBoundaryNonZoneDemand, pool.Loop.ObjectName
			}
			r := energyPathServiceBoundaryRestriction{Reason: reason, EndUse: endUse, ServiceKind: "heating", Carrier: carrier,
				PlantLoopName: loopName, DemandObjectType: pool.Component.ObjectType, DemandObjectName: pool.Component.ObjectName, MeterSourceIDs: meterIDs}
			for _, observation := range evidence.Observations {
				if !reviewedCarrier {
					continue
				} // Unknown fuel cannot acquire positive source-disjointness authority.
				if observation.Definition.Role != energyPathPoolPurchasedConstituent || observation.Definition.EndUse != endUse || observation.Definition.Carrier != carrier {
					continue
				}
				if reason == energyPathBoundaryNonZoneDemand && observation.Loop.ObjectIndex != pool.Loop.ObjectIndex {
					continue
				}
				for _, sourceID := range observation.Series.SourceIDs {
					if energyPathBoundarySQLSourceIDValid(sourceID) {
						r.ConsumerSourceIDs = appendUniqueStrings(r.ConsumerSourceIDs, sourceID)
					}
				}
			}
			restrictions = append(restrictions, r)
		}
		node.serviceBoundaryRestrictions = unionEnergyPathServiceBoundaryRestrictions(node.serviceBoundaryRestrictions, restrictions)
	}
	return out
}

// Surface values remain physical reported values. Only the interpretation is
// qualified: a pool-bearing floor is not an isolated passive envelope element.
func qualifyEnergyPathPoolSurfaceSources(sources []EnergyDataSource, inventory energyPathPoolInventory) []EnergyDataSource {
	if !inventory.HasNativePool {
		return sources
	}
	out := append([]EnergyDataSource(nil), sources...)
	for i := range out {
		for _, pool := range inventory.Pools {
			if !pool.SurfaceOwnerValid || !strings.EqualFold(strings.TrimSpace(out[i].KeyValue), strings.TrimSpace(pool.Surface.ObjectName)) || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(out[i].Name)), "surface ") {
				continue
			}
			out[i].Explanation += " Pool-bearing surface: the reported surface transfer is retained, but this boundary is not an isolated passive floor. Pool-water plant heat is separate nonadditive context, not an additional Zone-air driver."
			out[i].RelatedEntityIDs = appendUniqueStrings(out[i].RelatedEntityIDs, pool.Component.ID, pool.Surface.ID)
		}
	}
	return out
}
