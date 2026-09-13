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

func TestEnergyPathLoadContextInspectorBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser Energy Path load-context harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-load-context", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathLoadContextHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--no-sandbox",
		"--no-first-run",
		"--no-default-browser-check",
		"--virtual-time-budget=10000",
		"--user-data-dir="+t.TempDir(),
		"--dump-dom",
		server.URL+"/energy-path-load-context",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("Energy Path load-context browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("Energy Path load-context browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-load-context-status="passed"`) {
		t.Fatalf("Energy Path load-context inspector contract failed in browser:\n%s", document)
	}
}

const energyPathLoadContextHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Energy Path load-context harness</title></head>
<body data-energy-path-load-context-status="pending"><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
try {
  const module = await import("/src/js/views/energy-path-view.js");
  const loadNode = {
    id: "load.cooling.zone-office",
    level: "load",
    label: "Cooling load",
    value: 100,
    serviceKind: "cooling",
    driverCategory: "load.cooling",
    sourceIds: ["actual-total"],
  };
  const explanation = {
    schema: module.ENERGY_PATH_SCHEMA_V2,
    sources: [
      {
        id: "actual-total", name: "Zone Ideal Loads Zone Total Cooling Energy",
        zoneName: "Office", driverCategory: "load.cooling", driverRole: "main_flow",
        inspectorSection: "breakdown", driverComponent: "load.delivered.combined",
      },
      {
        id: "lower-system", name: "Cooling Coil Total Cooling Energy",
        zoneName: "Office", driverCategory: "load.cooling", driverRole: "context",
        inspectorSection: "context", driverComponent: "load.delivered.combined",
      },
      {
        id: "predicted-raw", name: "Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate",
        zoneName: "Office", driverCategory: "load.cooling", driverRole: "context",
        inspectorSection: "context", driverComponent: "load.predicted.sensible",
      },
      {
        id: "predicted-delta", name: "Predicted vs delivered sensible cooling",
        zoneName: "Office", driverCategory: "load.cooling", driverRole: "context",
        inspectorSection: "balance", driverComponent: "load.predicted_vs_delivered.sensible",
        sourceType: "derived_formula", formula: "predicted - delivered",
        inputSourceIds: ["actual-sensible", "predicted-raw"],
      },
      {
        id: "other-zone-context", name: "Other-zone lower load",
        zoneName: "Lobby", driverCategory: "load.cooling", driverRole: "context",
        inspectorSection: "context", driverComponent: "load.delivered.combined",
      },
      {
        id: "other-service-context", name: "Heating prediction",
        zoneName: "Office", driverCategory: "load.heating", driverRole: "context",
        inspectorSection: "context", driverComponent: "load.predicted.sensible",
      },
      {
        id: "other-category-context", name: "Mechanical ventilation context",
        zoneName: "Office", driverCategory: "air.mechanical_ventilation", driverRole: "context",
        inspectorSection: "context", driverComponent: "mechanical_ventilation.sensible",
      },
    ],
  };
  const zoneState = {
    simulationEnergyScopeKind: "zone",
    simulationEnergyZoneName: "Office",
    simulationEnergyPeriod: "annual",
    simulationEnergyService: "cooling",
  };
  const sources = module.energyPathInspectorSources(explanation, loadNode, zoneState);
  const ids = sources.map((source) => source.id);
  assert(loadNode.sourceIds.length === 1 && loadNode.sourceIds[0] === "actual-total", "load node sourceIds must remain selected-only");
  for (const id of ["actual-total", "lower-system", "predicted-raw", "predicted-delta"]) {
    assert(ids.includes(id), "load inspector omitted " + id + ": " + ids.join(","));
  }
  for (const id of ["other-zone-context", "other-service-context", "other-category-context"]) {
    assert(!ids.includes(id), "load inspector leaked unrelated source " + id);
  }
  assert(new Set(ids).size === ids.length, "load inspector duplicated a source");

  const html = module.renderEnergyPathNodeInspector(explanation, [loadNode], loadNode.id, zoneState);
  const inspector = document.createElement("div");
  inspector.innerHTML = html;
  assert(!inspector.querySelector('[data-energy-path-detail-section="sources"], [data-energy-path-detail-section="entities"], [data-energy-path-source]'), "load inspector still renders removed Source data or Related model entities");
  const sourceHTML = module.renderEnergyPathSourceDetails(sources, zoneState);
  assert(sourceHTML.includes('data-energy-path-inspector-section="context"'), "standalone source details did not render Context section");
  assert(sourceHTML.includes('data-energy-path-inspector-section="balance"'), "standalone source details did not render Balance section");
  assert(sourceHTML.includes("Cooling Coil Total Cooling Energy"), "standalone source details lost lower-tier context");
  assert(sourceHTML.includes("Zone Predicted Sensible Load"), "standalone source details lost predicted raw context");
  assert(sourceHTML.includes("predicted - delivered"), "standalone source details lost derived comparison formula");
  assert(sourceHTML.includes("actual-sensible") && sourceHTML.includes("predicted-raw"), "standalone source details lost derived comparison inputs");
  assert(!sourceHTML.includes("Other-zone lower load") && !sourceHTML.includes("Heating prediction") && !sourceHTML.includes("Mechanical ventilation context"), "unrelated context leaked into standalone source details");

  document.body.dataset.energyPathLoadContextStatus = "passed";
  document.getElementById("result").textContent = JSON.stringify({ ids });
} catch (error) {
  document.body.dataset.energyPathLoadContextStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
