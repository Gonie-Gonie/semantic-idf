package simulation

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const energyPathZoneHVACAllocationEpsilon = 1e-9

// energyPathZoneHVACAllocationPlan is private adapter state. It lets a Zone
// graph project a central HVAC meter without changing the frozen v1 payload,
// while the Building result can report the exact allocation/remainder ledger.
type energyPathZoneHVACAllocationPlan struct {
	Edges                      []EnergyExplanationEdge
	Records                    []energyPathZoneHVACAllocationRecord
	CentralEndUseNodeIDs       map[string]bool
	AnnualAuthoritativeGroups  map[string]bool
	AnnualDirectOverrideGroups map[string]bool
}

type energyPathZoneHVACAllocationRecord struct {
	Period          string
	ServiceKind     string
	Carrier         string
	Unit            string
	ExpectedValue   float64
	DirectValue     float64
	AllocatedValue  float64
	UnassignedValue float64
	OvermappedValue float64
	UsedServicePath bool
	UsedZoneLoad    bool
	SourceIDs       []string
}

type energyPathZoneHVACDirectValue struct {
	ZoneName  string
	Present   bool
	Value     float64
	SourceIDs []string
}

type energyPathZoneHVACLoadTarget struct {
	Node      EnergyExplanationNode
	Value     float64
	PathIDs   []string
	SourceIDs []string
}

type energyPathZoneHVACPathEvidence struct {
	Present   bool
	PathIDs   []string
	SourceIDs []string
}

