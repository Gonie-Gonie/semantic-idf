package simulation

import (
	"math"
	"slices"
	"sort"
	"strings"
)

// A service attribution is not another thermal outlet or equipment conversion.
// FromValue is the actual served thermal reference; ToValue is a share of an
// already accounted auxiliary site-energy budget. Neither quantity is COP.
const energyPathRelationLoadToAuxiliary = "load_to_auxiliary"

type energyPathAuxiliaryLinkBudget struct {
	owner          EnergyExplanationNode
	zone           string
	value          float64
	paths, sources []string
	explanation    string
}

func appendEnergyPathAuxiliaryServiceLinks(result *EnergyExplanationResult, input EnergyExplanationV1, annualPlan energyPathZoneAuxiliaryAllocationPlan, periodPlans map[string]energyPathZoneAuxiliaryAllocationPlan) {
	if result == nil {
		return
	}
	legacyPeriods := map[string]EnergyPeriod{}
	for _, period := range input.Periods {
		legacyPeriods[strings.ToLower(strings.TrimSpace(period.ID))] = period
	}
	monthly := []EnergyPeriod{}
	for i := range result.Periods {
		period := &result.Periods[i]
		if strings.EqualFold(period.Kind, "annual") || strings.EqualFold(period.ID, "annual") {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(period.ID))
		legacy := legacyPeriods[key]
		attributions := buildEnergyPathAuxiliaryServiceLinks(period.Nodes, legacy.Nodes, result.Sources, periodPlans[key], input.servicePathIndex, input.auxiliaryFanPools, result.Scope, period.ID)
		period.Links = append(period.Links, attributions...)
		if strings.EqualFold(period.Kind, "monthly") {
			// The physical graph already has its annual aggregation. Only sum
			// these new references, rather than copying that graph a second time.
			monthly = append(monthly, EnergyPeriod{ID: period.ID, Kind: period.Kind, Links: attributions})
		}
	}
	if len(monthly) > 0 {
		// Never recompute an annual service split from annual loads: operation
		// and auxiliary budgets can have different seasonal distributions.
		_, links, _, _ := aggregateEnergyPathV2MonthlyPeriods(monthly)
		for _, link := range links {
			if link.Relation == energyPathRelationLoadToAuxiliary {
				result.Links = append(result.Links, link)
			}
		}
	} else {
		result.Links = append(result.Links, buildEnergyPathAuxiliaryServiceLinks(result.Nodes, input.Nodes, result.Sources, annualPlan, input.servicePathIndex, input.auxiliaryFanPools, result.Scope, "annual")...)
	}
	result.Links = filterEnergyPathAuxiliaryServiceLinks(result.Nodes, result.Links, result.Sources, result.Scope, "annual")
	for i := range result.Periods {
		period := &result.Periods[i]
		if !strings.EqualFold(period.Kind, "annual") && !strings.EqualFold(period.ID, "annual") {
			continue
		}
		for _, link := range result.Links {
			if link.Relation == energyPathRelationLoadToAuxiliary {
				period.Links = append(period.Links, link)
			}
		}
		period.Links = filterEnergyPathAuxiliaryServiceLinks(period.Nodes, period.Links, result.Sources, result.Scope, period.ID)
	}
}

