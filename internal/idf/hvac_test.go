package idf

import (
	"encoding/json"
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
	if !stringSliceContainsFold(relation.RuleIDs, hvacRuleZoneHasEquipmentList) || !stringSliceContainsFold(relation.RuleIDs, hvacRuleAirLoopZoneSplitterToTerminal) || !stringSliceContainsFold(relation.RuleIDs, hvacRulePlantComponentOnDemandBranch) {
		t.Fatalf("relation rule ids = %#v, want equipment list, air loop, and plant rules", relation.RuleIDs)
	}
	if len(relation.AirLoopRelations) != 1 || !stringSliceContainsFold(relation.AirLoopRelations[0].RuleIDs, hvacRuleAirLoopZoneSplitterToTerminal) {
		t.Fatalf("air loop relations = %#v, want rule-resolved relation", relation.AirLoopRelations)
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
	if len(report.RuleGraph.Nodes) == 0 || len(report.RuleGraph.Edges) == 0 {
		t.Fatalf("rule graph was not built: %#v", report.RuleGraph)
	}
	for _, edge := range report.RuleGraph.Edges {
		if edge.RuleID == "" || edge.SourceObjectIndex < 0 || len(edge.SourceFieldIndexes) == 0 {
			t.Fatalf("rule edge missing trace: %#v", edge)
		}
	}
	for _, ruleID := range []string{
		hvacRuleBranchListIncludesBranch,
		hvacRuleBranchComponentSeries,
		hvacRuleZoneHasEquipmentConnections,
		hvacRuleZoneHasEquipmentList,
		hvacRuleZoneEquipmentListTerminal,
		hvacRuleZoneTerminalOutletMatchesInlet,
		hvacRuleCrossLoopSameWaterCoil,
	} {
		if !hasHVACRuleEdge(report.RuleGraph, ruleID) {
			t.Fatalf("rule graph missing %s edge: %#v", ruleID, report.RuleGraph.Edges)
		}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(encoded)
	for _, forbidden := range []string{"relationSource", "relationConfidence", "relationEvidence", `"confidence"`, `"evidence"`, `"inferred"`, `"weak"`} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("HVAC JSON contains legacy relation vocabulary %q:\n%s", forbidden, jsonText)
		}
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

func TestAnalyzeHVACUnresolvedComponentsIncludeSourceFields(t *testing.T) {
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
			{Value: "Main Air Branch", Comment: "Name"},
			{Value: "", Comment: "Pressure Drop Curve Name"},
			{Value: "Fan:ConstantVolume", Comment: "Component 1 Object Type"},
			{Value: "Missing Fan", Comment: "Component 1 Name"},
			{Value: "Air Supply Inlet", Comment: "Component 1 Inlet Node Name"},
			{Value: "Air Supply Outlet", Comment: "Component 1 Outlet Node Name"},
		}},
		{Index: 3, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 4, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office", Comment: "Zone Name"},
			{Value: "Office Equipment", Comment: "Zone Conditioning Equipment List Name"},
			{Value: "Office Supply Inlet", Comment: "Zone Air Inlet Node or NodeList Name"},
			{Value: "", Comment: "Zone Air Exhaust Node or NodeList Name"},
			{Value: "Office Zone Air Node", Comment: "Zone Air Node Name"},
			{Value: "Office Return Node", Comment: "Zone Return Air Node or NodeList Name"},
		}},
		{Index: 5, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment", Comment: "Name"},
			{Value: "AirTerminal:SingleDuct:ConstantVolume:NoReheat", Comment: "Zone Equipment 1 Object Type"},
			{Value: "Missing Terminal", Comment: "Zone Equipment 1 Name"},
			{Value: "1", Comment: "Zone Equipment 1 Cooling Sequence"},
			{Value: "1", Comment: "Zone Equipment 1 Heating or No-Load Sequence"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	loop := findHVACTestingLoop(report, "Main Air Loop")
	if loop == nil || len(loop.SupplySide.Branches) == 0 || len(loop.SupplySide.Branches[0].Components) == 0 {
		t.Fatalf("air loop branch component missing from report: %#v", report.Loops)
	}
	branchComponent := loop.SupplySide.Branches[0].Components[0]
	if branchComponent.Exists || branchComponent.SourceOwnerType != "Branch" || branchComponent.SourceOwnerName != "Main Air Branch" {
		t.Fatalf("branch component source = %#v, want unresolved Branch/Main Air Branch", branchComponent)
	}
	if branchComponent.TypeFieldIndex != 2 || branchComponent.NameFieldIndex != 3 || branchComponent.ExpectedObjectType != "Fan:ConstantVolume" {
		t.Fatalf("branch component fields = %#v, want type/name field 2/3", branchComponent)
	}

	relation := findHVACTestingZoneRelation(report, "Office")
	if relation == nil || len(relation.ZoneEquipment) == 0 {
		t.Fatalf("zone equipment missing from report: %#v", report.ZoneRelations)
	}
	zoneEquipment := relation.ZoneEquipment[0]
	if zoneEquipment.Exists || zoneEquipment.SourceOwnerType != "ZoneHVAC:EquipmentList" || zoneEquipment.SourceOwnerName != "Office Equipment" {
		t.Fatalf("zone equipment source = %#v, want unresolved ZoneHVAC:EquipmentList/Office Equipment", zoneEquipment)
	}
	if zoneEquipment.TypeFieldIndex != 1 || zoneEquipment.NameFieldIndex != 2 || zoneEquipment.ExpectedObjectType != "AirTerminal:SingleDuct:ConstantVolume:NoReheat" {
		t.Fatalf("zone equipment fields = %#v, want type/name field 1/2", zoneEquipment)
	}

	branchWarning := findHVACWarningByCode(report.Warnings, "missing_branch_component")
	if branchWarning == nil || branchWarning.FieldIndex != 3 || branchWarning.SourceFieldIndex != 3 || branchWarning.Value != "Missing Fan" {
		t.Fatalf("branch warning = %#v, want missing component name field metadata", branchWarning)
	}
	zoneWarning := findHVACWarningByCode(report.Warnings, "missing_zone_equipment")
	if zoneWarning == nil || zoneWarning.FieldIndex != 2 || zoneWarning.SourceFieldIndex != 2 || zoneWarning.Value != "Missing Terminal" {
		t.Fatalf("zone warning = %#v, want missing equipment name field metadata", zoneWarning)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{"source_reference:", `source_owner: "ZoneHVAC:EquipmentList \"Office Equipment\""`, "name_field_index: 2"} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic unresolved component source missing %q:\n%s", expected, projection.Text)
		}
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

func TestDumpHVACConnectionFieldsUsesCatalogOrder(t *testing.T) {
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
			{Value: "AirTerminal:SingleDuct:ConstantVolume:NoReheat"},
			{Value: "Office Terminal"},
			{Value: "1"},
			{Value: "1"},
		}},
	}}

	rows := dumpHVACConnectionFields(doc)
	if len(rows) != 1 {
		t.Fatalf("debug rows = %#v, want one row", rows)
	}
	row := rows[0]
	if row.RawField0 != "Office" || row.RawField1 != "Office Equipment" {
		t.Fatalf("raw fields = %#v, want Zone Name / Equipment List fields", row)
	}
	if row.ResolvedZoneName != "Office" || row.ResolvedEquipmentListName != "Office Equipment" || !row.EquipmentListObjectFound {
		t.Fatalf("resolved fields = %#v, want Office and matching equipment list", row)
	}
}

