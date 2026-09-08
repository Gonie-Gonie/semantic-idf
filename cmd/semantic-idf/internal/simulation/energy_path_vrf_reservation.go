package simulation

import (
	"math"
	"strings"
)

// Reserve the entire observed native VRF subtotal before the generic allocator
// distributes a broad meter's remainder. An unallocated outdoor pool is still
// owned by that VRF system; it is never another system's available electricity.
// This operates on private accounting only and does not manufacture direct
// series or relax the generic direct-first ownership guards.
func reserveEnergyPathVRFAllocation(plan energyPathZoneHVACAllocationPlan, nodes []EnergyExplanationNode, native energyPathVRFAllocationPlan, periodID string, displays ...energyPathVRFAllocationPlan) energyPathZoneHVACAllocationPlan {
	type reservation struct {
		direct, shared, allocated float64
		complete                  bool
		foreignCarrier            bool
		owners                    map[string]bool
		sources                   []string
	}
	groups := map[string]*reservation{}
	for _, row := range native.Services {
		if !strings.EqualFold(row.PeriodID, periodID) {
			continue
		}
		key := energyPathZoneHVACAllocationGroupKey(row.ServiceKind, row.Carrier)
		group := groups[key]
		if group == nil {
			group = &reservation{complete: true, owners: map[string]bool{}}
			groups[key] = group
		}
		group.direct += row.DirectValue
		group.shared += row.SharedValue
		group.allocated += row.AllocatedValue
		group.complete = group.complete && row.DirectKnown && row.SharedKnown
		group.sources = appendUniqueStrings(group.sources, row.ConsumptionSourceIDs...)
		group.sources = appendUniqueStrings(group.sources, row.LoadSourceIDs...)
	}
	if len(groups) == 0 {
		return plan
	}
	displayValues := map[string][2]float64{}
	if len(displays) > 0 {
		for _, row := range displays[0].Services {
			if !strings.EqualFold(row.PeriodID, periodID) {
				continue
			}
			key := energyPathZoneHVACAllocationGroupKey(row.ServiceKind, row.Carrier)
			value := displayValues[key]
			value[0] += row.DirectValue
			value[1] += row.AllocatedValue
			displayValues[key] = value
		}
	}
	for _, row := range native.Zones {
		if strings.EqualFold(row.PeriodID, periodID) {
			if group := groups[energyPathZoneHVACAllocationGroupKey(row.ServiceKind, row.Carrier)]; group != nil {
				group.owners[energyPathVRFName(row.ZoneName)] = true
			}
		}
	}
	// An electric VRF path alone cannot qualify its owner for another broad
	// carrier. Keep unrelated recipients' existing shares unchanged rather
	// than claiming that this newly unassigned amount belongs entirely to them.
	for _, record := range plan.Records {
		key := energyPathZoneHVACAllocationGroupKey(record.ServiceKind, record.Carrier)
		if groups[key] != nil {
			continue
		}
		owners := map[string]bool{}
		for _, row := range native.Zones {
			if strings.EqualFold(row.PeriodID, periodID) && row.ServiceKind == record.ServiceKind {
				owners[energyPathVRFName(row.ZoneName)] = true
			}
		}
		if len(owners) > 0 {
			groups[key] = &reservation{complete: true, foreignCarrier: true, owners: owners}
		}
	}
	nodeByID := map[string]EnergyExplanationNode{}
	for _, node := range nodes {
		nodeByID[node.ID] = node
	}
	edgesByGroup := map[string][]EnergyExplanationEdge{}
	out := plan
	out.Edges = nil
	out.Records = append([]energyPathZoneHVACAllocationRecord(nil), plan.Records...)
	for _, edge := range plan.Edges {
		endUse, exists := nodeByID[edge.FromID]
		key := energyPathZoneHVACAllocationGroupKey(endUse.EndUse, endUse.Carrier)
		group := groups[key]
		if !exists || group == nil {
			out.Edges = append(out.Edges, edge)
			continue
		}
		zone := firstNonEmpty(edge.ZoneName, nodeByID[edge.ToID].ZoneName)
		if !group.owners[energyPathVRFName(zone)] || group.foreignCarrier && strings.TrimSpace(endUse.LoopName) != "" {
			edgesByGroup[key] = append(edgesByGroup[key], edge)
		}
	}
	for i := range out.Records {
		record := &out.Records[i]
		key := energyPathZoneHVACAllocationGroupKey(record.ServiceKind, record.Carrier)
		group := groups[key]
		if group == nil {
			continue
		}
		// Generic exact direct observations have never contained these private
		// VRF constituents. Their direct coverage is additive, not a replacement.
		remaining := math.Max(0, roundedEnergyNumber(record.ExpectedValue-record.DirectValue-group.direct-group.shared))
		if !group.complete {
			// With an unknown constituent, the meter cannot distinguish missing
			// VRF consumption from another central system. Preserve exact direct
			// observations, but don't label that unidentified residue an allocation.
			remaining = 0
		}
		edges := edgesByGroup[key]
		weights := make([]float64, len(edges))
		for j, edge := range edges {
			weights[j] = edge.Value
		}
		shares := energyPathZoneHVACProportionalValues(remaining, weights)
		if group.foreignCarrier {
			shares = weights
		}
		genericAllocated := 0.0
		usedPath, usedLoad := group.allocated > energyPathZoneHVACAllocationEpsilon, false
		for j, edge := range edges {
			if j >= len(shares) || shares[j] <= 0 {
				continue
			}
			edge.Value = shares[j]
			if !group.foreignCarrier {
				edge.Formula += "; reserves complete original VRF local and outdoor consumption before the remaining non-VRF share"
			}
			out.Edges = append(out.Edges, edge)
			genericAllocated += edge.Value
			usedPath = usedPath || edge.Basis == "service_path_allocation"
			usedLoad = usedLoad || edge.Basis == "zone_load_allocation"
		}
		directDisplay, allocatedDisplay := group.direct, group.allocated
		if value, present := displayValues[key]; present && !group.foreignCarrier {
			// Ownership/reservation above uses original quantities. These two
			// published ledger columns must instead match the completed monthly
			// source budgets shown by the Zone graphs, including Annual sums.
			directDisplay, allocatedDisplay = value[0], value[1]
		}
		record.DirectValue = roundedEnergyNumber(record.DirectValue + directDisplay)
		record.AllocatedValue = roundedEnergyNumber(genericAllocated + allocatedDisplay)
		record.UnassignedValue, record.OvermappedValue = 0, 0
		residual := roundedEnergyNumber(record.ExpectedValue - record.DirectValue - record.AllocatedValue)
		if residual > 0 {
			record.UnassignedValue = residual
		} else if residual < 0 {
			record.OvermappedValue = -residual
		}
		record.UsedServicePath, record.UsedZoneLoad = usedPath, usedLoad
		record.SourceIDs = appendUniqueStrings(append([]string(nil), record.SourceIDs...), group.sources...)
	}
	sortEnergyExplanationEdges(out.Edges)
	return out
}
