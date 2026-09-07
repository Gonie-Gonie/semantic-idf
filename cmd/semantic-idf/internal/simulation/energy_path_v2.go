package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const energyPathRelationSourceCorrespondence = "source_correspondence"

// UpgradeEnergyExplanationV1 is the single compatibility boundary for stored
// energy-explanation/v1 payloads. The returned graph always follows the v2
// driver -> load -> end-use -> carrier direction and never needs a v1 renderer.
func UpgradeEnergyExplanationV1(input EnergyExplanationV1) EnergyExplanationResult {
	input = normalizeEnergyExplanationV1Units(input)
	scope := normalizeEnergyExplanationScope(input.scope)
	allocationPolicy := normalizePurposeAllocationPolicy(input.AllocationPolicy)
	zoneHVACAllocationEnabled := allocationPolicy == PurposeAllocationPolicyByServicePathLoadShare
	annualZoneHVACAllocation := energyPathZoneHVACAllocationPlan{}
	periodZoneHVACAllocations := map[string]energyPathZoneHVACAllocationPlan{}
	annualZoneAuxiliaryAllocation := energyPathZoneAuxiliaryAllocationPlan{}
	periodZoneAuxiliaryAllocations := map[string]energyPathZoneAuxiliaryAllocationPlan{}
	if zoneHVACAllocationEnabled {
		annualZoneHVACAllocation = buildEnergyPathZoneHVACAllocationPlan(input.Nodes, input.Edges, input.zoneDirectUseSeries, "annual", "annual", input.canonicalMonthlyBasis)
		annualZoneAuxiliaryAllocation = buildEnergyPathZoneAuxiliaryAllocationPlan(input.Nodes, input.zoneDirectUseSeries, input.servicePathIndex, "annual", "annual", input.canonicalMonthlyBasis)
		monthlyZoneHVACAllocations := []energyPathZoneHVACAllocationPlan{}
		monthlyZoneAuxiliaryAllocations := []energyPathZoneAuxiliaryAllocationPlan{}
		for _, period := range input.Periods {
			plan := buildEnergyPathZoneHVACAllocationPlan(period.Nodes, period.Edges, input.zoneDirectUseSeries, period.ID, period.Kind, input.canonicalMonthlyBasis)
			periodZoneHVACAllocations[strings.ToLower(strings.TrimSpace(period.ID))] = plan
			auxiliaryPlan := buildEnergyPathZoneAuxiliaryAllocationPlan(period.Nodes, input.zoneDirectUseSeries, input.servicePathIndex, period.ID, period.Kind, input.canonicalMonthlyBasis)
			periodZoneAuxiliaryAllocations[strings.ToLower(strings.TrimSpace(period.ID))] = auxiliaryPlan
			if strings.EqualFold(strings.TrimSpace(period.Kind), "monthly") {
				monthlyZoneHVACAllocations = append(monthlyZoneHVACAllocations, plan)
				monthlyZoneAuxiliaryAllocations = append(monthlyZoneAuxiliaryAllocations, auxiliaryPlan)
			}
		}
		if input.canonicalMonthlyBasis && len(monthlyZoneHVACAllocations) > 0 {
			monthlyPlan := aggregateEnergyPathZoneHVACAllocationPlans(monthlyZoneHVACAllocations)
			annualZoneHVACAllocation = energyPathZoneHVACAllocationPlanWithAnnualFallback(monthlyPlan, annualZoneHVACAllocation, input.Nodes)
		}
		if input.canonicalMonthlyBasis && len(monthlyZoneAuxiliaryAllocations) > 0 {
			monthlyPlan := aggregateEnergyPathZoneAuxiliaryAllocationPlans(monthlyZoneAuxiliaryAllocations)
			annualZoneAuxiliaryAllocation = energyPathZoneAuxiliaryAllocationPlanWithAnnualFallback(monthlyPlan, annualZoneAuxiliaryAllocation, input.Nodes)
		}
	}
	var directZoneSeries []energyExplanationSeries
	if scope.Kind == "zone" {
		directZoneSeries = energyPathDirectZoneSeriesForScope(input.zoneDirectUseSeries, scope.ZoneName)
	}
	var directNodes []EnergyExplanationNode
	var directEdges []EnergyExplanationEdge
	if scope.Kind == "zone" {
		directNodes, directEdges = buildEnergyPathDirectZoneLegacyGraph(directZoneSeries, "annual", "annual", input.canonicalMonthlyBasis)
	}
	annualLegacyNodes := append(append([]EnergyExplanationNode(nil), input.Nodes...), directNodes...)
	annualInputEdges := append([]EnergyExplanationEdge(nil), input.Edges...)
	if scope.Kind == "building" && len(input.buildingHVACAllocationEdges) > 0 {
		annualInputEdges = append([]EnergyExplanationEdge(nil), input.buildingHVACAllocationEdges...)
	}
	if scope.Kind == "zone" && zoneHVACAllocationEnabled {
		annualInputEdges = applyEnergyPathZoneHVACAllocationPlan(annualInputEdges, input.Nodes, annualZoneHVACAllocation)
		annualInputEdges = applyEnergyPathZoneAuxiliaryAllocationPlan(annualInputEdges, annualZoneAuxiliaryAllocation)
	}
	annualLegacyEdges := append(annualInputEdges, directEdges...)
	annualLegacyEdges = appendEnergyPathDirectZoneHVACEdges(annualLegacyEdges, annualLegacyNodes)
	annualLegacyEdges = appendEnergyPathDirectZoneCorrespondenceEdges(annualLegacyEdges, annualLegacyNodes)
	annotatedSources := annotateLegacyEnergyDriverSources(input.Sources, annualLegacyNodes)
	allLegacyNodes := append([]EnergyExplanationNode(nil), annualLegacyNodes...)
	for _, period := range input.Periods {
		var periodDirectNodes []EnergyExplanationNode
		if scope.Kind == "zone" {
			periodDirectNodes, _ = buildEnergyPathDirectZoneLegacyGraph(directZoneSeries, period.ID, period.Kind, input.canonicalMonthlyBasis)
		}
		allLegacyNodes = append(allLegacyNodes, period.Nodes...)
		allLegacyNodes = append(allLegacyNodes, periodDirectNodes...)
	}
	annotatedSources = annotateEnergyPathWaterContextSources(annotatedSources, allLegacyNodes)
	annotatedSources = annotateLegacyEnergyLoadDetailSources(annotatedSources, allLegacyNodes)
	legacyNodes := foldLegacyEnergyLoadDetailNodes(inferLegacyEnergyDriverProjectionGuards(annualLegacyNodes, annotatedSources))
	nodes, links := upgradeEnergyExplanationGraph(legacyNodes, annualLegacyEdges, annotatedSources, scope, allocationPolicy, input.canonicalMonthlyBasis)
	reconciliation, warnings := upgradeEnergyExplanationAccounting(input.Reconciliation, input.Warnings, legacyNodes, annualLegacyEdges, scope, allocationPolicy)
	reconciliation = filterEnergyPathNonSiteEnergyReconciliation(reconciliation)
	reconciliation = removeEnergyPathWaterReconciliation(reconciliation)
	reconciliation = reconcileEnergyPathCarrierTotals(nodes, links, reconciliation, "annual")
	reconciliation = filterEnergyPathContextOnlyWaterReconciliation(reconciliation, nodes)
	nodes, links = rebuildEnergyPathCarrierResidualPresentation(nodes, links, reconciliation, "annual")
	completeness := normalizeEnergyPathWaterContextCompleteness(input.Completeness, annotatedSources, reconciliation)
	completeness = energyPathCompletenessFromCarrierReconciliation(completeness, scope, reconciliation)
	periods := make([]EnergyPeriod, 0, len(input.Periods))
	for _, period := range input.Periods {
		var periodDirectNodes []EnergyExplanationNode
		var periodDirectEdges []EnergyExplanationEdge
		if scope.Kind == "zone" {
			periodDirectNodes, periodDirectEdges = buildEnergyPathDirectZoneLegacyGraph(directZoneSeries, period.ID, period.Kind, input.canonicalMonthlyBasis)
		}
		periodLegacyNodes := append(append([]EnergyExplanationNode(nil), period.Nodes...), periodDirectNodes...)
		periodInputEdges := append([]EnergyExplanationEdge(nil), period.Edges...)
		if scope.Kind == "building" {
			if preserved := input.buildingHVACAllocationPeriodEdges[strings.ToLower(strings.TrimSpace(period.ID))]; len(preserved) > 0 {
				periodInputEdges = append([]EnergyExplanationEdge(nil), preserved...)
			}
		}
		if scope.Kind == "zone" && zoneHVACAllocationEnabled {
			periodInputEdges = applyEnergyPathZoneHVACAllocationPlan(periodInputEdges, period.Nodes, periodZoneHVACAllocations[strings.ToLower(strings.TrimSpace(period.ID))])
			periodInputEdges = applyEnergyPathZoneAuxiliaryAllocationPlan(periodInputEdges, periodZoneAuxiliaryAllocations[strings.ToLower(strings.TrimSpace(period.ID))])
		}
		periodLegacyEdges := append(periodInputEdges, periodDirectEdges...)
		periodLegacyEdges = appendEnergyPathDirectZoneHVACEdges(periodLegacyEdges, periodLegacyNodes)
		periodLegacyEdges = appendEnergyPathDirectZoneCorrespondenceEdges(periodLegacyEdges, periodLegacyNodes)
		legacyPeriodNodes := foldLegacyEnergyLoadDetailNodes(inferLegacyEnergyDriverProjectionGuards(periodLegacyNodes, annotatedSources))
		periodNodes, periodLinks := upgradeEnergyExplanationGraph(legacyPeriodNodes, periodLegacyEdges, annotatedSources, scope, allocationPolicy, input.canonicalMonthlyBasis)
		periodReconciliation, periodWarnings := upgradeEnergyExplanationAccounting(period.Reconciliation, period.Warnings, legacyPeriodNodes, periodLegacyEdges, scope, allocationPolicy)
		periodReconciliation = filterEnergyPathNonSiteEnergyReconciliation(periodReconciliation)
		periodReconciliation = removeEnergyPathWaterReconciliation(periodReconciliation)
		periodReconciliation = reconcileEnergyPathCarrierTotals(periodNodes, periodLinks, periodReconciliation, period.ID)
		periodReconciliation = filterEnergyPathContextOnlyWaterReconciliation(periodReconciliation, periodNodes)
		periodNodes, periodLinks = rebuildEnergyPathCarrierResidualPresentation(periodNodes, periodLinks, periodReconciliation, period.ID)
		periodZoneContributions := buildEnergyExplanationZoneContributions(legacyPeriodNodes, periodLegacyEdges, scope, allocationPolicy)
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
			Completeness:      completeness,
			Warnings:          periodWarnings,
			ZoneContributions: periodZoneContributions,
		})
		periodSummary.Period = period.ID
		periodSummary.AllocationPolicy = allocationPolicy
		upgradedPeriod.Summary = &periodSummary
		periods = append(periods, upgradedPeriod)
	}
	sources := filterEnergyDataSourcesForV2(annotatedSources, legacyNodes, annualLegacyEdges, nodes, links, reconciliation, scope, allocationPolicy)
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
		Completeness:      completeness,
		Warnings:          warnings,
		ZoneContributions: buildEnergyExplanationZoneContributions(legacyNodes, annualLegacyEdges, scope, allocationPolicy),
		AvailableZones:    availableZones,
		legacyNodes:       scopedEnergyExplanationLegacyNodes(legacyNodes, annualLegacyEdges, scope, allocationPolicy),
	}
	if input.canonicalMonthlyBasis {
		applyCanonicalMonthlyBasisToEnergyPathResult(&result)
		if scope.Kind == "zone" && zoneHVACAllocationEnabled {
			applyEnergyPathAnnualZoneHVACOverrides(&result, nodes, links, annualZoneHVACAllocation)
			applyEnergyPathAnnualZoneAuxiliaryOverrides(&result, nodes, links, annualZoneAuxiliaryAllocation)
		}
	}
	applyEnergyPathDirectZoneCoverage(&result)
	sortEnergyPathPeriods(result.Periods)
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
	if zoneHVACAllocationEnabled {
		appendEnergyPathZoneHVACAllocationAccounting(&result, annualZoneHVACAllocation, periodZoneHVACAllocations, input.canonicalMonthlyBasis)
		appendEnergyPathZoneAuxiliaryAllocationAccounting(&result, annualZoneAuxiliaryAllocation, periodZoneAuxiliaryAllocations, input.canonicalMonthlyBasis)
	}
	refreshEnergyPathQuality(&result)
	orderEnergyPathAccounting(&result)
	return result
}

func energyPathDirectZoneSeriesForScope(series []energyExplanationSeries, zoneName string) []energyExplanationSeries {
	zoneName = strings.TrimSpace(zoneName)
	if zoneName == "" || len(series) == 0 {
		return nil
	}
	out := make([]energyExplanationSeries, 0, len(series))
	for _, item := range series {
		if strings.EqualFold(strings.TrimSpace(item.ZoneName), zoneName) {
			out = append(out, item)
		}
	}
	return out
}

// buildEnergyPathDirectZoneLegacyGraph carries runtime-only zone-keyed energy
// variables across the frozen v1 builder boundary. The generated carrier is an
// observed subtotal of these direct branches, never a copy or allocation of a
// facility meter.
func buildEnergyPathDirectZoneLegacyGraph(series []energyExplanationSeries, periodID string, periodKind string, canonicalMonthlyBasis bool) ([]EnergyExplanationNode, []EnergyExplanationEdge) {
	if len(series) == 0 {
		return nil, nil
	}
	endUses := map[string]*EnergyExplanationNode{}
	carriers := map[string]*EnergyExplanationNode{}
	for _, original := range series {
		item := canonicalEnergyExplanationSeries(original)
		if item.Stage != "end_use" || strings.TrimSpace(item.ZoneName) == "" {
			continue
		}
		value, rawValue, sourceIDs, ok := energyPathDirectZonePeriodValue(item, periodID, periodKind, canonicalMonthlyBasis)
		value = math.Abs(value)
		rawValue = math.Abs(rawValue)
		if !ok || value == 0 {
			continue
		}
		zoneName := strings.TrimSpace(item.ZoneName)
		zoneToken := firstNonEmpty(energyExplanationZoneSuffix(zoneName), metricID(zoneName), "zone")
		carrier := canonicalEnergyPathCarrier(firstNonEmpty(item.Carrier, "other"))
		endUse := firstNonEmpty(item.EndUse, energyExplanationKindSuffix(item.Kind), "other")
		endUseToken := canonicalEnergyPathPart(endUse)
		endUseID := strings.Join([]string{"energy", "direct_zone", "end_use", endUseToken, carrier, zoneToken}, ".")
		carrierID := strings.Join([]string{"energy", "direct_zone", "carrier", carrier, zoneToken}, ".")

		node := endUses[endUseID]
		if node == nil {
			node = &EnergyExplanationNode{
				ID:                  endUseID,
				Level:               "energy",
				Kind:                firstNonEmpty(item.Kind, "energy."+endUseToken),
				Label:               firstNonEmpty(item.Label, canonicalEnergyPathEndUseLabel(endUseToken)),
				Unit:                item.Unit,
				Period:              periodID,
				ZoneName:            zoneName,
				ServiceKind:         item.ServiceKind,
				Carrier:             carrier,
				EndUse:              endUse,
				MeterHierarchyLevel: "zone_direct_use",
				Basis:               "direct_zone_energy",
				RelatedEntityIDs:    appendUniqueStrings(nil, item.RelatedEntityIDs...),
			}
			endUses[endUseID] = node
		}
		node.Value = roundedEnergyNumber(node.Value + value)
		node.RawValue = roundedEnergyNumber(node.RawValue + rawValue)
		node.EffectiveValue = roundedEnergyNumber(node.EffectiveValue + value)
		node.AllocatedValue = node.EffectiveValue
		node.DisplayValue = node.Value
		node.SourceIDs = appendUniqueStrings(node.SourceIDs, sourceIDs...)
		node.RelatedEntityIDs = appendUniqueStrings(node.RelatedEntityIDs, item.RelatedEntityIDs...)
		if node.RawValue != 0 {
			node.Multiplier = roundedEnergyNumber(node.EffectiveValue / node.RawValue)
		}
		if node.Multiplier == 0 {
			node.Multiplier = 1
		}

		carrierNode := carriers[carrierID]
		if carrierNode == nil {
			carrierNode = &EnergyExplanationNode{
				ID:                  carrierID,
				Level:               "carrier",
				Kind:                "energy." + carrier + ".direct_zone_subtotal",
				Label:               energyCarrierLabel(carrier) + " observed direct-use subtotal",
				Unit:                item.Unit,
				Period:              periodID,
				ZoneName:            zoneName,
				Carrier:             carrier,
				EndUse:              "total",
				MeterHierarchyLevel: "zone_direct_subtotal",
				Basis:               "direct_zone_energy",
				Multiplier:          1,
			}
			carriers[carrierID] = carrierNode
		}
		carrierNode.Value = roundedEnergyNumber(carrierNode.Value + value)
		carrierNode.RawValue = roundedEnergyNumber(carrierNode.RawValue + rawValue)
		carrierNode.EffectiveValue = roundedEnergyNumber(carrierNode.EffectiveValue + value)
		carrierNode.AllocatedValue = carrierNode.EffectiveValue
		carrierNode.DisplayValue = carrierNode.Value
		carrierNode.SourceIDs = appendUniqueStrings(carrierNode.SourceIDs, sourceIDs...)
		if carrierNode.RawValue != 0 {
			carrierNode.Multiplier = roundedEnergyNumber(carrierNode.EffectiveValue / carrierNode.RawValue)
		}
	}

	nodes := make([]EnergyExplanationNode, 0, len(endUses)+len(carriers))
	endUseIDs := make([]string, 0, len(endUses))
	for id := range endUses {
		endUseIDs = append(endUseIDs, id)
	}
	carrierIDs := make([]string, 0, len(carriers))
	for id := range carriers {
		carrierIDs = append(carrierIDs, id)
	}
	sort.Strings(endUseIDs)
	sort.Strings(carrierIDs)
	for _, id := range carrierIDs {
		sort.Strings(carriers[id].SourceIDs)
		nodes = append(nodes, *carriers[id])
	}
	for _, id := range endUseIDs {
		sort.Strings(endUses[id].SourceIDs)
		sort.Strings(endUses[id].RelatedEntityIDs)
		nodes = append(nodes, *endUses[id])
	}
	edges := make([]EnergyExplanationEdge, 0, len(endUseIDs))
	for _, endUseID := range endUseIDs {
		endUse := endUses[endUseID]
		carrierID := strings.Join([]string{"energy", "direct_zone", "carrier", canonicalEnergyPathCarrier(endUse.Carrier), firstNonEmpty(energyExplanationZoneSuffix(endUse.ZoneName), metricID(endUse.ZoneName), "zone")}, ".")
		edges = append(edges, EnergyExplanationEdge{
			ID:          edgeID("direct_zone_energy", periodID, carrierID, endUseID),
			FromID:      carrierID,
			ToID:        endUseID,
			Value:       endUse.Value,
			Unit:        endUse.Unit,
			Period:      periodID,
			Relation:    "energy_variable",
			Basis:       "direct_zone_energy",
			Formula:     "sum of exact zone-keyed direct-use energy variables",
			RuleID:      energyRelationshipRuleMeasuredEnergyVariable,
			SourceIDs:   appendUniqueStrings(nil, endUse.SourceIDs...),
			ZoneName:    endUse.ZoneName,
			ServiceKind: energyCanonicalServiceKind(endUse.EndUse),
		})
	}
	return nodes, edges
}

