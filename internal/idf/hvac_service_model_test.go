package idf

import (
	"os"
	"strings"
	"testing"
)

func TestHVACServiceModelReferenceLargeOfficeZonePaths(t *testing.T) {
	text, err := os.ReadFile("../../frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf")
	if err != nil {
		t.Fatalf("read reference sample: %v", err)
	}
	doc, err := Parse(string(text))
	if err != nil {
		t.Fatalf("parse reference sample: %v", err)
	}

	report := AnalyzeHVAC(doc)
	coreBottom := findHVACTestingZoneService(report.ServiceModel, "Core_bottom")
	if coreBottom == nil {
		t.Fatalf("Core_bottom zone service not found: %#v", report.ServiceModel.ZoneServices)
	}
	if !hasZoneServicePath(coreBottom.Paths, "cooling", "central_air_with_plant", "CoolSys1", "VAV_1", "Core_bottom VAV Box") {
		t.Fatalf("Core_bottom paths = %#v, want cooling CoolSys1 -> VAV_1 -> terminal path", coreBottom.Paths)
	}
	if !hasZoneServicePath(coreBottom.Paths, "heating", "central_air_with_plant", "HeatSys1", "VAV_1", "Core_bottom VAV Box") {
		t.Fatalf("Core_bottom paths = %#v, want heating HeatSys1 -> VAV_1 -> terminal path", coreBottom.Paths)
	}
	if !hasZoneServicePath(coreBottom.Paths, "ventilation", "central_air", "", "VAV_1", "Core_bottom VAV Box") {
		t.Fatalf("Core_bottom paths = %#v, want ventilation VAV_1 -> terminal path", coreBottom.Paths)
	}
	for _, path := range coreBottom.Paths {
		if path.DeliveryEquipment.DeliveryType == "fan_coil" || path.DeliveryEquipment.DeliveryType == "zone_direct_unit" {
			t.Fatalf("Core_bottom VAV path misclassified as direct zone equipment: %#v", path)
		}
		if path.ServiceKind == "heating" && path.AirLoop == nil {
			t.Fatalf("Core_bottom heating service missing air loop context: %#v", path)
		}
	}
}

