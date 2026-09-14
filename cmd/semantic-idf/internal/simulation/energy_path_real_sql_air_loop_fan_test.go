package simulation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// A broad-meter estimate is not a measured Zone or native fan observation.
// This finite original-model proof is for one central variable-volume fan;
// multiple/local/mixed fans retain their existing independent pool contracts.
type epathRealSQLAirLoopFan struct {
	SiteID      string   `json:"siteId"`
	ObjectType  string   `json:"objectType"`
	ObjectName  string   `json:"objectName"`
	AirLoopName string   `json:"airLoopName"`
	ServedZones []string `json:"servedZones"`
}

func epathSQLAirLoopFanZones(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" || seen[key] {
			return nil, fmt.Errorf("empty or repeated original fan Zone")
		}
		seen[key] = true
		out = append(out, key)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("central fan requires served Zones")
	}
	sort.Strings(out)
	return out, nil
}

func epathSQLAirLoopFanOriginal(doc idf.Document, declaration epathRealSQLAirLoopFan) error {
	if declaration.SiteID == "" || declaration.ObjectType != "Fan:VariableVolume" || strings.TrimSpace(declaration.ObjectName) == "" || strings.TrimSpace(declaration.AirLoopName) == "" {
		return fmt.Errorf("central fan requires an exact typed original declaration")
	}
	field := epathSQLBaseboardField // Low-level original field access only.
	one := func(typ, name string) (idf.Object, error) {
		return epathSQLBaseboardOne(doc, typ, name)
	}
	fan, err := one(declaration.ObjectType, declaration.ObjectName)
	if err != nil {
		return err
	}
	count := 0
	for _, object := range doc.Objects {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(object.Type)), "fan:") {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("broad Fans meter has unknown or multiple consuming fans")
	}
	air, err := one("AirLoopHVAC", declaration.AirLoopName)
	if err != nil {
		return err
	}
	list, err := one("BranchList", field(air, 4))
	if err != nil {
		return err
	}
	if len(list.Fields) != 2 {
		return fmt.Errorf("central fan requires one proved main branch")
	}
	branch, err := (epathSQLSharedOriginal{doc: doc}).branch(field(list, 1), "air")
	if err != nil {
		return err
	}
	if (len(branch.Fields)-2)%4 != 0 {
		return fmt.Errorf("malformed fan branch roster")
	}
	if field(air, 6) == "" || !strings.EqualFold(field(air, 6), field(branch, 4)) {
		return fmt.Errorf("fan air-loop supply inlet contradicts original main-branch inlet")
	}
	branchUses, listUses := 0, 0
	for _, object := range doc.Objects {
		if strings.EqualFold(object.Type, "BranchList") {
			for index := 1; index < len(object.Fields); index++ {
				if strings.EqualFold(field(object, index), field(branch, 0)) {
					branchUses++
				}
			}
		}
		if strings.EqualFold(object.Type, "AirLoopHVAC") && strings.EqualFold(field(object, 4), field(list, 0)) {
			listUses++
		}
	}
	if branchUses != 1 || listUses != 1 {
		return fmt.Errorf("fan branch/list has ambiguous air-loop ownership")
	}
	occurrences := 0
	for _, object := range doc.Objects {
		for index := 0; index+1 < len(object.Fields); index++ {
			if !strings.EqualFold(field(object, index), declaration.ObjectType) || !strings.EqualFold(field(object, index+1), declaration.ObjectName) {
				continue
			}
			occurrences++
			if !strings.EqualFold(object.Type, "Branch") || !strings.EqualFold(field(object, 0), field(branch, 0)) || index < 2 || (index-2)%4 != 0 || index+4 != len(branch.Fields) {
				return fmt.Errorf("fan is not uniquely the declared air-loop outlet component")
			}
			if field(fan, 15) == "" || field(fan, 16) == "" || !strings.EqualFold(field(fan, 15), field(object, index+2)) || !strings.EqualFold(field(fan, 16), field(object, index+3)) || !strings.EqualFold(field(fan, 16), field(air, 9)) {
				return fmt.Errorf("fan native ports contradict original branch/air-loop ports")
			}
		}
	}
	if occurrences != 1 {
		return fmt.Errorf("fan has absent or shared original branch ownership")
	}
	// The independently parsed air route also validates terminal native ports,
	// ADU ownership, EquipmentConnections and exact Zone inlets.
	zones, err := epathSQLHVACOriginalAirLoopZones(doc, declaration.AirLoopName)
	if err != nil {
		return err
	}
	actual, err := epathSQLAirLoopFanZones(zones)
	if err != nil {
		return err
	}
	want, err := epathSQLAirLoopFanZones(declaration.ServedZones)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, want) {
		return fmt.Errorf("central fan declared served Zones differ from original air route")
	}
	return nil
}

func epathSQLBindAirLoopFans(original string, model epathRealSQLModel, frames *epathSQLFrames) error {
	if len(model.AirLoopFans) == 0 {
		return nil
	}
	if frames == nil || len(frames.AirLoopFans) != 0 || strings.TrimSpace(original) == "" {
		return fmt.Errorf("central fan lacks fresh original-bound frames")
	}
	doc, err := idf.Parse(original)
	if err != nil {
		return err
	}
	out := map[string]epathRealSQLAirLoopFan{}
	for _, declaration := range model.AirLoopFans {
		if _, duplicate := out[declaration.SiteID]; duplicate {
			return fmt.Errorf("duplicate central fan site declaration")
		}
		if err := epathSQLAirLoopFanOriginal(doc, declaration); err != nil {
			return err
		}
		out[declaration.SiteID] = declaration
	}
	frames.AirLoopFans = out
	return nil
}

func epathSQLValidateAirLoopFanFrames(frames epathSQLFrames, model epathRealSQLModel, auxiliary epathRealSQLAuxiliary) error {
	actual, exists := frames.AirLoopFans[auxiliary.SiteID]
	if !exists || actual.SiteID != auxiliary.SiteID || actual.ObjectType != "Fan:VariableVolume" || actual.ObjectName == "" || actual.AirLoopName == "" || auxiliary.Weight != "cooling_plus_heating" || auxiliary.AllocationMethod != "air_loop_load_share" {
		return fmt.Errorf("fan allocation requires its independent original central-fan binding")
	}
	count := 0
	for _, declaration := range model.AirLoopFans {
		if declaration.SiteID == auxiliary.SiteID {
			count++
			if !reflect.DeepEqual(actual, declaration) {
				return fmt.Errorf("central fan frame was changed after original binding")
			}
		}
	}
	if count != 1 {
		return fmt.Errorf("central fan frame lacks its unique original declaration")
	}
	want, err := epathSQLAirLoopFanZones(actual.ServedZones)
	if err != nil {
		return err
	}
	got, err := epathSQLAirLoopFanZones(auxiliary.ServedZones)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(want, got) {
		return fmt.Errorf("central fan allocation escapes its original served Zones")
	}
	return nil
}
