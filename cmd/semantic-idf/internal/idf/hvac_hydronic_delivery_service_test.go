package idf

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestHVACHydronicDeliveryOriginalFanCoilPlantServices(t *testing.T) {
	doc := hvacHydronicOriginalFanCoil(t)
	report := AnalyzeHVAC(doc)
	if len(report.ServiceModel.ZoneServices) != 3 {
		t.Fatalf("original model must retain exactly three Zone services, got %d", len(report.ServiceModel.ZoneServices))
	}
	for i, zone := range []string{"West Zone", "EAST ZONE", "NORTH ZONE"} {
		summary := findHVACTestingZoneService(report.ServiceModel, zone)
		if summary == nil {
			t.Fatalf("missing original Zone %s", zone)
		}
		want := map[string]string{"cooling": "Chilled Water Loop", "heating": "Hot Water Loop"}
		found, ventilation := map[string]bool{}, false
		plantPaths := 0
		for _, path := range summary.Paths {
			if path.Delivery.ObjectName != fmt.Sprintf("Zone%dFanCoil", i+1) || path.AirLoop != nil {
				t.Errorf("%s escaped its local original FanCoil ownership: %s", zone, path.ID)
			}
			if path.PlantLoop == nil {
				if path.ServiceKind != "ventilation" {
					t.Errorf("water-only %s invented plant-free %s service", zone, path.ServiceKind)
				}
				ventilation = ventilation || path.ServiceKind == "ventilation"
				if path.SourceSystem != nil || len(path.Conditioning) != 0 || path.PathType != "direct_zone_air" {
					t.Errorf("local ventilation borrowed a plant/coil source: %#v", path)
				}
				continue
			}
			plantPaths++
			if path.PlantLoop.Name != want[path.ServiceKind] || path.PathType != "direct_zone_hydronic" {
				t.Errorf("%s has Cartesian/untyped plant service %s -> %s", zone, path.PlantLoop.Name, path.ServiceKind)
				continue
			}
			coilType, coilName := "Coil:Cooling:Water", fmt.Sprintf("Zone%dFanCoilCoolingCoil", i+1)
			if path.ServiceKind == "heating" {
				coilType, coilName = "Coil:Heating:Water", fmt.Sprintf("Zone%dFanCoilHeatingCoil", i+1)
			}
			if len(path.Conditioning) != 1 {
				t.Errorf("%s %s retained unrelated/duplicate conditioning: %#v", zone, path.ServiceKind, path.Conditioning)
			}
			for _, conditioning := range path.Conditioning {
				if conditioning.ObjectType == coilType && conditioning.ObjectName == coilName {
					found[path.ServiceKind] = true
				} else {
					t.Errorf("%s %s borrowed conditioning from another service/owner: %#v", zone, path.ServiceKind, conditioning)
				}
			}
			if source := path.SourceSystem; source != nil {
				wantSource := "DistrictCooling Purchased Cooling"
				if path.ServiceKind == "heating" {
					wantSource = "DistrictHeating:Water Purchased Heating"
				}
				if source.Type != "source" || source.Name != wantSource {
					t.Errorf("%s %s borrowed another plant source: %#v", zone, path.ServiceKind, source)
				}
			}
		}
		if !found["cooling"] || !found["heating"] || !ventilation || plantPaths != 2 {
			t.Errorf("%s lost typed conditioning or local ventilation: cooling=%v heating=%v ventilation=%v", zone, found["cooling"], found["heating"], ventilation)
		}
	}
}

func TestHVACHydronicDeliveryLegacyFirstPathCannotBorrowMetadata(t *testing.T) {
	doc := hvacHydronicOriginalFanCoil(t)
	report := AnalyzeHVAC(doc)
	ctx := newHVACContext(doc)
	for _, loop := range report.Loops {
		registerLoopComponents(ctx, loop)
	}
	for _, mutation := range []string{"foreign conditioning", "foreign source", "correct source"} {
		t.Run(mutation, func(t *testing.T) {
			relations := append([]HVACZoneChain(nil), report.ZoneRelations...)
			for i := range relations {
				if !strings.EqualFold(relations[i].ZoneName, "West Zone") {
					continue
				}
				chain := HVACServicePath{
					ZoneName: "West Zone", TerminalName: "Zone1FanCoil", Component: "Zone1FanCoilCoolingCoil",
					PlantLoop: "Chilled Water Loop", SourceComponent: "DistrictCooling Purchased Cooling",
				}
				if mutation == "foreign conditioning" {
					chain.Component = "Zone2FanCoilCoolingCoil"
				}
				if mutation == "foreign source" {
					chain.SourceComponent = "DistrictHeating:Water Purchased Heating"
				}
				relations[i].ServiceChains = append([]HVACServicePath{chain}, relations[i].ServiceChains...)
			}
			paths := buildZoneServicePaths(ctx, report.Loops, relations, report.RuleGraph, ComponentIndex{}, CouplingIndex{})
			count := 0
			for _, path := range paths {
				if !strings.EqualFold(path.ZoneName, "West Zone") || path.ServiceKind != "cooling" || path.PlantLoop == nil {
					continue
				}
				count++
				if path.PlantLoop.Name != "Chilled Water Loop" || path.PathType != "direct_zone_hydronic" || path.AirLoop != nil ||
					len(path.Conditioning) != 1 || path.Conditioning[0].ObjectType != "Coil:Cooling:Water" || path.Conditioning[0].ObjectName != "Zone1FanCoilCoolingCoil" {
					t.Errorf("first legacy chain polluted exact typed path: %#v", path)
				}
				if path.SourceSystem != nil && (path.SourceSystem.Type != "source" || path.SourceSystem.Name != "DistrictCooling Purchased Cooling") {
					t.Errorf("foreign plant source survived first-wins: %#v", path.SourceSystem)
				}
				if mutation == "correct source" && path.SourceSystem == nil {
					t.Error("valid legacy plant source trace was discarded")
				}
			}
			if count != 1 {
				t.Errorf("got %d authoritative West cooling paths, want exactly one", count)
			}
		})
	}
}

