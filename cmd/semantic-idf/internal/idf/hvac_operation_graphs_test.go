package idf

import (
	"encoding/json"
	"strings"
	"testing"
)

func hvacGraphTestNode(t *testing.T, graph HVACControlSequence, id string) HVACControlNode {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("missing graph node %q: %#v", id, graph.Nodes)
	return HVACControlNode{}
}

func hvacGraphTestHasEdge(graph HVACControlSequence, from, to, relation, polarity, condition string) bool {
	for _, edge := range graph.Edges {
		if edge.From == from && edge.To == to && edge.Relation == relation && edge.Polarity == polarity && edge.Condition == condition {
			return true
		}
	}
	return false
}

func TestHVACOperatingGraphsSeparateRawAvailabilityAndOnOffConditions(t *testing.T) {
	loopObj := hvacOperationTestObject(0, "AirLoopHVAC", "Air", "Availability Manager List Name", "Managers")
	fan := hvacOperationTestObject(1, "Fan:OnOff", "Fan", "Availability Schedule Name", "Fan Hours")
	coil := hvacOperationTestObject(2, "Coil:Heating:Electric", "Coil", "Availability Schedule Name", "Coil Hours")
	doc := Document{Objects: []Object{loopObj, fan, coil,
		hvacOperationTestRaw(3, "AvailabilityManagerAssignmentList", "Managers", "AvailabilityManager:ScheduledOn", "Start", "AvailabilityManager:NightCycle", "Night"),
		hvacOperationTestRaw(4, "AvailabilityManager:ScheduledOn", "Start", "Override Hours"),
		hvacOperationTestRaw(5, "AvailabilityManager:NightCycle", "Night", "Applicability", "Fan Hours", "CycleOnAny", "1", "FixedRunTime", "3600"),
		hvacOperationTestRaw(6, "Schedule:Constant", "Fan Hours", "", "0"),
		hvacOperationTestRaw(7, "Schedule:Constant", "Coil Hours", "", "1"),
		hvacOperationTestRaw(8, "Schedule:Constant", "Override Hours", "", "1"),
		hvacOperationTestRaw(9, "Schedule:Constant", "Applicability", "", "0.5"),
	}}
	loop := HVACLoop{ID: "air:a", ObjectIndex: 0, Type: "AirLoopHVAC", SupplySide: HVACLoopSide{Branches: []HVACBranch{{Components: []HVACComponent{hvacOperationTestComponent(fan, "In", "Middle"), hvacOperationTestComponent(coil, "Middle", "Out")}}}}}
	profile := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), []HVACLoop{loop}, nil), 2, loop.ID)
	for _, node := range profile.AvailabilityDefinitions.Nodes {
		if node.Kind == "decision" || node.Code == "availability_permission" || node.Code == "air_managers" || node.Code == "fan_gate" {
			t.Fatalf("algorithm leaked into raw declarations: %#v", node)
		}
	}
	for _, id := range []string{"definition:0", "definition:1", "definition:2", "manager_list:3", "manager:4", "manager:5", "schedule:6", "schedule:7", "schedule:8", "schedule:9"} {
		hvacGraphTestNode(t, profile.AvailabilityDefinitions, id)
	}
	if !hvacGraphTestHasEdge(profile.AvailabilityDefinitions, "manager:5", "schedule:9", "references_schedule", "", "") || !hvacGraphTestHasEdge(profile.AvailabilityDefinitions, "manager:5", "schedule:6", "references_schedule", "", "") {
		t.Fatal("NightCycle raw applicability/fan schedules missing")
	}
	for _, node := range profile.OnOff.Nodes {
		if node.Code == "manager_scheduled_on" && node.Source.ObjectIndex != 4 || node.Code == "manager_unknown" && node.Source.ObjectIndex != 5 {
			t.Fatalf("inherited loop-manager source was replaced by fan/layer owner: %#v", node)
		}
	}
	for _, branch := range []struct{ from, to, relation, condition string }{{"permission", "off", "blocked", "no"}, {"permission", "unknown", "unresolved", "unknown"}, {"permission", "decision", "permits", "yes"}, {"decision", "on", "requested", "yes"}, {"decision", "off", "no_request", "no"}} {
		if !hvacGraphTestHasEdge(profile.OnOff, branch.from, branch.to, branch.relation, "", branch.condition) {
			t.Fatalf("missing on/off branch %#v", branch)
		}
	}
	for _, edge := range profile.OnOff.Edges {
		if strings.Contains(edge.From, "condition:0:") && strings.Contains(edge.To, "condition:0:") && strings.Count(edge.From, ":") == strings.Count(edge.To, ":") {
			t.Fatalf("unordered manager siblings presented as execution order: %#v", edge)
		}
	}
	if len(profile.Control.Nodes) == 0 || profile.FinalAvailability.Op == "" {
		t.Fatal("legacy operating-profile wire was removed")
	}
}

