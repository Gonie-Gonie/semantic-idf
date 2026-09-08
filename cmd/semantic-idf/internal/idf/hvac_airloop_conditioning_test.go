package idf

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestHVACAirLoopConditioningOriginalSmallOffice(t *testing.T) {
	doc := hvacSmallOfficeConditioningDocument(t)
	report := AnalyzeHVAC(doc)
	zones := []string{"Core_ZN", "Perimeter_ZN_1", "Perimeter_ZN_2", "Perimeter_ZN_3", "Perimeter_ZN_4"}
	for index, zone := range zones {
		loop := fmt.Sprintf("PSZ-AC:%d", index+1)
		for _, service := range []string{"cooling", "heating", "ventilation"} {
			paths := hvacSmallOfficeConditioningPaths(report, zone, service)
			if len(paths) != 1 {
				t.Fatalf("%s/%s: got %d paths, want exactly one", zone, service, len(paths))
			}
			path := paths[0]
			if path.AirLoop == nil || path.AirLoop.Name != loop || path.PlantLoop != nil || path.PathType != "central_air" ||
				path.Delivery.ObjectType != "AirTerminal:SingleDuct:ConstantVolume:NoReheat" {
				t.Fatalf("wrong actual delivery context: %+v", path)
			}
			want := []string(nil)
			switch service {
			case "cooling":
				want = []string{"CoilSystem:Cooling:DX|" + loop + "_CoolC", "Coil:Cooling:DX:SingleSpeed|" + loop + "_CoolC DXCoil"}
				if !hvacSmallOfficeHasString(path.TraceIDs, hvacRuleComponentReferencesComponent) {
					t.Fatal("DX wrapper-to-actual-coil reference trace is missing")
				}
			case "heating":
				want = []string{"Coil:Heating:Fuel|" + loop + "_HeatC"}
			}
			var actual []string
			for _, component := range path.Conditioning {
				actual = append(actual, component.ObjectType+"|"+component.ObjectName)
				if component.ObjectIndex < 0 || doc.Objects[component.ObjectIndex].Type != component.ObjectType || objectName(doc.Objects[component.ObjectIndex]) != component.ObjectName {
					t.Fatalf("conditioning lacks exact original object identity: %+v", component)
				}
			}
			if !reflect.DeepEqual(actual, want) {
				t.Fatalf("%s/%s conditioning = %v, want %v", zone, service, actual, want)
			}
			if service != "ventilation" && (!hvacSmallOfficeHasString(path.TraceIDs, hvacRuleBranchComponentOccurrence) || !hvacSmallOfficeHasString(path.TraceIDs, hvacRuleAirLoopZoneSplitterToTerminal)) {
				t.Fatalf("conditioning must retain both supply-branch and exact Zone delivery evidence: %v", path.TraceIDs)
			}
		}
	}
	for _, service := range []string{"cooling", "heating"} {
		if paths := hvacSmallOfficeConditioningPaths(report, "Attic", service); len(paths) != 0 {
			t.Fatalf("unserved Attic acquired conditioning: %+v", paths)
		}
	}
}

