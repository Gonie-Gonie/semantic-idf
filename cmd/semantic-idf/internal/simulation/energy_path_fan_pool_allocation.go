package simulation

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type energyPathFanSourceAllocation struct {
	SourceID string
	ZoneName string
	Value    float64
	Method   string
}

func allocateEnergyPathReportedFanPools(plan *energyPathZoneAuxiliaryAllocationPlan, record energyPathZoneAuxiliaryAllocationRecord, owners, nodes []EnergyExplanationNode, topology energyServicePathIndex, directZones map[string]bool, pools []energyPathFanPool) energyPathZoneAuxiliaryAllocationRecord {
	record.Method = "unassigned"
	record.UnassignedValue = math.Max(0, roundedEnergyNumber(record.ExpectedValue-record.DirectValue))
	if record.DirectValue > record.ExpectedValue {
		record.OvermappedValue = roundedEnergyNumber(record.DirectValue - record.ExpectedValue)
	}
	for _, pool := range pools {
		if pool.Source.ID != "" {
			record.SourceIDs = appendUniqueStrings(record.SourceIDs, pool.Source.ID)
		}
	}
	// An exact Zone observation cannot be subtracted from arbitrary AirLoops.
	// Keep its existing precedence, but do not allocate overlapping unknown pools.
	if len(owners) != 1 || len(directZones) > 0 {
		record.poolReason = "ambiguous broad owner or overlapping direct Zone observation; no pool allocation"
		return record
	}
	poolTotal, validCount := 0.0, 0
	for _, pool := range pools {
		if pool.Invalid {
			record.poolReason = "invalid or duplicate reported pool identity; no pool allocation"
			return record
		}
		if value, known := energyPathFanPoolValue(pool, record.Period); known {
			poolTotal += value
			validCount++
		}
	}
	bound := float64(validCount+1) * 0.0005
	if !energyPathFinite(poolTotal) || poolTotal-record.ExpectedValue > bound {
		record.poolReason = "reported pool sum exceeds the broad Fans meter beyond its counted serialization bound; no pool allocation"
		return record
	}
	owner := owners[0]
	ownerPaths := map[string]bool{}
	for _, id := range owner.RelatedPathIDs {
		ownerPaths[strings.ToLower(strings.TrimSpace(id))] = true
	}
	pathIdentity := map[string]energyPathAuxiliaryServicePath{}
	for _, path := range topology.auxiliaryPaths {
		if prior, exists := pathIdentity[path.ID]; exists && prior != path {
			record.poolReason = "conflicting exact service-path identity; no pool allocation"
			return record
		}
		pathIdentity[path.ID] = path
	}
	edges := map[string]*EnergyExplanationEdge{}
	methods := map[string]bool{}
	allKnownAndResolved := validCount == len(pools)
	for _, pool := range pools {
		value, known := energyPathFanPoolValue(pool, record.Period)
		if !known {
			allKnownAndResolved = false
			continue
		}
		pathIDs := []string{}
		for _, path := range topology.auxiliaryPaths {
			if len(ownerPaths) > 0 && !ownerPaths[strings.ToLower(strings.TrimSpace(path.ID))] {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(path.AirLoopName), pool.AirLoopName) && path.ZoneName != "" && path.ID != "" {
				pathIDs = appendUniqueStrings(pathIDs, path.ID)
			}
		}
		if len(pathIDs) == 0 {
			allKnownAndResolved = false
			continue
		}
		if value == 0 {
			continue // observed zero requires no fabricated positive-load target
		}
		eligible := energyPathZoneAuxiliaryEligiblePaths("fans", pathIDs, topology.auxiliaryPaths)
		targets, method := energyPathZoneAuxiliaryTargetsFromPaths("fans", eligible, nodes, nil)
		if len(targets) == 0 {
			allKnownAndResolved = false
			continue
		}
		if airflow, ok := energyPathZoneAuxiliaryAirflowTargets(targets, nodes); ok {
			targets, method = airflow, "airflow_share"
		}
		allocations := energyPathFanPoolShares(value, targets)
		if len(allocations) != len(targets) {
			allKnownAndResolved = false
			continue
		}
		for i, target := range targets {
			if i >= len(allocations) || allocations[i] <= 0 {
				continue
			}
			allocated := allocations[i]
			record.AllocatedValue = roundedEnergyNumber(record.AllocatedValue + allocated)
			methods[method] = true
			plan.FanSourceAllocations = append(plan.FanSourceAllocations, energyPathFanSourceAllocation{SourceID: pool.Source.ID, ZoneName: target.ZoneName, Value: allocated, Method: method})
			key := edgeID("allocation_zone_auxiliary", record.Period, owner.ID, target.NodeID)
			edge := edges[key]
			formula := "reported AirLoop fan pool allocated only within its exact served paths by " + energyPathZoneAuxiliaryAllocationFormula(method)
			if edge == nil {
				edge = &EnergyExplanationEdge{ID: key, FromID: owner.ID, ToID: target.NodeID, Unit: record.Unit, Period: record.Period, Relation: energyPathAuxiliaryAllocationRelation, Basis: "service_path_allocation", RuleID: energyRelationshipRuleAllocatedAuxiliaryServicePath, ZoneName: target.ZoneName, ServiceKind: target.ServiceKind,
					Formula: formula}
				edges[key] = edge
			} else if edge.Formula != formula {
				edge.Formula = "reported AirLoop fan pools allocated within their exact served paths by mixed source-specific auxiliary evidence"
			}
			edge.Value = roundedEnergyNumber(edge.Value + allocated)
			edge.SourceIDs = appendUniqueStrings(edge.SourceIDs, pool.Source.ID)
			edge.SourceIDs = appendUniqueStrings(edge.SourceIDs, owner.SourceIDs...)
			edge.SourceIDs = appendUniqueStrings(edge.SourceIDs, target.SourceIDs...)
			edge.RelatedPathIDs = appendUniqueStrings(edge.RelatedPathIDs, target.PathIDs...)
		}
	}
	keys := make([]string, 0, len(edges))
	for key := range edges {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		edge := edges[key]
		sort.Strings(edge.SourceIDs)
		sort.Strings(edge.RelatedPathIDs)
		if record.ExpectedValue > 0 {
			edge.Formula += fmt.Sprintf("; allocation factor %.6f of broad end use (source-specific pool shares retained separately)", edge.Value/record.ExpectedValue)
		}
		plan.Edges = append(plan.Edges, *edge)
	}
	residual := roundedEnergyNumber(record.ExpectedValue - record.DirectValue - record.AllocatedValue)
	record.UnassignedValue, record.OvermappedValue = math.Max(0, residual), math.Max(0, -residual)
	if allKnownAndResolved && math.Abs(poolTotal-record.ExpectedValue) <= bound && math.Abs(residual) <= bound {
		record.poolRoundingBound = bound
		record.UnassignedValue, record.OvermappedValue = 0, 0
	} else {
		record.poolReason = "incomplete or unresolved pool coverage; remaining broad meter energy is unassigned"
	}
	if len(methods) == 1 {
		for method := range methods {
			record.Method = method
		}
	} else if len(methods) > 1 {
		record.Method = "mixed"
	}
	sort.SliceStable(plan.FanSourceAllocations, func(i, j int) bool {
		left, right := plan.FanSourceAllocations[i], plan.FanSourceAllocations[j]
		return left.SourceID+"\x00"+left.ZoneName < right.SourceID+"\x00"+right.ZoneName
	})
	sort.Strings(record.SourceIDs)
	return record
}

