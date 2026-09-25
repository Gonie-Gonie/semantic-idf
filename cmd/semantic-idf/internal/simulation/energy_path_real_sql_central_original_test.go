package simulation

// Finite original/executed proof for the pinned simultaneous fixture. Only the
// lexical parser and existing test-only raw-object helpers are shared. No
// production binding, HVAC report, classifier, or allocation is authority.
import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathSQLCentralRecipient struct {
	ZoneName                                                    string
	Zone, Terminal, ADU, EquipmentList, Connection, HeatingCoil int
}

type epathSQLCentralOriginalProof struct {
	OriginalSHA256                                        string
	System, CoolingLoop, HeatingLoop, SourceLoop, AirLoop int
	Recipients                                            []epathSQLCentralRecipient
	Objects                                               []epathSQLPVOriginalObject
}

func epathSQLValidateCentralOriginal(original string) (epathSQLCentralOriginalProof, error) {
	var proof epathSQLCentralOriginalProof
	doc, err := idf.Parse(original)
	if err != nil {
		return proof, err
	}
	g, f, same := epathSQLSharedOriginal{doc: doc}, epathSQLSharedField, epathSQLSharedSame
	var failure error
	need := func(ok bool, why string) {
		if !ok && failure == nil {
			failure = fmt.Errorf("finite Central original: %s", why)
		}
	}
	one := func(typ, name string) idf.Object {
		o, e := g.one(typ, name)
		if e != nil && failure == nil {
			failure = e
		}
		return o
	}
	eq := func(a, b string) { need(same(a, b), "disconnected native identity/node "+a+" / "+b) }
	counts := map[string]int{}
	for _, o := range doc.Objects {
		counts[strings.ToLower(o.Type)]++
	}
	for typ, count := range map[string]int{"version": 1, "centralheatpumpsystem": 1, "chillerheaterperformance:electric:eir": 1, "plantloop": 2, "condenserloop": 1, "airloophvac": 1, "fan:constantvolume": 1, "coil:cooling:water:detailedgeometry": 1, "coil:heating:water": 5, "controller:watercoil": 1, "airloophvac:controllerlist": 1, "airloophvac:zonesplitter": 1, "airloophvac:supplypath": 1, "airloophvac:returnplenum": 1, "airloophvac:returnpath": 1, "airterminal:singleduct:constantvolume:reheat": 5, "zonehvac:airdistributionunit": 5, "zonehvac:equipmentlist": 5, "zonehvac:equipmentconnections": 5, "zone": 6, "zonegroup": 0, "zonelist": 0, "branch": 31, "branchlist": 7, "connectorlist": 6, "connector:splitter": 6, "connector:mixer": 6, "nodelist": 9, "pump:variablespeed": 3} {
		need(counts[typ] == count, fmt.Sprintf("%s census %d want %d", typ, counts[typ], count))
	}
	for _, o := range doc.Objects {
		typ := strings.ToLower(o.Type)
		if strings.HasPrefix(typ, "airterminal:") {
			need(typ == "airterminal:singleduct:constantvolume:reheat", "foreign terminal family")
		}
		if strings.HasPrefix(typ, "zonehvac:") {
			need(typ == "zonehvac:airdistributionunit" || typ == "zonehvac:equipmentlist" || typ == "zonehvac:equipmentconnections", "foreign Zone equipment")
		}
		if strings.HasPrefix(typ, "coil:") {
			need(typ == "coil:cooling:water:detailedgeometry" || typ == "coil:heating:water", "foreign coil family")
		}
		if strings.HasPrefix(typ, "fan:") {
			need(typ == "fan:constantvolume", "foreign fan family")
		}
	}
	one("Version", "25.1")
	system := one("CentralHeatPumpSystem", "ChillerBank")
	eq(f(system, 1), "SmartMixing")
	eq(f(system, 10), "ChillerHeaterPerformance:Electric:EIR")
	eq(f(system, 11), f(one("ChillerHeaterPerformance:Electric:EIR", "ChillerHeaterModule"), 0))
	eq(f(system, 12), f(one("Schedule:Compact", "ON"), 0))
	need(f(system, 13) == "3" && len(system.Fields) == 14, "finite three-module roster changed; count is not an electricity multiplier")
	loops := []idf.Object{one("PlantLoop", "Chilled Water Loop"), one("PlantLoop", "Hot Water Loop"), one("CondenserLoop", "Chilled Water Condenser Loop")}
	for n, loop := range loops {
		for _, base := range []int{10, 14} {
			if e := epathSQLCentralPlantSide(doc, loop, base, system); e != nil && failure == nil {
				failure = e
			}
		}
		branchName, port, base := "Big Chiller Branch", 2, 10
		if n == 1 {
			branchName, port = "Hot Water Branch", 6
		}
		if n == 2 {
			branchName, port, base = "Big Chiller Condenser Branch", 4, 14
		}
		branch := one("Branch", branchName)
		eq(f(branch, 2), system.Type)
		eq(f(branch, 3), f(system, 0))
		eq(f(branch, 4), f(system, port))
		eq(f(branch, 5), f(system, port+1))
		list := one("BranchList", f(loop, base+2))
		occurrences := 0
		for p := 1; p < len(list.Fields); p++ {
			if same(f(list, p), branchName) {
				occurrences++
			}
		}
		need(occurrences == 1, "system circuit is not in its exact native side")
	}
	air := one("AirLoopHVAC", "Typical Terminal Reheat 1")
	branch := one("Branch", "Air Loop Main Branch")
	list := one("BranchList", f(air, 4))
	need(len(list.Fields) == 2 && len(branch.Fields) == 10 && f(air, 5) == "", "unexpected supply network")
	eq(f(list, 1), f(branch, 0))
	eq(f(branch, 2), "Fan:ConstantVolume")
	eq(f(branch, 6), "Coil:Cooling:Water:DetailedGeometry")
	fan, cool := one(f(branch, 2), f(branch, 3)), one(f(branch, 6), f(branch, 7))
	eq(f(fan, 7), f(branch, 4))
	eq(f(fan, 8), f(branch, 5))
	eq(f(cool, 20), f(branch, 8))
	eq(f(cool, 21), f(branch, 9))
	eq(f(branch, 5), f(branch, 8))
	eq(f(air, 6), f(branch, 4))
	eq(f(air, 9), f(branch, 9))
	controllers := one("AirLoopHVAC:ControllerList", f(air, 1))
	need(len(controllers.Fields) == 3, "extra controller")
	eq(f(controllers, 1), "Controller:WaterCoil")
	controller := one(f(controllers, 1), f(controllers, 2))
	eq(f(controller, 1), "Temperature")
	eq(f(controller, 2), "Reverse")
	eq(f(controller, 3), "Flow")
	eq(f(controller, 4), f(cool, 21))
	eq(f(controller, 5), f(cool, 18))
	supply := one("AirLoopHVAC:SupplyPath", "TermReheatSupplyPath")
	splitter := one("AirLoopHVAC:ZoneSplitter", "Zone Supply Air Splitter")
	need(len(supply.Fields) == 4 && len(splitter.Fields) == 7, "not the exact five-recipient supply")
	eq(f(supply, 1), f(air, 8))
	eq(f(supply, 2), splitter.Type)
	eq(f(supply, 3), f(splitter, 0))
	eq(f(splitter, 1), f(air, 8))
	back := one("AirLoopHVAC:ReturnPath", "ReturnAirPath1")
	plenum := one("AirLoopHVAC:ReturnPlenum", "Return-Plenum-1")
	need(len(back.Fields) == 4 && len(plenum.Fields) == 10 && f(plenum, 4) == "", "not exact five-inlet return plenum")
	eq(f(back, 1), f(air, 7))
	eq(f(back, 2), plenum.Type)
	eq(f(back, 3), f(plenum, 0))
	eq(f(plenum, 3), f(air, 7))
	eq(f(plenum, 1), f(one("Zone", "PLENUM-1"), 0))
	need(f(plenum, 2) != "" && !same(f(plenum, 2), f(plenum, 3)), "return plenum Zone node missing")
	// These are literal reviewed declarations, not ownership inferred from a
	// naming pattern. Every typed edge/node is checked against the original.
	for n, row := range [][5]string{
		{"SPACE1-1", "Reheat Zone 1", "Zone1TermReheat", "Zone1Equipment", "Reheat Coil Zone 1"},
		{"SPACE2-1", "Reheat Zone 2", "Zone2TermReheat", "Zone2Equipment", "Reheat Coil Zone 2"},
		{"SPACE3-1", "Reheat Zone 3", "Zone3TermReheat", "Zone3Equipment", "Reheat Coil Zone 3"},
		{"SPACE4-1", "Reheat Zone 4", "Zone4TermReheat", "Zone4Equipment", "Reheat Coil Zone 4"},
		{"SPACE5-1", "Reheat Zone 5", "Zone5TermReheat", "Zone5Equipment", "Reheat Coil Zone 5"},
	} {
		zone, terminal := one("Zone", row[0]), one("AirTerminal:SingleDuct:ConstantVolume:Reheat", row[1])
		adu, equipment := one("ZoneHVAC:AirDistributionUnit", row[2]), one("ZoneHVAC:EquipmentList", row[3])
		connection, heat := one("ZoneHVAC:EquipmentConnections", row[0]), one("Coil:Heating:Water", row[4])
		need(epathSQLPVNumber(zone, 6, 1, 1), "representative Zone multiplier changed")
		eq(f(splitter, n+2), f(terminal, 3))
		eq(f(terminal, 5), heat.Type)
		eq(f(terminal, 6), f(heat, 0))
		eq(f(heat, 6), f(terminal, 3))
		eq(f(heat, 7), f(terminal, 2))
		eq(f(adu, 1), f(terminal, 2))
		eq(f(adu, 2), terminal.Type)
		eq(f(adu, 3), f(terminal, 0))
		need(len(equipment.Fields) == 8 && f(equipment, 4) == "1" && f(equipment, 5) == "1" && f(equipment, 6) == "" && f(equipment, 7) == "", "additional/shared Zone equipment")
		eq(f(equipment, 1), "SequentialLoad")
		eq(f(equipment, 2), adu.Type)
		eq(f(equipment, 3), f(adu, 0))
		eq(f(connection, 1), f(equipment, 0))
		need(len(connection.Fields) == 6 && f(connection, 3) == "" && f(connection, 4) != "", "unexpected Zone connection topology")
		nodes := one("NodeList", f(connection, 2))
		need(len(nodes.Fields) == 2, "Zone inlet selector is not single-node")
		eq(f(nodes, 1), f(adu, 1))
		eq(f(connection, 5), f(plenum, n+5))
		proof.Recipients = append(proof.Recipients, epathSQLCentralRecipient{row[0], zone.Index, terminal.Index, adu.Index, equipment.Index, connection.Index, heat.Index})
	}
	if failure != nil {
		return epathSQLCentralOriginalProof{}, failure
	}
	proof.OriginalSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(original)))
	proof.System, proof.CoolingLoop, proof.HeatingLoop, proof.SourceLoop, proof.AirLoop = system.Index, loops[0].Index, loops[1].Index, loops[2].Index, air.Index
	proof.Objects = epathSQLPVHVACPhysical(doc)
	if len(proof.Objects) != 312 {
		return epathSQLCentralOriginalProof{}, fmt.Errorf("finite Central physical/control census is %d, want312", len(proof.Objects))
	}
	return proof, nil
}

