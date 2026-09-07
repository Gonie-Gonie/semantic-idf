package simulation

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func epathOracleWireFixture() map[string]any {
	graph := func() map[string]any {
		quality := map[string]any{"driverToLoadClosedPct": 0, "endUseToCarrierClosedPct": 0, "zoneAllocatedPct": 0, "unassignedPct": 0}
		for _, name := range []string{"drivers", "loads", "endUses", "carriers", "ratios"} {
			quality[name] = map[string]any{"found": 0, "total": 0}
		}
		return map[string]any{
			"nodes":          []any{map[string]any{"value": 0, "loadBreakdown": []any{map[string]any{"value": 0}}}},
			"links":          []any{map[string]any{"fromValue": 0, "toValue": 0}},
			"reconciliation": []any{map[string]any{"expectedValue": 0, "explainedValue": 0, "residualValue": 0}},
			"sources":        []any{map[string]any{}},
			"quality":        quality,
		}
	}
	root, zone := graph(), graph()
	root["schema"] = energyExplanationSchema
	root["periods"] = []any{graph()}
	zone["periods"] = []any{graph()}
	root["zoneResults"] = []any{zone}
	return map[string]any{"energyExplanation": root}
}

func epathOracleWireFixtureGraph(bundle map[string]any, context string) map[string]any {
	graph := bundle["energyExplanation"].(map[string]any)
	if strings.HasPrefix(context, "zone") {
		graph = graph["zoneResults"].([]any)[0].(map[string]any)
	}
	if strings.HasSuffix(context, "period") {
		graph = graph["periods"].([]any)[0].(map[string]any)
	}
	return graph
}

func epathOracleWireFixtureCheck(t *testing.T, bundle map[string]any) error {
	t.Helper()
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	return epathValidateOracleCandidateWire(bytes.NewReader(data))
}

func TestEnergyPathRealOracleCandidateWirePreservesZeroAndOptionalOmission(t *testing.T) {
	if err := epathOracleWireFixtureCheck(t, epathOracleWireFixture()); err != nil {
		t.Fatal(err)
	}
	for _, context := range []string{"building", "building-period", "zone", "zone-period"} {
		bundle := epathOracleWireFixture()
		graph := epathOracleWireFixtureGraph(bundle, context)
		for _, field := range []string{"nodes", "links", "reconciliation", "sources"} {
			delete(graph, field)
		}
		if err := epathOracleWireFixtureCheck(t, bundle); err != nil {
			t.Fatalf("canonical empty graph collections rejected: %v", err)
		}
	}
}

func TestEnergyPathRealOracleCandidateWireRejectsLostNumbers(t *testing.T) {
	for _, context := range []string{"building", "building-period", "zone", "zone-period"} {
		for _, collection := range []struct {
			name   string
			fields []string
		}{
			{"nodes", []string{"value"}},
			{"links", []string{"fromValue", "toValue"}},
			{"reconciliation", []string{"expectedValue", "explainedValue", "residualValue"}},
			{"loadBreakdown", []string{"value"}},
			{"quality", []string{"driverToLoadClosedPct", "endUseToCarrierClosedPct", "zoneAllocatedPct", "unassignedPct"}},
			{"drivers", []string{"found", "total"}},
			{"loads", []string{"found", "total"}},
			{"endUses", []string{"found", "total"}},
			{"carriers", []string{"found", "total"}},
			{"ratios", []string{"found", "total"}},
		} {
			for _, field := range collection.fields {
				for _, mutation := range []string{"absent", "null", "string", "boolean"} {
					t.Run(context+"/"+collection.name+"/"+field+"/"+mutation, func(t *testing.T) {
						bundle := epathOracleWireFixture()
						graph := epathOracleWireFixtureGraph(bundle, context)
						var record map[string]any
						switch collection.name {
						case "quality":
							record = graph["quality"].(map[string]any)
						case "drivers", "loads", "endUses", "carriers", "ratios":
							record = graph["quality"].(map[string]any)[collection.name].(map[string]any)
						case "loadBreakdown":
							record = graph["nodes"].([]any)[0].(map[string]any)["loadBreakdown"].([]any)[0].(map[string]any)
						default:
							record = graph[collection.name].([]any)[0].(map[string]any)
						}
						switch mutation {
						case "absent":
							delete(record, field)
						case "null":
							record[field] = nil
						case "string":
							record[field] = "0"
						case "boolean":
							record[field] = false
						}
						if err := epathOracleWireFixtureCheck(t, bundle); err == nil || !strings.Contains(err.Error(), "."+field) {
							t.Fatalf("lost number not rejected at its exact field: %v", err)
						}
					})
				}
			}
		}
	}
}

