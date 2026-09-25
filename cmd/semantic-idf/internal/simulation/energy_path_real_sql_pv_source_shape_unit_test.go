package simulation

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func epathSQLPVShapeHandSource(family, frequency string) (epathRealSQLSource, map[string]any) {
	name, key, group, meter := "Electric Storage Charge Energy", "BATTERY", "System", false
	if family == "facility" {
		name, key, group, meter = "Electricity:Facility", "", "Facility:Electricity", true
	}
	if family == "cogeneration" {
		name, key, group, meter = "Cogeneration:Electricity", "", "Facility:Electricity:Cogeneration", true
	}
	native := epathRealSQLSource{DictionaryIndex: 2000, Name: name, KeyValue: key, IsMeter: meter, SourceUnit: "J", ReportingFrequency: frequency, IndexGroup: group, RawSum: epathOracleNumber(1.235 * 3600000), EnergyKWh: epathOracleNumber(1.235)}
	wire := map[string]any{"id": "sql-rdd-2000", "sourceType": "sql_report_data", "name": name, "keyValue": key, "isMeter": meter, "sourceUnit": "J", "units": "J", "normalizedUnit": "kWh", "reportingFrequency": frequency, "indexGroup": group, "aggregationMethod": "sum_report_data", "rawValue": 1.235, "effectiveValue": 1.235, "aggregationBasis": "model_total", "effectiveMultiplier": 1, "multiplierApplication": "already_model_total", "nativeElectricalObservation": map[string]any{"schema": "semantic-idf.native-electrical-observation/v1"}}
	return native, wire
}

func epathSQLPVShapeDecode(t *testing.T, source map[string]any) PurposeResultBundle {
	t.Helper()
	wire := map[string]any{"energyExplanation": map[string]any{"schema": energyExplanationSchema, "scope": map[string]any{"kind": "building"}, "sources": []any{source}}}
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), data...)
	bundle, err := epathDecodeOriginalOracleCandidate(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, original) {
		t.Fatal("original wire mutated during independent decode")
	}
	return bundle
}

// This hand lookup tests only the scalar+shape row consumer/decoder, NOT the
// complete40/80 registry. The separate frozen transactional SQL test proves it.
func epathSQLPVShapeHandCore(t *testing.T, native epathRealSQLSource) (epathSQLModelCheck, epathSQLPVValidatedSources) {
	t.Helper()
	specID := "storage.charge"
	if native.IsMeter {
		specID = "facility.demand"
	}
	identity := epathSQLPVSourceIdentity{Spec: epathSQLPVNativeSpec{ID: specID, Name: native.Name, Key: native.KeyValue, MetricGroup: "carriers", Sign: "nonnegative", IsMeter: native.IsMeter}, Source: native, NativeAnnualJ: 1.235 * 3600000, NativeAnnualKWh: 1.235, AggregationBasis: "model_total", EffectiveMultiplier: 1}
	check, err := epathSQLPVCanonicalSourceCheck("literal-hand-system", identity, "rawValue")
	if err != nil {
		t.Fatal(err)
	}
	prepared := epathSQLPVValidatedSources{canonical: map[string]epathSQLModelCheck{check.Want.Key: check}, identities: map[int]epathSQLPVValidatedIdentity{2000: {DictionaryIndex: 2000, Name: native.Name, Key: native.KeyValue, Frequency: native.ReportingFrequency, IndexGroup: native.IndexGroup, IsMeter: native.IsMeter, Quantity: *check.Quantity}}}
	return check, prepared
}

