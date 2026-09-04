package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// UpgradeEnergyExplanationV1 is the single compatibility boundary for stored
// energy-explanation/v1 payloads. The returned graph always follows the v2
// driver -> load -> end-use -> carrier direction and never needs a v1 renderer.
func UpgradeEnergyExplanationV1(input EnergyExplanationV1) EnergyExplanationResult {
	scope := normalizeEnergyExplanationScope(input.scope)
	allocationPolicy := normalizePurposeAllocationPolicy(input.AllocationPolicy)
	annotatedSources := annotateLegacyEnergyDriverSources(input.Sources, input.Nodes)
	allLegacyNodes := append([]EnergyExplanationNode(nil), input.Nodes...)
	for _, period := range input.Periods {
		allLegacyNodes = append(allLegacyNodes, period.Nodes...)
	}
	annotatedSources = annotateLegacyEnergyLoadDetailSources(annotatedSources, allLegacyNodes)
	legacyNodes := foldLegacyEnergyLoadDetailNodes(inferLegacyEnergyDriverProjectionGuards(input.Nodes, annotatedSources))
	nodes, links := upgradeEnergyExplanationGraph(legacyNodes, input.Edges, annotatedSources, scope, allocationPolicy, input.canonicalMonthlyBasis)
	reconciliation, warnings := upgradeEnergyExplanationAccounting(input.Reconciliation, input.Warnings, legacyNodes, input.Edges, scope, allocationPolicy)
	reconciliation = reconcileEnergyPathCarrierTotals(nodes, links, reconciliation, "annual")
	periods := make([]EnergyPeriod, 0, len(input.Periods))
	for _, period := range input.Periods {
		legacyPeriodNodes := foldLegacyEnergyLoadDetailNodes(inferLegacyEnergyDriverProjectionGuards(period.Nodes, annotatedSources))
		periodNodes, periodLinks := upgradeEnergyExplanationGraph(legacyPeriodNodes, period.Edges, annotatedSources, scope, allocationPolicy, input.canonicalMonthlyBasis)
		periodReconciliation, periodWarnings := upgradeEnergyExplanationAccounting(period.Reconciliation, period.Warnings, legacyPeriodNodes, period.Edges, scope, allocationPolicy)
		periodReconciliation = reconcileEnergyPathCarrierTotals(periodNodes, periodLinks, periodReconciliation, period.ID)
		periodZoneContributions := buildEnergyExplanationZoneContributions(legacyPeriodNodes, period.Edges, scope, allocationPolicy)
		upgradedPeriod := EnergyPeriod{
			ID:                period.ID,
			Label:             period.Label,
			Kind:              period.Kind,
			Nodes:             periodNodes,
			Links:             periodLinks,
			Reconciliation:    periodReconciliation,
			Warnings:          periodWarnings,
			ZoneContributions: periodZoneContributions,
		}
		periodSummary := buildEnergyExplanationSummaryV2(EnergyExplanationResult{
			Schema:            energyExplanationSchema,
			Purpose:           input.Purpose,
			Scope:             scope,
			Frequency:         input.Frequency,
			AllocationPolicy:  allocationPolicy,
			Nodes:             periodNodes,
			Links:             periodLinks,
			Reconciliation:    periodReconciliation,
			Completeness:      input.Completeness,
			Warnings:          periodWarnings,
			ZoneContributions: periodZoneContributions,
		})
		periodSummary.Period = period.ID
		periodSummary.AllocationPolicy = allocationPolicy
		upgradedPeriod.Summary = &periodSummary
		periods = append(periods, upgradedPeriod)
	}
	sources := filterEnergyDataSourcesForV2(annotatedSources, legacyNodes, input.Edges, nodes, links, reconciliation, scope, allocationPolicy)
	availableZones := energyExplanationAvailableZones(input)
	result := EnergyExplanationResult{
		Schema:            energyExplanationSchema,
		Purpose:           input.Purpose,
		Scope:             scope,
		Frequency:         input.Frequency,
		AllocationPolicy:  allocationPolicy,
		RelationshipRules: upgradeEnergyRelationshipRules(input.RelationshipRules),
		Periods:           periods,
		Nodes:             nodes,
		Links:             links,
		Reconciliation:    reconciliation,
		Sources:           sources,
		Completeness:      input.Completeness,
		Warnings:          warnings,
		ZoneContributions: buildEnergyExplanationZoneContributions(legacyNodes, input.Edges, scope, allocationPolicy),
		AvailableZones:    availableZones,
		legacyNodes:       scopedEnergyExplanationLegacyNodes(legacyNodes, input.Edges, scope, allocationPolicy),
	}
	if input.canonicalMonthlyBasis {
		applyCanonicalMonthlyBasisToEnergyPathResult(&result)
	}
	if scope.Kind == "building" {
		result.ZoneResults = make([]EnergyExplanationZoneResult, 0, len(availableZones))
		for _, zoneName := range availableZones {
			zoneInput := input
			zoneInput.scope = EnergyExplanationScope{Kind: "zone", ZoneName: zoneName, AggregationBasis: scope.AggregationBasis}
			zone := UpgradeEnergyExplanationV1(zoneInput)
			zoneSummary := buildEnergyExplanationSummary(zone)
			result.ZoneResults = append(result.ZoneResults, EnergyExplanationZoneResult{
				Scope:             zone.Scope,
				Summary:           zoneSummary,
				Completeness:      zone.Completeness,
				Periods:           energyPathInteractivePeriods(zone.Periods),
				Nodes:             zone.Nodes,
				Links:             zone.Links,
				Reconciliation:    zone.Reconciliation,
				Warnings:          zone.Warnings,
				ZoneContributions: zone.ZoneContributions,
			})
			appendEnergyDataSourceScopeDetails(result.Sources, zone.Sources, zone.Scope)
		}
	}
	return result
}

func applyCanonicalMonthlyBasisToEnergyPathResult(result *EnergyExplanationResult) {
	if result == nil {
		return
	}
	monthly := make([]EnergyPeriod, 0, 12)
	for _, period := range result.Periods {
		if strings.EqualFold(period.Kind, "monthly") {
			monthly = append(monthly, period)
		}
	}
	if len(monthly) == 0 {
		return
	}
	initialNodes := append([]EnergyExplanationNode(nil), result.Nodes...)
	initialLinks := append([]EnergyPathLink(nil), result.Links...)
	initialReconciliation := append([]EnergyReconciliation(nil), result.Reconciliation...)
	nodes, links, reconciliation, warnings := aggregateEnergyPathV2MonthlyPeriods(monthly)

	nodeIndex := make(map[string]int, len(nodes)+len(initialNodes))
	for index, node := range nodes {
		nodeIndex[node.ID] = index
	}
	fallbackNodeIDs := map[string]bool{}
	fallbackSourceIDs := map[string]bool{}
	for _, node := range initialNodes {
		if _, exists := nodeIndex[node.ID]; exists || node.Level != "carrier" && node.Level != "end_use" {
			continue
		}
		node.Period = "annual"
		fallbackNodeIDs[node.ID] = true
		for _, sourceID := range node.SourceIDs {
			fallbackSourceIDs[sourceID] = true
		}
		nodeIndex[node.ID] = len(nodes)
		nodes = append(nodes, node)
	}
	for _, node := range initialNodes {
		if _, exists := nodeIndex[node.ID]; exists || node.Level != "residual" || !energyExplanationSourcesIntersect(node.SourceIDs, fallbackSourceIDs) {
			continue
		}
		node.Period = "annual"
		nodeIndex[node.ID] = len(nodes)
		nodes = append(nodes, node)
	}

	linkIndex := make(map[string]int, len(links)+len(initialLinks))
	for index, link := range links {
		linkIndex[energyPathV2LinkAggregationKey(link)] = index
	}
	for _, link := range initialLinks {
		key := energyPathV2LinkAggregationKey(link)
		if _, exists := linkIndex[key]; exists {
			continue
		}
		if !fallbackNodeIDs[link.FromID] && !fallbackNodeIDs[link.ToID] && !energyExplanationSourcesIntersect(link.SourceIDs, fallbackSourceIDs) {
			continue
		}
		if _, ok := nodeIndex[link.FromID]; !ok {
			continue
		}
		if _, ok := nodeIndex[link.ToID]; !ok {
			continue
		}
		link.Period = "annual"
		link.ID = energyPathLinkID(link)
		linkIndex[key] = len(links)
		links = append(links, link)
	}

	reconciliationIndex := make(map[string]int, len(reconciliation)+len(initialReconciliation))
	for index, item := range reconciliation {
		reconciliationIndex[energyExplanationReconciliationAggregationKey(item)] = index
	}
	for _, item := range initialReconciliation {
		key := energyExplanationReconciliationAggregationKey(item)
		if _, exists := reconciliationIndex[key]; exists || !energyExplanationSourcesIntersect(item.SourceIDs, fallbackSourceIDs) {
			continue
		}
		item.ID = energyExplanationAnnualReconciliationID(item.ID)
		item.Period = "annual"
		reconciliationIndex[key] = len(reconciliation)
		reconciliation = append(reconciliation, item)
	}
	for _, warning := range result.Warnings {
		warning.Period = "annual"
		warnings = appendEnergyDriverWarning(warnings, warning)
	}

	sortEnergyExplanationNodes(nodes)
	sort.SliceStable(links, func(i, j int) bool { return links[i].ID < links[j].ID })
	result.Nodes = nodes
	result.Links = links
	result.Reconciliation = reconciliation
	result.Warnings = warnings
	for index := range result.Periods {
		if !strings.EqualFold(result.Periods[index].Kind, "annual") && !strings.EqualFold(result.Periods[index].ID, "annual") {
			continue
		}
		result.Periods[index].Nodes = cloneEnergyExplanationNodes(nodes)
		result.Periods[index].Links = append([]EnergyPathLink(nil), links...)
		result.Periods[index].Reconciliation = append([]EnergyReconciliation(nil), reconciliation...)
		result.Periods[index].Warnings = append([]EnergyWarning(nil), warnings...)
		summary := buildEnergyExplanationSummaryV2(EnergyExplanationResult{
			Schema:            result.Schema,
			Purpose:           result.Purpose,
			Scope:             result.Scope,
			Frequency:         result.Frequency,
			AllocationPolicy:  result.AllocationPolicy,
			Nodes:             nodes,
			Links:             links,
			Reconciliation:    reconciliation,
			Completeness:      result.Completeness,
			Warnings:          warnings,
			ZoneContributions: result.Periods[index].ZoneContributions,
		})
		summary.Period = "annual"
		result.Periods[index].Summary = &summary
	}
}

func aggregateEnergyPathV2MonthlyPeriods(periods []EnergyPeriod) ([]EnergyExplanationNode, []EnergyPathLink, []EnergyReconciliation, []EnergyWarning) {
	nodes := []EnergyExplanationNode{}
	links := []EnergyPathLink{}
	reconciliation := []EnergyReconciliation{}
	warnings := []EnergyWarning{}
	nodeIndex := map[string]int{}
	linkIndex := map[string]int{}
	reconciliationIndex := map[string]int{}
	for _, period := range periods {
		for _, node := range period.Nodes {
			index, exists := nodeIndex[node.ID]
			if !exists {
				node.Period = "annual"
				node.LoadBreakdown = cloneEnergyExplanationLoadComponents(node.LoadBreakdown)
				node.OffsetEffects = cloneEnergyExplanationOffsetEffects(node.OffsetEffects)
				node.SimultaneousLoad = cloneEnergyExplanationSimultaneousLoad(node.SimultaneousLoad)
				node.allocationSourceIDs = appendUniqueStrings(nil, node.allocationSourceIDs...)
				node.simultaneousLoadContributions = cloneEnergyExplanationSimultaneousLoadContributions(node.simultaneousLoadContributions)
				node.endUseCarriers = appendUniqueStrings(nil, node.endUseCarriers...)
				nodeIndex[node.ID] = len(nodes)
				nodes = append(nodes, node)
				continue
			}
			mergeEnergyExplanationV2Node(map[string]*EnergyExplanationNode{node.ID: &nodes[index]}, node)
			nodes[index].Period = "annual"
		}
		for _, link := range period.Links {
			key := energyPathV2LinkAggregationKey(link)
			index, exists := linkIndex[key]
			if !exists {
				link.Period = "annual"
				link.ID = energyPathLinkID(link)
				linkIndex[key] = len(links)
				links = append(links, link)
				continue
			}
			current := &links[index]
			current.FromValue = roundedEnergyNumber(current.FromValue + link.FromValue)
			current.ToValue = roundedEnergyNumber(current.ToValue + link.ToValue)
			current.SourceIDs = appendUniqueStrings(current.SourceIDs, link.SourceIDs...)
			current.RelatedPathIDs = appendUniqueStrings(current.RelatedPathIDs, link.RelatedPathIDs...)
			finalizeEnergyPathLinkRatio(current)
		}
		for _, item := range period.Reconciliation {
			key := energyExplanationReconciliationAggregationKey(item)
			index, exists := reconciliationIndex[key]
			if !exists {
				item.ID = energyExplanationAnnualReconciliationID(item.ID)
				item.Period = "annual"
				reconciliationIndex[key] = len(reconciliation)
				reconciliation = append(reconciliation, item)
				continue
			}
			current := &reconciliation[index]
			current.ExpectedValue = roundedEnergyNumber(current.ExpectedValue + item.ExpectedValue)
			current.ExplainedValue = roundedEnergyNumber(current.ExplainedValue + item.ExplainedValue)
			current.ResidualValue = roundedEnergyNumber(current.ResidualValue + item.ResidualValue)
			current.Status = energyReconciliationStatus(current.ExpectedValue, current.ResidualValue)
			current.SourceIDs = appendUniqueStrings(current.SourceIDs, item.SourceIDs...)
		}
		for _, warning := range period.Warnings {
			warning.Period = "annual"
			warnings = appendEnergyDriverWarning(warnings, warning)
		}
	}
	canonicalNodes := make(map[string]*EnergyExplanationNode, len(nodes))
	for index := range nodes {
		canonicalNodes[nodes[index].ID] = &nodes[index]
	}
	synchronizeEnergyExplanationSimultaneousLoads(canonicalNodes)
	return nodes, links, reconciliation, warnings
}

