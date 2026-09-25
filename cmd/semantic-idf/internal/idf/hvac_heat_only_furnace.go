package idf

import "strings"

const nativeHeatOnlyFurnaceType = "AirLoopHVAC:Unitary:Furnace:HeatOnly"

// 25.1 native fields are explicit; comments, the loop name and a SingleCooling
// thermostat are never equipment capability or port authority. The index/NodeList
// primitives are shared with WindowAC, but none of its cooling policy is reused.
func hvacHeatOnlyFurnaceOuterPortsMatch(doc Document, obj Object, component HVACComponent) bool {
	if !strings.EqualFold(component.ObjectType, nativeHeatOnlyFurnaceType) || !windowACReviewedSchema(doc) {
		return true
	}
	return windowACField(obj, 2) != "" && windowACField(obj, 3) != "" && strings.EqualFold(windowACField(obj, 2), component.InletNode) && strings.EqualFold(windowACField(obj, 3), component.OutletNode)
}

func hvacHeatOnlyFurnaceBranchOwner(index windowACOriginalIndex, parentPosition int) (Object, bool) {
	parent := index.doc.Objects[parentPosition]
	refs := index.references[windowACKey(parent.Type, windowACField(parent, 0))]
	if len(refs) != 1 {
		return Object{}, false
	}
	branch := index.doc.Objects[refs[0].object]
	if !strings.EqualFold(branch.Type, "Branch") || refs[0].field < 2 {
		return Object{}, false
	}
	if _, valid := index.unique("Branch", windowACField(branch, 0)); !valid {
		return Object{}, false
	}
	listName, listCount := "", 0
	for _, object := range index.doc.Objects {
		if !strings.EqualFold(object.Type, "BranchList") {
			continue
		}
		for field := 1; field < len(object.Fields); field++ {
			if strings.EqualFold(windowACField(object, field), windowACField(branch, 0)) {
				listName = windowACField(object, 0)
				listCount++
			}
		}
	}
	if listCount != 1 {
		return Object{}, false
	}
	if _, valid := index.unique("BranchList", listName); !valid {
		return Object{}, false
	}
	var loop Object
	count := 0
	for _, object := range index.doc.Objects {
		if strings.EqualFold(object.Type, "AirLoopHVAC") && strings.EqualFold(windowACField(object, 4), listName) {
			loop = object
			count++
		}
	}
	if count != 1 {
		return Object{}, false
	}
	if _, valid := index.unique("AirLoopHVAC", windowACField(loop, 0)); !valid {
		return Object{}, false
	}
	return loop, true
}

func hvacNativeHeatOnlyFurnaceConditioning(ctx *hvacContext, graph HVACRuleGraph, component HVACComponent) (string, []ComponentRef, []string) {
	if ctx == nil || !strings.EqualFold(component.ObjectType, nativeHeatOnlyFurnaceType) || !windowACReviewedSchema(ctx.doc) {
		return "", nil, nil
	}
	index := newWindowACOriginalIndex(ctx.doc)
	parent, valid := index.unique(nativeHeatOnlyFurnaceType, component.ObjectName)
	if !valid || parent.Index != component.ObjectIndex || !hvacHeatOnlyFurnaceOuterPortsMatch(ctx.doc, parent, component) {
		return "", nil, nil
	}
	positions := index.objects[windowACKey(parent.Type, windowACField(parent, 0))]
	position := positions[0]
	// This finite native combustion path does not impersonate the distinct
	// water/steam PlantLoop contracts. Other existing routes stay with their
	// respective service builders. No supply fan consumption is coil output.
	fan, fanOK := index.child(position, 8, "Fan:OnOff")
	coil, coilOK := index.child(position, 11, "Coil:Heating:Fuel")
	if !fanOK || !coilOK {
		return "", nil, nil
	}
	loop, owned := hvacHeatOnlyFurnaceBranchOwner(index, position)
	if !owned {
		return "", nil, nil
	}
	control, controlOK := index.unique("Zone", windowACField(parent, 7))
	if !controlOK {
		return "", nil, nil
	}
	loopID := hvacRuleLoopNodeIDForName(graph, "AirLoopHVAC", windowACField(loop, 0))
	controlID := hvacRuleSubjectNodeIDForRelation(graph, HVACZoneChain{ZoneName: windowACField(control, 0)})
	if loopID == "" || controlID == "" {
		return "", nil, nil
	}
	if _, connected := hvacRuleGraphPath(graph, loopID, controlID); !connected {
		return "", nil, nil
	}
	outerIn, outerOut := windowACField(parent, 2), windowACField(parent, 3)
	fanIn, fanOut := windowACField(fan, 7), windowACField(fan, 8)
	coilIn, coilOut := windowACField(coil, 5), windowACField(coil, 6)
	if !windowACDistinctNodes(outerIn, outerOut) || !windowACDistinctNodes(fanIn, fanOut) || !windowACDistinctNodes(coilIn, coilOut) {
		return "", nil, nil
	}
	placement := windowACField(parent, 10)
	if placement == "" {
		placement = "BlowThrough"
	} // native IDD default
	switch strings.ToLower(placement) {
	case "blowthrough":
		if !strings.EqualFold(outerIn, fanIn) || !strings.EqualFold(fanOut, coilIn) || !strings.EqualFold(coilOut, outerOut) {
			return "", nil, nil
		}
	case "drawthrough":
		if !strings.EqualFold(outerIn, coilIn) || !strings.EqualFold(coilOut, fanIn) || !strings.EqualFold(fanOut, outerOut) {
			return "", nil, nil
		}
	default:
		return "", nil, nil
	}
	fanComponent := newHVACComponent(ctx, fan.Type, windowACField(fan, 0))
	coilComponent := newHVACComponent(ctx, coil.Type, windowACField(coil, 0))
	parentID := hvacRuleComponentSourceNodeID(component)
	for _, child := range []HVACComponent{fanComponent, coilComponent} {
		count := 0
		for _, edge := range graph.Edges {
			if edge.RuleID == hvacRuleComponentReferencesComponent && edge.FromID == parentID && edge.ToID == hvacRuleComponentSourceNodeID(child) {
				count++
			}
		}
		if count != 1 {
			return "", nil, nil
		}
	}
	return "heating", []ComponentRef{componentRefFromHVACComponent(component), componentRefFromHVACComponent(coilComponent)}, []string{hvacRuleBranchComponentOccurrence, hvacRuleComponentSourceOccurrence, hvacRuleComponentReferencesComponent}
}

