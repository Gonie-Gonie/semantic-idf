package simulation

import (
	"encoding/json"
	"fmt"
	"io"
)

// Acceptance examines original runtime output, not a repaired stored result.
// These plain types suppress application compatibility readers. Raw Node and
// Source leaves are intercepted too: their production readers now repair some
// public fields, which would hide incorrect original evidence from this oracle.
type epathOraclePlainBundle PurposeResultBundle
type epathOraclePlainResult EnergyExplanationResult
type epathOraclePlainPeriod EnergyPeriod
type epathOraclePlainZone EnergyExplanationZoneResult
type epathOraclePlainSummary EnergyExplanationSummary

// Keep the original legacy collection separate from canonical Ratios. No
// application compatibility reader, recalculation or cleanup is invoked.
type epathOracleOriginalSummaryWire struct {
	epathOraclePlainSummary
	DerivedKPIs []EnergyExplanationSummaryItem `json:"derivedKpis"`
}

func (wire epathOracleOriginalSummaryWire) originalSummary() EnergyExplanationSummary {
	summary := EnergyExplanationSummary(wire.epathOraclePlainSummary)
	summary.DerivedKPIs = wire.DerivedKPIs
	return summary
}

func epathDecodeOriginalOracleCandidate(input io.Reader) (PurposeResultBundle, error) {
	var plain epathOraclePlainBundle
	wire := struct {
		*epathOraclePlainBundle
		Result  json.RawMessage                 `json:"energyExplanation"`
		Summary *epathOracleOriginalSummaryWire `json:"energyExplanationSummary"`
	}{epathOraclePlainBundle: &plain}
	decoder := json.NewDecoder(input)
	if err := decoder.Decode(&wire); err != nil {
		return PurposeResultBundle{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return PurposeResultBundle{}, fmt.Errorf("trailing candidate JSON")
	}
	result, err := epathDecodeOriginalOracleResult(wire.Result)
	if err != nil {
		return PurposeResultBundle{}, err
	}
	plain.EnergyExplanation = result
	if wire.Summary != nil {
		plain.EnergyExplanationSummary = wire.Summary.originalSummary()
	}
	return PurposeResultBundle(plain), nil
}

func epathDecodeOriginalOracleResult(data json.RawMessage) (EnergyExplanationResult, error) {
	if err := epathValidateOracleOriginalHourlyEnergy(data); err != nil {
		return EnergyExplanationResult{}, err
	}
	if err := epathValidateOracleOriginalNodeValues(data); err != nil {
		return EnergyExplanationResult{}, err
	}
	var plain epathOraclePlainResult
	wire := struct {
		*epathOraclePlainResult
		Sources []json.RawMessage `json:"sources"`
		Nodes   []json.RawMessage `json:"nodes"`
		Periods []json.RawMessage `json:"periods"`
		Zones   []json.RawMessage `json:"zoneResults"`
	}{epathOraclePlainResult: &plain}
	if err := json.Unmarshal(data, &wire); err != nil {
		return EnergyExplanationResult{}, err
	}
	if plain.Schema != energyExplanationSchema || plain.Scope.Kind != "building" || plain.Scope.ZoneName != "" {
		return EnergyExplanationResult{}, fmt.Errorf("snapshot must contain original Building canonical v2 result")
	}
	var err error
	plain.Sources, err = epathDecodeOriginalOracleSources(wire.Sources)
	if err != nil {
		return EnergyExplanationResult{}, err
	}
	plain.Nodes, err = epathDecodeOriginalOracleNodes(wire.Nodes)
	if err != nil {
		return EnergyExplanationResult{}, err
	}
	plain.Periods, err = epathDecodeOriginalOraclePeriods(wire.Periods)
	if err != nil {
		return EnergyExplanationResult{}, err
	}
	for _, data := range wire.Zones {
		var zone epathOraclePlainZone
		zoneWire := struct {
			*epathOraclePlainZone
			Nodes   []json.RawMessage               `json:"nodes"`
			Periods []json.RawMessage               `json:"periods"`
			Summary *epathOracleOriginalSummaryWire `json:"summary"`
		}{epathOraclePlainZone: &zone}
		if err := json.Unmarshal(data, &zoneWire); err != nil {
			return EnergyExplanationResult{}, err
		}
		zone.Nodes, err = epathDecodeOriginalOracleNodes(zoneWire.Nodes)
		if err != nil {
			return EnergyExplanationResult{}, err
		}
		zone.Periods, err = epathDecodeOriginalOraclePeriods(zoneWire.Periods)
		if err != nil {
			return EnergyExplanationResult{}, err
		}
		if zoneWire.Summary != nil {
			zone.Summary = zoneWire.Summary.originalSummary()
		}
		plain.ZoneResults = append(plain.ZoneResults, EnergyExplanationZoneResult(zone))
	}
	return EnergyExplanationResult(plain), nil
}

func epathDecodeOriginalOraclePeriods(rows []json.RawMessage) ([]EnergyPeriod, error) {
	var periods []EnergyPeriod
	for _, data := range rows {
		var period epathOraclePlainPeriod
		wire := struct {
			*epathOraclePlainPeriod
			Nodes   []json.RawMessage               `json:"nodes"`
			Summary *epathOracleOriginalSummaryWire `json:"summary"`
		}{epathOraclePlainPeriod: &period}
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, err
		}
		var err error
		period.Nodes, err = epathDecodeOriginalOracleNodes(wire.Nodes)
		if err != nil {
			return nil, err
		}
		if wire.Summary != nil {
			summary := wire.Summary.originalSummary()
			period.Summary = &summary
		}
		periods = append(periods, EnergyPeriod(period))
	}
	return periods, nil
}