func energyPathDirectZonePeriodValue(item energyExplanationSeries, periodID string, periodKind string, canonicalMonthlyBasis bool) (float64, float64, []string, bool) {
	periodKind = strings.ToLower(strings.TrimSpace(periodKind))
	periodID = strings.TrimSpace(periodID)
	value := 0.0
	rawValue := 0.0
	sourceIDs := item.SourceIDs
	ok := true
	switch periodKind {
	case "monthly":
		index, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(periodID), "M"))
		if err != nil {
			return 0, 0, nil, false
		}
		value, ok = item.Monthly[index]
		rawValue = item.RawMonthly[index]
		sourceIDs = energyExplanationPeriodSourceIDs(item.MonthlySourceIDs, item.SourceIDs)
	case "daily":
		index, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(periodID), "D"))
		if err != nil {
			return 0, 0, nil, false
		}
		value, ok = item.Daily[index]
		rawValue = item.RawDaily[index]
		sourceIDs = energyExplanationPeriodSourceIDs(item.DailySourceIDs, item.SourceIDs)
	case "hourly":
		index, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(periodID), "H"))
		if err != nil {
			return 0, 0, nil, false
		}
		value, ok = item.Hourly[index]
		rawValue = item.RawHourly[index]
		sourceIDs = energyExplanationPeriodSourceIDs(item.HourlySourceIDs, item.SourceIDs)
	case "selected_range":
		value = item.SelectedRange
		rawValue = item.RawSelectedRange
		ok = item.HasSelectedRange
		sourceIDs = energyExplanationPeriodSourceIDs(item.SelectedRangeSourceIDs, item.SourceIDs)
	default:
		if canonicalMonthlyBasis && len(item.Monthly) > 0 {
			value = sumEnergyExplanationPeriodValues(item.Monthly)
			rawValue = sumEnergyExplanationPeriodValues(item.RawMonthly)
			sourceIDs = energyExplanationPeriodSourceIDs(item.MonthlySourceIDs, item.SourceIDs)
		} else {
			value = item.Total
			rawValue = item.RawTotal
			sourceIDs = energyExplanationPeriodSourceIDs(item.AnnualSourceIDs, item.SourceIDs)
		}
	}
	if rawValue == 0 && value != 0 {
		rawValue = energyExplanationRawValue(value, item.EffectiveMultiplier)
	}
	return roundedEnergyNumber(value), roundedEnergyNumber(rawValue), appendUniqueStrings(nil, sourceIDs...), ok
}

func appendEnergyPathDirectZoneHVACEdges(edges []EnergyExplanationEdge, nodes []EnergyExplanationNode) []EnergyExplanationEdge {
	existing := map[string]bool{}
	for _, edge := range edges {
		if legacyEnergyLinkIsLoadToEndUse(edge) {
			existing[edge.FromID+"|"+edge.ToID] = true
		}
	}
	directEndUses := make([]EnergyExplanationNode, 0)
	loads := make([]EnergyExplanationNode, 0)
	for _, node := range nodes {
		switch {
		case energyPathLegacyNodeIsDirectZoneEnergy(node) && !legacyEnergyNodeIsCarrier(node):
			service := energyCanonicalServiceKind(firstNonEmpty(node.ServiceKind, node.EndUse, energyExplanationKindSuffix(node.Kind)))
			if service == "cooling" || service == "heating" {
				directEndUses = append(directEndUses, node)
			}
		case strings.EqualFold(strings.TrimSpace(node.Level), "load"):
			loads = append(loads, node)
		}
	}
	sort.SliceStable(directEndUses, func(i, j int) bool { return directEndUses[i].ID < directEndUses[j].ID })
	sort.SliceStable(loads, func(i, j int) bool { return loads[i].ID < loads[j].ID })
	for _, endUse := range directEndUses {
		service := energyCanonicalServiceKind(firstNonEmpty(endUse.ServiceKind, endUse.EndUse, energyExplanationKindSuffix(endUse.Kind)))
		for _, load := range loads {
			if !strings.EqualFold(strings.TrimSpace(load.ZoneName), strings.TrimSpace(endUse.ZoneName)) ||
				energyCanonicalServiceKind(firstNonEmpty(load.ServiceKind, energyExplanationKindSuffix(load.Kind))) != service {
				continue
			}
			key := endUse.ID + "|" + load.ID
			if existing[key] {
				continue
			}
			value := math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(load), load.Value))
			edges = append(edges, EnergyExplanationEdge{
				ID:             edgeID("direct_zone_load", firstNonEmpty(load.Period, endUse.Period), endUse.ID, load.ID),
				FromID:         endUse.ID,
				ToID:           load.ID,
				Value:          value,
				Unit:           load.Unit,
				Period:         firstNonEmpty(load.Period, endUse.Period),
				Relation:       "delivered_load",
				Basis:          "direct_zone_energy",
				Formula:        "exact zone HVAC component energy related to the matching measured zone service load",
				RuleID:         energyRelationshipRuleMeasuredLoad,
				SourceIDs:      appendUniqueStrings(append([]string(nil), endUse.SourceIDs...), load.SourceIDs...),
				ZoneName:       endUse.ZoneName,
				ServiceKind:    service,
				RelatedPathIDs: appendUniqueStrings(append([]string(nil), endUse.RelatedPathIDs...), load.RelatedPathIDs...),
			})
			existing[key] = true
		}
	}
	return edges
}

func appendEnergyPathDirectZoneCorrespondenceEdges(edges []EnergyExplanationEdge, nodes []EnergyExplanationNode) []EnergyExplanationEdge {
	if len(nodes) == 0 {
		return edges
	}
	existing := map[string]bool{}
	for _, edge := range edges {
		if legacyEnergyLinkIsSourceCorrespondence(edge) {
			existing[edge.FromID+"|"+edge.ToID] = true
			existing[edge.ToID+"|"+edge.FromID] = true
		}
	}
	for _, endUse := range nodes {
		if !energyPathLegacyNodeIsDirectZoneEnergy(endUse) || legacyEnergyNodeIsCarrier(endUse) || strings.TrimSpace(endUse.ZoneName) == "" {
			continue
		}
		endUseKind := canonicalEnergyPathEndUse(firstNonEmpty(endUse.EndUse, energyExplanationKindSuffix(endUse.Kind)))
		if endUseKind != "lighting" && endUseKind != "equipment" {
			continue
		}
		for _, driver := range nodes {
			if !strings.EqualFold(strings.TrimSpace(driver.ZoneName), strings.TrimSpace(endUse.ZoneName)) || !energyPathDirectUseDriverMatchesEndUse(driver, endUseKind) {
				continue
			}
			key := endUse.ID + "|" + driver.ID
			if existing[key] {
				continue
			}
			value := math.Abs(firstNonZero(driver.DisplayValue, energyExplanationEffectiveNodeValue(driver), driver.Value))
			edges = append(edges, EnergyExplanationEdge{
				ID:           edgeID("source_correspondence", firstNonEmpty(driver.Period, endUse.Period), endUse.ID, driver.ID),
				FromID:       endUse.ID,
				ToID:         driver.ID,
				Value:        value,
				DisplayValue: value,
				Unit:         driver.Unit,
				Period:       firstNonEmpty(driver.Period, endUse.Period),
				Relation:     energyPathRelationSourceCorrespondence,
				Basis:        "direct_zone_energy",
				Formula:      "related zone-keyed energy use and thermal effect; non-flow correspondence",
				RuleID:       energyRelationshipRuleInternalGainHeat,
				SourceIDs:    appendUniqueStrings(append([]string(nil), endUse.SourceIDs...), driver.SourceIDs...),
				ZoneName:     endUse.ZoneName,
				ServiceKind:  driver.ServiceKind,
			})
			existing[key] = true
		}
	}
	return edges
}

func energyPathDirectUseDriverMatchesEndUse(node EnergyExplanationNode, endUse string) bool {
	level := strings.ToLower(strings.TrimSpace(node.Level))
	if level != "heat" && level != "driver" {
		return false
	}
	category := canonicalEnergyDriverCategory(firstNonEmpty(node.DriverCategory, node.Kind))
	return endUse == "lighting" && category == energyDriverCategoryLighting || endUse == "equipment" && category == energyDriverCategoryEquipment
}

func applyEnergyPathDirectZoneCoverage(result *EnergyExplanationResult) {
	if result == nil || result.Scope.Kind != "zone" {
		return
	}
	// Exact-zone variables provide, at most, an observed subset of Stage 3
	// energy. Even when every requested variable is absent or explicitly zero,
	// a selected zone must not inherit Building-level "complete" coverage.
	result.Completeness = energyPathDirectZonePartialCompleteness(result.Completeness)
	result.Warnings = appendEnergyDriverWarning(result.Warnings, energyPathDirectZoneCoverageWarning(result.Scope.ZoneName, "annual"))
	markEnergyPathDirectZoneReconciliationPartial(result.Reconciliation, result.Links, result.Scope.ZoneName)
	for index := range result.Periods {
		period := &result.Periods[index]
		period.Warnings = appendEnergyDriverWarning(period.Warnings, energyPathDirectZoneCoverageWarning(result.Scope.ZoneName, period.ID))
		markEnergyPathDirectZoneReconciliationPartial(period.Reconciliation, period.Links, result.Scope.ZoneName)
		if period.Summary != nil {
			period.Summary.Completeness = energyPathDirectZonePartialCompleteness(period.Summary.Completeness)
		}
	}
}

func energyPathDirectZonePartialCompleteness(input EnergyCompleteness) EnergyCompleteness {
	// EnergyExplanationV1 is a value at the compatibility boundary, but its
	// completeness slices still share backing arrays with the caller. Clone
	// before annotating the scoped result so the frozen v1 payload stays intact.
	originalItems := append([]EnergyCompletenessLevel(nil), input.Items...)
	originalMissing := append([]string(nil), input.MissingCategories...)
	originalAvailability := append([]EnergySourceAvailabilityEntry(nil), input.SourceAvailability...)
	input.Status = "partial"
	input.MappedPercent = 0
	input.EnergyUse.Level = "energy"
	input.EnergyUse.Status = "partial"
	input.EnergyUse.Found = 0
	input.EnergyUse.Total = 0
	input.EnergyUse.Message = "Zone energy use combines exact zone-keyed observations with explicitly allocated central HVAC energy; a complete zone carrier total is not available."
	input.Items = make([]EnergyCompletenessLevel, 0, len(originalItems)+1)
	foundEnergyItem := false
	for _, item := range originalItems {
		if strings.EqualFold(strings.TrimSpace(item.Level), "energy") {
			if !foundEnergyItem {
				input.Items = append(input.Items, input.EnergyUse)
				foundEnergyItem = true
			}
			continue
		}
		input.Items = append(input.Items, item)
	}
	if !foundEnergyItem {
		input.Items = append([]EnergyCompletenessLevel{input.EnergyUse}, input.Items...)
	}
	input.MissingCategories = make([]string, 0, len(originalMissing)+1)
	for _, missing := range originalMissing {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(missing)), "energy:") {
			continue
		}
		input.MissingCategories = append(input.MissingCategories, missing)
	}
	missing := "energy: complete zone carrier total"
	input.MissingCategories = appendUniqueStrings(input.MissingCategories, missing)
	input.SourceAvailability = make([]EnergySourceAvailabilityEntry, 0, len(originalAvailability)+1)
	for _, item := range originalAvailability {
		if strings.EqualFold(strings.TrimSpace(item.Level), "energy") {
			continue
		}
		input.SourceAvailability = append(input.SourceAvailability, item)
	}
	input.SourceAvailability = append(input.SourceAvailability, EnergySourceAvailabilityEntry{
		Name:   "Complete zone carrier total",
		Level:  "energy",
		Status: "missing",
	})
	return input
}

func energyPathDirectZoneCoverageWarning(zoneName string, period string) EnergyWarning {
	return EnergyWarning{
		Severity: "warning",
		Code:     "zone_direct_energy_partial_coverage",
		Message:  fmt.Sprintf("%s energy uses exact zone-keyed observations and, when enabled, explicitly allocated central HVAC energy; no complete zone carrier total was inferred.", firstNonEmpty(strings.TrimSpace(zoneName), "Selected zone")),
		Period:   period,
	}
}

