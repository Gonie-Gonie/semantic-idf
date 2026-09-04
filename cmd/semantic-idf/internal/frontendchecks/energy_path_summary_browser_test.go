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

func TestEnergyPathSummaryV2AndV1AdapterBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser Energy Path summary harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-summary", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathSummaryHarnessHTML)
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
		server.URL+"/energy-path-summary",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("Energy Path summary browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("Energy Path summary browser harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-energy-path-summary-status="passed"`) {
		t.Fatalf("Energy Path summary v2/v1 contract failed in browser:\n%s", document)
	}
}

const energyPathSummaryHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Energy Path summary harness</title></head>
<body data-energy-path-summary-status="pending">
<pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => {
  if (!condition) throw new Error(message);
};
try {
  const summaryModule = await import("/src/js/energy-path-summary.js");
  const viewModule = await import("/src/js/views/energy-path-view.js");
  const v2 = {
    schema: "semantic-idf.energy-explanation-summary/v2",
    period: "annual",
    scope: { kind: "building", aggregationBasis: "model_total" },
    drivers: [{ id: "driver.people.cooling.building", label: "People", value: 30, rawValue: 35, allocatedValue: 30, unit: "kWh thermal", basis: "zone_air_heat_balance", aggregationBasis: "model_total" }],
    loads: [
      { id: "load.cooling.building", label: "Cooling", value: 100, serviceKind: "cooling", unit: "kWh thermal" },
      { id: "load.heating.building", label: "Heating", value: 40, serviceKind: "heating", unit: "kWh thermal" },
    ],
    endUses: [{ id: "end_use.cooling.building", label: "Cooling equipment", value: 25, unit: "kWh site" }],
    carriers: [
      { id: "carrier.electricity.building", label: "Electricity", value: 25, unit: "kWh site" },
      { id: "carrier.natural_gas.building", label: "Natural gas", value: 15, unit: "kWh site" },
    ],
    ratios: [{ id: "kpi.cooling_cop", label: "Cooling COP", value: 4, rawValue: 100, allocatedValue: 25, basis: "load_to_end_use" }],
    residuals: [{ id: "residual.cooling.building", label: "Cooling residual", value: 2, unit: "kWh thermal" }],
    topZones: [{ id: "zone.office", label: "Office", value: 18, unit: "kWh thermal" }],
    completeness: { mappedPercent: 88, status: "partial" },
    energyByCarrier: [{ id: "legacy-trap", label: "Legacy trap", value: 999 }],
    energyByEndUse: [{ id: "legacy-end-use-trap", value: 999 }],
    deliveredLoadByService: [{ id: "legacy-load-trap", value: 999 }],
    heatDrivers: [{ id: "legacy-driver-trap", value: 999 }],
  };
  const canonicalKeys = ["drivers", "loads", "endUses", "carriers", "ratios", "residuals", "topZones"];
  const v2Groups = summaryModule.energyPathSummaryGroups(v2);
  assert(JSON.stringify(v2Groups.map((group) => group.key)) === JSON.stringify(canonicalKeys), "v2 group order is not canonical");
  assert(!v2Groups.flatMap((group) => group.items).some((item) => item.id.includes("trap")), "v2 read a legacy summary key");
  assert(v2Groups[0].items[0].rawValue === 35 && v2Groups[0].items[0].allocatedValue === 30, "v2 raw/allocated values were lost");
  assert(v2Groups[0].items[0].basis === "zone_air_heat_balance", "v2 basis was lost");
  assert(JSON.stringify(summaryModule.energyPathSummaryGroups(v2, { comparison: true }).map((group) => group.key)) === JSON.stringify(canonicalKeys), "v2 summary-only comparison groups are incomplete");

  const kpis = Object.fromEntries(summaryModule.energyPathSummaryKPIValues(v2).map((item) => [item.id, item.value]));
  assert(kpis.total_site_energy === 40, "site-energy KPI did not use carriers");
  assert(kpis.cooling_load === 100 && kpis.heating_load === 40, "load KPIs did not use loads");
  assert(kpis.coverage === 88, "coverage KPI did not use completeness");

  const overview = viewModule.renderEnergyPathSummaryOverview(v2);
  const kpiHTML = viewModule.renderEnergyPathKPI(v2);
  assert(overview.includes("People") && overview.includes("Raw: 35 kWh thermal") && overview.includes("Allocated: 30 kWh thermal"), "v2 overview omitted summary detail");
  assert(!overview.includes("Legacy trap"), "v2 overview rendered a legacy collection");
  assert(kpiHTML.includes('data-energy-path-kpi="total_site_energy"') && kpiHTML.includes("40 kWh site"), "v2 KPI markup is not summary-driven");

  const v1 = {
    schema: "semantic-idf.energy-explanation-summary/v1",
    energyByCarrier: [{ id: "legacy-carrier", value: 12 }],
    energyByEndUse: [{ id: "legacy-end-use", value: 8 }],
    deliveredLoadByService: [{ id: "legacy-load", value: 20 }],
    derivedKpis: [{ id: "legacy-kpi", value: 2.5 }],
    heatDrivers: [{ id: "legacy-driver", value: 4 }],
    residuals: [{ id: "legacy-residual", value: 1 }],
    topHeatDrivers: [{ id: "legacy-top-driver", value: 4 }],
    topZones: [{ id: "legacy-zone", value: 3 }],
    drivers: [{ id: "canonical-trap", value: 999 }],
  };
  const v1Keys = ["energyByCarrier", "energyByEndUse", "deliveredLoadByService", "derivedKpis", "heatDrivers", "residuals", "topHeatDrivers", "topZones"];
  const v1Groups = summaryModule.energyPathSummaryGroups(v1);
  assert(JSON.stringify(v1Groups.map((group) => group.key)) === JSON.stringify(v1Keys), "v1 adapter group order changed");
  assert(v1Groups.flatMap((group) => group.items).some((item) => item.id === "legacy-carrier"), "v1 adapter did not retain legacy collections");
  assert(!v1Groups.flatMap((group) => group.items).some((item) => item.id === "canonical-trap"), "v1 adapter read a v2 collection");
  assert(summaryModule.energyPathSummaryKPIValues(v1).length === 0, "v1 bypassed its KPI fallback");
  assert(viewModule.renderEnergyPathSummaryOverview(v1) === "" && viewModule.renderEnergyPathKPI(v1) === "", "v1 bypassed its renderer fallback");

  document.body.dataset.energyPathSummaryStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathSummaryStatus = "failed";
  document.getElementById("result").textContent = error?.stack || error?.message || String(error);
}
</script>
</body>
</html>`
