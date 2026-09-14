package idf

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
)

const hvacDistrictCoolingLoop = "Chilled Water Loop Chilled Water Loop"
const hvacDistrictHeatingLoop = "Hot Water Loop Hot Water Loop"

func hvacDistrictOriginal(t *testing.T) Document {
	t.Helper()
	const path = "../simulation/testdata/energy_path_real_models/models/25.1/5ZoneFanCoilDOAS_ERVOnAirLoopMainBranch.idf"
	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(input)); got != "3b7712676a8ea7cb034cec7eb0b843731d12ba79cf72d3d54f3b3e6481b765da" {
		t.Fatalf("original District input changed: %s", got)
	}
	t.Cleanup(func() {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(input, after) {
			t.Error("District topology test changed the immutable original input")
		}
	})
	doc, err := Parse(string(input))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func hvacDistrictObject(t *testing.T, doc Document, kind, name string) Object {
	t.Helper()
	var result Object
	count := 0
	for _, object := range doc.Objects {
		if strings.EqualFold(object.Type, kind) && strings.EqualFold(objectName(object), name) {
			result, count = object, count+1
		}
	}
	if count != 1 {
		t.Fatalf("original typed object %s/%s occurs %d times", kind, name, count)
	}
	return result
}

func hvacDistrictAssertWaterBranch(t *testing.T, doc Document, report HVACReport, owner, service string) {
	t.Helper()
	prefix, loop, suffix := "Cooling", hvacDistrictCoolingLoop, "ChW"
	if service == "heating" {
		prefix, loop, suffix = "Heating", hvacDistrictHeatingLoop, "HW"
	}
	name, kind := owner+" "+prefix+" Coil", "Coil:"+prefix+":Water"
	coil := hvacDistrictObject(t, doc, kind, name)
	inlet, outlet := name+" "+suffix+" Inlet", name+" "+suffix+" Outlet"
	if fieldValueByCatalogName(coil, "Water Inlet Node Name") != inlet || fieldValueByCatalogName(coil, "Water Outlet Node Name") != outlet {
		t.Fatalf("original %s has different water ports", name)
	}
	water, air := 0, 0
	for _, actualLoop := range report.Loops {
		for _, side := range []HVACLoopSide{actualLoop.SupplySide, actualLoop.DemandSide} {
			for _, branch := range side.Branches {
				for _, component := range branch.Components {
					if component.ObjectType != kind || component.ObjectName != name {
						continue
					}
					if !component.Exists || component.ObjectIndex != coil.Index {
						t.Fatalf("branch lost exact typed physical identity: %#v", component)
					}
					branchObject := hvacDistrictObject(t, doc, "Branch", branch.Name)
					gotIn := hvacFieldValue(branchObject, component.InletFieldIndex)
					gotOut := hvacFieldValue(branchObject, component.OutletFieldIndex)
					if actualLoop.Type == "AirLoopHVAC" {
						air++
						if owner != "DOAS" || actualLoop.Name != "DOAS" || branch.Name != "DOAS Main Branch" ||
							gotIn != fieldValueByCatalogName(coil, "Air Inlet Node Name") || gotOut != fieldValueByCatalogName(coil, "Air Outlet Node Name") {
							t.Fatalf("water coil has a foreign/mismatched air occurrence: %#v", component)
						}
						continue
					}
					water++
					if actualLoop.Type != "PlantLoop" || actualLoop.Name != loop || !strings.EqualFold(side.Name, "demand") ||
						branch.Name != name+" "+suffix+" Branch" || gotIn != inlet || gotOut != outlet {
						t.Fatalf("%s has crossed/unbound water Branch: loop=%s side=%s branch=%s ports=%s/%s", name, actualLoop.Name, side.Name, branch.Name, gotIn, gotOut)
					}
				}
			}
		}
	}
	wantAir := 0
	if owner == "DOAS" {
		wantAir = 1 // The same coil legitimately occurs on one air and one water Branch.
	}
	if water != 1 || air != wantAir {
		t.Fatalf("%s water/air occurrences=%d/%d, want 1/%d", name, water, air, wantAir)
	}
}

