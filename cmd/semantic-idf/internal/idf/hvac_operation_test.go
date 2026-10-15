package idf

import (
	"encoding/json"
	"strings"
	"testing"
)

func hvacOperationTestObject(index int, objectType, name string, values ...string) Object {
	obj := Object{Index: index, Type: objectType, Fields: []Field{{Value: name}}}
	for offset := 0; offset+1 < len(values); offset += 2 {
		fieldName, value := values[offset], values[offset+1]
		fieldIndex := -1
		if spec, exists := objectFieldCatalog[normalizeFieldCatalogKey(objectType)]; exists {
			for _, field := range spec.Fields {
				if strings.EqualFold(field.Name, fieldName) {
					fieldIndex = field.Index
					break
				}
			}
			if fieldIndex < 0 {
				fieldIndex = len(spec.Fields)
			}
		}
		if strings.EqualFold(fieldName, "Availability Manager List Name") {
			if actual, exists := hvacOperatingManagerFields[normalizeName(objectType)]; exists {
				fieldIndex = actual
			}
		}
		for actual, name := range hvacOperatingControlFieldNames[normalizeName(objectType)] {
			if strings.EqualFold(fieldName, name) {
				fieldIndex = actual
				break
			}
		}
		if fieldIndex < 0 {
			fieldIndex = len(obj.Fields)
		}
		for fieldIndex < len(obj.Fields) && obj.Fields[fieldIndex].Value != "" && !strings.EqualFold(obj.Fields[fieldIndex].Comment, fieldName) {
			fieldIndex++
		}
		for len(obj.Fields) <= fieldIndex {
			obj.Fields = append(obj.Fields, Field{})
		}
		obj.Fields[fieldIndex] = Field{Value: value, Comment: fieldName}
	}
	return obj
}

func hvacOperationTestRaw(index int, objectType string, values ...string) Object {
	obj := Object{Index: index, Type: objectType}
	for _, value := range values {
		obj.Fields = append(obj.Fields, Field{Value: value})
	}
	return obj
}

func hvacOperationTestComponent(obj Object, inlet, outlet string) HVACComponent {
	return HVACComponent{ObjectIndex: obj.Index, ObjectType: obj.Type, ObjectName: objectName(obj), Exists: true, InletNode: inlet, OutletNode: outlet}
}

func hvacOperationTestProfile(t *testing.T, profiles []HVACOperatingProfile, index int, loopID string) HVACOperatingProfile {
	t.Helper()
	for _, profile := range profiles {
		if profile.ComponentIndex == index && profile.LoopID == loopID {
			return profile
		}
	}
	t.Fatalf("missing profile %d on %q", index, loopID)
	return HVACOperatingProfile{}
}