func TestHVACOperatingGraphsPreserveUnresolvedAndNamespaceSpecificDefinitions(t *testing.T) {
	fan := hvacOperationTestObject(0, "Fan:OnOff", "Fan", "Availability Schedule Name", "Hours")
	doc := Document{Objects: []Object{fan,
		hvacOperationTestRaw(1, "Schedule:Day:Hourly", "Hours", "", "0"),
		hvacOperationTestRaw(2, "Schedule:Constant", "Hours", "", "1"),
		hvacOperationTestRaw(4, "ExternalInterface:FunctionalMockupUnitImport:To:Variable", "Hours", "unused"),
	}}
	ctx := newHVACContext(doc)
	b := hvacOperatingBuilder{ctx: ctx, objects: map[int]Object{0: fan, 1: doc.Objects[1], 2: doc.Objects[2]}}
	profile := b.profile(hvacOperatingOccurrence{object: fan, ambiguous: true})
	hvacGraphTestNode(t, profile.AvailabilityDefinitions, "definition:unresolved_parent")
	if hvacGraphTestNode(t, profile.AvailabilityDefinitions, "schedule:2").Source.ObjectIndex != 2 {
		t.Fatal("annual availability matched same-named day schedule")
	}
	owner := hvacGraphTestNode(t, profile.AvailabilityDefinitions, "definition:0")
	if len(owner.Parameters) != 1 || owner.Parameters[0].FieldIndex != 1 || owner.Parameters[0].Value != "Hours" {
		t.Fatalf("owner declaration field evidence attached to wrong node: %#v", owner)
	}
	doc.Objects = append(doc.Objects, hvacOperationTestRaw(3, "Schedule:Constant", "Hours", "", "0"))
	profile = hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), nil, nil), 0, "")
	unknown := hvacGraphTestNode(t, profile.AvailabilityDefinitions, "schedule_missing:annual:hours")
	if unknown.Kind != "unknown" {
		t.Fatal("duplicate schedules gained an arbitrary definition")
	}
	if strings.Contains(string(mustHVACGraphJSON(t, profile.AvailabilityDefinitions)), "schedule:2") {
		t.Fatal("ambiguous schedule's first source leaked into definition graph")
	}
}

