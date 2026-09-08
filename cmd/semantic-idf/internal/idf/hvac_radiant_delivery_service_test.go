package idf

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func hvacRadiantOriginal(t *testing.T) Document {
	t.Helper()
	path := "../simulation/testdata/energy_path_real_models/models/25.1/RadLoTempCFloHeatCool.idf"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != "c4988a4211b37a9508dd0265ee19cb1f4f7174c57e214a38ec3935e2f918388e" {
		t.Fatal("original radiant fixture changed")
	}
	t.Cleanup(func() {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, after) {
			t.Error("original radiant file was modified")
		}
	})
	doc, err := Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func hvacRadiantEditObject(t *testing.T, doc *Document, kind, name string, mutate func(*Object)) {
	t.Helper()
	for i := range doc.Objects {
		if strings.EqualFold(doc.Objects[i].Type, kind) && strings.EqualFold(objectName(doc.Objects[i]), name) {
			mutate(&doc.Objects[i])
			return
		}
	}
	t.Fatalf("missing original object %s/%s", kind, name)
}

func hvacRadiantMoveBranch(doc *Document, branch, from, to string) {
	for i := range doc.Objects {
		object := &doc.Objects[i]
		if object.Type != "BranchList" {
			continue
		}
		if objectName(*object) == from {
			fields := []Field{}
			for _, field := range object.Fields {
				if field.Value != branch {
					fields = append(fields, field)
				}
			}
			object.Fields = fields
		}
		if objectName(*object) == to {
			object.Fields = append(object.Fields, Field{Value: branch})
		}
	}
}

func TestHVACRadiantDeliveryRejectsAmbiguousOriginalConnectivity(t *testing.T) {
	for _, mutation := range []string{"crossed ports", "unknown ports", "missing design", "foreign Zone", "foreign surface", "unsupported schema", "duplicate parent", "duplicate Branch", "detached Branch", "duplicate EquipmentList entry", "foreign typed wrapper", "two plants", "supply side", "disconnected demand"} {
		t.Run(mutation, func(t *testing.T) {
			doc := hvacRadiantOriginal(t)
			const parent = "West Zone Radiant Floor"
			switch mutation {
			case "crossed ports":
				hvacHydronicReplaceField(t, &doc, "Branch", "Zone 1 Cooling Branch", "Zone 1 Cooling Water Outlet Node", "West Zone Radiant Water Outlet Node")
			case "unknown ports":
				hvacHydronicReplaceField(t, &doc, "Branch", "Zone 1 Cooling Branch", "Zone 1 Cooling Water Inlet Node", "Detached Node")
			case "missing design":
				hvacHydronicReplaceField(t, &doc, "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", parent, parent+" Design", "Missing Design")
			case "foreign Zone":
				hvacHydronicReplaceField(t, &doc, "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", parent, "West Zone", "East Zone")
			case "foreign surface":
				hvacHydronicReplaceField(t, &doc, "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", parent, "Zn001:Flr001", "Zn002:Flr001")
			case "unsupported schema":
				hvacRadiantEditObject(t, &doc, "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", parent, func(object *Object) { object.Fields = object.Fields[:20] })
			case "duplicate parent", "duplicate Branch", "detached Branch":
				kind, name := "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", parent
				if mutation != "duplicate parent" {
					kind, name = "Branch", "Zone 1 Cooling Branch"
				}
				var duplicate Object
				hvacRadiantEditObject(t, &doc, kind, name, func(object *Object) { duplicate = *object; duplicate.Fields = append([]Field(nil), object.Fields...) })
				duplicate.Index = len(doc.Objects)
				if mutation == "detached Branch" {
					duplicate.Fields[0].Value = "Unconnected Cooling Branch"
				}
				doc.Objects = append(doc.Objects, duplicate)
			case "duplicate EquipmentList entry":
				for i := range doc.Objects {
					object := &doc.Objects[i]
					if object.Type == "ZoneHVAC:EquipmentList" && len(object.Fields) > 3 && strings.EqualFold(object.Fields[3].Value, parent) {
						object.Fields = append(object.Fields, append([]Field(nil), object.Fields[2:8]...)...)
						break
					}
				}
			case "foreign typed wrapper":
				doc.Objects = append(doc.Objects, Object{Index: len(doc.Objects), Type: "Unsupported:Wrapper", Fields: []Field{{Value: "Foreign"}, {Value: "ZoneHVAC:LowTemperatureRadiant:ConstantFlow"}, {Value: parent}}})
			case "two plants":
				hvacRadiantEditObject(t, &doc, "BranchList", "Heating Demand Side Branches", func(object *Object) { object.Fields = append(object.Fields, Field{Value: "Zone 1 Cooling Branch"}) })
			case "supply side":
				hvacRadiantMoveBranch(&doc, "Zone 1 Cooling Branch", "Cooling Demand Side Branches", "Cooling Supply Side Branches")
			case "disconnected demand":
				hvacRadiantMoveBranch(&doc, "Zone 1 Cooling Branch", "Cooling Demand Side Branches", "No Existing Branch List")
			}
			report := AnalyzeHVAC(doc)
			for _, summary := range report.ServiceModel.ZoneServices {
				for _, path := range summary.Paths {
					if strings.EqualFold(path.Delivery.ObjectName, parent) {
						t.Errorf("malformed original owner/ports acquired service %s", path.ID)
					}
				}
			}
		})
	}
}

