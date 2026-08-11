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
	for _, signal := range []string{`"deterministic":true`, `"noOverlap":true`, `"portsAvoidCenters":true`, `"externalPlacement":true`, `"parallelOffset":true`, `"selectionCacheStable":true`, `"scopeCacheChanges":true`, `"neighborNodes":3`, `"selfLoop":true`} {
		if !strings.Contains(document, signal) {
			t.Fatalf("thermal topology layout result is missing %s:\n%s", signal, document)
		}
	}
}

func TestThermalTopologyRendererBrowserHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser thermal topology renderer harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/thermal-topology-renderer", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, thermalTopologyRendererHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check",
		"--virtual-time-budget=30000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/thermal-topology-renderer",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("thermal topology renderer harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("thermal topology renderer harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-thermal-renderer-status="passed"`) {
		t.Fatalf("thermal topology renderer harness did not pass:\n%s", document)
	}
	for _, signal := range []string{`"svg":true`, `"metricWidth":true`, `"inspector":true`, `"accessibleTargets":true`, `"deterministicTabOrder":true`, `"keyboardNode":true`, `"inspectorHeading":true`, `"patternLegend":true`, `"zoneOnly":true`, `"grossOnly":true`} {
		if !strings.Contains(document, signal) {
			t.Fatalf("thermal topology renderer result is missing %s:\n%s", signal, document)
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
    sourceModelHash: "fixture-hash",
    nodes: [
      { id: "zone:a", kind: "zone", label: "A", storyIndex: 0, centroid: { x: 0, y: 0, z: 0 } },
      { id: "zone:b", kind: "zone", label: "B", storyIndex: 0, centroid: { x: 10, y: 0, z: 0 } },
      { id: "zone:c", kind: "zone", label: "C", storyIndex: 1, centroid: { x: 5, y: 8, z: 3 } },
      { id: "thermal-environment:outdoors", kind: "outdoors", label: "Outdoors" },
      { id: "thermal-environment:ground", kind: "ground", label: "Ground" },
    ],
    connections: [
      { id: "edge:ab", fromNodeId: "zone:a", toNodeId: "zone:b", relationKind: "interzone", surfaceCount: 2, physicalGrossArea: 20 },
      { id: "edge:ab:air", fromNodeId: "zone:a", toNodeId: "zone:b", relationKind: "air_coupling", airCouplingIds: ["air:ab"] },
      { id: "edge:bc", fromNodeId: "zone:b", toNodeId: "zone:c", relationKind: "interzone", surfaceCount: 2, physicalGrossArea: 18 },
      { id: "edge:out", fromNodeId: "zone:a", toNodeId: "thermal-environment:outdoors", relationKind: "outdoors", orientations: ["North"], surfaceCount: 1 },
      { id: "edge:ground", fromNodeId: "zone:c", toNodeId: "thermal-environment:ground", relationKind: "ground", surfaceCount: 1 },
      { id: "edge:loop", fromNodeId: "zone:b", toNodeId: "zone:b", relationKind: "adiabatic", surfaceCount: 1 },
    ],
    boundaries: [], openings: [], airCouplings: [],
  }};
  const options = { layout: "spatial", scope: "building", selectedEntityId: "zone:b", showAirCoupling: true };
  const model = layout.createThermalTopologyLayoutModel(geometry, options);
  const first = layout.computeThermalTopologyLayout(model, { width: 900, height: 600 });
  const second = layout.computeThermalTopologyLayout(model, { width: 900, height: 600 });
  const deterministic = JSON.stringify(first) === JSON.stringify(second);
  assert(deterministic, "layout is not deterministic");
  const internalPositions = ["zone:a", "zone:b", "zone:c"].map((id) => first.positions[id]);
  const noOverlap = internalPositions.every((left, index) => internalPositions.slice(index + 1).every((right) => Math.abs(left.x - right.x) >= layout.THERMAL_NODE_WIDTH || Math.abs(left.y - right.y) >= layout.THERMAL_NODE_HEIGHT));
  assert(noOverlap, "fixture nodes overlap");
  const ab = first.edges.find((edge) => edge.id === "edge:ab");
  const portsAvoidCenters = ab.route.sourcePort !== "center" && ab.route.targetPort !== "center" && /C/.test(ab.route.path);
  assert(portsAvoidCenters, "edge routing used node centers");
  const externalPlacement = first.positions["thermal-environment:ground"].y > first.positions["zone:c"].y && first.positions["thermal-environment:outdoors"].x > first.positions["zone:a"].x;
  assert(externalPlacement, "external nodes are not outside the zone field");
  const parallelOffset = ab.route.path !== first.edges.find((edge) => edge.id === "edge:ab:air").route.path;
  assert(parallelOffset, "conductive and air paths overlap");
  const selectionCacheStable = layout.thermalTopologyLayoutCacheKey(geometry, options, {width:900,height:600}) === layout.thermalTopologyLayoutCacheKey(geometry, {...options,selectedEntityId:"zone:a"}, {width:900,height:600});
  const scopeCacheChanges = layout.thermalTopologyLayoutCacheKey(geometry, {...options,scope:"neighbors",selectedEntityId:"zone:a"}, {width:900,height:600}) !== layout.thermalTopologyLayoutCacheKey(geometry, {...options,scope:"neighbors",selectedEntityId:"zone:b"}, {width:900,height:600});
  assert(selectionCacheStable && scopeCacheChanges, "selection/layout cache boundary is incorrect");
  const neighborModel = layout.createThermalTopologyLayoutModel(geometry, { ...options, scope: "neighbors", selectedEntityId: "zone:a" });
  const neighborNodes = neighborModel.nodes.filter((node) => node.kind === "zone").length;
  assert(neighborNodes === 2, "one-hop scope did not isolate selected neighbors");
  const selfLoop = first.edges.find((edge) => edge.id === "edge:loop").route.selfLoop;
  assert(selfLoop, "adiabatic self-loop route missing");
  document.getElementById("result").textContent = JSON.stringify({ deterministic, noOverlap, portsAvoidCenters, externalPlacement, parallelOffset, selectionCacheStable, scopeCacheChanges, neighborNodes: neighborModel.nodes.length, selfLoop });
  document.body.dataset.thermalLayoutStatus = "passed";
} catch (error) {
  document.getElementById("result").textContent = error.stack || String(error);
  document.body.dataset.thermalLayoutStatus = "failed";
}
</script></body></html>`

const thermalTopologyRendererHarnessHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Thermal topology renderer harness</title></head>
<body data-thermal-renderer-status="pending">
<div id="thermalTopologyGraph" style="width:900px;height:600px"></div><aside id="thermalTopologyInspector"></aside>
<select id="thermalTopologyMetric"><option value="topology">topology</option><option value="area">area</option><option value="ua">ua</option><option value="qa">qa</option><option value="air">air</option></select>
<select id="thermalTopologyScope"><option value="building">building</option><option value="selection">selection</option></select>
<select id="thermalTopologyLayout"><option value="spatial">spatial</option><option value="network">network</option></select>
<input id="thermalTopologyShowAirCoupling" type="checkbox"><input id="thermalTopologyExpandExternalTargets" type="checkbox">
<pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
try {
  const stateModule = await import("/src/js/state.js");
  const view = await import("/src/js/views/thermal-topology-view.js");
  const geometry = { topology: {
    schema: "semantic-idf.thermal-topology/v1",
    nodes: [
      { id: "zone:a", entityId: "zone:a", kind: "zone", label: "Zone A", storyIndex: 0, floorArea: 40, volume: 120, centroid: {x:0,y:0,z:0} },
      { id: "thermal-environment:outdoors", kind: "outdoors", label: "Outdoors" },
    ],
    boundaries: [{ id:"thermal-boundary:surface:a", surfaceId:"surface:a", surfaceEntityId:"surface:a", surfaceName:"Wall A", surfaceType:"Wall", ownerZoneId:"zone:a", relationKind:"outdoors", targetId:"thermal-environment:outdoors", targetName:"Outdoors", constructionName:"Wall Construction", constructionObjectIndex:3, hasUValue:true, uValue:0.4, physicalGrossArea:10, physicalOpaqueArea:8, physicalOpeningArea:2, effectiveGrossArea:20, effectiveOpaqueArea:16, effectiveOpeningArea:4, opaqueUa:6.4, openingUa:4, totalUa:10.4, hasUa:true, orientation:"North", openingIds:["thermal-opening:window:a"], diagnosticIds:["topology-issue:a"], sourceAnchors:[{objectIndex:1,objectType:"BuildingSurface:Detailed"}] }],
    openings: [{ id:"thermal-opening:window:a", windowId:"window:a", entityId:"window:a", name:"Window A", baseSurfaceId:"surface:a", constructionName:"Window", hasUValue:true, uValue:1, physicalArea:2, effectiveArea:4, ua:4, hasUa:true }],
    connections: [{ id:"thermal-connection:a:outdoors", fromNodeId:"zone:a", toNodeId:"thermal-environment:outdoors", relationKind:"outdoors", boundaryIds:["thermal-boundary:surface:a"], openingIds:["thermal-opening:window:a"], surfaceCount:1, openingCount:1, physicalGrossArea:10, physicalOpaqueArea:8, physicalOpeningArea:2, effectiveGrossArea:20, effectiveOpaqueArea:16, effectiveOpeningArea:4, opaqueUa:6.4, openingUa:4, totalUa:10.4, hasUa:true, physicalOpaqueUa:3.2, physicalOpeningUa:2, physicalTotalUa:5.2, hasPhysicalUa:true, orientations:["North"], diagnosticIds:["topology-issue:a"] }],
    airCouplings: [], zoneSignatures:[{zoneId:"zone:a",zoneName:"Zone A",exteriorArea:20,groundArea:0,interzoneArea:0,adiabaticArea:0,otherBoundaryArea:0,exteriorUa:10.4,totalUa:10.4,hasTotalUa:true,closedShell:false,openEdgeCount:1}],
    issueLinks:[{id:"topology-issue:a",code:"surface_pair_area_mismatch",severity:"warning",message:"Area mismatch",boundaryId:"thermal-boundary:surface:a"}], adjacencyObservations:[],
  }};
  const state = stateModule.state;
  state.geometryMode = "thermal"; state.report = {geometry}; state.thermalTopologyMetric = "area"; state.thermalTopologyScope = "building"; state.thermalTopologySelectedEntityId = "thermal-connection:a:outdoors"; state.selectedGeometryKind = "thermal_connection"; state.selectedGeometryId = "thermal-connection:a:outdoors";
  const helpers = { navigationAttributes: () => 'data-entity-id="test"', selectGeometry: async () => true, setGeometryMode: () => {} };
  view.renderThermalTopology(geometry, helpers);
  const svg = Boolean(document.querySelector(".thermal-topology-svg"));
  const metricWidth = /--thermal-edge-width:(?!2\.00)/.test(document.querySelector(".thermal-edge").getAttribute("style"));
  const tableHeaders = [...document.querySelectorAll("#thermalTopologyInspector .thermal-inspector-table thead th")].map((cell) => cell.textContent.trim());
  const tableRows = new Map([...document.querySelectorAll("#thermalTopologyInspector .thermal-inspector-table tbody tr")].map((row) => {
    const cells = [...row.querySelectorAll("th,td")].map((cell) => cell.textContent.trim());
    return [cells[0], {value:cells[1],unit:cells[2]}];
  }));
  const sourceBoundary = document.querySelector('[data-thermal-inspector-kind="thermal_boundary"][data-thermal-inspector-id="thermal-boundary:surface:a"]');
  const removedInspectorUI = !document.querySelector('[data-inspector-semantic],[data-inspector-source],[data-inspector-diagnostic],[data-inspector-mode],.thermal-inspector-actions');
  const inspector = tableHeaders.join() === "Variable,Value,Unit" && tableRows.get("Multiplier")?.value === "2" && tableRows.get("Gross area")?.value === "10" && tableRows.get("Total UA")?.value === "5.2" && Boolean(sourceBoundary) && removedInspectorUI;
  const targets = [...document.querySelectorAll(".thermal-edge-group[tabindex='0'], .thermal-node[tabindex='0']")];
  const accessibleTargets = targets.length === 3 && targets.every((target) => ["entity", "relation", "metric", "issues"].every((term) => target.getAttribute("aria-label").includes(term)));
  const edgeOrder = [...document.querySelectorAll(".thermal-edge-group")].map((target) => target.dataset.thermalTargetId);
  const nodeOrder = [...document.querySelectorAll(".thermal-node")].map((target) => target.dataset.thermalTargetId);
  const deterministicTabOrder = edgeOrder.join() === [...edgeOrder].sort().join() && nodeOrder.join() === [...nodeOrder].sort().join();
  const zoneTarget = document.querySelector(".thermal-node[data-thermal-target-id='zone:a']");
  zoneTarget.focus(); zoneTarget.dispatchEvent(new KeyboardEvent("keydown", {key:"Enter",bubbles:true}));
  const keyboardNode = state.thermalTopologySelectedEntityId === "zone:a";
  const inspectorHeading = document.getElementById("thermalTopologyInspector").getAttribute("aria-labelledby") === "thermalTopologyInspectorHeading" && Boolean(document.getElementById("thermalTopologyInspectorHeading"));
  const patternLegend = document.querySelectorAll(".thermal-legend-line").length >= 4;
  document.querySelector(".thermal-edge-group").dispatchEvent(new KeyboardEvent("keydown", {key:"Enter",bubbles:true}));
  const zoneOnly = state.thermalTopologySelectedEntityKind === "thermal_connection" && !document.querySelector(".thermal-node.thermal_boundary") && !document.querySelector("[data-topology-back]") && !document.getElementById("thermalTopologyGraphLevel");
  const grossOnly = !document.getElementById("thermalTopologyAreaComponent") && !document.getElementById("thermalTopologyAreaBasis") && tableRows.get("Gross area")?.value === "10" && tableRows.get("Multiplier")?.value === "2";
  state.thermalTopologyMetric = "ua"; view.renderThermalTopology(geometry, helpers);

  assert(svg && metricWidth && inspector && accessibleTargets && deterministicTabOrder && keyboardNode && inspectorHeading && patternLegend && zoneOnly && grossOnly, "renderer contract failed");
  document.getElementById("result").textContent = JSON.stringify({svg,metricWidth,inspector,accessibleTargets,deterministicTabOrder,keyboardNode,inspectorHeading,patternLegend,zoneOnly,grossOnly});
  document.body.dataset.thermalRendererStatus = "passed";
} catch (error) {
  document.getElementById("result").textContent = error.stack || String(error);
  document.body.dataset.thermalRendererStatus = "failed";
}
</script></body></html>`
