package simulation

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// This oracle walk intentionally shares only the low-level parser. No runtime
// target, HVAC analysis, multiplier, classifier or recipient helper is called.
const epathSQLNativeBaseboardType = "ZoneHVAC:Baseboard:RadiantConvective:Electric"

func epathSQLBaseboardField(object idf.Object, position int) string {
	if position < 0 || position >= len(object.Fields) {
		return ""
	}
	return strings.TrimSpace(object.Fields[position].Value)
}

func epathSQLBaseboardOne(doc idf.Document, typ, name string) (idf.Object, error) {
	var result idf.Object
	count := 0
	for _, object := range doc.Objects {
		if strings.EqualFold(strings.TrimSpace(object.Type), typ) && strings.EqualFold(epathSQLBaseboardField(object, 0), strings.TrimSpace(name)) {
			result = object
			count++
		}
	}
	if strings.TrimSpace(name) == "" || count != 1 {
		return result, fmt.Errorf("independent native baseboard proof requires one original %s/%s", typ, name)
	}
	return result, nil
}

func epathSQLValidateNativeBaseboardOriginalOwner(doc idf.Document, owner epathRealSQLDirectHVACOwner) error {
	if owner.EquipmentType != epathSQLNativeBaseboardType || owner.ComponentType != epathSQLNativeBaseboardType || !strings.EqualFold(strings.TrimSpace(owner.KeyValue), strings.TrimSpace(owner.EquipmentName)) || strings.TrimSpace(owner.ZoneName) == "" {
		return fmt.Errorf("native baseboard requires its exact self-owned type/key and Zone")
	}
	parent, err := epathSQLBaseboardOne(doc, epathSQLNativeBaseboardType, owner.KeyValue)
	if err != nil {
		return err
	}
	if _, err := epathSQLBaseboardOne(doc, "Zone", owner.ZoneName); err != nil {
		return err
	}
	reportingNames := 0
	for _, object := range doc.Objects {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(object.Type)), "zonehvac:baseboard:") && strings.EqualFold(epathSQLBaseboardField(object, 0), owner.KeyValue) {
			reportingNames++
		}
	}
	if reportingNames != 1 {
		return fmt.Errorf("native baseboard SQL reporting key collides across original object types")
	}
	var list idf.Object
	references := 0
	for _, object := range doc.Objects {
		for field := 0; field+1 < len(object.Fields); field++ {
			if !strings.EqualFold(epathSQLBaseboardField(object, field), epathSQLNativeBaseboardType) || !strings.EqualFold(epathSQLBaseboardField(object, field+1), owner.KeyValue) {
				continue
			}
			references++
			if !strings.EqualFold(object.Type, "ZoneHVAC:EquipmentList") || field < 2 || (field-2)%6 != 0 {
				return fmt.Errorf("native baseboard referenced outside an exact EquipmentList slot")
			}
			if _, err := epathSQLBaseboardOne(doc, object.Type, epathSQLBaseboardField(object, 0)); err != nil {
				return err
			}
			list = object
		}
	}
	if references != 1 {
		return fmt.Errorf("native baseboard has missing/shared/disconnected EquipmentList ownership")
	}
	connections, zoneConnections := 0, 0
	for _, object := range doc.Objects {
		if !strings.EqualFold(object.Type, "ZoneHVAC:EquipmentConnections") {
			continue
		}
		if strings.EqualFold(epathSQLBaseboardField(object, 0), owner.ZoneName) {
			zoneConnections++
		}
		if strings.EqualFold(epathSQLBaseboardField(object, 1), epathSQLBaseboardField(list, 0)) {
			connections++
			if !strings.EqualFold(epathSQLBaseboardField(object, 0), owner.ZoneName) {
				return fmt.Errorf("baseboard original EquipmentList belongs to another Zone")
			}
		}
	}
	if connections != 1 || zoneConnections != 1 {
		return fmt.Errorf("native baseboard original Zone/list connections are ambiguous")
	}
	// Native 25.1 capacity-method shape. Fractions are physical declarations,
	// not weights for splitting observed surface convection or delivered load.
	if len(parent.Fields) < 9 || (len(parent.Fields)-9)%2 != 0 {
		return fmt.Errorf("unsupported native baseboard field layout")
	}
	fraction := func(position int) (float64, error) {
		number, err := strconv.ParseFloat(epathSQLBaseboardField(parent, position), 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || number > 1 {
			return 0, fmt.Errorf("invalid original baseboard fraction at field %d", position)
		}
		return number, nil
	}
	efficiency, err := fraction(6)
	if err != nil || efficiency == 0 {
		return fmt.Errorf("invalid original baseboard efficiency")
	}
	radiant, err := fraction(7)
	if err != nil {
		return err
	}
	people, err := fraction(8)
	if err != nil {
		return err
	}
	sum := people
	seen := map[string]bool{}
	for position := 9; position < len(parent.Fields); position += 2 {
		name := epathSQLBaseboardField(parent, position)
		if name == "" && epathSQLBaseboardField(parent, position+1) == "" {
			continue
		}
		share, err := fraction(position + 1)
		if err != nil {
			return err
		}
		surface, err := epathSQLBaseboardOne(doc, "BuildingSurface:Detailed", name)
		if err != nil {
			return err
		}
		key := strings.ToLower(name)
		if seen[key] || !strings.EqualFold(epathSQLBaseboardField(surface, 3), owner.ZoneName) {
			return fmt.Errorf("native baseboard recipient is duplicate or in another Zone")
		}
		seen[key] = true
		if spaceName := epathSQLBaseboardField(surface, 4); spaceName != "" {
			space, err := epathSQLBaseboardOne(doc, "Space", spaceName)
			if err != nil || !strings.EqualFold(epathSQLBaseboardField(space, 1), owner.ZoneName) {
				return fmt.Errorf("native baseboard recipient has unresolved/foreign Space")
			}
		}
		sum += share
	}
	if radiant > 0 && math.Abs(sum-1) > 1e-9 {
		return fmt.Errorf("native baseboard original radiant recipient fractions do not close")
	}
	return nil
}

// A context output proves the same original equipment owner, but is not a
// direct-consuming constituent or an additional sensible Zone-air load.
func epathSQLBaseboardOwner(zone, name string) epathRealSQLDirectHVACOwner {
	return epathRealSQLDirectHVACOwner{KeyValue: name, ZoneName: zone, EquipmentType: epathSQLNativeBaseboardType, EquipmentName: name, ComponentType: epathSQLNativeBaseboardType}
}
