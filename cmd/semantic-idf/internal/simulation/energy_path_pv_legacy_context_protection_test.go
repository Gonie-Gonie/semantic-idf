package simulation

// Legacy context protection. Requires the narrow root marker dispatch in the note.
import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPVLegacyContextDraftActualRemovedIDsOnlyAndNoNewObservation(t *testing.T) {
	original := []EnergyDataSource{
		{ID: "removed", Name: "Electric Storage Discharge Energy", ReportingFrequency: "Timestep", RawValue: 1.25, EffectiveValue: 7.5,
			AggregationBasis: "legacy_unresolved", MultiplierApplication: "legacy_unknown", EffectiveMultiplier: 6,
			ZoneName: "STALE ZONE", AllocationApplied: true, AllocationFactor: .5, AllocatedValue: 3.75,
			AllocationExplanation: "stale graph", AllocationFormula: "stale allocation", Formula: "stale allocation",
			ScopeDetails: []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "STALE ZONE"}}}},
		{ID: "same-name-not-removed", Name: "Electric Storage Discharge Energy", ReportingFrequency: "Timestep", RawValue: 9},
		{ID: "ordinary-context", DriverRole: energyDriverSourceRoleContext, RawValue: 2},
		protectEnergyPathPVSourceObservation(EnergyDataSource{ID: "native", RawValue: 0, NormalizedUnit: "kWh", observedValuePresence: energySourceObservedRaw}),
	}
	got := protectEnergyPathPVLegacyContextSources(original, []string{"removed", "native", "missing", ""})
	source := got[0]
	if source.pvObservationProtection == nil || !source.pvObservationProtection.LegacyContextOnly ||
		source.RawValue != 1.25 || source.EffectiveValue != 7.5 || source.observedValuePresence != 0 || source.inspectorValuePresence != 0 || source.inspectorDecodedFromJSON ||
		source.AggregationBasis != "legacy_unresolved" || source.MultiplierApplication != "legacy_unknown" || source.EffectiveMultiplier != 6 {
		t.Fatalf("legacy context changed source values/presence/basis: %+v", source)
	}
	if source.ZoneName != "" || source.AllocationApplied || source.AllocatedValue != 0 || source.AllocationFactor != 0 || len(source.ScopeDetails) > 0 || source.Formula != "" {
		t.Fatalf("allocation state retained: %+v", source)
	}
	if !reflect.DeepEqual(got[1], original[1]) || !reflect.DeepEqual(got[2], original[2]) || !reflect.DeepEqual(got[3], original[3]) || got[3].pvObservationProtection.LegacyContextOnly {
		t.Fatal("unselected source/native marker changed")
	}
	if original[0].ZoneName != "STALE ZONE" || original[0].pvObservationProtection != nil {
		t.Fatal("input source mutated")
	}
	if next := protectEnergyPathPVLegacyContextSources(got, []string{"removed"}); !reflect.DeepEqual(next, got) {
		t.Fatal("not idempotent")
	}
}

