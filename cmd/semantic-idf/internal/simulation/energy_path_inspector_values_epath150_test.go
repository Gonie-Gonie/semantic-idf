package simulation

import (
	"encoding/json"
	"testing"
)

func TestEPATH150AllocatedDriverJSONDistinguishesZeroFromMissing(t *testing.T) {
	for _, test := range []struct {
		name, stored string
		present      bool
	}{
		{name: "fresh allocated zero pressure", present: true},
		{name: "stored missing pressure", stored: `{"level":"driver","allocationApplied":true,"value":7,"allocatedValue":7}`},
		{name: "stored explicit zero pressure", stored: `{"level":"driver","allocationApplied":true,"value":7,"allocatedValue":7,"rawValue":0,"effectiveValue":0}`, present: true},
		{name: "stored null is unavailable", stored: `{"level":"driver","allocationApplied":true,"value":7,"allocatedValue":7,"rawValue":null,"effectiveValue":null}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			node := EnergyExplanationNode{Level: "driver", Value: 7, AllocatedValue: 7, AllocationApplied: true}
			if test.stored != "" {
				if err := json.Unmarshal([]byte(test.stored), &node); err != nil {
					t.Fatal(err)
				}
			}
			for round := 0; round < 3; round++ {
				encoded, err := json.Marshal(node)
				if err != nil {
					t.Fatal(err)
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(encoded, &fields); err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{"rawValue", "effectiveValue"} {
					if raw, present := fields[key]; present != test.present || present && string(raw) != "0" {
						t.Fatalf("round %d %s = %s, present %v; want explicit zero %v", round, key, raw, present, test.present)
					}
				}
				if string(fields["allocatedValue"]) != "7" || string(fields["value"]) != "7" {
					t.Fatalf("allocated contribution changed: %s", encoded)
				}
				if err := json.Unmarshal(encoded, &node); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestEPATH150GeneratedOtherZeroSurvivesResultPeriodAndZoneWire(t *testing.T) {
	result := review080BuildResult([]energyExplanationSeries{
		review080Load("lab-cooling", "Lab", "cooling", map[int]float64{1: 3, 2: 4}),
	}, map[string]float64{"Lab": 1})
	for round := 0; round < 3; round++ {
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		found := 0
		var visit func(any)
		visit = func(value any) {
			switch item := value.(type) {
			case map[string]any:
				if item["level"] == "driver" && item["driverCategory"] == energyDriverCategoryStorageOther && item["allocationApplied"] == true {
					found++
					if item["rawValue"] != float64(0) || item["effectiveValue"] != float64(0) || item["allocatedValue"] != item["value"] {
						t.Fatalf("round %d lost Other/storage zero pressure: %#v", round, item)
					}
				}
				for _, child := range item {
					visit(child)
				}
			case []any:
				for _, child := range item {
					visit(child)
				}
			}
		}
		visit(decoded)
		if found < 6 {
			t.Fatalf("expected building/zone annual and monthly Other/storage nodes, got %d", found)
		}
		if err := json.Unmarshal(encoded, &result); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEPATH150NodeJSONKeepsLegacyShapeAndRejectsInvalidNumber(t *testing.T) {
	for _, level := range []string{"heat", "energy", "load", "end_use", "carrier"} {
		encoded, err := json.Marshal(EnergyExplanationNode{Level: level, Value: 7, AllocationApplied: true})
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(encoded, &fields)
		for _, key := range []string{"rawValue", "effectiveValue", "allocatedValue"} {
			if _, present := fields[key]; present {
				t.Fatalf("legacy/non-driver %s unexpectedly gained %s", level, key)
			}
		}
	}
	var node EnergyExplanationNode
	if err := json.Unmarshal([]byte(`{"level":"driver","rawValue":"not a number"}`), &node); err == nil {
		t.Fatal("invalid numeric field was accepted")
	}
	if err := json.Unmarshal([]byte(`{"level":"driver","allocationApplied":true,"rawValue":0,"effectiveValue":0,"allocatedValue":0}`), &node); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"level":"driver","allocationApplied":true}`), &node); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(node)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &fields)
	if _, present := fields["rawValue"]; present {
		t.Fatal("reused decode target retained stale presence")
	}
}
