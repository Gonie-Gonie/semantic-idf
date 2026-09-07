package frontendchecks

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

type energyPathV1UIContract struct {
	PrimarySubviewOrder []string `json:"primarySubviewOrder"`
	PrimaryControls     []string `json:"primaryControls"`
	CapturedFrom        string   `json:"capturedFrom"`
}

type energyPathV1BatchExportContract struct {
	CSV struct {
		Filename string   `json:"filename"`
		Headers  []string `json:"headers"`
		Sections []string `json:"sections"`
	} `json:"csv"`
}

func TestEnergyPathV1UIContractGoldenRemainsHistorical(t *testing.T) {
	var golden energyPathV1UIContract
	readEnergyPathV1Golden(t, "v1_ui_contract.golden.json", &golden)
	if golden.CapturedFrom == "" {
		t.Fatal("v1 UI golden is missing capturedFrom")
	}

	// EPATH-140 acceptance now verifies the live single view. Keep the captured
	// pre-refactor UI snapshot intact as historical evidence, not a requirement
	// to resurrect old tabs. Stored V1 data and export contracts remain active.
	if want := []string{"overview", "sankey", "monthly", "zones", "systems"}; !reflect.DeepEqual(golden.PrimarySubviewOrder, want) {
		t.Fatalf("historical V1 subview snapshot changed: %#v", golden.PrimarySubviewOrder)
	}
	if want := []string{"period-kind", "period", "period-index", "focus-mode", "zone-focus", "service-path-focus", "loop-focus", "sankey-mode", "sign-mode", "node-limit"}; !reflect.DeepEqual(golden.PrimaryControls, want) {
		t.Fatalf("historical V1 controls snapshot changed: %#v", golden.PrimaryControls)
	}
}

func TestEnergyPathV1BatchCSVGoldenMatchesCurrentExporter(t *testing.T) {
	var golden energyPathV1BatchExportContract
	readEnergyPathV1Golden(t, "v1_batch_export.golden.json", &golden)

	source := readTestFile(t, "frontend/src/js/batch/batch-simulation.js")
	exporter := energyPathV1SourceSlice(t, source, "function exportMultiSimulationCSV", "async function exportMultiSimulationXLSX")
	headerBody := energyPathV1SourceMatch(t, exporter, `(?s)const rows = \[\[(.*?)\]\];`, "Batch CSV header")
	headers := energyPathV1QuotedStrings(headerBody)
	if !reflect.DeepEqual(headers, golden.CSV.Headers) {
		t.Fatalf("Batch CSV headers = %#v, v1 golden = %#v", headers, golden.CSV.Headers)
	}

	filenamePattern := regexp.MustCompile(`downloadCSV\(rows,\s*"([^"]+)"\)`)
	filenameMatch := filenamePattern.FindStringSubmatch(exporter)
	if len(filenameMatch) != 2 {
		t.Fatal("Batch CSV download filename is missing")
	}
	if filenameMatch[1] != golden.CSV.Filename {
		t.Fatalf("Batch CSV filename = %q, v1 golden = %q", filenameMatch[1], golden.CSV.Filename)
	}

	sectionAnchors := []struct {
		name   string
		anchor string
	}{
		{name: "purpose_metrics", anchor: `for (const metric of item.purposeMetrics || [])`},
		{name: "energy_summary", anchor: `energyExplanationSummaryExportItems(explanationSummary).forEach`},
		{name: "energy_sources", anchor: `energyExplanationSourceExportItems(explanation).forEach`},
		{name: "source_availability", anchor: `energyExplanationSourceAvailabilityExportItems(explanation).forEach`},
		{name: "energy_nodes", anchor: `energyExplanationNodeExportItems(explanation).forEach`},
		{name: "energy_edges", anchor: `energyExplanationEdgeExportItems(explanation).forEach`},
		{name: "energy_reconciliation", anchor: `energyExplanationReconciliationExportItems(explanation).forEach`},
		{name: "energy_warnings", anchor: `energyExplanationWarningExportItems(explanation).forEach`},
	}
	sections := make([]string, 0, len(sectionAnchors))
	previous := -1
	for _, section := range sectionAnchors {
		index := strings.Index(exporter, section.anchor)
		if index < 0 {
			t.Fatalf("Batch CSV section %q anchor is missing: %s", section.name, section.anchor)
		}
		if index <= previous {
			t.Fatalf("Batch CSV section %q is out of order", section.name)
		}
		previous = index
		sections = append(sections, section.name)
	}
	if !reflect.DeepEqual(sections, golden.CSV.Sections) {
		t.Fatalf("Batch CSV sections = %#v, v1 golden = %#v", sections, golden.CSV.Sections)
	}
}

func readEnergyPathV1Golden(t *testing.T, name string, target any) {
	t.Helper()
	payload := readTestFile(t, "internal/simulation/testdata/energy_path/"+name)
	if err := json.Unmarshal([]byte(payload), target); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
}

func energyPathV1SourceMatch(t *testing.T, source string, pattern string, label string) string {
	t.Helper()
	match := regexp.MustCompile(pattern).FindStringSubmatch(source)
	if len(match) != 2 {
		t.Fatalf("%s was not found", label)
	}
	return match[1]
}

func energyPathV1QuotedStrings(source string) []string {
	matches := regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(source, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, match[1])
	}
	return values
}

func energyPathV1SourceSlice(t *testing.T, source string, start string, end string) string {
	t.Helper()
	startIndex := strings.Index(source, start)
	if startIndex < 0 {
		t.Fatalf("source start marker is missing: %s", start)
	}
	endIndex := strings.Index(source[startIndex+len(start):], end)
	if endIndex < 0 {
		t.Fatalf("source end marker is missing: %s", end)
	}
	return source[startIndex : startIndex+len(start)+endIndex]
}
