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

func TestEnergyPathAllServiceDriverProjectionBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser Energy Path driver projection harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-driver-taxonomy", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathDriverTaxonomyHarnessHTML)
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
		server.URL+"/energy-path-driver-taxonomy",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("Energy Path driver projection browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("Energy Path driver projection browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-driver-taxonomy-status="passed"`) {
		t.Fatalf("Energy Path driver projection contract failed in browser:\n%s", document)
	}
}

const energyPathDriverTaxonomyHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Energy Path driver taxonomy harness</title></head>
<body data-energy-path-driver-taxonomy-status="pending"><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
try {
  const module = await import("/src/js/views/energy-path-view.js");
  const categories = [
    ["surface.exterior_walls", "Wall heat exchange"],
    ["surface.roofs", "Roof heat exchange"],
    ["surface.ground_floors", "Ground / floor heat exchange"],
    ["surface.windows_doors", "Window / door heat exchange"],
    ["surface.interzone", "Interzone surfaces"],
    ["air.infiltration", "Infiltration"],
    ["air.mechanical_ventilation", "Mechanical ventilation"],
    ["air.interzone", "Interzone air"],
    ["internal.people", "People"],
    ["internal.lighting", "Lighting"],
    ["internal.equipment", "Equipment"],
    ["balance.storage_other", "Other / storage"],
  ];
  const nodes = [
    { id: "load.cooling.building", level: "load", label: "Cooling load", value: 100, serviceKind: "cooling" },
    { id: "load.heating.building", level: "load", label: "Heating load", value: 100, serviceKind: "heating" },
    { id: "driver.zero.cooling.building", level: "driver", label: "Zero driver trap", value: 0, driverCategory: "internal.people", serviceKind: "cooling" },
  ];
  const links = [];
  for (const service of ["cooling", "heating"]) {
    categories.forEach(([category, label], index) => {
      const value = category === "internal.equipment" ? 1 : index + 2;
      const id = "driver." + category + "." + service + ".building";
      nodes.push({
        id, level: "driver", kind: "driver." + category, driverCategory: category,
        label, value, rawValue: value, allocatedValue: value, serviceKind: service,
        sourceIds: [category + "." + service], relatedEntityIds: ["entity." + category + "." + service],
      });
      links.push({
        id: "link." + category + "." + service, fromId: id, toId: "load." + service + ".building",
        relation: "driver_to_load", fromValue: value, toValue: value, serviceKind: service,
        sourceIds: [category + "." + service],
      });
    });
  }
  const explanation = {
    schema: module.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    nodes, links, periods: [],
  };
  const allState = {
    simulationEnergyScopeKind: "building", simulationEnergyPeriod: "annual",
    simulationEnergyService: "all", simulationEnergySelection: "",
  };
  const graph = module.energyPathGraphForState(explanation, allState);
  const drivers = graph.nodes.filter((node) => node.level === "driver" && Number(node.value));
  assert(drivers.length === categories.length, "Taxonomy categories at or above 1% must not be removed to meet a node-count limit: " + drivers.length);
  assert(drivers.every((node) => node.id.includes(".all.building")), "All-services drivers were not projected to virtual all-service IDs");
  const wall = drivers.find((node) => node.driverCategory === "surface.exterior_walls");
  assert(wall && wall.value === 4, "Cooling/heating wall values were not merged");
  assert(wall.sourceIds.includes("surface.exterior_walls.cooling") && wall.sourceIds.includes("surface.exterior_walls.heating"), "Merged wall source provenance was lost");
  assert(wall.relatedEntityIds.length === 2, "Merged wall related entities were lost");
  const equipment = drivers.find((node) => node.driverCategory === "internal.equipment");
  assert(equipment && equipment.value === 2, "Equipment above 1% was incorrectly compacted");
  assert(drivers.some((node) => node.driverCategory === "balance.storage_other"), "Original Other / storage node is missing");
  const nodeIDs = new Set(graph.nodes.map((node) => node.id));
  assert(graph.links.every((link) => nodeIDs.has(link.fromId) && nodeIDs.has(link.toId)), "Projected graph has a dangling link endpoint");
  assert(graph.links.some((link) => link.toId === "load.cooling.building") && graph.links.some((link) => link.toId === "load.heating.building"), "Projected drivers did not retain both service branches");

  const html = module.renderEnergyPathView(explanation, allState);
  assert(!html.includes("Zero driver trap"), "Zero-value driver was rendered");
  assert(html.indexOf("Wall heat exchange") < html.indexOf("Roof heat exchange"), "Driver display order is not canonical");
  assert(html.indexOf("Roof heat exchange") < html.indexOf("Ground / floor heat exchange"), "Driver display order is not canonical");

  const cooling = module.energyPathGraphForState(explanation, { ...allState, simulationEnergyService: "cooling" });
  assert(cooling.nodes.some((node) => node.id === "driver.surface.exterior_walls.cooling.building"), "Single-service view lost the canonical service node");
  document.body.dataset.energyPathDriverTaxonomyStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathDriverTaxonomyStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
