package simulation

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func epathRebuildWireReadScalar(t *testing.T, source EnergyDataSource, field, scope string) *float64 {
	t.Helper()
	item := epathRealOracleMetricRecipe{Scope: scope, Period: "annual", Target: epathRealOracleTarget{
		Collection: "sources", Field: field, SourceName: source.Name, SourceKey: source.KeyValue,
		Frequency: source.ReportingFrequency, SourceUnit: source.SourceUnit, Unit: source.NormalizedUnit,
	}}
	if scope == "zone" {
		item.Zone = "Office"
	}
	value, err := epathReadOracleSourceCandidate([]EnergyDataSource{source}, item)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestEnergyPathRealRebuildWirePreservesObservedZero(t *testing.T) {
	// Actual SQL parser proof, not a graph-value heuristic: twelve independently
	// declared Weather Monthly rows of zero for an ordinary non-driver meter.
	path := epathRealZeroSourceSQL(t, "zero")
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, nil, energyDriverBuildContext{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	legacyBefore, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	native := PurposeResultBundle{EnergyExplanation: UpgradeEnergyExplanationV1(legacy)}
	if len(native.EnergyExplanation.Sources) != 1 {
		t.Fatal("known-zero original meter missing")
	}
	original := native.EnergyExplanation.Sources[0]
	if original.inspectorDecodedFromJSON || original.DriverRole != "" || !energyDataSourceValueKnown(original, energySourceObservedRaw) || !energyDataSourceValueKnown(original, energySourceObservedEffective) {
		t.Fatalf("fixture does not expose the native/wire presence boundary: %+v", original)
	}
	if got := epathRebuildWireReadScalar(t, original, "effectiveValue", "building"); got != nil {
		t.Fatal("fixture no longer reproduces the original native-only oracle mismatch")
	}
	actual, err := epathRealRebuiltBundleForOracle(native)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual.EnergyExplanation.Sources) != 1 {
		t.Fatal("wire boundary dropped original source")
	}
	for _, field := range []string{"rawValue", "effectiveValue"} {
		got := epathRebuildWireReadScalar(t, actual.EnergyExplanation.Sources[0], field, "building")
		if got == nil || *got != 0 {
			t.Fatalf("actual v2 wire lost observed %s=0: %v", field, got)
		}
	}
	if !reflect.DeepEqual(native.EnergyExplanation.Sources[0], original) {
		t.Fatal("wire verification mutated native source proof")
	}
	legacyAfter, err := json.Marshal(legacy)
	if err != nil || !bytes.Equal(legacyBefore, legacyAfter) {
		t.Fatal("saved verification changed the frozen v1 writer/input")
	}
	var wire struct {
		Sources []map[string]json.RawMessage `json:"sources"`
	}
	if err := json.Unmarshal(legacyAfter, &wire); err != nil || len(wire.Sources) != 1 {
		t.Fatal("invalid frozen v1 fixture")
	}
	for _, field := range []string{"rawValue", "effectiveValue"} {
		if _, exists := wire.Sources[0][field]; exists {
			t.Fatalf("v2 known-zero emission leaked into v1 %s", field)
		}
	}
}

func TestEnergyPathRealRebuildWireDoesNotInventSourceZeros(t *testing.T) {
	for _, tc := range []struct {
		name   string
		proof  uint8
		stored string
		want   uint8
	}{
		{"native unknown", 0, "", 0},
		{"native raw only", energySourceObservedRaw, "", 1},
		{"native effective only", energySourceObservedEffective, "", 2},
		{"native both known", energySourceObservedRaw | energySourceObservedEffective, "", 3},
		{"stored fields absent", 0, `{}`, 0},
		{"stored fields null", 0, `{"rawValue":null,"effectiveValue":null}`, 0},
		{"stored raw zero effective null", 0, `{"rawValue":0,"effectiveValue":null}`, 1},
		{"stored effective zero raw absent", 0, `{"effectiveValue":0}`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := EnergyDataSource{observedValuePresence: tc.proof}
			if tc.stored != "" {
				if err := json.Unmarshal([]byte(tc.stored), &source); err != nil {
					t.Fatal(err)
				}
			}
			source.ID, source.Name, source.KeyValue = "original", "Heating:Electricity", ""
			source.SourceType, source.IsMeter = "sql_report_data", true
			source.SourceUnit, source.Units, source.NormalizedUnit = "J", "J", "kWh"
			source.ReportingFrequency = "Monthly"
			// Building observed proof must never become an unknown Zone's proof.
			source.ScopeDetails = []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"},
				EffectiveMultiplier: 1, MultiplierApplication: energyMultiplierAlreadyModelTotal, inspectorScopedValuePresence: true}}
			bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema,
				Scope: EnergyExplanationScope{Kind: "building"}, Quality: &EnergyPathQuality{}, Sources: []EnergyDataSource{source}}}
			for pass := 0; pass < 2; pass++ {
				var err error
				bundle, err = epathRealRebuiltBundleForOracle(bundle)
				if err != nil {
					t.Fatal(err)
				}
				actual := bundle.EnergyExplanation.Sources[0]
				for index, field := range []string{"rawValue", "effectiveValue"} {
					got := epathRebuildWireReadScalar(t, actual, field, "building")
					if (got != nil) != (tc.want&(1<<index) != 0) || got != nil && *got != 0 {
						t.Errorf("pass%d %s presence/value=%v, want bit%d", pass, field, got, tc.want)
					}
					if got := epathRebuildWireReadScalar(t, actual, field, "zone"); got != nil {
						t.Errorf("Building proof fabricated Zone %s=0", field)
					}
				}
			}
		})
	}
}

func TestEnergyPathRealRebuildWireUsesStrictOriginalBoundary(t *testing.T) {
	base := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema,
		Scope: EnergyExplanationScope{Kind: "building"}, Quality: &EnergyPathQuality{}}}
	for _, tc := range []struct {
		name, message string
		mutate        func(*PurposeResultBundle)
	}{
		{"legacy not upgraded", "original canonical v2", func(b *PurposeResultBundle) { b.EnergyExplanation.Schema = energyExplanationV1Schema }},
		{"missing quality not repaired", "quality", func(b *PurposeResultBundle) { b.EnergyExplanation.Quality = nil }},
		{"nonfinite not sanitized", "unsupported value", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes = []EnergyExplanationNode{{ID: "invalid", Value: math.NaN()}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundle := base
			tc.mutate(&bundle)
			if _, err := epathRealRebuiltBundleForOracle(bundle); err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("invalid rebuilt candidate bypassed strict boundary: %v", err)
			}
		})
	}
}