func buildEnergyPathAuxiliaryServiceLinks(nodes, original []EnergyExplanationNode, sources []EnergyDataSource, plan energyPathZoneAuxiliaryAllocationPlan, topology energyServicePathIndex, pools []energyPathFanPool, scope EnergyExplanationScope, period string) []EnergyPathLink {
	legacy := foldLegacyEnergyLoadDetailNodes(original)
	byID := map[string]EnergyExplanationNode{}
	for _, node := range legacy {
		if _, exists := byID[node.ID]; exists {
			return nil
		}
		byID[node.ID] = node
	}
	// Conflicting path identities cannot establish a service or a recipient.
	paths := map[string]energyPathAuxiliaryServicePath{}
	for _, path := range topology.auxiliaryPaths {
		key := strings.ToLower(strings.TrimSpace(path.ID))
		if prior, exists := paths[key]; exists && prior != path {
			return nil
		}
		paths[key] = path
	}
	budgets := []energyPathAuxiliaryLinkBudget{}
	for _, edge := range plan.Edges {
		owner, ownerOK := byID[edge.FromID]
		target, targetOK := byID[edge.ToID]
		endUse := canonicalEnergyPathEndUse(firstNonEmpty(owner.EndUse, energyExplanationKindSuffix(owner.Kind)))
		if !ownerOK || !targetOK || !energyPathAuxiliaryEndUse(endUse) || target.Level != "load" || target.ZoneName == "" ||
			edge.Relation != energyPathAuxiliaryAllocationRelation || edge.RuleID != energyRelationshipRuleAllocatedAuxiliaryServicePath ||
			canonicalEnergyPathBasis(edge.Basis, edge.RuleID) != "service_path_allocation" || !energyPathPositiveFinite(edge.Value) ||
			len(edge.RelatedPathIDs) == 0 || !strings.EqualFold(edge.ZoneName, target.ZoneName) ||
			scope.Kind == "zone" && !strings.EqualFold(scope.ZoneName, target.ZoneName) {
			continue
		}
		budget := energyPathAuxiliaryLinkBudget{owner: owner, zone: target.ZoneName, value: edge.Value,
			paths:   appendUniqueStrings(appendUniqueStrings(nil, edge.RelatedPathIDs...), owner.RelatedPathIDs...),
			sources: appendUniqueStrings(nil, edge.SourceIDs...), explanation: edge.Formula}
		if endUse == "fans" {
			// The old Zone allocation retains only observed load paths. Recover
			// the same proven AirLoop's other service paths before checking that
			// an absent heating/cooling observation was not silently read as zero.
			loops := map[string]bool{}
			for _, path := range topology.auxiliaryPaths {
				if strings.EqualFold(path.ZoneName, budget.zone) && slices.Contains(budget.paths, path.ID) && strings.TrimSpace(path.AirLoopName) != "" {
					loops[normalizePurposeToken(path.AirLoopName)] = true
				}
			}
			for _, path := range topology.auxiliaryPaths {
				if strings.EqualFold(path.ZoneName, budget.zone) && loops[normalizePurposeToken(path.AirLoopName)] {
					budget.paths = appendUniqueStrings(budget.paths, path.ID)
				}
			}
		}
		if endUse == "fans" && len(plan.FanSourceAllocations) > 0 {
			// A merged Zone edge may combine fans from different AirLoops. Keep
			// each measured pool's paths and share before splitting services.
			budgets = append(budgets, energyPathAuxiliaryFanLinkBudgets(budget, plan.FanSourceAllocations, pools, topology)...)
		} else if endUse == "pumps" && len(plan.PoolPumpSourceAllocations) > 0 {
			budgets = append(budgets, energyPathAuxiliaryPumpLinkBudgets(budget, plan.PoolPumpSourceAllocations)...)
		} else {
			budgets = append(budgets, budget)
		}
	}
	// Direct observations need their own original path proof. A Zone identity,
	// ventilation fan aggregate, or another component's topology is insufficient.
	for _, node := range nodes {
		endUse := canonicalEnergyPathEndUse(firstNonEmpty(node.EndUse, energyExplanationKindSuffix(node.Kind)))
		if !energyPathAuxiliaryEndUse(endUse) || !node.nativeWindowACPathQualified || !energyPathLegacyNodeIsDirectZoneEnergy(node) || legacyEnergyNodeIsCarrier(node) ||
			len(node.RelatedPathIDs) == 0 || scope.Kind == "zone" && !strings.EqualFold(scope.ZoneName, node.ZoneName) ||
			energyPathNodeHasOnlySimpleVentilationSources(node, sources, scope) {
			continue
		}
		budgets = append(budgets, energyPathAuxiliaryLinkBudget{owner: node, zone: node.ZoneName,
			value: math.Abs(energyExplanationEffectiveNodeValue(energyExplanationLegacyNodeWithEffectiveValues(node))),
			paths: appendUniqueStrings(nil, node.RelatedPathIDs...), sources: appendUniqueStrings(nil, node.SourceIDs...),
			explanation: "Exact Zone auxiliary site observation with original service-path ownership"})
	}
	type attribution struct {
		link  EnergyPathLink
		loads map[string]bool
	}
	groups := map[string]*attribution{}
	for _, budget := range budgets {
		endUse := canonicalEnergyPathEndUse(firstNonEmpty(budget.owner.EndUse, energyExplanationKindSuffix(budget.owner.Kind)))
		eligible := energyPathZoneAuxiliaryEligiblePaths(endUse, budget.paths, topology.auxiliaryPaths)
		if endUse == "fans" && budget.owner.nativeWindowACPathQualified && energyPathLegacyNodeIsDirectZoneEnergy(budget.owner) {
			// The original WindowAC qualifier proves the exact native fan and
			// single direct Zone cooling path; it deliberately has no AirLoop.
			for _, path := range topology.auxiliaryPaths {
				if strings.EqualFold(path.ZoneName, budget.zone) && path.ServiceKind == "cooling" &&
					path.AirLoopName == "" && path.PlantLoopName == "" && path.CondenserLoopName == "" && slices.Contains(budget.paths, path.ID) {
					eligible[strings.ToLower(strings.TrimSpace(path.ID))] = path
				}
			}
		}
		targets, complete := energyPathAuxiliaryServiceTargets(budget.zone, legacy, eligible)
		if !complete || !energyPathPositiveFinite(budget.value) {
			continue
		}
		shares := energyPathFanPoolShares(budget.value, targets)
		for i, target := range targets {
			if i >= len(shares) || shares[i] <= 0 {
				continue
			}
			load := byID[target.NodeID]
			from, to := upgradeEnergyExplanationNode(load, scope), upgradeEnergyExplanationNode(budget.owner, scope)
			link := EnergyPathLink{FromID: from.ID, ToID: to.ID, Relation: energyPathRelationLoadToAuxiliary,
				Basis: "service_path_allocation", RuleID: energyRelationshipRuleAllocatedAuxiliaryServicePath,
				FromUnit: from.Unit, ToUnit: to.Unit, Period: period, ZoneName: scope.ZoneName, ServiceKind: target.ServiceKind}
			key := energyPathV2LinkAggregationKey(link)
			group := groups[key]
			if group == nil {
				group = &attribution{link: link, loads: map[string]bool{}}
				groups[key] = group
			}
			if !group.loads[load.ID] {
				group.link.FromValue = roundedEnergyNumber(group.link.FromValue + target.Value)
				group.loads[load.ID] = true
			}
			group.link.ToValue = roundedEnergyNumber(group.link.ToValue + shares[i])
			group.link.SourceIDs = appendUniqueStrings(group.link.SourceIDs, budget.sources...)
			group.link.SourceIDs = appendUniqueStrings(group.link.SourceIDs, target.SourceIDs...)
			group.link.RelatedPathIDs = appendUniqueStrings(group.link.RelatedPathIDs, target.PathIDs...)
			group.link.Explanation = "Auxiliary service attribution, not thermal conversion: existing Zone auxiliary site budget * matching service load / complete related cooling + heating load; thermal reference is counted once; no COP or additional thermal outlet. " + budget.explanation
		}
	}
	out := make([]EnergyPathLink, 0, len(groups))
	for _, group := range groups {
		link := group.link
		sort.Strings(link.SourceIDs)
		sort.Strings(link.RelatedPathIDs)
		link.ID = energyPathLinkID(link)
		out = append(out, link)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return filterEnergyPathAuxiliaryServiceLinks(nodes, out, sources, scope, period)
}

func energyPathAuxiliaryEndUse(endUse string) bool {
	return endUse == "fans" || endUse == "pumps" || endUse == "heat_rejection"
}

func energyPathAuxiliaryServiceTargets(zone string, nodes []EnergyExplanationNode, eligible map[string]energyPathAuxiliaryServicePath) ([]energyPathZoneAuxiliaryTarget, bool) {
	required := map[string]bool{}
	for _, path := range eligible {
		service := energyPathAuxiliaryCanonicalServiceKind(path.ServiceKind)
		if strings.EqualFold(path.ZoneName, zone) && (service == "cooling" || service == "heating") {
			required[service] = true
		}
	}
	if len(required) == 0 {
		return nil, false
	}
	observed, seen := map[string]bool{}, map[string]bool{}
	out := []energyPathZoneAuxiliaryTarget{}
	for _, original := range nodes {
		node := energyExplanationLegacyNodeWithEffectiveValues(original)
		service := energyCanonicalServiceKind(firstNonEmpty(node.ServiceKind, energyExplanationKindSuffix(node.Kind)))
		if node.Level != "load" || !strings.EqualFold(node.ZoneName, zone) || !required[service] || seen[node.ID] {
			continue
		}
		matching := []string{}
		for _, path := range eligible {
			if strings.EqualFold(path.ZoneName, zone) && energyPathAuxiliaryCanonicalServiceKind(path.ServiceKind) == service &&
				len(energyPathZoneHVACIntersectPaths(node.RelatedPathIDs, []string{path.ID})) > 0 {
				matching = appendUniqueStrings(matching, path.ID)
			}
		}
		value := energyExplanationEffectiveNodeValue(node)
		if len(matching) == 0 || !energyPathFinite(value) || value < 0 || len(node.SourceIDs) == 0 {
			continue
		}
		seen[node.ID], observed[service] = true, true
		if value > 0 {
			out = append(out, energyPathZoneAuxiliaryTarget{NodeID: node.ID, ZoneName: zone, ServiceKind: service,
				Value: value, PathIDs: matching, SourceIDs: appendUniqueStrings(nil, node.SourceIDs...)})
		}
	}
	for service := range required {
		if !observed[service] {
			return nil, false // An absent opposite-service observation is not zero.
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out, len(out) > 0
}

func energyPathAuxiliaryFanLinkBudgets(budget energyPathAuxiliaryLinkBudget, rows []energyPathFanSourceAllocation, pools []energyPathFanPool, topology energyServicePathIndex) []energyPathAuxiliaryLinkBudget {
	valid := buildEnergyPathFanConsumptionSources(pools)
	byID := map[string]energyPathFanPool{}
	poolIDs := map[string]bool{}
	for _, pool := range pools {
		byID[pool.Source.ID], poolIDs[pool.Source.ID] = pool, true
	}
	out, total := []energyPathAuxiliaryLinkBudget{}, 0.0
	seen := map[string]bool{}
	for _, row := range rows {
		if !strings.EqualFold(row.ZoneName, budget.zone) || !valid[row.SourceID] || !slices.Contains(budget.sources, row.SourceID) {
			continue
		}
		if seen[row.SourceID] {
			return nil
		}
		seen[row.SourceID] = true
		paths := []string{}
		for _, path := range topology.auxiliaryPaths {
			if strings.EqualFold(path.ZoneName, budget.zone) && strings.EqualFold(path.AirLoopName, byID[row.SourceID].AirLoopName) &&
				len(energyPathZoneHVACIntersectPaths(budget.paths, []string{path.ID})) > 0 {
				paths = appendUniqueStrings(paths, path.ID)
			}
		}
		if len(paths) == 0 || !energyPathFinite(row.Value) || row.Value < 0 {
			return nil
		}
		sources := []string{row.SourceID}
		for _, id := range budget.sources {
			if !poolIDs[id] {
				sources = appendUniqueStrings(sources, id)
			}
		}
		out = append(out, energyPathAuxiliaryLinkBudget{owner: budget.owner, zone: budget.zone, value: row.Value, paths: paths, sources: sources, explanation: budget.explanation})
		total = roundedEnergyNumber(total + row.Value)
	}
	if math.Abs(total-budget.value) > energyPathZoneHVACAllocationEpsilon {
		return nil
	}
	return out
}

func energyPathAuxiliaryPumpLinkBudgets(budget energyPathAuxiliaryLinkBudget, rows []energyPathPoolPumpSourceAllocation) []energyPathAuxiliaryLinkBudget {
	out, total := []energyPathAuxiliaryLinkBudget{}, 0.0
	seen := map[string]bool{}
	for _, row := range rows {
		if !strings.EqualFold(row.ZoneName, budget.zone) || !slices.Contains(budget.sources, row.SourceID) ||
			len(energyPathZoneHVACIntersectPaths(budget.paths, row.RelatedPathIDs)) == 0 {
			continue
		}
		key := row.SourceID + "\x00" + row.ServiceKind
		if seen[key] {
			return nil
		}
		seen[key] = true
		if !energyPathFinite(row.Value) || row.Value < 0 {
			return nil
		}
		sources := appendUniqueStrings(appendUniqueStrings(nil, budget.owner.SourceIDs...), row.LoadSourceIDs...)
		sources = appendUniqueStrings(sources, row.SourceID)
		out = append(out, energyPathAuxiliaryLinkBudget{owner: budget.owner, zone: budget.zone, value: row.Value,
			paths: appendUniqueStrings(nil, row.RelatedPathIDs...), sources: sources, explanation: budget.explanation})
		total = roundedEnergyNumber(total + row.Value)
	}
	if math.Abs(total-budget.value) > energyPathZoneHVACAllocationEpsilon {
		return nil
	}
	return out
}

// Stored v2 links retain their source-qualified attribution, but cannot acquire
// conversions, foreign endpoints, another scope/period, or extra site budgets.
func filterEnergyPathAuxiliaryServiceLinks(nodes []EnergyExplanationNode, links []EnergyPathLink, sources []EnergyDataSource, scope EnergyExplanationScope, period string) []EnergyPathLink {
	byID, knownSources := map[string]EnergyExplanationNode{}, map[string]bool{}
	sourceCounts, nodeCounts, linkCounts := map[string]int{}, map[string]int{}, map[string]int{}
	for _, node := range nodes {
		byID[node.ID] = node
		nodeCounts[node.ID]++
	}
	for _, source := range sources {
		sourceCounts[source.ID]++
	}
	for _, link := range links {
		if link.Relation == energyPathRelationLoadToAuxiliary {
			linkCounts[link.ID]++
		}
	}
	for _, source := range sources {
		knownSources[source.ID] = source.ID != "" && sourceCounts[source.ID] == 1
	}
	valid := make([]bool, len(links))
	totals, seen := map[string]float64{}, map[string]bool{}
	for i, link := range links {
		if link.Relation != energyPathRelationLoadToAuxiliary {
			valid[i] = true
			continue
		}
		from, fromOK := byID[link.FromID]
		to, toOK := byID[link.ToID]
		service := energyCanonicalServiceKind(firstNonEmpty(from.ServiceKind, energyExplanationKindSuffix(from.Kind)))
		if !fromOK || !toOK || from.Level != "load" || from.ScaleDomain != "thermal" || to.Level != "end_use" || to.ScaleDomain != "site" ||
			nodeCounts[link.FromID] != 1 || nodeCounts[link.ToID] != 1 ||
			!energyPathAuxiliaryEndUse(canonicalEnergyPathEndUse(to.EndUse)) || (service != "cooling" && service != "heating") || link.ServiceKind != service ||
			link.Basis != "service_path_allocation" || link.RuleID != energyRelationshipRuleAllocatedAuxiliaryServicePath ||
			link.Ratio != 0 || link.RatioKind != "" || link.RatioLabel != "" || !energyPathPositiveFinite(link.FromValue) || !energyPathPositiveFinite(link.ToValue) ||
			energyPathConversionUnitBase(link.FromUnit) != "kwh" || energyPathConversionUnitBase(link.ToUnit) != "kwh" ||
			energyPathConversionUnitBase(from.Unit) != "kwh" || energyPathConversionUnitBase(to.Unit) != "kwh" ||
			!energyPathFinite(energyExplanationEffectiveNodeValue(from)) || energyExplanationEffectiveNodeValue(from) < 0 ||
			!energyPathFinite(energyExplanationEffectiveNodeValue(to)) || energyExplanationEffectiveNodeValue(to) < 0 ||
			len(link.SourceIDs) == 0 || len(link.RelatedPathIDs) == 0 || !strings.EqualFold(link.Period, period) ||
			!strings.EqualFold(link.ZoneName, scope.ZoneName) || !strings.EqualFold(from.ZoneName, scope.ZoneName) || !strings.EqualFold(to.ZoneName, scope.ZoneName) ||
			link.FromValue-math.Abs(energyExplanationEffectiveNodeValue(from)) > energyPathZoneHVACAllocationEpsilon ||
			!energyPathAuxiliarySourceOverlap(link.SourceIDs, from.SourceIDs) || !energyPathAuxiliarySourceOverlap(link.SourceIDs, to.SourceIDs) ||
			link.ID == "" || linkCounts[link.ID] != 1 || seen[link.ID] {
			continue
		}
		allKnown := true
		for _, id := range link.SourceIDs {
			allKnown = allKnown && knownSources[id]
		}
		for _, id := range link.RelatedPathIDs {
			allKnown = allKnown && len(energyPathZoneHVACIntersectPaths(from.RelatedPathIDs, []string{id})) > 0
			if len(to.RelatedPathIDs) > 0 {
				allKnown = allKnown && len(energyPathZoneHVACIntersectPaths(to.RelatedPathIDs, []string{id})) > 0
			}
		}
		if !allKnown {
			continue
		}
		valid[i], seen[link.ID] = true, true
		totals[link.ToID] = roundedEnergyNumber(totals[link.ToID] + link.ToValue)
	}
	out := make([]EnergyPathLink, 0, len(links))
	for i, link := range links {
		if !valid[i] {
			continue
		}
		if link.Relation == energyPathRelationLoadToAuxiliary && totals[link.ToID]-math.Abs(energyExplanationEffectiveNodeValue(byID[link.ToID])) > energyPathZoneHVACAllocationEpsilon {
			continue
		}
		out = append(out, link)
	}
	return out
}

func energyPathAuxiliarySourceOverlap(left, right []string) bool {
	for _, id := range left {
		if slices.Contains(right, id) {
			return true
		}
	}
	return false
}
