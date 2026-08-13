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

func TestHVACLoopNavigationBrowserHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser HVAC loop navigation harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	index := readTestFile(t, "frontend/src/index.html")
	hvacMain := sliceBetween(index, `<section class="hvac-main"`, `<aside class="hvac-side"`)
	if !strings.Contains(hvacMain, `id="hvacViewportActions"`) || !strings.Contains(hvacMain, `id="hvacGraph"`) {
		t.Fatal("HVAC main viewport fragment is missing its fixed actions or graph")
	}
	harness := strings.Replace(hvacLoopNavigationHarnessHTML, "{{HVAC_MAIN}}", hvacMain, 1)

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/hvac-loop-navigation", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, harness)
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
		server.URL+"/hvac-loop-navigation",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("HVAC loop navigation harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("HVAC loop navigation harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-hvac-loop-navigation-status="passed"`) {
		t.Fatalf("HVAC loop navigation harness did not pass:\n%s", document)
	}
	for _, signal := range []string{
		`"fourCards":true`,
		`"onlyCanonicalCards":true`,
		`"emptyOtherVisible":true`,
		`"airDiagram":true`,
		`"loopViewportMarkup":true`,
		`"ctrlWheelLocal":true`,
		`"metaWheelLocal":true`,
		`"ctrlWheelStopped":true`,
		`"normalWheelPasses":true`,
		`"outsideCtrlWheelPasses":true`,
		`"zoomBounds":true`,
		`"sameKeyRerenderRetainsViewport":true`,
		`"wheelHistoryNeutral":true`,
		`"plantDiagram":true`,
		`"loopKeySwitchResetsViewport":true`,
		`"otherDiagram":true`,
		`"diagramStructure":true`,
		`"backRestoresPlant":true`,
		`"forwardRestoresOther":true`,
		`"zoneServicesReturn":true`,
		`"dropdownFilters":true`,
		`"compactDropdownWidths":true`,
		`"wrongFilterRecoverable":true`,
		`"fixedFocusedView":true`,
		`"legacyGraphChoicesAbsent":true`,
		`"narrowToolbarNotClipped":true`,
		`"serviceViewportMarkup":true`,
		`"serviceCtrlWheelLocal":true`,
		`"fitButtonResetsViewport":true`,
		`"legacyCouplingsCanonicalized":true`,
		`"documentResetResetsViewport":true`,
	} {
		if !strings.Contains(document, signal) {
			t.Fatalf("HVAC loop navigation result is missing %s:\n%s", signal, document)
		}
	}
}

const hvacLoopNavigationHarnessHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>HVAC loop navigation harness</title>
  <link rel="stylesheet" href="/src/styles/base.css">
  <link rel="stylesheet" href="/src/styles/hvac.css">
  <style>
    body { margin: 0; padding: 20px; }
    #hvacSummary { width: 1120px; }
    .hvac-layout { display: block; width: 1120px; height: 760px; }
    .hvac-main { width: 1120px; height: 760px; }
  </style>
</head>
<body data-hvac-loop-navigation-status="pending">
  <span id="runtimeStatus"></span>
  <div id="hvacSummary" class="hvac-summary"></div>
  <div class="hvac-layout">
    {{HVAC_MAIN}}
    <aside id="hvacSide">
      <span id="hvacInspectorStats"></span>
      <div id="hvacInspector"></div>
    </aside>
  </div>
  <pre id="result">pending</pre>
  <script type="module">
    const resultElement = document.getElementById("result");
    const assert = (condition, message) => {
      if (!condition) throw new Error(message);
    };
    const settle = () => new Promise((resolve) => setTimeout(resolve, 0));
    const changeSelect = async (select, value) => {
      assert(select, "filter select missing");
      select.value = value;
      select.dispatchEvent(new Event("change", { bubbles: true }));
      await settle();
    };
    const closeEnough = (left, right, epsilon = 0.000001) => Math.abs(Number(left) - Number(right)) <= epsilon;
    const diagramState = (state) => ({
      key: state.hvacDiagramViewportKey || "",
      scale: Number(state.hvacDiagramScale),
      panX: Number(state.hvacDiagramPanX),
      panY: Number(state.hvacDiagramPanY),
    });
    const isDefaultDiagramState = (state) => {
      const viewport = diagramState(state);
      return closeEnough(viewport.scale, 1) && closeEnough(viewport.panX, 0) && closeEnough(viewport.panY, 0);
    };
    const dispatchWheel = (target, options = {}) => {
      const bounds = target.getBoundingClientRect();
      const event = new WheelEvent("wheel", {
        bubbles: true,
        cancelable: true,
        deltaY: options.deltaY ?? -120,
        ctrlKey: Boolean(options.ctrlKey),
        metaKey: Boolean(options.metaKey),
        clientX: options.clientX ?? bounds.left + bounds.width * 0.72,
        clientY: options.clientY ?? bounds.top + bounds.height * 0.64,
      });
      const allowed = target.dispatchEvent(event);
      return { allowed, prevented: event.defaultPrevented };
    };
    const diagramMarkupReady = (svg) => Boolean(svg?.dataset.hvacDiagramKey)
      && Boolean(svg.querySelector("[data-hvac-diagram-content].hvac-diagram-panzoom"));
    const component = (name, type, inlet, outlet) => ({
      objectType: type,
      objectName: name,
      displayLabel: name,
      objectIndex: 10,
      exists: true,
      inletNode: inlet,
      outletNode: outlet,
    });
    const makeLoop = (id, type, name, supplyType, demandType) => ({
      id,
      type,
      name,
      objectIndex: 1,
      supplySide: {
        name: "Supply side",
        inletNode: name + " Supply Inlet",
        outletNode: name + " Supply Outlet",
        connectors: [],
        branches: [{
          name: name + " Supply Branch",
          objectIndex: 2,
          inletNode: name + " Supply Inlet",
          outletNode: name + " Supply Outlet",
          components: [component(name + " Supply Equipment", supplyType, name + " Supply Inlet", name + " Supply Outlet")],
        }],
      },
      demandSide: {
        name: "Demand side",
        inletNode: name + " Demand Inlet",
        outletNode: name + " Demand Outlet",
        connectors: [],
        branches: [{
          name: name + " Demand Branch",
          objectIndex: 3,
          inletNode: name + " Demand Inlet",
          outletNode: name + " Demand Outlet",
          components: [component(name + " Demand Equipment", demandType, name + " Demand Inlet", name + " Demand Outlet")],
        }],
      },
      relatedZones: ["Zone A"],
      relatedLoops: [],
      warnings: [],
    });

    try {
      const stateModule = await import("/src/js/state.js");
      const view = await import("/src/js/views/hvac-views.js");
      const state = stateModule.state;
      const air = makeLoop("air-1", "AirLoopHVAC", "Main Air Loop", "Fan:VariableVolume", "AirTerminal:SingleDuct:VAV:Reheat");
      const plant = makeLoop("plant-1", "PlantLoop", "Chilled Water Loop", "Pump:VariableSpeed", "Coil:Cooling:Water");
      const other = makeLoop("condenser-1", "CondenserLoop", "Condenser Water Loop", "Pump:ConstantSpeed", "CoolingTower:SingleSpeed");
      const servicePath = {
        id: "service-path-1",
        pathType: "central_air",
        serviceKind: "cooling",
        servedSubject: { kind: "zone", zoneName: "Zone A", name: "Zone A" },
        airLoop: { id: "air-1", type: "AirLoopHVAC", name: "Main Air Loop" },
        delivery: {
          id: "terminal-1",
          objectType: "AirTerminal:SingleDuct:VAV:Reheat",
          objectName: "Zone A Terminal",
          displayName: "Zone A Terminal",
          deliveryType: "air_terminal",
          inletNode: "Zone A Inlet",
          outletNode: "Zone A Outlet",
        },
        deliveryEquipment: { deliveryType: "air_terminal", displayFamily: "Air terminal" },
        conditioning: [],
        supportingCouplingIds: [],
        traceIds: [],
      };
      const hvac = {
        loops: [air, plant],
        airLoopCount: 1,
        plantLoopCount: 1,
        condenserLoopCount: 0,
        nodeUsages: [],
        nodeOutputMonitors: [],
        serviceModel: {
          zoneServices: [{
            zoneName: "Zone A",
            servedSubject: servicePath.servedSubject,
            paths: [servicePath],
          }],
          systems: [],
          components: [],
          couplings: [],
          networks: [],
          navigation: { entities: [], links: [], byLoop: {} },
        },
      };
      state.report = { hvac };
      state.activeResultTab = "hvac";
      state.activeHVACView = "services";
      state.activeHVACLoopId = "";
      state.activeHVACGraphKey = "";
      state.activeHVACNodeName = "";
      state.activeHVACEntity = { id: "", kind: "", label: "" };
      state.activeHVACContext = { pathId: "", zoneId: "", loopId: "", componentId: "", couplingId: "", previousView: "" };
      state.hvacNavigationStack = [];
      state.hvacForwardStack = [];
      state.hvacDiagramViewportKey = "";
      state.hvacDiagramScale = 1;
      state.hvacDiagramPanX = 0;
      state.hvacDiagramPanY = 0;

      view.initializeHVACControls();
      view.renderHVAC(hvac);
      await settle();

      const cardState = () => {
        const cards = [...document.querySelectorAll("#hvacSummary > .hvac-navigator > .hvac-nav-card")];
        return {
          cards,
          labels: cards.map((card) => card.querySelector("summary strong")?.textContent.trim() || ""),
        };
      };
      let currentCards = cardState();
      const fourCards = currentCards.cards.length === 4;
      const onlyCanonicalCards = currentCards.labels.join("|") === "Zone Services|AirLoopHVAC|PlantLoop|Other";
      const emptyOther = currentCards.cards.find((card) => card.dataset.hvacLoopKind === "other");
      const emptyOtherVisible = Boolean(emptyOther)
        && emptyOther.querySelector("summary b")?.textContent.trim() === "0"
        && Boolean(emptyOther.querySelector(".empty"));

      hvac.loops = [air, plant, other];
      hvac.condenserLoopCount = 1;
      view.renderHVAC(hvac);
      await settle();

      const selectLoop = async (kind, id, expectedName) => {
        const card = document.querySelector('#hvacSummary [data-hvac-loop-kind="' + kind + '"]');
        assert(card, kind + " card missing");
        card.open = true;
        const choice = card.querySelector('[data-hvac-loop-id="' + id + '"]');
        assert(choice, id + " choice missing");
        choice.click();
        await settle();
        const svg = document.querySelector("#hvacGraph .hvac-loop-svg");
        const visible = Boolean(svg)
          && !svg.closest("details")
          && svg.getClientRects().length > 0
          && svg.getBoundingClientRect().width > 0
          && svg.getBoundingClientRect().height > 0;
        const name = svg?.querySelector(".hvac-loop-name")?.textContent.trim() || "";
        return visible
          && name === expectedName
          && state.activeHVACView === "loop"
          && state.activeHVACLoopId === id
          && document.querySelector('#hvacSummary [data-hvac-loop-kind="' + kind + '"]')?.classList.contains("active");
      };

      const airDiagram = await selectLoop("air", "air-1", "Main Air Loop");
      const firstSVG = document.querySelector("#hvacGraph .hvac-loop-svg");
      const loopViewportMarkup = diagramMarkupReady(firstSVG);
      const diagramStructure = Boolean(firstSVG?.querySelector(".hvac-loop-side-block.supply"))
        && Boolean(firstSVG?.querySelector(".hvac-loop-side-block.demand"))
        && firstSVG.querySelectorAll(".hvac-loop-branch-path").length >= 2
        && firstSVG.querySelectorAll(".hvac-loop-equipment").length >= 2
        && firstSVG.querySelectorAll(".hvac-loop-endpoint").length >= 4;

      let bubbledWheelCount = 0;
      window.addEventListener("wheel", () => { bubbledWheelCount += 1; });
      const rootBeforeZoom = document.documentElement.getBoundingClientRect();
      const historyBeforeWheel = JSON.stringify({
        back: state.hvacNavigationStack,
        forward: state.hvacForwardStack,
      });
      const loopTransformBefore = firstSVG.querySelector("[data-hvac-diagram-content]")?.getAttribute("transform") || "";
      const bubblesBeforeCtrl = bubbledWheelCount;
      const ctrlWheel = dispatchWheel(firstSVG, { ctrlKey: true, deltaY: -180 });
      const ctrlViewport = diagramState(state);
      const loopTransformAfterCtrl = firstSVG.querySelector("[data-hvac-diagram-content]")?.getAttribute("transform") || "";
      const rootAfterZoom = document.documentElement.getBoundingClientRect();
      const ctrlWheelLocal = ctrlWheel.prevented && !ctrlWheel.allowed
        && ctrlViewport.scale > 1
        && loopTransformAfterCtrl !== loopTransformBefore
        && closeEnough(rootBeforeZoom.width, rootAfterZoom.width)
        && closeEnough(rootBeforeZoom.height, rootAfterZoom.height);
      const ctrlWheelStopped = bubbledWheelCount === bubblesBeforeCtrl;

      const scaleBeforeMeta = Number(state.hvacDiagramScale);
      const bubblesBeforeMeta = bubbledWheelCount;
      const metaWheel = dispatchWheel(firstSVG, { metaKey: true, deltaY: 90 });
      const metaWheelLocal = metaWheel.prevented && !metaWheel.allowed
        && !closeEnough(state.hvacDiagramScale, scaleBeforeMeta)
        && bubbledWheelCount === bubblesBeforeMeta;

      const stateBeforeNormal = diagramState(state);
      const bubblesBeforeNormal = bubbledWheelCount;
      const normalWheel = dispatchWheel(firstSVG, { deltaY: 120 });
      const stateAfterNormal = diagramState(state);
      const normalWheelPasses = !normalWheel.prevented && normalWheel.allowed
        && JSON.stringify(stateAfterNormal) === JSON.stringify(stateBeforeNormal)
        && bubbledWheelCount === bubblesBeforeNormal + 1;

      const outsideTarget = document.querySelector("#hvacGraph .hvac-legend");
      const stateBeforeOutside = diagramState(state);
      const bubblesBeforeOutside = bubbledWheelCount;
      const outsideCtrlWheel = dispatchWheel(outsideTarget, { ctrlKey: true, deltaY: -120 });
      const outsideCtrlWheelPasses = !outsideCtrlWheel.prevented && outsideCtrlWheel.allowed
        && JSON.stringify(diagramState(state)) === JSON.stringify(stateBeforeOutside)
        && bubbledWheelCount === bubblesBeforeOutside + 1;

      for (let index = 0; index < 48; index += 1) {
        dispatchWheel(firstSVG, { ctrlKey: true, deltaY: -1000 });
      }
      const upperBoundReached = closeEnough(state.hvacDiagramScale, 8);
      for (let index = 0; index < 96; index += 1) {
        dispatchWheel(firstSVG, { ctrlKey: true, deltaY: 1000 });
      }
      const zoomBounds = upperBoundReached && closeEnough(state.hvacDiagramScale, 0.1);

      dispatchWheel(firstSVG, { ctrlKey: true, deltaY: -260 });
      const retainedViewport = diagramState(state);
      view.renderHVAC(hvac);
      await settle();
      const rerenderedAirSVG = document.querySelector("#hvacGraph .hvac-loop-svg");
      const sameKeyRerenderRetainsViewport = diagramMarkupReady(rerenderedAirSVG)
        && JSON.stringify(diagramState(state)) === JSON.stringify(retainedViewport)
        && (rerenderedAirSVG.querySelector("[data-hvac-diagram-content]")?.getAttribute("transform") || "") !== "translate(0 0) scale(1)";
      const wheelHistoryNeutral = historyBeforeWheel === JSON.stringify({
        back: state.hvacNavigationStack,
        forward: state.hvacForwardStack,
      });

      const plantDiagram = await selectLoop("plant", "plant-1", "Chilled Water Loop");
      const plantViewport = diagramState(state);
      const loopKeySwitchResetsViewport = isDefaultDiagramState(state)
        && Boolean(plantViewport.key)
        && plantViewport.key !== retainedViewport.key;
      const otherDiagram = await selectLoop("other", "condenser-1", "Condenser Water Loop");

      const backRestoresPlant = view.backHVAC()
        && state.activeHVACView === "loop"
        && state.activeHVACLoopId === "plant-1"
        && document.querySelector("#hvacGraph .hvac-loop-name")?.textContent.trim() === "Chilled Water Loop";
      const forwardRestoresOther = view.forwardHVAC()
        && state.activeHVACView === "loop"
        && state.activeHVACLoopId === "condenser-1"
        && document.querySelector("#hvacGraph .hvac-loop-name")?.textContent.trim() === "Condenser Water Loop";

      document.getElementById("hvacZoneServicesButton").click();
      await settle();
      const zoneServicesReturn = state.activeHVACView === "services"
        && state.activeHVACEntity.id === ""
        && state.activeHVACGraphKey === ""
        && document.querySelector("#hvacSummary .hvac-nav-card")?.classList.contains("active")
        && !document.querySelector("#hvacGraph .hvac-loop-svg");

      view.renderHVAC(hvac);
      await settle();
      const fixedFocusedView = !("activeHVACGraphScope" in state)
        && !document.querySelector("#hvacGraph [data-hvac-graph-scope], #hvacGraph .hvac-graph-scope");
      const initialFilters = [...document.querySelectorAll("#hvacGraph select[data-hvac-filter-kind]")];
      const dropdownFilters = initialFilters.length === 3
        && initialFilters.map((select) => select.dataset.hvacFilterKind).join("|") === "service|path|medium"
        && initialFilters.every((select) => Boolean(select.querySelector('option[value="all"]')))
        && initialFilters.every((select) => Boolean(select.getAttribute("aria-label") || select.labels.length));
      const initialFilterWidths = initialFilters.map((select) => select.getBoundingClientRect().width);
      const compactDropdownWidths = initialFilterWidths.length === 3
        && initialFilterWidths[0] <= 130
        && initialFilterWidths.slice(1).every((width) => width <= 138)
        && initialFilterWidths.every((width) => width >= 110);
      const legacyGraphChoicesAbsent = !document.querySelector("#hvacGraph [data-hvac-graph-scale], #hvacGraph .hvac-graph-scale, #hvacGraph [data-hvac-graph-scope], #hvacGraph .hvac-graph-scope");

      await changeSelect(document.querySelector('#hvacGraph select[data-hvac-filter-kind="service"]'), "heating");
      const noMatchSelect = document.querySelector('#hvacGraph select[data-hvac-filter-kind="service"]');
      const noMatchFilters = document.querySelectorAll("#hvacGraph select[data-hvac-filter-kind]");
      const noMatchRecoverable = Boolean(noMatchSelect)
        && noMatchFilters.length === 3
        && noMatchSelect.value === "heating"
        && !document.querySelector("#hvacGraph .hvac-service-svg")
        && Boolean(document.querySelector("#hvacGraph .empty"));
      await changeSelect(noMatchSelect, "all");
      const wrongFilterRecoverable = noMatchRecoverable
        && document.querySelector('#hvacGraph select[data-hvac-filter-kind="service"]')?.value === "all"
        && diagramMarkupReady(document.querySelector("#hvacGraph .hvac-service-svg"));

      const layoutElement = document.querySelector(".hvac-layout");
      const mainElement = document.querySelector(".hvac-main");
      const summaryElement = document.getElementById("hvacSummary");
      layoutElement.style.width = "360px";
      mainElement.style.width = "360px";
      summaryElement.style.width = "360px";
      void mainElement.offsetWidth;
      const filterToolbar = document.querySelector("#hvacGraph .hvac-graph-toolbar");
      const filterToolbarRect = filterToolbar?.getBoundingClientRect();
      const graphElement = document.getElementById("hvacGraph");
      const graphRect = graphElement.getBoundingClientRect();
      const actionRect = document.getElementById("hvacViewportActions").getBoundingClientRect();
      const narrowSelects = [...document.querySelectorAll("#hvacGraph select[data-hvac-filter-kind]")];
      const filterToolbarContained = Boolean(filterToolbar && filterToolbarRect)
        && filterToolbar.scrollWidth <= filterToolbar.clientWidth + 1;
      const graphContentContained = graphElement.scrollWidth <= graphElement.clientWidth + 1;
      const selectRects = narrowSelects.map((select) => select.getBoundingClientRect());
      const controlsBelowActions = selectRects.length > 0 && Math.min(...selectRects.map((rect) => rect.top)) >= actionRect.bottom - 1;
      const selectsContained = narrowSelects.length === 3
        && selectRects.every((rect) => rect.width > 0 && rect.left >= graphRect.left - 1 && rect.right <= graphRect.right + 1);
      const narrowToolbarNotClipped = filterToolbarContained && graphContentContained && controlsBelowActions && selectsContained;
      layoutElement.style.width = "1120px";
      mainElement.style.width = "1120px";
      summaryElement.style.width = "1120px";
      void mainElement.offsetWidth;

      const serviceSVG = document.querySelector("#hvacGraph .hvac-service-svg");
      const serviceViewportMarkup = diagramMarkupReady(serviceSVG);
      const serviceTransformBefore = serviceSVG?.querySelector("[data-hvac-diagram-content]")?.getAttribute("transform") || "";
      const serviceBubblesBefore = bubbledWheelCount;
      const serviceCtrlWheel = dispatchWheel(serviceSVG, { ctrlKey: true, deltaY: -180 });
      const serviceTransformAfter = serviceSVG?.querySelector("[data-hvac-diagram-content]")?.getAttribute("transform") || "";
      const serviceCtrlWheelLocal = serviceCtrlWheel.prevented && !serviceCtrlWheel.allowed
        && state.hvacDiagramScale > 1
        && serviceTransformAfter !== serviceTransformBefore
        && bubbledWheelCount === serviceBubblesBefore;

      const fitButton = document.getElementById("hvacFitButton");
      fitButton?.click();
      await settle();
      const resetServiceSVG = document.querySelector("#hvacGraph .hvac-service-svg");
      const fitButtonResetsViewport = Boolean(fitButton)
        && isDefaultDiagramState(state)
        && diagramMarkupReady(resetServiceSVG);

      view.navigateHVAC({ view: "couplings", graphKey: "", context: {} }, { pushHistory: true });
      await settle();
      const legacyCouplingsCanonicalized = state.activeHVACView === "services"
        && !document.querySelector("#hvacSummary [data-hvac-open-view='couplings']");

      const postLegacyServiceSVG = document.querySelector("#hvacGraph .hvac-service-svg");
      dispatchWheel(postLegacyServiceSVG, { ctrlKey: true, deltaY: -180 });
      window.dispatchEvent(new Event("idfAnalyzer:documentChanged"));
      const documentResetResetsViewport = state.hvacDiagramViewportKey === ""
        && isDefaultDiagramState(state)
        && state.hvacNavigationStack.length === 0
        && state.hvacForwardStack.length === 0;

      const result = {
        fourCards,
        onlyCanonicalCards,
        emptyOtherVisible,
        airDiagram,
        loopViewportMarkup,
        ctrlWheelLocal,
        metaWheelLocal,
        ctrlWheelStopped,
        normalWheelPasses,
        outsideCtrlWheelPasses,
        zoomBounds,
        sameKeyRerenderRetainsViewport,
        wheelHistoryNeutral,
        plantDiagram,
        loopKeySwitchResetsViewport,
        otherDiagram,
        diagramStructure,
        backRestoresPlant,
        forwardRestoresOther,
        zoneServicesReturn,
        dropdownFilters,
        compactDropdownWidths,
        wrongFilterRecoverable,
        fixedFocusedView,
        legacyGraphChoicesAbsent,
        narrowToolbarNotClipped,
        serviceViewportMarkup,
        serviceCtrlWheelLocal,
        fitButtonResetsViewport,
        legacyCouplingsCanonicalized,
        documentResetResetsViewport,
      };
      resultElement.textContent = JSON.stringify(result);
      const failed = Object.entries(result).filter(([, value]) => value !== true).map(([key]) => key);
      assert(failed.length === 0, failed.join(", ") + " contract failed: " + JSON.stringify(result));
      document.body.dataset.hvacLoopNavigationStatus = "passed";
    } catch (error) {
      resultElement.textContent = error.stack || String(error);
      document.body.dataset.hvacLoopNavigationStatus = "failed";
    }
  </script>
</body>
</html>`
