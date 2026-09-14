package idf

import (
	"sort"
	"strings"
)

const nativeWindowACType = "ZoneHVAC:WindowAirConditioner"

// NativeWindowACBinding is an original-document, exclusively Zone-owned
// Fan:OnOff + SingleSpeed DX package. It is not a general WindowAC capability
// census: other native fan/coil combinations retain their existing service
// representation but cannot borrow this reviewed direct-consumption roster.
type NativeWindowACBinding struct {
	Parent, Mixer, Fan, Coil ComponentRef
	ZoneName                 string
}

type windowACReference struct{ object, field int }
type windowACOriginalIndex struct {
	doc        Document
	objects    map[string][]int
	references map[string][]windowACReference
}

func windowACField(object Object, field int) string {
	if field < 0 || field >= len(object.Fields) {
		return ""
	}
	return strings.TrimSpace(object.Fields[field].Value)
}

func hasNativeWindowAC(doc Document) bool {
	for _, object := range doc.Objects {
		if strings.EqualFold(object.Type, nativeWindowACType) {
			return true
		}
	}
	return false
}

func windowACReviewedSchema(doc Document) bool {
	count, reviewed := 0, false
	for _, object := range doc.Objects {
		if strings.EqualFold(object.Type, "Version") {
			count++
			reviewed = windowACField(object, 0) == "25.1" || windowACField(object, 0) == "25.1.0"
		}
	}
	return count == 1 && reviewed
}

func windowACKey(objectType, name string) string {
	return strings.ToLower(strings.TrimSpace(objectType)) + "\x00" + strings.ToLower(strings.TrimSpace(name))
}

func newWindowACOriginalIndex(doc Document) windowACOriginalIndex {
	index := windowACOriginalIndex{doc: doc, objects: map[string][]int{}, references: map[string][]windowACReference{}}
	for position, object := range doc.Objects {
		key := windowACKey(object.Type, windowACField(object, 0))
		index.objects[key] = append(index.objects[key], position)
		for field := 0; field+1 < len(object.Fields); field++ {
			key := windowACKey(windowACField(object, field), windowACField(object, field+1))
			index.references[key] = append(index.references[key], windowACReference{position, field})
		}
	}
	return index
}

func (index windowACOriginalIndex) unique(objectType, name string) (Object, bool) {
	positions := index.objects[windowACKey(objectType, name)]
	if name == "" || len(positions) != 1 {
		return Object{}, false
	}
	return index.doc.Objects[positions[0]], true
}

func (index windowACOriginalIndex) child(parentPosition, field int, objectType string) (Object, bool) {
	parent := index.doc.Objects[parentPosition]
	if !strings.EqualFold(windowACField(parent, field), objectType) {
		return Object{}, false
	}
	name := windowACField(parent, field+1)
	child, found := index.unique(objectType, name)
	refs := index.references[windowACKey(objectType, name)]
	if !found || len(refs) != 1 || refs[0] != (windowACReference{parentPosition, field}) {
		return Object{}, false
	}
	return child, true
}

// ReportData keys do not contain object type. Count the full native namespace,
// including disconnected objects not registered as direct-consumption types.
func (index windowACOriginalIndex) reportingNameUnique(name, namespace string) bool {
	count := 0
	for _, object := range index.doc.Objects {
		kind := strings.ToLower(strings.TrimSpace(object.Type))
		matches := namespace == "fan" && windowACFanReportingType(kind) || namespace == "coil" && windowACCoolingReportingType(kind)
		if matches && strings.EqualFold(windowACField(object, 0), name) {
			count++
		}
	}
	return count == 1
}

func windowACFanReportingType(kind string) bool {
	// v25.1 Fans.cc and Fans:SystemModel share Fan Electricity Energy.
	// FanPerformance metadata is not a fan reporting entity.
	switch kind {
	case "fan:onoff", "fan:constantvolume", "fan:variablevolume", "fan:systemmodel", "fan:zoneexhaust", "fan:componentmodel":
		return true
	default:
		return false
	}
}

func windowACCoolingReportingType(kind string) bool {
	// v25.1 DXCoils.cc, VariableSpeedCoils.cc, WaterToAirHeatPump*.cc and
	// Coils/CoilCoolingDX.cc / PackagedThermalStorageCoil.cc output registration:
	// these cooling coils report the
	// primary electricity name and/or the separate cooling crankcase name.
	// Heat-pump water heaters also expose that crankcase name. Heating:Fuel
	// and water coils do not share these reporting identities.
	switch kind {
	case "coil:cooling:dx:singlespeed", "coil:cooling:dx:twospeed", "coil:cooling:dx:multispeed", "coil:cooling:dx:twostagewithhumiditycontrolmode",
		"coil:cooling:dx", "coil:cooling:dx:variablespeed", "coil:cooling:dx:singlespeed:thermalstorage", "coil:cooling:watertoairheatpump:variablespeedequationfit",
		"coil:cooling:watertoairheatpump:equationfit", "coil:cooling:watertoairheatpump:parameterestimation",
		"coil:waterheating:airtowaterheatpump:pumped", "coil:waterheating:airtowaterheatpump:wrapped", "coil:waterheating:airtowaterheatpump:variablespeed":
		return true
	default:
		return false
	}
}