func TestHVACAirLoopConditioningRequiresExactConnectedObjects(t *testing.T) {
	tests := []struct {
		name             string
		edit             func(*testing.T, *Document)
		cooling, heating int
	}{
		{"missing actual DX child", func(t *testing.T, doc *Document) {
			hvacSmallOfficeObject(t, doc, "Coil:Cooling:DX:SingleSpeed", "PSZ-AC:1_CoolC DXCoil").Fields[0].Value = "unreferenced child"
		}, 0, 1},
		{"duplicate actual DX child", func(t *testing.T, doc *Document) {
			obj := *hvacSmallOfficeObject(t, doc, "Coil:Cooling:DX:SingleSpeed", "PSZ-AC:1_CoolC DXCoil")
			obj.Index = len(doc.Objects)
			doc.Objects = append(doc.Objects, obj)
		}, 0, 1},
		{"wrong typed child", func(t *testing.T, doc *Document) {
			obj := hvacSmallOfficeObject(t, doc, "CoilSystem:Cooling:DX", "PSZ-AC:1_CoolC")
			obj.Fields[5].Value, obj.Fields[6].Value = "Coil:Heating:Fuel", "PSZ-AC:1_HeatC"
		}, 0, 1},
		{"child inlet disconnected", func(t *testing.T, doc *Document) {
			hvacSmallOfficeSetField(t, hvacSmallOfficeObject(t, doc, "Coil:Cooling:DX:SingleSpeed", "PSZ-AC:1_CoolC DXCoil"), "Air Inlet Node Name", "disconnected DX inlet")
		}, 0, 1},
		{"wrapper inlet disconnected", func(t *testing.T, doc *Document) {
			hvacSmallOfficeObject(t, doc, "CoilSystem:Cooling:DX", "PSZ-AC:1_CoolC").Fields[2].Value = "disconnected wrapper inlet"
		}, 0, 0},
		{"branch chain disconnected", func(t *testing.T, doc *Document) {
			hvacSmallOfficeObject(t, doc, "Branch", "PSZ-AC:1 Air Loop Main Branch").Fields[8].Value = "disconnected branch inlet"
		}, 0, 0},
		{"duplicate branch wrapper", func(t *testing.T, doc *Document) {
			obj := *hvacSmallOfficeObject(t, doc, "CoilSystem:Cooling:DX", "PSZ-AC:1_CoolC")
			obj.Index = len(doc.Objects)
			doc.Objects = append(doc.Objects, obj)
		}, 0, 0},
		{"terminal not served by loop", func(t *testing.T, doc *Document) {
			obj := hvacSmallOfficeObject(t, doc, "AirTerminal:SingleDuct:ConstantVolume:NoReheat", "Core_ZN Direct Air")
			hvacSmallOfficeSetField(t, obj, "Air Inlet Node Name", "unserved terminal inlet")
		}, 0, 0},
		{"ambiguous terminal identity", func(t *testing.T, doc *Document) {
			obj := *hvacSmallOfficeObject(t, doc, "AirTerminal:SingleDuct:ConstantVolume:NoReheat", "Core_ZN Direct Air")
			obj.Index = len(doc.Objects)
			doc.Objects = append(doc.Objects, obj)
		}, 0, 0},
		{"unresolved connector declaration", func(t *testing.T, doc *Document) {
			hvacSmallOfficeObject(t, doc, "AirLoopHVAC", "PSZ-AC:1").Fields[5].Value = "missing connector list"
		}, 0, 0},
		{"cooling only", func(t *testing.T, doc *Document) {
			hvacSmallOfficeKeepCoreConditioning(t, doc, true, false)
		}, 1, 0},
		{"heating only", func(t *testing.T, doc *Document) {
			hvacSmallOfficeKeepCoreConditioning(t, doc, false, true)
		}, 0, 1},
		{"unreferenced coils do not serve NoReheat", func(t *testing.T, doc *Document) {
			hvacSmallOfficeKeepCoreConditioning(t, doc, false, false)
		}, 0, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := hvacSmallOfficeConditioningDocument(t)
			test.edit(t, &doc)
			report := AnalyzeHVAC(doc)
			if got := len(hvacSmallOfficeConditioningPaths(report, "Core_ZN", "cooling")); got != test.cooling {
				t.Fatalf("Core cooling paths = %d, want %d", got, test.cooling)
			}
			if got := len(hvacSmallOfficeConditioningPaths(report, "Core_ZN", "heating")); got != test.heating {
				t.Fatalf("Core heating paths = %d, want %d", got, test.heating)
			}
			for _, service := range []string{"cooling", "heating"} {
				if got := len(hvacSmallOfficeConditioningPaths(report, "Perimeter_ZN_1", service)); got != 1 {
					t.Fatalf("unmodified sibling loop lost %s service: %d", service, got)
				}
			}
		})
	}
}

func TestHVACAirLoopConditioningNoReheatDoesNotInferPhysicsFromName(t *testing.T) {
	for _, name := range []string{"Plain outlet", "Cooling DX chiller", "Heating boiler", "Both heat cool"} {
		delivery := HVACComponent{ObjectType: "AirTerminal:SingleDuct:ConstantVolume:NoReheat", ObjectName: name}
		if got := serviceKindForServiceChain(HVACServicePath{}, delivery); got != "ventilation" {
			t.Fatalf("NoReheat terminal %q inferred %s", name, got)
		}
	}
	// Existing plant/source-backed service semantics are not replaced.
	for _, service := range []string{"cooling", "heating"} {
		chain := HVACServicePath{Component: "Coil:" + service + ":Water source"}
		if got := serviceKindForServiceChain(chain, HVACComponent{ObjectType: "AirTerminal:SingleDuct:ConstantVolume:NoReheat"}); got != service {
			t.Fatalf("typed source chain changed: %s -> %s", service, got)
		}
	}
}