func TestHVACServiceModelKeepsFanCoilAsDirectZoneDelivery(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office", Comment: "Zone Name"},
			{Value: "Office Equipment", Comment: "Zone Conditioning Equipment List Name"},
			{Value: "Office Supply Inlet", Comment: "Zone Air Inlet Node or NodeList Name"},
			{Value: "", Comment: "Zone Air Exhaust Node or NodeList Name"},
			{Value: "Office Zone Air Node", Comment: "Zone Air Node Name"},
			{Value: "", Comment: "Zone Return Air Node or NodeList Name"},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment", Comment: "Name"},
			{Value: "ZoneHVAC:FourPipeFanCoil", Comment: "Zone Equipment 1 Object Type"},
			{Value: "Office FPFC", Comment: "Zone Equipment 1 Name"},
			{Value: "1", Comment: "Zone Equipment 1 Cooling Sequence"},
			{Value: "1", Comment: "Zone Equipment 1 Heating or No-Load Sequence"},
		}},
		{Index: 3, Type: "ZoneHVAC:FourPipeFanCoil", Fields: []Field{
			{Value: "Office FPFC", Comment: "Name"},
			{Value: "", Comment: "Availability Schedule Name"},
			{Value: "Autosize", Comment: "Maximum Supply Air Flow Rate"},
			{Value: "Office Supply Inlet", Comment: "Air Inlet Node Name"},
			{Value: "Office Zone Air Node", Comment: "Air Outlet Node Name"},
			{Value: "HW Inlet", Comment: "Hot Water Inlet Node Name"},
			{Value: "HW Outlet", Comment: "Hot Water Outlet Node Name"},
			{Value: "CHW Inlet", Comment: "Chilled Water Inlet Node Name"},
			{Value: "CHW Outlet", Comment: "Chilled Water Outlet Node Name"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	office := findHVACTestingZoneService(report.ServiceModel, "Office")
	if office == nil {
		t.Fatalf("Office zone service not found: %#v", report.ServiceModel.ZoneServices)
	}
	if !hasZoneServicePath(office.Paths, "cooling", "direct_zone_air", "", "", "Office FPFC") {
		t.Fatalf("Office paths = %#v, want fan coil cooling as direct zone air", office.Paths)
	}
	if !hasZoneServicePath(office.Paths, "heating", "direct_zone_air", "", "", "Office FPFC") {
		t.Fatalf("Office paths = %#v, want fan coil heating as direct zone air", office.Paths)
	}
	for _, path := range office.Paths {
		if path.AirLoop != nil {
			t.Fatalf("fan coil path gained an air loop: %#v", path)
		}
		if path.DeliveryEquipment.DeliveryType != "fan_coil" || path.DeliveryEquipment.RequiresAirLoop {
			t.Fatalf("fan coil delivery classification = %#v", path.DeliveryEquipment)
		}
	}
}

func TestHVACServiceModelClassifiesPackagedLocalEquipment(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: "Office Supply Inlet"},
			{Value: ""},
			{Value: "Office Zone Air Node"},
			{Value: ""},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "ZoneHVAC:PackagedTerminalHeatPump"},
			{Value: "Office PTHP"},
			{Value: "1"},
			{Value: "1"},
		}},
		{Index: 3, Type: "ZoneHVAC:PackagedTerminalHeatPump", Fields: []Field{
			{Value: "Office PTHP"},
			{Value: ""},
			{Value: "Autosize"},
			{Value: "Office Supply Inlet"},
			{Value: "Office Zone Air Node"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	office := findHVACTestingZoneService(report.ServiceModel, "Office")
	if office == nil {
		t.Fatalf("Office zone service not found: %#v", report.ServiceModel.ZoneServices)
	}
	for _, serviceKind := range []string{"cooling", "heating"} {
		path := findZoneServicePath(office.Paths, serviceKind, "direct_zone_air", "", "", "Office PTHP")
		if path == nil {
			t.Fatalf("Office paths = %#v, want %s packaged heat pump path", office.Paths, serviceKind)
		}
		if path.DeliveryEquipment.DeliveryType != "pthp" || path.DeliveryEquipment.RequiresAirLoop {
			t.Fatalf("PTHP delivery classification = %#v", path.DeliveryEquipment)
		}
		if path.SourceSystem == nil || path.SourceSystem.Name == "" {
			t.Fatalf("PTHP path missing local source system: %#v", path)
		}
		if strings.Contains(strings.ToLower(path.Delivery.DisplayFamily), "terminal") {
			t.Fatalf("PTHP delivery label still looks terminal-like: %#v", path.Delivery)
		}
	}
}

func TestHVACServiceModelBuildsRadiantPlantPath(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: ""},
			{Value: ""},
			{Value: "Office Zone Air Node"},
			{Value: ""},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "ZoneHVAC:LowTemperatureRadiant:VariableFlow"},
			{Value: "Office Radiant"},
			{Value: "1"},
			{Value: "1"},
		}},
		{Index: 3, Type: "ZoneHVAC:LowTemperatureRadiant:VariableFlow", Fields: []Field{
			{Value: "Office Radiant"},
			{Value: "Radiant Design"},
			{Value: ""},
			{Value: "Office"},
			{Value: "Office Radiant Surfaces"},
			{Value: "100"},
			{Value: "Autosize"},
			{Value: "Autosize"},
			{Value: "HW Demand Inlet"},
			{Value: "HW Demand Outlet"},
		}},
		{Index: 4, Type: "PlantLoop", Fields: []Field{
			{Value: "Heating Water Loop"},
			{Value: "Water"},
			{Value: ""},
			{Value: ""},
			{Value: "HW Setpoint"},
			{Value: "80"},
			{Value: "20"},
			{Value: "Autosize"},
			{Value: "0"},
			{Value: "Autosize"},
			{Value: "HW Supply Inlet"},
			{Value: "HW Supply Outlet"},
			{Value: "HW Supply Branches"},
			{Value: ""},
			{Value: "HW Demand Inlet"},
			{Value: "HW Demand Outlet"},
			{Value: "HW Demand Branches"},
			{Value: ""},
		}},
		{Index: 5, Type: "BranchList", Fields: []Field{
			{Value: "HW Demand Branches"},
			{Value: "Radiant Branch"},
		}},
		{Index: 6, Type: "Branch", Fields: []Field{
			{Value: "Radiant Branch"},
			{Value: ""},
			{Value: "ZoneHVAC:LowTemperatureRadiant:VariableFlow"},
			{Value: "Office Radiant"},
			{Value: "HW Demand Inlet"},
			{Value: "HW Demand Outlet"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	office := findHVACTestingZoneService(report.ServiceModel, "Office")
	if office == nil {
		t.Fatalf("Office zone service not found: %#v", report.ServiceModel.ZoneServices)
	}
	path := findZoneServicePath(office.Paths, "radiant_heating", "radiant", "Heating Water Loop", "", "Office Radiant")
	if path == nil {
		t.Fatalf("Office paths = %#v, want radiant plant path", office.Paths)
	}
	if path.AirLoop != nil {
		t.Fatalf("radiant path gained an air loop: %#v", path)
	}
	if path.DeliveryEquipment.DeliveryType != "radiant_floor" {
		t.Fatalf("radiant delivery classification = %#v", path.DeliveryEquipment)
	}
}

func TestHVACServiceModelBuildsVRFRefrigerantPath(t *testing.T) {
	vrfSystemFields := make([]Field, 37)
	vrfSystemFields[0] = Field{Value: "Office VRF Outdoor"}
	vrfSystemFields[36] = Field{Value: "Office VRF Terminal List"}
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: "Office Inlet"},
			{Value: ""},
			{Value: "Office Air Node"},
			{Value: ""},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "SequentialLoad"},
			{Value: "ZoneHVAC:TerminalUnit:VariableRefrigerantFlow"},
			{Value: "Office VRF Terminal"},
			{Value: "1"},
			{Value: "1"},
			{Value: ""},
			{Value: ""},
		}},
		{Index: 3, Type: "ZoneHVAC:TerminalUnit:VariableRefrigerantFlow", Fields: []Field{
			{Value: "Office VRF Terminal"},
			{Value: "Always On"},
			{Value: "Office VRF Inlet"},
			{Value: "Office Inlet"},
		}},
		{Index: 4, Type: "AirConditioner:VariableRefrigerantFlow", Fields: vrfSystemFields},
		{Index: 5, Type: "ZoneTerminalUnitList", Fields: []Field{
			{Value: "Office VRF Terminal List"},
			{Value: "Office VRF Terminal"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	office := findHVACTestingZoneService(report.ServiceModel, "Office")
	if office == nil {
		t.Fatalf("Office zone service not found: %#v", report.ServiceModel.ZoneServices)
	}
	for _, serviceKind := range []string{"cooling", "heating"} {
		path := findZoneServicePath(office.Paths, serviceKind, "direct_zone_refrigerant", "", "", "Office VRF Terminal")
		if path == nil {
			t.Fatalf("Office paths = %#v, want %s VRF refrigerant path", office.Paths, serviceKind)
		}
		if path.RefrigerantSystem == nil || path.RefrigerantSystem.Name != "Office VRF Outdoor" {
			t.Fatalf("VRF path refrigerant system = %#v", path.RefrigerantSystem)
		}
		if path.PlantLoop != nil || path.AirLoop != nil {
			t.Fatalf("VRF path gained plant/air loop: %#v", path)
		}
	}
}

func TestHVACServiceModelClassifiesSupportingSystemCouplings(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "ThermalStorage:Ice:Simple", Fields: []Field{{Value: "Ice Tank"}}},
		{Index: 1, Type: "ThermalStorage:ChilledWater:Mixed", Fields: []Field{{Value: "CHW Tank"}}},
		{Index: 2, Type: "CoolingTower:SingleSpeed", Fields: []Field{{Value: "Tower"}}},
		{Index: 3, Type: "Generator:Photovoltaic", Fields: []Field{{Value: "PV Array"}}},
		{Index: 4, Type: "ElectricLoadCenter:Storage:Simple", Fields: []Field{{Value: "Battery"}}},
		{Index: 5, Type: "WaterHeater:Mixed", Fields: []Field{{Value: "DHW Heater"}}},
		{Index: 6, Type: "WaterUse:Equipment", Fields: []Field{{Value: "Lavatory"}}},
		{Index: 7, Type: "SetpointManager:Scheduled", Fields: []Field{{Value: "SAT Reset"}}},
		{Index: 8, Type: "FaultModel:TemperatureSensorOffset:OutdoorAir", Fields: []Field{{Value: "OA Sensor Fault"}}},
		{Index: 9, Type: "EnergyManagementSystem:Program", Fields: []Field{{Value: "EMS Program"}}},
		{Index: 10, Type: "PythonPlugin:Instance", Fields: []Field{{Value: "Python Plugin"}}},
		{Index: 11, Type: "ExternalInterface:Schedule", Fields: []Field{{Value: "External Schedule"}}},
		{Index: 12, Type: "PlantEquipmentOperation:ThermalEnergyStorage", Fields: []Field{{Value: "TES Operation"}}},
		{Index: 13, Type: "FluidCooler:SingleSpeed", Fields: []Field{{Value: "Fluid Cooler"}}},
		{Index: 14, Type: "EvaporativeFluidCooler:SingleSpeed", Fields: []Field{{Value: "Evap Fluid Cooler"}}},
		{Index: 15, Type: "GroundHeatExchanger:Vertical", Fields: []Field{{Value: "Ground HX"}}},
		{Index: 16, Type: "PipingSystem:Underground:Domain", Fields: []Field{{Value: "Underground Domain"}}},
		{Index: 17, Type: "HeatExchanger:FluidToFluid", Fields: []Field{{Value: "Loop HX"}}},
		{Index: 18, Type: "Generator:FuelCell", Fields: []Field{{Value: "Fuel Cell"}}},
		{Index: 19, Type: "Generator:FuelCell:ExhaustGasToWaterHeatExchanger", Fields: []Field{{Value: "Fuel Cell HX"}}},
		{Index: 20, Type: "Generator:FuelSupply", Fields: []Field{{Value: "Fuel Supply"}}},
	}}

	report := AnalyzeHVAC(doc)
	couplings := report.ServiceModel.Couplings
	for _, expected := range []struct {
		objectType   string
		couplingType string
		role         string
	}{
		{"ThermalStorage:Ice:Simple", "thermal_storage", "ice_storage"},
		{"ThermalStorage:ChilledWater:Mixed", "thermal_storage", "chilled_water_storage"},
		{"CoolingTower:SingleSpeed", "heat_rejection", "cooling_tower"},
		{"Generator:Photovoltaic", "generator", "pv"},
		{"ElectricLoadCenter:Storage:Simple", "electric_storage", "battery"},
		{"WaterHeater:Mixed", "service_water", "water_heater"},
		{"WaterUse:Equipment", "service_water", "water_use"},
		{"SetpointManager:Scheduled", "control_overlay", "setpoint_manager"},
		{"FaultModel:TemperatureSensorOffset:OutdoorAir", "fault_overlay", "fault_model"},
		{"EnergyManagementSystem:Program", "control_overlay", "ems_external"},
		{"PythonPlugin:Instance", "control_overlay", "ems_external"},
		{"ExternalInterface:Schedule", "control_overlay", "ems_external"},
		{"PlantEquipmentOperation:ThermalEnergyStorage", "operation_scheme", "thermal_storage_operation"},
		{"FluidCooler:SingleSpeed", "heat_rejection", "fluid_cooler"},
		{"EvaporativeFluidCooler:SingleSpeed", "heat_rejection", "fluid_cooler"},
		{"GroundHeatExchanger:Vertical", "source_sink", "ground_hx"},
		{"PipingSystem:Underground:Domain", "source_sink", "ground_hx"},
		{"HeatExchanger:FluidToFluid", "heat_recovery", "fluid_heat_exchanger"},
		{"Generator:FuelCell", "generator", "fuel_cell"},
		{"Generator:FuelCell:ExhaustGasToWaterHeatExchanger", "heat_recovery", "fuel_cell_heat_recovery"},
		{"Generator:FuelSupply", "source_sink", "fuel_supply"},
	} {
		if !hasSystemCoupling(couplings, expected.objectType, expected.couplingType, expected.role) {
			t.Fatalf("couplings = %#v, missing %#v", couplings, expected)
		}
	}
	if !hasEnergyNetwork(report.ServiceModel, "electric_network") {
		t.Fatalf("networks = %#v, want electric network", report.ServiceModel.Networks)
	}
	if !hasEnergyNetwork(report.ServiceModel, "service_water") {
		t.Fatalf("networks = %#v, want service water network", report.ServiceModel.Networks)
	}
}