func TestHVACHydronicDeliveryOriginalFanCoilRejectsAmbiguousCoil(t *testing.T) {
	for _, mutation := range []string{"mismatched water nodes", "coil on two plant loops", "duplicate coil identity", "missing typed coil", "shared coil owner", "detached branch reference"} {
		t.Run(mutation, func(t *testing.T) {
			doc := hvacHydronicOriginalFanCoil(t)
			switch mutation {
			case "mismatched water nodes":
				hvacHydronicReplaceField(t, &doc, "Branch", "Zone1ChWBranch", "Zone1FanCoilChWInletNode", "Unconnected Water Node")
			case "coil on two plant loops":
				hvacHydronicReplaceField(t, &doc, "Branch", "Zone1HWBranch", "Coil:Heating:Water", "Coil:Cooling:Water")
				hvacHydronicReplaceField(t, &doc, "Branch", "Zone1HWBranch", "Zone1FanCoilHeatingCoil", "Zone1FanCoilCoolingCoil")
				hvacHydronicReplaceField(t, &doc, "Branch", "Zone1HWBranch", "Zone1FanCoilHWInletNode", "Zone1FanCoilChWInletNode")
				hvacHydronicReplaceField(t, &doc, "Branch", "Zone1HWBranch", "Zone1FanCoilHWOutletNode", "Zone1FanCoilChWOutletNode")
			case "duplicate coil identity":
				for _, obj := range doc.Objects {
					if obj.Type == "Coil:Cooling:Water" && objectName(obj) == "Zone1FanCoilCoolingCoil" {
						obj.Index = len(doc.Objects)
						obj.Fields = append([]Field(nil), obj.Fields...)
						doc.Objects = append(doc.Objects, obj)
						break
					}
				}
			case "missing typed coil":
				hvacHydronicReplaceField(t, &doc, "ZoneHVAC:FourPipeFanCoil", "Zone1FanCoil", "Zone1FanCoilCoolingCoil", "Missing Cooling Coil")
			case "shared coil owner":
				hvacHydronicReplaceField(t, &doc, "ZoneHVAC:FourPipeFanCoil", "Zone2FanCoil", "Zone2FanCoilCoolingCoil", "Zone1FanCoilCoolingCoil")
			case "detached branch reference":
				for _, obj := range doc.Objects {
					if obj.Type == "Branch" && objectName(obj) == "Zone1ChWBranch" {
						obj.Index = len(doc.Objects)
						obj.Fields = append([]Field(nil), obj.Fields...)
						obj.Fields[0].Value = "Detached Duplicate Cooling Branch"
						doc.Objects = append(doc.Objects, obj)
						break
					}
				}
			}
			report := AnalyzeHVAC(doc)
			summary := findHVACTestingZoneService(report.ServiceModel, "West Zone")
			if summary == nil {
				t.Fatal("malformed conditioning must not remove its Zone")
			}
			for _, path := range summary.Paths {
				if path.ServiceKind == "cooling" && path.PlantLoop != nil {
					t.Errorf("ambiguous/unconnected cooling acquired authoritative plant %s", path.PlantLoop.Name)
				}
				if path.PlantLoop != nil && path.ServiceKind == "ventilation" {
					t.Errorf("malformed coil relabelled district plant as ventilation: %s", path.PlantLoop.Name)
				}
			}
		})
	}
}

