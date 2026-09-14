package simulation

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

const energyRelationshipRuleAllocatedHVACConsumptionPool = "allocation.by_hvac_consumption_pool_service_load_share"

// A pool binds native consuming-component observations to an actual broad
// service/carrier meter. It is private reader/adapter evidence, not another
// Building end use. Valid means that original typed identities, meter membership
// and ownership are unambiguous; it never means that absent observations are 0.
// An original member remains in Members even when its Series has no observations.
type energyPathHVACConsumptionPool struct {
	ID             string
	ServiceKind    string
	Carrier        string
	MeterSourceIDs []string
	Members        []energyPathHVACConsumptionMember
	Valid          bool
}

type energyPathHVACConsumptionMember struct {
	ID             string
	ObjectType     string
	ObjectName     string
	OutputName     string
	ZoneName       string                  // Nonempty is an exact local consuming component.
	RelatedPathIDs []string                // Shared components require original, source-local paths.
	Series         energyExplanationSeries // Actual model-total observations only.
}

// These rows are recorded before broad-end-use/Zone edge merging. An edge may
// sum different boilers with different recipient sets; no edge-wide factor can
// reconstruct an individual component's actual allocation afterwards.
type energyPathHVACConsumptionSourceAllocation struct {
	Period         string
	PoolID         string
	MemberID       string
	ServiceKind    string
	Carrier        string
	ZoneName       string
	SourceIDs      []string
	LoadSourceIDs  []string
	RelatedPathIDs []string
	ObservedValue  float64
	AllocatedValue float64
}

