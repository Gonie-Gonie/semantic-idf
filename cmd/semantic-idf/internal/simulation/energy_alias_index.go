package simulation

// These indexes contain only the fixed catalogs, never arbitrary model names.
// Build once and then read concurrently without mutating either maps or values.
// Catalog order remains authoritative when normalized aliases collide.
var energyAliasDefinitions = newEnergyAliasDefinitionIndex()

type energyAliasDefinitionIndex struct {
	meter    map[string]energyMeterAliasDefinition
	variable map[string]energyMeterAliasDefinition
	load     map[string]energyLoadAliasDefinition
	heat     map[string]energyHeatAliasDefinition
}

func newEnergyAliasDefinitionIndex() energyAliasDefinitionIndex {
	return energyAliasDefinitionIndex{
		meter:    indexEnergyMeterAliases(energyMeterAliasCatalog()),
		variable: indexEnergyMeterAliases(append(energyVariableAliasCatalog(), energyPathDirectUseVariableAliasCatalog()...)),
		load:     indexEnergyLoadAliases(energyLoadAliasCatalog()),
		heat:     indexEnergyHeatAliases(energyHeatAliasCatalog()),
	}
}

func indexEnergyMeterAliases(definitions []energyMeterAliasDefinition) map[string]energyMeterAliasDefinition {
	out := map[string]energyMeterAliasDefinition{}
	for _, definition := range definitions {
		for _, alias := range definition.Aliases {
			key := normalizeEnergyOutputName(alias)
			if _, exists := out[key]; !exists {
				out[key] = cloneEnergyMeterAliasDefinition(definition)
			}
		}
	}
	return out
}

func indexEnergyLoadAliases(definitions []energyLoadAliasDefinition) map[string]energyLoadAliasDefinition {
	out := map[string]energyLoadAliasDefinition{}
	for _, definition := range definitions {
		for _, alias := range definition.Aliases {
			key := normalizeEnergyOutputName(alias)
			if _, exists := out[key]; !exists {
				out[key] = cloneEnergyLoadAliasDefinition(definition)
			}
		}
	}
	return out
}

func indexEnergyHeatAliases(definitions []energyHeatAliasDefinition) map[string]energyHeatAliasDefinition {
	out := map[string]energyHeatAliasDefinition{}
	for _, definition := range definitions {
		for _, alias := range definition.Aliases {
			key := normalizeEnergyOutputName(alias)
			if _, exists := out[key]; !exists {
				out[key] = cloneEnergyHeatAliasDefinition(definition)
			}
		}
	}
	return out
}

func cloneEnergyAliasNames(names []string) []string {
	if names == nil {
		return nil
	}
	out := make([]string, len(names))
	copy(out, names)
	return out
}

// Lookup callers previously received freshly constructed catalog slices. Keep
// that ownership contract, including the distinction between nil and empty.
func cloneEnergyMeterAliasDefinition(definition energyMeterAliasDefinition) energyMeterAliasDefinition {
	definition.Aliases = cloneEnergyAliasNames(definition.Aliases)
	definition.LegacyAliases = cloneEnergyAliasNames(definition.LegacyAliases)
	definition.OutputRequestAliases = cloneEnergyAliasNames(definition.OutputRequestAliases)
	return definition
}

func cloneEnergyLoadAliasDefinition(definition energyLoadAliasDefinition) energyLoadAliasDefinition {
	definition.Aliases = cloneEnergyAliasNames(definition.Aliases)
	definition.LegacyAliases = cloneEnergyAliasNames(definition.LegacyAliases)
	return definition
}

func cloneEnergyHeatAliasDefinition(definition energyHeatAliasDefinition) energyHeatAliasDefinition {
	definition.Aliases = cloneEnergyAliasNames(definition.Aliases)
	definition.OutputRequestAliases = cloneEnergyAliasNames(definition.OutputRequestAliases)
	return definition
}
