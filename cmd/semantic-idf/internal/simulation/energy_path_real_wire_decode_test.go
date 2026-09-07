package simulation

import (
	"encoding/json"
	"fmt"
	"io"
)

// Acceptance examines original runtime output, not a repaired stored result.
// These aliases suppress the application's result/period/summary compatibility
// readers. Leaf node/source readers only retain scalar presence and are kept.
type epathOraclePlainBundle PurposeResultBundle
type epathOraclePlainResult EnergyExplanationResult
type epathOraclePlainPeriod EnergyPeriod
type epathOraclePlainZone EnergyExplanationZoneResult
type epathOraclePlainSummary EnergyExplanationSummary

func epathDecodeOriginalOracleCandidate(input io.Reader) (PurposeResultBundle, error) {
	var plain epathOraclePlainBundle
	wire := struct {
		*epathOraclePlainBundle
		Result  json.RawMessage          `json:"energyExplanation"`
		Summary *epathOraclePlainSummary `json:"energyExplanationSummary"`
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
		plain.EnergyExplanationSummary = EnergyExplanationSummary(*wire.Summary)
	}
	return PurposeResultBundle(plain), nil
}

func epathDecodeOriginalOracleResult(data json.RawMessage) (EnergyExplanationResult, error) {
	var plain epathOraclePlainResult
	wire := struct {
		*epathOraclePlainResult
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
	plain.Periods, err = epathDecodeOriginalOraclePeriods(wire.Periods)
	if err != nil {
		return EnergyExplanationResult{}, err
	}
	for _, data := range wire.Zones {
		var zone epathOraclePlainZone
		zoneWire := struct {
			*epathOraclePlainZone
			Periods []json.RawMessage        `json:"periods"`
			Summary *epathOraclePlainSummary `json:"summary"`
		}{epathOraclePlainZone: &zone}
		if err := json.Unmarshal(data, &zoneWire); err != nil {
			return EnergyExplanationResult{}, err
		}
		zone.Periods, err = epathDecodeOriginalOraclePeriods(zoneWire.Periods)
		if err != nil {
			return EnergyExplanationResult{}, err
		}
		if zoneWire.Summary != nil {
			zone.Summary = EnergyExplanationSummary(*zoneWire.Summary)
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
			Summary *epathOraclePlainSummary `json:"summary"`
		}{epathOraclePlainPeriod: &period}
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, err
		}
		if wire.Summary != nil {
			summary := EnergyExplanationSummary(*wire.Summary)
			period.Summary = &summary
		}
		periods = append(periods, EnergyPeriod(period))
	}
	return periods, nil
}
