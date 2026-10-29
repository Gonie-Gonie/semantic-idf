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

func TestInputViewsKeepConstantEventListenerCountsAcrossRenders(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser input event delegation harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}

	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/input-event-delegation", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, inputEventDelegationHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--no-sandbox",
		"--no-first-run",
		"--no-default-browser-check",
		"--virtual-time-budget=15000",
		"--window-size=1280,900",
		"--user-data-dir="+t.TempDir(),
		"--dump-dom",
		server.URL+"/input-event-delegation",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("input event delegation browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("input event delegation browser harness failed: %v\n%s", err, output)
	}
	document := string(output)
	if !strings.Contains(document, `data-input-event-delegation-status="passed"`) {
		t.Fatalf("input event delegation browser harness did not pass:\n%s", document)
	}
	for _, signal := range []string{
		`"semanticSurfaceAbsent":true`,
		`"jsonListeners":1`,
		`"textListeners":3`,
		`"tableListeners":4`,
		`"rerenderStable":true`,
		`"dynamicControlsWork":true`,
		`"selectedSourcesMaterialized":true`,
		`"tableCapSourceReachable":true`,
	} {
		if !strings.Contains(document, signal) {
			t.Fatalf("input event delegation result is missing %s:\n%s", signal, document)
		}
	}
}

const inputEventDelegationHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Input event delegation harness</title></head>
<body data-input-event-delegation-status="pending">
  <div id="textObjectView"></div>
  <div id="jsonStructuredView"></div>
  <div id="fieldTable"></div>
  <pre id="result">pending</pre>
  <script type="module">
    const assert = (condition, message) => { if (!condition) throw new Error(message); };
    const listenerCounts = new WeakMap();
    const originalAddEventListener = EventTarget.prototype.addEventListener;
    EventTarget.prototype.addEventListener = function(type, listener, options) {
      let counts = listenerCounts.get(this);
      if (!counts) {
        counts = new Map();
        listenerCounts.set(this, counts);
      }
      counts.set(type, (counts.get(type) || 0) + 1);
      return originalAddEventListener.call(this, type, listener, options);
    };
    const countListeners = (element, types) => types.reduce((total, type) => total + (listenerCounts.get(element)?.get(type) || 0), 0);

    try {
      const [{ state, elements, setDocumentText }, inputViews] = await Promise.all([
        import("/src/js/state.js"),
        import("/src/js/views/input-views.js"),
      ]);
      const sourceObject = {
        index: 0,
        sourceIndex: 0,
        type: "Version",
        name: "24.2",
        fields: [{ key: "version_identifier", comment: "Version Identifier", value: "24.2" }],
      };
      setDocumentText("Version,24.2;");
      state.report = { objects: [sourceObject] };
      state.model = { format: "idf", version: { raw: "24.2" }, objects: [sourceObject] };
      state.reportAnalyzedText = state.documentText;
      state.semanticProjection = {
        schema: "eplus-semantic/0.1",
        energyplusVersion: "24.2",
        objectCount: 1,
        lines: [],
        navigation: { entities: [], occurrences: [], byEntityId: {}, byObjectId: {}, byObjectIndex: {}, byViewTarget: {} },
      };

      const countsAfterTwoRenders = (view, root, types) => {
        state.activeInputView = view;
        inputViews.renderInputViews();
        const first = countListeners(root, types);
        inputViews.renderInputViews();
        const second = countListeners(root, types);
        assert(first === second, view + " listeners changed after rerender: " + first + " -> " + second);
        return second;
      };

      const semanticSurfaceAbsent = !document.getElementById("semanticEditor") && !("semanticEditor" in elements);
      assert(semanticSurfaceAbsent, "removed Semantic renderer remains allocated");

      const jsonListeners = countsAfterTwoRenders("json", elements.jsonStructuredView, ["click"]);
      assert([...elements.jsonStructuredView.querySelectorAll("details")].every((detail) => detail.open), "JSON tree is not fully expanded");
      elements.jsonStructuredView.querySelector(".json-object-summary").click();
      assert(String(state.jsonSelectedObjectIndex) === "0", "delegated JSON object selection did not update state");

      const textListeners = countsAfterTwoRenders("text", elements.textObjectView, ["click", "focusin", "focusout", "keydown"]);
      elements.textObjectView.querySelector(".text-object-head").click();

      const tableListeners = countsAfterTwoRenders("table", elements.fieldTable, ["click", "focusin", "focusout", "keydown"]);
      const orientation = elements.fieldTable.querySelector(".object-orientation-button");
      const nextOrientation = orientation.dataset.nextOrientation;
      orientation.click();
      assert(state.tableGroupOrientations.get("Version") === nextOrientation, "delegated table orientation control did not update state");

      const selectionController = await import("/src/js/selection-controller.js");
      const objects = Array.from({length:600},(_,index)=>({index,sourceIndex:index,type:"Zone",name:"Zone "+index,
        fields:[{key:"name",comment:"Name",value:"Zone "+index}]}));
      const sourceAnchor = {objectIndex:550,objectType:"Zone",objectName:"Zone 550",fieldIndex:0,fieldName:"name"};
      const sourceSelection = {entityId:"fixture:zone-550",entityKind:"zone",occurrenceId:"fixture:source-550",sourceAnchor};
      setDocumentText(objects.map(object=>"Zone,"+object.name+";").join("\n"));
      state.report = {objects};state.model = {format:"idf",version:{raw:"24.2"},objects};state.reportAnalyzedText=state.documentText;
      state.semanticProjection = {schema:"eplus-semantic/0.2",navigation:{
        entities:[{id:sourceSelection.entityId,kind:"zone",label:"Zone 550"}],
        occurrences:[{occurrenceId:sourceSelection.occurrenceId,entityId:sourceSelection.entityId,path:"zones/550",sourceAnchor}],
        byEntityId:{[sourceSelection.entityId]:[sourceSelection.occurrenceId]},byObjectIndex:{"550":[sourceSelection.occurrenceId]},byObjectId:{},byViewTarget:{},
      }};
      const sourceBefore=JSON.stringify({report:state.report,model:state.model,projection:state.semanticProjection});
      const controller=selectionController.configureSelectionController({state,getNavigationIndex:()=>state.semanticProjection.navigation,isAnalysisCurrent:()=>true,
        onSelectionChange:detail=>window.dispatchEvent(new CustomEvent("idfAnalyzer:semanticSelectionChanged",{detail}))});
      const selectSource=()=>controller.selectSemanticEntity(sourceSelection,{originView:"profile",follow:false,recordHistory:false});
      const clearSource=()=>controller.clearSemanticSelection({follow:false,recordHistory:false});
      const inputRoot=view=>({text:elements.textObjectView,json:elements.jsonStructuredView,table:elements.fieldTable})[view];
      const objectIDs=root=>new Set([...root.querySelectorAll("[data-object-index]")].map(node=>node.dataset.objectIndex));
      for(const view of["text","json","table"]){
        state.activeInputView=view;inputViews.setInputFilter("does-not-match-any-source");
        assert(objectIDs(inputRoot(view)).size===0,view+" unmatched filter unexpectedly showed source objects");
        await selectSource();const identity=JSON.stringify(controller.currentSemanticSelection());inputViews.renderInputViews();
        const ids=objectIDs(inputRoot(view));
        assert(ids.size===1&&ids.has("550")&&state.inputFilterQuery==="does-not-match-any-source"&&JSON.stringify(controller.currentSemanticSelection())===identity,
          view+" did not materialize only the exact selected source while preserving filter and selection identity");
        await clearSource();
        assert(objectIDs(inputRoot(view)).size===0&&state.inputFilterQuery==="does-not-match-any-source",
          view+" retained a temporary source exception after clearing selection");
      }
      const selectedSourcesMaterialized=true;
      state.activeInputView="table";inputViews.setInputFilter("Zone");
      const ordinaryIDs=objectIDs(elements.fieldTable);
      assert(ordinaryIDs.size===500&&!ordinaryIDs.has("550"),"ordinary table render limit did not retain its first 500 sources");
      await selectSource();const capIdentity=JSON.stringify(controller.currentSemanticSelection());inputViews.renderInputViews();
      const expandedIDs=objectIDs(elements.fieldTable);
      const tableCapSourceReachable=expandedIDs.size===501&&expandedIDs.has("550")&&[...ordinaryIDs].every(id=>expandedIDs.has(id))
        &&state.inputFilterQuery==="Zone"&&JSON.stringify(controller.currentSemanticSelection())===capIdentity;
      assert(tableCapSourceReachable,"selected source beyond the table cap was omitted, displaced ordinary sources, or changed the filter/selection");
      await clearSource();
      assert(objectIDs(elements.fieldTable).size===500&&!objectIDs(elements.fieldTable).has("550")&&state.inputFilterQuery==="Zone",
        "clearing table selection did not restore the ordinary render cap without changing the filter");
      assert(JSON.stringify({report:state.report,model:state.model,projection:state.semanticProjection})===sourceBefore,"source materialization mutated the canonical input or navigation data");

      const result = {
        semanticSurfaceAbsent,
        jsonListeners,
        textListeners,
        tableListeners,
        rerenderStable: true,
        dynamicControlsWork: true,
        selectedSourcesMaterialized,
        tableCapSourceReachable,
      };
      assert(jsonListeners === 1, "unexpected JSON listener count");
      assert(textListeners === 3, "unexpected Text listener count");
      assert(tableListeners === 4, "unexpected Table listener count");
      document.querySelector("#result").textContent = JSON.stringify(result);
      document.body.dataset.inputEventDelegationStatus = "passed";
    } catch (error) {
      document.querySelector("#result").textContent = String(error?.stack || error);
      document.body.dataset.inputEventDelegationStatus = "failed";
    }
  </script>
</body>
</html>`
