package simulation

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"math"
	"time"
)

const (
	energySourceObservedRaw uint8 = 1 << iota
	energySourceObservedEffective
)

// A source's scalar presence is independent of graph allocation and of its
// other scalar. Explicit stored zeros survive repeated reads; absent/null
// fields and unknown scope values do not acquire a reported-zero claim.
func energyDataSourceValueKnown(source EnergyDataSource, bit uint8) bool {
	return source.observedValuePresence&bit != 0 ||
		source.inspectorDecodedFromJSON && source.inspectorValuePresence&bit != 0
}

// This is an observed reporting axis, not an assumed annual calendar. A short
// run can contain three Monthly records; each source must actually report all
// three. EnergyPlus Monthly Time records omit WarmupFlag (NULL), unlike a
// timestep's explicit warmup marker. Missing schema cannot prove this axis.
func energyPathObservedMonthlyTimeAxis(db *sql.DB) map[int64]bool {
	rows, err := db.Query(`SELECT t.TimeIndex,t.Year,t.Month,t.Day,t."Interval",t.EnvironmentPeriodIndex
		FROM "Time" t JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex
		WHERE e.EnvironmentType=3 AND t.IntervalType=3 AND (t.WarmupFlag IS NULL OR t.WarmupFlag=0)`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	axis := map[int64]bool{}
	var environment int64
	for rows.Next() {
		var index, year, month, day, env sql.NullInt64
		var minutes sql.NullFloat64
		if err := rows.Scan(&index, &year, &month, &day, &minutes, &env); err != nil {
			return nil
		}
		if !index.Valid || index.Int64 <= 0 || !year.Valid || year.Int64 <= 0 || !month.Valid || month.Int64 < 1 || month.Int64 > 12 || !day.Valid || !env.Valid || env.Int64 <= 0 ||
			!minutes.Valid || minutes.Float64 <= 0 || math.IsNaN(minutes.Float64) || math.IsInf(minutes.Float64, 0) {
			return nil
		}
		last := time.Date(int(year.Int64), time.Month(month.Int64)+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if day.Int64 < 1 || day.Int64 > int64(last) || axis[index.Int64] || environment != 0 && environment != env.Int64 {
			return nil
		}
		environment = env.Int64
		axis[index.Int64] = true
	}
	if rows.Err() != nil || len(axis) == 0 {
		return nil
	}
	return axis
}

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
	prepared := energyDataSourceHasPreparedValues(source)
	type detailWire struct {
		EnergyDataSourceScopeDetail
		RawValue       *float64 `json:"rawValue,omitempty"`
		EffectiveValue *float64 `json:"effectiveValue,omitempty"`
	}
	details := make([]detailWire, len(source.ScopeDetails))
	for index, detail := range source.ScopeDetails {
		known := prepared && detail.MultiplierApplication != "" && detail.EffectiveMultiplier > 0
		exactPresence := detail.inspectorDecodedFromJSON || detail.inspectorScopedValuePresence
		details[index] = detailWire{detail,
			energyPathPreparedSourceValue(detail.RawValue, known || detail.inspectorValuePresence&1 != 0, exactPresence, detail.inspectorValuePresence, 1),
			energyPathPreparedSourceValue(detail.EffectiveValue, known || detail.inspectorValuePresence&2 != 0, exactPresence, detail.inspectorValuePresence, 2)}
	}
	value := func(number float64, bit uint8) *float64 {
		if energyDataSourceValueKnown(source, bit) {
			return &number
		}
		return energyPathPreparedSourceValue(number, prepared, source.inspectorDecodedFromJSON, source.inspectorValuePresence, bit)
	}
	return json.Marshal(struct {
		EnergyDataSource
		RawValue       *float64     `json:"rawValue,omitempty"`
		EffectiveValue *float64     `json:"effectiveValue,omitempty"`
		ScopeDetails   []detailWire `json:"scopeDetails,omitempty"`
	}{source,
		value(source.RawValue, energySourceObservedRaw),
		value(source.EffectiveValue, energySourceObservedEffective), details})
}

func energyPathPreparedSourceValue(value float64, known, decoded bool, presence, bit uint8) *float64 {
	if value != 0 || known && (!decoded || presence&bit != 0) {
		return &value
	}
	return nil
}
