package simulation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEPATH180DocumentedJSONUsesActualGoWireFields(t *testing.T) {
	doc := epath180ReadSchemaDocumentation(t)
	data := []byte(epath180Fence(t, doc, "JSON", "json"))
	var example map[string]any
	if err := json.Unmarshal(data, &example); err != nil {
		t.Fatalf("documented JSON is not valid: %v", err)
	}
	epath180ValidateJSONFields(t, example, reflect.TypeOf(EnergyExplanationResult{}), "example")
	if example["schema"] != energyExplanationSchema {
		t.Fatalf("documented schema=%v", example["schema"])
	}
	var result EnergyExplanationResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("documented JSON cannot cross actual Go read boundary: %v", err)
	}
	levels := map[string]bool{}
	nodes := map[string]map[string]any{}
	for _, item := range example["nodes"].([]any) {
		node := item.(map[string]any)
		levels[node["level"].(string)] = true
		nodes[node["id"].(string)] = node
	}
	for _, level := range []string{"driver", "load", "end_use", "carrier"} {
		if !levels[level] {
			t.Fatalf("documented JSON omits primary stage%s", level)
		}
	}
	conversion, correspondence := false, false
	for _, item := range example["links"].([]any) {
		link := item.(map[string]any)
		from, to := nodes[link["fromId"].(string)], nodes[link["toId"].(string)]
		if from == nil || to == nil {
			t.Fatalf("documented link has unresolved endpoint: %v", link)
		}
		if link["relation"] == "load_to_end_use" {
			conversion = true
			if from["scaleDomain"] != "thermal" || to["scaleDomain"] != "site" || link["fromValue"] == link["toValue"] {
				t.Fatalf("documented conversion erases separate energy domains: %v", link)
			}
			if link["fromUnit"] == "" || link["toUnit"] == "" {
				t.Fatal("documented conversion lost dual units")
			}
		}
		if link["relation"] == "source_correspondence" {
			correspondence = true
			if _, exists := link["ratio"]; exists {
				t.Fatal("source correspondence example must not claim equipment ratio")
			}
		}
	}
	if !conversion || !correspondence {
		t.Fatal("documentation must demonstrate both unequal dual-value conversion and non-flow correspondence")
	}
}

