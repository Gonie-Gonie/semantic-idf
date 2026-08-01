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

func TestThermalTopologyLayoutBrowserHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser thermal topology layout harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/thermal-topology-layout", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, thermalTopologyLayoutHarnessHTML)
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
		"--virtual-time-budget=30000",
		"--user-data-dir="+t.TempDir(),
		"--dump-dom",
		server.URL+"/thermal-topology-layout",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("thermal topology layout harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("thermal topology layout harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-thermal-layout-status="passed"`) {
		t.Fatalf("thermal topology layout harness did not pass:\n%s", document)
	}
	for _, signal := range []string{`"deterministic":true`, `"portsAvoidCenters":true`, `"neighborNodes":3`, `"selfLoop":true`} {
		if !strings.Contains(document, signal) {
			t.Fatalf("thermal topology layout result is missing %s:\n%s", signal, document)
		}
	}
}

const thermalTopologyLayoutHarnessHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Thermal topology layout harness</title></head>
<body data-thermal-layout-status="pending"><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
try {
  const layout = await import("/src/js/views/thermal-topology-layout.js");
  const geometry = { topology: {
    schema: "semantic-idf.thermal-topology/v1",
    nodes: [
      { id: "zone:a", kind: "zone", label: "A", storyIndex: 0, centroid: { x: 0, y: 0, z: 0 } },
      { id: "zone:b", kind: "zone", label: "B", storyIndex: 0, centroid: { x: 10, y: 0, z: 0 } },
      { id: "zone:c", kind: "zone", label: "C", storyIndex: 1, centroid: { x: 5, y: 8, z: 3 } },
      { id: "thermal-environment:outdoors", kind: "outdoors", label: "Outdoors" },
      { id: "thermal-environment:ground", kind: "ground", label: "Ground" },
    ],
    connections: [
      { id: "edge:ab", fromNodeId: "zone:a", toNodeId: "zone:b", relationKind: "interzone", surfaceCount: 2, effectiveGrossArea: 20 },
      { id: "edge:bc", fromNodeId: "zone:b", toNodeId: "zone:c", relationKind: "interzone", surfaceCount: 2, effectiveGrossArea: 18 },
      { id: "edge:out", fromNodeId: "zone:a", toNodeId: "thermal-environment:outdoors", relationKind: "outdoors", orientations: ["North"], surfaceCount: 1 },
      { id: "edge:ground", fromNodeId: "zone:c", toNodeId: "thermal-environment:ground", relationKind: "ground", surfaceCount: 1 },
      { id: "edge:loop", fromNodeId: "zone:b", toNodeId: "zone:b", relationKind: "adiabatic", surfaceCount: 1 },
    ],
    boundaries: [], openings: [], airCouplings: [],
  }};
  const options = { graphLevel: "zone", layout: "spatial", scope: "building", areaBasis: "effective", selectedEntityId: "zone:b", neighborDepth: 1 };
  const model = layout.createThermalTopologyLayoutModel(geometry, options);
  const first = layout.computeThermalTopologyLayout(model, { width: 900, height: 600 });
  const second = layout.computeThermalTopologyLayout(model, { width: 900, height: 600 });
  const deterministic = JSON.stringify(first) === JSON.stringify(second);
  assert(deterministic, "layout is not deterministic");
  const ab = first.edges.find((edge) => edge.id === "edge:ab");
  const portsAvoidCenters = ab.route.sourcePort !== "center" && ab.route.targetPort !== "center" && /C/.test(ab.route.path);
  assert(portsAvoidCenters, "edge routing used node centers");
  assert(first.positions["thermal-environment:ground"].y > first.positions["zone:c"].y, "ground node is not below zones");
  const neighborModel = layout.createThermalTopologyLayoutModel(geometry, { ...options, scope: "neighbors", selectedEntityId: "zone:a" });
  const neighborNodes = neighborModel.nodes.filter((node) => node.kind === "zone").length;
  assert(neighborNodes === 2, "one-hop scope did not isolate selected neighbors");
  const selfLoop = first.edges.find((edge) => edge.id === "edge:loop").route.selfLoop;
  assert(selfLoop, "adiabatic self-loop route missing");
  document.getElementById("result").textContent = JSON.stringify({ deterministic, portsAvoidCenters, neighborNodes: neighborModel.nodes.length, selfLoop });
  document.body.dataset.thermalLayoutStatus = "passed";
} catch (error) {
  document.getElementById("result").textContent = error.stack || String(error);
  document.body.dataset.thermalLayoutStatus = "failed";
}
</script></body></html>`
