package simulation

import "strings"

// Native resource group meters are subtotals of the existing end uses, not
// additional consumption or a substitute Facility meter. Preserve their source
// and period observations while keeping them outside additive Energy Path flow.
func energyMeterGroupDefinitionForName(name string) (energyMeterAliasDefinition, bool) {
	parts := strings.Split(strings.TrimSpace(name), ":")
	if len(parts) != 2 {
		return energyMeterAliasDefinition{}, false
	}
	carrier, ok := energyCarrierToken(strings.TrimSpace(parts[0]))
	if !ok {
		return energyMeterAliasDefinition{}, false
	}
	group := strings.ToLower(strings.TrimSpace(parts[1]))
	switch group {
	case "building", "hvac", "plant":
		return energyMeterAliasDefinition{
			Kind: "energy.meter_group." + group, Label: strings.TrimSpace(name) + " subtotal",
			Carrier: carrier, EndUse: group + "_subtotal", HierarchyLevel: "meter_group", Aliases: []string{name},
		}, true
	default:
		return energyMeterAliasDefinition{}, false
	}
}

func filterEnergyMeterGroupContextSeries(series []energyExplanationSeries, sources []EnergyDataSource) ([]energyExplanationSeries, []EnergyDataSource) {
	contextIDs := map[string]bool{}
	out := make([]energyExplanationSeries, 0, len(series))
	for _, item := range series {
		if item.MeterHierarchyLevel != "meter_group" {
			out = append(out, item)
			continue
		}
		for _, id := range item.SourceIDs {
			contextIDs[id] = true
		}
	}
	retained := append([]EnergyDataSource(nil), sources...)
	for i := range retained {
		if contextIDs[retained[i].ID] {
			retained[i].InspectorSection = "context"
			retained[i].Explanation = "Reported Building/HVAC/Plant subtotal overlaps end-use consumption; retained as context, not an additional end use or facility total."
		}
	}
	return out, retained
}