func (index windowACOriginalIndex) nodeSelector(selector string) ([]string, bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, false
	}
	positions := index.objects[windowACKey("NodeList", selector)]
	if len(positions) == 0 {
		return []string{selector}, true
	}
	if len(positions) != 1 {
		return nil, false
	}
	object := index.doc.Objects[positions[0]]
	var nodes []string
	seen := map[string]bool{}
	for field := 1; field < len(object.Fields); field++ {
		node := windowACField(object, field)
		key := strings.ToLower(node)
		if node == "" {
			continue
		}
		// Native NodeInputManager permits only literal members. A member that
		// is itself any NodeList name (including this list) is an input error,
		// not a recursively expandable selector.
		if seen[key] || len(index.objects[windowACKey("NodeList", node)]) != 0 {
			return nil, false
		}
		seen[key] = true
		nodes = append(nodes, node)
	}
	return nodes, len(nodes) > 0
}

func windowACNodeIn(nodes []string, node string) bool {
	for _, candidate := range nodes {
		if strings.EqualFold(candidate, node) {
			return true
		}
	}
	return false
}

// Even a malformed duplicate NodeList on another connection is evidence of a
// competing owner if any of its declarations contains the relevant air port.
func (index windowACOriginalIndex) nodeSelectorMentions(selector, node string) bool {
	selector, node = strings.TrimSpace(selector), strings.TrimSpace(node)
	if selector == "" || node == "" {
		return false
	}
	if strings.EqualFold(selector, node) {
		return true
	}
	for _, position := range index.objects[windowACKey("NodeList", selector)] {
		object := index.doc.Objects[position]
		for field := 1; field < len(object.Fields); field++ {
			if strings.EqualFold(windowACField(object, field), node) {
				return true
			}
		}
	}
	return false
}

func (index windowACOriginalIndex) owner(parentPosition int) (string, bool) {
	parent := index.doc.Objects[parentPosition]
	key := windowACKey(parent.Type, windowACField(parent, 0))
	refs := index.references[key]
	if len(index.objects[key]) != 1 || len(refs) != 1 {
		return "", false
	}
	ref := refs[0]
	list := index.doc.Objects[ref.object]
	// Native EquipmentList fields: name, load-distribution scheme, then six
	// fields per equipment. Do not accept a typed token at an unrelated slot.
	if !strings.EqualFold(list.Type, "ZoneHVAC:EquipmentList") || ref.field < 2 || (ref.field-2)%6 != 0 {
		return "", false
	}
	if _, valid := index.unique(list.Type, windowACField(list, 0)); !valid {
		return "", false
	}
	var connection Object
	connectionCount := 0
	for _, object := range index.doc.Objects {
		if strings.EqualFold(object.Type, "ZoneHVAC:EquipmentConnections") && strings.EqualFold(windowACField(object, 1), windowACField(list, 0)) {
			connection, connectionCount = object, connectionCount+1
		}
	}
	if connectionCount != 1 || windowACField(connection, 4) == "" {
		return "", false
	}
	zone, valid := index.unique("Zone", windowACField(connection, 0))
	if !valid {
		return "", false
	}
	zoneName := windowACField(zone, 0)
	inlets, inletOK := index.nodeSelector(windowACField(connection, 2))
	exhausts, exhaustOK := index.nodeSelector(windowACField(connection, 3))
	if !inletOK || !exhaustOK || !windowACNodeIn(inlets, windowACField(parent, 5)) || !windowACNodeIn(exhausts, windowACField(parent, 4)) {
		return "", false
	}
	zoneConnections := 0
	for _, object := range index.doc.Objects {
		if !strings.EqualFold(object.Type, "ZoneHVAC:EquipmentConnections") {
			continue
		}
		if strings.EqualFold(windowACField(object, 0), zoneName) {
			zoneConnections++
			continue
		}
		for _, selectorField := range []int{2, 3} {
			if index.nodeSelectorMentions(windowACField(object, selectorField), windowACField(parent, 4)) ||
				index.nodeSelectorMentions(windowACField(object, selectorField), windowACField(parent, 5)) {
				return "", false
			}
		}
	}
	return zoneName, zoneConnections == 1
}