// Apportion the fixed 3dp budget by largest fractional remainders. Rounding
// early targets independently and dumping their combined error on the last
// Zone can put that Zone outside its own one-quantum allocation bound, even
// for ordinary positive pools. Every target instead receives floor or ceil
// of its exact quota, and ties use stable semantic identity, not input order.
// The measured pool scalar is not scaled or overwritten.
func energyPathFanPoolShares(total float64, targets []energyPathZoneAuxiliaryTarget) []float64 {
	weightTotal := 0.0
	for _, target := range targets {
		if target.Value < 0 || !energyPathFinite(target.Value) {
			return nil
		}
		weightTotal += target.Value
	}
	if !energyPathFinite(total) || !energyPathFinite(weightTotal) || total < 0 || weightTotal <= 0 {
		return nil
	}
	budget := math.Round(total * 1000)
	if !energyPathFinite(budget) || budget > 1<<53 {
		return nil
	}
	type remainder struct {
		index    int
		fraction float64
		identity string
	}
	remainders := make([]remainder, len(targets))
	values := make([]float64, len(targets))
	used := 0.0
	for i, target := range targets {
		exact := budget * (target.Value / weightTotal)
		values[i] = math.Floor(exact)
		used += values[i]
		remainders[i] = remainder{i, exact - values[i], target.NodeID + "\x00" + target.ZoneName}
	}
	remaining := budget - used
	if remaining < 0 || remaining > float64(len(targets)) {
		return nil
	}
	sort.SliceStable(remainders, func(i, j int) bool {
		if remainders[i].fraction != remainders[j].fraction {
			return remainders[i].fraction > remainders[j].fraction
		}
		return remainders[i].identity < remainders[j].identity
	})
	for i := 0; i < int(remaining); i++ {
		values[remainders[i].index]++
	}
	for i := range values {
		values[i] /= 1000
	}
	return values
}

