package idf

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Literal original component indices and native port roles, not names or
// production scalar output, define the intended five-Zone service cohorts.
func assertCentralHeatPumpOriginalRoutes(t *testing.T, report HVACReport, cooling, heating int) {
	t.Helper()
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, summary := range report.ServiceModel.ZoneServices {
		for _, path := range summary.Paths {
			if path.SourceSystem == nil || !strings.HasPrefix(strings.ToLower(path.SourceSystem.Name), "centralheatpumpsystem ") {
				continue
			}
			zone := 0
			for number := 1; number <= 5; number++ {
				if path.ZoneName == fmt.Sprintf("SPACE%d-1", number) {
					zone = number
				}
			}
			if zone == 0 || path.SpaceName != "" || path.PlantLoop == nil || path.AirLoop == nil || path.AirLoop.ObjectIndex != 178 || path.CondenserLoop != nil || path.RefrigerantSystem != nil || path.PathType != "central_air_with_plant" || path.Delivery.ObjectIndex != 148+zone || path.Delivery.ObjectType != "AirTerminal:SingleDuct:ConstantVolume:Reheat" || path.DeliveryWrapper == nil || path.DeliveryWrapper.ObjectIndex != 153+zone || len(path.Conditioning) != 1 {
				t.Fatalf("Central path borrowed source loop/plenum/foreign terminal or lacks exact coil: %#v", path)
			}
			key := path.ZoneName + "/" + path.ServiceKind
			if seen[key] {
				t.Fatalf("duplicate native service occurrence: %s", key)
			}
			seen[key] = true
			switch path.ServiceKind {
			case "cooling":
				if path.PlantLoop.ObjectIndex != 274 || path.Conditioning[0].ObjectIndex != 170 || path.Conditioning[0].ObjectType != "Coil:Cooling:Water:DetailedGeometry" {
					t.Fatalf("cooling route crossed native water/air roles: %#v", path)
				}
			case "heating":
				if path.PlantLoop.ObjectIndex != 275 || path.Conditioning[0].ObjectIndex != 170+zone || path.Conditioning[0].ObjectType != "Coil:Heating:Water" {
					t.Fatalf("heating route was dropped or relabeled through ChillerBank: %#v", path)
				}
			default:
				t.Fatalf("source exchange became space service: %#v", path)
			}
			counts[path.ServiceKind]++
		}
	}
	if counts["cooling"] != cooling || counts["heating"] != heating {
		t.Fatalf("native exact five-Zone service census=%v, want cooling%d heating%d", counts, cooling, heating)
	}
}

func TestNativeCentralHeatPumpIntegrationOriginalBothSupplyOccurrencesAndFiveRoutes(t *testing.T) {
	for _, stripped := range []bool{false, true} {
		doc := nativeCentralHeatPumpOriginal(t)
		if stripped {
			for oi := range doc.Objects {
				for fi := range doc.Objects[oi].Fields {
					doc.Objects[oi].Fields[fi].Comment = ""
				}
			}
		}
		before := doc.String()
		report := AnalyzeHVAC(doc)
		if doc.String() != before {
			t.Fatal("analysis modified original input")
		}
		assertCentralHeatPumpOriginalRoutes(t, report, 5, 5)
		ctx := newHVACContext(doc)
		selected := plantSourceEquipmentForLoopNames(ctx, report.Loops, []string{"Hot Water Loop", "Chilled Water Loop"})
		ports := map[int]string{}
		for _, source := range selected {
			if source.ObjectType != nativeCentralHeatPumpType {
				continue
			}
			port, ok := hvacCentralHeatPumpSupplyPort(ctx, source, source.LoopName)
			if !ok || source.ObjectIndex != 269 {
				t.Fatalf("unbound original Central occurrence: %#v", source)
			}
			ports[port.Loop.ObjectIndex] = port.Role
		}
		if !reflect.DeepEqual(ports, map[int]string{274: "cooling", 275: "heating"}) {
			t.Fatalf("object-only seen key lost a native supply occurrence: %v", ports)
		}
	}
}

func TestNativeCentralHeatPumpIntegrationNativeRoleIgnoresChillerAndHeatingNames(t *testing.T) {
	doc := nativeCentralHeatPumpOriginal(t)
	for oi := range doc.Objects {
		object := &doc.Objects[oi]
		if object.Type == nativeCentralHeatPumpType {
			object.Fields[0].Value = "Chiller Cooling Heater Both"
		}
		if object.Type == "Branch" {
			for field := 2; field+3 < len(object.Fields); field += 4 {
				if object.Fields[field].Value == nativeCentralHeatPumpType {
					object.Fields[field+1].Value = "Chiller Cooling Heater Both"
				}
			}
		}
		if object.Type == "Coil:Cooling:Water:DetailedGeometry" || object.Type == "Coil:Heating:Water" {
			for fi := range object.Fields {
				object.Fields[fi].Comment = "Water Inlet Node Name"
			}
		}
	}
	assertCentralHeatPumpOriginalRoutes(t, AnalyzeHVAC(doc), 5, 5)
}

func TestNativeCentralHeatPumpIntegrationRejectsBrokenSelectedNativeRoutes(t *testing.T) {
	for _, test := range []struct {
		name, kind, object      string
		field, cooling, heating int
	}{
		{"cooling native water inlet", "Coil:Cooling:Water:DetailedGeometry", "Timestep Cooling Coil", 18, 0, 0},
		{"cooling native air outlet", "Coil:Cooling:Water:DetailedGeometry", "Timestep Cooling Coil", 21, 0, 0},
		{"main fan native inlet", "Fan:ConstantVolume", "Supply Fan 1", 7, 0, 0},
		{"heating native water inlet", "Coil:Heating:Water", "Reheat Coil Zone 1", 4, 5, 0},
		{"heating native air inlet", "Coil:Heating:Water", "Reheat Coil Zone 1", 6, 0, 0},
		{"terminal native inlet", "AirTerminal:SingleDuct:ConstantVolume:Reheat", "Reheat Zone 1", 3, 0, 0},
		{"terminal native coil ref", "AirTerminal:SingleDuct:ConstantVolume:Reheat", "Reheat Zone 1", 6, 0, 0},
		{"ADU native outlet", "ZoneHVAC:AirDistributionUnit", "Zone1TermReheat", 1, 0, 0},
		{"system heating outlet", nativeCentralHeatPumpType, "ChillerBank", 7, 0, 0},
		{"system source outlet", nativeCentralHeatPumpType, "ChillerBank", 5, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc := nativeCentralHeatPumpOriginal(t)
			centralHeatPumpTestObject(t, &doc, test.kind, test.object).Fields[test.field].Value = "Disconnected native port"
			before := doc.String()
			assertCentralHeatPumpOriginalRoutes(t, AnalyzeHVAC(doc), test.cooling, test.heating)
			if doc.String() != before {
				t.Fatal("route guard rewrote input")
			}
		})
	}
}
