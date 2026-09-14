package simulation

// Independent acceptance support. Independent, finite original-model oracle. The lexical parser and
// previously reviewed test-only helpers are shared; no production inventory,
// HVAC analysis, service routes, quantities or candidate values are authority.

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathRealSQLPoolSystem struct {
	ID                     string                     `json:"id"`
	PoolName               string                     `json:"poolName"`
	SurfaceName            string                     `json:"surfaceName"`
	ZoneName               string                     `json:"zoneName"`
	HotWaterLoopName       string                     `json:"hotWaterLoopName"`
	ChilledWaterLoopName   string                     `json:"chilledWaterLoopName"`
	BoilerName             string                     `json:"boilerName"`
	HotWaterPumpName       string                     `json:"hotWaterPumpName"`
	ChilledWaterPumpName   string                     `json:"chilledWaterPumpName"`
	ChillerName            string                     `json:"chillerName"`
	AirLoopName            string                     `json:"airLoopName"`
	FanName                string                     `json:"fanName"`
	OutdoorAirSystemName   string                     `json:"outdoorAirSystemName"`
	OutdoorAirMixerName    string                     `json:"outdoorAirMixerName"`
	MainHeatingCoilName    string                     `json:"mainHeatingCoilName"`
	MainCoolingCoilName    string                     `json:"mainCoolingCoilName"`
	OutdoorHeatingCoilName string                     `json:"outdoorHeatingCoilName"`
	OutdoorCoolingCoilName string                     `json:"outdoorCoolingCoilName"`
	ReturnPlenumName       string                     `json:"returnPlenumName"`
	ReturnPlenumZoneName   string                     `json:"returnPlenumZoneName"`
	ServedZones            []string                   `json:"servedZones"`
	Terminals              []epathRealSQLPoolTerminal `json:"terminals"`
}

type epathRealSQLPoolTerminal struct {
	ZoneName        string `json:"zoneName"`
	TerminalName    string `json:"terminalName"`
	ADUName         string `json:"aduName"`
	HeatingCoilName string `json:"heatingCoilName"`
}

type epathSQLPoolOriginalOwner struct {
	ObjectType, ObjectName            string
	ObjectIndex                       int
	FuelType, PlantLoopName, ZoneName string
}

type epathSQLPoolOriginalProof struct {
	Declaration                          epathRealSQLPoolSystem
	OriginalSHA256                       string
	Owners                               []epathSQLPoolOriginalOwner // Pool, boiler, HW pump, CW pump, chiller, fan.
	HotWaterDemands, ChilledWaterDemands []epathSQLPoolOriginalOwner
	ServedZones                          []string
	SurfaceObjectIndex                   int
}

type epathSQLPoolWalk struct {
	epathSQLSharedOriginal
	branches, branchLists, connectorLists map[int]bool
}

func epathSQLPoolOwner(object idf.Object, loop, zone string) epathSQLPoolOriginalOwner {
	owner := epathSQLPoolOriginalOwner{ObjectType: object.Type, ObjectName: epathSQLSharedField(object, 0), ObjectIndex: object.Index, PlantLoopName: loop, ZoneName: zone}
	if strings.EqualFold(object.Type, "Boiler:HotWater") {
		owner.FuelType = epathSQLSharedField(object, 1)
	}
	return owner
}

func epathSQLPoolSet(values []string) ([]string, error) {
	out, seen := []string{}, map[string]bool{}
	for _, value := range values {
		key := epathSQLSharedKey(value)
		if key == "" || seen[key] {
			return nil, fmt.Errorf("pool original empty/duplicate declared identity %q", value)
		}
		seen[key] = true
		out = append(out, key)
	}
	sort.Strings(out)
	return out, nil
}

