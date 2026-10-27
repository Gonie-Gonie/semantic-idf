package idf

import "strings"

// These narrow fallbacks fill gaps in the shared (sometimes truncated) manual
// catalog without making field comments authoritative. Positions were checked
// against the canonical EnergyPlus v25.1.0 IDD:
// https://raw.githubusercontent.com/NREL/EnergyPlus/v25.1.0/idd/Energy%2B.idd.in
// Unknown equipment types stay unknown rather than borrowing a family's layout.
var hvacOperatingAvailabilityFields = func() map[string]int {
	result := map[string]int{}
	groups := map[int]string{
		1: `ZoneHVAC:IdealLoadsAirSystem ZoneHVAC:FourPipeFanCoil ZoneHVAC:WindowAirConditioner
ZoneHVAC:PackagedTerminalAirConditioner ZoneHVAC:PackagedTerminalHeatPump ZoneHVAC:WaterToAirHeatPump
ZoneHVAC:Dehumidifier:DX ZoneHVAC:EnergyRecoveryVentilator ZoneHVAC:UnitVentilator ZoneHVAC:UnitHeater
ZoneHVAC:EvaporativeCoolerUnit ZoneHVAC:HybridUnitaryHVAC ZoneHVAC:OutdoorAirUnit
ZoneHVAC:TerminalUnit:VariableRefrigerantFlow ZoneHVAC:Baseboard:RadiantConvective:Electric
ZoneHVAC:CoolingPanel:RadiantConvective:Water ZoneHVAC:Baseboard:Convective:Water
ZoneHVAC:Baseboard:Convective:Electric ZoneHVAC:LowTemperatureRadiant:Electric ZoneHVAC:HighTemperatureRadiant
ZoneHVAC:VentilatedSlab ZoneHVAC:RefrigerationChillerSet ZoneHVAC:ExhaustControl
AirTerminal:SingleDuct:ConstantVolume:Reheat AirTerminal:SingleDuct:ConstantVolume:NoReheat
AirTerminal:SingleDuct:VAV:NoReheat AirTerminal:SingleDuct:VAV:Reheat
AirTerminal:SingleDuct:VAV:Reheat:VariableSpeedFan AirTerminal:SingleDuct:VAV:HeatAndCool:NoReheat
AirTerminal:SingleDuct:VAV:HeatAndCool:Reheat AirTerminal:SingleDuct:SeriesPIU:Reheat
AirTerminal:SingleDuct:ParallelPIU:Reheat AirTerminal:SingleDuct:ConstantVolume:FourPipeInduction
AirTerminal:SingleDuct:ConstantVolume:CooledBeam AirTerminal:DualDuct:ConstantVolume
AirTerminal:DualDuct:VAV AirTerminal:DualDuct:VAV:OutdoorAir
Fan:SystemModel Fan:ConstantVolume Fan:VariableVolume Fan:OnOff Fan:ZoneExhaust
Coil:Cooling:Water Coil:Cooling:Water:DetailedGeometry Coil:Cooling:DX:SingleSpeed Coil:Cooling:DX:TwoSpeed
Coil:Cooling:DX:MultiSpeed Coil:Cooling:DX:TwoStageWithHumidityControlMode
Coil:Cooling:DX:VariableRefrigerantFlow Coil:Heating:DX:VariableRefrigerantFlow
Coil:Cooling:DX:VariableRefrigerantFlow:FluidTemperatureControl Coil:Heating:DX:VariableRefrigerantFlow:FluidTemperatureControl
Coil:Heating:Water Coil:Heating:Steam Coil:Heating:Electric Coil:Heating:Electric:MultiStage Coil:Heating:Fuel
Coil:Heating:Gas:MultiStage Coil:Heating:Desuperheater Coil:Heating:DX:SingleSpeed Coil:Heating:DX:MultiSpeed
Coil:WaterHeating:Desuperheater CoilSystem:Cooling:DX CoilSystem:Heating:DX Coil:Cooling:DX:SingleSpeed:ThermalStorage
AirLoopHVAC:Unitary:Furnace:HeatOnly AirLoopHVAC:Unitary:Furnace:HeatCool AirLoopHVAC:UnitaryHeatOnly
AirLoopHVAC:UnitaryHeatCool AirLoopHVAC:UnitaryHeatPump:AirToAir AirLoopHVAC:UnitaryHeatPump:WaterToAir
AirLoopHVAC:UnitaryHeatCool:VAVChangeoverBypass AirLoopHVAC:UnitaryHeatPump:AirToAir:MultiSpeed
AirConditioner:VariableRefrigerantFlow AirConditioner:VariableRefrigerantFlow:FluidTemperatureControl
AirConditioner:VariableRefrigerantFlow:FluidTemperatureControl:HR
WaterHeater:HeatPump:PumpedCondenser WaterHeater:HeatPump:WrappedCondenser
EvaporativeCooler:Direct:CelDekPad EvaporativeCooler:Indirect:CelDekPad EvaporativeCooler:Indirect:WetCoil
EvaporativeCooler:Indirect:ResearchSpecial EvaporativeCooler:Direct:ResearchSpecial
Humidifier:Steam:Electric Humidifier:Steam:Gas HeatExchanger:AirToAir:FlatPlate
HeatExchanger:AirToAir:SensibleAndLatent HeatExchanger:Desiccant:BalancedFlow HeatExchanger:FluidToFluid
ThermalStorage:Ice:Detailed`,
		2: `ZoneHVAC:Baseboard:RadiantConvective:Water ZoneHVAC:Baseboard:RadiantConvective:Steam
ZoneHVAC:LowTemperatureRadiant:VariableFlow ZoneHVAC:LowTemperatureRadiant:ConstantFlow
SolarCollector:UnglazedTranspired`,
		3: `Fan:ComponentModel CoilSystem:Cooling:Water Coil:Cooling:DX`,
		4: `AirLoopHVAC:UnitarySystem`,
	}
	for index, types := range groups {
		for _, typ := range strings.Fields(types) {
			result[normalizeName(typ)] = index
		}
	}
	return result
}()