func TestEPATH180DocumentedPythonReconstructsActualV2AndMigratedV1(t *testing.T) {
	doc := epath180ReadSchemaDocumentation(t)
	script := epath180Fence(t, doc, "PYTHON", "python")
	python := epath180Python(t)
	docData := []byte(epath180Fence(t, doc, "JSON", "json"))
	docGraph := epath180Object(t, docData)
	t.Run("documented example", func(t *testing.T) {
		output := epath180RunPython(t, python, script, docData, false)
		epath180AssertReconstructed(t, docGraph, docGraph, output, "annual")
	})
	cooling := epath121GeneratedConversion(t, "cooling", "electricity", 100, 25)
	heating := epath121GeneratedConversion(t, "heating", "natural_gas", 85, 100)
	correspondence := UpgradeEnergyExplanationV1(epath093BackendBuildingFixture())
	for _, test := range []struct {
		name  string
		value EnergyExplanationResult
	}{{"actual cooling100 thermal to25 site", cooling}, {"actual heating85 thermal to100 gas", heating}, {"actual source correspondence", correspondence}} {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(test.value)
			if err != nil {
				t.Fatal(err)
			}
			graph := epath180Object(t, data)
			output := epath180RunPython(t, python, script, data, false)
			epath180AssertReconstructed(t, graph, graph, output, "annual")
		})
	}
	t.Run("upgraded thermal residual stays auxiliary context", func(t *testing.T) {
		legacy := epath092AuditConversionFixture("cooling", []epath092AuditCarrierValue{{"electricity", 25}}, 100, "kWh thermal", "kWh")
		legacy.Nodes = append(legacy.Nodes, EnergyExplanationNode{ID: "residual.heat.cooling", Level: "residual", Kind: "heat.residual", Label: "Unclosed thermal context", Value: 7, Unit: "kWh thermal", ServiceKind: "cooling", Basis: "residual", SourceIDs: []string{"load.cooling"}})
		legacy.Edges = append(legacy.Edges, EnergyExplanationEdge{ID: "thermal residual evidence", FromID: "load.cooling", ToID: "residual.heat.cooling", Relation: "residual", Value: 7, Unit: "kWh thermal", ServiceKind: "cooling", Basis: "residual", SourceIDs: []string{"load.cooling"}})
		upgraded := UpgradeEnergyExplanationV1(legacy)
		found := false
		for _, link := range upgraded.Links {
			from := energyPathV2NodeByID(upgraded.Nodes, link.FromID)
			to := energyPathV2NodeByID(upgraded.Nodes, link.ToID)
			if link.Relation == "residual" && from != nil && to != nil && from.ScaleDomain == "thermal" && to.Level == "load" {
				found = true
			}
		}
		if !found {
			t.Fatal("actual Go upgrade fixture must retain a thermal residual-to-load context link")
		}
		data, err := json.Marshal(upgraded)
		if err != nil {
			t.Fatal(err)
		}
		graph := epath180Object(t, data)
		output := epath180RunPython(t, python, script, data, false)
		epath180AssertReconstructed(t, graph, graph, output, "annual")
	})
	legacy, err := os.ReadFile(filepath.Join("testdata", "energy_path", "v1_energy_explanation.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Run("legacy reader boundary", func(t *testing.T) {
		rejected := epath180RunPython(t, python, script, legacy, true)
		if !strings.Contains(strings.ToLower(string(rejected)), "v1") {
			t.Fatalf("raw v1 rejection lacks migration guidance: %s", rejected)
		}
		var upgraded EnergyExplanationResult
		if err := json.Unmarshal(legacy, &upgraded); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(upgraded)
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]json.RawMessage
		if err := json.Unmarshal(data, &wire); err != nil {
			t.Fatal(err)
		}
		if _, exists := wire["edges"]; exists {
			t.Fatal("Go migration emitted deprecated edges")
		}
		if upgraded.Schema != energyExplanationSchema || len(upgraded.Links) == 0 {
			t.Fatal("frozen v1 fixture did not use actual canonical upgrade")
		}
		root := epath180Object(t, data)
		selected := root
		for _, period := range epath180Records(root["periods"]) {
			if period["id"] == "annual" {
				selected = period
				break
			}
		}
		output := epath180RunPython(t, python, script, data, false)
		epath180AssertReconstructed(t, root, selected, output, "annual")
	})
	t.Run("wrappers and strict selected month", func(t *testing.T) {
		month := EnergyPeriod{ID: "M1", Label: "January", Kind: "monthly", Nodes: append([]EnergyExplanationNode(nil), cooling.Nodes...), Links: append([]EnergyPathLink(nil), cooling.Links...)}
		for index := range month.Nodes {
			node := &month.Nodes[index]
			node.Period = "M1"
			node.Value /= 2
			node.RawValue /= 2
			node.EffectiveValue /= 2
			node.AllocatedValue /= 2
			node.DisplayValue /= 2
		}
		for index := range month.Links {
			link := &month.Links[index]
			link.Period = "M1"
			link.FromValue /= 2
			link.ToValue /= 2
		}
		cooling.Periods = []EnergyPeriod{month}
		data, err := json.Marshal(cooling)
		if err != nil {
			t.Fatal(err)
		}
		root := epath180Object(t, data)
		selected := epath180Records(root["periods"])[0]
		for _, wrapper := range []map[string]any{{"energyExplanation": root}, {"purposeResults": map[string]any{"energyExplanation": root}}} {
			wrapped, _ := json.Marshal(wrapper)
			output := epath180RunPython(t, python, script, wrapped, false, "M1")
			epath180AssertReconstructed(t, root, selected, output, "M1")
		}
		_ = epath180RunPython(t, python, script, data, true, "M2")
	})
	t.Run("Zone selection uses scoped graph and root source records", func(t *testing.T) {
		zone := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "effective"}, Nodes: append([]EnergyExplanationNode(nil), cooling.Nodes...), Links: append([]EnergyPathLink(nil), cooling.Links...)}
		ids := map[string]string{}
		for index := range zone.Nodes {
			node := &zone.Nodes[index]
			ids[node.ID] = fmt.Sprintf("opaque Office node %d", index)
			node.ID = ids[node.ID]
			node.ZoneName = "Office"
			node.AggregationBasis = "effective"
		}
		for index := range zone.Links {
			link := &zone.Links[index]
			link.FromID = ids[link.FromID]
			link.ToID = ids[link.ToID]
			link.ZoneName = "Office"
		}
		cooling.ZoneResults = []EnergyExplanationZoneResult{zone}
		data, err := json.Marshal(cooling)
		if err != nil {
			t.Fatal(err)
		}
		root := epath180Object(t, data)
		selected := epath180Records(root["zoneResults"])[0]
		expectedRoot := map[string]any{"scope": selected["scope"], "sources": root["sources"]}
		output := epath180RunPython(t, python, script, data, false, "annual", "Office")
		epath180AssertReconstructed(t, expectedRoot, selected, output, "annual")
		_ = epath180RunPython(t, python, script, data, true, "M1", "Office") // Building M1 cannot fill a missing Zone month.
		_ = epath180RunPython(t, python, script, data, true, "annual", "Missing Zone")
	})
}

