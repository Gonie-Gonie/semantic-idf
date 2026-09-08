package simulation

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const (
	energyPathVRFOutdoorType  = "AirConditioner:VariableRefrigerantFlow"
	energyPathVRFTerminalType = "ZoneHVAC:TerminalUnit:VariableRefrigerantFlow"
	energyPathVRFCoolingType  = "Coil:Cooling:DX:VariableRefrigerantFlow"
	energyPathVRFHeatingType  = "Coil:Heating:DX:VariableRefrigerantFlow"
)

// Local terminal electricity and shared outdoor electricity are additive
// constituents, not aliases or alternative measurements of the same subtotal.
type energyPathVRFConsumptionDefinition struct {
	ID         string
	ObjectType string
	Energy     energyMeterAliasDefinition
	Shared     bool
}

type energyPathVRFConsumptionTarget struct {
	Definition  energyPathVRFConsumptionDefinition
	KeyValue    string
	ZoneName    string // Empty for the shared outdoor source, never a selected owner.
	ObjectIndex int
}

type energyPathVRFTerminal struct {
	ZoneName    string
	Terminal    idf.ComponentRef
	CoolingCoil idf.ComponentRef
	HeatingCoil idf.ComponentRef
	Fan         idf.ComponentRef
}

type energyPathVRFSystem struct {
	OutdoorUnit      idf.ComponentRef
	TerminalUnitList idf.ComponentRef
	Terminals        []energyPathVRFTerminal // Complete original owners, before scope.
	Targets          []energyPathVRFConsumptionTarget
}

func energyPathVRFConsumptionDefinitions() []energyPathVRFConsumptionDefinition {
	items := []struct {
		id, name, service string
		shared            bool
	}{
		{"cooling.vrf.terminal_electricity", "Zone VRF Air Terminal Cooling Electricity Energy", "cooling", false},
		{"heating.vrf.terminal_electricity", "Zone VRF Air Terminal Heating Electricity Energy", "heating", false},
		{"cooling.vrf.outdoor_electricity", "VRF Heat Pump Cooling Electricity Energy", "cooling", true},
		{"cooling.vrf.crankcase_electricity", "VRF Heat Pump Crankcase Heater Electricity Energy", "cooling", true},
		{"heating.vrf.outdoor_electricity", "VRF Heat Pump Heating Electricity Energy", "heating", true},
		{"heating.vrf.defrost_electricity", "VRF Heat Pump Defrost Electricity Energy", "heating", true},
	}
	out := make([]energyPathVRFConsumptionDefinition, 0, len(items))
	for _, item := range items {
		objectType, hierarchy := energyPathVRFTerminalType, "zone_direct_use"
		if item.shared {
			objectType, hierarchy = energyPathVRFOutdoorType, "shared_hvac_component"
		}
		out = append(out, energyPathVRFConsumptionDefinition{
			ID: item.id, ObjectType: objectType, Shared: item.shared,
			Energy: energyMeterAliasDefinition{Kind: "energy." + item.service, Label: item.name,
				EndUse: item.service, Carrier: "electricity", HierarchyLevel: hierarchy, Aliases: []string{item.name}},
		})
	}
	return out
}

// Recognition alone is not ownership. A reader must additionally match the
// original system's exact typed target, key, frequency and request scope.
func energyPathVRFConsumptionDefinitionForName(name string) (energyPathVRFConsumptionDefinition, bool) {
	for _, definition := range energyPathVRFConsumptionDefinitions() {
		if strings.EqualFold(strings.TrimSpace(name), definition.Energy.Aliases[0]) {
			return definition, true
		}
	}
	return energyPathVRFConsumptionDefinition{}, false
}

type energyPathVRFOwnershipIndex struct {
	doc          idf.Document
	objects      map[string][]int
	typedRefs    map[string][]int
	tuLists      map[string][]int
	listOwners   map[string][]int
	outdoorNames map[string]int
}