var hvacOperatingManagerFields = map[string]int{
	"airloophvac": 2, "plantloop": 19,
	"zonehvac:fourpipefancoil": 24, "zonehvac:windowairconditioner": 15,
	"zonehvac:packagedterminalairconditioner": 21, "zonehvac:packagedterminalheatpump": 27,
	"zonehvac:watertoairheatpump": 26, "zonehvac:energyrecoveryventilator": 10,
	"zonehvac:unitventilator": 23, "zonehvac:unitheater": 14,
	"zonehvac:evaporativecoolerunit": 2, "zonehvac:hybridunitaryhvac": 2,
	"zonehvac:outdoorairunit": 18, "zonehvac:terminalunit:variablerefrigerantflow": 24,
	"zonehvac:ventilatedslab": 38,
}

var hvacOperatingNoAvailabilityField = func() map[string]bool {
	result := map[string]bool{}
	for _, typ := range strings.Fields(`
AirLoopHVAC PlantLoop CondenserLoop AirTerminal:SingleDuct:Mixer
ZoneHVAC:AirDistributionUnit AirLoopHVAC:OutdoorAirSystem AirLoopHVAC:SupplyPlenum AirLoopHVAC:ReturnPlenum
Coil:Cooling:DX:VariableSpeed Coil:Heating:DX:VariableSpeed
Coil:Cooling:WaterToAirHeatPump:ParameterEstimation Coil:Heating:WaterToAirHeatPump:ParameterEstimation
Coil:Cooling:WaterToAirHeatPump:EquationFit Coil:Cooling:WaterToAirHeatPump:VariableSpeedEquationFit
Coil:Heating:WaterToAirHeatPump:EquationFit Coil:Heating:WaterToAirHeatPump:VariableSpeedEquationFit
Coil:WaterHeating:AirToWaterHeatPump:Pumped Coil:WaterHeating:AirToWaterHeatPump:Wrapped
Coil:WaterHeating:AirToWaterHeatPump:VariableSpeed CoilSystem:Cooling:Water:HeatExchangerAssisted
CoilSystem:Cooling:DX:HeatExchangerAssisted CoilSystem:IntegratedHeatPump:AirSource
Pipe:Adiabatic Pipe:Adiabatic:Steam Pipe:Indoor Pipe:Outdoor Pipe:Underground Duct
Pump:VariableSpeed Pump:ConstantSpeed Pump:VariableSpeed:Condensate HeaderedPumps:ConstantSpeed HeaderedPumps:VariableSpeed
Boiler:HotWater Boiler:Steam Chiller:Electric:ASHRAE205 Chiller:Electric:EIR Chiller:Electric:ReformulatedEIR
Chiller:Electric Chiller:Absorption:Indirect Chiller:Absorption Chiller:ConstantCOP Chiller:EngineDriven Chiller:CombustionTurbine
HeatPump:PlantLoop:EIR:Cooling HeatPump:PlantLoop:EIR:Heating HeatPump:AirToWater:FuelFired:Heating
HeatPump:AirToWater:FuelFired:Cooling HeatPump:WaterToWater:EquationFit:Heating HeatPump:WaterToWater:EquationFit:Cooling
HeatPump:WaterToWater:ParameterEstimation:Cooling HeatPump:WaterToWater:ParameterEstimation:Heating
DistrictCooling DistrictHeating DistrictHeating:Water DistrictHeating:Steam PlantComponent:TemperatureSource
CoolingTower:SingleSpeed CoolingTower:TwoSpeed CoolingTower:VariableSpeed:Merkel CoolingTower:VariableSpeed
FluidCooler:SingleSpeed FluidCooler:TwoSpeed EvaporativeFluidCooler:SingleSpeed EvaporativeFluidCooler:TwoSpeed
GroundHeatExchanger:System GroundHeatExchanger:Pond GroundHeatExchanger:Surface GroundHeatExchanger:HorizontalTrench GroundHeatExchanger:Slinky
WaterHeater:Mixed WaterHeater:Stratified ThermalStorage:Ice:Simple
SolarCollector:FlatPlate:Water SolarCollector:FlatPlate:PhotovoltaicThermal SolarCollector:IntegralCollectorStorage
`) {
		result[normalizeName(typ)] = true
	}
	return result
}()

