package simulation

// Independent acceptance support. Original lexical mutations, never production route output or
// candidate numbers, are the authority for this finite topology oracle.

import (
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Shared with independent source tests: identity declarations only, no values,
// native dictionary indices, execution paths or production helper calls.
func epathSQLPoolOriginalDeclaration() epathRealSQLPoolSystem {
	return epathRealSQLPoolSystem{
		ID: "pool-shared-plant", PoolName: "Test Pool", SurfaceName: "F1-1", ZoneName: "SPACE1-1",
		HotWaterLoopName: "Hot Water Loop", ChilledWaterLoopName: "Chilled Water Loop",
		BoilerName: "Central Boiler", HotWaterPumpName: "HW Circ Pump", ChilledWaterPumpName: "CW Circ Pump", ChillerName: "Central Chiller",
		AirLoopName: "VAV Sys 1", FanName: "Supply Fan 1", OutdoorAirSystemName: "OA Sys 1", OutdoorAirMixerName: "OA Mixing Box 1",
		MainHeatingCoilName: "Main Heating Coil 1", MainCoolingCoilName: "Main Cooling Coil 1",
		OutdoorHeatingCoilName: "OA Heating Coil 1", OutdoorCoolingCoilName: "OA Cooling Coil 1",
		ReturnPlenumName: "Return-Plenum-1", ReturnPlenumZoneName: "PLENUM-1",
		ServedZones: []string{"SPACE1-1", "SPACE2-1", "SPACE3-1", "SPACE4-1", "SPACE5-1"},
		Terminals: []epathRealSQLPoolTerminal{
			{ZoneName: "SPACE1-1", TerminalName: "SPACE1-1 VAV Reheat", ADUName: "SPACE1-1 ATU", HeatingCoilName: "SPACE1-1 Zone Coil"},
			{ZoneName: "SPACE2-1", TerminalName: "SPACE2-1 VAV Reheat", ADUName: "SPACE2-1 ATU", HeatingCoilName: "SPACE2-1 Zone Coil"},
			{ZoneName: "SPACE3-1", TerminalName: "SPACE3-1 VAV Reheat", ADUName: "SPACE3-1 ATU", HeatingCoilName: "SPACE3-1 Zone Coil"},
			{ZoneName: "SPACE4-1", TerminalName: "SPACE4-1 VAV Reheat", ADUName: "SPACE4-1 ATU", HeatingCoilName: "SPACE4-1 Zone Coil"},
			{ZoneName: "SPACE5-1", TerminalName: "SPACE5-1 VAV Reheat", ADUName: "SPACE5-1 ATU", HeatingCoilName: "SPACE5-1 Zone Coil"},
		},
	}
}

func epathSQLPoolOriginalFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/energy_path_real_models/models/25.1/5ZoneSwimmingPoolZoneMultipliers.idf")
	if err != nil {
		t.Fatal(err)
	}
	if actual := fmt.Sprintf("%x", sha256.Sum256(raw)); actual != "e8fe7fdc91c4127f55f645ac059323771fa4a92f16e86900953fa8d2ceff9301" {
		t.Fatalf("untouched Pool original hash differs: %s", actual)
	}
	return string(raw)
}

func epathSQLPoolMutatedOriginal(t *testing.T, original string, mutate func(*idf.Document)) string {
	t.Helper()
	doc, err := idf.Parse(original)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&doc)
	return doc.String()
}

func epathSQLPoolUnitObject(t *testing.T, doc *idf.Document, typ, name string) *idf.Object {
	t.Helper()
	for i := range doc.Objects {
		if strings.EqualFold(doc.Objects[i].Type, typ) && strings.EqualFold(epathSQLSharedField(doc.Objects[i], 0), name) {
			return &doc.Objects[i]
		}
	}
	t.Fatalf("missing exact original object %s/%s", typ, name)
	return nil
}

