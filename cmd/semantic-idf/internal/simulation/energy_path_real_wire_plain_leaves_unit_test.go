package simulation

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestEnergyPathRealOraclePlainLeavesPreserveOrdinaryOptOut(t *testing.T) {
	// Comparison with the existing unmarked leaf path is compatibility evidence
	// only; no production decoding is used by the independent helpers above.
	sourceWire := []byte(`{"id":"ordinary","sourceType":"sql_report_data","rawValue":1.23456789,"effectiveValue":0,"aggregationBasis":"representative_zone","effectiveMultiplier":3,"allocationApplied":true,"allocatedValue":7,"scopeDetails":[{"scope":{"kind":"zone","zoneName":"Office"},"rawValue":0,"effectiveValue":null,"allocatedValue":2,"allocationApplied":true}]}`)
	var ordinary EnergyDataSource
	if err := json.Unmarshal(sourceWire, &ordinary); err != nil {
		t.Fatal(err)
	}
	sources, err := epathDecodeOriginalOracleSources([]json.RawMessage{sourceWire})
	if err != nil || len(sources) != 1 || !reflect.DeepEqual(ordinary, sources[0]) {
		t.Fatalf("unmarked source snapshot changed: %v\nold=%#v\nnew=%#v", err, ordinary, sources)
	}
	nodeWire := []byte(`{"id":"ordinary","level":"end_use","zoneName":"Office","value":2.34567891,"rawValue":0,"effectiveValue":null,"allocatedValue":0}`)
	var oldNode EnergyExplanationNode
	if err := json.Unmarshal(nodeWire, &oldNode); err != nil {
		t.Fatal(err)
	}
	nodes, err := epathDecodeOriginalOracleNodes([]json.RawMessage{nodeWire})
	if err != nil || len(nodes) != 1 || !reflect.DeepEqual(oldNode, nodes[0]) {
		t.Fatalf("unmarked node snapshot changed: %v", err)
	}
	if sources[0].ScopeDetails[0].inspectorValuePresence != 1 || nodes[0].inspectorValuePresence != 5 {
		t.Fatal("source/node scalar presence changed")
	}
}

func TestEnergyPathRealOraclePlainLeavesPreserveMarkedRawSourceEvidence(t *testing.T) {
	for _, marker := range []string{`{"schema":"semantic-idf.native-electrical-observation/v1"}`, `null`, `17`, `{"schema":"future"}`} {
		raw := []byte(`{"id":"sql-rdd-2000","rawValue":1.23456789,"effectiveValue":-0.0004,"zoneName":"Office","aggregationBasis":"representative_zone","effectiveMultiplier":7,"multiplierApplication":"zone_multiplier","allocationApplied":true,"allocationFactor":0.5,"allocatedValue":8,"formula":"unsafe allocation","inputSourceIds":["foreign"],"relatedEntityIds":["component:999"],"scopeDetails":[{"scope":{"kind":"zone","zoneName":"Office"},"rawValue":0,"allocatedValue":3,"allocationApplied":true}],"nativeElectricalObservation":` + marker + `}`)
		before := append([]byte(nil), raw...)
		rows, err := epathDecodeOriginalOracleSources([]json.RawMessage{raw})
		if err != nil || len(rows) != 1 {
			t.Fatalf("raw source leaf: %v", err)
		}
		s := rows[0]
		if s.RawValue != 1.23456789 || s.EffectiveValue != -.0004 || s.ZoneName != "Office" || s.AggregationBasis != "representative_zone" || s.EffectiveMultiplier != 7 || s.MultiplierApplication != "zone_multiplier" ||
			!s.AllocationApplied || s.AllocationFactor != .5 || s.AllocatedValue != 8 || s.Formula != "unsafe allocation" || !reflect.DeepEqual(s.InputSourceIDs, []string{"foreign"}) || !reflect.DeepEqual(s.RelatedEntityIDs, []string{"component:999"}) ||
			len(s.ScopeDetails) != 1 || s.ScopeDetails[0].Scope.ZoneName != "Office" || s.ScopeDetails[0].AllocatedValue != 3 || s.ScopeDetails[0].inspectorValuePresence != 1 || s.inspectorValuePresence != 3 || s.pvObservationProtection != nil || !bytes.Equal(raw, before) {
			t.Fatalf("marked original source was repaired or gained metadata authority: %#v", s)
		}
	}
	for _, raw := range []string{`{"rawValue":0}`, `{"rawValue":null,"RAWVALUE":0}`, `{"rawValue":0,"RAWVALUE":null}`, `{"effectiveValue":0}`, `{"rawValue":null,"effectiveValue":null}`} {
		rows, err := epathDecodeOriginalOracleSources([]json.RawMessage{json.RawMessage(raw)})
		if err != nil {
			t.Fatal(err)
		}
		want := uint8(0)
		switch raw {
		case `{"rawValue":0}`, `{"rawValue":null,"RAWVALUE":0}`:
			want = 1
		case `{"effectiveValue":0}`:
			want = 2
		}
		if rows[0].inspectorValuePresence != want {
			t.Fatalf("JSON original presence order changed: %s -> %d want%d", raw, rows[0].inspectorValuePresence, want)
		}
	}
}

