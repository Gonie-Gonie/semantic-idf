package simulation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const energyPathRadiantConstantFlowType = "ZoneHVAC:LowTemperatureRadiant:ConstantFlow"

// Component is the physical original object, not an Output:Variable index.
// These targets establish ownership only. They do not equate surface-source
// thermal energy with Zone-air sensible energy or prescribe a multiplier.
type energyPathRadiantLoadTarget struct {
	Component   idf.ComponentRef
	KeyValue    string
	ZoneName    string
	SurfaceName string
}

func energyPathIsRadiantLoadVariable(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "zone radiant hvac cooling energy", "zone radiant hvac cooling rate",
		"zone radiant hvac heating energy", "zone radiant hvac heating rate":
		return true
	default:
		return false
	}
}

func energyPathRadiantField(object idf.Object, field int) string {
	if field < 0 || field >= len(object.Fields) {
		return ""
	}
	return strings.TrimSpace(object.Fields[field].Value)
}

func energyPathRadiantName(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

type energyPathRadiantReference struct{ object, field int }

// The bounded native contract is the Design-Object ConstantFlow schema in the
// IDF field catalog (Zone=3, surface=4, heating ports=10/11, cooling=16/17).
// Legacy inline-design, VariableFlow, Electric and SurfaceGroup targets are not
// guessed. The existing HVAC analyzer supplies the typed EquipmentList owner;
// raw-document uniqueness checks below prevent its deduplication from hiding
// duplicate, disconnected or contradictory original declarations.
func energyPathRadiantLoadTargets(doc idf.Document) []energyPathRadiantLoadTarget {
	hasNative := false
	for _, object := range doc.Objects {
		if strings.EqualFold(strings.TrimSpace(object.Type), energyPathRadiantConstantFlowType) {
			hasNative = true
			break
		}
	}
	if !hasNative {
		return nil // No additional HVAC analysis for non-radiant fixtures.
	}
	objects := map[string][]int{}
	references := map[string][]energyPathRadiantReference{}
	emitterNames := map[string]int{}
	surfaceUsers := map[string]int{}
	key := func(objectType, name string) string {
		return energyPathRadiantName(objectType) + "|" + energyPathRadiantName(name)
	}
	for i, object := range doc.Objects {
		objects[key(object.Type, energyPathRadiantField(object, 0))] = append(objects[key(object.Type, energyPathRadiantField(object, 0))], i)
		for field := 0; field+1 < len(object.Fields); field++ {
			ref := key(energyPathRadiantField(object, field), energyPathRadiantField(object, field+1))
			references[ref] = append(references[ref], energyPathRadiantReference{i, field})
		}
		switch energyPathRadiantName(object.Type) {
		case "zonehvac:lowtemperatureradiant:constantflow", "zonehvac:lowtemperatureradiant:variableflow":
			emitterNames[energyPathRadiantName(energyPathRadiantField(object, 0))]++
			surfaceUsers[energyPathRadiantName(energyPathRadiantField(object, 4))]++
		case "zonehvac:lowtemperatureradiant:electric":
			emitterNames[energyPathRadiantName(energyPathRadiantField(object, 0))]++
			surfaceUsers[energyPathRadiantName(energyPathRadiantField(object, 3))]++
		case "zonehvac:lowtemperatureradiant:surfacegroup":
			// Even an unsupported group cannot silently share a direct surface.
			for field := 1; field < len(object.Fields); field += 2 {
				surfaceUsers[energyPathRadiantName(energyPathRadiantField(object, field))]++
			}
		}
	}
	unique := func(objectType, name string) (idf.Object, bool) {
		positions := objects[key(objectType, name)]
		if strings.TrimSpace(name) == "" || len(positions) != 1 {
			return idf.Object{}, false
		}
		return doc.Objects[positions[0]], true
	}
	typedOwners := map[string][]string{}
	for _, relation := range idf.AnalyzeHVAC(doc).ZoneRelations {
		for _, component := range relation.ZoneEquipment {
			if strings.EqualFold(component.ObjectType, energyPathRadiantConstantFlowType) {
				typedOwners[key(component.ObjectType, component.ObjectName)] = append(typedOwners[key(component.ObjectType, component.ObjectName)], relation.ZoneName)
			}
		}
	}
	var out []energyPathRadiantLoadTarget
	for _, object := range doc.Objects {
		if !strings.EqualFold(strings.TrimSpace(object.Type), energyPathRadiantConstantFlowType) || len(object.Fields) < 22 {
			continue
		}
		name := energyPathRadiantField(object, 0)
		objectKey := key(object.Type, name)
		if len(objects[objectKey]) != 1 || emitterNames[energyPathRadiantName(name)] != 1 {
			continue
		}
		design, designOK := unique(energyPathRadiantConstantFlowType+":Design", energyPathRadiantField(object, 1))
		zone, zoneOK := unique("Zone", energyPathRadiantField(object, 3))
		surfaceName := energyPathRadiantField(object, 4)
		surface, surfaceOK := unique("BuildingSurface:Detailed", surfaceName)
		if !designOK || len(design.Fields) < 2 || !zoneOK || !surfaceOK ||
			surfaceUsers[energyPathRadiantName(surfaceName)] != 1 ||
			len(objects[key("ZoneHVAC:LowTemperatureRadiant:SurfaceGroup", surfaceName)]) != 0 {
			continue
		}
		zoneName := energyPathRadiantField(zone, 0)
		if !strings.EqualFold(zoneName, energyPathRadiantField(surface, 3)) ||
			len(typedOwners[objectKey]) != 1 || !strings.EqualFold(typedOwners[objectKey][0], zoneName) {
			continue
		}
		// The reviewed surface schema has a separate optional Space owner.
		if spaceName := energyPathRadiantField(surface, 4); spaceName != "" {
			space, ok := unique("Space", spaceName)
			if !ok || !strings.EqualFold(energyPathRadiantField(space, 1), zoneName) {
				continue
			}
		}
		ports := map[string]bool{}
		for _, field := range []int{10, 11, 16, 17} {
			port := energyPathRadiantName(energyPathRadiantField(object, field))
			if port != "" {
				ports[port] = true
			}
		}
		if len(ports) != 4 {
			continue // Do not interpret an inline legacy schema as this variant.
		}
		listPosition, listCount, valid := -1, 0, true
		branchPorts := map[string]bool{}
		for _, ref := range references[objectKey] {
			parent := doc.Objects[ref.object]
			switch energyPathRadiantName(parent.Type) {
			case "zonehvac:equipmentlist":
				listPosition, listCount = ref.object, listCount+1
			case "branch":
				// Each physical parent legitimately occurs on two different water
				// branches. These are connections, not extra Zone owners.
				inlet, outlet := energyPathRadiantField(parent, ref.field+2), energyPathRadiantField(parent, ref.field+3)
				pair := energyPathRadiantName(inlet) + "|" + energyPathRadiantName(outlet)
				matches := false
				for _, field := range []int{10, 16} {
					matches = matches || strings.EqualFold(inlet, energyPathRadiantField(object, field)) && strings.EqualFold(outlet, energyPathRadiantField(object, field+1))
				}
				if !matches || branchPorts[pair] || len(objects[key(parent.Type, energyPathRadiantField(parent, 0))]) != 1 {
					valid = false
				}
				branchPorts[pair] = true
			default:
				valid = false
			}
		}
		if !valid || listCount != 1 {
			continue
		}
		list := doc.Objects[listPosition]
		listName := energyPathRadiantField(list, 0)
		if _, ok := unique("ZoneHVAC:EquipmentList", listName); !ok {
			continue
		}
		listConnections, zoneConnections := 0, 0
		for _, connection := range doc.Objects {
			if !strings.EqualFold(strings.TrimSpace(connection.Type), "ZoneHVAC:EquipmentConnections") {
				continue
			}
			connectionZone := energyPathRadiantField(connection, 0)
			if strings.EqualFold(connectionZone, zoneName) {
				zoneConnections++
			}
			if strings.EqualFold(energyPathRadiantField(connection, 1), listName) {
				listConnections++
				valid = valid && strings.EqualFold(connectionZone, zoneName)
			}
		}
		if !valid || listConnections != 1 || zoneConnections != 1 {
			continue
		}
		out = append(out, energyPathRadiantLoadTarget{KeyValue: name, ZoneName: zoneName, SurfaceName: energyPathRadiantField(surface, 0),
			Component: idf.ComponentRef{ID: fmt.Sprintf("component:%d", object.Index), ObjectIndex: object.Index,
				ObjectType: object.Type, ObjectName: name, DisplayName: name}})
	}
	sort.Slice(out, func(i, j int) bool {
		return energyPathRadiantName(out[i].KeyValue) < energyPathRadiantName(out[j].KeyValue)
	})
	return out
}
