package simulation

import (
	"math"
	"sort"
	"strings"
)

const (
	energyDriverAllocationExplanation = "Deterministic signed heat-balance share allocation (non-causal; not a direct causal decomposition)."
	energyDriverAllocationFormula     = "actual canonical service load * matching driver signed pressure / sum of matching signed pressures"
	energyDriverOffsetBasis           = "signed_heat_balance_offset"
	energyDriverOffsetExplanation     = "Opposite-sign heat-balance pressure is non-additive, non-causal context and is not an avoided-load quantity."
)

type energyDriverAllocationCandidate struct {
	id       string
	pressure float64
}

type energyDriverAllocationLoad struct {
	value     float64
	unit      string
	zoneName  string
	period    string
	nodeIDs   []string
	sourceIDs []string
}

// allocateCanonicalEnergyDriverNodes runs only for the canonical Energy Path
// build. The caller invokes it once per graph period; because annual canonical
// output is rebuilt from monthly graphs, Building and annual values can only be
// sums of completed zone-month allocations.
func allocateCanonicalEnergyDriverNodes(nodes map[string]*energyExplanationNodeAccumulator, loadNodesByZoneService map[string][]string, enabled bool) bool {
	if !enabled {
		return false
	}
	loads := collectEnergyDriverAllocationLoads(nodes, loadNodesByZoneService)
	appendEnergyDriverOffsetAndSimultaneousMetadata(nodes, loads)

	// Explicitly mark every zone-native driver, including a non-zero raw driver
	// whose corresponding actual load is zero. An allocated zero is data, not a
	// missing value that may fall back to the raw pressure during v1->v2 upgrade.
	for _, entry := range nodes {
		if entry == nil || !strings.EqualFold(entry.node.Level, "heat") || entry.node.driverBuildingOnly {
			continue
		}
		setEnergyDriverAllocatedContribution(&entry.node, 0)
	}

	keys := make([]string, 0, len(loadNodesByZoneService))
	for key := range loadNodesByZoneService {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		zoneID, service := splitEnergyExplanationZoneServiceKey(key)
		service = energyCanonicalServiceKind(service)
		if zoneID == "" || (service != "cooling" && service != "heating") {
			continue
		}
		loadValue := 0.0
		loadUnit := "kWh"
		loadZoneName := zoneID
		loadSourceIDs := []string{}
		for _, loadID := range loadNodesByZoneService[key] {
			load := nodes[loadID]
			if load == nil {
				continue
			}
			loadValue = roundedEnergyNumber(loadValue + math.Abs(load.node.Value))
			loadUnit = firstNonEmpty(load.node.Unit, loadUnit)
			loadZoneName = firstNonEmpty(load.node.ZoneName, loadZoneName)
			loadSourceIDs = appendUniqueStrings(loadSourceIDs, load.node.SourceIDs...)
		}
		if loadValue <= 0 {
			continue
		}

		candidates := make([]energyDriverAllocationCandidate, 0)
		denominator := 0.0
		for id, entry := range nodes {
			if entry == nil || !strings.EqualFold(entry.node.Level, "heat") || entry.node.driverBuildingOnly ||
				!strings.EqualFold(entry.node.ZoneName, loadZoneName) {
				continue
			}
			// The pre-allocation closure term is useful reconciliation context, but
			// feeding it back into the denominator would make every physical driver
			// keep its raw pressure instead of sharing the actual delivered load.
			if energyDriverIsSyntheticPreAllocationClosure(entry.node) {
				continue
			}
			pressure := energyDriverSignedPressure(entry.node.SignedValue, service)
			if pressure <= 0 {
				continue
			}
			candidates = append(candidates, energyDriverAllocationCandidate{id: id, pressure: pressure})
			denominator += pressure
		}
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].id < candidates[j].id })
		if denominator <= 1e-12 || len(candidates) == 0 {
			appendEnergyDriverZeroPressureFallback(nodes, loadZoneName, service, loadUnit, loadValue, loadSourceIDs)
			continue
		}

		remaining := loadValue
		for index, candidate := range candidates {
			contribution := remaining
			if index < len(candidates)-1 {
				contribution = roundedEnergyNumber(loadValue * candidate.pressure / denominator)
				if contribution < 0 {
					contribution = 0
				}
				if contribution > remaining {
					contribution = remaining
				}
			}
			contribution = roundedEnergyNumber(contribution)
			setEnergyDriverAllocatedContribution(&nodes[candidate.id].node, contribution)
			remaining = roundedEnergyNumber(remaining - contribution)
		}
	}
	allocateCanonicalBuildingInterzoneNodes(nodes)
	return true
}