func hvacOperationExpressionJSON(t *testing.T, value HVACOperatingExpression) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestHVACOperatingProfilesInheritOnlyActualWrapperReferences(t *testing.T) {
	loopObj := hvacOperationTestObject(0, "AirLoopHVAC", "Air", "Availability Manager List Name", "Managers")
	managerList := hvacOperationTestRaw(1, "AvailabilityManagerAssignmentList", "Managers", "AvailabilityManager:ScheduledOn", "On")
	manager := hvacOperationTestRaw(2, "AvailabilityManager:ScheduledOn", "On", "Loop Schedule")
	wrapper := hvacOperationTestObject(3, "AirLoopHVAC:UnitarySystem", "Unit", "Availability Schedule Name", "Parent Schedule", "Supply Fan Object Type", "Fan:OnOff", "Supply Fan Name", "Fan")
	fan := hvacOperationTestObject(4, "Fan:OnOff", "Fan", "Availability Schedule Name", "Fan Schedule")
	unrelated := hvacOperationTestObject(5, "Fan:OnOff", "Fan Similar", "Availability Schedule Name", "Unrelated Schedule")
	doc := Document{Objects: []Object{loopObj, managerList, manager, wrapper, fan, unrelated}}
	loop := HVACLoop{ID: "air:air", Type: "AirLoopHVAC", Name: "Air", ObjectIndex: 0, SupplySide: HVACLoopSide{Branches: []HVACBranch{{Components: []HVACComponent{hvacOperationTestComponent(wrapper, "in", "out")}}}}}
	profiles := buildHVACOperatingProfiles(newHVACContext(doc), []HVACLoop{loop}, nil)
	profile := hvacOperationTestProfile(t, profiles, 4, loop.ID)
	if len(profile.AvailabilityLayers) != 3 {
		t.Fatalf("layers = %#v", profile.AvailabilityLayers)
	}
	for index, want := range []int{0, 3, 4} {
		if profile.AvailabilityLayers[index].Owner.ObjectIndex != want {
			t.Fatalf("layer %d = %#v", index, profile.AvailabilityLayers[index])
		}
	}
	local := profile.AvailabilityLayers[2].Expression
	if local.Op != "fan_gate" || local.Children[1].ScheduleName != "Fan Schedule" {
		t.Fatalf("fan override = %#v", local)
	}
	jsonText := hvacOperationExpressionJSON(t, profile.FinalAvailability)
	if strings.Contains(jsonText, "Unrelated Schedule") || !strings.Contains(jsonText, "Parent Schedule") || !strings.Contains(jsonText, "Loop Schedule") {
		t.Fatalf("incorrect inherited expression: %s", jsonText)
	}
	if hvacOperationTestProfile(t, profiles, 5, "").AvailabilityLayers[0].Owner.ObjectIndex != 5 {
		t.Fatal("unrelated fan inherited another object's parent")
	}
}

func TestHVACOperatingManagerOrderAndLoopSemantics(t *testing.T) {
	doc := Document{Objects: []Object{
		hvacOperationTestObject(0, "AirLoopHVAC", "Air", "Availability Manager List Name", "Managers"),
		hvacOperationTestObject(1, "PlantLoop", "Plant", "Availability Manager List Name", "Managers"),
		hvacOperationTestRaw(2, "AvailabilityManagerAssignmentList", "Managers", "AvailabilityManager:ScheduledOn", "On", "AvailabilityManager:ScheduledOff", "Off", "AvailabilityManager:NightCycle", "Night"),
		hvacOperationTestRaw(3, "AvailabilityManager:ScheduledOn", "On", "Enable"),
		hvacOperationTestRaw(4, "AvailabilityManager:ScheduledOff", "Off", "Disable"),
		hvacOperationTestRaw(5, "AvailabilityManager:NightCycle", "Night", "Applicability"),
	}}
	b := hvacOperatingBuilder{ctx: newHVACContext(doc)}
	for _, test := range []struct {
		index    int
		kind, op string
	}{{0, "AirLoopHVAC", "air_managers"}, {1, "PlantLoop", "plant_managers"}} {
		expr, found := b.managerExpression(doc.Objects[test.index], test.kind)
		if !found || expr.Op != test.op || len(expr.Children) != 3 {
			t.Fatalf("manager group = %#v", expr)
		}
		if expr.Children[0].Op != "manager_scheduled_on" || expr.Children[1].Op != "manager_scheduled_off" || expr.Children[2].Op != "manager_unknown" {
			t.Fatalf("manager order/status semantics lost: %#v", expr.Children)
		}
	}
}

func TestHVACOperatingZoneFanManagerOverridesWrapperSchedule(t *testing.T) {
	unit := hvacOperationTestObject(0, "ZoneHVAC:UnitHeater", "Heater", "Availability Schedule Name", "Wrapper Hours", "Availability Manager List Name", "Managers")
	doc := Document{Objects: []Object{unit, hvacOperationTestRaw(1, "AvailabilityManagerAssignmentList", "Managers", "AvailabilityManager:ScheduledOn", "Start"), hvacOperationTestRaw(2, "AvailabilityManager:ScheduledOn", "Start", "Override Hours")}}
	profile := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), nil, nil), 0, "")
	if profile.FinalAvailability.Op != "fan_gate" || profile.FinalAvailability.Target != "zone_fan" || profile.FinalAvailability.Children[1].ScheduleName != "Wrapper Hours" {
		t.Fatalf("wrapper schedule incorrectly ANDed with manager override: %#v", profile.FinalAvailability)
	}
}

