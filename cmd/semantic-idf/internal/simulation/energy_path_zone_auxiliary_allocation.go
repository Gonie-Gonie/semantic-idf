package simulation

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const energyPathAuxiliaryAllocationRelation = "auxiliary_allocation"

type energyPathAuxiliaryServicePath struct {
	ID                string
	ZoneName          string
	ServiceKind       string
	AirLoopName       string
	PlantLoopName     string
	CondenserLoopName string
}

type energyPathZoneAuxiliaryAllocationPlan struct {
	Edges                      []EnergyExplanationEdge
	Records                    []energyPathZoneAuxiliaryAllocationRecord
	CentralEndUseNodeIDs       map[string]bool
	SourceExpectedByNode       map[string]float64
	AnnualAuthoritativeGroups  map[string]bool
	AnnualDirectOverrideGroups map[string]bool
}

type energyPathZoneAuxiliaryAllocationRecord struct {
	Period          string
	EndUse          string
	Carrier         string
	Unit            string
	ExpectedValue   float64
	DirectValue     float64
	AllocatedValue  float64
	UnassignedValue float64
	OvermappedValue float64
	Method          string
	SourceIDs       []string
}

type energyPathZoneAuxiliaryTarget struct {
	NodeID      string
	ZoneName    string
	ServiceKind string
	Value       float64
	PathIDs     []string
	SourceIDs   []string
}

type energyPathZoneAuxiliaryDirectValue struct {
	ZoneName  string
	Present   bool
	Value     float64
	SourceIDs []string
}

func buildEnergyPathZoneAuxiliaryAllocationPlan(nodes []EnergyExplanationNode, directSeries []energyExplanationSeries, topology energyServicePathIndex, periodID, periodKind string, canonicalMonthlyBasis bool) energyPathZoneAuxiliaryAllocationPlan {
	plan := energyPathZoneAuxiliaryAllocationPlan{CentralEndUseNodeIDs: map[string]bool{}, SourceExpectedByNode: map[string]float64{}}
	periodID = firstNonEmpty(strings.TrimSpace(periodID), "annual")

	type endUseGroup struct {
		endUse   string
		carrier  string
		unit     string
		expected float64
		nodes    []EnergyExplanationNode
		sources  []string
	}
	groups := map[string]*endUseGroup{}
	groupKeys := []string{}
	foldedNodes := foldLegacyEnergyLoadDetailNodes(nodes)
	for _, original := range foldedNodes {
		node := energyExplanationLegacyNodeWithEffectiveValues(original)
		if strings.TrimSpace(node.ZoneName) != "" || legacyEnergyNodeIsCarrier(node) || legacyEnergyNodeIsSupport(node) {
			continue
		}
		level := strings.ToLower(strings.TrimSpace(node.Level))
		if level != "energy" && level != "end_use" {
			continue
		}
		endUse := canonicalEnergyPathEndUse(firstNonEmpty(node.EndUse, energyExplanationKindSuffix(node.Kind)))
		if !energyPathIsZoneAllocatableAuxiliary(endUse) {
			continue
		}
		carrier := canonicalEnergyPathPart(node.Carrier)
		if carrier == "" {
			continue
		}
		value := math.Abs(energyExplanationEffectiveNodeValue(node))
		if !energyPathFinite(value) {
			continue
		}
		key := energyPathZoneAuxiliaryGroupKey(endUse, carrier)
		group := groups[key]
		if group == nil {
			group = &endUseGroup{endUse: endUse, carrier: carrier, unit: node.Unit}
			groups[key] = group
			groupKeys = append(groupKeys, key)
		}
		group.expected = roundedEnergyNumber(group.expected + value)
		group.nodes = append(group.nodes, node)
		group.sources = appendUniqueStrings(group.sources, node.SourceIDs...)
		plan.CentralEndUseNodeIDs[node.ID] = true
		plan.SourceExpectedByNode[node.ID] = roundedEnergyNumber(plan.SourceExpectedByNode[node.ID] + value)
	}
	if len(groups) == 0 {
		return plan
	}

	directByTarget := energyPathZoneAuxiliaryDirectValues(nodes, directSeries, periodID, periodKind, canonicalMonthlyBasis)
	sort.Strings(groupKeys)
	for _, groupKey := range groupKeys {
		group := groups[groupKey]
		sort.SliceStable(group.nodes, func(i, j int) bool { return group.nodes[i].ID < group.nodes[j].ID })
		sort.Strings(group.sources)
		record := energyPathZoneAuxiliaryAllocationRecord{
			Period:        periodID,
			EndUse:        group.endUse,
			Carrier:       group.carrier,
			Unit:          group.unit,
			ExpectedValue: roundedEnergyNumber(group.expected),
			SourceIDs:     appendUniqueStrings(nil, group.sources...),
		}
		directZones := map[string]bool{}
		directPrefix := groupKey + "\x00"
		directKeys := []string{}
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
		if math.Abs(record.ExpectedValue) <= energyPathZoneHVACAllocationEpsilon && math.Abs(record.DirectValue) <= energyPathZoneHVACAllocationEpsilon {
			continue
		}

		remaining := roundedEnergyNumber(group.expected - record.DirectValue)
		if remaining < 0 {
			remaining = 0
		}
		nodeWeights := make([]float64, len(group.nodes))
		for index := range group.nodes {
			nodeWeights[index] = math.Abs(energyExplanationEffectiveNodeValue(group.nodes[index]))
		}
		nodePools := energyPathZoneHVACProportionalValues(remaining, nodeWeights)
		allocatedMethods := map[string]bool{}
		for nodeIndex, endUseNode := range group.nodes {
			if nodeIndex >= len(nodePools) || nodePools[nodeIndex] <= 0 {
				continue
			}
			targets, method := energyPathZoneAuxiliaryTargets(group.endUse, endUseNode.RelatedPathIDs, foldedNodes, topology, directZones)
			if group.endUse == "fans" && len(targets) > 0 {
				if airflowTargets, ok := energyPathZoneAuxiliaryAirflowTargets(targets, foldedNodes); ok {
					targets = airflowTargets
					method = "airflow_share"
				}
			}
			weights := make([]float64, len(targets))
			for index := range targets {
				weights[index] = targets[index].Value
			}
			nodeTargetValues := energyPathZoneHVACProportionalValues(nodePools[nodeIndex], weights)
			nodeAllocated := 0.0
			for targetIndex, target := range targets {
				if targetIndex >= len(nodeTargetValues) || nodeTargetValues[targetIndex] <= 0 {
					continue
				}
				value := nodeTargetValues[targetIndex]
				nodeAllocated = roundedEnergyNumber(nodeAllocated + value)
				record.AllocatedValue = roundedEnergyNumber(record.AllocatedValue + value)
				record.SourceIDs = appendUniqueStrings(record.SourceIDs, target.SourceIDs...)
				paths := appendUniqueStrings(nil, target.PathIDs...)
				sources := appendUniqueStrings(append([]string(nil), endUseNode.SourceIDs...), target.SourceIDs...)
				sort.Strings(paths)
				sort.Strings(sources)
				plan.Edges = append(plan.Edges, EnergyExplanationEdge{
					ID:             edgeID("allocation_zone_auxiliary", periodID, endUseNode.ID, target.NodeID),
					FromID:         endUseNode.ID,
					ToID:           target.NodeID,
					Value:          value,
					Unit:           firstNonEmpty(endUseNode.Unit, group.unit),
					Period:         periodID,
					Relation:       energyPathAuxiliaryAllocationRelation,
					Basis:          "service_path_allocation",
					Formula:        fmt.Sprintf("%s; allocation factor %.6f", energyPathZoneAuxiliaryAllocationFormula(method), value/math.Abs(energyExplanationEffectiveNodeValue(endUseNode))),
					RuleID:         energyRelationshipRuleAllocatedAuxiliaryServicePath,
					SourceIDs:      sources,
					ZoneName:       target.ZoneName,
					ServiceKind:    target.ServiceKind,
					RelatedPathIDs: paths,
				})
			}
			if nodeAllocated > energyPathZoneHVACAllocationEpsilon {
				allocatedMethods[method] = true
			}
		}
		residual := roundedEnergyNumber(record.ExpectedValue - record.DirectValue - record.AllocatedValue)
		if residual > energyPathZoneHVACAllocationEpsilon {
			record.UnassignedValue = residual
		} else if residual < -energyPathZoneHVACAllocationEpsilon {
			record.OvermappedValue = math.Abs(residual)
		}
		switch {
		case record.AllocatedValue > energyPathZoneHVACAllocationEpsilon:
			for method := range allocatedMethods {
				record.Method = method
			}
			if len(allocatedMethods) > 1 {
				record.Method = "mixed"
			}
		case record.DirectValue > energyPathZoneHVACAllocationEpsilon && record.UnassignedValue <= energyPathZoneHVACAllocationEpsilon:
			record.Method = "direct_only"
		default:
			record.Method = "unassigned"
		}
		record.SourceIDs = appendUniqueStrings(nil, record.SourceIDs...)
		sort.Strings(record.SourceIDs)
		plan.Records = append(plan.Records, record)
	}
	sortEnergyExplanationEdges(plan.Edges)
	return plan
}

