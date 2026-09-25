package idf

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These tests are intended for internal/idf after explicit installation.
// The original is repository-vendored; no installed-engine fallback, engine
// invocation, production result, numerical candidate, or mutable capture input.
func nativeCentralHeatPumpOriginal(t *testing.T) Document {
	t.Helper()
	path := filepath.Join("..", "simulation", "testdata", "energy_path_real_models", "models", "25.1", "CentralChillerHeaterSystem_Simultaneous_Cooling_Heating.idf")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if hash := fmt.Sprintf("%x", sha256.Sum256(raw)); hash != "84d389f1c40ab098eea05105664dd71d54c58509a4d812bf6d5dfe4e933c28f0" {
		t.Fatalf("original identity changed: %s", hash)
	}
	doc, err := Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func centralHeatPumpTestObject(t *testing.T, doc *Document, kind, name string) *Object {
	t.Helper()
	for i := range doc.Objects {
		if centralHeatPumpEqual(doc.Objects[i].Type, kind) && centralHeatPumpEqual(centralHeatPumpField(doc.Objects[i], 0), name) {
			return &doc.Objects[i]
		}
	}
	t.Fatalf("missing fixture %s/%s", kind, name)
	return nil
}

func centralHeatPumpTestCopy(doc *Document, object Object) {
	object.Fields = append([]Field(nil), object.Fields...)
	maxIndex := -1
	for _, prior := range doc.Objects {
		if prior.Index > maxIndex {
			maxIndex = prior.Index
		}
	}
	object.Index = maxIndex + 1
	doc.Objects = append(doc.Objects, object)
}

func TestNativeCentralHeatPumpOriginalThreePortOccurrences(t *testing.T) {
	doc := nativeCentralHeatPumpOriginal(t)
	before := doc.String()
	bindings := ResolveNativeCentralHeatPumpBindings(doc)
	if before != doc.String() {
		t.Fatal("resolver mutated original")
	}
	if len(bindings) != 1 {
		t.Fatalf("native system count = %d", len(bindings))
	}
	binding := bindings[0]
	if !binding.ReportingIdentityValid || !binding.NativeDefinitionValid || !binding.PortBindingsComplete || len(binding.Issues) != 0 {
		t.Fatalf("original native binding: %+v", binding)
	}
	if binding.System.ObjectIndex != 269 || binding.System.ID != "component:269" || binding.System.ObjectName != "ChillerBank" || binding.System.ObjectType != nativeCentralHeatPumpType {
		t.Fatalf("wrong system identity: %+v", binding.System)
	}
	if len(binding.Modules) != 1 || binding.Modules[0].Performance.ObjectIndex != 268 || binding.Modules[0].Performance.ObjectName != "ChillerHeaterModule" || binding.Modules[0].ControlSchedule.ObjectName != "ON" || binding.Modules[0].Count != 3 || binding.Modules[0].FirstUnit != 1 {
		t.Fatalf("three native units were collapsed or multiplied: %+v", binding.Modules)
	}
	for _, item := range []struct {
		port                                                  NativeCentralHeatPumpPort
		role, side, loopType, loopName, branch, inlet, outlet string
		loopIndex, branchIndex, inletField, outletField       int
	}{
		{binding.Cooling, "cooling", "supply", "PlantLoop", "Chilled Water Loop", "Big Chiller Branch", "Big Chiller Inlet Node", "Big Chiller Outlet Node", 274, 190, 2, 3},
		{binding.Heating, "heating", "supply", "PlantLoop", "Hot Water Loop", "Hot Water Branch", "HotwInlet", "HotwOutlet", 275, 212, 6, 7},
		{binding.Source, "source", "demand", "CondenserLoop", "Chilled Water Condenser Loop", "Big Chiller Condenser Branch", "Big Chiller Condenser Inlet Node", "Big Chiller Condenser Outlet Node", 276, 197, 4, 5},
	} {
		p := item.port
		if !p.Bound || p.Role != item.role || p.Side != item.side || p.Loop.Type != item.loopType || p.Loop.Name != item.loopName || p.Loop.ObjectIndex != item.loopIndex || p.Branch.ObjectIndex != item.branchIndex || p.Branch.ObjectName != item.branch || p.InletNode != item.inlet || p.OutletNode != item.outlet || p.InletField != item.inletField || p.OutletField != item.outletField || p.BranchComponentField != 2 {
			t.Fatalf("incorrect %s physical occurrence: %+v", item.role, p)
		}
	}
	for _, service := range []string{"cooling", "heating"} {
		want := binding.Cooling
		if service == "heating" {
			want = binding.Heating
		}
		port, ok := NativeCentralHeatPumpServicePort(binding, service, want.Loop.Type, want.Loop.Name)
		if !ok || !reflect.DeepEqual(port, want) {
			t.Fatalf("correct %s occurrence not retained", service)
		}
		for _, wrong := range []NativeCentralHeatPumpPort{binding.Cooling, binding.Heating, binding.Source} {
			if wrong.Role == service {
				continue
			}
			if _, ok := NativeCentralHeatPumpServicePort(binding, service, wrong.Loop.Type, wrong.Loop.Name); ok {
				t.Fatalf("%s borrowed %s loop", service, wrong.Role)
			}
		}
	}
	if _, ok := NativeCentralHeatPumpServicePort(binding, "source", binding.Source.Loop.Type, binding.Source.Loop.Name); ok {
		t.Fatal("source heat exchange became purchased/space supply")
	}
}

func TestNativeCentralHeatPumpOriginalRejectsAmbiguousOrDisconnectedPorts(t *testing.T) {
	for _, mutation := range []struct {
		name   string
		mutate func(*testing.T, *Document)
	}{
		{"cooling inlet", func(t *testing.T, d *Document) {
			centralHeatPumpTestObject(t, d, nativeCentralHeatPumpType, "ChillerBank").Fields[2].Value = "Disconnected"
		}},
		{"heating outlet", func(t *testing.T, d *Document) {
			centralHeatPumpTestObject(t, d, nativeCentralHeatPumpType, "ChillerBank").Fields[7].Value = "Disconnected"
		}},
		{"source outlet", func(t *testing.T, d *Document) {
			centralHeatPumpTestObject(t, d, nativeCentralHeatPumpType, "ChillerBank").Fields[5].Value = "Disconnected"
		}},
		{"branch wrong inlet", func(t *testing.T, d *Document) {
			centralHeatPumpTestObject(t, d, "Branch", "Big Chiller Branch").Fields[4].Value = "Disconnected"
		}},
		{"branch reversed source", func(t *testing.T, d *Document) {
			o := centralHeatPumpTestObject(t, d, "Branch", "Big Chiller Condenser Branch")
			o.Fields[4], o.Fields[5] = o.Fields[5], o.Fields[4]
		}},
		{"fourth disconnected occurrence", func(t *testing.T, d *Document) {
			o := *centralHeatPumpTestObject(t, d, "Branch", "Big Chiller Branch")
			o.Fields = append([]Field(nil), o.Fields...)
			o.Fields[0].Value = "Orphan extra"
			centralHeatPumpTestCopy(d, o)
		}},
		{"duplicate branch name", func(t *testing.T, d *Document) {
			centralHeatPumpTestCopy(d, *centralHeatPumpTestObject(t, d, "Branch", "Hot Water Branch"))
		}},
		{"off stride shadow reference", func(t *testing.T, d *Document) {
			o := *centralHeatPumpTestObject(t, d, "Branch", "Big Chiller Branch")
			o.Fields = append([]Field(nil), o.Fields...)
			o.Fields[0].Value = "Off stride"
			o.Fields = append(o.Fields[:2], append([]Field{{Value: "x"}}, o.Fields[2:]...)...)
			centralHeatPumpTestCopy(d, o)
		}},
		{"list omits cooling occurrence", func(t *testing.T, d *Document) {
			o := centralHeatPumpTestObject(t, d, "BranchList", "Cooling Supply Side Branches")
			o.Fields = append(o.Fields[:2], o.Fields[3:]...)
		}},
		{"splitter omits heating occurrence", func(t *testing.T, d *Document) {
			o := centralHeatPumpTestObject(t, d, "Connector:Splitter", "Heating Supply Splitter")
			o.Fields = append(o.Fields[:2], o.Fields[3:]...)
		}},
		{"mixer duplicates sibling", func(t *testing.T, d *Document) {
			o := centralHeatPumpTestObject(t, d, "Connector:Mixer", "CW Loop Mixer")
			o.Fields[2] = o.Fields[3]
		}},
		{"unowned alias BranchList", func(t *testing.T, d *Document) {
			o := *centralHeatPumpTestObject(t, d, "BranchList", "Cooling Supply Side Branches")
			o.Fields = append([]Field(nil), o.Fields...)
			o.Fields[0].Value = "Unowned alias"
			centralHeatPumpTestCopy(d, o)
		}},
		{"loop duplicate", func(t *testing.T, d *Document) {
			centralHeatPumpTestCopy(d, *centralHeatPumpTestObject(t, d, "PlantLoop", "Hot Water Loop"))
		}},
		{"loop second owner distinct name", func(t *testing.T, d *Document) {
			o := *centralHeatPumpTestObject(t, d, "PlantLoop", "Hot Water Loop")
			o.Fields = append([]Field(nil), o.Fields...)
			o.Fields[0].Value = "Second owner"
			centralHeatPumpTestCopy(d, o)
		}},
		{"source on wrong side", func(t *testing.T, d *Document) {
			o := centralHeatPumpTestObject(t, d, "CondenserLoop", "Chilled Water Condenser Loop")
			for i := 0; i < 4; i++ {
				o.Fields[10+i], o.Fields[14+i] = o.Fields[14+i], o.Fields[10+i]
			}
		}},
		{"source mislabeled PlantLoop", func(t *testing.T, d *Document) {
			centralHeatPumpTestObject(t, d, "CondenserLoop", "Chilled Water Condenser Loop").Type = "PlantLoop"
		}},
		{"loop inlet disconnected", func(t *testing.T, d *Document) {
			centralHeatPumpTestObject(t, d, "PlantLoop", "Chilled Water Loop").Fields[10].Value = "Disconnected"
		}},
		{"boundary component disconnected", func(t *testing.T, d *Document) {
			centralHeatPumpTestObject(t, d, "Branch", "Cooling Supply Outlet").Fields[5].Value = "Disconnected"
		}},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			doc := nativeCentralHeatPumpOriginal(t)
			mutation.mutate(t, &doc)
			bindings := ResolveNativeCentralHeatPumpBindings(doc)
			if len(bindings) != 1 || !bindings[0].ReportingIdentityValid || !bindings[0].NativeDefinitionValid || bindings[0].PortBindingsComplete || len(bindings[0].Issues) == 0 {
				t.Fatalf("must retain observed-source identity but deny allocation topology: %+v", bindings)
			}
			for _, service := range []string{"cooling", "heating"} {
				if _, ok := NativeCentralHeatPumpServicePort(bindings[0], service, "PlantLoop", "Chilled Water Loop"); ok {
					t.Fatal("partial topology granted a service")
				}
			}
		})
	}
}

