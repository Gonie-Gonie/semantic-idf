package simulation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Independent native 25.1 walk: only the lexical IDF parser is shared with
// production. A SQL equipment key does not itself prove plant or Zone ownership.
type epathSQLSharedOriginal struct{ doc idf.Document }

func epathSQLSharedField(o idf.Object, n int) string {
	if n < 0 || n >= len(o.Fields) {
		return ""
	}
	return strings.TrimSpace(o.Fields[n].Value)
}
func epathSQLSharedKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func (g epathSQLSharedOriginal) one(typ, name string) (idf.Object, error) {
	var found idf.Object
	count := 0
	for _, o := range g.doc.Objects {
		if strings.EqualFold(o.Type, typ) && strings.EqualFold(epathSQLSharedField(o, 0), name) {
			found = o
			count++
		}
	}
	if strings.TrimSpace(name) == "" || count != 1 {
		return found, fmt.Errorf("shared original requires exactly one %s/%s", typ, name)
	}
	return found, nil
}
func epathSQLSharedSame(a, b string) bool { return a != "" && b != "" && strings.EqualFold(a, b) }

func (g epathSQLSharedOriginal) ports(typ, name, medium string) (string, string, error) {
	o, err := g.one(typ, name)
	if err != nil {
		return "", "", err
	}
	in, out := -1, -1
	switch strings.ToLower(typ) {
	case "boiler:hotwater":
		if medium == "water" {
			in, out = 10, 11
		}
	case "coil:heating:water":
		if medium == "water" {
			in, out = 4, 5
		} else {
			in, out = 6, 7
		}
	case "coil:cooling:water":
		if medium == "water" {
			in, out = 9, 10
		} else {
			in, out = 11, 12
		}
	case "pump:variablespeed", "pipe:adiabatic":
		if medium == "water" {
			in, out = 1, 2
		}
	case "fan:variablevolume":
		if medium == "air" {
			in, out = 15, 16
		}
	case "airloophvac:outdoorairsystem":
		if medium == "air" {
			list, e := g.one("AirLoopHVAC:OutdoorAirSystem:EquipmentList", epathSQLSharedField(o, 2))
			if e != nil {
				return "", "", e
			}
			if len(list.Fields) != 3 || !strings.EqualFold(epathSQLSharedField(list, 1), "OutdoorAir:Mixer") {
				return "", "", fmt.Errorf("unreviewed shared supply outdoor-air path")
			}
			mixer, e := g.one("OutdoorAir:Mixer", epathSQLSharedField(list, 2))
			if e != nil {
				return "", "", e
			}
			return epathSQLSharedField(mixer, 4), epathSQLSharedField(mixer, 1), nil
		}
	}
	if in < 0 || epathSQLSharedField(o, in) == "" || epathSQLSharedField(o, out) == "" || epathSQLSharedSame(epathSQLSharedField(o, in), epathSQLSharedField(o, out)) {
		return "", "", fmt.Errorf("unsupported or disconnected shared original %s/%s ports", typ, name)
	}
	return epathSQLSharedField(o, in), epathSQLSharedField(o, out), nil
}

func (g epathSQLSharedOriginal) branch(name, medium string) (idf.Object, error) {
	b, err := g.one("Branch", name)
	if err != nil {
		return b, err
	}
	if len(b.Fields) < 6 || (len(b.Fields)-2)%4 != 0 {
		return b, fmt.Errorf("shared original has an unsupported native Branch shape")
	}
	previous := ""
	components := map[string]bool{}
	for n := 2; n < len(b.Fields); n += 4 {
		typ, name := epathSQLSharedField(b, n), epathSQLSharedField(b, n+1)
		key := epathSQLSharedKey(typ) + "|" + epathSQLSharedKey(name)
		in, out, e := g.ports(typ, name, medium)
		if e != nil {
			return b, e
		}
		if components[key] || !epathSQLSharedSame(in, epathSQLSharedField(b, n+2)) || !epathSQLSharedSame(out, epathSQLSharedField(b, n+3)) || previous != "" && !epathSQLSharedSame(previous, in) {
			return b, fmt.Errorf("shared original Branch has duplicate or disconnected typed component ports")
		}
		components[key], previous = true, out
	}
	return b, nil
}

