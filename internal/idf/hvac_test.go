package idf

import (
	"os"
	"strings"
	"testing"
)

func TestAnalyzeHVACBuildsLoopAndZoneRelations(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "AirLoopHVAC", Fields: []Field{
			{Value: "Main Air Loop"},
			{Value: ""},
			{Value: ""},
			{Value: "Autosize"},
			{Value: "Air Branches"},
			{Value: ""},
			{Value: "Air Supply Inlet"},
			{Value: "Air Demand Outlet"},
			{Value: "Air Demand Inlet"},
			{Value: "Air Supply Outlet"},
		}},
		{Index: 1, Type: "BranchList", Fields: []Field{
			{Value: "Air Branches"},
			{Value: "Main Air Branch"},
		}},
		{Index: 2, Type: "Branch", Fields: []Field{
			{Value: "Main Air Branch"},
			{Value: ""},
			{Value: "Fan:ConstantVolume"},
			{Value: "Supply Fan"},
			{Value: "Air Supply Inlet"},
			{Value: "Fan Outlet"},
			{Value: "Coil:Cooling:Water"},
			{Value: "Cooling Coil"},
			{Value: "Fan Outlet"},
			{Value: "Air Supply Outlet"},
		}},
		{Index: 3, Type: "Fan:ConstantVolume", Fields: []Field{
			{Value: "Supply Fan", Comment: "Name"},
			{Value: "Air Supply Inlet", Comment: "Air Inlet Node Name"},
			{Value: "Fan Outlet", Comment: "Air Outlet Node Name"},
		}},
		{Index: 4, Type: "Coil:Cooling:Water", Fields: []Field{
			{Value: "Cooling Coil", Comment: "Name"},
			{Value: "Fan Outlet", Comment: "Air Inlet Node Name"},
			{Value: "Air Supply Outlet", Comment: "Air Outlet Node Name"},
			{Value: "CHW Supply", Comment: "Water Inlet Node Name"},
			{Value: "CHW Return", Comment: "Water Outlet Node Name"},
		}},
		{Index: 5, Type: "PlantLoop", Fields: []Field{
			{Value: "Chilled Water Loop"},
			{Value: "Water"},
			{Value: ""},
			{Value: ""},
			{Value: "CHW Setpoint"},
			{Value: "15"},
			{Value: "5"},
			{Value: "Autosize"},
			{Value: "0"},
			{Value: "Autosize"},
			{Value: "Plant Supply Inlet"},
			{Value: "Plant Supply Outlet"},
			{Value: ""},
			{Value: ""},
			{Value: "CHW Supply"},
			{Value: "CHW Return"},
			{Value: "CHW Demand Branches"},
			{Value: ""},
		}},
		{Index: 6, Type: "BranchList", Fields: []Field{
			{Value: "CHW Demand Branches"},
			{Value: "CHW Coil Branch"},
		}},
		{Index: 7, Type: "Branch", Fields: []Field{
			{Value: "CHW Coil Branch"},
			{Value: ""},
			{Value: "Coil:Cooling:Water"},
			{Value: "Cooling Coil"},
			{Value: "CHW Supply"},
			{Value: "CHW Return"},
		}},
		{Index: 8, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 9, Type: "NodeList", Fields: []Field{
			{Value: "Office Inlets"},
			{Value: "Office Supply Inlet"},
		}},
		{Index: 10, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: "Office Inlets"},
			{Value: ""},
			{Value: "Office Zone Air Node"},
			{Value: "Office Return Node"},
		}},
		{Index: 11, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "AirTerminal:SingleDuct:ConstantVolume:NoReheat"},
			{Value: "Office Terminal"},
			{Value: "1"},
			{Value: "1"},
		}},
		{Index: 12, Type: "AirTerminal:SingleDuct:ConstantVolume:NoReheat", Fields: []Field{
			{Value: "Office Terminal", Comment: "Name"},
			{Value: "Air Demand Inlet", Comment: "Air Inlet Node Name"},
			{Value: "Office Supply Inlet", Comment: "Air Outlet Node Name"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	if report.AirLoopCount != 1 || report.PlantLoopCount != 1 {
		t.Fatalf("loop counts = air %d plant %d, want 1/1", report.AirLoopCount, report.PlantLoopCount)
	}
	airLoop := findHVACTestingLoop(report, "Main Air Loop")
	if airLoop == nil {
		t.Fatalf("Main Air Loop not found in %#v", report.Loops)
	}
	if got := len(airLoop.SupplySide.Branches[0].Components); got != 2 {
		t.Fatalf("air branch components = %d, want 2", got)
	}
	if len(airLoop.RelatedLoops) == 0 {
		t.Fatalf("expected cross-loop relation for shared cooling coil")
	}
	if len(report.ZoneRelations) != 1 {
		t.Fatalf("zone relation count = %d, want 1", len(report.ZoneRelations))
	}
	relation := report.ZoneRelations[0]
	if !stringSliceContainsFold(relation.AirLoopNames, "Main Air Loop") {
		t.Fatalf("air loop names = %#v, want Main Air Loop", relation.AirLoopNames)
	}
	if !stringSliceContainsFold(relation.PlantLoopNames, "Chilled Water Loop") {
		t.Fatalf("plant loop names = %#v, want Chilled Water Loop", relation.PlantLoopNames)
	}
	if relation.RelationSource != "cross_confirmed" || relation.Confidence != "high" {
		t.Fatalf("relation evidence = source %q confidence %q, want cross_confirmed/high", relation.RelationSource, relation.Confidence)
	}
	if len(relation.AirLoopRelations) != 1 || relation.AirLoopRelations[0].Confidence != "high" {
		t.Fatalf("air loop relations = %#v, want high-confidence relation", relation.AirLoopRelations)
	}
	if len(relation.TerminalUnits) != 1 || !relation.TerminalUnits[0].InletOnAirLoopDemand || !relation.TerminalUnits[0].OutletMatchesZoneInlet {
		t.Fatalf("terminal evidence = %#v, want demand path and zone inlet match", relation.TerminalUnits)
	}
	if relation.TerminalUnits[0].Family != "terminal" {
		t.Fatalf("terminal family = %q, want terminal", relation.TerminalUnits[0].Family)
	}
	if hasHVACWarningCode(report.Warnings, "water_coil_missing_plant_loop") {
		t.Fatalf("unexpected water coil warning: %#v", report.Warnings)
	}
}

func TestAnalyzeHVACReportsMissingBranch(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "AirLoopHVAC", Fields: []Field{
			{Value: "Main Air Loop"},
			{Value: ""},
			{Value: ""},
			{Value: "Autosize"},
			{Value: "Air Branches"},
		}},
		{Index: 1, Type: "BranchList", Fields: []Field{
			{Value: "Air Branches"},
			{Value: "Missing Branch"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	if !hasHVACWarningCode(report.Warnings, "missing_branch") {
		t.Fatalf("warnings = %#v, want missing_branch", report.Warnings)
	}
}

func TestAnalyzeHVACReadsZoneEquipmentListWithLoadDistributionScheme(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "NodeList", Fields: []Field{
			{Value: "Office Inlets"},
			{Value: "Office Supply Inlet"},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: "Office Inlets"},
			{Value: ""},
			{Value: "Office Zone Air Node"},
			{Value: "Office Return Node"},
		}},
		{Index: 3, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "SequentialLoad"},
			{Value: "AirTerminal:SingleDuct:ConstantVolume:NoReheat"},
			{Value: "Office Terminal"},
			{Value: "1"},
			{Value: "1"},
		}},
		{Index: 4, Type: "AirTerminal:SingleDuct:ConstantVolume:NoReheat", Fields: []Field{
			{Value: "Office Terminal", Comment: "Name"},
			{Value: "Air Demand Inlet", Comment: "Air Inlet Node Name"},
			{Value: "Office Supply Inlet", Comment: "Air Outlet Node Name"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	if len(report.ZoneRelations) != 1 {
		t.Fatalf("zone relation count = %d, want 1", len(report.ZoneRelations))
	}
	relation := report.ZoneRelations[0]
	if got := len(relation.ZoneEquipment); got != 1 {
		t.Fatalf("zone equipment count = %d, want 1: %#v", got, relation.ZoneEquipment)
	}
	if got := len(relation.TerminalUnits); got != 1 {
		t.Fatalf("terminal count = %d, want 1: %#v", got, relation.TerminalUnits)
	}
	if relation.ZoneEquipment[0].ObjectName != "Office Terminal" {
		t.Fatalf("zone equipment = %#v, want Office Terminal", relation.ZoneEquipment[0])
	}
}

func TestAnalyzeHVACReadsZoneEquipmentListSixFieldGroup(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "NodeList", Fields: []Field{{Value: "Office Inlets"}, {Value: "Office Supply Inlet"}}},
		{Index: 2, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: "Office Inlets"},
			{Value: "Office Exhaust"},
			{Value: "Office Zone Air Node"},
			{Value: "Office Return Node"},
		}},
		{Index: 3, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "SequentialLoad"},
			{Value: "AirTerminal:SingleDuct:ConstantVolume:NoReheat"},
			{Value: "Office Terminal"},
			{Value: "1"},
			{Value: "2"},
			{Value: "CoolFrac"},
			{Value: "HeatFrac"},
		}},
		{Index: 4, Type: "AirTerminal:SingleDuct:ConstantVolume:NoReheat", Fields: []Field{
			{Value: "Office Terminal", Comment: "Name"},
			{Value: "Air Demand Inlet", Comment: "Air Inlet Node Name"},
			{Value: "Office Supply Inlet", Comment: "Air Outlet Node Name"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	if len(report.ZoneRelations) != 1 || len(report.ZoneRelations[0].ZoneEquipment) != 1 {
		t.Fatalf("zone equipment not parsed: %#v", report.ZoneRelations)
	}
	equipment := report.ZoneRelations[0].ZoneEquipment[0]
	if equipment.CoolingSequence != "1" || equipment.HeatingSequence != "2" || equipment.CoolingFractionSchedule != "CoolFrac" || equipment.HeatingFractionSchedule != "HeatFrac" {
		t.Fatalf("six-field ZoneHVAC metadata = %#v", equipment)
	}
	if got := report.ZoneRelations[0].Nodes.ExhaustNodes; !stringSliceContainsFold(got, "Office Exhaust") {
		t.Fatalf("exhaust nodes = %#v, want Office Exhaust", got)
	}
	if len(report.ZoneRelations[0].Nodes.Sources) == 0 {
		t.Fatalf("node source expansion is empty: %#v", report.ZoneRelations[0].Nodes)
	}
}

func TestAnalyzeHVACBuildsSpaceHVACRelation(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Open Office"}}},
		{Index: 1, Type: "Space", Fields: []Field{
			{Value: "Open Office Space", Comment: "Name"},
			{Value: "Open Office", Comment: "Zone Name"},
		}},
		{Index: 2, Type: "NodeList", Fields: []Field{
			{Value: "Open Office Space Inlets"},
			{Value: "Open Office Space Supply Inlet"},
		}},
		{Index: 3, Type: "SpaceHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Open Office Space", Comment: "Space Name"},
			{Value: "Open Office Space Equipment", Comment: "Space Conditioning Equipment List Name"},
			{Value: "Open Office Space Inlets", Comment: "Space Air Inlet Node or NodeList Name"},
			{Value: "", Comment: "Space Air Exhaust Node or NodeList Name"},
			{Value: "Open Office Space Air Node", Comment: "Space Air Node Name"},
			{Value: "Open Office Space Return Node", Comment: "Space Return Air Node or NodeList Name"},
		}},
		{Index: 4, Type: "SpaceHVAC:EquipmentList", Fields: []Field{
			{Value: "Open Office Space Equipment"},
			{Value: "SequentialLoad"},
			{Value: "AirTerminal:SingleDuct:ConstantVolume:NoReheat"},
			{Value: "Open Office Space Terminal"},
			{Value: "1"},
			{Value: "1"},
		}},
		{Index: 5, Type: "AirTerminal:SingleDuct:ConstantVolume:NoReheat", Fields: []Field{
			{Value: "Open Office Space Terminal", Comment: "Name"},
			{Value: "Air Demand Inlet", Comment: "Air Inlet Node Name"},
			{Value: "Open Office Space Supply Inlet", Comment: "Air Outlet Node Name"},
		}},
		{Index: 6, Type: "SpaceHVAC:ZoneEquipmentSplitter", Fields: []Field{
			{Value: "Open Office Splitter"},
			{Value: "Open Office"},
			{Value: "Open Office Space"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	if len(report.ZoneRelations) != 1 {
		t.Fatalf("space relation count = %d, want 1: %#v", len(report.ZoneRelations), report.ZoneRelations)
	}
	relation := report.ZoneRelations[0]
	if relation.RelationScope != "space" || relation.SpaceName != "Open Office Space" || relation.ZoneName != "Open Office" {
		t.Fatalf("space relation identity = %#v", relation)
	}
	if relation.SpaceObjectIndex != 1 || relation.ZoneObjectIndex != 0 {
		t.Fatalf("space/zone object indexes = %d/%d, want 1/0", relation.SpaceObjectIndex, relation.ZoneObjectIndex)
	}
	if got := len(relation.ZoneEquipment); got != 1 {
		t.Fatalf("space equipment count = %d, want 1: %#v", got, relation.ZoneEquipment)
	}
	if relation.ZoneEquipment[0].RoleHere != "zone_terminal" {
		t.Fatalf("space equipment role = %q, want zone_terminal", relation.ZoneEquipment[0].RoleHere)
	}
	if len(relation.TerminalUnits) != 1 || !relation.TerminalUnits[0].OutletMatchesZoneInlet {
		t.Fatalf("space terminal evidence = %#v", relation.TerminalUnits)
	}
	hasNodeListSource := false
	for _, source := range relation.Nodes.Sources {
		if source.SourceType == "node_list_expansion" && source.InputValue == "Open Office Space Inlets" {
			hasNodeListSource = true
		}
	}
	if !hasNodeListSource {
		t.Fatalf("space node source expansion = %#v", relation.Nodes.Sources)
	}
	if !stringSliceContainsFold(relation.Evidence, "Open Office Splitter") {
		t.Fatalf("space evidence = %#v, want splitter evidence", relation.Evidence)
	}
}

func TestAnalyzeHVACBuildsAirLoopDemandGraphFromSupplyAndReturnPaths(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "AirLoopHVAC", Fields: []Field{
			{Value: "Main Air Loop"},
			{Value: ""},
			{Value: ""},
			{Value: "Autosize"},
			{Value: ""},
			{Value: ""},
			{Value: "Supply Side Inlet"},
			{Value: "Return Path Outlet"},
			{Value: "Supply Path Inlet"},
			{Value: "Supply Side Outlet"},
		}},
		{Index: 1, Type: "AirLoopHVAC:SupplyPath", Fields: []Field{
			{Value: "Main Supply Path"},
			{Value: "Supply Path Inlet"},
			{Value: "AirLoopHVAC:ZoneSplitter"},
			{Value: "Main Splitter"},
		}},
		{Index: 2, Type: "AirLoopHVAC:ZoneSplitter", Fields: []Field{
			{Value: "Main Splitter"},
			{Value: "Supply Path Inlet"},
			{Value: "Office Terminal Inlet"},
		}},
		{Index: 3, Type: "AirLoopHVAC:ReturnPath", Fields: []Field{
			{Value: "Main Return Path"},
			{Value: "Return Path Outlet"},
			{Value: "AirLoopHVAC:ZoneMixer"},
			{Value: "Main Mixer"},
		}},
		{Index: 4, Type: "AirLoopHVAC:ZoneMixer", Fields: []Field{
			{Value: "Main Mixer"},
			{Value: "Return Path Outlet"},
			{Value: "Office Return Node"},
		}},
		{Index: 5, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 6, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: "Office Supply Inlet"},
			{Value: ""},
			{Value: "Office Zone Air Node"},
			{Value: "Office Return Node"},
		}},
		{Index: 7, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "AirTerminal:SingleDuct:ConstantVolume:NoReheat"},
			{Value: "Office Terminal"},
			{Value: "1"},
			{Value: "1"},
		}},
		{Index: 8, Type: "AirTerminal:SingleDuct:ConstantVolume:NoReheat", Fields: []Field{
			{Value: "Office Terminal", Comment: "Name"},
			{Value: "Office Terminal Inlet", Comment: "Air Inlet Node Name"},
			{Value: "Office Supply Inlet", Comment: "Air Outlet Node Name"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	loop := findHVACTestingLoop(report, "Main Air Loop")
	if loop == nil {
		t.Fatalf("Main Air Loop not found: %#v", report.Loops)
	}
	if loop.DemandGraph.SupplyPath == nil || loop.DemandGraph.ReturnPath == nil {
		t.Fatalf("demand graph paths = %#v, want supply and return paths", loop.DemandGraph)
	}
	if !hasDemandGraphEdge(loop.DemandGraph, "zone_splitter", "Supply Path Inlet", "Office Terminal Inlet") {
		t.Fatalf("demand graph edges = %#v, want zone splitter edge", loop.DemandGraph.Edges)
	}
	if !hasDemandGraphEdge(loop.DemandGraph, "zone_mixer", "Office Return Node", "Return Path Outlet") {
		t.Fatalf("demand graph edges = %#v, want zone mixer edge", loop.DemandGraph.Edges)
	}

	relation := findHVACTestingZoneRelation(report, "Office")
	if relation == nil || len(relation.AirLoopRelations) != 1 {
		t.Fatalf("Office air loop relation = %#v", report.ZoneRelations)
	}
	airRelation := relation.AirLoopRelations[0]
	if airRelation.Source != "terminal_inlet_on_demand_path" || airRelation.Confidence != "high" {
		t.Fatalf("air relation = %#v, want terminal_inlet_on_demand_path/high", airRelation)
	}
	if !stringSliceContainsFold(airRelation.Evidence, `Terminal inlet node "Office Terminal Inlet" is on the AirLoop demand graph.`) {
		t.Fatalf("air relation evidence = %#v, want terminal inlet graph evidence", airRelation.Evidence)
	}
	if !stringSliceContainsFold(airRelation.Evidence, "Zone return node is present on the AirLoop return path graph.") {
		t.Fatalf("air relation evidence = %#v, want return path evidence", airRelation.Evidence)
	}
	if len(relation.TerminalUnits) != 1 || !relation.TerminalUnits[0].InletOnAirLoopDemand {
		t.Fatalf("terminal demand evidence = %#v, want inlet on demand graph", relation.TerminalUnits)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{"demand_graph:", "supply_path:", "return_path:", "role: zone_splitter", "role: zone_mixer"} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic HVAC demand graph missing %q:\n%s", expected, projection.Text)
		}
	}
}

func TestAnalyzeHVACUsesTypedComponentReferenceGraphForPlantRelation(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: "Office Supply Inlet"},
			{Value: ""},
			{Value: "Office Zone Air Node"},
			{Value: "Office Return Node"},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "AirTerminal:SingleDuct:VAV:Reheat"},
			{Value: "Office VAV"},
			{Value: "1"},
			{Value: "1"},
		}},
		{Index: 3, Type: "AirTerminal:SingleDuct:VAV:Reheat", Fields: []Field{
			{Value: "Office VAV", Comment: "Name"},
			{Value: "Office Terminal Inlet", Comment: "Air Inlet Node Name"},
			{Value: "Office Supply Inlet", Comment: "Air Outlet Node Name"},
			{Value: "Coil:Heating:Water", Comment: "Reheat Coil Object Type"},
			{Value: "Office Reheat Coil", Comment: "Reheat Coil Name"},
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
			{Value: ""},
			{Value: ""},
			{Value: "HW Demand Inlet"},
			{Value: "HW Demand Outlet"},
			{Value: "HW Demand Branches"},
			{Value: ""},
		}},
		{Index: 5, Type: "BranchList", Fields: []Field{
			{Value: "HW Demand Branches"},
			{Value: "Reheat Coil Branch"},
		}},
		{Index: 6, Type: "Branch", Fields: []Field{
			{Value: "Reheat Coil Branch"},
			{Value: ""},
			{Value: "Coil:Heating:Water"},
			{Value: "Office Reheat Coil"},
			{Value: "HW Demand Inlet"},
			{Value: "HW Demand Outlet"},
		}},
		{Index: 7, Type: "Coil:Heating:Water", Fields: []Field{
			{Value: "Office Reheat Coil", Comment: "Name"},
			{Value: "HW Demand Inlet", Comment: "Water Inlet Node Name"},
			{Value: "HW Demand Outlet", Comment: "Water Outlet Node Name"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	relation := findHVACTestingZoneRelation(report, "Office")
	if relation == nil {
		t.Fatalf("Office relation not found: %#v", report.ZoneRelations)
	}
	if !stringSliceContainsFold(relation.PlantLoopNames, "Heating Water Loop") {
		t.Fatalf("plant loops = %#v, want Heating Water Loop from terminal->coil reference", relation.PlantLoopNames)
	}
	if len(report.ComponentReferences) == 0 {
		t.Fatalf("component references are empty")
	}
	if !hasHVACComponentReference(report.ComponentReferences, "Office VAV", "Coil:Heating:Water", "Office Reheat Coil", "internal_component_reference") {
		t.Fatalf("component references = %#v, want Office VAV -> Office Reheat Coil", report.ComponentReferences)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{"component_references:", `target_class: "Coil:Heating:Water"`, "relation_role: internal_component_reference"} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic component references missing %q:\n%s", expected, projection.Text)
		}
	}
}

func TestAnalyzeHVACCondenserLoopAndLoopRuleWarnings(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "CondenserLoop", Fields: []Field{
			{Value: "Condenser Loop"},
			{Value: "Water"},
			{Value: ""},
			{Value: "Condenser Operation"},
			{Value: "Cnd Setpoint"},
			{Value: "35"},
			{Value: "5"},
			{Value: "Autosize"},
			{Value: "0"},
			{Value: "Autosize"},
			{Value: "Cnd Supply Inlet"},
			{Value: "Cnd Supply Outlet"},
			{Value: "Cnd Branches"},
			{Value: "Cnd Connectors"},
			{Value: "Cnd Demand Inlet"},
			{Value: "Cnd Demand Outlet"},
			{Value: ""},
			{Value: ""},
		}},
		{Index: 1, Type: "BranchList", Fields: []Field{
			{Value: "Cnd Branches"},
			{Value: "Outlet Branch"},
			{Value: "Outlet Branch"},
		}},
		{Index: 2, Type: "Branch", Fields: []Field{
			{Value: "Outlet Branch"},
			{Value: ""},
			{Value: "Connector:Splitter"},
			{Value: "Bad Splitter"},
			{Value: "Wrong Inlet"},
			{Value: "Wrong Outlet"},
		}},
		{Index: 3, Type: "ConnectorList", Fields: []Field{
			{Value: "Cnd Connectors"},
			{Value: "Connector:Splitter"},
			{Value: "Split 1"},
			{Value: "Connector:Splitter"},
			{Value: "Split 2"},
		}},
		{Index: 4, Type: "Connector:Splitter", Fields: []Field{{Value: "Split 1"}, {Value: "Not First Branch"}, {Value: "Outlet Branch"}}},
		{Index: 5, Type: "Connector:Splitter", Fields: []Field{{Value: "Split 2"}, {Value: "Outlet Branch"}, {Value: "Outlet Branch"}}},
	}}

	report := AnalyzeHVAC(doc)
	if report.CondenserLoopCount != 1 {
		t.Fatalf("condenser loop count = %d, want 1", report.CondenserLoopCount)
	}
	for _, code := range []string{
		"duplicate_branch_in_branch_list",
		"branch_list_order_mismatch",
		"connector_inside_branch_component_list",
		"connector_list_invalid_composition",
		"splitter_inlet_not_loop_inlet_branch",
	} {
		if !hasHVACWarningCode(report.Warnings, code) {
			t.Fatalf("warnings = %#v, want %s", report.Warnings, code)
		}
	}
}