func cloneEnergyExplanationNodes(input []EnergyExplanationNode) []EnergyExplanationNode {
	if len(input) == 0 {
		return nil
	}
	out := make([]EnergyExplanationNode, len(input))
	for index, node := range input {
		out[index] = node
		out[index].LoadBreakdown = cloneEnergyExplanationLoadComponents(node.LoadBreakdown)
		out[index].OffsetEffects = cloneEnergyExplanationOffsetEffects(node.OffsetEffects)
		out[index].SimultaneousLoad = cloneEnergyExplanationSimultaneousLoad(node.SimultaneousLoad)
		out[index].Badges = appendUniqueStrings(nil, node.Badges...)
		out[index].RelatedPathIDs = appendUniqueStrings(nil, node.RelatedPathIDs...)
		out[index].RelatedEntityIDs = appendUniqueStrings(nil, node.RelatedEntityIDs...)
		out[index].SourceIDs = appendUniqueStrings(nil, node.SourceIDs...)
		out[index].allocationSourceIDs = appendUniqueStrings(nil, node.allocationSourceIDs...)
		out[index].simultaneousLoadContributions = cloneEnergyExplanationSimultaneousLoadContributions(node.simultaneousLoadContributions)
		out[index].endUseCarriers = appendUniqueStrings(nil, node.endUseCarriers...)
	}
	return out
}

func energyPathV2LinkAggregationKey(link EnergyPathLink) string {
	return strings.Join([]string{
		link.FromID,
		link.ToID,
		normalizeEnergyOutputName(link.Relation),
		normalizeEnergyOutputName(link.RuleID),
		normalizeEnergyOutputName(link.ServiceKind),
		normalizeEnergyOutputName(link.ZoneName),
		normalizeEnergyOutputName(link.Basis),
	}, "|")
}

func energyExplanationAvailableZones(input EnergyExplanationV1) []string {
	byName := map[string]string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, exists := byName[key]; !exists {
			byName[key] = value
		}
	}
	for _, node := range input.Nodes {
		add(node.ZoneName)
	}
	for _, edge := range input.Edges {
		add(edge.ZoneName)
	}
	for _, item := range input.Reconciliation {
		add(item.ZoneName)
	}
	for _, period := range input.Periods {
		for _, node := range period.Nodes {
			add(node.ZoneName)
		}
		for _, edge := range period.Edges {
			add(edge.ZoneName)
		}
		for _, item := range period.Reconciliation {
			add(item.ZoneName)
		}
	}
	add(input.scope.ZoneName)
	out := make([]string, 0, len(byName))
	for _, value := range byName {
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

func energyPathInteractivePeriods(input []EnergyPeriod) []EnergyPeriod {
	out := make([]EnergyPeriod, 0, len(input))
	for _, period := range input {
		kind := strings.ToLower(strings.TrimSpace(period.Kind))
		id := strings.ToUpper(strings.TrimSpace(period.ID))
		if kind == "annual" || kind == "monthly" || strings.EqualFold(period.ID, "annual") || strings.HasPrefix(id, "M") {
			out = append(out, period)
		}
	}
	return out
}

func normalizeEnergyExplanationScope(scope EnergyExplanationScope) EnergyExplanationScope {
	kind := strings.ToLower(strings.TrimSpace(scope.Kind))
	zoneName := strings.TrimSpace(scope.ZoneName)
	if kind != "zone" || zoneName == "" {
		kind = "building"
		zoneName = ""
	}
	return EnergyExplanationScope{
		Kind:             kind,
		ZoneName:         zoneName,
		AggregationBasis: firstNonEmpty(strings.TrimSpace(scope.AggregationBasis), "model_total"),
	}
}

// Stored v1 payloads may contain humidification/dehumidification as historical
// load nodes. V2 keeps those reports as inspector detail without adding their
// values to the authoritative Cooling/Heating load a second time.
func foldLegacyEnergyLoadDetailNodes(input []EnergyExplanationNode) []EnergyExplanationNode {
	detailSources := map[string][]string{}
	for _, node := range input {
		service, ok := energyLoadHumidityDetailService(node.ServiceKind)
		if !ok || !strings.EqualFold(node.Level, "load") {
			continue
		}
		key := normalizeEnergySurfaceKey(node.ZoneName) + "|" + service
		detailSources[key] = appendUniqueStrings(detailSources[key], node.SourceIDs...)
	}
	out := make([]EnergyExplanationNode, 0, len(input))
	for _, node := range input {
		if _, detail := energyLoadHumidityDetailService(node.ServiceKind); detail && strings.EqualFold(node.Level, "load") {
			continue
		}
		service, primary := energyLoadCanonicalSelectionService(node.ServiceKind)
		if primary && strings.EqualFold(node.Level, "load") {
			key := normalizeEnergySurfaceKey(node.ZoneName) + "|" + service
			node.SourceIDs = appendUniqueStrings(node.SourceIDs, detailSources[key]...)
		}
		out = append(out, node)
	}
	return out
}

func annotateLegacyEnergyLoadDetailSources(input []EnergyDataSource, nodes []EnergyExplanationNode) []EnergyDataSource {
	type detailMetadata struct {
		service   string
		component string
	}
	metadata := map[string]detailMetadata{}
	for _, node := range nodes {
		service, ok := energyLoadHumidityDetailService(node.ServiceKind)
		if !ok || !strings.EqualFold(node.Level, "load") {
			continue
		}
		component := "load." + strings.ToLower(strings.TrimSpace(node.ServiceKind))
		for _, sourceID := range node.SourceIDs {
			metadata[sourceID] = detailMetadata{service: service, component: component}
		}
	}
	out := append([]EnergyDataSource(nil), input...)
	for index := range out {
		detail, ok := metadata[out[index].ID]
		if !ok {
			continue
		}
		out[index].DriverRole = energyDriverSourceRoleContext
		out[index].DriverCategory = "load." + detail.service
		out[index].DriverComponent = detail.component
		out[index].HeatDirection = detail.service
		out[index].InspectorSection = energyDriverInspectorSectionBreakdown
		out[index].Explanation = "Humidity delivery is retained as latent load breakdown and is not a separate primary load."
	}
	return out
}

func energyExplanationScopeForPlan(plan *PurposeRunPlan) EnergyExplanationScope {
	if plan != nil && len(plan.ZoneNames) == 1 && strings.EqualFold(strings.TrimSpace(plan.ZoneMode), "selected") {
		return normalizeEnergyExplanationScope(EnergyExplanationScope{
			Kind:             "zone",
			ZoneName:         plan.ZoneNames[0],
			AggregationBasis: "model_total",
		})
	}
	return normalizeEnergyExplanationScope(EnergyExplanationScope{})
}

func energyExplanationScopeToken(scope EnergyExplanationScope) string {
	if scope.Kind == "zone" && scope.ZoneName != "" {
		return firstNonEmpty(metricID(scope.ZoneName), "zone")
	}
	return "building"
}

func upgradeEnergyExplanationGraph(legacyNodes []EnergyExplanationNode, legacyEdges []EnergyExplanationEdge, sources []EnergyDataSource, scope EnergyExplanationScope, allocationPolicy string, canonicalMonthlyBasis bool) ([]EnergyExplanationNode, []EnergyPathLink) {
	scope = normalizeEnergyExplanationScope(scope)
	suppressedInterzone := energyExplanationSuppressedInterzoneTraces(legacyNodes, scope)
	zoneAllocationFactors := energyExplanationZoneAllocationFactors(legacyNodes, legacyEdges, scope, allocationPolicy)
	nodeByLegacyID := make(map[string]EnergyExplanationNode, len(legacyNodes))
	canonicalIDByLegacyID := make(map[string]string, len(legacyNodes))
	canonicalNodes := map[string]*EnergyExplanationNode{}
	scopedLegacyNodes := make([]EnergyExplanationNode, 0, len(legacyNodes))
	for _, original := range legacyNodes {
		legacy, include := energyExplanationNodeForScope(original, scope, zoneAllocationFactors)
		if !include {
			continue
		}
		scopedLegacyNodes = append(scopedLegacyNodes, legacy)
	}
	scopedLegacyNodes = selectEnergyDriverMainFlowNodes(scopedLegacyNodes, sources)
	driverPresentation := buildEnergyDriverPresentationPlan(scopedLegacyNodes, scope)
	for _, legacy := range scopedLegacyNodes {
		legacy = driverPresentation.apply(legacy)
		if energyDriverNodeUsesCanonicalTaxonomy(legacy) && legacy.AllocationApplied && legacy.AllocatedValue == 0 {
			// Explicitly allocated-zero drivers remain in Sources for inspection,
			// but must not merge their raw/effective values or provenance into a
			// visible canonical driver that shares the same presentation category.
			continue
		}
		canonical := upgradeEnergyExplanationNode(legacy, scope)
		if canonical.ID == "" {
			continue
		}
		if canonical.Level == "end_use" && energyExplanationEffectiveNodeValue(canonical) == 0 {
			// A zero carrier-qualified end use is not a contributor. Excluding it
			// before idMap/source merging prevents its meter from claiming a
			// non-zero canonical Other (or any other shared taxonomy node).
			continue
		}
		nodeByLegacyID[legacy.ID] = legacy
		canonicalIDByLegacyID[legacy.ID] = canonical.ID
		mergeEnergyExplanationV2Node(canonicalNodes, canonical)
	}

	loadTotalsByEndUse := map[string]float64{}
	endUsesWithLoads := map[string]bool{}
	for _, edge := range legacyEdges {
		if !legacyEnergyLinkIsLoadToEndUse(edge) {
			continue
		}
		if _, ok := canonicalIDByLegacyID[edge.ToID]; !ok {
			continue
		}
		loadNode := nodeByLegacyID[edge.ToID]
		endUseID := canonicalIDByLegacyID[edge.FromID]
		loadTotalsByEndUse[endUseID] += math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(loadNode), edge.Value))
		endUsesWithLoads[endUseID] = true
	}

	links := map[string]*EnergyPathLink{}
	for _, edge := range legacyEdges {
		link, ok := upgradeEnergyExplanationLink(edge, nodeByLegacyID, canonicalIDByLegacyID, canonicalNodes, loadTotalsByEndUse, endUsesWithLoads, canonicalMonthlyBasis)
		if !ok {
			continue
		}
		mergeEnergyPathLink(links, link)
	}
	if len(suppressedInterzone) > 0 {
		closeBuildingLoadsAfterInterzoneProjection(canonicalNodes, links, suppressedInterzone, scope)
	}
	synchronizeEnergyExplanationSimultaneousLoads(canonicalNodes)

	outNodes := make([]EnergyExplanationNode, 0, len(canonicalNodes))
	visibleNodeIDs := make(map[string]bool, len(canonicalNodes))
	for _, node := range canonicalNodes {
		if node.Level == "driver" && energyExplanationEffectiveNodeValue(*node) == 0 {
			continue
		}
		if node.Level == "end_use" && energyExplanationEffectiveNodeValue(*node) == 0 {
			continue
		}
		if node.Level == "end_use" {
			sort.Strings(node.SourceIDs)
		}
		outNodes = append(outNodes, *node)
		visibleNodeIDs[node.ID] = true
	}
	sortEnergyExplanationNodes(outNodes)
	outLinks := make([]EnergyPathLink, 0, len(links))
	for _, link := range links {
		if !visibleNodeIDs[link.FromID] || !visibleNodeIDs[link.ToID] {
			continue
		}
		if link.Relation == "end_use_to_carrier" || link.Relation == "direct_end_use_to_carrier" {
			sort.Strings(link.SourceIDs)
		}
		finalizeEnergyPathLinkRatio(link)
		outLinks = append(outLinks, *link)
	}
	sort.SliceStable(outLinks, func(i, j int) bool { return outLinks[i].ID < outLinks[j].ID })
	return outNodes, outLinks
}

type energyExplanationSuppressedTrace struct {
	sourceIDs        []string
	relatedEntityIDs []string
	suppressedValue  float64
	retainedValue    float64
}

func energyExplanationSuppressedInterzoneTraces(nodes []EnergyExplanationNode, scope EnergyExplanationScope) map[string]energyExplanationSuppressedTrace {
	if scope.Kind != "building" {
		return nil
	}
	traces := map[string]energyExplanationSuppressedTrace{}
	for _, node := range nodes {
		if !energyDriverNodeUsesCanonicalTaxonomy(node) {
			continue
		}
		service := energyCanonicalServiceKind(node.ServiceKind)
		if service != "cooling" && service != "heating" {
			if node.SignedValue < 0 {
				service = "heating"
			} else {
				service = "cooling"
			}
		}
		trace := traces[service]
		value := math.Abs(energyExplanationEffectiveNodeValue(energyExplanationLegacyNodeWithEffectiveValues(node)))
		switch {
		case node.driverZoneOnly:
			trace.suppressedValue += value
			trace.sourceIDs = appendUniqueStrings(trace.sourceIDs, node.SourceIDs...)
			trace.relatedEntityIDs = appendUniqueStrings(trace.relatedEntityIDs, node.RelatedEntityIDs...)
		case node.driverBuildingOnly || node.Kind == "heat.interzone_building_residual":
			trace.retainedValue += value
		}
		traces[service] = trace
	}
	for service, trace := range traces {
		if trace.suppressedValue <= 1e-9 {
			delete(traces, service)
		}
	}
	return traces
}

