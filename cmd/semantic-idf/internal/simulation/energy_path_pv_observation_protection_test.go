package simulation

// Literal source-presence, persistence and graph-backfill regressions.
import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func pvProtectedLiteralSource(id string, value float64, presence uint8) EnergyDataSource {
	return protectEnergyPathPVSourceObservation(EnergyDataSource{
		ID: id, SourceType: "sql_report_data", Name: "Electric Storage Production Decrement Energy", KeyValue: "BATTERY",
		Units: "J", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly",
		RawValue: value, EffectiveValue: value, observedValuePresence: presence,
	})
}

func TestPVObservationProtectionDraftDoesNotGrantKnownness(t *testing.T) {
	unknown := pvProtectedLiteralSource("sql-rdd-10", 99, 0)
	if !energyPathPVSourceObservationProtected(unknown) || unknown.RawValue != 0 || unknown.EffectiveValue != 0 || energyPathPVProtectedSourceScalar(unknown, energySourceObservedRaw) != nil {
		t.Fatal("marker promoted a stale graph value to an observed scalar")
	}
	zero := pvProtectedLiteralSource("sql-rdd-11", 0, energySourceObservedRaw)
	if raw := energyPathPVProtectedSourceScalar(zero, energySourceObservedRaw); raw == nil || *raw != 0 {
		t.Fatal("actual raw zero was lost")
	}
	if energyPathPVProtectedSourceScalar(zero, energySourceObservedEffective) != nil || energyDataSourceValueKnown(zero, energySourceObservedEffective) {
		t.Fatal("raw knownness authorized effective zero")
	}
	legacy := EnergyDataSource{DriverRole: energyDriverSourceRoleContext, MultiplierApplication: "already_model_total", EffectiveMultiplier: 1}
	if energyPathPVSourceObservationProtected(legacy) {
		t.Fatal("generic context was silently opted into native protection")
	}
	bad := pvProtectedLiteralSource("sql-rdd-12", math.Inf(1), 3)
	if energyDataSourceValueKnown(bad, energySourceObservedRaw) || energyDataSourceValueKnown(bad, energySourceObservedEffective) {
		t.Fatal("nonfinite value remained observed")
	}
}

func pvProtectedAssertWireValues(t *testing.T, raw []byte) {
	t.Helper()
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("source roster=%d", len(rows))
	}
	for _, field := range []string{"rawValue", "effectiveValue"} {
		if string(rows[0][field]) != "0" || len(rows[1][field]) != 0 || string(rows[2][field]) != "-0.003" || len(rows[3][field]) != 0 {
			t.Fatalf("zero/unknown/signed/legacy %s contract changed: %s", field, raw)
		}
	}
	for index := range rows {
		if (len(rows[index]["nativeElectricalObservation"]) > 0) != (index < 3) {
			t.Fatal("marker lost or leaked to unrelated legacy source")
		}
	}
}

func pvProtectedLiteralRoster() []EnergyDataSource {
	return []EnergyDataSource{
		pvProtectedLiteralSource("sql-rdd-10", 0, 3),
		pvProtectedLiteralSource("sql-rdd-11", 0, 0),
		pvProtectedLiteralSource("sql-rdd-12", -.0034, 3),
		{ID: "legacy-zero", RawValue: 0, EffectiveValue: 0},
	}
}

func TestPVObservationProtectionDraftTwoV1JSONLoadsPreserveOnlyMarkedZeros(t *testing.T) {
	input := EnergyExplanationV1{Schema: energyExplanationV1Schema, Purpose: "basic_energy", Sources: pvProtectedLiteralRoster()}
	for round := 0; round < 2; round++ {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		pvProtectedAssertWireValues(t, fields["sources"])
		if err := json.Unmarshal(raw, &input); err != nil {
			t.Fatal(err)
		}
		if !energyDataSourceValueKnown(input.Sources[0], energySourceObservedRaw) || energyDataSourceValueKnown(input.Sources[1], energySourceObservedRaw) || energyDataSourceValueKnown(input.Sources[3], energySourceObservedRaw) {
			t.Fatal("V1 read fabricated/erased scalar presence")
		}
	}
}

func TestPVObservationProtectionDraftUnmarkedV1WriterBytesStayLegacy(t *testing.T) {
	input := EnergyExplanationV1{Schema: energyExplanationV1Schema, Purpose: "basic_energy", Frequency: "Monthly", Sources: []EnergyDataSource{{ID: "old", RawValue: 0, EffectiveValue: 0, observedValuePresence: 3}, {ID: "other", RawValue: 1.234567}}}
	type priorV1Writer EnergyExplanationV1
	want, err := json.Marshal(priorV1Writer(input))
	if err != nil {
		t.Fatal(err)
	}
	actual, err := json.Marshal(input)
	if err != nil || string(actual) != string(want) {
		t.Fatalf("unrelated legacy bytes changed: %v\n%s\n%s", err, actual, want)
	}
}