func TestNativeCentralHeatPumpStructuralDefinitionsAndNativeDefaults(t *testing.T) {
	for _, field := range []int{1, 8, 9, 13} {
		doc := nativeCentralHeatPumpOriginal(t)
		centralHeatPumpTestObject(t, &doc, nativeCentralHeatPumpType, "ChillerBank").Fields[field].Value = ""
		binding := ResolveNativeCentralHeatPumpBindings(doc)[0]
		if !binding.NativeDefinitionValid || !binding.PortBindingsComplete {
			t.Fatalf("native default field %d rejected: %+v", field, binding)
		}
		if field == 13 && binding.Modules[0].Count != 1 {
			t.Fatal("native blank module count must default to one, not observed system-energy multiplier")
		}
	}
	for _, mutation := range []struct {
		field int
		value string
	}{
		{1, "UnknownControl"}, {8, "-1"}, {8, "NaN"}, {8, "Inf"}, {9, "MissingSchedule"},
		{10, "HeatPump:Unreviewed"}, {11, "MissingModule"}, {12, ""}, {12, "MissingSchedule"},
		{13, "0"}, {13, "1.5"}, {13, "NaN"}, {13, "Inf"}, {13, "1e100"}, {4, "Big Chiller Inlet Node"},
	} {
		doc := nativeCentralHeatPumpOriginal(t)
		centralHeatPumpTestObject(t, &doc, nativeCentralHeatPumpType, "ChillerBank").Fields[mutation.field].Value = mutation.value
		binding := ResolveNativeCentralHeatPumpBindings(doc)[0]
		if !binding.ReportingIdentityValid || binding.NativeDefinitionValid || binding.PortBindingsComplete {
			t.Fatalf("invalid structural field %d=%q accepted: %+v", mutation.field, mutation.value, binding)
		}
	}
	for _, kindName := range [][2]string{{"ChillerHeaterPerformance:Electric:EIR", "ChillerHeaterModule"}, {"Schedule:Compact", "ON"}} {
		doc := nativeCentralHeatPumpOriginal(t)
		centralHeatPumpTestCopy(&doc, *centralHeatPumpTestObject(t, &doc, kindName[0], kindName[1]))
		binding := ResolveNativeCentralHeatPumpBindings(doc)[0]
		if !binding.ReportingIdentityValid || binding.NativeDefinitionValid {
			t.Fatalf("ambiguous module/control borrowed one original: %+v", binding)
		}
	}
	for _, kindName := range [][2]string{{nativeCentralHeatPumpType, "ChillerBank"}, {"Version", "25.1"}} {
		doc := nativeCentralHeatPumpOriginal(t)
		centralHeatPumpTestCopy(&doc, *centralHeatPumpTestObject(t, &doc, kindName[0], kindName[1]))
		for _, binding := range ResolveNativeCentralHeatPumpBindings(doc) {
			if binding.ReportingIdentityValid || binding.PortBindingsComplete {
				t.Fatalf("ambiguous reporting identity accepted: %+v", binding)
			}
		}
	}
	doc := nativeCentralHeatPumpOriginal(t)
	centralHeatPumpTestObject(t, &doc, "Version", "25.1").Fields[0].Value = "22.1"
	if binding := ResolveNativeCentralHeatPumpBindings(doc)[0]; binding.ReportingIdentityValid || binding.NativeDefinitionValid || binding.PortBindingsComplete {
		t.Fatal("unreviewed version borrowed 25.1 native offsets")
	}
}

