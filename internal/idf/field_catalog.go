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
	"airterminal:singleduct:seriespiu:reheat": catalogObject("AirTerminal:SingleDuct:SeriesPIU:Reheat",
		field("Name", fieldRoleName),
		field("Availability Schedule Name", fieldRoleScheduleRef),
		numericUnitField("Maximum Air Flow Rate", "m3/s", true),
		numericUnitField("Maximum Primary Air Flow Rate", "m3/s", true),
		numericField("Minimum Primary Air Flow Fraction", true),
		field("Supply Air Inlet Node Name", fieldRoleNodeRef),
		field("Secondary Air Inlet Node Name", fieldRoleNodeRef),
		field("Outlet Node Name", fieldRoleNodeRef),
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
