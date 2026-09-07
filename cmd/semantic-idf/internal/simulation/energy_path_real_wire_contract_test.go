package simulation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
)

// Check the original wire before compatibility readers can turn an absent/null
// number into Go's zero value. This is an actual-candidate gate, not a change to
// the application's intentionally permissive saved-result compatibility API.
func epathValidateOracleCandidateWire(input io.Reader) error {
	var bundle struct {
		EnergyExplanation json.RawMessage `json:"energyExplanation"`
	}
	decoder := json.NewDecoder(input)
	if err := decoder.Decode(&bundle); err != nil {
		return fmt.Errorf("candidate wire: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("candidate wire: trailing JSON")
	}
	root, err := epathOracleWireObject(bundle.EnergyExplanation, "energyExplanation")
	if err != nil {
		return err
	}
	var schema string
	if err := json.Unmarshal(root["schema"], &schema); err != nil || schema != energyExplanationSchema {
		return fmt.Errorf("candidate wire: original canonical v2 schema required")
	}
	return epathValidateOracleWireGraph(root, "energyExplanation")
}

func epathOracleWireObject(raw json.RawMessage, path string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("candidate wire: %s requires a non-null object", path)
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("candidate wire: %s: %w", path, err)
	}
	return fields, nil
}

func epathOracleWireNumber(fields map[string]json.RawMessage, field, path string, required bool) error {
	raw, present := fields[field]
	if !present && !required {
		// Omission is retained as unknown here. Only the independently reviewed
		// selector may interpret an omitempty field in its own accounting policy.
		return nil
	}
	if !present || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("candidate wire: %s.%s is absent/null, not reported zero", path, field)
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("candidate wire: %s.%s requires a finite JSON number", path, field)
	}
	return nil
}

func epathOracleWireRows(fields map[string]json.RawMessage, field, path string, visit func(map[string]json.RawMessage, string) error) error {
	raw, present := fields[field]
	if !present {
		return nil // Canonical empty monthly graph collections use omitempty.
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return fmt.Errorf("candidate wire: %s.%s requires an array: %w", path, field, err)
	}
	for index, row := range rows {
		rowPath := fmt.Sprintf("%s.%s[%d]", path, field, index)
		object, err := epathOracleWireObject(row, rowPath)
		if err != nil {
			return err
		}
		if err := visit(object, rowPath); err != nil {
			return err
		}
	}
	return nil
}

func epathValidateOracleWireGraph(graph map[string]json.RawMessage, path string) error {
	for _, collection := range []struct {
		name     string
		required []string
		optional []string
	}{
		{"nodes", []string{"value"}, []string{"rawValue", "effectiveValue", "allocatedValue"}},
		{"links", []string{"fromValue", "toValue"}, []string{"ratio"}},
		{"reconciliation", []string{"expectedValue", "explainedValue", "residualValue"}, []string{"directValue", "allocatedValue", "unassignedValue", "overmappedValue"}},
		{"sources", nil, []string{"rawValue", "effectiveValue", "allocatedValue"}},
	} {
		err := epathOracleWireRows(graph, collection.name, path, func(row map[string]json.RawMessage, rowPath string) error {
			for _, field := range collection.required {
				if err := epathOracleWireNumber(row, field, rowPath, true); err != nil {
					return err
				}
			}
			for _, field := range collection.optional {
				if err := epathOracleWireNumber(row, field, rowPath, false); err != nil {
					return err
				}
			}
			if collection.name == "nodes" {
				return epathOracleWireRows(row, "loadBreakdown", rowPath, func(component map[string]json.RawMessage, componentPath string) error {
					return epathOracleWireNumber(component, "value", componentPath, true)
				})
			}
			if collection.name == "sources" {
				return epathOracleWireRows(row, "scopeDetails", rowPath, func(detail map[string]json.RawMessage, detailPath string) error {
					for _, field := range []string{"rawValue", "effectiveValue", "allocatedValue"} {
						if err := epathOracleWireNumber(detail, field, detailPath, false); err != nil {
							return err
						}
					}
					return nil
				})
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	qualityPath := path + ".quality"
	quality, err := epathOracleWireObject(graph["quality"], qualityPath)
	if err != nil {
		return err
	}
	for _, name := range []string{"drivers", "loads", "endUses", "carriers", "ratios"} {
		countPath := qualityPath + "." + name
		counts, err := epathOracleWireObject(quality[name], countPath)
		if err != nil {
			return err
		}
		for _, field := range []string{"found", "total"} {
			if err := epathOracleWireNumber(counts, field, countPath, true); err != nil {
				return err
			}
			var count int
			if err := json.Unmarshal(counts[field], &count); err != nil || count < 0 {
				return fmt.Errorf("candidate wire: %s.%s requires a nonnegative integer", countPath, field)
			}
		}
		var found, total int
		_ = json.Unmarshal(counts["found"], &found)
		_ = json.Unmarshal(counts["total"], &total)
		if found > total {
			return fmt.Errorf("candidate wire: %s.found exceeds its total (%d/%d); unknown requested counts must not become observed/0", countPath, found, total)
		}
	}
	for _, field := range []string{"driverToLoadClosedPct", "endUseToCarrierClosedPct", "zoneAllocatedPct", "unassignedPct"} {
		if err := epathOracleWireNumber(quality, field, qualityPath, true); err != nil {
			return err
		}
	}
	for _, field := range []string{"periods", "zoneResults"} {
		if err := epathOracleWireRows(graph, field, path, epathValidateOracleWireGraph); err != nil {
			return err
		}
	}
	return nil
}
