package simulation

// Finite Shop25.1 original proof. Only independent test helpers and lexical
// parsing are shared; no production inventory/routes/allocator is authority.
import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathRealSQLPVHVACLoop struct {
	ZoneName           string `json:"zoneName"`
	AirLoopName        string `json:"airLoopName"`
	CoolingWrapperName string `json:"coolingWrapperName"`
	CoolingCoilName    string `json:"coolingCoilName"`
	HeatingCoilName    string `json:"heatingCoilName"`
	FanName            string `json:"fanName"`
	TerminalName       string `json:"terminalName"`
	ADUName            string `json:"aduName"`
}

type epathSQLPVHVACOriginalProof struct {
	OriginalSHA256                                       string
	Loops                                                []epathRealSQLPVHVACLoop
	Objects                                              []epathSQLPVOriginalObject // All original physical/control fields; not merely owners.
	DomesticWaterLoop, DomesticPump, DomesticWaterHeater string
}

type epathSQLPVHVACWalk struct {
	doc idf.Document
	err error
}

func (w *epathSQLPVHVACWalk) need(ok bool, message string) {
	if !ok && w.err == nil {
		w.err = fmt.Errorf("finite Shop HVAC original: %s", message)
	}
}
func (w *epathSQLPVHVACWalk) one(typ, name string) idf.Object {
	o, err := (epathSQLSharedOriginal{doc: w.doc}).one(typ, name)
	if err != nil && w.err == nil {
		w.err = err
	}
	return o
}
func (w *epathSQLPVHVACWalk) eq(a, b string) {
	w.need(epathSQLSharedSame(a, b), "disconnected/blank native node or identity: "+a+" / "+b)
}
func (w *epathSQLPVHVACWalk) typed(typ, name, ownerType, ownerName string, position int) {
	count := 0
	for _, o := range w.doc.Objects {
		for n := 0; n+1 < len(o.Fields); n++ {
			if epathSQLSharedSame(epathSQLSharedField(o, n), typ) && epathSQLSharedSame(epathSQLSharedField(o, n+1), name) {
				count++
				w.need(strings.EqualFold(o.Type, ownerType) && epathSQLSharedSame(epathSQLSharedField(o, 0), ownerName) && n == position, "typed component has a foreign/shared owner: "+typ+"/"+name)
			}
		}
	}
	w.need(count == 1, "typed component reference is not unique: "+typ+"/"+name)
}
func (w *epathSQLPVHVACWalk) fieldUses(typ string, positions []int, value string) int {
	count := 0
	for _, o := range w.doc.Objects {
		if !strings.EqualFold(o.Type, typ) {
			continue
		}
		for _, n := range positions {
			if epathSQLSharedSame(epathSQLSharedField(o, n), value) {
				count++
			}
		}
	}
	return count
}
func (w *epathSQLPVHVACWalk) listUses(typ string, start int, value string) int {
	count := 0
	for _, o := range w.doc.Objects {
		if !strings.EqualFold(o.Type, typ) {
			continue
		}
		for n := start; n < len(o.Fields); n++ {
			if epathSQLSharedSame(epathSQLSharedField(o, n), value) {
				count++
			}
		}
	}
	return count
}
func (w *epathSQLPVHVACWalk) schedule(name string) {
	count := 0
	for _, o := range w.doc.Objects {
		if strings.HasPrefix(strings.ToLower(o.Type), "schedule:") && epathSQLSharedSame(epathSQLSharedField(o, 0), name) {
			count++
		}
	}
	w.need(name != "" && count == 1, "missing/ambiguous schedule "+name)
}
func (w *epathSQLPVHVACWalk) singleNodeList(name, want string) {
	o := w.one("NodeList", name)
	w.need(len(o.Fields) == 2, "native selector must be one explicit NodeList member")
	w.eq(epathSQLSharedField(o, 1), want)
}
func (w *epathSQLPVHVACWalk) outdoorNode(want string) {
	// OutdoorAir:NodeList is nameless. This finite original uses a named
	// NodeList selector, never its first field as an object name.
	count := 0
	for _, o := range w.doc.Objects {
		if !strings.EqualFold(o.Type, "OutdoorAir:NodeList") {
			continue
		}
		w.need(len(o.Fields) == 1, "unreviewed OutdoorAir:NodeList shape")
		list := w.one("NodeList", epathSQLSharedField(o, 0))
		w.need(len(list.Fields) == 2, "outdoor NodeList must have one real node")
		if epathSQLSharedSame(epathSQLSharedField(list, 1), want) {
			count++
		}
	}
	w.need(want != "" && count == 1, "outdoor node lacks one exact native declaration")
}
func (w *epathSQLPVHVACWalk) path(typ, node string) idf.Object {
	var found idf.Object
	count := 0
	for _, o := range w.doc.Objects {
		if strings.EqualFold(o.Type, typ) && epathSQLSharedSame(epathSQLSharedField(o, 1), node) {
			found = o
			count++
		}
	}
	w.need(node != "" && count == 1, "missing/shared path "+typ+"/"+node)
	return w.one(typ, epathSQLSharedField(found, 0))
}