func TestEnergyPathRealOracleCandidateWireRejectsNullOptionalNumbers(t *testing.T) {
	for _, collection := range []struct {
		name   string
		fields []string
	}{
		{"nodes", []string{"rawValue", "effectiveValue", "allocatedValue"}},
		{"links", []string{"ratio"}},
		{"reconciliation", []string{"directValue", "allocatedValue", "unassignedValue", "overmappedValue"}},
		{"sources", []string{"rawValue", "effectiveValue", "allocatedValue"}},
	} {
		for _, field := range collection.fields {
			bundle := epathOracleWireFixture()
			row := epathOracleWireFixtureGraph(bundle, "building")[collection.name].([]any)[0].(map[string]any)
			row[field] = nil
			if err := epathOracleWireFixtureCheck(t, bundle); err == nil {
				t.Fatalf("%s.%s null accepted", collection.name, field)
			}
			row[field] = 0
			if err := epathOracleWireFixtureCheck(t, bundle); err != nil {
				t.Fatalf("%s.%s explicit zero rejected: %v", collection.name, field, err)
			}
		}
	}
}

func TestEnergyPathRealOracleCandidateWireScopedNumbersRemainOptionalButNotNull(t *testing.T) {
	for _, field := range []string{"rawValue", "effectiveValue", "allocatedValue"} {
		bundle := epathOracleWireFixture()
		source := epathOracleWireFixtureGraph(bundle, "building")["sources"].([]any)[0].(map[string]any)
		detail := map[string]any{"scope": map[string]any{"kind": "zone", "zoneName": "Office"}}
		source["scopeDetails"] = []any{detail}
		if err := epathOracleWireFixtureCheck(t, bundle); err != nil {
			t.Fatalf("unknown scoped source quantity rejected: %v", err)
		}
		for _, invalid := range []any{nil, "0", false} {
			detail[field] = invalid
			if err := epathOracleWireFixtureCheck(t, bundle); err == nil {
				t.Fatalf("scoped %s=%v accepted as reported zero", field, invalid)
			}
		}
		detail[field] = 0
		if err := epathOracleWireFixtureCheck(t, bundle); err != nil {
			t.Fatalf("known scoped zero rejected: %v", err)
		}
	}
}

func TestEnergyPathRealOracleCandidateWireRejectsInvalidEnvelope(t *testing.T) {
	for _, raw := range []string{`{}`, `{"energyExplanation":null}`, `{"energyExplanation":[]}`, `{"energyExplanation":{"schema":"semantic-idf.energy-explanation/v1"}}`, `{"energyExplanation":{}} {}`} {
		if err := epathValidateOracleCandidateWire(strings.NewReader(raw)); err == nil {
			t.Fatalf("invalid envelope accepted: %s", raw)
		}
	}
	for _, value := range []any{-1, 0.5, 1} {
		bundle := epathOracleWireFixture()
		epathOracleWireFixtureGraph(bundle, "zone-period")["quality"].(map[string]any)["loads"].(map[string]any)["found"] = value
		if err := epathOracleWireFixtureCheck(t, bundle); err == nil {
			t.Fatalf("invalid count accepted: %v", value)
		}
	}
}

// Read-only wire inspection can also diagnose a preserved older snapshot. It
// does not bypass the production/input SHA binding of the actual SQL oracle.
func TestEnergyPathRealOracleCandidateWireSaved(t *testing.T) {
	path := os.Getenv("EPATH_REAL_ORACLE_WIRE_PATH")
	if path == "" {
		t.Skip("explicit saved candidate path required; wire check only, not acceptance")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := epathValidateOracleCandidateWire(file); err != nil {
		t.Fatal(err)
	}
	t.Log("original candidate wire numeric presence checked; no compatibility repair, SQL approval, or writes")
}
