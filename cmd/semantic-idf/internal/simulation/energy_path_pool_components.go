package simulation

// The reviewed schema is EnergyPlus 25.1. Water-side identity/ownership is not
// proof of an air-side service route, engine-input validity, or an energy split.

import (
	"fmt"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type energyPathPoolInventory struct {
	HasNativePool         bool
	SchemaReviewed        bool
	Pools                 []energyPathPoolTarget
	Sources               []energyPathPoolSource
	Loops                 []energyPathPoolPlant
	UnresolvedPoolIndices []int
}

// Keep every original pool, including rejected/ambiguous owners. Presence must
// never disappear merely because the stronger target proof failed.
type energyPathPoolTarget struct {
	Component         idf.ComponentRef
	Surface           idf.ComponentRef
	ZoneName          string
	Loop              idf.ComponentRef
	Branch            idf.ComponentRef
	IdentityValid     bool
	SurfaceOwnerValid bool
	WaterOwnerValid   bool
}

func (target energyPathPoolTarget) resolved() bool {
	return target.IdentityValid && target.SurfaceOwnerValid && target.WaterOwnerValid
}

// A unique native reporting identity can still be observed when allocation is
// forbidden. WaterOwnerValid and the loop's independent completeness flags must
// be checked separately; neither a boiler efficiency nor a pool transfer can
// replace a measured resource budget. These native observations have factor 1.
type energyPathPoolSource struct {
	Component       idf.ComponentRef
	Loop            idf.ComponentRef
	Branch          idf.ComponentRef
	Kind            string // boiler, pump, chiller; not an output-definition type
	FuelType        string // original explicit Boiler:HotWater field 1
	IdentityValid   bool
	WaterOwnerValid bool
}

type energyPathPoolPlant struct {
	Component             idf.ComponentRef
	Supply                energyPathPoolSide
	Demand                energyPathPoolSide
	Sources               []energyPathPoolSource
	Demands               []energyPathPoolDemand
	WaterTopologyComplete bool
	SupplyRosterComplete  bool
	DemandRosterComplete  bool
	HasNonZoneDemand      bool
	AirRoutesComplete     bool
}

type energyPathPoolSide struct {
	BranchListName    string
	ConnectorListName string
	Components        []energyPathPoolWaterComponent // includes pipes and unknowns
	Complete          bool
	Issues            []string
}

// Missing native objects retain the declared type/name with ObjectIndex=-1.
// An unrecognized demand is evidence, not a pipe and not a removable zero.
type energyPathPoolWaterComponent struct {
	Component       idf.ComponentRef
	Branch          idf.ComponentRef
	IdentityValid   bool
	WaterOwnerValid bool
}

type energyPathPoolDemand struct {
	Water            energyPathPoolWaterComponent
	Kind             string // heating_coil, cooling_coil, pool, unknown
	ZoneName         string // pool surface owner only; never a ZoneHVAC path
	NonZoneDemand    bool
	AirRouteComplete bool
	RelatedPathIDs   []string // reserved for a separate exact air-route proof
}

func energyPathHasNativePool(doc idf.Document) bool {
	for _, object := range doc.Objects {
		if energyPathPoolEqual(object.Type, "SwimmingPool:Indoor") {
			return true
		}
	}
	return false
}

func energyPathNativePoolInventory(doc idf.Document) energyPathPoolInventory {
	out := energyPathPoolInventory{HasNativePool: energyPathHasNativePool(doc)}
	if !out.HasNativePool {
		return out
	}
	index := newEnergyPathPoolIndex(doc)
	versions := 0
	for _, object := range doc.Objects {
		if energyPathPoolEqual(object.Type, "Version") {
			versions++
			out.SchemaReviewed = energyPathPoolField(object, 0) == "25.1" || energyPathPoolField(object, 0) == "25.1.0"
		}
	}
	out.SchemaReviewed = out.SchemaReviewed && versions == 1
	for _, object := range doc.Objects {
		if energyPathPoolEqual(object.Type, "SwimmingPool:Indoor") {
			target := energyPathPoolTarget{Component: energyPathPoolRef(object)}
			if out.SchemaReviewed {
				target.IdentityValid = index.identityValid(object)
				target.Component.WaterInletNode, target.Component.WaterOutletNode, _ = energyPathPoolWaterPorts(object)
				target.Surface, target.ZoneName, target.SurfaceOwnerValid = index.poolSurface(object)
				target.Loop, target.Branch, _, target.WaterOwnerValid = index.waterOwner(object)
				target.WaterOwnerValid = target.WaterOwnerValid && index.poolOnDemand(object)
			}
			if !target.resolved() {
				out.UnresolvedPoolIndices = append(out.UnresolvedPoolIndices, object.Index)
			}
			out.Pools = append(out.Pools, target)
		}
		kind := energyPathPoolSourceKind(object.Type)
		if kind == "" {
			continue
		}
		source := energyPathPoolSource{Component: energyPathPoolRef(object), Kind: kind}
		if kind == "boiler" {
			source.FuelType = energyPathPoolField(object, 1)
		}
		if out.SchemaReviewed {
			source.IdentityValid = index.identityValid(object)
			source.Component.WaterInletNode, source.Component.WaterOutletNode, _ = energyPathPoolWaterPorts(object)
			var side string
			source.Loop, source.Branch, side, source.WaterOwnerValid = index.waterOwner(object)
			source.WaterOwnerValid = source.WaterOwnerValid && side == "supply"
		}
		out.Sources = append(out.Sources, source)
	}
	if !out.SchemaReviewed {
		return out
	}
	for _, object := range doc.Objects {
		if !energyPathPoolEqual(object.Type, "PlantLoop") {
			continue
		}
		plant := energyPathPoolPlant{Component: energyPathPoolRef(object)}
		plant.Supply = index.side(object, "supply")
		plant.Demand = index.side(object, "demand")
		plant.WaterTopologyComplete = plant.Supply.Complete && plant.Demand.Complete
		plant.SupplyRosterComplete = plant.Supply.Complete
		plant.DemandRosterComplete = plant.Demand.Complete
		for _, water := range plant.Supply.Components {
			if energyPathPoolEqual(water.Component.ObjectType, "Pipe:Adiabatic") {
				continue
			}
			if energyPathPoolSourceKind(water.Component.ObjectType) == "" {
				plant.SupplyRosterComplete = false
			}
		}
		for _, source := range out.Sources {
			if source.WaterOwnerValid && source.Loop.ObjectIndex == object.Index {
				plant.Sources = append(plant.Sources, source)
			}
		}
		for _, water := range plant.Demand.Components {
			if energyPathPoolEqual(water.Component.ObjectType, "Pipe:Adiabatic") {
				continue
			}
			demand := energyPathPoolDemand{Water: water, Kind: "unknown"}
			switch energyPathPoolToken(water.Component.ObjectType) {
			case "coil:heating:water":
				demand.Kind = "heating_coil"
			case "coil:cooling:water":
				demand.Kind = "cooling_coil"
			case "swimmingpool:indoor":
				demand.Kind, demand.NonZoneDemand = "pool", true
				plant.HasNonZoneDemand = true
				for _, pool := range out.Pools {
					if pool.Component.ObjectIndex == water.Component.ObjectIndex && pool.resolved() {
						demand.ZoneName = pool.ZoneName
					}
				}
				if demand.ZoneName == "" {
					plant.DemandRosterComplete = false
				}
			default:
				plant.DemandRosterComplete = false
			}
			plant.Demands = append(plant.Demands, demand)
		}
		// Deliberately false, even for the CW loop with two known cooling coils.
		// A future proof must cover EVERY demand's full air route, including the
		// outside-air wrapper and all terminal owners, without using names.
		plant.AirRoutesComplete = false
		out.Loops = append(out.Loops, plant)
	}
	return out
}

type energyPathPoolIndex struct {
	doc     idf.Document
	objects map[string][]int
}

func newEnergyPathPoolIndex(doc idf.Document) energyPathPoolIndex {
	index := energyPathPoolIndex{doc: doc, objects: map[string][]int{}}
	for position, object := range doc.Objects {
		key := energyPathPoolKey(object.Type, energyPathPoolField(object, 0))
		index.objects[key] = append(index.objects[key], position)
	}
	return index
}

func energyPathPoolField(object idf.Object, field int) string {
	if field < 0 || field >= len(object.Fields) {
		return ""
	}
	return strings.TrimSpace(object.Fields[field].Value)
}

func energyPathPoolToken(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
func energyPathPoolEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
func energyPathPoolKey(objectType, name string) string {
	return energyPathPoolToken(objectType) + "\x00" + energyPathPoolToken(name)
}

func energyPathPoolRef(object idf.Object) idf.ComponentRef {
	name := energyPathPoolField(object, 0)
	return idf.ComponentRef{ID: fmt.Sprintf("component:%d", object.Index), ObjectType: object.Type, ObjectName: name, ObjectIndex: object.Index, DisplayName: name}
}

func (index energyPathPoolIndex) unique(objectType, name string) (idf.Object, bool) {
	positions := index.objects[energyPathPoolKey(objectType, name)]
	if strings.TrimSpace(name) == "" || len(positions) != 1 {
		return idf.Object{}, false
	}
	return index.doc.Objects[positions[0]], true
}

func energyPathPoolSourceKind(objectType string) string {
	switch energyPathPoolToken(objectType) {
	case "boiler:hotwater":
		return "boiler"
	case "pump:variablespeed":
		return "pump"
	case "chiller:electric":
		return "chiller"
	}
	return ""
}

func (index energyPathPoolIndex) identityValid(object idf.Object) bool {
	name := energyPathPoolField(object, 0)
	_, unique := index.unique(object.Type, name)
	if !unique {
		return false
	}
	family := energyPathPoolSourceKind(object.Type)
	if family == "" {
		return true
	}
	count := 0
	for _, peer := range index.doc.Objects {
		kind := energyPathPoolToken(peer.Type)
		sharedNamespace := strings.HasPrefix(kind, family+":")
		if family == "pump" {
			sharedNamespace = sharedNamespace || strings.HasPrefix(kind, "headeredpumps:")
		}
		if sharedNamespace && energyPathPoolEqual(energyPathPoolField(peer, 0), name) {
			count++
		}
	}
	return count == 1
}

func (index energyPathPoolIndex) poolSurface(pool idf.Object) (idf.ComponentRef, string, bool) {
	name := energyPathPoolField(pool, 1)
	surface, found := index.unique("BuildingSurface:Detailed", name)
	if !found || !energyPathPoolEqual(energyPathPoolField(surface, 1), "Floor") || energyPathPoolField(surface, 4) != "" {
		return idf.ComponentRef{}, "", false
	}
	// Native surface names share one namespace. Unreviewed legacy geometry is
	// not an alternate route around an ambiguous BuildingSurface identity.
	surfaces, owners := 0, 0
	for _, object := range index.doc.Objects {
		kind := energyPathPoolToken(object.Type)
		for _, prefix := range []string{"buildingsurface:", "fenestrationsurface:", "wall:", "floor:", "roofceiling:", "roof:", "ceiling:", "window:", "door:", "glazeddoor:"} {
			if strings.HasPrefix(kind, prefix) && energyPathPoolEqual(energyPathPoolField(object, 0), name) {
				surfaces++
				break
			}
		}
		if energyPathPoolEqual(object.Type, "SwimmingPool:Indoor") && energyPathPoolEqual(energyPathPoolField(object, 1), name) {
			owners++
		}
	}
	zoneName := energyPathPoolField(surface, 3)
	zone, zoneUnique := index.unique("Zone", zoneName)
	if surfaces != 1 || owners != 1 || !zoneUnique {
		return energyPathPoolRef(surface), "", false
	}
	return energyPathPoolRef(surface), energyPathPoolField(zone, 0), true
}

// Zero-based 25.1 field positions; Branch uses four-field extensibles. A
// Chiller:Electric chilled-water pair is 4/5, never condenser pair 6/7.
func energyPathPoolWaterPorts(object idf.Object) (string, string, bool) {
	a, b := -1, -1
	switch energyPathPoolToken(object.Type) {
	case "swimmingpool:indoor", "boiler:hotwater":
		a, b = 10, 11
	case "pump:variablespeed", "pipe:adiabatic":
		a, b = 1, 2
	case "chiller:electric", "coil:heating:water":
		a, b = 4, 5
	case "coil:cooling:water":
		a, b = 9, 10
	}
	inlet, outlet := energyPathPoolField(object, a), energyPathPoolField(object, b)
	return inlet, outlet, inlet != "" && outlet != "" && !energyPathPoolEqual(inlet, outlet)
}

func (index energyPathPoolIndex) branchOwner(branch idf.Object) (idf.Object, string, bool) {
	name := energyPathPoolField(branch, 0)
	if _, ok := index.unique("Branch", name); !ok {
		return idf.Object{}, "", false
	}
	lists, owners := 0, 0
	var selected idf.Object
	for _, list := range index.doc.Objects {
		if !energyPathPoolEqual(list.Type, "BranchList") {
			continue
		}
		for field := 1; field < len(list.Fields); field++ {
			if energyPathPoolEqual(energyPathPoolField(list, field), name) {
				lists++
				selected = list
			}
		}
	}
	if lists != 1 {
		return idf.Object{}, "", false
	}
	listName := energyPathPoolField(selected, 0)
	if _, ok := index.unique("BranchList", listName); !ok {
		return idf.Object{}, "", false
	}
	var selectedLoop idf.Object
	selectedSide := ""
	for _, loop := range index.doc.Objects {
		fields := []int(nil)
		switch energyPathPoolToken(loop.Type) {
		case "plantloop", "condenserloop":
			fields = []int{12, 16}
		case "airloophvac":
			fields = []int{4}
		}
		for _, field := range fields {
			if energyPathPoolEqual(energyPathPoolField(loop, field), listName) {
				owners++
				selectedLoop = loop
				selectedSide = "supply"
				if field == 16 {
					selectedSide = "demand"
				}
			}
		}
	}
	_, unique := index.unique(selectedLoop.Type, energyPathPoolField(selectedLoop, 0))
	return selectedLoop, selectedSide, owners == 1 && unique
}

func (index energyPathPoolIndex) waterOwner(object idf.Object) (idf.ComponentRef, idf.ComponentRef, string, bool) {
	inlet, outlet, portsOK := energyPathPoolWaterPorts(object)
	if !portsOK || !index.identityValid(object) {
		return idf.ComponentRef{}, idf.ComponentRef{}, "", false
	}
	count, valid := 0, true
	var ownedLoop, ownedBranch idf.ComponentRef
	ownedSide := ""
	for _, branch := range index.doc.Objects {
		if !energyPathPoolEqual(branch.Type, "Branch") {
			continue
		}
		for field := 2; field+1 < len(branch.Fields); field++ {
			if !energyPathPoolEqual(energyPathPoolField(branch, field), object.Type) || !energyPathPoolEqual(energyPathPoolField(branch, field+1), energyPathPoolField(object, 0)) {
				continue
			}
			loop, side, uniqueOwner := index.branchOwner(branch)
			// A water coil legitimately appears on a separately owned air branch.
			// Only exact native air ports on a unique AirLoop owner are excluded.
			airIn, airOut := -1, -1
			if energyPathPoolEqual(object.Type, "Coil:Heating:Water") {
				airIn, airOut = 6, 7
			} else if energyPathPoolEqual(object.Type, "Coil:Cooling:Water") {
				airIn, airOut = 11, 12
			}
			if uniqueOwner && energyPathPoolEqual(loop.Type, "AirLoopHVAC") && airIn >= 0 && (field-2)%4 == 0 &&
				energyPathPoolField(object, airIn) != "" && energyPathPoolField(object, airOut) != "" &&
				energyPathPoolEqual(energyPathPoolField(branch, field+2), energyPathPoolField(object, airIn)) &&
				energyPathPoolEqual(energyPathPoolField(branch, field+3), energyPathPoolField(object, airOut)) {
				continue
			}
			count++
			valid = valid && uniqueOwner && energyPathPoolEqual(loop.Type, "PlantLoop") &&
				len(branch.Fields) >= 6 && (len(branch.Fields)-2)%4 == 0 && (field-2)%4 == 0 &&
				energyPathPoolEqual(energyPathPoolField(branch, field+2), inlet) && energyPathPoolEqual(energyPathPoolField(branch, field+3), outlet)
			ownedLoop, ownedBranch, ownedSide = energyPathPoolRef(loop), energyPathPoolRef(branch), side
		}
	}
	return ownedLoop, ownedBranch, ownedSide, valid && count == 1
}

func (index energyPathPoolIndex) poolOnDemand(pool idf.Object) bool {
	_, _, side, ok := index.waterOwner(pool)
	return ok && side == "demand"
}

func (index energyPathPoolIndex) side(loop idf.Object, sideName string) energyPathPoolSide {
	base := 10
	if sideName == "demand" {
		base = 14
	}
	out := energyPathPoolSide{BranchListName: energyPathPoolField(loop, base+2), ConnectorListName: energyPathPoolField(loop, base+3), Complete: true}
	fail := func(reason string) { out.Complete = false; out.Issues = append(out.Issues, reason) }
	if _, ok := index.unique(loop.Type, energyPathPoolField(loop, 0)); !ok || !energyPathPoolEqual(energyPathPoolField(loop, 1), "Water") {
		fail("ambiguous loop identity or unreviewed fluid")
	}
	list, listOK := index.unique("BranchList", out.BranchListName)
	if !listOK || len(list.Fields) < 4 {
		fail("missing, ambiguous, or unreviewed branch list")
		return out
	}
	branchNames := make([]string, 0, len(list.Fields)-1)
	seenBranches, seenBoundaryNodes := map[string]bool{}, map[string]bool{}
	for field := 1; field < len(list.Fields); field++ {
		name := energyPathPoolField(list, field)
		branchNames = append(branchNames, name)
		branch, unique := index.unique("Branch", name)
		key := energyPathPoolToken(name)
		if !unique || seenBranches[key] {
			fail("missing, duplicate, or ambiguous branch: " + name)
			continue
		}
		seenBranches[key] = true
		owner, ownerSide, owned := index.branchOwner(branch)
		if !owned || owner.Index != loop.Index || !energyPathPoolEqual(owner.Type, loop.Type) || ownerSide != sideName {
			fail("non-unique loop-side ownership: " + name)
		}
		if len(branch.Fields) < 6 || (len(branch.Fields)-2)%4 != 0 {
			fail("malformed four-field component roster: " + name)
		}
		previous := ""
		for start := 2; start < len(branch.Fields); start += 4 {
			objectType, objectName := energyPathPoolField(branch, start), energyPathPoolField(branch, start+1)
			object, found := index.unique(objectType, objectName)
			water := energyPathPoolWaterComponent{Component: idf.ComponentRef{ObjectType: objectType, ObjectName: objectName, ObjectIndex: -1}, Branch: energyPathPoolRef(branch)}
			if found {
				water.Component = energyPathPoolRef(object)
				water.IdentityValid = index.identityValid(object)
				parent, parentBranch, parentSide, valid := index.waterOwner(object)
				water.WaterOwnerValid = valid && parent.ObjectIndex == loop.Index && parentBranch.ObjectIndex == branch.Index && parentSide == sideName
			}
			inlet, outlet := energyPathPoolField(branch, start+2), energyPathPoolField(branch, start+3)
			water.Component.WaterInletNode, water.Component.WaterOutletNode = inlet, outlet
			if !water.IdentityValid || !water.WaterOwnerValid || inlet == "" || outlet == "" || energyPathPoolEqual(inlet, outlet) || (previous != "" && !energyPathPoolEqual(previous, inlet)) {
				fail("unproved native water component or continuity: " + objectType + "/" + objectName)
			}
			previous = outlet
			out.Components = append(out.Components, water)
		}
		first, last := energyPathPoolField(branch, 4), energyPathPoolField(branch, len(branch.Fields)-1)
		for _, node := range []string{first, last} {
			key := energyPathPoolToken(node)
			if key == "" || seenBoundaryNodes[key] {
				fail("blank or shared parallel branch boundary node: " + node)
			}
			seenBoundaryNodes[key] = true
		}
		if field == 1 && !energyPathPoolEqual(first, energyPathPoolField(loop, base)) {
			fail("loop inlet does not match first branch")
		}
		if field == len(list.Fields)-1 && !energyPathPoolEqual(last, energyPathPoolField(loop, base+1)) {
			fail("loop outlet does not match last branch")
		}
	}
	if !index.connectors(loop, base+3, branchNames) {
		fail("unproved splitter/mixer exact branch roster or connector ownership")
	}
	return out
}

func (index energyPathPoolIndex) connectors(loop idf.Object, field int, branches []string) bool {
	name := energyPathPoolField(loop, field)
	list, unique := index.unique("ConnectorList", name)
	if !unique || len(list.Fields) != 5 || len(branches) < 3 {
		return false
	}
	owners := 0
	for _, peer := range index.doc.Objects {
		var fields []int
		switch energyPathPoolToken(peer.Type) {
		case "plantloop", "condenserloop":
			fields = []int{13, 17}
		case "airloophvac":
			fields = []int{5}
		}
		for _, slot := range fields {
			if energyPathPoolEqual(energyPathPoolField(peer, slot), name) {
				owners++
			}
		}
	}
	if owners != 1 {
		return false
	}
	seen := map[string]bool{}
	for _, slot := range []int{1, 3} {
		kind, connectorName := energyPathPoolField(list, slot), energyPathPoolField(list, slot+1)
		token := energyPathPoolToken(kind)
		if (token != "connector:splitter" && token != "connector:mixer") || seen[token] {
			return false
		}
		seen[token] = true
		connector, found := index.unique(kind, connectorName)
		if !found || len(connector.Fields) != len(branches) {
			return false
		}
		references := 0
		for _, peer := range index.doc.Objects {
			if energyPathPoolEqual(peer.Type, "ConnectorList") {
				// Count even a malformed/off-stride competing reference. It is
				// not evidence that this connector has one unique owner.
				for at := 1; at+1 < len(peer.Fields); at++ {
					if energyPathPoolEqual(energyPathPoolField(peer, at), kind) && energyPathPoolEqual(energyPathPoolField(peer, at+1), connectorName) {
						references++
					}
				}
			}
		}
		boundary := branches[0]
		if token == "connector:mixer" {
			boundary = branches[len(branches)-1]
		}
		if references != 1 || !energyPathPoolEqual(energyPathPoolField(connector, 1), boundary) {
			return false
		}
		wanted := map[string]bool{}
		for _, branch := range branches[1 : len(branches)-1] {
			key := energyPathPoolToken(branch)
			if key == "" || wanted[key] {
				return false
			}
			wanted[key] = true
		}
		for at := 2; at < len(connector.Fields); at++ {
			key := energyPathPoolToken(energyPathPoolField(connector, at))
			if !wanted[key] {
				return false
			}
			delete(wanted, key)
		}
		if len(wanted) != 0 {
			return false
		}
	}
	return len(seen) == 2
}