func buildEnergyPathZoneHVACAllocationPlan(nodes []EnergyExplanationNode, edges []EnergyExplanationEdge, directSeries []energyExplanationSeries, periodID string, periodKind string, canonicalMonthlyBasis bool) energyPathZoneHVACAllocationPlan {
	plan := energyPathZoneHVACAllocationPlan{CentralEndUseNodeIDs: map[string]bool{}}
	periodID = firstNonEmpty(strings.TrimSpace(periodID), "annual")

	type endUseGroup struct {
		service   string
		carrier   string
		unit      string
		expected  float64
		nodes     []EnergyExplanationNode
		pathIDs   []string
		sourceIDs []string
	}
	groups := map[string]*endUseGroup{}
	groupOrder := []string{}
	foldedNodes := foldLegacyEnergyLoadDetailNodes(nodes)
	nodeByID := make(map[string]EnergyExplanationNode, len(foldedNodes))
	loadsByService := map[string][]EnergyExplanationNode{}
	for _, original := range foldedNodes {
		node := energyExplanationLegacyNodeWithEffectiveValues(original)
		nodeByID[node.ID] = node
		if strings.EqualFold(strings.TrimSpace(node.Level), "load") && strings.TrimSpace(node.ZoneName) != "" {
			service := energyCanonicalServiceKind(firstNonEmpty(node.ServiceKind, energyExplanationKindSuffix(node.Kind)))
			if (service == "cooling" || service == "heating") && math.Abs(energyExplanationEffectiveNodeValue(node)) > 0 {
				loadsByService[service] = append(loadsByService[service], node)
			}
			continue
		}
		if strings.TrimSpace(node.ZoneName) != "" || legacyEnergyNodeIsCarrier(node) || legacyEnergyNodeIsSupport(node) {
			continue
		}
		level := strings.ToLower(strings.TrimSpace(node.Level))
		if level != "energy" && level != "end_use" {
			continue
		}
		service := canonicalEnergyPathEndUse(firstNonEmpty(node.EndUse, energyExplanationKindSuffix(node.Kind)))
		if service != "cooling" && service != "heating" {
			continue
		}
		carrier := canonicalEnergyPathPart(strings.TrimSpace(node.Carrier))
		if carrier == "" {
			continue
		}
		value := math.Abs(energyExplanationEffectiveNodeValue(node))
		if !energyPathFinite(value) {
			continue
		}
		key := energyPathZoneHVACAllocationGroupKey(service, carrier)
		group := groups[key]
		if group == nil {
			group = &endUseGroup{service: service, carrier: carrier, unit: node.Unit}
			groups[key] = group
			groupOrder = append(groupOrder, key)
		}
		group.expected = roundedEnergyNumber(group.expected + value)
		group.nodes = append(group.nodes, node)
		group.pathIDs = appendUniqueStrings(group.pathIDs, node.RelatedPathIDs...)
		group.sourceIDs = appendUniqueStrings(group.sourceIDs, node.SourceIDs...)
		plan.CentralEndUseNodeIDs[node.ID] = true
	}
	if len(groups) == 0 {
		return plan
	}
	for service := range loadsByService {
		sort.SliceStable(loadsByService[service], func(i, j int) bool {
			leftZone := strings.ToLower(strings.TrimSpace(loadsByService[service][i].ZoneName))
			rightZone := strings.ToLower(strings.TrimSpace(loadsByService[service][j].ZoneName))
			if leftZone != rightZone {
				return leftZone < rightZone
			}
			return loadsByService[service][i].ID < loadsByService[service][j].ID
		})
	}

	directByTarget := energyPathZoneHVACDirectValues(nodes, directSeries, periodID, periodKind, canonicalMonthlyBasis)
	groupEvidence := map[string]energyPathZoneHVACPathEvidence{}
	serviceEvidence := map[string]energyPathZoneHVACPathEvidence{}
	for _, edge := range edges {
		if !legacyEnergyLinkIsLoadToEndUse(edge) {
			continue
		}
		endUse, endUseOK := nodeByID[edge.FromID]
		load, loadOK := nodeByID[edge.ToID]
		if !endUseOK || !loadOK || strings.TrimSpace(endUse.ZoneName) != "" || strings.TrimSpace(load.ZoneName) == "" || !strings.EqualFold(load.Level, "load") {
			continue
		}
		service := canonicalEnergyPathEndUse(firstNonEmpty(endUse.EndUse, energyExplanationKindSuffix(endUse.Kind)))
		loadService := energyCanonicalServiceKind(firstNonEmpty(load.ServiceKind, energyExplanationKindSuffix(load.Kind), edge.ServiceKind))
		if (service != "cooling" && service != "heating") || loadService != service {
			continue
		}
		carrier := canonicalEnergyPathPart(strings.TrimSpace(endUse.Carrier))
		// Edge enrichment unions both endpoints, so edge.RelatedPathIDs alone
		// cannot prove that this target belongs to a source path. Keep target
		// paths separate; eligibility below uses the exact source/target set
		// intersection.
		paths := appendUniqueStrings(nil, load.RelatedPathIDs...)
		// Enriched legacy edges can contain both endpoint sources. Service-level
		// evidence may be reused for a sibling carrier, so strip the current
		// central meter here and add the carrier-qualified meter back only when
		// emitting that group's edge below.
		targetSources := energyPathZoneHVACWithoutSources(edge.SourceIDs, endUse.SourceIDs)
		targetSources = appendUniqueStrings(targetSources, load.SourceIDs...)
		groupSources := appendUniqueStrings(append([]string(nil), targetSources...), endUse.SourceIDs...)
		groupKey := energyPathZoneHVACAllocationGroupKey(service, carrier) + "\x00" + load.ID
		groupEvidence[groupKey] = mergeEnergyPathZoneHVACPathEvidence(groupEvidence[groupKey], paths, groupSources)
		serviceKey := service + "\x00" + load.ID
		// Cross-carrier reuse carries only target load/path evidence. The sibling
		// central meter is not a source for this carrier's allocation.
		serviceEvidence[serviceKey] = mergeEnergyPathZoneHVACPathEvidence(serviceEvidence[serviceKey], paths, targetSources)
	}

	sort.Strings(groupOrder)
	for _, groupKey := range groupOrder {
		group := groups[groupKey]
		sort.SliceStable(group.nodes, func(i, j int) bool { return group.nodes[i].ID < group.nodes[j].ID })
		sort.Strings(group.pathIDs)
		sort.Strings(group.sourceIDs)
		record := energyPathZoneHVACAllocationRecord{
			Period:        periodID,
			ServiceKind:   group.service,
			Carrier:       group.carrier,
			Unit:          group.unit,
			ExpectedValue: roundedEnergyNumber(group.expected),
			SourceIDs:     appendUniqueStrings(nil, group.sourceIDs...),
		}
		directZones := map[string]bool{}
		directPrefix := groupKey + "\x00"
		directKeys := make([]string, 0)
		for key := range directByTarget {
			if strings.HasPrefix(key, directPrefix) {
				directKeys = append(directKeys, key)
			}
		}
		sort.Strings(directKeys)
		for _, key := range directKeys {
			direct := directByTarget[key]
			if !direct.Present {
				continue
			}
			directZones[energyPathZoneHVACZoneKey(direct.ZoneName)] = true
			record.DirectValue = roundedEnergyNumber(record.DirectValue + math.Abs(direct.Value))
			record.SourceIDs = appendUniqueStrings(record.SourceIDs, direct.SourceIDs...)
		}
		// A reported zero Building end use with no direct zone observation has
		// nothing to allocate or reconcile. Keep the central node ID registered so
		// projection cannot fall back to a generic service share, but omit the
		// otherwise noisy balanced 0/0 ledger row.
		if math.Abs(record.ExpectedValue) <= energyPathZoneHVACAllocationEpsilon && math.Abs(record.DirectValue) <= energyPathZoneHVACAllocationEpsilon {
			continue
		}

		remaining := roundedEnergyNumber(group.expected - record.DirectValue)
		if remaining < 0 {
			remaining = 0
		}
		eligible := make([]energyPathZoneHVACLoadTarget, 0, len(loadsByService[group.service]))
		matched := make([]energyPathZoneHVACLoadTarget, 0, len(loadsByService[group.service]))
		for _, load := range loadsByService[group.service] {
			if directZones[energyPathZoneHVACZoneKey(load.ZoneName)] {
				continue
			}
			value := math.Abs(energyExplanationEffectiveNodeValue(load))
			if value <= 0 || !energyPathFinite(value) {
				continue
			}
			target := energyPathZoneHVACLoadTarget{
				Node:      load,
				Value:     value,
				PathIDs:   appendUniqueStrings(nil, load.RelatedPathIDs...),
				SourceIDs: appendUniqueStrings(nil, load.SourceIDs...),
			}
			eligible = append(eligible, target)

			evidence := groupEvidence[groupKey+"\x00"+load.ID]
			if !evidence.Present {
				// A sibling carrier can reuse source provenance for the same HVAC
				// service, but never its path eligibility: that remains the exact
				// source/target path intersection above.
				evidence = serviceEvidence[group.service+"\x00"+load.ID]
			}
			paths := []string(nil)
			if len(group.pathIDs) > 0 {
				// Explicit source paths are authoritative: unrelated target paths
				// cannot dilute this end use's denominator.
				paths = energyPathZoneHVACIntersectPaths(load.RelatedPathIDs, group.pathIDs)
			} else if evidence.Present {
				// Runtime broad end-use meters have no loop/path identity. Their
				// existing same-service load edges plus the target's ServiceModel
				// paths are the bounded relationship evidence. A sibling carrier
				// can reuse that same service evidence without another load edge.
				paths = appendUniqueStrings(nil, load.RelatedPathIDs...)
			}
			if len(paths) == 0 {
				continue
			}
			target.PathIDs = paths
			target.SourceIDs = appendUniqueStrings(target.SourceIDs, evidence.SourceIDs...)
			matched = append(matched, target)
		}

		targets := matched
		basis := "service_path_allocation"
		ruleID := energyRelationshipRuleAllocatedServicePathLoad
		explanation := "Allocated by HVAC service-path load share"
		if len(targets) == 0 {
			targets = eligible
			basis = "zone_load_allocation"
			ruleID = energyRelationshipRuleAllocatedZoneLoad
			explanation = "Allocated by zone service load share"
		}
		weights := make([]float64, len(targets))
		for index := range targets {
			weights[index] = targets[index].Value
		}
		targetShares := energyPathZoneHVACProportionalValues(remaining, weights)
		edgeContextSourceIDs := appendUniqueStrings(nil, record.SourceIDs...)
		nodeWeights := make([]float64, len(group.nodes))
		for index := range group.nodes {
			nodeWeights[index] = math.Abs(energyExplanationEffectiveNodeValue(group.nodes[index]))
		}
		nodePools := energyPathZoneHVACProportionalValues(remaining, nodeWeights)
		if remaining > energyPathZoneHVACAllocationEpsilon && len(targetShares) > 0 {
			if basis == "service_path_allocation" {
				record.UsedServicePath = true
			} else {
				record.UsedZoneLoad = true
			}
		}
		for nodeIndex, endUse := range group.nodes {
			if nodeIndex >= len(nodePools) || nodePools[nodeIndex] <= 0 {
				continue
			}
			nodeTargetValues := energyPathZoneHVACProportionalValues(nodePools[nodeIndex], weights)
			for targetIndex, target := range targets {
				if targetIndex >= len(nodeTargetValues) || nodeTargetValues[targetIndex] <= 0 {
					continue
				}
				value := nodeTargetValues[targetIndex]
				paths := appendUniqueStrings(nil, target.PathIDs...)
				sources := appendUniqueStrings(append([]string(nil), endUse.SourceIDs...), target.SourceIDs...)
				sources = appendUniqueStrings(sources, edgeContextSourceIDs...)
				sort.Strings(paths)
				sort.Strings(sources)
				plan.Edges = append(plan.Edges, EnergyExplanationEdge{
					ID:             edgeID("allocation_zone_hvac", periodID, endUse.ID, target.Node.ID),
					FromID:         endUse.ID,
					ToID:           target.Node.ID,
					Value:          value,
					Unit:           firstNonEmpty(endUse.Unit, group.unit),
					Period:         periodID,
					Relation:       "allocation",
					Basis:          basis,
					Formula:        explanation + "; (building HVAC end use - exact direct zone HVAC energy) * eligible zone service load / eligible service load total",
					RuleID:         ruleID,
					SourceIDs:      sources,
					ZoneName:       target.Node.ZoneName,
					ServiceKind:    group.service,
					RelatedPathIDs: paths,
				})
			}
		}
		for _, value := range targetShares {
			record.AllocatedValue = roundedEnergyNumber(record.AllocatedValue + value)
		}
		periodResidual := roundedEnergyNumber(record.ExpectedValue - record.DirectValue - record.AllocatedValue)
		if periodResidual > energyPathZoneHVACAllocationEpsilon {
			record.UnassignedValue = periodResidual
		} else if periodResidual < -energyPathZoneHVACAllocationEpsilon {
			record.OvermappedValue = math.Abs(periodResidual)
		}
		if len(targetShares) > 0 {
			for _, target := range targets {
				record.SourceIDs = appendUniqueStrings(record.SourceIDs, target.SourceIDs...)
			}
		}
		record.SourceIDs = appendUniqueStrings(nil, record.SourceIDs...)
		sort.Strings(record.SourceIDs)
		plan.Records = append(plan.Records, record)
	}
	sortEnergyExplanationEdges(plan.Edges)
	return plan
}