func energyPathIsZoneAllocatableAuxiliary(endUse string) bool {
	switch canonicalEnergyPathEndUse(endUse) {
	case "fans", "pumps", "heat_rejection":
		return true
	default:
		return false
	}
}

func energyPathZoneAuxiliaryGroupKey(endUse, carrier string) string {
	return canonicalEnergyPathEndUse(endUse) + "\x00" + canonicalEnergyPathPart(carrier)
}

func energyPathZoneAuxiliaryTargets(endUse string, restrictedPathIDs []string, nodes []EnergyExplanationNode, topology energyServicePathIndex, directZones map[string]bool) ([]energyPathZoneAuxiliaryTarget, string) {
	endUse = canonicalEnergyPathEndUse(endUse)
	if !topology.auxiliaryResolvable[endUse] {
		return nil, "unassigned"
	}
	eligiblePaths := energyPathZoneAuxiliaryEligiblePaths(endUse, restrictedPathIDs, topology.auxiliaryPaths)
	method := "unassigned"
	switch canonicalEnergyPathEndUse(endUse) {
	case "fans":
		method = "air_loop_load_share"
	case "pumps":
		method = "plant_loop_load_share"
	case "heat_rejection":
		method = "condenser_loop_load_share"
	}
	if len(eligiblePaths) == 0 {
		return nil, "unassigned"
	}

	type aggregate struct {
		target energyPathZoneAuxiliaryTarget
	}
	byZone := map[string]*aggregate{}
	zoneKeys := []string{}
	for _, original := range nodes {
		node := energyExplanationLegacyNodeWithEffectiveValues(original)
		if !strings.EqualFold(strings.TrimSpace(node.Level), "load") || strings.TrimSpace(node.ZoneName) == "" {
			continue
		}
		zoneKey := energyPathZoneHVACZoneKey(node.ZoneName)
		if directZones[zoneKey] {
			continue
		}
		service := energyCanonicalServiceKind(firstNonEmpty(node.ServiceKind, energyExplanationKindSuffix(node.Kind)))
		if service != "cooling" && service != "heating" {
			continue
		}
		value := math.Abs(energyExplanationEffectiveNodeValue(node))
		if value <= 0 || !energyPathFinite(value) {
			continue
		}
		paths := energyPathZoneAuxiliaryTargetPaths(endUse, node, eligiblePaths)
		if len(paths) == 0 {
			continue
		}
		current := byZone[zoneKey]
		if current == nil {
			current = &aggregate{target: energyPathZoneAuxiliaryTarget{
				NodeID:      node.ID,
				ZoneName:    node.ZoneName,
				ServiceKind: service,
			}}
			byZone[zoneKey] = current
			zoneKeys = append(zoneKeys, zoneKey)
		}
		current.target.Value = roundedEnergyNumber(current.target.Value + value)
		current.target.PathIDs = appendUniqueStrings(current.target.PathIDs, paths...)
		current.target.SourceIDs = appendUniqueStrings(current.target.SourceIDs, node.SourceIDs...)
		if current.target.ServiceKind != service {
			current.target.ServiceKind = "hvac"
		}
	}
	sort.Strings(zoneKeys)
	out := make([]energyPathZoneAuxiliaryTarget, 0, len(zoneKeys))
	for _, key := range zoneKeys {
		target := byZone[key].target
		sort.Strings(target.PathIDs)
		sort.Strings(target.SourceIDs)
		out = append(out, target)
	}
	if len(out) == 0 {
		method = "unassigned"
	}
	return out, method
}