func epath180AssertReconstructed(t *testing.T, root, selected map[string]any, data []byte, period string) {
	t.Helper()
	output := epath180Object(t, data)
	if output["period"] != period {
		t.Fatalf("Python selectedperiod=%v want%s", output["period"], period)
	}
	if !reflect.DeepEqual(output["scope"], root["scope"]) {
		t.Fatalf("Python changed selected scope: got%v want%v", output["scope"], root["scope"])
	}
	stages, ok := output["stages"].(map[string]any)
	if !ok {
		t.Fatal("Python omitted fourstage graph")
	}
	primary := map[string]bool{"driver": true, "load": true, "end_use": true, "carrier": true}
	nodes := map[string]map[string]any{}
	wantStages := map[string][]map[string]any{"driver": {}, "load": {}, "end_use": {}, "carrier": {}}
	var auxiliary []map[string]any
	for _, node := range epath180Records(selected["nodes"]) {
		nodes[node["id"].(string)] = node
		level, _ := node["level"].(string)
		if primary[level] {
			wantStages[level] = append(wantStages[level], node)
		} else {
			auxiliary = append(auxiliary, node)
		}
	}
	if len(stages) != 4 {
		t.Fatalf("Python invented quantitative stages: %v", stages)
	}
	for _, field := range []string{"flows", "nonFlowRelations", "contextLinks", "auxiliaryNodes", "sources"} {
		if _, ok := output[field].([]any); !ok {
			t.Fatalf("Python%s must be a JSON array, including when empty", field)
		}
	}
	for level, want := range wantStages {
		if _, ok := stages[level].([]any); !ok {
			t.Fatalf("Python stage%s must be a JSON array", level)
		}
		epath180AssertRecordSet(t, "stage "+level, want, epath180Records(stages[level]))
	}
	epath180AssertRecordSet(t, "auxiliary nodes", auxiliary, epath180Records(output["auxiliaryNodes"]))
	var flows, nonFlow, contextLinks []map[string]any
	for _, link := range epath180Records(selected["links"]) {
		relation, _ := link["relation"].(string)
		switch relation {
		case "driver_to_load", "load_to_end_use", "end_use_to_carrier", "direct_end_use_to_carrier":
			flows = append(flows, link)
		case "source_correspondence":
			nonFlow = append(nonFlow, link)
		case "residual":
			from, _ := link["fromValue"].(float64)
			to, _ := link["toValue"].(float64)
			start := nodes[link["fromId"].(string)]
			end := nodes[link["toId"].(string)]
			if start["level"] == "residual" && start["scaleDomain"] == "site" && end["level"] == "carrier" && end["scaleDomain"] == "site" && from > 0 && to > 0 {
				flows = append(flows, link)
			} else {
				contextLinks = append(contextLinks, link)
			}
		default:
			contextLinks = append(contextLinks, link)
		}
	}
	epath180AssertRecordSet(t, "physical flows", flows, epath180Records(output["flows"]))
	epath180AssertRecordSet(t, "non-flow correspondences", nonFlow, epath180Records(output["nonFlowRelations"]))
	epath180AssertRecordSet(t, "context links", contextLinks, epath180Records(output["contextLinks"]))
	epath180AssertRecordSet(t, "root source provenance", epath180Records(root["sources"]), epath180Records(output["sources"]))
}