func TestHVACDistrictOriginalTypedLocalAndSharedServices(t *testing.T) {
	doc := hvacDistrictOriginal(t)
	report := AnalyzeHVAC(doc)
	counts := map[string]int{}
	for _, object := range doc.Objects {
		counts[strings.ToLower(object.Type)]++
	}
	for kind, want := range map[string]int{"zone": 6, "zonehvac:equipmentconnections": 5, "zonehvac:fourpipefancoil": 5,
		"airloophvac": 1, "plantloop": 2, "pump:constantspeed": 2, "coil:cooling:water": 6, "coil:heating:water": 6,
		"districtcooling": 1, "districtheating:water": 1, "fan:onoff": 5, "fan:variablevolume": 1, "condenserloop": 0} {
		if counts[kind] != want {
			t.Errorf("original inventory %s=%d, want %d", kind, counts[kind], want)
		}
	}
	for _, service := range []string{"cooling", "heating"} {
		hvacDistrictAssertWaterBranch(t, doc, report, "DOAS", service)
	}
	recovery := 0
	for _, item := range report.ServiceModel.Components {
		if item.Component.ObjectType != "HeatExchanger:AirToAir:SensibleAndLatent" || item.Component.ObjectName != "DOAS Heat Recovery" {
			continue
		}
		for _, occurrence := range item.Occurrences {
			if occurrence.LoopType == "AirLoopHVAC" && occurrence.LoopName == "DOAS" && occurrence.BranchName == "DOAS Main Branch" {
				recovery++
			}
		}
	}
	if recovery != 1 {
		t.Errorf("main-Branch heat recovery context must remain one real component occurrence, got %d", recovery)
	}
	seenIDs := map[string]bool{}
	for _, summary := range report.ServiceModel.ZoneServices {
		for _, path := range summary.Paths {
			if path.ID == "" || seenIDs[path.ID] || path.ZoneName == "PLENUM-1" || path.CondenserLoop != nil || path.RefrigerantSystem != nil {
				t.Errorf("duplicate/foreign/invented original path: %#v", path)
			}
			seenIDs[path.ID] = true
			if path.PlantLoop != nil {
				want := map[string]string{"cooling": hvacDistrictCoolingLoop, "heating": hvacDistrictHeatingLoop}[path.ServiceKind]
				if want == "" || path.PlantLoop.Name != want {
					t.Errorf("crossed service or ventilation on a district plant: %#v", path)
				}
			}
		}
	}
	for i := 1; i <= 5; i++ {
		zone := fmt.Sprintf("SPACE%d-1", i)
		summary := findHVACTestingZoneService(report.ServiceModel, zone)
		if summary == nil {
			t.Fatalf("missing original conditioned Zone %s", zone)
		}
		local, shared := map[string]int{}, map[string]bool{}
		for _, service := range []string{"cooling", "heating"} {
			hvacDistrictAssertWaterBranch(t, doc, report, zone, service)
		}
		for _, path := range summary.Paths {
			if path.Delivery.ObjectType == "ZoneHVAC:FourPipeFanCoil" && (path.ServiceKind == "cooling" || path.ServiceKind == "heating") {
				local[path.ServiceKind]++
				prefix, loop := "Cooling", hvacDistrictCoolingLoop
				if path.ServiceKind == "heating" {
					prefix, loop = "Heating", hvacDistrictHeatingLoop
				}
				coil := hvacDistrictObject(t, doc, "Coil:"+prefix+":Water", zone+" "+prefix+" Coil")
				if path.Delivery.ObjectName != zone+" Fan Coil" || path.PathType != "direct_zone_hydronic" || path.AirLoop != nil ||
					path.PlantLoop == nil || path.PlantLoop.Name != loop || len(path.Conditioning) != 1 ||
					path.Conditioning[0].ObjectType != coil.Type || path.Conditioning[0].ObjectName != objectName(coil) || path.Conditioning[0].ObjectIndex != coil.Index {
					t.Errorf("local typed conditioning borrowed another delivery/coil/loop: %#v", path)
				}
				if source := path.SourceSystem; source != nil {
					want := "DistrictCooling Purchased Cooling"
					if path.ServiceKind == "heating" {
						want = "DistrictHeating:Water Purchased Heating"
					}
					if source.Type != "source" || source.Name != want {
						t.Errorf("local path borrowed another plant's source: %#v", source)
					}
				}
			}
			if path.AirLoop == nil || path.AirLoop.Name != "DOAS" {
				continue
			}
			if path.Delivery.ObjectType != "AirTerminal:SingleDuct:VAV:NoReheat" || path.Delivery.ObjectName != zone+" DOAS Air Terminal" ||
				!path.Delivery.ResolvedFromADU || path.Delivery.DistributionUnitName != zone+" DOAS ATU" {
				t.Errorf("DOAS delivery crossed its original terminal/ADU owner: %#v", path)
			}
			// Legacy plant-backed chains carry the ADU identity in Delivery;
			// direct central-air paths additionally expose DeliveryWrapper.
			if wrapper := path.DeliveryWrapper; wrapper != nil && (wrapper.ObjectType != "ZoneHVAC:AirDistributionUnit" || wrapper.ObjectName != zone+" DOAS ATU") {
				t.Errorf("explicit wrapper contradicts the exact original ADU: %#v", wrapper)
			}
			if path.ServiceKind == "cooling" || path.ServiceKind == "heating" {
				if path.PathType != "central_air_with_plant" || path.PlantLoop == nil {
					t.Errorf("native water conditioning lost its plant-backed boundary: %#v", path)
				}
			}
			for _, conditioning := range path.Conditioning {
				if conditioning.ObjectType == "Coil:Cooling:Water" && conditioning.ObjectName == "DOAS Cooling Coil" && path.ServiceKind == "cooling" {
					shared["cooling"] = true
				}
				if conditioning.ObjectType == "Coil:Heating:Water" && conditioning.ObjectName == "DOAS Heating Coil" && path.ServiceKind == "heating" {
					shared["heating"] = true
				}
			}
		}
		if local["cooling"] != 1 || local["heating"] != 1 || !shared["cooling"] || !shared["heating"] {
			t.Errorf("%s lost distinct local/DOAS thermal paths: local=%v shared=%v", zone, local, shared)
		}
	}
}

