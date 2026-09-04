package frontendchecks

import (
	"encoding/json"
	"reflect"
	"regexp"
	"sort"
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

func TestEnergyPathV1UIContractGoldenMatchesCurrentRenderer(t *testing.T) {
	var golden energyPathV1UIContract
	readEnergyPathV1Golden(t, "v1_ui_contract.golden.json", &golden)
	if golden.CapturedFrom == "" {
		t.Fatal("v1 UI golden is missing capturedFrom")
	}

	source := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	allowedBody := energyPathV1SourceMatch(t, source,
		`(?s)function energySubview\(.*?const allowed = \[(.*?)\];`,
		"Energy subview allow-list",
	)
	allowed := energyPathV1QuotedStrings(allowedBody)
	if !reflect.DeepEqual(allowed, golden.PrimarySubviewOrder) {
		t.Fatalf("Energy subview allow-list = %#v, v1 golden = %#v", allowed, golden.PrimarySubviewOrder)
	}

	tabsBody := energyPathV1SourceMatch(t, source,
		`(?s)function renderEnergySubviewControls\(.*?const tabs = \[(.*?)\];`,
		"Energy subview tabs",
	)
	tabPattern := regexp.MustCompile(`\[\s*"([^"]+)"\s*,`)
	tabMatches := tabPattern.FindAllStringSubmatch(tabsBody, -1)
	tabs := make([]string, 0, len(tabMatches))
	for _, match := range tabMatches {
		tabs = append(tabs, match[1])
	}
	if !reflect.DeepEqual(tabs, golden.PrimarySubviewOrder) {
		t.Fatalf("Energy subview tabs = %#v, v1 golden = %#v", tabs, golden.PrimarySubviewOrder)
	}

	controlsSource := energyPathV1SourceSlice(t, source, "function renderEnergyPeriodControls", "function normalizeSimulationEnergyFocusState")
	controlPattern := regexp.MustCompile(`data-simulation-energy-([a-z-]+)`)
	controlMatches := controlPattern.FindAllStringSubmatch(controlsSource, -1)
	controls := make([]string, 0, len(controlMatches))
	seen := map[string]bool{}
	for _, match := range controlMatches {
		if !seen[match[1]] {
			seen[match[1]] = true
			controls = append(controls, match[1])
		}
	}
	wantControls := append([]string(nil), golden.PrimaryControls...)
	sort.Strings(controls)
	sort.Strings(wantControls)
	if !reflect.DeepEqual(controls, wantControls) {
		t.Fatalf("Energy primary controls = %#v, v1 golden = %#v", controls, wantControls)
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
