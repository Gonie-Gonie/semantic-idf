package simulation

import (
	"math"
	"sort"
	"strings"
)

type energyPathPoolPumpSourceAllocation struct {
	SourceID, ZoneName, ServiceKind string
	Value                           float64
	LoadSourceIDs, RelatedPathIDs   []string
}

// Replace only the broad Pumps allocation. An unquantified HW pool demand does
// not identify a Zone share. A distinct observed CW component can retain its own
// estimate after its complete water AND air demand roster has been proved.
func reserveEnergyPathPoolPumpAllocation(plan energyPathZoneAuxiliaryAllocationPlan, nodes []EnergyExplanationNode, topology energyServicePathIndex, evidence energyPathPoolEvidence, periodID, periodKind string, canonicalMonthlyBasis bool) energyPathZoneAuxiliaryAllocationPlan {
	guarded := evidence.Inventory.HasNativePool
	for _, node := range nodes {
		if energyPathServiceBoundaryEndUse(node) == "pumps" && len(node.serviceBoundaryRestrictions) > 0 {
			guarded = true
		}
	}
	if !guarded {
		return plan
	}
	owners := []EnergyExplanationNode{}
	ownerIDs := map[string]bool{}
	for _, node := range nodes {
		if !plan.CentralEndUseNodeIDs[node.ID] || energyPathServiceBoundaryEndUse(node) != "pumps" || canonicalEnergyPathPart(node.Carrier) != "electricity" {
			continue
		}
		owners = append(owners, node)
		ownerIDs[node.ID] = true
	}
	if len(owners) == 0 {
		return plan
	}
	out := plan
	out.Edges = nil
	out.Records = append([]energyPathZoneAuxiliaryAllocationRecord(nil), plan.Records...)
	out.PoolPumpSourceAllocations = nil
	for _, edge := range plan.Edges {
		if !ownerIDs[edge.FromID] {
			out.Edges = append(out.Edges, edge)
		}
	}
	recordIndex := -1
	for i, record := range out.Records {
		if record.EndUse == "pumps" && record.Carrier == "electricity" {
			recordIndex = i
			break
		}
	}
	if recordIndex < 0 {
		record := energyPathZoneAuxiliaryAllocationRecord{Period: periodID, EndUse: "pumps", Carrier: "electricity", Unit: "kWh"}
		for _, owner := range owners {
			record.ExpectedValue += math.Abs(energyExplanationEffectiveNodeValue(owner))
			record.SourceIDs = appendUniqueStrings(record.SourceIDs, owner.SourceIDs...)
		}
		out.Records = append(out.Records, record)
		recordIndex = len(out.Records) - 1
	}
	record := &out.Records[recordIndex]
	record.SourceIDs = appendUniqueStrings(nil, record.SourceIDs...)
	record.AllocatedValue, record.UnassignedValue, record.OvermappedValue = 0, 0, 0
	record.Method, record.poolReason = "unassigned", "Shared pool-water demand has no measured fuel/pump service split; unresolved meter remainder is unassigned."
	finish := func() energyPathZoneAuxiliaryAllocationPlan {
		residual := roundedEnergyNumber(record.ExpectedValue - record.DirectValue - record.AllocatedValue)
		record.UnassignedValue, record.OvermappedValue = math.Max(0, residual), math.Max(0, -residual)
		sort.Strings(record.SourceIDs)
		sortEnergyExplanationEdges(out.Edges)
		return out
	}
	if len(owners) != 1 || record.DirectValue != 0 {
		return finish()
	}
	owner := owners[0]
	observedTotal, observedCount := 0.0, 0
	seen := map[string]bool{}
	for _, observation := range evidence.Observations {
		if observation.Definition.ID != "pump.electricity" {
			continue
		}
		if !observation.Valid {
			continue
		}
		value, _, ids, known := energyPathHVACConsumptionPeriodValue(observation.Series, periodID, periodKind, canonicalMonthlyBasis)
		if !known {
			continue
		}
		if seen[ids[0]] || len(energyPathZoneHVACIntersectPaths(ids, owner.SourceIDs)) > 0 {
			return finish()
		}
		seen[ids[0]] = true
		observedTotal += value
		observedCount++
		record.SourceIDs = appendUniqueStrings(record.SourceIDs, ids...)
	}
	// The count covers only native constituent and broad-meter 0.001 kWh
	// serialization; it is not a relaxed physical closure or an invented budget.
	if !energyPathFinite(observedTotal) || observedTotal-record.ExpectedValue > float64(observedCount+1)*0.0005 {
		return finish()
	}
	merged := map[string]EnergyExplanationEdge{}
	for _, observation := range evidence.Observations {
		if observation.Definition.ID != "pump.electricity" || !observation.Valid {
			continue
		}
		value, _, ids, known := energyPathHVACConsumptionPeriodValue(observation.Series, periodID, periodKind, canonicalMonthlyBasis)
		if !known {
			continue
		}
		for _, service := range []string{"heating", "cooling"} {
			paths := energyPathPoolEligibleSourcePaths(evidence.Inventory, observation.Component, service)
			if len(paths) == 0 || !energyPathPoolLoadRosterComplete(evidence.LoadSeries, topology, paths, service, periodID, periodKind, canonicalMonthlyBasis) {
				continue
			}
			targets := energyPathHVACConsumptionTargets(nodes, topology, service, paths)
			if !energyPathPoolPositiveLoadTargetsComplete(targets, evidence.LoadSeries, topology, paths, service, periodID, periodKind, canonicalMonthlyBasis) {
				continue
			}
			weights := make([]energyPathZoneAuxiliaryTarget, len(targets))
			for i, target := range targets {
				weights[i] = energyPathZoneAuxiliaryTarget{NodeID: target.Node.ID, ZoneName: target.Node.ZoneName, Value: target.Value}
			}
			shares := energyPathFanPoolShares(value, weights)
			for i, target := range targets {
				if i >= len(shares) {
					continue
				}
				out.PoolPumpSourceAllocations = append(out.PoolPumpSourceAllocations, energyPathPoolPumpSourceAllocation{SourceID: ids[0], ZoneName: target.Node.ZoneName,
					ServiceKind: service, Value: shares[i], LoadSourceIDs: appendUniqueStrings(nil, target.SourceIDs...), RelatedPathIDs: appendUniqueStrings(nil, target.PathIDs...)})
				if shares[i] <= 0 {
					continue
				}
				key := edgeID("allocation_zone_auxiliary", periodID, owner.ID, target.Node.ID)
				edge, exists := merged[key]
				if !exists {
					edge = EnergyExplanationEdge{ID: key, FromID: owner.ID, ToID: target.Node.ID, Unit: "kWh", Period: periodID, Relation: energyPathAuxiliaryAllocationRelation,
						Basis: "service_path_allocation", RuleID: energyRelationshipRuleAllocatedAuxiliaryServicePath, ZoneName: target.Node.ZoneName, ServiceKind: service,
						Formula:                       "Independently observed native pump energy * eligible Zone service load / its complete original water-and-air demand recipient load total; unquantified pool-water shares remain unassigned",
						serviceBoundaryExactConsumers: true}
				}
				edge.Value = roundedEnergyNumber(edge.Value + shares[i])
				edge.SourceIDs = appendUniqueStrings(edge.SourceIDs, owner.SourceIDs...)
				edge.SourceIDs = appendUniqueStrings(edge.SourceIDs, ids...)
				edge.SourceIDs = appendUniqueStrings(edge.SourceIDs, target.SourceIDs...)
				edge.RelatedPathIDs = appendUniqueStrings(edge.RelatedPathIDs, target.PathIDs...)
				edge.serviceBoundaryConsumerSourceIDs = appendUniqueStrings(edge.serviceBoundaryConsumerSourceIDs, ids...)
				merged[key] = edge
			}
		}
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		edge := merged[key]
		sort.Strings(edge.SourceIDs)
		sort.Strings(edge.RelatedPathIDs)
		out.Edges = append(out.Edges, edge)
		record.AllocatedValue = roundedEnergyNumber(record.AllocatedValue + edge.Value)
		record.SourceIDs = appendUniqueStrings(record.SourceIDs, edge.SourceIDs...)
	}
	if record.AllocatedValue > 0 {
		record.Method = "service_load_share"
	}
	return finish()
}

