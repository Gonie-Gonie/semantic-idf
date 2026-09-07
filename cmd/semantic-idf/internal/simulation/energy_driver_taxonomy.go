package simulation

import (
	"math"
	"strings"
)

const (
	energyDriverCategoryExteriorWalls         = "surface.exterior_walls"
	energyDriverCategoryRoofs                 = "surface.roofs"
	energyDriverCategoryGroundFloors          = "surface.ground_floors"
	energyDriverCategoryWindowsDoors          = "surface.windows_doors"
	energyDriverCategoryInterzoneSurfaces     = "surface.interzone"
	energyDriverCategoryInfiltration          = "air.infiltration"
	energyDriverCategoryMechanicalVentilation = "air.mechanical_ventilation"
	energyDriverCategoryInterzoneAir          = "air.interzone"
	energyDriverCategoryPeople                = "internal.people"
	energyDriverCategoryLighting              = "internal.lighting"
	energyDriverCategoryEquipment             = "internal.equipment"
	energyDriverCategoryInternalOther         = "internal.other"
	energyDriverCategoryStorageOther          = "balance.storage_other"

	// This is a presentation-only category. Source rows keep one of the two
	// canonical interzone categories above so inspector/export breakdowns do
	// not lose whether the transfer was through a surface or through air.
	energyDriverDisplayCategoryInterzoneTransfer = "interzone.transfer"

	energyDriverSourceRoleMainFlow       = "main_flow"
	energyDriverSourceRoleContext        = "context"
	energyDriverSourceRoleReconciliation = "reconciliation"

	energyDriverInspectorSectionBreakdown = "Breakdown"
	energyDriverInspectorSectionContext   = "Context"
	energyDriverInspectorSectionBalance   = "Balance"
	energyDriverSurfaceExplanation        = "This is surface-to-zone-air heat exchange, not isolated wall conduction."

	energyDriverInterzoneThreshold  = 0.05
	energyDriverBalanceWarningRatio = 0.05
)

type energyDriverCategoryDefinition struct {
	Category string
	Label    string
	Order    int
}

var energyDriverCategoryDefinitions = []energyDriverCategoryDefinition{
	{Category: energyDriverCategoryExteriorWalls, Label: "Wall heat exchange", Order: 0},
	{Category: energyDriverCategoryRoofs, Label: "Roof heat exchange", Order: 1},
	{Category: energyDriverCategoryGroundFloors, Label: "Ground / floor heat exchange", Order: 2},
	{Category: energyDriverCategoryWindowsDoors, Label: "Window / door heat exchange", Order: 3},
	{Category: energyDriverCategoryInterzoneSurfaces, Label: "Interzone surfaces", Order: 4},
	{Category: energyDriverCategoryInfiltration, Label: "Infiltration", Order: 5},
	{Category: energyDriverCategoryMechanicalVentilation, Label: "Mechanical ventilation", Order: 6},
	{Category: energyDriverCategoryInterzoneAir, Label: "Interzone air", Order: 7},
	{Category: energyDriverCategoryPeople, Label: "People", Order: 8},
	{Category: energyDriverCategoryLighting, Label: "Lighting", Order: 9},
	{Category: energyDriverCategoryEquipment, Label: "Equipment", Order: 10},
	{Category: energyDriverDisplayCategoryInterzoneTransfer, Label: "Interzone transfer", Order: 11},
	{Category: energyDriverCategoryInternalOther, Label: "Other / storage", Order: 12},
	{Category: energyDriverCategoryStorageOther, Label: "Other / storage", Order: 12},
}

type energyDriverSourcePolicy struct {
	Role        string
	Category    string
	Label       string
	Explanation string
}

type energyDriverPresentationPlan struct {
	categoryByService map[string]map[string]string
}

