package idf

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func nativeWindowACFixture(t *testing.T) Document {
	t.Helper()
	doc, err := Parse(`Version,25.1;
Zone,Office,0,0,0,0,1,1;
Zone,Other,0,0,0,0,1,1;
ZoneHVAC:EquipmentConnections,Office,Equipment,Outlet,Inlet,Office Air,;
ZoneHVAC:EquipmentList,Equipment,SequentialLoad,ZoneHVAC:WindowAirConditioner,Office Window AC,1,2,,,ZoneHVAC:Baseboard:Convective:Electric,Office Heater,2,1,,;
ZoneHVAC:WindowAirConditioner,Office Window AC,Always,Autosize,Autosize,Inlet,Outlet,OutdoorAir:Mixer,Mixer,Fan:OnOff,Fan,Coil:Cooling:DX:SingleSpeed,Coil,Cycling,BlowThrough,.001;
OutdoorAir:Mixer,Mixer,Mixed,Outdoor,Relief,Inlet;
OutdoorAir:Node,Outdoor;
Fan:OnOff,Fan,Always,.7,75,Autosize,.9,1,Mixed,Fan Outlet;
Coil:Cooling:DX:SingleSpeed,Coil,Always,Autosize,.75,3,Autosize,,934.4,Fan Outlet,Outlet;
ZoneHVAC:Baseboard:Convective:Electric,Office Heater,Always,HeatingDesignCapacity,1000,,,1;
`)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func nativeWindowACObject(t *testing.T, doc *Document, kind, name string) *Object {
	t.Helper()
	for i := range doc.Objects {
		if strings.EqualFold(doc.Objects[i].Type, kind) && windowACField(doc.Objects[i], 0) == name {
			return &doc.Objects[i]
		}
	}
	t.Fatalf("missing original fixture object %s/%s", kind, name)
	return nil
}

func nativeWindowACAppend(doc *Document, kind string, values ...string) {
	object := Object{Index: len(doc.Objects), Type: kind}
	for _, value := range values {
		object.Fields = append(object.Fields, Field{Value: value})
	}
	doc.Objects = append(doc.Objects, object)
}

func nativeWindowACServicePaths(doc Document) []ZoneServicePath {
	var paths []ZoneServicePath
	for _, summary := range AnalyzeHVAC(doc).ServiceModel.ZoneServices {
		for _, path := range summary.Paths {
			if strings.EqualFold(path.Delivery.ObjectType, nativeWindowACType) {
				paths = append(paths, path)
			}
		}
	}
	return paths
}

func TestNativeWindowACBindingsRequireExactOriginalAirChain(t *testing.T) {
	for _, placement := range []string{"BlowThrough", "DrawThrough"} {
		t.Run(placement, func(t *testing.T) {
			doc := nativeWindowACFixture(t)
			if placement == "DrawThrough" {
				nativeWindowACObject(t, &doc, nativeWindowACType, "Office Window AC").Fields[13].Value = placement
				fan := nativeWindowACObject(t, &doc, "Fan:OnOff", "Fan")
				fan.Fields[7].Value, fan.Fields[8].Value = "Coil Outlet", "Outlet"
				coil := nativeWindowACObject(t, &doc, "Coil:Cooling:DX:SingleSpeed", "Coil")
				coil.Fields[8].Value, coil.Fields[9].Value = "Mixed", "Coil Outlet"
			}
			before := doc.String()
			bindings := ResolveNativeWindowACBindings(doc)
			if len(bindings) != 1 {
				t.Fatalf("native %s bindings=%+v", placement, bindings)
			}
			got := bindings[0]
			if got.ZoneName != "Office" || got.Parent.ObjectIndex != 5 || got.Mixer.ObjectIndex != 6 || got.Fan.ObjectIndex != 8 || got.Coil.ObjectIndex != 9 || got.Parent.InletNode != "Inlet" || got.Parent.OutletNode != "Outlet" {
				t.Fatalf("native original identities/ports=%+v", got)
			}
			paths := nativeWindowACServicePaths(doc)
			if len(paths) != 1 || paths[0].ServiceKind != "cooling" || paths[0].PathType != "direct_zone_air" || paths[0].AirLoop != nil || paths[0].PlantLoop != nil || paths[0].SourceSystem == nil || paths[0].SourceSystem.Name != "Local DX" {
				t.Fatalf("native cooling service=%+v", paths)
			}
			found := false
			for _, coil := range paths[0].Conditioning {
				found = found || coil.ID == got.Coil.ID
			}
			if !found || doc.String() != before {
				t.Fatal("exact child coil proof missing or original document changed")
			}
		})
	}
}

func TestNativeWindowACBindingsRejectBrokenOrAmbiguousPhysicalOwnership(t *testing.T) {
	for _, mutation := range []string{"short parent", "parent inlet", "parent outlet", "mixer return", "mixer mixed", "fan inlet", "fan outlet", "coil inlet", "coil outlet", "placement", "empty outdoor", "missing outdoor declaration", "duplicate outdoor declaration", "duplicate parent", "duplicate mixer", "duplicate fan", "duplicate coil", "duplicate Zone", "duplicate EquipmentList", "duplicate connection", "missing Zone air node", "foreign Zone inlet", "foreign Zone exhaust NodeList", "ambiguous NodeList", "missing equipment reference", "second parent reference", "second fan reference", "second coil reference", "wrong slot type", "short child"} {
		t.Run(mutation, func(t *testing.T) {
			doc := nativeWindowACFixture(t)
			parent := nativeWindowACObject(t, &doc, nativeWindowACType, "Office Window AC")
			switch mutation {
			case "short parent":
				parent.Fields = parent.Fields[:5]
			case "parent inlet":
				parent.Fields[4].Value = "Wrong"
			case "parent outlet":
				parent.Fields[5].Value = "Wrong"
			case "mixer return":
				nativeWindowACObject(t, &doc, "OutdoorAir:Mixer", "Mixer").Fields[4].Value = "Wrong"
			case "mixer mixed":
				nativeWindowACObject(t, &doc, "OutdoorAir:Mixer", "Mixer").Fields[1].Value = "Wrong"
			case "fan inlet":
				nativeWindowACObject(t, &doc, "Fan:OnOff", "Fan").Fields[7].Value = "Wrong"
			case "fan outlet":
				nativeWindowACObject(t, &doc, "Fan:OnOff", "Fan").Fields[8].Value = "Wrong"
			case "coil inlet":
				nativeWindowACObject(t, &doc, "Coil:Cooling:DX:SingleSpeed", "Coil").Fields[8].Value = "Wrong"
			case "coil outlet":
				nativeWindowACObject(t, &doc, "Coil:Cooling:DX:SingleSpeed", "Coil").Fields[9].Value = "Wrong"
			case "placement":
				parent.Fields[13].Value = "Unknown"
			case "empty outdoor":
				nativeWindowACObject(t, &doc, "OutdoorAir:Mixer", "Mixer").Fields[2].Value = ""
			case "missing outdoor declaration":
				nativeWindowACObject(t, &doc, "OutdoorAir:Node", "Outdoor").Fields[0].Value = "Other Outdoor"
			case "duplicate outdoor declaration":
				nativeWindowACAppend(&doc, "OutdoorAir:NodeList", "Outdoor")
			case "duplicate parent":
				nativeWindowACAppend(&doc, nativeWindowACType, "Office Window AC")
			case "duplicate mixer":
				nativeWindowACAppend(&doc, "OutdoorAir:Mixer", "Mixer")
			case "duplicate fan":
				nativeWindowACAppend(&doc, "Fan:OnOff", "Fan")
			case "duplicate coil":
				nativeWindowACAppend(&doc, "Coil:Cooling:DX:SingleSpeed", "Coil")
			case "duplicate Zone":
				nativeWindowACAppend(&doc, "Zone", "Office")
			case "duplicate EquipmentList":
				nativeWindowACAppend(&doc, "ZoneHVAC:EquipmentList", "Equipment")
			case "duplicate connection":
				nativeWindowACAppend(&doc, "ZoneHVAC:EquipmentConnections", "Office", "Equipment", "Outlet", "Inlet", "Office Air", "")
			case "missing Zone air node":
				nativeWindowACObject(t, &doc, "ZoneHVAC:EquipmentConnections", "Office").Fields[4].Value = ""
			case "foreign Zone inlet":
				nativeWindowACAppend(&doc, "ZoneHVAC:EquipmentConnections", "Other", "Other Equipment", "Outlet", "Other Inlet", "Other Air", "")
			case "foreign Zone exhaust NodeList":
				nativeWindowACAppend(&doc, "NodeList", "Foreign Nodes", "Inlet")
				nativeWindowACAppend(&doc, "ZoneHVAC:EquipmentConnections", "Other", "Other Equipment", "Other Outlet", "Foreign Nodes", "Other Air", "")
			case "ambiguous NodeList":
				nativeWindowACAppend(&doc, "NodeList", "Outlet", "Outlet")
				nativeWindowACAppend(&doc, "NodeList", "Outlet", "Other Outlet")
			case "missing equipment reference":
				nativeWindowACObject(t, &doc, "ZoneHVAC:EquipmentList", "Equipment").Fields[3].Value = "Missing"
			case "second parent reference":
				nativeWindowACAppend(&doc, "ZoneHVAC:EquipmentList", "Other Equipment", "SequentialLoad", nativeWindowACType, "Office Window AC", "1", "1", "", "")
			case "second fan reference":
				nativeWindowACAppend(&doc, "AirLoopHVAC:UnitarySystem", "Foreign", "Fan:OnOff", "Fan")
			case "second coil reference":
				nativeWindowACAppend(&doc, "AirLoopHVAC:UnitarySystem", "Foreign", "Coil:Cooling:DX:SingleSpeed", "Coil")
			case "wrong slot type":
				parent.Fields[10].Value = "Coil:Heating:Fuel"
			case "short child":
				nativeWindowACObject(t, &doc, "Fan:OnOff", "Fan").Fields = nativeWindowACObject(t, &doc, "Fan:OnOff", "Fan").Fields[:8]
			}
			before := doc.String()
			if got := ResolveNativeWindowACBindings(doc); len(got) != 0 {
				t.Fatalf("invalid original acquired native targets: %+v", got)
			}
			if got := nativeWindowACServicePaths(doc); len(got) != 0 {
				t.Fatalf("invalid native route acquired cooling service: %+v", got)
			}
			if doc.String() != before {
				t.Fatal("read-only resolver changed original")
			}
		})
	}
}

func TestNativeWindowACReportingCollisionDoesNotEraseTypedPhysicalService(t *testing.T) {
	for _, kind := range []string{"Fan:ConstantVolume", "Fan:VariableVolume", "Fan:SystemModel", "Fan:ZoneExhaust", "Fan:ComponentModel", "Coil:Cooling:DX:TwoSpeed", "Coil:Cooling:DX:MultiSpeed", "Coil:Cooling:DX:TwoStageWithHumidityControlMode", "Coil:Cooling:DX:VariableSpeed", "Coil:Cooling:DX", "Coil:Cooling:DX:SingleSpeed:ThermalStorage", "Coil:Cooling:WaterToAirHeatPump:VariableSpeedEquationFit", "Coil:Cooling:WaterToAirHeatPump:EquationFit", "Coil:Cooling:WaterToAirHeatPump:ParameterEstimation", "Coil:WaterHeating:AirToWaterHeatPump:Pumped", "Coil:WaterHeating:AirToWaterHeatPump:Wrapped", "Coil:WaterHeating:AirToWaterHeatPump:VariableSpeed"} {
		t.Run(kind, func(t *testing.T) {
			doc := nativeWindowACFixture(t)
			name := "Coil"
			if strings.HasPrefix(kind, "Fan:") {
				name = "Fan"
			}
			nativeWindowACAppend(&doc, kind, name)
			if got := ResolveNativeWindowACBindings(doc); len(got) != 0 {
				t.Fatalf("competing same-output key acquired direct targets: %+v", got)
			}
			if got := nativeWindowACServicePaths(doc); len(got) != 1 || got[0].ServiceKind != "cooling" {
				t.Fatalf("reporting collision erased valid typed route: %+v", got)
			}
		})
	}
	for _, kind := range []string{"Coil:Heating:Fuel", "Coil:Heating:Water", "Coil:Cooling:Water", "Coil:Cooling:DX:CurveFit:Performance", "FanPerformance:NightVentilation"} {
		t.Run("noncollision "+kind, func(t *testing.T) {
			doc := nativeWindowACFixture(t)
			name := "Coil"
			if strings.HasPrefix(kind, "Fan") {
				name = "Fan"
			}
			nativeWindowACAppend(&doc, kind, name)
			if got := ResolveNativeWindowACBindings(doc); len(got) != 1 {
				t.Fatalf("unrelated output namespace erased native identity: %+v", got)
			}
		})
	}
}

func TestNativeWindowACOutdoorNodeListsMatchNativeExpansionAndUnion(t *testing.T) {
	for _, tc := range []struct {
		name  string
		valid bool
	}{
		{"direct list", true}, {"one-level list", true}, {"overlapping lists", true}, {"repeated list selector", true}, {"single-member selector on single", true},
		{"single overlaps indirect list", false}, {"alias overlaps actual single", false}, {"two aliases same single", false}, {"duplicate single", false}, {"duplicate NodeList", false}, {"empty NodeList", false}, {"nested NodeList", false}, {"self NodeList", false}, {"duplicate NodeList members", false}, {"multiple members on single", false}, {"unrelated selector", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := nativeWindowACFixture(t)
			// Remove only the explicit declaration, preserving original indices.
			nativeWindowACObject(t, &doc, "OutdoorAir:Node", "Outdoor").Type = "CommentOnlyFixtureObject"
			switch tc.name {
			case "direct list":
				nativeWindowACAppend(&doc, "OutdoorAir:NodeList", "Outdoor")
			case "one-level list":
				nativeWindowACAppend(&doc, "NodeList", "OA Set", "Outdoor", "Other Outdoor")
				nativeWindowACAppend(&doc, "OutdoorAir:NodeList", "OA Set")
			case "overlapping lists":
				nativeWindowACAppend(&doc, "NodeList", "OA Set", "Outdoor", "Other Outdoor")
				nativeWindowACAppend(&doc, "OutdoorAir:NodeList", "OA Set")
				nativeWindowACAppend(&doc, "OutdoorAir:NodeList", "Outdoor")
			case "repeated list selector":
				nativeWindowACAppend(&doc, "NodeList", "OA Set", "Outdoor")
				nativeWindowACAppend(&doc, "OutdoorAir:NodeList", "OA Set", "OA Set")
			case "single-member selector on single":
				nativeWindowACAppend(&doc, "NodeList", "OA Set", "Outdoor")
				nativeWindowACAppend(&doc, "OutdoorAir:Node", "OA Set")
			case "single overlaps indirect list":
				nativeWindowACAppend(&doc, "NodeList", "OA Set", "Outdoor")
				nativeWindowACAppend(&doc, "OutdoorAir:NodeList", "OA Set")
				nativeWindowACAppend(&doc, "OutdoorAir:Node", "Outdoor")
			case "alias overlaps actual single":
				nativeWindowACAppend(&doc, "NodeList", "OA Set", "Outdoor")
				nativeWindowACAppend(&doc, "OutdoorAir:Node", "OA Set")
				nativeWindowACAppend(&doc, "OutdoorAir:Node", "Outdoor")
			case "two aliases same single":
				nativeWindowACAppend(&doc, "NodeList", "OA Set", "Outdoor")
				nativeWindowACAppend(&doc, "NodeList", "OA Alias", "Outdoor")
				nativeWindowACAppend(&doc, "OutdoorAir:Node", "OA Set")
				nativeWindowACAppend(&doc, "OutdoorAir:Node", "OA Alias")
			case "duplicate single":
				nativeWindowACAppend(&doc, "OutdoorAir:Node", "Outdoor")
				nativeWindowACAppend(&doc, "OutdoorAir:Node", "Outdoor")
			case "duplicate NodeList":
				nativeWindowACAppend(&doc, "NodeList", "OA Set", "Outdoor")
				nativeWindowACAppend(&doc, "NodeList", "OA Set", "Outdoor")
				nativeWindowACAppend(&doc, "OutdoorAir:NodeList", "OA Set")
			case "empty NodeList":
				nativeWindowACAppend(&doc, "NodeList", "OA Set")
				nativeWindowACAppend(&doc, "OutdoorAir:NodeList", "OA Set")
			case "nested NodeList":
				nativeWindowACAppend(&doc, "NodeList", "OA Set", "Inner")
				nativeWindowACAppend(&doc, "NodeList", "Inner", "Outdoor")
				nativeWindowACAppend(&doc, "OutdoorAir:NodeList", "OA Set")
			case "self NodeList":
				nativeWindowACAppend(&doc, "NodeList", "Outdoor", "Outdoor")
				nativeWindowACAppend(&doc, "OutdoorAir:NodeList", "Outdoor")
			case "duplicate NodeList members":
				nativeWindowACAppend(&doc, "NodeList", "OA Set", "Outdoor", "Outdoor")
				nativeWindowACAppend(&doc, "OutdoorAir:NodeList", "OA Set")
			case "multiple members on single":
				nativeWindowACAppend(&doc, "NodeList", "OA Set", "Outdoor", "Other Outdoor")
				nativeWindowACAppend(&doc, "OutdoorAir:Node", "OA Set")
			case "unrelated selector":
				nativeWindowACAppend(&doc, "OutdoorAir:NodeList", "Other Outdoor")
			}
			before := doc.String()
			want := 0
			if tc.valid {
				want = 1
			}
			if got := ResolveNativeWindowACBindings(doc); len(got) != want {
				t.Fatalf("outdoor native direct proof=%+v, want %d", got, want)
			}
			if got := nativeWindowACServicePaths(doc); len(got) != want {
				t.Fatalf("outdoor native physical path=%+v, want %d", got, want)
			}
			if doc.String() != before {
				t.Fatal("outdoor resolution mutated original")
			}
		})
	}
}

