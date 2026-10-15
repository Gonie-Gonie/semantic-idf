package idf

import "testing"

func TestHVACOperatingFieldFallbacksResolveCommentFreeAvailability(t *testing.T) {
	for typ, index := range hvacOperatingAvailabilityFields {
		t.Run(typ, func(t *testing.T) {
			obj := Object{Type: typ, Fields: make([]Field, index+1)}
			obj.Fields[0].Value = "Equipment"
			obj.Fields[index].Value = "Operation Schedule"
			if !hvacOperatingAvailabilityKnown(obj) || hvacOperatingAvailabilitySchedule(obj) != "Operation Schedule" {
				t.Fatalf("schedule index %d not resolved for %s", index, typ)
			}
		})
	}
	for typ, index := range hvacOperatingManagerFields {
		t.Run(typ+" manager", func(t *testing.T) {
			obj := Object{Type: typ, Fields: make([]Field, index+1)}
			obj.Fields[0].Value = "Equipment"
			obj.Fields[index].Value = "Manager List"
			if hvacOperatingManagerListName(obj) != "Manager List" {
				t.Fatalf("manager index %d not resolved for %s", index, typ)
			}
		})
	}
}

func TestHVACOperatingFieldFallbacksDistinguishAbsentUnsupportedAndMisleadingComments(t *testing.T) {
	boiler := Object{Type: "Boiler:HotWater", Fields: []Field{{Value: "Boiler"}, {Value: "NaturalGas", Comment: "Availability Schedule Name"}}}
	if !hvacOperatingAvailabilityKnown(boiler) || hvacOperatingAvailabilitySchedule(boiler) != "" {
		t.Fatal("boiler fuel must not become an availability schedule")
	}
	unknown := Object{Type: "Coil:FutureModel", Fields: []Field{{Value: "Coil"}, {Value: "Pretend", Comment: "Availability Schedule Name"}}}
	if hvacOperatingAvailabilityKnown(unknown) || hvacOperatingAvailabilitySchedule(unknown) != "" {
		t.Fatal("unknown schema must not become known merely from comments or family prefix")
	}
	plant := Object{Type: "PlantLoop", Fields: make([]Field, 20)}
	plant.Fields[18] = Field{Value: "SequentialLoad", Comment: "Availability Manager List Name"}
	plant.Fields[19].Value = "Actual Managers"
	if hvacOperatingManagerListName(plant) != "Actual Managers" {
		t.Fatal("plant load distribution field misread as manager list")
	}
	condenser := plant
	condenser.Type = "CondenserLoop"
	if hvacOperatingManagerListName(condenser) != "" {
		t.Fatal("CondenserLoop has no availability manager field")
	}
	if hvacOperatingFieldName(plant, 18) != "Load Distribution Scheme" {
		t.Fatal("canonical fallback must precede misleading comments")
	}
}

func TestHVACOperatingControlMetadataHasMeaningWithoutComments(t *testing.T) {
	for _, test := range []struct {
		typ   string
		index int
		name  string
	}{
		{"Pump:VariableSpeed", 13, "Pump Control Type"},
		{"Pump:VariableSpeed", 14, "Pump Flow Rate Schedule Name"},
		{"Pump:ConstantSpeed", 8, "Pump Control Type"},
		{"HeaderedPumps:VariableSpeed", 15, "Pump Control Type"},
		{"Boiler:HotWater", 13, "Boiler Flow Mode"},
		{"Chiller:Electric:EIR", 22, "Chiller Flow Mode"},
		{"WaterHeater:Mixed", 3, "Deadband Temperature Difference"},
	} {
		obj := Object{Type: test.typ, Fields: make([]Field, test.index+1)}
		if name := hvacOperatingFieldName(obj, test.index); name != test.name {
			t.Errorf("%s index %d = %q, want %q", test.typ, test.index, name, test.name)
		}
	}
}
