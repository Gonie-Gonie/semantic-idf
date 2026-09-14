package simulation

import (
	"encoding/json"
	"reflect"
	"testing"
)

// This reference intentionally retains the two old independent metadata scans.
// It does not call the proposed combined helper.
func sourceObservationDecodeReviewFormerMetadata(data []byte) (uint8, json.RawMessage, error) {
	presence, err := energyPathSourceValuePresence(data)
	if err != nil {
		return 0, nil, err
	}
	var metadata struct {
		NativeElectricalObservation json.RawMessage `json:"nativeElectricalObservation"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return 0, nil, err
	}
	return presence, metadata.NativeElectricalObservation, nil
}

func TestSourceObservationDecodeReviewCombinedScanPreservesJSONSemantics(t *testing.T) {
	tests := []struct {
		name, data string
		presence   uint8
		protected  bool
	}{
		{"absent", `{}`, 0, false},
		{"ordinary non-PV zero and null", `{"rawValue":0,"effectiveValue":null,"hourlyEnergy":{"values":[0,1,null,-1]}}`, 1, false},
		{"known effective zero only", `{"effectiveValue":0}`, 2, false},
		{"case-folded metadata and presence", `{"RAWVALUE":0,"effectiveVALUE":2,"NATIVEELECTRICALOBSERVATION":{"schema":"semantic-idf.native-electrical-observation/v1"}}`, 3, true},
		{"escaped field names", `{"raw\u0056alue":0,"effective\u0056alue":null,"nativeElectrical\u004fbservation":{"schema":"semantic-idf.native-electrical-observation/v1"}}`, 1, true},
		{"explicit null marker", `{"rawValue":0,"nativeElectricalObservation":null}`, 1, true},
		{"wrong marker type", `{"nativeElectricalObservation":17}`, 0, true},
		{"future schema remains denial", `{"rawValue":0,"nativeElectricalObservation":{"schema":"future-v2"}}`, 1, true},
		{"malformed present legacy mode", `{"nativeElectricalObservation":{"schema":"semantic-idf.native-electrical-observation/v1","legacyContextOnly":"yes"}}`, 0, true},
		{"unknown marker field remains denial", `{"nativeElectricalObservation":{"schema":"semantic-idf.native-electrical-observation/v1","future":true}}`, 0, true},
		{"duplicate exact keys last null", `{"rawValue":0,"rawValue":null,"nativeElectricalObservation":{"schema":"semantic-idf.native-electrical-observation/v1"},"nativeElectricalObservation":null}`, 0, true},
		{"duplicate folded keys retain decoder ordering", `{"rawValue":null,"RAWVALUE":0,"nativeElectricalObservation":null,"NativeElectricalObservation":{"schema":"semantic-idf.native-electrical-observation/v1","legacyContextOnly":true}}`, 1, true},
		{"nested or quoted name is not a top-level marker", `{"explanation":"nativeElectricalObservation","other":{"nativeElectricalObservation":null}}`, 0, false},
		{"top-level null", `null`, 0, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := []byte(test.data)
			wantPresence, wantMarker, wantErr := sourceObservationDecodeReviewFormerMetadata(data)
			gotPresence, gotMarker, gotErr := energyPathSourceObservationFields(data)
			if wantErr != nil || gotErr != nil || gotPresence != test.presence || gotPresence != wantPresence || !reflect.DeepEqual(gotMarker, wantMarker) {
				t.Fatalf("combined metadata differs: got (%d,%s,%v), former (%d,%s,%v), literal presence %d", gotPresence, gotMarker, gotErr, wantPresence, wantMarker, wantErr, test.presence)
			}
			gotProtection := decodeEnergyPathPVObservationProtection(gotMarker)
			wantProtection := decodeEnergyPathPVObservationProtection(wantMarker)
			if (gotProtection != nil) != test.protected || !reflect.DeepEqual(gotProtection, wantProtection) {
				t.Fatalf("marker denial changed: got %#v, former %#v, want protected %v", gotProtection, wantProtection, test.protected)
			}
		})
	}
	for _, data := range []string{`{`, `[]`, `17`, `{"rawValue":0,}`, `{"nativeElectricalObservation":`} {
		_, _, formerErr := sourceObservationDecodeReviewFormerMetadata([]byte(data))
		_, _, combinedErr := energyPathSourceObservationFields([]byte(data))
		// Non-object helper inputs name different anonymous struct types in an
		// UnmarshalTypeError. The public source decoder rejects them in its
		// unchanged first typed pass; require the same error class here.
		if formerErr == nil || combinedErr == nil || reflect.TypeOf(formerErr) != reflect.TypeOf(combinedErr) {
			t.Fatalf("error semantics changed for %q: combined %v; former %v", data, combinedErr, formerErr)
		}
	}
}

// After the exact call-site replacement this remains a comparison against the
// former source reader, including typed errors, knownness and marker denial.
func sourceObservationDecodeReviewFormerSource(data []byte) (EnergyDataSource, error) {
	type plainSource EnergyDataSource
	var decoded plainSource
	if err := json.Unmarshal(data, &decoded); err != nil {
		return EnergyDataSource{}, err
	}
	presence, marker, err := sourceObservationDecodeReviewFormerMetadata(data)
	if err != nil {
		return EnergyDataSource{}, err
	}
	source := EnergyDataSource(decoded)
	source.inspectorDecodedFromJSON, source.inspectorValuePresence = true, presence
	source.pvObservationProtection = decodeEnergyPathPVObservationProtection(marker)
	if energyPathPVSourceObservationProtected(source) {
		source = energyPathPVSourceObservationPresentation(source)
	}
	return source, nil
}

func TestSourceObservationDecodeReviewSourceAndTwoV1ReloadsMatchFormerReader(t *testing.T) {
	tests := []string{
		`{"id":"ordinary","rawValue":0,"effectiveValue":null,"normalizedUnit":"kWh"}`,
		`{"id":"signed","RAWVALUE":-2.0004,"effectiveValue":0,"nativeElectrical\u004fbservation":{"schema":"semantic-idf.native-electrical-observation/v1"}}`,
		`{"id":"unknown","normalizedUnit":"kWh","nativeElectricalObservation":null}`,
		`{"id":"legacy","rawValue":0,"aggregationBasis":"representative_zone","effectiveMultiplier":7,"nativeElectricalObservation":{"schema":"semantic-idf.native-electrical-observation/v1","legacyContextOnly":true}}`,
		`{"id":"malformed-marker","nativeElectricalObservation":{"schema":"semantic-idf.native-electrical-observation/v1","legacyContextOnly":null}}`,
	}
	for _, input := range tests {
		want, err := sourceObservationDecodeReviewFormerSource([]byte(input))
		if err != nil {
			t.Fatal(err)
		}
		var got EnergyDataSource
		if err := json.Unmarshal([]byte(input), &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("source changed for %s: got %#v former %#v", input, got, want)
		}
		for pass := 0; pass < 2; pass++ {
			encoded, err := json.Marshal(EnergyExplanationV1{Sources: []EnergyDataSource{got}})
			if err != nil {
				t.Fatal(err)
			}
			var literal struct {
				Sources []json.RawMessage `json:"sources"`
			}
			if err := json.Unmarshal(encoded, &literal); err != nil {
				t.Fatal(err)
			}
			if len(literal.Sources) != 1 {
				t.Fatalf("source lost in pass %d", pass)
			}
			want, err = sourceObservationDecodeReviewFormerSource(literal.Sources[0])
			if err != nil {
				t.Fatal(err)
			}
			var next EnergyExplanationV1
			if err := json.Unmarshal(encoded, &next); err != nil {
				t.Fatal(err)
			}
			if len(next.Sources) != 1 || !reflect.DeepEqual(next.Sources[0], want) {
				t.Fatalf("V1 source differs from former reader in pass %d", pass)
			}
			got = next.Sources[0]
		}
	}
	for _, input := range []string{`{"rawValue":"not-a-number"}`, `{"effectiveValue":{}}`, `{"nativeElectricalObservation":`} {
		_, wantErr := sourceObservationDecodeReviewFormerSource([]byte(input))
		var got EnergyDataSource
		gotErr := json.Unmarshal([]byte(input), &got)
		if wantErr == nil || gotErr == nil || wantErr.Error() != gotErr.Error() {
			t.Fatalf("typed source error changed: got %v former %v", gotErr, wantErr)
		}
	}
}