func TestNativeWindowACZoneSelectorsRejectNestedOrSelfNodeLists(t *testing.T) {
	for _, field := range []int{2, 3} {
		for _, kind := range []string{"nested", "self", "duplicate members"} {
			t.Run(fmt.Sprintf("field%d %s", field, kind), func(t *testing.T) {
				doc := nativeWindowACFixture(t)
				connection := nativeWindowACObject(t, &doc, "ZoneHVAC:EquipmentConnections", "Office")
				port := connection.Fields[field].Value
				connection.Fields[field].Value = "Outer"
				switch kind {
				case "nested":
					nativeWindowACAppend(&doc, "NodeList", "Outer", port, "Inner")
					nativeWindowACAppend(&doc, "NodeList", "Inner", "Unrelated")
				case "self":
					nativeWindowACAppend(&doc, "NodeList", "Outer", port, "Outer")
				case "duplicate members":
					nativeWindowACAppend(&doc, "NodeList", "Outer", port, port)
				}
				if got := ResolveNativeWindowACBindings(doc); len(got) != 0 {
					t.Fatalf("invalid Zone selector became direct owner: %+v", got)
				}
				if got := nativeWindowACServicePaths(doc); len(got) != 0 {
					t.Fatalf("invalid Zone selector became physical path: %+v", got)
				}
			})
		}
	}
}

