package simulation

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const epathSQLConvectiveBaseboardType = "ZoneHVAC:Baseboard:Convective:Electric"
const epathSQLWindowACType = "ZoneHVAC:WindowAirConditioner"
const epathSQLConvectiveBaseboardFamily = "heating.baseboard.convective_electricity"
const epathSQLDirectFanFamily = "fans.zone_equipment.electricity"
const epathSQLOriginalMultiplierContract = "zone_and_group/v1"

type epathSQLOriginalZoneFactors struct{ Zone, List float64 }

var epathSQLNativeDecimal = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

// Only the parser is shared with production. Factors are read from original
// typed fields and then compared separately with SQL, never fitted to a graph.
func epathSQLOriginalZoneMultipliers(text string) (map[string]epathSQLOriginalZoneFactors, error) {
	doc, err := idf.Parse(text)
	if err != nil {
		return nil, err
	}
	field := epathSQLBaseboardField
	key := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	positive := func(s string, blank bool) (float64, error) {
		if s == "" && blank {
			return 1, nil
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil || !epathOracleFinite(n) || n <= 0 || n != float64(int64(n)) {
			return 0, fmt.Errorf("invalid original integer Zone multiplier")
		}
		return n, nil
	}
	out := map[string]epathSQLOriginalZoneFactors{}
	lists, groups := map[string]idf.Object{}, map[string]bool{}
	for _, object := range doc.Objects {
		name := key(field(object, 0))
		switch key(object.Type) {
		case "zone":
			if name == "" || out[name].Zone != 0 {
				return nil, fmt.Errorf("duplicate/empty original Zone identity")
			}
			m, err := positive(field(object, 6), true)
			if err != nil {
				return nil, err
			}
			out[name] = epathSQLOriginalZoneFactors{m, 1}
		case "zonelist":
			if name == "" || lists[name].Type != "" {
				return nil, fmt.Errorf("duplicate/empty original ZoneList")
			}
			lists[name] = object
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("original multiplier proof has no Zones")
	}
	assigned, boundLists := map[string]bool{}, map[string]bool{}
	for _, object := range doc.Objects {
		if !strings.EqualFold(object.Type, "ZoneGroup") {
			continue
		}
		name, listName := key(field(object, 0)), key(field(object, 1))
		list, exists := lists[listName]
		if name == "" || groups[name] || !exists || boundLists[listName] || len(list.Fields) < 2 {
			return nil, fmt.Errorf("ambiguous/unresolved original ZoneGroup list")
		}
		groups[name], boundLists[listName] = true, true
		// HeatBalanceManager v25.1 reads defaulted rNumericArgs(1), with
		// no selected-capacity-style blank rejection. Native default is 1.
		m, err := positive(field(object, 2), true)
		if err != nil {
			return nil, err
		}
		for index := 1; index < len(list.Fields); index++ {
			zone := key(field(list, index))
			factors, exists := out[zone]
			if !exists || assigned[zone] {
				return nil, fmt.Errorf("foreign/duplicate/overlapping grouped original Zone")
			}
			assigned[zone] = true
			factors.List = m
			if !epathOracleFinite(factors.Zone * m) {
				return nil, fmt.Errorf("overflowing original Zone factors")
			}
			out[zone] = factors
		}
	}
	return out, nil
}

func epathSQLValidateOriginalZoneMultipliers(db *sql.DB, original string, contract string) error {
	if contract == "" {
		return nil
	} // Legacy mathematical frames do not assert an original-file proof.
	if contract != epathSQLOriginalMultiplierContract || strings.TrimSpace(original) == "" {
		return fmt.Errorf("unknown/missing original Zone multiplier proof contract")
	}
	want, err := epathSQLOriginalZoneMultipliers(original)
	if err != nil {
		return err
	}
	rows, err := db.Query(`SELECT ZoneName,Multiplier,ListMultiplier FROM Zones`)
	if err != nil {
		return err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var name string
		var zone, list float64
		if err := rows.Scan(&name, &zone, &list); err != nil {
			return err
		}
		key := strings.ToLower(strings.TrimSpace(name))
		factors, exists := want[key]
		if !exists || seen[key] || zone != factors.Zone || list != factors.List {
			return fmt.Errorf("SQL Zone factors do not equal original Zone and Group separately: %s", name)
		}
		seen[key] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(seen) != len(want) {
		return fmt.Errorf("SQL multiplier proof omits original Zones")
	}
	return nil
}

func epathSQLValidateSelfEquipmentOwner(doc idf.Document, owner epathRealSQLDirectHVACOwner) (idf.Object, idf.Object, error) {
	field := epathSQLBaseboardField
	parent, err := epathSQLBaseboardOne(doc, owner.EquipmentType, owner.EquipmentName)
	if err != nil {
		return parent, idf.Object{}, err
	}
	if _, err := epathSQLBaseboardOne(doc, "Zone", owner.ZoneName); err != nil {
		return parent, idf.Object{}, err
	}
	var list, connection idf.Object
	references := 0
	for _, object := range doc.Objects {
		for index := 0; index+1 < len(object.Fields); index++ {
			if !strings.EqualFold(field(object, index), owner.EquipmentType) || !strings.EqualFold(field(object, index+1), owner.EquipmentName) {
				continue
			}
			if !strings.EqualFold(object.Type, "ZoneHVAC:EquipmentList") || index < 2 || (index-2)%6 != 0 {
				return parent, connection, fmt.Errorf("original local equipment has a foreign typed reference")
			}
			if _, err := epathSQLBaseboardOne(doc, object.Type, field(object, 0)); err != nil {
				return parent, connection, err
			}
			references++
			list = object
		}
	}
	if references != 1 {
		return parent, connection, fmt.Errorf("original equipment has missing/shared list ownership")
	}
	connections, zoneConnections := 0, 0
	for _, object := range doc.Objects {
		if !strings.EqualFold(object.Type, "ZoneHVAC:EquipmentConnections") {
			continue
		}
		if strings.EqualFold(field(object, 0), owner.ZoneName) {
			zoneConnections++
		}
		if strings.EqualFold(field(object, 1), field(list, 0)) {
			if !strings.EqualFold(field(object, 0), owner.ZoneName) {
				return parent, connection, fmt.Errorf("equipment list belongs to another original Zone")
			}
			connections++
			connection = object
		}
	}
	if connections != 1 || zoneConnections != 1 {
		return parent, connection, fmt.Errorf("ambiguous original Zone equipment connection")
	}
	return parent, connection, nil
}

func epathSQLValidateConvectiveBaseboardOriginalOwner(doc idf.Document, owner epathRealSQLDirectHVACOwner) error {
	if owner.EquipmentType != epathSQLConvectiveBaseboardType || owner.ComponentType != epathSQLConvectiveBaseboardType || !strings.EqualFold(owner.KeyValue, owner.EquipmentName) {
		return fmt.Errorf("convective baseboard requires exact self-owned native type/key")
	}
	parent, _, err := epathSQLValidateSelfEquipmentOwner(doc, owner)
	if err != nil {
		return err
	}
	field := epathSQLBaseboardField
	if len(parent.Fields) < 4 || len(parent.Fields) > 7 {
		return fmt.Errorf("convective baseboard must fit its seven-field native schema with explicit active capacity")
	}
	// IdfParser pads native fields; method and efficiency have defaults. The
	// engine nevertheless rejects a blank selected capacity, even where an
	// IDD numeric default is named. Defaults are not guessed from shape alone.
	method := strings.ToLower(field(parent, 2))
	if method == "" {
		method = "heatingdesigncapacity"
	}
	capacityIndex := 3
	switch method {
	case "heatingdesigncapacity":
	case "capacityperfloorarea":
		capacityIndex = 4
	case "fractionofautosizedheatingcapacity":
		capacityIndex = 5
	default:
		return fmt.Errorf("unknown native convective capacity method")
	}
	capacityText := field(parent, capacityIndex)
	if method != "heatingdesigncapacity" || !strings.EqualFold(capacityText, "Autosize") {
		capacity, err := strconv.ParseFloat(capacityText, 64)
		if err != nil || !epathSQLNativeDecimal.MatchString(capacityText) || !epathOracleFinite(capacity) || capacity < 0 || method == "capacityperfloorarea" && capacity == 0 {
			return fmt.Errorf("missing/invalid explicit convective capacity for selected method")
		}
	}
	effText := field(parent, 6)
	if effText == "" {
		effText = "1"
	}
	eff, err := strconv.ParseFloat(effText, 64)
	if err != nil || !epathSQLNativeDecimal.MatchString(effText) || !epathOracleFinite(eff) || eff <= 0 || eff > 1 {
		return fmt.Errorf("invalid original convective baseboard efficiency")
	}
	reporting := 0
	for _, object := range doc.Objects {
		if strings.HasPrefix(strings.ToLower(object.Type), "zonehvac:baseboard:") && strings.EqualFold(field(object, 0), owner.KeyValue) {
			reporting++
		}
	}
	if reporting != 1 {
		return fmt.Errorf("baseboard native reporting identity collides across types")
	}
	return nil
}

func epathSQLValidateWindowACOriginalOwner(doc idf.Document, owner epathRealSQLDirectHVACOwner) error {
	if owner.EquipmentType != epathSQLWindowACType {
		return fmt.Errorf("native Window AC requires exact parent type")
	}
	parent, connection, err := epathSQLValidateSelfEquipmentOwner(doc, owner)
	if err != nil {
		return err
	}
	field := epathSQLBaseboardField
	if len(parent.Fields) < 14 || field(parent, 8) != "Fan:OnOff" || field(parent, 10) != "Coil:Cooling:DX:SingleSpeed" || field(parent, 6) != "OutdoorAir:Mixer" {
		return fmt.Errorf("unsupported Window AC exact fan/coil/mixer shape")
	}
	position := 10
	if owner.ComponentType == "Fan:OnOff" {
		position = 8
	} else if owner.ComponentType != "Coil:Cooling:DX:SingleSpeed" {
		return fmt.Errorf("Window AC cannot own a heating constituent")
	}
	if !strings.EqualFold(field(parent, position), owner.ComponentType) || !strings.EqualFold(field(parent, position+1), owner.KeyValue) {
		return fmt.Errorf("Window AC constituent not in its exact original slot")
	}
	// Independent finite 25.1 output-emitter roster. A same-named fuel/water
	// heating coil does not emit these native cooling electricity identities.
	// Sources: DXCoils.cc, VariableSpeedCoils.cc, WaterToAirHeatPumpSimple.cc,
	// WaterToAirHeatPump.cc, Coils/CoilCoolingDX.cc and Fans.cc (v25.1.0).
	coolingEmitters := []string{"Coil:Cooling:DX:SingleSpeed", "Coil:Cooling:DX:SingleSpeed:ThermalStorage", "Coil:Cooling:DX:TwoSpeed", "Coil:Cooling:DX:MultiSpeed", "Coil:Cooling:DX:TwoStageWithHumidityControlMode", "Coil:Cooling:DX", "Coil:Cooling:DX:VariableSpeed", "Coil:Cooling:WaterToAirHeatPump:VariableSpeedEquationFit", "Coil:Cooling:WaterToAirHeatPump:EquationFit", "Coil:Cooling:WaterToAirHeatPump:ParameterEstimation", "Coil:WaterHeating:AirToWaterHeatPump:Pumped", "Coil:WaterHeating:AirToWaterHeatPump:Wrapped", "Coil:WaterHeating:AirToWaterHeatPump:VariableSpeed"}
	fanEmitters := []string{"Fan:OnOff", "Fan:ConstantVolume", "Fan:VariableVolume", "Fan:SystemModel", "Fan:ZoneExhaust", "Fan:ComponentModel"}
	emitters := coolingEmitters
	if owner.ComponentType == "Fan:OnOff" {
		emitters = fanEmitters
	}
	reporting := 0
	for _, object := range doc.Objects {
		if !strings.EqualFold(field(object, 0), owner.KeyValue) {
			continue
		}
		for _, typ := range emitters {
			if strings.EqualFold(object.Type, typ) {
				reporting++
				break
			}
		}
	}
	if reporting != 1 {
		return fmt.Errorf("Window AC native reporting key collides across actual output emitters")
	}
	objects := map[int]idf.Object{}
	for _, slot := range []int{6, 8, 10} {
		component, err := epathSQLBaseboardOne(doc, field(parent, slot), field(parent, slot+1))
		if err != nil {
			return err
		}
		refs := 0
		for _, object := range doc.Objects {
			for index := 0; index+1 < len(object.Fields); index++ {
				if strings.EqualFold(field(object, index), component.Type) && strings.EqualFold(field(object, index+1), field(component, 0)) {
					if object.Index != parent.Index || index != slot {
						return fmt.Errorf("Window AC component is shared or referenced outside its exact role")
					}
					refs++
				}
			}
		}
		if refs != 1 {
			return fmt.Errorf("Window AC native component lacks unique reference")
		}
		objects[slot] = component
	}
	mixer, fan, coil := objects[6], objects[8], objects[10]
	member := func(name, declared string) bool {
		if name == "" || declared == "" {
			return false
		}
		if strings.EqualFold(name, declared) {
			return true
		}
		list, err := epathSQLBaseboardOne(doc, "NodeList", declared)
		if err != nil {
			return false
		}
		count := 0
		for index := 1; index < len(list.Fields); index++ {
			if strings.EqualFold(field(list, index), name) {
				count++
			}
		}
		return count == 1
	}
	// Native Fan:OnOff inlet/outlet are fields 7/8, SingleSpeed coil 8/9;
	// OA mixer mixed/outside/relief/return are fields 1/2/3/4.
	if !member(field(parent, 4), field(connection, 3)) || !member(field(parent, 5), field(connection, 2)) || !strings.EqualFold(field(mixer, 4), field(parent, 4)) {
		return fmt.Errorf("Window AC ports do not belong to its original Zone exhaust/inlet")
	}
	chain := []string{}
	switch strings.ToLower(field(parent, 13)) {
	case "blowthrough":
		chain = []string{field(mixer, 1), field(fan, 7), field(fan, 8), field(coil, 8), field(coil, 9), field(parent, 5)}
	case "drawthrough":
		chain = []string{field(mixer, 1), field(coil, 8), field(coil, 9), field(fan, 7), field(fan, 8), field(parent, 5)}
	default:
		return fmt.Errorf("unknown Window AC fan placement")
	}
	for index := 0; index < len(chain); index += 2 {
		if chain[index] == "" || !strings.EqualFold(chain[index], chain[index+1]) {
			return fmt.Errorf("disconnected Window AC native fan/coil node chain")
		}
	}
	return nil
}