// Reviewed native parallel branch network. Membership alone is not connection:
// both splitter and mixer must include every internal branch exactly once.
func (g epathSQLSharedOriginal) plantSide(loop idf.Object, offset int) ([]idf.Object, error) {
	listName := epathSQLSharedField(loop, offset+2)
	list, err := g.one("BranchList", listName)
	if err != nil {
		return nil, err
	}
	if len(list.Fields) < 4 {
		return nil, fmt.Errorf("shared original requires the reviewed parallel plant network")
	}
	owners := 0
	for _, o := range g.doc.Objects {
		if strings.EqualFold(o.Type, "PlantLoop") || strings.EqualFold(o.Type, "CondenserLoop") {
			for _, pos := range []int{12, 16} {
				if strings.EqualFold(epathSQLSharedField(o, pos), listName) {
					owners++
				}
			}
		}
	}
	if owners != 1 {
		return nil, fmt.Errorf("shared original plant branch list has ambiguous ownership")
	}
	branches := make([]idf.Object, 0, len(list.Fields)-1)
	names := map[string]bool{}
	for n := 1; n < len(list.Fields); n++ {
		name := epathSQLSharedField(list, n)
		key := epathSQLSharedKey(name)
		if names[key] {
			return nil, fmt.Errorf("shared original repeats a plant branch")
		}
		names[key] = true
		b, e := g.branch(name, "water")
		if e != nil {
			return nil, e
		}
		branches = append(branches, b)
		uses := 0
		for _, o := range g.doc.Objects {
			if strings.EqualFold(o.Type, "BranchList") {
				for p := 1; p < len(o.Fields); p++ {
					if strings.EqualFold(epathSQLSharedField(o, p), name) {
						uses++
					}
				}
			}
		}
		if uses != 1 {
			return nil, fmt.Errorf("shared original plant branch is shared across lists")
		}
	}
	first, last := branches[0], branches[len(branches)-1]
	if !epathSQLSharedSame(epathSQLSharedField(loop, offset), epathSQLSharedField(first, 4)) || !epathSQLSharedSame(epathSQLSharedField(loop, offset+1), epathSQLSharedField(last, len(last.Fields)-1)) {
		return nil, fmt.Errorf("shared original plant outer nodes are disconnected")
	}
	connectors, err := g.one("ConnectorList", epathSQLSharedField(loop, offset+3))
	if err != nil {
		return nil, err
	}
	if len(connectors.Fields) != 5 || !strings.EqualFold(epathSQLSharedField(connectors, 1), "Connector:Splitter") || !strings.EqualFold(epathSQLSharedField(connectors, 3), "Connector:Mixer") {
		return nil, fmt.Errorf("shared original requires exact splitter/mixer connector types")
	}
	for _, spec := range []struct{ typ, name, end string }{{"Connector:Splitter", epathSQLSharedField(connectors, 2), epathSQLSharedField(first, 0)}, {"Connector:Mixer", epathSQLSharedField(connectors, 4), epathSQLSharedField(last, 0)}} {
		c, e := g.one(spec.typ, spec.name)
		if e != nil {
			return nil, e
		}
		if len(c.Fields) != len(branches) || !epathSQLSharedSame(epathSQLSharedField(c, 1), spec.end) {
			return nil, fmt.Errorf("shared original connector has missing outer branch")
		}
		seen := map[string]bool{}
		for n := 2; n < len(c.Fields); n++ {
			k := epathSQLSharedKey(epathSQLSharedField(c, n))
			if !names[k] || seen[k] || k == epathSQLSharedKey(epathSQLSharedField(first, 0)) || k == epathSQLSharedKey(epathSQLSharedField(last, 0)) {
				return nil, fmt.Errorf("shared original connector has missing/duplicate/foreign branch")
			}
			seen[k] = true
		}
	}
	return branches, nil
}

