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

func TestEPATH090EndUseCarrierSplitBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser EPATH-090 end-use/carrier harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-end-use-carrier", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathEndUseCarrierSplitHarnessHTML)
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
		server.URL+"/energy-path-end-use-carrier",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("EPATH-090 end-use/carrier browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("EPATH-090 end-use/carrier browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-end-use-carrier-status="passed"`) {
		t.Fatalf("EPATH-090 end-use/carrier frontend contract failed:\n%s", document)
	}
}

const energyPathEndUseCarrierSplitHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>EPATH-090 end-use/carrier split</title></head>
<body data-energy-path-end-use-carrier-status="pending">
<div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
const electricityMeter = "meter.heating.electricity";
const gasMeter = "meter.heating.natural_gas";
const electricityFacilityMeter = "meter.facility.electricity";
const gasFacilityMeter = "meter.facility.natural_gas";
const heatingID = "end_use.heating.building";
const electricityID = "carrier.electricity.building";
const gasID = "carrier.natural_gas.building";
const electricityLinkID = "link.heating.electricity";
const gasLinkID = "link.heating.natural_gas";

try {
  const module = await import("/src/js/views/energy-path-view.js");
  const explanation = {
    schema: module.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    nodes: [
      { id: heatingID, level: "end_use", kind: "end_use.heating", label: "Heating", value: 100, rawValue: 100, effectiveValue: 100, allocatedValue: 100, unit: "kWh", scaleDomain: "site", period: "annual", serviceKind: "heating", endUse: "heating", sourceIds: [electricityMeter, gasMeter] },
      { id: electricityID, level: "carrier", kind: "carrier.electricity", label: "Electricity", value: 40, rawValue: 40, effectiveValue: 40, allocatedValue: 40, unit: "kWh", scaleDomain: "site", period: "annual", carrier: "electricity", sourceIds: [electricityFacilityMeter] },
      { id: gasID, level: "carrier", kind: "carrier.natural_gas", label: "Natural gas", value: 60, rawValue: 60, effectiveValue: 60, allocatedValue: 60, unit: "kWh", scaleDomain: "site", period: "annual", carrier: "natural_gas", sourceIds: [gasFacilityMeter] },
    ],
    links: [
      { id: electricityLinkID, fromId: heatingID, toId: electricityID, relation: "end_use_to_carrier", basis: "reported_meter", fromValue: 40, fromUnit: "kWh", toValue: 40, toUnit: "kWh", period: "annual", serviceKind: "heating", sourceIds: [electricityMeter] },
      { id: gasLinkID, fromId: heatingID, toId: gasID, relation: "end_use_to_carrier", basis: "reported_meter", fromValue: 60, fromUnit: "kWh", toValue: 60, toUnit: "kWh", period: "annual", serviceKind: "heating", sourceIds: [gasMeter] },
    ],
    sources: [
      { id: electricityMeter, sourceType: "sql_meter", isMeter: true, keyValue: "Heating:Electricity", name: "Heating:Electricity", reportingFrequency: "Monthly" },
      { id: gasMeter, sourceType: "sql_meter", isMeter: true, keyValue: "Heating:NaturalGas", name: "Heating:NaturalGas", reportingFrequency: "Monthly" },
      { id: electricityFacilityMeter, sourceType: "sql_meter", isMeter: true, keyValue: "Electricity:Facility", name: "Electricity:Facility", reportingFrequency: "Monthly" },
      { id: gasFacilityMeter, sourceType: "sql_meter", isMeter: true, keyValue: "NaturalGas:Facility", name: "NaturalGas:Facility", reportingFrequency: "Monthly" },
    ],
  };
  const state = { simulationEnergyScopeKind: "building", simulationEnergyPeriod: "annual", simulationEnergyService: "heating", simulationEnergySelection: heatingID };
  const graph = module.energyPathGraphForState(explanation, state);
  const endUses = graph.nodes.filter((node) => node.level === "end_use");
  const carriers = graph.nodes.filter((node) => node.level === "carrier");
  const splits = graph.links.filter((link) => link.relation === "end_use_to_carrier" && link.fromId === heatingID);

  assert(endUses.length === 1 && endUses[0].id === heatingID, "carrier-qualified Heating meters became duplicate primary end-use nodes");
  assert(endUses[0].value === 100 && !endUses[0].carrier, "Heating is not a carrier-neutral sum of its carrier energy");
  assert(carriers.length === 2 && carriers.some((node) => node.id === electricityID) && carriers.some((node) => node.id === gasID), "end-use and carrier stages are not semantically distinct");
  assert(splits.length === 2, "one Heating end use did not preserve its two carrier branches");

  const electricityLink = splits.find((link) => link.toId === electricityID);
  const gasLink = splits.find((link) => link.toId === gasID);
  assert(electricityLink?.fromValue === 40 && electricityLink?.toValue === 40, "electricity split value was changed or lost");
  assert(gasLink?.fromValue === 60 && gasLink?.toValue === 60, "natural-gas split value was changed or lost");
  assert(JSON.stringify(electricityLink?.sourceIds) === JSON.stringify([electricityMeter]), "electricity split source IDs were broadened or contaminated");
  assert(JSON.stringify(gasLink?.sourceIds) === JSON.stringify([gasMeter]), "natural-gas split source IDs were broadened or contaminated");
  assert(!splits.some((link) => link.fromId.startsWith("carrier.") || link.toId.startsWith("end_use.")), "a carrier-to-end-use reverse link entered the v2 graph");
  assert(splits.reduce((total, link) => total + link.fromValue, 0) === endUses[0].value, "Heating total does not close to outgoing carrier splits");

  const mount = document.getElementById("mount");
  mount.innerHTML = module.renderEnergyPathView(explanation, state);
  const endUseStage = mount.querySelector('[data-energy-path-stage="end_use"]');
  const carrierStage = mount.querySelector('[data-energy-path-stage="carrier"]');
  assert(endUseStage && carrierStage && endUseStage !== carrierStage, "End-use Energy and Energy Source collapsed into one rendered stage");
  assert(endUseStage.querySelectorAll('[data-energy-explanation-node]').length === 1, "rendered end-use stage contains duplicate carrier-qualified Heating nodes");
  assert(endUseStage.querySelector('[data-energy-explanation-node="' + heatingID + '"]')?.textContent.includes("100"), "rendered Heating node does not show the all-carrier total");
  assert(carrierStage.querySelector('[data-energy-explanation-node="' + electricityID + '"]')?.textContent.includes("40"), "Electricity branch is absent from the carrier stage");
  assert(carrierStage.querySelector('[data-energy-explanation-node="' + gasID + '"]')?.textContent.includes("60"), "Natural-gas branch is absent from the carrier stage");

  const inspector = mount.querySelector('[data-energy-path-inspector="' + heatingID + '"]');
  assert(inspector && !inspector.querySelector('[data-energy-path-detail-section="sources"], [data-energy-path-detail-section="entities"], [data-energy-path-source]'), "Heating inspector retained Source data or Related model entities");
  const heatingSources = module.energyPathInspectorSources(explanation, endUses[0], state);
  assert(heatingSources.some((source) => source.id === electricityMeter && source.name === "Heating:Electricity"), "Heating selection lost its electricity source meter");
  assert(heatingSources.some((source) => source.id === gasMeter && source.name === "Heating:NaturalGas"), "Heating selection lost its natural-gas source meter");
  assert(heatingSources.length === 2, "Heating selection broadened end-use provenance beyond its two source meters");

  const v1 = { schema: "semantic-idf.energy-explanation/v1", nodes: [], edges: [] };
  assert(module.isEnergyPathV2(v1) === false, "v1 payload was routed into the v2 Energy Path contract");

  document.body.dataset.energyPathEndUseCarrierStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathEndUseCarrierStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
