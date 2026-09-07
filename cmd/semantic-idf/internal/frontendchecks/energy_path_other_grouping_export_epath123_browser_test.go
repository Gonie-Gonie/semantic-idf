package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestEPATH123OtherGroupingPreservesActualBatchCSVExport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser EPATH-123 actual CSV export test in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	// Load the real Tools page, including tools.js, downloadCSV, and csvCell.
	// Only the desktop API supplies fixture data; no export serializer is replaced.
	page := readTestFile(t, "frontend/src/tools.html")
	page = strings.Replace(page, "</head>", epath123ExportSetupScript+"</head>", 1)
	page = strings.Replace(page, "</body>", epath123ExportAssertionsScript+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/src/epath123-export.html", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check",
		"--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/src/epath123-export.html",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("EPATH-123 actual CSV export browser timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("EPATH-123 actual CSV export browser failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath123-export-status="passed"`) {
		t.Fatalf("EPATH-123 grouping/export contract failed:\n%s", output)
	}
}

const epath123ExportSetupScript = `<script>
(() => {
  const driver = (category, value, sourceID, label) => ({
    id: "driver." + category + ".cooling.building", level: "driver", kind: "driver." + category,
    driverCategory: category, label, value, rawValue: value * 2, effectiveValue: value * 2,
    allocatedValue: value, displayValue: value, allocationApplied: true,
    allocationExplanation: "Actual cooling load multiplied by the signed heat-pressure share.",
    unit: "kWh", scaleDomain: "thermal", period: "annual", serviceKind: "cooling",
    basis: "heat_balance_share", sourceIds: [sourceID],
  });
  const drivers = [
    driver("surface.exterior_walls", 99.5, "source.walls", "Exterior walls"),
    driver("internal.people", 0.2, "source.people", 'People, "Office"'),
    driver("internal.lighting", 0.1, "source.lighting", "Lighting heat"),
    driver("internal.equipment", 0.2, "source.equipment", "Equipment heat"),
  ];
  const nodes = [...drivers,
    { id: "load.cooling.building", level: "load", kind: "load.zone_cooling", label: "Cooling load", value: 100, unit: "kWh", scaleDomain: "thermal", serviceKind: "cooling", sourceIds: ["source.load"] },
    { id: "end_use.cooling.building", level: "end_use", kind: "energy.cooling", label: "Cooling", value: 25, unit: "kWh", scaleDomain: "site", endUse: "cooling", sourceIds: ["source.cooling"] },
    { id: "carrier.electricity.building", level: "carrier", kind: "carrier.electricity", label: "Electricity", value: 25, unit: "kWh", scaleDomain: "site", carrier: "electricity", sourceIds: ["source.facility"] },
  ];
  const links = drivers.map(node => ({
    id: "link." + node.id, fromId: node.id, toId: "load.cooling.building", relation: "driver_to_load",
    fromValue: node.value, toValue: node.value, fromUnit: "kWh", toUnit: "kWh", serviceKind: "cooling",
    basis: "heat_balance_share", sourceIds: [...node.sourceIds, "source.load"],
  }));
  links.push(
    { id: "link.conversion", fromId: "load.cooling.building", toId: "end_use.cooling.building", relation: "load_to_end_use", fromValue: 100, toValue: 25, fromUnit: "kWh", toUnit: "kWh", serviceKind: "cooling", basis: "reported_meter", ratio: 4, ratioKind: "coefficient_of_performance", ratioLabel: "COP", sourceIds: ["source.load", "source.cooling"] },
    { id: "link.carrier", fromId: "end_use.cooling.building", toId: "carrier.electricity.building", relation: "end_use_to_carrier", fromValue: 25, toValue: 25, fromUnit: "kWh", toUnit: "kWh", basis: "reported_meter", sourceIds: ["source.cooling"] },
  );
  const sources = nodes.map(node => ({
    id: node.sourceIds[0], sourceType: node.scaleDomain === "site" ? "sql_meter" : "sql_variable",
    isMeter: node.scaleDomain === "site", name: node.label + " Energy", keyValue: 'Office, "reference"',
    sourceUnit: "J", normalizedUnit: "kWh", units: "J", reportingFrequency: "Monthly",
    rawValue: node.rawValue ?? node.value, effectiveValue: node.effectiveValue ?? node.value,
  }));
  const explanation = { schema: "semantic-idf.energy-explanation/v2", purpose: "basic_energy", scope: { kind: "building", aggregationBasis: "model_total" }, nodes, links, sources };
  const result = { total: 1, completed: 1, succeeded: 1, failed: 0, workers: 1, results: [{
    filename: 'model, "reference".idf', inputPath: "C:/fixtures/model.idf", runId: "epath123-export", status: "succeeded",
    purposeMetrics: [], purposeResults: { energyExplanation: explanation },
  }] };
  const freeze = value => {
    if (value && typeof value === "object") { Object.values(value).forEach(freeze); Object.freeze(value); }
    return value;
  };
  window.epath123ExportFixture = freeze({ explanation, result, memberIDs: drivers.slice(1).map(node => node.id), memberSourceIDs: drivers.slice(1).map(node => node.sourceIds[0]) });
  window.epath123ExportCalls = 0;
  window.go = { main: { App: {
    GetSettings: async () => ({ version: 1, appearance: { theme: "light", language: "en" } }),
    GetAppInfo: async () => ({ name: "SemanticIDF", version: "test" }),
    GetSimulationEnvironment: async () => ({ weatherFolders: [], defaultWorkerCount: 1 }),
    SelectSimulationInputFiles: async () => ({ paths: ["C:/fixtures/model.idf"], rootDirectory: "C:/fixtures" }),
    RunMultipleSimulations: async () => { window.epath123ExportCalls++; return result; },
  } } };
  window.runtime = { EventsOn() {} };
  const blobs = new Map();
  const createURL = URL.createObjectURL.bind(URL);
  URL.createObjectURL = blob => { const url = createURL(blob); blobs.set(url, blob); return url; };
  const click = HTMLAnchorElement.prototype.click;
  window.epath123Downloads = [];
  HTMLAnchorElement.prototype.click = function() {
    if (this.download) {
      window.epath123Downloads.push({ filename: this.download, blob: blobs.get(this.href) });
      return;
    }
    return click.call(this);
  };
})();
</script>`

const epath123ExportAssertionsScript = `<div id="epath123-export-view"></div><pre id="epath123-export-result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const until = async (predicate, message) => {
  for (let attempt = 0; attempt < 200; attempt++) { if (predicate()) return; await new Promise(resolve => setTimeout(resolve, 10)); }
  throw new Error(message);
};
const parseCSV = text => {
  const rows = []; let row = [], field = "", quoted = false;
  for (let index = 0; index < text.length; index++) {
    const char = text[index];
    if (char === '"') {
      if (quoted && text[index + 1] === '"') { field += '"'; index++; } else quoted = !quoted;
    } else if (!quoted && char === ',') { row.push(field); field = ""; }
    else if (!quoted && (char === '\r' || char === '\n')) {
      if (char === '\r' && text[index + 1] === '\n') index++;
      row.push(field); rows.push(row); row = []; field = "";
    } else field += char;
  }
  if (row.length || field) { row.push(field); rows.push(row); }
  return rows;
};
try {
  await import("./js/tools.js");
  const view = await import("./js/views/energy-path-view.js");
  const fixture = window.epath123ExportFixture;
  assert(fixture.explanation.schema === view.ENERGY_PATH_SCHEMA_V2, "export fixture is not the actual v2 schema");
  const originalJSON = JSON.stringify(fixture.explanation);
  const exportButton = document.getElementById("multiSimulationExport");
  document.getElementById("multiSimulationSelectFiles").click();
  await until(() => !document.getElementById("multiSimulationRun").disabled, "normal file selection did not enable batch run");
  document.getElementById("multiSimulationRun").click();
  await until(() => window.epath123ExportCalls === 1 && !exportButton.disabled, "actual batch run did not enable the real CSV callback");
  exportButton.click();
  assert(window.epath123Downloads.length === 1, "real export callback did not create a downloadable CSV Blob");
  const beforeCSV = await window.epath123Downloads[0].blob.text();
  const state = { simulationEnergyScopeKind: "building", simulationEnergyPeriod: "annual", simulationEnergyService: "cooling" };
  const graph = view.energyPathGraphForState(fixture.explanation, state);
  const originalIDs = new Set(fixture.explanation.nodes.map(node => node.id));
  const group = graph.nodes.find(node => node.automaticOther && node.level === "driver" && !originalIDs.has(node.id) && fixture.memberSourceIDs.every(id => node.sourceIds?.includes(id)));
  assert(group, "real presentation did not combine the three sub-1% thermal driver contributions");
  assert(fixture.memberIDs.every(id => group.originalNodeIds.includes(id)), "group lost original node IDs before export");
  assert(graph.nodes.filter(node => node.level === "driver").length < fixture.explanation.nodes.filter(node => node.level === "driver").length, "grouping did not reduce the displayed driver count");
  state.simulationEnergySelection = group.id;
  const mount = document.getElementById("epath123-export-view");
  mount.innerHTML = view.renderEnergyPathView(fixture.explanation, state);
  assert(mount.querySelector('[data-energy-explanation-node="' + group.id + '"]'), "group node is not rendered");
  exportButton.click();
  assert(window.epath123Downloads.length === 2, "second real CSV export was not captured");
  const afterCSV = await window.epath123Downloads[1].blob.text();
  assert(beforeCSV === afterCSV, "presentation grouping changed actual exported CSV bytes");
  assert(JSON.stringify(fixture.explanation) === originalJSON, "grouping modified the original immutable simulation result");
  const rows = parseCSV(afterCSV), header = rows.shift();
  const column = name => { const index = header.indexOf(name); assert(index >= 0, "missing real CSV column: " + name); return index; };
  const typeIndex = column("metric_type"), idIndex = column("metric_id"), sourceIndex = column("source_ids"), valueIndex = column("value");
  const nodeRows = rows.filter(row => row[typeIndex] === "energy_explanation.node");
  const sourceRows = rows.filter(row => row[typeIndex] === "energy_explanation.source");
  assert(nodeRows.length === fixture.explanation.nodes.length, "CSV lost an original node row");
  assert(sourceRows.length === fixture.explanation.sources.length, "CSV lost an original source row");
  for (const node of fixture.explanation.nodes) {
    const row = nodeRows.find(row => row[idIndex] === node.id);
    assert(row && Number(row[valueIndex]) === node.value, "CSV dropped or changed original node " + node.id);
    for (const sourceID of node.sourceIds) assert(row[sourceIndex].split("; ").includes(sourceID), "original node CSV row lost source " + sourceID);
  }
  for (const memberID of fixture.memberIDs) assert(nodeRows.some(row => row[idIndex] === memberID), "group member disappeared from export: " + memberID);
  for (const sourceID of fixture.memberSourceIDs) assert(sourceRows.some(row => row[idIndex] === sourceID), "group source disappeared from export: " + sourceID);
  assert(!nodeRows.some(row => row[idIndex] === group.id), "synthetic presentation group replaced original export records");
  assert(nodeRows.every(row => row[column("file")] === fixture.result.results[0].filename), "actual CSV quoting lost commas/quotes in filename");
  assert(sourceRows.some(row => row[column("source_key")] === 'Office, "reference"'), "actual CSV quoting lost source trace text");
  assert(window.epath123Downloads.every(download => download.filename === "batch-simulation-purpose-results.csv" && download.blob.type === "text/csv"), "test did not traverse the real CSV download path");
  document.body.dataset.epath123ExportStatus = "passed";
  document.getElementById("epath123-export-result").textContent = JSON.stringify({ originalNodes: nodeRows.length, originalSources: sourceRows.length, groupMembers: fixture.memberIDs.length, identicalCSV: true });
} catch (error) {
  document.body.dataset.epath123ExportStatus = "failed";
  document.getElementById("epath123-export-result").textContent = String(error?.stack || error);
}
</script>`