func epathSQLValidatePoolOriginal(original string, declarations []epathRealSQLPoolSystem) ([]epathSQLPoolOriginalProof, error) {
	if len(declarations) == 0 {
		return nil, nil
	}
	if len(declarations) != 1 || strings.TrimSpace(original) == "" {
		return nil, fmt.Errorf("pool oracle requires one explicit finite original system")
	}
	d := declarations[0]
	for _, value := range []string{d.ID, d.PoolName, d.SurfaceName, d.ZoneName, d.HotWaterLoopName, d.ChilledWaterLoopName, d.BoilerName, d.HotWaterPumpName, d.ChilledWaterPumpName, d.ChillerName, d.AirLoopName, d.FanName, d.OutdoorAirSystemName, d.OutdoorAirMixerName, d.MainHeatingCoilName, d.MainCoolingCoilName, d.OutdoorHeatingCoilName, d.OutdoorCoolingCoilName, d.ReturnPlenumName, d.ReturnPlenumZoneName} {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("pool original declaration has an empty identity")
		}
	}
	if len(d.ServedZones) != 5 || len(d.Terminals) != 5 {
		return nil, fmt.Errorf("pool oracle requires exact five-Zone terminal roster")
	}
	wantZones, err := epathSQLPoolSet(d.ServedZones)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(d.ReturnPlenumZoneName, d.ZoneName) {
		return nil, fmt.Errorf("pool floor cannot borrow return-plenum ownership")
	}
	doc, err := idf.Parse(original)
	if err != nil {
		return nil, err
	}
	w := epathSQLPoolWalk{epathSQLSharedOriginal: epathSQLSharedOriginal{doc: doc}, branches: map[int]bool{}, branchLists: map[int]bool{}, connectorLists: map[int]bool{}}
	// Finite type census closes hidden/unconnected source and demand loopholes.
	counts := map[string]int{}
	for _, object := range doc.Objects {
		counts[epathSQLSharedKey(object.Type)]++
	}
	for typ, count := range map[string]int{"version": 1, "zone": 6, "swimmingpool:indoor": 1, "plantloop": 2, "condenserloop": 0, "boiler:hotwater": 1, "pump:variablespeed": 2, "chiller:electric": 1, "fan:variablevolume": 1, "coil:heating:water": 7, "coil:cooling:water": 2, "airloophvac": 1, "airloophvac:outdoorairsystem": 1, "outdoorair:mixer": 1, "airterminal:singleduct:vav:reheat": 5, "zonehvac:airdistributionunit": 5, "zonehvac:equipmentlist": 5, "zonehvac:equipmentconnections": 5, "airloophvac:returnpath": 1, "airloophvac:returnplenum": 1, "airloophvac:supplypath": 1, "airloophvac:zonesplitter": 1, "controller:watercoil": 4, "controller:outdoorair": 1, "airloophvac:controllerlist": 2, "connector:splitter": 4, "connector:mixer": 4} {
		if counts[typ] != count {
			return nil, fmt.Errorf("pool original %s census=%d want%d", typ, counts[typ], count)
		}
	}
	for _, object := range doc.Objects {
		if strings.EqualFold(object.Type, "Version") && epathSQLSharedField(object, 0) != "25.1" {
			return nil, fmt.Errorf("pool original version is not reviewed25.1")
		}
	}
	if counts["airloophvac:outdoorairsystem:equipmentlist"] != 1 {
		return nil, fmt.Errorf("pool original OA equipment-list census differs")
	}
	for _, family := range []struct {
		prefixes []string
		count    int
	}{{[]string{"boiler:"}, 1}, {[]string{"pump:", "headeredpumps:"}, 2}, {[]string{"chiller:"}, 1}, {[]string{"fan:"}, 1}, {[]string{"coil:"}, 9}, {[]string{"airterminal:"}, 5}, {[]string{"zonehvac:"}, 15}} {
		count := 0
		for _, object := range doc.Objects {
			for _, prefix := range family.prefixes {
				if strings.HasPrefix(epathSQLSharedKey(object.Type), prefix) {
					count++
					break
				}
			}
		}
		if count != family.count {
			return nil, fmt.Errorf("pool original native reporting family census is ambiguous: %v", family.prefixes)
		}
	}
	pool, err := w.one("SwimmingPool:Indoor", d.PoolName)
	if err != nil {
		return nil, err
	}
	surface, err := w.one("BuildingSurface:Detailed", d.SurfaceName)
	if err != nil {
		return nil, err
	}
	if !epathSQLSharedSame(epathSQLSharedField(pool, 1), d.SurfaceName) || !strings.EqualFold(epathSQLSharedField(surface, 1), "Floor") || !strings.EqualFold(epathSQLSharedField(surface, 5), "Ground") || epathSQLSharedField(surface, 4) != "" || !epathSQLSharedSame(epathSQLSharedField(surface, 3), d.ZoneName) {
		return nil, fmt.Errorf("pool original Floor/Ground/Zone identity differs")
	}
	surfaceNames := 0
	for _, object := range doc.Objects {
		for _, prefix := range []string{"buildingsurface:", "fenestrationsurface:", "floor:", "wall:", "roof:", "roofceiling:", "ceiling:", "window:", "door:", "glazeddoor:"} {
			if strings.HasPrefix(epathSQLSharedKey(object.Type), prefix) && epathSQLSharedSame(epathSQLSharedField(object, 0), d.SurfaceName) {
				surfaceNames++
				break
			}
		}
	}
	if surfaceNames != 1 {
		return nil, fmt.Errorf("pool original surface reporting identity is ambiguous")
	}
	actualZones := []string{}
	for _, object := range doc.Objects {
		if strings.EqualFold(object.Type, "Zone") {
			actualZones = append(actualZones, epathSQLSharedField(object, 0))
		}
	}
	allZones, err := epathSQLPoolSet(actualZones)
	if err != nil {
		return nil, err
	}
	wantedAll, err := epathSQLPoolSet(append(append([]string(nil), d.ServedZones...), d.ReturnPlenumZoneName))
	if err != nil || !reflect.DeepEqual(allZones, wantedAll) {
		return nil, fmt.Errorf("pool original six Zone/return-plenum census differs")
	}
	if _, err = w.one("Zone", d.ZoneName); err != nil {
		return nil, err
	}
	hw, err := w.one("PlantLoop", d.HotWaterLoopName)
	if err != nil {
		return nil, err
	}
	cw, err := w.one("PlantLoop", d.ChilledWaterLoopName)
	if err != nil {
		return nil, err
	}
	if hw.Index == cw.Index {
		return nil, fmt.Errorf("pool original HW and CW loops are not distinct")
	}
	boiler, err := w.one("Boiler:HotWater", d.BoilerName)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(epathSQLSharedField(boiler, 1), "NaturalGas") {
		return nil, fmt.Errorf("pool original boiler fuel is not native NaturalGas")
	}
	hwp, err := w.one("Pump:VariableSpeed", d.HotWaterPumpName)
	if err != nil {
		return nil, err
	}
	cwp, err := w.one("Pump:VariableSpeed", d.ChilledWaterPumpName)
	if err != nil {
		return nil, err
	}
	if hwp.Index == cwp.Index {
		return nil, fmt.Errorf("pool original pumps are not distinct identities")
	}
	chiller, err := w.one("Chiller:Electric", d.ChillerName)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(epathSQLSharedField(chiller, 1), "AirCooled") {
		return nil, fmt.Errorf("pool original chiller is not reviewed AirCooled")
	}
	heating := [][2]string{{"Coil:Heating:Water", d.OutdoorHeatingCoilName}, {"Coil:Heating:Water", d.MainHeatingCoilName}, {"SwimmingPool:Indoor", d.PoolName}}
	terminalZones := []string{}
	for _, terminal := range d.Terminals {
		heating = append(heating, [2]string{"Coil:Heating:Water", terminal.HeatingCoilName})
		terminalZones = append(terminalZones, terminal.ZoneName)
	}
	tz, err := epathSQLPoolSet(terminalZones)
	if err != nil || !reflect.DeepEqual(tz, wantZones) {
		return nil, fmt.Errorf("pool original declared terminal Zones differ")
	}
	if _, err = w.side(hw, 10, [][2]string{{"Boiler:HotWater", d.BoilerName}, {"Pump:VariableSpeed", d.HotWaterPumpName}}); err != nil {
		return nil, err
	}
	hwDemand, err := w.side(hw, 14, heating)
	if err != nil {
		return nil, err
	}
	if _, err = w.side(cw, 10, [][2]string{{"Chiller:Electric", d.ChillerName}, {"Pump:VariableSpeed", d.ChilledWaterPumpName}}); err != nil {
		return nil, err
	}
	cwDemand, err := w.side(cw, 14, [][2]string{{"Coil:Cooling:Water", d.MainCoolingCoilName}, {"Coil:Cooling:Water", d.OutdoorCoolingCoilName}})
	if err != nil {
		return nil, err
	}
	fan, err := w.air(d)
	if err != nil {
		return nil, err
	}
	for _, object := range doc.Objects {
		var seen map[int]bool
		switch strings.ToLower(object.Type) {
		case "branch":
			seen = w.branches
		case "branchlist":
			seen = w.branchLists
		case "connectorlist":
			seen = w.connectorLists
		default:
			continue
		}
		if !seen[object.Index] {
			return nil, fmt.Errorf("pool original hidden or unowned %s/%s", object.Type, epathSQLSharedField(object, 0))
		}
	}
	d.ServedZones = append([]string(nil), d.ServedZones...)
	d.Terminals = append([]epathRealSQLPoolTerminal(nil), d.Terminals...)
	proof := epathSQLPoolOriginalProof{Declaration: d, OriginalSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(original))), SurfaceObjectIndex: surface.Index, ServedZones: append([]string(nil), d.ServedZones...),
		Owners: []epathSQLPoolOriginalOwner{epathSQLPoolOwner(pool, d.HotWaterLoopName, d.ZoneName), epathSQLPoolOwner(boiler, d.HotWaterLoopName, ""), epathSQLPoolOwner(hwp, d.HotWaterLoopName, ""), epathSQLPoolOwner(cwp, d.ChilledWaterLoopName, ""), epathSQLPoolOwner(chiller, d.ChilledWaterLoopName, ""), epathSQLPoolOwner(fan, "", "")}}
	for _, object := range hwDemand {
		zone := ""
		if object.Index == pool.Index {
			zone = d.ZoneName
		}
		for _, terminal := range d.Terminals {
			if strings.EqualFold(object.Type, "Coil:Heating:Water") && epathSQLSharedSame(epathSQLSharedField(object, 0), terminal.HeatingCoilName) {
				zone = terminal.ZoneName
			}
		}
		proof.HotWaterDemands = append(proof.HotWaterDemands, epathSQLPoolOwner(object, d.HotWaterLoopName, zone))
	}
	for _, object := range cwDemand {
		proof.ChilledWaterDemands = append(proof.ChilledWaterDemands, epathSQLPoolOwner(object, d.ChilledWaterLoopName, ""))
	}
	return []epathSQLPoolOriginalProof{proof}, nil
}