func TestNativeWindowACServiceGatePreservesCoolingCapabilityAcrossNamesAndVersions(t *testing.T) {
	t.Run("heating label cannot change native capability", func(t *testing.T) {
		doc := nativeWindowACFixture(t)
		for i := range doc.Objects {
			for j := range doc.Objects[i].Fields {
				if doc.Objects[i].Fields[j].Value == "Office Window AC" {
					doc.Objects[i].Fields[j].Value = "Office Heating Unit"
				}
			}
		}
		paths := nativeWindowACServicePaths(doc)
		if len(paths) != 1 || paths[0].ServiceKind != "cooling" || strings.Contains(paths[0].ID, ":heating:") || !strings.Contains(paths[0].ID, ":cooling:") {
			t.Fatalf("misleading name contaminated service identity: %+v", paths)
		}
	})
	t.Run("legacy 22.1 native offsets remain generic", func(t *testing.T) {
		doc := nativeWindowACFixture(t)
		nativeWindowACObject(t, &doc, "Version", "25.1").Fields[0].Value = "22.1"
		coil := nativeWindowACObject(t, &doc, "Coil:Cooling:DX:SingleSpeed", "Coil")
		coil.Fields = append(coil.Fields[:7], coil.Fields[8:]...)
		if len(ResolveNativeWindowACBindings(doc)) != 0 {
			t.Fatal("unreviewed legacy schema borrowed 25.1 direct proof")
		}
		if paths := nativeWindowACServicePaths(doc); len(paths) != 1 || paths[0].ServiceKind != "cooling" {
			t.Fatalf("valid legacy native capability erased: %+v", paths)
		}
	})
	t.Run("other explicit native fan variant remains generic", func(t *testing.T) {
		doc := nativeWindowACFixture(t)
		nativeWindowACObject(t, &doc, nativeWindowACType, "Office Window AC").Fields[8].Value = "Fan:ConstantVolume"
		nativeWindowACObject(t, &doc, "Fan:OnOff", "Fan").Type = "Fan:ConstantVolume"
		if len(ResolveNativeWindowACBindings(doc)) != 0 {
			t.Fatal("unreviewed variant borrowed OnOff direct proof")
		}
		if paths := nativeWindowACServicePaths(doc); len(paths) != 1 || paths[0].ServiceKind != "cooling" {
			t.Fatalf("valid native variant erased: %+v", paths)
		}
	})
	t.Run("broken WindowAC retains valid sibling heater", func(t *testing.T) {
		doc := nativeWindowACFixture(t)
		nativeWindowACObject(t, &doc, "Fan:OnOff", "Fan").Fields[8].Value = "Broken"
		found := 0
		for _, summary := range AnalyzeHVAC(doc).ServiceModel.ZoneServices {
			for _, path := range summary.Paths {
				if path.Delivery.ObjectName == "Office Heater" && path.ServiceKind == "heating" {
					found++
				}
			}
		}
		if found != 1 {
			t.Fatalf("local path failure removed valid sibling heater: %d", found)
		}
	})
	t.Run("empty document gate is identity", func(t *testing.T) {
		path := ZoneServicePath{ID: "existing", ServiceKind: "heating"}
		got, ok := buildHVACWindowACPathGate(Document{})(path)
		if !ok || !reflect.DeepEqual(got, path) || len(ResolveNativeWindowACBindings(Document{})) != 0 {
			t.Fatal("nonWindowAC contract changed")
		}
	})
}