func closeBuildingLoadsAfterInterzoneProjection(nodes map[string]*EnergyExplanationNode, links map[string]*EnergyPathLink, traces map[string]energyExplanationSuppressedTrace, scope EnergyExplanationScope) {
	if scope.Kind != "building" {
		return
	}
	for _, service := range []string{"cooling", "heating"} {
		trace, ok := traces[service]
		if !ok {
			continue
		}
		for nodeID, node := range nodes {
			if node.Level == "residual" && node.Kind == "heat.residual" && energyCanonicalServiceKind(node.ServiceKind) == service {
				delete(nodes, nodeID)
				for linkID, link := range links {
					if link.FromID == nodeID || link.ToID == nodeID {
						delete(links, linkID)
					}
				}
			}
		}
		loadID := "load." + service + ".building"
		load := nodes[loadID]
		if load == nil || load.Value <= 0 {
			continue
		}
		foldedValue := roundedEnergyNumber(trace.suppressedValue - trace.retainedValue)
		if foldedValue <= 1e-9 {
			continue
		}
		storageID := "driver." + canonicalEnergyPathCategory(energyDriverCategoryStorageOther) + "." + service + ".building"
		storage := nodes[storageID]
		if storage == nil {
			storage = &EnergyExplanationNode{
				ID:                    storageID,
				Level:                 "driver",
				Kind:                  "driver." + energyDriverCategoryStorageOther,
				Label:                 energyDriverCategoryLabel(energyDriverCategoryStorageOther),
				Unit:                  firstNonEmpty(load.Unit, "kWh"),
				ScaleDomain:           "thermal",
				Period:                load.Period,
				ServiceKind:           service,
				DriverCategory:        energyDriverCategoryStorageOther,
				ThermalComponent:      "combined",
				Basis:                 "heat_balance_share",
				AllocationApplied:     true,
				AllocationExplanation: energyDriverAllocationExplanation,
				AggregationBasis:      "model_total",
				Multiplier:            1,
				RelatedEntityIDs:      appendUniqueStrings(nil, trace.relatedEntityIDs...),
				SourceIDs:             appendUniqueStrings(trace.sourceIDs, load.SourceIDs...),
			}
			nodes[storageID] = storage
		}
		storage.Value = roundedEnergyNumber(storage.Value + foldedValue)
		storage.AllocationApplied = true
		storage.AllocationExplanation = energyDriverAllocationExplanation
		storage.RawValue = roundedEnergyNumber(storage.RawValue + foldedValue)
		storage.EffectiveValue = roundedEnergyNumber(storage.EffectiveValue + foldedValue)
		storage.AllocatedValue = roundedEnergyNumber(storage.AllocatedValue + foldedValue)
		storage.DisplayValue = roundedEnergyNumber(storage.DisplayValue + foldedValue)
		if service == "heating" {
			storage.SignedValue = roundedEnergyNumber(storage.SignedValue - foldedValue)
		} else {
			storage.SignedValue = roundedEnergyNumber(storage.SignedValue + foldedValue)
		}
		storage.RelatedEntityIDs = appendUniqueStrings(storage.RelatedEntityIDs, trace.relatedEntityIDs...)
		storage.SourceIDs = appendUniqueStrings(storage.SourceIDs, appendUniqueStrings(trace.sourceIDs, load.SourceIDs...)...)

		link := EnergyPathLink{
			FromID:      storageID,
			ToID:        loadID,
			Relation:    "driver_to_load",
			Basis:       "heat_balance_share",
			Explanation: energyDriverAllocationExplanation,
			RuleID:      energyRelationshipRuleHeatDriverBalance,
			FromValue:   foldedValue,
			FromUnit:    storage.Unit,
			ToValue:     foldedValue,
			ToUnit:      load.Unit,
			Period:      load.Period,
			ServiceKind: service,
			SourceIDs:   appendUniqueStrings(trace.sourceIDs, load.SourceIDs...),
		}
		link.ID = energyPathLinkID(link)
		mergeEnergyPathLink(links, link)
		if merged := links[link.ID]; merged != nil {
			merged.FromValue = storage.Value
			merged.ToValue = storage.Value
			merged.SourceIDs = appendUniqueStrings(merged.SourceIDs, storage.SourceIDs...)
		}
	}
}

func energyExplanationZoneAllocationFactors(nodes []EnergyExplanationNode, edges []EnergyExplanationEdge, scope EnergyExplanationScope, allocationPolicy string) map[string]float64 {
	if scope.Kind != "zone" || scope.ZoneName == "" {
		return nil
	}
	if normalizePurposeAllocationPolicy(allocationPolicy) == PurposeAllocationPolicyDirectOnly || !energyExplanationHasExplicitAllocation(edges) {
		return nil
	}
	nodeByID := make(map[string]EnergyExplanationNode, len(nodes))
	for _, node := range nodes {
		nodeByID[node.ID] = node
	}
	type allocationShare struct {
		total    float64
		selected float64
	}
	sharesByEndUse := map[string]allocationShare{}
	sharesByService := map[string]allocationShare{}
	seenEndUseLoads := map[string]bool{}
	seenServiceLoads := map[string]bool{}
	for _, edge := range edges {
		if !strings.EqualFold(edge.Relation, "allocation") {
			continue
		}
		load := nodeByID[edge.ToID]
		if strings.TrimSpace(load.ZoneName) == "" {
			continue
		}
		endUse := nodeByID[edge.FromID]
		// Explicit allocation edges already carry the period-specific allocated
		// site-energy contribution. Using the thermal load again would recompute
		// an annual share and lose the sum of monthly allocations.
		value := math.Abs(firstNonZero(edge.Value, energyExplanationEffectiveNodeValue(energyExplanationLegacyNodeWithEffectiveValues(load))))
		service := energyCanonicalServiceKind(firstNonEmpty(load.ServiceKind, edge.ServiceKind, endUse.EndUse))
		endUseKey := energyExplanationAllocationEndUseKey(endUse, service)
		selected := strings.EqualFold(strings.TrimSpace(load.ZoneName), scope.ZoneName)
		if key := endUseKey + "|" + edge.ToID; endUseKey != "" && !seenEndUseLoads[key] {
			share := sharesByEndUse[endUseKey]
			share.total += value
			if selected {
				share.selected += value
			}
			sharesByEndUse[endUseKey] = share
			seenEndUseLoads[key] = true
		}
		if key := service + "|" + edge.ToID; service != "" && !seenServiceLoads[key] {
			share := sharesByService[service]
			share.total += value
			if selected {
				share.selected += value
			}
			sharesByService[service] = share
			seenServiceLoads[key] = true
		}
	}
	factors := map[string]float64{}
	for _, node := range nodes {
		if strings.TrimSpace(node.ZoneName) != "" || !strings.EqualFold(node.Level, "energy") || legacyEnergyNodeIsCarrier(node) || legacyEnergyNodeIsSupport(node) {
			continue
		}
		key := energyExplanationAllocationEndUseKey(node, energyCanonicalServiceKind(node.EndUse))
		if share := sharesByEndUse[key]; key != "" && share.total > 0 {
			factors[node.ID] = share.selected / share.total
		}
	}
	allocatedByCarrier := map[string]float64{}
	for _, node := range nodes {
		factor, ok := factors[node.ID]
		if !ok || strings.TrimSpace(node.Carrier) == "" {
			continue
		}
		carrier := strings.ToLower(strings.TrimSpace(node.Carrier))
		allocatedByCarrier[carrier] += math.Abs(energyExplanationEffectiveNodeValue(energyExplanationLegacyNodeWithEffectiveValues(node))) * factor
	}
	for _, node := range nodes {
		if strings.TrimSpace(node.ZoneName) != "" || !legacyEnergyNodeIsCarrier(node) {
			continue
		}
		carrier := strings.ToLower(strings.TrimSpace(node.Carrier))
		value := math.Abs(energyExplanationEffectiveNodeValue(energyExplanationLegacyNodeWithEffectiveValues(node)))
		if value > 0 && allocatedByCarrier[carrier] > 0 {
			factor := allocatedByCarrier[carrier] / value
			factors[node.ID] = factor
		}
	}
	for _, node := range nodes {
		if strings.TrimSpace(node.ZoneName) != "" {
			continue
		}
		if _, ok := factors[node.ID]; ok {
			continue
		}
		if strings.EqualFold(node.Level, "residual") || legacyEnergyNodeIsSupport(node) {
			// Residual and onsite/storage support values have no defensible zone
			// assignment merely because another end use was load-share allocated.
			continue
		}
		service := energyCanonicalServiceKind(node.ServiceKind)
		if share := sharesByService[service]; service != "" && share.total > 0 {
			factors[node.ID] = share.selected / share.total
			continue
		}
	}
	return factors
}

func energyExplanationAllocationEndUseKey(node EnergyExplanationNode, service string) string {
	endUse := strings.ToLower(strings.TrimSpace(node.EndUse))
	if endUse == "" || endUse == "total" {
		endUse = energyCanonicalServiceKind(service)
	}
	if endUse == "" || endUse == "total" {
		return ""
	}
	return canonicalEnergyPathPart(endUse)
}

func energyExplanationHasExplicitAllocation(edges []EnergyExplanationEdge) bool {
	for _, edge := range edges {
		if strings.EqualFold(edge.Relation, "allocation") && (edge.RuleID == energyRelationshipRuleAllocatedZoneLoad || edge.RuleID == energyRelationshipRuleAllocatedServicePathLoad || canonicalEnergyPathBasis(edge.Basis, edge.RuleID) == "zone_load_allocation" || canonicalEnergyPathBasis(edge.Basis, edge.RuleID) == "service_path_allocation") {
			return true
		}
	}
	return false
}

func energyExplanationNodeForScope(node EnergyExplanationNode, scope EnergyExplanationScope, factors map[string]float64) (EnergyExplanationNode, bool) {
	node = energyExplanationLegacyNodeWithEffectiveValues(node)
	if scope.Kind != "zone" {
		if node.driverZoneOnly {
			return EnergyExplanationNode{}, false
		}
		return node, true
	}
	if node.driverBuildingOnly {
		return EnergyExplanationNode{}, false
	}
	if zoneName := strings.TrimSpace(node.ZoneName); zoneName != "" {
		return node, strings.EqualFold(zoneName, scope.ZoneName)
	}
	factor, ok := factors[node.ID]
	if !ok || factor <= 0 {
		return EnergyExplanationNode{}, false
	}
	effective := node.EffectiveValue
	node.AllocatedValue = roundedEnergyNumber(effective * factor)
	node.Value = node.AllocatedValue
	if node.SignedValue != 0 {
		node.SignedValue = roundedEnergyNumber(node.SignedValue * factor)
		node.DisplayValue = math.Abs(node.SignedValue)
	}
	return node, node.Value != 0
}

func energyExplanationLegacyNodeWithEffectiveValues(node EnergyExplanationNode) EnergyExplanationNode {
	if node.AllocationApplied {
		node.Value = node.AllocatedValue
		node.DisplayValue = math.Abs(node.AllocatedValue)
		if node.Multiplier == 0 {
			node.Multiplier = 1
		}
		return node
	}
	raw := firstNonZero(node.RawValue, node.Value)
	hadExplicitEffective := node.EffectiveValue != 0
	multiplier := node.Multiplier
	if multiplier == 0 {
		multiplier = 1
	}
	effective := node.EffectiveValue
	if effective == 0 {
		if node.AllocatedValue != 0 {
			// v1 used allocatedValue for the physical multiplier result.
			effective = node.AllocatedValue
		} else {
			effective = roundedEnergyNumber(raw * multiplier)
		}
	}
	allocated := firstNonZero(node.AllocatedValue, effective)
	node.RawValue = raw
	node.EffectiveValue = effective
	node.AllocatedValue = allocated
	node.Value = allocated
	node.Multiplier = multiplier
	if node.SignedValue != 0 {
		signedBasis := raw
		if hadExplicitEffective {
			signedBasis = effective
		}
		if signedBasis != 0 {
			node.SignedValue = roundedEnergyNumber(node.SignedValue * allocated / signedBasis)
		}
		node.DisplayValue = math.Abs(node.SignedValue)
	}
	return node
}

func scopedEnergyExplanationLegacyNodes(nodes []EnergyExplanationNode, edges []EnergyExplanationEdge, scope EnergyExplanationScope, allocationPolicy string) []EnergyExplanationNode {
	factors := energyExplanationZoneAllocationFactors(nodes, edges, scope, allocationPolicy)
	out := make([]EnergyExplanationNode, 0, len(nodes))
	for _, node := range nodes {
		if scoped, ok := energyExplanationNodeForScope(node, scope, factors); ok {
			out = append(out, scoped)
		}
	}
	return out
}

func upgradeEnergyExplanationAccounting(input []EnergyReconciliation, warnings []EnergyWarning, nodes []EnergyExplanationNode, edges []EnergyExplanationEdge, scope EnergyExplanationScope, allocationPolicy string) ([]EnergyReconciliation, []EnergyWarning) {
	scope = normalizeEnergyExplanationScope(scope)
	out := make([]EnergyReconciliation, 0, len(input))
	if scope.Kind != "zone" {
		for _, item := range input {
			item.Basis = canonicalEnergyPathBasis(item.Basis, "")
			out = append(out, item)
		}
		return out, append([]EnergyWarning(nil), warnings...)
	}

	selectedZoneRecords := map[string]bool{}
	for _, item := range input {
		if strings.EqualFold(strings.TrimSpace(item.ZoneName), scope.ZoneName) {
			selectedZoneRecords[strings.ToLower(item.Level)+"|"+strings.ToLower(item.ServiceKind)] = true
		}
	}
	factors := energyExplanationZoneAllocationFactors(nodes, edges, scope, allocationPolicy)
	for _, item := range input {
		zoneName := strings.TrimSpace(item.ZoneName)
		if zoneName != "" {
			if !strings.EqualFold(zoneName, scope.ZoneName) {
				continue
			}
			item.ZoneName = scope.ZoneName
			item.Basis = canonicalEnergyPathBasis(item.Basis, "")
			out = append(out, item)
			continue
		}
		key := strings.ToLower(item.Level) + "|" + strings.ToLower(item.ServiceKind)
		if selectedZoneRecords[key] {
			continue
		}
		factor := energyExplanationReconciliationScopeFactor(item, nodes, factors, scope)
		if factor <= 0 {
			continue
		}
		item.ID = firstNonEmpty(item.ID, "reconcile") + "." + energyExplanationScopeToken(scope)
		item.Label = strings.TrimSpace(item.Label + " - " + scope.ZoneName)
		item.ZoneName = scope.ZoneName
		item.ExpectedValue = roundedEnergyNumber(item.ExpectedValue * factor)
		item.ExplainedValue = roundedEnergyNumber(item.ExplainedValue * factor)
		item.ResidualValue = roundedEnergyNumber(item.ResidualValue * factor)
		item.Status = energyReconciliationStatus(item.ExpectedValue, item.ResidualValue)
		item.Basis = canonicalEnergyPathBasis(item.Basis, "")
		out = append(out, item)
	}
	return out, filterEnergyExplanationWarningsForScope(warnings, nodes, scope)
}