// Only two native port additions beyond the already independent water helpers.
func (w epathSQLPoolWalk) waterPorts(object idf.Object) (string, string, error) {
	a, b := -1, -1
	switch strings.ToLower(object.Type) {
	case "swimmingpool:indoor":
		a, b = 10, 11
	case "chiller:electric":
		a, b = 4, 5
	default:
		return w.ports(object.Type, epathSQLSharedField(object, 0), "water")
	}
	in, out := epathSQLSharedField(object, a), epathSQLSharedField(object, b)
	if in == "" || out == "" || epathSQLSharedSame(in, out) {
		return "", "", fmt.Errorf("pool original missing native water ports")
	}
	return in, out, nil
}

func (w *epathSQLPoolWalk) side(loop idf.Object, offset int, expected [][2]string) ([]idf.Object, error) {
	if !strings.EqualFold(epathSQLSharedField(loop, 1), "Water") {
		return nil, fmt.Errorf("pool original unreviewed plant fluid")
	}
	list, err := w.one("BranchList", epathSQLSharedField(loop, offset+2))
	if err != nil {
		return nil, err
	}
	if w.branchLists[list.Index] || len(list.Fields) < 4 {
		return nil, fmt.Errorf("pool original reused/malformed water branch list")
	}
	w.branchLists[list.Index] = true
	owners := 0
	for _, object := range w.doc.Objects {
		positions := []int(nil)
		switch strings.ToLower(object.Type) {
		case "plantloop", "condenserloop":
			positions = []int{12, 16}
		case "airloophvac":
			positions = []int{4}
		}
		for _, field := range positions {
			if epathSQLSharedSame(epathSQLSharedField(object, field), epathSQLSharedField(list, 0)) {
				owners++
			}
		}
	}
	if owners != 1 {
		return nil, fmt.Errorf("pool original branch list lacks unique loop-side owner")
	}
	want := map[string]bool{}
	for _, identity := range expected {
		key := epathSQLSharedKey(identity[0]) + "|" + epathSQLSharedKey(identity[1])
		if identity[1] == "" || want[key] {
			return nil, fmt.Errorf("pool original repeated declared demand/source")
		}
		want[key] = true
	}
	actual := map[string]bool{}
	result := []idf.Object{}
	names := []string{}
	for field := 1; field < len(list.Fields); field++ {
		name := epathSQLSharedField(list, field)
		branch, err := w.one("Branch", name)
		if err != nil {
			return nil, err
		}
		if w.branches[branch.Index] || len(branch.Fields) < 6 || (len(branch.Fields)-2)%4 != 0 {
			return nil, fmt.Errorf("pool original shared/malformed Branch %s", name)
		}
		w.branches[branch.Index] = true
		names = append(names, name)
		uses := 0
		for _, object := range w.doc.Objects {
			if strings.EqualFold(object.Type, "BranchList") {
				for n := 1; n < len(object.Fields); n++ {
					if epathSQLSharedSame(epathSQLSharedField(object, n), name) {
						uses++
					}
				}
			}
		}
		if uses != 1 {
			return nil, fmt.Errorf("pool original water branch has competing list owner")
		}
		previous := ""
		for at := 2; at < len(branch.Fields); at += 4 {
			object, err := w.one(epathSQLSharedField(branch, at), epathSQLSharedField(branch, at+1))
			if err != nil {
				return nil, err
			}
			in, out, err := w.waterPorts(object)
			if err != nil {
				return nil, err
			}
			if !epathSQLSharedSame(in, epathSQLSharedField(branch, at+2)) || !epathSQLSharedSame(out, epathSQLSharedField(branch, at+3)) || previous != "" && !epathSQLSharedSame(previous, in) {
				return nil, fmt.Errorf("pool original native water port continuity failed at %s", epathSQLSharedField(object, 0))
			}
			previous = out
			// Across all original branches, this exact water-port identity occurs
			// once. Legitimate central-coil air ports cannot substitute for it.
			refs := 0
			for _, peer := range w.doc.Objects {
				if strings.EqualFold(peer.Type, "Branch") {
					for n := 2; n+3 < len(peer.Fields); n += 4 {
						if strings.EqualFold(epathSQLSharedField(peer, n), object.Type) && epathSQLSharedSame(epathSQLSharedField(peer, n+1), epathSQLSharedField(object, 0)) && epathSQLSharedSame(epathSQLSharedField(peer, n+2), in) && epathSQLSharedSame(epathSQLSharedField(peer, n+3), out) {
							refs++
						}
					}
				}
			}
			if refs != 1 {
				return nil, fmt.Errorf("pool original native water component has competing branch owners")
			}
			if !strings.EqualFold(object.Type, "Pipe:Adiabatic") {
				key := epathSQLSharedKey(object.Type) + "|" + epathSQLSharedKey(epathSQLSharedField(object, 0))
				if !want[key] || actual[key] {
					return nil, fmt.Errorf("pool original hidden/duplicate non-pipe demand or source %s", key)
				}
				actual[key] = true
				result = append(result, object)
			}
		}
		if field == 1 && !epathSQLSharedSame(epathSQLSharedField(branch, 4), epathSQLSharedField(loop, offset)) {
			return nil, fmt.Errorf("pool original loop-side inlet mismatch")
		}
		if field == len(list.Fields)-1 && !epathSQLSharedSame(previous, epathSQLSharedField(loop, offset+1)) {
			return nil, fmt.Errorf("pool original loop-side outlet mismatch")
		}
	}
	if !reflect.DeepEqual(actual, want) {
		return nil, fmt.Errorf("pool original incomplete typed demand/source census")
	}
	connectors, err := w.one("ConnectorList", epathSQLSharedField(loop, offset+3))
	if err != nil {
		return nil, err
	}
	if w.connectorLists[connectors.Index] || len(connectors.Fields) != 5 {
		return nil, fmt.Errorf("pool original shared or malformed connector list")
	}
	w.connectorLists[connectors.Index] = true
	listUses := 0
	for _, object := range w.doc.Objects {
		fields := []int(nil)
		switch strings.ToLower(object.Type) {
		case "plantloop", "condenserloop":
			fields = []int{13, 17}
		case "airloophvac":
			fields = []int{5}
		}
		for _, at := range fields {
			if epathSQLSharedSame(epathSQLSharedField(object, at), epathSQLSharedField(connectors, 0)) {
				listUses++
			}
		}
	}
	if listUses != 1 {
		return nil, fmt.Errorf("pool original connector list has competing loop owners")
	}
	seen := map[string]bool{}
	for _, at := range []int{1, 3} {
		typ := epathSQLSharedField(connectors, at)
		key := epathSQLSharedKey(typ)
		if seen[key] || key != "connector:splitter" && key != "connector:mixer" {
			return nil, fmt.Errorf("pool original requires one splitter and mixer")
		}
		seen[key] = true
		connector, err := w.one(typ, epathSQLSharedField(connectors, at+1))
		if err != nil {
			return nil, err
		}
		boundary := names[0]
		if key == "connector:mixer" {
			boundary = names[len(names)-1]
		}
		if len(connector.Fields) != len(names) || !epathSQLSharedSame(epathSQLSharedField(connector, 1), boundary) {
			return nil, fmt.Errorf("pool original connector outer branch mismatch")
		}
		refs := 0
		for _, object := range w.doc.Objects {
			if strings.EqualFold(object.Type, "ConnectorList") {
				for n := 1; n+1 < len(object.Fields); n++ {
					if strings.EqualFold(epathSQLSharedField(object, n), typ) && epathSQLSharedSame(epathSQLSharedField(object, n+1), epathSQLSharedField(connector, 0)) {
						refs++
					}
				}
			}
		}
		if refs != 1 {
			return nil, fmt.Errorf("pool original splitter/mixer has ambiguous list ownership")
		}
		members := []string{}
		for n := 2; n < len(connector.Fields); n++ {
			members = append(members, epathSQLSharedField(connector, n))
		}
		actualMembers, err := epathSQLPoolSet(members)
		if err != nil {
			return nil, err
		}
		wantedMembers, err := epathSQLPoolSet(names[1 : len(names)-1])
		if err != nil || !reflect.DeepEqual(actualMembers, wantedMembers) {
			return nil, fmt.Errorf("pool original splitter/mixer omits or adds a branch")
		}
	}
	return result, nil
}

