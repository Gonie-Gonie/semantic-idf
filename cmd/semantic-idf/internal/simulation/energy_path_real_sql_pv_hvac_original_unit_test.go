package simulation

// Ignored, uninstalled test-only draft. These fixtures contain no native or
// candidate energy quantities; only the pinned original and literal mutations.
import (
	"reflect"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLPVHVACOriginalDeclarations() []epathRealSQLPVHVACLoop {
	return []epathRealSQLPVHVACLoop{
		{ZoneName: "ZN_1_FLR_1_SEC_1", AirLoopName: "ZN_1_FLR_1_SEC_1:Sys", CoolingWrapperName: "ZN_1_FLR_1_SEC_1:SysCoolC", CoolingCoilName: "ZN_1_FLR_1_SEC_1:SysCoolC DXCoil", HeatingCoilName: "ZN_1_FLR_1_SEC_1:SysHeatC", FanName: "ZN_1_FLR_1_SEC_1:Sys Fan", TerminalName: "ZN_1_FLR_1_SEC_1 Direct Air", ADUName: "ZN_1_FLR_1_SEC_1 Direct Air ADU"},
		{ZoneName: "ZN_1_FLR_1_SEC_2", AirLoopName: "ZN_1_FLR_1_SEC_2:Sys", CoolingWrapperName: "ZN_1_FLR_1_SEC_2:SysCoolC", CoolingCoilName: "ZN_1_FLR_1_SEC_2:SysCoolC DXCoil", HeatingCoilName: "ZN_1_FLR_1_SEC_2:SysHeatC", FanName: "ZN_1_FLR_1_SEC_2:Sys Fan", TerminalName: "ZN_1_FLR_1_SEC_2 Direct Air", ADUName: "ZN_1_FLR_1_SEC_2 Direct Air ADU"},
		{ZoneName: "ZN_1_FLR_1_SEC_3", AirLoopName: "ZN_1_FLR_1_SEC_3:Sys", CoolingWrapperName: "ZN_1_FLR_1_SEC_3:SysCoolC", CoolingCoilName: "ZN_1_FLR_1_SEC_3:SysCoolC DXCoil", HeatingCoilName: "ZN_1_FLR_1_SEC_3:SysHeatC", FanName: "ZN_1_FLR_1_SEC_3:Sys Fan", TerminalName: "ZN_1_FLR_1_SEC_3 Direct Air", ADUName: "ZN_1_FLR_1_SEC_3 Direct Air ADU"},
		{ZoneName: "ZN_1_FLR_1_SEC_4", AirLoopName: "ZN_1_FLR_1_SEC_4:Sys", CoolingWrapperName: "ZN_1_FLR_1_SEC_4:SysCoolC", CoolingCoilName: "ZN_1_FLR_1_SEC_4:SysCoolC DXCoil", HeatingCoilName: "ZN_1_FLR_1_SEC_4:SysHeatC", FanName: "ZN_1_FLR_1_SEC_4:Sys Fan", TerminalName: "ZN_1_FLR_1_SEC_4 Direct Air", ADUName: "ZN_1_FLR_1_SEC_4 Direct Air ADU"},
		{ZoneName: "ZN_1_FLR_1_SEC_5", AirLoopName: "ZN_1_FLR_1_SEC_5:Sys", CoolingWrapperName: "ZN_1_FLR_1_SEC_5:SysCoolC", CoolingCoilName: "ZN_1_FLR_1_SEC_5:SysCoolC DXCoil", HeatingCoilName: "ZN_1_FLR_1_SEC_5:SysHeatC", FanName: "ZN_1_FLR_1_SEC_5:Sys Fan", TerminalName: "ZN_1_FLR_1_SEC_5 Direct Air", ADUName: "ZN_1_FLR_1_SEC_5 Direct Air ADU"},
	}
}

func epathSQLPVHVACUnitDocument(t *testing.T, original string) idf.Document {
	t.Helper()
	doc, err := idf.Parse(original)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestEnergyPathSQLPVHVACOriginalExactFiveAirLoopsAndSeparateSHW(t *testing.T) {
	original := epathSQLPVOriginalFixture(t)
	declarations := epathSQLPVHVACOriginalDeclarations()
	proof, err := epathSQLValidatePVHVACOriginal(original, declarations)
	if err != nil {
		t.Fatal(err)
	}
	if len(proof.Loops) != 5 || proof.DomesticWaterLoop != "SHWSys1" || proof.DomesticPump != "SHWSys1 Pump" || proof.DomesticWaterHeater != "SHWSys1 Water Heater" || proof.OriginalSHA256 != "9c3f9b637b1e04c2e4b8911854c36ffd6442ea86cfe8e165fccaaeb8b2be9cd0" {
		t.Fatalf("incomplete/borrowed physical proof: %#v", proof)
	}
	doc := epathSQLPVHVACUnitDocument(t, original)
	if !reflect.DeepEqual(proof.Objects, epathSQLPVHVACPhysical(doc)) {
		t.Fatal("physical fields lost")
	}
	if len(proof.Objects) != 466 {
		t.Fatalf("complete physical census changed: got %d, want 466", len(proof.Objects))
	}
	declarations[0].ZoneName = "caller mutation"
	if proof.Loops[0].ZoneName == declarations[0].ZoneName {
		t.Fatal("proof aliases declaration")
	}
	before := proof.Objects[0].Fields[0]
	doc.Objects[0].Fields[0].Value = "caller physical mutation"
	if proof.Objects[0].Fields[0] != before {
		t.Fatal("proof aliases parsed physical fields")
	}
	if p, err := epathSQLValidatePVHVACOriginal("", nil); err != nil || !reflect.DeepEqual(p, epathSQLPVHVACOriginalProof{}) {
		t.Fatal("absent Shop declaration changed legacy applicability")
	}
	if _, err := epathSQLValidatePVHVACOriginal("", epathSQLPVHVACOriginalDeclarations()); err == nil {
		t.Fatal("missing original accepted")
	}
}

func TestEnergyPathSQLPVHVACOriginalPortOwnerAndControlMutations(t *testing.T) {
	original := epathSQLPVOriginalFixture(t)
	declarations := epathSQLPVHVACOriginalDeclarations()
	d, e := declarations[0], declarations[1]
	cases := []struct {
		name, typ, owner string
		field            int
		value            string
	}{
		{"loop branch ownership", "AirLoopHVAC", d.AirLoopName, 4, e.AirLoopName + " Air Loop Branches"},
		{"supply outer port", "AirLoopHVAC", d.AirLoopName, 6, "broken supply inlet"},
		{"stride4 internal connection", "Branch", d.AirLoopName + " Air Loop Main Branch", 9, "broken DX outlet"},
		{"DX child another owner", "CoilSystem:Cooling:DX", d.CoolingWrapperName, 6, e.CoolingCoilName},
		{"DX child wrong native type", "CoilSystem:Cooling:DX", d.CoolingWrapperName, 5, "Coil:Cooling:DX:TwoSpeed"},
		{"DX child native inlet", "Coil:Cooling:DX:SingleSpeed", d.CoolingCoilName, 8, "broken child inlet"},
		{"DX sensing node", "CoilSystem:Cooling:DX", d.CoolingWrapperName, 4, "broken outlet sensor"},
		{"DX missing native curve", "Coil:Cooling:DX:SingleSpeed", d.CoolingCoilName, 10, "missing curve"},
		{"electric heater native port", "Coil:Heating:Electric", d.HeatingCoilName, 4, "broken heater inlet"},
		{"electric heater sensor", "Coil:Heating:Electric", d.HeatingCoilName, 6, "broken heater sensor"},
		{"CV fan native inlet7", "Fan:ConstantVolume", d.FanName, 7, "broken fan inlet"},
		{"CV NoReheat native inlet2", "AirTerminal:SingleDuct:ConstantVolume:NoReheat", d.TerminalName, 2, "broken terminal inlet"},
		{"CV NoReheat native outlet3", "AirTerminal:SingleDuct:ConstantVolume:NoReheat", d.TerminalName, 3, "broken terminal outlet"},
		{"ADU wrong native outlet", "ZoneHVAC:AirDistributionUnit", d.ADUName, 1, "broken ADU outlet"},
		{"second equipment owner", "ZoneHVAC:EquipmentList", e.ZoneName + " Equipment", 3, d.ADUName},
		{"wrong Zone inlet NodeList", "NodeList", d.ZoneName + " Inlet Nodes", 1, e.ZoneName + " Direct Air Inlet Node"},
		{"return owner mismatch", "AirLoopHVAC:ZoneMixer", d.AirLoopName + " Return Air Mixer", 2, e.ZoneName + " Return Air Node"},
		{"OA controller return port", "Controller:OutdoorAir", "Controller" + d.AirLoopName + "_OA", 2, "broken OA return"},
		{"OA node selector resolves another node", "NodeList", d.AirLoopName + "_OANode List", 1, e.AirLoopName + "_OAInlet Node"},
		{"Zone setpoint owner mismatch", "SetpointManager:SingleZone:Reheat", "SupAirTemp Mngr" + d.ZoneName, 4, e.ZoneName},
		{"mixed-air fan compensation", "SetpointManager:MixedAir", d.AirLoopName + "_OAMixed Air Temp Manager", 3, "broken fan compensation inlet"},
		{"missing availability schedule", "AvailabilityManager:NightCycle", d.AirLoopName + " Availability Manager", 2, "missing schedule"},
		{"SHW shared HVAC recipient", "WaterUse:Equipment", d.ZoneName + "SHW_DEFAULT", 7, e.ZoneName},
		{"SHW supply topology", "PlantLoop", "SHWSys1", 10, "broken water inlet"},
		{"SHW heater native use inlet30", "WaterHeater:Mixed", "SHWSys1 Water Heater", 30, "broken heater water inlet"},
		{"SHW source-side coupling", "WaterHeater:Mixed", "SHWSys1 Water Heater", 33, "unreviewed source inlet"},
		{"SHW fuel differs", "WaterHeater:Mixed", "SHWSys1 Water Heater", 10, "NaturalGas"},
		{"SHW operation equipment mismatch", "PlantEquipmentList", "SHWSys1 Equipment List", 2, d.HeatingCoilName},
		{"Zone multiplier not one", "Zone", d.ZoneName, 6, "3"},
		{"Go hex is not native decimal", "Zone", d.ZoneName, 6, "0x1p0"},
		{"Go underscore is not native decimal", "Zone", d.ZoneName, 6, "0_1"},
		{"unreviewed version", "Version", "25.1", 0, "24.2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := epathSQLPVHVACUnitDocument(t, original)
			o := epathSQLPVUnitObject(t, &doc, tc.typ, tc.owner)
			o.Fields[tc.field].Value = tc.value
			if _, err := epathSQLValidatePVHVACOriginal(doc.String(), declarations); err == nil {
				t.Fatal("mutated original topology/control accepted")
			}
		})
	}
}

func TestEnergyPathSQLPVHVACOriginalHiddenBlankAndMalformedDenials(t *testing.T) {
	original := epathSQLPVOriginalFixture(t)
	declarations := epathSQLPVHVACOriginalDeclarations()
	d := declarations[0]
	for _, tc := range []struct {
		name   string
		mutate func(*idf.Document)
	}{
		{"hidden sixth fan", func(doc *idf.Document) {
			o := *epathSQLPVUnitObject(t, doc, "Fan:ConstantVolume", d.FanName)
			doc.Objects = append(doc.Objects, o)
		}},
		{"hidden sixth AirLoop", func(doc *idf.Document) {
			o := *epathSQLPVUnitObject(t, doc, "AirLoopHVAC", d.AirLoopName)
			doc.Objects = append(doc.Objects, o)
		}},
		{"unreviewed VAV terminal", func(doc *idf.Document) {
			epathSQLPVUnitObject(t, doc, "AirTerminal:SingleDuct:ConstantVolume:NoReheat", d.TerminalName).Type = "AirTerminal:SingleDuct:VAV:Reheat"
		}},
		{"hidden Zone heater", func(doc *idf.Document) {
			doc.Objects = append(doc.Objects, idf.Object{Type: "ZoneHVAC:Baseboard:Convective:Electric", Fields: []idf.Field{{Value: "unreviewed"}}})
		}},
		{"duplicate NodeList owner", func(doc *idf.Document) {
			o := *epathSQLPVUnitObject(t, doc, "NodeList", d.ZoneName+" Inlet Nodes")
			doc.Objects = append(doc.Objects, o)
		}},
		{"extra OA equipment", func(doc *idf.Document) {
			o := epathSQLPVUnitObject(t, doc, "AirLoopHVAC:OutdoorAirSystem:EquipmentList", d.AirLoopName+"_OAEquipment")
			o.Fields = append(o.Fields, idf.Field{Value: "Coil:Heating:Electric"}, idf.Field{Value: d.HeatingCoilName})
		}},
		{"blank selector cannot select nameless NodeList", func(doc *idf.Document) {
			epathSQLPVUnitObject(t, doc, "ZoneHVAC:EquipmentConnections", d.ZoneName).Fields[2].Value = ""
			epathSQLPVUnitObject(t, doc, "NodeList", d.ZoneName+" Inlet Nodes").Fields[0].Value = ""
		}},
		{"empty NodeList cannot panic", func(doc *idf.Document) {
			epathSQLPVUnitObject(t, doc, "NodeList", d.ZoneName+" Inlet Nodes").Fields = nil
		}},
		{"truncated main branch", func(doc *idf.Document) {
			o := epathSQLPVUnitObject(t, doc, "Branch", d.AirLoopName+" Air Loop Main Branch")
			o.Fields = o.Fields[:2]
		}},
		{"hidden SHW demand", func(doc *idf.Document) {
			o := epathSQLPVUnitObject(t, doc, "BranchList", "SHWSys1 Demand Branches")
			o.Fields = append(o.Fields, idf.Field{Value: d.AirLoopName + " Air Loop Main Branch"})
		}},
		{"SHW connector loses one demand", func(doc *idf.Document) {
			for i := range doc.Objects {
				o := &doc.Objects[i]
				if o.Type == "Connector:Splitter" && len(o.Fields) == 8 {
					o.Fields[2].Value = o.Fields[3].Value
					return
				}
			}
			t.Fatal("missing SHW demand splitter")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := epathSQLPVHVACUnitDocument(t, original)
			tc.mutate(&doc)
			if _, err := epathSQLValidatePVHVACOriginal(doc.String(), declarations); err == nil {
				t.Fatal("hidden/blank/malformed original accepted")
			}
		})
	}
	declarations[1] = declarations[0]
	if _, err := epathSQLValidatePVHVACOriginal(original, declarations); err == nil {
		t.Fatal("duplicate declaration accepted")
	}
}

func TestEnergyPathSQLPVHVACExecutedPhysicalBindingAndIndependentIndices(t *testing.T) {
	original := epathSQLPVOriginalFixture(t)
	declarations := epathSQLPVHVACOriginalDeclarations()
	proof, err := epathSQLValidatePVHVACOriginal(original, declarations)
	if err != nil {
		t.Fatal(err)
	}
	executed := "Output:Variable,*,Site Outdoor Air Drybulb Temperature,Hourly;\n" + original
	objects, err := epathSQLBindPVHVACExecuted(original, executed, proof)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != len(proof.Objects) {
		t.Fatal("executed physical roster changed")
	}
	for n, o := range objects {
		if o.ObjectIndex != proof.Objects[n].ObjectIndex+1 || !reflect.DeepEqual(o.Fields, proof.Objects[n].Fields) {
			t.Fatal("executed index not separately parsed or physical fields changed")
		}
	}
	// This +1 is a known hand-fixture insertion assertion, never the binder's
	// algorithm. Two new outputs must produce a different, independently read shift.
	more := "Output:Variable,*,Zone Mean Air Temperature,Hourly;\n" + executed
	shifted, err := epathSQLBindPVHVACExecuted(original, more, proof)
	if err != nil {
		t.Fatal(err)
	}
	if shifted[0].ObjectIndex != objects[0].ObjectIndex+1 {
		t.Fatal("binder inferred fixed original/executed offset")
	}
	for _, tc := range []struct {
		name, typ, owner string
		field            int
		value            string
	}{
		{"geometry", "Zone", declarations[0].ZoneName, 1, "0.125"},
		{"fan performance", "Fan:ConstantVolume", declarations[0].FanName, 3, "777.0"},
		{"control schedule", "Schedule:Compact", "ALWAYS_ON", 5, "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := epathSQLPVHVACUnitDocument(t, executed)
			o := epathSQLPVUnitObject(t, &doc, tc.typ, tc.owner)
			if tc.field >= len(o.Fields) {
				t.Fatal("incorrect literal mutation field")
			}
			o.Fields[tc.field].Value = tc.value
			if _, err := epathSQLBindPVHVACExecuted(original, doc.String(), proof); err == nil {
				t.Fatal("executed physical/control mutation accepted")
			}
		})
	}
	objects[0].Fields[0] = "detached caller mutation"
	if proof.Objects[0].Fields[0] == objects[0].Fields[0] {
		t.Fatal("executed fields alias original proof")
	}
	proof.Objects[0].ObjectIndex++
	if _, err := epathSQLBindPVHVACExecuted(original, executed, proof); err == nil {
		t.Fatal("forged original index stamp accepted")
	}
	if _, err := epathSQLBindPVHVACExecuted(original, executed, epathSQLPVHVACOriginalProof{}); err == nil {
		t.Fatal("removed executed-binding proof accepted")
	}
}