func TestEnergyPathSQLPoolOriginalExactSystem(t *testing.T) {
	original := epathSQLPoolOriginalFixture(t)
	d := epathSQLPoolOriginalDeclaration()
	proofs, err := epathSQLValidatePoolOriginal(original, []epathRealSQLPoolSystem{d})
	if err != nil {
		t.Fatal(err)
	}
	if len(proofs) != 1 {
		t.Fatalf("proof count=%d", len(proofs))
	}
	p := proofs[0]
	if p.OriginalSHA256 != "e8fe7fdc91c4127f55f645ac059323771fa4a92f16e86900953fa8d2ceff9301" || p.SurfaceObjectIndex != 78 || !reflect.DeepEqual(p.Declaration, d) || !reflect.DeepEqual(p.ServedZones, d.ServedZones) {
		t.Fatalf("original binding differs: %#v", p)
	}
	wantOwners := []epathSQLPoolOriginalOwner{
		{ObjectType: "SwimmingPool:Indoor", ObjectName: "Test Pool", ObjectIndex: 301, PlantLoopName: "Hot Water Loop", ZoneName: "SPACE1-1"},
		{ObjectType: "Boiler:HotWater", ObjectName: "Central Boiler", ObjectIndex: 264, FuelType: "NaturalGas", PlantLoopName: "Hot Water Loop"},
		{ObjectType: "Pump:VariableSpeed", ObjectName: "HW Circ Pump", ObjectIndex: 267, PlantLoopName: "Hot Water Loop"},
		{ObjectType: "Pump:VariableSpeed", ObjectName: "CW Circ Pump", ObjectIndex: 300, PlantLoopName: "Chilled Water Loop"},
		{ObjectType: "Chiller:Electric", ObjectName: "Central Chiller", ObjectIndex: 297, PlantLoopName: "Chilled Water Loop"},
		{ObjectType: "Fan:VariableVolume", ObjectName: "Supply Fan 1", ObjectIndex: 220},
	}
	if !reflect.DeepEqual(p.Owners, wantOwners) {
		t.Fatalf("native source ownership differs: got %#v want %#v", p.Owners, wantOwners)
	}
	if len(p.HotWaterDemands) != 8 || len(p.ChilledWaterDemands) != 2 {
		t.Fatalf("complete non-pipe demand census HW%d CW%d", len(p.HotWaterDemands), len(p.ChilledWaterDemands))
	}
	hwNames := map[string]string{"SPACE1-1 Zone Coil": "SPACE1-1", "SPACE2-1 Zone Coil": "SPACE2-1", "SPACE3-1 Zone Coil": "SPACE3-1", "SPACE4-1 Zone Coil": "SPACE4-1", "SPACE5-1 Zone Coil": "SPACE5-1", "OA Heating Coil 1": "", "Main Heating Coil 1": "", "Test Pool": "SPACE1-1"}
	for _, owner := range p.HotWaterDemands {
		zone, ok := hwNames[owner.ObjectName]
		wantType := "Coil:Heating:Water"
		if owner.ObjectName == "Test Pool" {
			wantType = "SwimmingPool:Indoor"
		}
		if !ok || owner.ObjectType != wantType || owner.ZoneName != zone || owner.PlantLoopName != "Hot Water Loop" || owner.ObjectIndex < 0 {
			t.Fatalf("wrong HW demand identity: %#v", owner)
		}
		delete(hwNames, owner.ObjectName)
	}
	if len(hwNames) != 0 {
		t.Fatalf("missing original HW demands: %v", hwNames)
	}
	cwNames := map[string]bool{"Main Cooling Coil 1": true, "OA Cooling Coil 1": true}
	for _, owner := range p.ChilledWaterDemands {
		if !cwNames[owner.ObjectName] || owner.ObjectType != "Coil:Cooling:Water" || owner.PlantLoopName != "Chilled Water Loop" || owner.ZoneName != "" {
			t.Fatalf("wrong CW demand identity: %#v", owner)
		}
		delete(cwNames, owner.ObjectName)
	}
	if len(cwNames) != 0 {
		t.Fatal("OA or main cooling demand disappeared")
	}
	for _, zone := range p.ServedZones {
		if zone == "PLENUM-1" {
			t.Fatal("return plenum became a served recipient")
		}
	}
	// Returned declaration and recipient slices cannot be changed through caller
	// storage. This does not turn a shallow or forged proof into evidence: source
	// compilation separately revalidates the entire original-bound proof.
	d.ServedZones[0] = "Mutated"
	d.Terminals[0].ZoneName = "Mutated"
	if p.Declaration.ServedZones[0] != "SPACE1-1" || p.ServedZones[0] != "SPACE1-1" || p.Declaration.Terminals[0].ZoneName != "SPACE1-1" {
		t.Fatal("original proof aliases caller declaration storage")
	}
}