// reconcileEnergyPathCarrierTotals makes the v2 stage boundary authoritative:
// every facility carrier total is compared with the sum of the carrier splits
// that arrive from carrier-neutral end-use nodes.  Stored v1 payloads are not
// guaranteed to contain reconciliation rows, so the adapter fills a missing
// row and refreshes an existing legacy row from the canonical graph.
func reconcileEnergyPathCarrierTotals(nodes []EnergyExplanationNode, links []EnergyPathLink, input []EnergyReconciliation, period string) []EnergyReconciliation {
	period = strings.TrimSpace(period)
	if period == "" {
		period = "annual"
	}
	type carrierTotal struct {
		carrier     string
		expected    float64
		explained   float64
		unit        string
		sourceIDs   []string
		existingRow int
	}
	byNodeID := map[string]*carrierTotal{}
	orderedNodeIDs := make([]string, 0)
	for _, node := range nodes {
		if node.Level != "carrier" {
			continue
		}
		carrier := canonicalEnergyPathPart(firstNonEmpty(node.Carrier, energyExplanationKindSuffix(node.Kind), "other"))
		byNodeID[node.ID] = &carrierTotal{
			carrier:     carrier,
			expected:    math.Abs(node.Value),
			unit:        node.Unit,
			sourceIDs:   appendUniqueStrings(nil, node.SourceIDs...),
			existingRow: -1,
		}
		orderedNodeIDs = append(orderedNodeIDs, node.ID)
	}
	if len(byNodeID) == 0 {
		return input
	}
	for _, link := range links {
		if link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier" {
			continue
		}
		total := byNodeID[link.ToID]
		if total == nil {
			continue
		}
		total.explained += math.Abs(link.ToValue)
		total.unit = firstNonEmpty(total.unit, link.ToUnit)
		total.sourceIDs = appendUniqueStrings(total.sourceIDs, link.SourceIDs...)
	}
	out := append([]EnergyReconciliation(nil), input...)
	for index := range out {
		for _, total := range byNodeID {
			if total.existingRow >= 0 || !energyPathCarrierReconciliationMatches(out[index], total.carrier, period) {
				continue
			}
			total.existingRow = index
			break
		}
	}
	sort.Strings(orderedNodeIDs)
	for _, nodeID := range orderedNodeIDs {
		total := byNodeID[nodeID]
		total.expected = roundedEnergyNumber(total.expected)
		total.explained = roundedEnergyNumber(total.explained)
		residual := roundedEnergyNumber(total.expected - total.explained)
		sort.Strings(total.sourceIDs)
		row := EnergyReconciliation{
			ID:             "reconcile.energy." + total.carrier + "." + period,
			Level:          "energy",
			Period:         period,
			Label:          energyCarrierLabel(total.carrier) + " total basis",
			Status:         energyReconciliationStatus(total.expected, residual),
			ExpectedValue:  total.expected,
			ExplainedValue: total.explained,
			ResidualValue:  residual,
			Unit:           total.unit,
			Basis:          "residual",
			Formula:        "facility carrier total - mapped carrier-qualified end-use meters",
			SourceIDs:      total.sourceIDs,
		}
		if total.existingRow >= 0 {
			// Retain the scope-qualified compatibility ID produced by the v1
			// adapter while replacing its accounting with the canonical v2 split.
			row.ID = out[total.existingRow].ID
			row.ZoneName = out[total.existingRow].ZoneName
			out[total.existingRow] = row
			continue
		}
		out = append(out, row)
	}
	return out
}

func energyPathCarrierReconciliationMatches(item EnergyReconciliation, carrier string, period string) bool {
	if !strings.EqualFold(item.Level, "energy") && !strings.EqualFold(item.Level, "carrier") {
		return false
	}
	if item.Period != "" && !strings.EqualFold(item.Period, period) {
		return false
	}
	prefix := "reconcile.energy." + strings.ToLower(strings.TrimSpace(carrier)) + "."
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.ID)), prefix)
}

func energyExplanationReconciliationScopeFactor(item EnergyReconciliation, nodes []EnergyExplanationNode, factors map[string]float64, scope EnergyExplanationScope) float64 {
	if len(factors) == 0 {
		return 0
	}
	if service := strings.TrimSpace(item.ServiceKind); service != "" {
		if factor, ok := energyExplanationZoneLoadShare(nodes, scope.ZoneName, service); ok {
			return factor
		}
	}
	lowerIdentity := strings.ToLower(item.ID + " " + item.Label)
	for _, node := range nodes {
		if !legacyEnergyNodeIsCarrier(node) || strings.TrimSpace(node.ZoneName) != "" {
			continue
		}
		carrier := canonicalEnergyPathPart(node.Carrier)
		if carrier != "" && strings.Contains(lowerIdentity, carrier) {
			if factor, ok := factors[node.ID]; ok {
				return factor
			}
		}
	}
	if factor, ok := energyExplanationZoneLoadShare(nodes, scope.ZoneName, ""); ok {
		return factor
	}
	return 0
}

func energyExplanationZoneLoadShare(nodes []EnergyExplanationNode, selectedZone string, service string) (float64, bool) {
	total := 0.0
	selected := 0.0
	for _, node := range nodes {
		if !strings.EqualFold(node.Level, "load") || strings.TrimSpace(node.ZoneName) == "" {
			continue
		}
		if service != "" && !strings.EqualFold(strings.TrimSpace(node.ServiceKind), service) {
			continue
		}
		value := math.Abs(energyExplanationEffectiveNodeValue(energyExplanationLegacyNodeWithEffectiveValues(node)))
		total += value
		if strings.EqualFold(strings.TrimSpace(node.ZoneName), selectedZone) {
			selected += value
		}
	}
	if total <= 0 {
		return 0, false
	}
	return selected / total, true
}

func filterEnergyExplanationWarningsForScope(input []EnergyWarning, nodes []EnergyExplanationNode, scope EnergyExplanationScope) []EnergyWarning {
	zoneNames := []string{}
	for _, node := range nodes {
		if name := strings.TrimSpace(node.ZoneName); name != "" {
			zoneNames = appendUniqueStrings(zoneNames, name)
		}
	}
	out := make([]EnergyWarning, 0, len(input))
	for _, warning := range input {
		message := strings.ToLower(warning.Message)
		mentionsSelected := strings.Contains(message, strings.ToLower(scope.ZoneName))
		mentionsOther := false
		for _, name := range zoneNames {
			if !strings.EqualFold(name, scope.ZoneName) && strings.Contains(message, strings.ToLower(name)) {
				mentionsOther = true
				break
			}
		}
		if !mentionsOther || mentionsSelected {
			out = append(out, warning)
		}
	}
	return out
}

func filterEnergyDataSourcesForV2(input []EnergyDataSource, legacyNodes []EnergyExplanationNode, legacyEdges []EnergyExplanationEdge, nodes []EnergyExplanationNode, links []EnergyPathLink, reconciliation []EnergyReconciliation, scope EnergyExplanationScope, allocationPolicy string) []EnergyDataSource {
	type sourceValues struct {
		raw                 float64
		effective           float64
		effectiveMultiplier float64
		allocationFactor    float64
		allocated           float64
		allocationApplied   bool
	}
	used := map[string]bool{}
	values := map[string]sourceValues{}
	sourceByID := make(map[string]EnergyDataSource, len(input))
	for _, source := range input {
		sourceByID[source.ID] = source
	}
	factors := energyExplanationZoneAllocationFactors(legacyNodes, legacyEdges, scope, allocationPolicy)
	for _, original := range legacyNodes {
		node := energyExplanationLegacyNodeWithEffectiveValues(original)
		allocationFactor := 1.0
		if scope.Kind == "zone" {
			if zoneName := strings.TrimSpace(node.ZoneName); zoneName != "" {
				if !strings.EqualFold(zoneName, scope.ZoneName) {
					continue
				}
			} else {
				var ok bool
				allocationFactor, ok = factors[node.ID]
				if !ok || allocationFactor <= 0 {
					continue
				}
			}
		}
		raw := math.Abs(firstNonZero(original.RawValue, original.Value))
		effective := math.Abs(firstNonZero(node.EffectiveValue, node.Value))
		multiplier := original.Multiplier
		if multiplier == 0 {
			multiplier = 1
		}
		allocated := roundedEnergyNumber(effective * allocationFactor)
		if original.AllocationApplied {
			allocated = math.Abs(original.AllocatedValue)
			if effective > 0 {
				allocationFactor *= allocated / effective
			} else {
				allocationFactor = 0
			}
			for _, sourceID := range original.SourceIDs {
				used[sourceID] = true
			}
		}
		allocationSourceIDs := original.SourceIDs
		if original.AllocationApplied {
			allocationSourceIDs = append([]string(nil), original.allocationSourceIDs...)
			candidateSourceIDs := original.SourceIDs
			if len(allocationSourceIDs) > 0 {
				candidateSourceIDs = allocationSourceIDs
			}
			allocationSourceIDs = make([]string, 0, len(candidateSourceIDs))
			for _, sourceID := range candidateSourceIDs {
				source, exists := sourceByID[sourceID]
				if !exists || !strings.EqualFold(strings.TrimSpace(source.DriverCategory), strings.TrimSpace(original.DriverCategory)) {
					continue
				}
				if nodeZone := strings.TrimSpace(original.ZoneName); nodeZone != "" && !strings.EqualFold(strings.TrimSpace(source.ZoneName), nodeZone) {
					continue
				}
				if source.DriverRole != "" && source.DriverRole != energyDriverSourceRoleMainFlow {
					continue
				}
				allocationSourceIDs = append(allocationSourceIDs, sourceID)
			}
		}
		for _, sourceID := range allocationSourceIDs {
			value := values[sourceID]
			// A SQL dictionary source may be propagated to residual and link
			// nodes. Keep its largest direct contribution instead of counting
			// those graph aliases more than once.
			if effective > value.effective || (original.AllocationApplied && !value.allocationApplied) {
				value = sourceValues{
					raw:                 raw,
					effective:           effective,
					effectiveMultiplier: multiplier,
					allocationFactor:    allocationFactor,
					allocated:           allocated,
					allocationApplied:   original.AllocationApplied,
				}
			}
			values[sourceID] = value
		}
	}
	for _, node := range nodes {
		for _, sourceID := range node.SourceIDs {
			used[sourceID] = true
		}
	}
	for _, link := range links {
		for _, sourceID := range link.SourceIDs {
			used[sourceID] = true
		}
	}
	for _, item := range reconciliation {
		for _, sourceID := range item.SourceIDs {
			used[sourceID] = true
		}
	}
	for _, source := range input {
		if source.DriverRole != energyDriverSourceRoleContext && source.DriverRole != energyDriverSourceRoleReconciliation {
			continue
		}
		if scope.Kind == "zone" && strings.TrimSpace(source.ZoneName) != "" && !strings.EqualFold(source.ZoneName, scope.ZoneName) {
			continue
		}
		used[source.ID] = true
	}
	out := make([]EnergyDataSource, 0, len(input))
	for _, source := range input {
		if scope.Kind == "zone" && !used[source.ID] {
			continue
		}
		value := values[source.ID]
		// Context preparation records the signed SQL value on the source. Keep
		// that provenance: graph nodes use absolute display values and cannot
		// reconstruct the original sign (notably for surface convection).
		if source.RawValue == 0 {
			source.RawValue = roundedEnergyNumber(value.raw)
		}
		if source.EffectiveValue == 0 {
			source.EffectiveValue = roundedEnergyNumber(value.effective)
		}
		if source.EffectiveMultiplier == 0 {
			source.EffectiveMultiplier = roundedEnergyNumber(value.effectiveMultiplier)
		}
		if value.allocationApplied {
			source.AllocationApplied = true
			source.AllocationFactor = roundedEnergyNumber(value.allocationFactor)
		} else if value.allocationFactor != 0 || source.AllocationFactor == 0 {
			source.AllocationFactor = roundedEnergyNumber(value.allocationFactor)
		}
		if source.AllocationApplied {
			source.AllocatedValue = roundedEnergyNumber(math.Abs(source.EffectiveValue) * value.allocationFactor)
			source.AllocationExplanation = energyDriverAllocationExplanation
			source.AllocationFormula = energyDriverAllocationFormula
			if source.Explanation == "" {
				source.Explanation = energyDriverAllocationExplanation
			}
			if source.Formula == "" {
				source.Formula = energyDriverAllocationFormula
			}
		} else if source.EffectiveValue != 0 {
			allocationFactor := source.AllocationFactor
			if allocationFactor == 0 {
				allocationFactor = 1
			}
			source.AllocatedValue = roundedEnergyNumber(source.EffectiveValue * allocationFactor)
		} else if value.allocated != 0 || source.AllocatedValue == 0 {
			source.AllocatedValue = roundedEnergyNumber(value.allocated)
		}
		source.AggregationBasis = scope.AggregationBasis
		out = append(out, source)
	}
	return out
}

func appendEnergyDataSourceScopeDetails(parent []EnergyDataSource, scoped []EnergyDataSource, scope EnergyExplanationScope) {
	indexByID := make(map[string]int, len(parent))
	for index := range parent {
		indexByID[parent[index].ID] = index
	}
	for _, source := range scoped {
		index, ok := indexByID[source.ID]
		if !ok {
			continue
		}
		detail := EnergyDataSourceScopeDetail{
			Scope:                 normalizeEnergyExplanationScope(scope),
			RawValue:              source.RawValue,
			EffectiveValue:        source.EffectiveValue,
			EffectiveMultiplier:   source.EffectiveMultiplier,
			MultiplierApplication: source.MultiplierApplication,
			AllocationFactor:      source.AllocationFactor,
			AllocatedValue:        source.AllocatedValue,
			AllocationApplied:     source.AllocationApplied,
			AggregationBasis:      firstNonEmpty(source.AggregationBasis, scope.AggregationBasis),
		}
		parent[index].ScopeDetails = append(parent[index].ScopeDetails, detail)
	}
}