func epathSQLPVHVACUnitInputs() ([]epathRealSQLFanPool, []epathRealSQLService, []epathRealSQLAuxiliary) {
	var pools []epathRealSQLFanPool
	var zones []string
	for _, d := range epathSQLPVHVACOriginalDeclarations() {
		zones = append(zones, d.ZoneName)
		pools = append(pools, epathRealSQLFanPool{SiteID: "fans.electricity", Name: "Air System Fan Electricity Energy", Key: d.AirLoopName, Frequency: "Hourly", Unit: "J", ServedZones: []string{d.ZoneName}})
	}
	services := []epathRealSQLService{
		{Service: "cooling", SiteIDs: []string{"cooling.electricity"}, ServedZones: append([]string(nil), zones...), Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: "coefficient_of_performance", FallbackRatioKind: "coefficient_of_performance"},
		{Service: "heating", SiteIDs: []string{"heating.electricity"}, ServedZones: append([]string(nil), zones...), Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: "load_to_site_energy", FallbackRatioKind: "load_to_site_energy"},
	}
	return pools, services, []epathRealSQLAuxiliary{{SiteID: "pumps.electricity", Weight: "unassigned", ReconciliationID: "allocation.pumps.annual"}}
}

func TestEnergyPathSQLPVHVACFanPoolServiceAndSHWDeclarationGate(t *testing.T) {
	original := epathSQLPVOriginalFixture(t)
	proof, err := epathSQLValidatePVHVACOriginal(original, epathSQLPVHVACOriginalDeclarations())
	if err != nil {
		t.Fatal(err)
	}
	pools, services, aux := epathSQLPVHVACUnitInputs()
	if err := epathSQLPVHVACDeclaredInputs(original, proof, pools, services, aux); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*[]epathRealSQLFanPool, *[]epathRealSQLService, *[]epathRealSQLAuxiliary)
	}{
		{"missing pool", func(p *[]epathRealSQLFanPool, _ *[]epathRealSQLService, _ *[]epathRealSQLAuxiliary) { *p = (*p)[:4] }},
		{"fan physical key is not AirLoop output key", func(p *[]epathRealSQLFanPool, _ *[]epathRealSQLService, _ *[]epathRealSQLAuxiliary) {
			(*p)[0].Key = proof.Loops[0].FanName
		}},
		{"fan borrowed Zone", func(p *[]epathRealSQLFanPool, _ *[]epathRealSQLService, _ *[]epathRealSQLAuxiliary) {
			(*p)[0].ServedZones[0] = proof.Loops[1].ZoneName
		}},
		{"fan wrong native frequency", func(p *[]epathRealSQLFanPool, _ *[]epathRealSQLService, _ *[]epathRealSQLAuxiliary) {
			(*p)[0].Frequency = "Monthly"
		}},
		{"missing heating", func(_ *[]epathRealSQLFanPool, s *[]epathRealSQLService, _ *[]epathRealSQLAuxiliary) { *s = (*s)[:1] }},
		{"gas service copied from SmallOffice", func(_ *[]epathRealSQLFanPool, s *[]epathRealSQLService, _ *[]epathRealSQLAuxiliary) {
			(*s)[1].SiteIDs = []string{"heating.natural_gas"}
		}},
		{"SHW added to space heating", func(_ *[]epathRealSQLFanPool, s *[]epathRealSQLService, _ *[]epathRealSQLAuxiliary) {
			(*s)[1].SiteIDs = append((*s)[1].SiteIDs, "water_systems.electricity")
		}},
		{"wrong heating ratio", func(_ *[]epathRealSQLFanPool, s *[]epathRealSQLService, _ *[]epathRealSQLAuxiliary) {
			(*s)[1].RatioKind = "thermal_efficiency"
		}},
		{"missing served Zone", func(_ *[]epathRealSQLFanPool, s *[]epathRealSQLService, _ *[]epathRealSQLAuxiliary) {
			(*s)[0].ServedZones = (*s)[0].ServedZones[:4]
		}},
		{"SHW pump assigned by HVAC loads", func(_ *[]epathRealSQLFanPool, _ *[]epathRealSQLService, a *[]epathRealSQLAuxiliary) {
			(*a)[0].ServedZones = []string{proof.Loops[0].ZoneName}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, s, a := epathSQLPVHVACUnitInputs()
			tc.mutate(&p, &s, &a)
			if err := epathSQLPVHVACDeclaredInputs(original, proof, p, s, a); err == nil {
				t.Fatal("unbound pool/service/SHW declaration accepted")
			}
		})
	}
	proof.Loops[0].FanName = "forged stamp"
	if err := epathSQLPVHVACDeclaredInputs(original, proof, pools, services, aux); err == nil {
		t.Fatal("removed/forged topology authority accepted")
	}
}
