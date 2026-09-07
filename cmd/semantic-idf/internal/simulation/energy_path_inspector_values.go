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

// Source fields retain their original presence on read. Only the v2 result's
// source wrapper emits prepared zero values, leaving the frozen v1 writer alone.
func (source *EnergyDataSource) UnmarshalJSON(data []byte) error {
	type plainSource EnergyDataSource
	var decoded plainSource
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	presence, err := energyPathSourceValuePresence(data)
	if err != nil {
		return err
	}
	*source = EnergyDataSource(decoded)
	source.inspectorDecodedFromJSON, source.inspectorValuePresence = true, presence
	return nil
}

func (detail *EnergyDataSourceScopeDetail) UnmarshalJSON(data []byte) error {
	type plainDetail EnergyDataSourceScopeDetail
	var decoded plainDetail
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	presence, err := energyPathSourceValuePresence(data)
	if err != nil {
		return err
	}
	*detail = EnergyDataSourceScopeDetail(decoded)
	detail.inspectorDecodedFromJSON, detail.inspectorValuePresence = true, presence
	return nil
}

func energyPathSourceValuePresence(data []byte) (uint8, error) {
	var fields struct {
		RawValue       json.RawMessage `json:"rawValue"`
		EffectiveValue json.RawMessage `json:"effectiveValue"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return 0, err
	}
	var presence uint8
	for index, raw := range []json.RawMessage{fields.RawValue, fields.EffectiveValue} {
		if len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			presence |= 1 << index
		}
	}
	return presence, nil
}

type energyPathSourceWire struct{ EnergyDataSource }

func energyPathSourcesForWire(sources []EnergyDataSource) []energyPathSourceWire {
	out := make([]energyPathSourceWire, len(sources))
	for index, source := range sources {
		out[index] = energyPathSourceWire{source}
	}
	return out
}

func (wire energyPathSourceWire) MarshalJSON() ([]byte, error) {
	source := wire.EnergyDataSource
	if !energyDataSourceHasPreparedValues(source) {
		return json.Marshal(source)
	}
	type detailWire struct {
		EnergyDataSourceScopeDetail
		RawValue       *float64 `json:"rawValue,omitempty"`
		EffectiveValue *float64 `json:"effectiveValue,omitempty"`
	}
	details := make([]detailWire, len(source.ScopeDetails))
	for index, detail := range source.ScopeDetails {
		known := detail.MultiplierApplication != "" && detail.EffectiveMultiplier > 0
		details[index] = detailWire{detail,
			energyPathPreparedSourceValue(detail.RawValue, known, detail.inspectorDecodedFromJSON, detail.inspectorValuePresence, 1),
			energyPathPreparedSourceValue(detail.EffectiveValue, known, detail.inspectorDecodedFromJSON, detail.inspectorValuePresence, 2)}
	}
	return json.Marshal(struct {
		EnergyDataSource
		RawValue       *float64     `json:"rawValue,omitempty"`
		EffectiveValue *float64     `json:"effectiveValue,omitempty"`
		ScopeDetails   []detailWire `json:"scopeDetails,omitempty"`
	}{source,
		energyPathPreparedSourceValue(source.RawValue, true, source.inspectorDecodedFromJSON, source.inspectorValuePresence, 1),
		energyPathPreparedSourceValue(source.EffectiveValue, true, source.inspectorDecodedFromJSON, source.inspectorValuePresence, 2), details})
}

func energyPathPreparedSourceValue(value float64, known, decoded bool, presence, bit uint8) *float64 {
	if value != 0 || known && (!decoded || presence&bit != 0) {
		return &value
	}
	return nil
}