func buildEnergyExplanationZoneContributions(nodes []EnergyExplanationNode, edges []EnergyExplanationEdge, scope EnergyExplanationScope, allocationPolicy string) []EnergyExplanationSummaryItem {
	groups := map[string]*EnergyExplanationSummaryItem{}
	for _, node := range scopedEnergyExplanationLegacyNodes(nodes, edges, scope, allocationPolicy) {
		if !strings.EqualFold(node.Level, "heat") || strings.TrimSpace(node.ZoneName) == "" {
			continue
		}
		value := energyExplanationSummaryValue(node)
		if value == 0 {
			continue
		}
		addEnergyExplanationSummaryNode(groups, node.ZoneName, EnergyExplanationNode{
			ID:               "zone." + metricID(node.ZoneName),
			Level:            "zone",
			Kind:             "zone.driver",
			Label:            node.ZoneName,
			Value:            value,
			RawValue:         firstNonZero(node.RawValue, value),
			AllocatedValue:   firstNonZero(node.AllocatedValue, value),
			Unit:             node.Unit,
			ScaleDomain:      "thermal",
			ZoneName:         node.ZoneName,
			ServiceKind:      node.ServiceKind,
			Basis:            canonicalEnergyPathBasis(node.Basis, ""),
			AggregationBasis: normalizeEnergyExplanationScope(scope).AggregationBasis,
			SourceIDs:        appendUniqueStrings(nil, node.SourceIDs...),
		}, value)
	}
	return sortedEnergyExplanationSummaryItems(groups)
}

func upgradeEnergyExplanationNode(input EnergyExplanationNode, scope EnergyExplanationScope) EnergyExplanationNode {
	out := input
	out.LoadBreakdown = cloneEnergyExplanationLoadComponents(input.LoadBreakdown)
	out.OffsetEffects = cloneEnergyExplanationOffsetEffects(input.OffsetEffects)
	out.SimultaneousLoad = cloneEnergyExplanationSimultaneousLoad(input.SimultaneousLoad)
	out.allocationSourceIDs = appendUniqueStrings(nil, input.allocationSourceIDs...)
	out.simultaneousLoadContributions = cloneEnergyExplanationSimultaneousLoadContributions(input.simultaneousLoadContributions)
	out.endUseCarriers = appendUniqueStrings(nil, input.endUseCarriers...)
	scopeToken := energyExplanationScopeToken(scope)
	out.AggregationBasis = firstNonEmpty(input.AggregationBasis, scope.AggregationBasis)
	out.Multiplier = input.Multiplier
	if out.Multiplier == 0 {
		out.Multiplier = 1
	}
	if input.AllocationApplied {
		out.RawValue = input.RawValue
		out.EffectiveValue = input.EffectiveValue
		out.AllocatedValue = input.AllocatedValue
	} else {
		out.RawValue = firstNonZero(input.RawValue, input.Value)
		out.EffectiveValue = input.EffectiveValue
		if out.EffectiveValue == 0 {
			out.EffectiveValue = roundedEnergyNumber(out.RawValue * out.Multiplier)
		}
		out.AllocatedValue = input.AllocatedValue
		if out.AllocatedValue == 0 {
			out.AllocatedValue = out.EffectiveValue
		}
	}
	out.Value = out.AllocatedValue
	if input.SignedValue != 0 {
		out.SignedValue = roundedEnergyNumber(input.SignedValue)
		if input.AllocationApplied {
			out.DisplayValue = math.Abs(out.AllocatedValue)
		} else {
			out.DisplayValue = math.Abs(out.SignedValue)
		}
	}
	out.Basis = canonicalEnergyPathBasis(input.Basis, "")
	if scope.Kind == "zone" {
		out.ZoneName = scope.ZoneName
	} else {
		out.ZoneName = ""
	}

	switch strings.ToLower(strings.TrimSpace(input.Level)) {
	case "driver", "heat":
		out.Level = "driver"
		out.ScaleDomain = "thermal"
		out.DriverCategory = energyExplanationDriverCategory(input)
		out.ThermalComponent = energyExplanationThermalComponent(input)
		out.ID = strings.Join([]string{"driver", canonicalEnergyPathCategory(out.DriverCategory), canonicalEnergyPathPart(firstNonEmpty(input.ServiceKind, "all")), scopeToken}, ".")
	case "load":
		out.Level = "load"
		out.ScaleDomain = "thermal"
		out.ThermalComponent = energyExplanationThermalComponent(input)
		service := strings.ToLower(strings.TrimSpace(firstNonEmpty(input.ServiceKind, energyExplanationKindSuffix(input.Kind), "other")))
		out.ID = strings.Join([]string{"load", canonicalEnergyPathPart(service), scopeToken}, ".")
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(input.DriverCategory)), "load.") {
			switch service {
			case "cooling":
				out.Label = "Cooling load"
			case "heating":
				out.Label = "Heating load"
			}
		}
	case "support":
		out.Level = "support"
		out.ScaleDomain = "site"
		out.ID = strings.Join([]string{"support", canonicalEnergyPathPart(firstNonEmpty(input.EndUse, energyExplanationKindSuffix(input.Kind), "other")), scopeToken}, ".")
	case "residual":
		out.Level = "residual"
		out.ScaleDomain = energyExplanationResidualScaleDomain(input)
		out.ID = strings.Join([]string{"residual", energyExplanationResidualDomain(input), scopeToken}, ".")
		if strings.TrimSpace(input.Basis) == "" {
			out.Basis = "residual"
		}
	case "carrier":
		out.Level = "carrier"
		out.ScaleDomain = "site"
		out.ID = strings.Join([]string{"carrier", canonicalEnergyPathPart(firstNonEmpty(input.Carrier, energyExplanationKindSuffix(input.Kind), "other")), scopeToken}, ".")
	case "end_use":
		out.Level = "end_use"
		out.ScaleDomain = "site"
		out.ID = strings.Join([]string{"end_use", canonicalEnergyPathEndUse(firstNonEmpty(input.EndUse, energyExplanationKindSuffix(input.Kind), "other")), scopeToken}, ".")
	default:
		if legacyEnergyNodeIsSupport(input) {
			out.Level = "support"
			out.ScaleDomain = "site"
			out.ID = strings.Join([]string{"support", canonicalEnergyPathPart(firstNonEmpty(input.EndUse, energyExplanationKindSuffix(input.Kind), "other")), scopeToken}, ".")
		} else if legacyEnergyNodeIsCarrier(input) {
			out.Level = "carrier"
			out.ScaleDomain = "site"
			out.ID = strings.Join([]string{"carrier", canonicalEnergyPathPart(firstNonEmpty(input.Carrier, "other")), scopeToken}, ".")
		} else {
			out.Level = "end_use"
			out.ScaleDomain = "site"
			out.ID = strings.Join([]string{"end_use", canonicalEnergyPathEndUse(firstNonEmpty(input.EndUse, energyExplanationKindSuffix(input.Kind), "other")), scopeToken}, ".")
		}
	}

	// v1 represented both carrier and end use with level "energy".
	if strings.EqualFold(input.Level, "energy") {
		switch {
		case legacyEnergyNodeIsSupport(input):
			out.Level = "support"
			out.ScaleDomain = "site"
			out.ID = strings.Join([]string{"support", canonicalEnergyPathPart(firstNonEmpty(input.EndUse, energyExplanationKindSuffix(input.Kind), "other")), scopeToken}, ".")
		case legacyEnergyNodeIsCarrier(input):
			out.Level = "carrier"
			out.ScaleDomain = "site"
			out.ID = strings.Join([]string{"carrier", canonicalEnergyPathPart(firstNonEmpty(input.Carrier, "other")), scopeToken}, ".")
		default:
			out.Level = "end_use"
			out.ScaleDomain = "site"
			out.ID = strings.Join([]string{"end_use", canonicalEnergyPathEndUse(firstNonEmpty(input.EndUse, energyExplanationKindSuffix(input.Kind), "other")), scopeToken}, ".")
		}
	}
	if out.Level == "end_use" {
		// Legacy meters are carrier-qualified (for example,
		// Heating:Electricity and Heating:NaturalGas).  The v2 end-use stage is
		// intentionally carrier-neutral, so its visible identity must not depend
		// on which carrier-qualified meter happened to be encountered first.
		endUse := canonicalEnergyPathEndUse(firstNonEmpty(input.EndUse, energyExplanationKindSuffix(input.Kind), "other"))
		out.EndUse = endUse
		out.Kind = "energy." + endUse
		out.Label = canonicalEnergyPathEndUseLabel(endUse)
		if carrier := strings.TrimSpace(input.Carrier); carrier != "" {
			out.endUseCarriers = appendUniqueStrings(out.endUseCarriers, canonicalEnergyPathPart(carrier))
		}
		out.Carrier = ""
	}
	finalizeEnergyExplanationLoadNode(&out)
	return out
}

func canonicalEnergyPathEndUse(value string) string {
	switch canonicalEnergyPathPart(value) {
	case "cooling":
		return "cooling"
	case "heating":
		return "heating"
	case "fans":
		return "fans"
	case "pumps":
		return "pumps"
	case "heat_rejection", "heatrejection":
		return "heat_rejection"
	case "humidification", "humidifier":
		return "humidification"
	case "heat_recovery", "heatrecovery":
		return "heat_recovery"
	case "lighting", "lights", "interior_lighting", "interior_lights", "interiorlighting", "interiorlights", "exterior_lighting", "exterior_lights", "exteriorlighting", "exteriorlights":
		return "lighting"
	case "equipment", "interior_equipment", "interiorequipment", "exterior_equipment", "exteriorequipment":
		return "equipment"
	case "water_systems", "watersystems", "dhw":
		return "water_systems"
	case "refrigeration":
		return "refrigeration"
	case "other":
		return "other"
	default:
		return "other"
	}
}

func canonicalEnergyPathEndUseLabel(endUse string) string {
	label := strings.ReplaceAll(canonicalEnergyPathPart(endUse), "_", " ")
	return cases.Title(language.English, cases.NoLower).String(label)
}

func mergeEnergyExplanationV2Node(nodes map[string]*EnergyExplanationNode, next EnergyExplanationNode) {
	current := nodes[next.ID]
	if current == nil {
		copy := next
		copy.LoadBreakdown = cloneEnergyExplanationLoadComponents(next.LoadBreakdown)
		copy.OffsetEffects = cloneEnergyExplanationOffsetEffects(next.OffsetEffects)
		copy.SimultaneousLoad = cloneEnergyExplanationSimultaneousLoad(next.SimultaneousLoad)
		copy.allocationSourceIDs = appendUniqueStrings(nil, next.allocationSourceIDs...)
		copy.simultaneousLoadContributions = cloneEnergyExplanationSimultaneousLoadContributions(next.simultaneousLoadContributions)
		copy.endUseCarriers = appendUniqueStrings(nil, next.endUseCarriers...)
		nodes[next.ID] = &copy
		return
	}
	current.Value = roundedEnergyNumber(current.Value + next.Value)
	current.SignedValue = roundedEnergyNumber(current.SignedValue + next.SignedValue)
	current.RawValue = roundedEnergyNumber(current.RawValue + next.RawValue)
	current.EffectiveValue = roundedEnergyNumber(current.EffectiveValue + next.EffectiveValue)
	current.AllocatedValue = roundedEnergyNumber(current.AllocatedValue + next.AllocatedValue)
	current.AllocationApplied = current.AllocationApplied || next.AllocationApplied
	current.AllocationExplanation = firstNonEmpty(current.AllocationExplanation, next.AllocationExplanation)
	if current.RawValue != 0 {
		current.Multiplier = roundedEnergyNumber(current.EffectiveValue / current.RawValue)
	}
	current.DisplayValue = roundedEnergyNumber(current.DisplayValue + next.DisplayValue)
	current.RelatedPathIDs = appendUniqueStrings(current.RelatedPathIDs, next.RelatedPathIDs...)
	current.RelatedEntityIDs = appendUniqueStrings(current.RelatedEntityIDs, next.RelatedEntityIDs...)
	current.SourceIDs = appendUniqueStrings(current.SourceIDs, next.SourceIDs...)
	current.LoadBreakdown = mergeEnergyExplanationLoadComponents(current.LoadBreakdown, next.LoadBreakdown)
	current.OffsetEffects = mergeEnergyExplanationOffsetEffects(current.OffsetEffects, next.OffsetEffects)
	current.simultaneousLoadContributions = mergeEnergyExplanationSimultaneousLoadContributions(current.simultaneousLoadContributions, next.simultaneousLoadContributions)
	current.endUseCarriers = appendUniqueStrings(current.endUseCarriers, next.endUseCarriers...)
	current.Badges = appendUniqueStrings(current.Badges, next.Badges...)
	if current.ThermalComponent == "" {
		current.ThermalComponent = next.ThermalComponent
	} else if next.ThermalComponent != "" && current.ThermalComponent != next.ThermalComponent {
		current.ThermalComponent = "combined"
	}
	if current.Level == "end_use" {
		current.Carrier = ""
	} else if current.Carrier != next.Carrier {
		current.Carrier = ""
	}
	if current.ZoneName != next.ZoneName {
		current.ZoneName = ""
	}
	if current.Basis != next.Basis {
		current.Basis = "derived_ratio"
	}
	finalizeEnergyExplanationLoadNode(current)
	finalizeEnergyExplanationSimultaneousLoad(current)
}

