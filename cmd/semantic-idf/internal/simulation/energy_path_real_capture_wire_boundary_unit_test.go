package simulation

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func epathCaptureWireFixture(source EnergyDataSource) epathRealRunEvidence {
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{
		Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"},
		Quality: &EnergyPathQuality{}, Sources: []EnergyDataSource{source},
	}}
	return epathRealRunEvidence{
		Fixture: epathRealFixture{ID: "capture-wire-unit"}, Version: "25.1", RunDirectory: "unchanged-capture",
		SQLSHA256: strings.Repeat("1", 64), ResultSHA256: strings.Repeat("2", 64), ManifestSHA256: strings.Repeat("3", 64),
		Run: &SimulationRunResult{Status: "succeeded", RunID: "captured-run", PurposeResults: &bundle}, Bundle: bundle,
	}
}

func epathCaptureWireSource() EnergyDataSource {
	// Real-shaped PLENUM Interzone Monthly rate: not a main-flow node, but its
	// zero is observed independently. The native-only oracle cannot infer it.
	return EnergyDataSource{ID: "sql-rdd-784", Name: "Zone Air Heat Balance Interzone Air Transfer Rate", KeyValue: "PLENUM-1",
		SourceType: "sql_report_data", ReportingFrequency: "Monthly", Units: "W", SourceUnit: "W", NormalizedUnit: "kWh",
		DriverRole: "reconciliation", DriverCategory: "air.interzone", EffectiveMultiplier: 1,
		MultiplierApplication: energyMultiplierRequiresZone, observedValuePresence: energySourceObservedRaw | energySourceObservedEffective}
}

func TestEnergyPathRealCaptureWirePreservesObservedReconciliationZero(t *testing.T) {
	source := epathCaptureWireSource()
	evidence := epathCaptureWireFixture(source)
	if got := epathRebuildWireReadScalar(t, source, "effectiveValue", "building"); got != nil {
		t.Fatal("fixture no longer reproduces native reconciliation-zero boundary")
	}
	before, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	// The normal capture artifact already contains both zero scalars before
	// independent validation; no reader/writer patch is needed to produce them.
	var recorded struct {
		Bundle struct {
			Explanation struct {
				Sources []map[string]json.RawMessage `json:"sources"`
			} `json:"energyExplanation"`
		} `json:"bundle"`
	}
	if err := json.Unmarshal(before, &recorded); err != nil {
		t.Fatal(err)
	}
	fields := recorded.Bundle.Explanation.Sources[0]
	if string(fields["rawValue"]) != "0" || string(fields["effectiveValue"]) != "0" {
		t.Fatal("fixture's untouched capture wire did not contain observed zeros")
	}
	actual, err := epathRealCaptureOracleEvidence(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if len(actual.Bundle.EnergyExplanation.Sources) != 1 {
		t.Fatal("verification dropped source")
	}
	for _, field := range []string{"rawValue", "effectiveValue"} {
		value := epathRebuildWireReadScalar(t, actual.Bundle.EnergyExplanation.Sources[0], field, "building")
		if value == nil || *value != 0 {
			t.Fatalf("strict original-wire %s not observed zero: %v", field, value)
		}
	}
	if !actual.Bundle.EnergyExplanation.Sources[0].inspectorDecodedFromJSON || evidence.Bundle.EnergyExplanation.Sources[0].inspectorDecodedFromJSON ||
		!reflect.DeepEqual(evidence.Bundle.EnergyExplanation.Sources[0], source) {
		t.Fatal("verification did not isolate decoded presence from the native source")
	}
	if actual.Run != evidence.Run || actual.Run.PurposeResults != evidence.Run.PurposeResults {
		t.Fatal("verification replaced the captured RunResult or its native bundle")
	}
	copyWithoutBundle := actual
	copyWithoutBundle.Bundle = evidence.Bundle
	if !reflect.DeepEqual(copyWithoutBundle, evidence) {
		t.Fatal("verification changed capture paths/provenance/status instead of only its oracle bundle")
	}
	after, err := json.Marshal(evidence)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("verification changed native capture/result bytes or recorded hashes")
	}
	// A later consumer's graph/source mutation cannot change the stored native
	// bundle through a shared slice left behind by a shallow conversion.
	actual.Bundle.EnergyExplanation.Sources[0].Name = "verification-only mutation"
	if evidence.Bundle.EnergyExplanation.Sources[0].Name != source.Name || evidence.Run.PurposeResults.EnergyExplanation.Sources[0].Name != source.Name {
		t.Fatal("verification source shares backing storage with native captured result")
	}
}

func TestEnergyPathRealCaptureWireUnknownNullAndZoneProofRemainUnknown(t *testing.T) {
	for _, tc := range []struct {
		name, raw   string
		proof, want uint8
	}{
		{"native absent", "", 0, 0},
		{"native raw only", "", energySourceObservedRaw, 1},
		{"native both known", "", energySourceObservedRaw | energySourceObservedEffective, 3},
		{"decoded absent", `{}`, 0, 0},
		{"decoded null", `{"rawValue":null,"effectiveValue":null}`, 0, 0},
		{"decoded raw zero effective null", `{"rawValue":0,"effectiveValue":null}`, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := EnergyDataSource{observedValuePresence: tc.proof}
			if tc.raw != "" {
				if err := json.Unmarshal([]byte(tc.raw), &source); err != nil {
					t.Fatal(err)
				}
			}
			source.ID, source.Name, source.KeyValue = "nullable-original", "Heating:Electricity", ""
			source.SourceType, source.IsMeter, source.ReportingFrequency = "sql_report_data", true, "Monthly"
			source.Units, source.SourceUnit, source.NormalizedUnit = "J", "J", "kWh"
			source.ScopeDetails = []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"},
				MultiplierApplication: energyMultiplierAlreadyModelTotal, EffectiveMultiplier: 1, inspectorScopedValuePresence: true}}
			actual, err := epathRealCaptureOracleEvidence(epathCaptureWireFixture(source))
			if err != nil {
				t.Fatal(err)
			}
			decoded := actual.Bundle.EnergyExplanation.Sources[0]
			for index, field := range []string{"rawValue", "effectiveValue"} {
				got := epathRebuildWireReadScalar(t, decoded, field, "building")
				if (got != nil) != (tc.want&(1<<index) != 0) || got != nil && *got != 0 {
					t.Errorf("capture boundary changed %s presence: %v", field, got)
				}
				if got := epathRebuildWireReadScalar(t, decoded, field, "zone"); got != nil {
					t.Errorf("Building observed proof manufactured unknown Zone %s", field)
				}
			}
		})
	}
}

func TestEnergyPathRealCaptureWireCannotRepairInvalidCandidate(t *testing.T) {
	for _, tc := range []struct {
		name, message string
		mutate        func(*PurposeResultBundle)
	}{
		{"legacy", "original canonical v2", func(bundle *PurposeResultBundle) { bundle.EnergyExplanation.Schema = energyExplanationV1Schema }},
		{"missing quality", "quality", func(bundle *PurposeResultBundle) { bundle.EnergyExplanation.Quality = nil }},
		{"nonfinite", "unsupported value", func(bundle *PurposeResultBundle) {
			bundle.EnergyExplanation.Nodes = []EnergyExplanationNode{{ID: "nonfinite", Value: math.NaN()}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evidence := epathCaptureWireFixture(epathCaptureWireSource())
			tc.mutate(&evidence.Bundle)
			if _, err := epathRealCaptureOracleEvidence(evidence); err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("invalid captured candidate was repaired or accepted: %v", err)
			}
		})
	}
}