func TestHVACHydronicDeliveryActualDualServicePlantIsNotSplit(t *testing.T) {
	doc := hvacHydronicOriginalFanCoil(t)
	// A plant with distinct cooling and heating water coils really has both
	// services. Preserve that topology so the auxiliary consumer can reject
	// a mixed-service plant; do not manufacture two single-service plants.
	for _, kind := range []string{"BranchList", "Connector:Splitter", "Connector:Mixer"} {
		for oi := range doc.Objects {
			obj := &doc.Objects[oi]
			if obj.Type != kind {
				continue
			}
			name := objectName(*obj)
			if name == "Cooling Demand Side Branches" || name == "Zones ChW Splitter" || name == "Zones ChW Mixer" {
				for i := 1; i <= 3; i++ {
					obj.Fields = append(obj.Fields, Field{Value: fmt.Sprintf("Zone%dHWBranch", i)})
				}
			}
			if name == "Heating Demand Side Branches" || name == "Zones HW Splitter" || name == "Zones HW Mixer" {
				filtered := make([]Field, 0, len(obj.Fields))
				for _, field := range obj.Fields {
					if field.Value != "Zone1HWBranch" && field.Value != "Zone2HWBranch" && field.Value != "Zone3HWBranch" {
						filtered = append(filtered, field)
					}
				}
				obj.Fields = filtered
			}
		}
	}
	report := AnalyzeHVAC(doc)
	for _, zone := range []string{"West Zone", "EAST ZONE", "NORTH ZONE"} {
		summary := findHVACTestingZoneService(report.ServiceModel, zone)
		if summary == nil {
			t.Fatal("missing dual-service Zone")
		}
		found := map[string]bool{}
		for _, path := range summary.Paths {
			if path.PlantLoop == nil {
				continue
			}
			if path.PlantLoop.Name != "Chilled Water Loop" || (path.ServiceKind != "cooling" && path.ServiceKind != "heating") {
				t.Errorf("shared plant was misrouted: %s %s", path.PlantLoop.Name, path.ServiceKind)
			}
			found[path.ServiceKind] = true
		}
		if !found["cooling"] || !found["heating"] {
			t.Errorf("%s lost actual mixed-service evidence: %v", zone, found)
		}
	}
}

func TestHVACHydronicDeliveryLocalElectricHeatingDoesNotUseCoolingPlant(t *testing.T) {
	doc := hvacHydronicOriginalFanCoil(t)
	hvacHydronicReplaceField(t, &doc, "ZoneHVAC:FourPipeFanCoil", "Zone1FanCoil", "Coil:Heating:Water", "Coil:Heating:Electric")
	for oi := range doc.Objects {
		obj := &doc.Objects[oi]
		if obj.Type == "Coil:Heating:Water" && objectName(*obj) == "Zone1FanCoilHeatingCoil" {
			obj.Type = "Coil:Heating:Electric"
			obj.Fields = []Field{
				{Value: "Zone1FanCoilHeatingCoil", Comment: "Name"},
				{Value: "FanAndCoilAvailSched", Comment: "Availability Schedule Name"},
				{Value: "1", Comment: "Efficiency"},
				{Value: "Autosize", Comment: "Nominal Capacity"},
				{Value: "Zone1FanCoilCCOutletNode", Comment: "Air Inlet Node Name"},
				{Value: "Zone1FanCoilAirOutletNode", Comment: "Air Outlet Node Name"},
			}
		}
	}
	report := AnalyzeHVAC(doc)
	zone := findHVACTestingZoneService(report.ServiceModel, "West Zone")
	if zone == nil {
		t.Fatal("missing native FanCoil Zone")
	}
	found := 0
	for _, path := range zone.Paths {
		if path.ServiceKind != "heating" {
			continue
		}
		found++
		if path.PlantLoop != nil || path.AirLoop != nil || path.PathType != "direct_zone_air" || path.SourceSystem != nil ||
			len(path.Conditioning) != 1 || path.Conditioning[0].ObjectType != "Coil:Heating:Electric" || path.Conditioning[0].ObjectName != "Zone1FanCoilHeatingCoil" {
			t.Errorf("local electric heating borrowed district/plant/source identity: %#v", path)
		}
	}
	if found != 1 {
		t.Errorf("got %d local electric heating paths, want one", found)
	}
}

func hvacHydronicOriginalFanCoil(t *testing.T) Document {
	t.Helper()
	const path = "../simulation/testdata/energy_path_real_models/models/25.1/FanCoilAutoSize.idf"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "d940e67912d5d4669773c11521b499ca671dae2d67ff8ce538c4a183ca0b43d3" {
		t.Fatalf("original official fixture changed: %s", got)
	}
	t.Cleanup(func() {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, after) {
			t.Error("read-only original fixture was modified")
		}
	})
	doc, err := Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func hvacHydronicReplaceField(t *testing.T, doc *Document, objectType, name, from, to string) {
	t.Helper()
	changed := 0
	for oi := range doc.Objects {
		obj := &doc.Objects[oi]
		if !strings.EqualFold(obj.Type, objectType) || !strings.EqualFold(objectName(*obj), name) {
			continue
		}
		for fi := range obj.Fields {
			if strings.EqualFold(obj.Fields[fi].Value, from) {
				obj.Fields[fi].Value = to
				changed++
			}
		}
	}
	if changed != 1 {
		t.Fatalf("fixture mutation %s %s %s matched %d fields", objectType, name, from, changed)
	}
}