func TestPVLegacyContextDraftTwoJSONsRetainZeroUnknownAndLegacyNonzero(t *testing.T) {
	initial := protectEnergyPathPVLegacyContextSources([]EnergyDataSource{
		{ID: "zero", RawValue: 0, NormalizedUnit: "kWh", AggregationBasis: "unchanged", observedValuePresence: energySourceObservedRaw},
		{ID: "unknown", NormalizedUnit: "kWh", AggregationBasis: "unchanged"},
		{ID: "legacy-number", RawValue: 1.25, EffectiveValue: 2.5, NormalizedUnit: "kWh", AggregationBasis: "old_basis", EffectiveMultiplier: 2, MultiplierApplication: "old_application"},
	}, []string{"zero", "unknown", "legacy-number"})
	for _, version := range []string{"v1", "v2-source-wire"} {
		t.Run(version, func(t *testing.T) {
			sources := initial
			for pass := 0; pass < 2; pass++ {
				var payload []byte
				var err error
				if version == "v1" {
					payload, err = json.Marshal(EnergyExplanationV1{Schema: energyExplanationV1Schema, Sources: sources})
				} else {
					payload, err = json.Marshal(struct {
						Sources []energyPathSourceWire `json:"sources"`
					}{energyPathSourcesForWire(sources)})
				}
				if err != nil {
					t.Fatal(err)
				}
				var raw struct {
					Sources []map[string]json.RawMessage `json:"sources"`
				}
				if err = json.Unmarshal(payload, &raw); err != nil {
					t.Fatal(err)
				}
				if string(raw.Sources[0]["rawValue"]) != "0" || len(raw.Sources[0]["effectiveValue"]) != 0 || len(raw.Sources[1]["rawValue"]) != 0 || len(raw.Sources[1]["effectiveValue"]) != 0 || string(raw.Sources[2]["rawValue"]) != "1.25" || string(raw.Sources[2]["effectiveValue"]) != "2.5" {
					t.Fatalf("pass%d presence changed: %s", pass, payload)
				}
				var decoded struct {
					Sources []EnergyDataSource `json:"sources"`
				}
				if err = json.Unmarshal(payload, &decoded); err != nil {
					t.Fatal(err)
				}
				sources = decoded.Sources
				for _, source := range sources {
					if source.pvObservationProtection == nil || !source.pvObservationProtection.LegacyContextOnly || source.ZoneName != "" || source.AllocationApplied || len(source.ScopeDetails) > 0 {
						t.Fatalf("denial lost: %+v", source)
					}
				}
				if sources[0].AggregationBasis != "unchanged" || sources[0].EffectiveMultiplier != 0 || sources[0].MultiplierApplication != "" || sources[2].AggregationBasis != "old_basis" || sources[2].EffectiveMultiplier != 2 || sources[2].MultiplierApplication != "old_application" {
					t.Fatal("model-total basis was invented")
				}
			}
		})
	}
}

func TestPVLegacyContextDraftStaleGraphCannotBackfillOrAllocate(t *testing.T) {
	source := protectEnergyPathPVLegacyContextSources([]EnergyDataSource{{ID: "legacy", RawValue: 1.25, NormalizedUnit: "kWh", observedValuePresence: energySourceObservedRaw}}, []string{"legacy"})[0]
	nodes := []EnergyExplanationNode{{ID: "stale", Level: "energy", Kind: "energy.electricity", Value: 80, RawValue: 80, EffectiveValue: 80, AllocatedValue: 40, SourceIDs: []string{"legacy"}}}
	for _, scope := range []EnergyExplanationScope{{Kind: "building"}, {Kind: "zone", ZoneName: "ZONE A"}} {
		got := filterEnergyDataSourcesForV2([]EnergyDataSource{source}, nodes, nil, nodes, nil, nil, scope, "")
		if len(got) != 1 || got[0].RawValue != 1.25 || got[0].EffectiveValue != 0 || energyDataSourceValueKnown(got[0], energySourceObservedEffective) || got[0].AllocatedValue != 0 || got[0].AllocationApplied || got[0].EffectiveMultiplier != 0 || got[0].AggregationBasis != "" {
			t.Fatalf("scope=%+v borrowed graph observation: %+v", scope, got)
		}
		parent := []EnergyDataSource{source}
		appendEnergyDataSourceScopeDetails(parent, got, scope)
		if len(parent[0].ScopeDetails) != 0 {
			t.Fatal("invented scoped allocation")
		}
	}
}

func TestPVLegacyContextDraftMalformedModeRetainsDenialWithoutNewBasis(t *testing.T) {
	for _, mode := range []string{"null", "1", "\"future\"", "[]"} {
		raw := []byte(`{"id":"old","rawValue":0,"aggregationBasis":"old","nativeElectricalObservation":{"schema":"semantic-idf.native-electrical-observation/v1","legacyContextOnly":` + mode + `}}`)
		var source EnergyDataSource
		if err := json.Unmarshal(raw, &source); err != nil {
			t.Fatal(err)
		}
		if source.pvObservationProtection == nil || !source.pvObservationProtection.LegacyContextOnly || !source.pvObservationProtection.Invalid || source.AggregationBasis != "old" || source.EffectiveMultiplier != 0 || !energyDataSourceValueKnown(source, energySourceObservedRaw) || energyDataSourceValueKnown(source, energySourceObservedEffective) {
			t.Fatalf("malformed mode lost denial or invented basis: %+v", source)
		}
	}
}