// reserveEnergyPathHVACConsumptionPools replaces only a bound meter group's
// generic remainder allocation. Direct member quantities already occur exactly
// once in the generic plan and the direct Zone graph; this helper does not add
// them again. A local component does not monopolize its owner's entire heating
// service: an independently measured central component can also serve that Zone.
// Only the observed shared constituents are allocated. A broad meter remainder
// is never relabeled as a boiler, even if the recognized roster appears complete.
func reserveEnergyPathHVACConsumptionPools(plan energyPathZoneHVACAllocationPlan, nodes []EnergyExplanationNode, topology energyServicePathIndex, pools []energyPathHVACConsumptionPool, periodID, periodKind string, canonicalMonthlyBasis bool) energyPathZoneHVACAllocationPlan {
	pools = energyPathBoundaryFallbackHVACConsumptionPools(pools, nodes)
	if len(pools) == 0 {
		return plan
	}
	type boundPool struct {
		pool  energyPathHVACConsumptionPool
		node  EnergyExplanationNode
		valid bool
	}
	nodeByID := map[string]EnergyExplanationNode{}
	endUses := map[string][]EnergyExplanationNode{}
	for _, original := range foldLegacyEnergyLoadDetailNodes(nodes) {
		node := energyExplanationLegacyNodeWithEffectiveValues(original)
		nodeByID[node.ID] = node
		if !plan.CentralEndUseNodeIDs[node.ID] {
			continue
		}
		key := energyPathZoneHVACAllocationGroupKey(canonicalEnergyPathEndUse(firstNonEmpty(node.EndUse, energyExplanationKindSuffix(node.Kind))), canonicalEnergyPathPart(node.Carrier))
		endUses[key] = append(endUses[key], node)
	}
	groups := map[string][]boundPool{}
	for _, pool := range pools {
		service := energyCanonicalServiceKind(pool.ServiceKind)
		carrier := canonicalEnergyPathPart(pool.Carrier)
		if (service != "cooling" && service != "heating") || carrier == "" || len(pool.MeterSourceIDs) == 0 {
			continue
		}
		key := energyPathZoneHVACAllocationGroupKey(service, carrier)
		matches := []EnergyExplanationNode{}
		for _, node := range endUses[key] {
			if len(energyPathZoneHVACIntersectPaths(node.SourceIDs, pool.MeterSourceIDs)) > 0 {
				matches = append(matches, node)
			}
		}
		if len(matches) == 0 {
			continue // A foreign meter ID cannot change an unrelated allocation.
		}
		groups[key] = append(groups[key], boundPool{pool: pool, node: matches[0], valid: len(matches) == 1 && pool.Valid})
	}
	if len(groups) == 0 {
		return plan
	}
	out := plan
	out.Edges = nil
	out.Records = append([]energyPathZoneHVACAllocationRecord(nil), plan.Records...)
	out.ConsumptionPoolGroups = map[string]bool{}
	for key := range plan.ConsumptionPoolGroups {
		out.ConsumptionPoolGroups[key] = true
	}
	for key := range groups {
		out.ConsumptionPoolGroups[key] = true
	}
	out.ConsumptionSourceAllocations = nil
	for _, row := range plan.ConsumptionSourceAllocations {
		if len(groups[energyPathZoneHVACAllocationGroupKey(row.ServiceKind, row.Carrier)]) == 0 {
			out.ConsumptionSourceAllocations = append(out.ConsumptionSourceAllocations, row)
		}
	}
	for _, edge := range plan.Edges {
		endUse, exists := nodeByID[edge.FromID]
		key := energyPathZoneHVACAllocationGroupKey(canonicalEnergyPathEndUse(firstNonEmpty(endUse.EndUse, energyExplanationKindSuffix(endUse.Kind))), canonicalEnergyPathPart(endUse.Carrier))
		if !exists || len(groups[key]) == 0 {
			out.Edges = append(out.Edges, edge)
		}
	}
	// A contradictory observed component may accompany an observed zero meter.
	// Keep the accounting boundary instead of losing that case to the generic
	// planner's intentionally omitted 0/0 row.
	recordIndex := map[string]int{}
	for i, record := range out.Records {
		recordIndex[energyPathZoneHVACAllocationGroupKey(record.ServiceKind, record.Carrier)] = i
	}
	groupKeys := make([]string, 0, len(groups))
	for key := range groups {
		groupKeys = append(groupKeys, key)
	}
	sort.Strings(groupKeys)
	for _, key := range groupKeys {
		if _, exists := recordIndex[key]; !exists {
			parts := strings.Split(key, "\x00")
			record := energyPathZoneHVACAllocationRecord{Period: periodID, ServiceKind: parts[0], Carrier: parts[1], Unit: "kWh"}
			for _, node := range endUses[key] {
				record.ExpectedValue = roundedEnergyNumber(record.ExpectedValue + math.Abs(energyExplanationEffectiveNodeValue(node)))
				record.SourceIDs = appendUniqueStrings(record.SourceIDs, node.SourceIDs...)
				record.Unit = firstNonEmpty(node.Unit, record.Unit)
			}
			recordIndex[key] = len(out.Records)
			out.Records = append(out.Records, record)
		}
		record := &out.Records[recordIndex[key]]
		record.SourceIDs = append([]string(nil), record.SourceIDs...)
		record.AllocatedValue, record.UnassignedValue, record.OvermappedValue = 0, 0, 0
		record.UsedServicePath, record.UsedZoneLoad = false, false
		bound := groups[key]
		sort.SliceStable(bound, func(i, j int) bool { return bound[i].pool.ID < bound[j].pool.ID })
		valid := true
		seenPools, seenMembers, seenSources := map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, item := range bound {
			poolID := normalizePurposeToken(item.pool.ID)
			if !item.valid || poolID == "" || seenPools[poolID] || len(item.pool.Members) == 0 {
				valid = false
			}
			seenPools[poolID] = true
			for _, member := range item.pool.Members {
				identity := normalizePurposeToken(member.ObjectType) + "\x00" + normalizePurposeToken(member.ObjectName) + "\x00" + normalizePurposeToken(member.OutputName)
				memberID := normalizePurposeToken(member.ID)
				if memberID == "" || strings.TrimSpace(member.ObjectType) == "" || strings.TrimSpace(member.ObjectName) == "" || strings.TrimSpace(member.OutputName) == "" || seenMembers["id:"+memberID] || seenMembers["identity:"+identity] {
					valid = false
				}
				seenMembers["id:"+memberID], seenMembers["identity:"+identity] = true, true
				if member.Series.Carrier != "" && canonicalEnergyPathPart(member.Series.Carrier) != record.Carrier ||
					firstNonEmpty(member.Series.ServiceKind, member.Series.EndUse) != "" && energyCanonicalServiceKind(firstNonEmpty(member.Series.ServiceKind, member.Series.EndUse)) != record.ServiceKind ||
					strings.TrimSpace(member.ZoneName) != "" && !strings.EqualFold(strings.TrimSpace(member.Series.ZoneName), strings.TrimSpace(member.ZoneName)) ||
					member.Series.SourceKey != "" && normalizePurposeToken(member.Series.SourceKey) != normalizePurposeToken(member.ObjectName) ||
					member.Series.SourceName != "" && normalizePurposeToken(member.Series.SourceName) != normalizePurposeToken(member.OutputName) {
					valid = false
				}
				_, _, sources, observed := energyPathHVACConsumptionPeriodValue(member.Series, periodID, periodKind, canonicalMonthlyBasis)
				if !observed {
					continue
				}
				record.SourceIDs = appendUniqueStrings(record.SourceIDs, sources...)
				for _, source := range sources {
					if seenSources[source] || len(energyPathZoneHVACIntersectPaths(item.pool.MeterSourceIDs, []string{source})) > 0 {
						valid = false
					}
					seenSources[source] = true
				}
			}
		}
		// Aggregate one edge per broad end-use/Zone. The v2 projection uses
		// that pair as its identity; separate constituent edges would be deduped.
		merged := map[string]EnergyExplanationEdge{}
		if valid {
			for _, item := range bound {
				members := append([]energyPathHVACConsumptionMember(nil), item.pool.Members...)
				sort.SliceStable(members, func(i, j int) bool { return members[i].ID < members[j].ID })
				for _, member := range members {
					value, _, sources, observed := energyPathHVACConsumptionPeriodValue(member.Series, periodID, periodKind, canonicalMonthlyBasis)
					if !observed {
						continue
					}
					record.SourceIDs = appendUniqueStrings(record.SourceIDs, sources...)
					if strings.TrimSpace(member.ZoneName) != "" {
						continue // Direct observations, including known zero, are already reserved.
					}
					targets := energyPathHVACConsumptionTargets(nodes, topology, record.ServiceKind, member.RelatedPathIDs)
					weights := make([]energyPathZoneAuxiliaryTarget, len(targets))
					for i, target := range targets {
						weights[i] = energyPathZoneAuxiliaryTarget{NodeID: target.Node.ID, ZoneName: target.Node.ZoneName, Value: target.Value}
					}
					// Reuse the existing constituent-budget largest-remainder policy
					// so small pools cannot create a negative final share.
					shares := energyPathFanPoolShares(value, weights)
					for i, target := range targets {
						if i >= len(shares) {
							continue
						}
						out.ConsumptionSourceAllocations = append(out.ConsumptionSourceAllocations, energyPathHVACConsumptionSourceAllocation{
							Period: periodID, PoolID: item.pool.ID, MemberID: member.ID, ServiceKind: record.ServiceKind, Carrier: record.Carrier,
							ZoneName: target.Node.ZoneName, SourceIDs: appendUniqueStrings(nil, sources...), LoadSourceIDs: appendUniqueStrings(nil, target.SourceIDs...),
							RelatedPathIDs: appendUniqueStrings(nil, target.PathIDs...), ObservedValue: value, AllocatedValue: shares[i],
						})
						if shares[i] <= 0 {
							continue
						}
						id := edgeID("allocation_zone_hvac", periodID, item.node.ID, target.Node.ID)
						edge := merged[id]
						if edge.ID == "" {
							edge = EnergyExplanationEdge{ID: id, FromID: item.node.ID, ToID: target.Node.ID, Unit: firstNonEmpty(item.node.Unit, record.Unit), Period: periodID, Relation: "allocation", Basis: "service_path_allocation", RuleID: energyRelationshipRuleAllocatedHVACConsumptionPool, ZoneName: target.Node.ZoneName, ServiceKind: record.ServiceKind, Formula: "Sum of independently observed shared HVAC consumption constituents * each constituent's eligible zone service load / its exact recipient service load total; direct local consumption reserves only its own source, not the entire Zone; unidentified meter remainder is unassigned"}
						}
						edge.Value = roundedEnergyNumber(edge.Value + shares[i])
						edge.SourceIDs = appendUniqueStrings(edge.SourceIDs, item.node.SourceIDs...)
						edge.SourceIDs = appendUniqueStrings(edge.SourceIDs, sources...)
						edge.SourceIDs = appendUniqueStrings(edge.SourceIDs, target.SourceIDs...)
						edge.RelatedPathIDs = appendUniqueStrings(edge.RelatedPathIDs, target.PathIDs...)
						merged[id] = edge
						record.SourceIDs = appendUniqueStrings(record.SourceIDs, target.SourceIDs...)
					}
				}
			}
		}
		for _, edge := range merged {
			sort.Strings(edge.SourceIDs)
			sort.Strings(edge.RelatedPathIDs)
			out.Edges = append(out.Edges, edge)
			record.AllocatedValue = roundedEnergyNumber(record.AllocatedValue + edge.Value)
		}
		record.UsedServicePath = record.AllocatedValue > energyPathZoneHVACAllocationEpsilon
		residual := roundedEnergyNumber(record.ExpectedValue - record.DirectValue - record.AllocatedValue)
		if residual > energyPathZoneHVACAllocationEpsilon {
			record.UnassignedValue = residual
		} else if residual < -energyPathZoneHVACAllocationEpsilon {
			record.OvermappedValue = -residual
		}
		sort.Strings(record.SourceIDs)
	}
	sortEnergyExplanationEdges(out.Edges)
	return out
}