func TestEnergyPathSQLPoolOriginalOptInAndDeclarations(t *testing.T) {
	if result, err := epathSQLValidatePoolOriginal("not an IDF", nil); err != nil || result != nil {
		t.Fatal("absent Pool opt-in changed legacy behavior")
	}
	original := epathSQLPoolOriginalFixture(t)
	mutations := map[string]func(*epathRealSQLPoolSystem){
		"blank key":                   func(d *epathRealSQLPoolSystem) { d.PoolName = " " },
		"unknown original key":        func(d *epathRealSQLPoolSystem) { d.PoolName = "Another Pool" },
		"same HW and CW loop":         func(d *epathRealSQLPoolSystem) { d.ChilledWaterLoopName = d.HotWaterLoopName },
		"same pump identity":          func(d *epathRealSQLPoolSystem) { d.ChilledWaterPumpName = d.HotWaterPumpName },
		"missing served Zone":         func(d *epathRealSQLPoolSystem) { d.ServedZones = d.ServedZones[:4] },
		"duplicate served Zone":       func(d *epathRealSQLPoolSystem) { d.ServedZones[4] = d.ServedZones[0] },
		"plenum fabricated as served": func(d *epathRealSQLPoolSystem) { d.ServedZones[4] = "PLENUM-1"; d.Terminals[4].ZoneName = "PLENUM-1" },
		"terminal owner mislabeled": func(d *epathRealSQLPoolSystem) {
			d.Terminals[0].ZoneName, d.Terminals[1].ZoneName = d.Terminals[1].ZoneName, d.Terminals[0].ZoneName
		},
		"ADU identity swapped":           func(d *epathRealSQLPoolSystem) { d.Terminals[0].ADUName = d.Terminals[1].ADUName },
		"terminal coil identity swapped": func(d *epathRealSQLPoolSystem) { d.Terminals[0].HeatingCoilName = d.Terminals[1].HeatingCoilName },
		"floor borrowed by another Zone": func(d *epathRealSQLPoolSystem) { d.ZoneName = "SPACE2-1" },
		"plenum owns pool":               func(d *epathRealSQLPoolSystem) { d.ZoneName = "PLENUM-1" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			d := epathSQLPoolOriginalDeclaration()
			mutate(&d)
			if _, err := epathSQLValidatePoolOriginal(original, []epathRealSQLPoolSystem{d}); err == nil {
				t.Fatal("invalid original declaration accepted")
			}
		})
	}
	d := epathSQLPoolOriginalDeclaration()
	if _, err := epathSQLValidatePoolOriginal(original, []epathRealSQLPoolSystem{d, d}); err == nil {
		t.Fatal("duplicate system declarations accepted")
	}
}