func TestHVACOperatingPumpFlowAndFanModeSchedulesRemainControls(t *testing.T) {
	pump := hvacOperationTestObject(0, "Pump:VariableSpeed", "Pump", "Pump Flow Rate Schedule Name", "Zero Flow", "Pump Control Type", "Intermittent")
	fan := hvacOperationTestObject(1, "Fan:OnOff", "Fan", "Availability Schedule Name", "Fan Available")
	unit := hvacOperationTestObject(2, "AirLoopHVAC:UnitarySystem", "Unit", "Supply Air Fan Operating Mode Schedule Name", "Cycling Mode")
	profiles := buildHVACOperatingProfiles(newHVACContext(Document{Objects: []Object{pump, fan, unit}}), nil, nil)
	for _, index := range []int{0, 2} {
		profile := hvacOperationTestProfile(t, profiles, index, "")
		if profile.FinalAvailability.Op != "always" {
			t.Fatalf("control schedule wrongly gates availability: %#v", profile)
		}
		foundSchedule := false
		for _, node := range profile.Control.Nodes {
			foundSchedule = foundSchedule || node.Kind == "schedule"
		}
		if !foundSchedule {
			t.Fatalf("control schedule disappeared: %#v", profile.Control)
		}
	}
	if hvacOperationTestProfile(t, profiles, 1, "").FinalAvailability.ScheduleName != "Fan Available" {
		t.Fatal("fan availability missing")
	}
}

func TestHVACOperatingPumpNoRequestOffDependsOnExplicitControlMode(t *testing.T) {
	for _, objectType := range []string{"Pump:VariableSpeed", "Pump:ConstantSpeed", "HeaderedPumps:VariableSpeed", "HeaderedPumps:ConstantSpeed"} {
		for _, mode := range []string{"Intermittent", "Continuous", ""} {
			t.Run(objectType+"/"+mode, func(t *testing.T) {
				pump := hvacOperationTestObject(0, objectType, "Pump", "Pump Control Type", mode)
				profile := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(Document{Objects: []Object{pump}}), nil, nil), 0, "")
				found := false
				for _, edge := range profile.Control.Edges {
					found = found || edge.From == "decision" && edge.To == "off" && edge.Relation == "no_request"
				}
				if found != (mode == "Intermittent") {
					t.Fatalf("no-request OFF branch = %v for Pump Control Type %q", found, mode)
				}
			})
		}
	}
}

func TestHVACOperatingDispatchMatchesTypeNameAndRetainsRuntimeConditions(t *testing.T) {
	boiler := hvacOperationTestObject(1, "Boiler:HotWater", "Source")
	unlisted := hvacOperationTestObject(2, "Boiler:HotWater", "Source Similar")
	loopObj := hvacOperationTestObject(0, "PlantLoop", "Plant", "Plant Equipment Operation Scheme Name", "Operations")
	doc := Document{Objects: []Object{loopObj, boiler, unlisted,
		hvacOperationTestRaw(3, "PlantEquipmentOperationSchemes", "Operations", "PlantEquipmentOperation:HeatingLoad", "Heating", "Heating Hours", "PlantEquipmentOperation:Uncontrolled", "Other", "Other Hours"),
		hvacOperationTestRaw(4, "PlantEquipmentOperation:HeatingLoad", "Heating", "0", "100000", "Heating List"),
		hvacOperationTestRaw(5, "PlantEquipmentList", "Heating List", "Boiler:HotWater", "Source"),
		hvacOperationTestRaw(6, "PlantEquipmentOperation:Uncontrolled", "Other", "Other List"),
		hvacOperationTestRaw(7, "PlantEquipmentList", "Other List", "Chiller:Electric:EIR", "Source"),
	}}
	loop := HVACLoop{ID: "plant:p", Type: "PlantLoop", ObjectIndex: 0, OperationScheme: "Operations", SupplySide: HVACLoopSide{Branches: []HVACBranch{{Components: []HVACComponent{hvacOperationTestComponent(boiler, "in", "out"), hvacOperationTestComponent(unlisted, "in2", "out2")}}}}}
	profiles := buildHVACOperatingProfiles(newHVACContext(doc), []HVACLoop{loop}, nil)
	profile := hvacOperationTestProfile(t, profiles, 1, loop.ID)
	text := hvacOperationExpressionJSON(t, profile.FinalAvailability)
	if !strings.Contains(text, "Heating Hours") || !strings.Contains(text, "runtime_dispatch") || strings.Contains(text, "Other Hours") {
		t.Fatalf("dispatch reference/condition mismatch: %s", text)
	}
	last := hvacOperationTestProfile(t, profiles, 2, loop.ID).AvailabilityLayers[1].Expression
	if last.Value == nil || *last.Value != 0 {
		t.Fatalf("unlisted equipment enabled: %#v", last)
	}
}