func TestHVACOperatingParameterGraphsPreserveThermostatTargetsAndPerformanceSources(t *testing.T) {
	unit := hvacOperationTestObject(0, "ZoneHVAC:IdealLoadsAirSystem", "Unit")
	doc := Document{Objects: []Object{unit,
		hvacOperationTestRaw(1, "ZoneControl:Thermostat", "Room Thermostat", "Room", "Mode", "ThermostatSetpoint:DualSetpoint", "Targets"),
		hvacOperationTestRaw(2, "ThermostatSetpoint:DualSetpoint", "Targets", "Heating", "Cooling"),
		hvacOperationTestRaw(3, "Schedule:Constant", "Mode", "", "4"),
		hvacOperationTestRaw(6, "Schedule:Day:Hourly", "Heating", "", "0"),
		hvacOperationTestRaw(4, "Schedule:Constant", "Heating", "", "20"),
		hvacOperationTestRaw(5, "Schedule:Constant", "Cooling", "", "26"),
	}}
	relations := []HVACZoneChain{{ZoneName: "Room", ZoneEquipment: []HVACComponent{hvacOperationTestComponent(unit, "", "")}}}
	profile := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), nil, relations), 0, "")
	graph := profile.ParameterControl
	for _, id := range []string{"schedule:3", "schedule:4", "schedule:5", "thermostat_setpoint:2"} {
		hvacGraphTestNode(t, graph, id)
		hvacGraphTestNode(t, profile.OnOff, id)
	}
	if !hvacGraphTestHasEdge(graph, "schedule:3", "thermostat:1", "selects_mode", "", "") || !hvacGraphTestHasEdge(graph, "schedule:4", "thermostat_setpoint:2", "sets_parameter", "", "") || !hvacGraphTestHasEdge(graph, "schedule:5", "thermostat_setpoint:2", "sets_parameter", "", "") {
		t.Fatal("thermostat mode/heating/cooling schedule source chain incomplete")
	}
	boiler := hvacOperationTestObject(0, "Boiler:HotWater", "Boiler", "Normalized Boiler Efficiency Curve Name", "Efficiency", "Nominal Capacity", "100000")
	curve := hvacOperationTestRaw(1, "Curve:Quadratic", "Efficiency", "1", "0", "0", "0", "1")
	profile = hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(Document{Objects: []Object{boiler, curve}}), nil, nil), 0, "")
	if !hvacGraphTestHasEdge(profile.ParameterControl, "curve:1", "equipment", "modifies_performance", "", "") || hvacGraphTestHasEdge(profile.ParameterControl, "curve:1", "modulation", "modifies_performance", "", "") {
		t.Fatal("efficiency curve incorrectly changes demand command rather than model response")
	}
	if !strings.Contains(string(mustHVACGraphJSON(t, profile.ParameterControl)), "100000") {
		t.Fatal("major source equipment design parameter disappeared")
	}
	doc.Objects = append(doc.Objects, hvacOperationTestRaw(7, "Schedule:Constant", "Heating", "", "21"))
	profile = hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), nil, relations), 0, "")
	hvacGraphTestNode(t, profile.OnOff, "schedule_missing:annual:heating")
	hvacGraphTestNode(t, profile.ParameterControl, "schedule_missing:annual:heating")
}

func TestHVACOperatingParameterReferenceChainsRespectTargetQuantities(t *testing.T) {
	coil := hvacOperationTestObject(0, "Coil:Cooling:Water", "Coil", "Water Inlet Node Name", "Water In", "Water Outlet Node Name", "Water Out", "Air Inlet Node Name", "Air In", "Air Outlet Node Name", "Air Out")
	doc := Document{Objects: []Object{coil,
		hvacOperationTestObject(1, "SetpointManager:MixedAir", "Mixed", "Control Variable", "Temperature", "Reference Setpoint Node Name", "Reference", "Fan Inlet Node Name", "Fan In", "Fan Outlet Node Name", "Fan Out", "Setpoint Node or NodeList Name", "Air Out"),
		hvacOperationTestObject(2, "SetpointManager:Scheduled", "Temperature", "Control Variable", "Temperature", "Schedule Name", "Temp", "Setpoint Node or NodeList Name", "Reference"),
		hvacOperationTestObject(3, "SetpointManager:Scheduled", "Humidity", "Control Variable", "MaximumHumidityRatio", "Schedule Name", "Humid", "Setpoint Node or NodeList Name", "Reference"),
	}}
	profile := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), nil, nil), 0, "")
	if !hvacGraphTestHasEdge(profile.ParameterControl, "setpoint_value:2:reference", "setpoint:1", "references_setpoint", "", "") || hvacGraphTestHasEdge(profile.ParameterControl, "setpoint_value:3:reference", "setpoint:1", "references_setpoint", "", "") {
		t.Fatal("MixedAir temperature reference took the humidity target at the same node")
	}
}