func energyPathZoneAuxiliaryEligiblePaths(endUse string, restrictedPathIDs []string, paths []energyPathAuxiliaryServicePath) map[string]energyPathAuxiliaryServicePath {
	plantServices := map[string]map[string]bool{}
	condenserServices := map[string]map[string]bool{}
	for _, path := range paths {
		service := energyPathAuxiliaryCanonicalServiceKind(path.ServiceKind)
		if key := normalizePurposeToken(path.PlantLoopName); key != "" {
			if plantServices[key] == nil {
				plantServices[key] = map[string]bool{}
			}
			serviceToken := service
			if serviceToken != "cooling" && serviceToken != "heating" {
				serviceToken = "unsupported"
			}
			plantServices[key][serviceToken] = true
		}
		if key := normalizePurposeToken(path.CondenserLoopName); key != "" {
			if condenserServices[key] == nil {
				condenserServices[key] = map[string]bool{}
			}
			serviceToken := service
			if serviceToken != "cooling" && serviceToken != "heating" {
				serviceToken = "unsupported"
			}
			condenserServices[key][serviceToken] = true
		}
	}
	restricted := map[string]bool{}
	for _, pathID := range restrictedPathIDs {
		restricted[strings.ToLower(strings.TrimSpace(pathID))] = true
	}
	out := map[string]energyPathAuxiliaryServicePath{}
	for _, path := range paths {
		path.ID = strings.TrimSpace(path.ID)
		path.ZoneName = strings.TrimSpace(path.ZoneName)
		path.ServiceKind = energyPathAuxiliaryCanonicalServiceKind(path.ServiceKind)
		pathKey := strings.ToLower(path.ID)
		validService := path.ServiceKind == "cooling" || path.ServiceKind == "heating" || canonicalEnergyPathEndUse(endUse) == "fans" && path.ServiceKind == "ventilation"
		if path.ID == "" || path.ZoneName == "" || !validService || len(restricted) > 0 && !restricted[pathKey] {
			continue
		}
		eligible := false
		switch canonicalEnergyPathEndUse(endUse) {
		case "fans":
			eligible = strings.TrimSpace(path.AirLoopName) != ""
		case "pumps":
			plantKey := normalizePurposeToken(path.PlantLoopName)
			eligible = plantKey != "" && len(plantServices[plantKey]) == 1
		case "heat_rejection":
			plantKey := normalizePurposeToken(path.PlantLoopName)
			condenserKey := normalizePurposeToken(path.CondenserLoopName)
			eligible = path.ServiceKind == "cooling" && plantKey != "" && condenserKey != "" &&
				len(plantServices[plantKey]) == 1 && plantServices[plantKey]["cooling"] &&
				len(condenserServices[condenserKey]) == 1 && condenserServices[condenserKey]["cooling"]
		}
		if eligible {
			out[pathKey] = path
		}
	}
	return out
}

func energyPathZoneAuxiliaryTargetPaths(endUse string, node EnergyExplanationNode, eligible map[string]energyPathAuxiliaryServicePath) []string {
	service := energyCanonicalServiceKind(firstNonEmpty(node.ServiceKind, energyExplanationKindSuffix(node.Kind)))
	zoneName := strings.TrimSpace(node.ZoneName)
	out := []string{}
	if len(node.RelatedPathIDs) > 0 {
		for _, pathID := range node.RelatedPathIDs {
			path, ok := eligible[strings.ToLower(strings.TrimSpace(pathID))]
			serviceMatches := energyPathAuxiliaryCanonicalServiceKind(path.ServiceKind) == service
			if canonicalEnergyPathEndUse(endUse) == "fans" {
				serviceMatches = true
			}
			if ok && strings.EqualFold(path.ZoneName, zoneName) && serviceMatches {
				out = appendUniqueStrings(out, path.ID)
			}
		}
	} else {
		for _, path := range eligible {
			if strings.EqualFold(path.ZoneName, zoneName) && energyPathAuxiliaryCanonicalServiceKind(path.ServiceKind) == service {
				out = appendUniqueStrings(out, path.ID)
			}
		}
	}
	sort.Strings(out)
	return out
}