func TestHVACAirLoopConditioningTwoLoopsInOneZoneDoNotCrossTerminals(t *testing.T) {
	doc := hvacSmallOfficeConditioningDocument(t)
	hvacSmallOfficeKeepLoopConditioning(t, &doc, "PSZ-AC:1", true, false)
	hvacSmallOfficeKeepLoopConditioning(t, &doc, "PSZ-AC:2", false, true)
	coreEquipment := hvacSmallOfficeObject(t, &doc, "ZoneHVAC:EquipmentList", "Core_ZN Equipment")
	otherEquipment := hvacSmallOfficeObject(t, &doc, "ZoneHVAC:EquipmentList", "Perimeter_ZN_1 Equipment")
	coreEquipment.Fields = append(coreEquipment.Fields, otherEquipment.Fields[2:]...)
	otherEquipment.Fields = otherEquipment.Fields[:2]
	coreInlets := hvacSmallOfficeObject(t, &doc, "NodeList", "Core_ZN Inlet Nodes")
	coreInlets.Fields = append(coreInlets.Fields, Field{Value: "Perimeter_ZN_1 Direct Air Inlet Node Name"})
	report := AnalyzeHVAC(doc)
	var coreRelation *HVACZoneChain
	for index := range report.ZoneRelations {
		if report.ZoneRelations[index].ZoneName == "Core_ZN" {
			coreRelation = &report.ZoneRelations[index]
		}
	}
	if coreRelation == nil || len(coreRelation.AirLoopNames) != 2 || len(coreRelation.TerminalUnits) != 2 {
		t.Fatalf("fixture must really expose two loops/two terminals in one Zone: %+v", coreRelation)
	}
	for _, test := range []struct{ service, loop, terminal, coil string }{
		{"cooling", "PSZ-AC:1", "Core_ZN Direct Air", "PSZ-AC:1_CoolC DXCoil"},
		{"heating", "PSZ-AC:2", "Perimeter_ZN_1 Direct Air", "PSZ-AC:2_HeatC"},
	} {
		paths := hvacSmallOfficeConditioningPaths(report, "Core_ZN", test.service)
		if len(paths) != 1 {
			t.Fatalf("%s has %d paths, want one exact coil/loop/terminal path", test.service, len(paths))
		}
		path := paths[0]
		if path.AirLoop == nil || path.AirLoop.Name != test.loop || path.Delivery.ObjectName != test.terminal {
			t.Fatalf("conditioning crossed terminal ownership: %+v", path)
		}
		found := false
		for _, component := range path.Conditioning {
			found = found || component.ObjectName == test.coil
		}
		if !found {
			t.Fatalf("exact original coil missing: %+v", path.Conditioning)
		}
	}
}

func hvacSmallOfficeConditioningDocument(t *testing.T) Document {
	t.Helper()
	data, err := os.ReadFile("../simulation/testdata/energy_path_real_models/models/25.1/RefBldgSmallOfficeNew2004_Chicago.idf")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func hvacSmallOfficeConditioningPaths(report HVACReport, zone, service string) []ZoneServicePath {
	var paths []ZoneServicePath
	for _, summary := range report.ServiceModel.ZoneServices {
		if strings.EqualFold(summary.ZoneName, zone) {
			for _, path := range summary.Paths {
				if path.ServiceKind == service {
					paths = append(paths, path)
				}
			}
		}
	}
	return paths
}

func hvacSmallOfficeObject(t *testing.T, doc *Document, objectType, name string) *Object {
	t.Helper()
	for index := range doc.Objects {
		obj := &doc.Objects[index]
		if strings.EqualFold(obj.Type, objectType) && strings.EqualFold(objectName(*obj), name) {
			return obj
		}
	}
	t.Fatalf("original object not found: %s %s", objectType, name)
	return nil
}

func hvacSmallOfficeSetField(t *testing.T, obj *Object, name, value string) {
	t.Helper()
	_, index, ok := fieldValueIndexByCatalogName(*obj, name)
	if !ok || index < 0 {
		t.Fatalf("missing original field %s in %s", name, obj.Type)
	}
	obj.Fields[index].Value = value
}

func hvacSmallOfficeKeepCoreConditioning(t *testing.T, doc *Document, cooling, heating bool) {
	t.Helper()
	hvacSmallOfficeKeepLoopConditioning(t, doc, "PSZ-AC:1", cooling, heating)
}

func hvacSmallOfficeKeepLoopConditioning(t *testing.T, doc *Document, loop string, cooling, heating bool) {
	t.Helper()
	branch := hvacSmallOfficeObject(t, doc, "Branch", loop+" Air Loop Main Branch")
	fields := append([]Field(nil), branch.Fields[:6]...) // Name, pressure, OA component.
	lastNode := branch.Fields[5].Value
	if cooling {
		fields = append(fields, branch.Fields[6:10]...)
		lastNode = branch.Fields[9].Value
	}
	if heating {
		heat := append([]Field(nil), branch.Fields[10:14]...)
		heat[2].Value = lastNode
		fields = append(fields, heat...)
		hvacSmallOfficeSetField(t, hvacSmallOfficeObject(t, doc, "Coil:Heating:Fuel", loop+"_HeatC"), "Air Inlet Node Name", lastNode)
		lastNode = heat[3].Value
	}
	fan := append([]Field(nil), branch.Fields[14:18]...)
	fan[2].Value = lastNode
	fields = append(fields, fan...)
	hvacSmallOfficeSetField(t, hvacSmallOfficeObject(t, doc, fan[0].Value, loop+"_Fan"), "Air Inlet Node Name", lastNode)
	branch.Fields = fields
}

func hvacSmallOfficeHasString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