func TestAnalyzeHVACTerminalMismatchWarningIncludesEdgeMetadata(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office", Comment: "Zone Name"},
			{Value: "Office Equipment", Comment: "Zone Conditioning Equipment List Name"},
			{Value: "Expected Zone Inlet", Comment: "Zone Air Inlet Node or NodeList Name"},
			{Value: "", Comment: "Zone Air Exhaust Node or NodeList Name"},
			{Value: "Office Zone Air Node", Comment: "Zone Air Node Name"},
			{Value: "Office Return Node", Comment: "Zone Return Air Node or NodeList Name"},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment", Comment: "Name"},
			{Value: "AirTerminal:SingleDuct:ConstantVolume:NoReheat", Comment: "Zone Equipment 1 Object Type"},
			{Value: "Office Terminal", Comment: "Zone Equipment 1 Name"},
			{Value: "1", Comment: "Zone Equipment 1 Cooling Sequence"},
			{Value: "1", Comment: "Zone Equipment 1 Heating or No-Load Sequence"},
		}},
		{Index: 3, Type: "AirTerminal:SingleDuct:ConstantVolume:NoReheat", Fields: []Field{
			{Value: "Office Terminal", Comment: "Name"},
			{Value: "Terminal Inlet", Comment: "Air Inlet Node Name"},
			{Value: "Wrong Outlet", Comment: "Air Outlet Node Name"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	warning := findHVACWarningByCode(report.Warnings, "terminal_not_connected_to_zone_inlet")
	if warning == nil {
		t.Fatalf("warnings = %#v, want terminal_not_connected_to_zone_inlet", report.Warnings)
	}
	if warning.EdgeID == "" || !strings.Contains(warning.EdgeID, "terminal_outlet") {
		t.Fatalf("warning edge id = %q, want terminal outlet edge", warning.EdgeID)
	}
	if warning.FieldIndex != 2 || warning.SourceFieldIndex != 2 || warning.Field != "Air Outlet Node Name" {
		t.Fatalf("warning source field = index %d source %d field %q, want outlet field 2", warning.FieldIndex, warning.SourceFieldIndex, warning.Field)
	}
	if !stringSliceContainsFold(warning.ExpectedNodes, "Expected Zone Inlet") || warning.ActualNode != "Wrong Outlet" {
		t.Fatalf("warning nodes = expected %#v actual %q, want Expected Zone Inlet/Wrong Outlet", warning.ExpectedNodes, warning.ActualNode)
	}
	if warning.SuggestedFixTarget == "" {
		t.Fatalf("warning missing suggested fix target: %#v", *warning)
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
	if !stringSliceContainsFold(relation.RuleTrace, "Open Office Splitter") {
		t.Fatalf("space rule trace = %#v, want splitter trace", relation.RuleTrace)
	}
}

func TestAnalyzeHVACKeepsFourPipeFanCoilAsZoneEquipment(t *testing.T) {
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
	relation := findHVACTestingZoneRelation(report, "Office")
	if relation == nil {
		t.Fatalf("Office relation not found: %#v", report.ZoneRelations)
	}
	if len(relation.AirLoopNames) != 0 || len(relation.TerminalUnits) != 0 {
		t.Fatalf("four-pipe fan coil relation resolved unexpected air terminal/loop: %#v", relation)
	}
	if len(relation.ZoneEquipment) != 1 {
		t.Fatalf("zone equipment = %#v, want one four-pipe fan coil", relation.ZoneEquipment)
	}
	equipment := relation.ZoneEquipment[0]
	if equipment.ObjectType != "ZoneHVAC:FourPipeFanCoil" || equipment.RoleHere != "zone_equipment" || !equipment.ListedInZoneEquipment {
		t.Fatalf("zone equipment metadata = %#v, want listed zone equipment", equipment)
	}
	if equipment.SourceOwnerType != "ZoneHVAC:EquipmentList" || equipment.TypeFieldIndex != 1 || equipment.NameFieldIndex != 2 {
		t.Fatalf("zone equipment source metadata = %#v, want equipment list fields", equipment)
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
	if !stringSliceContainsFold(airRelation.RuleIDs, hvacRuleAirLoopZoneSplitterToTerminal) {
		t.Fatalf("air relation = %#v, want %s rule", airRelation, hvacRuleAirLoopZoneSplitterToTerminal)
	}
	if !stringSliceContainsFold(airRelation.RuleTrace, `Terminal inlet node "Office Terminal Inlet" is on the AirLoop demand graph.`) {
		t.Fatalf("air relation trace = %#v, want terminal inlet graph trace", airRelation.RuleTrace)
	}
	if !stringSliceContainsFold(airRelation.RuleTrace, "Zone return node is present on the AirLoop return path graph.") {
		t.Fatalf("air relation trace = %#v, want return path trace", airRelation.RuleTrace)
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
	for _, forbidden := range []string{"relation_source:", "confidence:", "evidence:"} {
		if strings.Contains(projection.Text, forbidden) {
			t.Fatalf("semantic HVAC projection contains legacy relation vocabulary %q:\n%s", forbidden, projection.Text)
		}
	}
}

func TestBuildServiceChainsFromRuleGraphRequiresDirectedPath(t *testing.T) {
	relation := HVACZoneChain{
		ZoneName:       "Office",
		AirLoopNames:   []string{"Air 1", "Air 2"},
		PlantLoopNames: []string{"Plant 1", "Plant 2"},
		TerminalUnits: []HVACComponent{
			{ObjectType: "AirTerminal:SingleDuct:VAV:Reheat", ObjectName: "Office VAV", ObjectIndex: 3},
		},
		PlantEquipment: []HVACComponent{
			{ObjectType: "Boiler:HotWater", ObjectName: "Boiler 1", ObjectIndex: 4, LoopName: "Plant 1"},
			{ObjectType: "Boiler:HotWater", ObjectName: "Boiler 2", ObjectIndex: 5, LoopName: "Plant 2"},
		},
	}
	terminalID := hvacRuleComponentSourceNodeID(relation.TerminalUnits[0])
	boilerID := hvacRuleComponentSourceNodeID(relation.PlantEquipment[0])
	graph := HVACRuleGraph{
		Nodes: []HVACRuleNode{
			{ID: "loop:air:1", Kind: "loop", ObjectType: "AirLoopHVAC", ObjectName: "Air 1"},
			{ID: "loop:air:2", Kind: "loop", ObjectType: "AirLoopHVAC", ObjectName: "Air 2"},
			{ID: "loop:plant:1", Kind: "loop", ObjectType: "PlantLoop", ObjectName: "Plant 1"},
			{ID: "loop:plant:2", Kind: "loop", ObjectType: "PlantLoop", ObjectName: "Plant 2"},
			{ID: terminalID, Kind: "component", ObjectType: relation.TerminalUnits[0].ObjectType, ObjectName: relation.TerminalUnits[0].ObjectName, ObjectIndex: 3},
			{ID: boilerID, Kind: "component", ObjectType: relation.PlantEquipment[0].ObjectType, ObjectName: relation.PlantEquipment[0].ObjectName, ObjectIndex: 4},
			{ID: "zone:office", Kind: "zone", ObjectType: "Zone", ObjectName: "Office"},
		},
		Edges: []HVACRuleEdge{
			{RuleID: hvacRulePlantComponentOnSupplyBranch, FromID: boilerID, ToID: "loop:plant:1", SourceObjectIndex: 100},
			{RuleID: hvacRuleCrossLoopSameWaterCoil, FromID: "loop:plant:1", ToID: "loop:air:1", SourceObjectIndex: 101},
			{RuleID: hvacRuleAirLoopZoneSplitterToTerminal, FromID: "loop:air:1", ToID: terminalID, SourceObjectIndex: 102},
			{RuleID: hvacRuleZoneTerminalOutletMatchesInlet, FromID: terminalID, ToID: "zone:office", SourceObjectIndex: 103},
		},
	}

	paths := buildServiceChainsFromRuleGraph(relation, graph)
	if len(paths) != 3 {
		t.Fatalf("service path count = %d, want terminal + Air 1 + Plant 1 paths: %#v", len(paths), paths)
	}
	seenAir1 := false
	seenPlant1 := false
	seenDirectedPlantPath := false
	for _, path := range paths {
		if path.AirLoopName == "Air 2" || path.PlantLoop == "Plant 2" {
			t.Fatalf("service path includes disconnected loop: %#v", path)
		}
		if len(path.SourceRelations) == 0 {
			t.Fatalf("service path missing rule trace: %#v", path)
		}
		seenAir1 = seenAir1 || path.AirLoopName == "Air 1"
		seenPlant1 = seenPlant1 || path.PlantLoop == "Plant 1"
		if path.PlantLoop == "Plant 1" {
			if path.AirLoopName != "Air 1" || path.TerminalName == "" {
				t.Fatalf("directed plant service path = %#v, want Plant 1 -> Air 1 -> terminal -> zone", path)
			}
			seenDirectedPlantPath = true
		}
	}
	if !seenAir1 || !seenPlant1 || !seenDirectedPlantPath {
		t.Fatalf("service paths missing connected air/plant loops: %#v", paths)
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
			{Value: "HW Supply Branches"},
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
		{Index: 8, Type: "BranchList", Fields: []Field{
			{Value: "HW Supply Branches"},
			{Value: "Boiler Branch"},
		}},
		{Index: 9, Type: "Branch", Fields: []Field{
			{Value: "Boiler Branch"},
			{Value: ""},
			{Value: "Boiler:HotWater"},
			{Value: "HW Boiler"},
			{Value: "HW Supply Inlet"},
			{Value: "HW Supply Outlet"},
		}},
		{Index: 10, Type: "Boiler:HotWater", Fields: []Field{
			{Value: "HW Boiler", Comment: "Name"},
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
	if !hasHVACRuleEdge(report.RuleGraph, hvacRuleComponentServesParent) || !hasHVACRuleEdge(report.RuleGraph, hvacRulePlantComponentOnDemandBranch) {
		t.Fatalf("rule graph missing reheat source-to-terminal edges: %#v", report.RuleGraph.Edges)
	}
	if !hasHVACServiceChain(relation.ServiceChains, "Heating Water Loop", "Boiler:HotWater HW Boiler", "", "Office VAV") {
		t.Fatalf("service chains = %#v, want boiler -> Heat loop -> reheat coil -> terminal -> zone", relation.ServiceChains)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{"component_references:", `target_class: "Coil:Heating:Water"`, "relation_role: internal_component_reference"} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic component references missing %q:\n%s", expected, projection.Text)
		}
	}
}

func TestAnalyzeHVACVAVReheatTerminalUsesCatalogWithoutComments(t *testing.T) {
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
			{Value: "ZoneHVAC:AirDistributionUnit"},
			{Value: "Office ADU"},
			{Value: "1"},
			{Value: "1"},
		}},
		{Index: 3, Type: "ZoneHVAC:AirDistributionUnit", Fields: []Field{
			{Value: "Office ADU"},
			{Value: "Office Supply Inlet"},
			{Value: "AirTerminal:SingleDuct:VAV:Reheat"},
			{Value: "Office VAV"},
		}},
		{Index: 4, Type: "AirTerminal:SingleDuct:VAV:Reheat", Fields: []Field{
			{Value: "Office VAV"},
			{Value: "Always On"},
			{Value: "Office Damper Outlet"},
			{Value: "Office Terminal Inlet"},
			{Value: "Autosize"},
			{Value: "Constant"},
			{Value: "0.3"},
			{Value: ""},
			{Value: ""},
			{Value: "Coil:Heating:Water"},
			{Value: "Office Reheat Coil"},
			{Value: "Autosize"},
			{Value: "0"},
			{Value: "Office Supply Inlet"},
			{Value: "0.001"},
		}},
		{Index: 5, Type: "PlantLoop", Fields: []Field{
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
		{Index: 6, Type: "BranchList", Fields: []Field{
			{Value: "HW Demand Branches"},
			{Value: "Reheat Coil Branch"},
		}},
		{Index: 7, Type: "Branch", Fields: []Field{
			{Value: "Reheat Coil Branch"},
			{Value: ""},
			{Value: "Coil:Heating:Water"},
			{Value: "Office Reheat Coil"},
			{Value: "HW Demand Inlet"},
			{Value: "HW Demand Outlet"},
		}},
		{Index: 8, Type: "Coil:Heating:Water", Fields: []Field{
			{Value: "Office Reheat Coil"},
		}},
		{Index: 9, Type: "BranchList", Fields: []Field{
			{Value: "HW Supply Branches"},
			{Value: "Boiler Branch"},
		}},
		{Index: 10, Type: "Branch", Fields: []Field{
			{Value: "Boiler Branch"},
			{Value: ""},
			{Value: "Boiler:HotWater"},
			{Value: "HW Boiler"},
			{Value: "HW Supply Inlet"},
			{Value: "HW Supply Outlet"},
		}},
		{Index: 11, Type: "Boiler:HotWater", Fields: []Field{
			{Value: "HW Boiler"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	relation := findHVACTestingZoneRelation(report, "Office")
	if relation == nil {
		t.Fatalf("Office relation not found: %#v", report.ZoneRelations)
	}
	terminal := findHVACTestingComponent(relation.TerminalUnits, "Office VAV")
	if terminal == nil {
		t.Fatalf("terminal units = %#v, want Office VAV", relation.TerminalUnits)
	}
	if terminal.InletNode != "Office Terminal Inlet" || terminal.OutletNode != "Office Supply Inlet" || terminal.TerminalObjectOutletNode != "Office Supply Inlet" {
		t.Fatalf("terminal nodes = %#v, want catalog-derived inlet and final outlet", terminal)
	}
	if terminal.OutletFieldIndex != 13 {
		t.Fatalf("terminal outlet field index = %d, want final Air Outlet Node Name index 13", terminal.OutletFieldIndex)
	}
	if !terminal.ResolvedFromADU || terminal.DistributionUnitName != "Office ADU" || terminal.DistributionUnitOutletNode != "Office Supply Inlet" {
		t.Fatalf("terminal ADU trace = %#v, want Office ADU outlet trace", terminal)
	}
	if !hasHVACComponentReference(report.ComponentReferences, "Office VAV", "Coil:Heating:Water", "Office Reheat Coil", "internal_component_reference") {
		t.Fatalf("component references = %#v, want catalog-derived VAV reheat coil reference", report.ComponentReferences)
	}
	for _, ruleID := range []string{
		hvacRuleZoneEquipmentListADU,
		hvacRuleZoneADUResolvesTerminal,
		hvacRuleZoneADUOutletMatchesInlet,
		hvacRuleZoneTerminalOutletMatchesADU,
		hvacRuleComponentReferencesComponent,
		hvacRuleComponentServesParent,
	} {
		if !hasHVACRuleEdge(report.RuleGraph, ruleID) {
			t.Fatalf("rule graph missing %s edge: %#v", ruleID, report.RuleGraph.Edges)
		}
	}
	if !hasHVACServiceChain(relation.ServiceChains, "Heating Water Loop", "Boiler:HotWater HW Boiler", "", "Office VAV") {
		t.Fatalf("service chains = %#v, want boiler -> reheat coil -> terminal -> zone", relation.ServiceChains)
	}
}

func TestAnalyzeHVACServiceWaterLoopWarningIsNotice(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "PlantLoop", Fields: []Field{
			{Value: "Service Hot Water Loop"},
			{Value: "Water"},
			{Value: ""},
			{Value: ""},
			{Value: "SHW Setpoint"},
			{Value: "90"},
			{Value: "20"},
			{Value: "Autosize"},
			{Value: "0"},
			{Value: "Autosize"},
			{Value: "SHW Supply Inlet"},
			{Value: "SHW Supply Outlet"},
			{Value: ""},
			{Value: ""},
			{Value: "SHW Demand Inlet"},
			{Value: "SHW Demand Outlet"},
			{Value: "SHW Demand Branches"},
			{Value: ""},
		}},
		{Index: 1, Type: "BranchList", Fields: []Field{
			{Value: "SHW Demand Branches"},
			{Value: "Service Coil Branch"},
		}},
		{Index: 2, Type: "Branch", Fields: []Field{
			{Value: "Service Coil Branch"},
			{Value: ""},
			{Value: "Coil:Heating:Water"},
			{Value: "Service Coil"},
			{Value: "SHW Demand Inlet"},
			{Value: "SHW Demand Outlet"},
		}},
		{Index: 3, Type: "Coil:Heating:Water", Fields: []Field{
			{Value: "Service Coil", Comment: "Name"},
			{Value: "SHW Demand Inlet", Comment: "Water Inlet Node Name"},
			{Value: "SHW Demand Outlet", Comment: "Water Outlet Node Name"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	warning := findHVACWarningByCode(report.Warnings, "plant_demand_component_without_air_or_zone_use")
	if warning == nil {
		t.Fatalf("warnings = %#v, want service loop plant-demand notice", report.Warnings)
	}
	if warning.Severity != "notice" {
		t.Fatalf("service loop warning severity = %q, want notice: %#v", warning.Severity, *warning)
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
	if report.AirLoopCount != 4 || report.PlantLoopCount != 3 || report.CondenserLoopCount != 1 || report.ZoneRelationCount != 16 {
		t.Fatalf("counts = air %d plant %d condenser %d zones %d, want 4/3/1/16", report.AirLoopCount, report.PlantLoopCount, report.CondenserLoopCount, report.ZoneRelationCount)
	}
	if report.WarningCount != 0 {
		t.Fatalf("warnings = %#v, want none for reference sample", report.Warnings)
	}
	if hasHVACWarningCode(report.Warnings, "missing_zone_equipment_list") {
		t.Fatalf("missing zone equipment warnings = %#v, want none", report.Warnings)
	}
	debugRows := dumpHVACConnectionFields(doc)
	if len(debugRows) != 16 {
		t.Fatalf("connection debug rows = %d, want 16: %#v", len(debugRows), debugRows)
	}
	for _, row := range debugRows {
		if row.ResolvedZoneName == "" || row.ResolvedEquipmentListName == "" || !row.EquipmentListObjectFound {
			t.Fatalf("unresolved equipment connection row: %#v", row)
		}
	}
	for _, relation := range report.ZoneRelations {
		if len(relation.TerminalUnits) == 0 {
			t.Fatalf("zone relation has no terminal units: %#v", relation)
		}
		if len(relation.ServiceChains) == 0 {
			t.Fatalf("zone relation has no rule-backed service chains: %#v", relation)
		}
	}
	for _, loopName := range []string{"VAV_1", "VAV_2", "VAV_3", "VAV_5"} {
		loop := findHVACTestingLoop(report, loopName)
		if loop == nil || len(loop.RelatedZones) == 0 {
			t.Fatalf("%s related zones = %#v, want at least one zone", loopName, loop)
		}
	}
	vav1 := findHVACTestingLoop(report, "VAV_1")
	if vav1 == nil {
		t.Fatalf("VAV_1 loop not found")
	}
	if !hasHVACRelatedLoop(*vav1, "PlantLoop", "CoolSys1") || !hasHVACRelatedLoop(*vav1, "PlantLoop", "HeatSys1") {
		t.Fatalf("VAV_1 related loops = %#v, want CoolSys1 and HeatSys1", vav1.RelatedLoops)
	}
	if vav1.DemandGraph.SupplyPath == nil || vav1.DemandGraph.ReturnPath == nil || len(vav1.DemandGraph.Nodes) == 0 {
		t.Fatalf("VAV_1 demand graph is blank: %#v", vav1.DemandGraph)
	}
	coolingCoil := findHVACTestingComponent(vav1.SupplySideComponents(), "VAV_1_CoolC")
	if coolingCoil == nil || !stringSliceContainsFold(coolingCoil.RelatedLoopNames, "CoolSys1") {
		t.Fatalf("VAV_1_CoolC = %#v, want CoolSys1 related loop", coolingCoil)
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
	basementTerminal := findHVACTestingComponent(relation.TerminalUnits, "Basement VAV Box Component")
	if basementTerminal == nil || !basementTerminal.ResolvedFromADU || basementTerminal.DistributionUnitOutletNode == "" || basementTerminal.TerminalObjectOutletNode == "" {
		t.Fatalf("Basement ADU terminal trace = %#v, want ADU outlet and terminal outlet", basementTerminal)
	}
	if len(relation.RuleIDs) == 0 {
		t.Fatalf("Basement relation missing rule ids: %#v", relation)
	}
	if !stringSliceContainsFold(relation.PlantLoopNames, "HeatSys1") || !stringSliceContainsFold(relation.PlantLoopNames, "CoolSys1") {
		t.Fatalf("Basement plant loops = %#v, want HeatSys1 and CoolSys1", relation.PlantLoopNames)
	}
	if !componentSliceContainsName(relation.PlantEquipment, "HeatSys1 Boiler") || !componentSliceContainsName(relation.PlantEquipment, "CoolSys1 Chiller 1") {
		t.Fatalf("Basement plant equipment = %#v, want source equipment", relation.PlantEquipment)
	}
	if !hasHVACServiceChain(relation.ServiceChains, "CoolSys1", "CoolSys1 Chiller", "VAV_5", "Basement VAV Box") {
		t.Fatalf("Basement service chains = %#v, want cooling source -> VAV_5 -> terminal -> zone", relation.ServiceChains)
	}
	if !hasHVACServiceChain(relation.ServiceChains, "HeatSys1", "HeatSys1 Boiler", "", "Basement VAV Box") {
		t.Fatalf("Basement service chains = %#v, want heating source -> reheat terminal -> zone", relation.ServiceChains)
	}
	coreBottom := findHVACTestingZoneRelation(report, "Core_bottom")
	if coreBottom == nil {
		t.Fatalf("Core_bottom relation not found")
	}
	if !hasHVACServiceChain(coreBottom.ServiceChains, "CoolSys1", "CoolSys1 Chiller", "VAV_1", "Core_bottom VAV Box") {
		t.Fatalf("Core_bottom service chains = %#v, want CoolSys1 -> VAV_1 -> terminal -> zone", coreBottom.ServiceChains)
	}
	if !hasHVACServiceChain(coreBottom.ServiceChains, "HeatSys1", "HeatSys1 Boiler", "", "Core_bottom VAV Box") {
		t.Fatalf("Core_bottom service chains = %#v, want HeatSys1 -> reheat terminal -> zone", coreBottom.ServiceChains)
	}
	for _, ruleID := range []string{
		hvacRuleZoneEquipmentListADU,
		hvacRuleZoneADUResolvesTerminal,
		hvacRuleZoneADUOutletMatchesInlet,
		hvacRuleZoneTerminalOutletMatchesADU,
	} {
		if !hasHVACRuleEdge(report.RuleGraph, ruleID) {
			t.Fatalf("rule graph missing ADU rule %s: %#v", ruleID, report.RuleGraph.Edges)
		}
	}
	assertHVACJSONHasNoLegacyVocabulary(t, report)
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
	return findHVACWarningByCode(warnings, code) != nil
}

func assertHVACJSONHasNoLegacyVocabulary(t *testing.T, report HVACReport) {
	t.Helper()
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(encoded)
	for _, forbidden := range []string{
		"relationSource",
		"relationConfidence",
		"relationEvidence",
		`"confidence"`,
		`"evidence"`,
		`"inferred"`,
		`"weak"`,
		`"unsupported"`,
	} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("HVAC JSON contains legacy resolver vocabulary %q:\n%s", forbidden, jsonText)
		}
	}
}

func findHVACWarningByCode(warnings []HVACWarning, code string) *HVACWarning {
	for _, warning := range warnings {
		if warning.Code == code {
			return &warning
		}
	}
	return nil
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

func hasHVACRuleEdge(graph HVACRuleGraph, ruleID string) bool {
	for _, edge := range graph.Edges {
		if edge.RuleID == ruleID {
			return true
		}
	}
	return false
}

func hasHVACRelatedLoop(loop HVACLoop, loopType string, loopName string) bool {
	for _, relation := range loop.RelatedLoops {
		if strings.EqualFold(relation.LoopType, loopType) && strings.EqualFold(relation.LoopName, loopName) {
			return true
		}
	}
	return false
}

func hasHVACServiceChain(chains []HVACServicePath, plantLoop string, sourceNeedle string, airLoop string, terminalNeedle string) bool {
	for _, chain := range chains {
		if plantLoop != "" && !strings.EqualFold(chain.PlantLoop, plantLoop) {
			continue
		}
		if airLoop != "" && !strings.EqualFold(chain.AirLoopName, airLoop) {
			continue
		}
		if sourceNeedle != "" && !strings.Contains(strings.ToLower(chain.SourceComponent), strings.ToLower(sourceNeedle)) {
			continue
		}
		if terminalNeedle != "" && !strings.Contains(strings.ToLower(chain.TerminalName), strings.ToLower(terminalNeedle)) {
			continue
		}
		return true
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