func (w *epathSQLPVHVACWalk) air(d epathRealSQLPVHVACLoop) {
	f := epathSQLSharedField
	zone := w.one("Zone", d.ZoneName)
	w.need(!strings.ContainsAny(f(zone, 6), "xX_pP") && epathSQLPVNumber(zone, 6, 1, 1), "Shop Zone multiplier must remain decimal one")
	air := w.one("AirLoopHVAC", d.AirLoopName)
	w.need(f(air, 1) == "" && f(air, 5) == "", "unreviewed central controllers/supply connectors")
	w.need(w.fieldUses("AirLoopHVAC", []int{6}, f(air, 6)) == 1 && w.fieldUses("AirLoopHVAC", []int{7}, f(air, 7)) == 1 && w.fieldUses("AirLoopHVAC", []int{8}, f(air, 8)) == 1 && w.fieldUses("AirLoopHVAC", []int{9}, f(air, 9)) == 1, "shared native AirLoop interface")
	list := w.one("BranchList", f(air, 4))
	w.need(len(list.Fields) == 2 && w.fieldUses("AirLoopHVAC", []int{4}, f(list, 0)) == 1, "AirLoop must uniquely own one BranchList branch")
	branch := w.one("Branch", f(list, 1))
	w.need(len(branch.Fields) == 18 && w.listUses("BranchList", 1, f(branch, 0)) == 1, "Shop requires unique four-component stride4 main branch")
	w.eq(f(air, 6), f(branch, 4))
	w.eq(f(air, 9), f(branch, 17))
	for n := 2; n+7 < len(branch.Fields); n += 4 {
		w.eq(f(branch, n+3), f(branch, n+6))
	}
	oa := w.one("AirLoopHVAC:OutdoorAirSystem", f(branch, 3))
	components := []struct {
		typ, name string
		slot      int
	}{{"AirLoopHVAC:OutdoorAirSystem", f(oa, 0), 2}, {"CoilSystem:Cooling:DX", d.CoolingWrapperName, 6}, {"Coil:Heating:Electric", d.HeatingCoilName, 10}, {"Fan:ConstantVolume", d.FanName, 14}}
	for _, c := range components {
		w.eq(f(branch, c.slot), c.typ)
		w.eq(f(branch, c.slot+1), c.name)
		w.typed(c.typ, c.name, "Branch", f(branch, 0), c.slot)
	}
	equipment := w.one("AirLoopHVAC:OutdoorAirSystem:EquipmentList", f(oa, 2))
	w.need(len(equipment.Fields) == 3 && strings.EqualFold(f(equipment, 1), "OutdoorAir:Mixer") && w.fieldUses(oa.Type, []int{2}, f(equipment, 0)) == 1, "OA equipment is not uniquely one mixer")
	mixer := w.one("OutdoorAir:Mixer", f(equipment, 2))
	w.typed(mixer.Type, f(mixer, 0), equipment.Type, f(equipment, 0), 1)
	w.eq(f(mixer, 4), f(branch, 4))
	w.eq(f(mixer, 1), f(branch, 5))
	controllers := w.one("AirLoopHVAC:ControllerList", f(oa, 1))
	w.need(len(controllers.Fields) == 3 && strings.EqualFold(f(controllers, 1), "Controller:OutdoorAir") && w.fieldUses(oa.Type, []int{1}, f(controllers, 0)) == 1, "OA controller list is not uniquely one controller")
	controller := w.one("Controller:OutdoorAir", f(controllers, 2))
	w.typed(controller.Type, f(controller, 0), controllers.Type, f(controllers, 0), 1)
	for _, pair := range [][2]int{{1, 3}, {2, 4}, {3, 1}, {4, 2}} {
		w.eq(f(controller, pair[0]), f(mixer, pair[1]))
	}
	w.outdoorNode(f(mixer, 2))
	w.schedule(f(controller, 16))
	dx := w.one("CoilSystem:Cooling:DX", d.CoolingWrapperName)
	coil := w.one("Coil:Cooling:DX:SingleSpeed", d.CoolingCoilName)
	w.eq(f(dx, 5), coil.Type)
	w.eq(f(dx, 6), f(coil, 0))
	w.typed(coil.Type, f(coil, 0), dx.Type, f(dx, 0), 5)
	w.eq(f(dx, 2), f(branch, 8))
	w.eq(f(dx, 3), f(branch, 9))
	w.eq(f(coil, 8), f(dx, 2))
	w.eq(f(coil, 9), f(dx, 3))
	w.eq(f(dx, 4), f(air, 9))
	w.schedule(f(dx, 1))
	w.schedule(f(coil, 1))
	heat := w.one("Coil:Heating:Electric", d.HeatingCoilName)
	w.eq(f(heat, 4), f(branch, 12))
	w.eq(f(heat, 5), f(branch, 13))
	w.eq(f(heat, 6), f(air, 9))
	w.schedule(f(heat, 1))
	fan := w.one("Fan:ConstantVolume", d.FanName)
	w.eq(f(fan, 7), f(branch, 16))
	w.eq(f(fan, 8), f(branch, 17))
	w.schedule(f(fan, 1))
	for _, curve := range []struct {
		field int
		typ   string
	}{{10, "Curve:Biquadratic"}, {11, "Curve:Quadratic"}, {12, "Curve:Biquadratic"}, {13, "Curve:Quadratic"}, {14, "Curve:Quadratic"}} {
		w.one(curve.typ, f(coil, curve.field))
	}
	mixedCount := 0
	for _, sp := range w.doc.Objects {
		if strings.EqualFold(sp.Type, "SetpointManager:MixedAir") && epathSQLSharedSame(f(sp, 5), f(mixer, 1)) {
			mixedCount++
			w.one(sp.Type, f(sp, 0))
			w.eq(f(sp, 1), "Temperature")
			w.eq(f(sp, 2), f(air, 9))
			w.eq(f(sp, 3), f(fan, 7))
			w.eq(f(sp, 4), f(fan, 8))
		}
	}
	w.need(mixedCount == 1, "mixed air lacks one exact fan-compensated controller")
	supply := w.path("AirLoopHVAC:SupplyPath", f(air, 8))
	w.need(len(supply.Fields) == 4 && strings.EqualFold(f(supply, 2), "AirLoopHVAC:ZoneSplitter"), "unreviewed supply splitter shape")
	splitter := w.one("AirLoopHVAC:ZoneSplitter", f(supply, 3))
	w.typed(splitter.Type, f(splitter, 0), supply.Type, f(supply, 0), 2)
	w.need(len(splitter.Fields) == 3, "Shop splitter must serve one terminal")
	w.eq(f(splitter, 1), f(supply, 1))
	terminal := w.one("AirTerminal:SingleDuct:ConstantVolume:NoReheat", d.TerminalName)
	w.eq(f(terminal, 2), f(splitter, 2))
	w.schedule(f(terminal, 1))
	w.need(f(terminal, 5) == "" && f(terminal, 6) == "", "unreviewed terminal OA modulation")
	w.need(w.fieldUses(terminal.Type, []int{2}, f(terminal, 2)) == 1 && w.fieldUses(terminal.Type, []int{3}, f(terminal, 3)) == 1, "shared CV terminal inlet/outlet")
	adu := w.one("ZoneHVAC:AirDistributionUnit", d.ADUName)
	w.eq(f(adu, 1), f(terminal, 3))
	w.eq(f(adu, 2), terminal.Type)
	w.eq(f(adu, 3), f(terminal, 0))
	w.typed(terminal.Type, f(terminal, 0), adu.Type, f(adu, 0), 2)
	connections := w.one("ZoneHVAC:EquipmentConnections", d.ZoneName)
	w.singleNodeList(f(connections, 2), f(terminal, 3))
	w.need(f(connections, 3) == "" && f(connections, 4) != "" && f(connections, 5) != "", "unreviewed Zone exhaust or missing air/return node")
	zoneEquipment := w.one("ZoneHVAC:EquipmentList", f(connections, 1))
	w.need(len(zoneEquipment.Fields) == 8 && w.fieldUses(connections.Type, []int{1}, f(zoneEquipment, 0)) == 1, "Zone equipment is not one uniquely owned stride6 ADU")
	w.eq(f(zoneEquipment, 2), adu.Type)
	w.eq(f(zoneEquipment, 3), f(adu, 0))
	w.typed(adu.Type, f(adu, 0), zoneEquipment.Type, f(zoneEquipment, 0), 2)
	owner, err := (epathSQLSharedOriginal{doc: w.doc}).terminalOwner(terminal, f(terminal, 3))
	if err != nil && w.err == nil {
		w.err = err
	}
	w.eq(owner, d.ZoneName)
	returns := w.path("AirLoopHVAC:ReturnPath", f(air, 7))
	w.need(len(returns.Fields) == 4 && strings.EqualFold(f(returns, 2), "AirLoopHVAC:ZoneMixer"), "unreviewed return mixer shape")
	returnMixer := w.one("AirLoopHVAC:ZoneMixer", f(returns, 3))
	w.typed(returnMixer.Type, f(returnMixer, 0), returns.Type, f(returns, 0), 2)
	w.need(len(returnMixer.Fields) == 3, "Shop return mixer must have one Zone")
	w.eq(f(returnMixer, 1), f(returns, 1))
	w.eq(f(returnMixer, 2), f(connections, 5))
	w.need(w.fieldUses(connections.Type, []int{4}, f(connections, 4)) == 1 && w.fieldUses(connections.Type, []int{5}, f(connections, 5)) == 1, "shared Zone air/return node")
	count := 0
	for _, sp := range w.doc.Objects {
		if strings.EqualFold(sp.Type, "SetpointManager:SingleZone:Reheat") && epathSQLSharedSame(f(sp, 7), f(air, 9)) {
			count++
			w.one(sp.Type, f(sp, 0))
			w.eq(f(sp, 1), "Temperature")
			w.eq(f(sp, 4), d.ZoneName)
			w.eq(f(sp, 5), f(connections, 4))
			w.eq(f(sp, 6), f(terminal, 3))
		}
	}
	w.need(count == 1, "loop outlet lacks one exact Zone setpoint controller")
	availability := w.one("AvailabilityManagerAssignmentList", f(air, 2))
	w.need(len(availability.Fields) == 3 && strings.EqualFold(f(availability, 1), "AvailabilityManager:NightCycle") && w.fieldUses(air.Type, []int{2}, f(availability, 0)) == 1, "loop availability list is missing/shared")
	manager := w.one("AvailabilityManager:NightCycle", f(availability, 2))
	w.typed(manager.Type, f(manager, 0), availability.Type, f(availability, 0), 1)
	w.schedule(f(manager, 1))
	w.schedule(f(manager, 2))
	w.one("Sizing:System", d.AirLoopName)
}