func mustHVACGraphJSON(t *testing.T, graph HVACControlSequence) []byte {
	t.Helper()
	value, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestHVACOperatingParameterGraphUsesExactActuatorAndNativeResidual(t *testing.T) {
	coil := hvacOperationTestObject(0, "Coil:Cooling:Water", "Coil", "Water Inlet Node Name", "Water In", "Water Outlet Node Name", "Water Out", "Air Inlet Node Name", "Air In", "Air Outlet Node Name", "Air Out")
	controller := hvacOperationTestObject(1, "Controller:WaterCoil", "Controller", "Sensor Node Name", "Air Out", "Actuator Node Name", "Water In", "Control Variable", "Temperature", "Actuator Variable", "Flow", "Action", "Reverse")
	doc := Document{Objects: []Object{coil, controller,
		hvacOperationTestObject(2, "SetpointManager:Scheduled", "Temperature target", "Control Variable", "Temperature", "Schedule Name", "Temp", "Setpoint Node or NodeList Name", "Air Out"),
		hvacOperationTestObject(3, "SetpointManager:Scheduled", "Humidity target", "Control Variable", "MaximumHumidityRatio", "Schedule Name", "Humidity", "Setpoint Node or NodeList Name", "Air Out"),
		hvacOperationTestRaw(4, "Schedule:Constant", "Temp", "", "12"),
		hvacOperationTestRaw(5, "Schedule:Constant", "Humidity", "", "0.008"),
	}}
	profile := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), nil, nil), 0, "")
	graph := profile.ParameterControl
	if hvacGraphTestNode(t, graph, "controller:1:command").Kind != "control_variable" || hvacGraphTestNode(t, graph, "state").Kind != "state_variable" || hvacGraphTestNode(t, graph, "controller:1:residual").Kind != "comparison" || hvacGraphTestNode(t, graph, "setpoint_value:2:air out").Kind != "setpoint" {
		t.Fatal("manipulated command, physical state and reference/comparison roles collapsed")
	}
	actuators := 0
	for _, edge := range graph.Edges {
		if edge.To == "equipment" && edge.Relation == "actuates" {
			actuators++
			if edge.From != "controller:1:command" {
				t.Fatalf("generic command competes with exact water-flow actuator: %#v", edge)
			}
		}
		if edge.From == "setpoint_value:3:air out" && edge.To == "controller:1:residual" {
			t.Fatal("humidity manager was subtracted from a Temperature residual")
		}
		if edge.Polarity != "" && edge.Polarity != "unknown" && edge.To != "controller:1:residual" {
			t.Fatalf("Action was misread as controller/plant feedback gain: %#v", edge)
		}
	}
	if actuators != 1 || !hvacGraphTestHasEdge(graph, "port:air out", "controller:1:residual", "adds_measurement", "positive", "") || !hvacGraphTestHasEdge(graph, "setpoint_value:2:air out", "controller:1:residual", "subtracts_target", "negative", "") {
		t.Fatalf("exact actuator/native residual incomplete: %#v", graph.Edges)
	}
	for _, id := range []string{"port:air in", "port:water in", "port:air out", "port:water out", "output"} {
		if hvacGraphTestNode(t, graph, id).Boundary == "" {
			t.Fatalf("external I/O boundary missing from %s", id)
		}
	}
	// No provider is not a zero setpoint and does not erase the target input.
	doc.Objects = doc.Objects[:2]
	controller.Fields[2].Value = "UnsupportedAction"
	profile = hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), nil, nil), 0, "")
	if hvacGraphTestNode(t, profile.ParameterControl, "controller:1:target").Code != "unresolved_node_setpoint" || !hvacGraphTestHasEdge(profile.ParameterControl, "controller:1:target", "controller:1:residual", "subtracts_target", "negative", "") {
		t.Fatal("unknown setpoint provider became an implied zero/missing input")
	}
	if !strings.Contains(string(mustHVACGraphJSON(t, profile.ParameterControl)), "UnsupportedAction") {
		t.Fatal("unsupported raw controller Action was discarded")
	}
}