func TestEnergyPathSQLPoolOriginalRejectsNativeTopologyMutations(t *testing.T) {
	original := epathSQLPoolOriginalFixture(t)
	field := func(typ, name string, at int, value string) func(*idf.Document) {
		return func(doc *idf.Document) {
			o := epathSQLPoolUnitObject(t, doc, typ, name)
			if at >= len(o.Fields) {
				t.Fatalf("mutation field %s/%s[%d] missing", typ, name, at)
			}
			o.Fields[at].Value = value
		}
	}
	appendObject := func(typ string, values ...string) func(*idf.Document) {
		return func(doc *idf.Document) {
			o := idf.Object{Type: typ}
			for _, value := range values {
				o.Fields = append(o.Fields, idf.Field{Value: value})
			}
			doc.Objects = append(doc.Objects, o)
		}
	}
	mutations := map[string]func(*idf.Document){
		"unsupported version":                field("Version", "25.1", 0, "24.2"),
		"pool floor selector blank":          field("SwimmingPool:Indoor", "Test Pool", 1, ""),
		"pool no longer floor":               field("BuildingSurface:Detailed", "F1-1", 1, "Wall"),
		"pool ground boundary lost":          field("BuildingSurface:Detailed", "F1-1", 5, "Adiabatic"),
		"pool surface owned by another Zone": field("BuildingSurface:Detailed", "F1-1", 3, "SPACE2-1"),
		"pool native water inlet":            field("SwimmingPool:Indoor", "Test Pool", 10, "Broken pool inlet"),
		"pool native water outlet blank":     field("SwimmingPool:Indoor", "Test Pool", 11, ""),
		"boiler wrong fuel":                  field("Boiler:HotWater", "Central Boiler", 1, "Electricity"),
		"boiler native inlet":                field("Boiler:HotWater", "Central Boiler", 10, "Broken boiler inlet"),
		"CW pump native inlet":               field("Pump:VariableSpeed", "CW Circ Pump", 1, "Broken CW pump inlet"),
		"chiller no longer AirCooled":        field("Chiller:Electric", "Central Chiller", 1, "WaterCooled"),
		"chiller chilled inlet":              field("Chiller:Electric", "Central Chiller", 4, "Broken chilled inlet"),
		"HW demand outer inlet":              field("PlantLoop", "Hot Water Loop", 14, "Broken HW demand inlet"),
		"shared plant branch list":           field("PlantLoop", "Chilled Water Loop", 16, "Heating Demand Side Branches"),
		"foreign HW splitter member":         field("Connector:Splitter", "Heating Demand Splitter", 9, "Cooling Coil Branch"),
		"duplicate HW mixer member":          field("Connector:Mixer", "Heating Demand Mixer", 9, "SPACE1-1 Reheat Branch"),
		"OA heating outlet":                  field("Coil:Heating:Water", "OA Heating Coil 1", 7, "Broken OA heating outlet"),
		"OA cooling inlet":                   field("Coil:Cooling:Water", "OA Cooling Coil 1", 11, "Broken OA cooling inlet"),
		"mixer outdoor inlet":                field("OutdoorAir:Mixer", "OA Mixing Box 1", 2, "Broken mixer outside inlet"),
		"OA native alias missing":            field("NodeList", "OutsideAirInletNodes", 1, "Different outside node"),
		"OA native alias blank":              field("OutdoorAir:NodeList", "OutsideAirInletNodes", 0, ""),
		"OA controller actuator wrong coil":  field("Controller:WaterCoil", "OA HC Controller 1", 5, "Main Heating Coil 1 Water Inlet Node"),
		"OA controller sensor wrong coil":    field("Controller:WaterCoil", "OA CC Controller 1", 4, "Main Cooling Coil 1 Outlet Node"),
		"OA controller wrong action":         field("Controller:WaterCoil", "OA HC Controller 1", 2, "Reverse"),
		"OA controller outside port":         field("Controller:OutdoorAir", "OA Controller 1", 4, "Broken outside control port"),
		"main air inlet":                     field("AirLoopHVAC", "VAV Sys 1", 6, "Broken main inlet"),
		"fan native outlet":                  field("Fan:VariableVolume", "Supply Fan 1", 16, "Broken fan outlet"),
		"Zone splitter missing terminal":     field("AirLoopHVAC:ZoneSplitter", "Zone Supply Air Splitter 1", 6, "Broken terminal inlet"),
		"terminal native reheat outlet":      field("AirTerminal:SingleDuct:VAV:Reheat", "SPACE2-1 VAV Reheat", 13, "Broken reheat outlet"),
		"blank Zone inlet selector":          field("ZoneHVAC:EquipmentConnections", "SPACE2-1", 2, ""),
		"duplicate equipment owner":          field("ZoneHVAC:EquipmentConnections", "SPACE2-1", 1, "SPACE1-1 Eq"),
		"duplicate Zone air node":            field("ZoneHVAC:EquipmentConnections", "SPACE2-1", 4, "SPACE1-1 Node"),
		"return plenum inlet missing":        field("AirLoopHVAC:ReturnPlenum", "Return-Plenum-1", 9, "Broken Zone return"),
		"return plenum Zone confused":        field("AirLoopHVAC:ReturnPlenum", "Return-Plenum-1", 1, "SPACE1-1"),
		"return path outlet":                 field("AirLoopHVAC:ReturnPath", "ReturnAirPath1", 1, "Broken return path"),
		"hidden foreign demand object":       appendObject("Coil:Heating:Water", "Hidden coil"),
		"duplicate boiler native key":        appendObject("Boiler:Steam", "Central Boiler"),
		"duplicate pump native key":          appendObject("Pump:ConstantSpeed", "CW Circ Pump"),
		"duplicate headered pump native key": appendObject("HeaderedPumps:VariableSpeed", "HW Circ Pump"),
		"duplicate pool native key":          appendObject("SwimmingPool:Indoor", "Test Pool"),
		"duplicate surface reporting key":    appendObject("Floor:Detailed", "F1-1"),
		"duplicate OA NodeList identity":     appendObject("NodeList", "OutsideAirInletNodes", "Outside Air Inlet Node 1"),
		"duplicate OA node declaration":      appendObject("OutdoorAir:Node", "Outside Air Inlet Node 1"),
		"hidden foreign branch":              appendObject("Branch", "Hidden branch", "", "Pipe:Adiabatic", "Hidden pipe", "in", "out"),
		"duplicate air branch owner":         appendObject("BranchList", "Foreign air branches", "VAV Sys 1 Main Branch"),
		"blank unrelated NodeList cannot repair blank selector": func(doc *idf.Document) {
			field("ZoneHVAC:EquipmentConnections", "SPACE2-1", 2, "")(doc)
			appendObject("NodeList", "", "SPACE2-1 In Node")(doc)
		},
		"wrong chiller condenser ports used as chilled": func(doc *idf.Document) {
			chiller := epathSQLPoolUnitObject(t, doc, "Chiller:Electric", "Central Chiller")
			branch := epathSQLPoolUnitObject(t, doc, "Branch", "Central Chiller Branch")
			branch.Fields[4].Value = epathSQLSharedField(*chiller, 6)
			branch.Fields[5].Value = epathSQLSharedField(*chiller, 7)
		},
		"old Branch control stride": func(doc *idf.Document) {
			branch := epathSQLPoolUnitObject(t, doc, "Branch", "Swimming Pool Branch")
			branch.Fields = append(branch.Fields, idf.Field{Value: "Active"})
		},
		"Pool omitted consistently from network": func(doc *idf.Document) {
			for _, spec := range [][2]string{{"BranchList", "Heating Demand Side Branches"}, {"Connector:Splitter", "Heating Demand Splitter"}, {"Connector:Mixer", "Heating Demand Mixer"}} {
				o := epathSQLPoolUnitObject(t, doc, spec[0], spec[1])
				kept := []idf.Field{}
				for _, f := range o.Fields {
					if !strings.EqualFold(strings.TrimSpace(f.Value), "Swimming Pool Branch") {
						kept = append(kept, f)
					}
				}
				o.Fields = kept
			}
		},
		"OA cooling omitted consistently from network": func(doc *idf.Document) {
			for _, spec := range [][2]string{{"BranchList", "Cooling Demand Side Branches"}, {"Connector:Splitter", "CW Demand Splitter"}, {"Connector:Mixer", "CW Demand Mixer"}} {
				o := epathSQLPoolUnitObject(t, doc, spec[0], spec[1])
				kept := []idf.Field{}
				for _, f := range o.Fields {
					if !strings.EqualFold(strings.TrimSpace(f.Value), "OA Cooling Coil Branch") {
						kept = append(kept, f)
					}
				}
				o.Fields = kept
			}
		},
		"hidden equipment consumer": func(doc *idf.Document) {
			o := epathSQLPoolUnitObject(t, doc, "ZoneHVAC:EquipmentList", "SPACE1-1 Eq")
			for _, value := range []string{"ZoneHVAC:Baseboard:Convective:Electric", "Hidden baseboard", "2", "2", "", ""} {
				o.Fields = append(o.Fields, idf.Field{Value: value})
			}
		},
		"blank malformed branch": func(doc *idf.Document) {
			o := epathSQLPoolUnitObject(t, doc, "Branch", "Swimming Pool Branch")
			o.Fields = o.Fields[:1]
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			mutated := epathSQLPoolMutatedOriginal(t, original, mutate)
			if _, err := epathSQLValidatePoolOriginal(mutated, []epathRealSQLPoolSystem{epathSQLPoolOriginalDeclaration()}); err == nil {
				t.Fatal("incomplete, disconnected or ambiguous native original accepted")
			}
		})
	}
}