func hvacOperatingAvailabilityFieldName(name string) bool {
	switch normalizeFieldName(name) {
	case normalizeFieldName("Availability Schedule Name"), normalizeFieldName("Availability Schedule"), normalizeFieldName("Terminal Unit Availability Schedule"):
		return true
	}
	return false
}

func hvacOperatingAvailabilitySchedule(obj Object) string {
	for index := range obj.Fields {
		if spec, ok := fieldSpecAt(obj.Type, index); ok && hvacOperatingAvailabilityFieldName(spec.Name) {
			return hvacFieldValue(obj, index)
		}
	}
	if index, ok := hvacOperatingAvailabilityFields[normalizeName(obj.Type)]; ok {
		return hvacFieldValue(obj, index)
	}
	return ""
}

func hvacOperatingAvailabilityKnown(obj Object) bool {
	if hvacOperatingNoAvailabilityField[normalizeName(obj.Type)] {
		return true
	}
	if _, ok := hvacOperatingAvailabilityFields[normalizeName(obj.Type)]; ok {
		return true
	}
	if spec, ok := objectFieldCatalog[normalizeFieldCatalogKey(obj.Type)]; ok {
		for _, field := range spec.Fields {
			if hvacOperatingAvailabilityFieldName(field.Name) {
				return true
			}
		}
	}
	return false
}

func hvacOperatingManagerListName(obj Object) string {
	// CondenserLoop has no such field: its field 19 is not a manager list.
	if strings.EqualFold(obj.Type, "CondenserLoop") {
		return ""
	}
	for index := range obj.Fields {
		if spec, ok := fieldSpecAt(obj.Type, index); ok && (strings.EqualFold(spec.Name, "Availability Manager List Name") || strings.EqualFold(spec.Name, "Availability Manager Assignment List Name")) {
			return hvacFieldValue(obj, index)
		}
	}
	if index, ok := hvacOperatingManagerFields[normalizeName(obj.Type)]; ok {
		return hvacFieldValue(obj, index)
	}
	return ""
}