func TestHVACOperatingControlsMatchExactNodesAndExpandNodeLists(t *testing.T) {
	coil := hvacOperationTestObject(0, "Coil:Heating:Water", "Coil", "Water Inlet Node Name", "Water In", "Water Outlet Node Name", "Water Out", "Air Inlet Node Name", "Air In", "Air Outlet Node Name", "Air Out")
	controller := hvacOperationTestObject(1, "Controller:WaterCoil", "Controller", "Sensor Node Name", "Air Out", "Actuator Node Name", "Water In", "Control Variable", "Temperature")
	unrelated := hvacOperationTestObject(2, "Controller:WaterCoil", "Other", "Actuator Node Name", "Water In Similar")
	manager := hvacOperationTestObject(3, "SetpointManager:Scheduled", "Setpoint", "Schedule Name", "Temperature", "Setpoint Node or NodeList Name", "Targets")
	nodes := hvacOperationTestRaw(4, "NodeList", "Targets", "Air Out")
	doc := Document{Objects: []Object{coil, controller, unrelated, manager, nodes}}
	profile := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), nil, nil), 0, "")
	foundController, foundManager := false, false
	for _, node := range profile.Control.Nodes {
		if node.Kind == "controller" && node.Source.ObjectIndex == 2 {
			t.Fatal("similar node matched unrelated controller")
		}
		foundController = foundController || node.Kind == "controller" && node.Source.ObjectIndex == 1
		foundManager = foundManager || node.Kind == "setpoint" && node.Source.ObjectIndex == 3
	}
	if !foundController || !foundManager {
		t.Fatalf("missing controller/setpoint chain: %#v", profile.Control)
	}
	// The same coil on an air-loop branch still has a WATER flow actuator.
	air := hvacOperationTestObject(5, "AirLoopHVAC", "Air")
	doc.Objects = append(doc.Objects, air)
	loop := HVACLoop{ID: "air:a", Type: "AirLoopHVAC", ObjectIndex: 5, SupplySide: HVACLoopSide{Branches: []HVACBranch{{Components: []HVACComponent{hvacOperationTestComponent(coil, "Air In", "Air Out")}}}}}
	airProfile := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), []HVACLoop{loop}, nil), 0, loop.ID)
	foundController = false
	for _, node := range airProfile.Control.Nodes {
		foundController = foundController || node.Kind == "controller" && node.Source.ObjectIndex == 1
	}
	if !foundController {
		t.Fatal("air-circuit view lost the coil's water flow controller")
	}
}