func TestHVACServiceModelSeparatesTrueAirTerminalsFromZoneEquipment(t *testing.T) {
	if !isTrueAirTerminalType("AirTerminal:SingleDuct:VAV:Reheat") {
		t.Fatal("VAV reheat air terminal was not recognized")
	}
	for _, objectType := range []string{
		"ZoneHVAC:AirDistributionUnit",
		"ZoneHVAC:FourPipeFanCoil",
		"ZoneHVAC:IdealLoadsAirSystem",
		"AirLoopHVAC:UnitarySystem",
	} {
		if isTrueAirTerminalType(objectType) {
			t.Fatalf("%s was misclassified as a true air terminal", objectType)
		}
	}
}

func TestHVACFrontendDefaultCopyHidesRuleGraphVocabulary(t *testing.T) {
	for _, path := range []string{
		"../../frontend/src/js/hvac-views.js",
		"../../frontend/src/js/i18n.js",
		"../../frontend/src/styles.css",
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)
		for _, forbidden := range []string{
			"Rule edges",
			"Rule trace",
			"Rule path",
			"Terminal / Equipment",
			"Plant / Condenser",
			"Zone relations",
			"Cross-loop",
			"hvac.inferred",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contains default-HVAC forbidden copy %q", path, forbidden)
			}
		}
	}
}