func TestNativeWindowACNodeSelectorMentionsRejectsEmptyIdentities(t *testing.T) {
	doc := nativeWindowACFixture(t)
	nativeWindowACAppend(&doc, "NodeList")
	nativeWindowACAppend(&doc, "NodeList", "", "Inlet")
	nativeWindowACAppend(&doc, "NodeList", "Named Ports", "", "Inlet")
	index := newWindowACOriginalIndex(doc)
	for _, pair := range [][2]string{{"", ""}, {"", "Inlet"}, {" \t ", "Inlet"}, {"Named Ports", ""}, {"Named Ports", " \t "}, {"Inlet", ""}} {
		if index.nodeSelectorMentions(pair[0], pair[1]) {
			t.Fatalf("empty identity acquired a native port match: selector=%q node=%q", pair[0], pair[1])
		}
	}
	if !index.nodeSelectorMentions(" Inlet ", " inlet ") || !index.nodeSelectorMentions(" Named Ports ", " inlet ") {
		t.Fatal("nonblank literal or list member lost its native port match")
	}
}

func TestNativeWindowACEmptyNodeListDoesNotClaimForeignOwner(t *testing.T) {
	doc := nativeWindowACFixture(t)
	wantBindings, wantPaths := ResolveNativeWindowACBindings(doc), nativeWindowACServicePaths(doc)
	if len(wantBindings) != 1 || len(wantPaths) != 1 {
		t.Fatal("valid native WindowAC fixture lacks its original owner/path")
	}
	malformed, err := Parse("NodeList;")
	if err != nil || len(malformed.Objects) != 1 || len(malformed.Objects[0].Fields) != 0 {
		t.Fatalf("malformed original fixture did not retain its zero-field NodeList: %+v, %v", malformed, err)
	}
	malformed.Objects[0].Index = len(doc.Objects)
	doc.Objects = append(doc.Objects, malformed.Objects[0])
	nativeWindowACAppend(&doc, "ZoneHVAC:Baseboard:Convective:Electric", "Other Heater", "Always", "HeatingDesignCapacity", "1000", "", "", "1")
	nativeWindowACAppend(&doc, "ZoneHVAC:EquipmentList", "Other Equipment", "SequentialLoad", "ZoneHVAC:Baseboard:Convective:Electric", "Other Heater", "1", "1", "", "")
	nativeWindowACAppend(&doc, "ZoneHVAC:EquipmentConnections", "Other", "Other Equipment", "", "", "Other Air", "")
	before := doc.String()
	if got := ResolveNativeWindowACBindings(doc); !reflect.DeepEqual(got, wantBindings) {
		t.Fatalf("unrelated empty selectors changed or claimed the original WindowAC owner: got %+v, want %+v", got, wantBindings)
	}
	if got := nativeWindowACServicePaths(doc); !reflect.DeepEqual(got, wantPaths) {
		t.Fatalf("unrelated malformed NodeList changed the exact cooling route: got %+v, want %+v", got, wantPaths)
	}
	if doc.String() != before {
		t.Fatal("empty-selector guard mutated the original document")
	}
}