func canonicalEnergyDriverCategory(value string) string {
	value = strings.TrimSpace(value)
	for _, definition := range energyDriverCategoryDefinitions {
		if strings.EqualFold(value, definition.Category) {
			return definition.Category
		}
	}

	key := normalizeEnergyOutputName(strings.TrimPrefix(strings.TrimPrefix(value, "heat."), "driver."))
	switch {
	case strings.Contains(key, "exterior wall") || key == "wall" || key == "walls":
		return energyDriverCategoryExteriorWalls
	case strings.Contains(key, "roof") || strings.Contains(key, "ceiling"):
		return energyDriverCategoryRoofs
	case strings.Contains(key, "ground") || strings.Contains(key, "floor"):
		return energyDriverCategoryGroundFloors
	case strings.Contains(key, "window") || strings.Contains(key, "door") || strings.Contains(key, "fenestration"):
		return energyDriverCategoryWindowsDoors
	case strings.Contains(key, "interzone") && strings.Contains(key, "surface"):
		return energyDriverCategoryInterzoneSurfaces
	case strings.Contains(key, "infiltration"):
		return energyDriverCategoryInfiltration
	case strings.Contains(key, "ventilation") || strings.Contains(key, "outdoor air"):
		return energyDriverCategoryMechanicalVentilation
	case strings.Contains(key, "interzone") || strings.Contains(key, "mixing") || strings.Contains(key, "cross mixing"):
		return energyDriverCategoryInterzoneAir
	case strings.Contains(key, "people") || strings.Contains(key, "occupant"):
		return energyDriverCategoryPeople
	case strings.Contains(key, "lighting") || strings.Contains(key, "lights"):
		return energyDriverCategoryLighting
	case strings.Contains(key, "equipment"):
		return energyDriverCategoryEquipment
	case strings.Contains(key, "internal"):
		return energyDriverCategoryInternalOther
	default:
		return energyDriverCategoryStorageOther
	}
}

func energyDriverCategoryLabel(category string) string {
	for _, definition := range energyDriverCategoryDefinitions {
		if strings.EqualFold(category, definition.Category) {
			return definition.Label
		}
	}
	return "Other / storage"
}

func energyDriverCategoryOrder(category string) int {
	for _, definition := range energyDriverCategoryDefinitions {
		if strings.EqualFold(category, definition.Category) {
			return definition.Order
		}
	}
	return len(energyDriverCategoryDefinitions) + 1
}

func energyDriverSourcePolicyFor(name string, kind string) energyDriverSourcePolicy {
	key := normalizeEnergyOutputName(strings.Join([]string{name, kind}, " "))
	category := canonicalEnergyDriverCategory(kind)
	policy := energyDriverSourcePolicy{
		Role:     energyDriverSourceRoleMainFlow,
		Category: category,
		Label:    energyDriverCategoryLabel(category),
	}

	// The surface-to-zone-air term is the only opaque-surface quantity used in
	// the additive driver flow. It must win before the broad "convection"
	// checks below.
	if strings.Contains(key, "surface inside face convection") || strings.Contains(key, "surface to zone air") {
		policy.Explanation = energyDriverSurfaceExplanation
		return policy
	}
	if strings.Contains(key, "zone air heat balance air energy storage") {
		policy.Category = energyDriverCategoryStorageOther
		policy.Label = energyDriverCategoryLabel(policy.Category)
		policy.Explanation = "Zone-air storage is retained as a named Other / storage balance component."
		return policy
	}
	if strings.Contains(key, "zone air heat balance deviation") {
		policy.Role = energyDriverSourceRoleReconciliation
		policy.Category = energyDriverCategoryStorageOther
		policy.Label = energyDriverCategoryLabel(policy.Category)
		policy.Explanation = "Reported heat-balance deviation is diagnostic context; it is not added to the main flow."
		return policy
	}
	if strings.Contains(key, "zone total internal convective") || strings.Contains(key, "zone total internal latent") {
		policy.Role = energyDriverSourceRoleReconciliation
		policy.Category = energyDriverCategoryInternalOther
		policy.Label = energyDriverCategoryLabel(policy.Category)
		policy.Explanation = "Aggregate internal gain is used to reconcile detailed source families and is not added directly."
		return policy
	}
	if strings.Contains(key, "zone combined outdoor air") {
		policy.Role = energyDriverSourceRoleReconciliation
		policy.Category = energyDriverCategoryMechanicalVentilation
		policy.Label = energyDriverCategoryLabel(policy.Category)
		policy.Explanation = "Combined outdoor-air output is an aggregate reconciliation source and is consumed only by an explicit residual fallback."
		return policy
	}
	if strings.Contains(key, "zone ideal loads outdoor air") || strings.Contains(key, "zone ideal loads heat recovery") || strings.Contains(key, "air system outdoor air") || strings.Contains(key, "heat exchanger") && (strings.Contains(key, "heating") || strings.Contains(key, "cooling")) {
		policy.Role = energyDriverSourceRoleContext
		policy.Category = energyDriverCategoryMechanicalVentilation
		policy.Label = energyDriverCategoryLabel(policy.Category)
		policy.Explanation = "Outdoor-air conditioning or heat-recovery context is retained without duplicating delivered load or a direct ventilation driver."
		return policy
	}

	if strings.Contains(key, "solar") || strings.Contains(key, "radiant") || strings.Contains(key, "visible") || strings.Contains(key, "return air") || strings.Contains(key, "conduction") || strings.Contains(key, "lost heat") || strings.Contains(key, "zone windows total heat") || strings.Contains(key, "fan air heat gain") ||
		strings.Contains(key, "total heating") || strings.Contains(key, "people sensible heating") {
		policy.Role = energyDriverSourceRoleContext
		policy.Explanation = "Reference-only thermal context; it is excluded from the additive zone-air heat-balance flow to avoid double counting."
		return policy
	}

	if strings.Contains(key, "zone air heat balance") &&
		(strings.Contains(key, "surface convection") ||
			strings.Contains(key, "internal convective") ||
			strings.Contains(key, "outdoor air") ||
			strings.Contains(key, "interzone air") ||
			strings.Contains(key, "system air") ||
			strings.Contains(key, "system convective")) {
		policy.Role = energyDriverSourceRoleReconciliation
		policy.Explanation = "Aggregate zone-air heat-balance check; detailed components are used for the main flow."
		return policy
	}

	return policy
}