func (g epathSQLSharedOriginal) terminalOwner(terminal idf.Object, outlet string) (string, error) {
	var adu idf.Object
	count := 0
	for _, o := range g.doc.Objects {
		if strings.EqualFold(o.Type, "ZoneHVAC:AirDistributionUnit") && strings.EqualFold(epathSQLSharedField(o, 2), terminal.Type) && strings.EqualFold(epathSQLSharedField(o, 3), epathSQLSharedField(terminal, 0)) {
			adu = o
			count++
		}
	}
	if count != 1 || !epathSQLSharedSame(epathSQLSharedField(adu, 1), outlet) {
		return "", fmt.Errorf("shared original terminal has ambiguous/disconnected ADU")
	}
	if _, err := g.one(adu.Type, epathSQLSharedField(adu, 0)); err != nil {
		return "", err
	}
	var equipment idf.Object
	count = 0
	for _, o := range g.doc.Objects {
		for n := 0; n+1 < len(o.Fields); n++ {
			if strings.EqualFold(epathSQLSharedField(o, n), adu.Type) && strings.EqualFold(epathSQLSharedField(o, n+1), epathSQLSharedField(adu, 0)) {
				if !strings.EqualFold(o.Type, "ZoneHVAC:EquipmentList") || n < 2 || (n-2)%6 != 0 {
					return "", fmt.Errorf("shared ADU has a non-equipment-list reference")
				}
				equipment = o
				count++
			}
		}
	}
	if count != 1 {
		return "", fmt.Errorf("shared original ADU has missing/shared equipment-list ownership")
	}
	if _, err := g.one(equipment.Type, epathSQLSharedField(equipment, 0)); err != nil {
		return "", err
	}
	zone := ""
	count = 0
	for _, o := range g.doc.Objects {
		if strings.EqualFold(o.Type, "ZoneHVAC:EquipmentConnections") && strings.EqualFold(epathSQLSharedField(o, 1), epathSQLSharedField(equipment, 0)) {
			count++
			zone = epathSQLSharedField(o, 0)
			inlet := epathSQLSharedField(o, 2)
			if !epathSQLSharedSame(inlet, outlet) {
				nodes, err := g.one("NodeList", inlet)
				if err != nil {
					return "", err
				}
				matches := 0
				for n := 1; n < len(nodes.Fields); n++ {
					if epathSQLSharedSame(epathSQLSharedField(nodes, n), outlet) {
						matches++
					}
				}
				if matches != 1 {
					return "", fmt.Errorf("shared original ADU outlet is not a unique Zone inlet")
				}
			}
		}
	}
	if count != 1 {
		return "", fmt.Errorf("shared original equipment list has ambiguous Zone connections")
	}
	if _, err := g.one("Zone", zone); err != nil {
		return "", err
	}
	if _, err := g.one("ZoneHVAC:EquipmentConnections", zone); err != nil {
		return "", err
	}
	return zone, nil
}

func (g epathSQLSharedOriginal) terminal(typ, name string) (idf.Object, string, string, error) {
	o, err := g.one(typ, name)
	if err != nil {
		return o, "", "", err
	}
	switch strings.ToLower(typ) {
	case "airterminal:singleduct:vav:noreheat":
		return o, epathSQLSharedField(o, 3), epathSQLSharedField(o, 2), nil
	case "airterminal:singleduct:vav:reheat":
		if !strings.EqualFold(epathSQLSharedField(o, 9), "Coil:Heating:Water") {
			return o, "", "", fmt.Errorf("unreviewed shared terminal reheat type")
		}
		in, out, e := g.ports("Coil:Heating:Water", epathSQLSharedField(o, 10), "air")
		if e != nil {
			return o, "", "", e
		}
		if !epathSQLSharedSame(in, epathSQLSharedField(o, 2)) || !epathSQLSharedSame(out, epathSQLSharedField(o, 13)) {
			return o, "", "", fmt.Errorf("shared original reheat coil has disconnected air ports")
		}
		return o, epathSQLSharedField(o, 3), out, nil
	}
	return o, "", "", fmt.Errorf("unsupported shared original terminal type")
}