func (w *epathSQLPVHVACWalk) waterBranch(name string) idf.Object {
	f := epathSQLSharedField
	b := w.one("Branch", name)
	w.need(len(b.Fields) == 6 && w.listUses("BranchList", 1, name) == 1, "SHW branch must have one uniquely listed component")
	typ := f(b, 2)
	o := w.one(typ, f(b, 3))
	in, out := -1, -1
	switch strings.ToLower(typ) {
	case "pump:variablespeed", "pipe:adiabatic", "wateruse:connections":
		in, out = 1, 2
	case "waterheater:mixed":
		in, out = 30, 31
	default:
		w.need(false, "SHW branch contains a space HVAC/unsupported demand")
	}
	w.eq(f(o, in), f(b, 4))
	w.eq(f(o, out), f(b, 5))
	w.need(!epathSQLSharedSame(f(b, 4), f(b, 5)), "SHW branch short circuit")
	uses := 0
	for _, other := range w.doc.Objects {
		if !strings.EqualFold(other.Type, "Branch") {
			continue
		}
		for n := 2; n+3 < len(other.Fields); n += 4 {
			if epathSQLSharedSame(f(other, n), typ) && epathSQLSharedSame(f(other, n+1), f(o, 0)) {
				uses++
			}
		}
	}
	w.need(uses == 1, "SHW component shared by branches")
	return b
}
func (w *epathSQLPVHVACWalk) waterSide(loop idf.Object, offset, want int) []idf.Object {
	f := epathSQLSharedField
	list := w.one("BranchList", f(loop, offset+2))
	w.need(len(list.Fields) == want+1 && w.fieldUses("PlantLoop", []int{12, 16}, f(list, 0)) == 1, "SHW BranchList census/owner differs")
	var branches []idf.Object
	names := map[string]bool{}
	for n := 1; n < len(list.Fields); n++ {
		name := f(list, n)
		key := epathSQLSharedKey(name)
		w.need(key != "" && !names[key], "duplicate/blank SHW branch")
		names[key] = true
		branches = append(branches, w.waterBranch(name))
	}
	if len(branches) != want {
		return branches
	}
	first, last := branches[0], branches[len(branches)-1]
	w.eq(f(loop, offset), f(first, 4))
	w.eq(f(loop, offset+1), f(last, 5))
	connectors := w.one("ConnectorList", f(loop, offset+3))
	w.need(len(connectors.Fields) == 5 && w.fieldUses("PlantLoop", []int{13, 17}, f(connectors, 0)) == 1, "SHW connector list census/owner differs")
	for _, s := range []struct {
		typ   string
		slot  int
		outer string
	}{{"Connector:Splitter", 1, f(first, 0)}, {"Connector:Mixer", 3, f(last, 0)}} {
		w.eq(f(connectors, s.slot), s.typ)
		c := w.one(s.typ, f(connectors, s.slot+1))
		w.typed(c.Type, f(c, 0), connectors.Type, f(connectors, 0), s.slot)
		w.need(len(c.Fields) == want, "SHW connector does not cover every branch")
		w.eq(f(c, 1), s.outer)
		seen := map[string]bool{}
		for n := 2; n < len(c.Fields); n++ {
			key := epathSQLSharedKey(f(c, n))
			w.need(names[key] && !seen[key] && key != epathSQLSharedKey(f(first, 0)) && key != epathSQLSharedKey(f(last, 0)), "SHW connector has missing/duplicate/foreign branch")
			seen[key] = true
		}
		w.need(len(seen) == want-2, "SHW connector internal roster incomplete")
	}
	return branches
}
func (w *epathSQLPVHVACWalk) domestic(zones map[string]bool) (string, string, string) {
	f := epathSQLSharedField
	var loop idf.Object
	for _, o := range w.doc.Objects {
		if strings.EqualFold(o.Type, "PlantLoop") {
			loop = w.one(o.Type, f(o, 0))
		}
	}
	w.eq(f(loop, 1), "Water")
	w.need(f(loop, 2) == "", "unreviewed SHW fluid identity")
	supply, demand := w.waterSide(loop, 10, 4), w.waterSide(loop, 14, 8)
	var pump, heater string
	types := map[string]int{}
	for _, b := range supply {
		types[strings.ToLower(f(b, 2))]++
		switch strings.ToLower(f(b, 2)) {
		case "pump:variablespeed":
			pump = f(b, 3)
		case "waterheater:mixed":
			heater = f(b, 3)
		}
	}
	w.need(types["pump:variablespeed"] == 1 && types["waterheater:mixed"] == 1 && types["pipe:adiabatic"] == 2, "SHW supply is not pump/heater/two passive branches")
	if len(supply) == 4 {
		w.eq(f(supply[0], 2), "Pump:VariableSpeed")
		w.eq(f(supply[1], 2), "WaterHeater:Mixed")
		w.eq(f(supply[2], 2), "Pipe:Adiabatic")
		w.eq(f(supply[3], 2), "Pipe:Adiabatic")
	}
	waterHeater := w.one("WaterHeater:Mixed", heater)
	for _, n := range []int{10, 14, 17} {
		w.eq(f(waterHeater, n), "Electricity")
	}
	w.need(f(waterHeater, 21) == "" && f(waterHeater, 33) == "" && f(waterHeater, 34) == "", "SHW heater has unreviewed Zone/source-side coupling")
	w.schedule(f(waterHeater, 2))
	w.schedule(f(waterHeater, 20))
	w.eq(f(waterHeater, 19), "Schedule")
	w.eq(f(loop, 4), f(loop, 11))
	operations := w.one("PlantEquipmentOperationSchemes", f(loop, 3))
	w.need(len(operations.Fields) == 4 && w.fieldUses(loop.Type, []int{3}, f(operations, 0)) == 1, "SHW operation schemes owner differs")
	w.eq(f(operations, 1), "PlantEquipmentOperation:HeatingLoad")
	w.schedule(f(operations, 3))
	operation := w.one("PlantEquipmentOperation:HeatingLoad", f(operations, 2))
	w.typed(operation.Type, f(operation, 0), operations.Type, f(operations, 0), 1)
	w.need(len(operation.Fields) == 4, "SHW has multiple operation ranges")
	equipment := w.one("PlantEquipmentList", f(operation, 3))
	w.need(len(equipment.Fields) == 3 && w.fieldUses(operation.Type, []int{3}, f(equipment, 0)) == 1, "SHW operation equipment list differs")
	w.eq(f(equipment, 1), waterHeater.Type)
	w.eq(f(equipment, 2), heater)
	setpoints := 0
	for _, sp := range w.doc.Objects {
		if strings.EqualFold(sp.Type, "SetpointManager:Scheduled") && epathSQLSharedSame(f(sp, 3), f(loop, 4)) {
			setpoints++
			w.one(sp.Type, f(sp, 0))
			w.eq(f(sp, 1), "Temperature")
			w.schedule(f(sp, 2))
		}
	}
	w.need(setpoints == 1, "SHW lacks one own scheduled setpoint manager")
	w.one("Sizing:Plant", f(loop, 0))
	served := map[string]bool{}
	passive := 0
	for _, b := range demand {
		switch strings.ToLower(f(b, 2)) {
		case "pipe:adiabatic":
			passive++
		case "wateruse:connections":
			c := w.one("WaterUse:Connections", f(b, 3))
			w.need(len(c.Fields) == 11, "SHW demand is not one WaterUse equipment")
			e := w.one("WaterUse:Equipment", f(c, 10))
			z := epathSQLSharedKey(f(e, 7))
			w.need(zones[z] && !served[z] && w.fieldUses(c.Type, []int{10}, f(e, 0)) == 1, "SHW demand has wrong/shared Zone equipment")
			served[z] = true
			for _, n := range []int{3, 4, 5, 8, 9} {
				w.schedule(f(e, n))
			}
			w.need(f(e, 6) == "", "unreviewed SHW cold supply schedule")
		default:
			w.need(false, "SHW demand is a space HVAC coil")
		}
	}
	w.need(len(served) == 5 && passive == 3, "SHW five-demand/passive branch census differs")
	return f(loop, 0), pump, heater
}