func buildEnergyDriverPresentationPlan(nodes []EnergyExplanationNode, scope EnergyExplanationScope) energyDriverPresentationPlan {
	scope = normalizeEnergyExplanationScope(scope)
	plan := energyDriverPresentationPlan{categoryByService: map[string]map[string]string{}}
	loadTotals := energyDriverPreferredLoadTotals(nodes)
	categoryValues := map[string]map[string]float64{}
	for _, node := range nodes {
		if !energyDriverNodeUsesCanonicalTaxonomy(node) {
			continue
		}
		service := firstNonEmpty(strings.TrimSpace(node.ServiceKind), "all")
		category := canonicalEnergyDriverCategory(node.DriverCategory)
		if categoryValues[service] == nil {
			categoryValues[service] = map[string]float64{}
		}
		categoryValues[service][category] += math.Abs(energyExplanationEffectiveNodeValue(energyExplanationLegacyNodeWithEffectiveValues(node)))
	}

	for service, values := range categoryValues {
		mapping := map[string]string{
			energyDriverCategoryInternalOther: energyDriverCategoryStorageOther,
		}
		if scope.Kind == "building" {
			interzone := values[energyDriverCategoryInterzoneSurfaces] + values[energyDriverCategoryInterzoneAir]
			target := energyDriverCategoryStorageOther
			if load := loadTotals[service]; load > 0 && interzone/load >= energyDriverInterzoneThreshold {
				target = energyDriverDisplayCategoryInterzoneTransfer
			}
			mapping[energyDriverCategoryInterzoneSurfaces] = target
			mapping[energyDriverCategoryInterzoneAir] = target
		}
		plan.categoryByService[service] = mapping
		// Keep canonical contributors in the payload. Minor-category grouping
		// is a presentation projection with member/source IDs (EPATH-123),
		// never a count-based deletion before exports or correspondence links.
	}
	return plan
}

func (plan energyDriverPresentationPlan) apply(node EnergyExplanationNode) EnergyExplanationNode {
	if !energyDriverNodeUsesCanonicalTaxonomy(node) {
		return node
	}
	service := firstNonEmpty(strings.TrimSpace(node.ServiceKind), "all")
	category := canonicalEnergyDriverCategory(node.DriverCategory)
	if mapping := plan.categoryByService[service]; mapping != nil {
		if target := mapping[category]; target != "" {
			category = target
		}
	}
	node.DriverCategory = category
	node.Kind = "driver." + category
	node.Label = energyDriverCategoryLabel(category)
	return node
}

