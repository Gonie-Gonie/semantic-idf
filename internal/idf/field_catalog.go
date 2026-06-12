package idf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FieldSpec struct {
	Index               int
	Name                string
	Role                string
	TargetClass         string
	TargetCollection    string
	RelationshipType    string
	Choices             []string
	Numeric             bool
	AllowAutosize       bool
	Required            bool
	Units               string
	ExtensibleGroup     string
	ExtensibleGroupSize int
}

type ObjectSpec struct {
	Type         string
	Fields       []FieldSpec
	MinVersion   string
	MaxVersion   string
	SchemaSource string
}

type FieldValueSuggestion struct {
	Value  string `json:"value"`
	Label  string `json:"label,omitempty"`
	Source string `json:"source,omitempty"`
}

type schemaPropertySpec struct {
	Type        any      `json:"type"`
	Enum        []string `json:"enum"`
	Units       string   `json:"units"`
	Autosizable bool     `json:"autosizable"`
}

type schemaLegacyIDD struct {
	Fields    []string `json:"fields"`
	FieldInfo map[string]struct {
		FieldName string `json:"field_name"`
	} `json:"field_info"`
	Alphas struct {
		Fields []string `json:"fields"`
	} `json:"alphas"`
	Numerics struct {
		Fields []string `json:"fields"`
	} `json:"numerics"`
}

const (
	fieldRoleName            = "name"
	fieldRoleObjectRef       = "object_ref"
	fieldRoleObjectTypeRef   = "object_type_ref"
	fieldRoleNodeRef         = "node_ref"
	fieldRoleNodeListRef     = "node_list_ref"
	fieldRoleBranchRef       = "branch_ref"
	fieldRoleBranchListRef   = "branch_list_ref"
	fieldRoleConnectorRef    = "connector_ref"
	fieldRoleZoneRef         = "zone_ref"
	fieldRoleSpaceRef        = "space_ref"
	fieldRoleScheduleRef     = "schedule_ref"
	fieldRoleConstructionRef = "construction_ref"
)