func TestPVObservationProtectionDraftTwoV2SourceWireLoads(t *testing.T) {
	sources := pvProtectedLiteralRoster()
	for round := 0; round < 2; round++ {
		raw, err := json.Marshal(energyPathSourcesForWire(sources))
		if err != nil {
			t.Fatal(err)
		}
		pvProtectedAssertWireValues(t, raw)
		if err := json.Unmarshal(raw, &sources); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPVObservationProtectionDraftMalformedPresentMarkerRetainsDenial(t *testing.T) {
	for _, marker := range []string{`null`, `false`, `[]`, `"legacy"`, `{}`, `{"schema":"future"}`, `{"schema":42}`, `{"schema":"semantic-idf.native-electrical-observation/v1","permission":"allocate"}`} {
		t.Run(marker, func(t *testing.T) {
			raw := []byte(`{"id":"sql-rdd-10","rawValue":0,"nativeElectricalObservation":` + marker + `}`)
			var source EnergyDataSource
			if err := json.Unmarshal(raw, &source); err != nil {
				t.Fatal(err)
			}
			for round := 0; round < 2; round++ {
				if !energyPathPVSourceObservationProtected(source) || !source.pvObservationProtection.Invalid || !energyDataSourceValueKnown(source, energySourceObservedRaw) || energyDataSourceValueKnown(source, energySourceObservedEffective) {
					t.Fatal("malformed explicit metadata reverted to legacy or granted knownness")
				}
				encoded, err := marshalEnergyPathPVProtectedSource(source)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(encoded, &source); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	if decodeEnergyPathPVObservationProtection(nil) != nil {
		t.Fatal("absent marker became explicit denial")
	}
}

func TestPVObservationProtectionDraftPresentationClearsOnlyGraphMetadata(t *testing.T) {
	index := 7
	source := pvProtectedLiteralSource("sql-rdd-10", -.0034, 3)
	source.ObjectIndex = &index
	source.HourlyEnergy = &EnergySourceHourlyEnergy{Unit: "kWh", Basis: "reported_source", Values: []float64{-.002, 0, -.002}}
	source.ZoneName, source.Formula, source.AllocationFormula = "wrong Zone", "borrowed", "allocated"
	source.AllocatedValue, source.AllocationFactor, source.AllocationApplied = 99, 7, true
	source.RelatedEntityIDs, source.InputSourceIDs = []string{"component:999"}, []string{"derived"}
	source.ScopeDetails = []EnergyDataSourceScopeDetail{{RawValue: 99, EffectiveValue: 99}}
	actual := energyPathPVSourceObservationPresentation(source)
	if actual.RawValue != -.003 || actual.EffectiveValue != -.003 || actual.ObjectIndex == nil || *actual.ObjectIndex != 7 || actual.Name != source.Name || actual.KeyValue != source.KeyValue || actual.NormalizedUnit != "kWh" {
		t.Fatal("presentation changed native identity or the observed scalar")
	}
	if actual.ZoneName != "" || actual.Formula != "" || actual.AllocationFormula != "" || actual.AllocatedValue != 0 || actual.AllocationFactor != 0 || actual.AllocationApplied || len(actual.ScopeDetails)+len(actual.RelatedEntityIDs)+len(actual.InputSourceIDs) != 0 {
		t.Fatal("source retained node-derived allocation/Zone metadata")
	}
	if !reflect.DeepEqual(actual.HourlyEnergy.Values, []float64{-.002, 0, -.002}) || actual.RawValue == actual.HourlyEnergy.Values[0]+actual.HourlyEnergy.Values[2] {
		t.Fatal("native scalar was reconstructed from rounded chart points")
	}
	*actual.ObjectIndex = 8
	actual.HourlyEnergy.Values[0] = 999
	actual.pvObservationProtection.Invalid = true
	if *source.ObjectIndex != 7 || source.HourlyEnergy.Values[0] != -.002 || source.pvObservationProtection.Invalid || len(source.ScopeDetails) != 1 {
		t.Fatal("presentation copy mutated caller/evidence")
	}
}

func TestPVObservationProtectionDraftGraphCannotBackfillOrInventZoneDetail(t *testing.T) {
	unknown := pvProtectedLiteralSource("sql-rdd-10", 0, 0)
	known := pvProtectedLiteralSource("sql-rdd-11", -7, 3)
	legacy := []EnergyExplanationNode{{ID: "stale", Level: "energy", Kind: "energy.storage_charge", Value: 99, RawValue: 99, SourceIDs: []string{unknown.ID}}}
	scope := EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}
	actual := filterEnergyDataSourcesForV2([]EnergyDataSource{unknown, known}, legacy, nil, nil, nil, nil, scope, PurposeAllocationPolicyByServicePathLoadShare)
	if len(actual) != 2 || actual[0].RawValue != 0 || actual[0].EffectiveValue != 0 || energyDataSourceValueKnown(actual[0], energySourceObservedRaw) || actual[1].RawValue != -7 || actual[1].AllocatedValue != 0 {
		t.Fatal("graph backfilled an unknown or assigned a native observation")
	}
	appendEnergyDataSourceScopeDetails(actual, actual, EnergyExplanationScope{Kind: "zone", ZoneName: "SPACE1", AggregationBasis: "model_total"})
	if len(actual[0].ScopeDetails)+len(actual[1].ScopeDetails) != 0 {
		t.Fatal("Building-native observations acquired invented Zone detail")
	}
}