func TestEnergyPathSQLPoolOriginalControllerOrderAndLexicalPreservation(t *testing.T) {
	original := epathSQLPoolOriginalFixture(t)
	// Actual OA controller order is OA, cooling, heating, while physical outside
	// equipment order is heating, cooling, mixer. Reordering the two water-coil
	// controller references must not alter ownership established by native ports.
	mutated := epathSQLPoolMutatedOriginal(t, original, func(doc *idf.Document) {
		o := epathSQLPoolUnitObject(t, doc, "AirLoopHVAC:ControllerList", "OA Sys 1 Controllers")
		o.Fields[3], o.Fields[5] = o.Fields[5], o.Fields[3]
		o.Fields[4], o.Fields[6] = o.Fields[6], o.Fields[4]
		// An unselected nameless list is unrelated evidence, never a blank-node
		// lookup target. Its presence cannot panic an otherwise valid graph.
		doc.Objects = append(doc.Objects, idf.Object{Type: "NodeList", Fields: []idf.Field{{Value: ""}}})
	})
	proofs, err := epathSQLValidatePoolOriginal(mutated, []epathRealSQLPoolSystem{epathSQLPoolOriginalDeclaration()})
	if err != nil {
		t.Fatal(err)
	}
	if len(proofs) != 1 || proofs[0].OriginalSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(mutated))) {
		t.Fatal("proof hash is not bound to the exact supplied lexical document")
	}
	if proofs[0].OriginalSHA256 == fmt.Sprintf("%x", sha256.Sum256([]byte(original))) {
		t.Fatal("reformatted original was silently claimed as the saved original")
	}
	if after := epathSQLPoolOriginalFixture(t); after != original {
		t.Fatal("original fixture file changed")
	}
}
