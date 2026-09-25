package idf

import (
	"fmt"
	"strings"
)

// NativeHeatOnlyFurnaceCohort is allocation eligibility, not service visibility.
// True sibling paths remain visible when any other recipient is unresolved.
// Unknown ownership is retained as an incomplete cohort, never dropped.
type NativeHeatOnlyFurnaceCohort struct {
	System   ComponentRef
	AirLoop  *LoopRef
	Complete bool
}

// ResolveNativeHeatOnlyFurnaceCohorts checks the whole native 25.1 CV-NoReheat
// splitter/mixer roster against exact wrapper-qualified service paths. It does
// not derive recipients from the surviving path count or any energy quantity.
func ResolveNativeHeatOnlyFurnaceCohorts(doc Document, summaries []ZoneServiceSummary) []NativeHeatOnlyFurnaceCohort {
	var positions []int
	for n, object := range doc.Objects {
		if strings.EqualFold(object.Type, nativeHeatOnlyFurnaceType) {
			positions = append(positions, n)
		}
	}
	if len(positions) == 0 {
		return nil
	}
	index := newWindowACOriginalIndex(doc)
	var out []NativeHeatOnlyFurnaceCohort
	for _, position := range positions {
		parent := doc.Objects[position]
		cohort := NativeHeatOnlyFurnaceCohort{System: ComponentRef{ID: fmt.Sprintf("component:%d", parent.Index), ObjectIndex: parent.Index, ObjectType: parent.Type, ObjectName: windowACField(parent, 0)}}
		unique, ok := index.unique(parent.Type, windowACField(parent, 0))
		if !ok || unique.Index != parent.Index {
			out = append(out, cohort)
			continue
		}
		loop, owned := hvacHeatOnlyFurnaceBranchOwner(index, position)
		if owned {
			cohort.AirLoop = &LoopRef{Type: loop.Type, Name: windowACField(loop, 0), ObjectIndex: loop.Index}
		}
		if owned && windowACReviewedSchema(doc) {
			cohort.Complete = hvacHeatOnlyFurnaceWholeRecipients(index, parent, loop, summaries)
		}
		out = append(out, cohort)
	}
	return out
}

func hvacHeatOnlyFurnaceAirPath(index windowACOriginalIndex, kind, childKind, node string) (Object, bool) {
	position, count := -1, 0
	for n, object := range index.doc.Objects {
		if strings.EqualFold(object.Type, kind) && node != "" && strings.EqualFold(windowACField(object, 1), node) {
			position, count = n, count+1
		}
	}
	if count != 1 {
		return Object{}, false
	}
	path := index.doc.Objects[position]
	if _, ok := index.unique(kind, windowACField(path, 0)); !ok || len(path.Fields) != 4 {
		return Object{}, false
	}
	child, ok := index.child(position, 2, childKind)
	return child, ok && strings.EqualFold(windowACField(child, 1), node)
}

func hvacHeatOnlyFurnaceWholeRecipients(index windowACOriginalIndex, parent, loop Object, summaries []ZoneServiceSummary) bool {
	// A second loop cannot borrow the same physical demand boundary.
	for _, field := range []int{7, 8} {
		count, node := 0, windowACField(loop, field)
		for _, object := range index.doc.Objects {
			if strings.EqualFold(object.Type, "AirLoopHVAC") && node != "" && strings.EqualFold(windowACField(object, field), node) {
				count++
			}
		}
		if count != 1 {
			return false
		}
	}
	splitter, supplyOK := hvacHeatOnlyFurnaceAirPath(index, "AirLoopHVAC:SupplyPath", "AirLoopHVAC:ZoneSplitter", windowACField(loop, 8))
	mixer, returnOK := hvacHeatOnlyFurnaceAirPath(index, "AirLoopHVAC:ReturnPath", "AirLoopHVAC:ZoneMixer", windowACField(loop, 7))
	if !supplyOK || !returnOK || len(splitter.Fields) < 3 || len(splitter.Fields) != len(mixer.Fields) {
		return false
	}
	forward, back := map[string]bool{}, map[string]bool{}
	for _, item := range []struct {
		object Object
		nodes  map[string]bool
	}{{splitter, forward}, {mixer, back}} {
		for field := 2; field < len(item.object.Fields); field++ {
			node := strings.ToLower(windowACField(item.object, field))
			if node == "" || item.nodes[node] || strings.EqualFold(node, windowACField(item.object, 1)) {
				return false
			}
			item.nodes[node] = true
		}
	}
	seen, zones := map[string]bool{}, map[string]bool{}
	for _, summary := range summaries {
		for _, path := range summary.Paths {
			owned := false
			for _, component := range path.Conditioning {
				owned = owned || component.ObjectIndex == parent.Index && strings.EqualFold(component.ObjectType, parent.Type) && strings.EqualFold(component.ObjectName, windowACField(parent, 0))
			}
			if !owned {
				continue
			}
			if path.ServiceKind != "heating" || path.PathType != "central_air" || path.SpaceName != "" || path.AirLoop == nil || path.AirLoop.ObjectIndex != loop.Index || !strings.EqualFold(path.AirLoop.Type, loop.Type) || !strings.EqualFold(path.AirLoop.Name, windowACField(loop, 0)) || path.PlantLoop != nil || path.CondenserLoop != nil || path.RefrigerantSystem != nil {
				return false
			}
			terminal, ok := index.unique(path.Delivery.ObjectType, path.Delivery.ObjectName)
			if !ok || !strings.EqualFold(terminal.Type, "AirTerminal:SingleDuct:ConstantVolume:NoReheat") || terminal.Index != path.Delivery.ObjectIndex {
				return false
			}
			inlet, zone := strings.ToLower(windowACField(terminal, 2)), strings.ToLower(strings.TrimSpace(path.ZoneName))
			connection, ok := index.unique("ZoneHVAC:EquipmentConnections", path.ZoneName)
			if !ok || zone == "" || zones[zone] || !forward[inlet] || seen[inlet] {
				return false
			}
			outlets, ok := index.nodeSelector(windowACField(connection, 5))
			if !ok || len(outlets) != 1 || !back[strings.ToLower(outlets[0])] {
				return false
			}
			delete(back, strings.ToLower(outlets[0]))
			seen[inlet], zones[zone] = true, true
		}
	}
	return len(seen) == len(forward) && len(back) == 0 && zones[strings.ToLower(windowACField(parent, 7))]
}