func TestNativeWindowACEmptySelectorGuardRetainsDuplicateOwnerEvidence(t *testing.T) {
	for _, field := range []int{2, 3} {
		for _, port := range []string{"Inlet", "Outlet"} {
			for _, matchingFirst := range []bool{false, true} {
				t.Run(fmt.Sprintf("field%d/%s/matchingFirst%t", field, port, matchingFirst), func(t *testing.T) {
					doc := nativeWindowACFixture(t)
					nativeWindowACAppend(&doc, "NodeList")
					first, second := "Other Port", port
					if matchingFirst {
						first, second = second, first
					}
					nativeWindowACAppend(&doc, "NodeList", "Competing Ports", first)
					nativeWindowACAppend(&doc, "NodeList", "Competing Ports", second)
					connection := []string{"Other", "Other Equipment", "", "", "Other Air", ""}
					connection[field] = "Competing Ports"
					nativeWindowACAppend(&doc, "ZoneHVAC:EquipmentConnections", connection...)
					before := doc.String()
					if !newWindowACOriginalIndex(doc).nodeSelectorMentions("Competing Ports", port) {
						t.Fatal("nonblank duplicate-list member lost its competing port evidence")
					}
					if got := ResolveNativeWindowACBindings(doc); len(got) != 0 {
						t.Fatalf("actual competing port acquired a direct owner: %+v", got)
					}
					if got := nativeWindowACServicePaths(doc); len(got) != 0 {
						t.Fatalf("actual competing port acquired a cooling service: %+v", got)
					}
					if doc.String() != before {
						t.Fatal("duplicate-owner guard mutated the original document")
					}
				})
			}
		}
	}
}