func (g epathSQLSharedOriginal) coilZones(coil idf.Object) ([]string, error) {
	// A reheat coil has an exact terminal owner; a central coil has an exact
	// air Branch and its supply splitter independently establishes Zone reach.
	terminalReferences := 0
	for _, o := range g.doc.Objects {
		if strings.EqualFold(o.Type, "AirTerminal:SingleDuct:VAV:Reheat") && strings.EqualFold(epathSQLSharedField(o, 9), coil.Type) && strings.EqualFold(epathSQLSharedField(o, 10), epathSQLSharedField(coil, 0)) {
			terminalReferences++
		}
	}
	if terminalReferences > 1 {
		return nil, fmt.Errorf("shared demand heating coil has ambiguous terminal ownership")
	}
	for _, o := range g.doc.Objects {
		if strings.EqualFold(o.Type, "AirTerminal:SingleDuct:VAV:Reheat") && strings.EqualFold(epathSQLSharedField(o, 9), coil.Type) && strings.EqualFold(epathSQLSharedField(o, 10), epathSQLSharedField(coil, 0)) {
			t, _, out, err := g.terminal(o.Type, epathSQLSharedField(o, 0))
			if err != nil {
				return nil, err
			}
			zone, err := g.terminalOwner(t, out)
			return []string{zone}, err
		}
	}
	var branch idf.Object
	count := 0
	for _, o := range g.doc.Objects {
		if strings.EqualFold(o.Type, "Branch") {
			for n := 2; n+3 < len(o.Fields); n += 4 {
				if strings.EqualFold(epathSQLSharedField(o, n), coil.Type) && strings.EqualFold(epathSQLSharedField(o, n+1), epathSQLSharedField(coil, 0)) && epathSQLSharedSame(epathSQLSharedField(o, n+2), epathSQLSharedField(coil, 6)) && epathSQLSharedSame(epathSQLSharedField(o, n+3), epathSQLSharedField(coil, 7)) {
					branch = o
					count++
				}
			}
		}
	}
	if count != 1 {
		return nil, fmt.Errorf("shared central heating coil lacks one exact air Branch")
	}
	branch, err := g.branch(epathSQLSharedField(branch, 0), "air")
	if err != nil {
		return nil, err
	}
	var air idf.Object
	count = 0
	for _, o := range g.doc.Objects {
		if strings.EqualFold(o.Type, "AirLoopHVAC") {
			list, e := g.one("BranchList", epathSQLSharedField(o, 4))
			if e != nil {
				return nil, e
			}
			for n := 1; n < len(list.Fields); n++ {
				if strings.EqualFold(epathSQLSharedField(list, n), epathSQLSharedField(branch, 0)) {
					if len(list.Fields) != 2 {
						return nil, fmt.Errorf("unreviewed multi-branch shared AirLoop")
					}
					air = o
					count++
				}
			}
		}
	}
	if count != 1 || !epathSQLSharedSame(epathSQLSharedField(air, 6), epathSQLSharedField(branch, 4)) || !epathSQLSharedSame(epathSQLSharedField(air, 9), epathSQLSharedField(branch, len(branch.Fields)-1)) {
		return nil, fmt.Errorf("shared original AirLoop outer ports are disconnected/ambiguous")
	}
	if _, err := g.one(air.Type, epathSQLSharedField(air, 0)); err != nil {
		return nil, err
	}
	return epathSQLHVACOriginalAirLoopZones(g.doc, epathSQLSharedField(air, 0))
}

