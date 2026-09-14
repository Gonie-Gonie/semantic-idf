package simulation

// Combine source presence and observation metadata in one scan; the typed
// source decode remains first and unchanged.
import (
	"bytes"
	"encoding/json"
)

func energyPathSourceObservationFields(data []byte) (uint8, json.RawMessage, error) {
	var fields struct {
		RawValue                    json.RawMessage `json:"rawValue"`
		EffectiveValue              json.RawMessage `json:"effectiveValue"`
		NativeElectricalObservation json.RawMessage `json:"nativeElectricalObservation"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return 0, nil, err
	}
	var presence uint8
	for index, raw := range []json.RawMessage{fields.RawValue, fields.EffectiveValue} {
		if len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			presence |= 1 << index
		}
	}
	return presence, fields.NativeElectricalObservation, nil
}
