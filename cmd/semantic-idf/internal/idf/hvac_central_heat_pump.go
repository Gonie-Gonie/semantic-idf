package idf

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const nativeCentralHeatPumpType = "CentralHeatPumpSystem"

// NativeCentralHeatPumpBinding separates reporting identity from physical
// eligibility. PortBindingsComplete proves ONLY this original system's three
// native water-port occurrences and the corresponding connector-side rosters.
// It does not prove coil/terminal/Zone recipients or authorize allocation.
// NativeDefinitionValid concerns the structural binding inputs only, not the
// performance curves, sizing, control schedule values or engine solvability.
// This reviewed contract is 25.1 SmartMixing/Electric:EIR, not a generic chiller
// fallback. In particular, Source is never a space-heating/cooling supply.
type NativeCentralHeatPumpBinding struct {
	System                   ComponentRef
	ReportingIdentityValid   bool
	NativeDefinitionValid    bool
	PortBindingsComplete     bool
	Cooling, Heating, Source NativeCentralHeatPumpPort
	Modules                  []NativeCentralHeatPumpModule
	Issues                   []string
}

type NativeCentralHeatPumpPort struct {
	Role, Side, InletNode, OutletNode string
	InletField, OutletField           int
	Loop                              LoopRef
	Branch                            ComponentRef
	BranchComponentField              int
	Bound                             bool
}

type NativeCentralHeatPumpModule struct {
	Performance, ControlSchedule ComponentRef
	Count, FirstUnit             int
}

type centralHeatPumpIndex struct {
	doc     Document
	objects map[string][]Object
}

func centralHeatPumpField(object Object, field int) string {
	if field < 0 || field >= len(object.Fields) {
		return ""
	}
	return strings.TrimSpace(object.Fields[field].Value)
}

func centralHeatPumpKey(kind, name string) string {
	return strings.ToLower(strings.TrimSpace(kind)) + "\x00" + strings.ToLower(strings.TrimSpace(name))
}