func aggregateEnergyPathHVACConsumptionSourceAllocations(rows []energyPathHVACConsumptionSourceAllocation, period string) []energyPathHVACConsumptionSourceAllocation {
	groups := map[string]energyPathHVACConsumptionSourceAllocation{}
	keys := []string{}
	for _, row := range rows {
		sourceIDs := appendUniqueStrings(nil, row.SourceIDs...)
		sort.Strings(sourceIDs)
		key := row.PoolID + "\x00" + row.MemberID + "\x00" + energyPathZoneHVACAllocationGroupKey(row.ServiceKind, row.Carrier) + "\x00" + energyPathZoneHVACZoneKey(row.ZoneName) + "\x00" + strings.Join(sourceIDs, "\x00")
		current, exists := groups[key]
		if !exists {
			current = row
			current.Period = period
			current.ObservedValue, current.AllocatedValue = 0, 0
			current.SourceIDs, current.LoadSourceIDs, current.RelatedPathIDs = nil, nil, nil
			keys = append(keys, key)
		}
		current.ObservedValue = roundedEnergyNumber(current.ObservedValue + row.ObservedValue)
		current.AllocatedValue = roundedEnergyNumber(current.AllocatedValue + row.AllocatedValue)
		current.SourceIDs = appendUniqueStrings(current.SourceIDs, row.SourceIDs...)
		current.LoadSourceIDs = appendUniqueStrings(current.LoadSourceIDs, row.LoadSourceIDs...)
		current.RelatedPathIDs = appendUniqueStrings(current.RelatedPathIDs, row.RelatedPathIDs...)
		groups[key] = current
	}
	sort.Strings(keys)
	var out []energyPathHVACConsumptionSourceAllocation
	for _, key := range keys {
		row := groups[key]
		sort.Strings(row.SourceIDs)
		sort.Strings(row.LoadSourceIDs)
		sort.Strings(row.RelatedPathIDs)
		out = append(out, row)
	}
	return out
}