func upgradeEnergyExplanationLink(edge EnergyExplanationEdge, nodes map[string]EnergyExplanationNode, idMap map[string]string, canonicalNodes map[string]*EnergyExplanationNode, loadTotals map[string]float64, endUsesWithLoads map[string]bool, canonicalMonthlyBasis bool) (EnergyPathLink, bool) {
	legacyFrom, fromOK := nodes[edge.FromID]
	legacyTo, toOK := nodes[edge.ToID]
	fromID, mappedFrom := idMap[edge.FromID]
	toID, mappedTo := idMap[edge.ToID]
	if !fromOK || !toOK || !mappedFrom || !mappedTo || fromID == "" || toID == "" {
		return EnergyPathLink{}, false
	}

	carrierSplit := legacyEnergyLinkIsEndUseToCarrier(edge)
	sourceIDs := appendUniqueStrings(appendUniqueStrings(appendUniqueStrings(nil, edge.SourceIDs...), legacyFrom.SourceIDs...), legacyTo.SourceIDs...)
	if carrierSplit {
		// The split ribbon is measured by the carrier-qualified end-use meter,
		// not by the facility-total meter.  Runtime legacy edges already carry
		// that exact source; stored payloads may require the end-use endpoint as
		// a fallback.
		sourceIDs = appendUniqueStrings(nil, edge.SourceIDs...)
		if len(sourceIDs) == 0 {
			sourceIDs = appendUniqueStrings(nil, legacyTo.SourceIDs...)
		}
	}
	link := EnergyPathLink{
		RuleID:         edge.RuleID,
		Explanation:    edge.Formula,
		Period:         edge.Period,
		ZoneName:       firstNonEmpty(edge.ZoneName, legacyFrom.ZoneName, legacyTo.ZoneName),
		ServiceKind:    firstNonEmpty(edge.ServiceKind, legacyFrom.ServiceKind, legacyTo.ServiceKind),
		RelatedPathIDs: appendUniqueStrings(nil, edge.RelatedPathIDs...),
		SourceIDs:      sourceIDs,
	}
	link.RelatedPathIDs = appendUniqueStrings(link.RelatedPathIDs, legacyFrom.RelatedPathIDs...)
	link.RelatedPathIDs = appendUniqueStrings(link.RelatedPathIDs, legacyTo.RelatedPathIDs...)
	if canonical := canonicalNodes[fromID]; canonical != nil {
		if !carrierSplit {
			link.SourceIDs = appendUniqueStrings(link.SourceIDs, canonical.SourceIDs...)
		}
		link.RelatedPathIDs = appendUniqueStrings(link.RelatedPathIDs, canonical.RelatedPathIDs...)
		link.ZoneName = firstNonEmpty(link.ZoneName, canonical.ZoneName)
	}
	if canonical := canonicalNodes[toID]; canonical != nil {
		// A canonical end-use node owns the union of every carrier-qualified
		// source that contributes to it.  Copying that union onto an individual
		// end-use -> carrier branch makes (for example) the electricity ribbon
		// claim the natural-gas meter too.  The legacy endpoint and edge above
		// already carry the exact branch meter; only omit the merged endpoint
		// union for this relation.
		if !carrierSplit {
			link.SourceIDs = appendUniqueStrings(link.SourceIDs, canonical.SourceIDs...)
		}
		link.RelatedPathIDs = appendUniqueStrings(link.RelatedPathIDs, canonical.RelatedPathIDs...)
		link.ZoneName = firstNonEmpty(link.ZoneName, canonical.ZoneName)
	}
	value := math.Abs(firstNonZero(edge.DisplayValue, edge.Value))
	switch {
	case legacyEnergyLinkIsLoadToEndUse(edge):
		link.FromID = toID
		link.ToID = fromID
		link.Relation = "load_to_end_use"
		link.FromValue = math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(legacyTo), value))
		link.FromUnit = firstNonEmpty(legacyTo.Unit, edge.Unit)
		link.ToUnit = firstNonEmpty(legacyFrom.Unit, edge.Unit)
		if canonicalMonthlyBasis && (strings.EqualFold(edge.Relation, "allocation") || edge.RuleID == energyRelationshipRuleAllocatedZoneLoad || edge.RuleID == energyRelationshipRuleAllocatedServicePathLoad) {
			link.ToValue = value
		} else if total := loadTotals[fromID]; total > 0 {
			endUseValue := math.Abs(energyExplanationEffectiveNodeValue(legacyFrom))
			if canonical := canonicalNodes[fromID]; canonical != nil {
				endUseValue = math.Abs(canonical.Value)
			}
			link.ToValue = endUseValue * link.FromValue / total
		} else {
			link.ToValue = math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(legacyFrom), value))
		}
		link.RatioKind = "load_to_site_energy"
		link.RatioLabel = "Load / site energy"
		setEnergyPathConversionRatioKind(&link, legacyFrom, canonicalNodes[fromID])
	case legacyEnergyLinkIsDriverToLoad(edge):
		link.FromID = toID
		link.ToID = fromID
		link.Relation = "driver_to_load"
		link.FromValue = math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(legacyTo), legacyTo.DisplayValue, value))
		link.ToValue = link.FromValue
		link.FromUnit = firstNonEmpty(legacyTo.Unit, edge.Unit)
		link.ToUnit = firstNonEmpty(legacyFrom.Unit, edge.Unit)
	case legacyEnergyLinkIsEndUseToCarrier(edge):
		link.FromID = toID
		link.ToID = fromID
		link.Relation = "end_use_to_carrier"
		if !endUsesWithLoads[toID] {
			link.Relation = "direct_end_use_to_carrier"
		}
		link.FromValue = math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(legacyTo), value))
		link.ToValue = link.FromValue
		link.FromUnit = firstNonEmpty(legacyTo.Unit, edge.Unit)
		link.ToUnit = firstNonEmpty(legacyFrom.Unit, edge.Unit)
	case legacyEnergyLinkIsSupportSupply(edge):
		link.FromID = toID
		link.ToID = fromID
		link.Relation = "support_supply"
		link.FromValue = math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(legacyTo), value))
		link.ToValue = link.FromValue
		link.FromUnit = firstNonEmpty(legacyTo.Unit, edge.Unit)
		link.ToUnit = firstNonEmpty(legacyFrom.Unit, edge.Unit)
	case strings.EqualFold(edge.Relation, "internal_gain_heat"):
		link.FromID = toID
		link.ToID = fromID
		link.Relation = "source_correspondence"
		link.FromValue = math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(legacyTo), legacyTo.DisplayValue, value))
		link.ToValue = math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(legacyFrom), value))
		link.FromUnit = firstNonEmpty(legacyTo.Unit, edge.Unit)
		link.ToUnit = firstNonEmpty(legacyFrom.Unit, edge.Unit)
		link.RatioKind = "source_correspondence"
		link.RatioLabel = "Thermal / site energy"
	case strings.EqualFold(edge.Relation, "residual"):
		link.Relation = "residual"
		if strings.HasPrefix(toID, "residual.") {
			link.FromID = toID
			link.ToID = fromID
			link.FromUnit = firstNonEmpty(legacyTo.Unit, edge.Unit)
			link.ToUnit = firstNonEmpty(legacyFrom.Unit, edge.Unit)
		} else {
			link.FromID = fromID
			link.ToID = toID
			link.FromUnit = firstNonEmpty(legacyFrom.Unit, edge.Unit)
			link.ToUnit = firstNonEmpty(legacyTo.Unit, edge.Unit)
		}
		residualValue := value
		if strings.HasPrefix(toID, "residual.") {
			residualValue = math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(legacyTo), value))
		} else if strings.HasPrefix(fromID, "residual.") {
			residualValue = math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(legacyFrom), value))
		}
		link.FromValue = residualValue
		link.ToValue = residualValue
	default:
		link.FromID = fromID
		link.ToID = toID
		link.Relation = "source_correspondence"
		link.FromValue = math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(legacyFrom), value))
		link.ToValue = math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(legacyTo), value))
		link.FromUnit = firstNonEmpty(legacyFrom.Unit, edge.Unit)
		link.ToUnit = firstNonEmpty(legacyTo.Unit, edge.Unit)
	}
	link.Basis = canonicalEnergyPathBasis(edge.Basis, edge.RuleID)
	if carrierSplit {
		sort.Strings(link.SourceIDs)
	}
	link.ID = energyPathLinkID(link)
	finalizeEnergyPathLinkRatio(&link)
	return link, link.FromID != "" && link.ToID != "" && link.Relation != ""
}

func setEnergyPathConversionRatioKind(link *EnergyPathLink, legacyEndUse EnergyExplanationNode, canonicalEndUse *EnergyExplanationNode) {
	service := energyCanonicalServiceKind(link.ServiceKind)
	carrier := strings.ToLower(strings.TrimSpace(legacyEndUse.Carrier))
	if canonicalEndUse != nil {
		// End-use nodes are always publicly carrier-neutral. The private ledger
		// retains just enough information to label a single-carrier conversion;
		// multiple branches deliberately use the generic load/site ratio.
		if len(canonicalEndUse.endUseCarriers) == 1 {
			carrier = strings.ToLower(strings.TrimSpace(canonicalEndUse.endUseCarriers[0]))
		} else {
			carrier = ""
		}
	}
	if carrier == "electricity" && (service == "cooling" || service == "heating") {
		link.RatioKind = "coefficient_of_performance"
		link.RatioLabel = "COP"
		return
	}
	if service == "heating" && energyPathCarrierUsesCombustionEfficiency(carrier) {
		link.RatioKind = "efficiency"
		link.RatioLabel = "Efficiency"
	}
}

func energyPathCarrierUsesCombustionEfficiency(carrier string) bool {
	switch carrier {
	case "natural_gas", "propane", "fuel_oil_1", "fuel_oil_2", "other_fuel_1", "other_fuel_2", "coal", "diesel", "gasoline":
		return true
	default:
		return false
	}
}

func mergeEnergyPathLink(links map[string]*EnergyPathLink, next EnergyPathLink) {
	current := links[next.ID]
	if current == nil {
		copy := next
		links[next.ID] = &copy
		return
	}
	current.FromValue = roundedEnergyNumber(current.FromValue + next.FromValue)
	current.ToValue = roundedEnergyNumber(current.ToValue + next.ToValue)
	current.SourceIDs = appendUniqueStrings(current.SourceIDs, next.SourceIDs...)
	current.RelatedPathIDs = appendUniqueStrings(current.RelatedPathIDs, next.RelatedPathIDs...)
	current.Explanation = firstNonEmpty(current.Explanation, next.Explanation)
	if current.ZoneName != next.ZoneName {
		current.ZoneName = ""
	}
}

func finalizeEnergyPathLinkRatio(link *EnergyPathLink) {
	link.FromValue = roundedEnergyNumber(link.FromValue)
	link.ToValue = roundedEnergyNumber(link.ToValue)
	if link.RatioKind == "" || link.ToValue == 0 {
		link.Ratio = 0
		return
	}
	link.Ratio = roundedEnergyNumber(link.FromValue / link.ToValue)
}

func energyPathLinkID(link EnergyPathLink) string {
	return "link." + canonicalEnergyPathPart(link.Relation) + "." + metricID(link.FromID) + "." + metricID(link.ToID)
}

func canonicalEnergyPathBasis(basis string, ruleID string) string {
	switch strings.ToLower(strings.TrimSpace(basis)) {
	case "reported_meter", "reported_variable", "integrated_rate", "heat_balance_share", "service_path_allocation", "zone_load_allocation", "direct_zone_energy", "derived_ratio", "residual":
		return strings.ToLower(strings.TrimSpace(basis))
	case "measured_meter", "sql_tabular":
		return "reported_meter"
	case "measured_energy_variable":
		return "reported_variable"
	case "measured_variable":
		return "direct_zone_energy"
	case "derived_balance":
		return "heat_balance_share"
	case "measured_meter_plus_zone_gain_variable":
		return "derived_ratio"
	case "allocated":
		if ruleID == energyRelationshipRuleAllocatedServicePathLoad {
			return "service_path_allocation"
		}
		return "zone_load_allocation"
	default:
		if ruleID == energyRelationshipRuleAllocatedServicePathLoad {
			return "service_path_allocation"
		}
		if ruleID == energyRelationshipRuleAllocatedZoneLoad {
			return "zone_load_allocation"
		}
		return "derived_ratio"
	}
}

func upgradeEnergyRelationshipRules(input []EnergyRelationshipRule) []EnergyRelationshipRule {
	if len(input) == 0 {
		input = energyRelationshipRuleCatalog()
	}
	out := make([]EnergyRelationshipRule, 0, len(input))
	for _, rule := range input {
		upgraded := rule
		upgraded.Basis = canonicalEnergyPathBasis(rule.Basis, rule.ID)
		switch rule.ID {
		case energyRelationshipRuleMeterEndUse, energyRelationshipRuleMeasuredEnergyVariable:
			upgraded.FromLevel, upgraded.ToLevel = "end_use", "carrier"
			upgraded.FromKind, upgraded.ToKind = rule.ToKind, rule.FromKind
		case energyRelationshipRuleMeasuredLoad, energyRelationshipRuleAllocatedZoneLoad, energyRelationshipRuleAllocatedServicePathLoad:
			upgraded.FromLevel, upgraded.ToLevel = "load", "end_use"
			upgraded.FromKind, upgraded.ToKind = rule.ToKind, rule.FromKind
		case energyRelationshipRuleHeatDriverBalance:
			upgraded.FromLevel, upgraded.ToLevel = "driver", "load"
			upgraded.FromKind, upgraded.ToKind = rule.ToKind, rule.FromKind
			upgraded.Basis = "heat_balance_share"
			upgraded.Formula = energyDriverAllocationFormula + "; " + energyDriverAllocationExplanation
		case energyRelationshipRuleInternalGainHeat:
			upgraded.FromLevel, upgraded.ToLevel = "driver", "end_use"
			upgraded.FromKind, upgraded.ToKind = rule.ToKind, rule.FromKind
		case energyRelationshipRuleOnsiteProduction, energyRelationshipRuleStorageDischarge:
			upgraded.FromLevel, upgraded.ToLevel = "support", "carrier"
			upgraded.FromKind, upgraded.ToKind = rule.ToKind, rule.FromKind
		case energyRelationshipRuleEnergyResidual:
			upgraded.FromLevel, upgraded.ToLevel = "residual", "carrier"
			upgraded.FromKind, upgraded.ToKind = rule.ToKind, rule.FromKind
		case energyRelationshipRuleHeatResidual:
			upgraded.FromLevel, upgraded.ToLevel = "residual", "load"
			upgraded.FromKind, upgraded.ToKind = rule.ToKind, rule.FromKind
		}
		out = append(out, upgraded)
	}
	return out
}

func legacyEnergyLinkIsLoadToEndUse(edge EnergyExplanationEdge) bool {
	return strings.EqualFold(edge.Relation, "delivered_load") || strings.EqualFold(edge.Relation, "allocation")
}

func legacyEnergyLinkIsDriverToLoad(edge EnergyExplanationEdge) bool {
	return strings.EqualFold(edge.Relation, "heat_driver")
}

func legacyEnergyLinkIsEndUseToCarrier(edge EnergyExplanationEdge) bool {
	return strings.EqualFold(edge.Relation, "meter_enduse") || strings.EqualFold(edge.Relation, "energy_variable")
}