func markEnergyPathDirectZoneReconciliationPartial(reconciliation []EnergyReconciliation, links []EnergyPathLink, zoneName string) {
	zoneCarrierBasis := map[string]string{}
	for _, link := range links {
		basis := canonicalEnergyPathBasis(link.Basis, "")
		if energyPathAllocationBasisRank(basis) > 0 && (link.Relation == "end_use_to_carrier" || link.Relation == "direct_end_use_to_carrier") {
			zoneCarrierBasis[link.ToID] = energyPathPreferredAllocationBasis(zoneCarrierBasis[link.ToID], basis)
		}
	}
	if len(zoneCarrierBasis) == 0 {
		return
	}
	for index := range reconciliation {
		row := &reconciliation[index]
		for carrierID, basis := range zoneCarrierBasis {
			carrier := strings.TrimPrefix(strings.Split(strings.TrimPrefix(carrierID, "carrier."), ".")[0], "carrier.")
			if carrier == "" || !energyPathCarrierReconciliationMatches(*row, carrier, row.Period) {
				continue
			}
			row.Status = "partial"
			row.ZoneName = firstNonEmpty(row.ZoneName, zoneName)
			row.Basis = basis
			row.Label = energyCarrierLabel(carrier) + " observed / allocated zone subtotal"
			row.Formula = "sum of exact zone-keyed observations and explicitly allocated central HVAC energy; no complete zone carrier total is inferred"
			break
		}
	}
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
	// An annual-only allocated HVAC end use also needs its annual load endpoint;
	// otherwise the fallback node survives but its load -> end-use ribbon is
	// dropped because monthly periods had no instance of that service.
	initialNodeByID := make(map[string]EnergyExplanationNode, len(initialNodes))
	for _, node := range initialNodes {
		initialNodeByID[node.ID] = node
	}
	for _, link := range initialLinks {
		if link.Relation != "load_to_end_use" || !fallbackNodeIDs[link.ToID] {
			continue
		}
		if _, exists := nodeIndex[link.FromID]; exists {
			continue
		}
		load, exists := initialNodeByID[link.FromID]
		if !exists || load.Level != "load" {
			continue
		}
		load.Period = "annual"
		nodeIndex[load.ID] = len(nodes)
		fallbackNodeIDs[load.ID] = true
		nodes = append(nodes, load)
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
	// Annual-only direct-use meters are allowed to fall back into an otherwise
	// monthly-canonical graph. A source correspondence copied with that fallback
	// still carries the initial annual driver value, which may differ from the
	// authoritative sum of monthly driver nodes. Rebind both sides to the final
	// annual endpoints after every fallback node/link has been materialized.
	canonicalNodes := make(map[string]*EnergyExplanationNode, len(nodes))
	for index := range nodes {
		canonicalNodes[nodes[index].ID] = &nodes[index]
	}
	for index := range links {
		if !energyPathLinkIsSourceCorrespondence(links[index]) {
			continue
		}
		synchronizeEnergyPathSourceCorrespondence(&links[index], canonicalNodes)
		finalizeEnergyPathLinkRatio(&links[index])
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
	// Monthly residual nodes carry positive display magnitudes and therefore
	// cannot be summed into an annual accounting result. Recompute the signed
	// carrier closure from aggregated totals, then rebuild optional presentation.
	reconciliation = reconcileEnergyPathCarrierTotals(nodes, links, reconciliation, "annual")
	nodes, links = rebuildEnergyPathCarrierResidualPresentation(nodes, links, reconciliation, "annual")

	sortEnergyExplanationNodes(nodes)
	sort.SliceStable(links, func(i, j int) bool { return links[i].ID < links[j].ID })
	result.Nodes = nodes
	result.Links = links
	result.Reconciliation = reconciliation
	result.Completeness = energyPathCompletenessFromCarrierReconciliation(result.Completeness, result.Scope, reconciliation)
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
				finalizeEnergyPathLinkRatio(&link)
				linkIndex[key] = len(links)
				links = append(links, link)
				continue
			}
			current := &links[index]
			current.SourceIDs = appendUniqueStrings(current.SourceIDs, link.SourceIDs...)
			current.RelatedPathIDs = appendUniqueStrings(current.RelatedPathIDs, link.RelatedPathIDs...)
			if !energyPathLinkIsSourceCorrespondence(*current) {
				current.FromValue = roundedEnergyNumber(current.FromValue + link.FromValue)
				current.ToValue = roundedEnergyNumber(current.ToValue + link.ToValue)
			}
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
			if current.Status == "partial" || item.Status == "partial" || canonicalEnergyPathBasis(current.Basis, "") == "direct_zone_energy" || canonicalEnergyPathBasis(item.Basis, "") == "direct_zone_energy" {
				current.Status = "partial"
				current.Basis = "direct_zone_energy"
			} else {
				current.Status = energyReconciliationStatus(current.ExpectedValue, current.ResidualValue)
			}
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
	for index := range links {
		link := &links[index]
		if energyPathLinkIsSourceCorrespondence(*link) {
			synchronizeEnergyPathSourceCorrespondence(link, canonicalNodes)
		}
		if link.Relation == "load_to_end_use" {
			annotateEnergyPathDirectZoneTemporalCoverage(link, canonicalNodes[link.FromID])
			// Monthly carrier evidence can change across the year. Reclassify
			// after node aggregation so an electricity-only month plus a district
			// month becomes an order-independent mixed-carrier annual ratio.
			setEnergyPathConversionRatioKind(link, canonicalNodes[link.FromID], canonicalNodes[link.ToID])
		}
		finalizeEnergyPathLinkRatio(link)
	}
	return nodes, links, reconciliation, warnings
}

func annotateEnergyPathDirectZoneTemporalCoverage(link *EnergyPathLink, load *EnergyExplanationNode) {
	if link == nil || load == nil || link.Relation != "load_to_end_use" || canonicalEnergyPathBasis(link.Basis, "") != "direct_zone_energy" {
		return
	}
	loadValue := math.Abs(energyExplanationEffectiveNodeValue(*load))
	if loadValue <= 0 || math.Abs(link.FromValue-loadValue) <= 1e-9 {
		return
	}
	const note = "only periods with exact direct zone energy; partial temporal coverage"
	if strings.Contains(strings.ToLower(link.Explanation), note) {
		return
	}
	if strings.TrimSpace(link.Explanation) == "" {
		link.Explanation = note
	} else {
		link.Explanation = strings.TrimSpace(link.Explanation) + "; " + note
	}
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
	for _, item := range input.zoneDirectUseSeries {
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

func sortEnergyPathPeriods(periods []EnergyPeriod) {
	periodOrder := func(period EnergyPeriod) (int, int, string) {
		kind := strings.ToLower(strings.TrimSpace(period.Kind))
		id := strings.ToUpper(strings.TrimSpace(period.ID))
		rank := 6
		index := 0
		switch {
		case kind == "annual" || id == "ANNUAL":
			rank = 0
		case kind == "selected_range" || id == "SELECTED_RANGE":
			rank = 1
		case kind == "monthly" || strings.HasPrefix(id, "M"):
			rank = 2
			index, _ = strconv.Atoi(strings.TrimPrefix(id, "M"))
		case kind == "daily" || strings.HasPrefix(id, "D"):
			rank = 3
			index, _ = strconv.Atoi(strings.TrimPrefix(id, "D"))
		case kind == "hourly" || strings.HasPrefix(id, "H"):
			rank = 4
			index, _ = strconv.Atoi(strings.TrimPrefix(id, "H"))
		}
		return rank, index, id
	}
	sort.SliceStable(periods, func(i, j int) bool {
		leftRank, leftIndex, leftID := periodOrder(periods[i])
		rightRank, rightIndex, rightID := periodOrder(periods[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if leftIndex != rightIndex {
			return leftIndex < rightIndex
		}
		return leftID < rightID
	})
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
	zoneAllocationProjections := energyExplanationZoneAllocationProjections(legacyNodes, legacyEdges, scope, allocationPolicy)
	nodeByLegacyID := make(map[string]EnergyExplanationNode, len(legacyNodes))
	canonicalIDByLegacyID := make(map[string]string, len(legacyNodes))
	canonicalNodes := map[string]*EnergyExplanationNode{}
	scopedLegacyNodes := make([]EnergyExplanationNode, 0, len(legacyNodes))
	for _, original := range legacyNodes {
		legacy, include := energyExplanationNodeForScope(original, scope, zoneAllocationProjections)
		if !include {
			continue
		}
		if energyPathNodeHasInvalidCarrierUnit(legacy) {
			// A recognized energy carrier is only admissible to the site-energy
			// path when its quantity has an energy unit. Preserve its source for
			// audit, but never relabel a volume (or other context quantity) as kWh.
			continue
		}
		if energyPathNodeIsContextOnlyWater(legacy, sources) {
			// Native Water:Facility is volumetric utility context. Keeping it out
			// of the four energy stages prevents m3 from being presented or summed
			// as kWh site. Its source record remains available for context display.
			continue
		}
		if energyPathLegacyNodeIsPeopleDirectUse(legacy) {
			// People is a thermal load driver, never a metered Stage 3 use. Some
			// stored-v1/custom payloads nevertheless contain carrier-qualified
			// People or Occupants nodes. Drop only those exact semantics here;
			// unrelated unknown end uses still canonicalize to Other (EPATH-091).
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
		if canonical.Level == "end_use" && canonicalEnergyPathPart(legacy.EndUse) == "storage_charge" {
			// Charge remains consumption in canonical Other. Retain a separate
			// context-only observation so the supply inspector can identify real
			// storage activity without guessing its share of the merged end use.
			charge := canonical
			charge.ID = "support.storage_charge." + energyExplanationScopeToken(scope)
			charge.Level = "support"
			charge.Kind = "energy.storage_charge"
			charge.EndUse = "storage_charge"
			charge.Label = "Storage charge"
			charge.Carrier = canonicalEnergyPathCarrier(legacy.Carrier)
			mergeEnergyExplanationV2Node(canonicalNodes, charge)
		}
	}
	reportedCarrierIDs := map[string]bool{}
	for id, node := range canonicalNodes {
		if node.Level == "carrier" {
			reportedCarrierIDs[id] = true
		}
	}
	for _, legacy := range scopedLegacyNodes {
		endUseID := canonicalIDByLegacyID[legacy.ID]
		endUseNode := canonicalNodes[endUseID]
		if endUseNode == nil || endUseNode.Level != "end_use" || strings.TrimSpace(legacy.Carrier) == "" {
			continue
		}
		carrier := canonicalEnergyPathCarrier(legacy.Carrier)
		carrierID := strings.Join([]string{"carrier", carrier, energyExplanationScopeToken(scope)}, ".")
		if reportedCarrierIDs[carrierID] {
			continue
		}
		value := math.Abs(energyExplanationEffectiveNodeValue(legacy))
		if value == 0 {
			continue
		}
		mergeEnergyExplanationV2Node(canonicalNodes, EnergyExplanationNode{
			ID:                  carrierID,
			Level:               "carrier",
			Kind:                "energy." + carrier + ".observed_end_use_subtotal",
			Label:               energyCarrierLabel(carrier),
			Value:               value,
			RawValue:            math.Abs(firstNonZero(legacy.RawValue, legacy.Value)),
			EffectiveValue:      value,
			AllocatedValue:      value,
			DisplayValue:        value,
			Unit:                canonicalEnergyPathEnergyUnit(legacy.Unit),
			ScaleDomain:         "site",
			Period:              legacy.Period,
			ZoneName:            legacy.ZoneName,
			Carrier:             carrier,
			EndUse:              "total",
			MeterHierarchyLevel: "observed_end_use_subtotal",
			Badges:              []string{"partial", "observed_end_use_subtotal"},
			Basis:               "reported_end_use_subtotal",
			AggregationBasis:    scope.AggregationBasis,
			Multiplier:          1,
			SourceIDs:           appendUniqueStrings(nil, legacy.SourceIDs...),
		})
	}

	loadTotalsByEndUse := map[string]float64{}
	endUsesWithLoads := map[string]bool{}
	for _, edge := range legacyEdges {
		if !legacyEnergyLinkIsLoadToEndUse(edge) {
			continue
		}
		endUseNode, endUseOK := nodeByLegacyID[edge.FromID]
		loadNode, loadOK := nodeByLegacyID[edge.ToID]
		if !endUseOK || !loadOK {
			continue
		}
		if _, ok := energyPathThermalConversionService(edge, endUseNode, loadNode); !ok {
			// Only the measured Cooling/Heating end uses form the main thermal
			// conversion. Fans, pumps, heat rejection, and other auxiliaries
			// remain direct site-energy branches even if a malformed stored-v1
			// payload happens to point a delivered-load edge at one of them.
			continue
		}
		endUseID := canonicalIDByLegacyID[edge.FromID]
		loadTotalsByEndUse[endUseID] += math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(loadNode), edge.Value))
		endUsesWithLoads[endUseID] = true
	}

	links := map[string]*EnergyPathLink{}
	linkedLegacyEndUses := map[string]bool{}
	for _, edge := range legacyEdges {
		link, ok := upgradeEnergyExplanationLink(edge, nodeByLegacyID, canonicalIDByLegacyID, canonicalNodes, loadTotalsByEndUse, endUsesWithLoads, canonicalMonthlyBasis)
		if !ok {
			continue
		}
		mergeEnergyPathLink(links, link)
		if legacyEnergyLinkIsEndUseToCarrier(edge) {
			linkedLegacyEndUses[edge.ToID] = true
		}
	}
	for _, legacy := range scopedLegacyNodes {
		if linkedLegacyEndUses[legacy.ID] {
			continue
		}
		endUseID := canonicalIDByLegacyID[legacy.ID]
		endUseNode := canonicalNodes[endUseID]
		if endUseNode == nil || endUseNode.Level != "end_use" || strings.TrimSpace(legacy.Carrier) == "" {
			continue
		}
		carrier := canonicalEnergyPathCarrier(legacy.Carrier)
		carrierID := strings.Join([]string{"carrier", carrier, energyExplanationScopeToken(scope)}, ".")
		carrierNode := canonicalNodes[carrierID]
		if carrierNode == nil {
			continue
		}
		value := math.Abs(energyExplanationEffectiveNodeValue(legacy))
		if value == 0 {
			continue
		}
		relation := "direct_end_use_to_carrier"
		if endUsesWithLoads[endUseID] {
			relation = "end_use_to_carrier"
		}
		basis := canonicalEnergyPathBasis(legacy.Basis, "")
		if energyPathLegacyNodeIsDirectZoneEnergy(legacy) {
			basis = "direct_zone_energy"
		}
		link := EnergyPathLink{
			FromID:         endUseID,
			ToID:           carrierID,
			Relation:       relation,
			Basis:          basis,
			FromValue:      value,
			FromUnit:       endUseNode.Unit,
			ToValue:        value,
			ToUnit:         carrierNode.Unit,
			Period:         legacy.Period,
			ZoneName:       legacy.ZoneName,
			ServiceKind:    energyCanonicalServiceKind(legacy.EndUse),
			RelatedPathIDs: appendUniqueStrings(nil, legacy.RelatedPathIDs...),
			SourceIDs:      appendUniqueStrings(nil, legacy.SourceIDs...),
		}
		link.ID = energyPathLinkID(link)
		// Different exact legacy contributors can share one bounded taxonomy
		// node. Sum only contributors not already carried by explicit edges.
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
		if node.Level == "end_use" || node.Level == "carrier" {
			node.DisplayValue = math.Abs(node.Value)
		}
		sort.Strings(node.SourceIDs)
		sort.Strings(node.RelatedEntityIDs)
		sort.Strings(node.RelatedPathIDs)
		outNodes = append(outNodes, *node)
		visibleNodeIDs[node.ID] = true
	}
	sortEnergyExplanationNodes(outNodes)
	outLinks := make([]EnergyPathLink, 0, len(links))
	for _, link := range links {
		if !visibleNodeIDs[link.FromID] || !visibleNodeIDs[link.ToID] {
			continue
		}
		if link.Relation == "load_to_end_use" {
			synchronizeEnergyPathZoneHVACConversion(link, canonicalNodes)
			if canonicalEnergyPathBasis(link.Basis, "") == "direct_zone_energy" {
				annotateEnergyPathDirectZoneTemporalCoverage(link, canonicalNodes[link.FromID])
			}
			// A canonical end use can merge several carrier-qualified meters and
			// a Building load can merge several zone links. Classify the ratio
			// from those final endpoints, never from whichever legacy edge was
			// encountered first.
			setEnergyPathConversionRatioKind(link, canonicalNodes[link.FromID], canonicalNodes[link.ToID])
		}
		if energyPathLinkIsSourceCorrespondence(*link) {
			synchronizeEnergyPathSourceCorrespondence(link, canonicalNodes)
			sort.Strings(link.SourceIDs)
			sort.Strings(link.RelatedPathIDs)
		}
		if link.Relation == "end_use_to_carrier" || link.Relation == "direct_end_use_to_carrier" {
			sort.Strings(link.SourceIDs)
		}
		sort.Strings(link.SourceIDs)
		sort.Strings(link.RelatedPathIDs)
		finalizeEnergyPathLinkRatio(link)
		outLinks = append(outLinks, *link)
	}
	sort.SliceStable(outLinks, func(i, j int) bool { return outLinks[i].ID < outLinks[j].ID })
	return outNodes, outLinks
}

// synchronizeEnergyPathZoneHVACConversion makes the merged canonical
// endpoints authoritative for a direct or allocated Zone HVAC conversion. A
// service can have several carrier-qualified branches; their legacy edges
// collapse to one load -> end-use link and must not add the same physical load
// once per carrier.
func synchronizeEnergyPathZoneHVACConversion(link *EnergyPathLink, nodes map[string]*EnergyExplanationNode) {
	if link == nil || link.Relation != "load_to_end_use" {
		return
	}
	switch canonicalEnergyPathBasis(link.Basis, "") {
	case "direct_zone_energy", "service_path_allocation", "zone_load_allocation":
	default:
		return
	}
	load := nodes[link.FromID]
	endUse := nodes[link.ToID]
	if load == nil || endUse == nil || !strings.EqualFold(load.Level, "load") || !strings.EqualFold(endUse.Level, "end_use") {
		return
	}
	loadValue := math.Abs(energyExplanationEffectiveNodeValue(*load))
	// Duplicate carrier-qualified legacy edges can add the same measured load
	// more than once. Cap that duplicate at the canonical load, but retain a
	// smaller value: an annual link aggregated from only the months that had
	// exact direct energy intentionally represents that temporal overlap.
	link.FromValue = math.Abs(link.FromValue)
	if loadValue > 0 && link.FromValue > loadValue+1e-9 {
		link.FromValue = loadValue
	}
	link.ToValue = math.Abs(energyExplanationEffectiveNodeValue(*endUse))
	link.FromUnit = load.Unit
	link.ToUnit = endUse.Unit
	link.ZoneName = firstNonEmpty(endUse.ZoneName, load.ZoneName, link.ZoneName)
	link.SourceIDs = appendUniqueStrings(link.SourceIDs, load.SourceIDs...)
	link.SourceIDs = appendUniqueStrings(link.SourceIDs, endUse.SourceIDs...)
	link.RelatedPathIDs = appendUniqueStrings(link.RelatedPathIDs, load.RelatedPathIDs...)
	link.RelatedPathIDs = appendUniqueStrings(link.RelatedPathIDs, endUse.RelatedPathIDs...)
	sort.Strings(link.SourceIDs)
	sort.Strings(link.RelatedPathIDs)
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

type energyExplanationZoneAllocationProjection struct {
	Factor              float64
	Basis               string
	Explanation         string
	Formula             string
	RelatedPathIDs      []string
	AllocationSourceIDs []string
}

func energyExplanationZoneAllocationProjections(nodes []EnergyExplanationNode, edges []EnergyExplanationEdge, scope EnergyExplanationScope, allocationPolicy string) map[string]energyExplanationZoneAllocationProjection {
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
		basis    string
		formula  string
		paths    []string
		sources  []string
	}
	exactShares := map[string]allocationShare{}
	exactPresent := map[string]bool{}
	sharesByCarrierEndUse := map[string]allocationShare{}
	sharesByEndUse := map[string]allocationShare{}
	sharesByService := map[string]allocationShare{}
	directTargets := map[string]bool{}
	for _, node := range nodes {
		if !energyPathLegacyNodeIsDirectZoneEnergy(node) || legacyEnergyNodeIsCarrier(node) || !strings.EqualFold(strings.TrimSpace(node.ZoneName), scope.ZoneName) {
			continue
		}
		directTargets[energyPathDirectZoneTargetKey(node)] = true
	}
	seenExactLoads := map[string]bool{}
	seenCarrierEndUseLoads := map[string]bool{}
	seenEndUseLoads := map[string]bool{}
	seenServiceLoads := map[string]bool{}
	for _, edge := range edges {
		auxiliaryAllocation := strings.EqualFold(strings.TrimSpace(edge.Relation), energyPathAuxiliaryAllocationRelation)
		if !strings.EqualFold(edge.Relation, "allocation") && !auxiliaryAllocation {
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
		endUseKind := canonicalEnergyPathEndUse(firstNonEmpty(endUse.EndUse, energyExplanationKindSuffix(endUse.Kind)))
		if auxiliaryAllocation {
			if !energyPathIsZoneAllocatableAuxiliary(endUseKind) || !strings.EqualFold(strings.TrimSpace(load.Level), "load") {
				continue
			}
		} else if (endUseKind != "cooling" && endUseKind != "heating") || service != endUseKind {
			// A zone load share is meaningful only for the matching HVAC service.
			// Lighting/equipment/water/process meters are never projected through
			// a malformed allocation edge when their direct source is absent.
			continue
		}
		endUseKey := energyExplanationAllocationEndUseKey(endUse, service)
		carrierEndUseKey := energyExplanationAllocationCarrierEndUseKey(endUse, service)
		selected := strings.EqualFold(strings.TrimSpace(load.ZoneName), scope.ZoneName)
		basis := canonicalEnergyPathBasis(edge.Basis, edge.RuleID)
		updateShare := func(share allocationShare) allocationShare {
			share.total += value
			if selected {
				share.selected += value
				previousRank := energyPathAllocationBasisRank(share.basis)
				share.basis = energyPathPreferredAllocationBasis(share.basis, basis)
				if share.formula == "" || energyPathAllocationBasisRank(basis) > previousRank {
					share.formula = edge.Formula
				}
				share.paths = appendUniqueStrings(share.paths, edge.RelatedPathIDs...)
				share.sources = appendUniqueStrings(share.sources, edge.SourceIDs...)
			}
			return share
		}
		exactPresent[edge.FromID] = true
		if key := edge.FromID + "|" + edge.ToID; !seenExactLoads[key] {
			exactShares[edge.FromID] = updateShare(exactShares[edge.FromID])
			seenExactLoads[key] = true
		}
		if key := carrierEndUseKey + "|" + edge.ToID; carrierEndUseKey != "" && !seenCarrierEndUseLoads[key] {
			sharesByCarrierEndUse[carrierEndUseKey] = updateShare(sharesByCarrierEndUse[carrierEndUseKey])
			seenCarrierEndUseLoads[key] = true
		}
		if key := endUseKey + "|" + edge.ToID; endUseKey != "" && !seenEndUseLoads[key] {
			sharesByEndUse[endUseKey] = updateShare(sharesByEndUse[endUseKey])
			seenEndUseLoads[key] = true
		}
		if key := service + "|" + edge.ToID; service != "" && !seenServiceLoads[key] {
			sharesByService[service] = updateShare(sharesByService[service])
			seenServiceLoads[key] = true
		}
	}
	projections := map[string]energyExplanationZoneAllocationProjection{}
	projectionFromShare := func(factor float64, share allocationShare) energyExplanationZoneAllocationProjection {
		basis := firstNonEmpty(share.basis, "zone_load_allocation")
		explanation := "Allocated by zone service load share"
		if basis == "service_path_allocation" {
			formula := strings.ToLower(strings.TrimSpace(share.formula))
			if strings.Contains(formula, "airloop") && (strings.Contains(formula, "supply-air volume") || strings.Contains(formula, "supply air volume")) {
				explanation = "Allocated by related AirLoop supply-air volume share"
			} else {
				explanation = "Allocated by HVAC service-path load share"
			}
		}
		paths := appendUniqueStrings(nil, share.paths...)
		sources := appendUniqueStrings(nil, share.sources...)
		sort.Strings(paths)
		sort.Strings(sources)
		return energyExplanationZoneAllocationProjection{
			Factor:              factor,
			Basis:               basis,
			Explanation:         explanation,
			Formula:             share.formula,
			RelatedPathIDs:      paths,
			AllocationSourceIDs: sources,
		}
	}
	for _, node := range nodes {
		if strings.TrimSpace(node.ZoneName) != "" || !strings.EqualFold(node.Level, "energy") || legacyEnergyNodeIsCarrier(node) || legacyEnergyNodeIsSupport(node) {
			continue
		}
		if directTargets[energyPathDirectZoneTargetKey(node)] {
			continue
		}
		if exactPresent[node.ID] {
			share := exactShares[node.ID]
			value := math.Abs(energyExplanationEffectiveNodeValue(energyExplanationLegacyNodeWithEffectiveValues(node)))
			if value > 0 && share.selected > 0 {
				projections[node.ID] = projectionFromShare(share.selected/value, share)
			}
			continue
		}
		service := energyCanonicalServiceKind(node.EndUse)
		carrierKey := energyExplanationAllocationCarrierEndUseKey(node, service)
		if share := sharesByCarrierEndUse[carrierKey]; carrierKey != "" && share.total > 0 {
			if share.selected > 0 {
				projections[node.ID] = projectionFromShare(share.selected/share.total, share)
			}
			continue
		}
		key := energyExplanationAllocationEndUseKey(node, service)
		if share := sharesByEndUse[key]; key != "" && share.total > 0 && share.selected > 0 {
			projections[node.ID] = projectionFromShare(share.selected/share.total, share)
		}
	}
	allocatedByCarrier := map[string]float64{}
	carrierProjection := map[string]energyExplanationZoneAllocationProjection{}
	for _, node := range nodes {
		projection, ok := projections[node.ID]
		if !ok || strings.TrimSpace(node.Carrier) == "" {
			continue
		}
		carrier := strings.ToLower(strings.TrimSpace(node.Carrier))
		allocatedByCarrier[carrier] += math.Abs(energyExplanationEffectiveNodeValue(energyExplanationLegacyNodeWithEffectiveValues(node))) * projection.Factor
		current := carrierProjection[carrier]
		if current.Basis == "" || energyPathAllocationBasisRank(projection.Basis) > energyPathAllocationBasisRank(current.Basis) {
			current.Basis = projection.Basis
			current.Explanation = projection.Explanation
			current.Formula = projection.Formula
		}
		current.RelatedPathIDs = appendUniqueStrings(current.RelatedPathIDs, projection.RelatedPathIDs...)
		current.AllocationSourceIDs = appendUniqueStrings(current.AllocationSourceIDs, projection.AllocationSourceIDs...)
		carrierProjection[carrier] = current
	}
	for _, node := range nodes {
		if strings.TrimSpace(node.ZoneName) != "" || !legacyEnergyNodeIsCarrier(node) {
			continue
		}
		carrier := strings.ToLower(strings.TrimSpace(node.Carrier))
		value := math.Abs(energyExplanationEffectiveNodeValue(energyExplanationLegacyNodeWithEffectiveValues(node)))
		if value > 0 && allocatedByCarrier[carrier] > 0 {
			projection := carrierProjection[carrier]
			projection.Factor = allocatedByCarrier[carrier] / value
			projections[node.ID] = projection
		}
	}
	return projections
}

func energyExplanationZoneAllocationFactors(nodes []EnergyExplanationNode, edges []EnergyExplanationEdge, scope EnergyExplanationScope, allocationPolicy string) map[string]float64 {
	projections := energyExplanationZoneAllocationProjections(nodes, edges, scope, allocationPolicy)
	if len(projections) == 0 {
		return nil
	}
	factors := make(map[string]float64, len(projections))
	for id, projection := range projections {
		factors[id] = projection.Factor
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

func energyExplanationAllocationCarrierEndUseKey(node EnergyExplanationNode, service string) string {
	endUse := energyExplanationAllocationEndUseKey(node, service)
	carrier := canonicalEnergyPathCarrier(strings.TrimSpace(node.Carrier))
	if endUse == "" || carrier == "" {
		return ""
	}
	return endUse + "|" + carrier
}

func energyPathAllocationBasisRank(basis string) int {
	switch canonicalEnergyPathBasis(basis, "") {
	case "direct_zone_energy":
		return 3
	case "service_path_allocation":
		return 2
	case "zone_load_allocation":
		return 1
	default:
		return 0
	}
}

func energyPathPreferredAllocationBasis(left string, right string) string {
	if energyPathAllocationBasisRank(right) > energyPathAllocationBasisRank(left) {
		return canonicalEnergyPathBasis(right, "")
	}
	return canonicalEnergyPathBasis(left, "")
}

func energyExplanationHasExplicitAllocation(edges []EnergyExplanationEdge) bool {
	for _, edge := range edges {
		if strings.EqualFold(strings.TrimSpace(edge.Relation), energyPathAuxiliaryAllocationRelation) && edge.RuleID == energyRelationshipRuleAllocatedAuxiliaryServicePath {
			return true
		}
		if strings.EqualFold(edge.Relation, "allocation") && (edge.RuleID == energyRelationshipRuleAllocatedZoneLoad || edge.RuleID == energyRelationshipRuleAllocatedServicePathLoad || canonicalEnergyPathBasis(edge.Basis, edge.RuleID) == "zone_load_allocation" || canonicalEnergyPathBasis(edge.Basis, edge.RuleID) == "service_path_allocation") {
			return true
		}
	}
	return false
}

func energyExplanationNodeForScope(node EnergyExplanationNode, scope EnergyExplanationScope, projections map[string]energyExplanationZoneAllocationProjection) (EnergyExplanationNode, bool) {
	node = energyExplanationLegacyNodeWithEffectiveValues(node)
	if energyPathZoneAuxiliaryIsSupplyAirflow(node) {
		// Supply-air volume is optional allocation evidence, not an Energy Path
		// stage. It is consumed while projections are built and never rendered as
		// a site-energy end use.
		return EnergyExplanationNode{}, false
	}
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
		if energyPathLegacyNodeIsDirectZoneEnergy(node) {
			node.Basis = "direct_zone_energy"
		}
		return node, strings.EqualFold(zoneName, scope.ZoneName)
	}
	projection, ok := projections[node.ID]
	if !ok || projection.Factor <= 0 {
		return EnergyExplanationNode{}, false
	}
	effective := node.EffectiveValue
	node.AllocatedValue = roundedEnergyNumber(effective * projection.Factor)
	node.Value = node.AllocatedValue
	if node.SignedValue != 0 {
		node.SignedValue = roundedEnergyNumber(node.SignedValue * projection.Factor)
		node.DisplayValue = math.Abs(node.SignedValue)
	}
	node.AllocationApplied = true
	node.AllocationExplanation = projection.Explanation
	node.Basis = firstNonEmpty(projection.Basis, node.Basis)
	// Building end-use paths can span several zones. Once projected, retain only
	// the selected target paths so the Zone inspector cannot jump to a sibling
	// service path that did not contribute to this allocation.
	node.RelatedPathIDs = appendUniqueStrings(nil, projection.RelatedPathIDs...)
	node.SourceIDs = appendUniqueStrings(node.SourceIDs, projection.AllocationSourceIDs...)
	node.allocationSourceIDs = appendUniqueStrings(node.allocationSourceIDs, projection.AllocationSourceIDs...)
	sort.Strings(node.RelatedPathIDs)
	sort.Strings(node.SourceIDs)
	return node, node.Value != 0
}

func energyPathLegacyNodeIsDirectZoneEnergy(node EnergyExplanationNode) bool {
	if strings.TrimSpace(node.ZoneName) == "" || legacyEnergyNodeIsSupport(node) {
		return false
	}
	level := strings.ToLower(strings.TrimSpace(node.Level))
	if level != "energy" && level != "end_use" && level != "carrier" {
		return false
	}
	hierarchy := strings.ToLower(strings.TrimSpace(node.MeterHierarchyLevel))
	if strings.HasPrefix(hierarchy, "zone_direct") || canonicalEnergyPathBasis(node.Basis, "") == "direct_zone_energy" {
		return true
	}
	// Stored/custom canonical payloads may predate the hierarchy marker. An
	// exact zone-qualified site-energy endpoint is still direct evidence; broad
	// facility meters have an empty ZoneName and cannot enter this branch.
	return true
}

func energyPathDirectZoneTargetKey(node EnergyExplanationNode) string {
	if legacyEnergyNodeIsCarrier(node) || legacyEnergyNodeIsSupport(node) {
		return ""
	}
	endUse := canonicalEnergyPathEndUse(firstNonEmpty(node.EndUse, energyExplanationKindSuffix(node.Kind), "other"))
	carrier := canonicalEnergyPathCarrier(firstNonEmpty(node.Carrier, "other"))
	return endUse + "|" + carrier
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
	projections := energyExplanationZoneAllocationProjections(nodes, edges, scope, allocationPolicy)
	out := make([]EnergyExplanationNode, 0, len(nodes))
	for _, node := range nodes {
		if scoped, ok := energyExplanationNodeForScope(node, scope, projections); ok {
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
		carrier       string
		expected      float64
		explained     float64
		unit          string
		zoneName      string
		directZone    bool
		observedOnly  bool
		subtotalBasis string
		sourceIDs     []string
		existingRow   int
	}
	byNodeID := map[string]*carrierTotal{}
	orderedNodeIDs := make([]string, 0)
	for _, node := range nodes {
		if node.Level != "carrier" {
			continue
		}
		carrier := canonicalEnergyPathCarrier(firstNonEmpty(node.Carrier, energyExplanationKindSuffix(node.Kind), "other"))
		basis := canonicalEnergyPathBasis(node.Basis, "")
		byNodeID[node.ID] = &carrierTotal{
			carrier:    carrier,
			expected:   math.Abs(node.Value),
			unit:       node.Unit,
			zoneName:   node.ZoneName,
			directZone: canonicalEnergyPathBasis(node.Basis, "") == "direct_zone_energy",
			observedOnly: canonicalEnergyPathBasis(node.Basis, "") == "reported_end_use_subtotal" ||
				strings.EqualFold(strings.TrimSpace(node.MeterHierarchyLevel), "observed_end_use_subtotal"),
			subtotalBasis: func() string {
				if strings.TrimSpace(node.ZoneName) != "" && energyPathAllocationBasisRank(basis) > 0 {
					return basis
				}
				return ""
			}(),
			sourceIDs:   appendUniqueStrings(nil, node.SourceIDs...),
			existingRow: -1,
		}
		orderedNodeIDs = append(orderedNodeIDs, node.ID)
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
		if canonicalEnergyPathBasis(link.Basis, "") == "direct_zone_energy" {
			total.directZone = true
		}
		if strings.TrimSpace(total.zoneName) != "" && energyPathAllocationBasisRank(link.Basis) > energyPathAllocationBasisRank(total.subtotalBasis) {
			total.subtotalBasis = canonicalEnergyPathBasis(link.Basis, "")
		}
		total.unit = firstNonEmpty(total.unit, link.ToUnit)
		total.sourceIDs = appendUniqueStrings(total.sourceIDs, link.SourceIDs...)
	}
	claimedRows := map[int]bool{}
	for index := range input {
		for _, total := range byNodeID {
			if total.existingRow >= 0 || !energyPathCarrierReconciliationMatches(input[index], total.carrier, period) {
				continue
			}
			total.existingRow = index
			claimedRows[index] = true
			break
		}
	}
	out := make([]EnergyReconciliation, 0, len(input)+len(byNodeID))
	inputIndexToOutput := map[int]int{}
	for index, item := range input {
		if energyPathIsCarrierReconciliationRow(item) && !claimedRows[index] {
			// Carrier rows are graph-derived. Orphaned, duplicate, or wrong-period
			// rows from stored payloads must not survive as authoritative data.
			continue
		}
		inputIndexToOutput[index] = len(out)
		out = append(out, item)
	}
	for _, total := range byNodeID {
		if total.existingRow < 0 {
			continue
		}
		if outputIndex, ok := inputIndexToOutput[total.existingRow]; ok {
			total.existingRow = outputIndex
		} else {
			total.existingRow = -1
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
			Status:         energyCarrierReconciliationStatus(residual),
			ExpectedValue:  total.expected,
			ExplainedValue: total.explained,
			ResidualValue:  residual,
			Unit:           total.unit,
			Basis:          "residual",
			Formula:        "facility carrier total - mapped carrier-qualified end-use meters",
			SourceIDs:      total.sourceIDs,
		}
		if total.directZone || total.subtotalBasis == "service_path_allocation" || total.subtotalBasis == "zone_load_allocation" {
			row.Label = energyCarrierLabel(total.carrier) + " observed / allocated zone subtotal"
			row.Status = "partial"
			row.ZoneName = total.zoneName
			row.Basis = firstNonEmpty(total.subtotalBasis, "direct_zone_energy")
			row.Formula = "sum of exact zone-keyed observations and explicitly allocated central HVAC energy; no complete zone carrier total is inferred"
		} else if total.observedOnly {
			row.Label = energyCarrierLabel(total.carrier) + " observed end-use subtotal"
			row.Status = "partial"
			row.Basis = "reported_end_use_subtotal"
			row.Formula = "sum of carrier-qualified end-use meters; no facility carrier total was reported"
		}
		if total.existingRow >= 0 {
			// Retain the scope-qualified compatibility ID produced by the v1
			// adapter while replacing its accounting with the canonical v2 split.
			row.ID = out[total.existingRow].ID
			row.ZoneName = firstNonEmpty(row.ZoneName, out[total.existingRow].ZoneName)
			out[total.existingRow] = row
			continue
		}
		out = append(out, row)
	}
	return out
}

func energyPathIsCarrierReconciliationRow(item EnergyReconciliation) bool {
	if !strings.EqualFold(item.Level, "energy") && !strings.EqualFold(item.Level, "carrier") {
		return false
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(item.ID)), "reconcile.energy.")
}

func energyCarrierReconciliationStatus(residual float64) string {
	if residual < 0 {
		return "overmapped"
	}
	if residual > 0 {
		return "residual"
	}
	return "balanced"
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

// rebuildEnergyPathCarrierResidualPresentation keeps reconciliation as the
// accounting authority and treats a graph residual as optional presentation.
// Positive, material gaps become one additive Unclassified energy branch;
// small and overmapped gaps remain available through the carrier badge and
// inspector without distorting the main flow.
func rebuildEnergyPathCarrierResidualPresentation(nodes []EnergyExplanationNode, links []EnergyPathLink, reconciliation []EnergyReconciliation, period string) ([]EnergyExplanationNode, []EnergyPathLink) {
	period = firstNonEmpty(strings.TrimSpace(period), "annual")
	removedNodeIDs := map[string]bool{}
	outNodes := make([]EnergyExplanationNode, 0, len(nodes))
	carrierIndexes := map[string]int{}
	for _, original := range nodes {
		node := original
		node.Badges = appendUniqueStrings(nil, original.Badges...)
		if energyPathNodeIsCarrierResidual(node) {
			removedNodeIDs[node.ID] = true
			continue
		}
		if node.Level == "carrier" {
			filteredBadges := make([]string, 0, len(node.Badges))
			for _, badge := range node.Badges {
				if !strings.EqualFold(strings.TrimSpace(badge), "carrier_residual") {
					filteredBadges = append(filteredBadges, badge)
				}
			}
			node.Badges = filteredBadges
			carrier := canonicalEnergyPathCarrier(firstNonEmpty(node.Carrier, energyExplanationKindSuffix(node.Kind), "other"))
			carrierIndexes[carrier] = len(outNodes)
		}
		outNodes = append(outNodes, node)
	}

	outLinks := make([]EnergyPathLink, 0, len(links))
	for _, link := range links {
		if removedNodeIDs[link.FromID] || removedNodeIDs[link.ToID] {
			continue
		}
		outLinks = append(outLinks, link)
	}

	for carrier, nodeIndex := range carrierIndexes {
		var row *EnergyReconciliation
		for index := range reconciliation {
			if energyPathCarrierReconciliationMatches(reconciliation[index], carrier, period) {
				row = &reconciliation[index]
				break
			}
		}
		if row == nil {
			continue
		}
		carrierNode := &outNodes[nodeIndex]
		if roundedEnergyNumber(row.ResidualValue) != 0 {
			carrierNode.Badges = appendUniqueStrings(carrierNode.Badges, "carrier_residual")
		}
		if !energyCarrierResidualVisibleInGraph(row.ExpectedValue, row.ResidualValue) {
			continue
		}

		scopeToken := energyPathCarrierNodeScopeToken(carrierNode.ID, carrierNode.ZoneName)
		residualID := strings.Join([]string{"residual", "site_" + carrier, scopeToken}, ".")
		value := roundedEnergyNumber(row.ResidualValue)
		unit := firstNonEmpty(row.Unit, carrierNode.Unit, "kWh")
		sourceIDs := appendUniqueStrings(nil, row.SourceIDs...)
		sort.Strings(sourceIDs)
		outNodes = append(outNodes, EnergyExplanationNode{
			ID:               residualID,
			Level:            "residual",
			Kind:             "energy.unclassified",
			Label:            "Unclassified energy",
			Value:            value,
			SignedValue:      value,
			RawValue:         value,
			EffectiveValue:   value,
			AllocatedValue:   value,
			DisplayValue:     value,
			Unit:             unit,
			ScaleDomain:      "site",
			Period:           period,
			ZoneName:         carrierNode.ZoneName,
			Carrier:          carrier,
			Badges:           []string{"unclassified_energy"},
			Basis:            "residual",
			AggregationBasis: carrierNode.AggregationBasis,
			Multiplier:       1,
			SourceIDs:        sourceIDs,
		})
		link := EnergyPathLink{
			FromID:      residualID,
			ToID:        carrierNode.ID,
			FromValue:   value,
			ToValue:     value,
			FromUnit:    unit,
			ToUnit:      unit,
			Relation:    "residual",
			Period:      period,
			ZoneName:    carrierNode.ZoneName,
			Basis:       "residual",
			RuleID:      energyRelationshipRuleEnergyResidual,
			Explanation: "facility carrier total - mapped carrier-qualified end-use meters",
			SourceIDs:   sourceIDs,
		}
		link.ID = energyPathLinkID(link)
		outLinks = append(outLinks, link)
	}

	sortEnergyExplanationNodes(outNodes)
	sort.SliceStable(outLinks, func(i, j int) bool { return outLinks[i].ID < outLinks[j].ID })
	return outNodes, outLinks
}

func energyPathNodeIsCarrierResidual(node EnergyExplanationNode) bool {
	if !strings.EqualFold(strings.TrimSpace(node.Level), "residual") {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(node.ScaleDomain), "thermal") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(node.Kind)), "heat.") {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(node.Kind))
	id := strings.ToLower(strings.TrimSpace(node.ID))
	return kind == "energy.residual" || kind == "energy.unclassified" || kind == "residual.site_energy" ||
		strings.HasPrefix(id, "residual.energy.") || strings.HasPrefix(id, "residual.site_")
}

func energyPathCarrierNodeScopeToken(id string, zoneName string) string {
	parts := strings.Split(strings.TrimSpace(id), ".")
	if len(parts) >= 3 && strings.EqualFold(parts[0], "carrier") {
		return firstNonEmpty(strings.Join(parts[2:], "."), "building")
	}
	if strings.TrimSpace(zoneName) != "" {
		return firstNonEmpty(energyExplanationZoneSuffix(zoneName), metricID(zoneName), "zone")
	}
	return "building"
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
		carrier := canonicalEnergyPathCarrier(node.Carrier)
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
		raw                   float64
		effective             float64
		effectiveMultiplier   float64
		allocationFactor      float64
		allocated             float64
		allocationApplied     bool
		allocationExplanation string
		allocationFormula     string
	}
	used := map[string]bool{}
	values := map[string]sourceValues{}
	exactDriverAllocations := map[string]float64{}
	exactDriverNodes := map[string]bool{}
	ambiguousDriverAllocations := map[string]bool{}
	sourceByID := make(map[string]EnergyDataSource, len(input))
	for _, source := range input {
		sourceByID[source.ID] = source
	}
	projections := energyExplanationZoneAllocationProjections(legacyNodes, legacyEdges, scope, allocationPolicy)
	for _, original := range legacyNodes {
		node := energyExplanationLegacyNodeWithEffectiveValues(original)
		allocationFactor := 1.0
		projection, projectedAllocation := projections[node.ID]
		if scope.Kind == "zone" {
			if zoneName := strings.TrimSpace(node.ZoneName); zoneName != "" {
				if !strings.EqualFold(zoneName, scope.ZoneName) {
					continue
				}
			} else {
				if !projectedAllocation || projection.Factor <= 0 {
					continue
				}
				allocationFactor = projection.Factor
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
		allocationApplied := original.AllocationApplied || projectedAllocation && strings.TrimSpace(original.ZoneName) == ""
		if original.AllocationApplied && energyDriverNodeUsesCanonicalTaxonomy(original) && len(allocationSourceIDs) != 1 {
			for _, sourceID := range allocationSourceIDs {
				ambiguousDriverAllocations[sourceID] = true
			}
		}
		if original.AllocationApplied && energyDriverNodeUsesCanonicalTaxonomy(original) && len(allocationSourceIDs) == 1 && !exactDriverNodes[original.ID] {
			// Only a single qualified source proves that this entire directional
			// contribution belongs to that source. Annual node contributions are
			// already sums of monthly allocations; keep the cooling and heating
			// contributions independent of the signed annual source net.
			exactDriverAllocations[allocationSourceIDs[0]] += allocated
			exactDriverNodes[original.ID] = true
		}
		for _, sourceID := range allocationSourceIDs {
			value := values[sourceID]
			// A SQL dictionary source may be propagated to residual and link
			// nodes. Keep its largest direct contribution instead of counting
			// those graph aliases more than once.
			if effective > value.effective || (allocationApplied && !value.allocationApplied) {
				value = sourceValues{
					raw:                   raw,
					effective:             effective,
					effectiveMultiplier:   multiplier,
					allocationFactor:      allocationFactor,
					allocated:             allocated,
					allocationApplied:     allocationApplied,
					allocationExplanation: projection.Explanation,
					allocationFormula:     projection.Formula,
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
		if source.RawValue == 0 && !energyDataSourceHasPreparedValues(source) {
			source.RawValue = roundedEnergyNumber(value.raw)
		}
		if source.EffectiveValue == 0 && !energyDataSourceHasPreparedValues(source) {
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
			source.AllocationExplanation = firstNonEmpty(value.allocationExplanation, energyDriverAllocationExplanation)
			source.AllocationFormula = firstNonEmpty(value.allocationFormula, energyDriverAllocationFormula)
			if allocated, exact := exactDriverAllocations[source.ID]; exact && !ambiguousDriverAllocations[source.ID] {
				source.AllocatedValue = roundedEnergyNumber(allocated)
				source.AllocationExplanation = energyDriverAllocationExplanation + " Contribution is the sum of actual cooling and heating driver allocations uniquely attributed to this source. Raw and effective values retain the signed source net; the allocation factor is category context, not a ratio against that net."
				source.AllocationFormula = "sum(distinct directional driver allocatedValue with this sole allocation source); each allocation: " + energyDriverAllocationFormula
			}
			if source.Explanation == "" {
				source.Explanation = source.AllocationExplanation
			}
			if source.Formula == "" {
				source.Formula = source.AllocationFormula
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
		source.RelatedEntityIDs = appendUniqueStrings(nil, source.RelatedEntityIDs...)
		source.InputSourceIDs = appendUniqueStrings(nil, source.InputSourceIDs...)
		sort.Strings(source.RelatedEntityIDs)
		sort.Strings(source.InputSourceIDs)
		sort.SliceStable(source.ScopeDetails, func(i, j int) bool {
			left := strings.ToLower(source.ScopeDetails[i].Scope.Kind + "|" + source.ScopeDetails[i].Scope.ZoneName)
			right := strings.ToLower(source.ScopeDetails[j].Scope.Kind + "|" + source.ScopeDetails[j].Scope.ZoneName)
			return left < right
		})
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
		sort.SliceStable(parent[index].ScopeDetails, func(i, j int) bool {
			left := strings.ToLower(parent[index].ScopeDetails[i].Scope.Kind + "|" + parent[index].ScopeDetails[i].Scope.ZoneName)
			right := strings.ToLower(parent[index].ScopeDetails[j].Scope.Kind + "|" + parent[index].ScopeDetails[j].Scope.ZoneName)
			return left < right
		})
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
	inputLevel := strings.ToLower(strings.TrimSpace(input.Level))
	energyLike := input.Carrier != "" || inputLevel == "energy" || inputLevel == "end_use" || inputLevel == "carrier" || inputLevel == "support"
	if energyLike {
		out.Carrier = canonicalEnergyPathCarrier(input.Carrier)
		out.Unit = canonicalEnergyPathEnergyUnit(input.Unit)
	}

	switch inputLevel {
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
		out.ID = strings.Join([]string{"carrier", canonicalEnergyPathCarrier(firstNonEmpty(input.Carrier, energyExplanationKindSuffix(input.Kind), "other")), scopeToken}, ".")
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
			out.ID = strings.Join([]string{"carrier", canonicalEnergyPathCarrier(firstNonEmpty(input.Carrier, "other")), scopeToken}, ".")
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
			out.ID = strings.Join([]string{"carrier", canonicalEnergyPathCarrier(firstNonEmpty(input.Carrier, "other")), scopeToken}, ".")
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
			out.endUseCarriers = appendUniqueStrings(out.endUseCarriers, canonicalEnergyPathCarrier(carrier))
		}
		out.Carrier = ""
	}
	if out.Level == "carrier" {
		carrier := canonicalEnergyPathCarrier(firstNonEmpty(input.Carrier, out.Carrier, "other"))
		out.Carrier = carrier
		out.Label = energyCarrierLabel(carrier)
		if definition, ok := energyCarrierTaxonomyDefinitionFor(carrier); ok && definition.ScaleDomain == "site" && strings.TrimSpace(out.Unit) == "" {
			out.Unit = definition.Unit
		}
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

func energyPathLegacyNodeIsPeopleDirectUse(node EnergyExplanationNode) bool {
	if legacyEnergyNodeIsCarrier(node) || legacyEnergyNodeIsSupport(node) {
		return false
	}
	level := strings.ToLower(strings.TrimSpace(node.Level))
	if level != "energy" && level != "end_use" {
		return false
	}
	semantic := strings.TrimSpace(node.EndUse)
	if semantic == "" {
		semantic = energyExplanationKindSuffix(node.Kind)
	}
	switch canonicalEnergyPathPart(semantic) {
	case "people", "occupant", "occupants":
		return true
	default:
		return false
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
		leftBasis := current.Basis
		rightBasis := next.Basis
		preferred := energyPathPreferredAllocationBasis(current.Basis, next.Basis)
		if energyPathAllocationBasisRank(preferred) > 0 {
			current.Basis = preferred
			if preferred == "direct_zone_energy" && (energyPathAllocationBasisRank(leftBasis) > 0 && canonicalEnergyPathBasis(leftBasis, "") != "direct_zone_energy" || energyPathAllocationBasisRank(rightBasis) > 0 && canonicalEnergyPathBasis(rightBasis, "") != "direct_zone_energy") {
				current.AllocationExplanation = "Direct zone energy takes precedence; remaining carrier branches are allocated by HVAC service-path load share"
			}
		} else {
			current.Basis = "derived_ratio"
		}
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
	correspondenceFromID, correspondenceToID, sourceCorrespondence := energyPathSourceCorrespondenceEndpoints(edge, fromID, toID, canonicalNodes)
	if legacyEnergyLinkIsSourceCorrespondence(edge) && !sourceCorrespondence {
		// Only Lighting/Equipment thermal effects have a measured site-energy
		// counterpart. In particular, People heat is a driver only; accepting an
		// arbitrary legacy edge here would manufacture a direct People end use.
		return EnergyPathLink{}, false
	}

	conversionService := ""
	if legacyEnergyLinkIsLoadToEndUse(edge) {
		var valid bool
		conversionService, valid = energyPathThermalConversionService(edge, legacyFrom, legacyTo)
		if !valid {
			return EnergyPathLink{}, false
		}
	}
	carrierSplit := legacyEnergyLinkIsEndUseToCarrier(edge)
	supportSupply := legacyEnergyLinkIsSupportSupply(edge)
	if carrierSplit {
		// The carrier-qualified end-use node is the authoritative branch
		// evidence. A malformed stored-v1 edge must not redirect an electricity
		// meter into natural gas (or any other carrier) merely by naming the
		// wrong facility endpoint.
		if !legacyEnergyNodeIsCarrier(legacyFrom) || legacyEnergyNodeIsCarrier(legacyTo) ||
			canonicalEnergyPathCarrier(legacyFrom.Carrier) != canonicalEnergyPathCarrier(legacyTo.Carrier) {
			return EnergyPathLink{}, false
		}
	}
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
	if supportSupply {
		// Supply-context ribbons are evidenced by the purchased/produced/sold
		// or storage source, never by the facility consumption meter.
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
		if !carrierSplit && !supportSupply {
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
		if !carrierSplit && !supportSupply {
			link.SourceIDs = appendUniqueStrings(link.SourceIDs, canonical.SourceIDs...)
		}
		link.RelatedPathIDs = appendUniqueStrings(link.RelatedPathIDs, canonical.RelatedPathIDs...)
		link.ZoneName = firstNonEmpty(link.ZoneName, canonical.ZoneName)
	}
	value := math.Abs(firstNonZero(edge.DisplayValue, edge.Value))
	switch {
	case sourceCorrespondence:
		link.FromID = correspondenceFromID
		link.ToID = correspondenceToID
		link.Relation = energyPathRelationSourceCorrespondence
		synchronizeEnergyPathSourceCorrespondence(&link, canonicalNodes)
	case legacyEnergyLinkIsLoadToEndUse(edge):
		link.FromID = toID
		link.ToID = fromID
		link.Relation = "load_to_end_use"
		link.ServiceKind = conversionService
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
		setEnergyPathConversionRatioKind(&link, canonicalNodes[toID], canonicalNodes[fromID])
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
	case supportSupply:
		link.FromID = toID
		link.ToID = fromID
		link.Relation = "support_supply"
		link.FromValue = math.Abs(firstNonZero(energyExplanationEffectiveNodeValue(legacyTo), value))
		link.ToValue = link.FromValue
		link.FromUnit = firstNonEmpty(legacyTo.Unit, edge.Unit)
		link.ToUnit = firstNonEmpty(legacyFrom.Unit, edge.Unit)
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
		return EnergyPathLink{}, false
	}
	if canonical := canonicalNodes[link.FromID]; canonical != nil && strings.TrimSpace(canonical.Unit) != "" {
		link.FromUnit = canonical.Unit
	}
	if canonical := canonicalNodes[link.ToID]; canonical != nil && strings.TrimSpace(canonical.Unit) != "" {
		link.ToUnit = canonical.Unit
	}
	link.Basis = canonicalEnergyPathBasis(edge.Basis, edge.RuleID)
	if carrierSplit {
		endpointBasis := energyPathPreferredAllocationBasis(legacyFrom.Basis, legacyTo.Basis)
		if energyPathAllocationBasisRank(endpointBasis) > 0 {
			link.Basis = endpointBasis
			if endpointBasis == "service_path_allocation" {
				link.Explanation = firstNonEmpty(legacyTo.AllocationExplanation, legacyFrom.AllocationExplanation, link.Explanation)
			} else if endpointBasis == "zone_load_allocation" {
				link.Explanation = firstNonEmpty(legacyTo.AllocationExplanation, legacyFrom.AllocationExplanation, link.Explanation)
			}
		}
		if legacyTo.AllocationApplied {
			link.RelatedPathIDs = appendUniqueStrings(nil, legacyTo.RelatedPathIDs...)
		} else if legacyFrom.AllocationApplied {
			link.RelatedPathIDs = appendUniqueStrings(nil, legacyFrom.RelatedPathIDs...)
		}
	}
	if energyPathLegacyNodeIsDirectZoneEnergy(legacyFrom) || energyPathLegacyNodeIsDirectZoneEnergy(legacyTo) {
		link.Basis = "direct_zone_energy"
	}
	if carrierSplit || energyPathLinkIsSourceCorrespondence(link) {
		sort.Strings(link.SourceIDs)
		sort.Strings(link.RelatedPathIDs)
	}
	link.ID = energyPathLinkID(link)
	finalizeEnergyPathLinkRatio(&link)
	return link, link.FromID != "" && link.ToID != "" && link.Relation != ""
}

func setEnergyPathConversionRatioKind(link *EnergyPathLink, canonicalLoad *EnergyExplanationNode, canonicalEndUse *EnergyExplanationNode) {
	if link == nil {
		return
	}
	link.Ratio = 0
	link.RatioKind = ""
	link.RatioLabel = ""
	if canonicalLoad == nil || canonicalEndUse == nil ||
		!strings.EqualFold(canonicalLoad.Level, "load") || !strings.EqualFold(canonicalEndUse.Level, "end_use") {
		return
	}
	service := energyCanonicalServiceKind(firstNonEmpty(canonicalLoad.ServiceKind, link.ServiceKind))
	endUse := canonicalEnergyPathEndUse(canonicalEndUse.EndUse)
	if (service != "cooling" && service != "heating") || endUse != service {
		return
	}
	// Signed or non-finite canonical endpoints cannot support a physically
	// interpretable conversion label. The ribbon may retain its magnitude for
	// traceability, but no ratio is advertised.
	if !energyPathPositiveFinite(energyExplanationEffectiveNodeValue(*canonicalLoad)) ||
		!energyPathPositiveFinite(energyExplanationEffectiveNodeValue(*canonicalEndUse)) ||
		!energyPathConversionValuesValid(*link) {
		return
	}

	carriers := appendUniqueStrings(nil, canonicalEndUse.endUseCarriers...)
	for index := range carriers {
		carriers[index] = canonicalEnergyPathCarrier(carriers[index])
	}
	if len(carriers) != 1 {
		if len(carriers) > 1 {
			link.RatioKind = "load_to_site_energy"
			link.RatioLabel = "Load / site energy"
		}
		return
	}
	carrier := carriers[0]
	if energyPathCarrierIsPurchasedDistrictEnergy(carrier) {
		link.RatioKind = "load_to_purchased_energy"
		link.RatioLabel = "Load / purchased energy"
		return
	}
	if service == "cooling" && carrier == "electricity" {
		link.RatioKind = "coefficient_of_performance"
		link.RatioLabel = "COP"
		return
	}
	if service == "heating" && energyPathCarrierUsesCombustionEfficiency(carrier) {
		ratio := link.FromValue / link.ToValue
		if ratio <= 1+1e-9 {
			link.RatioKind = "efficiency"
			link.RatioLabel = "Efficiency"
		} else {
			// A delivered-load/fuel ratio remains useful above one, but calling
			// it an efficiency would assert a physical interpretation that the
			// meter-only evidence cannot support.
			link.RatioKind = "load_to_fuel"
			link.RatioLabel = "Load / fuel"
		}
		return
	}
	if service == "heating" && carrier == "electricity" {
		// Electricity alone does not distinguish resistance heat from a heat
		// pump, so do not infer a heating COP.
		link.RatioKind = "load_to_site_energy"
		link.RatioLabel = "Load / site energy"
	}
}

func energyPathCarrierIsPurchasedDistrictEnergy(carrier string) bool {
	switch canonicalEnergyPathCarrier(carrier) {
	case "district_cooling", "district_heating", "steam":
		return true
	default:
		return false
	}
}

func energyPathCarrierUsesCombustionEfficiency(carrier string) bool {
	switch canonicalEnergyPathCarrier(carrier) {
	case "natural_gas", "propane", "fuel_oil_1", "fuel_oil_2", "other_fuel_1", "other_fuel_2", "coal", "diesel", "gasoline":
		return true
	default:
		return false
	}
}

func energyPathLinkIsSourceCorrespondence(link EnergyPathLink) bool {
	return strings.EqualFold(strings.TrimSpace(link.Relation), energyPathRelationSourceCorrespondence)
}

func synchronizeEnergyPathSourceCorrespondence(link *EnergyPathLink, nodes map[string]*EnergyExplanationNode) {
	if link == nil || !energyPathLinkIsSourceCorrespondence(*link) {
		return
	}
	// A correspondence carries the independently measured values at its two
	// endpoints. It is not a conserved quantity and must never be accumulated
	// once per legacy edge like a flow ribbon.
	if from := nodes[link.FromID]; from != nil {
		link.FromValue = math.Abs(energyExplanationEffectiveNodeValue(*from))
		link.FromUnit = from.Unit
		link.ServiceKind = from.ServiceKind
		link.ZoneName = from.ZoneName
		link.SourceIDs = appendUniqueStrings(link.SourceIDs, from.SourceIDs...)
		link.RelatedPathIDs = appendUniqueStrings(link.RelatedPathIDs, from.RelatedPathIDs...)
	}
	if to := nodes[link.ToID]; to != nil {
		link.ToValue = math.Abs(energyExplanationEffectiveNodeValue(*to))
		link.ToUnit = to.Unit
		if link.ZoneName == "" {
			link.ZoneName = to.ZoneName
		} else if to.ZoneName != "" && !strings.EqualFold(link.ZoneName, to.ZoneName) {
			link.ZoneName = ""
		}
		link.SourceIDs = appendUniqueStrings(link.SourceIDs, to.SourceIDs...)
		link.RelatedPathIDs = appendUniqueStrings(link.RelatedPathIDs, to.RelatedPathIDs...)
	}
	link.Ratio = 0
	link.RatioKind = ""
	link.RatioLabel = ""
	link.SourceIDs = appendUniqueStrings(nil, link.SourceIDs...)
	link.RelatedPathIDs = appendUniqueStrings(nil, link.RelatedPathIDs...)
	sort.Strings(link.SourceIDs)
	sort.Strings(link.RelatedPathIDs)
}

func mergeEnergyPathLink(links map[string]*EnergyPathLink, next EnergyPathLink) {
	current := links[next.ID]
	if current == nil {
		copy := next
		links[next.ID] = &copy
		return
	}
	current.SourceIDs = appendUniqueStrings(current.SourceIDs, next.SourceIDs...)
	current.RelatedPathIDs = appendUniqueStrings(current.RelatedPathIDs, next.RelatedPathIDs...)
	if current.Basis != next.Basis {
		preferred := energyPathPreferredAllocationBasis(current.Basis, next.Basis)
		if energyPathAllocationBasisRank(preferred) > 0 {
			if energyPathAllocationBasisRank(next.Basis) > energyPathAllocationBasisRank(current.Basis) {
				current.Explanation = next.Explanation
				current.RuleID = next.RuleID
			}
			current.Basis = preferred
		} else {
			current.Basis = "derived_ratio"
		}
	}
	current.Explanation = firstNonEmpty(current.Explanation, next.Explanation)
	if energyPathLinkIsSourceCorrespondence(*current) && energyPathLinkIsSourceCorrespondence(next) {
		// Both values describe endpoints, not contributions. Every duplicate
		// canonical pair therefore merges provenance only.
		if current.FromValue == 0 {
			current.FromValue = next.FromValue
		}
		if current.ToValue == 0 {
			current.ToValue = next.ToValue
		}
		current.SourceIDs = appendUniqueStrings(nil, current.SourceIDs...)
		current.RelatedPathIDs = appendUniqueStrings(nil, current.RelatedPathIDs...)
		sort.Strings(current.SourceIDs)
		sort.Strings(current.RelatedPathIDs)
		finalizeEnergyPathLinkRatio(current)
		return
	}
	current.FromValue = roundedEnergyNumber(current.FromValue + next.FromValue)
	current.ToValue = roundedEnergyNumber(current.ToValue + next.ToValue)
	if current.ZoneName != next.ZoneName {
		current.ZoneName = ""
	}
	finalizeEnergyPathLinkRatio(current)
}

func finalizeEnergyPathLinkRatio(link *EnergyPathLink) {
	if link == nil {
		return
	}
	if !energyPathFinite(link.FromValue) {
		link.FromValue = 0
	}
	if !energyPathFinite(link.ToValue) {
		link.ToValue = 0
	}
	link.FromValue = roundedEnergyNumber(link.FromValue)
	link.ToValue = roundedEnergyNumber(link.ToValue)
	if energyPathLinkIsSourceCorrespondence(*link) {
		link.Ratio = 0
		link.RatioKind = ""
		link.RatioLabel = ""
		return
	}
	if link.RatioKind == "" {
		link.Ratio = 0
		link.RatioLabel = ""
		return
	}
	if !energyPathConversionValuesValid(*link) {
		link.Ratio = 0
		link.RatioKind = ""
		link.RatioLabel = ""
		return
	}
	ratio := link.FromValue / link.ToValue
	if !energyPathPositiveFinite(ratio) {
		link.Ratio = 0
		link.RatioKind = ""
		link.RatioLabel = ""
		return
	}
	link.Ratio = roundedEnergyNumber(ratio)
	if link.Ratio <= 0 {
		link.Ratio = 0
		link.RatioKind = ""
		link.RatioLabel = ""
	}
}

func energyPathConversionValuesValid(link EnergyPathLink) bool {
	if !energyPathPositiveFinite(link.FromValue) || !energyPathPositiveFinite(link.ToValue) {
		return false
	}
	if strings.EqualFold(link.Relation, "load_to_end_use") {
		fromUnit := energyPathConversionUnitBase(link.FromUnit)
		toUnit := energyPathConversionUnitBase(link.ToUnit)
		if fromUnit == "" || toUnit == "" || fromUnit != toUnit {
			return false
		}
	}
	return true
}

func energyPathPositiveFinite(value float64) bool {
	return value > 0 && energyPathFinite(value)
}

func energyPathFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func energyPathConversionUnitBase(unit string) string {
	normalized := normalizeEnergyOutputName(strings.NewReplacer(
		"(", " ", ")", " ", "[", " ", "]", " ", "_", " ",
	).Replace(unit))
	if normalized == "" {
		return ""
	}
	parts := strings.Fields(normalized)
	base := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "thermal", "site", "purchased", "fuel", "delivered", "load", "energy":
			continue
		default:
			base = append(base, part)
		}
	}
	value := strings.Join(base, " ")
	if !energyPathSupportedConversionEnergyUnit(value) {
		return ""
	}
	return value
}

func energyPathSupportedConversionEnergyUnit(unit string) bool {
	// Values reaching the v2 graph are normally normalized to kWh, while
	// stored-v1 payloads may retain another explicit energy unit. Matching
	// arbitrary labels is not enough: a dimensionless ratio is defensible only
	// when both endpoints identify a recognized energy quantity.
	switch normalizeUnitToken(unit) {
	case "j", "kj", "mj", "gj", "tj",
		"wh", "kwh", "mwh", "gwh",
		"btu", "kbtu", "mbtu", "mmbtu",
		"therm", "therms", "tonhour", "tonhours":
		return true
	default:
		return false
	}
}

func energyPathLinkID(link EnergyPathLink) string {
	return "link." + canonicalEnergyPathPart(link.Relation) + "." + metricID(link.FromID) + "." + metricID(link.ToID)
}

func canonicalEnergyPathBasis(basis string, ruleID string) string {
	switch strings.ToLower(strings.TrimSpace(basis)) {
	case "reported_meter", "reported_variable", "reported_end_use_subtotal", "integrated_rate", "heat_balance_share", "service_path_allocation", "zone_load_allocation", "direct_zone_energy", "derived_ratio", "residual":
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
		if ruleID == energyRelationshipRuleAllocatedServicePathLoad || ruleID == energyRelationshipRuleAllocatedAuxiliaryServicePath {
			return "service_path_allocation"
		}
		return "zone_load_allocation"
	default:
		if ruleID == energyRelationshipRuleAllocatedServicePathLoad {
			return "service_path_allocation"
		}
		if ruleID == energyRelationshipRuleAllocatedAuxiliaryServicePath {
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
		case energyRelationshipRuleAllocatedAuxiliaryServicePath:
			upgraded.FromLevel, upgraded.ToLevel = "end_use", "service_path_evidence"
			upgraded.Basis = "service_path_allocation"
		case energyRelationshipRuleHeatDriverBalance:
			upgraded.FromLevel, upgraded.ToLevel = "driver", "load"
			upgraded.FromKind, upgraded.ToKind = rule.ToKind, rule.FromKind
			upgraded.Basis = "heat_balance_share"
			upgraded.Formula = energyDriverAllocationFormula + "; " + energyDriverAllocationExplanation
		case energyRelationshipRuleInternalGainHeat:
			upgraded.FromLevel, upgraded.ToLevel = "driver", "end_use"
			upgraded.FromKind, upgraded.ToKind = rule.ToKind, rule.FromKind
		case energyRelationshipRulePurchasedElectricity, energyRelationshipRuleOnsiteProduction, energyRelationshipRuleStorageDischarge, energyRelationshipRuleSoldElectricity:
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

func energyPathThermalConversionService(edge EnergyExplanationEdge, legacyEndUse EnergyExplanationNode, legacyLoad EnergyExplanationNode) (string, bool) {
	if !legacyEnergyLinkIsLoadToEndUse(edge) || !strings.EqualFold(legacyLoad.Level, "load") ||
		legacyEnergyNodeIsCarrier(legacyEndUse) || legacyEnergyNodeIsSupport(legacyEndUse) {
		return "", false
	}
	endUse := canonicalEnergyPathEndUse(firstNonEmpty(legacyEndUse.EndUse, energyExplanationKindSuffix(legacyEndUse.Kind)))
	if endUse != "cooling" && endUse != "heating" {
		return "", false
	}
	// The load node is the authoritative service endpoint. Edge metadata is a
	// compatibility fallback only; trusting a stale edge service can otherwise
	// create Cooling load -> Heating energy (or an auxiliary conversion).
	service := energyCanonicalServiceKind(firstNonEmpty(legacyLoad.ServiceKind, energyExplanationKindSuffix(legacyLoad.Kind), edge.ServiceKind))
	if service != endUse {
		return "", false
	}
	return service, true
}

func legacyEnergyLinkIsDriverToLoad(edge EnergyExplanationEdge) bool {
	return strings.EqualFold(edge.Relation, "heat_driver")
}

func legacyEnergyLinkIsEndUseToCarrier(edge EnergyExplanationEdge) bool {
	return strings.EqualFold(edge.Relation, "meter_enduse") || strings.EqualFold(edge.Relation, "energy_variable")
}

func legacyEnergyLinkIsSupportSupply(edge EnergyExplanationEdge) bool {
	return strings.EqualFold(edge.Relation, "support_supply") ||
		strings.EqualFold(edge.Relation, "purchased_electricity") ||
		strings.EqualFold(edge.Relation, "onsite_production") ||
		strings.EqualFold(edge.Relation, "storage_discharge") ||
		strings.EqualFold(edge.Relation, "sold_electricity")
}

func legacyEnergyLinkIsSourceCorrespondence(edge EnergyExplanationEdge) bool {
	return strings.EqualFold(edge.Relation, "internal_gain_heat") ||
		strings.EqualFold(edge.Relation, energyPathRelationSourceCorrespondence) ||
		strings.EqualFold(edge.RuleID, energyRelationshipRuleInternalGainHeat)
}

// energyPathSourceCorrespondenceEndpoints validates and normalizes the only
// cross-stage non-flow relationships supported by the Energy Path contract.
// The direction is always Stage 1 driver -> Stage 3 end use so even consumers
// that inspect every link cannot introduce a reverse edge or a cycle.
func energyPathSourceCorrespondenceEndpoints(edge EnergyExplanationEdge, fromID string, toID string, nodes map[string]*EnergyExplanationNode) (string, string, bool) {
	if !legacyEnergyLinkIsSourceCorrespondence(edge) {
		return "", "", false
	}
	from := nodes[fromID]
	to := nodes[toID]
	if from == nil || to == nil {
		return "", "", false
	}
	driver := from
	endUse := to
	driverID := fromID
	endUseID := toID
	if strings.EqualFold(from.Level, "end_use") && strings.EqualFold(to.Level, "driver") {
		driver, endUse = to, from
		driverID, endUseID = toID, fromID
	}
	if !strings.EqualFold(driver.Level, "driver") || !strings.EqualFold(endUse.Level, "end_use") {
		return "", "", false
	}
	category := canonicalEnergyDriverCategory(firstNonEmpty(driver.DriverCategory, driver.Kind))
	endUseKind := canonicalEnergyPathEndUse(firstNonEmpty(endUse.EndUse, energyExplanationKindSuffix(endUse.Kind)))
	switch category {
	case energyDriverCategoryLighting:
		if endUseKind != "lighting" {
			return "", "", false
		}
	case energyDriverCategoryEquipment:
		if endUseKind != "equipment" {
			return "", "", false
		}
	default:
		return "", "", false
	}
	return driverID, endUseID, true
}

func legacyEnergyNodeIsCarrier(node EnergyExplanationNode) bool {
	return strings.EqualFold(node.Level, "carrier") || strings.EqualFold(node.EndUse, "total") || strings.Contains(strings.ToLower(node.ID), ".carrier.")
}

func legacyEnergyNodeIsSupport(node EnergyExplanationNode) bool {
	switch strings.ToLower(strings.TrimSpace(node.EndUse)) {
	case "generators", "storage_discharge", "onsite_production", "production", "electricity_purchased", "purchased_electricity", "electricity_sold", "sold_electricity", "surplus_sold":
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
		return canonicalEnergyPathPart("site_" + canonicalEnergyPathCarrier(firstNonEmpty(node.Carrier, "other")))
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

func canonicalEnergyPathCarrier(value string) string {
	if definition, ok := energyCarrierTaxonomyDefinitionFor(value); ok {
		return definition.Token
	}
	// `other` is retained as explicit unknown evidence for sources such as
	// Zone Other Equipment Fuel Energy. It is not a recognized carrier and
	// must never be promoted to a named fuel.
	if canonicalEnergyPathPart(value) == "other" {
		return "other"
	}
	return "other"
}

func canonicalEnergyPathEnergyUnit(unit string) string {
	if normalizeUnitToken(unit) == "kwh" {
		return "kWh"
	}
	return strings.TrimSpace(unit)
}

func energyPathEnergyUnitNormalization(unit string) (float64, bool) {
	factor, normalizedUnit, ok := energyPathGraphUnitNormalization(unit)
	if !ok || normalizedUnit != "kWh" {
		return 0, false
	}
	return factor, true
}

func energyPathGraphUnitNormalization(unit string) (float64, string, bool) {
	normalized := normalizeSimulationDisplayUnit(unit)
	if normalized.Unit != "kWh" && normalized.Unit != "m3" {
		return 0, "", false
	}
	return normalized.Factor, normalized.Unit, true
}

func energyPathNodeHasInvalidCarrierUnit(node EnergyExplanationNode) bool {
	evidence := strings.TrimSpace(node.Carrier)
	if evidence == "" && (strings.EqualFold(node.Level, "carrier") || strings.EqualFold(node.MeterHierarchyLevel, "facility_total")) {
		parts := strings.Split(strings.TrimSpace(node.Kind), ".")
		if len(parts) >= 3 && strings.EqualFold(parts[0], "energy") {
			evidence = strings.Join(parts[1:len(parts)-1], "_")
		}
	}
	definition, recognized := energyCarrierTaxonomyDefinitionFor(evidence)
	if !recognized || definition.Token == "water" {
		return false
	}
	return !energyExplanationUnitIsSiteEnergy(node.Unit)
}

func energyPathHasInvalidCarrierUnits(nodes []EnergyExplanationNode) bool {
	for _, node := range nodes {
		if energyPathNodeHasInvalidCarrierUnit(node) {
			return true
		}
	}
	return false
}

func filterEnergyPathNonSiteEnergyReconciliation(input []EnergyReconciliation) []EnergyReconciliation {
	out := make([]EnergyReconciliation, 0, len(input))
	for _, item := range input {
		level := strings.ToLower(strings.TrimSpace(item.Level))
		if (level == "energy" || level == "carrier") && !energyPathUnitIsCanonicalSiteEnergy(item.Unit) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func normalizeEnergyExplanationV1Units(input EnergyExplanationV1) EnergyExplanationV1 {
	out := input
	out.Nodes = normalizeLegacyEnergyExplanationNodes(input.Nodes)
	out.Edges = normalizeLegacyEnergyExplanationEdges(input.Edges)
	out.Reconciliation = normalizeLegacyEnergyExplanationReconciliation(input.Reconciliation)
	out.Periods = append([]EnergyPeriod(nil), input.Periods...)
	for index := range out.Periods {
		out.Periods[index].Nodes = normalizeLegacyEnergyExplanationNodes(input.Periods[index].Nodes)
		out.Periods[index].Edges = normalizeLegacyEnergyExplanationEdges(input.Periods[index].Edges)
		out.Periods[index].Reconciliation = normalizeLegacyEnergyExplanationReconciliation(input.Periods[index].Reconciliation)
	}
	out.Sources = normalizeLegacyEnergyDataSources(input.Sources)
	return out
}

func normalizeLegacyEnergyDataSources(input []EnergyDataSource) []EnergyDataSource {
	out := append([]EnergyDataSource(nil), input...)
	for index := range out {
		source := &out[index]
		factor, normalizedUnit, ok := energyPathGraphUnitNormalization(firstNonEmpty(source.NormalizedUnit, source.SourceUnit, source.Units))
		if !ok {
			continue
		}
		source.RawValue = roundedEnergyNumber(source.RawValue * factor)
		source.EffectiveValue = roundedEnergyNumber(source.EffectiveValue * factor)
		source.AllocatedValue = roundedEnergyNumber(source.AllocatedValue * factor)
		source.NormalizedUnit = normalizedUnit
		source.ScopeDetails = append([]EnergyDataSourceScopeDetail(nil), source.ScopeDetails...)
		for detailIndex := range source.ScopeDetails {
			detail := &source.ScopeDetails[detailIndex]
			detail.RawValue = roundedEnergyNumber(detail.RawValue * factor)
			detail.EffectiveValue = roundedEnergyNumber(detail.EffectiveValue * factor)
			detail.AllocatedValue = roundedEnergyNumber(detail.AllocatedValue * factor)
		}
	}
	return out
}

func normalizeLegacyEnergyExplanationNodes(input []EnergyExplanationNode) []EnergyExplanationNode {
	out := append([]EnergyExplanationNode(nil), input...)
	for index := range out {
		factor, normalizedUnit, ok := energyPathGraphUnitNormalization(out[index].Unit)
		if !ok {
			continue
		}
		node := &out[index]
		node.Value = roundedEnergyNumber(node.Value * factor)
		node.SignedValue = roundedEnergyNumber(node.SignedValue * factor)
		node.RawValue = roundedEnergyNumber(node.RawValue * factor)
		node.EffectiveValue = roundedEnergyNumber(node.EffectiveValue * factor)
		node.AllocatedValue = roundedEnergyNumber(node.AllocatedValue * factor)
		node.DisplayValue = roundedEnergyNumber(node.DisplayValue * factor)
		node.Unit = normalizedUnit
		node.LoadBreakdown = cloneEnergyExplanationLoadComponents(node.LoadBreakdown)
		for componentIndex := range node.LoadBreakdown {
			component := &node.LoadBreakdown[componentIndex]
			componentFactor := factor
			componentUnit := normalizedUnit
			if strings.TrimSpace(component.Unit) != "" {
				var componentOK bool
				componentFactor, componentUnit, componentOK = energyPathGraphUnitNormalization(component.Unit)
				if !componentOK {
					continue
				}
			}
			component.Value = roundedEnergyNumber(component.Value * componentFactor)
			component.Unit = componentUnit
		}
		node.OffsetEffects = cloneEnergyExplanationOffsetEffects(node.OffsetEffects)
		for effectIndex := range node.OffsetEffects {
			effect := &node.OffsetEffects[effectIndex]
			effectFactor := factor
			effectUnit := normalizedUnit
			if strings.TrimSpace(effect.Unit) != "" {
				var effectOK bool
				effectFactor, effectUnit, effectOK = energyPathGraphUnitNormalization(effect.Unit)
				if !effectOK {
					continue
				}
			}
			effect.RawValue = roundedEnergyNumber(effect.RawValue * effectFactor)
			effect.EffectiveValue = roundedEnergyNumber(effect.EffectiveValue * effectFactor)
			effect.Unit = effectUnit
		}
		if node.SimultaneousLoad != nil {
			node.SimultaneousLoad = cloneEnergyExplanationSimultaneousLoad(node.SimultaneousLoad)
			metricFactor := factor
			metricUnit := normalizedUnit
			metricOK := true
			if strings.TrimSpace(node.SimultaneousLoad.Unit) != "" {
				metricFactor, metricUnit, metricOK = energyPathGraphUnitNormalization(node.SimultaneousLoad.Unit)
			}
			if !metricOK {
				continue
			}
			node.SimultaneousLoad.Numerator = roundedEnergyNumber(node.SimultaneousLoad.Numerator * metricFactor)
			node.SimultaneousLoad.Denominator = roundedEnergyNumber(node.SimultaneousLoad.Denominator * metricFactor)
			node.SimultaneousLoad.Unit = metricUnit
		}
	}
	return out
}

func normalizeLegacyEnergyExplanationEdges(input []EnergyExplanationEdge) []EnergyExplanationEdge {
	out := append([]EnergyExplanationEdge(nil), input...)
	for index := range out {
		factor, normalizedUnit, ok := energyPathGraphUnitNormalization(out[index].Unit)
		if !ok {
			continue
		}
		out[index].Value = roundedEnergyNumber(out[index].Value * factor)
		out[index].SignedValue = roundedEnergyNumber(out[index].SignedValue * factor)
		out[index].DisplayValue = roundedEnergyNumber(out[index].DisplayValue * factor)
		out[index].Unit = normalizedUnit
	}
	return out
}

func normalizeLegacyEnergyExplanationReconciliation(input []EnergyReconciliation) []EnergyReconciliation {
	out := append([]EnergyReconciliation(nil), input...)
	for index := range out {
		factor, normalizedUnit, ok := energyPathGraphUnitNormalization(out[index].Unit)
		if !ok {
			continue
		}
		out[index].ExpectedValue = roundedEnergyNumber(out[index].ExpectedValue * factor)
		out[index].ExplainedValue = roundedEnergyNumber(out[index].ExplainedValue * factor)
		out[index].ResidualValue = roundedEnergyNumber(out[index].ResidualValue * factor)
		out[index].DirectValue = roundedEnergyNumber(out[index].DirectValue * factor)
		out[index].AllocatedValue = roundedEnergyNumber(out[index].AllocatedValue * factor)
		out[index].UnassignedValue = roundedEnergyNumber(out[index].UnassignedValue * factor)
		out[index].OvermappedValue = roundedEnergyNumber(out[index].OvermappedValue * factor)
		out[index].Unit = normalizedUnit
	}
	return out
}

func energyPathUnitIsCanonicalSiteEnergy(unit string) bool {
	switch normalizeUnitToken(unit) {
	case "kwh", "kwhsite":
		return true
	default:
		return false
	}
}

func energyPathSourceHasExplicitWaterConversion(source EnergyDataSource) bool {
	if !energyPathUnitIsCanonicalSiteEnergy(source.NormalizedUnit) ||
		!energyPathUnitIsWaterVolume(source.SourceUnit) ||
		strings.TrimSpace(source.Formula) == "" {
		return false
	}
	return true
}

func energyPathUnitIsWaterVolume(unit string) bool {
	return normalizeSimulationDisplayUnit(unit).Unit == "m3"
}

func energyPathWaterNodeHasExplicitSiteEnergyConversion(node EnergyExplanationNode, sources []EnergyDataSource) bool {
	if canonicalEnergyPathCarrier(node.Carrier) != "water" ||
		!strings.EqualFold(strings.TrimSpace(node.Basis), "derived_ratio") ||
		!energyPathUnitIsCanonicalSiteEnergy(node.Unit) {
		return false
	}
	sourceByID := make(map[string]EnergyDataSource, len(sources))
	for _, source := range sources {
		sourceByID[source.ID] = source
	}
	if len(node.SourceIDs) == 0 {
		return false
	}
	for _, sourceID := range node.SourceIDs {
		if source, ok := sourceByID[sourceID]; !ok || !energyPathSourceHasExplicitWaterConversion(source) {
			return false
		}
	}
	return true
}

func energyPathNodeIsContextOnlyWater(node EnergyExplanationNode, sources []EnergyDataSource) bool {
	return canonicalEnergyPathCarrier(node.Carrier) == "water" &&
		!energyPathWaterNodeHasExplicitSiteEnergyConversion(node, sources)
}

func annotateEnergyPathWaterContextSources(input []EnergyDataSource, nodes []EnergyExplanationNode) []EnergyDataSource {
	waterSourceIDs := map[string]bool{}
	unconvertedSourceIDs := map[string]bool{}
	for _, source := range input {
		if !energyExplanationNameIsWaterMeter(firstNonEmpty(source.Name, source.KeyValue)) {
			continue
		}
		waterSourceIDs[source.ID] = true
		if !energyPathSourceHasExplicitWaterConversion(source) {
			unconvertedSourceIDs[source.ID] = true
		}
	}
	for _, node := range nodes {
		if canonicalEnergyPathCarrier(node.Carrier) != "water" {
			continue
		}
		for _, sourceID := range node.SourceIDs {
			waterSourceIDs[sourceID] = true
			if energyPathNodeIsContextOnlyWater(node, input) {
				unconvertedSourceIDs[sourceID] = true
			}
		}
	}
	out := append([]EnergyDataSource(nil), input...)
	for index := range out {
		if !waterSourceIDs[out[index].ID] {
			continue
		}
		out[index].InspectorSection = "context"
		out[index].NormalizedUnit = firstNonEmpty(canonicalEnergyPathEnergyUnit(out[index].NormalizedUnit), strings.TrimSpace(out[index].SourceUnit))
		if normalized := normalizeSimulationDisplayUnit(out[index].NormalizedUnit); normalized.Unit != "" {
			out[index].NormalizedUnit = normalized.Unit
		}
		if unconvertedSourceIDs[out[index].ID] {
			out[index].Explanation = firstNonEmpty(
				out[index].Explanation,
				"Water utility use is context only and is excluded from the site-energy flow unless an explicit site-energy conversion is provided.",
			)
		} else {
			out[index].Explanation = firstNonEmpty(out[index].Explanation, "Water utility source with an explicit site-energy conversion.")
		}
	}
	return out
}

func normalizeEnergyPathWaterContextCompleteness(input EnergyCompleteness, sources []EnergyDataSource, reconciliation []EnergyReconciliation) EnergyCompleteness {
	out := input
	out.Items = append([]EnergyCompletenessLevel(nil), input.Items...)
	out.SourceAvailability = append([]EnergySourceAvailabilityEntry(nil), input.SourceAvailability...)
	sourceByID := make(map[string]EnergyDataSource, len(sources))
	rawWaterGroups := map[string]bool{}
	hasRawWaterContext := false
	for _, source := range sources {
		sourceByID[source.ID] = source
		if !strings.EqualFold(strings.TrimSpace(source.InspectorSection), "context") ||
			energyPathSourceHasExplicitWaterConversion(source) {
			continue
		}
		if energyExplanationNameIsWaterMeter(firstNonEmpty(source.Name, source.KeyValue)) {
			hasRawWaterContext = true
			rawWaterGroups[expectedEnergyExplanationOutputGroupKey(firstNonEmpty(source.Name, source.KeyValue), "energy")] = true
		}
	}

	movedWaterAvailability := false
	hasWaterAvailability := false
	for index := range out.SourceAvailability {
		entry := &out.SourceAvailability[index]
		if !energyPathAvailabilityIsWater(entry.Name) {
			continue
		}
		hasWaterAvailability = true
		if !strings.EqualFold(strings.TrimSpace(entry.Level), "energy") {
			continue
		}
		hasExplicitConversion := false
		for _, sourceID := range entry.SourceIDs {
			if energyPathSourceHasExplicitWaterConversion(sourceByID[sourceID]) {
				hasExplicitConversion = true
				break
			}
		}
		if hasExplicitConversion {
			continue
		}
		entry.Level = "context"
		movedWaterAvailability = true
		hasRawWaterContext = true
	}
	energyLevelChanged := false
	if movedWaterAvailability {
		foundByGroup := map[string]bool{}
		for _, entry := range out.SourceAvailability {
			if !strings.EqualFold(strings.TrimSpace(entry.Level), "energy") ||
				strings.EqualFold(entry.Status, "not_applicable") || strings.EqualFold(entry.Status, "not_requested") {
				continue
			}
			key := expectedEnergyExplanationOutputGroupKey(entry.Name, "energy")
			if _, exists := foundByGroup[key]; !exists {
				foundByGroup[key] = false
			}
			if strings.EqualFold(entry.Status, "found") {
				foundByGroup[key] = true
			}
		}
		found := 0
		for _, groupFound := range foundByGroup {
			if groupFound {
				found++
			}
		}
		out.EnergyUse = energyCompletenessLevel("energy", found, len(foundByGroup), "Energy Use")
		energyLevelChanged = true
	} else if hasRawWaterContext && !hasWaterAvailability && len(rawWaterGroups) > 0 {
		found := maxInt(0, out.EnergyUse.Found-len(rawWaterGroups))
		total := maxInt(0, out.EnergyUse.Total-len(rawWaterGroups))
		out.EnergyUse = energyCompletenessLevel("energy", found, total, "Energy Use")
		energyLevelChanged = true
	}
	if energyLevelChanged {
		for index := range out.Items {
			if strings.EqualFold(out.Items[index].Level, "energy") {
				out.Items[index] = out.EnergyUse
			}
		}
		out.Status = "complete"
		if out.EnergyUse.Status != "complete" || out.DeliveredLoad.Status == "missing" || out.HeatDrivers.Status == "missing" {
			out.Status = "partial"
		}
		if out.EnergyUse.Found == 0 && out.DeliveredLoad.Found == 0 && out.HeatDrivers.Found == 0 {
			out.Status = "missing"
		}
		out.MissingCategories = missingEnergySourceCategories(out.SourceAvailability)
	}
	hasCanonicalWaterEnergyReconciliation := false
	for _, item := range reconciliation {
		if strings.EqualFold(item.Level, "energy") && energyExplanationUnitIsSiteEnergy(item.Unit) && energyPathReconciliationIsWater(item) {
			hasCanonicalWaterEnergyReconciliation = true
			break
		}
	}
	if hasRawWaterContext || hasCanonicalWaterEnergyReconciliation {
		out.MappedPercent = energyExplanationMappedPercentFromReconciliation(reconciliation)
	}
	return out
}

func energyPathAvailabilityIsWater(name string) bool {
	return energyExplanationNameIsWaterMeter(name)
}

func filterEnergyPathContextOnlyWaterReconciliation(input []EnergyReconciliation, nodes []EnergyExplanationNode) []EnergyReconciliation {
	convertedWaterSourceIDs := map[string]bool{}
	for _, node := range nodes {
		if node.Level != "carrier" || canonicalEnergyPathCarrier(node.Carrier) != "water" ||
			!strings.EqualFold(strings.TrimSpace(node.Basis), "derived_ratio") ||
			!energyPathUnitIsCanonicalSiteEnergy(node.Unit) {
			continue
		}
		for _, sourceID := range node.SourceIDs {
			convertedWaterSourceIDs[sourceID] = true
		}
	}
	out := make([]EnergyReconciliation, 0, len(input))
	for _, item := range input {
		if !energyPathReconciliationIsWater(item) {
			out = append(out, item)
			continue
		}
		if len(convertedWaterSourceIDs) == 0 || !energyPathUnitIsCanonicalSiteEnergy(item.Unit) ||
			!energyExplanationSourcesIntersect(item.SourceIDs, convertedWaterSourceIDs) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func removeEnergyPathWaterReconciliation(input []EnergyReconciliation) []EnergyReconciliation {
	out := make([]EnergyReconciliation, 0, len(input))
	for _, item := range input {
		if !energyPathReconciliationIsWater(item) {
			out = append(out, item)
		}
	}
	return out
}

func energyPathReconciliationIsWater(item EnergyReconciliation) bool {
	id := strings.ToLower(strings.TrimSpace(item.ID))
	if strings.HasPrefix(id, "reconcile.energy.water.") || id == "reconcile.energy.water" {
		return true
	}
	label := normalizeEnergyOutputName(item.Label)
	return label == "watertotal" || strings.HasPrefix(label, "watertotalbasis") || strings.HasPrefix(label, "waterobserved")
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
	if explanation.Schema == "" {
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
		Quality:      BuildEnergyPathQuality(explanation, ""),
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
		if energyPathLinkIsSourceCorrespondence(link) || link.Ratio == 0 || link.RatioKind == "" {
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
		Sources           []energyPathSourceWire         `json:"sources,omitempty"`
		Completeness      EnergyCompleteness             `json:"completeness"`
		Quality           *EnergyPathQuality             `json:"quality,omitempty"`
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
		Sources:           energyPathSourcesForWire(result.Sources),
		Completeness:      result.Completeness,
		Quality:           result.Quality,
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
	result.sanitizedOnRead = sanitizeEnergyExplanationV2Result(result)
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
		Quality           *EnergyPathQuality             `json:"quality,omitempty"`
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
		Quality:           period.Quality,
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
		decoded.AllocationPolicy == "" && decoded.Quality == nil && !energyCompletenessHasContent(decoded.Completeness) {
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

func energyPathCompletenessFromCarrierReconciliation(input EnergyCompleteness, scope EnergyExplanationScope, reconciliation []EnergyReconciliation) EnergyCompleteness {
	input.MappedPercent = 0
	if normalizeEnergyExplanationScope(scope).Kind == "building" {
		input.MappedPercent = energyExplanationMappedPercentFromReconciliation(reconciliation)
	}
	return input
}

func sanitizeEnergyExplanationV2Result(result *EnergyExplanationResult) bool {
	if result == nil {
		return false
	}
	changed := false
	originalSources := result.Sources
	result.Sources = normalizeLegacyEnergyDataSources(result.Sources)
	allNodes := append([]EnergyExplanationNode(nil), result.Nodes...)
	for _, period := range result.Periods {
		allNodes = append(allNodes, period.Nodes...)
	}
	for _, zone := range result.ZoneResults {
		allNodes = append(allNodes, zone.Nodes...)
		for _, period := range zone.Periods {
			allNodes = append(allNodes, period.Nodes...)
		}
	}
	result.Sources = annotateEnergyPathWaterContextSources(result.Sources, allNodes)
	changed = changed || !reflect.DeepEqual(originalSources, result.Sources)

	var graphChanged bool
	result.Nodes, result.Links, result.Reconciliation, graphChanged = sanitizeEnergyExplanationV2Graph(
		result.Nodes, result.Links, result.Reconciliation, result.Sources, "annual", result.Scope,
	)
	changed = changed || graphChanged
	originalCompleteness := result.Completeness
	result.Completeness = normalizeEnergyPathWaterContextCompleteness(result.Completeness, result.Sources, result.Reconciliation)
	result.Completeness = energyPathCompletenessFromCarrierReconciliation(result.Completeness, result.Scope, result.Reconciliation)
	changed = changed || !reflect.DeepEqual(originalCompleteness, result.Completeness)
	result.ZoneContributions, graphChanged = filterEnergyPathRawWaterSummaryItems(result.ZoneContributions)
	changed = changed || graphChanged

	for index := range result.Periods {
		period := &result.Periods[index]
		period.Nodes, period.Links, period.Reconciliation, graphChanged = sanitizeEnergyExplanationV2Graph(
			period.Nodes, period.Links, period.Reconciliation, result.Sources, period.ID, result.Scope,
		)
		changed = changed || graphChanged
		period.ZoneContributions, graphChanged = filterEnergyPathRawWaterSummaryItems(period.ZoneContributions)
		changed = changed || graphChanged
		if period.Summary != nil {
			summary := buildEnergyExplanationSummaryV2(EnergyExplanationResult{
				Schema:            energyExplanationSchema,
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
			changed = changed || !reflect.DeepEqual(*period.Summary, summary)
			period.Summary = &summary
		}
	}

	for index := range result.ZoneResults {
		zone := &result.ZoneResults[index]
		zone.Scope = normalizeEnergyExplanationScope(zone.Scope)
		zone.Nodes, zone.Links, zone.Reconciliation, graphChanged = sanitizeEnergyExplanationV2Graph(
			zone.Nodes, zone.Links, zone.Reconciliation, result.Sources, "annual", zone.Scope,
		)
		changed = changed || graphChanged
		originalZoneCompleteness := zone.Completeness
		zone.Completeness = normalizeEnergyPathWaterContextCompleteness(zone.Completeness, result.Sources, zone.Reconciliation)
		zone.Completeness = energyPathCompletenessFromCarrierReconciliation(zone.Completeness, zone.Scope, zone.Reconciliation)
		changed = changed || !reflect.DeepEqual(originalZoneCompleteness, zone.Completeness)
		zone.ZoneContributions, graphChanged = filterEnergyPathRawWaterSummaryItems(zone.ZoneContributions)
		changed = changed || graphChanged
		for periodIndex := range zone.Periods {
			period := &zone.Periods[periodIndex]
			period.Nodes, period.Links, period.Reconciliation, graphChanged = sanitizeEnergyExplanationV2Graph(
				period.Nodes, period.Links, period.Reconciliation, result.Sources, period.ID, zone.Scope,
			)
			changed = changed || graphChanged
			period.ZoneContributions, graphChanged = filterEnergyPathRawWaterSummaryItems(period.ZoneContributions)
			changed = changed || graphChanged
			if period.Summary != nil {
				summary := buildEnergyExplanationSummaryV2(EnergyExplanationResult{
					Schema:            energyExplanationSchema,
					Purpose:           result.Purpose,
					Scope:             zone.Scope,
					Frequency:         result.Frequency,
					AllocationPolicy:  result.AllocationPolicy,
					Nodes:             period.Nodes,
					Links:             period.Links,
					Reconciliation:    period.Reconciliation,
					Completeness:      zone.Completeness,
					Warnings:          period.Warnings,
					ZoneContributions: period.ZoneContributions,
				})
				summary.Period = period.ID
				summary.AllocationPolicy = result.AllocationPolicy
				changed = changed || !reflect.DeepEqual(*period.Summary, summary)
				period.Summary = &summary
			}
		}
		zoneSummary := buildEnergyExplanationSummaryV2(EnergyExplanationResult{
			Schema:            energyExplanationSchema,
			Purpose:           result.Purpose,
			Scope:             zone.Scope,
			Frequency:         result.Frequency,
			AllocationPolicy:  result.AllocationPolicy,
			Nodes:             zone.Nodes,
			Links:             zone.Links,
			Reconciliation:    zone.Reconciliation,
			Completeness:      zone.Completeness,
			Warnings:          zone.Warnings,
			ZoneContributions: zone.ZoneContributions,
		})
		changed = changed || !reflect.DeepEqual(zone.Summary, zoneSummary)
		zone.Summary = zoneSummary
	}
	qualityChanged := refreshEnergyPathQuality(result)
	changed = changed || qualityChanged
	return changed
}

func sanitizeEnergyExplanationV2Graph(nodes []EnergyExplanationNode, links []EnergyPathLink, reconciliation []EnergyReconciliation, sources []EnergyDataSource, period string, scope EnergyExplanationScope) ([]EnergyExplanationNode, []EnergyPathLink, []EnergyReconciliation, bool) {
	originalNodes := nodes
	originalLinks := links
	originalReconciliation := reconciliation
	nodes = normalizeLegacyEnergyExplanationNodes(nodes)
	normalizeEnergyExplanationV2Nodes(nodes, scope)
	links = normalizeEnergyExplanationV2Links(links)
	reconciliation = normalizeLegacyEnergyExplanationReconciliation(reconciliation)
	reconciliation = filterEnergyPathNonSiteEnergyReconciliation(reconciliation)
	nodeByID := make(map[string]*EnergyExplanationNode, len(nodes))
	for index := range nodes {
		nodeByID[nodes[index].ID] = &nodes[index]
	}
	disallowed := map[string]bool{}
	for index := range nodes {
		node := &nodes[index]
		if (node.Level == "end_use" || node.Level == "support") && strings.TrimSpace(node.Unit) != "" && !energyExplanationUnitIsSiteEnergy(node.Unit) {
			disallowed[node.ID] = true
			continue
		}
		if node.Level != "carrier" {
			continue
		}
		evidence := energyPathV2CarrierEvidence(*node)
		definition, recognized := energyCarrierTaxonomyDefinitionFor(evidence)
		if !recognized {
			if strings.TrimSpace(evidence) != "" && canonicalEnergyPathPart(evidence) != "other" {
				disallowed[node.ID] = true
			}
			continue
		}
		if definition.Token == "water" {
			if !energyPathWaterNodeHasExplicitSiteEnergyConversion(*node, sources) {
				disallowed[node.ID] = true
				continue
			}
		} else if !energyExplanationUnitIsSiteEnergy(node.Unit) {
			disallowed[node.ID] = true
			continue
		}
		node.Carrier = definition.Token
		node.Label = definition.Label
		node.Unit = "kWh"
		node.ScaleDomain = "site"
	}

	linksByEndUse := map[string][]EnergyPathLink{}
	for _, link := range links {
		if link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier" {
			continue
		}
		from := nodeByID[link.FromID]
		to := nodeByID[link.ToID]
		if from == nil || to == nil || from.Level != "end_use" || to.Level != "carrier" {
			continue
		}
		linksByEndUse[link.FromID] = append(linksByEndUse[link.FromID], link)
	}
	for endUseID, branches := range linksByEndUse {
		allDisallowed := len(branches) > 0
		for _, branch := range branches {
			if !disallowed[branch.ToID] {
				allDisallowed = false
				break
			}
		}
		if allDisallowed {
			disallowed[endUseID] = true
		}
	}

	filteredNodes := make([]EnergyExplanationNode, 0, len(nodes))
	visibleNodeIDs := map[string]bool{}
	for _, node := range nodes {
		if disallowed[node.ID] {
			continue
		}
		if (node.Level == "end_use" || node.Level == "support") && energyExplanationUnitIsSiteEnergy(node.Unit) {
			node.Unit = "kWh"
			node.ScaleDomain = "site"
		}
		visibleNodeIDs[node.ID] = true
		filteredNodes = append(filteredNodes, node)
	}
	visibleNodeByID := make(map[string]EnergyExplanationNode, len(filteredNodes))
	for _, node := range filteredNodes {
		visibleNodeByID[node.ID] = node
	}
	filteredLinks := make([]EnergyPathLink, 0, len(links))
	prunedEndUseSplits := map[string]bool{}
	for _, link := range links {
		if !visibleNodeIDs[link.FromID] || !visibleNodeIDs[link.ToID] {
			if energyPathLinkIsCarrierSplit(link) && nodeByID[link.FromID] != nil && nodeByID[link.FromID].Level == "end_use" {
				prunedEndUseSplits[link.FromID] = true
			}
			continue
		}
		if energyPathLinkIsCarrierSplit(link) {
			// Preserve the original unit/domain evidence before endpoint labels
			// are copied onto a stored same-domain carrier split.
			if !energyPathStoredCarrierSplitValid(link, *nodeByID[link.FromID], *nodeByID[link.ToID]) {
				prunedEndUseSplits[link.FromID] = true
				continue
			}
		}
		if link.Relation == "load_to_end_use" {
			// Do not erase invalid unit evidence by merely copying endpoint
			// labels onto a stored conversion's unconverted values.
			fromUnit, toUnit := energyPathConversionUnitBase(link.FromUnit), energyPathConversionUnitBase(link.ToUnit)
			if fromUnit == "" || toUnit == "" ||
				!strings.EqualFold(nodeByID[link.FromID].ScaleDomain, "thermal") ||
				!strings.EqualFold(nodeByID[link.ToID].ScaleDomain, "site") ||
				fromUnit != energyPathConversionUnitBase(visibleNodeByID[link.FromID].Unit) ||
				toUnit != energyPathConversionUnitBase(visibleNodeByID[link.ToID].Unit) {
				continue
			}
		}
		if node := visibleNodeByID[link.FromID]; node.Unit != "" {
			link.FromUnit = node.Unit
		}
		if node := visibleNodeByID[link.ToID]; node.Unit != "" {
			link.ToUnit = node.Unit
		}
		if link.Relation == "support_supply" {
			support := visibleNodeByID[link.FromID]
			if support.Level != "support" {
				support = visibleNodeByID[link.ToID]
			}
			if support.Level == "support" {
				link.SourceIDs = appendUniqueStrings(nil, support.SourceIDs...)
			}
		}
		filteredLinks = append(filteredLinks, link)
	}
	filteredNodes, filteredLinks = refreshEnergyPathEndUseCarrierSplits(filteredNodes, filteredLinks, prunedEndUseSplits)
	filteredLinks = refreshEnergyPathAllocatedDriverLinks(filteredNodes, filteredLinks, firstNonEmpty(period, "annual"))
	filteredLinks = refreshEnergyPathConversionLinks(filteredNodes, filteredLinks)

	reconciliation = removeEnergyPathWaterReconciliation(reconciliation)
	reconciliation = reconcileEnergyPathCarrierTotals(filteredNodes, filteredLinks, reconciliation, firstNonEmpty(period, "annual"))
	reconciliation = filterEnergyPathContextOnlyWaterReconciliation(reconciliation, filteredNodes)
	filteredNodes, filteredLinks = rebuildEnergyPathCarrierResidualPresentation(filteredNodes, filteredLinks, reconciliation, firstNonEmpty(period, "annual"))
	changed := !reflect.DeepEqual(originalNodes, filteredNodes) || !reflect.DeepEqual(originalLinks, filteredLinks) || !reflect.DeepEqual(originalReconciliation, reconciliation)
	return filteredNodes, filteredLinks, reconciliation, changed
}

func normalizeEnergyExplanationV2Links(input []EnergyPathLink) []EnergyPathLink {
	out := append([]EnergyPathLink(nil), input...)
	for index := range out {
		link := &out[index]
		if factor, unit, ok := energyPathGraphUnitNormalization(link.FromUnit); ok {
			link.FromValue = roundedEnergyNumber(link.FromValue * factor)
			link.FromUnit = unit
		}
		if factor, unit, ok := energyPathGraphUnitNormalization(link.ToUnit); ok {
			link.ToValue = roundedEnergyNumber(link.ToValue * factor)
			link.ToUnit = unit
		}
	}
	return out
}

func energyPathV2CarrierEvidence(node EnergyExplanationNode) string {
	if strings.TrimSpace(node.Carrier) != "" {
		return node.Carrier
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(node.Kind)), "carrier.") {
		return energyExplanationKindSuffix(node.Kind)
	}
	parts := strings.Split(strings.TrimSpace(node.ID), ".")
	if len(parts) > 1 && strings.EqualFold(parts[0], "carrier") {
		return parts[1]
	}
	return ""
}

func filterEnergyPathRawWaterSummaryItems(input []EnergyExplanationSummaryItem) ([]EnergyExplanationSummaryItem, bool) {
	out := make([]EnergyExplanationSummaryItem, 0, len(input))
	for _, item := range input {
		carrier := item.Carrier
		if carrier == "" {
			parts := strings.Split(strings.TrimSpace(item.ID), ".")
			for index := 0; index+1 < len(parts); index++ {
				if strings.EqualFold(parts[index], "carrier") {
					carrier = parts[index+1]
					break
				}
			}
		}
		if canonicalEnergyPathCarrier(carrier) == "water" && !energyExplanationUnitIsSiteEnergy(item.Unit) {
			continue
		}
		out = append(out, item)
	}
	return out, !reflect.DeepEqual(input, out)
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
	if bundle.EnergyExplanation.Schema == energyExplanationSchema {
		if bundle.EnergyExplanation.upgradedFromV1 || bundle.EnergyExplanation.sanitizedOnRead || bundle.EnergyExplanationSummary.Schema == "" || !reflect.DeepEqual(bundle.EnergyExplanationSummary.Quality, bundle.EnergyExplanation.Quality) || len(bundle.EnergyExplanationSummary.Drivers)+len(bundle.EnergyExplanationSummary.Loads)+len(bundle.EnergyExplanationSummary.EndUses)+len(bundle.EnergyExplanationSummary.Carriers) == 0 {
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
			if item.Level != "driver" || canonicalEnergyPathBasis(item.Basis, "") != "heat_balance_share" {
				item.RawValue = firstNonZero(item.RawValue, item.Value)
				item.AllocatedValue = firstNonZero(item.AllocatedValue, item.Value)
			}
			item.AggregationBasis = firstNonEmpty(item.AggregationBasis, summary.Scope.AggregationBasis)
		}
	}
}

func normalizeEnergyExplanationV2Nodes(nodes []EnergyExplanationNode, scope EnergyExplanationScope) {
	for index := range nodes {
		node := &nodes[index]
		node.AggregationBasis = firstNonEmpty(node.AggregationBasis, scope.AggregationBasis)
		if node.Multiplier == 0 {
			node.Multiplier = 1
		}
		if node.AllocationApplied {
			// An applied allocation makes all three quantities independent.
			// Zero raw pressure is valid for Other/storage, and zero allocated
			// contribution must not resurrect an unallocated raw pressure.
			if node.Level == "driver" {
				node.Value = math.Abs(node.AllocatedValue)
				node.DisplayValue = node.Value
			}
		} else {
			node.RawValue = firstNonZero(node.RawValue, node.Value)
			node.EffectiveValue = firstNonZero(node.EffectiveValue, roundedEnergyNumber(node.RawValue*node.Multiplier), node.Value)
			node.AllocatedValue = firstNonZero(node.AllocatedValue, node.EffectiveValue, node.Value)
		}
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