func TestHVACDistrictInvalidLocalCoilCannotBorrowDOASOrOtherPlant(t *testing.T) {
	for _, mutation := range []string{"water ports", "missing coil", "shared coil", "duplicate coil"} {
		t.Run(mutation, func(t *testing.T) {
			doc := hvacDistrictOriginal(t)
			switch mutation {
			case "water ports":
				hvacHydronicReplaceField(t, &doc, "Branch", "SPACE1-1 Cooling Coil ChW Branch", "SPACE1-1 Cooling Coil ChW Inlet", "Unconnected Water Inlet")
			case "missing coil":
				hvacHydronicReplaceField(t, &doc, "ZoneHVAC:FourPipeFanCoil", "SPACE1-1 Fan Coil", "SPACE1-1 Cooling Coil", "Missing Coil")
			case "shared coil":
				hvacHydronicReplaceField(t, &doc, "ZoneHVAC:FourPipeFanCoil", "SPACE2-1 Fan Coil", "SPACE2-1 Cooling Coil", "SPACE1-1 Cooling Coil")
			case "duplicate coil":
				copy := hvacDistrictObject(t, doc, "Coil:Cooling:Water", "SPACE1-1 Cooling Coil")
				copy.Index, copy.Fields = len(doc.Objects), append([]Field(nil), copy.Fields...)
				doc.Objects = append(doc.Objects, copy)
			}
			report := AnalyzeHVAC(doc)
			summary := findHVACTestingZoneService(report.ServiceModel, "SPACE1-1")
			if summary == nil {
				t.Fatal("invalid cooling cannot delete its original Zone")
			}
			heating := 0
			for _, path := range summary.Paths {
				if path.Delivery.ObjectName != "SPACE1-1 Fan Coil" {
					continue
				}
				if path.ServiceKind == "cooling" && path.PlantLoop != nil {
					t.Errorf("invalid local cooling borrowed a valid DOAS/plant route: %#v", path)
				}
				if path.ServiceKind == "heating" && path.PlantLoop != nil && path.PlantLoop.Name == hvacDistrictHeatingLoop {
					heating++
				}
			}
			if heating != 1 {
				t.Errorf("unrelated valid local heating was lost: %d", heating)
			}
		})
	}
}