func energyPathZoneHVACDirectValues(nodes []EnergyExplanationNode, series []energyExplanationSeries, periodID string, periodKind string, canonicalMonthlyBasis bool) map[string]energyPathZoneHVACDirectValue {
	out := map[string]energyPathZoneHVACDirectValue{}
	add := func(zoneName string, service string, carrier string, value float64, present bool, sourceIDs []string) {
		zoneName = strings.TrimSpace(zoneName)
		service = energyCanonicalServiceKind(service)
		carrier = canonicalEnergyPathPart(carrier)
		if zoneName == "" || (service != "cooling" && service != "heating") || carrier == "" || !present || !energyPathFinite(value) {
			return
		}
		key := energyPathZoneHVACAllocationGroupKey(service, carrier) + "\x00" + energyPathZoneHVACZoneKey(zoneName)
		current := out[key]
		current.ZoneName = firstNonEmpty(current.ZoneName, zoneName)
		current.Present = true
		current.Value = roundedEnergyNumber(current.Value + math.Abs(value))
		current.SourceIDs = appendUniqueStrings(current.SourceIDs, sourceIDs...)
		sort.Strings(current.SourceIDs)
		out[key] = current
	}
	for _, original := range series {
		item := canonicalEnergyExplanationSeries(original)
		if item.Stage != "end_use" || strings.TrimSpace(item.ZoneName) == "" {
			continue
		}
		service := energyCanonicalServiceKind(firstNonEmpty(item.ServiceKind, item.EndUse, energyExplanationKindSuffix(item.Kind)))
		value, _, sourceIDs, ok := energyPathDirectZonePeriodValue(item, periodID, periodKind, canonicalMonthlyBasis)
		add(item.ZoneName, service, firstNonEmpty(item.Carrier, "other"), value, ok, sourceIDs)
	}
	for _, original := range nodes {
		node := energyExplanationLegacyNodeWithEffectiveValues(original)
		if !energyPathLegacyNodeIsDirectZoneEnergy(node) || legacyEnergyNodeIsCarrier(node) || legacyEnergyNodeIsSupport(node) {
			continue
		}
		service := energyCanonicalServiceKind(firstNonEmpty(node.ServiceKind, node.EndUse, energyExplanationKindSuffix(node.Kind)))
		add(node.ZoneName, service, firstNonEmpty(node.Carrier, "other"), energyExplanationEffectiveNodeValue(node), true, node.SourceIDs)
	}
	return out
}

