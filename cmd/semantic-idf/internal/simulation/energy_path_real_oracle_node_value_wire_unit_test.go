package simulation

// Independent acceptance support. Literal original JSON is the transport authority. No production
// decoder repairs values, and no UI requirement is added by these tests.
import (
	"bytes"
	"encoding/json"
	"testing"
)

func epathOracleNodeValueHandContext(t *testing.T, context string, node any) json.RawMessage {
	t.Helper()
	graph := map[string]any{"nodes": []any{node}}
	root := graph
	switch context {
	case "building":
	case "building period":
		root = map[string]any{"periods": []any{graph}}
	case "building annual alias":
		graph["id"] = "annual"
		root = map[string]any{"periods": []any{graph}}
	case "zone":
		root = map[string]any{"zoneResults": []any{graph}}
	case "zone period":
		root = map[string]any{"zoneResults": []any{map[string]any{"periods": []any{graph}}}}
	case "zone annual alias":
		graph["id"] = "annual"
		root = map[string]any{"zoneResults": []any{map[string]any{"periods": []any{graph}}}}
	default:
		t.Fatalf("unknown hand graph context %s", context)
	}
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestEnergyPathRealOracleNodeValueWireEveryCanonicalContext(t *testing.T) {
	contexts := []string{"building", "building period", "building annual alias", "zone", "zone period", "zone annual alias"}
	for _, context := range contexts {
		for _, literal := range []string{"0", "-0", "1.234567890123", "-2.5", "1e3", "1e308"} {
			data := epathOracleNodeValueHandContext(t, context, map[string]any{"id": "native-node", "value": json.RawMessage(literal), "rawValue": nil, "allocatedValue": 0})
			before := append([]byte(nil), data...)
			if err := epathValidateOracleOriginalNodeValues(data); err != nil {
				t.Fatalf("%s explicit finite %s rejected: %v", context, literal, err)
			}
			if !bytes.Equal(before, data) {
				t.Fatal("raw guard changed original node precision/knownness")
			}
		}
		for _, literal := range []string{"missing", "null", `"0"`, `"NaN"`, "true", "false", "[]", "{}", "1e999", "-1e999"} {
			node := map[string]any{"id": "native-node", "allocatedValue": 0}
			if literal != "missing" {
				node["value"] = json.RawMessage(literal)
			}
			data := epathOracleNodeValueHandContext(t, context, node)
			if err := epathValidateOracleOriginalNodeValues(data); err == nil {
				t.Fatalf("%s %s value acquired an explicit zero from allocatedValue:0", context, literal)
			}
		}
		for _, node := range []any{nil, 0, "node", []any{}} {
			if err := epathValidateOracleOriginalNodeValues(epathOracleNodeValueHandContext(t, context, node)); err == nil {
				t.Fatalf("%s non-object/null node supplied a required numeric value", context)
			}
		}
	}
}

func TestEnergyPathRealOracleNodeValueWireAbsentCollectionsRemainCoverageOwned(t *testing.T) {
	for _, raw := range []string{
		`{}`, `{"nodes":null}`, `{"nodes":[]}`,
		`{"periods":[{}, {"nodes":null}, {"nodes":[]}]}`,
		`{"zoneResults":[{}, {"nodes":null}, {"nodes":[],"periods":[{}, {"nodes":null}, {"nodes":[]}]}]}`,
		// Source/summary/link quantities have distinct nullable contracts. This
		// guard deliberately does not impose node Value rules upon them.
		`{"sources":[{"rawValue":null}],"summary":{"ratios":[{"value":null}]},"links":[{"fromValue":null}]}`,
	} {
		if err := epathValidateOracleOriginalNodeValues(json.RawMessage(raw)); err != nil {
			t.Fatalf("collection/other-record responsibility moved into node scalar guard: %v", err)
		}
	}
}

func TestEnergyPathRealOracleNodeValueWireRejectsNullBeforeLeafLosesPresence(t *testing.T) {
	for _, raw := range []string{`{"id":"native-node","value":null,"allocatedValue":0}`, `{"id":"native-node","allocatedValue":0}`} {
		var node EnergyExplanationNode
		if err := json.Unmarshal([]byte(raw), &node); err != nil {
			t.Fatal(err)
		}
		if node.Value != 0 || !node.inspectorDecodedFromJSON || node.inspectorValuePresence != 4 {
			t.Fatal("hand fixture no longer demonstrates value-null/absence lost while allocated zero is present")
		}
		if err := epathValidateOracleOriginalNodeValues(json.RawMessage(`{"nodes":[` + raw + `]}`)); err == nil {
			t.Fatal("unknown node Value was silently converted to explicit zero")
		}
	}
	// The actual public writer declares value without omitempty: even a
	// deliberately present zero node has a value member. This adds no layout,
	// navigation or positive-node requirement, only existing wire fidelity.
	raw, err := json.Marshal(EnergyExplanationNode{ID: "explicit-zero-node", Value: 0})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fields["value"], []byte("0")) {
		t.Fatal("public canonical node writer omitted a required explicit zero")
	}
	if err := epathValidateOracleOriginalNodeValues(json.RawMessage(`{"nodes":[` + string(raw) + `]}`)); err != nil {
		t.Fatal(err)
	}
}