func epath180AssertRecordSet(t *testing.T, label string, want, actual []map[string]any) {
	t.Helper()
	index := func(rows []map[string]any) map[string]map[string]any {
		result := map[string]map[string]any{}
		for _, row := range rows {
			id, ok := row["id"].(string)
			if !ok || id == "" {
				t.Fatalf("%s has missing opaqueID: %v", label, row)
			}
			if _, exists := result[id]; exists {
				t.Fatalf("%s duplicatedID%s", label, id)
			}
			result[id] = row
		}
		return result
	}
	if !reflect.DeepEqual(index(want), index(actual)) {
		t.Fatalf("Python changed%s fields/identity/units/source trace:\nwant=%v\ngot=%v", label, want, actual)
	}
}

func epath180Records(value any) []map[string]any {
	var records []map[string]any
	for _, item := range func() []any { rows, _ := value.([]any); return rows }() {
		if record, ok := item.(map[string]any); ok {
			records = append(records, record)
		}
	}
	return records
}
func epath180Object(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatalf("JSON object: %v\n%s", err, data)
	}
	return object
}

func epath180ReadSchemaDocumentation(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "energy-path-schema.md"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}
func epath180Fence(t *testing.T, doc, marker, language string) string {
	t.Helper()
	name := "<!-- EPATH180:" + marker + " -->"
	index := strings.Index(doc, name)
	if index < 0 {
		t.Fatalf("documentation marker%s missing", name)
	}
	rest := strings.TrimLeft(doc[index+len(name):], " \t\n")
	prefix := "```" + language + "\n"
	if !strings.HasPrefix(rest, prefix) {
		t.Fatalf("marker%s must immediately precede%s fence", name, language)
	}
	rest = rest[len(prefix):]
	end := strings.Index(rest, "\n```")
	if end < 0 {
		t.Fatalf("unclosed%s documentation fence", language)
	}
	return rest[:end]
}

func epath180Python(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python", "python3"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	t.Skip("Python3 is not installed; standard-library documentation example needs its interpreter")
	return ""
}
func epath180RunPython(t *testing.T, python, script string, input []byte, wantError bool, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, python, append([]string{"-X", "utf8", "-c", script}, args...)...)
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if wantError {
		if err == nil {
			t.Fatalf("Python accepted unsupported input/period: %s", output)
		}
		return output
	}
	if err != nil {
		t.Fatalf("documented Python failed: %v\n%s", err, output)
	}
	return output
}

// Actual custom UnmarshalJSON methods intentionally ignore unknown keys. Walk
// reflected wire tags too, so an attractive but invented doc field cannot pass.
func epath180ValidateJSONFields(t *testing.T, value any, kind reflect.Type, path string) {
	t.Helper()
	if value == nil {
		return
	}
	for kind.Kind() == reflect.Pointer {
		kind = kind.Elem()
	}
	switch kind.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("%s must be JSON object", path)
		}
		fields := map[string]reflect.Type{}
		for index := 0; index < kind.NumField(); index++ {
			field := kind.Field(index)
			if !field.IsExported() {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			fields[name] = field.Type
		}
		for key, child := range object {
			field, exists := fields[key]
			if !exists {
				t.Fatalf("documentation invented%s.%s; absent from%s wire", path, key, kind.Name())
			}
			epath180ValidateJSONFields(t, child, field, path+"."+key)
		}
	case reflect.Slice, reflect.Array:
		array, ok := value.([]any)
		if !ok {
			t.Fatalf("%s must be JSON array", path)
		}
		for index, child := range array {
			epath180ValidateJSONFields(t, child, kind.Elem(), fmt.Sprintf("%s[%d]", path, index))
		}
	case reflect.String:
		if _, ok := value.(string); !ok {
			t.Fatalf("%s must be JSON string", path)
		}
	case reflect.Bool:
		if _, ok := value.(bool); !ok {
			t.Fatalf("%s must be JSON bool", path)
		}
	case reflect.Float32, reflect.Float64, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if _, ok := value.(float64); !ok {
			t.Fatalf("%s must be JSON number", path)
		}
	}
}