// Independent native stride4/connector proof for the six original plant sides.
func epathSQLCentralPlantSide(doc idf.Document, loop idf.Object, base int, system idf.Object) error {
	g, f, same := epathSQLSharedOriginal{doc: doc}, epathSQLSharedField, epathSQLSharedSame
	list, err := g.one("BranchList", f(loop, base+2))
	if err != nil {
		return err
	}
	if len(list.Fields) < 4 {
		return fmt.Errorf("Central original missing parallel plant side")
	}
	branches := map[string]idf.Object{}
	for n := 1; n < len(list.Fields); n++ {
		b, e := g.one("Branch", f(list, n))
		if e != nil {
			return e
		}
		key := epathSQLSharedKey(f(b, 0))
		if _, exists := branches[key]; exists || len(b.Fields) != 6 {
			return fmt.Errorf("Central original duplicate/non-single water branch")
		}
		branches[key] = b
		o, e := g.one(f(b, 2), f(b, 3))
		if e != nil {
			return e
		}
		port := 1
		switch strings.ToLower(o.Type) {
		case "centralheatpumpsystem":
			port = 2
			if same(loop.Type, "CondenserLoop") {
				port = 4
			} else if same(f(loop, 0), "Hot Water Loop") {
				port = 6
			}
			if o.Index != system.Index || port == 4 && base != 14 || port != 4 && base != 10 {
				return fmt.Errorf("Central original circuit crossed plant side")
			}
		case "coil:cooling:water:detailedgeometry":
			port = 18
			if !same(f(loop, 0), "Chilled Water Loop") || base != 14 {
				return fmt.Errorf("Central cooling coil has wrong loop")
			}
		case "coil:heating:water":
			port = 4
			if !same(f(loop, 0), "Hot Water Loop") || base != 14 {
				return fmt.Errorf("Central reheat coil has wrong loop")
			}
		case "pipe:adiabatic", "pump:variablespeed", "districtcooling", "districtheating:water", "groundheatexchanger:system":
		default:
			return fmt.Errorf("unreviewed Central water component %s", o.Type)
		}
		if !same(f(o, port), f(b, 4)) || !same(f(o, port+1), f(b, 5)) {
			return fmt.Errorf("Central native water ports are disconnected")
		}
		uses := 0
		for _, candidate := range doc.Objects {
			if same(candidate.Type, "BranchList") {
				for p := 1; p < len(candidate.Fields); p++ {
					if same(f(candidate, p), f(b, 0)) {
						uses++
					}
				}
			}
		}
		if uses != 1 {
			return fmt.Errorf("Central water branch shared across lists")
		}
	}
	first, last := branches[epathSQLSharedKey(f(list, 1))], branches[epathSQLSharedKey(f(list, len(list.Fields)-1))]
	if !same(f(first, 4), f(loop, base)) || !same(f(last, 5), f(loop, base+1)) {
		return fmt.Errorf("Central plant outer nodes disconnected")
	}
	c, e := g.one("ConnectorList", f(loop, base+3))
	if e != nil {
		return e
	}
	if len(c.Fields) != 5 || !same(f(c, 1), "Connector:Splitter") || !same(f(c, 3), "Connector:Mixer") {
		return fmt.Errorf("Central native connector types changed")
	}
	for _, spec := range []struct {
		slot     int
		endpoint string
	}{{1, f(first, 0)}, {3, f(last, 0)}} {
		connector, e := g.one(f(c, spec.slot), f(c, spec.slot+1))
		if e != nil {
			return e
		}
		if len(connector.Fields) != len(list.Fields)-1 || !same(f(connector, 1), spec.endpoint) {
			return fmt.Errorf("Central connector outer endpoint/census changed")
		}
		seen := map[string]bool{}
		for n := 2; n < len(connector.Fields); n++ {
			key := epathSQLSharedKey(f(connector, n))
			if _, exists := branches[key]; !exists || seen[key] || same(key, f(first, 0)) || same(key, f(last, 0)) {
				return fmt.Errorf("Central connector lost/duplicated/borrowed internal branch")
			}
			seen[key] = true
		}
	}
	return nil
}

func epathSQLBindCentralExecuted(original, executed string, proof epathSQLCentralOriginalProof) ([]epathSQLPVOriginalObject, error) {
	if proof.OriginalSHA256 == "" || len(proof.Recipients) != 5 {
		return nil, fmt.Errorf("missing finite Central original proof")
	}
	rebuilt, err := epathSQLValidateCentralOriginal(original)
	if err != nil || !reflect.DeepEqual(rebuilt, proof) {
		return nil, fmt.Errorf("finite Central original proof changed/unbound")
	}
	run, err := epathSQLValidateCentralOriginal(executed)
	if err != nil {
		return nil, err
	}
	if len(run.Objects) != len(proof.Objects) {
		return nil, fmt.Errorf("Central executed physical/control census changed")
	}
	for n, want := range proof.Objects {
		got := run.Objects[n]
		if got.ObjectType != want.ObjectType || got.ObjectName != want.ObjectName || !reflect.DeepEqual(got.Fields, want.Fields) {
			return nil, fmt.Errorf("Central executed physical/control fields changed: %s/%s", want.ObjectType, want.ObjectName)
		}
	}
	return run.Objects, nil
}