func TestHVACRadiantDeliveryGenuineDualServiceLoopRemainsMixed(t *testing.T) {
	doc := hvacRadiantOriginal(t)
	for zone := 1; zone <= 3; zone++ {
		branch := fmt.Sprintf("Zone %d Radiant Branch", zone)
		hvacRadiantMoveBranch(&doc, branch, "Heating Demand Side Branches", "Cooling Demand Side Branches")
		for i := range doc.Objects {
			object := &doc.Objects[i]
			if object.Type != "Connector:Splitter" && object.Type != "Connector:Mixer" {
				continue
			}
			switch objectName(*object) {
			case "CW Demand Splitter", "CW Demand Mixer":
				object.Fields = append(object.Fields, Field{Value: branch})
			default:
				fields := []Field{}
				for _, field := range object.Fields {
					if field.Value != branch {
						fields = append(fields, field)
					}
				}
				object.Fields = fields
			}
		}
	}
	report := AnalyzeHVAC(doc)
	for _, zone := range []string{"West Zone", "EAST ZONE", "NORTH ZONE"} {
		summary := findHVACTestingZoneService(report.ServiceModel, zone)
		if summary == nil || len(summary.Paths) != 2 {
			t.Fatalf("dual-service owner lost its two original water circuits: %#v", summary)
		}
		services := map[string]bool{}
		for _, path := range summary.Paths {
			if path.PlantLoop == nil || path.PlantLoop.Name != "Chilled Water Loop" || (path.ServiceKind != "cooling" && path.ServiceKind != "heating") {
				t.Fatalf("genuine mixed plant was falsely split/relabelled: %s", path.ID)
			}
			services[path.ServiceKind] = true
		}
		if !services["cooling"] || !services["heating"] {
			t.Fatal("mixed-service evidence lost")
		}
	}
}

func TestHVACRadiantDeliveryLegacyFirstCannotBorrowSourceOrConditioning(t *testing.T) {
	doc := hvacRadiantOriginal(t)
	report := AnalyzeHVAC(doc)
	ctx := newHVACContext(doc)
	for _, loop := range report.Loops {
		registerLoopComponents(ctx, loop)
	}
	entries := buildHVACRadiantDeliveryServices(ctx, report.Loops, report.ZoneRelations)
	entry := entries[hvacObjectKey("ZoneHVAC:LowTemperatureRadiant:ConstantFlow", "West Zone Radiant Floor")]
	var original ZoneServicePath
	for _, path := range findHVACTestingZoneService(report.ServiceModel, "West Zone").Paths {
		if path.ServiceKind == "cooling" {
			original = path
		}
	}
	if _, valid := entry.bindPath(original); !valid {
		t.Fatal("valid original cooling path rejected")
	}
	for _, mutate := range []func(*ZoneServicePath){
		func(p *ZoneServicePath) { p.ServiceKind = "heating" },
		func(p *ZoneServicePath) { p.ServiceKind = "radiant_heating" },
		func(p *ZoneServicePath) {
			p.SourceSystem = &SystemRef{Type: "source", Name: "DistrictHeating:Water Purchased Heating"}
		},
		func(p *ZoneServicePath) {
			p.Conditioning = []ComponentRef{{ObjectType: "Chiller:Electric", ObjectName: "Big Chiller"}}
		},
		func(p *ZoneServicePath) { p.ZoneName = "Other" },
		func(p *ZoneServicePath) { p.Delivery.ObjectIndex++ },
		func(p *ZoneServicePath) { p.PlantLoop = nil },
		func(p *ZoneServicePath) { p.AirLoop = &LoopRef{Type: "AirLoopHVAC", Name: "Foreign"} },
	} {
		path := original
		mutate(&path)
		if _, valid := entry.bindPath(path); valid {
			t.Fatal("foreign legacy metadata passed pre-deduplication binding")
		}
	}
}

