package idf

import "strings"

// Select one already-proved native occurrence. A global system ID alone cannot
// identify either of its two PlantLoop supplies or its separate source circuit.
func hvacCentralHeatPumpSupplyPort(ctx *hvacContext, component HVACComponent, loopName string) (NativeCentralHeatPumpPort, bool) {
	if ctx == nil {
		return NativeCentralHeatPumpPort{}, false
	}
	for _, binding := range ctx.nativeCentralHeatPumpBindings {
		if component.ObjectIndex != binding.System.ObjectIndex || !centralHeatPumpEqual(component.ObjectType, binding.System.ObjectType) || !centralHeatPumpEqual(component.ObjectName, binding.System.ObjectName) {
			continue
		}
		for _, service := range []string{"cooling", "heating"} {
			port, ok := NativeCentralHeatPumpServicePort(binding, service, "PlantLoop", loopName)
			if ok && component.SourceOwnerObjectIndex == port.Branch.ObjectIndex && centralHeatPumpEqual(component.SourceOwnerType, "Branch") && centralHeatPumpEqual(component.SourceOwnerName, port.Branch.ObjectName) && component.TypeFieldIndex == port.BranchComponentField && component.InletFieldIndex == port.BranchComponentField+2 && component.OutletFieldIndex == port.BranchComponentField+3 && centralHeatPumpEqual(component.InletNode, port.InletNode) && centralHeatPumpEqual(component.OutletNode, port.OutletNode) {
				return port, true
			}
		}
	}
	return NativeCentralHeatPumpPort{}, false
}

// Native coil ports are fixed for the reviewed 25.1 cohort. Editable comments
// are not schema authority. Do not alter unrelated/unreviewed model families.
func hvacCentralHeatPumpCoilPorts(ctx *hvacContext, object Object, component *HVACComponent) {
	reviewed := false
	for _, binding := range ctx.nativeCentralHeatPumpBindings {
		reviewed = reviewed || binding.ReportingIdentityValid
	}
	if !reviewed {
		return
	}
	water, air := -1, -1
	switch strings.ToLower(strings.TrimSpace(object.Type)) {
	case "coil:cooling:water:detailedgeometry":
		water, air = 18, 20
	case "coil:heating:water":
		water, air = 4, 6
	default:
		return
	}
	component.WaterInletNode, component.WaterOutletNode = centralHeatPumpField(object, water), centralHeatPumpField(object, water+1)
	component.InletNode, component.OutletNode = centralHeatPumpField(object, air), centralHeatPumpField(object, air+1)
	component.InletFieldIndex, component.OutletFieldIndex = air, air+1
}

// Start downstream traversal at the selected PlantLoop, then attach only the
// exact native source-to-loop occurrence edge. The shared source node may lead
// to a different circuit and must never be the shortest-path starting point.
func hvacCentralHeatPumpRulePath(ctx *hvacContext, source HVACComponent, loopName, subjectID string, graph HVACRuleGraph) ([]HVACRuleEdge, string, bool) {
	port, ok := hvacCentralHeatPumpSupplyPort(ctx, source, loopName)
	if !ok {
		return nil, "", false
	}
	loopID := hvacRuleLoopNodeIDForName(graph, "PlantLoop", loopName)
	edges, ok := hvacRuleGraphPath(graph, loopID, subjectID)
	if !ok || loopID == "" {
		return nil, "", false
	}
	nodes := hvacRuleGraphNodeByID(graph)
	for _, edge := range edges {
		for _, id := range []string{edge.FromID, edge.ToID} {
			node := nodes[id]
			if node.Kind == "loop" && (strings.EqualFold(node.ObjectType, "PlantLoop") || strings.EqualFold(node.ObjectType, "CondenserLoop")) && id != loopID {
				return nil, "", false
			}
		}
	}
	for _, edge := range graph.Edges {
		if edge.RuleID == hvacRulePlantComponentOnSupplyBranch && edge.FromID == hvacRuleComponentSourceNodeID(source) && edge.ToID == loopID && edge.SourceObjectIndex == port.Branch.ObjectIndex && centralHeatPumpEqual(edge.SourceObjectType, "Branch") && centralHeatPumpEqual(edge.SourceObjectName, port.Branch.ObjectName) {
			return append([]HVACRuleEdge{edge}, edges...), port.Role, true
		}
	}
	return nil, "", false
}