func allocateCanonicalBuildingInterzoneNodes(nodes map[string]*energyExplanationNodeAccumulator) {
	suppressedByService := map[string]float64{}
	buildingByService := map[string][]string{}
	for id, entry := range nodes {
		if entry == nil || !strings.EqualFold(entry.node.Level, "heat") {
			continue
		}
		service := energyCanonicalServiceKind(entry.node.ServiceKind)
		if service != "cooling" && service != "heating" {
			continue
		}
		switch {
		case entry.node.driverZoneOnly:
			suppressedByService[service] = roundedEnergyNumber(suppressedByService[service] + math.Abs(entry.node.AllocatedValue))
		case entry.node.driverBuildingOnly || entry.node.Kind == "heat.interzone_building_residual":
			buildingByService[service] = append(buildingByService[service], id)
		}
	}
	for service, ids := range buildingByService {
		sort.Strings(ids)
		denominator := 0.0
		for _, id := range ids {
			denominator += math.Abs(nodes[id].node.SignedValue)
		}
		// A raw Building pair imbalance and a completed zone-month allocation
		// are different domains. Retain no more than either quantity: this keeps
		// a material raw pair net visible when the allocated pair contribution
		// can support it, while preventing that raw net from widening the
		// Building flow beyond the contribution already completed in the zones.
		target := math.Min(denominator, suppressedByService[service])
		remaining := target
		for index, id := range ids {
			contribution := 0.0
			if denominator > 0 {
				contribution = remaining
				if index < len(ids)-1 {
					contribution = roundedEnergyNumber(target * math.Abs(nodes[id].node.SignedValue) / denominator)
					if contribution > remaining {
						contribution = remaining
					}
				}
			}
			setEnergyDriverAllocatedContribution(&nodes[id].node, contribution)
			remaining = roundedEnergyNumber(remaining - contribution)
		}
	}
}

func collectEnergyDriverAllocationLoads(nodes map[string]*energyExplanationNodeAccumulator, loadNodesByZoneService map[string][]string) map[string]energyDriverAllocationLoad {
	out := make(map[string]energyDriverAllocationLoad, len(loadNodesByZoneService))
	for key, nodeIDs := range loadNodesByZoneService {
		load := energyDriverAllocationLoad{unit: "kWh", nodeIDs: appendUniqueStrings(nil, nodeIDs...)}
		for _, nodeID := range nodeIDs {
			entry := nodes[nodeID]
			if entry == nil {
				continue
			}
			load.value = roundedEnergyNumber(load.value + math.Abs(entry.node.Value))
			load.unit = firstNonEmpty(entry.node.Unit, load.unit)
			load.zoneName = firstNonEmpty(entry.node.ZoneName, load.zoneName)
			load.period = firstNonEmpty(entry.node.Period, load.period)
			load.sourceIDs = appendUniqueStrings(load.sourceIDs, entry.node.SourceIDs...)
		}
		out[key] = load
	}
	return out
}

