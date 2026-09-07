package simulation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEPATH192ExactDirectionalSourceAccountingRejectsPartialAttribution(t *testing.T) {
	source := EnergyDataSource{ID: "wall", RawValue: 0, EffectiveValue: 0, EffectiveMultiplier: 1, MultiplierApplication: energyMultiplierRequiresZone, DriverRole: energyDriverSourceRoleMainFlow, DriverCategory: energyDriverCategoryExteriorWalls, ZoneName: "Office"}
	other := source
	other.ID = "other-wall"
	other.RawValue, other.EffectiveValue = 10, 10
	node := func(id, service string, allocated float64, sourceIDs ...string) EnergyExplanationNode {
		return EnergyExplanationNode{ID: id, Level: "heat", DriverCategory: energyDriverCategoryExteriorWalls, ServiceKind: service, ZoneName: "Office", RawValue: allocated, EffectiveValue: allocated, Value: allocated, AllocatedValue: allocated, AllocationApplied: true, Multiplier: 1, SourceIDs: sourceIDs, allocationSourceIDs: sourceIDs}
	}
	heating := node("heat.wall.heating", "heating", 38, source.ID)
	cooling := node("heat.wall.cooling", "cooling", 38, source.ID)
	for _, mixed := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			nodes := []EnergyExplanationNode{heating, cooling}
			if mixed {
				nodes[1] = node("heat.wall.cooling", "cooling", 38, source.ID, other.ID)
			}
			if reverse {
				nodes[0], nodes[1] = nodes[1], nodes[0]
			}
			out := filterEnergyDataSourcesForV2([]EnergyDataSource{source, other}, nodes, nil, nil, nil, nil, EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}, PurposeAllocationPolicyByServicePathLoadShare)
			wall := energyExplanationSourceByID(out, source.ID)
			if wall == nil || wall.RawValue != 0 || wall.EffectiveValue != 0 || !wall.AllocationApplied {
				t.Fatalf("mixed=%v reverse=%v: signed zero source changed: %#v", mixed, reverse, wall)
			}
			if mixed {
				if wall.AllocatedValue != 0 || strings.HasPrefix(wall.AllocationFormula, "sum(") {
					t.Fatalf("mixed=%v reverse=%v: partial sole-source evidence invented complete allocation: %#v", mixed, reverse, wall)
				}
			} else if wall.AllocatedValue != 76 || !strings.HasPrefix(wall.AllocationFormula, "sum(") || !strings.Contains(wall.AllocationExplanation, "signed source net") {
				t.Fatalf("reverse=%v: exact directional contributions/formula missing: %#v", reverse, wall)
			}
		}
	}
}

func TestEPATH192PreparedSourceZerosSurviveV2WireWithoutInventingSparseValues(t *testing.T) {
	for _, test := range []struct {
		name        string
		fields      string
		fresh       bool
		wantPresent bool
	}{
		{name: "generated", fresh: true, wantPresent: true},
		{name: "explicit zero", fields: `,"rawValue":0,"effectiveValue":0`, wantPresent: true},
		{name: "stored absent"},
		{name: "stored null", fields: `,"rawValue":null,"effectiveValue":null`},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := EnergyDataSource{ID: "wall", DriverRole: energyDriverSourceRoleMainFlow, DriverCategory: energyDriverCategoryExteriorWalls, EffectiveMultiplier: 1, MultiplierApplication: energyMultiplierRequiresZone,
				ScopeDetails: []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}, EffectiveMultiplier: 1, MultiplierApplication: energyMultiplierRequiresZone}}}
			if !test.fresh {
				raw := `{"id":"wall","driverRole":"main_flow","driverCategory":"surface.exterior_walls","effectiveMultiplier":1,"multiplierApplication":"requires_zone_multiplier"` + test.fields + `,"scopeDetails":[{"scope":{"kind":"zone","zoneName":"Office"},"effectiveMultiplier":1,"multiplierApplication":"requires_zone_multiplier"` + test.fields + `}]}`
				if err := json.Unmarshal([]byte(raw), &source); err != nil {
					t.Fatal(err)
				}
			}
			result := EnergyExplanationResult{Schema: energyExplanationSchema, Sources: []EnergyDataSource{source}}
			for reload := 0; reload < 2; reload++ {
				data, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				var wire struct {
					Sources []map[string]json.RawMessage `json:"sources"`
				}
				if err := json.Unmarshal(data, &wire); err != nil || len(wire.Sources) != 1 {
					t.Fatalf("source wire: %s / %v", data, err)
				}
				var details []map[string]json.RawMessage
				if err := json.Unmarshal(wire.Sources[0]["scopeDetails"], &details); err != nil || len(details) != 1 {
					t.Fatalf("scope detail wire: %s / %v", data, err)
				}
				for _, row := range []map[string]json.RawMessage{wire.Sources[0], details[0]} {
					for _, field := range []string{"rawValue", "effectiveValue"} {
						value, present := row[field]
						if present != test.wantPresent || present && string(value) != "0" {
							t.Fatalf("reload %d %s = %s (present %v), want known-zero presence %v", reload, field, value, present, test.wantPresent)
						}
					}
				}
				if err := json.Unmarshal(data, &result); err != nil {
					t.Fatal(err)
				}
			}
			// The v1 source writer remains omitempty even for prepared sources;
			// the new explicit-zero behavior belongs solely to the v2 boundary.
			legacy, err := json.Marshal(EnergyExplanationV1{Schema: energyExplanationV1Schema, Sources: []EnergyDataSource{source}})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(legacy), `"rawValue"`) || strings.Contains(string(legacy), `"effectiveValue"`) {
				t.Fatalf("frozen v1 zero omission changed: %s", legacy)
			}
		})
	}
}