func energyPathZoneAuxiliaryAirflowTargets(targets []energyPathZoneAuxiliaryTarget, nodes []EnergyExplanationNode) ([]energyPathZoneAuxiliaryTarget, bool) {
	if len(targets) == 0 {
		return nil, false
	}
	out := append([]energyPathZoneAuxiliaryTarget(nil), targets...)
	for targetIndex := range out {
		value := 0.0
		sources := []string{}
		for _, original := range nodes {
			node := energyExplanationLegacyNodeWithEffectiveValues(original)
			if !energyPathZoneAuxiliaryIsSupplyAirflow(node) || !strings.EqualFold(strings.TrimSpace(node.ZoneName), strings.TrimSpace(out[targetIndex].ZoneName)) {
				continue
			}
			paths := energyPathZoneHVACIntersectPaths(node.RelatedPathIDs, out[targetIndex].PathIDs)
			if len(paths) == 0 {
				continue
			}
			current := math.Abs(energyExplanationEffectiveNodeValue(node))
			if current <= 0 || !energyPathFinite(current) {
				continue
			}
			value = roundedEnergyNumber(value + current)
			sources = appendUniqueStrings(sources, node.SourceIDs...)
		}
		if value <= 0 {
			return nil, false
		}
		out[targetIndex].Value = value
		out[targetIndex].SourceIDs = appendUniqueStrings(out[targetIndex].SourceIDs, sources...)
		sort.Strings(out[targetIndex].SourceIDs)
	}
	return out, true
}

func energyPathZoneAuxiliaryIsSupplyAirflow(node EnergyExplanationNode) bool {
	level := normalizeEnergyOutputName(node.Level)
	kind := normalizeEnergyOutputName(strings.NewReplacer("_", " ", ".", " ").Replace(node.Kind))
	if level != "airflow" && !strings.Contains(kind, "airflow") && !strings.Contains(kind, "air flow") {
		return false
	}
	return strings.Contains(kind, "supply") && (strings.Contains(kind, "volume") || strings.Contains(kind, "airflow") || strings.Contains(kind, "air flow"))
}

func energyPathZoneAuxiliaryAllocationFormula(method string) string {
	switch method {
	case "airflow_share":
		return "Allocated by related AirLoop supply-air volume share; building auxiliary end use * zone supply-air volume / eligible related AirLoop supply-air volume"
	case "air_loop_load_share":
		return "Allocated by related AirLoop service-path load share; building fan energy * eligible zone service load / eligible related AirLoop service load"
	case "plant_loop_load_share":
		return "Allocated by unambiguous related PlantLoop service-path load share; building pump energy * eligible zone service load / eligible related PlantLoop service load"
	case "condenser_loop_load_share":
		return "Allocated by related CondenserLoop to cooling PlantLoop service-path load share; building heat-rejection energy * eligible zone cooling load / eligible related condenser service load"
	default:
		return "Building HVAC auxiliary energy remains unassigned because no eligible related service path evidence is available"
	}
}

func energyPathZoneAuxiliaryDirectValues(nodes []EnergyExplanationNode, series []energyExplanationSeries, periodID, periodKind string, canonicalMonthlyBasis bool) map[string]energyPathZoneAuxiliaryDirectValue {
	out := map[string]energyPathZoneAuxiliaryDirectValue{}
	add := func(zoneName, endUse, carrier string, value float64, present bool, sourceIDs []string) {
		zoneName = strings.TrimSpace(zoneName)
		endUse = canonicalEnergyPathEndUse(endUse)
		carrier = canonicalEnergyPathPart(carrier)
		if zoneName == "" || !energyPathIsZoneAllocatableAuxiliary(endUse) || carrier == "" || !present || !energyPathFinite(value) {
			return
		}
		key := energyPathZoneAuxiliaryGroupKey(endUse, carrier) + "\x00" + energyPathZoneHVACZoneKey(zoneName)
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
		endUse := canonicalEnergyPathEndUse(firstNonEmpty(item.EndUse, energyExplanationKindSuffix(item.Kind)))
		value, _, sourceIDs, ok := energyPathDirectZonePeriodValue(item, periodID, periodKind, canonicalMonthlyBasis)
		add(item.ZoneName, endUse, firstNonEmpty(item.Carrier, "other"), value, ok, sourceIDs)
	}
	for _, original := range nodes {
		node := energyExplanationLegacyNodeWithEffectiveValues(original)
		if !energyPathLegacyNodeIsDirectZoneEnergy(node) || legacyEnergyNodeIsCarrier(node) || legacyEnergyNodeIsSupport(node) {
			continue
		}
		endUse := canonicalEnergyPathEndUse(firstNonEmpty(node.EndUse, energyExplanationKindSuffix(node.Kind)))
		add(node.ZoneName, endUse, firstNonEmpty(node.Carrier, "other"), energyExplanationEffectiveNodeValue(node), true, node.SourceIDs)
	}
	return out
}