func energyPathVRFName(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func energyPathVRFField(object idf.Object, index int) string {
	if index < 0 || index >= len(object.Fields) {
		return ""
	}
	return strings.TrimSpace(object.Fields[index].Value)
}

func energyPathVRFSameNode(left, right string) bool {
	return strings.TrimSpace(left) != "" && strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func energyPathVRFComponentRef(object idf.Object) idf.ComponentRef {
	return idf.ComponentRef{ID: fmt.Sprintf("component:%d", object.Index), ObjectType: object.Type,
		ObjectName: energyPathVRFField(object, 0), ObjectIndex: object.Index, DisplayName: energyPathVRFField(object, 0)}
}

func (index energyPathVRFOwnershipIndex) unique(objectType, name string) (idf.Object, int, bool) {
	positions := index.objects[energyPathDirectHVACComponentKey(objectType, name)]
	if strings.TrimSpace(name) == "" || len(positions) != 1 {
		return idf.Object{}, -1, false
	}
	return index.doc.Objects[positions[0]], positions[0], true
}

func energyPathVRFZero(value string) bool {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return err == nil && number == 0
}

func energyPathVRFNonnegative(value string) bool {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) && number >= 0
}

// This first native contract is intentionally limited to the reviewed electric,
// air-cooled non-heat-recovery VRF and CV/draw-through terminals without a
// supplemental heater. It does not guess another native variant's meter roster.
func energyPathVRFSystems(doc idf.Document) []energyPathVRFSystem {
	hasNative := false
	for _, object := range doc.Objects {
		if strings.EqualFold(strings.TrimSpace(object.Type), energyPathVRFOutdoorType) {
			hasNative = true
			break
		}
	}
	if !hasNative {
		return nil
	} // No HVAC analysis or new work on other fixtures.
	index := energyPathVRFOwnershipIndex{doc: doc, objects: map[string][]int{}, typedRefs: map[string][]int{},
		tuLists: map[string][]int{}, listOwners: map[string][]int{}, outdoorNames: map[string]int{}}
	for position, object := range doc.Objects {
		name := energyPathVRFField(object, 0)
		key := energyPathDirectHVACComponentKey(object.Type, name)
		index.objects[key] = append(index.objects[key], position)
		for field := 0; field+1 < len(object.Fields); field++ {
			key := energyPathDirectHVACComponentKey(energyPathVRFField(object, field), energyPathVRFField(object, field+1))
			index.typedRefs[key] = append(index.typedRefs[key], position)
		}
		if strings.EqualFold(object.Type, "ZoneTerminalUnitList") {
			for field := 1; field < len(object.Fields); field++ {
				key := energyPathVRFName(energyPathVRFField(object, field))
				index.tuLists[key] = append(index.tuLists[key], position)
			}
		}
		if strings.HasPrefix(energyPathVRFName(object.Type), energyPathVRFName(energyPathVRFOutdoorType)) {
			index.outdoorNames[energyPathVRFName(name)]++
			// Count even unsupported/disconnected outdoor variants. Their list
			// field positions differ, so match only names of actual list objects
			// in the second pass instead of treating a guessed index as ownership.
		}
	}
	for position, object := range doc.Objects {
		if len(object.Fields) == 0 || !strings.HasPrefix(energyPathVRFName(object.Type), energyPathVRFName(energyPathVRFOutdoorType)) {
			continue
		}
		for _, field := range object.Fields[1:] {
			name := strings.TrimSpace(field.Value)
			if len(index.objects[energyPathDirectHVACComponentKey("ZoneTerminalUnitList", name)]) > 0 {
				key := energyPathVRFName(name)
				index.listOwners[key] = append(index.listOwners[key], position)
			}
		}
	}
	var systems []energyPathVRFSystem
	for position, outdoor := range doc.Objects {
		if !strings.EqualFold(strings.TrimSpace(outdoor.Type), energyPathVRFOutdoorType) || len(outdoor.Fields) < 67 ||
			energyPathVRFField(outdoor, 0) == "" ||
			!strings.EqualFold(energyPathVRFField(outdoor, 37), "No") || !strings.EqualFold(energyPathVRFField(outdoor, 55), "AirCooled") ||
			!strings.EqualFold(energyPathVRFField(outdoor, 66), "Electricity") || !strings.EqualFold(energyPathVRFField(outdoor, 49), "Resistive") ||
			index.outdoorNames[energyPathVRFName(energyPathVRFField(outdoor, 0))] != 1 {
			continue
		}
		listName := energyPathVRFField(outdoor, 36)
		list, listPosition, ok := index.unique("ZoneTerminalUnitList", listName)
		owners := index.listOwners[energyPathVRFName(listName)]
		if !ok || len(list.Fields) < 2 || len(owners) != 1 || owners[0] != position {
			continue
		}
		system := energyPathVRFSystem{OutdoorUnit: energyPathVRFComponentRef(outdoor), TerminalUnitList: energyPathVRFComponentRef(list)}
		valid, zones := true, map[string]bool{}
		for field := 1; field < len(list.Fields); field++ {
			name := energyPathVRFField(list, field)
			membership := index.tuLists[energyPathVRFName(name)]
			terminal, ok := index.terminal(name)
			if len(membership) != 1 || membership[0] != listPosition || !ok || zones[energyPathVRFName(terminal.ZoneName)] {
				valid = false
				break
			}
			zones[energyPathVRFName(terminal.ZoneName)] = true
			system.Terminals = append(system.Terminals, terminal)
		}
		if !valid || len(system.Terminals) == 0 {
			continue
		}
		sort.Slice(system.Terminals, func(i, j int) bool {
			return energyPathVRFName(system.Terminals[i].ZoneName) < energyPathVRFName(system.Terminals[j].ZoneName)
		})
		for _, definition := range energyPathVRFConsumptionDefinitions() {
			if definition.Shared {
				system.Targets = append(system.Targets, energyPathVRFConsumptionTarget{Definition: definition, KeyValue: system.OutdoorUnit.ObjectName, ObjectIndex: outdoor.Index})
				continue
			}
			for _, terminal := range system.Terminals {
				system.Targets = append(system.Targets, energyPathVRFConsumptionTarget{Definition: definition,
					KeyValue: terminal.Terminal.ObjectName, ZoneName: terminal.ZoneName, ObjectIndex: terminal.Terminal.ObjectIndex})
			}
		}
		systems = append(systems, system)
	}
	sort.Slice(systems, func(i, j int) bool {
		return energyPathVRFName(systems[i].OutdoorUnit.ObjectName) < energyPathVRFName(systems[j].OutdoorUnit.ObjectName)
	})
	return systems
}

func (index energyPathVRFOwnershipIndex) terminal(name string) (energyPathVRFTerminal, bool) {
	var none energyPathVRFTerminal
	terminal, position, ok := index.unique(energyPathVRFTerminalType, name)
	if !ok || len(terminal.Fields) < 23 || !strings.EqualFold(energyPathVRFField(terminal, 12), "DrawThrough") ||
		!strings.EqualFold(energyPathVRFField(terminal, 13), "Fan:ConstantVolume") ||
		!strings.EqualFold(energyPathVRFField(terminal, 17), energyPathVRFCoolingType) ||
		!strings.EqualFold(energyPathVRFField(terminal, 19), energyPathVRFHeatingType) ||
		energyPathVRFField(terminal, 15) != "" || energyPathVRFField(terminal, 16) != "" ||
		energyPathVRFField(terminal, 26) != "" || energyPathVRFField(terminal, 27) != "" ||
		!energyPathVRFZero(energyPathVRFField(terminal, 8)) || !energyPathVRFZero(energyPathVRFField(terminal, 9)) || !energyPathVRFZero(energyPathVRFField(terminal, 10)) ||
		!energyPathVRFNonnegative(energyPathVRFField(terminal, 21)) || !energyPathVRFNonnegative(energyPathVRFField(terminal, 22)) {
		return none, false
	}
	components := make([]idf.Object, 0, 3)
	for _, pair := range []struct {
		kind  string
		field int
	}{{energyPathVRFCoolingType, 18}, {energyPathVRFHeatingType, 20}, {"Fan:ConstantVolume", 14}} {
		component, _, found := index.unique(pair.kind, energyPathVRFField(terminal, pair.field))
		refs := index.typedRefs[energyPathDirectHVACComponentKey(pair.kind, energyPathVRFField(terminal, pair.field))]
		if !found || len(refs) != 1 || refs[0] != position {
			return none, false
		}
		components = append(components, component)
	}
	cooling, heating, fan := components[0], components[1], components[2]
	if !energyPathVRFSameNode(energyPathVRFField(terminal, 2), energyPathVRFField(cooling, 7)) ||
		!energyPathVRFSameNode(energyPathVRFField(cooling, 8), energyPathVRFField(heating, 4)) ||
		!energyPathVRFSameNode(energyPathVRFField(heating, 5), energyPathVRFField(fan, 7)) ||
		!energyPathVRFSameNode(energyPathVRFField(fan, 8), energyPathVRFField(terminal, 3)) {
		return none, false
	}
	listPosition, mixerPosition := -1, -1
	for _, reference := range index.typedRefs[energyPathDirectHVACComponentKey(energyPathVRFTerminalType, name)] {
		switch energyPathVRFName(index.doc.Objects[reference].Type) {
		case "zonehvac:equipmentlist":
			if listPosition >= 0 {
				return none, false
			}
			listPosition = reference
		case "airterminal:singleduct:mixer":
			if mixerPosition >= 0 {
				return none, false
			}
			mixerPosition = reference
		default:
			return none, false
		}
	}
	// Only the reviewed, explicitly connected DOAS-mixer variant is supported.
	if listPosition < 0 || mixerPosition < 0 {
		return none, false
	}
	list, mixer := index.doc.Objects[listPosition], index.doc.Objects[mixerPosition]
	if !energyPathVRFEquipmentListMember(list, terminal.Type, name) {
		return none, false
	}
	if _, _, ok := index.unique(list.Type, energyPathVRFField(list, 0)); !ok {
		return none, false
	}
	if _, _, ok := index.unique(mixer.Type, energyPathVRFField(mixer, 0)); !ok {
		return none, false
	}
	connection, connectionCount := idf.Object{}, 0
	for _, object := range index.doc.Objects {
		if strings.EqualFold(object.Type, "ZoneHVAC:EquipmentConnections") && strings.EqualFold(energyPathVRFField(object, 1), energyPathVRFField(list, 0)) {
			connection, connectionCount = object, connectionCount+1
		}
	}
	zone, _, zoneOK := index.unique("Zone", energyPathVRFField(connection, 0))
	if connectionCount != 1 || !zoneOK || len(connection.Fields) < 4 {
		return none, false
	}
	zoneConnections := 0
	for _, object := range index.doc.Objects {
		if strings.EqualFold(object.Type, "ZoneHVAC:EquipmentConnections") && strings.EqualFold(energyPathVRFField(object, 0), energyPathVRFField(zone, 0)) {
			zoneConnections++
		}
	}
	if zoneConnections != 1 || !index.mixer(terminal, mixer, connection, listPosition) {
		return none, false
	}
	out := energyPathVRFTerminal{ZoneName: energyPathVRFField(zone, 0), Terminal: energyPathVRFComponentRef(terminal),
		CoolingCoil: energyPathVRFComponentRef(cooling), HeatingCoil: energyPathVRFComponentRef(heating), Fan: energyPathVRFComponentRef(fan)}
	out.Terminal.InletNode, out.Terminal.OutletNode = energyPathVRFField(terminal, 2), energyPathVRFField(terminal, 3)
	out.CoolingCoil.InletNode, out.CoolingCoil.OutletNode = energyPathVRFField(cooling, 7), energyPathVRFField(cooling, 8)
	out.HeatingCoil.InletNode, out.HeatingCoil.OutletNode = energyPathVRFField(heating, 4), energyPathVRFField(heating, 5)
	out.Fan.InletNode, out.Fan.OutletNode = energyPathVRFField(fan, 7), energyPathVRFField(fan, 8)
	return out, true
}

func (index energyPathVRFOwnershipIndex) nodeMember(nodeOrList, node string) bool {
	positions := index.objects[energyPathDirectHVACComponentKey("NodeList", nodeOrList)]
	if len(positions) == 0 {
		return energyPathVRFSameNode(nodeOrList, node)
	}
	if len(positions) != 1 {
		return false
	}
	count := 0
	for _, field := range index.doc.Objects[positions[0]].Fields[1:] {
		if energyPathVRFSameNode(field.Value, node) {
			count++
		}
	}
	return count == 1
}

// EnergyPlus 25.1 EquipmentList entries follow Name/Load Distribution Scheme
// in six-field groups. A type/name pair hidden in schedule fields is not an
// equipment reference, even if the conservative global reference index saw it.
func energyPathVRFEquipmentListMember(list idf.Object, objectType, name string) bool {
	count := 0
	for field := 2; field+3 < len(list.Fields); field += 6 {
		if strings.EqualFold(energyPathVRFField(list, field), objectType) && strings.EqualFold(energyPathVRFField(list, field+1), name) {
			count++
		}
	}
	return count == 1
}

func (index energyPathVRFOwnershipIndex) mixer(terminal, mixer, connection idf.Object, listPosition int) bool {
	if len(mixer.Fields) < 7 || !strings.EqualFold(energyPathVRFField(mixer, 1), energyPathVRFTerminalType) ||
		!strings.EqualFold(energyPathVRFField(mixer, 2), energyPathVRFField(terminal, 0)) || energyPathVRFField(mixer, 4) == "" {
		return false
	}
	refs := index.typedRefs[energyPathDirectHVACComponentKey(mixer.Type, energyPathVRFField(mixer, 0))]
	if len(refs) != 1 {
		return false
	}
	adu := index.doc.Objects[refs[0]]
	if !strings.EqualFold(adu.Type, "ZoneHVAC:AirDistributionUnit") ||
		!strings.EqualFold(energyPathVRFField(adu, 2), mixer.Type) || !strings.EqualFold(energyPathVRFField(adu, 3), energyPathVRFField(mixer, 0)) ||
		!energyPathVRFSameNode(energyPathVRFField(adu, 1), energyPathVRFField(mixer, 3)) ||
		!energyPathVRFEquipmentListMember(index.doc.Objects[listPosition], adu.Type, energyPathVRFField(adu, 0)) {
		return false
	}
	if _, _, ok := index.unique(adu.Type, energyPathVRFField(adu, 0)); !ok {
		return false
	}
	aduRefs := index.typedRefs[energyPathDirectHVACComponentKey(adu.Type, energyPathVRFField(adu, 0))]
	if len(aduRefs) != 1 || aduRefs[0] != listPosition {
		return false
	}
	switch energyPathVRFName(energyPathVRFField(mixer, 6)) {
	case "inletside":
		return energyPathVRFSameNode(energyPathVRFField(mixer, 3), energyPathVRFField(terminal, 2)) &&
			index.nodeMember(energyPathVRFField(connection, 2), energyPathVRFField(terminal, 3)) &&
			index.nodeMember(energyPathVRFField(connection, 3), energyPathVRFField(mixer, 5))
	case "supplyside":
		return energyPathVRFSameNode(energyPathVRFField(mixer, 5), energyPathVRFField(terminal, 3)) &&
			index.nodeMember(energyPathVRFField(connection, 2), energyPathVRFField(mixer, 3)) &&
			index.nodeMember(energyPathVRFField(connection, 3), energyPathVRFField(terminal, 2))
	default:
		return false
	}
}

func (builder *purposePlanBuilder) addEnergyPathVRFOutputs() {
	selected, scoped := purposeSelectedZoneSet(builder.request.Scope)
	for _, system := range energyPathVRFSystems(builder.doc) {
		intersects := !scoped
		for _, terminal := range system.Terminals {
			intersects = intersects || selected[normalizePurposeToken(terminal.ZoneName)]
		}
		if !intersects {
			continue
		}
		for _, target := range system.Targets {
			builder.addVariableWithReasonAndScopeZone(SimulationPurposeBasicEnergy, target.KeyValue, target.Definition.Energy.Aliases[0], "Monthly", "medium",
				"Monthly native VRF consumption constituent; preserves complete local and shared outdoor ownership, excluding fan totals.", "Basic Energy Path", target.ZoneName)
		}
		// A selected Zone must not shrink the shared outdoor denominator. These
		// are context measurements, not permission to broaden the displayed scope.
		for _, terminal := range system.Terminals {
			for _, name := range []string{"Zone Air System Sensible Cooling Energy", "Zone Air System Sensible Heating Energy"} {
				builder.addVariableWithReasonAndScopeZone(SimulationPurposeBasicEnergy, terminal.ZoneName, name, "Monthly", "medium",
					"Complete original VRF owner service-load denominator for monthly shared outdoor allocation.", "Basic Energy Path", terminal.ZoneName)
			}
		}
	}
}