func appendEnergyDriverOffsetAndSimultaneousMetadata(nodes map[string]*energyExplanationNodeAccumulator, loads map[string]energyDriverAllocationLoad) {
	zones := map[string]string{}
	for key, load := range loads {
		zone, _ := splitEnergyExplanationZoneServiceKey(key)
		zone = firstNonEmpty(load.zoneName, zone)
		if zone != "" {
			zones[normalizeEnergySurfaceKey(zone)] = zone
		}
	}
	zoneKeys := make([]string, 0, len(zones))
	for key := range zones {
		zoneKeys = append(zoneKeys, key)
	}
	sort.Strings(zoneKeys)
	for _, zoneKey := range zoneKeys {
		zoneName := zones[zoneKey]
		coolingKey := energyExplanationZoneServiceKey(zoneName, "cooling")
		heatingKey := energyExplanationZoneServiceKey(zoneName, "heating")
		cooling := loads[coolingKey]
		heating := loads[heatingKey]
		if cooling.value > 0 || heating.value > 0 {
			contribution := energyExplanationSimultaneousLoadContribution{
				Key:         normalizeEnergySurfaceKey(zoneName) + "|" + firstNonEmpty(cooling.period, heating.period),
				Numerator:   roundedEnergyNumber(math.Min(cooling.value, heating.value)),
				Denominator: roundedEnergyNumber(math.Max(cooling.value, heating.value)),
				Unit:        firstNonEmpty(cooling.unit, heating.unit, "kWh"),
				SourceIDs:   appendUniqueStrings(cooling.sourceIDs, heating.sourceIDs...),
			}
			for _, nodeID := range appendUniqueStrings(cooling.nodeIDs, heating.nodeIDs...) {
				if entry := nodes[nodeID]; entry != nil {
					entry.node.simultaneousLoadContributions = mergeEnergyExplanationSimultaneousLoadContributions(entry.node.simultaneousLoadContributions, []energyExplanationSimultaneousLoadContribution{contribution})
					finalizeEnergyExplanationSimultaneousLoad(&entry.node)
				}
			}
		}

		for _, entry := range nodes {
			if entry == nil || !strings.EqualFold(entry.node.Level, "heat") || entry.node.driverBuildingOnly ||
				!strings.EqualFold(entry.node.ZoneName, zoneName) || entry.node.SignedValue == 0 || energyDriverIsSyntheticPreAllocationClosure(entry.node) {
				continue
			}
			targetService := "heating"
			effectKind := "reduces_heating"
			direction := "gain"
			if entry.node.SignedValue < 0 {
				targetService = "cooling"
				effectKind = "reduces_cooling"
				direction = "loss"
			}
			effectSourceIDs := entry.node.allocationSourceIDs
			if len(effectSourceIDs) == 0 {
				effectSourceIDs = entry.node.SourceIDs
			}
			effect := EnergyExplanationOffsetEffect{
				EffectKind:     effectKind,
				TargetService:  targetService,
				DriverCategory: canonicalEnergyDriverCategory(firstNonEmpty(entry.node.DriverCategory, entry.node.Kind)),
				Label:          firstNonEmpty(entry.node.Label, energyDriverCategoryLabel(entry.node.DriverCategory)),
				HeatDirection:  direction,
				RawValue:       math.Abs(entry.node.RawValue),
				EffectiveValue: math.Abs(entry.node.SignedValue),
				Unit:           entry.node.Unit,
				Basis:          energyDriverOffsetBasis,
				Explanation:    energyDriverOffsetExplanation,
				SourceIDs:      appendUniqueStrings(nil, effectSourceIDs...),
			}
			entry.node.OffsetEffects = mergeEnergyExplanationOffsetEffects(entry.node.OffsetEffects, []EnergyExplanationOffsetEffect{effect})
			for _, loadNodeID := range loads[energyExplanationZoneServiceKey(zoneName, targetService)].nodeIDs {
				if loadNode := nodes[loadNodeID]; loadNode != nil {
					loadNode.node.OffsetEffects = mergeEnergyExplanationOffsetEffects(loadNode.node.OffsetEffects, []EnergyExplanationOffsetEffect{effect})
				}
			}
		}
	}
}

func energyDriverIsSyntheticPreAllocationClosure(node EnergyExplanationNode) bool {
	return strings.EqualFold(strings.TrimSpace(node.Kind), "heat.unmapped_zone_balance")
}

func energyDriverSignedPressure(signedValue float64, service string) float64 {
	switch energyCanonicalServiceKind(service) {
	case "cooling":
		return math.Max(signedValue, 0)
	case "heating":
		return math.Max(-signedValue, 0)
	default:
		return 0
	}
}

