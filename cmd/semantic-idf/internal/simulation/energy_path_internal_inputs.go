package simulation

import (
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// A missing People/Lights/equipment output in a plenum is not an unknown
// contribution to an otherwise complete Building total when the executed model
// has no object of that family there. Prove that omission from typed model
// ownership, never from a missing SQL dictionary or a guessed zero.
func energyPathInternalInputOwners(geometry idf.GeometryReport, documents ...idf.Document) map[string]map[string]bool {
	if len(documents) == 0 {
		return nil
	}
	doc := documents[0]
	targets := map[string]map[string]bool{}
	zoneKeys, spaceKeys := map[string]bool{}, map[string]bool{}
	addTarget := func(name string, zones map[string]bool) {
		key := normalizePurposeToken(name)
		if key == "" {
			return
		}
		if _, duplicate := targets[key]; duplicate {
			targets[key] = nil // ambiguous Zone/Space/List identities fail closed
			return
		}
		targets[key] = zones
	}
	for _, zone := range geometry.Zones {
		key := normalizePurposeToken(zone.Name)
		zoneKeys[key] = true
		addTarget(zone.Name, map[string]bool{key: true})
	}
	for _, space := range geometry.Spaces {
		key := normalizePurposeToken(space.ZoneName)
		if targets[key][key] {
			spaceKeys[normalizePurposeToken(space.Name)] = true
			addTarget(space.Name, map[string]bool{key: true})
		}
	}
	for _, object := range doc.Objects {
		if !strings.EqualFold(object.Type, "ZoneList") && !strings.EqualFold(object.Type, "SpaceList") || len(object.Fields) < 2 {
			continue
		}
		zones := map[string]bool{}
		valid := true
		for _, field := range object.Fields[1:] {
			key := normalizePurposeToken(field.Value)
			members := targets[key]
			if len(members) != 1 || strings.EqualFold(object.Type, "ZoneList") && !zoneKeys[key] || strings.EqualFold(object.Type, "SpaceList") && !spaceKeys[key] {
				valid = false
			}
			for key := range members {
				zones[key] = true
			}
		}
		if !valid {
			zones = nil
		}
		addTarget(purposeObjectName(object), zones)
	}
	owners := map[string]map[string]bool{}
	for _, definition := range energyPathThermalInputDefinitions {
		if !definition.optional {
			continue
		}
		owners[definition.kind] = map[string]bool{}
		valid := true
		for _, object := range doc.Objects {
			if !strings.EqualFold(object.Type, definition.objectType) {
				continue
			}
			targetIndex := 1
			if strings.EqualFold(object.Type, "OtherEquipment") {
				targetIndex = 2 // native Fuel Type precedes Zone/Space ownership
			}
			if len(object.Fields) <= targetIndex {
				valid = false
				continue
			}
			zones := targets[normalizePurposeToken(object.Fields[targetIndex].Value)]
			if len(zones) == 0 {
				valid = false
			}
			for key := range zones {
				owners[definition.kind][key] = true
			}
		}
		if !valid {
			owners[definition.kind] = nil
		}
	}
	return owners
}