func epathSQLHVACOriginalAirLoopZones(doc idf.Document, airLoopName string) ([]string, error) {
	g := epathSQLSharedOriginal{doc: doc}
	air, err := g.one("AirLoopHVAC", airLoopName)
	if err != nil {
		return nil, err
	}
	count := 0
	for _, o := range doc.Objects {
		if strings.EqualFold(o.Type, "AirLoopHVAC") && epathSQLSharedSame(epathSQLSharedField(o, 8), epathSQLSharedField(air, 8)) {
			count++
		}
	}
	if count != 1 {
		return nil, fmt.Errorf("shared original AirLoop demand inlet has ambiguous owners")
	}
	var path idf.Object
	count = 0
	for _, o := range g.doc.Objects {
		if strings.EqualFold(o.Type, "AirLoopHVAC:SupplyPath") && epathSQLSharedSame(epathSQLSharedField(o, 1), epathSQLSharedField(air, 8)) {
			path = o
			count++
		}
	}
	if count != 1 || len(path.Fields) != 4 || !strings.EqualFold(epathSQLSharedField(path, 2), "AirLoopHVAC:ZoneSplitter") {
		return nil, fmt.Errorf("shared original AirLoop lacks exact supply splitter path")
	}
	split, err := g.one("AirLoopHVAC:ZoneSplitter", epathSQLSharedField(path, 3))
	if err != nil {
		return nil, err
	}
	count = 0
	for _, o := range doc.Objects {
		if strings.EqualFold(o.Type, "AirLoopHVAC:SupplyPath") {
			for n := 2; n+1 < len(o.Fields); n += 2 {
				if strings.EqualFold(epathSQLSharedField(o, n), "AirLoopHVAC:ZoneSplitter") && strings.EqualFold(epathSQLSharedField(o, n+1), epathSQLSharedField(split, 0)) {
					count++
				}
			}
		}
	}
	if count != 1 {
		return nil, fmt.Errorf("shared original supply splitter has ambiguous path owners")
	}
	if !epathSQLSharedSame(epathSQLSharedField(split, 1), epathSQLSharedField(path, 1)) {
		return nil, fmt.Errorf("shared original supply splitter is disconnected")
	}
	zones := []string{}
	seen := map[string]bool{}
	for n := 2; n < len(split.Fields); n++ {
		node := epathSQLSharedField(split, n)
		var terminal idf.Object
		out := ""
		count = 0
		for _, o := range g.doc.Objects {
			if strings.EqualFold(o.Type, "AirTerminal:SingleDuct:VAV:Reheat") || strings.EqualFold(o.Type, "AirTerminal:SingleDuct:VAV:NoReheat") {
				if !epathSQLSharedSame(epathSQLSharedField(o, 3), node) {
					continue
				}
				var e error
				terminal, _, out, e = g.terminal(o.Type, epathSQLSharedField(o, 0))
				if e != nil {
					return nil, e
				}
				count++
			}
		}
		if count != 1 {
			return nil, fmt.Errorf("shared splitter outlet has no unique typed terminal")
		}
		zone, e := g.terminalOwner(terminal, out)
		if e != nil {
			return nil, e
		}
		key := epathSQLSharedKey(zone)
		if seen[key] {
			return nil, fmt.Errorf("shared splitter repeats a served Zone")
		}
		seen[key] = true
		zones = append(zones, zone)
	}
	return zones, nil
}