func centralHeatPumpEqual(a, b string) bool {
	return strings.TrimSpace(a) != "" && strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func (index centralHeatPumpIndex) unique(kind, name string) (Object, bool) {
	objects := index.objects[centralHeatPumpKey(kind, name)]
	if name == "" || len(objects) != 1 {
		return Object{}, false
	}
	return objects[0], true
}

func centralHeatPumpRef(object Object) ComponentRef {
	return ComponentRef{ID: fmt.Sprintf("component:%d", object.Index), ObjectType: object.Type,
		ObjectName: centralHeatPumpField(object, 0), ObjectIndex: object.Index}
}

func (index centralHeatPumpIndex) schedule(name string) (Object, bool) {
	var found Object
	count := 0
	// These native ScheduleNames emitters cover the reviewed contract; child
	// Day/Week definitions and ScheduleTypeLimits are not usable schedules.
	for _, kind := range []string{"Schedule:Compact", "Schedule:Constant", "Schedule:Year", "Schedule:File"} {
		for _, object := range index.objects[centralHeatPumpKey(kind, name)] {
			found = object
			count++
		}
	}
	return found, name != "" && count == 1
}

// ResolveNativeCentralHeatPumpBindings performs no AnalyzeHVAC call and does
// not borrow generic/comment-based field inference. Absent families take the
// fast path; unreviewed versions retain census records but grant no binding.
func ResolveNativeCentralHeatPumpBindings(doc Document) []NativeCentralHeatPumpBinding {
	var systems []Object
	for _, object := range doc.Objects {
		if centralHeatPumpEqual(object.Type, nativeCentralHeatPumpType) {
			systems = append(systems, object)
		}
	}
	if len(systems) == 0 {
		return nil
	}
	index := centralHeatPumpIndex{doc: doc, objects: map[string][]Object{}}
	versions, reviewed := 0, false
	indicesUnique, seenIndices := true, map[int]bool{}
	for _, object := range doc.Objects {
		if object.Index < 0 || seenIndices[object.Index] {
			indicesUnique = false
		}
		seenIndices[object.Index] = true
		key := centralHeatPumpKey(object.Type, centralHeatPumpField(object, 0))
		index.objects[key] = append(index.objects[key], object)
		if centralHeatPumpEqual(object.Type, "Version") {
			versions++
			version := centralHeatPumpField(object, 0)
			reviewed = version == "25.1" || version == "25.1.0"
		}
	}
	var result []NativeCentralHeatPumpBinding
	for _, system := range systems {
		binding := NativeCentralHeatPumpBinding{System: centralHeatPumpRef(system)}
		binding.Cooling = NativeCentralHeatPumpPort{Role: "cooling", Side: "supply", InletField: 2, OutletField: 3,
			InletNode: centralHeatPumpField(system, 2), OutletNode: centralHeatPumpField(system, 3)}
		binding.Source = NativeCentralHeatPumpPort{Role: "source", Side: "demand", InletField: 4, OutletField: 5,
			InletNode: centralHeatPumpField(system, 4), OutletNode: centralHeatPumpField(system, 5)}
		binding.Heating = NativeCentralHeatPumpPort{Role: "heating", Side: "supply", InletField: 6, OutletField: 7,
			InletNode: centralHeatPumpField(system, 6), OutletNode: centralHeatPumpField(system, 7)}
		_, unique := index.unique(nativeCentralHeatPumpType, binding.System.ObjectName)
		binding.ReportingIdentityValid = versions == 1 && reviewed && unique && indicesUnique
		if !binding.ReportingIdentityValid {
			binding.Issues = append(binding.Issues, "unreviewed version or nonunique original index/native system reporting identity")
			result = append(result, binding)
			continue
		}
		binding.Modules, binding.NativeDefinitionValid = index.definition(system)
		if !binding.NativeDefinitionValid {
			binding.Issues = append(binding.Issues, "native module/control/port definition is unresolved")
			result = append(result, binding)
			continue
		}
		ports := []*NativeCentralHeatPumpPort{&binding.Cooling, &binding.Heating, &binding.Source}
		occurrences, censusValid := index.occurrences(binding.System.ObjectName)
		if len(occurrences) != 3 || !censusValid {
			binding.Issues = append(binding.Issues, "system must have exactly three unambiguous native Branch occurrences")
			result = append(result, binding)
			continue
		}
		for _, port := range ports {
			matches := 0
			for _, occurrence := range occurrences {
				if !centralHeatPumpEqual(centralHeatPumpField(occurrence.branch, occurrence.field+2), port.InletNode) ||
					!centralHeatPumpEqual(centralHeatPumpField(occurrence.branch, occurrence.field+3), port.OutletNode) {
					continue
				}
				matches++
				loop, found := index.owner(occurrence.branch, port.Side)
				if !found {
					continue
				}
				wantType := "PlantLoop"
				if port.Role == "source" {
					wantType = "CondenserLoop"
				}
				if !centralHeatPumpEqual(loop.Type, wantType) {
					continue
				}
				loopPrefix := "plant:"
				if centralHeatPumpEqual(loop.Type, "CondenserLoop") {
					loopPrefix = "condenser:"
				}
				port.Loop = LoopRef{ID: loopPrefix + strings.ToLower(centralHeatPumpField(loop, 0)), Type: loop.Type,
					Name: centralHeatPumpField(loop, 0), ObjectIndex: loop.Index, Mediums: []string{"water"}}
				port.Branch, port.BranchComponentField = centralHeatPumpRef(occurrence.branch), occurrence.field
				port.Bound = true
			}
			if matches != 1 {
				port.Bound = false
			}
		}
		binding.PortBindingsComplete = binding.Cooling.Bound && binding.Heating.Bound && binding.Source.Bound &&
			binding.Cooling.Loop.ObjectIndex != binding.Heating.Loop.ObjectIndex &&
			binding.Cooling.Loop.ObjectIndex != binding.Source.Loop.ObjectIndex &&
			binding.Heating.Loop.ObjectIndex != binding.Source.Loop.ObjectIndex
		if !binding.PortBindingsComplete {
			binding.Issues = append(binding.Issues, "native cooling/heating/source port ownership is incomplete")
		}
		result = append(result, binding)
	}
	return result
}

func (index centralHeatPumpIndex) definition(system Object) ([]NativeCentralHeatPumpModule, bool) {
	mode := centralHeatPumpField(system, 1)
	if mode != "" && !centralHeatPumpEqual(mode, "SmartMixing") {
		return nil, false
	}
	seenNodes := map[string]bool{}
	for field := 2; field <= 7; field++ {
		node := strings.ToLower(centralHeatPumpField(system, field))
		if node == "" || seenNodes[node] {
			return nil, false
		}
		seenNodes[node] = true
	}
	if power := centralHeatPumpField(system, 8); power != "" {
		value, err := strconv.ParseFloat(power, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return nil, false
		}
	}
	if name := centralHeatPumpField(system, 9); name != "" {
		if _, ok := index.schedule(name); !ok {
			return nil, false
		}
	}
	// Native IDD has at most twenty four-field module groups. Trailing omitted
	// numeric count defaults to 1; interior empty groups are not silently skipped.
	last := len(system.Fields)
	for last > 10 && centralHeatPumpField(system, last-1) == "" {
		last--
	}
	if last < 13 || last > 90 {
		return nil, false
	}
	var modules []NativeCentralHeatPumpModule
	firstUnit := 1
	for field := 10; field < last; field += 4 {
		if !centralHeatPumpEqual(centralHeatPumpField(system, field), "ChillerHeaterPerformance:Electric:EIR") {
			return nil, false
		}
		performance, found := index.unique("ChillerHeaterPerformance:Electric:EIR", centralHeatPumpField(system, field+1))
		if !found {
			return nil, false
		}
		schedule, found := index.schedule(centralHeatPumpField(system, field+2))
		if !found {
			return nil, false
		}
		count := 1
		if raw := centralHeatPumpField(system, field+3); raw != "" {
			number, err := strconv.ParseFloat(raw, 64)
			if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 1 || number != math.Trunc(number) || number > float64(int(^uint(0)>>1))/2 {
				return nil, false
			}
			count = int(number)
		}
		if count > int(^uint(0)>>1)-firstUnit {
			return nil, false
		}
		modules = append(modules, NativeCentralHeatPumpModule{Performance: centralHeatPumpRef(performance), ControlSchedule: centralHeatPumpRef(schedule), Count: count, FirstUnit: firstUnit})
		firstUnit += count
	}
	return modules, len(modules) > 0
}

type centralHeatPumpOccurrence struct {
	branch Object
	field  int
}

func (index centralHeatPumpIndex) occurrences(name string) ([]centralHeatPumpOccurrence, bool) {
	var result []centralHeatPumpOccurrence
	valid := true
	for _, branch := range index.doc.Objects {
		if !centralHeatPumpEqual(branch.Type, "Branch") {
			continue
		}
		// Also notice a malformed off-stride reference, rather than dropping it
		// and approving the other three well-formed occurrences.
		for field := 0; field+1 < len(branch.Fields); field++ {
			if !centralHeatPumpEqual(centralHeatPumpField(branch, field), nativeCentralHeatPumpType) || !centralHeatPumpEqual(centralHeatPumpField(branch, field+1), name) {
				continue
			}
			result = append(result, centralHeatPumpOccurrence{branch, field})
			_, unique := index.unique("Branch", centralHeatPumpField(branch, 0))
			if !unique || field < 2 || (field-2)%4 != 0 {
				valid = false
			}
			if _, _, ok := index.branchEndpoints(branch); !ok {
				valid = false
			}
		}
	}
	return result, valid
}

// branchEndpoints validates the Branch chain and finite reviewed native water
// ports. Unknown component kinds cannot acquire physical eligibility from a
// unique name. Recipient/controller and source-role gates remain mandatory.
func (index centralHeatPumpIndex) branchEndpoints(branch Object) (string, string, bool) {
	if len(branch.Fields) < 6 || (len(branch.Fields)-2)%4 != 0 {
		return "", "", false
	}
	first, previous := "", ""
	for field := 2; field+3 < len(branch.Fields); field += 4 {
		kind, name := centralHeatPumpField(branch, field), centralHeatPumpField(branch, field+1)
		component, found := index.unique(kind, name)
		if !found {
			return "", "", false
		}
		inlet, outlet := centralHeatPumpField(branch, field+2), centralHeatPumpField(branch, field+3)
		if inlet == "" || outlet == "" || centralHeatPumpEqual(inlet, outlet) {
			return "", "", false
		}
		if !index.nativeWaterPortsMatch(component, inlet, outlet) {
			return "", "", false
		}
		if previous != "" && !centralHeatPumpEqual(previous, inlet) {
			return "", "", false
		}
		if first == "" {
			first = inlet
		}
		previous = outlet
	}
	return first, previous, first != "" && previous != ""
}

// Only the reviewed 25.1 branch cohort is admitted. These are native IDD
// fields, not editable comments or one positional fallback for arbitrary kinds.
func (index centralHeatPumpIndex) nativeWaterPortsMatch(component Object, inlet, outlet string) bool {
	field := -1
	switch strings.ToLower(strings.TrimSpace(component.Type)) {
	case "pipe:adiabatic", "pump:variablespeed", "districtcooling", "districtheating:water":
		field = 1
	case "coil:heating:water":
		field = 4
	case "coil:cooling:water:detailedgeometry":
		field = 18
	case "centralheatpumpsystem":
		if _, valid := index.definition(component); !valid {
			return false
		}
		matches := 0
		for _, native := range []int{2, 4, 6} {
			if centralHeatPumpEqual(centralHeatPumpField(component, native), inlet) && centralHeatPumpEqual(centralHeatPumpField(component, native+1), outlet) {
				matches++
			}
		}
		return matches == 1
	default:
		return false
	}
	return centralHeatPumpEqual(centralHeatPumpField(component, field), inlet) && centralHeatPumpEqual(centralHeatPumpField(component, field+1), outlet)
}

func (index centralHeatPumpIndex) loopNameUnique(name string) bool {
	return len(index.objects[centralHeatPumpKey("PlantLoop", name)])+len(index.objects[centralHeatPumpKey("CondenserLoop", name)]) == 1
}

func (index centralHeatPumpIndex) listUses(kind, name string) int {
	count := 0
	for _, object := range index.doc.Objects {
		if !centralHeatPumpEqual(object.Type, "PlantLoop") && !centralHeatPumpEqual(object.Type, "CondenserLoop") {
			continue
		}
		fields := []int{12, 16}
		if kind == "ConnectorList" {
			fields = []int{13, 17}
		}
		for _, field := range fields {
			if centralHeatPumpEqual(centralHeatPumpField(object, field), name) {
				count++
			}
		}
	}
	return count
}

func (index centralHeatPumpIndex) branchListUses(name string) int {
	count := 0
	for _, object := range index.doc.Objects {
		if !centralHeatPumpEqual(object.Type, "BranchList") {
			continue
		}
		for field := 1; field < len(object.Fields); field++ {
			if centralHeatPumpEqual(centralHeatPumpField(object, field), name) {
				count++
			}
		}
	}
	return count
}

func (index centralHeatPumpIndex) connectorUses(kind, name string) int {
	count := 0
	for _, object := range index.doc.Objects {
		if !centralHeatPumpEqual(object.Type, "ConnectorList") {
			continue
		}
		for field := 1; field+1 < len(object.Fields); field += 2 {
			if centralHeatPumpEqual(centralHeatPumpField(object, field), kind) && centralHeatPumpEqual(centralHeatPumpField(object, field+1), name) {
				count++
			}
		}
	}
	return count
}

func (index centralHeatPumpIndex) owner(target Object, side string) (Object, bool) {
	var found Object
	count := 0
	for _, loop := range index.doc.Objects {
		if !centralHeatPumpEqual(loop.Type, "PlantLoop") && !centralHeatPumpEqual(loop.Type, "CondenserLoop") {
			continue
		}
		for _, candidateSide := range []string{"supply", "demand"} {
			base := 10
			if candidateSide == "demand" {
				base = 14
			}
			list, ok := index.unique("BranchList", centralHeatPumpField(loop, base+2))
			if !ok {
				continue
			}
			for field := 1; field < len(list.Fields); field++ {
				if !centralHeatPumpEqual(centralHeatPumpField(list, field), centralHeatPumpField(target, 0)) {
					continue
				}
				count++
				if candidateSide != side || !index.sideRoster(loop, base, target) {
					continue
				}
				found = loop
			}
		}
	}
	return found, count == 1 && found.Type != ""
}

func (index centralHeatPumpIndex) sideRoster(loop Object, base int, target Object) bool {
	if !index.loopNameUnique(centralHeatPumpField(loop, 0)) || !centralHeatPumpEqual(centralHeatPumpField(loop, 1), "Water") {
		return false
	}
	list, ok := index.unique("BranchList", centralHeatPumpField(loop, base+2))
	if !ok || len(list.Fields) < 4 || index.listUses("BranchList", centralHeatPumpField(list, 0)) != 1 {
		return false
	}
	connectors, ok := index.unique("ConnectorList", centralHeatPumpField(loop, base+3))
	if !ok || len(connectors.Fields) != 5 || index.listUses("ConnectorList", centralHeatPumpField(connectors, 0)) != 1 {
		return false
	}
	var splitter, mixer Object
	for field := 1; field <= 3; field += 2 {
		kind, name := centralHeatPumpField(connectors, field), centralHeatPumpField(connectors, field+1)
		connector, found := index.unique(kind, name)
		if !found || index.connectorUses(kind, name) != 1 {
			return false
		}
		switch strings.ToLower(kind) {
		case "connector:splitter":
			if splitter.Type != "" {
				return false
			}
			splitter = connector
		case "connector:mixer":
			if mixer.Type != "" {
				return false
			}
			mixer = connector
		default:
			return false
		}
	}
	if splitter.Type == "" || mixer.Type == "" || len(splitter.Fields) < 3 || len(splitter.Fields) != len(mixer.Fields) {
		return false
	}
	branches := map[string]Object{}
	for field := 1; field < len(list.Fields); field++ {
		name := centralHeatPumpField(list, field)
		key := strings.ToLower(name)
		branch, found := index.unique("Branch", name)
		if !found || branches[key].Type != "" || index.branchListUses(name) != 1 {
			return false
		}
		if _, _, valid := index.branchEndpoints(branch); !valid {
			return false
		}
		branches[key] = branch
	}
	inName, outName := strings.ToLower(centralHeatPumpField(splitter, 1)), strings.ToLower(centralHeatPumpField(mixer, 1))
	if inName == "" || outName == "" || inName == outName {
		return false
	}
	inlet, _, inOK := index.branchEndpoints(branches[inName])
	_, outlet, outOK := index.branchEndpoints(branches[outName])
	if !inOK || !outOK || !centralHeatPumpEqual(inlet, centralHeatPumpField(loop, base)) || !centralHeatPumpEqual(outlet, centralHeatPumpField(loop, base+1)) {
		return false
	}
	parallel := map[string]bool{}
	for field := 2; field < len(splitter.Fields); field++ {
		name := strings.ToLower(centralHeatPumpField(splitter, field))
		if name == "" || name == inName || name == outName || parallel[name] || branches[name].Type == "" {
			return false
		}
		parallel[name] = true
	}
	if len(parallel)+2 != len(branches) || !parallel[strings.ToLower(centralHeatPumpField(target, 0))] {
		return false
	}
	seen := map[string]bool{}
	for field := 2; field < len(mixer.Fields); field++ {
		name := strings.ToLower(centralHeatPumpField(mixer, field))
		if !parallel[name] || seen[name] {
			return false
		}
		seen[name] = true
	}
	return len(seen) == len(parallel)
}

// NativeCentralHeatPumpServicePort is a physical occurrence selector, not an
// allocator. Callers still need exact demand/air/terminal ownership and an
// observed, source-local service budget. A source-loop request never succeeds.
func NativeCentralHeatPumpServicePort(binding NativeCentralHeatPumpBinding, service, loopType, loopName string) (NativeCentralHeatPumpPort, bool) {
	if !binding.ReportingIdentityValid || !binding.NativeDefinitionValid || !binding.PortBindingsComplete {
		return NativeCentralHeatPumpPort{}, false
	}
	var port NativeCentralHeatPumpPort
	switch strings.ToLower(strings.TrimSpace(service)) {
	case "cooling":
		port = binding.Cooling
	case "heating":
		port = binding.Heating
	default:
		return NativeCentralHeatPumpPort{}, false
	}
	if !port.Bound || !centralHeatPumpEqual(port.Loop.Type, loopType) || !centralHeatPumpEqual(port.Loop.Name, loopName) {
		return NativeCentralHeatPumpPort{}, false
	}
	return port, true
}
