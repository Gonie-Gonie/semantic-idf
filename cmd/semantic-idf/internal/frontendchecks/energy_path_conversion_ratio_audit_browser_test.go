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

func TestEPATH092AuditConversionRatioBrowserBoundaries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser EPATH-092 ratio-boundary audit in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-conversion-ratio-audit", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, epath092ConversionRatioAuditHarnessHTML)
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
		server.URL+"/energy-path-conversion-ratio-audit",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("EPATH-092 ratio-boundary browser audit timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("EPATH-092 ratio-boundary browser audit failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-epath092-ratio-audit="passed"`) {
		t.Fatalf("EPATH-092 ratio-boundary frontend contract failed:\n%s", document)
	}
}

const epath092ConversionRatioAuditHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>EPATH-092 ratio-boundary audit</title></head>
<body data-epath092-ratio-audit="pending"><main id="mount"></main><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };

try {
  const module = await import("/src/js/views/energy-path-view.js");
  const base = {
    id: "conversion.cooling",
    fromId: "load.cooling.building",
    toId: "end_use.cooling.building",
    relation: "load_to_end_use",
    ratioKind: "coefficient_of_performance",
    serviceKind: "cooling",
    fromUnit: "kWh thermal",
    toUnit: "kWh site",
  };

  // Go serializes these ratios after roundedEnergyNumber(...), i.e. to three
  // decimal places. The UI must accept that exact wire contract.
  const rounded = module.energyPathConversionRatio({ ...base, fromValue: 160, toValue: 45, ratio: 3.556 });
  assert(rounded?.kind === "coefficient_of_performance" && rounded?.label === "COP" && rounded?.value === 3.556, "valid backend-rounded COP was hidden");
  const repeating = module.energyPathConversionRatio({ ...base, fromValue: 100, toValue: 30, ratio: 3.333 });
  assert(repeating?.label === "COP", "valid repeating backend-rounded ratio was hidden");

  const fuelAboveOne = module.energyPathConversionRatio({
    ...base,
    ratioKind: "load_to_fuel",
    serviceKind: "heating",
    fromValue: 120,
    toValue: 100,
    ratio: 1.2,
  });
  assert(fuelAboveOne?.label === "Load / fuel", "valid load/fuel ratio above one was hidden");
  const district = module.energyPathConversionRatio({
    ...base,
    ratioKind: "load_to_purchased_energy",
    fromValue: 75,
    toValue: 25,
    ratio: 3,
  });
  assert(district?.label === "Load / purchased energy", "valid district-energy ratio was hidden");

  const invalid = [
    { ...base, fromValue: 100, toValue: 25, ratio: 4, fromUnit: "kWh thermal", toUnit: "MJ site" },
    { ...base, fromValue: 100, toValue: 25, ratio: 4, fromUnit: "widgets", toUnit: "widgets" },
    { ...base, ratioKind: "efficiency", serviceKind: "heating", fromValue: 120, toValue: 100, ratio: 1.2 },
    { ...base, fromValue: 160, toValue: 45, ratio: 3.55 },
    { ...base, fromValue: Number.NaN, toValue: 25, ratio: 4 },
    { ...base, fromValue: 100, toValue: Number.POSITIVE_INFINITY, ratio: 4 },
    { ...base, fromValue: -100, toValue: 25, ratio: -4 },
  ];
  for (const link of invalid) {
    assert(module.energyPathConversionRatio(link) === null, "invalid conversion exposed a ratio label: " + String(link.ratioKind));
  }

  const nodes = [
    { id: base.fromId, level: "load", label: "Cooling load", value: 160, unit: "kWh thermal", scaleDomain: "thermal", serviceKind: "cooling" },
    { id: base.toId, level: "end_use", label: "Cooling equipment", value: 45, unit: "kWh site", scaleDomain: "site", endUse: "cooling" },
  ];
  const links = [{ ...base, fromValue: 160, toValue: 45, ratio: 3.556 }];
  const mount = document.getElementById("mount");
  mount.innerHTML = module.renderEnergyPathFlowLanes(nodes, links);
  const ratioBadge = mount.querySelector('[data-energy-path-ratio-kind="coefficient_of_performance"]');
  assert(ratioBadge?.dataset.energyPathRatioValue === "3.556" && ratioBadge?.textContent.includes("COP"), "rounded ratio did not render in the conversion lane");

  mount.innerHTML = module.renderEnergyPathFlowLanes(nodes, [{ ...base, fromValue: 100, toValue: 25, ratio: 4, toUnit: "MJ site" }]);
  assert(!mount.querySelector("[data-energy-path-ratio-kind]"), "unit-mismatched ratio rendered in the conversion lane");

  document.body.dataset.epath092RatioAudit = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.epath092RatioAudit = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