func applyEnergyPathZoneAuxiliaryAllocationPlan(edges []EnergyExplanationEdge, plan energyPathZoneAuxiliaryAllocationPlan) []EnergyExplanationEdge {
	out := make([]EnergyExplanationEdge, 0, len(edges)+len(plan.Edges))
	for _, edge := range edges {
		if strings.EqualFold(strings.TrimSpace(edge.Relation), energyPathAuxiliaryAllocationRelation) && plan.CentralEndUseNodeIDs[edge.FromID] {
			continue
		}
		out = append(out, edge)
	}
	out = append(out, plan.Edges...)
	sortEnergyExplanationEdges(out)
	return out
}

func aggregateEnergyPathZoneAuxiliaryAllocationPlans(input []energyPathZoneAuxiliaryAllocationPlan) energyPathZoneAuxiliaryAllocationPlan {
	out := energyPathZoneAuxiliaryAllocationPlan{CentralEndUseNodeIDs: map[string]bool{}, SourceExpectedByNode: map[string]float64{}}
	edgesByKey := map[string]*EnergyExplanationEdge{}
	edgeFormulaByKey := map[string]string{}
	edgeKeys := []string{}
	records := []energyPathZoneAuxiliaryAllocationRecord{}
	for _, plan := range input {
		for id := range plan.CentralEndUseNodeIDs {
			out.CentralEndUseNodeIDs[id] = true
		}
		for id, value := range plan.SourceExpectedByNode {
			out.SourceExpectedByNode[id] = roundedEnergyNumber(out.SourceExpectedByNode[id] + value)
		}
		for _, edge := range plan.Edges {
			key := energyExplanationEdgeAggregationKey(edge)
			formula := energyPathZoneAuxiliaryFormulaWithoutFactor(edge.Formula)
			current := edgesByKey[key]
			if current == nil {
				copy := edge
				copy.Period = "annual"
				copy.ID = energyExplanationAnnualEdgeID(copy)
				copy.Formula = ""
				copy.SourceIDs = appendUniqueStrings(nil, edge.SourceIDs...)
				copy.RelatedPathIDs = appendUniqueStrings(nil, edge.RelatedPathIDs...)
				edgesByKey[key] = &copy
				edgeFormulaByKey[key] = formula
				edgeKeys = append(edgeKeys, key)
				continue
			}
			if !strings.EqualFold(edgeFormulaByKey[key], formula) {
				edgeFormulaByKey[key] = ""
			}
			current.Value = roundedEnergyNumber(current.Value + edge.Value)
			current.SourceIDs = appendUniqueStrings(current.SourceIDs, edge.SourceIDs...)
			current.RelatedPathIDs = appendUniqueStrings(current.RelatedPathIDs, edge.RelatedPathIDs...)
		}
		records = append(records, plan.Records...)
	}
	sort.Strings(edgeKeys)
	for _, key := range edgeKeys {
		edge := *edgesByKey[key]
		formula := edgeFormulaByKey[key]
		if formula == "" {
			formula = "Allocated by mixed related HVAC auxiliary service-path evidence"
		}
		if denominator := out.SourceExpectedByNode[edge.FromID]; denominator > energyPathZoneHVACAllocationEpsilon {
			formula = fmt.Sprintf("%s; allocation factor %.6f", formula, edge.Value/denominator)
		}
		edge.Formula = strings.TrimSpace(formula + "; annual sum of monthly auxiliary allocations; inspect monthly periods for period-specific factors")
		sort.Strings(edge.SourceIDs)
		sort.Strings(edge.RelatedPathIDs)
		out.Edges = append(out.Edges, edge)
	}
	out.Records = aggregateEnergyPathZoneAuxiliaryAllocationRecords(records, "annual")
	return out
}

func energyPathZoneAuxiliaryFormulaWithoutFactor(formula string) string {
	formula = strings.TrimSpace(formula)
	lower := strings.ToLower(formula)
	if index := strings.Index(lower, "; allocation factor "); index >= 0 {
		formula = strings.TrimSpace(formula[:index])
	}
	return formula
}

func aggregateEnergyPathZoneAuxiliaryAllocationRecords(input []energyPathZoneAuxiliaryAllocationRecord, period string) []energyPathZoneAuxiliaryAllocationRecord {
	byKey := map[string]*energyPathZoneAuxiliaryAllocationRecord{}
	keys := []string{}
	for _, record := range input {
		key := energyPathZoneAuxiliaryGroupKey(record.EndUse, record.Carrier)
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
		if current.Method != record.Method {
			current.Method = "mixed"
		}
		current.SourceIDs = appendUniqueStrings(current.SourceIDs, record.SourceIDs...)
	}
	sort.Strings(keys)
	out := make([]energyPathZoneAuxiliaryAllocationRecord, 0, len(keys))
	for _, key := range keys {
		record := *byKey[key]
		sort.Strings(record.SourceIDs)
		out = append(out, record)
	}
	return out
}