func TestNativeCentralHeatPumpBindingIgnoresCommentsAndMisleadingNames(t *testing.T) {
	doc := nativeCentralHeatPumpOriginal(t)
	baseline := ResolveNativeCentralHeatPumpBindings(doc)
	for oi := range doc.Objects {
		for fi := range doc.Objects[oi].Fields {
			doc.Objects[oi].Fields[fi].Comment = "Heating Loop Outlet Node Name -- deliberately false"
		}
	}
	if got := ResolveNativeCentralHeatPumpBindings(doc); !reflect.DeepEqual(got, baseline) {
		t.Fatal("editable comments changed native port authority")
	}
	for oi := range doc.Objects {
		for fi := range doc.Objects[oi].Fields {
			switch strings.TrimSpace(doc.Objects[oi].Fields[fi].Value) {
			case "Chilled Water Loop":
				doc.Objects[oi].Fields[fi].Value = "Heating word in cooling loop label"
			case "Hot Water Loop":
				doc.Objects[oi].Fields[fi].Value = "Cooling word in heating loop label"
			}
		}
	}
	binding := ResolveNativeCentralHeatPumpBindings(doc)[0]
	if !binding.PortBindingsComplete || binding.Cooling.Loop.Name != "Heating word in cooling loop label" || binding.Heating.Loop.Name != "Cooling word in heating loop label" {
		t.Fatalf("semantic labels replaced native circuit fields: %+v", binding)
	}
	if got := ResolveNativeCentralHeatPumpBindings(Document{}); got != nil {
		t.Fatal("absent family must preserve fast nil path")
	}
}
