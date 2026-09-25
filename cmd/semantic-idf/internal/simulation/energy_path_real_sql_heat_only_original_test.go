package simulation

// UNINSTALLED independent oracle draft. Uses only original lexical fields,
// independent typed lookup and existing ADU ownership validation. Never calls
// production HeatOnly resolution, AnalyzeHVAC, allocation, or candidate data.
import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathRealSQLHeatOnlyFurnace struct {
	AirLoopName  string   `json:"airLoopName"`
	FurnaceName  string   `json:"furnaceName"`
	FanName      string   `json:"fanName"`
	FuelCoilName string   `json:"fuelCoilName"`
	ControlZone  string   `json:"controlZone"`
	ServedZones  []string `json:"servedZones"`
}
type epathSQLHeatOnlyOriginalOwner struct {
	ObjectType, ObjectName string
	ObjectIndex            int
}
type epathSQLHeatOnlyFurnaceOriginalProof struct {
	Declaration    epathRealSQLHeatOnlyFurnace
	OriginalSHA256 string
	Owners         []epathSQLHeatOnlyOriginalOwner
	ControlZone    string
	ServedZones    []string
}

func epathSQLValidateHeatOnlyFurnaceOriginal(text string, d epathRealSQLHeatOnlyFurnace) (epathSQLHeatOnlyFurnaceOriginalProof, error) {
	d.ServedZones = append([]string(nil), d.ServedZones...)
	p := epathSQLHeatOnlyFurnaceOriginalProof{Declaration: d, OriginalSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(text)))}
	doc, err := idf.Parse(text)
	if err != nil {
		return p, err
	}
	g := epathSQLSharedOriginal{doc: doc}
	field := epathSQLSharedField
	same := epathSQLSharedSame
	want, err := epathSQLAirLoopFanZones(d.ServedZones)
	if err != nil || len(want) != 3 {
		return p, fmt.Errorf("HeatOnly needs its three independently declared recipients")
	}
	counts := map[string]int{}
	for _, o := range doc.Objects {
		typ := strings.ToLower(strings.TrimSpace(o.Type))
		counts[typ]++
		if strings.HasPrefix(typ, "zonehvac:") && typ != "zonehvac:airdistributionunit" && typ != "zonehvac:equipmentconnections" && typ != "zonehvac:equipmentlist" || strings.HasPrefix(typ, "airterminal:") && typ != "airterminal:singleduct:constantvolume:noreheat" || strings.HasPrefix(typ, "airloophvac:unitary") && typ != "airloophvac:unitary:furnace:heatonly" || strings.HasPrefix(typ, "coilsystem:") || strings.HasPrefix(typ, "airconditioner:") || strings.HasPrefix(typ, "heatpump:") || strings.HasPrefix(typ, "evaporativecooler:") {
			return p, fmt.Errorf("HeatOnly finite original includes an unreviewed delivery/source wrapper")
		}
		if typ == "version" && field(o, 0) != "25.1" {
			return p, fmt.Errorf("HeatOnly oracle native field contract is reviewed for 25.1 only")
		}
		if strings.HasPrefix(typ, "coil:") && typ != "coil:heating:fuel" || strings.HasPrefix(typ, "fan:") && typ != "fan:onoff" || strings.HasPrefix(typ, "chiller:") || strings.HasPrefix(typ, "districtcooling") || typ == "plantloop" {
			return p, fmt.Errorf("HeatOnly finite original includes an unreviewed physical source")
		}
	}
	for typ, count := range map[string]int{"version": 1, "zone": 3, "airloophvac": 1, "airloophvac:unitary:furnace:heatonly": 1, "coil:heating:fuel": 1, "fan:onoff": 1, "branch": 1, "branchlist": 1, "airloophvac:supplypath": 1, "airloophvac:returnpath": 1, "airloophvac:zonesplitter": 1, "airloophvac:zonemixer": 1, "airterminal:singleduct:constantvolume:noreheat": 3, "zonehvac:airdistributionunit": 3, "zonehvac:equipmentconnections": 3, "zonehvac:equipmentlist": 3} {
		if counts[typ] != count {
			return p, fmt.Errorf("HeatOnly original %s census differs", typ)
		}
	}
	air, err := g.one("AirLoopHVAC", d.AirLoopName)
	if err != nil {
		return p, err
	}
	furnace, err := g.one("AirLoopHVAC:Unitary:Furnace:HeatOnly", d.FurnaceName)
	if err != nil {
		return p, err
	}
	fan, err := g.one("Fan:OnOff", d.FanName)
	if err != nil {
		return p, err
	}
	coil, err := g.one("Coil:Heating:Fuel", d.FuelCoilName)
	if err != nil {
		return p, err
	}
	control, err := g.one("Zone", field(furnace, 7))
	if err != nil || !same(field(control, 0), d.ControlZone) {
		return p, fmt.Errorf("HeatOnly control Zone is not an exact original Zone")
	}
	if len(furnace.Fields) != 13 || !same(field(furnace, 8), fan.Type) || !same(field(furnace, 9), field(fan, 0)) || !same(field(furnace, 11), coil.Type) || !same(field(furnace, 12), field(coil, 0)) || !same(field(coil, 2), "NaturalGas") {
		return p, fmt.Errorf("HeatOnly native fan/fuel slots differ")
	}
	for _, node := range []string{field(furnace, 2), field(furnace, 3), field(fan, 7), field(fan, 8), field(coil, 5), field(coil, 6)} {
		if strings.TrimSpace(node) == "" {
			return p, fmt.Errorf("HeatOnly native air port is missing")
		}
	}
	for _, pair := range []struct{ name, inlet, outlet string }{{"wrapper", field(furnace, 2), field(furnace, 3)}, {"fan", field(fan, 7), field(fan, 8)}, {"coil", field(coil, 5), field(coil, 6)}} {
		if same(pair.inlet, pair.outlet) {
			return p, fmt.Errorf("HeatOnly %s native inlet and outlet must be distinct", pair.name)
		}
	}
	placement := strings.ToLower(field(furnace, 10))
	if placement == "" {
		placement = "blowthrough"
	}
	connected := false
	switch placement {
	case "blowthrough":
		connected = same(field(furnace, 2), field(fan, 7)) && same(field(fan, 8), field(coil, 5)) && same(field(coil, 6), field(furnace, 3))
	case "drawthrough":
		connected = same(field(furnace, 2), field(coil, 5)) && same(field(coil, 6), field(fan, 7)) && same(field(fan, 8), field(furnace, 3))
	}
	if !connected {
		return p, fmt.Errorf("HeatOnly internal native fan/coil route is disconnected")
	}
	list, err := g.one("BranchList", field(air, 4))
	if err != nil || len(list.Fields) != 2 {
		return p, fmt.Errorf("HeatOnly requires one exact original main Branch")
	}
	branch, err := g.one("Branch", field(list, 1))
	if err != nil || len(branch.Fields) != 6 {
		return p, fmt.Errorf("HeatOnly main Branch has an unreviewed component")
	}
	if !same(field(branch, 2), furnace.Type) || !same(field(branch, 3), field(furnace, 0)) || !same(field(branch, 4), field(furnace, 2)) || !same(field(branch, 5), field(furnace, 3)) || !same(field(air, 6), field(branch, 4)) || !same(field(air, 9), field(branch, 5)) {
		return p, fmt.Errorf("HeatOnly outer native Branch/AirLoop ports differ")
	}
	// The wrapper, fan and fuel coil have distinct original parents. No shared
	// child or loose name match may justify a broad-meter source boundary.
	for _, child := range []struct {
		object idf.Object
		parent idf.Object
		slot   int
	}{{furnace, branch, 2}, {fan, furnace, 8}, {coil, furnace, 11}} {
		uses := 0
		for _, o := range doc.Objects {
			for slot := 0; slot+1 < len(o.Fields); slot++ {
				if same(field(o, slot), child.object.Type) && same(field(o, slot+1), field(child.object, 0)) {
					uses++
					if o.Index != child.parent.Index || slot != child.slot {
						return p, fmt.Errorf("HeatOnly child is referenced outside its original parent slot")
					}
				}
			}
		}
		if uses != 1 {
			return p, fmt.Errorf("HeatOnly child reference is absent/shared")
		}
	}
	var supply, ret idf.Object
	supplies, returns := 0, 0
	for _, o := range doc.Objects {
		if strings.EqualFold(o.Type, "AirLoopHVAC:SupplyPath") && same(field(o, 1), field(air, 8)) {
			supplies++
			supply = o
		}
		if strings.EqualFold(o.Type, "AirLoopHVAC:ReturnPath") && same(field(o, 1), field(air, 7)) {
			returns++
			ret = o
		}
	}
	if supplies != 1 || returns != 1 || len(supply.Fields) != 4 || len(ret.Fields) != 4 || !same(field(supply, 2), "AirLoopHVAC:ZoneSplitter") || !same(field(ret, 2), "AirLoopHVAC:ZoneMixer") {
		return p, fmt.Errorf("HeatOnly lacks exact supply/return paths")
	}
	split, err := g.one("AirLoopHVAC:ZoneSplitter", field(supply, 3))
	if err != nil {
		return p, err
	}
	mixer, err := g.one("AirLoopHVAC:ZoneMixer", field(ret, 3))
	if err != nil {
		return p, err
	}
	if len(split.Fields) != 5 || len(mixer.Fields) != 5 || !same(field(split, 1), field(supply, 1)) || !same(field(mixer, 1), field(ret, 1)) {
		return p, fmt.Errorf("HeatOnly three-way splitter/mixer port census differs")
	}
	seenZones, returnNodes := map[string]bool{}, map[string]bool{}
	for slot := 2; slot < len(split.Fields); slot++ {
		var terminal idf.Object
		matches := 0
		for _, o := range doc.Objects {
			if strings.EqualFold(o.Type, "AirTerminal:SingleDuct:ConstantVolume:NoReheat") && same(field(o, 2), field(split, slot)) {
				terminal = o
				matches++
			}
		}
		if matches != 1 || strings.TrimSpace(field(terminal, 3)) == "" {
			return p, fmt.Errorf("HeatOnly splitter has no unique native CV/no-reheat terminal")
		}
		if _, err := g.one(terminal.Type, field(terminal, 0)); err != nil {
			return p, err
		}
		// Reuse the generic independent ADU/list/connection/NodeList proof, but
		// not the VAV-specific g.terminal() with incompatible inlet/outlet slots.
		zone, err := g.terminalOwner(terminal, field(terminal, 3))
		if err != nil {
			return p, err
		}
		key := strings.ToLower(zone)
		if seenZones[key] {
			return p, fmt.Errorf("HeatOnly duplicates one served Zone")
		}
		seenZones[key] = true
		connection, err := g.one("ZoneHVAC:EquipmentConnections", zone)
		if err != nil {
			return p, err
		}
		returnNode := strings.ToLower(field(connection, 5))
		if returnNode == "" || returnNodes[returnNode] {
			return p, fmt.Errorf("HeatOnly return ownership is absent/shared")
		}
		returnNodes[returnNode] = true
		p.ServedZones = append(p.ServedZones, zone)
	}
	for slot := 2; slot < len(mixer.Fields); slot++ {
		key := strings.ToLower(field(mixer, slot))
		if !returnNodes[key] {
			return p, fmt.Errorf("HeatOnly mixer has an unowned/duplicate return input")
		}
		delete(returnNodes, key)
	}
	actual, err := epathSQLAirLoopFanZones(p.ServedZones)
	if err != nil || !reflect.DeepEqual(actual, want) || !seenZones[strings.ToLower(d.ControlZone)] || len(returnNodes) != 0 {
		return p, fmt.Errorf("HeatOnly recipients differ from original route; thermostat is not the only recipient")
	}
	p.ControlZone = field(control, 0)
	for _, o := range []idf.Object{furnace, fan, coil, air, branch, list, split, mixer} {
		p.Owners = append(p.Owners, epathSQLHeatOnlyOriginalOwner{o.Type, field(o, 0), o.Index})
	}
	return p, nil
}
