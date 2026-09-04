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

func TestEnergyPathBuildingLoadPresentationReviewBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser Energy Path building-load presentation harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-building-load-presentation", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathBuildingLoadPresentationReviewHarnessHTML)
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
		server.URL+"/energy-path-building-load-presentation",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("Energy Path building-load presentation browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("Energy Path building-load presentation browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-building-load-presentation-status="passed"`) {
		t.Fatalf("Energy Path building-load presentation contract failed in browser:\n%s", document)
	}
}

const energyPathBuildingLoadPresentationReviewHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Energy Path building-load presentation harness</title></head>
<body data-energy-path-building-load-presentation-status="pending">
<div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const component = (name, value, share, sourceIds) => ({ component: name, value, share, unit: "kWh", sourceIds });
const load = (id, label, value, serviceKind, sensible, latent, badges = []) => ({
  id, level: "load", kind: "load.zone_" + serviceKind, label, value,
  rawValue: value, effectiveValue: value, allocatedValue: value,
  unit: "kWh", scaleDomain: "thermal", period: id.includes(".building") ? "annual" : "",
  serviceKind, driverCategory: "load." + serviceKind, thermalComponent: "combined",
  loadBreakdown: [
    component("sensible", sensible, sensible / value, [serviceKind + "-sensible"]),
    component("latent", latent, latent / value, [serviceKind + "-latent"]),
  ],
  latentShare: latent / value, badges,
  sourceIds: [serviceKind + "-sensible", serviceKind + "-latent"],
});
const period = (id, cooling, heating) => ({ id, kind: "monthly", nodes: [cooling, heating], links: [], warnings: [] });

try {
  const module = await import("/src/js/views/energy-path-view.js");
  const annualCooling = load("load.cooling.building", "Cooling load", 320, "cooling", 280, 40, ["dehumidification_significant"]);
  const annualHeating = load("load.heating.building", "Heating load", 100, "heating", 91, 9);
  const m1Cooling = load("load.cooling.building", "Cooling load", 240, "cooling", 222, 18);
  const m1Heating = load("load.heating.building", "Heating load", 50, "heating", 46, 4);
  const m2Cooling = load("load.cooling.building", "Cooling load", 80, "cooling", 58, 22, ["dehumidification_significant"]);
  const m2Heating = load("load.heating.building", "Heating load", 50, "heating", 45, 5, ["humidification_significant"]);

  const officeAnnualCooling = load("load.cooling.office", "Cooling load", 240, "cooling", 200, 40, ["dehumidification_significant"]);
  officeAnnualCooling.zoneName = "Office";
  officeAnnualCooling.sourceIds = ["office-cooling-sensible", "office-cooling-latent"];
  const officeAnnualHeating = load("load.heating.office", "Heating load", 100, "heating", 91, 9);
  officeAnnualHeating.zoneName = "Office";
  officeAnnualHeating.sourceIds = ["office-heating-sensible", "office-heating-latent"];
  const officeM1Cooling = load("load.cooling.office", "Cooling load", 200, "cooling", 182, 18);
  officeM1Cooling.zoneName = "Office";
  officeM1Cooling.sourceIds = ["office-cooling-sensible", "office-cooling-latent"];
  const officeM1Heating = load("load.heating.office", "Heating load", 50, "heating", 46, 4);
  officeM1Heating.zoneName = "Office";
  officeM1Heating.sourceIds = ["office-heating-sensible", "office-heating-latent"];
  const officeM2Cooling = load("load.cooling.office", "Cooling load", 40, "cooling", 18, 22, ["dehumidification_significant"]);
  officeM2Cooling.zoneName = "Office";
  officeM2Cooling.sourceIds = ["office-cooling-sensible", "office-cooling-latent"];
  const officeM2Heating = load("load.heating.office", "Heating load", 50, "heating", 45, 5, ["humidification_significant"]);
  officeM2Heating.zoneName = "Office";
  officeM2Heating.sourceIds = ["office-heating-sensible", "office-heating-latent"];

  const labCooling = load("load.cooling.lab", "Cooling load", 80, "cooling", 80, 0);
  labCooling.zoneName = "Lab";
  labCooling.loadBreakdown = [component("sensible", 80, 1, ["lab-cooling-sensible"])];
  labCooling.latentShare = 0;
  labCooling.sourceIds = ["lab-cooling-sensible"];
  const labHeating = load("load.heating.lab", "Heating load", 10, "heating", 10, 0);
  labHeating.zoneName = "Lab";
  labHeating.loadBreakdown = [component("sensible", 10, 1, ["lab-heating-sensible"])];
  labHeating.latentShare = 0;
  labHeating.sourceIds = ["lab-heating-sensible"];

  const explanation = {
    schema: module.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    availableZones: ["Office", "Lab"],
    nodes: [annualCooling, annualHeating],
    links: [],
    periods: [period("M1", m1Cooling, m1Heating), period("M2", m2Cooling, m2Heating)],
    zoneResults: [
      {
        scope: { kind: "zone", zoneName: "Office", aggregationBasis: "model_total" },
        nodes: [officeAnnualCooling, officeAnnualHeating], links: [],
        periods: [period("M1", officeM1Cooling, officeM1Heating), period("M2", officeM2Cooling, officeM2Heating)],
      },
      {
        scope: { kind: "zone", zoneName: "Lab", aggregationBasis: "model_total" },
        nodes: [labCooling, labHeating], links: [], periods: [],
      },
    ],
    sources: [
      { id: "office-cooling-sensible", name: "Office cooling sensible", zoneName: "Office", driverCategory: "load.cooling", driverComponent: "load.delivered.sensible", inspectorSection: "breakdown", driverRole: "main_flow" },
      { id: "office-cooling-latent", name: "Office cooling latent", zoneName: "Office", driverCategory: "load.cooling", driverComponent: "load.delivered.latent", inspectorSection: "breakdown", driverRole: "main_flow" },
      { id: "office-heating-sensible", name: "Office heating sensible", zoneName: "Office", driverCategory: "load.heating", driverComponent: "load.delivered.sensible", inspectorSection: "breakdown", driverRole: "main_flow" },
      { id: "office-heating-latent", name: "Office heating latent", zoneName: "Office", driverCategory: "load.heating", driverComponent: "load.delivered.latent", inspectorSection: "breakdown", driverRole: "main_flow" },
      { id: "lab-cooling-sensible", name: "Lab cooling sensible", zoneName: "Lab", driverCategory: "load.cooling", driverComponent: "load.delivered.sensible", inspectorSection: "breakdown", driverRole: "main_flow" },
      { id: "lab-heating-sensible", name: "Lab heating sensible", zoneName: "Lab", driverCategory: "load.heating", driverComponent: "load.delivered.sensible", inspectorSection: "breakdown", driverRole: "main_flow" },
      { id: "office-cooling-context", name: "Office cooling context", zoneName: "Office", driverCategory: "load.cooling", driverComponent: "load.predicted.sensible", inspectorSection: "context", driverRole: "context" },
      { id: "lab-cooling-context", name: "Lab cooling context", zoneName: "Lab", driverCategory: "load.cooling", driverComponent: "load.predicted.sensible", inspectorSection: "context", driverRole: "context" },
      { id: "office-heating-context", name: "Office heating context", zoneName: "Office", driverCategory: "load.heating", driverComponent: "load.predicted.sensible", inspectorSection: "context", driverRole: "context" },
    ],
  };
  const mount = document.getElementById("mount");
  const render = (state) => {
    mount.innerHTML = module.renderEnergyPathView(explanation, state);
    return mount;
  };
  const loadButtons = (root) => [...root.querySelectorAll('[data-energy-path-stage="load"] [data-energy-explanation-node]')];
  const button = (root, id) => root.querySelector('[data-energy-explanation-node="' + id + '"]');

  const annualState = {
    simulationEnergyScopeKind: "building", simulationEnergyPeriod: "annual",
    simulationEnergyService: "all", simulationEnergySelection: "load.cooling.building",
  };
  let root = render(annualState);
  let loads = loadButtons(root);
  assert(loads.length === 2, "annual Building Load stage must contain only Cooling and Heating: " + loads.map((item) => item.dataset.energyExplanationNode));
  assert(button(root, "load.cooling.building") && button(root, "load.heating.building"), "annual Building cooling/heating nodes are missing");
  assert(button(root, "load.cooling.building").querySelector('[data-energy-path-load-latent-badge="node"]'), ">=10% annual cooling latent share has no node badge");
  assert(!button(root, "load.heating.building").querySelector('[data-energy-path-load-latent-badge]'), "<10% annual heating latent share received a badge");
  let inspector = root.querySelector('[data-energy-path-inspector="load.cooling.building"]');
  assert(inspector?.querySelector('[data-energy-path-load-latent-badge="inspector"]'), ">=10% annual latent share has no inspector emphasis");
  assert(inspector?.querySelector('[data-energy-path-load-breakdown-component="latent"][data-energy-path-load-breakdown-emphasized="true"]'), "annual latent breakdown row is not emphasized");
  assert(inspector.textContent.includes("12.5%"), "annual inspector did not show the period-local 12.5% share");

  root = render({ ...annualState, simulationEnergyPeriod: "M1", simulationEnergySelection: "load.cooling.building" });
  assert(loadButtons(root).length === 2, "M1 Load stage gained a non-primary node");
  assert(!button(root, "load.cooling.building").querySelector('[data-energy-path-load-latent-badge]'), "M1 7.5% cooling share leaked the annual badge");
  inspector = root.querySelector('[data-energy-path-inspector="load.cooling.building"]');
  assert(!inspector?.querySelector('[data-energy-path-load-latent-badge]'), "M1 7.5% cooling share leaked annual inspector emphasis");
  assert(inspector?.querySelector('[data-energy-path-load-breakdown-component="latent"][data-energy-path-load-breakdown-emphasized="false"]'), "M1 latent row should remain un-emphasized");
  assert(inspector.textContent.includes("7.5%") && !inspector.textContent.includes("12.5%"), "M1 inspector reused the annual latent share");

  root = render({ ...annualState, simulationEnergyPeriod: "M2", simulationEnergySelection: "load.cooling.building" });
  assert(button(root, "load.cooling.building").querySelector('[data-energy-path-load-latent-badge="node"]'), "M2 27.5% cooling share has no badge");
  inspector = root.querySelector('[data-energy-path-inspector="load.cooling.building"]');
  assert(inspector?.querySelector('[data-energy-path-load-latent-badge="inspector"]') && inspector.textContent.includes("27.5%"), "M2 inspector did not use its period-local share");

  const officeState = {
    simulationEnergyScopeKind: "zone", simulationEnergyZoneName: "Office",
    simulationEnergyPeriod: "annual", simulationEnergyService: "cooling",
    simulationEnergySelection: "load.cooling.office",
  };
  root = render(officeState);
  inspector = root.querySelector('[data-energy-path-inspector="load.cooling.office"]');
  assert(inspector, "Office cooling inspector is missing");
  assert(inspector.textContent.includes("Office cooling sensible") && inspector.textContent.includes("Office cooling latent") && inspector.textContent.includes("Office cooling context"), "Office/cooling provenance is incomplete");
  assert(!inspector.textContent.includes("Lab cooling") && !inspector.textContent.includes("Office heating"), "zone or category provenance leaked into Office cooling inspector");
  assert(loadButtons(root).length === 1 && loadButtons(root)[0].dataset.energyExplanationNode === "load.cooling.office", "zone cooling filter rendered another load service");

  document.body.dataset.energyPathBuildingLoadPresentationStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathBuildingLoadPresentationStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