// Missing, negative, nonfinite and sourceless observations are not measured
// zeros. Component quantities are already model totals; Zone multipliers are
// applied to the denominator's load only, never to these native observations.
func energyPathHVACConsumptionPeriodValue(series energyExplanationSeries, periodID, periodKind string, canonicalMonthlyBasis bool) (float64, float64, []string, bool) {
	if !strings.EqualFold(strings.TrimSpace(series.Unit), "kWh") {
		return 0, 0, nil, false
	}
	// Validate before the legacy helper rounds: a small negative quantity must
	// not become an apparently valid observed zero, or cancel in an annual sum.
	values := []float64{}
	switch strings.ToLower(strings.TrimSpace(periodKind)) {
	case "monthly", "daily", "hourly":
		kind := strings.ToLower(strings.TrimSpace(periodKind))
		prefix, periods := "M", series.Monthly
		if kind == "daily" {
			prefix, periods = "D", series.Daily
		} else if kind == "hourly" {
			prefix, periods = "H", series.Hourly
		}
		index, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(periodID)), prefix))
		value, present := periods[index]
		if err != nil || !present {
			return 0, 0, nil, false
		}
		values = append(values, value)
	case "selected_range":
		if !series.HasSelectedRange {
			return 0, 0, nil, false
		}
		values = append(values, series.SelectedRange)
	default:
		if canonicalMonthlyBasis && len(series.Monthly) > 0 {
			for _, value := range series.Monthly {
				values = append(values, value)
			}
		} else if len(series.AnnualSourceIDs) > 0 {
			values = append(values, series.Total)
		} else {
			return 0, 0, nil, false
		}
	}
	for _, value := range values {
		if !energyPathFinite(value) || value < 0 {
			return 0, 0, nil, false
		}
	}
	value, raw, sources, observed := energyPathDirectZonePeriodValue(series, periodID, periodKind, canonicalMonthlyBasis)
	if !observed || len(sources) != 1 || !energyPathFinite(value) || value < 0 {
		return 0, 0, nil, false
	}
	return value, raw, sources, true
}