func (w *epathSQLPoolWalk) air(d epathRealSQLPoolSystem) (idf.Object, error) {
	air, err := w.one("AirLoopHVAC", d.AirLoopName)
	if err != nil {
		return idf.Object{}, err
	}
	list, err := w.one("BranchList", epathSQLSharedField(air, 4))
	if err != nil {
		return idf.Object{}, err
	}
	if len(list.Fields) != 2 || w.branchLists[list.Index] || epathSQLSharedField(air, 5) != "" {
		return idf.Object{}, fmt.Errorf("pool original requires one unambiguous main air branch")
	}
	w.branchLists[list.Index] = true
	branch, err := w.one("Branch", epathSQLSharedField(list, 1))
	if err != nil {
		return idf.Object{}, err
	}
	if len(branch.Fields) != 18 || w.branches[branch.Index] {
		return idf.Object{}, fmt.Errorf("pool original main air component census differs")
	}
	w.branches[branch.Index] = true
	branchUses := 0
	for _, object := range w.doc.Objects {
		if strings.EqualFold(object.Type, "BranchList") {
			for at := 1; at < len(object.Fields); at++ {
				if epathSQLSharedSame(epathSQLSharedField(object, at), epathSQLSharedField(branch, 0)) {
					branchUses++
				}
			}
		}
	}
	if branchUses != 1 {
		return idf.Object{}, fmt.Errorf("pool original main air branch has competing list owners")
	}
	oa, err := w.one("AirLoopHVAC:OutdoorAirSystem", d.OutdoorAirSystemName)
	if err != nil {
		return idf.Object{}, err
	}
	mixer, err := w.one("OutdoorAir:Mixer", d.OutdoorAirMixerName)
	if err != nil {
		return idf.Object{}, err
	}
	steps := [][2]string{{"AirLoopHVAC:OutdoorAirSystem", d.OutdoorAirSystemName}, {"Coil:Cooling:Water", d.MainCoolingCoilName}, {"Coil:Heating:Water", d.MainHeatingCoilName}, {"Fan:VariableVolume", d.FanName}}
	previous := epathSQLSharedField(air, 6)
	for i, step := range steps {
		at := 2 + 4*i
		if !strings.EqualFold(epathSQLSharedField(branch, at), step[0]) || !epathSQLSharedSame(epathSQLSharedField(branch, at+1), step[1]) {
			return idf.Object{}, fmt.Errorf("pool original main air sequence differs")
		}
		in, out := epathSQLSharedField(mixer, 4), epathSQLSharedField(mixer, 1)
		if i > 0 {
			in, out, err = w.ports(step[0], step[1], "air")
			if err != nil {
				return idf.Object{}, err
			}
		}
		if !epathSQLSharedSame(in, previous) || !epathSQLSharedSame(in, epathSQLSharedField(branch, at+2)) || !epathSQLSharedSame(out, epathSQLSharedField(branch, at+3)) {
			return idf.Object{}, fmt.Errorf("pool original main air/native ports are disconnected")
		}
		previous = out
	}
	if !epathSQLSharedSame(previous, epathSQLSharedField(air, 9)) {
		return idf.Object{}, fmt.Errorf("pool original fan outlet/air loop outlet mismatch")
	}
	oalist, err := w.one("AirLoopHVAC:OutdoorAirSystem:EquipmentList", epathSQLSharedField(oa, 2))
	if err != nil {
		return idf.Object{}, err
	}
	if len(oalist.Fields) != 7 {
		return idf.Object{}, fmt.Errorf("pool original OA equipment census differs")
	}
	for i, step := range [][2]string{{"Coil:Heating:Water", d.OutdoorHeatingCoilName}, {"Coil:Cooling:Water", d.OutdoorCoolingCoilName}, {"OutdoorAir:Mixer", d.OutdoorAirMixerName}} {
		if !strings.EqualFold(epathSQLSharedField(oalist, 1+2*i), step[0]) || !epathSQLSharedSame(epathSQLSharedField(oalist, 2+2*i), step[1]) {
			return idf.Object{}, fmt.Errorf("pool original outside-air equipment sequence differs")
		}
	}
	hin, hout, err := w.ports("Coil:Heating:Water", d.OutdoorHeatingCoilName, "air")
	if err != nil {
		return idf.Object{}, err
	}
	cin, cout, err := w.ports("Coil:Cooling:Water", d.OutdoorCoolingCoilName, "air")
	if err != nil {
		return idf.Object{}, err
	}
	if !epathSQLSharedSame(hout, cin) || !epathSQLSharedSame(cout, epathSQLSharedField(mixer, 2)) {
		return idf.Object{}, fmt.Errorf("pool original outside-air coil path does not reach mixer outdoor inlet")
	}
	if err = w.outdoorNode(hin); err != nil {
		return idf.Object{}, err
	}
	if err = w.controllers(epathSQLSharedField(oa, 1), [][2]string{{"Coil:Heating:Water", d.OutdoorHeatingCoilName}, {"Coil:Cooling:Water", d.OutdoorCoolingCoilName}}, &mixer, hin); err != nil {
		return idf.Object{}, err
	}
	if err = w.controllers(epathSQLSharedField(air, 1), [][2]string{{"Coil:Heating:Water", d.MainHeatingCoilName}, {"Coil:Cooling:Water", d.MainCoolingCoilName}}, nil, ""); err != nil {
		return idf.Object{}, err
	}
	// Existing independent supply-splitter / terminal / ADU / equipment-list
	// walker proves actual reachability; declared labels alone never suffice.
	zones, err := epathSQLHVACOriginalAirLoopZones(w.doc, d.AirLoopName)
	if err != nil {
		return idf.Object{}, err
	}
	actual, err := epathSQLPoolSet(zones)
	if err != nil {
		return idf.Object{}, err
	}
	want, err := epathSQLPoolSet(d.ServedZones)
	if err != nil || !reflect.DeepEqual(actual, want) {
		return idf.Object{}, fmt.Errorf("pool original five served Zone census differs")
	}
	returns, zoneAirNodes, zoneInlets := []string{}, []string{}, []string{}
	for _, decl := range d.Terminals {
		terminal, _, out, err := w.terminal("AirTerminal:SingleDuct:VAV:Reheat", decl.TerminalName)
		if err != nil {
			return idf.Object{}, err
		}
		if !epathSQLSharedSame(epathSQLSharedField(terminal, 10), decl.HeatingCoilName) {
			return idf.Object{}, fmt.Errorf("pool original terminal/reheat declaration differs")
		}
		zone, err := w.terminalOwner(terminal, out)
		if err != nil || !epathSQLSharedSame(zone, decl.ZoneName) {
			return idf.Object{}, fmt.Errorf("pool original exact terminal Zone owner differs: %v", err)
		}
		adu, err := w.one("ZoneHVAC:AirDistributionUnit", decl.ADUName)
		if err != nil {
			return idf.Object{}, err
		}
		if !strings.EqualFold(epathSQLSharedField(adu, 2), terminal.Type) || !epathSQLSharedSame(epathSQLSharedField(adu, 3), decl.TerminalName) || !epathSQLSharedSame(epathSQLSharedField(adu, 1), out) {
			return idf.Object{}, fmt.Errorf("pool original ADU declaration/ports differ")
		}
		connection, err := w.one("ZoneHVAC:EquipmentConnections", decl.ZoneName)
		if err != nil {
			return idf.Object{}, err
		}
		if epathSQLSharedField(connection, 3) != "" || epathSQLSharedField(connection, 4) == "" || epathSQLSharedField(connection, 5) == "" {
			return idf.Object{}, fmt.Errorf("pool original unreviewed exhaust/blank Zone-air or return selector")
		}
		equipment, err := w.one("ZoneHVAC:EquipmentList", epathSQLSharedField(connection, 1))
		if err != nil {
			return idf.Object{}, err
		}
		if len(equipment.Fields) != 8 || !strings.EqualFold(epathSQLSharedField(equipment, 2), adu.Type) || !epathSQLSharedSame(epathSQLSharedField(equipment, 3), decl.ADUName) {
			return idf.Object{}, fmt.Errorf("pool original Zone equipment has an additional or different consumer")
		}
		returns = append(returns, epathSQLSharedField(connection, 5))
		zoneAirNodes = append(zoneAirNodes, epathSQLSharedField(connection, 4))
		zoneInlets = append(zoneInlets, out)
	}
	if _, err = epathSQLPoolSet(zoneAirNodes); err != nil {
		return idf.Object{}, fmt.Errorf("pool original Zone-air node ownership: %w", err)
	}
	if _, err = epathSQLPoolSet(zoneInlets); err != nil {
		return idf.Object{}, fmt.Errorf("pool original terminal outlet ownership: %w", err)
	}
	plenum, err := w.one("AirLoopHVAC:ReturnPlenum", d.ReturnPlenumName)
	if err != nil {
		return idf.Object{}, err
	}
	if len(plenum.Fields) != 10 || !epathSQLSharedSame(epathSQLSharedField(plenum, 1), d.ReturnPlenumZoneName) || epathSQLSharedField(plenum, 2) == "" || epathSQLSharedField(plenum, 4) != "" || !epathSQLSharedSame(epathSQLSharedField(plenum, 3), epathSQLSharedField(air, 7)) {
		return idf.Object{}, fmt.Errorf("pool original return plenum ownership/outer port differs")
	}
	for _, zone := range d.ServedZones {
		if epathSQLSharedSame(zone, d.ReturnPlenumZoneName) {
			return idf.Object{}, fmt.Errorf("return plenum fabricated as air served")
		}
	}
	for _, node := range zoneAirNodes {
		if epathSQLSharedSame(node, epathSQLSharedField(plenum, 2)) {
			return idf.Object{}, fmt.Errorf("pool original return-plenum Zone-air node has a served owner")
		}
	}
	returnMembers := []string{}
	for at := 5; at < len(plenum.Fields); at++ {
		returnMembers = append(returnMembers, epathSQLSharedField(plenum, at))
	}
	rs, err := epathSQLPoolSet(returnMembers)
	if err != nil {
		return idf.Object{}, err
	}
	ws, err := epathSQLPoolSet(returns)
	if err != nil || !reflect.DeepEqual(rs, ws) {
		return idf.Object{}, fmt.Errorf("pool original return-plenum inlet roster differs from served Zone returns")
	}
	for _, object := range w.doc.Objects {
		if strings.EqualFold(object.Type, "AirLoopHVAC:ReturnPath") {
			if len(object.Fields) != 4 || !epathSQLSharedSame(epathSQLSharedField(object, 1), epathSQLSharedField(air, 7)) || !strings.EqualFold(epathSQLSharedField(object, 2), plenum.Type) || !epathSQLSharedSame(epathSQLSharedField(object, 3), d.ReturnPlenumName) {
				return idf.Object{}, fmt.Errorf("pool original return path is disconnected")
			}
		}
	}
	return w.one("Fan:VariableVolume", d.FanName)
}