func epathSQLValidateHVACConsumptionOriginalModel(text string, pools []epathRealSQLHVACConsumptionPool, executed ...string) error {
	if _, _, err := epathSQLCentralSharedOriginal(text, pools, executed...); err != nil {
		return err
	}
	if len(pools) == 0 {
		return nil
	}
	doc, err := idf.Parse(text)
	if err != nil {
		return err
	}
	g := epathSQLSharedOriginal{doc: doc}
	seen := map[string]bool{}
	for _, pool := range pools {
		if pool.SiteID == "" || len(pool.Shared) == 0 {
			return fmt.Errorf("shared original requires an explicit nonempty site pool")
		}
		for _, member := range pool.Shared {
			if err := epathSQLValidateHVACSharedDeclaration(member); err != nil {
				return err
			}
			if member.ObjectType == epathSQLCentralSharedType {
				continue
			} // Separately proved physical C/H circuit roles above.
			key := epathSQLSharedKey(member.ObjectType) + "|" + epathSQLSharedKey(member.ObjectName)
			if seen[key] {
				return fmt.Errorf("shared original member is reused")
			}
			seen[key] = true
			for _, o := range doc.Objects {
				if strings.EqualFold(o.Type, "Output:Variable") && (epathSQLSharedField(o, 0) == "*" || strings.EqualFold(epathSQLSharedField(o, 0), member.ObjectName)) && (strings.EqualFold(epathSQLSharedField(o, 1), epathSQLSharedBoilerEnergy) || strings.EqualFold(epathSQLSharedField(o, 1), epathSQLSharedBoilerRate)) {
					return fmt.Errorf("shared boiler original output requests need a separately reviewed opener policy; current proof requires injected-only requests")
				}
			}
			boiler, e := g.one(member.ObjectType, member.ObjectName)
			if e != nil {
				return e
			}
			reporting := 0
			for _, o := range doc.Objects {
				if strings.HasPrefix(strings.ToLower(o.Type), "boiler:") && strings.EqualFold(epathSQLSharedField(o, 0), member.ObjectName) {
					reporting++
				}
			}
			if reporting != 1 {
				return fmt.Errorf("shared boiler reporting key collides across types")
			}
			loop, e := g.one("PlantLoop", member.PlantLoopName)
			if e != nil {
				return e
			}
			supply, e := g.plantSide(loop, 10)
			if e != nil {
				return e
			}
			demand, e := g.plantSide(loop, 14)
			if e != nil {
				return e
			}
			count := 0
			global := 0
			for _, o := range doc.Objects {
				if strings.EqualFold(o.Type, "Branch") {
					for n := 2; n+1 < len(o.Fields); n += 4 {
						if strings.EqualFold(epathSQLSharedField(o, n), member.ObjectType) && strings.EqualFold(epathSQLSharedField(o, n+1), member.ObjectName) {
							global++
						}
					}
				}
			}
			for _, b := range supply {
				for n := 2; n < len(b.Fields); n += 4 {
					if strings.EqualFold(epathSQLSharedField(b, n), boiler.Type) && strings.EqualFold(epathSQLSharedField(b, n+1), member.ObjectName) {
						count++
					}
				}
			}
			if count != 1 || global != 1 {
				return fmt.Errorf("shared boiler does not have one exact supply Branch owner")
			}
			served := map[string]bool{}
			coils := 0
			for _, b := range demand {
				for n := 2; n < len(b.Fields); n += 4 {
					if !strings.EqualFold(epathSQLSharedField(b, n), "Coil:Heating:Water") {
						continue
					}
					coil, e := g.one("Coil:Heating:Water", epathSQLSharedField(b, n+1))
					if e != nil {
						return e
					}
					zones, e := g.coilZones(coil)
					if e != nil {
						return e
					}
					coils++
					for _, zone := range zones {
						served[epathSQLSharedKey(zone)] = true
					}
				}
			}
			actual, want := []string{}, []string{}
			for zone := range served {
				actual = append(actual, zone)
			}
			for _, zone := range member.ServedZones {
				want = append(want, epathSQLSharedKey(zone))
			}
			sort.Strings(actual)
			sort.Strings(want)
			if coils == 0 || !reflect.DeepEqual(actual, want) {
				return fmt.Errorf("shared original plant served Zones differ: original=%v declared=%v", actual, want)
			}
		}
	}
	return nil
}