func TestAnalyzeHVACComponentFamiliesAndContextRoles(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "CondenserLoop", Fields: []Field{
			{Value: "Condenser Loop"},
			{Value: "Water"},
			{Value: ""},
			{Value: ""},
			{Value: "Cnd Setpoint"},
			{Value: "35"},
			{Value: "5"},
			{Value: "Autosize"},
			{Value: "0"},
			{Value: "Autosize"},
			{Value: "Cnd Supply Inlet"},
			{Value: "Cnd Supply Outlet"},
			{Value: "Cnd Branches"},
			{Value: ""},
		}},
		{Index: 1, Type: "BranchList", Fields: []Field{
			{Value: "Cnd Branches"},
			{Value: "Tower Branch"},
		}},
		{Index: 2, Type: "Branch", Fields: []Field{
			{Value: "Tower Branch"},
			{Value: ""},
			{Value: "Pipe:Adiabatic"},
			{Value: "Bypass Pipe"},
			{Value: "Cnd Supply Inlet"},
			{Value: "Tower Inlet"},
			{Value: "CoolingTower:SingleSpeed"},
			{Value: "Heat Rejection Tower"},
			{Value: "Tower Inlet"},
			{Value: "Cnd Supply Outlet"},
		}},
		{Index: 3, Type: "Pipe:Adiabatic", Fields: []Field{{Value: "Bypass Pipe"}}},
		{Index: 4, Type: "CoolingTower:SingleSpeed", Fields: []Field{{Value: "Heat Rejection Tower"}}},
	}}

	report := AnalyzeHVAC(doc)
	loop := findHVACTestingLoop(report, "Condenser Loop")
	if loop == nil || len(loop.SupplySide.Branches) != 1 {
		t.Fatalf("condenser loop supply branch not parsed: %#v", report.Loops)
	}
	components := loop.SupplySide.Branches[0].Components
	pipe := findHVACTestingComponent(components, "Bypass Pipe")
	if pipe == nil {
		t.Fatalf("pipe component not found: %#v", components)
	}
	if pipe.Family != "pipe" || pipe.DisplayLabel != "Pipe" || pipe.RoleHere != "bypass_pipe" {
		t.Fatalf("pipe classification = %#v, want pipe/Pipe/bypass_pipe", *pipe)
	}
	tower := findHVACTestingComponent(components, "Heat Rejection Tower")
	if tower == nil {
		t.Fatalf("cooling tower component not found: %#v", components)
	}
	if tower.Family != "cooling_tower" || tower.DisplayLabel != "Cooling Tower" || tower.RoleHere != "condenser_supply_reject" {
		t.Fatalf("tower classification = %#v, want cooling_tower/Cooling Tower/condenser_supply_reject", *tower)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{"cooling_towers:", "pipes:", "display_label: Pipe", "role_here: condenser_supply_reject"} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic HVAC catalog missing %q:\n%s", expected, projection.Text)
		}
	}
}