func TestHVACOperatingParameterGraphPreservesSharedCircuitAndBlankPortEvidence(t *testing.T) {
	chiller := hvacOperationTestObject(0, "Chiller:Electric:EIR", "Shared", "Chilled Water Inlet Node Name", "Evap In", "Chilled Water Outlet Node Name", "Evap Out", "Condenser Inlet Node Name", "Cond In", "Condenser Outlet Node Name", "Cond Out")
	doc := Document{Objects: []Object{chiller,
		hvacOperationTestObject(1, "PlantLoop", "CHW"), hvacOperationTestObject(2, "PlantLoop", "Condenser"),
		hvacOperationTestObject(3, "SetpointManager:Scheduled", "Cold", "Control Variable", "Temperature", "Schedule Name", "Cold Schedule", "Setpoint Node or NodeList Name", "Evap Out"),
		hvacOperationTestObject(4, "SetpointManager:Scheduled", "Warm", "Control Variable", "Temperature", "Schedule Name", "Warm Schedule", "Setpoint Node or NodeList Name", "Cond Out"),
	}}
	loops := []HVACLoop{
		{ID: "plant:chw", ObjectIndex: 1, Type: "PlantLoop", SupplySide: HVACLoopSide{Branches: []HVACBranch{{Components: []HVACComponent{hvacOperationTestComponent(chiller, "Evap In", "Evap Out")}}}}},
		{ID: "plant:cond", ObjectIndex: 2, Type: "PlantLoop", DemandSide: HVACLoopSide{Branches: []HVACBranch{{Components: []HVACComponent{hvacOperationTestComponent(chiller, "Cond In", "Cond Out")}}}}},
	}
	profiles := buildHVACOperatingProfiles(newHVACContext(doc), loops, nil)
	for _, test := range []struct{ loop, expected, forbidden string }{{"plant:chw", "Evap Out", "Cond Out"}, {"plant:cond", "Cond Out", "Evap Out"}} {
		graph := hvacOperationTestProfile(t, profiles, 0, test.loop).ParameterControl
		encoded := string(mustHVACGraphJSON(t, graph))
		if !strings.Contains(encoded, test.expected) || strings.Contains(encoded, test.forbidden) {
			t.Fatalf("shared equipment circuits merged in %s: %s", test.loop, encoded)
		}
	}
	// Unresolved circuit identity cannot merge every circuit's otherwise valid ports.
	loops[0].SupplySide.Branches[0].Components[0].InletNode = ""
	loops[0].SupplySide.Branches[0].Components[0].OutletNode = ""
	profile := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), loops[:1], nil), 0, loops[0].ID)
	hvacGraphTestNode(t, profile.ParameterControl, "port:unresolved")
	encoded := string(mustHVACGraphJSON(t, profile.ParameterControl))
	if strings.Contains(encoded, "Evap Out") || strings.Contains(encoded, "Cond Out") {
		t.Fatalf("unresolved circuit borrowed another circuit's ports/setpoints: %s", encoded)
	}
}

func TestHVACOperatingGraphsRetainContinuousPumpAndPassiveSemantics(t *testing.T) {
	for _, mode := range []string{"Continuous", "Intermittent", ""} {
		pump := hvacOperationTestObject(0, "Pump:VariableSpeed", "Pump", "Pump Control Type", mode, "Pump Flow Rate Schedule Name", "Rate")
		doc := Document{Objects: []Object{pump, hvacOperationTestRaw(1, "Schedule:Constant", "Rate", "", "0")}}
		profile := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), nil, nil), 0, "")
		if hvacGraphTestHasEdge(profile.OnOff, "decision", "off", "no_request", "", "no") != (mode == "Intermittent") {
			t.Fatalf("continuous/default pump falsely gains no-request OFF for %q", mode)
		}
		if strings.Contains(string(mustHVACGraphJSON(t, profile.AvailabilityDefinitions)), "Rate") {
			t.Fatal("pump flow control schedule became availability")
		}
		hvacGraphTestNode(t, profile.ParameterControl, "schedule:1")
	}
	pipe := hvacOperationTestRaw(0, "Pipe:Adiabatic", "Pipe", "In", "Out")
	profile := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(Document{Objects: []Object{pipe}}), nil, nil), 0, "")
	for _, node := range profile.OnOff.Nodes {
		if node.Code == "on_command" || node.Code == "off_command" {
			t.Fatal("passive pipe gained an invented actuator")
		}
	}
	hvacGraphTestNode(t, profile.OnOff, "passive")
}