// This engineering guard checks selected native coil/terminal routes before
// first-wins path deduplication. It is not full fixture acceptance: independent
// controller/return/whole-recipient proofs and native source budgets remain
// necessary before assigning actual system electricity to these paths.
func buildHVACCentralHeatPumpPathGate(ctx *hvacContext, loops []HVACLoop, relations []HVACZoneChain, graph HVACRuleGraph) func(ZoneServicePath) (ZoneServicePath, bool) {
	index := centralHeatPumpIndex{doc: ctx.doc, objects: map[string][]Object{}}
	if len(ctx.nativeCentralHeatPumpBindings) > 0 {
		for _, object := range ctx.doc.Objects {
			key := centralHeatPumpKey(object.Type, centralHeatPumpField(object, 0))
			index.objects[key] = append(index.objects[key], object)
		}
	}
	type rosterKey struct {
		air, plant int
		service    string
	}
	rosters := map[rosterKey]map[int]string{}
	return func(path ZoneServicePath) (ZoneServicePath, bool) {
		if path.SourceSystem == nil || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(path.SourceSystem.Name)), "centralheatpumpsystem ") {
			return path, true
		}
		if path.SourceSystem.Type != "source" || path.PlantLoop == nil || path.AirLoop == nil || path.CondenserLoop != nil || path.RefrigerantSystem != nil || path.SpaceName != "" || path.PathType != "central_air_with_plant" || len(path.Conditioning) != 1 {
			return path, false
		}
		var port NativeCentralHeatPumpPort
		found := false
		for _, binding := range ctx.nativeCentralHeatPumpBindings {
			if centralHeatPumpEqual(path.SourceSystem.Name, binding.System.ObjectType+" "+binding.System.ObjectName) {
				port, found = NativeCentralHeatPumpServicePort(binding, path.ServiceKind, path.PlantLoop.Type, path.PlantLoop.Name)
			}
		}
		if !found || path.PlantLoop.ObjectIndex != port.Loop.ObjectIndex {
			return path, false
		}
		key := rosterKey{path.AirLoop.ObjectIndex, port.Loop.ObjectIndex, port.Role}
		roster, checked := rosters[key]
		if !checked {
			roster = hvacCentralHeatPumpRecipientRoster(index, port, path.AirLoop.ObjectIndex)
			rosters[key] = roster
		}
		if !centralHeatPumpEqual(roster[path.Delivery.ObjectIndex], path.ZoneName) {
			return path, false
		}
		coil, unique := index.unique(path.Conditioning[0].ObjectType, path.Conditioning[0].ObjectName)
		water, air := 18, 20
		wantType := "Coil:Cooling:Water:DetailedGeometry"
		if path.ServiceKind == "heating" {
			water, air, wantType = 4, 6, "Coil:Heating:Water"
		}
		if !unique || coil.Index != path.Conditioning[0].ObjectIndex || !centralHeatPumpEqual(coil.Type, wantType) {
			return path, false
		}
		waterBranches := 0
		for _, loop := range loops {
			if loop.Type != "PlantLoop" {
				continue
			}
			for _, side := range []HVACLoopSide{loop.SupplySide, loop.DemandSide} {
				for _, branch := range side.Branches {
					for _, component := range branch.Components {
						if component.ObjectIndex != coil.Index || !centralHeatPumpEqual(component.ObjectType, coil.Type) {
							continue
						}
						waterBranches++
						loopObject, loopOK := index.unique(loop.Type, loop.Name)
						branchObject, branchOK := index.unique("Branch", branch.Name)
						if !loopOK || !branchOK || loop.ObjectIndex != port.Loop.ObjectIndex || !strings.EqualFold(side.Name, "demand") || !centralHeatPumpEqual(component.InletNode, centralHeatPumpField(coil, water)) || !centralHeatPumpEqual(component.OutletNode, centralHeatPumpField(coil, water+1)) || !index.sideRoster(loopObject, 14, branchObject) {
							return path, false
						}
					}
				}
			}
		}
		if waterBranches != 1 {
			return path, false
		}
		matches := 0
		for _, relation := range relations {
			if relation.SpaceName != "" || !centralHeatPumpEqual(relation.ZoneName, path.ZoneName) {
				continue
			}
			for _, terminal := range relation.TerminalUnits {
				if terminal.ObjectIndex != path.Delivery.ObjectIndex || !centralHeatPumpEqual(terminal.ObjectType, "AirTerminal:SingleDuct:ConstantVolume:Reheat") || !centralHeatPumpEqual(terminal.ObjectName, path.Delivery.ObjectName) {
					continue
				}
				object, ok := index.unique(terminal.ObjectType, terminal.ObjectName)
				adu, aduOK := index.unique("ZoneHVAC:AirDistributionUnit", terminal.DistributionUnitName)
				if !ok || !aduOK || !terminal.ResolvedFromADU || !centralHeatPumpEqual(centralHeatPumpField(adu, 1), centralHeatPumpField(object, 2)) || !centralHeatPumpEqual(centralHeatPumpField(adu, 2), object.Type) || !centralHeatPumpEqual(centralHeatPumpField(adu, 3), terminal.ObjectName) || !centralHeatPumpEqual(terminal.InletNode, centralHeatPumpField(object, 3)) || !centralHeatPumpEqual(terminal.OutletNode, centralHeatPumpField(object, 2)) {
					return path, false
				}
				if _, connected := hvacAirConditioningDeliveryTrace(ctx, loops, graph, relation, terminal, path.AirLoop.Name); !connected {
					return path, false
				}
				reheat, reheatOK := index.unique(centralHeatPumpField(object, 5), centralHeatPumpField(object, 6))
				if !reheatOK || !centralHeatPumpEqual(reheat.Type, "Coil:Heating:Water") || !centralHeatPumpEqual(centralHeatPumpField(reheat, 6), centralHeatPumpField(object, 3)) || !centralHeatPumpEqual(centralHeatPumpField(reheat, 7), centralHeatPumpField(object, 2)) {
					return path, false
				}
				if path.ServiceKind == "heating" {
					if !centralHeatPumpEqual(centralHeatPumpField(object, 5), coil.Type) || !centralHeatPumpEqual(centralHeatPumpField(object, 6), centralHeatPumpField(coil, 0)) || !centralHeatPumpEqual(centralHeatPumpField(object, 3), centralHeatPumpField(coil, air)) || !centralHeatPumpEqual(centralHeatPumpField(object, 2), centralHeatPumpField(coil, air+1)) {
						return path, false
					}
				}
				var coolingCoil *Object
				if path.ServiceKind == "cooling" {
					coolingCoil = &coil
				}
				if !hvacCentralHeatPumpAirSupplyBranch(loops, index, path.AirLoop, coolingCoil) {
					return path, false
				}
				wrapper := centralHeatPumpRef(adu)
				path.DeliveryWrapper = &wrapper
				matches++
			}
		}
		return path, matches == 1
	}
}

