package simulation

import (
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type comfortOwnership struct {
	zones  map[string]string
	people map[string]string
}

// People output keys belong to the executed model, not to similarly named
// zones. EnergyPlus expands a ZoneList/SpaceList or a multi-space Zone with
// "<Space Name> <People Name>" keys (People object IDD memo).
func buildComfortOwnership(doc *idf.Document) comfortOwnership {
	index := comfortOwnership{zones: map[string]string{}, people: map[string]string{}}
	if doc == nil {
		return index
	}
	spaces := map[string]string{}
	spaceNames := map[string]string{}
	zoneSpaces := map[string][]string{}
	lists := map[string][]string{}
	for _, obj := range doc.Objects {
		name := purposeObjectName(obj)
		key := normalizePurposeToken(name)
		switch normalizePurposeToken(obj.Type) {
		case "zone":
			index.zones[key] = name
		case "space":
			zone := energyObjectStringField(obj, 1, "zone name")
			spaces[key], spaceNames[key] = zone, name
			zoneSpaces[normalizePurposeToken(zone)] = append(zoneSpaces[normalizePurposeToken(zone)], name)
		case "zonelist", "spacelist":
			for i := 1; i < len(obj.Fields); i++ {
				lists[key] = append(lists[key], strings.TrimSpace(obj.Fields[i].Value))
			}
		}
	}
	add := func(name, zone string) {
		zone = index.zones[normalizePurposeToken(zone)]
		if name == "" || zone == "" {
			return
		}
		key := normalizePurposeToken(name)
		if previous, exists := index.people[key]; exists && !strings.EqualFold(previous, zone) {
			index.people[key] = "" // Ambiguous keys must never be assigned by order.
			return
		}
		index.people[key] = zone
	}
	for _, obj := range doc.Objects {
		if !strings.EqualFold(obj.Type, "People") {
			continue
		}
		name := purposeObjectName(obj)
		target := normalizePurposeToken(energyObjectStringField(obj, 1, "zone or zonelist"))
		if zone := index.zones[target]; zone != "" {
			members := zoneSpaces[target]
			if len(members) <= 1 {
				add(name, zone)
			} else {
				for _, space := range members {
					add(space+" "+name, zone)
				}
			}
			continue
		}
		if zone := spaces[target]; zone != "" {
			add(name, zone)
			continue
		}
		for _, member := range lists[target] {
			memberKey := normalizePurposeToken(member)
			if zone := index.zones[memberKey]; zone != "" {
				members := zoneSpaces[memberKey]
				if len(members) == 0 {
					members = []string{zone}
				}
				for _, space := range members {
					add(space+" "+name, zone)
				}
			} else if zone := spaces[memberKey]; zone != "" {
				add(spaceNames[memberKey]+" "+name, zone)
			}
		}
	}
	return index
}

func comfortPeopleVariable(name string) bool {
	return strings.HasPrefix(normalizePurposeToken(name), "zone thermal comfort fanger model ")
}

func comfortBuildingVariable(name string) bool {
	return strings.HasPrefix(normalizePurposeToken(name), "facility ")
}

func (index comfortOwnership) zoneForMetric(key, name string) string {
	if comfortPeopleVariable(name) {
		return index.people[normalizePurposeToken(key)]
	}
	if len(index.zones) == 0 {
		return strings.TrimSpace(key)
	} // Older result archives may lack the executed IDF.
	return index.zones[normalizePurposeToken(key)]
}

func comfortZoneInScope(zone string, scope SimulationPurposeScope) bool {
	keys, scoped, _ := purposeZoneKeysForScope(scope)
	if !scoped {
		return true
	}
	for _, key := range keys {
		if strings.EqualFold(strings.TrimSpace(key), zone) {
			return true
		}
	}
	return false
}

func scopeComfortUnmetSummaries(items []ComfortUnmetSummary, doc *idf.Document, scope SimulationPurposeScope) []ComfortUnmetSummary {
	ownership := buildComfortOwnership(doc)
	out := []ComfortUnmetSummary{}
	for _, item := range items {
		key := normalizePurposeToken(item.ZoneName)
		zone := ownership.zones[key]
		if zone != "" {
			item.Scope, item.ZoneName = "zone", zone
		}
		if item.Scope == "building" {
			out = append(out, item)
			continue
		}
		if zone == "" && len(ownership.zones) > 0 {
			continue
		}
		if comfortZoneInScope(item.ZoneName, scope) {
			item.Scope = "zone"
			out = append(out, item)
		}
	}
	return out
}
