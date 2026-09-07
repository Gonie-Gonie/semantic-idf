package simulation

import (
	"bytes"
	"encoding/json"
)

// Allocated driver algorithms know all three quantities, including the zero
// pressure of an Other/storage contribution. Stored sparse nodes do not carry
// that guarantee: retain field presence instead of manufacturing reported zero.
func (node EnergyExplanationNode) MarshalJSON() ([]byte, error) {
	type plainNode EnergyExplanationNode
	if node.Level != "driver" || !node.AllocationApplied {
		return json.Marshal(plainNode(node))
	}
	value := func(number float64, bit uint8) *float64 {
		if number != 0 || !node.inspectorDecodedFromJSON || node.inspectorValuePresence&bit != 0 {
			return &number
		}
		return nil
	}
	return json.Marshal(struct {
		plainNode
		RawValue       *float64 `json:"rawValue,omitempty"`
		EffectiveValue *float64 `json:"effectiveValue,omitempty"`
		AllocatedValue *float64 `json:"allocatedValue,omitempty"`
	}{plainNode(node), value(node.RawValue, 1), value(node.EffectiveValue, 2), value(node.AllocatedValue, 4)})
}

func (node *EnergyExplanationNode) UnmarshalJSON(data []byte) error {
	type plainNode EnergyExplanationNode
	var decoded plainNode
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields struct {
		RawValue       json.RawMessage `json:"rawValue"`
		EffectiveValue json.RawMessage `json:"effectiveValue"`
		AllocatedValue json.RawMessage `json:"allocatedValue"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*node = EnergyExplanationNode(decoded)
	node.inspectorDecodedFromJSON = true
	for index, raw := range []json.RawMessage{fields.RawValue, fields.EffectiveValue, fields.AllocatedValue} {
		if len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			node.inspectorValuePresence |= 1 << index
		}
	}
	return nil
}