func legacyEnergyLinkIsSupportSupply(edge EnergyExplanationEdge) bool {
	return strings.EqualFold(edge.Relation, "onsite_production") || strings.EqualFold(edge.Relation, "storage_discharge")
}

func legacyEnergyNodeIsCarrier(node EnergyExplanationNode) bool {
	return strings.EqualFold(node.Level, "carrier") || strings.EqualFold(node.EndUse, "total") || strings.Contains(strings.ToLower(node.ID), ".carrier.")
}

func legacyEnergyNodeIsSupport(node EnergyExplanationNode) bool {
	switch strings.ToLower(strings.TrimSpace(node.EndUse)) {
	case "generators", "storage_discharge", "onsite_production", "production":
		return true
	default:
		return strings.EqualFold(node.Level, "support")
	}
}

func energyExplanationDriverCategory(node EnergyExplanationNode) string {
	if value := strings.TrimSpace(node.DriverCategory); value != "" {
		return canonicalEnergyDriverCategory(value)
	}
	kind := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(node.Kind)), "heat."), "driver.")
	return canonicalEnergyDriverCategory(firstNonEmpty(kind, node.HeatCategory, "other"))
}

func energyExplanationThermalComponent(node EnergyExplanationNode) string {
	if value := strings.ToLower(strings.TrimSpace(node.ThermalComponent)); value == "sensible" || value == "latent" || value == "combined" {
		return value
	}
	text := strings.ToLower(strings.Join([]string{node.Kind, node.Label}, " "))
	if strings.Contains(text, "latent") {
		return "latent"
	}
	if strings.Contains(text, "sensible") {
		return "sensible"
	}
	return "combined"
}

func energyExplanationResidualScaleDomain(node EnergyExplanationNode) string {
	if node.ScaleDomain == "thermal" || node.ScaleDomain == "site" {
		return node.ScaleDomain
	}
	if node.Carrier != "" || strings.Contains(strings.ToLower(node.Kind+" "+node.ID), "energy") {
		return "site"
	}
	return "thermal"
}

func energyExplanationResidualDomain(node EnergyExplanationNode) string {
	if energyExplanationResidualScaleDomain(node) == "site" {
		return canonicalEnergyPathPart("site_" + firstNonEmpty(node.Carrier, "energy"))
	}
	return canonicalEnergyPathPart("thermal_" + firstNonEmpty(node.ServiceKind, "load"))
}

func energyExplanationKindSuffix(kind string) string {
	parts := strings.Split(strings.TrimSpace(kind), ".")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func canonicalEnergyPathPart(value string) string {
	return firstNonEmpty(metricID(value), "other")
}

func canonicalEnergyPathCategory(value string) string {
	parts := strings.Split(strings.TrimSpace(value), ".")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if token := metricID(part); token != "" {
			out = append(out, token)
		}
	}
	if len(out) == 0 {
		return "other"
	}
	return strings.Join(out, ".")
}

func firstNonZero(values ...float64) float64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func energyExplanationEffectiveNodeValue(node EnergyExplanationNode) float64 {
	if node.AllocationApplied {
		return node.AllocatedValue
	}
	if node.AllocatedValue != 0 {
		return node.AllocatedValue
	}
	if node.EffectiveValue != 0 {
		return node.EffectiveValue
	}
	if node.RawValue != 0 && node.Multiplier != 0 {
		return roundedEnergyNumber(node.RawValue * node.Multiplier)
	}
	return node.Value
}

func buildEnergyExplanationSummary(input any) EnergyExplanationSummary {
	var explanation EnergyExplanationResult
	var legacySummary EnergyExplanationSummary
	switch value := input.(type) {
	case EnergyExplanationV1:
		legacySummary = buildEnergyExplanationSummaryV1(value)
		explanation = UpgradeEnergyExplanationV1(value)
	case EnergyExplanationResult:
		explanation = value
		if energyExplanationNodesUseV1Levels(value.Nodes) {
			legacySummary = buildEnergyExplanationSummaryV1(EnergyExplanationV1{
				Schema:           energyExplanationV1Schema,
				Purpose:          value.Purpose,
				Frequency:        value.Frequency,
				AllocationPolicy: value.AllocationPolicy,
				Nodes:            value.Nodes,
				Edges:            value.Edges,
				Completeness:     value.Completeness,
				scope:            value.Scope,
			})
		}
		if value.Schema != energyExplanationSchema || energyExplanationNodesUseV1Levels(value.Nodes) {
			explanation = UpgradeEnergyExplanationV1(EnergyExplanationV1{
				Schema:           energyExplanationV1Schema,
				Purpose:          value.Purpose,
				Frequency:        value.Frequency,
				AllocationPolicy: value.AllocationPolicy,
				Periods:          value.Periods,
				Nodes:            value.Nodes,
				Edges:            value.Edges,
				Reconciliation:   value.Reconciliation,
				Sources:          value.Sources,
				Completeness:     value.Completeness,
				Warnings:         value.Warnings,
				scope:            value.Scope,
			})
		}
	default:
		return EnergyExplanationSummary{}
	}
	if explanation.Schema == "" || len(explanation.Nodes) == 0 {
		return EnergyExplanationSummary{}
	}

	summary := buildEnergyExplanationSummaryV2(explanation)
	if len(legacySummary.EnergyByCarrier) > 0 || len(legacySummary.EnergyByEndUse) > 0 || len(legacySummary.DeliveredLoadByService) > 0 || len(legacySummary.HeatDrivers) > 0 {
		summary.AllocationPolicy = legacySummary.AllocationPolicy
		summary.EnergyByCarrier = legacySummary.EnergyByCarrier
		summary.EnergyByEndUse = legacySummary.EnergyByEndUse
		summary.DeliveredLoadByService = legacySummary.DeliveredLoadByService
		summary.DerivedKPIs = legacySummary.DerivedKPIs
		summary.HeatDrivers = legacySummary.HeatDrivers
		summary.TopHeatDrivers = legacySummary.TopHeatDrivers
		if len(summary.TopZones) == 0 {
			summary.TopZones = legacySummary.TopZones
		}
	} else {
		summary.AllocationPolicy = firstNonEmpty(explanation.AllocationPolicy, PurposeAllocationPolicyDirectOnly)
	}
	return summary
}

func buildEnergyExplanationSummaryForPeriod(input any, periodID string) EnergyExplanationSummary {
	periodID = strings.TrimSpace(periodID)
	if periodID == "" || strings.EqualFold(periodID, "annual") {
		return buildEnergyExplanationSummary(input)
	}
	var explanation EnergyExplanationResult
	switch value := input.(type) {
	case EnergyExplanationV1:
		explanation = UpgradeEnergyExplanationV1(value)
	case EnergyExplanationResult:
		explanation = value
		if value.Schema != energyExplanationSchema || energyExplanationNodesUseV1Levels(value.Nodes) {
			explanation = UpgradeEnergyExplanationV1(EnergyExplanationV1{
				Schema:            energyExplanationV1Schema,
				Purpose:           value.Purpose,
				Frequency:         value.Frequency,
				AllocationPolicy:  value.AllocationPolicy,
				RelationshipRules: value.RelationshipRules,
				Periods:           value.Periods,
				Nodes:             value.Nodes,
				Edges:             value.Edges,
				Reconciliation:    value.Reconciliation,
				Sources:           value.Sources,
				Completeness:      value.Completeness,
				Warnings:          value.Warnings,
				scope:             value.Scope,
			})
		}
	default:
		return EnergyExplanationSummary{}
	}
	for _, period := range explanation.Periods {
		if !strings.EqualFold(period.ID, periodID) {
			continue
		}
		if period.Summary != nil && period.Summary.Schema == energyExplanationSummarySchema {
			return *period.Summary
		}
		selected := explanation
		selected.Periods = nil
		selected.Nodes = period.Nodes
		selected.Links = period.Links
		selected.Reconciliation = period.Reconciliation
		selected.Warnings = period.Warnings
		selected.ZoneContributions = period.ZoneContributions
		selected.legacyNodes = nil
		summary := buildEnergyExplanationSummaryV2(selected)
		summary.Period = period.ID
		summary.AllocationPolicy = firstNonEmpty(explanation.AllocationPolicy, PurposeAllocationPolicyDirectOnly)
		return summary
	}
	return EnergyExplanationSummary{}
}

func buildEnergyExplanationSummaryV2(explanation EnergyExplanationResult) EnergyExplanationSummary {
	summary := EnergyExplanationSummary{
		Schema:       energyExplanationSummarySchema,
		Period:       "annual",
		Scope:        normalizeEnergyExplanationScope(explanation.Scope),
		Completeness: explanation.Completeness,
	}
	drivers := map[string]*EnergyExplanationSummaryItem{}
	loads := map[string]*EnergyExplanationSummaryItem{}
	endUses := map[string]*EnergyExplanationSummaryItem{}
	carriers := map[string]*EnergyExplanationSummaryItem{}
	residuals := map[string]*EnergyExplanationSummaryItem{}
	zones := map[string]*EnergyExplanationSummaryItem{}
	for _, node := range explanation.Nodes {
		value := energyExplanationSummaryValue(node)
		if value == 0 {
			continue
		}
		switch node.Level {
		case "driver":
			addEnergyExplanationSummaryNodeV2(drivers, node.ID, node, value)
		case "load":
			addEnergyExplanationSummaryNodeV2(loads, node.ID, node, value)
		case "end_use":
			addEnergyExplanationSummaryNodeV2(endUses, node.ID, node, value)
		case "carrier":
			addEnergyExplanationSummaryNodeV2(carriers, node.ID, node, value)
		case "residual":
			addEnergyExplanationSummaryNodeV2(residuals, node.ID, node, value)
		}
	}
	for _, item := range explanation.ZoneContributions {
		copy := item
		key := firstNonEmpty(item.ZoneName, item.ID)
		zones[key] = &copy
	}
	for _, node := range explanation.legacyNodes {
		if len(explanation.ZoneContributions) > 0 {
			break
		}
		if !strings.EqualFold(node.Level, "heat") || strings.TrimSpace(node.ZoneName) == "" {
			continue
		}
		if summary.Scope.Kind == "zone" && !strings.EqualFold(strings.TrimSpace(node.ZoneName), summary.Scope.ZoneName) {
			continue
		}
		value := energyExplanationSummaryValue(node)
		if value == 0 {
			continue
		}
		addEnergyExplanationSummaryNode(zones, node.ZoneName, EnergyExplanationNode{
			ID:               "zone." + metricID(node.ZoneName),
			Level:            "zone",
			Kind:             "zone.driver",
			Label:            node.ZoneName,
			Value:            value,
			RawValue:         firstNonZero(node.RawValue, value),
			AllocatedValue:   firstNonZero(node.AllocatedValue, value),
			Unit:             node.Unit,
			ScaleDomain:      "thermal",
			ZoneName:         node.ZoneName,
			ServiceKind:      node.ServiceKind,
			Basis:            canonicalEnergyPathBasis(node.Basis, ""),
			AggregationBasis: summary.Scope.AggregationBasis,
			SourceIDs:        node.SourceIDs,
		}, value)
	}

	summary.Drivers = sortedEnergyExplanationSummaryItems(drivers)
	summary.Loads = sortedEnergyExplanationSummaryItems(loads)
	summary.EndUses = sortedEnergyExplanationSummaryItems(endUses)
	summary.Carriers = sortedEnergyExplanationSummaryItems(carriers)
	summary.Residuals = sortedEnergyExplanationSummaryItems(residuals)
	summary.TopZones = limitEnergyExplanationSummaryItems(sortedEnergyExplanationSummaryItems(zones), 5)
	for _, link := range explanation.Links {
		if link.Ratio == 0 || link.RatioKind == "" {
			continue
		}
		id := "ratio." + metricID(link.ID)
		kind := "ratio." + canonicalEnergyPathPart(link.RatioKind)
		label := firstNonEmpty(link.RatioLabel, link.RatioKind)
		if link.RatioKind == "coefficient_of_performance" && link.ServiceKind != "" {
			id = "kpi." + canonicalEnergyPathPart(link.ServiceKind) + "_cop"
			kind = id
			label = energyServiceLabel(link.ServiceKind) + " COP"
		}
		summary.Ratios = append(summary.Ratios, EnergyExplanationSummaryItem{
			ID:               id,
			Level:            "ratio",
			Kind:             kind,
			Label:            label,
			Value:            link.Ratio,
			RawValue:         link.FromValue,
			AllocatedValue:   link.ToValue,
			ServiceKind:      link.ServiceKind,
			Basis:            link.Basis,
			AggregationBasis: summary.Scope.AggregationBasis,
			NumeratorLabel:   link.FromID,
			NumeratorValue:   link.FromValue,
			NumeratorUnit:    link.FromUnit,
			DenominatorLabel: link.ToID,
			DenominatorValue: link.ToValue,
			DenominatorUnit:  link.ToUnit,
			SourceIDs:        appendUniqueStrings(nil, link.SourceIDs...),
		})
	}
	if len(summary.Ratios) == 0 {
		for _, item := range buildEnergyExplanationDerivedKPIs(summary.EndUses, summary.Loads) {
			item.Level = "ratio"
			item.Basis = "derived_ratio"
			item.RawValue = item.NumeratorValue
			item.AllocatedValue = item.DenominatorValue
			item.AggregationBasis = summary.Scope.AggregationBasis
			summary.Ratios = append(summary.Ratios, item)
		}
	}
	sort.SliceStable(summary.Ratios, func(i, j int) bool { return summary.Ratios[i].ID < summary.Ratios[j].ID })
	return summary
}

func addEnergyExplanationSummaryNodeV2(groups map[string]*EnergyExplanationSummaryItem, key string, node EnergyExplanationNode, value float64) {
	addEnergyExplanationSummaryNode(groups, key, node, value)
	if item := groups[key]; item != nil {
		item.ID = node.ID
	}
}

func (summary EnergyExplanationSummary) DriverItems() []EnergyExplanationSummaryItem {
	if len(summary.Drivers) > 0 {
		return summary.Drivers
	}
	return summary.HeatDrivers
}

func (summary EnergyExplanationSummary) LoadItems() []EnergyExplanationSummaryItem {
	if len(summary.Loads) > 0 {
		return summary.Loads
	}
	return summary.DeliveredLoadByService
}