func TestHVACDistrictDisconnectedDOASTerminalCannotBorrowLocalRoute(t *testing.T) {
	for _, mutation := range []string{"terminal outlet", "splitter outlet"} {
		t.Run(mutation, func(t *testing.T) {
			doc := hvacDistrictOriginal(t)
			if mutation == "terminal outlet" {
				hvacHydronicReplaceField(t, &doc, "AirTerminal:SingleDuct:VAV:NoReheat", "SPACE1-1 DOAS Air Terminal", "SPACE1-1 DOAS Supply Inlet", "Disconnected DOAS Outlet")
			} else {
				hvacHydronicReplaceField(t, &doc, "AirLoopHVAC:ZoneSplitter", "DOAS Zone Splitter", "SPACE1-1 Zone Equip Inlet", "Disconnected Splitter Outlet")
			}
			report := AnalyzeHVAC(doc)
			if mutation == "terminal outlet" {
				relation := findHVACTestingZoneRelation(report, "SPACE1-1")
				if relation == nil {
					t.Fatal("disconnected native terminal must remain available as original context")
				}
				terminal := findHVACTestingComponent(relation.TerminalUnits, "SPACE1-1 DOAS Air Terminal")
				if terminal == nil || terminal.TerminalObjectOutletNode != "Disconnected DOAS Outlet" ||
					terminal.DistributionUnitOutletNode != "SPACE1-1 DOAS Supply Inlet" || terminal.OutletNode != "SPACE1-1 DOAS Supply Inlet" {
					t.Fatalf("original terminal/ADU contradiction or wrapper display metadata was erased: %#v", terminal)
				}
				for _, edge := range report.RuleGraph.Edges {
					if edge.FromID == hvacRuleComponentSourceNodeID(*terminal) && edge.RuleID == hvacRuleZoneTerminalOutletMatchesInlet {
						t.Errorf("ADU display outlet fabricated native terminal connectivity: %#v", edge)
					}
				}
			}
			summary := findHVACTestingZoneService(report.ServiceModel, "SPACE1-1")
			if summary == nil {
				t.Fatal("local delivery must preserve the Zone")
			}
			local := map[string]bool{}
			for _, path := range summary.Paths {
				if path.ServiceKind != "cooling" && path.ServiceKind != "heating" {
					continue
				}
				if path.Delivery.ObjectName == "SPACE1-1 DOAS Air Terminal" && path.AirLoop != nil {
					t.Errorf("disconnected terminal acquired shared conditioning through local ownership: %#v", path)
				}
				if path.Delivery.ObjectName == "SPACE1-1 Fan Coil" && path.PlantLoop != nil {
					local[path.ServiceKind] = true
				}
			}
			if !local["cooling"] || !local["heating"] {
				t.Errorf("disconnected DOAS erased independent valid local water coils: %v", local)
			}
		})
	}
}