func setEnergyDriverAllocatedContribution(node *EnergyExplanationNode, contribution float64) {
	if node == nil {
		return
	}
	contribution = roundedEnergyNumber(math.Max(contribution, 0))
	node.AllocationApplied = true
	node.AllocationExplanation = energyDriverAllocationExplanation
	node.AllocatedValue = contribution
	node.Value = contribution
	node.DisplayValue = contribution
	node.Basis = "heat_balance_share"
}

func appendEnergyDriverZeroPressureFallback(nodes map[string]*energyExplanationNodeAccumulator, zoneName string, service string, unit string, loadValue float64, sourceIDs []string) {
	id := strings.Join([]string{"heat", "allocation_fallback_storage", canonicalEnergyPathPart(service), canonicalEnergyPathPart(zoneName)}, ".")
	sign := "positive"
	if service == "heating" {
		sign = "negative"
	}
	node := EnergyExplanationNode{
		ID:                    id,
		Level:                 "heat",
		Kind:                  "heat.allocation_fallback_storage",
		Label:                 energyDriverCategoryLabel(energyDriverCategoryStorageOther),
		Value:                 roundedEnergyNumber(loadValue),
		AllocatedValue:        roundedEnergyNumber(loadValue),
		AllocationApplied:     true,
		AllocationExplanation: energyDriverAllocationExplanation,
		DisplayValue:          roundedEnergyNumber(loadValue),
		Unit:                  firstNonEmpty(unit, "kWh"),
		ZoneName:              zoneName,
		ServiceKind:           service,
		DriverCategory:        energyDriverCategoryStorageOther,
		ThermalComponent:      "combined",
		Sign:                  sign,
		Basis:                 "heat_balance_share",
		Multiplier:            1,
		SourceIDs:             appendUniqueStrings(nil, sourceIDs...),
	}
	nodes[id] = &energyExplanationNodeAccumulator{node: node}
}

func mergeEnergyExplanationOffsetEffects(current []EnergyExplanationOffsetEffect, next []EnergyExplanationOffsetEffect) []EnergyExplanationOffsetEffect {
	out := cloneEnergyExplanationOffsetEffects(current)
	indexByKey := make(map[string]int, len(out))
	for index, effect := range out {
		indexByKey[energyExplanationOffsetEffectKey(effect)] = index
	}
	for _, effect := range next {
		key := energyExplanationOffsetEffectKey(effect)
		if index, exists := indexByKey[key]; exists {
			out[index].RawValue = roundedEnergyNumber(out[index].RawValue + effect.RawValue)
			out[index].EffectiveValue = roundedEnergyNumber(out[index].EffectiveValue + effect.EffectiveValue)
			out[index].Unit = firstNonEmpty(out[index].Unit, effect.Unit)
			out[index].Label = firstNonEmpty(out[index].Label, effect.Label)
			out[index].Basis = firstNonEmpty(out[index].Basis, effect.Basis)
			out[index].Explanation = firstNonEmpty(out[index].Explanation, effect.Explanation)
			out[index].SourceIDs = appendUniqueStrings(out[index].SourceIDs, effect.SourceIDs...)
			continue
		}
		effect.RawValue = roundedEnergyNumber(effect.RawValue)
		effect.EffectiveValue = roundedEnergyNumber(effect.EffectiveValue)
		effect.SourceIDs = appendUniqueStrings(nil, effect.SourceIDs...)
		indexByKey[key] = len(out)
		out = append(out, effect)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left, right := energyExplanationOffsetEffectKey(out[i]), energyExplanationOffsetEffectKey(out[j])
		return left < right
	})
	return out
}

func cloneEnergyExplanationOffsetEffects(input []EnergyExplanationOffsetEffect) []EnergyExplanationOffsetEffect {
	if len(input) == 0 {
		return nil
	}
	out := make([]EnergyExplanationOffsetEffect, len(input))
	for index, effect := range input {
		out[index] = effect
		out[index].SourceIDs = appendUniqueStrings(nil, effect.SourceIDs...)
	}
	return out
}

