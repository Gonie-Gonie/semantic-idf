package simulation

import (
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Constituents are additive measurements, not aliases of one another. In
// particular, compressor and crankcase electricity, and primary and ancillary
// gas, must retain distinct identities even when they feed the same end use.
type energyPathDirectHVACComponentDefinition struct {
	ID         string
	ObjectType string
	Energy     energyMeterAliasDefinition
}

type energyPathDirectHVACComponentTarget struct {
	Definition energyPathDirectHVACComponentDefinition
	KeyValue   string
	ZoneName   string
}

func energyPathDirectHVACComponentDefinitions() []energyPathDirectHVACComponentDefinition {
	type constituent struct{ id, objectType, name, service, carrier string }
	items := []constituent{
		{"cooling.coil.electricity", "Coil:Cooling:DX:SingleSpeed", "Cooling Coil Electricity Energy", "cooling", "electricity"},
		{"cooling.coil.crankcase_electricity", "Coil:Cooling:DX:SingleSpeed", "Cooling Coil Crankcase Heater Electricity Energy", "cooling", "electricity"},
		{"heating.coil.natural_gas", "Coil:Heating:Fuel", "Heating Coil NaturalGas Energy", "heating", "natural_gas"},
		{"heating.coil.ancillary_natural_gas", "Coil:Heating:Fuel", "Heating Coil Ancillary NaturalGas Energy", "heating", "natural_gas"},
		{"heating.coil.electricity", "Coil:Heating:Fuel", "Heating Coil Electricity Energy", "heating", "electricity"},
	}
	definitions := make([]energyPathDirectHVACComponentDefinition, 0, len(items))
	for _, item := range items {
		definitions = append(definitions, energyPathDirectHVACComponentDefinition{
			ID: item.id, ObjectType: item.objectType,
			Energy: energyMeterAliasDefinition{Kind: "energy." + item.service, Label: item.name,
				Carrier: item.carrier, EndUse: item.service, HierarchyLevel: "zone_direct_use", Aliases: []string{item.name}},
		})
	}
	return definitions
}

func energyPathDirectHVACComponentDefinitionForName(name string) (energyPathDirectHVACComponentDefinition, bool) {
	for _, definition := range energyPathDirectHVACComponentDefinitions() {
		for _, alias := range definition.Energy.Aliases {
			if strings.EqualFold(strings.TrimSpace(name), alias) {
				return definition, true
			}
		}
	}
	return energyPathDirectHVACComponentDefinition{}, false
}

func energyPathDirectHVACComponentKey(objectType, name string) string {
	return strings.ToLower(strings.TrimSpace(objectType)) + "\x00" + strings.ToLower(strings.TrimSpace(name))
}

// Ownership is established against the whole document before any output scope
// is applied. A central/shared coil, unresolved reference, duplicate object, or
// ambiguous equipment-list owner cannot become direct merely by selecting one
// Zone. This deliberately supports only the reviewed native PTAC coil pair.
func energyPathDirectHVACComponentTargets(doc idf.Document) []energyPathDirectHVACComponentTarget {
	const ptacType = "ZoneHVAC:PackagedTerminalAirConditioner"
	hasPTAC := false
	for _, object := range doc.Objects {
		if strings.EqualFold(strings.TrimSpace(object.Type), ptacType) {
			hasPTAC = true
			break
		}
	}
	if !hasPTAC {
		return nil // Do not add a full HVAC analysis to non-PTAC result building.
	}
	objects := map[string][]int{}
	references := map[string][]int{}
	for index, object := range doc.Objects {
		if len(object.Fields) > 0 {
			key := energyPathDirectHVACComponentKey(object.Type, object.Fields[0].Value)
			objects[key] = append(objects[key], index)
		}
		// Native equipment, wrapper and Branch references use adjacent typed
		// object/name fields. Count even disconnected parents, not just paths
		// surviving the HVAC graph's resolution/deduplication.
		for field := 0; field+1 < len(object.Fields); field++ {
			key := energyPathDirectHVACComponentKey(object.Fields[field].Value, object.Fields[field+1].Value)
			references[key] = append(references[key], index)
		}
	}
	model := idf.AnalyzeHVAC(doc).ServiceModel
	components := map[string][]idf.ComponentIndexItem{}
	for _, item := range model.Components {
		key := energyPathDirectHVACComponentKey(item.Component.ObjectType, item.Component.ObjectName)
		components[key] = append(components[key], item)
	}
	var targets []energyPathDirectHVACComponentTarget
	for index, parent := range doc.Objects {
		if !strings.EqualFold(strings.TrimSpace(parent.Type), ptacType) || len(parent.Fields) == 0 {
			continue
		}
		parentKey := energyPathDirectHVACComponentKey(parent.Type, parent.Fields[0].Value)
		if len(objects[parentKey]) != 1 || len(components[parentKey]) != 1 {
			continue
		}
		listIndex, listCount, mixerCount, parentRefsValid := -1, 0, 0, true
		for _, referenceIndex := range references[parentKey] {
			reference := doc.Objects[referenceIndex]
			switch strings.ToLower(strings.TrimSpace(reference.Type)) {
			case "zonehvac:equipmentlist":
				listIndex, listCount = referenceIndex, listCount+1
			case "airterminal:singleduct:mixer":
				// A DOAS mixer references the packaged unit to connect air nodes;
				// it is not a second equipment owner. Require that exact native
				// connection, not an arbitrary wrapper bearing a typed reference.
				mixerCount++
				if len(reference.Fields) < 7 || len(parent.Fields) < 4 ||
					len(objects[energyPathDirectHVACComponentKey(reference.Type, reference.Fields[0].Value)]) != 1 ||
					energyPathDirectHVACComponentKey(reference.Fields[1].Value, reference.Fields[2].Value) != parentKey {
					parentRefsValid = false
					continue
				}
				connection := strings.TrimSpace(reference.Fields[6].Value)
				if (strings.EqualFold(connection, "InletSide") && (strings.TrimSpace(parent.Fields[2].Value) == "" || !strings.EqualFold(strings.TrimSpace(reference.Fields[3].Value), strings.TrimSpace(parent.Fields[2].Value)))) ||
					(strings.EqualFold(connection, "SupplySide") && (strings.TrimSpace(parent.Fields[3].Value) == "" || !strings.EqualFold(strings.TrimSpace(reference.Fields[5].Value), strings.TrimSpace(parent.Fields[3].Value)))) ||
					(!strings.EqualFold(connection, "InletSide") && !strings.EqualFold(connection, "SupplySide")) {
					parentRefsValid = false
				}
			default:
				parentRefsValid = false
			}
		}
		if !parentRefsValid || listCount != 1 || mixerCount > 1 {
			continue
		}
		list := doc.Objects[listIndex]
		if !strings.EqualFold(strings.TrimSpace(list.Type), "ZoneHVAC:EquipmentList") || len(list.Fields) == 0 ||
			len(objects[energyPathDirectHVACComponentKey(list.Type, list.Fields[0].Value)]) != 1 {
			continue
		}
		zoneName, connectionCount := "", 0
		for _, connection := range doc.Objects {
			if strings.EqualFold(strings.TrimSpace(connection.Type), "ZoneHVAC:EquipmentConnections") && len(connection.Fields) >= 2 &&
				strings.EqualFold(strings.TrimSpace(connection.Fields[1].Value), strings.TrimSpace(list.Fields[0].Value)) {
				zoneName = strings.TrimSpace(connection.Fields[0].Value)
				connectionCount++
			}
		}
		if zoneName == "" || connectionCount != 1 || len(objects[energyPathDirectHVACComponentKey("Zone", zoneName)]) != 1 {
			continue
		}
		item := components[parentKey][0]
		if len(item.Occurrences) != 1 || item.Occurrences[0].ContextType != "zone_equipment" ||
			!strings.EqualFold(item.Occurrences[0].ZoneName, zoneName) || item.Occurrences[0].SpaceName != "" {
			continue
		}
		services := map[string]bool{}
		for _, summary := range model.ZoneServices {
			if !strings.EqualFold(summary.ZoneName, zoneName) {
				continue
			}
			for _, path := range summary.Paths {
				if energyPathDirectHVACComponentKey(path.Delivery.ObjectType, path.Delivery.ObjectName) == parentKey &&
					path.AirLoop == nil && path.PlantLoop == nil {
					services[path.ServiceKind] = true
				}
			}
		}
		if !services["cooling"] || !services["heating"] {
			continue
		}
		resolved := map[string][]idf.ComponentRef{}
		for _, ref := range item.InternalRefs {
			resolved[strings.ToLower(strings.TrimSpace(ref.ObjectType))] = append(resolved[strings.ToLower(strings.TrimSpace(ref.ObjectType))], ref)
		}
		valid := true
		for _, objectType := range []string{"Fan:OnOff", "Coil:Cooling:DX:SingleSpeed", "Coil:Heating:Fuel"} {
			refs := resolved[strings.ToLower(objectType)]
			if len(refs) != 1 {
				valid = false
				break
			}
			key := energyPathDirectHVACComponentKey(objectType, refs[0].ObjectName)
			if len(objects[key]) != 1 || len(references[key]) != 1 || references[key][0] != index {
				valid = false
				break
			}
			if objectType == "Coil:Heating:Fuel" {
				coil := doc.Objects[objects[key][0]]
				if len(coil.Fields) < 3 || !strings.EqualFold(strings.TrimSpace(coil.Fields[2].Value), "NaturalGas") {
					valid = false
				}
			}
		}
		if !valid {
			continue
		}
		for _, definition := range energyPathDirectHVACComponentDefinitions() {
			ref := resolved[strings.ToLower(definition.ObjectType)][0]
			targets = append(targets, energyPathDirectHVACComponentTarget{Definition: definition, KeyValue: ref.ObjectName, ZoneName: zoneName})
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		left, right := targets[i], targets[j]
		return energyPathDirectHVACComponentKey(left.ZoneName, left.KeyValue)+left.Definition.ID < energyPathDirectHVACComponentKey(right.ZoneName, right.KeyValue)+right.Definition.ID
	})
	return targets
}

func (builder *purposePlanBuilder) addEnergyPathDirectHVACComponentOutputs() {
	selected, scoped := purposeSelectedZoneSet(builder.request.Scope)
	for _, target := range energyPathDirectHVACComponentTargets(builder.doc) {
		if scoped && !selected[normalizePurposeToken(target.ZoneName)] {
			continue
		}
		for _, name := range target.Definition.Energy.Aliases {
			builder.addVariableWithReasonAndScopeZone(SimulationPurposeBasicEnergy, target.KeyValue, name, "Monthly", "medium",
				"Monthly consumption of an exclusively Zone-owned PTAC coil constituent; excludes package and supply-fan totals.", "Basic Energy Path", target.ZoneName)
		}
	}
}