func epathSQLPVHVACPhysical(doc idf.Document) []epathSQLPVOriginalObject {
	var out []epathSQLPVOriginalObject
	for _, o := range doc.Objects {
		typ := strings.ToLower(strings.TrimSpace(o.Type))
		if strings.HasPrefix(typ, "output") || typ == "runperiod" || typ == "simulationcontrol" {
			continue
		}
		out = append(out, epathSQLPVObject(o))
	}
	return out
}

func epathSQLValidatePVHVACOriginal(original string, declarations []epathRealSQLPVHVACLoop) (epathSQLPVHVACOriginalProof, error) {
	var out epathSQLPVHVACOriginalProof
	if len(declarations) == 0 {
		return out, nil
	}
	if len(declarations) != 5 || strings.TrimSpace(original) == "" {
		return out, fmt.Errorf("finite Shop HVAC requires five declared original loops")
	}
	doc, err := idf.Parse(original)
	if err != nil {
		return out, err
	}
	w := epathSQLPVHVACWalk{doc: doc}
	counts := map[string]int{}
	indices := map[int]bool{}
	for _, o := range doc.Objects {
		typ := strings.ToLower(strings.TrimSpace(o.Type))
		counts[typ]++
		w.need(o.Index >= 0 && !indices[o.Index], "duplicate/invalid original object index")
		indices[o.Index] = true
		if strings.HasPrefix(typ, "fan:") {
			w.need(typ == "fan:constantvolume", "additional fan reporting owner")
		}
		if strings.HasPrefix(typ, "coil:") {
			w.need(typ == "coil:cooling:dx:singlespeed" || typ == "coil:heating:electric", "additional/shared HVAC coil")
		}
		if strings.HasPrefix(typ, "coilsystem:") {
			w.need(typ == "coilsystem:cooling:dx", "unreviewed coil wrapper")
		}
		if strings.HasPrefix(typ, "airterminal:") {
			w.need(typ == "airterminal:singleduct:constantvolume:noreheat", "unreviewed terminal owner")
		}
		if strings.HasPrefix(typ, "pump:") {
			w.need(typ == "pump:variablespeed", "additional pump owner")
		}
		if strings.HasPrefix(typ, "waterheater:") {
			w.need(typ == "waterheater:mixed", "additional water heating owner")
		}
		if strings.HasPrefix(typ, "zonehvac:") {
			w.need(typ == "zonehvac:airdistributionunit" || typ == "zonehvac:equipmentconnections" || typ == "zonehvac:equipmentlist", "additional Zone equipment owner")
		}
	}
	for typ, n := range map[string]int{"setpointmanager:mixedair": 5, "setpointmanager:scheduled": 1, "plantequipmentoperationschemes": 1, "plantequipmentoperation:heatingload": 1, "plantequipmentlist": 1, "sizing:plant": 1} {
		w.need(counts[typ] == n, fmt.Sprintf("%s census %d want %d", typ, counts[typ], n))
	}
	for typ, n := range map[string]int{"version": 1, "zone": 5, "zonegroup": 0, "zonelist": 0, "airloophvac": 5, "airloophvac:outdoorairsystem": 5, "airloophvac:outdoorairsystem:equipmentlist": 5, "airloophvac:controllerlist": 5, "outdoorair:mixer": 5, "controller:outdoorair": 5, "outdoorair:nodelist": 5, "nodelist": 10, "coilsystem:cooling:dx": 5, "coil:cooling:dx:singlespeed": 5, "coil:heating:electric": 5, "fan:constantvolume": 5, "airloophvac:supplypath": 5, "airloophvac:zonesplitter": 5, "airloophvac:returnpath": 5, "airloophvac:zonemixer": 5, "airterminal:singleduct:constantvolume:noreheat": 5, "zonehvac:airdistributionunit": 5, "zonehvac:equipmentconnections": 5, "zonehvac:equipmentlist": 5, "setpointmanager:singlezone:reheat": 5, "availabilitymanagerassignmentlist": 5, "availabilitymanager:nightcycle": 5, "sizing:system": 5, "plantloop": 1, "condenserloop": 0, "pump:variablespeed": 1, "waterheater:mixed": 1, "wateruse:connections": 5, "wateruse:equipment": 5, "branchlist": 7, "branch": 17, "connectorlist": 2, "connector:splitter": 2, "connector:mixer": 2, "pipe:adiabatic": 5} {
		w.need(counts[typ] == n, fmt.Sprintf("%s census %d want %d", typ, counts[typ], n))
	}
	for _, o := range doc.Objects {
		if strings.EqualFold(o.Type, "Version") {
			w.eq(epathSQLSharedField(o, 0), "25.1")
		}
	}
	zones := map[string]bool{}
	names := map[string]bool{}
	for _, d := range declarations {
		for n, value := range []string{d.ZoneName, d.AirLoopName, d.CoolingWrapperName, d.CoolingCoilName, d.HeatingCoilName, d.FanName, d.TerminalName, d.ADUName} {
			key := fmt.Sprintf("%d/%s", n, epathSQLSharedKey(value))
			w.need(value != "" && strings.TrimSpace(value) == value && !names[key], "blank/repeated declaration identity")
			names[key] = true
		}
		zones[epathSQLSharedKey(d.ZoneName)] = true
		w.air(d)
	}
	loop, pump, heater := w.domestic(zones)
	if w.err != nil {
		return out, w.err
	}
	out = epathSQLPVHVACOriginalProof{OriginalSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(original))), Loops: append([]epathRealSQLPVHVACLoop(nil), declarations...), Objects: epathSQLPVHVACPhysical(doc), DomesticWaterLoop: loop, DomesticPump: pump, DomesticWaterHeater: heater}
	return out, nil
}

