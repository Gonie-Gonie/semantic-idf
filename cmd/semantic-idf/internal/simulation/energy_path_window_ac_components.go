package simulation

import (
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type energyPathWindowACTarget = idf.NativeWindowACBinding

func energyPathWindowACTargets(doc idf.Document) []energyPathWindowACTarget {
	return idf.ResolveNativeWindowACBindings(doc)
}

func energyPathWindowACFanElectricityDefinition() energyPathDirectHVACComponentDefinition {
	return energyPathDirectHVACComponentDefinition{ID: "fans.zone_equipment.electricity", ObjectType: "Fan:OnOff", Energy: energyMeterAliasDefinition{
		Kind: "energy.fans", Label: "Fan electricity", Carrier: "electricity", EndUse: "fans", HierarchyLevel: "zone_direct_use", Aliases: []string{"Fan Electricity Energy"},
	}}
}

func energyPathWindowACDirectTargets(targets []energyPathWindowACTarget) []energyPathDirectHVACComponentTarget {
	var out []energyPathDirectHVACComponentTarget
	for _, target := range targets {
		for _, definition := range energyPathDirectHVACComponentDefinitions() {
			if definition.ID == "cooling.coil.electricity" || definition.ID == "cooling.coil.crankcase_electricity" {
				out = append(out, energyPathDirectHVACComponentTarget{Definition: definition, KeyValue: target.Coil.ObjectName, ZoneName: target.ZoneName})
			}
		}
		out = append(out, energyPathDirectHVACComponentTarget{Definition: energyPathWindowACFanElectricityDefinition(), KeyValue: target.Fan.ObjectName, ZoneName: target.ZoneName})
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i], out[j]
		return strings.ToLower(left.ZoneName+"\x00"+left.KeyValue)+left.Definition.ID < strings.ToLower(right.ZoneName+"\x00"+right.KeyValue)+right.Definition.ID
	})
	return out
}