// Control metadata is deliberately limited to the fields needed by the compact
// operating graph. Existing catalog fields retain priority over these fallbacks.
var hvacOperatingControlFieldNames = map[string]map[int]string{
	"setpointmanager:followsystemnodetemperature":       {1: "Control Variable", 2: "Reference Node Name", 3: "Reference Temperature Type", 4: "Offset Temperature Difference", 5: "Maximum Limit Setpoint Temperature", 6: "Minimum Limit Setpoint Temperature", 7: "Setpoint Node or NodeList Name"},
	"setpointmanager:singlezone:humidity:maximum":       {1: "Setpoint Node or NodeList Name", 2: "Control Zone Air Node Name"},
	"setpointmanager:singlezone:humidity:minimum":       {1: "Setpoint Node or NodeList Name", 2: "Control Zone Air Node Name"},
	"setpointmanager:multizone:maximumhumidity:average": {1: "HVAC Air Loop Name", 2: "Minimum Setpoint Humidity Ratio", 3: "Maximum Setpoint Humidity Ratio", 4: "Setpoint Node or NodeList Name"},
	"setpointmanager:multizone:minimumhumidity:average": {1: "HVAC Air Loop Name", 2: "Minimum Setpoint Humidity Ratio", 3: "Maximum Setpoint Humidity Ratio", 4: "Setpoint Node or NodeList Name"},
	"setpointmanager:multizone:humidity:maximum":        {1: "HVAC Air Loop Name", 2: "Minimum Setpoint Humidity Ratio", 3: "Maximum Setpoint Humidity Ratio", 4: "Setpoint Node or NodeList Name"},
	"setpointmanager:multizone:humidity:minimum":        {1: "HVAC Air Loop Name", 2: "Minimum Setpoint Humidity Ratio", 3: "Maximum Setpoint Humidity Ratio", 4: "Setpoint Node or NodeList Name"},
	"zonecontrol:thermostat":                            {1: "Zone or ZoneList Name", 2: "Control Type Schedule Name"},
	"thermostatsetpoint:singleheating":                  {1: "Setpoint Temperature Schedule Name"},
	"thermostatsetpoint:singlecooling":                  {1: "Setpoint Temperature Schedule Name"},
	"thermostatsetpoint:singleheatingorcooling":         {1: "Setpoint Temperature Schedule Name"},
	"thermostatsetpoint:dualsetpoint":                   {1: "Heating Setpoint Temperature Schedule Name", 2: "Cooling Setpoint Temperature Schedule Name"},
	// These manager references are declarations only. Runtime logic stays
	// unknown unless the existing expression evaluator explicitly supports it.
	// Schedule positions agree with EnergyPlus 22.1 IDD and 24.2 IO Reference.
	"availabilitymanager:scheduled":             {1: "Schedule Name"},
	"availabilitymanager:scheduledon":           {1: "Schedule Name"},
	"availabilitymanager:scheduledoff":          {1: "Schedule Name"},
	"availabilitymanager:nightcycle":            {1: "Applicability Schedule Name", 2: "Fan Schedule Name", 3: "Control Type", 4: "Thermostat Tolerance", 5: "Cycling Run Time Control Type", 6: "Cycling Run Time", 7: "Control Zone or Zone List Name", 8: "Cooling Control Zone or Zone List Name", 9: "Heating Control Zone or Zone List Name", 10: "Heating Zone Fans Only Zone or Zone List Name"},
	"availabilitymanager:optimumstart":          {1: "Applicability Schedule Name", 2: "Fan Schedule Name"},
	"availabilitymanager:nightventilation":      {1: "Applicability Schedule Name", 2: "Fan Schedule Name", 3: "Ventilation Temperature Schedule Name"},
	"availabilitymanager:lowtemperatureturnoff": {1: "Sensor Node Name", 2: "Temperature", 3: "Applicability Schedule Name"},
	"plantloop":                   {18: "Load Distribution Scheme", 19: "Availability Manager List Name", 20: "Plant Loop Demand Calculation Scheme", 21: "Common Pipe Simulation", 22: "Pressure Simulation Type"},
	"pump:variablespeed":          {3: "Design Maximum Flow Rate", 4: "Design Pump Head", 8: "Coefficient 1 of the Part Load Performance Curve", 9: "Coefficient 2 of the Part Load Performance Curve", 10: "Coefficient 3 of the Part Load Performance Curve", 11: "Coefficient 4 of the Part Load Performance Curve", 12: "Design Minimum Flow Rate", 13: "Pump Control Type", 14: "Pump Flow Rate Schedule Name", 15: "Pump Curve Name", 17: "VFD Control Type", 18: "Pump RPM Schedule Name", 19: "Minimum Pressure Schedule", 20: "Maximum Pressure Schedule", 21: "Minimum RPM Schedule", 22: "Maximum RPM Schedule", 28: "Design Minimum Flow Rate Fraction"},
	"pump:constantspeed":          {3: "Design Flow Rate", 4: "Design Pump Head", 8: "Pump Control Type", 9: "Pump Flow Rate Schedule Name", 10: "Pump Curve Name", 12: "Rotational Speed"},
	"headeredpumps:constantspeed": {3: "Total Design Flow Rate", 4: "Number of Pumps in Bank", 5: "Flow Sequencing Control Scheme", 6: "Design Pump Head", 10: "Pump Control Type", 11: "Pump Flow Rate Schedule Name"},
	"headeredpumps:variablespeed": {3: "Total Design Flow Rate", 4: "Number of Pumps in Bank", 5: "Flow Sequencing Control Scheme", 6: "Design Pump Head", 10: "Coefficient 1 of the Part Load Performance Curve", 11: "Coefficient 2 of the Part Load Performance Curve", 12: "Coefficient 3 of the Part Load Performance Curve", 13: "Coefficient 4 of the Part Load Performance Curve", 14: "Minimum Flow Rate Fraction", 15: "Pump Control Type", 16: "Pump Flow Rate Schedule Name"},
	"boiler:hotwater":             {2: "Nominal Capacity", 4: "Efficiency Curve Temperature Evaluation Variable", 5: "Normalized Boiler Efficiency Curve Name", 6: "Design Water Flow Rate", 7: "Minimum Part Load Ratio", 8: "Maximum Part Load Ratio", 9: "Optimum Part Load Ratio", 12: "Water Outlet Upper Temperature Limit", 13: "Boiler Flow Mode"},
	"chiller:electric:eir":        {7: "Cooling Capacity Function of Temperature Curve Name", 8: "Electric Input to Cooling Output Ratio Function of Temperature Curve Name", 9: "Electric Input to Cooling Output Ratio Function of Part Load Ratio Curve Name", 10: "Minimum Part Load Ratio", 11: "Maximum Part Load Ratio", 12: "Optimum Part Load Ratio", 13: "Minimum Unloading Ratio", 21: "Leaving Chilled Water Lower Temperature Limit", 22: "Chiller Flow Mode", 29: "Basin Heater Operating Schedule Name", 31: "Heat Recovery Inlet High Temperature Limit Schedule Name", 32: "Heat Recovery Leaving Temperature Setpoint Node Name", 34: "Condenser Flow Control", 35: "Condenser Loop Flow Rate Fraction Function of Loop Part Load Ratio Curve Name", 36: "Temperature Difference Across Condenser Schedule Name", 37: "Condenser Minimum Flow Fraction"},
	"waterheater:mixed":           {2: "Setpoint Temperature Schedule Name", 3: "Deadband Temperature Difference", 4: "Maximum Temperature Limit", 5: "Heater Control Type", 6: "Heater Maximum Capacity", 7: "Heater Minimum Capacity", 12: "Part Load Factor Curve Name", 20: "Ambient Temperature Schedule Name", 28: "Use Flow Rate Fraction Schedule Name", 29: "Cold Water Supply Temperature Schedule Name", 39: "Source Side Flow Control Mode", 40: "Indirect Alternate Setpoint Temperature Schedule Name"},
	"fan:zoneexhaust":             {9: "System Availability Manager Coupling Mode", 10: "Minimum Zone Temperature Limit Schedule Name"},
}

func hvacOperatingFieldName(obj Object, index int) string {
	if spec, ok := fieldSpecAt(obj.Type, index); ok && spec.Name != "" {
		return spec.Name
	}
	typ := normalizeName(obj.Type)
	if name := hvacOperatingControlFieldNames[typ][index]; name != "" {
		return name
	}
	if fieldIndex, ok := hvacOperatingAvailabilityFields[typ]; ok && index == fieldIndex {
		if typ == "zonehvac:terminalunit:variablerefrigerantflow" {
			return "Terminal Unit Availability Schedule"
		}
		return "Availability Schedule Name"
	}
	if fieldIndex, ok := hvacOperatingManagerFields[typ]; ok && index == fieldIndex {
		return "Availability Manager List Name"
	}
	return catalogFieldName(obj, index)
}