func TestHVACOperatingNestedParameterPortsFollowTypedAirOccurrence(t *testing.T) {
	wrapper := hvacOperationTestObject(0, "AirLoopHVAC:UnitarySystem", "Unit", "Heating Coil Object Type", "Coil:Heating:Water", "Heating Coil Name", "Coil")
	coil := hvacOperationTestObject(1, "Coil:Heating:Water", "Coil", "Water Inlet Node Name", "Water In", "Water Outlet Node Name", "Water Out", "Air Inlet Node Name", "Air In", "Air Outlet Node Name", "Air Out")
	air := hvacOperationTestObject(2, "AirLoopHVAC", "Air")
	doc := Document{Objects: []Object{wrapper, coil, air}}
	loop := HVACLoop{ID: "air:a", ObjectIndex: 2, Type: "AirLoopHVAC", SupplySide: HVACLoopSide{Branches: []HVACBranch{{Components: []HVACComponent{hvacOperationTestComponent(wrapper, "Unit In", "Unit Out")}}}}}
	profile := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), []HVACLoop{loop}, nil), 1, loop.ID)
	for _, id := range []string{"port:air in", "port:air out", "port:water in", "port:water out"} {
		hvacGraphTestNode(t, profile.ParameterControl, id)
	}
	for _, node := range profile.ParameterControl.Nodes {
		if node.Code == "unresolved_circuit_ports" {
			t.Fatal("typed wrapper's nested air ports became unresolved merely because coil is not a Branch entry")
		}
	}
}

func TestHVACOperatingWaterCoilTargetsPreserveMissingHumidityChannels(t *testing.T) {
	for _, test := range []struct {
		name, variable string
		quantities     []string
		missing        []string
	}{
		{"humidity_missing", "HumidityRatio", nil, []string{"humidityratio"}},
		{"humidity_wrong_temperature", "HumidityRatio", []string{"Temperature"}, []string{"humidityratio"}},
		{"humidity_resolved", "HumidityRatio", []string{"MaximumHumidityRatio"}, nil},
		{"humidity_direct_resolved", "HumidityRatio", []string{"HumidityRatio"}, nil},
		{"combined_missing", "TemperatureAndHumidityRatio", nil, []string{"temperature", "maximumhumidityratio"}},
		{"combined_temperature_only", "TemperatureAndHumidityRatio", []string{"Temperature"}, []string{"maximumhumidityratio"}},
		{"combined_humidity_only", "TemperatureAndHumidityRatio", []string{"MaximumHumidityRatio"}, []string{"temperature"}},
		{"combined_resolved", "TemperatureAndHumidityRatio", []string{"Temperature", "MaximumHumidityRatio"}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			coil := hvacOperationTestObject(0, "Coil:Cooling:Water", "Coil", "Water Inlet Node Name", "Water In", "Water Outlet Node Name", "Water Out", "Air Inlet Node Name", "Air In", "Air Outlet Node Name", "Air Out")
			controller := hvacOperationTestObject(1, "Controller:WaterCoil", "Controller", "Sensor Node Name", "Air Out", "Actuator Node Name", "Water In", "Control Variable", test.variable, "Actuator Variable", "Flow", "Action", "Reverse")
			doc := Document{Objects: []Object{coil, controller}}
			for index, quantity := range test.quantities {
				doc.Objects = append(doc.Objects, hvacOperationTestObject(index+2, "SetpointManager:Scheduled", quantity+" target", "Control Variable", quantity, "Schedule Name", quantity+" schedule", "Setpoint Node or NodeList Name", "Air Out"))
			}
			graph := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), nil, nil), 0, "").ParameterControl
			missing := map[string]bool{}
			for _, quantity := range test.missing {
				missing["controller:1:target:"+quantity] = true
			}
			unresolved := 0
			for _, node := range graph.Nodes {
				if node.Code != "unresolved_node_setpoint" {
					continue
				}
				unresolved++
				if !missing[node.ID] || node.Kind != "external_input" || node.Boundary != "input" || node.Source.ObjectIndex != 1 {
					t.Fatalf("missing/known target channels merged or provider fabricated: %#v", node)
				}
				if !hvacGraphTestHasEdge(graph, node.ID, "controller:1", "sets_target", "unknown", "") {
					t.Fatalf("unsupported comparison gained a sign or lost explicit target: %#v", node)
				}
				for _, parameter := range node.Parameters {
					if parameter.Name == "Control Variable" && (parameter.Value != test.variable || parameter.FieldIndex != 1) {
						t.Fatalf("raw controller field evidence was rewritten to a derived channel: %#v", parameter)
					}
				}
			}
			if unresolved != len(missing) {
				t.Fatalf("unresolved target count=%d, want %d (%#v)", unresolved, len(missing), graph.Nodes)
			}
			for _, edge := range graph.Edges {
				if edge.Polarity == "positive" || edge.Polarity == "negative" {
					t.Fatalf("unsupported humidity/combined comparison gained inferred polarity: %#v", edge)
				}
			}
		})
	}
}