func energyExplanationOffsetEffectKey(effect EnergyExplanationOffsetEffect) string {
	return strings.Join([]string{
		normalizeEnergyOutputName(effect.EffectKind),
		normalizeEnergyOutputName(effect.TargetService),
		normalizeEnergyOutputName(effect.DriverCategory),
		normalizeEnergyOutputName(effect.HeatDirection),
	}, "|")
}

func mergeEnergyExplanationSimultaneousLoadContributions(current []energyExplanationSimultaneousLoadContribution, next []energyExplanationSimultaneousLoadContribution) []energyExplanationSimultaneousLoadContribution {
	out := cloneEnergyExplanationSimultaneousLoadContributions(current)
	indexByKey := make(map[string]int, len(out))
	for index, contribution := range out {
		indexByKey[contribution.Key] = index
	}
	for _, contribution := range next {
		if index, exists := indexByKey[contribution.Key]; exists {
			out[index].SourceIDs = appendUniqueStrings(out[index].SourceIDs, contribution.SourceIDs...)
			continue
		}
		contribution.SourceIDs = appendUniqueStrings(nil, contribution.SourceIDs...)
		indexByKey[contribution.Key] = len(out)
		out = append(out, contribution)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func cloneEnergyExplanationSimultaneousLoadContributions(input []energyExplanationSimultaneousLoadContribution) []energyExplanationSimultaneousLoadContribution {
	if len(input) == 0 {
		return nil
	}
	out := make([]energyExplanationSimultaneousLoadContribution, len(input))
	for index, contribution := range input {
		out[index] = contribution
		out[index].SourceIDs = appendUniqueStrings(nil, contribution.SourceIDs...)
	}
	return out
}

func finalizeEnergyExplanationSimultaneousLoad(node *EnergyExplanationNode) {
	if node == nil || !strings.EqualFold(node.Level, "load") || len(node.simultaneousLoadContributions) == 0 {
		return
	}
	metric := EnergyExplanationSimultaneousLoad{
		Available: true,
		Basis:     "simultaneous_min_over_max",
	}
	for _, contribution := range node.simultaneousLoadContributions {
		metric.Numerator = roundedEnergyNumber(metric.Numerator + contribution.Numerator)
		metric.Denominator = roundedEnergyNumber(metric.Denominator + contribution.Denominator)
		metric.Unit = firstNonEmpty(metric.Unit, contribution.Unit)
		metric.SourceIDs = appendUniqueStrings(metric.SourceIDs, contribution.SourceIDs...)
	}
	if metric.Denominator > 0 {
		metric.Ratio = roundSimulationDisplayNumber(metric.Numerator / metric.Denominator)
	}
	node.SimultaneousLoad = &metric
}

func cloneEnergyExplanationSimultaneousLoad(input *EnergyExplanationSimultaneousLoad) *EnergyExplanationSimultaneousLoad {
	if input == nil {
		return nil
	}
	out := *input
	out.SourceIDs = appendUniqueStrings(nil, input.SourceIDs...)
	return &out
}

func synchronizeEnergyExplanationSimultaneousLoads(nodes map[string]*EnergyExplanationNode) {
	all := []energyExplanationSimultaneousLoadContribution{}
	for _, node := range nodes {
		if node.Level != "load" {
			continue
		}
		service := energyCanonicalServiceKind(node.ServiceKind)
		if service != "cooling" && service != "heating" {
			continue
		}
		all = mergeEnergyExplanationSimultaneousLoadContributions(all, node.simultaneousLoadContributions)
	}
	if len(all) == 0 {
		return
	}
	for _, node := range nodes {
		if node.Level != "load" {
			continue
		}
		service := energyCanonicalServiceKind(node.ServiceKind)
		if service != "cooling" && service != "heating" {
			continue
		}
		node.simultaneousLoadContributions = cloneEnergyExplanationSimultaneousLoadContributions(all)
		finalizeEnergyExplanationSimultaneousLoad(node)
	}
}