// Positive targets alone cannot prove the denominator: a missing Zone must not
// shrink it. Retain known-zero canonical load observations as roster evidence.
func energyPathPoolLoadRosterComplete(series []energyExplanationSeries, topology energyServicePathIndex, paths []string, service, periodID, periodKind string, canonicalMonthlyBasis bool) bool {
	owners := map[string]bool{}
	wanted := map[string]bool{}
	for _, pathID := range paths {
		if strings.TrimSpace(pathID) == "" {
			return false
		}
		wanted[pathID] = true
	}
	seen := map[string]energyPathAuxiliaryServicePath{}
	for _, path := range topology.auxiliaryPaths {
		if !wanted[path.ID] {
			continue
		}
		if previous, exists := seen[path.ID]; exists && previous != path {
			return false
		}
		seen[path.ID] = path
		if path.ZoneName == "" || energyCanonicalServiceKind(path.ServiceKind) != service {
			return false
		}
		owners[energyPathPoolToken(path.ZoneName)] = true
	}
	if len(owners) == 0 || len(seen) != len(wanted) {
		return false
	}
	for zone := range owners {
		found := 0
		for _, item := range series {
			primaryService, primary := energyLoadCanonicalSelectionService(item.ServiceKind)
			if !primary || item.Stage != "load" || energyPathPoolToken(item.ZoneName) != zone || primaryService != service {
				continue
			}
			// Only selected sensible/total load authority can establish a Zone
			// recipient. A latent-only observation is not substitute evidence.
			if !strings.EqualFold(item.Unit, "kWh") {
				return false
			}
			value, _, ids, known := energyPathDirectZonePeriodValue(item, periodID, periodKind, canonicalMonthlyBasis)
			if !known || len(ids) == 0 || !energyPathFinite(value) || value < 0 {
				return false
			}
			if strings.EqualFold(periodKind, "annual") && canonicalMonthlyBasis && len(item.Monthly) != 12 {
				return false
			}
			found++
		}
		if found != 1 {
			return false
		}
	}
	return true
}