func energyDriverPreferredLoadTotals(nodes []EnergyExplanationNode) map[string]float64 {
	zoneTotals := map[string]float64{}
	allTotals := map[string]float64{}
	hasZone := map[string]bool{}
	for _, node := range nodes {
		if !strings.EqualFold(node.Level, "load") {
			continue
		}
		service := firstNonEmpty(strings.TrimSpace(node.ServiceKind), "all")
		value := math.Abs(energyExplanationEffectiveNodeValue(energyExplanationLegacyNodeWithEffectiveValues(node)))
		allTotals[service] += value
		if strings.TrimSpace(node.ZoneName) != "" {
			zoneTotals[service] += value
			hasZone[service] = true
		}
	}
	for service, total := range allTotals {
		if !hasZone[service] {
			zoneTotals[service] = total
		}
	}
	return zoneTotals
}

func energyDriverNodeUsesCanonicalTaxonomy(node EnergyExplanationNode) bool {
	return strings.EqualFold(node.Level, "heat") || strings.EqualFold(node.Level, "driver")
}

func annotateLegacyEnergyDriverSources(sources []EnergyDataSource, nodes []EnergyExplanationNode) []EnergyDataSource {
	out := append([]EnergyDataSource(nil), sources...)
	indexByID := make(map[string]int, len(out))
	for index := range out {
		indexByID[out[index].ID] = index
	}
	for _, node := range nodes {
		if !energyDriverNodeUsesCanonicalTaxonomy(node) {
			continue
		}
		for _, sourceID := range node.SourceIDs {
			index, ok := indexByID[sourceID]
			if !ok {
				continue
			}
			source := &out[index]
			if source.DriverRole == "" {
				policy := energyDriverSourcePolicyFor(strings.Join([]string{source.Name, source.KeyValue, node.Label}, " "), node.Kind)
				source.DriverRole = policy.Role
				source.DriverCategory = policy.Category
				source.Explanation = policy.Explanation
				if policy.Role != energyDriverSourceRoleMainFlow {
					source.InspectorSection = energyDriverInspectorSectionContext
				}
			}
			if source.DriverCategory == "" {
				source.DriverCategory = canonicalEnergyDriverCategory(firstNonEmpty(node.DriverCategory, node.Kind, node.HeatCategory))
			}
			if source.ZoneName == "" {
				source.ZoneName = node.ZoneName
			}
			if source.RawValue == 0 {
				source.RawValue = firstNonZero(node.RawValue, node.SignedValue, node.Value)
			}
			source.RelatedEntityIDs = appendUniqueStrings(source.RelatedEntityIDs, node.RelatedEntityIDs...)
		}
	}
	return out
}

func inferLegacyEnergyDriverProjectionGuards(nodes []EnergyExplanationNode, sources []EnergyDataSource) []EnergyExplanationNode {
	out := append([]EnergyExplanationNode(nil), nodes...)
	sourcesByID := make(map[string]EnergyDataSource, len(sources))
	for _, source := range sources {
		sourcesByID[source.ID] = source
	}
	for index := range out {
		node := &out[index]
		if node.driverZoneOnly || !energyDriverNodeUsesCanonicalTaxonomy(*node) {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(node.Kind))
		aggregateOnly := kind == "heat.interzone_air" || kind == "heat.interzone_zone_aggregate"
		for _, sourceID := range node.SourceIDs {
			source, ok := sourcesByID[sourceID]
			if !ok {
				continue
			}
			name := normalizeEnergyOutputName(source.Name)
			if strings.Contains(name, "zone mixing ") || strings.Contains(name, "zone cross mixing ") ||
				strings.Contains(name, "zone crossmixing ") || strings.Contains(name, "zone refrigeration door mixing ") ||
				strings.Contains(name, "zone air heat balance interzone air") {
				aggregateOnly = true
				break
			}
		}
		if aggregateOnly {
			// Stored v1 payloads do not serialize driverZoneOnly. Reconstruct the
			// guard so receiving-zone aggregate outputs remain inspectable for the
			// Zone but cannot be promoted into a Building interzone ribbon.
			node.driverZoneOnly = true
		}
	}
	return out
}