func TestHVACOperatingImplicitHumidityManagersPreserveQuantityAndNodeEvidence(t *testing.T) {
	for _, manager := range []Object{
		hvacOperationTestRaw(3, "SetpointManager:SingleZone:Humidity:Maximum", "Humidity", "Air Out", "Room Air"),
		hvacOperationTestRaw(3, "SetpointManager:MultiZone:MaximumHumidity:Average", "Humidity", "Air Loop", "0.008", "0.015", "Air Out"),
		hvacOperationTestRaw(3, "SetpointManager:MultiZone:Humidity:Maximum", "Humidity", "Air Loop", "0.008", "0.015", "Air Out"),
	} {
		t.Run(manager.Type, func(t *testing.T) {
			coil := hvacOperationTestObject(0, "Coil:Cooling:Water", "Coil", "Water Inlet Node Name", "Water In", "Water Outlet Node Name", "Water Out", "Air Inlet Node Name", "Air In", "Air Outlet Node Name", "Air Out")
			controller := hvacOperationTestObject(1, "Controller:WaterCoil", "Controller", "Sensor Node Name", "Air Out", "Actuator Node Name", "Water In", "Control Variable", "TemperatureAndHumidityRatio", "Actuator Variable", "Flow")
			temperature := hvacOperationTestObject(2, "SetpointManager:Scheduled", "Temperature", "Control Variable", "Temperature", "Schedule Name", "Temp", "Setpoint Node or NodeList Name", "Air Out")
			doc := Document{Objects: []Object{coil, controller, temperature, manager}}
			graph := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), nil, nil), 0, "").ParameterControl
			if !hvacGraphTestHasEdge(graph, "setpoint_value:2:air out", "controller:1", "sets_target", "", "") || !hvacGraphTestHasEdge(graph, "setpoint_value:3:air out", "controller:1", "sets_target", "", "") {
				t.Fatalf("implicit humidity manager target was not connected: %#v", graph.Edges)
			}
			for _, node := range graph.Nodes {
				if node.Code == "unresolved_node_setpoint" {
					t.Fatalf("known implicit humidity manager became an unknown provider: %#v", node)
				}
			}
			quantity := hvacOperatingSetpointQuantity(manager)
			if quantity.Value != "MaximumHumidityRatio" || quantity.FieldIndex != -1 {
				t.Fatalf("implicit type quantity misrepresented as a literal field: %#v", quantity)
			}
			// The valid manager must match the exact sensed node, not merely
			// its specialized type or a similar node name.
			for index := range manager.Fields {
				if manager.Fields[index].Value == "Air Out" {
					manager.Fields[index].Value = "Air Out Similar"
				}
			}
			graph = hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), nil, nil), 0, "").ParameterControl
			hvacGraphTestNode(t, graph, "controller:1:target:maximumhumidityratio")
			if hvacGraphTestHasEdge(graph, "setpoint_value:3:air out similar", "controller:1", "sets_target", "", "") {
				t.Fatal("similar node satisfied a missing humidity channel")
			}
		})
	}
	minimum := hvacOperationTestRaw(3, "SetpointManager:SingleZone:Humidity:Minimum", "Minimum", "Air Out", "Room Air")
	if hvacOperatingSetpointQuantity(minimum).Value != "MinimumHumidityRatio" || hvacOperatingControlQuantityMatches("TemperatureAndHumidityRatio", "MinimumHumidityRatio") {
		t.Fatal("humidifying minimum target was treated as dehumidifying maximum target")
	}
}