func hvacCentralHeatPumpAirSupplyBranch(loops []HVACLoop, index centralHeatPumpIndex, selected *LoopRef, coolingCoil *Object) bool {
	matches := 0
	for _, loop := range loops {
		if !centralHeatPumpEqual(loop.Type, "AirLoopHVAC") || loop.ObjectIndex != selected.ObjectIndex || !centralHeatPumpEqual(loop.Name, selected.Name) {
			continue
		}
		if len(loop.SupplySide.Branches) != 1 || len(loop.SupplySide.Branches[0].Components) != 2 || loop.SupplySide.ConnectorListName != "" {
			return false
		}
		branch := loop.SupplySide.Branches[0]
		fan, fanOK := index.unique(branch.Components[0].ObjectType, branch.Components[0].ObjectName)
		airCoil := branch.Components[1]
		coil, coilOK := index.unique(airCoil.ObjectType, airCoil.ObjectName)
		if !coilOK || !centralHeatPumpEqual(coil.Type, "Coil:Cooling:Water:DetailedGeometry") || coolingCoil != nil && coolingCoil.Index != coil.Index {
			return false
		}
		if !fanOK || !centralHeatPumpEqual(fan.Type, "Fan:ConstantVolume") || airCoil.ObjectIndex != coil.Index || !centralHeatPumpEqual(branch.Components[0].InletNode, centralHeatPumpField(fan, 7)) || !centralHeatPumpEqual(branch.Components[0].OutletNode, centralHeatPumpField(fan, 8)) || !centralHeatPumpEqual(loop.SupplySide.InletNode, centralHeatPumpField(fan, 7)) || !centralHeatPumpEqual(centralHeatPumpField(fan, 8), centralHeatPumpField(coil, 20)) || !centralHeatPumpEqual(airCoil.InletNode, centralHeatPumpField(coil, 20)) || !centralHeatPumpEqual(airCoil.OutletNode, centralHeatPumpField(coil, 21)) || !centralHeatPumpEqual(loop.SupplySide.OutletNode, centralHeatPumpField(coil, 21)) {
			return false
		}
		matches++
	}
	return matches == 1
}