func (w epathSQLPoolWalk) outdoorNode(node string) error {
	if node == "" {
		return fmt.Errorf("pool original outside-air inlet is blank")
	}
	count := 0
	for _, object := range w.doc.Objects {
		switch strings.ToLower(object.Type) {
		case "outdoorair:node":
			if epathSQLSharedSame(epathSQLSharedField(object, 0), node) {
				count++
			}
		case "outdoorair:nodelist":
			for at := 0; at < len(object.Fields); at++ {
				selector := epathSQLSharedField(object, at)
				if selector == "" {
					return fmt.Errorf("pool original blank outdoor-air NodeList selector")
				}
				matches := 0
				for _, list := range w.doc.Objects {
					if strings.EqualFold(list.Type, "NodeList") && epathSQLSharedSame(epathSQLSharedField(list, 0), selector) {
						matches++
					}
				}
				if matches == 0 {
					if epathSQLSharedSame(selector, node) {
						count++
					}
					continue
				}
				list, err := w.one("NodeList", selector)
				if err != nil {
					return err
				}
				for n := 1; n < len(list.Fields); n++ {
					if epathSQLSharedField(list, n) == "" {
						return fmt.Errorf("pool original blank native outdoor node alias member")
					}
					if epathSQLSharedSame(epathSQLSharedField(list, n), node) {
						count++
					}
				}
			}
		}
	}
	if count != 1 {
		return fmt.Errorf("pool original outside-air native node has missing/duplicate declaration")
	}
	return nil
}