var objectFieldCatalog = map[string]ObjectSpec{
	"zone": catalogObject("Zone",
		field("Name", fieldRoleName),
		numericUnitField("Direction of Relative North", "deg", false),
		numericUnitField("X Origin", "m", false),
		numericUnitField("Y Origin", "m", false),
		numericUnitField("Z Origin", "m", false),
		choiceField("Type", "", "1"),
		numericField("Multiplier", false),
		numericUnitField("Ceiling Height", "m", true),
		numericUnitField("Volume", "m3", true),
		numericUnitField("Floor Area", "m2", true),
		field("Zone Inside Convection Algorithm", ""),
		field("Zone Outside Convection Algorithm", ""),
		choiceField("Part of Total Floor Area", "", "Yes", "No"),
	),
	"space": catalogObject("Space",
		field("Name", fieldRoleName),
		targetField("Zone Name", fieldRoleZoneRef, "zone", "zones", "contains"),
		targetField("Space Type", fieldRoleObjectRef, "space_type", "space_types", "uses"),
		numericUnitField("Floor Area", "m2", true),
		numericUnitField("Volume", "m3", true),
	),
	"spacelist": catalogObject("SpaceList",
		field("Name", fieldRoleName),
		extensibleField("Space Name", fieldRoleSpaceRef, "spaces", 1),
	),
	"zonelist": catalogObject("ZoneList",
		field("Name", fieldRoleName),
		extensibleField("Zone Name", fieldRoleZoneRef, "zones", 1),
	),
	"zonegroup": catalogObject("ZoneGroup",
		field("Name", fieldRoleName),
		targetField("Zone List Name", fieldRoleObjectRef, "zone_list", "zone_lists", "expands"),
		numericField("Zone List Multiplier", false),
	),
	"building": catalogObject("Building",
		field("Name", fieldRoleName),
		numericUnitField("North Axis", "deg", false),
		field("Terrain", ""),
	),
	"globalgeometryrules": catalogObject("GlobalGeometryRules",
		choiceField("Starting Vertex Position", "", "UpperLeftCorner", "LowerLeftCorner", "UpperRightCorner", "LowerRightCorner"),
		choiceField("Vertex Entry Direction", "", "Clockwise", "Counterclockwise"),
		choiceField("Coordinate System", "", "Relative", "World"),
		choiceField("Daylighting Reference Point Coordinate System", "", "Relative", "World"),
		choiceField("Rectangular Surface Coordinate System", "", "Relative", "World"),
	),
	"buildingsurface:detailed": catalogObject("BuildingSurface:Detailed",
		field("Name", fieldRoleName),
		field("Surface Type", ""),
		targetField("Construction Name", fieldRoleConstructionRef, "construction", "constructions", "uses"),
		targetField("Zone Name", fieldRoleZoneRef, "zone", "zones", "contained_by"),
		targetField("Space Name", fieldRoleSpaceRef, "space", "spaces", "contained_by"),
		field("Outside Boundary Condition", ""),
		targetField("Outside Boundary Condition Object", fieldRoleObjectRef, "surface", "surfaces", "references"),
		choiceField("Sun Exposure", "", "SunExposed", "NoSun"),
		choiceField("Wind Exposure", "", "WindExposed", "NoWind"),
		numericField("View Factor to Ground", true),
		numericField("Number of Vertices", false),
	),
	"fenestrationsurface:detailed": catalogObject("FenestrationSurface:Detailed",
		field("Name", fieldRoleName),
		field("Surface Type", ""),
		targetField("Construction Name", fieldRoleConstructionRef, "construction", "constructions", "uses"),
		targetField("Building Surface Name", fieldRoleObjectRef, "surface", "surfaces", "subsurface_of"),
		numericField("View Factor to Ground", true),
		targetField("Frame and Divider Name", fieldRoleObjectRef, "frame_divider", "frames", "uses"),
		numericField("Multiplier", false),
		numericField("Number of Vertices", false),
	),
	"airloophvac": catalogObject("AirLoopHVAC",
		field("Name", fieldRoleName),
		field("Controller List Name", fieldRoleObjectRef),
		field("Availability Manager List Name", fieldRoleObjectRef),
		numericField("Design Supply Air Flow Rate", true),
		field("Branch List Name", fieldRoleBranchListRef),
		field("Connector List Name", fieldRoleConnectorRef),
		field("Supply Side Inlet Node Name", fieldRoleNodeRef),
		field("Demand Side Outlet Node Name", fieldRoleNodeRef),
		field("Demand Side Inlet Node Names", fieldRoleNodeRef),
		field("Supply Side Outlet Node Names", fieldRoleNodeRef),
	),
	"plantloop": catalogObject("PlantLoop",
		field("Name", fieldRoleName),
		choiceField("Fluid Type", "", "Water", "Steam", "UserDefined", "Glycol"),
		field("User Defined Fluid Type", ""),
		field("Plant Equipment Operation Scheme Name", fieldRoleObjectRef),
		field("Loop Temperature Setpoint Node Name", fieldRoleNodeRef),
		numericField("Maximum Loop Temperature", false),
		numericField("Minimum Loop Temperature", false),
		numericField("Maximum Loop Flow Rate", true),
		numericField("Minimum Loop Flow Rate", false),
		numericField("Plant Loop Volume", true),
		field("Plant Side Inlet Node Name", fieldRoleNodeRef),
		field("Plant Side Outlet Node Name", fieldRoleNodeRef),
		field("Plant Side Branch List Name", fieldRoleBranchListRef),
		field("Plant Side Connector List Name", fieldRoleConnectorRef),
		field("Demand Side Inlet Node Name", fieldRoleNodeRef),
		field("Demand Side Outlet Node Name", fieldRoleNodeRef),
		field("Demand Side Branch List Name", fieldRoleBranchListRef),
		field("Demand Side Connector List Name", fieldRoleConnectorRef),
	),
	"condenserloop": catalogObject("CondenserLoop",
		field("Name", fieldRoleName),
		choiceField("Fluid Type", "", "Water", "Steam", "UserDefined", "Glycol"),
		field("User Defined Fluid Type", ""),
		field("Condenser Equipment Operation Scheme Name", fieldRoleObjectRef),
		field("Condenser Loop Temperature Setpoint Node Name", fieldRoleNodeRef),
		numericField("Maximum Loop Temperature", false),
		numericField("Minimum Loop Temperature", false),
		numericField("Maximum Loop Flow Rate", true),
		numericField("Minimum Loop Flow Rate", false),
		numericField("Condenser Loop Volume", true),
		field("Condenser Side Inlet Node Name", fieldRoleNodeRef),
		field("Condenser Side Outlet Node Name", fieldRoleNodeRef),
		field("Condenser Side Branch List Name", fieldRoleBranchListRef),
		field("Condenser Side Connector List Name", fieldRoleConnectorRef),
		field("Demand Side Inlet Node Name", fieldRoleNodeRef),
		field("Demand Side Outlet Node Name", fieldRoleNodeRef),
		field("Demand Side Branch List Name", fieldRoleBranchListRef),
		field("Demand Side Connector List Name", fieldRoleConnectorRef),
	),
	"branchlist": catalogObject("BranchList",
		field("Name", fieldRoleName),
		extensibleField("Branch Name", fieldRoleBranchRef, "branches", 1),
	),
	"branch": catalogObject("Branch",
		field("Name", fieldRoleName),
		field("Pressure Drop Curve Name", fieldRoleObjectRef),
		extensibleField("Component Object Type", fieldRoleObjectTypeRef, "components", 4),
		extensibleField("Component Name", fieldRoleObjectRef, "components", 4),
		extensibleField("Component Inlet Node Name", fieldRoleNodeRef, "components", 4),
		extensibleField("Component Outlet Node Name", fieldRoleNodeRef, "components", 4),
	),
	"connectorlist": catalogObject("ConnectorList",
		field("Name", fieldRoleName),
		extensibleField("Connector Object Type", fieldRoleObjectTypeRef, "connectors", 2),
		extensibleField("Connector Name", fieldRoleObjectRef, "connectors", 2),
	),
	"connector:splitter": catalogObject("Connector:Splitter",
		field("Name", fieldRoleName),
		field("Inlet Branch Name", fieldRoleBranchRef),
		extensibleField("Outlet Branch Name", fieldRoleBranchRef, "branches", 1),
	),
	"connector:mixer": catalogObject("Connector:Mixer",
		field("Name", fieldRoleName),
		field("Outlet Branch Name", fieldRoleBranchRef),
		extensibleField("Inlet Branch Name", fieldRoleBranchRef, "branches", 1),
	),
	"airloophvac:supplypath": catalogObject("AirLoopHVAC:SupplyPath",
		field("Name", fieldRoleName),
		field("Supply Air Path Inlet Node Name", fieldRoleNodeRef),
		extensibleField("Component Object Type", fieldRoleObjectTypeRef, "components", 2),
		extensibleField("Component Name", fieldRoleObjectRef, "components", 2),
	),
	"airloophvac:returnpath": catalogObject("AirLoopHVAC:ReturnPath",
		field("Name", fieldRoleName),
		field("Return Air Path Outlet Node Name", fieldRoleNodeRef),
		extensibleField("Component Object Type", fieldRoleObjectTypeRef, "components", 2),
		extensibleField("Component Name", fieldRoleObjectRef, "components", 2),
	),
	"airloophvac:zonesplitter": catalogObject("AirLoopHVAC:ZoneSplitter",
		field("Name", fieldRoleName),
		field("Inlet Node Name", fieldRoleNodeRef),
		extensibleField("Outlet Node Name", fieldRoleNodeRef, "zone_inlets", 1),
	),
	"airloophvac:zonemixer": catalogObject("AirLoopHVAC:ZoneMixer",
		field("Name", fieldRoleName),
		field("Outlet Node Name", fieldRoleNodeRef),
		extensibleField("Inlet Node Name", fieldRoleNodeRef, "zone_returns", 1),
	),
	"airloophvac:supplyplenum": catalogObject("AirLoopHVAC:SupplyPlenum",
		field("Name", fieldRoleName),
		targetField("Zone Name", fieldRoleZoneRef, "zone", "zones", "serves"),
		field("Zone Node Name", fieldRoleNodeRef),
		field("Inlet Node Name", fieldRoleNodeRef),
		extensibleField("Outlet Node Name", fieldRoleNodeRef, "zone_inlets", 1),
	),
	"airloophvac:returnplenum": catalogObject("AirLoopHVAC:ReturnPlenum",
		field("Name", fieldRoleName),
		targetField("Zone Name", fieldRoleZoneRef, "zone", "zones", "serves"),
		field("Zone Node Name", fieldRoleNodeRef),
		field("Outlet Node Name", fieldRoleNodeRef),
		extensibleField("Inlet Node Name", fieldRoleNodeRef, "zone_returns", 1),
	),
	"outdoorair:mixer": catalogObject("OutdoorAir:Mixer",
		field("Name", fieldRoleName),
		field("Mixed Air Node Name", fieldRoleNodeRef),
		field("Outdoor Air Stream Node Name", fieldRoleNodeRef),
		field("Relief Air Stream Node Name", fieldRoleNodeRef),
		field("Return Air Stream Node Name", fieldRoleNodeRef),
	),
	"coil:heating:steam": catalogObject("Coil:Heating:Steam",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Maximum Steam Flow Rate", "m3/s", true),
		numericUnitField("Degree of SubCooling", "C", false),
		numericUnitField("Degree of Loop SubCooling", "C", false),
		field("Water Inlet Node Name", fieldRoleNodeRef),
		field("Water Outlet Node Name", fieldRoleNodeRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		choiceField("Coil Control Type", "", "TemperatureSetpointControl", "ZoneLoadControl"),
		field("Temperature Setpoint Node Name", fieldRoleNodeRef),
	),
	"coil:cooling:dx:singlespeed": catalogObject("Coil:Cooling:DX:SingleSpeed",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Gross Rated Total Cooling Capacity", "W", true),
		numericField("Gross Rated Sensible Heat Ratio", true),
		numericUnitField("Gross Rated Cooling COP", "W/W", false),
		numericUnitField("Rated Air Flow Rate", "m3/s", true),
		numericUnitField("2017 Rated Evaporator Fan Power Per Volume Flow Rate", "W/(m3/s)", false),
		numericUnitField("2023 Rated Evaporator Fan Power Per Volume Flow Rate", "W/(m3/s)", false),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
	),
	"coil:heating:dx:singlespeed": catalogObject("Coil:Heating:DX:SingleSpeed",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Gross Rated Heating Capacity", "W", true),
		numericUnitField("Gross Rated Heating COP", "W/W", false),
		numericUnitField("Rated Air Flow Rate", "m3/s", true),
		numericUnitField("2017 Rated Supply Fan Power Per Volume Flow Rate", "W/(m3/s)", false),
		numericUnitField("2023 Rated Supply Fan Power Per Volume Flow Rate", "W/(m3/s)", false),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
	),
	"coil:cooling:dx:variablerefrigerantflow": catalogObject("Coil:Cooling:DX:VariableRefrigerantFlow",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Gross Rated Total Cooling Capacity", "W", true),
		numericField("Gross Rated Sensible Heat Ratio", true),
		numericUnitField("Rated Air Flow Rate", "m3/s", true),
		field("Cooling Capacity Ratio Modifier Function of Temperature Curve Name", fieldRoleObjectRef),
		field("Cooling Capacity Modifier Curve Function of Flow Fraction Name", fieldRoleObjectRef),
		field("Coil Air Inlet Node", fieldRoleNodeRef),
		field("Coil Air Outlet Node", fieldRoleNodeRef),
		field("Name of Water Storage Tank for Condensate Collection", fieldRoleObjectRef),
	),
	"coil:heating:dx:variablerefrigerantflow": catalogObject("Coil:Heating:DX:VariableRefrigerantFlow",
		field("Name", fieldRoleName),
		field("Availability Schedule", fieldRoleScheduleRef),
		numericUnitField("Gross Rated Heating Capacity", "W", true),
		numericUnitField("Rated Air Flow Rate", "m3/s", true),
		field("Coil Air Inlet Node", fieldRoleNodeRef),
		field("Coil Air Outlet Node", fieldRoleNodeRef),
		field("Heating Capacity Ratio Modifier Function of Temperature Curve Name", fieldRoleObjectRef),
		field("Heating Capacity Modifier Function of Flow Fraction Curve Name", fieldRoleObjectRef),
	),
	"coilsystem:cooling:dx": catalogObject("CoilSystem:Cooling:DX",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("DX Cooling Coil System Inlet Node Name", fieldRoleNodeRef),
		field("DX Cooling Coil System Outlet Node Name", fieldRoleNodeRef),
		field("DX Cooling Coil System Sensor Node Name", fieldRoleNodeRef),
		field("Cooling Coil Object Type", fieldRoleObjectTypeRef),
		field("Cooling Coil Name", fieldRoleObjectRef),
	),
	"coilsystem:heating:dx": catalogObject("CoilSystem:Heating:DX",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Heating Coil Name", fieldRoleObjectRef),
	),
	"airloophvac:unitarysystem": catalogObject("AirLoopHVAC:UnitarySystem",
		field("Name", fieldRoleName),
		choiceField("Control Type", "", "Load", "SetPoint", "SingleZoneVAV"),
		field("Controlling Zone or Thermostat Location", fieldRoleZoneRef),
		field("Dehumidification Control Type", ""),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Supply Fan Object Type", fieldRoleObjectTypeRef),
		field("Supply Fan Name", fieldRoleObjectRef),
		choiceField("Fan Placement", "", "BlowThrough", "DrawThrough"),
		field("Supply Air Fan Operating Mode Schedule Name", fieldRoleScheduleRef),
		field("Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Heating Coil Name", fieldRoleObjectRef),
		numericField("DX Heating Coil Sizing Ratio", false),
		field("Cooling Coil Object Type", fieldRoleObjectTypeRef),
		field("Cooling Coil Name", fieldRoleObjectRef),
		choiceField("Use DOAS DX Cooling Coil", "", "Yes", "No"),
		numericUnitField("Minimum Supply Air Temperature", "C", false),
		field("Latent Load Control", ""),
		field("Supplemental Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Supplemental Heating Coil Name", fieldRoleObjectRef),
	),
	"airloophvac:unitaryheatcool": catalogObject("AirLoopHVAC:UnitaryHeatCool",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Unitary System Air Inlet Node Name", fieldRoleNodeRef),
		field("Unitary System Air Outlet Node Name", fieldRoleNodeRef),
		field("Supply Air Fan Operating Mode Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Maximum Supply Air Temperature", "C", false),
		numericUnitField("Cooling Supply Air Flow Rate", "m3/s", true),
		numericUnitField("Heating Supply Air Flow Rate", "m3/s", true),
		numericUnitField("No Load Supply Air Flow Rate", "m3/s", true),
		field("Controlling Zone or Thermostat Location", fieldRoleZoneRef),
		field("Supply Fan Object Type", fieldRoleObjectTypeRef),
		field("Supply Fan Name", fieldRoleObjectRef),
		choiceField("Fan Placement", "", "BlowThrough", "DrawThrough"),
		field("Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Heating Coil Name", fieldRoleObjectRef),
		field("Cooling Coil Object Type", fieldRoleObjectTypeRef),
		field("Cooling Coil Name", fieldRoleObjectRef),
		field("Dehumidification Control Type", ""),
		field("Reheat Coil Object Type", fieldRoleObjectTypeRef),
		field("Reheat Coil Name", fieldRoleObjectRef),
	),
	"airloophvac:unitary:furnace:heatcool": catalogObject("AirLoopHVAC:Unitary:Furnace:HeatCool",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Furnace Air Inlet Node Name", fieldRoleNodeRef),
		field("Furnace Air Outlet Node Name", fieldRoleNodeRef),
		field("Supply Air Fan Operating Mode Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Maximum Supply Air Temperature", "C", false),
		numericUnitField("Cooling Supply Air Flow Rate", "m3/s", true),
		numericUnitField("Heating Supply Air Flow Rate", "m3/s", true),
		numericUnitField("No Load Supply Air Flow Rate", "m3/s", true),
		field("Controlling Zone or Thermostat Location", fieldRoleZoneRef),
		field("Supply Fan Object Type", fieldRoleObjectTypeRef),
		field("Supply Fan Name", fieldRoleObjectRef),
		choiceField("Fan Placement", "", "BlowThrough", "DrawThrough"),
		field("Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Heating Coil Name", fieldRoleObjectRef),
		field("Cooling Coil Object Type", fieldRoleObjectTypeRef),
		field("Cooling Coil Name", fieldRoleObjectRef),
		field("Dehumidification Control Type", ""),
		field("Reheat Coil Object Type", fieldRoleObjectTypeRef),
		field("Reheat Coil Name", fieldRoleObjectRef),
	),
	"airloophvac:unitaryheatpump:airtoair": catalogObject("AirLoopHVAC:UnitaryHeatPump:AirToAir",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Cooling Supply Air Flow Rate", "m3/s", true),
		numericUnitField("Heating Supply Air Flow Rate", "m3/s", true),
		numericUnitField("No Load Supply Air Flow Rate", "m3/s", true),
		field("Controlling Zone or Thermostat Location", fieldRoleZoneRef),
		field("Supply Air Fan Object Type", fieldRoleObjectTypeRef),
		field("Supply Air Fan Name", fieldRoleObjectRef),
		field("Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Heating Coil Name", fieldRoleObjectRef),
		field("Cooling Coil Object Type", fieldRoleObjectTypeRef),
		field("Cooling Coil Name", fieldRoleObjectRef),
		field("Supplemental Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Supplemental Heating Coil Name", fieldRoleObjectRef),
	),
	"chiller:electric:eir": catalogObject("Chiller:Electric:EIR",
		field("Name", fieldRoleName),
		numericUnitField("Reference Capacity", "W", true),
		numericUnitField("Reference COP", "W/W", false),
		numericUnitField("Reference Leaving Chilled Water Temperature", "C", false),
		numericUnitField("Reference Entering Condenser Fluid Temperature", "C", false),
		numericUnitField("Reference Chilled Water Flow Rate", "m3/s", true),
		numericUnitField("Reference Condenser Fluid Flow Rate", "m3/s", true),
		field("Cooling Capacity Function of Temperature Curve Name", fieldRoleObjectRef),
		field("Electric Input to Cooling Output Ratio Function of Temperature Curve Name", fieldRoleObjectRef),
		field("Electric Input to Cooling Output Ratio Function of Part Load Ratio Curve Name", fieldRoleObjectRef),
		numericField("Minimum Part Load Ratio", false),
		numericField("Maximum Part Load Ratio", false),
		numericField("Optimum Part Load Ratio", false),
		numericField("Minimum Unloading Ratio", false),
		field("Chilled Water Inlet Node Name", fieldRoleNodeRef),
		field("Chilled Water Outlet Node Name", fieldRoleNodeRef),
		field("Condenser Inlet Node Name", fieldRoleNodeRef),
		field("Condenser Outlet Node Name", fieldRoleNodeRef),
	),
	"boiler:hotwater": catalogObject("Boiler:HotWater",
		field("Name", fieldRoleName),
		choiceField("Fuel Type", "", "NaturalGas", "Electricity", "Propane", "FuelOilNo1", "FuelOilNo2", "OtherFuel1", "OtherFuel2"),
		numericUnitField("Nominal Capacity", "W", true),
		numericField("Nominal Thermal Efficiency", false),
		choiceField("Efficiency Curve Temperature Evaluation Variable", "", "EnteringBoiler", "LeavingBoiler"),
		field("Normalized Boiler Efficiency Curve Name", fieldRoleObjectRef),
		numericUnitField("Design Water Flow Rate", "m3/s", true),
		numericField("Minimum Part Load Ratio", false),
		numericField("Maximum Part Load Ratio", false),
		numericField("Optimum Part Load Ratio", false),
		field("Boiler Water Inlet Node Name", fieldRoleNodeRef),
		field("Boiler Water Outlet Node Name", fieldRoleNodeRef),
	),
	"heatpump:plantloop:eir:heating": catalogObject("HeatPump:PlantLoop:EIR:Heating",
		field("Name", fieldRoleName),
		field("Load Side Inlet Node Name", fieldRoleNodeRef),
		field("Load Side Outlet Node Name", fieldRoleNodeRef),
		field("Condenser Type", ""),
		field("Source Side Inlet Node Name", fieldRoleNodeRef),
		field("Source Side Outlet Node Name", fieldRoleNodeRef),
		field("Heat Recovery Inlet Node Name", fieldRoleNodeRef),
		field("Heat Recovery Outlet Node Name", fieldRoleNodeRef),
		field("Companion Heat Pump Name", fieldRoleObjectRef),
	),
	"heatpump:plantloop:eir:cooling": catalogObject("HeatPump:PlantLoop:EIR:Cooling",
		field("Name", fieldRoleName),
		field("Load Side Inlet Node Name", fieldRoleNodeRef),
		field("Load Side Outlet Node Name", fieldRoleNodeRef),
		field("Condenser Type", ""),
		field("Source Side Inlet Node Name", fieldRoleNodeRef),
		field("Source Side Outlet Node Name", fieldRoleNodeRef),
		field("Heat Recovery Inlet Node Name", fieldRoleNodeRef),
		field("Heat Recovery Outlet Node Name", fieldRoleNodeRef),
		field("Companion Heat Pump Name", fieldRoleObjectRef),
	),
	"coolingtower:singlespeed": catalogObject("CoolingTower:SingleSpeed",
		field("Name", fieldRoleName),
		field("Water Inlet Node Name", fieldRoleNodeRef),
		field("Water Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Design Water Flow Rate", "m3/s", true),
		numericUnitField("Design Air Flow Rate", "m3/s", true),
	),
	"heatexchanger:fluidtofluid": catalogObject("HeatExchanger:FluidToFluid",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Loop Demand Side Inlet Node Name", fieldRoleNodeRef),
		field("Loop Demand Side Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Loop Demand Side Design Flow Rate", "m3/s", true),
		field("Loop Supply Side Inlet Node Name", fieldRoleNodeRef),
		field("Loop Supply Side Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Loop Supply Side Design Flow Rate", "m3/s", true),
	),
	"districtcooling": catalogObject("DistrictCooling",
		field("Name", fieldRoleName),
		field("Chilled Water Inlet Node Name", fieldRoleNodeRef),
		field("Chilled Water Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Nominal Capacity", "W", true),
		field("Capacity Fraction Schedule Name", fieldRoleScheduleRef),
	),
	"districtheating": catalogObject("DistrictHeating",
		field("Name", fieldRoleName),
		field("Hot Water Inlet Node Name", fieldRoleNodeRef),
		field("Hot Water Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Nominal Capacity", "W", true),
		field("Capacity Fraction Schedule Name", fieldRoleScheduleRef),
	),
	"districtheating:water": catalogObject("DistrictHeating:Water",
		field("Name", fieldRoleName),
		field("Hot Water Inlet Node Name", fieldRoleNodeRef),
		field("Hot Water Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Nominal Capacity", "W", true),
		field("Capacity Fraction Schedule Name", fieldRoleScheduleRef),
	),
	"groundheatexchanger:system": catalogObject("GroundHeatExchanger:System",
		field("Name", fieldRoleName),
		field("Inlet Node Name", fieldRoleNodeRef),
		field("Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Design Flow Rate", "m3/s", true),
		numericField("Design Flow Rate Sizing Factor", false),
		field("Undisturbed Ground Temperature Model Name", fieldRoleObjectRef),
	),
	"fan:constantvolume": catalogObject("Fan:ConstantVolume",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		numericField("Fan Total Efficiency", false),
		numericUnitField("Pressure Rise", "Pa", false),
		numericUnitField("Maximum Flow Rate", "m3/s", true),
		numericField("Motor Efficiency", false),
		numericField("Motor In Airstream Fraction", false),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
	),
	"fan:onoff": catalogObject("Fan:OnOff",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		numericField("Fan Total Efficiency", false),
		numericUnitField("Pressure Rise", "Pa", false),
		numericUnitField("Maximum Flow Rate", "m3/s", true),
		numericField("Motor Efficiency", false),
		numericField("Motor In Airstream Fraction", false),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
	),
	"fan:systemmodel": catalogObject("Fan:SystemModel",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Design Maximum Air Flow Rate", "m3/s", true),
		choiceField("Speed Control Method", "", "Discrete", "Continuous"),
	),
	"pump:variablespeed": catalogObject("Pump:VariableSpeed",
		field("Name", fieldRoleName),
		field("Inlet Node Name", fieldRoleNodeRef),
		field("Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Design Maximum Flow Rate", "m3/s", true),
	),
	"pump:constantspeed": catalogObject("Pump:ConstantSpeed",
		field("Name", fieldRoleName),
		field("Inlet Node Name", fieldRoleNodeRef),
		field("Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Design Flow Rate", "m3/s", true),
	),
	"pipe:adiabatic": catalogObject("Pipe:Adiabatic",
		field("Name", fieldRoleName),
		field("Inlet Node Name", fieldRoleNodeRef),
		field("Outlet Node Name", fieldRoleNodeRef),
	),
	"duct": catalogObject("Duct",
		field("Name", fieldRoleName),
		field("Inlet Node Name", fieldRoleNodeRef),
		field("Outlet Node Name", fieldRoleNodeRef),
	),
	"setpointmanager:scheduled": catalogObject("SetpointManager:Scheduled",
		field("Name", fieldRoleName),
		choiceField("Control Variable", "", "Temperature", "MaximumTemperature", "MinimumTemperature", "HumidityRatio", "MassFlowRate"),
		field("Schedule Name", fieldRoleScheduleRef),
		field("Setpoint Node or NodeList Name", fieldRoleNodeListRef),
	),
	"setpointmanager:mixedair": catalogObject("SetpointManager:MixedAir",
		field("Name", fieldRoleName),
		choiceField("Control Variable", "", "Temperature", "MaximumTemperature", "MinimumTemperature"),
		field("Reference Setpoint Node Name", fieldRoleNodeRef),
		field("Fan Inlet Node Name", fieldRoleNodeRef),
		field("Fan Outlet Node Name", fieldRoleNodeRef),
		field("Setpoint Node or NodeList Name", fieldRoleNodeListRef),
		field("Cooling Coil Inlet Node Name", fieldRoleNodeRef),
		field("Cooling coil Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Minimum Temperature at Cooling Coil Outlet Node", "C", false),
	),
	"controller:watercoil": catalogObject("Controller:WaterCoil",
		field("Name", fieldRoleName),
		choiceField("Control Variable", "", "Temperature", "HumidityRatio", "TemperatureAndHumidityRatio", "Flow"),
		choiceField("Action", "", "Normal", "Reverse"),
		choiceField("Actuator Variable", "", "Flow"),
		field("Sensor Node Name", fieldRoleNodeRef),
		field("Actuator Node Name", fieldRoleNodeRef),
		numericField("Controller Convergence Tolerance", false),
		numericUnitField("Maximum Actuated Flow", "m3/s", true),
		numericUnitField("Minimum Actuated Flow", "m3/s", false),
	),
	"availabilitymanagerassignmentlist": catalogObject("AvailabilityManagerAssignmentList",
		field("Name", fieldRoleName),
		extensibleField("Availability Manager Object Type", fieldRoleObjectTypeRef, "availability_managers", 2),
		extensibleField("Availability Manager Name", fieldRoleObjectRef, "availability_managers", 2),
	),
	"zonehvac:airdistributionunit": catalogObject("ZoneHVAC:AirDistributionUnit",
		field("Name", fieldRoleName),
		field("Air Distribution Unit Outlet Node Name", fieldRoleNodeRef),
		field("Air Terminal Object Type", fieldRoleObjectTypeRef),
		field("Air Terminal Name", fieldRoleObjectRef),
	),
	"airterminal:singleduct:constantvolume:noreheat": catalogObject("AirTerminal:SingleDuct:ConstantVolume:NoReheat",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Maximum Air Flow Rate", "m3/s", true),
		field("Design Specification Outdoor Air Object Name", fieldRoleObjectRef),
		choiceField("Per Person Ventilation Rate Mode", "", "CurrentOccupancy", "DesignOccupancy"),
	),
	"airterminal:singleduct:constantvolume:reheat": catalogObject("AirTerminal:SingleDuct:ConstantVolume:Reheat",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		numericUnitField("Maximum Air Flow Rate", "m3/s", true),
		field("Reheat Coil Object Type", fieldRoleObjectTypeRef),
		field("Reheat Coil Name", fieldRoleObjectRef),
		numericUnitField("Maximum Hot Water or Steam Flow Rate", "m3/s", true),
		numericUnitField("Minimum Hot Water or Steam Flow Rate", "m3/s", false),
		numericField("Convergence Tolerance", false),
		numericUnitField("Maximum Reheat Air Temperature", "C", false),
	),
	"airterminal:singleduct:vav:noreheat": catalogObject("AirTerminal:SingleDuct:VAV:NoReheat",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		numericUnitField("Maximum Air Flow Rate", "m3/s", true),
		choiceField("Zone Minimum Air Flow Input Method", "", "Constant", "FixedFlowRate", "Scheduled"),
		numericField("Constant Minimum Air Flow Fraction", true),
		numericUnitField("Fixed Minimum Air Flow Rate", "m3/s", true),
		field("Minimum Air Flow Fraction Schedule Name", fieldRoleScheduleRef),
		field("Design Specification Outdoor Air Object Name", fieldRoleObjectRef),
		field("Minimum Air Flow Turndown Schedule Name", fieldRoleScheduleRef),
	),
	"airterminal:singleduct:vav:reheat": catalogObject("AirTerminal:SingleDuct:VAV:Reheat",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Damper Air Outlet Node Name", fieldRoleNodeRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		numericUnitField("Maximum Air Flow Rate", "m3/s", true),
		choiceField("Zone Minimum Air Flow Input Method", "", "Constant", "FixedFlowRate", "Scheduled"),
		numericField("Constant Minimum Air Flow Fraction", true),
		numericUnitField("Fixed Minimum Air Flow Rate", "m3/s", true),
		field("Minimum Air Flow Fraction Schedule Name", fieldRoleScheduleRef),
		field("Reheat Coil Object Type", fieldRoleObjectTypeRef),
		field("Reheat Coil Name", fieldRoleObjectRef),
		numericUnitField("Maximum Hot Water or Steam Flow Rate", "m3/s", true),
		numericUnitField("Minimum Hot Water or Steam Flow Rate", "m3/s", false),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		numericField("Convergence Tolerance", false),
		choiceField("Damper Heating Action", "", "Normal", "Reverse", "ReverseWithLimits"),
		numericUnitField("Maximum Flow per Zone Floor Area During Reheat", "m3/s-m2", true),
		numericField("Maximum Flow Fraction During Reheat", true),
		numericUnitField("Maximum Reheat Air Temperature", "C", false),
		field("Design Specification Outdoor Air Object Name", fieldRoleObjectRef),
		field("Minimum Air Flow Turndown Schedule Name", fieldRoleScheduleRef),
	),
	"airterminal:singleduct:vav:heatandcool:noreheat": catalogObject("AirTerminal:SingleDuct:VAV:HeatAndCool:NoReheat",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		numericUnitField("Maximum Air Flow Rate", "m3/s", true),
		numericField("Zone Minimum Air Flow Fraction", false),
		field("Minimum Air Flow Turndown Schedule Name", fieldRoleScheduleRef),
	),
	"airterminal:singleduct:vav:heatandcool:reheat": catalogObject("AirTerminal:SingleDuct:VAV:HeatAndCool:Reheat",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Damper Air Outlet Node Name", fieldRoleNodeRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		numericUnitField("Maximum Air Flow Rate", "m3/s", true),
		numericField("Zone Minimum Air Flow Fraction", false),
		field("Reheat Coil Object Type", fieldRoleObjectTypeRef),
		field("Reheat Coil Name", fieldRoleObjectRef),
		numericUnitField("Maximum Hot Water or Steam Flow Rate", "m3/s", true),
		numericUnitField("Minimum Hot Water or Steam Flow Rate", "m3/s", false),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		numericField("Convergence Tolerance", false),
		numericUnitField("Maximum Reheat Air Temperature", "C", false),
		field("Minimum Air Flow Turndown Schedule Name", fieldRoleScheduleRef),
	),
	"airterminal:singleduct:seriespiu:reheat": catalogObject("AirTerminal:SingleDuct:SeriesPIU:Reheat",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Maximum Air Flow Rate", "m3/s", true),
		numericUnitField("Maximum Primary Air Flow Rate", "m3/s", true),
		numericField("Minimum Primary Air Flow Fraction", true),
		field("Supply Air Inlet Node Name", fieldRoleNodeRef),
		field("Secondary Air Inlet Node Name", fieldRoleNodeRef),
		field("Outlet Node Name", fieldRoleNodeRef),
		field("Reheat Coil Air Inlet Node Name", fieldRoleNodeRef),
		field("Zone Mixer Name", fieldRoleObjectRef),
		field("Fan Name", fieldRoleObjectRef),
		field("Reheat Coil Object Type", fieldRoleObjectTypeRef),
		field("Reheat Coil Name", fieldRoleObjectRef),
		numericUnitField("Maximum Hot Water or Steam Flow Rate", "m3/s", true),
		numericUnitField("Minimum Hot Water or Steam Flow Rate", "m3/s", false),
		numericField("Convergence Tolerance", false),
		choiceField("Fan Control Type", "", "VariableSpeed", "ConstantSpeed"),
		numericField("Minimum Fan Turn Down Ratio", false),
		choiceField("Heating Control Type", "", "Staged", "Modulated"),
		numericUnitField("Design Heating Discharge Air Temperature", "C", false),
		numericUnitField("High Limit Heating Discharge Air Temperature", "C", false),
	),
	"airterminal:singleduct:parallelpiu:reheat": catalogObject("AirTerminal:SingleDuct:ParallelPIU:Reheat",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Maximum Primary Air Flow Rate", "m3/s", true),
		numericUnitField("Maximum Secondary Air Flow Rate", "m3/s", true),
		numericField("Minimum Primary Air Flow Fraction", true),
		numericField("Fan On Flow Fraction", true),
		field("Supply Air Inlet Node Name", fieldRoleNodeRef),
		field("Secondary Air Inlet Node Name", fieldRoleNodeRef),
		field("Outlet Node Name", fieldRoleNodeRef),
		field("Reheat Coil Air Inlet Node Name", fieldRoleNodeRef),
		field("Zone Mixer Name", fieldRoleObjectRef),
		field("Fan Name", fieldRoleObjectRef),
		field("Reheat Coil Object Type", fieldRoleObjectTypeRef),
		field("Reheat Coil Name", fieldRoleObjectRef),
		numericUnitField("Maximum Hot Water or Steam Flow Rate", "m3/s", true),
		numericUnitField("Minimum Hot Water or Steam Flow Rate", "m3/s", false),
		numericField("Convergence Tolerance", false),
		choiceField("Fan Control Type", "", "VariableSpeed", "ConstantSpeed"),
		numericField("Minimum Fan Turn Down Ratio", false),
		choiceField("Heating Control Type", "", "Staged", "Modulated"),
		numericUnitField("Design Heating Discharge Air Temperature", "C", false),
		numericUnitField("High Limit Heating Discharge Air Temperature", "C", false),
		field("Backdraft Damper Leakage Fraction Curve Name", fieldRoleObjectRef),
		targetField("Backdraft Damper Leakage Zone Name", fieldRoleZoneRef, "zone", "zones", "references"),
	),
	"airterminal:singleduct:constantvolume:fourpipeinduction": catalogObject("AirTerminal:SingleDuct:ConstantVolume:FourPipeInduction",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Maximum Total Air Flow Rate", "m3/s", true),
		numericField("Induction Ratio", false),
		field("Supply Air Inlet Node Name", fieldRoleNodeRef),
		field("Induced Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Heating Coil Name", fieldRoleObjectRef),
		numericUnitField("Maximum Hot Water Flow Rate", "m3/s", true),
		numericUnitField("Minimum Hot Water Flow Rate", "m3/s", false),
		numericField("Heating Convergence Tolerance", false),
		field("Cooling Coil Object Type", fieldRoleObjectTypeRef),
		field("Cooling Coil Name", fieldRoleObjectRef),
		numericUnitField("Maximum Cold Water Flow Rate", "m3/s", true),
		numericUnitField("Minimum Cold Water Flow Rate", "m3/s", false),
		numericField("Cooling Convergence Tolerance", false),
		field("Zone Mixer Name", fieldRoleObjectRef),
	),
	"airterminal:singleduct:constantvolume:fourpipebeam": catalogObject("AirTerminal:SingleDuct:ConstantVolume:FourPipeBeam",
		field("Name", fieldRoleName),
		field("Primary Air Availability Schedule Name", fieldRoleScheduleRef),
		field("Cooling Availability Schedule Name", fieldRoleScheduleRef),
		field("Heating Availability Schedule Name", fieldRoleScheduleRef),
		field("Primary Air Inlet Node Name", fieldRoleNodeRef),
		field("Primary Air Outlet Node Name", fieldRoleNodeRef),
		field("Chilled Water Inlet Node Name", fieldRoleNodeRef),
		field("Chilled Water Outlet Node Name", fieldRoleNodeRef),
		field("Hot Water Inlet Node Name", fieldRoleNodeRef),
		field("Hot Water Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Design Primary Air Volume Flow Rate", "m3/s", true),
		numericUnitField("Design Chilled Water Volume Flow Rate", "m3/s", true),
		numericUnitField("Design Hot Water Volume Flow Rate", "m3/s", true),
		numericUnitField("Zone Total Beam Length", "m", true),
		numericUnitField("Rated Primary Air Flow Rate per Beam Length", "m3/s-m", false),
		numericUnitField("Beam Rated Cooling Capacity per Beam Length", "W/m", false),
		numericUnitField("Beam Rated Cooling Room Air Chilled Water Temperature Difference", "deltaC", false),
		numericUnitField("Beam Rated Chilled Water Volume Flow Rate per Beam Length", "m3/s-m", false),
		field("Beam Cooling Capacity Temperature Difference Modification Factor Curve Name", fieldRoleObjectRef),
		field("Beam Cooling Capacity Air Flow Modification Factor Curve Name", fieldRoleObjectRef),
		field("Beam Cooling Capacity Chilled Water Flow Modification Factor Curve Name", fieldRoleObjectRef),
		numericUnitField("Beam Rated Heating Capacity per Beam Length", "W/m", false),
		numericUnitField("Beam Rated Heating Room Air Hot Water Temperature Difference", "deltaC", false),
		numericUnitField("Beam Rated Hot Water Volume Flow Rate per Beam Length", "m3/s-m", false),
		field("Beam Heating Capacity Temperature Difference Modification Factor Curve Name", fieldRoleObjectRef),
		field("Beam Heating Capacity Air Flow Modification Factor Curve Name", fieldRoleObjectRef),
		field("Beam Heating Capacity Hot Water Flow Modification Factor Curve Name", fieldRoleObjectRef),
	),
	"airterminal:singleduct:mixer": catalogObject("AirTerminal:SingleDuct:Mixer",
		field("Name", fieldRoleName),
		field("ZoneHVAC Unit Object Type", fieldRoleObjectTypeRef),
		field("ZoneHVAC Unit Object Name", fieldRoleObjectRef),
		field("Mixer Outlet Node Name", fieldRoleNodeRef),
		field("Mixer Primary Air Inlet Node Name", fieldRoleNodeRef),
		field("Mixer Secondary Air Inlet Node Name", fieldRoleNodeRef),
		choiceField("Mixer Connection Type", "", "InletSide", "SupplySide"),
		field("Design Specification Outdoor Air Object Name", fieldRoleObjectRef),
		choiceField("Per Person Ventilation Rate Mode", "", "CurrentOccupancy", "DesignOccupancy"),
	),
	"airterminal:dualduct:constantvolume": catalogObject("AirTerminal:DualDuct:ConstantVolume",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Hot Air Inlet Node Name", fieldRoleNodeRef),
		field("Cold Air Inlet Node Name", fieldRoleNodeRef),
		numericUnitField("Maximum Air Flow Rate", "m3/s", true),
	),
	"airterminal:dualduct:vav": catalogObject("AirTerminal:DualDuct:VAV",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Hot Air Inlet Node Name", fieldRoleNodeRef),
		field("Cold Air Inlet Node Name", fieldRoleNodeRef),
		numericUnitField("Maximum Damper Air Flow Rate", "m3/s", true),
		numericField("Zone Minimum Air Flow Fraction", false),
		field("Design Specification Outdoor Air Object Name", fieldRoleObjectRef),
		field("Minimum Air Flow Turndown Schedule Name", fieldRoleScheduleRef),
	),
	"airterminal:singleduct:userdefined": catalogObject("AirTerminal:SingleDuct:UserDefined",
		field("Name", fieldRoleName),
		field("Overall Model Simulation Program Calling Manager Name", ""),
		field("Model Setup and Sizing Program Calling Manager Name", ""),
		field("Primary Air Inlet Node Name", fieldRoleNodeRef),
		field("Primary Air Outlet Node Name", fieldRoleNodeRef),
		field("Secondary Air Inlet Node Name", fieldRoleNodeRef),
		field("Secondary Air Outlet Node Name", fieldRoleNodeRef),
		numericField("Number of Plant Loop Connections", false),
		field("Plant Connection 1 Inlet Node Name", fieldRoleNodeRef),
		field("Plant Connection 1 Outlet Node Name", fieldRoleNodeRef),
		field("Plant Connection 2 Inlet Node Name", fieldRoleNodeRef),
		field("Plant Connection 2 Outlet Node Name", fieldRoleNodeRef),
		field("Supply Inlet Water Storage Tank Name", fieldRoleObjectRef),
		field("Collection Outlet Water Storage Tank Name", fieldRoleObjectRef),
		field("Ambient Zone Name", fieldRoleZoneRef),
	),
	"zonehvac:idealloadsairsystem": catalogObject("ZoneHVAC:IdealLoadsAirSystem",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Zone Supply Air Node Name", fieldRoleNodeRef),
		field("Zone Exhaust Air Node Name", fieldRoleNodeRef),
		field("System Inlet Air Node Name", fieldRoleNodeRef),
		numericUnitField("Maximum Heating Supply Air Temperature", "C", false),
		numericUnitField("Minimum Cooling Supply Air Temperature", "C", false),
		numericField("Maximum Heating Supply Air Humidity Ratio", false),
		numericField("Minimum Cooling Supply Air Humidity Ratio", false),
		choiceField("Heating Limit", "", "NoLimit", "LimitFlowRate", "LimitCapacity", "LimitFlowRateAndCapacity"),
		numericUnitField("Maximum Heating Air Flow Rate", "m3/s", true),
		numericUnitField("Maximum Sensible Heating Capacity", "W", true),
		choiceField("Cooling Limit", "", "NoLimit", "LimitFlowRate", "LimitCapacity", "LimitFlowRateAndCapacity"),
		numericUnitField("Maximum Cooling Air Flow Rate", "m3/s", true),
		numericUnitField("Maximum Total Cooling Capacity", "W", true),
		field("Heating Availability Schedule Name", fieldRoleScheduleRef),
		field("Cooling Availability Schedule Name", fieldRoleScheduleRef),
		choiceField("Dehumidification Control Type", "", "None", "ConstantSensibleHeatRatio", "Humidistat", "ConstantSupplyHumidityRatio"),
		numericField("Cooling Sensible Heat Ratio", false),
	),
	"zonehvac:fourpipefancoil": catalogObject("ZoneHVAC:FourPipeFanCoil",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		choiceField("Capacity Control Method", "", "ConstantFanVariableFlow", "CyclingFan", "VariableFanVariableFlow", "MultiSpeedFan"),
		numericUnitField("Maximum Supply Air Flow Rate", "m3/s", true),
		numericField("Low Speed Supply Air Flow Ratio", false),
		numericField("Medium Speed Supply Air Flow Ratio", false),
		numericUnitField("Maximum Outdoor Air Flow Rate", "m3/s", true),
		field("Outdoor Air Schedule Name", fieldRoleScheduleRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Outdoor Air Mixer Object Type", fieldRoleObjectTypeRef),
		field("Outdoor Air Mixer Name", fieldRoleObjectRef),
		field("Supply Air Fan Object Type", fieldRoleObjectTypeRef),
		field("Supply Air Fan Name", fieldRoleObjectRef),
		field("Cooling Coil Object Type", fieldRoleObjectTypeRef),
		field("Cooling Coil Name", fieldRoleObjectRef),
		numericUnitField("Maximum Cold Water Flow Rate", "m3/s", true),
		numericUnitField("Minimum Cold Water Flow Rate", "m3/s", false),
		numericField("Cooling Convergence Tolerance", false),
		field("Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Heating Coil Name", fieldRoleObjectRef),
		numericUnitField("Maximum Hot Water Flow Rate", "m3/s", true),
		numericUnitField("Minimum Hot Water Flow Rate", "m3/s", false),
		numericField("Heating Convergence Tolerance", false),
	),
	"zonehvac:unitheater": catalogObject("ZoneHVAC:UnitHeater",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Supply Air Fan Object Type", fieldRoleObjectTypeRef),
		field("Supply Air Fan Name", fieldRoleObjectRef),
		numericUnitField("Maximum Supply Air Flow Rate", "m3/s", true),
		field("Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Heating Coil Name", fieldRoleObjectRef),
		field("Supply Air Fan Operating Mode Schedule Name", fieldRoleScheduleRef),
		choiceField("Supply Air Fan Operation During No Heating", "", "Yes", "No"),
		numericUnitField("Maximum Hot Water or Steam Flow Rate", "m3/s", true),
		numericUnitField("Minimum Hot Water or Steam Flow Rate", "m3/s", false),
		numericField("Heating Convergence Tolerance", false),
	),
	"zonehvac:packagedterminalairconditioner": catalogObject("ZoneHVAC:PackagedTerminalAirConditioner",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Outdoor Air Mixer Object Type", fieldRoleObjectTypeRef),
		field("Outdoor Air Mixer Name", fieldRoleObjectRef),
		numericUnitField("Cooling Supply Air Flow Rate", "m3/s", true),
		numericUnitField("Heating Supply Air Flow Rate", "m3/s", true),
		numericUnitField("No Load Supply Air Flow Rate", "m3/s", true),
		choiceField("No Load Supply Air Flow Rate Control Set To Low Speed", "", "Yes", "No"),
		numericUnitField("Cooling Outdoor Air Flow Rate", "m3/s", true),
		numericUnitField("Heating Outdoor Air Flow Rate", "m3/s", true),
		numericUnitField("No Load Outdoor Air Flow Rate", "m3/s", true),
		field("Supply Air Fan Object Type", fieldRoleObjectTypeRef),
		field("Supply Air Fan Name", fieldRoleObjectRef),
		field("Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Heating Coil Name", fieldRoleObjectRef),
		field("Cooling Coil Object Type", fieldRoleObjectTypeRef),
		field("Cooling Coil Name", fieldRoleObjectRef),
		choiceField("Fan Placement", "", "BlowThrough", "DrawThrough"),
		field("Supply Air Fan Operating Mode Schedule Name", fieldRoleScheduleRef),
	),
	"zonehvac:packagedterminalheatpump": catalogObject("ZoneHVAC:PackagedTerminalHeatPump",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Outdoor Air Mixer Object Type", fieldRoleObjectTypeRef),
		field("Outdoor Air Mixer Name", fieldRoleObjectRef),
		numericUnitField("Cooling Supply Air Flow Rate", "m3/s", true),
		numericUnitField("Heating Supply Air Flow Rate", "m3/s", true),
		numericUnitField("No Load Supply Air Flow Rate", "m3/s", true),
		choiceField("No Load Supply Air Flow Rate Control Set To Low Speed", "", "Yes", "No"),
		numericUnitField("Cooling Outdoor Air Flow Rate", "m3/s", true),
		numericUnitField("Heating Outdoor Air Flow Rate", "m3/s", true),
		numericUnitField("No Load Outdoor Air Flow Rate", "m3/s", true),
		field("Supply Air Fan Object Type", fieldRoleObjectTypeRef),
		field("Supply Air Fan Name", fieldRoleObjectRef),
		field("Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Heating Coil Name", fieldRoleObjectRef),
		numericField("Heating Convergence Tolerance", false),
		field("Cooling Coil Object Type", fieldRoleObjectTypeRef),
		field("Cooling Coil Name", fieldRoleObjectRef),
		numericField("Cooling Convergence Tolerance", false),
		field("Supplemental Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Supplemental Heating Coil Name", fieldRoleObjectRef),
		numericUnitField("Maximum Supply Air Temperature from Supplemental Heater", "C", true),
		numericUnitField("Maximum Outdoor Dry-Bulb Temperature for Supplemental Heater Operation", "C", false),
		choiceField("Fan Placement", "", "BlowThrough", "DrawThrough"),
		field("Supply Air Fan Operating Mode Schedule Name", fieldRoleScheduleRef),
	),
	"zonehvac:watertoairheatpump": catalogObject("ZoneHVAC:WaterToAirHeatPump",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Outdoor Air Mixer Object Type", fieldRoleObjectTypeRef),
		field("Outdoor Air Mixer Name", fieldRoleObjectRef),
		numericUnitField("Cooling Supply Air Flow Rate", "m3/s", true),
		numericUnitField("Heating Supply Air Flow Rate", "m3/s", true),
		numericUnitField("No Load Supply Air Flow Rate", "m3/s", true),
		choiceField("No Load Supply Air Flow Rate Control Set To Low Speed", "", "Yes", "No"),
		numericUnitField("Cooling Outdoor Air Flow Rate", "m3/s", true),
		numericUnitField("Heating Outdoor Air Flow Rate", "m3/s", true),
		numericUnitField("No Load Outdoor Air Flow Rate", "m3/s", true),
		field("Supply Air Fan Object Type", fieldRoleObjectTypeRef),
		field("Supply Air Fan Name", fieldRoleObjectRef),
		field("Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Heating Coil Name", fieldRoleObjectRef),
		field("Cooling Coil Object Type", fieldRoleObjectTypeRef),
		field("Cooling Coil Name", fieldRoleObjectRef),
		field("Supplemental Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Supplemental Heating Coil Name", fieldRoleObjectRef),
		numericUnitField("Maximum Supply Air Temperature from Supplemental Heater", "C", true),
		numericUnitField("Maximum Outdoor Dry-Bulb Temperature for Supplemental Heater Operation", "C", false),
		field("Outdoor Dry-Bulb Temperature Sensor Node Name", fieldRoleNodeRef),
		choiceField("Fan Placement", "", "BlowThrough", "DrawThrough"),
		field("Supply Air Fan Operating Mode Schedule Name", fieldRoleScheduleRef),
	),
	"zonehvac:unitventilator": catalogObject("ZoneHVAC:UnitVentilator",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Maximum Supply Air Flow Rate", "m3/s", true),
		choiceField("Outdoor Air Control Type", "", "FixedTemperature", "VariablePercent", "FixedAmount"),
		numericUnitField("Minimum Outdoor Air Flow Rate", "m3/s", true),
		field("Minimum Outdoor Air Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Maximum Outdoor Air Flow Rate", "m3/s", true),
		field("Maximum Outdoor Air Fraction or Temperature Schedule Name", fieldRoleScheduleRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Outdoor Air Node Name", fieldRoleNodeRef),
		field("Exhaust Air Node Name", fieldRoleNodeRef),
		field("Mixed Air Node Name", fieldRoleNodeRef),
		field("Supply Air Fan Object Type", fieldRoleObjectTypeRef),
		field("Supply Air Fan Name", fieldRoleObjectRef),
		choiceField("Coil Option", "", "None", "Heating", "Cooling", "HeatingAndCooling"),
		field("Supply Air Fan Operating Mode Schedule Name", fieldRoleScheduleRef),
		field("Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Heating Coil Name", fieldRoleObjectRef),
		numericField("Heating Convergence Tolerance", false),
		field("Cooling Coil Object Type", fieldRoleObjectTypeRef),
		field("Cooling Coil Name", fieldRoleObjectRef),
		numericField("Cooling Convergence Tolerance", false),
	),
	"zonehvac:windowairconditioner": catalogObject("ZoneHVAC:WindowAirConditioner",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Maximum Supply Air Flow Rate", "m3/s", true),
		numericUnitField("Maximum Outdoor Air Flow Rate", "m3/s", true),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Outdoor Air Mixer Object Type", fieldRoleObjectTypeRef),
		field("Outdoor Air Mixer Name", fieldRoleObjectRef),
		field("Supply Air Fan Object Type", fieldRoleObjectTypeRef),
		field("Supply Air Fan Name", fieldRoleObjectRef),
		field("Cooling Coil Object Type", fieldRoleObjectTypeRef),
		field("DX Cooling Coil Name", fieldRoleObjectRef),
		field("Supply Air Fan Operating Mode Schedule Name", fieldRoleScheduleRef),
		choiceField("Fan Placement", "", "BlowThrough", "DrawThrough"),
		numericField("Cooling Convergence Tolerance", false),
	),
	"zonehvac:energyrecoveryventilator": catalogObject("ZoneHVAC:EnergyRecoveryVentilator",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Heat Exchanger Name", fieldRoleObjectRef),
		numericUnitField("Supply Air Flow Rate", "m3/s", true),
		numericUnitField("Exhaust Air Flow Rate", "m3/s", true),
		field("Supply Air Fan Name", fieldRoleObjectRef),
		field("Exhaust Air Fan Name", fieldRoleObjectRef),
		field("Controller Name", fieldRoleObjectRef),
	),
	"zonehvac:dehumidifier:dx": catalogObject("ZoneHVAC:Dehumidifier:DX",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Rated Water Removal", "L/day", false),
		numericUnitField("Rated Energy Factor", "L/kWh", false),
		numericUnitField("Rated Air Flow Rate", "m3/s", true),
		field("Water Removal Curve Name", fieldRoleObjectRef),
		field("Energy Factor Curve Name", fieldRoleObjectRef),
		field("Part Load Fraction Correlation Curve Name", fieldRoleObjectRef),
		numericUnitField("Minimum Dry-Bulb Temperature for Dehumidifier Operation", "C", false),
		numericUnitField("Maximum Dry-Bulb Temperature for Dehumidifier Operation", "C", false),
		numericUnitField("Off-Cycle Parasitic Electric Load", "W", false),
	),
	"zonehvac:baseboard:convective:water": catalogObject("ZoneHVAC:Baseboard:Convective:Water",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Inlet Node Name", fieldRoleNodeRef),
		field("Outlet Node Name", fieldRoleNodeRef),
		choiceField("Heating Design Capacity Method", "", "HeatingDesignCapacity", "CapacityPerFloorArea", "FractionOfAutosizedHeatingCapacity"),
		numericUnitField("Heating Design Capacity", "W", true),
		numericUnitField("Heating Design Capacity Per Floor Area", "W/m2", false),
		numericField("Fraction of Autosized Heating Design Capacity", false),
		numericUnitField("U-Factor Times Area Value", "W/K", true),
		numericUnitField("Maximum Water Flow Rate", "m3/s", true),
		numericField("Convergence Tolerance", false),
	),
	"zonehvac:baseboard:radiantconvective:water": catalogObject("ZoneHVAC:Baseboard:RadiantConvective:Water",
		field("Name", fieldRoleName),
		field("Design Object", fieldRoleObjectRef),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Inlet Node Name", fieldRoleNodeRef),
		field("Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Rated Average Water Temperature", "C", false),
		numericUnitField("Rated Water Mass Flow Rate", "kg/s", false),
		numericUnitField("Heating Design Capacity", "W", true),
		numericUnitField("Maximum Water Flow Rate", "m3/s", true),
		extensibleField("Surface Name", fieldRoleObjectRef, "surfaces", 2),
		extensibleField("Fraction of Radiant Energy to Surface", "", "surfaces", 2),
	),
	"zonehvac:baseboard:radiantconvective:steam": catalogObject("ZoneHVAC:Baseboard:RadiantConvective:Steam",
		field("Name", fieldRoleName),
		field("Design Object", fieldRoleObjectRef),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Inlet Node Name", fieldRoleNodeRef),
		field("Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Heating Design Capacity", "W", true),
		numericUnitField("Degree of SubCooling", "deltaC", false),
		numericUnitField("Maximum Steam Flow Rate", "m3/s", true),
		extensibleField("Surface Name", fieldRoleObjectRef, "surfaces", 2),
		extensibleField("Fraction of Radiant Energy to Surface", "", "surfaces", 2),
	),
	"zonehvac:baseboard:radiantconvective:electric": catalogObject("ZoneHVAC:Baseboard:RadiantConvective:Electric",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		choiceField("Heating Design Capacity Method", "", "HeatingDesignCapacity", "CapacityPerFloorArea", "FractionOfAutosizedHeatingCapacity"),
		numericUnitField("Heating Design Capacity", "W", true),
		numericUnitField("Heating Design Capacity Per Floor Area", "W/m2", false),
		numericField("Fraction of Autosized Heating Design Capacity", false),
		numericField("Efficiency", false),
		numericField("Fraction Radiant", false),
		numericField("Fraction of Radiant Energy Incident on People", false),
		extensibleField("Surface Name", fieldRoleObjectRef, "surfaces", 2),
		extensibleField("Fraction of Radiant Energy to Surface", "", "surfaces", 2),
	),
	"zonehvac:coolingpanel:radiantconvective:water": catalogObject("ZoneHVAC:CoolingPanel:RadiantConvective:Water",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Water Inlet Node Name", fieldRoleNodeRef),
		field("Water Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Rated Inlet Water Temperature", "C", false),
		numericUnitField("Rated Inlet Space Temperature", "C", false),
		numericUnitField("Rated Water Mass Flow Rate", "kg/s", false),
		choiceField("Cooling Design Capacity Method", "", "CoolingDesignCapacity", "CapacityPerFloorArea", "FractionOfAutosizedCoolingCapacity"),
		numericUnitField("Cooling Design Capacity", "W", true),
		numericUnitField("Cooling Design Capacity Per Floor Area", "W/m2", false),
		numericField("Fraction of Autosized Cooling Design Capacity", false),
		numericUnitField("Maximum Chilled Water Flow Rate", "m3/s", true),
		choiceField("Control Type", "", "MeanAirTemperature", "MeanRadiantTemperature", "OperativeTemperature", "OutdoorDryBulbTemperature", "OutdoorWetBulbTemperature", "ZoneAirDewpointTemperature"),
		numericUnitField("Cooling Control Throttling Range", "deltaC", false),
		field("Cooling Control Temperature Schedule Name", fieldRoleScheduleRef),
		choiceField("Condensation Control Type", "", "Off", "SimpleOff"),
		numericUnitField("Condensation Control Dewpoint Offset", "C", false),
		numericField("Fraction Radiant", false),
		numericField("Fraction of Radiant Energy Incident on People", false),
		extensibleField("Surface Name", fieldRoleObjectRef, "surfaces", 2),
		extensibleField("Fraction of Radiant Energy to Surface", "", "surfaces", 2),
	),
	"zonehvac:lowtemperatureradiant:variableflow": catalogObject("ZoneHVAC:LowTemperatureRadiant:VariableFlow",
		field("Name", fieldRoleName),
		field("Design Object", fieldRoleObjectRef),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		targetField("Zone Name", fieldRoleZoneRef, "zone", "zones", "serves"),
		field("Surface Name or Radiant Surface Group Name", fieldRoleObjectRef),
		numericUnitField("Hydronic Tubing Length", "m", true),
		numericUnitField("Heating Design Capacity", "W", true),
		numericUnitField("Maximum Hot Water Flow", "m3/s", true),
		field("Heating Water Inlet Node Name", fieldRoleNodeRef),
		field("Heating Water Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Cooling Design Capacity", "W", true),
		numericUnitField("Maximum Cold Water Flow", "m3/s", true),
		field("Cooling Water Inlet Node Name", fieldRoleNodeRef),
		field("Cooling Water Outlet Node Name", fieldRoleNodeRef),
		numericField("Number of Circuits", false),
		numericUnitField("Circuit Length", "m", false),
	),
	"zonehvac:lowtemperatureradiant:constantflow": catalogObject("ZoneHVAC:LowTemperatureRadiant:ConstantFlow",
		field("Name", fieldRoleName),
		field("Design Object", fieldRoleObjectRef),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		targetField("Zone Name", fieldRoleZoneRef, "zone", "zones", "serves"),
		field("Surface Name or Radiant Surface Group Name", fieldRoleObjectRef),
		numericUnitField("Hydronic Tubing Length", "m", true),
		numericUnitField("Rated Flow Rate", "m3/s", true),
		field("Pump Flow Rate Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Rated Pump Head", "Pa", false),
		numericUnitField("Rated Power Consumption", "W", false),
		field("Heating Water Inlet Node Name", fieldRoleNodeRef),
		field("Heating Water Outlet Node Name", fieldRoleNodeRef),
		field("Heating High Water Temperature Schedule Name", fieldRoleScheduleRef),
		field("Heating Low Water Temperature Schedule Name", fieldRoleScheduleRef),
		field("Heating High Control Temperature Schedule Name", fieldRoleScheduleRef),
		field("Heating Low Control Temperature Schedule Name", fieldRoleScheduleRef),
		field("Cooling Water Inlet Node Name", fieldRoleNodeRef),
		field("Cooling Water Outlet Node Name", fieldRoleNodeRef),
		field("Cooling High Water Temperature Schedule Name", fieldRoleScheduleRef),
		field("Cooling Low Water Temperature Schedule Name", fieldRoleScheduleRef),
		field("Cooling High Control Temperature Schedule Name", fieldRoleScheduleRef),
		field("Cooling Low Control Temperature Schedule Name", fieldRoleScheduleRef),
		numericField("Number of Circuits", false),
		numericUnitField("Circuit Length", "m", false),
	),
	"zonehvac:lowtemperatureradiant:electric": catalogObject("ZoneHVAC:LowTemperatureRadiant:Electric",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		targetField("Zone Name", fieldRoleZoneRef, "zone", "zones", "serves"),
		field("Surface Name or Radiant Surface Group Name", fieldRoleObjectRef),
		choiceField("Heating Design Capacity Method", "", "HeatingDesignCapacity", "CapacityPerFloorArea", "FractionOfAutosizedHeatingCapacity"),
		numericUnitField("Heating Design Capacity", "W", true),
		numericUnitField("Heating Design Capacity Per Floor Area", "W/m2", false),
		numericField("Fraction of Autosized Heating Design Capacity", false),
		choiceField("Temperature Control Type", "", "MeanAirTemperature", "MeanRadiantTemperature", "OperativeTemperature", "OutdoorDryBulbTemperature", "OutdoorWetBulbTemperature", "SurfaceFaceTemperature"),
		choiceField("Setpoint Control Type", "", "ZeroFlowPower", "HalfFlowPower", "MeanAirTemperature", "MeanRadiantTemperature", "OperativeTemperature", "OutdoorDryBulbTemperature", "OutdoorWetBulbTemperature", "SurfaceFaceTemperature"),
		numericUnitField("Heating Throttling Range", "deltaC", false),
		field("Heating Setpoint Temperature Schedule Name", fieldRoleScheduleRef),
	),
	"zonehvac:hightemperatureradiant": catalogObject("ZoneHVAC:HighTemperatureRadiant",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		targetField("Zone Name", fieldRoleZoneRef, "zone", "zones", "serves"),
		choiceField("Heating Design Capacity Method", "", "HeatingDesignCapacity", "CapacityPerFloorArea", "FractionOfAutosizedHeatingCapacity"),
		numericUnitField("Heating Design Capacity", "W", true),
		numericUnitField("Heating Design Capacity Per Floor Area", "W/m2", false),
		numericField("Fraction of Autosized Heating Design Capacity", false),
		field("Fuel Type", ""),
		numericField("Combustion Efficiency", false),
		numericField("Fraction of Input Converted to Radiant Energy", false),
		numericField("Fraction of Input Converted to Latent Energy", false),
		numericField("Fraction of Input that Is Lost", false),
		choiceField("Temperature Control Type", "", "MeanAirTemperature", "MeanRadiantTemperature", "OperativeTemperature", "OperativeTemperatureSetpoint"),
		numericUnitField("Heating Throttling Range", "deltaC", false),
		field("Heating Setpoint Temperature Schedule Name", fieldRoleScheduleRef),
		numericField("Fraction of Radiant Energy Incident on People", false),
		extensibleField("Surface Name", fieldRoleObjectRef, "surfaces", 2),
		extensibleField("Fraction of Radiant Energy to Surface", "", "surfaces", 2),
	),
	"zonehvac:ventilatedslab": catalogObject("ZoneHVAC:VentilatedSlab",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		targetField("Zone Name", fieldRoleZoneRef, "zone", "zones", "serves"),
		field("Surface Name or Radiant Surface Group Name", fieldRoleObjectRef),
		numericUnitField("Maximum Air Flow Rate", "m3/s", true),
		choiceField("Outdoor Air Control Type", "", "FixedTemperature", "VariablePercent", "FixedAmount"),
		numericUnitField("Minimum Outdoor Air Flow Rate", "m3/s", true),
		field("Minimum Outdoor Air Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Maximum Outdoor Air Flow Rate", "m3/s", true),
		field("Maximum Outdoor Air Fraction or Temperature Schedule Name", fieldRoleScheduleRef),
		choiceField("System Configuration Type", "", "SlabOnly", "SlabAndZone", "SeriesSlabs"),
		numericUnitField("Hollow Core Inside Diameter", "m", false),
		numericUnitField("Hollow Core Length", "m", false),
		numericField("Number of Cores", false),
		choiceField("Temperature Control Type", "", "MeanAirTemperature", "MeanRadiantTemperature", "OperativeTemperature", "OutdoorDryBulbTemperature", "OutdoorWetBulbTemperature"),
		field("Heating High Air Temperature Schedule Name", fieldRoleScheduleRef),
		field("Heating Low Air Temperature Schedule Name", fieldRoleScheduleRef),
		field("Heating High Control Temperature Schedule Name", fieldRoleScheduleRef),
		field("Heating Low Control Temperature Schedule Name", fieldRoleScheduleRef),
		field("Cooling High Air Temperature Schedule Name", fieldRoleScheduleRef),
		field("Cooling Low Air Temperature Schedule Name", fieldRoleScheduleRef),
		field("Cooling High Control Temperature Schedule Name", fieldRoleScheduleRef),
		field("Cooling Low Control Temperature Schedule Name", fieldRoleScheduleRef),
		field("Return Air Node Name", fieldRoleNodeRef),
		field("Slab In Node Name", fieldRoleNodeRef),
		field("Zone Supply Air Node Name", fieldRoleNodeRef),
		field("Outdoor Air Node Name", fieldRoleNodeRef),
		field("Relief Air Node Name", fieldRoleNodeRef),
		field("Outdoor Air Mixer Outlet Node Name", fieldRoleNodeRef),
		field("Fan Outlet Node Name", fieldRoleNodeRef),
		field("Fan Name", fieldRoleObjectRef),
		choiceField("Coil Option Type", "", "None", "Heating", "Cooling", "HeatingAndCooling"),
		field("Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Heating Coil Name", fieldRoleObjectRef),
		field("Hot Water or Steam Inlet Node Name", fieldRoleNodeRef),
		field("Cooling Coil Object Type", fieldRoleObjectTypeRef),
		field("Cooling Coil Name", fieldRoleObjectRef),
		field("Cold Water Inlet Node Name", fieldRoleNodeRef),
	),
	"zonehvac:outdoorairunit": catalogObject("ZoneHVAC:OutdoorAirUnit",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		targetField("Zone Name", fieldRoleZoneRef, "zone", "zones", "serves"),
		numericUnitField("Outdoor Air Flow Rate", "m3/s", true),
		field("Outdoor Air Schedule Name", fieldRoleScheduleRef),
		field("Supply Fan Name", fieldRoleObjectRef),
		choiceField("Supply Fan Placement", "", "BlowThrough", "DrawThrough"),
		field("Exhaust Fan Name", fieldRoleObjectRef),
		numericUnitField("Exhaust Air Flow Rate", "m3/s", true),
		field("Exhaust Air Schedule Name", fieldRoleScheduleRef),
		choiceField("Unit Control Type", "", "NeutralControl", "TemperatureControl", "TemperatureAndHumidityControl"),
		field("High Air Control Temperature Schedule Name", fieldRoleScheduleRef),
		field("Low Air Control Temperature Schedule Name", fieldRoleScheduleRef),
		field("Outdoor Air Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Supply Fan Outlet Node Name", fieldRoleNodeRef),
		field("Outdoor Air Unit List Name", fieldRoleObjectRef),
	),
	"zonehvac:outdoorairunit:equipmentlist": catalogObject("ZoneHVAC:OutdoorAirUnit:EquipmentList",
		field("Name", fieldRoleName),
		extensibleField("Component Object Type", fieldRoleObjectTypeRef, "components", 2),
		extensibleField("Component Name", fieldRoleObjectRef, "components", 2),
	),
	"zonehvac:hybridunitaryhvac": catalogObject("ZoneHVAC:HybridUnitaryHVAC",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Availability Manager List Name", fieldRoleObjectRef),
		field("Minimum Supply Air Temperature Schedule Name", fieldRoleScheduleRef),
		field("Maximum Supply Air Temperature Schedule Name", fieldRoleScheduleRef),
		field("Minimum Supply Air Humidity Ratio Schedule Name", fieldRoleScheduleRef),
		field("Maximum Supply Air Humidity Ratio Schedule Name", fieldRoleScheduleRef),
		choiceField("Method to Choose Controlled Inputs and Part Runtime Fraction", "", "Automatic", "User Defined"),
		field("Return Air Node Name", fieldRoleNodeRef),
		field("Outdoor Air Node Name", fieldRoleNodeRef),
		field("Supply Air Node Name", fieldRoleNodeRef),
		field("Relief Node Name", fieldRoleNodeRef),
		numericUnitField("System Maximum Supply Air Flow Rate", "m3/s", true),
		numericUnitField("External Static Pressure at System Maximum Supply Air Flow Rate", "Pa", false),
		choiceField("Fan Heat Included in Lookup Tables", "", "No", "Yes"),
		choiceField("Fan Heat Gain Location", "", "MixedAirStream", "SupplyAirStream"),
		numericField("Fan Heat In Air Stream Fraction", false),
		numericField("Scaling Factor", false),
		numericUnitField("Minimum Time Between Mode Change", "minutes", false),
		choiceField("First Fuel Type", "", "None", "Electricity", "NaturalGas", "Propane", "FuelOilNo1", "FuelOilNo2", "Diesel", "Gasoline", "Coal", "OtherFuel1", "OtherFuel2", "DistrictHeatingWater", "DistrictHeatingSteam", "DistrictCooling"),
		choiceField("Second Fuel Type", "", "None", "Electricity", "NaturalGas", "Propane", "FuelOilNo1", "FuelOilNo2", "Diesel", "Gasoline", "Coal", "OtherFuel1", "OtherFuel2", "DistrictHeatingWater", "DistrictHeatingSteam", "DistrictCooling"),
		choiceField("Third Fuel Type", "", "None", "Electricity", "NaturalGas", "Propane", "FuelOilNo1", "FuelOilNo2", "Diesel", "Gasoline", "Coal", "OtherFuel1", "OtherFuel2", "DistrictHeatingWater", "DistrictHeatingSteam", "DistrictCooling"),
		choiceField("Objective Function to Minimize", "", "Electricity Use", "Second Fuel Use", "Third Fuel Use", "Water Use"),
		field("Design Specification Outdoor Air Object Name", fieldRoleObjectRef),
		field("Mode 0 Name", ""),
		field("Mode 0 Supply Air Temperature Lookup Table Name", fieldRoleObjectRef),
		field("Mode 0 Supply Air Humidity Ratio Lookup Table Name", fieldRoleObjectRef),
		field("Mode 0 System Electric Power Lookup Table Name", fieldRoleObjectRef),
		field("Mode 0 Supply Fan Electric Power Lookup Table Name", fieldRoleObjectRef),
		field("Mode 0 External Static Pressure Lookup Table Name", fieldRoleObjectRef),
		field("Mode 0 System Second Fuel Consumption Lookup Table Name", fieldRoleObjectRef),
		field("Mode 0 System Third Fuel Consumption Lookup Table Name", fieldRoleObjectRef),
		field("Mode 0 System Water Use Lookup Table Name", fieldRoleObjectRef),
		numericField("Mode 0 Outdoor Air Fraction", false),
		numericField("Mode 0 Supply Air Mass Flow Rate Ratio", false),
		extensibleField("Mode 1 Name", "", "modes", 25),
		extensibleField("Mode 1 Supply Air Temperature Lookup Table Name", fieldRoleObjectRef, "modes", 25),
		extensibleField("Mode 1 Supply Air Humidity Ratio Lookup Table Name", fieldRoleObjectRef, "modes", 25),
		extensibleField("Mode 1 System Electric Power Lookup Table Name", fieldRoleObjectRef, "modes", 25),
		extensibleField("Mode 1 Supply Fan Electric Power Lookup Table Name", fieldRoleObjectRef, "modes", 25),
		extensibleField("Mode 1 External Static Pressure Lookup Table Name", fieldRoleObjectRef, "modes", 25),
		extensibleField("Mode 1 System Second Fuel Consumption Lookup Table Name", fieldRoleObjectRef, "modes", 25),
		extensibleField("Mode 1 System Third Fuel Consumption Lookup Table Name", fieldRoleObjectRef, "modes", 25),
		extensibleField("Mode 1 System Water Use Lookup Table Name", fieldRoleObjectRef, "modes", 25),
		extensibleField("Mode 1 Minimum Outdoor Air Temperature", "", "modes", 25),
		extensibleField("Mode 1 Maximum Outdoor Air Temperature", "", "modes", 25),
		extensibleField("Mode 1 Minimum Outdoor Air Humidity Ratio", "", "modes", 25),
		extensibleField("Mode 1 Maximum Outdoor Air Humidity Ratio", "", "modes", 25),
		extensibleField("Mode 1 Minimum Outdoor Air Relative Humidity", "", "modes", 25),
		extensibleField("Mode 1 Maximum Outdoor Air Relative Humidity", "", "modes", 25),
		extensibleField("Mode 1 Minimum Return Air Temperature", "", "modes", 25),
		extensibleField("Mode 1 Maximum Return Air Temperature", "", "modes", 25),
		extensibleField("Mode 1 Minimum Return Air Humidity Ratio", "", "modes", 25),
		extensibleField("Mode 1 Maximum Return Air Humidity Ratio", "", "modes", 25),
		extensibleField("Mode 1 Minimum Return Air Relative Humidity", "", "modes", 25),
		extensibleField("Mode 1 Maximum Return Air Relative Humidity", "", "modes", 25),
		extensibleField("Mode 1 Minimum Outdoor Air Fraction", "", "modes", 25),
		extensibleField("Mode 1 Maximum Outdoor Air Fraction", "", "modes", 25),
		extensibleField("Mode 1 Minimum Supply Air Mass Flow Rate Ratio", "", "modes", 25),
		extensibleField("Mode 1 Maximum Supply Air Mass Flow Rate Ratio", "", "modes", 25),
	),
	"zonehvac:refrigerationchillerset": catalogObject("ZoneHVAC:RefrigerationChillerSet",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		field("Zone Name", fieldRoleZoneRef),
		field("Air Inlet Node Name", fieldRoleNodeRef),
		field("Air Outlet Node Name", fieldRoleNodeRef),
		extensibleField("Air Chiller Name", fieldRoleObjectRef, "air_chillers", 1),
	),
	"zonehvac:forcedair:userdefined": catalogObject("ZoneHVAC:ForcedAir:UserDefined",
		field("Name", fieldRoleName),
		field("Overall Model Simulation Program Calling Manager Name", ""),
		field("Model Setup and Sizing Program Calling Manager Name", ""),
		field("Primary Air Inlet Node Name", fieldRoleNodeRef),
		field("Primary Air Outlet Node Name", fieldRoleNodeRef),
		field("Secondary Air Inlet Node Name", fieldRoleNodeRef),
		field("Secondary Air Outlet Node Name", fieldRoleNodeRef),
		numericField("Number of Plant Loop Connections", false),
		field("Plant Connection 1 Inlet Node Name", fieldRoleNodeRef),
		field("Plant Connection 1 Outlet Node Name", fieldRoleNodeRef),
		field("Plant Connection 2 Inlet Node Name", fieldRoleNodeRef),
		field("Plant Connection 2 Outlet Node Name", fieldRoleNodeRef),
		field("Plant Connection 3 Inlet Node Name", fieldRoleNodeRef),
		field("Plant Connection 3 Outlet Node Name", fieldRoleNodeRef),
		field("Supply Inlet Water Storage Tank Name", fieldRoleObjectRef),
		field("Collection Outlet Water Storage Tank Name", fieldRoleObjectRef),
		field("Ambient Zone Name", fieldRoleZoneRef),
	),
	"zonehvac:lowtemperatureradiant:surfacegroup": catalogObject("ZoneHVAC:LowTemperatureRadiant:SurfaceGroup",
		field("Name", fieldRoleName),
		extensibleField("Surface Name", fieldRoleObjectRef, "surfaces", 2),
		extensibleField("Flow Fraction for Surface", "", "surfaces", 2),
	),
	"zonehvac:terminalunit:variablerefrigerantflow": catalogObject("ZoneHVAC:TerminalUnit:VariableRefrigerantFlow",
		field("Zone Terminal Unit Name", fieldRoleName),
		field("Terminal Unit Availability Schedule", fieldRoleScheduleRef),
		field("Terminal Unit Air Inlet Node Name", fieldRoleNodeRef),
		field("Terminal Unit Air Outlet Node Name", fieldRoleNodeRef),
		numericUnitField("Cooling Supply Air Flow Rate", "m3/s", true),
		numericUnitField("No Cooling Supply Air Flow Rate", "m3/s", true),
		numericUnitField("Heating Supply Air Flow Rate", "m3/s", true),
		numericUnitField("No Heating Supply Air Flow Rate", "m3/s", true),
		numericUnitField("Cooling Outdoor Air Flow Rate", "m3/s", true),
		numericUnitField("Heating Outdoor Air Flow Rate", "m3/s", true),
		numericUnitField("No Load Outdoor Air Flow Rate", "m3/s", true),
		field("Supply Air Fan Operating Mode Schedule Name", fieldRoleScheduleRef),
		choiceField("Supply Air Fan Placement", "", "BlowThrough", "DrawThrough"),
		field("Supply Air Fan Object Type", fieldRoleObjectTypeRef),
		field("Supply Air Fan Object Name", fieldRoleObjectRef),
		field("Outside Air Mixer Object Type", fieldRoleObjectTypeRef),
		field("Outside Air Mixer Object Name", fieldRoleObjectRef),
		field("Cooling Coil Object Type", fieldRoleObjectTypeRef),
		field("Cooling Coil Object Name", fieldRoleObjectRef),
		field("Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Heating Coil Object Name", fieldRoleObjectRef),
		numericField("Zone Terminal Unit On Parasitic Electric Energy Use", false),
		numericField("Zone Terminal Unit Off Parasitic Electric Energy Use", false),
		numericField("Rated Heating Capacity Sizing Ratio", false),
		field("Availability Manager List Name", fieldRoleObjectRef),
		field("Design Specification ZoneHVAC Sizing Object Name", fieldRoleObjectRef),
		field("Supplemental Heating Coil Object Type", fieldRoleObjectTypeRef),
		field("Supplemental Heating Coil Name", fieldRoleObjectRef),
		numericUnitField("Maximum Supply Air Temperature from Supplemental Heater", "C", true),
		numericUnitField("Maximum Outdoor Dry-Bulb Temperature for Supplemental Heater Operation", "C", false),
		field("Controlling Zone or Thermostat Location", fieldRoleZoneRef),
		field("Design Specification Multispeed Object Type", fieldRoleObjectTypeRef),
		field("Design Specification Multispeed Object Name", fieldRoleObjectRef),
	),
	"airconditioner:variablerefrigerantflow": catalogObject("AirConditioner:VariableRefrigerantFlow",
		field("Heat Pump Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Gross Rated Total Cooling Capacity", "W", true),
		numericField("Gross Rated Cooling COP", false),
		numericUnitField("Minimum Condenser Inlet Node Temperature in Cooling Mode", "C", false),
		numericUnitField("Maximum Condenser Inlet Node Temperature in Cooling Mode", "C", false),
		field("Cooling Capacity Ratio Modifier Function of Low Temperature Curve Name", fieldRoleObjectRef),
		field("Cooling Capacity Ratio Boundary Curve Name", fieldRoleObjectRef),
		field("Cooling Capacity Ratio Modifier Function of High Temperature Curve Name", fieldRoleObjectRef),
		field("Cooling Energy Input Ratio Modifier Function of Low Temperature Curve Name", fieldRoleObjectRef),
		field("Cooling Energy Input Ratio Boundary Curve Name", fieldRoleObjectRef),
		field("Cooling Energy Input Ratio Modifier Function of High Temperature Curve Name", fieldRoleObjectRef),
		field("Cooling Energy Input Ratio Modifier Function of Low Part-Load Ratio Curve Name", fieldRoleObjectRef),
		field("Cooling Energy Input Ratio Modifier Function of High Part-Load Ratio Curve Name", fieldRoleObjectRef),
		field("Cooling Combination Ratio Correction Factor Curve Name", fieldRoleObjectRef),
		field("Cooling Part-Load Fraction Correlation Curve Name", fieldRoleObjectRef),
		numericUnitField("Gross Rated Heating Capacity", "W", true),
		numericField("Rated Heating Capacity Sizing Ratio", false),
		numericField("Gross Rated Heating COP", false),
		numericUnitField("Minimum Condenser Inlet Node Temperature in Heating Mode", "C", false),
		numericUnitField("Maximum Condenser Inlet Node Temperature in Heating Mode", "C", false),
		field("Heating Capacity Ratio Modifier Function of Low Temperature Curve Name", fieldRoleObjectRef),
		field("Heating Capacity Ratio Boundary Curve Name", fieldRoleObjectRef),
		field("Heating Capacity Ratio Modifier Function of High Temperature Curve Name", fieldRoleObjectRef),
		field("Heating Energy Input Ratio Modifier Function of Low Temperature Curve Name", fieldRoleObjectRef),
		field("Heating Energy Input Ratio Boundary Curve Name", fieldRoleObjectRef),
		field("Heating Energy Input Ratio Modifier Function of High Temperature Curve Name", fieldRoleObjectRef),
		choiceField("Heating Performance Curve Outdoor Temperature Type", "", "DryBulbTemperature", "WetBulbTemperature"),
		field("Heating Energy Input Ratio Modifier Function of Low Part-Load Ratio Curve Name", fieldRoleObjectRef),
		field("Heating Energy Input Ratio Modifier Function of High Part-Load Ratio Curve Name", fieldRoleObjectRef),
		field("Heating Combination Ratio Correction Factor Curve Name", fieldRoleObjectRef),
		field("Heating Part-Load Fraction Correlation Curve Name", fieldRoleObjectRef),
		numericField("Minimum Heat Pump Part-Load Ratio", false),
		field("Zone Name for Master Thermostat Location", fieldRoleZoneRef),
		choiceField("Master Thermostat Priority Control Type", "", "LoadPriority", "ZonePriority", "ThermostatOffsetPriority", "Scheduled"),
		field("Thermostat Priority Schedule Name", fieldRoleScheduleRef),
		targetField("Zone Terminal Unit List Name", fieldRoleObjectRef, "zone_terminal_unit_list", "zone_terminal_unit_lists", "expands"),
	),
	"zoneterminalunitlist": catalogObject("ZoneTerminalUnitList",
		field("Zone Terminal Unit List Name", fieldRoleName),
		extensibleField("Zone Terminal Unit Name", fieldRoleObjectRef, "zone_terminal_units", 1),
	),
	"zonehvac:equipmentconnections": catalogObject("ZoneHVAC:EquipmentConnections",
		field("Zone Name", fieldRoleZoneRef),
		field("Zone Conditioning Equipment List Name", fieldRoleObjectRef),
		field("Zone Air Inlet Node or NodeList Name", fieldRoleNodeListRef),
		field("Zone Air Exhaust Node or NodeList Name", fieldRoleNodeListRef),
		field("Zone Air Node Name", fieldRoleNodeRef),
		field("Zone Return Air Node or NodeList Name", fieldRoleNodeListRef),
	),
	"zonehvac:equipmentlist": catalogObject("ZoneHVAC:EquipmentList",
		field("Name", fieldRoleName),
		field("Load Distribution Scheme", ""),
		extensibleField("Zone Equipment Object Type", fieldRoleObjectTypeRef, "equipment", 6),
		extensibleField("Zone Equipment Name", fieldRoleObjectRef, "equipment", 6),
		extensibleField("Zone Equipment Cooling Sequence", "", "equipment", 6),
		extensibleField("Zone Equipment Heating or No-Load Sequence", "", "equipment", 6),
		extensibleField("Sequential Cooling Fraction Schedule Name", fieldRoleScheduleRef, "equipment", 6),
		extensibleField("Sequential Heating Fraction Schedule Name", fieldRoleScheduleRef, "equipment", 6),
	),
	"spacehvac:equipmentconnections": catalogObject("SpaceHVAC:EquipmentConnections",
		field("Space Name", fieldRoleSpaceRef),
		field("Space Conditioning Equipment List Name", fieldRoleObjectRef),
		field("Space Air Inlet Node or NodeList Name", fieldRoleNodeListRef),
		field("Space Air Exhaust Node or NodeList Name", fieldRoleNodeListRef),
		field("Space Air Node Name", fieldRoleNodeRef),
		field("Space Return Air Node or NodeList Name", fieldRoleNodeListRef),
	),
	"spacehvac:equipmentlist": catalogObject("SpaceHVAC:EquipmentList",
		field("Name", fieldRoleName),
		field("Load Distribution Scheme", ""),
		extensibleField("Space Equipment Object Type", fieldRoleObjectTypeRef, "equipment", 6),
		extensibleField("Space Equipment Name", fieldRoleObjectRef, "equipment", 6),
		extensibleField("Space Equipment Cooling Sequence", "", "equipment", 6),
		extensibleField("Space Equipment Heating or No-Load Sequence", "", "equipment", 6),
		extensibleField("Sequential Cooling Fraction Schedule Name", fieldRoleScheduleRef, "equipment", 6),
		extensibleField("Sequential Heating Fraction Schedule Name", fieldRoleScheduleRef, "equipment", 6),
	),
	"spacehvac:zoneequipmentsplitter": catalogObject("SpaceHVAC:ZoneEquipmentSplitter",
		field("Name", fieldRoleName),
		targetField("Zone Name", fieldRoleZoneRef, "zone", "zones", "serves"),
		field("Zone Equipment Object Type", fieldRoleObjectTypeRef),
		field("Zone Equipment Name", fieldRoleObjectRef),
		field("Zone Equipment Outlet Node Name", fieldRoleNodeRef),
		choiceField("Thermostat Control Method", "", "SingleSpace", "Maximum", "Ideal"),
		targetField("Control Space Name", fieldRoleSpaceRef, "space", "spaces", "controls"),
		choiceField("Space Fraction Method", "", "DesignCoolingLoad", "DesignHeatingLoad", "FloorArea", "Volume", "PerimeterLength"),
		extensibleField("Space Name", fieldRoleSpaceRef, "spaces", 3),
		extensibleField("Space Fraction", "", "spaces", 3),
		extensibleField("Space Supply Node Name", fieldRoleNodeRef, "spaces", 3),
	),
	"nodelist": catalogObject("NodeList",
		field("Name", fieldRoleName),
		extensibleField("Node Name", fieldRoleNodeRef, "nodes", 1),
	),
	"scheduletypelimits": catalogObject("ScheduleTypeLimits",
		field("Name", fieldRoleName),
		numericField("Lower Limit Value", false),
		numericField("Upper Limit Value", false),
		choiceField("Numeric Type", "", "Continuous", "Discrete"),
		field("Unit Type", ""),
	),
	"output:variable": catalogObject("Output:Variable",
		field("Key Value", ""),
		field("Variable Name", ""),
		choiceField("Reporting Frequency", "", "Detailed", "Timestep", "Hourly", "Daily", "Monthly", "RunPeriod", "Annual"),
		field("Schedule Name", fieldRoleObjectRef),
	),
	"output:meter": catalogObject("Output:Meter",
		field("Key Name", ""),
		choiceField("Reporting Frequency", "", "Detailed", "Timestep", "Hourly", "Daily", "Monthly", "RunPeriod", "Annual"),
	),
	"output:meter:meterfileonly": catalogObject("Output:Meter:MeterFileOnly",
		field("Key Name", ""),
		choiceField("Reporting Frequency", "", "Detailed", "Timestep", "Hourly", "Daily", "Monthly", "RunPeriod", "Annual"),
	),
	"output:meter:cumulative": catalogObject("Output:Meter:Cumulative",
		field("Key Name", ""),
		choiceField("Reporting Frequency", "", "Detailed", "Timestep", "Hourly", "Daily", "Monthly", "RunPeriod", "Annual"),
	),
	"output:meter:cumulativemeterfileonly": catalogObject("Output:Meter:Cumulative:MeterFileOnly",
		field("Key Name", ""),
		choiceField("Reporting Frequency", "", "Detailed", "Timestep", "Hourly", "Daily", "Monthly", "RunPeriod", "Annual"),
	),
	"output:variabledictionary": catalogObject("Output:VariableDictionary",
		choiceField("Key Field", "", "IDF", "regular", "Regular", "Name", "None"),
	),
	"output:table:summaryreports": catalogObject("Output:Table:SummaryReports",
		field("Report 1 Name", ""),
	),
	"output:table:monthly": catalogObject("Output:Table:Monthly",
		field("Name", fieldRoleName),
		field("Digits After Decimal", ""),
	),
	"output:table:annual": catalogObject("Output:Table:Annual",
		field("Name", fieldRoleName),
		field("Filter", ""),
	),
	"output:table:timebins": catalogObject("Output:Table:TimeBins",
		field("Name", fieldRoleName),
	),
	"output:sqlite": catalogObject("Output:SQLite",
		choiceField("Option Type", "", "Simple", "SimpleAndTabular"),
		choiceField("Unit Conversion for Tabular Data", "", "None", "JtoKWH", "JtoMJ", "JtoGJ", "InchPound"),
	),
	"output:json": catalogObject("Output:JSON",
		choiceField("Option Type", "", "TimeSeries", "TimeSeriesAndTabular"),
	),
	"outputcontrol:files": catalogObject("OutputControl:Files",
		choiceField("Output CSV", "", "Yes", "No"),
		choiceField("Output MTR", "", "Yes", "No"),
		choiceField("Output ESO", "", "Yes", "No"),
		choiceField("Output EIO", "", "Yes", "No"),
		choiceField("Output Tabular", "", "Yes", "No"),
		choiceField("Output SQLite", "", "Yes", "No"),
		choiceField("Output JSON", "", "Yes", "No"),
	),
	"outputcontrol:table:style": catalogObject("OutputControl:Table:Style",
		choiceField("Column Separator", "", "Tab", "Fixed", "Comma", "HTML", "XML", "All"),
		choiceField("Unit Conversion", "", "None", "JtoKWH", "JtoMJ", "JtoGJ", "InchPound"),
	),
	"output:diagnostics": catalogObject("Output:Diagnostics",
		field("Key 1", ""),
	),
}

type EnergyPlusSchemaAdapter struct {
	Version        string
	AdapterVersion string
	SourcePath     string
	Objects        map[string]ObjectSpec
}

const manualFieldCatalogAdapterVersion = "manual-catalog-fallback/0.3"

func catalogObject(objectType string, fields ...FieldSpec) ObjectSpec {
	for index := range fields {
		fields[index].Index = index
	}
	return ObjectSpec{Type: objectType, Fields: fields}
}

func field(name string, role string) FieldSpec {
	spec := FieldSpec{Name: name, Role: role}
	return enrichFieldTarget(spec)
}

func choiceField(name string, role string, choices ...string) FieldSpec {
	spec := FieldSpec{Name: name, Role: role, Choices: choices}
	return enrichFieldTarget(spec)
}

func numericField(name string, allowAutosize bool) FieldSpec {
	spec := FieldSpec{Name: name, Numeric: true, AllowAutosize: allowAutosize}
	return enrichFieldTarget(spec)
}

func numericUnitField(name string, unit string, allowAutosize bool) FieldSpec {
	spec := numericField(name, allowAutosize)
	spec.Units = unit
	return spec
}

func targetField(name string, role string, targetClass string, targetCollection string, relationship string) FieldSpec {
	spec := field(name, role)
	spec.TargetClass = targetClass
	spec.TargetCollection = targetCollection
	spec.RelationshipType = relationship
	return spec
}

func extensibleField(name string, role string, group string, groupSize int) FieldSpec {
	spec := field(name, role)
	spec.ExtensibleGroup = group
	spec.ExtensibleGroupSize = groupSize
	return spec
}

func enrichFieldTarget(spec FieldSpec) FieldSpec {
	if spec.RelationshipType == "" && spec.Role != "" && spec.Role != fieldRoleName {
		spec.RelationshipType = "references"
	}
	if spec.TargetClass != "" || spec.TargetCollection != "" {
		return spec
	}
	switch spec.Role {
	case fieldRoleZoneRef:
		spec.TargetClass = "zone"
		spec.TargetCollection = "zones"
	case fieldRoleSpaceRef:
		spec.TargetClass = "space"
		spec.TargetCollection = "spaces"
	case fieldRoleScheduleRef:
		spec.TargetClass = "schedule"
		spec.TargetCollection = "schedules"
	case fieldRoleConstructionRef:
		spec.TargetClass = "construction"
		spec.TargetCollection = "constructions"
	case fieldRoleNodeRef:
		spec.TargetClass = "node"
		spec.TargetCollection = "hvac.nodes"
	case fieldRoleNodeListRef:
		spec.TargetClass = "node_or_node_list"
		spec.TargetCollection = "hvac.nodes"
	case fieldRoleBranchRef:
		spec.TargetClass = "branch"
		spec.TargetCollection = "hvac.branches"
	case fieldRoleBranchListRef:
		spec.TargetClass = "branch_list"
		spec.TargetCollection = "hvac.branch_lists"
	case fieldRoleConnectorRef:
		spec.TargetClass = "connector_list"
		spec.TargetCollection = "hvac.connector_lists"
	case fieldRoleObjectTypeRef:
		spec.TargetClass = "object_type"
		spec.TargetCollection = "idf.object_types"
	case fieldRoleObjectRef:
		spec.TargetClass = "object"
		spec.TargetCollection = "idf.objects"
	}
	return spec
}

func fieldSpecAt(objectType string, fieldIndex int) (FieldSpec, bool) {
	spec, ok := objectFieldCatalog[normalizeFieldCatalogKey(objectType)]
	if !ok || fieldIndex < 0 || fieldIndex >= len(spec.Fields) {
		if !ok || fieldIndex < 0 {
			return FieldSpec{}, false
		}
		for _, field := range spec.Fields {
			if field.ExtensibleGroup == "" || field.ExtensibleGroupSize <= 0 {
				continue
			}
			if fieldIndex < field.Index {
				continue
			}
			offset := (fieldIndex - field.Index) % field.ExtensibleGroupSize
			templateIndex := field.Index + offset
			if templateIndex < 0 || templateIndex >= len(spec.Fields) {
				continue
			}
			template := spec.Fields[templateIndex]
			if template.ExtensibleGroup != field.ExtensibleGroup {
				continue
			}
			template.Index = fieldIndex
			return template, true
		}
		return FieldSpec{}, false
	}
	return spec.Fields[fieldIndex], true
}

func fieldCatalogAdapter(version string) EnergyPlusSchemaAdapter {
	return EnergyPlusSchemaAdapter{
		Version:        strings.TrimSpace(version),
		AdapterVersion: manualFieldCatalogAdapterVersion,
		Objects:        objectFieldCatalog,
	}
}

func loadEnergyPlusSchemaAdapter(version string, roots ...string) (EnergyPlusSchemaAdapter, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "unknown"
	}
	for _, root := range roots {
		path := filepath.Join(root, "energyplus", version, "Energy+.schema.epJSON")
		adapter, err := loadEnergyPlusSchemaAdapterFile(version, path)
		if err == nil {
			return adapter, nil
		}
		if !os.IsNotExist(err) {
			return EnergyPlusSchemaAdapter{}, err
		}
	}
	return fieldCatalogAdapter(version), nil
}

func loadEnergyPlusSchemaAdapterFile(version string, path string) (EnergyPlusSchemaAdapter, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return EnergyPlusSchemaAdapter{}, err
	}
	var raw struct {
		Properties map[string]struct {
			Properties map[string]schemaPropertySpec `json:"properties"`
			LegacyIDD  schemaLegacyIDD               `json:"legacy_idd"`
			Required   []string                      `json:"required"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(content, &raw); err != nil {
		return EnergyPlusSchemaAdapter{}, err
	}
	adapter := EnergyPlusSchemaAdapter{
		Version:        version,
		AdapterVersion: "epjson-schema/1",
		SourcePath:     path,
		Objects:        map[string]ObjectSpec{},
	}
	for objectType, objectSchema := range raw.Properties {
		required := map[string]bool{}
		for _, name := range objectSchema.Required {
			required[normalizeFieldName(name)] = true
		}
		spec := ObjectSpec{Type: objectType, SchemaSource: path}
		names := schemaFieldOrder(objectSchema.Properties, objectSchema.LegacyIDD.Fields, objectSchema.LegacyIDD.Alphas.Fields, objectSchema.LegacyIDD.Numerics.Fields)
		for _, name := range names {
			fieldSchema := objectSchema.Properties[name]
			fieldName := humanizeSchemaFieldName(name)
			if info, ok := objectSchema.LegacyIDD.FieldInfo[name]; ok && strings.TrimSpace(info.FieldName) != "" {
				fieldName = strings.TrimSpace(info.FieldName)
			}
			fieldSpec := FieldSpec{
				Name:          fieldName,
				Role:          roleFromFieldComment(name),
				Choices:       append([]string(nil), fieldSchema.Enum...),
				Units:         fieldSchema.Units,
				AllowAutosize: fieldSchema.Autosizable,
				Required:      required[normalizeFieldName(name)],
			}
			fieldSpec.Numeric = schemaTypeIncludesNumber(fieldSchema.Type)
			spec.Fields = append(spec.Fields, enrichFieldTarget(fieldSpec))
		}
		for index := range spec.Fields {
			spec.Fields[index].Index = index
		}
		adapter.Objects[normalizeFieldCatalogKey(objectType)] = spec
	}
	return adapter, nil
}

func schemaFieldOrder(properties map[string]schemaPropertySpec, iddFields []string, alphaFields []string, numericFields []string) []string {
	var names []string
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		if _, ok := properties[value]; !ok {
			return
		}
		names = append(names, value)
		seen[value] = true
	}
	for _, name := range iddFields {
		add(name)
	}
	if len(names) == 0 {
		for _, name := range alphaFields {
			add(name)
		}
		for _, name := range numericFields {
			add(name)
		}
	}
	remaining := make([]string, 0, len(properties))
	for name := range properties {
		if !seen[name] {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	for _, name := range remaining {
		add(name)
	}
	return names
}

func humanizeSchemaFieldName(name string) string {
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	parts := strings.Fields(name)
	for index := range parts {
		if len(parts[index]) > 0 {
			parts[index] = strings.ToUpper(parts[index][:1]) + parts[index][1:]
		}
	}
	return strings.Join(parts, " ")
}

func schemaTypeIncludesNumber(value any) bool {
	switch typed := value.(type) {
	case string:
		return typed == "number" || typed == "integer"
	case []any:
		for _, item := range typed {
			if schemaTypeIncludesNumber(item) {
				return true
			}
		}
	}
	return false
}

func fieldByCatalogName(obj Object, names ...string) (Field, int, bool) {
	wanted := map[string]bool{}
	for _, name := range names {
		key := normalizeFieldName(name)
		if key != "" {
			wanted[key] = true
		}
	}
	if len(wanted) == 0 {
		return Field{}, -1, false
	}
	for index, field := range obj.Fields {
		if spec, ok := fieldSpecAt(obj.Type, index); ok && wanted[normalizeFieldName(spec.Name)] {
			return field, index, true
		}
		if wanted[normalizeFieldName(field.Comment)] {
			return field, index, true
		}
	}
	return Field{}, -1, false
}

func fieldValueByCatalogName(obj Object, names ...string) string {
	field, _, ok := fieldByCatalogName(obj, names...)
	if !ok {
		return ""
	}
	return strings.TrimSpace(field.Value)
}

func fieldValueIndexByCatalogName(obj Object, names ...string) (string, int, bool) {
	field, index, ok := fieldByCatalogName(obj, names...)
	if !ok {
		return "", -1, false
	}
	return strings.TrimSpace(field.Value), index, true
}

func catalogFieldName(obj Object, fieldIndex int) string {
	if spec, ok := fieldSpecAt(obj.Type, fieldIndex); ok && spec.Name != "" {
		return spec.Name
	}
	if fieldIndex >= 0 && fieldIndex < len(obj.Fields) {
		return strings.TrimSpace(obj.Fields[fieldIndex].Comment)
	}
	return ""
}

func catalogFieldRole(obj Object, fieldIndex int) string {
	if spec, ok := fieldSpecAt(obj.Type, fieldIndex); ok {
		return spec.Role
	}
	return ""
}

func SuggestFieldValues(doc Document, objectIndex int, fieldIndex int) []FieldValueSuggestion {
	if objectIndex < 0 || objectIndex >= len(doc.Objects) {
		return nil
	}
	obj := doc.Objects[objectIndex]
	if fieldIndex < 0 || fieldIndex >= len(obj.Fields) {
		return nil
	}
	spec, _ := fieldSpecAt(obj.Type, fieldIndex)
	suggestions := suggestionsForFieldSpec(doc, spec)
	if len(suggestions) == 0 {
		suggestions = suggestionsForComment(doc, obj.Fields[fieldIndex].Comment)
	}
	return uniqueFieldSuggestions(suggestions)
}

func fieldCatalogDiagnostics(doc Document) []Diagnostic {
	var diagnostics []Diagnostic
	for _, obj := range doc.Objects {
		for fieldIndex, field := range obj.Fields {
			value := strings.TrimSpace(field.Value)
			if value == "" {
				continue
			}
			spec, hasSpec := fieldSpecAt(obj.Type, fieldIndex)
			if !hasSpec {
				spec = FieldSpec{
					Name: catalogFieldName(obj, fieldIndex),
					Role: roleFromFieldComment(field.Comment),
				}
			}
			diagnostics = append(diagnostics, validateCatalogField(doc, obj, fieldIndex, field, spec)...)
		}
	}
	return diagnostics
}

func validateCatalogField(doc Document, obj Object, fieldIndex int, field Field, spec FieldSpec) []Diagnostic {
	value := strings.TrimSpace(field.Value)
	if value == "" {
		return nil
	}
	fieldName := catalogFieldName(obj, fieldIndex)
	if fieldName == "" {
		fieldName = spec.Name
	}
	if len(spec.Choices) > 0 && !stringInSet(value, spec.Choices) {
		return []Diagnostic{{
			Severity:    DiagnosticWarning,
			Category:    "Field Value",
			Code:        "invalid_choice",
			Source:      "energyplus_rule",
			Confidence:  "high",
			Message:     fmt.Sprintf("%s expects one of: %s.", fieldName, strings.Join(spec.Choices, ", ")),
			ObjectIndex: obj.Index,
			ObjectType:  obj.Type,
			ObjectName:  objectName(obj),
			FieldIndex:  fieldIndex,
			Field:       fieldName,
			Value:       value,
			Evidence:    "Field catalog enumerated choice list.",
		}}
	}
	if spec.Numeric && !(spec.AllowAutosize && isFlexibleSizingValue(value)) {
		if _, ok := parseFloatField(value); !ok {
			return []Diagnostic{{
				Severity:    DiagnosticError,
				Category:    "Field Value",
				Code:        "invalid_number",
				Source:      "energyplus_rule",
				Confidence:  "high",
				Message:     fmt.Sprintf("%s expects a numeric value.", fieldName),
				ObjectIndex: obj.Index,
				ObjectType:  obj.Type,
				ObjectName:  objectName(obj),
				FieldIndex:  fieldIndex,
				Field:       fieldName,
				Value:       value,
				Evidence:    "Field catalog numeric type.",
			}}
		}
	}
	if spec.Role == "" {
		return nil
	}
	if referenceRoleAllowsImplicitValue(spec.Role) {
		return nil
	}
	if !referenceExistsForRole(doc, spec.Role, value) {
		return []Diagnostic{{
			Severity:    DiagnosticError,
			Category:    "Reference",
			Code:        "missing_catalog_reference",
			Source:      "energyplus_rule",
			Confidence:  "high",
			Message:     fmt.Sprintf("%s references missing value %q.", fieldName, value),
			ObjectIndex: obj.Index,
			ObjectType:  obj.Type,
			ObjectName:  objectName(obj),
			FieldIndex:  fieldIndex,
			Field:       fieldName,
			Value:       value,
			Evidence:    "Field catalog role " + spec.Role,
		}}
	}
	return nil
}

func suggestionsForFieldSpec(doc Document, spec FieldSpec) []FieldValueSuggestion {
	var suggestions []FieldValueSuggestion
	for _, choice := range spec.Choices {
		suggestions = append(suggestions, FieldValueSuggestion{Value: choice, Source: "catalog"})
	}
	if spec.Numeric && spec.AllowAutosize {
		suggestions = append(suggestions, FieldValueSuggestion{Value: "Autosize", Source: "catalog"})
	}
	switch spec.Role {
	case fieldRoleZoneRef:
		suggestions = append(suggestions, objectNameSuggestions(doc, "Zone")...)
	case fieldRoleSpaceRef:
		suggestions = append(suggestions, objectNameSuggestions(doc, "Space")...)
	case fieldRoleScheduleRef:
		suggestions = append(suggestions, objectNameSuggestionsByPredicate(doc, isScheduleType)...)
	case fieldRoleConstructionRef:
		suggestions = append(suggestions, objectNameSuggestionsByPredicate(doc, func(objectType string) bool {
			lower := normalizeFieldCatalogKey(objectType)
			return lower == "construction" || strings.HasPrefix(lower, "construction:")
		})...)
	case fieldRoleBranchRef:
		suggestions = append(suggestions, objectNameSuggestions(doc, "Branch")...)
	case fieldRoleBranchListRef:
		suggestions = append(suggestions, objectNameSuggestions(doc, "BranchList")...)
	case fieldRoleConnectorRef:
		suggestions = append(suggestions, objectNameSuggestions(doc, "ConnectorList", "Connector:Splitter", "Connector:Mixer")...)
	case fieldRoleNodeListRef:
		suggestions = append(suggestions, objectNameSuggestions(doc, "NodeList")...)
		suggestions = append(suggestions, nodeNameSuggestions(doc)...)
	case fieldRoleNodeRef:
		suggestions = append(suggestions, nodeNameSuggestions(doc)...)
	case fieldRoleObjectTypeRef:
		suggestions = append(suggestions, objectTypeSuggestions(doc)...)
	case fieldRoleObjectRef:
		suggestions = append(suggestions, namedObjectSuggestions(doc)...)
	}
	return suggestions
}

func suggestionsForComment(doc Document, comment string) []FieldValueSuggestion {
	role := roleFromFieldComment(comment)
	if role == "" {
		return nil
	}
	return suggestionsForFieldSpec(doc, FieldSpec{Role: role})
}

func roleFromFieldComment(comment string) string {
	normalized := normalizeFieldName(comment)
	switch {
	case strings.Contains(normalized, "space") && strings.Contains(normalized, "name"):
		return fieldRoleSpaceRef
	case strings.Contains(normalized, "construction") && strings.Contains(normalized, "name"):
		return fieldRoleConstructionRef
	case strings.Contains(normalized, "node list"):
		return fieldRoleNodeListRef
	case strings.Contains(normalized, "node") && strings.Contains(normalized, "name"):
		return fieldRoleNodeRef
	case strings.Contains(normalized, "branch list"):
		return fieldRoleBranchListRef
	case strings.Contains(normalized, "branch") && strings.Contains(normalized, "name"):
		return fieldRoleBranchRef
	case strings.Contains(normalized, "connector list"):
		return fieldRoleConnectorRef
	case strings.Contains(normalized, "zone") && strings.Contains(normalized, "name"):
		return fieldRoleZoneRef
	case strings.Contains(normalized, "schedule") && strings.Contains(normalized, "name"):
		return fieldRoleScheduleRef
	default:
		return ""
	}
}

func objectNameSuggestions(doc Document, types ...string) []FieldValueSuggestion {
	allowed := map[string]bool{}
	for _, objectType := range types {
		allowed[normalizeFieldCatalogKey(objectType)] = true
	}
	var suggestions []FieldValueSuggestion
	for _, obj := range doc.Objects {
		if len(allowed) > 0 && !allowed[normalizeFieldCatalogKey(obj.Type)] {
			continue
		}
		if name := objectName(obj); name != "" {
			suggestions = append(suggestions, FieldValueSuggestion{Value: name, Label: obj.Type, Source: "document"})
		}
	}
	return suggestions
}

func namedObjectSuggestions(doc Document) []FieldValueSuggestion {
	return objectNameSuggestions(doc)
}

func objectTypeSuggestions(doc Document) []FieldValueSuggestion {
	seen := map[string]bool{}
	var suggestions []FieldValueSuggestion
	for _, obj := range doc.Objects {
		if seen[strings.ToLower(obj.Type)] {
			continue
		}
		seen[strings.ToLower(obj.Type)] = true
		suggestions = append(suggestions, FieldValueSuggestion{Value: obj.Type, Source: "document"})
	}
	return suggestions
}

func nodeNameSuggestions(doc Document) []FieldValueSuggestion {
	seen := map[string]bool{}
	var suggestions []FieldValueSuggestion
	for _, obj := range doc.Objects {
		for index, field := range obj.Fields {
			if !fieldLooksLikeNodeRef(obj, index, field) {
				continue
			}
			value := strings.TrimSpace(field.Value)
			if value == "" || seen[normalizeName(value)] {
				continue
			}
			seen[normalizeName(value)] = true
			suggestions = append(suggestions, FieldValueSuggestion{Value: value, Label: obj.Type, Source: "document"})
		}
	}
	return suggestions
}

func fieldLooksLikeNodeRef(obj Object, fieldIndex int, field Field) bool {
	role := catalogFieldRole(obj, fieldIndex)
	if role == fieldRoleNodeRef || role == fieldRoleNodeListRef {
		return true
	}
	comment := normalizeFieldName(field.Comment)
	return strings.Contains(comment, "node") && strings.Contains(comment, "name")
}

func referenceExistsForRole(doc Document, role string, value string) bool {
	switch role {
	case fieldRoleZoneRef:
		return namedObjectExists(doc, value, "Zone")
	case fieldRoleSpaceRef:
		return namedObjectExists(doc, value, "Space")
	case fieldRoleBranchRef:
		return namedObjectExists(doc, value, "Branch")
	case fieldRoleBranchListRef:
		return namedObjectExists(doc, value, "BranchList")
	case fieldRoleConnectorRef:
		return namedObjectExists(doc, value, "ConnectorList", "Connector:Splitter", "Connector:Mixer")
	case fieldRoleObjectRef:
		return namedObjectExists(doc, value)
	case fieldRoleScheduleRef:
		return namedObjectExistsByPredicate(doc, value, isScheduleType)
	case fieldRoleConstructionRef:
		return namedObjectExistsByPredicate(doc, value, func(objectType string) bool {
			lower := normalizeFieldCatalogKey(objectType)
			return lower == "construction" || strings.HasPrefix(lower, "construction:")
		})
	default:
		return true
	}
}

func namedObjectExists(doc Document, value string, types ...string) bool {
	key := normalizeName(value)
	allowed := map[string]bool{}
	for _, objectType := range types {
		allowed[normalizeFieldCatalogKey(objectType)] = true
	}
	for _, obj := range doc.Objects {
		if len(allowed) > 0 && !allowed[normalizeFieldCatalogKey(obj.Type)] {
			continue
		}
		if normalizeName(objectName(obj)) == key {
			return true
		}
	}
	return false
}

func namedObjectExistsByPredicate(doc Document, value string, match func(string) bool) bool {
	key := normalizeName(value)
	for _, obj := range doc.Objects {
		if !match(obj.Type) {
			continue
		}
		if normalizeName(objectName(obj)) == key {
			return true
		}
	}
	return false
}

func referenceRoleAllowsImplicitValue(role string) bool {
	return role == fieldRoleNodeRef || role == fieldRoleNodeListRef || role == fieldRoleObjectTypeRef
}

func uniqueFieldSuggestions(values []FieldValueSuggestion) []FieldValueSuggestion {
	seen := map[string]bool{}
	var out []FieldValueSuggestion
	for _, suggestion := range values {
		if strings.TrimSpace(suggestion.Value) == "" {
			continue
		}
		key := normalizeName(suggestion.Value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, suggestion)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return strings.ToLower(out[i].Value) < strings.ToLower(out[j].Value)
	})
	return out
}

func stringInSet(value string, choices []string) bool {
	for _, choice := range choices {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(choice)) {
			return true
		}
	}
	return false
}

func isFlexibleSizingValue(value string) bool {
	return strings.EqualFold(value, "autosize") || strings.EqualFold(value, "autocalculate")
}

func normalizeFieldCatalogKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeFieldName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.Join(strings.Fields(value), " ")
	return value
}