func energyPathZoneAuxiliaryAllocationPlanWithAnnualFallback(monthly, annual energyPathZoneAuxiliaryAllocationPlan, nodes []EnergyExplanationNode) energyPathZoneAuxiliaryAllocationPlan {
	out := energyPathZoneAuxiliaryAllocationPlan{
		CentralEndUseNodeIDs:       map[string]bool{},
		SourceExpectedByNode:       map[string]float64{},
		AnnualAuthoritativeGroups:  map[string]bool{},
		AnnualDirectOverrideGroups: map[string]bool{},
	}
	for id := range monthly.CentralEndUseNodeIDs {
		out.CentralEndUseNodeIDs[id] = true
	}
	for id := range annual.CentralEndUseNodeIDs {
		out.CentralEndUseNodeIDs[id] = true
	}
	for id, value := range monthly.SourceExpectedByNode {
		out.SourceExpectedByNode[id] = value
	}
	for id, value := range annual.SourceExpectedByNode {
		if _, exists := out.SourceExpectedByNode[id]; !exists {
			out.SourceExpectedByNode[id] = value
		}
	}
	monthlyGroups := map[string]energyPathZoneAuxiliaryAllocationRecord{}
	for _, record := range monthly.Records {
		monthlyGroups[energyPathZoneAuxiliaryGroupKey(record.EndUse, record.Carrier)] = record
	}
	for _, record := range annual.Records {
		key := energyPathZoneAuxiliaryGroupKey(record.EndUse, record.Carrier)
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
		if annual.CentralEndUseNodeIDs[node.ID] {
			nodeGroup[node.ID] = energyPathZoneAuxiliaryGroupKey(firstNonEmpty(node.EndUse, energyExplanationKindSuffix(node.Kind)), node.Carrier)
		}
	}
	for _, record := range monthly.Records {
		if !out.AnnualAuthoritativeGroups[energyPathZoneAuxiliaryGroupKey(record.EndUse, record.Carrier)] {
			out.Records = append(out.Records, record)
		}
	}
	for _, record := range annual.Records {
		if out.AnnualAuthoritativeGroups[energyPathZoneAuxiliaryGroupKey(record.EndUse, record.Carrier)] {
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
		return energyPathZoneAuxiliaryGroupKey(out.Records[i].EndUse, out.Records[i].Carrier) < energyPathZoneAuxiliaryGroupKey(out.Records[j].EndUse, out.Records[j].Carrier)
	})
	sortEnergyExplanationEdges(out.Edges)
	return out
}