func (w epathSQLPoolWalk) controllers(name string, coils [][2]string, mixer *idf.Object, outside string) error {
	list, err := w.one("AirLoopHVAC:ControllerList", name)
	if err != nil {
		return err
	}
	wantSize := 1 + 2*len(coils)
	if mixer != nil {
		wantSize += 2
	}
	if len(list.Fields) != wantSize {
		return fmt.Errorf("pool original controller roster differs")
	}
	owners := 0
	for _, object := range w.doc.Objects {
		if (strings.EqualFold(object.Type, "AirLoopHVAC") || strings.EqualFold(object.Type, "AirLoopHVAC:OutdoorAirSystem")) && epathSQLSharedSame(epathSQLSharedField(object, 1), name) {
			owners++
		}
	}
	if owners != 1 {
		return fmt.Errorf("pool original controller list is shared")
	}
	seen := map[string]bool{}
	outdoors := 0
	for at := 1; at < len(list.Fields); at += 2 {
		controller, err := w.one(epathSQLSharedField(list, at), epathSQLSharedField(list, at+1))
		if err != nil {
			return err
		}
		if strings.EqualFold(controller.Type, "Controller:OutdoorAir") {
			outdoors++
			if mixer == nil || !epathSQLSharedSame(epathSQLSharedField(controller, 1), epathSQLSharedField(*mixer, 3)) || !epathSQLSharedSame(epathSQLSharedField(controller, 2), epathSQLSharedField(*mixer, 4)) || !epathSQLSharedSame(epathSQLSharedField(controller, 3), epathSQLSharedField(*mixer, 1)) || !epathSQLSharedSame(epathSQLSharedField(controller, 4), outside) {
				return fmt.Errorf("pool original outdoor controller physical ports differ")
			}
			continue
		}
		if !strings.EqualFold(controller.Type, "Controller:WaterCoil") {
			return fmt.Errorf("pool original unsupported controller type")
		}
		matched := ""
		for _, coil := range coils {
			water, _, err := w.ports(coil[0], coil[1], "water")
			if err != nil {
				return err
			}
			_, air, err := w.ports(coil[0], coil[1], "air")
			if err != nil {
				return err
			}
			if epathSQLSharedSame(epathSQLSharedField(controller, 4), air) && epathSQLSharedSame(epathSQLSharedField(controller, 5), water) {
				if matched != "" {
					return fmt.Errorf("pool original controller matches multiple coils")
				}
				matched = epathSQLSharedKey(coil[1])
				action := "Normal"
				if strings.EqualFold(coil[0], "Coil:Cooling:Water") {
					action = "Reverse"
				}
				if !strings.EqualFold(epathSQLSharedField(controller, 1), "Temperature") || !strings.EqualFold(epathSQLSharedField(controller, 2), action) || !strings.EqualFold(epathSQLSharedField(controller, 3), "Flow") {
					return fmt.Errorf("pool original water-coil control variables/action differ")
				}
			}
		}
		if matched == "" || seen[matched] {
			return fmt.Errorf("pool original water-coil control has missing/duplicate port ownership")
		}
		seen[matched] = true
	}
	wantOA := 0
	if mixer != nil {
		wantOA = 1
	}
	if outdoors != wantOA || len(seen) != len(coils) {
		return fmt.Errorf("pool original controller completeness failed")
	}
	return nil
}