func epathSQLBindPVHVACExecuted(original, executed string, proof epathSQLPVHVACOriginalProof) ([]epathSQLPVOriginalObject, error) {
	if len(proof.Loops) != 5 || proof.OriginalSHA256 == "" {
		return nil, fmt.Errorf("missing mandatory Shop HVAC original proof")
	}
	rebuilt, err := epathSQLValidatePVHVACOriginal(original, proof.Loops)
	if err != nil || !reflect.DeepEqual(rebuilt, proof) {
		return nil, fmt.Errorf("Shop HVAC original proof changed/unbound")
	}
	run, err := epathSQLValidatePVHVACOriginal(executed, proof.Loops)
	if err != nil {
		return nil, err
	}
	if len(run.Objects) != len(proof.Objects) || run.DomesticWaterLoop != proof.DomesticWaterLoop || run.DomesticPump != proof.DomesticPump || run.DomesticWaterHeater != proof.DomesticWaterHeater {
		return nil, fmt.Errorf("Shop executed physical/component roster changed")
	}
	for n, want := range proof.Objects {
		got := run.Objects[n]
		if got.ObjectType != want.ObjectType || got.ObjectName != want.ObjectName || !reflect.DeepEqual(got.Fields, want.Fields) {
			return nil, fmt.Errorf("Shop executed physical/control fields changed: %s/%s", want.ObjectType, want.ObjectName)
		}
	}
	return run.Objects, nil
}