func TestHVACOperatingSequenceSeparatesPermissionModulationAndPassiveFlow(t *testing.T) {
	boiler := hvacOperationTestObject(0, "Boiler:HotWater", "Boiler", "Normalized Boiler Efficiency Curve Name", "Efficiency Curve", "Minimum Part Load Ratio", "0.2")
	curve := hvacOperationTestRaw(1, "Curve:Quadratic", "Efficiency Curve", "1", "0", "0", "0", "1")
	pipe := hvacOperationTestObject(2, "Pipe:Adiabatic", "Pipe")
	pump := hvacOperationTestObject(3, "Pump:VariableSpeed", "Pump")
	loopObj := hvacOperationTestObject(4, "PlantLoop", "Plant")
	setpoint := hvacOperationTestObject(5, "SetpointManager:Scheduled", "Loop Target", "Schedule Name", "Target Temperature", "Setpoint Node or NodeList Name", "Loop Out")
	loop := HVACLoop{ID: "plant:p", Type: "PlantLoop", ObjectIndex: 4, SetpointNode: "Loop Out", SupplySide: HVACLoopSide{Branches: []HVACBranch{{Components: []HVACComponent{hvacOperationTestComponent(boiler, "Boiler In", "Boiler Out"), hvacOperationTestComponent(pipe, "Pipe In", "Pipe Out"), hvacOperationTestComponent(pump, "Pump In", "Pump Out")}}}}}
	profiles := buildHVACOperatingProfiles(newHVACContext(Document{Objects: []Object{boiler, curve, pipe, pump, loopObj, setpoint}}), []HVACLoop{loop}, nil)
	profile := hvacOperationTestProfile(t, profiles, 0, loop.ID)
	hasOff, curveModulates := false, false
	for _, edge := range profile.Control.Edges {
		hasOff = hasOff || edge.To == "off" && edge.Relation == "blocked"
		if edge.From == "curve:1" {
			if edge.To != "modulation" {
				t.Fatalf("performance curve controls on/off: %#v", edge)
			}
			curveModulates = true
		}
	}
	if !hasOff || !curveModulates {
		t.Fatalf("missing off/modulation semantics: %#v", profile.Control)
	}
	passive := hvacOperationTestProfile(t, profiles, 2, loop.ID)
	if len(passive.Control.Nodes) != 2 || passive.Control.Edges[0].Relation != "transfers" {
		t.Fatalf("passive pipe has active control: %#v", passive.Control)
	}
	for _, node := range hvacOperationTestProfile(t, profiles, 3, loop.ID).Control.Nodes {
		if node.Kind == "setpoint" {
			t.Fatal("thermal loop setpoint wrongly controls the pump")
		}
	}
}

func TestHVACOperatingCoilIncludesActualAirLoopPrimeMover(t *testing.T) {
	air := hvacOperationTestObject(0, "AirLoopHVAC", "Air")
	fan := hvacOperationTestObject(1, "Fan:OnOff", "Fan", "Availability Schedule Name", "Fan Hours")
	coil := hvacOperationTestObject(2, "Coil:Heating:Water", "Coil", "Availability Schedule Name", "Coil Hours")
	loop := HVACLoop{ID: "air:a", Type: "AirLoopHVAC", ObjectIndex: 0, SupplySide: HVACLoopSide{Branches: []HVACBranch{{Components: []HVACComponent{hvacOperationTestComponent(fan, "A", "B"), hvacOperationTestComponent(coil, "B", "C")}}}}}
	profiles := buildHVACOperatingProfiles(newHVACContext(Document{Objects: []Object{air, fan, coil}}), []HVACLoop{loop}, nil)
	profile := hvacOperationTestProfile(t, profiles, 2, loop.ID)
	text := hvacOperationExpressionJSON(t, profile.FinalAvailability)
	if !strings.Contains(text, "Fan Hours") || !strings.Contains(text, "Coil Hours") {
		t.Fatalf("coil permission ignored prime mover: %s", text)
	}
	if profile.AvailabilityLayers[1].Role != "air_transport" || profile.AvailabilityLayers[1].Owner.ObjectIndex != 1 {
		t.Fatalf("untraceable prime mover layer: %#v", profile.AvailabilityLayers)
	}
}