func TestAnalyzeHVACReferenceLargeOfficeRelations(t *testing.T) {
	text, err := os.ReadFile("../../frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf")
	if err != nil {
		t.Fatalf("read reference sample: %v", err)
	}
	doc, err := Parse(string(text))
	if err != nil {
		t.Fatalf("parse reference sample: %v", err)
	}

	report := AnalyzeHVAC(doc)
	if report.AirLoopCount != 4 || report.PlantLoopCount != 3 || report.ZoneRelationCount != 16 {
		t.Fatalf("counts = air %d plant %d zones %d, want 4/3/16", report.AirLoopCount, report.PlantLoopCount, report.ZoneRelationCount)
	}
	if report.WarningCount != 0 {
		t.Fatalf("warnings = %#v, want none for reference sample", report.Warnings)
	}
	relation := findHVACTestingZoneRelation(report, "Basement")
	if relation == nil {
		t.Fatalf("Basement relation not found")
	}
	if !componentSliceContainsName(relation.ZoneEquipment, "Basement VAV Box") {
		t.Fatalf("Basement zone equipment = %#v, want ADU wrapper", relation.ZoneEquipment)
	}
	if !componentSliceContainsName(relation.TerminalUnits, "Basement VAV Box Component") {
		t.Fatalf("Basement terminals = %#v, want resolved VAV terminal", relation.TerminalUnits)
	}
	if relation.Confidence == "" || relation.RelationSource == "" {
		t.Fatalf("Basement relation missing confidence/source: %#v", relation)
	}
	if !stringSliceContainsFold(relation.PlantLoopNames, "HeatSys1") || !stringSliceContainsFold(relation.PlantLoopNames, "CoolSys1") {
		t.Fatalf("Basement plant loops = %#v, want HeatSys1 and CoolSys1", relation.PlantLoopNames)
	}
	if !componentSliceContainsName(relation.PlantEquipment, "HeatSys1 Boiler") || !componentSliceContainsName(relation.PlantEquipment, "CoolSys1 Chiller 1") {
		t.Fatalf("Basement plant equipment = %#v, want source equipment", relation.PlantEquipment)
	}
}

