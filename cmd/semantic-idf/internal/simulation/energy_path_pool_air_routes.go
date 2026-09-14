package simulation

// Native 25.1 topology proof for the reviewed single-branch air
// architecture. Names, output values and AnalyzeHVAC are not ownership proof.
// The broader water inventory remains authoritative for all competing demands.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type energyPathPoolAirRoute struct {
	Coil                      idf.ComponentRef
	PlantLoop                 idf.ComponentRef
	AirLoop                   idf.ComponentRef
	ServiceKind               string
	Position                  string // outside_air, main_branch, terminal_reheat
	ServedZoneNames           []string
	Deliveries                []energyPathPoolAirDelivery
	Trace                     []idf.ComponentRef
	Complete                  bool
	RelatedPathIDs            []string
	NavigationAnchor          idf.ComponentRef
	NavigationAnchorPlantLoop idf.ComponentRef
	NavigationBindings        []energyPathPoolAirNavigationBinding
	Issues                    []string
}

type energyPathPoolAirNavigationBinding struct {
	ZoneName string
	PathIDs  []string
	Mode     string // exact_coil, proved_oa_via_same_loop_main_coil
	Anchor   idf.ComponentRef
}

type energyPathPoolAirDelivery struct {
	Zone          idf.ComponentRef
	Connection    idf.ComponentRef
	EquipmentList idf.ComponentRef
	ADU           idf.ComponentRef
	Terminal      idf.ComponentRef
	ReheatCoil    idf.ComponentRef
	ZoneAirNode   string
	ReturnNode    string
}

type energyPathPoolAirProof struct {
	AirLoop idf.ComponentRef
	Routes  []energyPathPoolAirRoute
	Issues  []string
}

// A separate late binder may attach existing path IDs. Its absence cannot
// change independently proved ownership, nor may a supplied path repair an
// incomplete original route. No caller-owned water/source evidence is erased.
func applyEnergyPathPoolAirRoutes(doc idf.Document, inventory *energyPathPoolInventory) []energyPathPoolAirRoute {
	if inventory == nil || !inventory.HasNativePool {
		return nil
	}
	for loop := range inventory.Loops {
		inventory.Loops[loop].AirRoutesComplete = false
		for demand := range inventory.Loops[loop].Demands {
			inventory.Loops[loop].Demands[demand].AirRouteComplete = false
			inventory.Loops[loop].Demands[demand].RelatedPathIDs = nil
		}
	}
	if !inventory.SchemaReviewed {
		return nil
	}
	index := newEnergyPathPoolIndex(doc)
	byCoil := map[int][]energyPathPoolAirRoute{}
	var proofIssues []string
	for _, air := range doc.Objects {
		if !energyPathPoolEqual(air.Type, "AirLoopHVAC") {
			continue
		}
		proof := index.poolAirProof(air)
		proofIssues = append(proofIssues, proof.Issues...)
		if len(proof.Issues) != 0 {
			continue
		}
		for _, route := range proof.Routes {
			byCoil[route.Coil.ObjectIndex] = append(byCoil[route.Coil.ObjectIndex], route)
		}
	}
	var routes []energyPathPoolAirRoute
	for loopIndex := range inventory.Loops {
		loop := &inventory.Loops[loopIndex]
		complete := loop.WaterTopologyComplete && loop.DemandRosterComplete && len(loop.Demands) > 0 && !loop.HasNonZoneDemand
		airID := -1
		var union []string
		for demandIndex := range loop.Demands {
			demand := &loop.Demands[demandIndex]
			if demand.Kind != "heating_coil" && demand.Kind != "cooling_coil" {
				complete = false
				continue // pool never becomes a fake ZoneHVAC delivery
			}
			route := energyPathPoolAirRoute{Coil: demand.Water.Component, PlantLoop: loop.Component, ServiceKind: strings.TrimSuffix(demand.Kind, "_coil")}
			found := byCoil[demand.Water.Component.ObjectIndex]
			if len(found) == 1 && demand.Water.IdentityValid && demand.Water.WaterOwnerValid &&
				energyPathPoolAirSameRef(found[0].Coil, demand.Water.Component) && found[0].ServiceKind == route.ServiceKind {
				route = found[0]
				route.PlantLoop = loop.Component
				demand.AirRouteComplete = true
				if airID == -1 {
					airID = route.AirLoop.ObjectIndex
				} else if airID != route.AirLoop.ObjectIndex {
					complete = false
				}
				union = energyPathPoolAirUnion(union, route.ServedZoneNames...)
			} else {
				route.Issues = append(route.Issues, "no unique complete native air route for this original water demand")
				route.Issues = append(route.Issues, proofIssues...)
				complete = false
			}
			routes = append(routes, route)
		}
		// Different central/terminal coil recipient sets may legitimately form
		// the complete union. No PLENUM recipient is added from native load values.
		loop.AirRoutesComplete = complete && airID >= 0 && len(union) > 0
	}
	return routes
}

