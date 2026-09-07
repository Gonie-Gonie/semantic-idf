package simulation

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEnergyPathRealOracleOriginalDecoderDoesNotRepairGraphs(t *testing.T) {
	for _, context := range []string{"building", "building-period", "zone", "zone-period"} {
		t.Run(context, func(t *testing.T) {
			wire := epathOracleWireFixture()
			root := wire["energyExplanation"].(map[string]any)
			root["scope"] = map[string]any{"kind": "building", "aggregationBasis": "model_total"}
			graph := epathOracleWireFixtureGraph(wire, context)
			graph["nodes"] = []any{map[string]any{
				"id": "original-node", "level": "load", "scaleDomain": "thermal", "value": 1.23456789, "unit": "J", "rawValue": 0,
			}}
			graph["links"] = []any{map[string]any{
				"id": "original-bad-link", "fromId": "original-node", "toId": "missing-endpoint", "relation": "load_to_end_use",
				"fromValue": -2.3456789, "toValue": 4.56789123, "fromUnit": "J", "toUnit": "kWh", "ratio": 999,
			}}
			graph["reconciliation"] = []any{
				map[string]any{"id": "duplicate", "expectedValue": 1.23456789, "explainedValue": 0, "residualValue": 1.23456789},
				map[string]any{"id": "duplicate", "expectedValue": 9.87654321, "explainedValue": 0, "residualValue": 9.87654321},
			}
			data, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			original := append([]byte(nil), data...)
			bundle, err := epathDecodeOriginalOracleCandidate(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			result := bundle.EnergyExplanation
			nodes, links, rows := result.Nodes, result.Links, result.Reconciliation
			periods := result.Periods
			if strings.HasPrefix(context, "zone") {
				zone := result.ZoneResults[0]
				nodes, links, rows, periods = zone.Nodes, zone.Links, zone.Reconciliation, zone.Periods
			}
			if strings.HasSuffix(context, "period") {
				nodes, links, rows = periods[0].Nodes, periods[0].Links, periods[0].Reconciliation
			}
			if len(nodes) != 1 || nodes[0].Value != 1.23456789 || nodes[0].Unit != "J" || !nodes[0].inspectorDecodedFromJSON || nodes[0].inspectorValuePresence&1 == 0 || nodes[0].inspectorValuePresence&2 != 0 {
				t.Fatalf("original node precision/unit/presence repaired: %#v", nodes)
			}
			if len(links) != 1 || links[0].ToID != "missing-endpoint" || links[0].FromValue != -2.3456789 || links[0].ToValue != 4.56789123 || links[0].Ratio != 999 {
				t.Fatalf("invalid original link removed or repaired: %#v", links)
			}
			if len(rows) != 2 || rows[0].ID != rows[1].ID || rows[0].ExpectedValue != 1.23456789 || rows[1].ExpectedValue != 9.87654321 {
				t.Fatalf("original reconciliation repaired: %#v", rows)
			}
			if err := epathValidateOracleGraphRecords(nodes, links, rows, "building", "", "annual"); err == nil {
				t.Fatal("original invalid graph escaped independent rejection")
			}
			if result.sanitizedOnRead || result.upgradedFromV1 || !bytes.Equal(original, data) {
				t.Fatal("compatibility transform or input mutation reached original candidate reader")
			}
		})
	}
}

func TestEnergyPathRealOracleOriginalDecoderRejectsSubstitutedContext(t *testing.T) {
	for _, mutation := range []func(map[string]any){
		func(root map[string]any) { root["schema"] = energyExplanationV1Schema },
		func(root map[string]any) { delete(root, "schema") },
		func(root map[string]any) { root["scope"] = map[string]any{"kind": "zone", "zoneName": "Office"} },
		func(root map[string]any) { root["scope"] = map[string]any{"kind": "building", "zoneName": "Office"} },
		func(root map[string]any) { root["scope"] = map[string]any{"kind": "BUILDING"} },
	} {
		wire := epathOracleWireFixture()
		root := wire["energyExplanation"].(map[string]any)
		root["scope"] = map[string]any{"kind": "building"}
		mutation(root)
		data, _ := json.Marshal(wire)
		if _, err := epathDecodeOriginalOracleCandidate(bytes.NewReader(data)); err == nil {
			t.Fatal("noncanonical original context was silently upgraded/normalized")
		}
	}
}

func TestEnergyPathRealOracleOriginalDecoderRetainsSourcesAndSummaries(t *testing.T) {
	wire := epathOracleWireFixture()
	root := wire["energyExplanation"].(map[string]any)
	root["scope"] = map[string]any{"kind": "building"}
	root["sources"] = []any{map[string]any{
		"id": "original", "name": "original source", "rawValue": 0, "normalizedUnit": "J",
		"scopeDetails": []any{map[string]any{"scope": map[string]any{"kind": "zone", "zoneName": "Office"}, "rawValue": 0}},
	}}
	summary := map[string]any{
		"schema": "unreviewed-original-schema", "period": "unreviewed-original-period",
		"drivers": []any{map[string]any{"value": 1.23456789, "unit": "J"}},
	}
	wire["energyExplanationSummary"] = summary
	epathOracleWireFixtureGraph(wire, "building-period")["summary"] = summary
	epathOracleWireFixtureGraph(wire, "zone")["summary"] = summary
	epathOracleWireFixtureGraph(wire, "zone-period")["summary"] = summary
	data, _ := json.Marshal(wire)
	bundle, err := epathDecodeOriginalOracleCandidate(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	result := bundle.EnergyExplanation
	if len(result.Sources) != 1 || len(result.Sources[0].ScopeDetails) != 1 {
		t.Fatal("original source ownership records lost")
	}
	source := result.Sources[0]
	detail := source.ScopeDetails[0]
	if source.RawValue != 0 || source.NormalizedUnit != "J" || !source.inspectorDecodedFromJSON || source.inspectorValuePresence != 1 ||
		detail.RawValue != 0 || detail.Scope.ZoneName != "Office" || !detail.inspectorDecodedFromJSON || detail.inspectorValuePresence != 1 {
		t.Fatalf("raw reported-zero/effective unknown distinction changed: source=%#v detail=%#v", source, detail)
	}
	if result.Periods[0].Summary == nil || result.ZoneResults[0].Periods[0].Summary == nil {
		t.Fatal("original nested summaries lost")
	}
	for _, actual := range []EnergyExplanationSummary{
		bundle.EnergyExplanationSummary, *result.Periods[0].Summary,
		result.ZoneResults[0].Summary, *result.ZoneResults[0].Periods[0].Summary,
	} {
		if actual.Schema != "unreviewed-original-schema" || actual.Period != "unreviewed-original-period" || len(actual.Drivers) != 1 || actual.Drivers[0].Value != 1.23456789 || actual.Drivers[0].Unit != "J" {
			t.Fatalf("original summary repaired or recalculated: %#v", actual)
		}
	}
}