func findHVACTestingLoop(report HVACReport, name string) *HVACLoop {
	for index := range report.Loops {
		if report.Loops[index].Name == name {
			return &report.Loops[index]
		}
	}
	return nil
}

func findHVACTestingZoneRelation(report HVACReport, name string) *HVACZoneChain {
	for index := range report.ZoneRelations {
		if strings.EqualFold(report.ZoneRelations[index].ZoneName, name) {
			return &report.ZoneRelations[index]
		}
	}
	return nil
}

func hasHVACWarningCode(warnings []HVACWarning, code string) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}

func hasDemandGraphEdge(graph AirLoopDemandGraph, role string, fromNode string, toNode string) bool {
	for _, edge := range graph.Edges {
		if edge.Role == role && strings.EqualFold(edge.FromNode, fromNode) && strings.EqualFold(edge.ToNode, toNode) {
			return true
		}
	}
	return false
}

func hasHVACComponentReference(references []HVACComponentReference, fromName string, targetType string, targetName string, role string) bool {
	for _, reference := range references {
		if strings.EqualFold(reference.FromObjectName, fromName) &&
			strings.EqualFold(reference.TargetObjectType, targetType) &&
			strings.EqualFold(reference.TargetObjectName, targetName) &&
			reference.RelationRole == role {
			return true
		}
	}
	return false
}

func componentSliceContainsName(components []HVACComponent, name string) bool {
	for _, component := range components {
		if strings.EqualFold(component.ObjectName, name) {
			return true
		}
	}
	return false
}

func findHVACTestingComponent(components []HVACComponent, name string) *HVACComponent {
	for index := range components {
		if strings.EqualFold(components[index].ObjectName, name) {
			return &components[index]
		}
	}
	return nil
}