func appendEnergyPathZoneAuxiliaryAllocationAccounting(result *EnergyExplanationResult, annual energyPathZoneAuxiliaryAllocationPlan, periodPlans map[string]energyPathZoneAuxiliaryAllocationPlan, canonicalMonthlyBasis bool) {
	if result == nil {
		return
	}
	includeWarnings := result.Scope.Kind == "building"
	result.Reconciliation, result.Warnings = appendEnergyPathZoneAuxiliaryAllocationRecords(result.Reconciliation, result.Warnings, annual.Records, "annual", includeWarnings)
	for index := range result.Periods {
		period := &result.Periods[index]
		records := periodPlans[strings.ToLower(strings.TrimSpace(period.ID))].Records
		if canonicalMonthlyBasis && (strings.EqualFold(period.Kind, "annual") || strings.EqualFold(period.ID, "annual")) {
			records = annual.Records
		}
		period.Reconciliation, period.Warnings = appendEnergyPathZoneAuxiliaryAllocationRecords(period.Reconciliation, period.Warnings, records, period.ID, includeWarnings)
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

// applyEnergyPathAnnualZoneAuxiliaryOverrides keeps an exact annual-only
// auxiliary observation out of the monthly profile while preserving it in the
// annual Zone branch. Only the affected auxiliary end-use/carrier branch is
// replaced; sibling carriers and unrelated end uses keep their monthly sums.
func applyEnergyPathAnnualZoneAuxiliaryOverrides(result *EnergyExplanationResult, initialNodes []EnergyExplanationNode, initialLinks []EnergyPathLink, plan energyPathZoneAuxiliaryAllocationPlan) {
	if result == nil || result.Scope.Kind != "zone" || len(plan.AnnualAuthoritativeGroups) == 0 {
		return
	}
	affectedGroups := plan.AnnualAuthoritativeGroups
	initialByID := make(map[string]EnergyExplanationNode, len(initialNodes))
	resultByID := make(map[string]EnergyExplanationNode, len(result.Nodes))
	for _, node := range initialNodes {
		initialByID[node.ID] = node
	}
	for _, node := range result.Nodes {
		resultByID[node.ID] = node
	}
	linkGroup := func(link EnergyPathLink, nodeByID map[string]EnergyExplanationNode) (string, bool) {
		if link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier" {
			return "", false
		}
		endUseNode, endUseOK := nodeByID[link.FromID]
		carrierNode, carrierOK := nodeByID[link.ToID]
		if !endUseOK || !carrierOK || endUseNode.Level != "end_use" || carrierNode.Level != "carrier" {
			return "", false
		}
		endUse := canonicalEnergyPathEndUse(firstNonEmpty(endUseNode.EndUse, energyExplanationKindSuffix(endUseNode.Kind)))
		carrier := canonicalEnergyPathPart(firstNonEmpty(carrierNode.Carrier, energyExplanationKindSuffix(carrierNode.Kind)))
		if !energyPathIsZoneAllocatableAuxiliary(endUse) || carrier == "" {
			return "", false
		}
		return energyPathZoneAuxiliaryGroupKey(endUse, carrier), true
	}

	affectedEndUseIDs := map[string]bool{}
	directOverrideEndUseIDs := map[string]bool{}
	affectedCarrierIDs := map[string]bool{}
	links := make([]EnergyPathLink, 0, len(result.Links)+len(initialLinks))
	for _, link := range result.Links {
		group, ok := linkGroup(link, resultByID)
		if ok && affectedGroups[group] {
			affectedEndUseIDs[link.FromID] = true
			affectedCarrierIDs[link.ToID] = true
			continue
		}
		links = append(links, link)
	}
	for _, link := range initialLinks {
		group, ok := linkGroup(link, initialByID)
		if !ok || !affectedGroups[group] {
			continue
		}
		links = append(links, link)
		affectedEndUseIDs[link.FromID] = true
		affectedCarrierIDs[link.ToID] = true
		if plan.AnnualDirectOverrideGroups[group] {
			directOverrideEndUseIDs[link.FromID] = true
		}
	}
	if len(affectedEndUseIDs) == 0 {
		return
	}

	nodes := cloneEnergyExplanationNodes(result.Nodes)
	nodeIndex := map[string]int{}
	for index := range nodes {
		nodeIndex[nodes[index].ID] = index
	}
	for nodeID := range affectedEndUseIDs {
		if _, exists := nodeIndex[nodeID]; exists {
			continue
		}
		if node, ok := initialByID[nodeID]; ok {
			nodes = append(nodes, node)
			nodeIndex[nodeID] = len(nodes) - 1
		}
	}
	for carrierID := range affectedCarrierIDs {
		if _, exists := nodeIndex[carrierID]; exists {
			continue
		}
		if carrier, ok := initialByID[carrierID]; ok && carrier.Level == "carrier" {
			nodes = append(nodes, carrier)
			nodeIndex[carrierID] = len(nodes) - 1
		}
	}

	for endUseID := range affectedEndUseIDs {
		index, exists := nodeIndex[endUseID]
		if !exists {
			continue
		}
		if template, ok := initialByID[endUseID]; ok {
			nodes[index] = template
		}
		total := 0.0
		sourceIDs := []string{}
		pathIDs := []string{}
		carriers := []string{}
		basis := ""
		explanation := ""
		for _, link := range links {
			if link.FromID != endUseID || link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier" {
				continue
			}
			total = roundedEnergyNumber(total + math.Abs(link.FromValue))
			sourceIDs = appendUniqueStrings(sourceIDs, link.SourceIDs...)
			pathIDs = appendUniqueStrings(pathIDs, link.RelatedPathIDs...)
			if carrier, ok := initialByID[link.ToID]; ok {
				carriers = appendUniqueStrings(carriers, canonicalEnergyPathPart(firstNonEmpty(carrier.Carrier, energyExplanationKindSuffix(carrier.Kind))))
			} else if carrier, ok := resultByID[link.ToID]; ok {
				carriers = appendUniqueStrings(carriers, canonicalEnergyPathPart(firstNonEmpty(carrier.Carrier, energyExplanationKindSuffix(carrier.Kind))))
			}
			if energyPathAllocationBasisRank(link.Basis) > energyPathAllocationBasisRank(basis) {
				basis = canonicalEnergyPathBasis(link.Basis, "")
				explanation = link.Explanation
			}
		}
		node := &nodes[index]
		node.Value = total
		node.AllocatedValue = total
		node.DisplayValue = math.Abs(total)
		node.Period = "annual"
		node.AllocationApplied = true
		node.SourceIDs = sourceIDs
		node.RelatedPathIDs = pathIDs
		node.endUseCarriers = carriers
		node.allocationSourceIDs = appendUniqueStrings(nil, sourceIDs...)
		if basis != "" {
			node.Basis = basis
		}
		if explanation != "" {
			node.AllocationExplanation = explanation
		}
		if directOverrideEndUseIDs[endUseID] {
			const note = "annual-only exact direct zone auxiliary energy; monthly allocation retained without fabricating a direct profile; partial temporal coverage"
			if !strings.Contains(strings.ToLower(node.AllocationExplanation), "partial temporal coverage") {
				node.AllocationExplanation = strings.TrimSpace(strings.TrimSpace(node.AllocationExplanation) + "; " + note)
				node.AllocationExplanation = strings.TrimPrefix(node.AllocationExplanation, "; ")
			}
		}
		sort.Strings(node.SourceIDs)
		sort.Strings(node.RelatedPathIDs)
		sort.Strings(node.endUseCarriers)
	}

	for carrierID := range affectedCarrierIDs {
		index, exists := nodeIndex[carrierID]
		if !exists {
			continue
		}
		total := 0.0
		sourceIDs := []string{}
		pathIDs := []string{}
		basis := ""
		explanation := ""
		for _, link := range links {
			if link.ToID != carrierID || link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier" {
				continue
			}
			total = roundedEnergyNumber(total + math.Abs(link.ToValue))
			sourceIDs = appendUniqueStrings(sourceIDs, link.SourceIDs...)
			pathIDs = appendUniqueStrings(pathIDs, link.RelatedPathIDs...)
			if energyPathAllocationBasisRank(link.Basis) > energyPathAllocationBasisRank(basis) {
				basis = canonicalEnergyPathBasis(link.Basis, "")
				explanation = link.Explanation
			}
		}
		carrier := &nodes[index]
		carrier.Value = total
		carrier.RawValue = total
		carrier.EffectiveValue = total
		carrier.AllocatedValue = total
		carrier.DisplayValue = total
		carrier.Multiplier = 1
		carrier.SourceIDs = sourceIDs
		carrier.RelatedPathIDs = pathIDs
		carrier.allocationSourceIDs = appendUniqueStrings(nil, sourceIDs...)
		if basis != "" {
			carrier.Basis = basis
		}
		if explanation != "" {
			carrier.AllocationExplanation = explanation
		}
		sort.Strings(carrier.SourceIDs)
		sort.Strings(carrier.RelatedPathIDs)
	}

	sortEnergyExplanationNodes(nodes)
	sort.SliceStable(links, func(i, j int) bool { return links[i].ID < links[j].ID })
	result.Nodes = nodes
	result.Links = links
	result.Reconciliation = reconcileEnergyPathCarrierTotals(result.Nodes, result.Links, result.Reconciliation, "annual")
	result.Nodes, result.Links = rebuildEnergyPathCarrierResidualPresentation(result.Nodes, result.Links, result.Reconciliation, "annual")
	nodes, links = result.Nodes, result.Links
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