// Call only after the existing air-demand graph trace succeeds. This adds the
// exact original ADU/EquipmentList/Connections owner proof for a native Furnace
// path; other equipment/service families are untouched. The controlling Zone
// is not a substitute for every physically served delivery Zone.
func hvacHeatOnlyFurnaceDeliveryMatches(ctx *hvacContext, conditioning []ComponentRef, relation HVACZoneChain, terminal HVACComponent) bool {
	furnace := false
	for _, component := range conditioning {
		furnace = furnace || strings.EqualFold(component.ObjectType, nativeHeatOnlyFurnaceType)
	}
	if !furnace {
		return true
	}
	if ctx == nil || relation.SpaceName != "" || !strings.EqualFold(terminal.ObjectType, "AirTerminal:SingleDuct:ConstantVolume:NoReheat") || !terminal.ResolvedFromADU {
		return false
	}
	index := newWindowACOriginalIndex(ctx.doc)
	actual, found := index.unique(terminal.ObjectType, terminal.ObjectName)
	if !found || actual.Index != terminal.ObjectIndex {
		return false
	}
	adu, found := index.unique("ZoneHVAC:AirDistributionUnit", terminal.DistributionUnitName)
	if !found {
		return false
	}
	positions := index.objects[windowACKey(adu.Type, windowACField(adu, 0))]
	child, found := index.child(positions[0], 2, actual.Type)
	if !found || child.Index != actual.Index || !windowACDistinctNodes(windowACField(actual, 2), windowACField(actual, 3)) || !strings.EqualFold(windowACField(actual, 2), terminal.InletNode) || !strings.EqualFold(windowACField(actual, 3), terminal.OutletNode) || !strings.EqualFold(windowACField(adu, 1), windowACField(actual, 3)) {
		return false
	}
	refs := index.references[windowACKey(adu.Type, windowACField(adu, 0))]
	if len(refs) != 1 {
		return false
	}
	ref := refs[0]
	list := index.doc.Objects[ref.object]
	if !strings.EqualFold(list.Type, "ZoneHVAC:EquipmentList") || ref.field < 2 || (ref.field-2)%6 != 0 {
		return false
	}
	if _, found := index.unique(list.Type, windowACField(list, 0)); !found {
		return false
	}
	var connection Object
	count := 0
	for _, object := range index.doc.Objects {
		if strings.EqualFold(object.Type, "ZoneHVAC:EquipmentConnections") && strings.EqualFold(windowACField(object, 1), windowACField(list, 0)) {
			connection = object
			count++
		}
	}
	if count != 1 || !strings.EqualFold(windowACField(connection, 0), relation.ZoneName) || windowACField(connection, 4) == "" {
		return false
	}
	if _, found := index.unique("Zone", relation.ZoneName); !found {
		return false
	}
	inlets, found := index.nodeSelector(windowACField(connection, 2))
	if !found || !windowACNodeIn(inlets, windowACField(actual, 3)) {
		return false
	}
	zoneCount := 0
	for _, object := range index.doc.Objects {
		if !strings.EqualFold(object.Type, "ZoneHVAC:EquipmentConnections") {
			continue
		}
		if strings.EqualFold(windowACField(object, 0), relation.ZoneName) {
			zoneCount++
			continue
		}
		for _, field := range []int{2, 3} {
			if index.nodeSelectorMentions(windowACField(object, field), windowACField(actual, 3)) {
				return false
			}
		}
	}
	return zoneCount == 1
}