func energyPathPoolAirSameRef(a, b idf.ComponentRef) bool {
	return a.ObjectIndex == b.ObjectIndex && a.ID == b.ID && energyPathPoolEqual(a.ObjectType, b.ObjectType) && energyPathPoolEqual(a.ObjectName, b.ObjectName)
}

func energyPathPoolAirUnion(values []string, next ...string) []string {
	out := append([]string(nil), values...)
	for _, value := range next {
		found := false
		for _, previous := range out {
			found = found || energyPathPoolEqual(previous, value)
		}
		if !found && strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return energyPathPoolToken(out[i]) < energyPathPoolToken(out[j]) })
	return out
}

func energyPathPoolAirDistinct(nodes ...string) bool {
	seen := map[string]bool{}
	for _, node := range nodes {
		key := energyPathPoolToken(node)
		if key == "" || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func (index energyPathPoolIndex) poolAirSelector(value string) ([]string, bool) {
	if strings.TrimSpace(value) == "" {
		return nil, false
	}
	positions := index.objects[energyPathPoolKey("NodeList", value)]
	if len(positions) == 0 {
		return []string{strings.TrimSpace(value)}, true
	}
	if len(positions) != 1 {
		return nil, false
	}
	list := index.doc.Objects[positions[0]]
	var nodes []string
	for field := 1; field < len(list.Fields); field++ {
		node := energyPathPoolField(list, field)
		if node == "" {
			continue
		}
		if len(index.objects[energyPathPoolKey("NodeList", node)]) != 0 {
			return nil, false
		}
		nodes = append(nodes, node)
	}
	return nodes, len(nodes) > 0 && energyPathPoolAirDistinct(nodes...)
}

func (index energyPathPoolIndex) poolAirOneNode(selector string) (string, bool) {
	nodes, ok := index.poolAirSelector(selector)
	if !ok || len(nodes) != 1 {
		return "", false
	}
	return nodes[0], true
}

func (index energyPathPoolIndex) poolAirSelectorMentions(selector, node string) bool {
	if strings.TrimSpace(selector) == "" || strings.TrimSpace(node) == "" {
		return false
	}
	if energyPathPoolEqual(selector, node) {
		return true
	}
	for _, position := range index.objects[energyPathPoolKey("NodeList", selector)] {
		for field := 1; field < len(index.doc.Objects[position].Fields); field++ {
			if energyPathPoolEqual(energyPathPoolField(index.doc.Objects[position], field), node) {
				return true
			}
		}
	}
	return false
}

func (index energyPathPoolIndex) poolAirOutdoorNode(node string) bool {
	// Native OutAirNodeManager unions OutdoorAir:NodeList selectors; a single
	// OutdoorAir:Node may expand a one-member NodeList but must not overlap.
	listed, singles := false, 0
	for _, object := range index.doc.Objects {
		switch energyPathPoolToken(object.Type) {
		case "outdoorair:nodelist":
			if len(object.Fields) == 0 {
				return false
			}
			for _, field := range object.Fields {
				if strings.TrimSpace(field.Value) == "" {
					continue
				}
				nodes, ok := index.poolAirSelector(field.Value)
				if !ok {
					return false
				}
				for _, expanded := range nodes {
					listed = listed || energyPathPoolEqual(expanded, node)
				}
			}
		case "outdoorair:node":
			expanded, ok := index.poolAirOneNode(energyPathPoolField(object, 0))
			if !ok {
				return false
			}
			if energyPathPoolEqual(expanded, node) {
				singles++
			}
		}
	}
	return listed && singles == 0 || !listed && singles == 1
}

func energyPathPoolAirPorts(object idf.Object) (string, string, bool) {
	a, b := -1, -1
	switch energyPathPoolToken(object.Type) {
	case "coil:heating:water":
		a, b = 6, 7
	case "coil:cooling:water":
		a, b = 11, 12
	case "fan:variablevolume":
		a, b = 15, 16
	}
	inlet, outlet := energyPathPoolField(object, a), energyPathPoolField(object, b)
	return inlet, outlet, energyPathPoolAirDistinct(inlet, outlet)
}

// Raw adjacent type/name claims count even at a malformed owner slot. This is
// a deny-only ambiguity check; only native validated slots authorize a route.
func (index energyPathPoolIndex) poolAirTypedClaims(target idf.Object) int {
	count := 0
	for _, owner := range index.doc.Objects {
		for field := 0; field+1 < len(owner.Fields); field++ {
			if energyPathPoolEqual(energyPathPoolField(owner, field), target.Type) && energyPathPoolEqual(energyPathPoolField(owner, field+1), energyPathPoolField(target, 0)) {
				count++
			}
		}
	}
	return count
}

func (index energyPathPoolIndex) poolAirUnique(target idf.Object) bool {
	one, ok := index.unique(target.Type, energyPathPoolField(target, 0))
	return ok && one.Index == target.Index
}

func (index energyPathPoolIndex) poolAirProof(air idf.Object) energyPathPoolAirProof {
	proof := energyPathPoolAirProof{AirLoop: energyPathPoolRef(air)}
	fail := func(reason string) energyPathPoolAirProof {
		proof.Routes = nil
		proof.Issues = []string{reason}
		return proof
	}
	if !index.poolAirUnique(air) || energyPathPoolField(air, 5) != "" {
		return fail("ambiguous AirLoop or unreviewed connector architecture")
	}
	supplyIn := energyPathPoolField(air, 6)
	returnOut := energyPathPoolField(air, 7)
	demandIn, demandOK := index.poolAirOneNode(energyPathPoolField(air, 8))
	supplyOut, supplyOK := index.poolAirOneNode(energyPathPoolField(air, 9))
	if !demandOK || !supplyOK || !energyPathPoolAirDistinct(supplyIn, returnOut, demandIn, supplyOut) {
		return fail("missing, duplicated or unreviewed AirLoop outer port selectors")
	}
	for _, other := range index.doc.Objects {
		if !energyPathPoolEqual(other.Type, "AirLoopHVAC") || other.Index == air.Index {
			continue
		}
		for _, field := range []int{6, 7, 8, 9} {
			for _, node := range []string{supplyIn, returnOut, demandIn, supplyOut} {
				if index.poolAirSelectorMentions(energyPathPoolField(other, field), node) {
					return fail("AirLoop outer node has competing original owner")
				}
			}
		}
	}
	list, ok := index.unique("BranchList", energyPathPoolField(air, 4))
	if !ok || len(list.Fields) != 2 {
		return fail("missing or unreviewed multi-branch AirLoop")
	}
	branch, ok := index.unique("Branch", energyPathPoolField(list, 1))
	if !ok || len(branch.Fields) != 18 {
		return fail("main branch is not the reviewed four-component sequence")
	}
	owner, _, owned := index.branchOwner(branch)
	if !owned || owner.Index != air.Index || !energyPathPoolEqual(owner.Type, air.Type) {
		return fail("main branch ownership is not unique")
	}
	types := []string{"AirLoopHVAC:OutdoorAirSystem", "Coil:Cooling:Water", "Coil:Heating:Water", "Fan:VariableVolume"}
	components := make([]idf.Object, 4)
	previous := supplyIn
	for slot, kind := range types {
		field := 2 + slot*4
		if !energyPathPoolEqual(energyPathPoolField(branch, field), kind) {
			return fail("unreviewed main branch component or sequence")
		}
		component, found := index.unique(kind, energyPathPoolField(branch, field+1))
		if !found || !energyPathPoolEqual(energyPathPoolField(branch, field+2), previous) {
			return fail("main branch missing typed component or broken port continuity")
		}
		if slot > 0 {
			inlet, outlet, portsOK := energyPathPoolAirPorts(component)
			if !portsOK || !energyPathPoolEqual(inlet, energyPathPoolField(branch, field+2)) || !energyPathPoolEqual(outlet, energyPathPoolField(branch, field+3)) {
				return fail("main component native air ports do not match Branch ports")
			}
		}
		components[slot] = component
		previous = energyPathPoolField(branch, field+3)
	}
	if !energyPathPoolEqual(previous, supplyOut) {
		return fail("fan outlet disconnected from AirLoop supply outlet")
	}
	if index.poolAirTypedClaims(components[0]) != 1 || index.poolAirTypedClaims(components[3]) != 1 {
		return fail("duplicate OA wrapper or fan ownership")
	}
	for _, coil := range components[1:3] {
		if index.poolAirTypedClaims(coil) != 2 {
			return fail("central coil must have exactly one typed air and one typed water branch owner")
		}
		_, _, side, waterOK := index.waterOwner(coil)
		if !waterOK || side != "demand" {
			return fail("central coil lacks exact independent water demand owner")
		}
	}
	oa := components[0]
	oaList, ok := index.unique("AirLoopHVAC:OutdoorAirSystem:EquipmentList", energyPathPoolField(oa, 2))
	if !ok || len(oaList.Fields) != 7 {
		return fail("OA equipment list must contain heating, cooling and mixer only")
	}
	for _, other := range index.doc.Objects {
		if energyPathPoolEqual(other.Type, oa.Type) && other.Index != oa.Index && energyPathPoolEqual(energyPathPoolField(other, 2), energyPathPoolField(oaList, 0)) {
			return fail("OA equipment list has competing wrapper owner")
		}
	}
	oaTypes := []string{"Coil:Heating:Water", "Coil:Cooling:Water", "OutdoorAir:Mixer"}
	oaComponents := make([]idf.Object, 3)
	for slot, kind := range oaTypes {
		field := 1 + 2*slot
		if !energyPathPoolEqual(energyPathPoolField(oaList, field), kind) {
			return fail("OA components are not in native heating/cooling/mixer simulation order")
		}
		component, found := index.unique(kind, energyPathPoolField(oaList, field+1))
		if !found {
			return fail("OA component typed identity missing or ambiguous")
		}
		oaComponents[slot] = component
		if slot < 2 {
			if index.poolAirTypedClaims(component) != 2 {
				return fail("OA coil has competing or missing typed air/water owner")
			}
			_, _, side, waterOK := index.waterOwner(component)
			if !waterOK || side != "demand" {
				return fail("OA coil lacks exact water demand owner")
			}
		} else if index.poolAirTypedClaims(component) != 1 {
			return fail("OA mixer has competing owner")
		}
	}
	hIn, hOut, hOK := energyPathPoolAirPorts(oaComponents[0])
	cIn, cOut, cOK := energyPathPoolAirPorts(oaComponents[1])
	mixer := oaComponents[2]
	mixed, outdoor, relief, returned := energyPathPoolField(mixer, 1), energyPathPoolField(mixer, 2), energyPathPoolField(mixer, 3), energyPathPoolField(mixer, 4)
	if !hOK || !cOK || !energyPathPoolAirDistinct(hIn, hOut, cOut, mixed, relief, returned) ||
		!energyPathPoolEqual(hOut, cIn) || !energyPathPoolEqual(cOut, outdoor) || !energyPathPoolEqual(returned, supplyIn) ||
		!energyPathPoolEqual(mixed, energyPathPoolField(branch, 5)) || !index.poolAirOutdoorNode(hIn) {
		return fail("OA native outside/pretreat/mixer/return ports are incomplete")
	}
	mainControllers, err := index.poolAirControllers(air, []idf.Object{components[1], components[2]}, nil, "")
	if err != nil {
		return fail(err.Error())
	}
	oaControllers, err := index.poolAirControllers(oa, []idf.Object{oaComponents[0], oaComponents[1]}, &mixer, hIn)
	if err != nil {
		return fail(err.Error())
	}
	deliveries, supplyTrace, err := index.poolAirDeliveries(air, demandIn, returnOut)
	if err != nil {
		return fail(err.Error())
	}
	var trace []idf.ComponentRef
	for _, object := range append(append([]idf.Object{air, list, branch, oaList}, components...), oaComponents...) {
		trace = append(trace, energyPathPoolRef(object))
	}
	trace = append(trace, mainControllers...)
	trace = append(trace, oaControllers...)
	trace = append(trace, supplyTrace...)
	var zoneNames []string
	for _, delivery := range deliveries {
		zoneNames = append(zoneNames, delivery.Zone.ObjectName)
	}
	zoneNames = energyPathPoolAirUnion(nil, zoneNames...)
	for _, item := range []struct {
		coil     idf.Object
		position string
	}{{components[1], "main_branch"}, {components[2], "main_branch"}, {oaComponents[0], "outside_air"}, {oaComponents[1], "outside_air"}} {
		service := "heating"
		if energyPathPoolEqual(item.coil.Type, "Coil:Cooling:Water") {
			service = "cooling"
		}
		route := energyPathPoolAirRoute{Coil: energyPathPoolRef(item.coil), AirLoop: proof.AirLoop, ServiceKind: service, Position: item.position, ServedZoneNames: append([]string(nil), zoneNames...), Deliveries: append([]energyPathPoolAirDelivery(nil), deliveries...), Trace: append([]idf.ComponentRef(nil), trace...), Complete: true}
		if item.position == "outside_air" {
			anchor := components[2]
			if service == "cooling" {
				anchor = components[1]
			}
			anchorLoop, _, side, owned := index.waterOwner(anchor)
			if owned && side == "demand" {
				route.NavigationAnchor = energyPathPoolRef(anchor)
				route.NavigationAnchorPlantLoop = anchorLoop
			}
		}
		proof.Routes = append(proof.Routes, route)
	}
	for _, delivery := range deliveries {
		proof.Routes = append(proof.Routes, energyPathPoolAirRoute{Coil: delivery.ReheatCoil, AirLoop: proof.AirLoop, ServiceKind: "heating", Position: "terminal_reheat", ServedZoneNames: []string{delivery.Zone.ObjectName}, Deliveries: []energyPathPoolAirDelivery{delivery}, Trace: append([]idf.ComponentRef(nil), trace...), Complete: true})
	}
	return proof
}

func (index energyPathPoolIndex) poolAirControllers(owner idf.Object, coils []idf.Object, mixer *idf.Object, outside string) ([]idf.ComponentRef, error) {
	list, ok := index.unique("AirLoopHVAC:ControllerList", energyPathPoolField(owner, 1))
	expected := len(coils)
	if mixer != nil {
		expected++
	}
	if !ok || len(list.Fields) != 1+2*expected {
		return nil, fmt.Errorf("missing or extra native coil/OA controllers")
	}
	owners := 0
	for _, object := range index.doc.Objects {
		if (energyPathPoolEqual(object.Type, "AirLoopHVAC") || energyPathPoolEqual(object.Type, "AirLoopHVAC:OutdoorAirSystem")) && energyPathPoolEqual(energyPathPoolField(object, 1), energyPathPoolField(list, 0)) {
			owners++
		}
	}
	if owners != 1 {
		return nil, fmt.Errorf("controller list has competing owners")
	}
	seenCoils := map[int]bool{}
	oaCount := 0
	refs := []idf.ComponentRef{energyPathPoolRef(list)}
	for field := 1; field+1 < len(list.Fields); field += 2 {
		controller, found := index.unique(energyPathPoolField(list, field), energyPathPoolField(list, field+1))
		if !found || index.poolAirTypedClaims(controller) != 1 {
			return nil, fmt.Errorf("controller typed identity/ownership ambiguous")
		}
		switch energyPathPoolToken(controller.Type) {
		case "controller:watercoil":
			matched := -1
			for _, coil := range coils {
				_, airOut, airOK := energyPathPoolAirPorts(coil)
				waterIn, _, waterOK := energyPathPoolWaterPorts(coil)
				if airOK && waterOK && energyPathPoolEqual(energyPathPoolField(controller, 4), airOut) && energyPathPoolEqual(energyPathPoolField(controller, 5), waterIn) {
					if matched != -1 {
						return nil, fmt.Errorf("water controller matches multiple original coils")
					}
					matched = coil.Index
					action := energyPathPoolField(controller, 2)
					want := "Normal"
					if energyPathPoolEqual(coil.Type, "Coil:Cooling:Water") {
						want = "Reverse"
					}
					if action != "" && !energyPathPoolEqual(action, want) {
						return nil, fmt.Errorf("water controller action conflicts with native coil")
					}
				}
			}
			if matched == -1 || seenCoils[matched] || !energyPathPoolEqual(energyPathPoolField(controller, 1), "Temperature") || !energyPathPoolEqual(energyPathPoolField(controller, 3), "Flow") {
				return nil, fmt.Errorf("coil controller sensor/actuator/role incomplete")
			}
			seenCoils[matched] = true
			for _, peer := range index.doc.Objects {
				if energyPathPoolEqual(peer.Type, controller.Type) && peer.Index != controller.Index && energyPathPoolEqual(energyPathPoolField(peer, 5), energyPathPoolField(controller, 5)) {
					return nil, fmt.Errorf("coil water actuator has competing controller")
				}
			}
		case "controller:outdoorair":
			if mixer == nil {
				return nil, fmt.Errorf("unexpected outdoor controller on main coil controller list")
			}
			oaCount++
			for _, ports := range [][2]int{{1, 3}, {2, 4}, {3, 1}} {
				if !energyPathPoolEqual(energyPathPoolField(controller, ports[0]), energyPathPoolField(*mixer, ports[1])) {
					return nil, fmt.Errorf("OA controller return/relief/mixed ports do not match mixer")
				}
			}
			if !energyPathPoolEqual(energyPathPoolField(controller, 4), outside) {
				return nil, fmt.Errorf("OA controller actuator is not first pretreat outside inlet")
			}
			for _, peer := range index.doc.Objects {
				if !energyPathPoolEqual(peer.Type, controller.Type) || peer.Index == controller.Index {
					continue
				}
				for _, field := range []int{1, 2, 3, 4} {
					if energyPathPoolEqual(energyPathPoolField(peer, field), energyPathPoolField(controller, field)) {
						return nil, fmt.Errorf("OA controller port has competing controller owner")
					}
				}
			}
		default:
			return nil, fmt.Errorf("unreviewed air controller type")
		}
		refs = append(refs, energyPathPoolRef(controller))
	}
	if len(seenCoils) != len(coils) || (mixer == nil && oaCount != 0) || (mixer != nil && oaCount != 1) {
		return nil, fmt.Errorf("controller roster incomplete")
	}
	return refs, nil
}

func (index energyPathPoolIndex) poolAirDeliveries(air idf.Object, demandIn, returnOut string) ([]energyPathPoolAirDelivery, []idf.ComponentRef, error) {
	var supply, returned idf.Object
	supplies, returns := 0, 0
	for _, object := range index.doc.Objects {
		if energyPathPoolEqual(object.Type, "AirLoopHVAC:SupplyPath") && energyPathPoolEqual(energyPathPoolField(object, 1), demandIn) {
			supply = object
			supplies++
		}
		if energyPathPoolEqual(object.Type, "AirLoopHVAC:ReturnPath") && energyPathPoolEqual(energyPathPoolField(object, 1), returnOut) {
			returned = object
			returns++
		}
	}
	if supplies != 1 || returns != 1 || !index.poolAirUnique(supply) || !index.poolAirUnique(returned) || len(supply.Fields) != 4 || len(returned.Fields) != 4 ||
		!energyPathPoolEqual(energyPathPoolField(supply, 2), "AirLoopHVAC:ZoneSplitter") || !energyPathPoolEqual(energyPathPoolField(returned, 2), "AirLoopHVAC:ReturnPlenum") {
		return nil, nil, fmt.Errorf("exact single splitter/return-plenum paths missing or ambiguous")
	}
	splitter, splitOK := index.unique(energyPathPoolField(supply, 2), energyPathPoolField(supply, 3))
	plenum, plenumOK := index.unique(energyPathPoolField(returned, 2), energyPathPoolField(returned, 3))
	if !splitOK || !plenumOK || index.poolAirTypedClaims(splitter) != 1 || index.poolAirTypedClaims(plenum) != 1 ||
		!energyPathPoolEqual(energyPathPoolField(splitter, 1), demandIn) || !energyPathPoolEqual(energyPathPoolField(plenum, 3), returnOut) || energyPathPoolField(plenum, 4) != "" {
		return nil, nil, fmt.Errorf("splitter/return-plenum native outer port or ownership failure")
	}
	plenumZone, ok := index.unique("Zone", energyPathPoolField(plenum, 1))
	if !ok || !energyPathPoolAirDistinct(energyPathPoolField(plenum, 2), returnOut) {
		return nil, nil, fmt.Errorf("return-plenum Zone or native Zone node missing")
	}
	for _, object := range index.doc.Objects {
		if energyPathPoolEqual(object.Type, "ZoneHVAC:EquipmentConnections") && energyPathPoolEqual(energyPathPoolField(object, 0), energyPathPoolField(plenumZone, 0)) {
			return nil, nil, fmt.Errorf("return plenum cannot become an equipment-served recipient")
		}
		if energyPathPoolEqual(object.Type, plenum.Type) && object.Index != plenum.Index && (energyPathPoolEqual(energyPathPoolField(object, 1), energyPathPoolField(plenum, 1)) || energyPathPoolEqual(energyPathPoolField(object, 2), energyPathPoolField(plenum, 2)) || energyPathPoolEqual(energyPathPoolField(object, 3), returnOut)) {
			return nil, nil, fmt.Errorf("return plenum has competing Zone/node owner")
		}
	}
	// Finite reviewed architecture: five complete recipient routes. This is a
	// structural cardinality, not a Zone-name whitelist or a value-based roster.
	if len(splitter.Fields) != 7 || len(plenum.Fields) != 10 {
		return nil, nil, fmt.Errorf("unreviewed splitter/return-plenum recipient count")
	}
	var outlets, returnNodes []string
	for field := 2; field < len(splitter.Fields); field++ {
		outlets = append(outlets, energyPathPoolField(splitter, field))
	}
	for field := 5; field < len(plenum.Fields); field++ {
		returnNodes = append(returnNodes, energyPathPoolField(plenum, field))
	}
	if !energyPathPoolAirDistinct(outlets...) || !energyPathPoolAirDistinct(returnNodes...) {
		return nil, nil, fmt.Errorf("duplicate or empty splitter/return-plenum member")
	}
	var deliveries []energyPathPoolAirDelivery
	seenZones, seenReturns := map[string]bool{}, map[string]bool{}
	trace := []idf.ComponentRef{energyPathPoolRef(supply), energyPathPoolRef(splitter), energyPathPoolRef(returned), energyPathPoolRef(plenum), energyPathPoolRef(plenumZone)}
	for _, outlet := range outlets {
		var terminal idf.Object
		count := 0
		for _, object := range index.doc.Objects {
			if energyPathPoolEqual(object.Type, "AirTerminal:SingleDuct:VAV:Reheat") && energyPathPoolEqual(energyPathPoolField(object, 3), outlet) {
				terminal = object
				count++
			}
		}
		if count != 1 || !index.poolAirUnique(terminal) || index.poolAirTypedClaims(terminal) != 1 {
			return nil, nil, fmt.Errorf("splitter outlet lacks a unique native terminal/ADU")
		}
		if !energyPathPoolEqual(energyPathPoolField(terminal, 9), "Coil:Heating:Water") {
			return nil, nil, fmt.Errorf("unreviewed terminal reheat coil type")
		}
		coil, ok := index.unique(energyPathPoolField(terminal, 9), energyPathPoolField(terminal, 10))
		if !ok || index.poolAirTypedClaims(coil) != 2 {
			return nil, nil, fmt.Errorf("terminal reheat coil has ambiguous air/water ownership")
		}
		inlet, coilOut, portsOK := energyPathPoolAirPorts(coil)
		_, _, side, waterOK := index.waterOwner(coil)
		if !portsOK || !waterOK || side != "demand" || !energyPathPoolEqual(inlet, energyPathPoolField(terminal, 2)) || !energyPathPoolEqual(coilOut, energyPathPoolField(terminal, 13)) || !energyPathPoolAirDistinct(outlet, inlet, coilOut) {
			return nil, nil, fmt.Errorf("terminal/reheat coil native air or water port failure")
		}
		delivery, err := index.poolAirDeliveryOwner(terminal, coil, coilOut)
		if err != nil {
			return nil, nil, err
		}
		zoneKey, returnKey := energyPathPoolToken(delivery.Zone.ObjectName), energyPathPoolToken(delivery.ReturnNode)
		if seenZones[zoneKey] || seenReturns[returnKey] || energyPathPoolEqual(delivery.Zone.ObjectName, energyPathPoolField(plenumZone, 0)) {
			return nil, nil, fmt.Errorf("duplicate recipient Zone/return node or plenum included as recipient")
		}
		foundReturn := 0
		for _, node := range returnNodes {
			if energyPathPoolEqual(node, delivery.ReturnNode) {
				foundReturn++
			}
		}
		if foundReturn != 1 {
			return nil, nil, fmt.Errorf("Zone return is disconnected from exact return plenum")
		}
		for _, peer := range index.doc.Objects {
			if energyPathPoolEqual(peer.Type, "AirLoopHVAC:ZoneSplitter") && peer.Index != splitter.Index {
				for field := 2; field < len(peer.Fields); field++ {
					if energyPathPoolEqual(energyPathPoolField(peer, field), outlet) {
						return nil, nil, fmt.Errorf("terminal inlet has competing splitter owner")
					}
				}
			}
			if energyPathPoolEqual(peer.Type, "AirLoopHVAC:ReturnPlenum") && peer.Index != plenum.Index {
				for field := 5; field < len(peer.Fields); field++ {
					if energyPathPoolEqual(energyPathPoolField(peer, field), delivery.ReturnNode) {
						return nil, nil, fmt.Errorf("Zone return has competing return plenum owner")
					}
				}
			}
		}
		seenZones[zoneKey], seenReturns[returnKey] = true, true
		deliveries = append(deliveries, delivery)
		trace = append(trace, delivery.Zone, delivery.Connection, delivery.EquipmentList, delivery.ADU, delivery.Terminal, delivery.ReheatCoil)
	}
	if len(seenReturns) != len(returnNodes) {
		return nil, nil, fmt.Errorf("return-plenum union incomplete")
	}
	return deliveries, trace, nil
}

func (index energyPathPoolIndex) poolAirDeliveryOwner(terminal, coil idf.Object, outlet string) (energyPathPoolAirDelivery, error) {
	fail := func(message string) (energyPathPoolAirDelivery, error) {
		return energyPathPoolAirDelivery{}, fmt.Errorf("%s", message)
	}
	var adu, list, connection idf.Object
	adus, lists, connections := 0, 0, 0
	for _, object := range index.doc.Objects {
		if energyPathPoolEqual(object.Type, "ZoneHVAC:AirDistributionUnit") && energyPathPoolEqual(energyPathPoolField(object, 2), terminal.Type) && energyPathPoolEqual(energyPathPoolField(object, 3), energyPathPoolField(terminal, 0)) {
			adu = object
			adus++
		}
	}
	if adus != 1 || !index.poolAirUnique(adu) || index.poolAirTypedClaims(adu) != 1 || !energyPathPoolEqual(energyPathPoolField(adu, 1), outlet) {
		return fail("native ADU owner/outlet missing or ambiguous")
	}
	for _, object := range index.doc.Objects {
		if !energyPathPoolEqual(object.Type, "ZoneHVAC:EquipmentList") {
			continue
		}
		for field := 2; field+1 < len(object.Fields); field += 6 {
			if energyPathPoolEqual(energyPathPoolField(object, field), adu.Type) && energyPathPoolEqual(energyPathPoolField(object, field+1), energyPathPoolField(adu, 0)) {
				list = object
				lists++
			}
		}
	}
	if lists != 1 || !index.poolAirUnique(list) || len(list.Fields) != 8 {
		return fail("ADU does not have one exact single-member equipment list")
	}
	for _, object := range index.doc.Objects {
		if energyPathPoolEqual(object.Type, "ZoneHVAC:EquipmentConnections") && energyPathPoolEqual(energyPathPoolField(object, 1), energyPathPoolField(list, 0)) {
			connection = object
			connections++
		}
	}
	zone, zoneOK := index.unique("Zone", energyPathPoolField(connection, 0))
	inlet, inletOK := index.poolAirOneNode(energyPathPoolField(connection, 2))
	returned, returnOK := index.poolAirOneNode(energyPathPoolField(connection, 5))
	if connections != 1 || !zoneOK || !inletOK || !returnOK || !energyPathPoolEqual(inlet, outlet) || energyPathPoolField(connection, 3) != "" ||
		!energyPathPoolAirDistinct(outlet, energyPathPoolField(connection, 4), returned) {
		return fail("Zone native inlet, return and air-node connection incomplete")
	}
	zoneConnections := 0
	for _, peer := range index.doc.Objects {
		if !energyPathPoolEqual(peer.Type, "ZoneHVAC:EquipmentConnections") {
			continue
		}
		if energyPathPoolEqual(energyPathPoolField(peer, 0), energyPathPoolField(zone, 0)) {
			zoneConnections++
		}
		if peer.Index == connection.Index {
			continue
		}
		for _, field := range []int{2, 3, 4, 5} {
			for _, node := range []string{outlet, returned, energyPathPoolField(connection, 4)} {
				if index.poolAirSelectorMentions(energyPathPoolField(peer, field), node) {
					return fail("Zone air node/port has a competing connection owner")
				}
			}
		}
	}
	if zoneConnections != 1 {
		return fail("Zone has duplicate EquipmentConnections")
	}
	return energyPathPoolAirDelivery{Zone: energyPathPoolRef(zone), Connection: energyPathPoolRef(connection), EquipmentList: energyPathPoolRef(list), ADU: energyPathPoolRef(adu), Terminal: energyPathPoolRef(terminal), ReheatCoil: energyPathPoolRef(coil), ZoneAirNode: energyPathPoolField(connection, 4), ReturnNode: returned}, nil
}

// Existing navigation identity is a post-proof convenience, not topology
// authority. If an OA coil has no dedicated existing path, its independently
// proved OA -> Mixer -> main coil chain may reuse the SAME AirLoop, PlantLoop,
// service and exact terminal's main-coil path ID. This does not manufacture an
// OA HVAC path or turn navigation into consumer/denominator evidence.
func bindEnergyPathPoolAirRoutePaths(routes []energyPathPoolAirRoute, summaries []idf.ZoneServiceSummary, inventory *energyPathPoolInventory) {
	pathCounts := map[string]int{}
	for _, summary := range summaries {
		for _, candidate := range summary.Paths {
			if candidate.ID != "" {
				pathCounts[candidate.ID]++
			}
		}
	}
	for at := range routes {
		route := &routes[at]
		route.RelatedPathIDs = nil
		route.NavigationBindings = nil
		if !route.Complete {
			continue
		}
		allZones := true
		for _, delivery := range route.Deliveries {
			var exact, fallback []string
			allowFallback := route.Position == "outside_air" && route.NavigationAnchor.ID != "" &&
				energyPathPoolAirSameRef(route.NavigationAnchorPlantLoop, route.PlantLoop)
			for _, summary := range summaries {
				if !energyPathPoolAirPathZone(delivery.Zone.ObjectName, summary.ZoneName, summary.ServedSubject.ZoneName) || summary.SpaceName != "" || summary.ServedSubject.SpaceName != "" {
					continue
				}
				for _, candidate := range summary.Paths {
					if candidate.ID == "" || pathCounts[candidate.ID] != 1 || candidate.ServiceKind != route.ServiceKind || candidate.SpaceName != "" || candidate.ServedSubject.SpaceName != "" ||
						!energyPathPoolAirPathZone(delivery.Zone.ObjectName, candidate.ZoneName, candidate.ServedSubject.ZoneName) || candidate.AirLoop == nil ||
						candidate.AirLoop.ObjectIndex != route.AirLoop.ObjectIndex || !energyPathPoolEqual(candidate.AirLoop.Type, route.AirLoop.ObjectType) || !energyPathPoolEqual(candidate.AirLoop.Name, route.AirLoop.ObjectName) ||
						!energyPathPoolAirSameRef(candidate.Delivery, delivery.Terminal) || candidate.CondenserLoop != nil || candidate.RefrigerantSystem != nil {
						continue
					}
					if candidate.DeliveryWrapper != nil && !energyPathPoolAirSameRef(*candidate.DeliveryWrapper, delivery.ADU) {
						continue
					}
					contains, anchor := false, false
					for _, component := range candidate.Conditioning {
						contains = contains || energyPathPoolAirSameRef(component, route.Coil)
						anchor = anchor || energyPathPoolAirSameRef(component, route.NavigationAnchor)
					}
					if candidate.PlantLoop != nil && (candidate.PlantLoop.ObjectIndex != route.PlantLoop.ObjectIndex || !energyPathPoolEqual(candidate.PlantLoop.Type, route.PlantLoop.ObjectType) || !energyPathPoolEqual(candidate.PlantLoop.Name, route.PlantLoop.ObjectName)) {
						continue
					}
					if contains {
						exact = energyPathPoolAirUnion(exact, candidate.ID)
					}
					if allowFallback && anchor && candidate.PlantLoop != nil {
						fallback = energyPathPoolAirUnion(fallback, candidate.ID)
					}
				}
			}
			matched, mode, anchor := exact, "exact_coil", route.Coil
			if len(matched) == 0 && len(fallback) > 0 {
				matched, mode, anchor = fallback, "proved_oa_via_same_loop_main_coil", route.NavigationAnchor
			}
			if len(matched) == 0 {
				allZones = false
			}
			route.RelatedPathIDs = energyPathPoolAirUnion(route.RelatedPathIDs, matched...)
			if len(matched) > 0 {
				route.NavigationBindings = append(route.NavigationBindings, energyPathPoolAirNavigationBinding{ZoneName: delivery.Zone.ObjectName, PathIDs: append([]string(nil), matched...), Mode: mode, Anchor: anchor})
			}
		}
		if !allZones {
			route.RelatedPathIDs = nil
			route.NavigationBindings = nil
		}
	}
	if inventory == nil {
		return
	}
	for loop := range inventory.Loops {
		for demand := range inventory.Loops[loop].Demands {
			item := &inventory.Loops[loop].Demands[demand]
			item.RelatedPathIDs = nil
			if !item.AirRouteComplete {
				continue
			}
			for _, route := range routes {
				if route.Complete && energyPathPoolAirSameRef(route.Coil, item.Water.Component) && energyPathPoolAirSameRef(route.PlantLoop, inventory.Loops[loop].Component) {
					item.RelatedPathIDs = append([]string(nil), route.RelatedPathIDs...)
				}
			}
		}
	}
}

func energyPathPoolAirPathZone(want string, names ...string) bool {
	observed := false
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if !energyPathPoolEqual(want, name) {
			return false
		}
		observed = true
	}
	return observed
}