func selectEnergyDriverMainFlowNodes(nodes []EnergyExplanationNode, sources []EnergyDataSource) []EnergyExplanationNode {
	sourcesByID := make(map[string]EnergyDataSource, len(sources))
	for _, source := range sources {
		sourcesByID[source.ID] = source
	}
	type classifiedNode struct {
		node   EnergyExplanationNode
		policy energyDriverSourcePolicy
		family string
		key    string
	}
	classified := make([]classifiedNode, 0, len(nodes))
	mainFamilies := map[string]bool{}
	for _, node := range nodes {
		if !energyDriverNodeUsesCanonicalTaxonomy(node) {
			classified = append(classified, classifiedNode{node: node})
			continue
		}
		policy := energyDriverPolicyForNode(node, sourcesByID)
		family := energyDriverNodeFamily(node)
		key := energyDriverNodeScopeKey(node) + "|" + family
		classified = append(classified, classifiedNode{node: node, policy: policy, family: family, key: key})
		if policy.Role == energyDriverSourceRoleMainFlow {
			mainFamilies[key] = true
		}
	}

	out := make([]EnergyExplanationNode, 0, len(nodes))
	for _, item := range classified {
		if !energyDriverNodeUsesCanonicalTaxonomy(item.node) {
			out = append(out, item.node)
			continue
		}
		switch item.policy.Role {
		case energyDriverSourceRoleContext:
			continue
		case energyDriverSourceRoleReconciliation:
			if mainFamilies[item.key] {
				continue
			}
		}
		if strings.TrimSpace(item.node.DriverCategory) == "" {
			item.node.DriverCategory = canonicalEnergyDriverCategory(firstNonEmpty(item.policy.Category, item.node.Kind, item.node.HeatCategory))
		}
		out = append(out, item.node)
	}
	return out
}

func energyDriverPolicyForNode(node EnergyExplanationNode, sources map[string]EnergyDataSource) energyDriverSourcePolicy {
	policy := energyDriverSourcePolicyFor(node.Label, firstNonEmpty(node.DriverCategory, node.Kind))
	// A derived node exposes its raw formula inputs in SourceIDs for traceability.
	// Those inputs keep their own context/reconciliation roles, but must not
	// reclassify the formula result itself away from its explicit derived role.
	for _, sourceID := range node.SourceIDs {
		source, ok := sources[sourceID]
		if !ok || source.SourceType != "derived_formula" || source.DriverRole == "" {
			continue
		}
		return energyDriverSourcePolicy{
			Category:    firstNonEmpty(source.DriverCategory, node.DriverCategory),
			Role:        source.DriverRole,
			Label:       node.Label,
			Explanation: source.Explanation,
		}
	}
	for _, sourceID := range node.SourceIDs {
		source, ok := sources[sourceID]
		if !ok {
			continue
		}
		sourcePolicy := energyDriverSourcePolicyFor(strings.Join([]string{source.Name, source.KeyValue, node.Label}, " "), firstNonEmpty(source.DriverCategory, node.DriverCategory, node.Kind))
		if source.DriverRole != "" {
			sourcePolicy.Role = source.DriverRole
		}
		if source.DriverCategory != "" {
			sourcePolicy.Category = source.DriverCategory
		}
		if source.Explanation != "" {
			sourcePolicy.Explanation = source.Explanation
		}
		if energyDriverSourceRoleRank(sourcePolicy.Role) > energyDriverSourceRoleRank(policy.Role) {
			policy = sourcePolicy
		}
	}
	return policy
}

func energyDriverSourceRoleRank(role string) int {
	switch role {
	case energyDriverSourceRoleContext:
		return 3
	case energyDriverSourceRoleReconciliation:
		return 2
	case energyDriverSourceRoleMainFlow:
		return 1
	default:
		return 0
	}
}

func energyDriverNodeFamily(node EnergyExplanationNode) string {
	key := normalizeEnergyOutputName(strings.Join([]string{node.Kind, node.DriverCategory, node.HeatCategory, node.Label}, " "))
	switch {
	case strings.Contains(key, "surface") || strings.Contains(key, "window") || strings.Contains(key, "wall") || strings.Contains(key, "roof") || strings.Contains(key, "floor"):
		return "surface"
	case strings.Contains(key, "internal") || strings.Contains(key, "people") || strings.Contains(key, "lighting") || strings.Contains(key, "lights") || strings.Contains(key, "equipment"):
		return "internal"
	case strings.Contains(key, "air") || strings.Contains(key, "infiltration") || strings.Contains(key, "ventilation") || strings.Contains(key, "mixing"):
		return "air"
	default:
		return canonicalEnergyDriverCategory(firstNonEmpty(node.DriverCategory, node.Kind))
	}
}

func energyDriverNodeScopeKey(node EnergyExplanationNode) string {
	return strings.Join([]string{
		normalizeEnergyOutputName(node.ZoneName),
		normalizeEnergyOutputName(node.ServiceKind),
	}, "|")
}