func TestHVACOperatingPlantCoilKeepsContainerAvailabilityWithoutOtherLoopManagers(t *testing.T) {
	unit := hvacOperationTestObject(0, "ZoneHVAC:UnitHeater", "Heater", "Availability Schedule Name", "Unit Hours", "Heating Coil Object Type", "Coil:Heating:Water", "Heating Coil Name", "Coil")
	coil := hvacOperationTestObject(1, "Coil:Heating:Water", "Coil", "Availability Schedule Name", "Coil Hours")
	plant := hvacOperationTestObject(2, "PlantLoop", "Plant")
	loop := HVACLoop{ID: "plant:p", Type: "PlantLoop", ObjectIndex: 2, DemandSide: HVACLoopSide{Branches: []HVACBranch{{Components: []HVACComponent{hvacOperationTestComponent(coil, "Water In", "Water Out")}}}}}
	profile := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(Document{Objects: []Object{unit, coil, plant}}), []HVACLoop{loop}, nil), 1, loop.ID)
	text := hvacOperationExpressionJSON(t, profile.FinalAvailability)
	if !strings.Contains(text, "Unit Hours") || !strings.Contains(text, "Coil Hours") {
		t.Fatalf("plant occurrence erased real container: %s", text)
	}
	if profile.AvailabilityLayers[1].Owner.ObjectIndex != 0 {
		t.Fatalf("container order = %#v", profile.AvailabilityLayers)
	}
}

func TestHVACOperatingLocalTerminalFanPermissionIsRetained(t *testing.T) {
	air := hvacOperationTestObject(0, "AirLoopHVAC", "Air")
	terminal := hvacOperationTestObject(1, "AirTerminal:SingleDuct:SeriesPIU:Reheat", "PIU", "Availability Schedule Name", "Terminal Hours", "Fan Name", "Local Fan")
	fan := hvacOperationTestObject(2, "Fan:OnOff", "Local Fan", "Availability Schedule Name", "Local Fan Hours")
	loop := HVACLoop{ID: "air:a", Type: "AirLoopHVAC", Name: "Air", ObjectIndex: 0}
	relation := HVACZoneChain{ZoneName: "Zone", AirLoopNames: []string{"Air"}, TerminalUnits: []HVACComponent{hvacOperationTestComponent(terminal, "In", "Out")}}
	profile := hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(Document{Objects: []Object{air, terminal, fan}}), []HVACLoop{loop}, []HVACZoneChain{relation}), 1, loop.ID)
	text := hvacOperationExpressionJSON(t, profile.FinalAvailability)
	if !strings.Contains(text, "Local Fan Hours") {
		t.Fatalf("local fan off could become allowed: %s", text)
	}
	if profile.AvailabilityLayers[1].Owner.ObjectIndex != 2 {
		t.Fatalf("local airflow layer missing: %#v", profile.AvailabilityLayers)
	}
}

func TestHVACOperatingDispatchHighlightsOnlyEquipmentRange(t *testing.T) {
	equipment := hvacOperationTestObject(0, "Boiler:HotWater", "Boiler")
	selected := hvacOperationTestRaw(1, "PlantEquipmentList", "Selected", "Boiler:HotWater", "Boiler")
	op := hvacOperationTestRaw(2, "PlantEquipmentOperation:HeatingLoad", "Heating", "0", "10", "Other", "20", "30", "Selected")
	params := hvacOperatingDispatchParameters(hvacOperatingDispatchChoice{operation: op, lists: []Object{selected}}, equipment)
	if len(params) != 3 || params[0].Value != "20" || params[1].Value != "30" {
		t.Fatalf("selected equipment got another range: %#v", params)
	}
	op.Fields = append(op.Fields, Field{Value: "40"}, Field{Value: "50"}, Field{Value: "Selected"})
	if params = hvacOperatingDispatchParameters(hvacOperatingDispatchChoice{operation: op, lists: []Object{selected}}, equipment); len(params) != 0 {
		t.Fatalf("ambiguous multiple ranges advertised as single range: %#v", params)
	}
}