func (summary EnergyExplanationSummary) EndUseItems() []EnergyExplanationSummaryItem {
	if len(summary.EndUses) > 0 {
		return summary.EndUses
	}
	return summary.EnergyByEndUse
}

func (summary EnergyExplanationSummary) CarrierItems() []EnergyExplanationSummaryItem {
	if len(summary.Carriers) > 0 {
		return summary.Carriers
	}
	return summary.EnergyByCarrier
}

func (summary EnergyExplanationSummary) RatioItems() []EnergyExplanationSummaryItem {
	if len(summary.Ratios) > 0 {
		return summary.Ratios
	}
	return summary.DerivedKPIs
}

func energyExplanationNodesUseV1Levels(nodes []EnergyExplanationNode) bool {
	for _, node := range nodes {
		if node.Level == "energy" || node.Level == "heat" {
			return true
		}
	}
	return false
}

// MarshalJSON guarantees the write side of the compatibility window: v2 and
// links only. Even a legacy-shaped value assembled by an older Go caller is
// normalized before it is serialized.
func (result EnergyExplanationResult) MarshalJSON() ([]byte, error) {
	if result.Schema == "" && result.Purpose == "" && len(result.Nodes) == 0 && len(result.Links) == 0 && len(result.Edges) == 0 && len(result.Periods) == 0 && len(result.AvailableZones) == 0 && len(result.ZoneResults) == 0 {
		return []byte("{}"), nil
	}
	if result.Schema != energyExplanationSchema || len(result.Links) == 0 && len(result.Edges) > 0 {
		result = UpgradeEnergyExplanationV1(EnergyExplanationV1{
			Schema:            firstNonEmpty(result.Schema, energyExplanationV1Schema),
			Purpose:           result.Purpose,
			Frequency:         result.Frequency,
			AllocationPolicy:  result.AllocationPolicy,
			RelationshipRules: result.RelationshipRules,
			Periods:           result.Periods,
			Nodes:             result.Nodes,
			Edges:             result.Edges,
			Reconciliation:    result.Reconciliation,
			Sources:           result.Sources,
			Completeness:      result.Completeness,
			Warnings:          result.Warnings,
			scope:             result.Scope,
		})
	}
	type wireResult struct {
		Schema            string                         `json:"schema"`
		Purpose           string                         `json:"purpose"`
		Scope             EnergyExplanationScope         `json:"scope"`
		Frequency         string                         `json:"frequency"`
		AllocationPolicy  string                         `json:"allocationPolicy,omitempty"`
		RelationshipRules []EnergyRelationshipRule       `json:"relationshipRules,omitempty"`
		Periods           []EnergyPeriod                 `json:"periods,omitempty"`
		Nodes             []EnergyExplanationNode        `json:"nodes"`
		Links             []EnergyPathLink               `json:"links"`
		Reconciliation    []EnergyReconciliation         `json:"reconciliation,omitempty"`
		Sources           []EnergyDataSource             `json:"sources,omitempty"`
		Completeness      EnergyCompleteness             `json:"completeness"`
		Warnings          []EnergyWarning                `json:"warnings,omitempty"`
		ZoneContributions []EnergyExplanationSummaryItem `json:"zoneContributions,omitempty"`
		AvailableZones    []string                       `json:"availableZones,omitempty"`
		ZoneResults       []EnergyExplanationZoneResult  `json:"zoneResults,omitempty"`
	}
	return json.Marshal(wireResult{
		Schema:            energyExplanationSchema,
		Purpose:           result.Purpose,
		Scope:             normalizeEnergyExplanationScope(result.Scope),
		Frequency:         result.Frequency,
		AllocationPolicy:  result.AllocationPolicy,
		RelationshipRules: result.RelationshipRules,
		Periods:           result.Periods,
		Nodes:             result.Nodes,
		Links:             result.Links,
		Reconciliation:    result.Reconciliation,
		Sources:           result.Sources,
		Completeness:      result.Completeness,
		Warnings:          result.Warnings,
		ZoneContributions: result.ZoneContributions,
		AvailableZones:    result.AvailableZones,
		ZoneResults:       result.ZoneResults,
	})
}

func (result *EnergyExplanationResult) UnmarshalJSON(data []byte) error {
	if result == nil {
		return fmt.Errorf("cannot unmarshal energy explanation into nil result")
	}
	var rawObject map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawObject); err != nil {
		return err
	}
	if len(rawObject) == 0 {
		*result = EnergyExplanationResult{}
		return nil
	}
	var header struct {
		Schema string          `json:"schema"`
		Links  json.RawMessage `json:"links"`
		Edges  json.RawMessage `json:"edges"`
	}
	_ = json.Unmarshal(data, &header)
	if strings.EqualFold(strings.TrimSpace(header.Schema), energyExplanationV1Schema) || len(header.Links) == 0 && len(header.Edges) > 0 {
		var legacy EnergyExplanationV1
		if err := json.Unmarshal(data, &legacy); err != nil {
			return err
		}
		*result = UpgradeEnergyExplanationV1(legacy)
		result.upgradedFromV1 = true
		return nil
	}
	type plainResult EnergyExplanationResult
	var decoded plainResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*result = EnergyExplanationResult(decoded)
	result.Schema = energyExplanationSchema
	result.Scope = normalizeEnergyExplanationScope(result.Scope)
	normalizeEnergyExplanationV2Nodes(result.Nodes, result.Scope)
	for index := range result.Periods {
		normalizeEnergyExplanationV2Nodes(result.Periods[index].Nodes, result.Scope)
	}
	for index := range result.ZoneResults {
		zone := &result.ZoneResults[index]
		zone.Scope = normalizeEnergyExplanationScope(zone.Scope)
		normalizeEnergyExplanationV2Nodes(zone.Nodes, zone.Scope)
		for periodIndex := range zone.Periods {
			normalizeEnergyExplanationV2Nodes(zone.Periods[periodIndex].Nodes, zone.Scope)
		}
	}
	return nil
}

func (period *EnergyPeriod) UnmarshalJSON(data []byte) error {
	type plainPeriod EnergyPeriod
	var decoded struct {
		plainPeriod
		Edges []EnergyExplanationEdge `json:"edges,omitempty"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*period = EnergyPeriod(decoded.plainPeriod)
	period.Edges = decoded.Edges
	return nil
}

func (period EnergyPeriod) MarshalJSON() ([]byte, error) {
	type wirePeriod struct {
		ID                string                         `json:"id"`
		Label             string                         `json:"label"`
		Kind              string                         `json:"kind"`
		Summary           *EnergyExplanationSummary      `json:"summary,omitempty"`
		Nodes             []EnergyExplanationNode        `json:"nodes,omitempty"`
		Links             []EnergyPathLink               `json:"links,omitempty"`
		Reconciliation    []EnergyReconciliation         `json:"reconciliation,omitempty"`
		Warnings          []EnergyWarning                `json:"warnings,omitempty"`
		ZoneContributions []EnergyExplanationSummaryItem `json:"zoneContributions,omitempty"`
	}
	return json.Marshal(wirePeriod{
		ID:                period.ID,
		Label:             period.Label,
		Kind:              period.Kind,
		Summary:           period.Summary,
		Nodes:             period.Nodes,
		Links:             period.Links,
		Reconciliation:    period.Reconciliation,
		Warnings:          period.Warnings,
		ZoneContributions: period.ZoneContributions,
	})
}

func (summary *EnergyExplanationSummary) UnmarshalJSON(data []byte) error {
	if summary == nil {
		return fmt.Errorf("cannot unmarshal energy explanation summary into nil result")
	}
	type plainSummary EnergyExplanationSummary
	var decoded struct {
		plainSummary
		AllocationPolicy       string                         `json:"allocationPolicy,omitempty"`
		EnergyByCarrier        []EnergyExplanationSummaryItem `json:"energyByCarrier,omitempty"`
		EnergyByEndUse         []EnergyExplanationSummaryItem `json:"energyByEndUse,omitempty"`
		DeliveredLoadByService []EnergyExplanationSummaryItem `json:"deliveredLoadByService,omitempty"`
		DerivedKPIs            []EnergyExplanationSummaryItem `json:"derivedKpis,omitempty"`
		HeatDrivers            []EnergyExplanationSummaryItem `json:"heatDrivers,omitempty"`
		TopHeatDrivers         []EnergyExplanationSummaryItem `json:"topHeatDrivers,omitempty"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.Schema == "" && decoded.Period == "" && decoded.Scope.Kind == "" && decoded.Scope.ZoneName == "" && decoded.Scope.AggregationBasis == "" &&
		len(decoded.Drivers)+len(decoded.Loads)+len(decoded.EndUses)+len(decoded.Carriers)+len(decoded.Ratios)+len(decoded.Residuals)+len(decoded.TopZones) == 0 &&
		len(decoded.EnergyByCarrier)+len(decoded.EnergyByEndUse)+len(decoded.DeliveredLoadByService)+len(decoded.DerivedKPIs)+len(decoded.HeatDrivers)+len(decoded.TopHeatDrivers) == 0 &&
		decoded.AllocationPolicy == "" && !energyCompletenessHasContent(decoded.Completeness) {
		*summary = EnergyExplanationSummary{}
		return nil
	}
	*summary = EnergyExplanationSummary(decoded.plainSummary)
	summary.Schema = energyExplanationSummarySchema
	summary.Scope = normalizeEnergyExplanationScope(summary.Scope)
	summary.AllocationPolicy = decoded.AllocationPolicy
	summary.EnergyByCarrier = decoded.EnergyByCarrier
	summary.EnergyByEndUse = decoded.EnergyByEndUse
	summary.DeliveredLoadByService = decoded.DeliveredLoadByService
	summary.DerivedKPIs = decoded.DerivedKPIs
	summary.HeatDrivers = decoded.HeatDrivers
	summary.TopHeatDrivers = decoded.TopHeatDrivers
	if len(summary.Carriers) == 0 {
		summary.Carriers = append([]EnergyExplanationSummaryItem(nil), decoded.EnergyByCarrier...)
	}
	if len(summary.EndUses) == 0 {
		summary.EndUses = append([]EnergyExplanationSummaryItem(nil), decoded.EnergyByEndUse...)
	}
	if len(summary.Loads) == 0 {
		summary.Loads = append([]EnergyExplanationSummaryItem(nil), decoded.DeliveredLoadByService...)
	}
	if len(summary.Ratios) == 0 {
		summary.Ratios = append([]EnergyExplanationSummaryItem(nil), decoded.DerivedKPIs...)
	}
	if len(summary.Drivers) == 0 {
		summary.Drivers = append([]EnergyExplanationSummaryItem(nil), decoded.HeatDrivers...)
	}
	normalizeEnergyExplanationSummaryItems(summary)
	return nil
}

func energyCompletenessHasContent(completeness EnergyCompleteness) bool {
	return completeness.Status != "" || completeness.MappedPercent != 0 ||
		completeness.EnergyUse.Level != "" || completeness.EnergyUse.Status != "" || completeness.EnergyUse.Found != 0 || completeness.EnergyUse.Total != 0 || completeness.EnergyUse.Message != "" ||
		completeness.DeliveredLoad.Level != "" || completeness.DeliveredLoad.Status != "" || completeness.DeliveredLoad.Found != 0 || completeness.DeliveredLoad.Total != 0 || completeness.DeliveredLoad.Message != "" ||
		completeness.HeatDrivers.Level != "" || completeness.HeatDrivers.Status != "" || completeness.HeatDrivers.Found != 0 || completeness.HeatDrivers.Total != 0 || completeness.HeatDrivers.Message != "" ||
		len(completeness.Items)+len(completeness.MissingCategories)+len(completeness.SourceAvailability) > 0
}

func (bundle *PurposeResultBundle) UnmarshalJSON(data []byte) error {
	if bundle == nil {
		return fmt.Errorf("cannot unmarshal purpose result bundle into nil result")
	}
	type plainBundle PurposeResultBundle
	var decoded plainBundle
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*bundle = PurposeResultBundle(decoded)
	if bundle.EnergyExplanation.Schema == energyExplanationSchema && len(bundle.EnergyExplanation.Nodes) > 0 {
		if bundle.EnergyExplanation.upgradedFromV1 || bundle.EnergyExplanationSummary.Schema == "" || len(bundle.EnergyExplanationSummary.Drivers)+len(bundle.EnergyExplanationSummary.Loads)+len(bundle.EnergyExplanationSummary.EndUses)+len(bundle.EnergyExplanationSummary.Carriers) == 0 {
			bundle.EnergyExplanationSummary = buildEnergyExplanationSummary(bundle.EnergyExplanation)
		}
	}
	return nil
}

func normalizeEnergyExplanationSummaryItems(summary *EnergyExplanationSummary) {
	collections := [][]EnergyExplanationSummaryItem{
		summary.Drivers,
		summary.Loads,
		summary.EndUses,
		summary.Carriers,
		summary.Ratios,
		summary.Residuals,
		summary.TopZones,
	}
	for collectionIndex := range collections {
		for itemIndex := range collections[collectionIndex] {
			item := &collections[collectionIndex][itemIndex]
			item.RawValue = firstNonZero(item.RawValue, item.Value)
			item.AllocatedValue = firstNonZero(item.AllocatedValue, item.Value)
			item.AggregationBasis = firstNonEmpty(item.AggregationBasis, summary.Scope.AggregationBasis)
		}
	}
}

func normalizeEnergyExplanationV2Nodes(nodes []EnergyExplanationNode, scope EnergyExplanationScope) {
	for index := range nodes {
		node := &nodes[index]
		node.AggregationBasis = firstNonEmpty(node.AggregationBasis, scope.AggregationBasis)
		node.RawValue = firstNonZero(node.RawValue, node.Value)
		if node.Multiplier == 0 {
			node.Multiplier = 1
		}
		node.EffectiveValue = firstNonZero(node.EffectiveValue, roundedEnergyNumber(node.RawValue*node.Multiplier), node.Value)
		node.AllocatedValue = firstNonZero(node.AllocatedValue, node.EffectiveValue, node.Value)
		if node.ScaleDomain == "" {
			switch node.Level {
			case "driver", "load":
				node.ScaleDomain = "thermal"
			default:
				node.ScaleDomain = "site"
			}
		}
	}
}