// This gate binds declaration-only fan/service math to the finite original.
// It does not compute a source scalar, a delivered load, or a Zone allocation.
func epathSQLPVHVACDeclaredInputs(original string, proof epathSQLPVHVACOriginalProof, pools []epathRealSQLFanPool, services []epathRealSQLService, auxiliaries []epathRealSQLAuxiliary) error {
	if len(proof.Loops) != 5 || proof.OriginalSHA256 == "" {
		return fmt.Errorf("missing mandatory Shop HVAC declaration proof")
	}
	rebuilt, err := epathSQLValidatePVHVACOriginal(original, proof.Loops)
	if err != nil || !reflect.DeepEqual(rebuilt, proof) {
		return fmt.Errorf("unbound Shop HVAC declaration proof")
	}
	if len(pools) != 5 || len(services) != 2 {
		return fmt.Errorf("Shop requires five native fan pools and both services")
	}
	seen := map[string]bool{}
	zones := []string{}
	for _, d := range proof.Loops {
		zones = append(zones, epathSQLSharedKey(d.ZoneName))
		count := 0
		for _, p := range pools {
			if !strings.EqualFold(p.Key, d.AirLoopName) {
				continue
			}
			count++
			if p.SiteID != "fans.electricity" || p.Name != "Air System Fan Electricity Energy" || p.Frequency != "Hourly" || p.Unit != "J" || len(p.ServedZones) != 1 || !strings.EqualFold(p.ServedZones[0], d.ZoneName) {
				return fmt.Errorf("Shop native fan pool borrowed another owner/Zone/source")
			}
		}
		if count != 1 {
			return fmt.Errorf("Shop loop has missing/repeated fan pool")
		}
	}
	sort.Strings(zones)
	for _, s := range services {
		if seen[s.Service] || s.Service != "cooling" && s.Service != "heating" {
			return fmt.Errorf("Shop services changed")
		}
		seen[s.Service] = true
		wantRatio := "coefficient_of_performance"
		if s.Service == "heating" {
			wantRatio = "load_to_site_energy"
		}
		actual, err := epathSQLAirLoopFanZones(s.ServedZones)
		if err != nil || !reflect.DeepEqual(actual, zones) || !reflect.DeepEqual(s.SiteIDs, []string{s.Service + ".electricity"}) || s.Basis != "service_path_allocation" || s.FallbackBasis != "zone_load_allocation" || s.RatioKind != wantRatio || s.FallbackRatioKind != wantRatio {
			return fmt.Errorf("Shop service declaration borrowed SHW/fuel/Zone or ratio boundary")
		}
	}
	for _, a := range auxiliaries {
		if a.SiteID == "pumps.electricity" && len(a.ServedZones) > 0 {
			return fmt.Errorf("domestic-water pump cannot use HVAC Zone load recipients")
		}
	}
	return nil
}