func TestHVACOperatingSharedEquipmentKeepsLoopCircuitSetpointsSeparate(t *testing.T) {
	chiller := hvacOperationTestObject(0, "Chiller:Electric:EIR", "Chiller", "Chilled Water Inlet Node Name", "Evap In", "Chilled Water Outlet Node Name", "Evap Out", "Condenser Inlet Node Name", "Cond In", "Condenser Outlet Node Name", "Cond Out")
	plant := hvacOperationTestObject(1, "PlantLoop", "Plant")
	condenser := hvacOperationTestObject(2, "CondenserLoop", "Condenser")
	evap := hvacOperationTestObject(3, "SetpointManager:Scheduled", "Evaporator Setpoint", "Schedule Name", "CHW Temp", "Setpoint Node or NodeList Name", "Evap Out")
	cond := hvacOperationTestObject(4, "SetpointManager:Scheduled", "Condenser Setpoint", "Schedule Name", "CW Temp", "Setpoint Node or NodeList Name", "Cond Out")
	loops := []HVACLoop{
		{ID: "plant:p", Type: "PlantLoop", ObjectIndex: 1, SupplySide: HVACLoopSide{Branches: []HVACBranch{{Components: []HVACComponent{hvacOperationTestComponent(chiller, "Evap In", "Evap Out")}}}}},
		{ID: "condenser:c", Type: "CondenserLoop", ObjectIndex: 2, DemandSide: HVACLoopSide{Branches: []HVACBranch{{Components: []HVACComponent{hvacOperationTestComponent(chiller, "Cond In", "Cond Out")}}}}},
	}
	profiles := buildHVACOperatingProfiles(newHVACContext(Document{Objects: []Object{chiller, plant, condenser, evap, cond}}), loops, nil)
	for _, test := range []struct {
		loopID            string
		wanted, forbidden int
	}{{"plant:p", 3, 4}, {"condenser:c", 4, 3}} {
		profile := hvacOperationTestProfile(t, profiles, 0, test.loopID)
		found := false
		for _, node := range profile.Control.Nodes {
			if node.Kind != "setpoint" {
				continue
			}
			if node.Source.ObjectIndex == test.forbidden {
				t.Fatalf("other circuit setpoint leaked to %s", test.loopID)
			}
			found = found || node.Source.ObjectIndex == test.wanted
		}
		if !found {
			t.Fatalf("missing circuit setpoint on %s", test.loopID)
		}
	}
}

func TestHVACOperatingRecipesCoverEquipmentFamiliesWithoutInventedFallback(t *testing.T) {
	for _, objectType := range []string{"Boiler:HotWater", "Chiller:Electric:EIR", "Pump:VariableSpeed", "HeaderedPumps:ConstantSpeed", "Fan:SystemModel", "Coil:Heating:Water", "Coil:Cooling:DX:SingleSpeed", "AirLoopHVAC:UnitarySystem", "ZoneHVAC:FourPipeFanCoil", "AirTerminal:SingleDuct:VAV:Reheat", "AirConditioner:VariableRefrigerantFlow", "CoolingTower:VariableSpeed", "HeatExchanger:FluidToFluid", "WaterHeater:Mixed", "DistrictHeating:Water", "Pipe:Adiabatic"} {
		family, input, output := hvacOperatingRecipe(objectType)
		if family == "unresolved" || input == "" || output == "" {
			t.Fatalf("missing recipe for %s", objectType)
		}
	}
	if family, _, _ := hvacOperatingRecipe("PlantComponent:UserDefined"); family != "unresolved" {
		t.Fatalf("custom control fabricated: %s", family)
	}
	doc := Document{Objects: []Object{hvacOperationTestObject(0, "Fan:OnOff", "Fan", "Availability Schedule Name", "Hours"), hvacOperationTestRaw(1, "Schedule:Constant", "Hours", "", "1")}}
	report := AnalyzeHVAC(doc)
	if len(report.OperatingProfiles) != 1 || len(report.OperatingSchedules) != 1 {
		t.Fatalf("operating contract not attached: %#v", report)
	}
}