func TestEnergyPathRealOraclePlainLeafGluePreservesMarkedSourceAndAllNodeContexts(t *testing.T) {
	for _, context := range []string{"building", "building-period", "zone", "zone-period"} {
		t.Run(context, func(t *testing.T) {
			wire := epathOracleWireFixture()
			root := wire["energyExplanation"].(map[string]any)
			root["scope"] = map[string]any{"kind": "building"}
			root["sources"] = []any{map[string]any{"id": "marked", "rawValue": 1.23456789, "effectiveValue": 0, "effectiveMultiplier": 7, "aggregationBasis": "representative_zone", "zoneName": "Office", "allocatedValue": 8, "allocationApplied": true, "nativeElectricalObservation": map[string]any{"schema": "semantic-idf.native-electrical-observation/v1"}}}
			graph := epathOracleWireFixtureGraph(wire, context)
			graph["nodes"] = []any{map[string]any{"id": "unsafe-stored-node", "level": "end_use", "endUse": "storage_charge", "zoneName": "Office", "value": 1.23456789, "rawValue": 0, "effectiveValue": nil, "allocatedValue": 0, "storageChargeBoundaries": []any{map[string]any{"reason": "explicit denial"}}, "serviceBoundaryRestrictions": []any{map[string]any{"reason": "original restriction"}}}}
			data, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			bundle, err := epathDecodeOriginalOracleCandidate(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			sources := bundle.EnergyExplanation.Sources
			if len(sources) != 1 || sources[0].RawValue != 1.23456789 || sources[0].EffectiveValue != 0 || sources[0].inspectorValuePresence != 3 || sources[0].EffectiveMultiplier != 7 || sources[0].AggregationBasis != "representative_zone" || sources[0].ZoneName != "Office" || !sources[0].AllocationApplied || sources[0].AllocatedValue != 8 {
				t.Fatalf("top raw source override omitted: %#v", sources)
			}
			nodes, periods := bundle.EnergyExplanation.Nodes, bundle.EnergyExplanation.Periods
			if strings.HasPrefix(context, "zone") {
				nodes, periods = bundle.EnergyExplanation.ZoneResults[0].Nodes, bundle.EnergyExplanation.ZoneResults[0].Periods
			}
			if strings.HasSuffix(context, "period") {
				nodes = periods[0].Nodes
			}
			if len(nodes) != 1 || nodes[0].Level != "end_use" || nodes[0].ZoneName != "Office" || nodes[0].Value != 1.23456789 || nodes[0].inspectorValuePresence != 5 || len(nodes[0].storageChargeBoundaries) != 0 || len(nodes[0].serviceBoundaryRestrictions) != 0 {
				t.Fatalf("%s original node semantics repaired before oracle validation: %#v", context, nodes)
			}
		})
	}
}

func TestEnergyPathRealOraclePlainLeavesRejectInvalidTypedScalars(t *testing.T) {
	for _, input := range []string{`null`, `[]`, `17`, `{"rawValue":"not-a-number"}`, `{"effectiveValue":{}}`, `{"rawValue":1e999}`} {
		if _, err := epathDecodeOriginalOracleSources([]json.RawMessage{json.RawMessage(input)}); err == nil {
			t.Fatalf("invalid original source scalar accepted: %s", input)
		}
		if _, err := epathDecodeOriginalOracleNodes([]json.RawMessage{json.RawMessage(input)}); err == nil {
			t.Fatalf("invalid original node scalar accepted: %s", input)
		}
	}
	if _, err := epathDecodeOriginalOracleSources([]json.RawMessage{json.RawMessage(`{"scopeDetails":[null]}`)}); err == nil {
		t.Fatal("null source scope became a zero placeholder")
	}
}