func appendEnergyPathFanPoolSources(sources []EnergyDataSource, pools []energyPathFanPool, plan energyPathZoneAuxiliaryAllocationPlan, scope EnergyExplanationScope) []EnergyDataSource {
	out := append([]EnergyDataSource(nil), sources...)
	existing := map[string]bool{}
	for _, source := range out {
		existing[source.ID] = true
	}
	for _, pool := range pools {
		if pool.Source.ID == "" || existing[pool.Source.ID] {
			continue
		}
		source := pool.Source
		if scope.Kind == "zone" {
			allocated := 0.0
			method := ""
			found := false
			for _, allocation := range plan.FanSourceAllocations {
				if allocation.SourceID == source.ID && strings.EqualFold(allocation.ZoneName, scope.ZoneName) {
					allocated = roundedEnergyNumber(allocated + allocation.Value)
					if found && method != allocation.Method {
						method = "mixed"
					} else {
						method = allocation.Method
					}
					found = true
				}
			}
			if !found {
				continue
			}
			// A measured AirLoop total is not a measured individual Zone value.
			source.RawValue, source.EffectiveValue = 0, 0
			source.observedValuePresence &^= energySourceObservedRaw | energySourceObservedEffective
			source.inspectorValuePresence &^= energySourceObservedRaw | energySourceObservedEffective
			source.EffectiveMultiplier, source.AllocationFactor = 0, 0
			source.MultiplierApplication = ""
			source.AllocatedValue, source.AllocationApplied = allocated, true
			source.AllocationExplanation = "Allocated from the exact reported AirLoop fan pool by " + method + "; raw individual-Zone energy is unavailable."
			source.AllocationFormula = "sum of monthly source-local allocations within the exact AirLoop served paths"
			if total, known := energyPathFanPoolValue(pool, "annual"); known && total > 0 {
				source.AllocationFactor = allocated / total
			}
		} else if total, known := energyPathFanPoolValue(pool, "annual"); known {
			source.RawValue, source.EffectiveValue = total, total
			source.observedValuePresence = energySourceObservedRaw | energySourceObservedEffective
		}
		source.AggregationBasis = scope.AggregationBasis
		out = append(out, source)
		existing[source.ID] = true
	}
	return out
}
