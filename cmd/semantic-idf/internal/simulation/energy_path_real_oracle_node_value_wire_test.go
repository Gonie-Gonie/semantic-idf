package simulation

// Independent acceptance support. Acceptance-only raw transport validation, not a production
// compatibility reader or graph repair. EnergyExplanationNode.Value is a
// required json:"value" scalar (no omitempty), in every canonical graph.
import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Root calls this on the original energyExplanation JSON before typed
// unmarshalling. The node leaf tracks raw/effective/allocated presence, but not
// Value: Go would otherwise turn value:null or an absent value into numeric 0.
// Missing/null/empty node COLLECTIONS remain for the existing graph/coverage
// validator. Only actual node records impose this required scalar contract.
func epathValidateOracleOriginalNodeValues(data json.RawMessage) error {
	checkNodes := func(nodes []json.RawMessage, path string) error {
		for index, raw := range nodes {
			var node map[string]json.RawMessage
			if err := json.Unmarshal(raw, &node); err != nil {
				return fmt.Errorf("%s.nodes[%d] is not a node object: %w", path, index, err)
			}
			value := bytes.TrimSpace(node["value"])
			if len(value) == 0 || bytes.Equal(value, []byte("null")) {
				return fmt.Errorf("%s.nodes[%d].value is absent/null, not an explicit zero", path, index)
			}
			var number float64
			if err := json.Unmarshal(value, &number); err != nil || !epathOracleFinite(number) {
				return fmt.Errorf("%s.nodes[%d].value must be a finite JSON number", path, index)
			}
		}
		return nil
	}
	var checkScope func(json.RawMessage, string, bool) error
	checkScope = func(raw json.RawMessage, path string, includeZones bool) error {
		var scope struct {
			Nodes   []json.RawMessage `json:"nodes"`
			Periods []json.RawMessage `json:"periods"`
			Zones   []json.RawMessage `json:"zoneResults"`
		}
		if err := json.Unmarshal(raw, &scope); err != nil {
			return err
		}
		if err := checkNodes(scope.Nodes, path); err != nil {
			return err
		}
		for index, rawPeriod := range scope.Periods {
			var period struct {
				Nodes []json.RawMessage `json:"nodes"`
			}
			if err := json.Unmarshal(rawPeriod, &period); err != nil {
				return err
			}
			if err := checkNodes(period.Nodes, fmt.Sprintf("%s.periods[%d]", path, index)); err != nil {
				return err
			}
		}
		if includeZones {
			for index, rawZone := range scope.Zones {
				if err := checkScope(rawZone, fmt.Sprintf("%s.zoneResults[%d]", path, index), false); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return checkScope(data, "energyExplanation", true)
}