func energyPathPoolPositiveLoadTargetsComplete(targets []energyPathZoneHVACLoadTarget, series []energyExplanationSeries, topology energyServicePathIndex, paths []string, service, periodID, periodKind string, canonicalMonthlyBasis bool) bool {
	owners, positive, represented := map[string]bool{}, map[string]bool{}, map[string]bool{}
	expectedValues, actualValues := map[string]float64{}, map[string]float64{}
	expectedSources, actualSources := map[string][]string{}, map[string][]string{}
	for _, path := range topology.auxiliaryPaths {
		if len(energyPathZoneHVACIntersectPaths(paths, []string{path.ID})) > 0 {
			owners[energyPathPoolToken(path.ZoneName)] = true
		}
	}
	for _, item := range series {
		zone := energyPathPoolToken(item.ZoneName)
		primaryService, primary := energyLoadCanonicalSelectionService(item.ServiceKind)
		if !primary || !owners[zone] || item.Stage != "load" || primaryService != service {
			continue
		}
		value, _, ids, known := energyPathDirectZonePeriodValue(item, periodID, periodKind, canonicalMonthlyBasis)
		if known && value > 0 {
			positive[zone] = true
			expectedValues[zone] += value
			expectedSources[zone] = appendUniqueStrings(expectedSources[zone], ids...)
		}
	}
	for _, target := range targets {
		zone := energyPathPoolToken(target.Node.ZoneName)
		if !owners[zone] || !positive[zone] || !energyPathFinite(target.Value) || target.Value <= 0 {
			return false
		}
		represented[zone] = true
		actualValues[zone] += target.Value
		actualSources[zone] = appendUniqueStrings(actualSources[zone], target.SourceIDs...)
	}
	for zone := range positive {
		if !represented[zone] || math.Abs(expectedValues[zone]-actualValues[zone]) > 0.0005 || len(energyPathZoneHVACIntersectPaths(expectedSources[zone], actualSources[zone])) != len(expectedSources[zone]) {
			return false
		}
	}
	return true
}

func applyEnergyPathPoolSourceAllocations(sources []EnergyDataSource, plan energyPathZoneAuxiliaryAllocationPlan, evidence energyPathPoolEvidence, scope EnergyExplanationScope) []EnergyDataSource {
	if !evidence.Inventory.HasNativePool {
		return sources
	}
	pool := energyPathHVACConsumptionPool{}
	for _, observation := range evidence.Observations {
		// Boiler purchased sources already receive the richer HVAC allocator's
		// own trace; do not erase independently eligible non-pool boiler shares.
		if observation.Definition.EndUse == "heating" && observation.Definition.Role == energyPathPoolPurchasedConstituent {
			continue
		}
		pool.Members = append(pool.Members, energyPathHVACConsumptionMember{ObjectType: observation.Component.ObjectType, ObjectName: observation.Component.ObjectName,
			OutputName: observation.Definition.EnergyName, Series: observation.Series})
	}
	trace := energyPathZoneHVACAllocationPlan{}
	for _, row := range plan.PoolPumpSourceAllocations {
		trace.ConsumptionSourceAllocations = append(trace.ConsumptionSourceAllocations, energyPathHVACConsumptionSourceAllocation{SourceIDs: []string{row.SourceID}, ZoneName: row.ZoneName,
			AllocatedValue: row.Value, LoadSourceIDs: row.LoadSourceIDs, RelatedPathIDs: row.RelatedPathIDs})
	}
	return applyEnergyPathHVACConsumptionSourceAllocations(sources, trace, []energyPathHVACConsumptionPool{pool}, scope)
}
