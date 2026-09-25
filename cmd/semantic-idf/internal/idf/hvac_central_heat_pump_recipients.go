package idf

import "strings"

// Complete the reviewed CV-reheat/return-plenum air route before giving one
// CentralHeatPumpSystem constituent a recipient. All checks are native 25.1
// fields. This is not a generic VAV/air-loop fallback and does not calculate
// energy. The caller caches the result by actual plant occurrence and AirLoop.
func hvacCentralHeatPumpRecipientRoster(index centralHeatPumpIndex, port NativeCentralHeatPumpPort, airIndex int) map[int]string {
	f, same := centralHeatPumpField, centralHeatPumpEqual
	var air Object
	for _, o := range index.doc.Objects {
		if o.Index == airIndex && same(o.Type, "AirLoopHVAC") {
			air = o
		}
	}
	if _, ok := index.unique("AirLoopHVAC", f(air, 0)); !ok || f(air, 5) != "" {
		return nil
	}
	for _, pos := range []int{6, 7, 8, 9} {
		if index.centralFieldUses("AirLoopHVAC", pos, f(air, pos)) != 1 {
			return nil
		}
	}
	list, ok := index.unique("BranchList", f(air, 4))
	if !ok || len(list.Fields) != 2 || index.centralFieldUses("AirLoopHVAC", 4, f(list, 0)) != 1 {
		return nil
	}
	branch, ok := index.unique("Branch", f(list, 1))
	if !ok || len(branch.Fields) != 10 || index.branchListUses(f(branch, 0)) != 1 || !same(f(branch, 2), "Fan:ConstantVolume") || !same(f(branch, 6), "Coil:Cooling:Water:DetailedGeometry") {
		return nil
	}
	fan, fanOK := index.unique(f(branch, 2), f(branch, 3))
	coil, coilOK := index.unique(f(branch, 6), f(branch, 7))
	if !fanOK || !coilOK || !same(f(fan, 7), f(branch, 4)) || !same(f(fan, 8), f(branch, 5)) || !same(f(coil, 20), f(branch, 8)) || !same(f(coil, 21), f(branch, 9)) || !same(f(branch, 5), f(branch, 8)) || !same(f(air, 6), f(branch, 4)) || !same(f(air, 9), f(branch, 9)) || index.centralTypedUses(fan.Type, f(fan, 0)) != 1 || index.centralTypedUses(coil.Type, f(coil, 0)) != 2 {
		return nil
	}
	if _, ok := index.schedule(f(fan, 1)); !ok {
		return nil
	}
	if _, ok := index.schedule(f(coil, 1)); !ok {
		return nil
	}
	controllers, ok := index.unique("AirLoopHVAC:ControllerList", f(air, 1))
	if !ok || len(controllers.Fields) != 3 || !same(f(controllers, 1), "Controller:WaterCoil") || index.centralFieldUses("AirLoopHVAC", 1, f(controllers, 0)) != 1 {
		return nil
	}
	controller, ok := index.unique(f(controllers, 1), f(controllers, 2))
	if !ok || index.centralTypedUses(controller.Type, f(controller, 0)) != 1 || !same(f(controller, 1), "Temperature") || !same(f(controller, 2), "Reverse") || !same(f(controller, 3), "Flow") || !same(f(controller, 4), f(coil, 21)) || !same(f(controller, 5), f(coil, 18)) || index.centralFieldUses(controller.Type, 5, f(coil, 18)) != 1 {
		return nil
	}
	supply, ok := index.centralPath("AirLoopHVAC:SupplyPath", f(air, 8))
	if !ok || len(supply.Fields) != 4 || !same(f(supply, 2), "AirLoopHVAC:ZoneSplitter") {
		return nil
	}
	splitter, ok := index.unique(f(supply, 2), f(supply, 3))
	if !ok || len(splitter.Fields) < 3 || !same(f(splitter, 1), f(supply, 1)) || index.centralTypedUses(splitter.Type, f(splitter, 0)) != 1 {
		return nil
	}
	back, ok := index.centralPath("AirLoopHVAC:ReturnPath", f(air, 7))
	if !ok || len(back.Fields) != 4 || !same(f(back, 2), "AirLoopHVAC:ReturnPlenum") {
		return nil
	}
	plenum, ok := index.unique(f(back, 2), f(back, 3))
	if !ok || len(plenum.Fields)-5 != len(splitter.Fields)-2 || f(plenum, 4) != "" || f(plenum, 2) == "" || !same(f(plenum, 3), f(back, 1)) || index.centralTypedUses(plenum.Type, f(plenum, 0)) != 1 {
		return nil
	}
	if _, ok := index.unique("Zone", f(plenum, 1)); !ok || index.centralFieldUses("ZoneHVAC:EquipmentConnections", 0, f(plenum, 1)) != 0 {
		return nil
	}
	returns := map[string]bool{}
	for n := 5; n < len(plenum.Fields); n++ {
		key := strings.ToLower(f(plenum, n))
		if key == "" || returns[key] || same(key, f(plenum, 2)) || same(key, f(plenum, 3)) {
			return nil
		}
		returns[key] = true
	}
	result, zones, inlets, outlets := map[int]string{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	waterCoils := map[int]Object{}
	if port.Role == "cooling" {
		waterCoils[coil.Index] = coil
	}
	for n := 2; n < len(splitter.Fields); n++ {
		node := f(splitter, n)
		if node == "" || inlets[strings.ToLower(node)] {
			return nil
		}
		inlets[strings.ToLower(node)] = true
		var terminal Object
		count := 0
		for _, o := range index.doc.Objects {
			if strings.HasPrefix(strings.ToLower(o.Type), "airterminal:") && same(f(o, 3), node) {
				if !same(o.Type, "AirTerminal:SingleDuct:ConstantVolume:Reheat") {
					return nil
				}
				terminal, count = o, count+1
			}
		}
		if count != 1 || index.centralTypedUses(terminal.Type, f(terminal, 0)) != 1 || !same(f(terminal, 5), "Coil:Heating:Water") {
			return nil
		}
		outlet := strings.ToLower(f(terminal, 2))
		if outlet == "" || outlets[outlet] || same(outlet, node) {
			return nil
		}
		outlets[outlet] = true
		if _, ok := index.unique(terminal.Type, f(terminal, 0)); !ok {
			return nil
		}
		if _, ok := index.schedule(f(terminal, 1)); !ok {
			return nil
		}
		heat, ok := index.unique(f(terminal, 5), f(terminal, 6))
		if !ok || !same(f(heat, 6), node) || !same(f(heat, 7), f(terminal, 2)) || index.centralTypedUses(heat.Type, f(heat, 0)) != 2 {
			return nil
		}
		if _, ok := index.schedule(f(heat, 1)); !ok {
			return nil
		}
		if port.Role == "heating" {
			waterCoils[heat.Index] = heat
		}
		var adu Object
		count = 0
		for _, o := range index.doc.Objects {
			if same(o.Type, "ZoneHVAC:AirDistributionUnit") && same(f(o, 2), terminal.Type) && same(f(o, 3), f(terminal, 0)) {
				adu, count = o, count+1
			}
		}
		if count != 1 || !same(f(adu, 1), f(terminal, 2)) || index.centralTypedUses(adu.Type, f(adu, 0)) != 1 {
			return nil
		}
		if _, ok := index.unique(adu.Type, f(adu, 0)); !ok {
			return nil
		}
		var equipment Object
		count = 0
		for _, o := range index.doc.Objects {
			if same(o.Type, "ZoneHVAC:EquipmentList") && same(f(o, 2), adu.Type) && same(f(o, 3), f(adu, 0)) {
				equipment, count = o, count+1
			}
		}
		if count != 1 || len(equipment.Fields) != 8 || !same(f(equipment, 1), "SequentialLoad") || f(equipment, 4) != "1" || f(equipment, 5) != "1" || f(equipment, 6) != "" || f(equipment, 7) != "" {
			return nil
		}
		if _, ok := index.unique(equipment.Type, f(equipment, 0)); !ok {
			return nil
		}
		var connection Object
		count = 0
		for _, o := range index.doc.Objects {
			if same(o.Type, "ZoneHVAC:EquipmentConnections") && same(f(o, 1), f(equipment, 0)) {
				connection, count = o, count+1
			}
		}
		zone, out := f(connection, 0), strings.ToLower(f(connection, 5))
		if count != 1 || len(connection.Fields) != 6 || f(connection, 3) != "" || f(connection, 4) == "" || same(f(connection, 4), f(plenum, 2)) || same(f(connection, 4), f(plenum, 3)) || zones[strings.ToLower(zone)] || same(zone, f(plenum, 1)) || !returns[out] || index.centralFieldUses(connection.Type, 4, f(connection, 4)) != 1 || index.centralFieldUses(connection.Type, 5, f(connection, 5)) != 1 {
			return nil
		}
		if _, ok := index.unique("Zone", zone); !ok {
			return nil
		}
		if _, ok := index.unique(connection.Type, zone); !ok {
			return nil
		}
		nodes, ok := index.unique("NodeList", f(connection, 2))
		if !ok || len(nodes.Fields) != 2 || !same(f(nodes, 1), f(adu, 1)) || index.centralFieldUses(connection.Type, 2, f(nodes, 0)) != 1 {
			return nil
		}
		uses := 0
		for _, other := range index.doc.Objects {
			if !same(other.Type, connection.Type) {
				continue
			}
			if same(f(other, 2), outlet) {
				uses++
			}
			if otherList, found := index.unique("NodeList", f(other, 2)); found {
				for p := 1; p < len(otherList.Fields); p++ {
					if same(f(otherList, p), outlet) {
						uses++
					}
				}
			}
		}
		if uses != 1 {
			return nil
		}
		zones[strings.ToLower(zone)], result[terminal.Index] = true, zone
		delete(returns, out)
	}
	if len(returns) != 0 || len(result) == 0 || !index.centralDemandCoils(port, waterCoils) {
		return nil
	}
	return result
}

func (index centralHeatPumpIndex) centralFieldUses(typ string, field int, value string) int {
	count := 0
	for _, o := range index.doc.Objects {
		if centralHeatPumpEqual(o.Type, typ) && centralHeatPumpEqual(centralHeatPumpField(o, field), value) {
			count++
		}
	}
	return count
}

func (index centralHeatPumpIndex) centralTypedUses(typ, name string) int {
	count := 0
	for _, o := range index.doc.Objects {
		for n := 0; n+1 < len(o.Fields); n++ {
			if centralHeatPumpEqual(centralHeatPumpField(o, n), typ) && centralHeatPumpEqual(centralHeatPumpField(o, n+1), name) {
				count++
			}
		}
	}
	return count
}

func (index centralHeatPumpIndex) centralPath(typ, node string) (Object, bool) {
	var found Object
	count := 0
	for _, o := range index.doc.Objects {
		if centralHeatPumpEqual(o.Type, typ) && centralHeatPumpEqual(centralHeatPumpField(o, 1), node) {
			found, count = o, count+1
		}
	}
	_, unique := index.unique(typ, centralHeatPumpField(found, 0))
	return found, count == 1 && unique
}

// The whole selected water-demand cohort must be accounted for. A broken or
// additional recipient must not cause the paid source to renormalize onto the
// surviving subset. Other source systems and their allocations are untouched.
func (index centralHeatPumpIndex) centralDemandCoils(port NativeCentralHeatPumpPort, coils map[int]Object) bool {
	f, same := centralHeatPumpField, centralHeatPumpEqual
	loop, ok := index.unique(port.Loop.Type, port.Loop.Name)
	if !ok || len(coils) == 0 || !same(loop.Type, "PlantLoop") {
		return false
	}
	list, ok := index.unique("BranchList", f(loop, 16))
	if !ok {
		return false
	}
	seen := map[int]bool{}
	for n := 1; n < len(list.Fields); n++ {
		branch, ok := index.unique("Branch", f(list, n))
		if !ok {
			return false
		}
		for p := 2; p+3 < len(branch.Fields); p += 4 {
			o, ok := index.unique(f(branch, p), f(branch, p+1))
			if !ok {
				return false
			}
			water := 1
			if _, expected := coils[o.Index]; expected {
				if seen[o.Index] || !index.sideRoster(loop, 14, branch) {
					return false
				}
				seen[o.Index] = true
				water = 4
				if port.Role == "cooling" {
					water = 18
				}
			} else if !same(o.Type, "Pipe:Adiabatic") {
				return false
			}
			if !same(f(o, water), f(branch, p+2)) || !same(f(o, water+1), f(branch, p+3)) {
				return false
			}
		}
	}
	return len(seen) == len(coils)
}