func (index windowACOriginalIndex) outdoorNodeUnique(name string) bool {
	// OutdoorAir:NodeList accepts Node or NodeList selectors and unions their
	// expanded nodes. Repetition across outdoor lists is legal; an explicit
	// OutdoorAir:Node must not overlap any list or another explicit single.
	// These are v25.1 OutAirNodeManager.cc lines 199-293 semantics.
	listed, singles := false, 0
	for _, object := range index.doc.Objects {
		switch strings.ToLower(strings.TrimSpace(object.Type)) {
		case "outdoorair:node":
			nodes, valid := index.nodeSelector(windowACField(object, 0))
			if !valid || len(nodes) != 1 {
				return false
			}
			if windowACNodeIn(nodes, name) {
				singles++
			}
		case "outdoorair:nodelist":
			if len(object.Fields) == 0 || windowACField(object, 0) == "" {
				return false
			}
			for _, field := range object.Fields {
				if strings.TrimSpace(field.Value) == "" {
					continue
				}
				nodes, valid := index.nodeSelector(field.Value)
				if !valid {
					return false
				}
				listed = listed || windowACNodeIn(nodes, name)
			}
		}
	}
	return listed && singles == 0 || !listed && singles == 1
}

func windowACDistinctNodes(nodes ...string) bool {
	seen := map[string]bool{}
	for _, node := range nodes {
		key := strings.ToLower(strings.TrimSpace(node))
		if key == "" || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func windowACComponent(object Object, inlet, outlet string) ComponentRef {
	name := windowACField(object, 0)
	return ComponentRef{ID: componentRefID(object.Type, name, object.Index), ObjectType: object.Type, ObjectName: name, ObjectIndex: object.Index, DisplayName: name, InletNode: inlet, OutletNode: outlet}
}

// ResolveNativeWindowACBindings does not call AnalyzeHVAC, so the same exact
// native proof can gate service paths without recursively analyzing the model.
func ResolveNativeWindowACBindings(doc Document) []NativeWindowACBinding {
	return resolveNativeWindowACBindings(doc, true)
}

func resolveNativeWindowACBindings(doc Document, requireReportingIdentity bool) []NativeWindowACBinding {
	if !hasNativeWindowAC(doc) || !windowACReviewedSchema(doc) {
		return nil
	}
	index := newWindowACOriginalIndex(doc)
	var bindings []NativeWindowACBinding
	for position, parent := range doc.Objects {
		if !strings.EqualFold(parent.Type, nativeWindowACType) || len(parent.Fields) < 15 {
			continue
		}
		zone, owned := index.owner(position)
		mixer, mixerOK := index.child(position, 6, "OutdoorAir:Mixer")
		fan, fanOK := index.child(position, 8, "Fan:OnOff")
		coil, coilOK := index.child(position, 10, "Coil:Cooling:DX:SingleSpeed")
		if !owned || !mixerOK || !fanOK || !coilOK {
			continue
		}
		if requireReportingIdentity && (!index.reportingNameUnique(windowACField(fan, 0), "fan") || !index.reportingNameUnique(windowACField(coil, 0), "coil")) {
			continue
		}
		parentIn, parentOut := windowACField(parent, 4), windowACField(parent, 5)
		mixed, outdoor, relief, returnAir := windowACField(mixer, 1), windowACField(mixer, 2), windowACField(mixer, 3), windowACField(mixer, 4)
		fanIn, fanOut := windowACField(fan, 7), windowACField(fan, 8)
		coilIn, coilOut := windowACField(coil, 8), windowACField(coil, 9)
		if !strings.EqualFold(parentIn, returnAir) || !windowACDistinctNodes(parentIn, parentOut, mixed, outdoor, relief) || !index.outdoorNodeUnique(outdoor) {
			continue
		}
		connected := false
		switch strings.ToLower(windowACField(parent, 13)) {
		case "blowthrough":
			connected = strings.EqualFold(mixed, fanIn) && strings.EqualFold(fanOut, coilIn) && strings.EqualFold(coilOut, parentOut) && windowACDistinctNodes(mixed, fanOut, parentOut, parentIn, outdoor, relief)
		case "drawthrough":
			connected = strings.EqualFold(mixed, coilIn) && strings.EqualFold(coilOut, fanIn) && strings.EqualFold(fanOut, parentOut) && windowACDistinctNodes(mixed, coilOut, parentOut, parentIn, outdoor, relief)
		}
		if !connected {
			continue
		}
		bindings = append(bindings, NativeWindowACBinding{Parent: windowACComponent(parent, parentIn, parentOut), Mixer: windowACComponent(mixer, returnAir, mixed), Fan: windowACComponent(fan, fanIn, fanOut), Coil: windowACComponent(coil, coilIn, coilOut), ZoneName: zone})
	}
	sort.Slice(bindings, func(i, j int) bool {
		return windowACKey(bindings[i].ZoneName, bindings[i].Parent.ObjectName) < windowACKey(bindings[j].ZoneName, bindings[j].Parent.ObjectName)
	})
	return bindings
}

// This gate runs before service-path IDs/deduplication. It verifies the reviewed
// native chain strictly without withdrawing other explicitly native WindowAC
// variants from the existing service model. Those variants never become direct
// consumption targets through ResolveNativeWindowACBindings.
func buildHVACWindowACPathGate(doc Document) func(ZoneServicePath) (ZoneServicePath, bool) {
	if !hasNativeWindowAC(doc) {
		return func(path ZoneServicePath) (ZoneServicePath, bool) { return path, true }
	}
	index := newWindowACOriginalIndex(doc)
	reviewedSchema := windowACReviewedSchema(doc)
	bindings := map[string]NativeWindowACBinding{}
	// A cross-type SQL reporting-key collision prevents direct consumption,
	// but does not erase an otherwise unambiguous typed physical air route.
	for _, binding := range resolveNativeWindowACBindings(doc, false) {
		bindings[windowACKey(binding.Parent.ObjectType, binding.Parent.ObjectName)] = binding
	}
	return func(path ZoneServicePath) (ZoneServicePath, bool) {
		if !strings.EqualFold(path.Delivery.ObjectType, nativeWindowACType) {
			return path, true
		}
		key := windowACKey(path.Delivery.ObjectType, path.Delivery.ObjectName)
		positions := index.objects[key]
		if len(positions) != 1 {
			return path, false
		}
		position := positions[0]
		parent := doc.Objects[position]
		if len(parent.Fields) < 15 || path.Delivery.ObjectIndex != parent.Index || path.Delivery.ID != componentRefID(parent.Type, windowACField(parent, 0), parent.Index) ||
			!windowACDistinctNodes(windowACField(parent, 4), windowACField(parent, 5)) {
			return path, false
		}
		fanType, coilType := strings.ToLower(windowACField(parent, 8)), strings.ToLower(windowACField(parent, 10))
		if fanType != "fan:onoff" && fanType != "fan:constantvolume" && fanType != "fan:systemmodel" {
			return path, false
		}
		if coilType != "coil:cooling:dx:singlespeed" && coilType != "coil:cooling:dx:variablespeed" && coilType != "coilsystem:cooling:dx:heatexchangerassisted" {
			return path, false
		}
		placement := strings.ToLower(windowACField(parent, 13))
		if placement != "blowthrough" && placement != "drawthrough" {
			return path, false
		}
		owner, ownerOK := index.owner(position)
		if !ownerOK || path.SpaceName != "" || path.ServedSubject.SpaceName != "" || strings.EqualFold(path.ServedSubject.Kind, "space") ||
			path.AirLoop != nil || path.PlantLoop != nil || path.CondenserLoop != nil || path.RefrigerantSystem != nil || path.PathType != "direct_zone_air" {
			return path, false
		}
		if path.ZoneName == "" && path.ServedSubject.ZoneName == "" {
			return path, false
		}
		for _, declared := range []string{path.ZoneName, path.ServedSubject.ZoneName} {
			if strings.TrimSpace(declared) != "" && !strings.EqualFold(strings.TrimSpace(declared), owner) {
				return path, false
			}
		}
		for _, child := range []struct {
			field int
			kind  string
		}{{6, "OutdoorAir:Mixer"}, {8, windowACField(parent, 8)}, {10, windowACField(parent, 10)}} {
			if _, valid := index.child(position, child.field, child.kind); !valid {
				return path, false
			}
		}
		if reviewedSchema && fanType == "fan:onoff" && coilType == "coil:cooling:dx:singlespeed" {
			binding, valid := bindings[key]
			if !valid || !strings.EqualFold(binding.ZoneName, owner) {
				return path, false
			}
			path.Conditioning = appendUniqueComponentRef(path.Conditioning, binding.Coil)
			path.TraceIDs = appendUniqueStrings(path.TraceIDs, hvacRuleComponentReferencesComponent)
		}
		// Heating/no-load sequencing and arbitrary object names cannot turn a
		// native cooling-only WindowAC into the neighboring baseboard heater.
		path.ServiceKind = "cooling"
		path.SourceSystem = localSourceSystemForDelivery("window_ac", "cooling")
		return path, true
	}
}