func energyPathHVACConsumptionTargets(nodes []EnergyExplanationNode, topology energyServicePathIndex, service string, sourcePaths []string) []energyPathZoneHVACLoadTarget {
	if len(sourcePaths) == 0 {
		return nil
	}
	targets := []energyPathZoneHVACLoadTarget{}
	for _, original := range foldLegacyEnergyLoadDetailNodes(nodes) {
		node := energyExplanationLegacyNodeWithEffectiveValues(original)
		if !strings.EqualFold(node.Level, "load") || strings.TrimSpace(node.ZoneName) == "" || energyCanonicalServiceKind(firstNonEmpty(node.ServiceKind, energyExplanationKindSuffix(node.Kind))) != service {
			continue
		}
		value := energyExplanationEffectiveNodeValue(node)
		if !energyPathFinite(value) || value <= 0 {
			continue
		}
		zoneKey := normalizePurposeToken(node.ZoneName)
		known := appendUniqueStrings(nil, topology.byZoneService[zoneKey+"|"+service]...)
		known = appendUniqueStrings(known, topology.byZoneService[zoneKey+"|mixed"]...)
		if len(node.RelatedPathIDs) > 0 {
			known = energyPathZoneHVACIntersectPaths(known, node.RelatedPathIDs)
		}
		paths := energyPathZoneHVACIntersectPaths(known, sourcePaths)
		if len(paths) == 0 {
			continue
		}
		targets = append(targets, energyPathZoneHVACLoadTarget{Node: node, Value: value, PathIDs: paths, SourceIDs: appendUniqueStrings(nil, node.SourceIDs...)})
	}
	sort.SliceStable(targets, func(i, j int) bool {
		left, right := energyPathZoneHVACZoneKey(targets[i].Node.ZoneName), energyPathZoneHVACZoneKey(targets[j].Node.ZoneName)
		if left != right {
			return left < right
		}
		return targets[i].Node.ID < targets[j].Node.ID
	})
	return targets
}