func TestHVACOperatingOnOffExpressionsKeepStatusRulesAndOpaqueActivation(t *testing.T) {
	manager := hvacOperationTestRaw(3, "AvailabilityManager:NightCycle", "Night", "App", "Fan Hours", "CycleOnAny", "1", "FixedRunTime", "1800")
	profile := hvacConfiguredTestNightCycleProfile(t, manager, nil, "AirLoopHVAC")
	expression := profile.OnOffExpression
	if expression.Op != "all" || len(expression.Children) != 2 || expression.Children[1].Op != "runtime_condition" || expression.Children[1].ReferenceID != "decision" {
		t.Fatalf("on/off expression lost the permission/activation distinction: %#v", expression)
	}
	if hvacOperationExpressionJSON(t, expression.Children[0]) != hvacOperationExpressionJSON(t, profile.FinalAvailability) {
		t.Fatal("compact on/off expression changed native manager status priority or fan override")
	}
	if !hvacConfiguredTestHasReason(expression.Children[0], "runtime_manager") {
		t.Fatal("full on/off expression accidentally reused minimum configured permission")
	}
	activation := hvacGraphTestNode(t, profile.OnOff, expression.Children[1].ReferenceID)
	if activation.Code != "fan_on_off" || expression.Children[1].Source == nil || expression.Children[1].Source.ObjectIndex != activation.Source.ObjectIndex || len(expression.Children[1].Children) != 0 {
		t.Fatal("activation dependency was reinterpreted as a fabricated Boolean demand formula")
	}
	for _, mode := range []string{"Continuous", "Intermittent", ""} {
		pump := hvacOperationTestObject(0, "Pump:VariableSpeed", "Pump", "Pump Control Type", mode)
		profile = hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(Document{Objects: []Object{pump}}), nil, nil), 0, "")
		activation := profile.OnOffExpression.Children[1]
		if activation.Op != "runtime_condition" || len(activation.Children) != 0 || activation.Value != nil || activation.ScheduleName != "" {
			t.Fatalf("pump %q gained a demand>0 or zero-request OFF condition: %#v", mode, activation)
		}
		hvacGraphTestNode(t, profile.OnOff, activation.ReferenceID)
	}
	pipe := hvacOperationTestRaw(0, "Pipe:Adiabatic", "Pipe", "In", "Out")
	profile = hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(Document{Objects: []Object{pipe}}), nil, nil), 0, "")
	if profile.OnOffExpression.Op != "passive_flow" || profile.OnOffExpression.ReferenceID != "flow" || len(profile.OnOffExpression.Children) != 0 {
		t.Fatal("passive pipe gained an active ON/OFF Boolean formula")
	}
	hvacGraphTestNode(t, profile.OnOff, profile.OnOffExpression.ReferenceID)
	unsupported := hvacOperationTestRaw(0, "PlantComponent:UserDefined", "Unsupported")
	profile = hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(Document{Objects: []Object{unsupported}}), nil, nil), 0, "")
	if profile.OnOffExpression.Op != "all" || profile.OnOffExpression.Children[1].Op != "unknown" || profile.OnOffExpression.Children[1].Reason != "unresolved_control" {
		t.Fatal("unknown equipment family gained an invented control algorithm")
	}
	hvacGraphTestNode(t, profile.OnOff, profile.OnOffExpression.Children[1].ReferenceID)
}
