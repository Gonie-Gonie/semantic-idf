package idf

import (
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func hvacHeatOnlyOriginal(t *testing.T, commentless bool) Document {
	t.Helper()
	data, err := os.ReadFile("../simulation/testdata/energy_path_real_models/models/25.1/Furnace.idf")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != "2a221defefdffa3792d69924b439f466094ccb4a9fe14aa750e5ffe817fcd1f8" {
		t.Fatal("official no-cooling original changed")
	}
	doc, err := Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if commentless {
		for i := range doc.Objects {
			for f := range doc.Objects[i].Fields {
				doc.Objects[i].Fields[f].Comment = ""
			}
		}
	}
	return doc
}

func hvacHeatOnlyPaths(t *testing.T, doc Document, want map[string]int) HVACReport {
	t.Helper()
	report := AnalyzeHVAC(doc)
	for _, zone := range []string{"West Zone", "EAST ZONE", "NORTH ZONE"} {
		paths := hvacSmallOfficeConditioningPaths(report, zone, "heating")
		if len(paths) != want[strings.ToLower(zone)] {
			t.Fatalf("%s heating paths=%d want%d", zone, len(paths), want[strings.ToLower(zone)])
		}
		if len(hvacSmallOfficeConditioningPaths(report, zone, "cooling")) != 0 {
			t.Fatalf("heating-only Furnace invented cooling service for %s", zone)
		}
	}
	return report
}

func hvacHeatOnlyAllPaths() map[string]int {
	return map[string]int{"west zone": 1, "east zone": 1, "north zone": 1}
}

func TestHVACHeatOnlyFurnaceOriginalNativeAndCommentless(t *testing.T) {
	for _, commentless := range []bool{false, true} {
		t.Run(fmt.Sprintf("commentless_%t", commentless), func(t *testing.T) {
			doc := hvacHeatOnlyOriginal(t, commentless)
			report := hvacHeatOnlyPaths(t, doc, hvacHeatOnlyAllPaths())
			for _, zone := range []string{"West Zone", "EAST ZONE", "NORTH ZONE"} {
				path := hvacSmallOfficeConditioningPaths(report, zone, "heating")[0]
				if path.ServiceKind != "heating" || path.PathType != "central_air" || path.AirLoop == nil || path.AirLoop.Name != "Typical Terminal Reheat 1" || path.PlantLoop != nil || path.Delivery.ObjectType != "AirTerminal:SingleDuct:ConstantVolume:NoReheat" || path.DeliveryWrapper == nil {
					t.Fatalf("native heating path changed original boundary: %+v", path)
				}
				actual := []string{}
				for _, component := range path.Conditioning {
					actual = append(actual, fmt.Sprintf("%s|%s|%d", component.ObjectType, component.ObjectName, component.ObjectIndex))
				}
				if !reflect.DeepEqual(actual, []string{"AirLoopHVAC:Unitary:Furnace:HeatOnly|Gas Furnace 1|92", "Coil:Heating:Fuel|Furnace Coil|106"}) {
					t.Fatalf("original wrapper/coil provenance changed or fan became coil heat: %v", actual)
				}
				if !hvacSmallOfficeHasString(path.TraceIDs, hvacRuleComponentReferencesComponent) || !hvacSmallOfficeHasString(path.TraceIDs, hvacRuleAirLoopZoneSplitterToTerminal) {
					t.Fatalf("missing native child or exact delivery trace: %v", path.TraceIDs)
				}
			}
		})
	}
}

func TestHVACHeatOnlyFurnacePlacementAndControlDoNotInventCooling(t *testing.T) {
	for _, variant := range []string{"native default", "DrawThrough", "control West", "control North", "misleading names"} {
		t.Run(variant, func(t *testing.T) {
			doc := hvacHeatOnlyOriginal(t, true)
			parent := hvacSmallOfficeObject(t, &doc, nativeHeatOnlyFurnaceType, "Gas Furnace 1")
			switch variant {
			case "native default":
				parent.Fields[10].Value = ""
			case "DrawThrough":
				parent.Fields[10].Value = "DrawThrough"
				fan := hvacSmallOfficeObject(t, &doc, "Fan:OnOff", "Supply Fan 1")
				coil := hvacSmallOfficeObject(t, &doc, "Coil:Heating:Fuel", "Furnace Coil")
				coil.Fields[5].Value = parent.Fields[2].Value
				coil.Fields[6].Value = "Native DrawThrough Internal"
				fan.Fields[7].Value = "Native DrawThrough Internal"
				fan.Fields[8].Value = parent.Fields[3].Value
			case "control West":
				parent.Fields[7].Value = "West Zone"
				hvacSmallOfficeObject(t, &doc, "ZoneControl:Thermostat", "Zone 2 Thermostat").Fields[1].Value = "West Zone"
			case "control North":
				parent.Fields[7].Value = "NORTH ZONE"
				hvacSmallOfficeObject(t, &doc, "ZoneControl:Thermostat", "Zone 2 Thermostat").Fields[1].Value = "NORTH ZONE"
			case "misleading names":
				for i := range doc.Objects {
					for field := range doc.Objects[i].Fields {
						if strings.EqualFold(doc.Objects[i].Fields[field].Value, "Gas Furnace 1") {
							doc.Objects[i].Fields[field].Value = "Cooling DX Chiller named furnace"
						}
					}
				}
			}
			hvacHeatOnlyPaths(t, doc, hvacHeatOnlyAllPaths())
		})
	}
}

func TestHVACHeatOnlyFurnaceRejectsDisconnectedOrAmbiguousNativeSupply(t *testing.T) {
	for _, mutation := range []string{"wrapper inlet", "wrapper outlet", "fan inlet", "fan outlet", "coil inlet", "coil outlet", "wrong placement", "missing fan", "missing coil", "wrong coil type", "duplicate fan", "duplicate coil", "shared fan", "shared coil", "shared wrapper", "shared branch", "shared branch list", "missing control", "wrong version"} {
		t.Run(mutation, func(t *testing.T) {
			doc := hvacHeatOnlyOriginal(t, true)
			parent := hvacSmallOfficeObject(t, &doc, nativeHeatOnlyFurnaceType, "Gas Furnace 1")
			fan := hvacSmallOfficeObject(t, &doc, "Fan:OnOff", "Supply Fan 1")
			coil := hvacSmallOfficeObject(t, &doc, "Coil:Heating:Fuel", "Furnace Coil")
			clone := func(object Object, name string) {
				object.Fields = append([]Field(nil), object.Fields...)
				object.Fields[0].Value = name
				object.Index = len(doc.Objects)
				doc.Objects = append(doc.Objects, object)
			}
			switch mutation {
			case "wrapper inlet":
				parent.Fields[2].Value = "Disconnected"
			case "wrapper outlet":
				parent.Fields[3].Value = "Disconnected"
			case "fan inlet":
				fan.Fields[7].Value = "Disconnected"
			case "fan outlet":
				fan.Fields[8].Value = "Disconnected"
			case "coil inlet":
				coil.Fields[5].Value = "Disconnected"
			case "coil outlet":
				coil.Fields[6].Value = "Disconnected"
			case "wrong placement":
				parent.Fields[10].Value = "DrawThrough"
			case "missing fan":
				parent.Fields[9].Value = "Missing"
			case "missing coil":
				parent.Fields[12].Value = "Missing"
			case "wrong coil type":
				parent.Fields[11].Value = "Coil:Cooling:DX:SingleSpeed"
			case "duplicate fan":
				clone(*fan, "Supply Fan 1")
			case "duplicate coil":
				clone(*coil, "Furnace Coil")
			case "shared fan", "shared coil":
				copy := *parent
				copy.Fields = append([]Field(nil), parent.Fields...)
				if mutation == "shared fan" {
					copy.Fields[12].Value = "Unrelated Coil"
				} else {
					copy.Fields[9].Value = "Unrelated Fan"
				}
				clone(copy, "Unconnected second Furnace")
			case "shared wrapper":
				clone(*hvacSmallOfficeObject(t, &doc, "Branch", "Air Loop Main Branch"), "Unconnected second Branch")
			case "shared branch":
				clone(*hvacSmallOfficeObject(t, &doc, "BranchList", "Air Loop Branches"), "Second BranchList")
			case "shared branch list":
				clone(*hvacSmallOfficeObject(t, &doc, "AirLoopHVAC", "Typical Terminal Reheat 1"), "Second AirLoop")
			case "missing control":
				parent.Fields[7].Value = "Missing Zone"
			case "wrong version":
				changed := 0
				for i := range doc.Objects {
					if strings.EqualFold(doc.Objects[i].Type, "Version") {
						if len(doc.Objects[i].Fields) != 1 || !strings.HasPrefix(doc.Objects[i].Fields[0].Value, "25.1") {
							t.Fatal("unexpected pinned original version")
						}
						doc.Objects[i].Fields[0].Value = "22.1"
						changed++
					}
				}
				if changed != 1 {
					t.Fatal("wrong-version mutation did not target exactly one Version object")
				}
			}
			hvacHeatOnlyPaths(t, doc, map[string]int{})
		})
	}
}

func TestHVACHeatOnlyFurnaceRequiresOriginalPerTerminalOwner(t *testing.T) {
	for _, mutation := range []string{"terminal inlet", "terminal outlet", "ADU outlet", "borrowed ADU", "duplicate connection", "foreign shared inlet", "nested inlet list"} {
		t.Run(mutation, func(t *testing.T) {
			doc := hvacHeatOnlyOriginal(t, true)
			terminal := hvacSmallOfficeObject(t, &doc, "AirTerminal:SingleDuct:ConstantVolume:NoReheat", "Zone1DirectAir")
			adu := hvacSmallOfficeObject(t, &doc, "ZoneHVAC:AirDistributionUnit", "Zone1DirectAir ADU")
			switch mutation {
			case "terminal inlet":
				terminal.Fields[2].Value = "Disconnected"
			case "terminal outlet":
				terminal.Fields[3].Value = "Disconnected"
			case "ADU outlet":
				adu.Fields[1].Value = "Disconnected"
			case "borrowed ADU":
				list := hvacSmallOfficeObject(t, &doc, "ZoneHVAC:EquipmentList", "Zone2Equipment")
				list.Fields = append(list.Fields, Field{}, Field{}, Field{Value: "ZoneHVAC:AirDistributionUnit"}, Field{Value: objectName(*adu)}, Field{Value: "2"}, Field{Value: "2"})
			case "duplicate connection":
				connection := *hvacSmallOfficeObject(t, &doc, "ZoneHVAC:EquipmentConnections", "West Zone")
				connection.Index = len(doc.Objects)
				doc.Objects = append(doc.Objects, connection)
			case "foreign shared inlet":
				other := hvacSmallOfficeObject(t, &doc, "NodeList", "Zone3Inlets")
				other.Fields = append(other.Fields, Field{Value: terminal.Fields[3].Value})
			case "nested inlet list":
				inlets := hvacSmallOfficeObject(t, &doc, "NodeList", "Zone1Inlets")
				inlets.Fields[1].Value = "Zone3Inlets"
			}
			want := hvacHeatOnlyAllPaths()
			want["west zone"] = 0
			hvacHeatOnlyPaths(t, doc, want)
		})
	}
}

func TestHVACHeatOnlyFurnaceChildFailureIsLocalButOuterPortBreaksBranch(t *testing.T) {
	doc := hvacHeatOnlyOriginal(t, false)
	parent := hvacSmallOfficeObject(t, &doc, nativeHeatOnlyFurnaceType, "Gas Furnace 1")
	parent.Fields[12].Value = "Missing local coil"
	coil := *hvacSmallOfficeObject(t, &doc, "Coil:Heating:Fuel", "Furnace Coil")
	coil.Fields = append([]Field(nil), coil.Fields...)
	coil.Fields[0].Value = "Independent downstream heater"
	coil.Fields[5].Value = "Air Loop Outlet Node"
	coil.Fields[6].Value = "Downstream Branch Outlet"
	coil.Index = len(doc.Objects)
	doc.Objects = append(doc.Objects, coil)
	hvacSmallOfficeObject(t, &doc, "AirLoopHVAC", "Typical Terminal Reheat 1").Fields[9].Value = "Downstream Branch Outlet"
	branch := hvacSmallOfficeObject(t, &doc, "Branch", "Air Loop Main Branch")
	branch.Fields = append(branch.Fields, Field{Value: "Coil:Heating:Fuel"}, Field{Value: "Independent downstream heater"}, Field{Value: "Air Loop Outlet Node"}, Field{Value: "Downstream Branch Outlet"})
	report := hvacHeatOnlyPaths(t, doc, hvacHeatOnlyAllPaths())
	for _, zone := range []string{"West Zone", "EAST ZONE", "NORTH ZONE"} {
		path := hvacSmallOfficeConditioningPaths(report, zone, "heating")[0]
		if len(path.Conditioning) != 1 || path.Conditioning[0].ObjectName != "Independent downstream heater" {
			t.Fatalf("missing local child hid or impersonated a valid downstream heating source: %+v", path.Conditioning)
		}
	}
	hvacSmallOfficeObject(t, &doc, nativeHeatOnlyFurnaceType, "Gas Furnace 1").Fields[2].Value = "Disconnected outer inlet"
	hvacHeatOnlyPaths(t, doc, map[string]int{})
	if !hvacHeatOnlyFurnaceDeliveryMatches(nil, nil, HVACZoneChain{}, HVACComponent{}) {
		t.Fatal("HeatOnly delivery guard changed unrelated service paths")
	}
}