func findHVACTestingZoneService(model HVACServiceModel, zoneName string) *ZoneServiceSummary {
	for index := range model.ZoneServices {
		if strings.EqualFold(model.ZoneServices[index].ZoneName, zoneName) {
			return &model.ZoneServices[index]
		}
	}
	return nil
}

func hasZoneServicePath(paths []ZoneServicePath, serviceKind string, pathType string, plantLoop string, airLoop string, deliveryNeedle string) bool {
	return findZoneServicePath(paths, serviceKind, pathType, plantLoop, airLoop, deliveryNeedle) != nil
}

func findZoneServicePath(paths []ZoneServicePath, serviceKind string, pathType string, plantLoop string, airLoop string, deliveryNeedle string) *ZoneServicePath {
	for index := range paths {
		path := paths[index]
		if serviceKind != "" && path.ServiceKind != serviceKind {
			continue
		}
		if pathType != "" && path.PathType != pathType {
			continue
		}
		if plantLoop != "" {
			if path.PlantLoop == nil || !strings.EqualFold(path.PlantLoop.Name, plantLoop) {
				continue
			}
		}
		if airLoop != "" {
			if path.AirLoop == nil || !strings.EqualFold(path.AirLoop.Name, airLoop) {
				continue
			}
		}
		deliveryName := path.Delivery.ObjectName + " " + path.Delivery.DisplayName + " " + path.DeliveryEquipment.Component.ObjectName
		if deliveryNeedle != "" && !strings.Contains(strings.ToLower(deliveryName), strings.ToLower(deliveryNeedle)) {
			continue
		}
		return &paths[index]
	}
	return nil
}

func hasSystemCoupling(couplings []SystemCoupling, objectType string, couplingType string, role string) bool {
	for _, coupling := range couplings {
		if strings.EqualFold(coupling.Object.ObjectType, objectType) && coupling.CouplingType == couplingType && coupling.Role == role {
			return true
		}
	}
	return false
}

func hasEnergyNetwork(model HVACServiceModel, networkType string) bool {
	for _, network := range model.Networks {
		if network.NetworkType == networkType {
			return true
		}
	}
	return false
}