func TestHVACRadiantDeliveryNonNativePathsRemainUnchanged(t *testing.T) {
	doc := hvacHydronicOriginalFanCoil(t)
	before := AnalyzeHVAC(doc)
	if buildHVACRadiantDeliveryServices(newHVACContext(doc), nil, nil) != nil {
		t.Fatal("non-radiant model lost the fast return")
	}
	doc.Objects = append(doc.Objects, Object{Index: len(doc.Objects), Type: "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", Fields: []Field{{Value: "Unowned Unsupported Shape"}}})
	after := AnalyzeHVAC(doc)
	for _, zone := range []string{"West Zone", "EAST ZONE", "NORTH ZONE"} {
		left, right := findHVACTestingZoneService(before.ServiceModel, zone), findHVACTestingZoneService(after.ServiceModel, zone)
		if left == nil || right == nil || !reflect.DeepEqual(left.Paths, right.Paths) {
			t.Fatalf("unowned radiant input changed ordinary FanCoil paths for %s", zone)
		}
	}
}

func TestHVACRadiantDeliveryOriginalTypedPlantServices(t *testing.T) {
	report := AnalyzeHVAC(hvacRadiantOriginal(t))
	if len(report.ServiceModel.ZoneServices) != 3 {
		t.Fatal("original radiant Zone roster changed")
	}
	for _, zone := range []string{"West Zone", "EAST ZONE", "NORTH ZONE"} {
		summary := findHVACTestingZoneService(report.ServiceModel, zone)
		if summary == nil {
			t.Fatalf("missing Zone %s", zone)
		}
		seen := map[string]bool{}
		for _, path := range summary.Paths {
			want := map[string]string{"cooling": "Chilled Water Loop", "heating": "Hot Water Loop"}[path.ServiceKind]
			if want == "" || path.PlantLoop == nil || path.PlantLoop.Type != "PlantLoop" || path.PlantLoop.Name != want || path.PathType != "radiant" || path.AirLoop != nil || path.CondenserLoop != nil || path.RefrigerantSystem != nil ||
				path.Delivery.ObjectType != "ZoneHVAC:LowTemperatureRadiant:ConstantFlow" || !strings.EqualFold(path.Delivery.ObjectName, zone+" Radiant Floor") || len(path.Conditioning) != 0 || seen[path.ServiceKind] {
				t.Errorf("%s has crossed/duplicate/untyped original radiant service: %#v", zone, path)
				continue
			}
			seen[path.ServiceKind] = true
			if source := path.SourceSystem; source != nil {
				allowed := path.ServiceKind == "heating" && source.Name == "DistrictHeating:Water Purchased Heating" || path.ServiceKind == "cooling" && (source.Name == "Chiller:Electric Big Chiller" || source.Name == "Chiller:ConstantCOP Little Chiller" || source.Name == "DistrictCooling Purchased Cooling")
				if source.Type != "source" || !allowed {
					t.Errorf("crossed legacy source survived: %#v", source)
				}
			}
		}
		if len(summary.Paths) != 2 || !seen["cooling"] || !seen["heating"] {
			t.Errorf("%s must have exactly its original H/C port-bound services; got %d", zone, len(summary.Paths))
		}
	}
}
