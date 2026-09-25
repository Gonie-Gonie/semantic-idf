package idf

import "testing"

func TestNativeCentralHeatPumpWholeRecipientRoster(t *testing.T) {
	for _, test := range []struct {
		name, typ, object string
		field             int
	}{
		{"controller sensor", "Controller:WaterCoil", "Main Cooling Coil Controller", 4},
		{"controller actuator", "Controller:WaterCoil", "Main Cooling Coil Controller", 5},
		{"controller list", "AirLoopHVAC:ControllerList", "Reheat System 1 Controllers", 2},
		{"supply splitter", "AirLoopHVAC:ZoneSplitter", "Zone Supply Air Splitter", 6},
		{"supply path", "AirLoopHVAC:SupplyPath", "TermReheatSupplyPath", 1},
		{"return path", "AirLoopHVAC:ReturnPath", "ReturnAirPath1", 1},
		{"plenum outlet", "AirLoopHVAC:ReturnPlenum", "Return-Plenum-1", 3},
		{"plenum last inlet", "AirLoopHVAC:ReturnPlenum", "Return-Plenum-1", 9},
		{"equipment type", "ZoneHVAC:EquipmentList", "Zone5Equipment", 2},
		{"equipment owner", "ZoneHVAC:EquipmentConnections", "SPACE5-1", 1},
		{"Zone inlet selector", "NodeList", "Zone5Inlets", 1},
		{"Zone return", "ZoneHVAC:EquipmentConnections", "SPACE5-1", 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc := nativeCentralHeatPumpOriginal(t)
			centralHeatPumpTestObject(t, &doc, test.typ, test.object).Fields[test.field].Value = "Unbound original node"
			assertCentralHeatPumpOriginalRoutes(t, AnalyzeHVAC(doc), 0, 0)
			bindings := ResolveNativeCentralHeatPumpBindings(doc)
			if len(bindings) != 1 || !bindings[0].ReportingIdentityValid {
				t.Fatal("air recipient failure erased independently known reporting identity")
			}
		})
	}
	for _, typName := range [][2]string{{"ZoneHVAC:EquipmentConnections", "SPACE1-1"}, {"ZoneHVAC:EquipmentList", "Zone1Equipment"}, {"AirLoopHVAC:ReturnPath", "ReturnAirPath1"}, {"Controller:WaterCoil", "Main Cooling Coil Controller"}} {
		t.Run("duplicate "+typName[0], func(t *testing.T) {
			doc := nativeCentralHeatPumpOriginal(t)
			centralHeatPumpTestCopy(&doc, *centralHeatPumpTestObject(t, &doc, typName[0], typName[1]))
			assertCentralHeatPumpOriginalRoutes(t, AnalyzeHVAC(doc), 0, 0)
		})
	}
}
