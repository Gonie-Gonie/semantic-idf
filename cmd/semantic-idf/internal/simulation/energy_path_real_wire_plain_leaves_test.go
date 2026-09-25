package simulation

// Independent original-wire leaf decoding. Production Node/Source UnmarshalJSON
// may sanitize explicit boundary/observation metadata; acceptance must inspect
// the original public fields first. Private production markers are not evidence.
import (
	"bytes"
	"encoding/json"
	"fmt"
)

type epathOraclePlainNode EnergyExplanationNode
type epathOraclePlainSource EnergyDataSource
type epathOraclePlainSourceScope EnergyDataSourceScopeDetail

func epathOracleOriginalScalarPresence(data []byte) (uint8, error) {
	// A struct retains encoding/json's exact/folded duplicate-key ordering. A
	// map indexed only by lower-case names could disagree with typed decoding.
	var fields struct {
		RawValue       json.RawMessage `json:"rawValue"`
		EffectiveValue json.RawMessage `json:"effectiveValue"`
		AllocatedValue json.RawMessage `json:"allocatedValue"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return 0, err
	}
	var presence uint8
	for i, raw := range []json.RawMessage{fields.RawValue, fields.EffectiveValue, fields.AllocatedValue} {
		if len(raw) != 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			presence |= 1 << i
		}
	}
	return presence, nil
}

func epathOracleOriginalObject(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("original oracle leaf must be an object, not a null/scalar placeholder")
	}
	return nil
}

func epathDecodeOriginalOracleNodes(rows []json.RawMessage) ([]EnergyExplanationNode, error) {
	if rows == nil {
		return nil, nil
	}
	out := make([]EnergyExplanationNode, len(rows))
	for i, raw := range rows {
		if err := epathOracleOriginalObject(raw); err != nil {
			return nil, err
		}
		var plain epathOraclePlainNode
		if err := json.Unmarshal(raw, &plain); err != nil {
			return nil, err
		}
		presence, err := epathOracleOriginalScalarPresence(raw)
		if err != nil {
			return nil, err
		}
		out[i] = EnergyExplanationNode(plain)
		out[i].inspectorDecodedFromJSON, out[i].inspectorValuePresence = true, presence
		// No production storageChargeBoundaries/serviceBoundaryRestrictions union
		// and no Level/Zone normalization. Original public semantics stay intact.
	}
	return out, nil
}

func epathDecodeOriginalOracleSourceScopes(rows []json.RawMessage) ([]EnergyDataSourceScopeDetail, error) {
	if rows == nil {
		return nil, nil
	}
	out := make([]EnergyDataSourceScopeDetail, len(rows))
	for i, raw := range rows {
		if err := epathOracleOriginalObject(raw); err != nil {
			return nil, err
		}
		var plain epathOraclePlainSourceScope
		if err := json.Unmarshal(raw, &plain); err != nil {
			return nil, err
		}
		presence, err := epathOracleOriginalScalarPresence(raw)
		if err != nil {
			return nil, err
		}
		out[i] = EnergyDataSourceScopeDetail(plain)
		out[i].inspectorDecodedFromJSON, out[i].inspectorValuePresence = true, presence&3
	}
	return out, nil
}

func epathDecodeOriginalOracleSources(rows []json.RawMessage) ([]EnergyDataSource, error) {
	if rows == nil {
		return nil, nil
	}
	out := make([]EnergyDataSource, len(rows))
	for i, raw := range rows {
		if err := epathOracleOriginalObject(raw); err != nil {
			return nil, err
		}
		var plain epathOraclePlainSource
		wire := struct {
			*epathOraclePlainSource
			ScopeDetails []json.RawMessage `json:"scopeDetails"`
		}{epathOraclePlainSource: &plain}
		// ScopeDetails is intercepted too, so no production leaf method runs.
		if err := json.Unmarshal(raw, &wire); err != nil {
			return nil, err
		}
		presence, err := epathOracleOriginalScalarPresence(raw)
		if err != nil {
			return nil, err
		}
		out[i] = EnergyDataSource(plain)
		out[i].inspectorDecodedFromJSON, out[i].inspectorValuePresence = true, presence&3
		out[i].ScopeDetails, err = epathDecodeOriginalOracleSourceScopes(wire.ScopeDetails)
		if err != nil {
			return nil, err
		}
		// nativeElectricalObservation is deliberately not decoded as a private
		// application marker. It grants no native identity/presence/flow proof,
		// and cannot round values or clear Zone/allocation/model-basis evidence.
	}
	return out, nil
}