func mergeEnergyPathZoneHVACPathEvidence(current energyPathZoneHVACPathEvidence, pathIDs []string, sourceIDs []string) energyPathZoneHVACPathEvidence {
	current.Present = true
	current.PathIDs = appendUniqueStrings(current.PathIDs, pathIDs...)
	current.SourceIDs = appendUniqueStrings(current.SourceIDs, sourceIDs...)
	sort.Strings(current.PathIDs)
	sort.Strings(current.SourceIDs)
	return current
}

func energyPathZoneHVACWithoutSources(input []string, excluded []string) []string {
	if len(input) == 0 {
		return nil
	}
	excludedSet := make(map[string]bool, len(excluded))
	for _, sourceID := range excluded {
		excludedSet[strings.TrimSpace(sourceID)] = true
	}
	out := make([]string, 0, len(input))
	for _, sourceID := range input {
		if !excludedSet[strings.TrimSpace(sourceID)] {
			out = appendUniqueStrings(out, sourceID)
		}
	}
	return out
}

func energyPathZoneHVACIntersectPaths(left []string, right []string) []string {
	wanted := map[string]bool{}
	for _, value := range right {
		if key := strings.ToLower(strings.TrimSpace(value)); key != "" {
			wanted[key] = true
		}
	}
	out := []string{}
	for _, value := range left {
		if wanted[strings.ToLower(strings.TrimSpace(value))] {
			out = appendUniqueStrings(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func energyPathZoneHVACProportionalValues(total float64, weights []float64) []float64 {
	if total <= 0 || len(weights) == 0 {
		return nil
	}
	weightTotal := 0.0
	lastPositive := -1
	for index, weight := range weights {
		if weight > 0 && energyPathFinite(weight) {
			weightTotal += weight
			lastPositive = index
		}
	}
	if weightTotal <= 0 || lastPositive < 0 {
		return nil
	}
	out := make([]float64, len(weights))
	allocated := 0.0
	for index, weight := range weights {
		if weight <= 0 || !energyPathFinite(weight) {
			continue
		}
		value := roundedEnergyNumber(total * weight / weightTotal)
		if index == lastPositive {
			value = roundedEnergyNumber(total - allocated)
		}
		if value < 0 && math.Abs(value) <= energyPathZoneHVACAllocationEpsilon {
			value = 0
		}
		out[index] = value
		allocated = roundedEnergyNumber(allocated + value)
	}
	return out
}

func energyPathZoneHVACAllocationGroupKey(service string, carrier string) string {
	return energyCanonicalServiceKind(service) + "\x00" + canonicalEnergyPathPart(carrier)
}

func energyPathZoneHVACZoneKey(zoneName string) string {
	return strings.ToLower(strings.TrimSpace(zoneName))
}

func applyEnergyPathZoneHVACAllocationPlan(edges []EnergyExplanationEdge, nodes []EnergyExplanationNode, plan energyPathZoneHVACAllocationPlan) []EnergyExplanationEdge {
	if len(plan.CentralEndUseNodeIDs) == 0 {
		return append([]EnergyExplanationEdge(nil), edges...)
	}
	nodeByID := make(map[string]EnergyExplanationNode, len(nodes))
	for _, node := range nodes {
		nodeByID[node.ID] = node
	}
	out := make([]EnergyExplanationEdge, 0, len(edges)+len(plan.Edges))
	for _, edge := range edges {
		if plan.CentralEndUseNodeIDs[edge.FromID] && legacyEnergyLinkIsLoadToEndUse(edge) {
			endUse := nodeByID[edge.FromID]
			load := nodeByID[edge.ToID]
			service := canonicalEnergyPathEndUse(firstNonEmpty(endUse.EndUse, energyExplanationKindSuffix(endUse.Kind)))
			loadService := energyCanonicalServiceKind(firstNonEmpty(load.ServiceKind, energyExplanationKindSuffix(load.Kind), edge.ServiceKind))
			if (service == "cooling" || service == "heating") && service == loadService {
				continue
			}
		}
		out = append(out, edge)
	}
	out = append(out, plan.Edges...)
	sortEnergyExplanationEdges(out)
	return out
}

func appendEnergyPathZoneHVACAllocationAccounting(result *EnergyExplanationResult, annual energyPathZoneHVACAllocationPlan, periodPlans map[string]energyPathZoneHVACAllocationPlan, canonicalMonthlyBasis bool) {
	if result == nil || result.Scope.Kind != "building" {
		return
	}
	annualRecords := annual.Records
	result.Reconciliation, result.Warnings = appendEnergyPathZoneHVACAllocationRecords(result.Reconciliation, result.Warnings, annualRecords, "annual")
	for index := range result.Periods {
		period := &result.Periods[index]
		records := periodPlans[strings.ToLower(strings.TrimSpace(period.ID))].Records
		if canonicalMonthlyBasis && (strings.EqualFold(period.Kind, "annual") || strings.EqualFold(period.ID, "annual")) {
			records = annualRecords
		}
		period.Reconciliation, period.Warnings = appendEnergyPathZoneHVACAllocationRecords(period.Reconciliation, period.Warnings, records, period.ID)
		if period.Summary != nil {
			summary := buildEnergyExplanationSummaryV2(EnergyExplanationResult{
				Schema:            result.Schema,
				Purpose:           result.Purpose,
				Scope:             result.Scope,
				Frequency:         result.Frequency,
				AllocationPolicy:  result.AllocationPolicy,
				Nodes:             period.Nodes,
				Links:             period.Links,
				Reconciliation:    period.Reconciliation,
				Completeness:      result.Completeness,
				Warnings:          period.Warnings,
				ZoneContributions: period.ZoneContributions,
			})
			summary.Period = period.ID
			summary.AllocationPolicy = result.AllocationPolicy
			period.Summary = &summary
		}
	}
}

// applyEnergyPathAnnualZoneHVACOverrides preserves exact annual-only direct
// evidence without fabricating a monthly profile. Monthly graphs remain
// period-local; the annual graph uses the direct-first annual plan for the
// affected service/carrier and is therefore intentionally not their sum.
func applyEnergyPathAnnualZoneHVACOverrides(result *EnergyExplanationResult, initialNodes []EnergyExplanationNode, initialLinks []EnergyPathLink, plan energyPathZoneHVACAllocationPlan) {
	if result == nil || result.Scope.Kind != "zone" || len(plan.AnnualAuthoritativeGroups) == 0 {
		return
	}
	affectedServices := map[string]bool{}
	directOverrideServices := map[string]bool{}
	affectedCarriers := map[string]bool{}
	for key := range plan.AnnualAuthoritativeGroups {
		parts := strings.Split(key, "\x00")
		if len(parts) != 2 {
			continue
		}
		affectedServices[parts[0]] = true
		affectedCarriers[parts[1]] = true
	}
	for key := range plan.AnnualDirectOverrideGroups {
		parts := strings.Split(key, "\x00")
		if len(parts) == 2 {
			directOverrideServices[parts[0]] = true
		}
	}
	shouldReplaceNode := func(node EnergyExplanationNode) bool {
		switch node.Level {
		case "load":
			return affectedServices[energyCanonicalServiceKind(node.ServiceKind)]
		case "end_use":
			return affectedServices[canonicalEnergyPathEndUse(node.EndUse)]
		case "carrier":
			return affectedCarriers[canonicalEnergyPathPart(firstNonEmpty(node.Carrier, energyExplanationKindSuffix(node.Kind)))]
		default:
			return false
		}
	}
	initialByID := map[string]EnergyExplanationNode{}
	affectedNodeIDs := map[string]bool{}
	for _, node := range initialNodes {
		if shouldReplaceNode(node) {
			initialByID[node.ID] = node
			affectedNodeIDs[node.ID] = true
		}
	}
	nodes := make([]EnergyExplanationNode, 0, len(result.Nodes)+len(initialByID))
	emitted := map[string]bool{}
	for _, node := range result.Nodes {
		if replacement, ok := initialByID[node.ID]; ok {
			nodes = append(nodes, replacement)
			emitted[node.ID] = true
			continue
		}
		if shouldReplaceNode(node) {
			continue
		}
		nodes = append(nodes, node)
	}
	for _, node := range initialNodes {
		if _, ok := initialByID[node.ID]; ok && !emitted[node.ID] {
			nodes = append(nodes, node)
			emitted[node.ID] = true
		}
	}
	links := make([]EnergyPathLink, 0, len(result.Links)+len(initialLinks))
	for _, link := range result.Links {
		if affectedNodeIDs[link.FromID] || affectedNodeIDs[link.ToID] {
			continue
		}
		links = append(links, link)
	}
	for _, link := range initialLinks {
		if affectedNodeIDs[link.FromID] || affectedNodeIDs[link.ToID] {
			if link.Relation == "load_to_end_use" && canonicalEnergyPathBasis(link.Basis, "") == "direct_zone_energy" && directOverrideServices[energyCanonicalServiceKind(link.ServiceKind)] {
				const note = "annual-only exact direct zone energy; monthly allocation retained without fabricating a direct profile; partial temporal coverage"
				if !strings.Contains(strings.ToLower(link.Explanation), "partial temporal coverage") {
					if strings.TrimSpace(link.Explanation) == "" {
						link.Explanation = note
					} else {
						link.Explanation = strings.TrimSpace(link.Explanation) + "; " + note
					}
				}
			}
			links = append(links, link)
		}
	}
	sortEnergyExplanationNodes(nodes)
	sort.SliceStable(links, func(i, j int) bool { return links[i].ID < links[j].ID })
	result.Nodes = nodes
	result.Links = links
	result.Reconciliation = reconcileEnergyPathCarrierTotals(result.Nodes, result.Links, result.Reconciliation, "annual")
	for index := range result.Periods {
		if !strings.EqualFold(result.Periods[index].Kind, "annual") && !strings.EqualFold(result.Periods[index].ID, "annual") {
			continue
		}
		result.Periods[index].Nodes = cloneEnergyExplanationNodes(nodes)
		result.Periods[index].Links = append([]EnergyPathLink(nil), links...)
		result.Periods[index].Reconciliation = append([]EnergyReconciliation(nil), result.Reconciliation...)
		summary := buildEnergyExplanationSummaryV2(EnergyExplanationResult{
			Schema:            result.Schema,
			Purpose:           result.Purpose,
			Scope:             result.Scope,
			Frequency:         result.Frequency,
			AllocationPolicy:  result.AllocationPolicy,
			Nodes:             nodes,
			Links:             links,
			Reconciliation:    result.Reconciliation,
			Completeness:      result.Completeness,
			Warnings:          result.Periods[index].Warnings,
			ZoneContributions: result.Periods[index].ZoneContributions,
		})
		summary.Period = result.Periods[index].ID
		summary.AllocationPolicy = result.AllocationPolicy
		result.Periods[index].Summary = &summary
	}
}

func aggregateEnergyPathZoneHVACAllocationRecords(input []energyPathZoneHVACAllocationRecord, period string) []energyPathZoneHVACAllocationRecord {
	byKey := map[string]*energyPathZoneHVACAllocationRecord{}
	keys := []string{}
	for _, record := range input {
		key := energyPathZoneHVACAllocationGroupKey(record.ServiceKind, record.Carrier)
		current := byKey[key]
		if current == nil {
			copy := record
			copy.Period = period
			copy.SourceIDs = appendUniqueStrings(nil, record.SourceIDs...)
			byKey[key] = &copy
			keys = append(keys, key)
			continue
		}
		current.ExpectedValue = roundedEnergyNumber(current.ExpectedValue + record.ExpectedValue)
		current.DirectValue = roundedEnergyNumber(current.DirectValue + record.DirectValue)
		current.AllocatedValue = roundedEnergyNumber(current.AllocatedValue + record.AllocatedValue)
		current.UnassignedValue = roundedEnergyNumber(current.UnassignedValue + record.UnassignedValue)
		current.OvermappedValue = roundedEnergyNumber(current.OvermappedValue + record.OvermappedValue)
		current.UsedServicePath = current.UsedServicePath || record.UsedServicePath
		current.UsedZoneLoad = current.UsedZoneLoad || record.UsedZoneLoad
		current.SourceIDs = appendUniqueStrings(current.SourceIDs, record.SourceIDs...)
	}
	sort.Strings(keys)
	out := make([]energyPathZoneHVACAllocationRecord, 0, len(keys))
	for _, key := range keys {
		record := *byKey[key]
		sort.Strings(record.SourceIDs)
		out = append(out, record)
	}
	return out
}

func aggregateEnergyPathZoneHVACAllocationPlans(input []energyPathZoneHVACAllocationPlan) energyPathZoneHVACAllocationPlan {
	out := energyPathZoneHVACAllocationPlan{CentralEndUseNodeIDs: map[string]bool{}}
	edgesByKey := map[string]*EnergyExplanationEdge{}
	edgeKeys := []string{}
	records := []energyPathZoneHVACAllocationRecord{}
	for _, plan := range input {
		for id := range plan.CentralEndUseNodeIDs {
			out.CentralEndUseNodeIDs[id] = true
		}
		for _, edge := range plan.Edges {
			key := energyExplanationEdgeAggregationKey(edge)
			current := edgesByKey[key]
			if current == nil {
				copy := edge
				copy.Period = "annual"
				copy.ID = energyExplanationAnnualEdgeID(copy)
				copy.Formula = strings.TrimSpace(copy.Formula + "; annual sum of monthly service path allocations")
				copy.SourceIDs = appendUniqueStrings(nil, edge.SourceIDs...)
				copy.RelatedPathIDs = appendUniqueStrings(nil, edge.RelatedPathIDs...)
				edgesByKey[key] = &copy
				edgeKeys = append(edgeKeys, key)
				continue
			}
			current.Value = roundedEnergyNumber(current.Value + edge.Value)
			current.SignedValue = roundedEnergyNumber(current.SignedValue + edge.SignedValue)
			current.DisplayValue = roundedEnergyNumber(current.DisplayValue + edge.DisplayValue)
			current.SourceIDs = appendUniqueStrings(current.SourceIDs, edge.SourceIDs...)
			current.RelatedPathIDs = appendUniqueStrings(current.RelatedPathIDs, edge.RelatedPathIDs...)
		}
		records = append(records, plan.Records...)
	}
	sort.Strings(edgeKeys)
	for _, key := range edgeKeys {
		edge := *edgesByKey[key]
		sort.Strings(edge.SourceIDs)
		sort.Strings(edge.RelatedPathIDs)
		out.Edges = append(out.Edges, edge)
	}
	out.Records = aggregateEnergyPathZoneHVACAllocationRecords(records, "annual")
	return out
}

// energyPathZoneHVACAllocationPlanWithAnnualFallback keeps an annual-only
// service/carrier when other HVAC groups have monthly evidence. A monthly
// group remains authoritative whenever it appears in at least one month.
func energyPathZoneHVACAllocationPlanWithAnnualFallback(monthly energyPathZoneHVACAllocationPlan, annual energyPathZoneHVACAllocationPlan, nodes []EnergyExplanationNode) energyPathZoneHVACAllocationPlan {
	out := energyPathZoneHVACAllocationPlan{
		CentralEndUseNodeIDs:       map[string]bool{},
		AnnualAuthoritativeGroups:  map[string]bool{},
		AnnualDirectOverrideGroups: map[string]bool{},
	}
	if out.CentralEndUseNodeIDs == nil {
		out.CentralEndUseNodeIDs = map[string]bool{}
	}
	for id := range monthly.CentralEndUseNodeIDs {
		out.CentralEndUseNodeIDs[id] = true
	}
	for id := range annual.CentralEndUseNodeIDs {
		out.CentralEndUseNodeIDs[id] = true
	}
	monthlyGroups := map[string]energyPathZoneHVACAllocationRecord{}
	for _, record := range monthly.Records {
		monthlyGroups[energyPathZoneHVACAllocationGroupKey(record.ServiceKind, record.Carrier)] = record
	}
	for _, record := range annual.Records {
		key := energyPathZoneHVACAllocationGroupKey(record.ServiceKind, record.Carrier)
		monthlyRecord, monthlyExists := monthlyGroups[key]
		directMismatch := math.Abs(record.DirectValue-monthlyRecord.DirectValue) > energyPathZoneHVACAllocationEpsilon
		if !monthlyExists || directMismatch {
			out.AnnualAuthoritativeGroups[key] = true
		}
		if directMismatch {
			out.AnnualDirectOverrideGroups[key] = true
		}
	}
	nodeGroup := map[string]string{}
	for _, node := range nodes {
		if !annual.CentralEndUseNodeIDs[node.ID] {
			continue
		}
		service := canonicalEnergyPathEndUse(firstNonEmpty(node.EndUse, energyExplanationKindSuffix(node.Kind)))
		carrier := canonicalEnergyPathPart(node.Carrier)
		nodeGroup[node.ID] = energyPathZoneHVACAllocationGroupKey(service, carrier)
	}
	for _, record := range monthly.Records {
		key := energyPathZoneHVACAllocationGroupKey(record.ServiceKind, record.Carrier)
		if !out.AnnualAuthoritativeGroups[key] {
			out.Records = append(out.Records, record)
		}
	}
	for _, record := range annual.Records {
		key := energyPathZoneHVACAllocationGroupKey(record.ServiceKind, record.Carrier)
		if out.AnnualAuthoritativeGroups[key] {
			out.Records = append(out.Records, record)
		}
	}
	for _, edge := range monthly.Edges {
		if !out.AnnualAuthoritativeGroups[nodeGroup[edge.FromID]] {
			out.Edges = append(out.Edges, edge)
		}
	}
	for _, edge := range annual.Edges {
		if out.AnnualAuthoritativeGroups[nodeGroup[edge.FromID]] {
			out.Edges = append(out.Edges, edge)
		}
	}
	sort.SliceStable(out.Records, func(i, j int) bool {
		return energyPathZoneHVACAllocationGroupKey(out.Records[i].ServiceKind, out.Records[i].Carrier) < energyPathZoneHVACAllocationGroupKey(out.Records[j].ServiceKind, out.Records[j].Carrier)
	})
	sortEnergyExplanationEdges(out.Edges)
	return out
}

func appendEnergyPathZoneHVACAllocationRecords(reconciliation []EnergyReconciliation, warnings []EnergyWarning, records []energyPathZoneHVACAllocationRecord, period string) ([]EnergyReconciliation, []EnergyWarning) {
	filteredRows := make([]EnergyReconciliation, 0, len(reconciliation)+len(records))
	for _, row := range reconciliation {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(row.ID)), "reconcile.zone_hvac_allocation.") {
			filteredRows = append(filteredRows, row)
		}
	}
	filteredWarnings := make([]EnergyWarning, 0, len(warnings)+len(records))
	for _, warning := range warnings {
		if warning.Code != "unassigned_building_hvac_energy" && warning.Code != "direct_zone_hvac_energy_exceeds_building" {
			filteredWarnings = append(filteredWarnings, warning)
		}
	}
	sorted := append([]energyPathZoneHVACAllocationRecord(nil), records...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left := energyPathZoneHVACAllocationGroupKey(sorted[i].ServiceKind, sorted[i].Carrier)
		right := energyPathZoneHVACAllocationGroupKey(sorted[j].ServiceKind, sorted[j].Carrier)
		return left < right
	})
	for _, record := range sorted {
		record.Period = firstNonEmpty(strings.TrimSpace(period), record.Period, "annual")
		explained := roundedEnergyNumber(record.DirectValue + record.AllocatedValue)
		residual := roundedEnergyNumber(record.ExpectedValue - explained)
		label := "Building HVAC energy allocation"
		status := "balanced"
		if residual > energyPathZoneHVACAllocationEpsilon {
			label = "Unassigned building HVAC energy"
			status = "partial"
		} else if residual < -energyPathZoneHVACAllocationEpsilon {
			label = "Direct zone HVAC energy exceeds building HVAC energy"
			status = "overmapped"
		} else if record.UnassignedValue > energyPathZoneHVACAllocationEpsilon || record.OvermappedValue > energyPathZoneHVACAllocationEpsilon {
			label = "Building HVAC allocation has period-level gaps or overlaps"
			status = "partial"
		}
		if record.UnassignedValue > energyPathZoneHVACAllocationEpsilon {
			filteredWarnings = appendEnergyDriverWarning(filteredWarnings, EnergyWarning{
				Severity: "warning",
				Code:     "unassigned_building_hvac_energy",
				Message:  fmt.Sprintf("Unassigned building HVAC energy remains for %s %s: %g %s could not be linked to an eligible non-direct zone.", energyServiceLabel(record.ServiceKind), energyCarrierLabel(record.Carrier), record.UnassignedValue, record.Unit),
				Period:   record.Period,
			})
		}
		if record.OvermappedValue > energyPathZoneHVACAllocationEpsilon {
			filteredWarnings = appendEnergyDriverWarning(filteredWarnings, EnergyWarning{
				Severity: "warning",
				Code:     "direct_zone_hvac_energy_exceeds_building",
				Message:  fmt.Sprintf("Exact direct zone %s %s energy exceeds the Building end-use meter by %g %s; direct observations are retained and no remainder is allocated.", energyServiceLabel(record.ServiceKind), energyCarrierLabel(record.Carrier), record.OvermappedValue, record.Unit),
				Period:   record.Period,
			})
		}
		formula := "building HVAC end use - exact direct zone HVAC energy - allocated non-direct zone HVAC energy"
		filteredRows = append(filteredRows, EnergyReconciliation{
			ID:              strings.Join([]string{"reconcile", "zone_hvac_allocation", canonicalEnergyPathPart(record.ServiceKind), canonicalEnergyPathPart(record.Carrier), canonicalEnergyPathPart(record.Period)}, "."),
			Level:           "allocation",
			Period:          record.Period,
			Label:           label,
			Status:          status,
			ServiceKind:     record.ServiceKind,
			ExpectedValue:   roundedEnergyNumber(record.ExpectedValue),
			ExplainedValue:  explained,
			ResidualValue:   residual,
			DirectValue:     roundedEnergyNumber(record.DirectValue),
			AllocatedValue:  roundedEnergyNumber(record.AllocatedValue),
			UnassignedValue: roundedEnergyNumber(record.UnassignedValue),
			OvermappedValue: roundedEnergyNumber(record.OvermappedValue),
			AllocationMethod: func() string {
				if record.UsedServicePath {
					return "service_path_load_share"
				}
				if record.UsedZoneLoad {
					return "zone_load_share"
				}
				if record.DirectValue > energyPathZoneHVACAllocationEpsilon && record.UnassignedValue <= energyPathZoneHVACAllocationEpsilon {
					return "direct_only"
				}
				return "unassigned"
			}(),
			Unit:      record.Unit,
			Basis:     "service_path_allocation",
			Formula:   formula,
			SourceIDs: appendUniqueStrings(nil, record.SourceIDs...),
		})
	}
	return filteredRows, filteredWarnings
}