func appendEnergyPathZoneAuxiliaryAllocationRecords(reconciliation []EnergyReconciliation, warnings []EnergyWarning, records []energyPathZoneAuxiliaryAllocationRecord, period string, includeWarnings bool) ([]EnergyReconciliation, []EnergyWarning) {
	filteredRows := make([]EnergyReconciliation, 0, len(reconciliation)+len(records))
	for _, row := range reconciliation {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(row.ID)), "reconcile.zone_auxiliary_allocation.") {
			filteredRows = append(filteredRows, row)
		}
	}
	filteredWarnings := make([]EnergyWarning, 0, len(warnings)+len(records))
	for _, warning := range warnings {
		if warning.Code != "unassigned_building_hvac_auxiliary_energy" && warning.Code != "direct_zone_hvac_auxiliary_energy_exceeds_building" {
			filteredWarnings = append(filteredWarnings, warning)
		}
	}
	sorted := append([]energyPathZoneAuxiliaryAllocationRecord(nil), records...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return energyPathZoneAuxiliaryGroupKey(sorted[i].EndUse, sorted[i].Carrier) < energyPathZoneAuxiliaryGroupKey(sorted[j].EndUse, sorted[j].Carrier)
	})
	for _, record := range sorted {
		record.Period = firstNonEmpty(strings.TrimSpace(period), record.Period, "annual")
		explained := roundedEnergyNumber(record.DirectValue + record.AllocatedValue)
		residual := roundedEnergyNumber(record.ExpectedValue - explained)
		label := "Building HVAC auxiliary allocation coverage"
		status := "balanced"
		if residual > energyPathZoneHVACAllocationEpsilon {
			label = "Unassigned building HVAC auxiliary energy"
			status = "partial"
		} else if residual < -energyPathZoneHVACAllocationEpsilon {
			label = "Direct zone HVAC auxiliary energy exceeds building energy"
			status = "overmapped"
		} else if record.UnassignedValue > energyPathZoneHVACAllocationEpsilon || record.OvermappedValue > energyPathZoneHVACAllocationEpsilon {
			label = "Building HVAC auxiliary allocation has period-level gaps or overlaps"
			status = "partial"
		}
		if includeWarnings && record.UnassignedValue > energyPathZoneHVACAllocationEpsilon {
			filteredWarnings = appendEnergyDriverWarning(filteredWarnings, EnergyWarning{
				Severity: "warning",
				Code:     "unassigned_building_hvac_auxiliary_energy",
				Message:  fmt.Sprintf("Unassigned building HVAC auxiliary energy remains for %s %s: %g %s could not be linked to an eligible unambiguous service path.", canonicalEnergyPathEndUseLabel(record.EndUse), energyCarrierLabel(record.Carrier), record.UnassignedValue, record.Unit),
				Period:   record.Period,
			})
		}
		if includeWarnings && record.OvermappedValue > energyPathZoneHVACAllocationEpsilon {
			filteredWarnings = appendEnergyDriverWarning(filteredWarnings, EnergyWarning{
				Severity: "warning",
				Code:     "direct_zone_hvac_auxiliary_energy_exceeds_building",
				Message:  fmt.Sprintf("Exact direct zone %s %s energy exceeds the Building end-use meter by %g %s; direct observations are retained and no remainder is allocated.", canonicalEnergyPathEndUseLabel(record.EndUse), energyCarrierLabel(record.Carrier), record.OvermappedValue, record.Unit),
				Period:   record.Period,
			})
		}
		filteredRows = append(filteredRows, EnergyReconciliation{
			ID:               strings.Join([]string{"reconcile", "zone_auxiliary_allocation", canonicalEnergyPathPart(record.EndUse), canonicalEnergyPathPart(record.Carrier), canonicalEnergyPathPart(record.Period)}, "."),
			Level:            "allocation",
			Period:           record.Period,
			Label:            label,
			Status:           status,
			ServiceKind:      record.EndUse,
			ExpectedValue:    roundedEnergyNumber(record.ExpectedValue),
			ExplainedValue:   explained,
			ResidualValue:    residual,
			DirectValue:      roundedEnergyNumber(record.DirectValue),
			AllocatedValue:   roundedEnergyNumber(record.AllocatedValue),
			UnassignedValue:  roundedEnergyNumber(record.UnassignedValue),
			OvermappedValue:  roundedEnergyNumber(record.OvermappedValue),
			AllocationMethod: record.Method,
			Unit:             record.Unit,
			Basis:            "service_path_allocation",
			Formula:          "building HVAC auxiliary end use = exact direct zone auxiliary energy + allocated related service-path auxiliary energy + unassigned building auxiliary energy",
			SourceIDs:        appendUniqueStrings(nil, record.SourceIDs...),
		})
	}
	return filteredRows, filteredWarnings
}