func TestEnergyPathRealSQLPVSourceShapeMarkedOriginalObservation(t *testing.T) {
	for _, family := range []string{"component", "facility", "cogeneration"} {
		for _, frequency := range []string{"Monthly", "Hourly"} {
			t.Run(family+"/"+frequency, func(t *testing.T) {
				native, wire := epathSQLPVShapeHandSource(family, frequency)
				bundle := epathSQLPVShapeDecode(t, wire)
				before := bundle.EnergyExplanation.Sources[0]
				if err := epathSQLCheckPVNativeSourceShape(bundle, native); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, bundle.EnergyExplanation.Sources[0]) {
					t.Fatal("shape validator repaired original source")
				}
				// Marker presence/absence cannot grant observation or allocation proof.
				delete(wire, "nativeElectricalObservation")
				if err := epathSQLCheckPVNativeSourceShape(epathSQLPVShapeDecode(t, wire), native); err != nil {
					t.Fatal(err)
				}
				if family != "cogeneration" {
					check, prepared := epathSQLPVShapeHandCore(t, native)
					if err := epathSQLCheckPVSourceConsumerWithShape(bundle, check, prepared); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestEnergyPathRealSQLPVSourceShapeRejectsMarkedAllocationOnlyMutations(t *testing.T) {
	cases := []struct {
		name   string
		change func(map[string]any)
	}{
		{"allocation flag only", func(s map[string]any) { s["allocationApplied"] = true }},
		{"allocated quantity only", func(s map[string]any) { s["allocatedValue"] = .001 }},
		{"allocation factor only", func(s map[string]any) { s["allocationFactor"] = .5 }},
		{"allocation explanation only", func(s map[string]any) { s["allocationExplanation"] = "Zone share" }},
		{"allocation formula only", func(s map[string]any) { s["allocationFormula"] = "native * Zone share" }},
		{"Zone only", func(s map[string]any) { s["zoneName"] = "Office" }},
		{"Zone scope only", func(s map[string]any) {
			s["scopeDetails"] = []any{map[string]any{"scope": map[string]any{"kind": "zone", "zoneName": "Office"}, "rawValue": 0}}
		}},
		{"Building scope restatement", func(s map[string]any) {
			s["scopeDetails"] = []any{map[string]any{"scope": map[string]any{"kind": "building"}, "rawValue": 1.235}}
		}},
		{"representative basis", func(s map[string]any) { s["aggregationBasis"] = "representative_zone" }},
		{"multiplier only", func(s map[string]any) { s["effectiveMultiplier"] = 7 }},
		{"multiplier method only", func(s map[string]any) { s["multiplierApplication"] = "zone_multiplier" }},
		{"derived input only", func(s map[string]any) { s["inputSourceIds"] = []string{"sql-rdd-9999"} }},
		{"derived formula only", func(s map[string]any) { s["formula"] = "other source" }},
		{"related owner only", func(s map[string]any) { s["relatedEntityIds"] = []string{"component:999"} }},
	}
	for _, family := range []string{"component", "facility", "cogeneration"} {
		for _, tc := range cases {
			t.Run(family+"/"+tc.name, func(t *testing.T) {
				native, wire := epathSQLPVShapeHandSource(family, "Monthly")
				tc.change(wire)
				bundle := epathSQLPVShapeDecode(t, wire)
				if err := epathSQLCheckPVNativeSourceShape(bundle, native); err == nil {
					t.Fatalf("marked original %s field was repaired or accepted", tc.name)
				}
				if family != "cogeneration" {
					check, prepared := epathSQLPVShapeHandCore(t, native)
					if err := epathSQLCheckPVSourceConsumerWithShape(bundle, check, prepared); err == nil {
						t.Fatal("core scalar consumer wrapper omitted mandatory shape")
					}
				}
			})
		}
	}
}

func TestEnergyPathRealSQLPVSourceShapeKeepsIndependentScalarAndIdentityGuards(t *testing.T) {
	native, wire := epathSQLPVShapeHandSource("component", "Monthly")
	check, prepared := epathSQLPVShapeHandCore(t, native)
	// Production marked-source reading would round this to1.235. Original
	// decoding must retain1.23456789 so the frozen scalar consumer rejects it.
	wire["rawValue"] = 1.23456789
	bundle := epathSQLPVShapeDecode(t, wire)
	if bundle.EnergyExplanation.Sources[0].RawValue != 1.23456789 {
		t.Fatal("marked raw precision repaired before scalar proof")
	}
	if err := epathSQLCheckPVSourceConsumerWithShape(bundle, check, prepared); err == nil {
		t.Fatal("marked rounding forgery passed scalar+shape wrapper")
	}
	native, wire = epathSQLPVShapeHandSource("component", "Monthly")
	wire["rawValue"] = nil
	if err := epathSQLCheckPVSourceConsumerWithShape(epathSQLPVShapeDecode(t, wire), check, prepared); err == nil {
		t.Fatal("shape replaced missing scalar proof")
	}
	_, wire = epathSQLPVShapeHandSource("component", "Monthly")
	bundle = epathSQLPVShapeDecode(t, wire)
	for _, change := range []func(*epathRealSQLSource){
		func(s *epathRealSQLSource) { s.DictionaryIndex = 0 }, func(s *epathRealSQLSource) { s.DictionaryIndex++ }, func(s *epathRealSQLSource) { s.KeyValue = "foreign" },
		func(s *epathRealSQLSource) { s.SourceUnit = "W" }, func(s *epathRealSQLSource) { s.ReportingFrequency = "Timestep" }, func(s *epathRealSQLSource) { s.IsMeter = true },
	} {
		bad := native
		change(&bad)
		if err := epathSQLCheckPVNativeSourceShape(bundle, bad); err == nil {
			t.Fatal("shape accepted wrong independent native identity")
		}
	}
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		bad := bundle
		bad.EnergyExplanation.Sources = append([]EnergyDataSource(nil), bundle.EnergyExplanation.Sources...)
		bad.EnergyExplanation.Sources[0].AllocationFactor = v
		if err := epathSQLCheckPVNativeSourceShape(bad, native); err == nil {
			t.Fatal("nonfinite allocation metadata accepted")
		}
	}
}
