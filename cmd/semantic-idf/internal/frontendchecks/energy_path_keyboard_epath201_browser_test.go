package frontendchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// These are trusted browser inputs, not dispatched DOM KeyboardEvents. Native
// Tab navigation and button default actions are part of the acceptance contract.
func TestEPATH201ActualGraphTrustedKeyboardBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("fresh headless-browser trusted keyboard acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	page := readTestFile(t, "frontend/src/index.html")
	for _, script := range []string{`<script type="module" src="./app.js"></script>`, `<script type="module" src="./js/main.js"></script>`} {
		if !strings.Contains(page, script) {
			t.Fatalf("actual index bootstrap changed: %s", script)
		}
		page = strings.Replace(page, script, "", 1)
	}
	page = strings.Replace(page, "</body>", `<script>globalThis.__epath201Calls={layout:0,ribbons:0,projection:0};</script>`+epath142LayoutHTML+epath201KeyboardHTML+"</body>", 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	// Count the actual exported calculation entry points in test-served module
	// responses. No production telemetry, build bypass or alternate renderer.
	for _, item := range []struct{ file, function, counter string }{
		{"js/energy-path-layout.js", "energyPathLayout", "layout"},
		{"js/energy-path-ribbons.js", "energyPathRibbons", "ribbons"},
		{"js/views/energy-path-view.js", "energyPathGraphForState", "projection"},
	} {
		source := readTestFile(t, "frontend/src/"+item.file)
		start := strings.Index(source, "export function "+item.function+"(")
		if start < 0 {
			t.Fatalf("missing measured entry point %s", item.function)
		}
		body := strings.Index(source[start:], ") {")
		if body < 0 {
			t.Fatalf("unrecognized measured function signature %s", item.function)
		}
		insert := start + body + len(") {")
		source = source[:insert] + "globalThis.__epath201Calls." + item.counter + "++;" + source[insert:]
		mux.HandleFunc("/src/"+item.file, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = fmt.Fprint(w, source)
		})
	}
	mux.HandleFunc("/src/epath201-keyboard.html", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, page)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	browser := epath201FreshBrowser(t, ctx, chrome)
	browser.call("Page.navigate", map[string]any{"url": server.URL + "/src/epath201-keyboard.html?manual=1"}, nil)
	ready := false
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		status := string(browser.evaluate(`document.body?.dataset.epath201Status || "pending"`))
		if status == `"ready"` {
			ready = true
			break
		}
		if status == `"failed"` {
			t.Fatalf("keyboard fixture: %s", browser.evaluate(`document.querySelector('#epath201-result')?.textContent`))
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("actual app never became ready: %s", browser.evaluate(`document.body?.innerText`))
	}
	var targets []struct {
		Key, ID, Kind string
		X, Y          float64
	}
	if err := json.Unmarshal(browser.evaluate(`__epath201.targets`), &targets); err != nil {
		t.Fatal(err)
	}
	if len(targets) < 40 || targets[0].Kind != "node" {
		t.Fatalf("representative graph keyboard fixture is incomplete: %#v", targets)
	}
	assert := func(expression, label string) {
		t.Helper()
		if string(browser.evaluate(expression)) != "true" {
			t.Fatalf("%s: %s", label, browser.evaluate(`__epath201.proof()`))
		}
	}
	assert(`__epath201.initialValid`, "real viewport, four stages and meaningful calculation counters")
	// Establish focus with a real pointer activation, then clear through the
	// real Escape handler. There are no programmatic focus/click calls below.
	browser.call("Input.dispatchMouseEvent", map[string]any{"type": "mousePressed", "x": targets[0].X, "y": targets[0].Y, "button": "left", "clickCount": 1}, nil)
	browser.call("Input.dispatchMouseEvent", map[string]any{"type": "mouseReleased", "x": targets[0].X, "y": targets[0].Y, "button": "left", "clickCount": 1}, nil)
	browser.key("Escape", false)
	current := 0
	checkFocus := func(index int, selection string) {
		t.Helper()
		key, _ := json.Marshal(targets[index].Key)
		selected, _ := json.Marshal(selection)
		assert(fmt.Sprintf(`__epath201.focused(%s,%s)`, key, selected), "trusted focus "+targets[index].Key)
	}
	for index := range targets {
		checkFocus(index, "")
		browser.key("Tab", false)
	}
	assert(`!__epath201.targets.some(target=>target.key===__epath201.activeKey())`, "Tab must leave graph after its final unique stop")
	for index := len(targets) - 1; index >= 0; index-- {
		browser.key("Tab", true)
		checkFocus(index, "")
	}
	move := func(index int, selected string) {
		for current != index {
			backward := current > index
			browser.key("Tab", backward)
			if backward {
				current--
			} else {
				current++
			}
			checkFocus(current, selected)
		}
	}
	activate := func(index int, key string) {
		move(index, "")
		browser.key(key, false)
		id, _ := json.Marshal(targets[index].ID)
		kind, _ := json.Marshal(targets[index].Kind)
		assert(fmt.Sprintf(`__epath201.selected(%s,%s)`, id, kind), "trusted "+key+" activates "+targets[index].Key)
		checkFocus(index, targets[index].ID)
		browser.key("Escape", false)
		checkFocus(index, "")
	}
	find := func(kind, contains string) int {
		for index, target := range targets {
			if target.Kind == kind && strings.Contains(target.ID, contains) {
				return index
			}
		}
		t.Fatalf("missing %s keyboard target %q", kind, contains)
		return -1
	}
	for _, kind := range []string{"node", "ratio", "edge"} {
		index := find(kind, "")
		activate(index, "Enter")
		activate(index, " ")
	}
	lighting := find("node", "internal.lighting")
	move(lighting, "")
	browser.key("Enter", false)
	move(find("node", "carrier.electricity"), targets[lighting].ID)
	// Keep real CSS transition timing; the test does not finish animations or
	// disable styles. Focusing a dim card must not select its physical branch.
	time.Sleep(180 * time.Millisecond)
	assert(`__epath201.focusReadability()`, "focused dim card remains readable without undimming its quantitative bars")
	browser.key("Escape", false)
	checkFocus(current, "")
	move(find("ratio", ""), "")
	time.Sleep(180 * time.Millisecond)
	assert(`__epath201.tooltipVisible()`, "native ratio focus exposes its complete accessible tooltip")
	assert(`__epath201.unchanged()`, "keyboard selection must preserve raw payload, context, graph objects and calculation counts")
	assert(`__epath201.events.length>100&&__epath201.events.every(event=>event.trusted)&&['Tab','Enter',' ','Escape'].every(key=>__epath201.events.some(event=>event.key===key))`, "all exercised keyboard events must originate from trusted browser input")
	t.Logf("PASS: %d native graph stops traversed forward/backward; node/ratio/SVG Enter+Space+Escape; focus-only readability and tooltip; no graph recomputation or Analyze/Run", len(targets))
}

type epath201Browser struct {
	t        *testing.T
	socket   *websocket.Conn
	sequence int
}

// Test-owned loopback browser/profile only, never a user or selected browser.
func epath201FreshBrowser(t *testing.T, ctx context.Context, chrome string) *epath201Browser {
	t.Helper()
	profile := t.TempDir()
	process := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--remote-debugging-port=0", "--force-device-scale-factor=1", "--user-data-dir="+profile, "about:blank")
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Process.Kill(); _ = process.Wait() })
	endpoint := ""
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort")); err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) >= 2 {
				endpoint = "http://127.0.0.1:" + strings.TrimSpace(lines[0])
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if endpoint == "" {
		t.Fatal("fresh trusted-input browser did not start")
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Get(endpoint + "/json/list")
	if err != nil {
		t.Fatal(err)
	}
	var targets []struct {
		Type   string `json:"type"`
		Socket string `json:"webSocketDebuggerUrl"`
	}
	err = json.NewDecoder(response.Body).Decode(&targets)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	var address string
	for _, target := range targets {
		if target.Type == "page" {
			address = target.Socket
			break
		}
	}
	if address == "" {
		t.Fatal("fresh browser has no page target")
	}
	socket, _, err := websocket.DefaultDialer.DialContext(ctx, address, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = socket.Close() })
	browser := &epath201Browser{t: t, socket: socket}
	browser.call("Page.enable", map[string]any{}, nil)
	browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 1600, "height": 900, "deviceScaleFactor": 1, "mobile": false}, nil)
	return browser
}

func (browser *epath201Browser) call(method string, params, result any) {
	browser.t.Helper()
	browser.sequence++
	if err := browser.socket.WriteJSON(map[string]any{"id": browser.sequence, "method": method, "params": params}); err != nil {
		browser.t.Fatal(err)
	}
	_ = browser.socket.SetReadDeadline(time.Now().Add(15 * time.Second))
	for {
		var message struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := browser.socket.ReadJSON(&message); err != nil {
			browser.t.Fatalf("%s: %v", method, err)
		}
		if message.ID != browser.sequence {
			continue
		}
		if len(message.Error) > 0 {
			browser.t.Fatalf("%s: %s", method, message.Error)
		}
		if result != nil {
			if err := json.Unmarshal(message.Result, result); err != nil {
				browser.t.Fatal(err)
			}
		}
		return
	}
}

func (browser *epath201Browser) evaluate(expression string) json.RawMessage {
	browser.t.Helper()
	var result struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		Exception json.RawMessage `json:"exceptionDetails"`
	}
	browser.call("Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true}, &result)
	if len(result.Exception) > 0 {
		browser.t.Fatalf("keyboard assertion: %s", result.Exception)
	}
	return result.Result.Value
}

func (browser *epath201Browser) key(key string, shift bool) {
	code, virtual := key, 0
	switch key {
	case "Tab":
		virtual = 9
	case "Enter":
		virtual = 13
	case " ":
		code, virtual = "Space", 32
	case "Escape":
		virtual = 27
	default:
		browser.t.Fatalf("unsupported trusted test key %s", key)
	}
	modifiers := 0
	if shift {
		modifiers = 8
	}
	for _, eventType := range []string{"rawKeyDown", "keyUp"} {
		params := map[string]any{"type": eventType, "key": key, "code": code, "windowsVirtualKeyCode": virtual, "nativeVirtualKeyCode": virtual, "modifiers": modifiers}
		// Native button activation on Enter includes the character-producing
		// keyDown, not only a raw physical key event. Space likewise carries its
		// printable text; keyUp completes the browser's native Space action.
		if eventType == "rawKeyDown" && (key == "Enter" || key == " ") {
			text := key
			if key == "Enter" {
				text = "\r"
			}
			params["type"], params["text"], params["unmodifiedText"] = "keyDown", text, text
		}
		browser.call("Input.dispatchKeyEvent", params, nil)
	}
}

const epath201KeyboardHTML = `<pre id="epath201-result" hidden>pending</pre><script type="module">
try {
 for(let i=0;document.body.dataset.epath142Status!=='manual'&&i<400;i++)await new Promise(resolve=>setTimeout(resolve,10));
 if(document.body.dataset.epath142Status!=='manual')throw Error('actual142 bootstrap failed');
 const {state}=await import('/src/js/state.js');
 const host=document.getElementById('simulationEnergyDashboard'),canvas=host.querySelector('[data-energy-path-canvas]');
 const original=JSON.stringify(state.simulationResult),counts=JSON.stringify(__epath201Calls),context=JSON.stringify([state.simulationEnergyScopeKind,state.simulationEnergyZoneName,state.simulationEnergyPeriod,state.simulationEnergyService]);
 let analyze=0,run=0;
 for(const name of ['AnalyzeInputText','AnalyzeInputDiagnosticsText'])window.go.main.App[name]=async()=>{analyze++;throw Error('unexpected '+name);};
 for(const name of ['RunSimulation','RunSimulationText','RunPurposeSimulationText'])window.go.main.App[name]=async()=>{run++;throw Error('unexpected '+name);};
 const controls=[...host.querySelectorAll('[data-energy-path-layout-node],[data-energy-path-bridge-ratio],[data-energy-path-link-hit]')].filter(element=>element.tabIndex===0);
 const key=element=>element?.dataset.energyPathLayoutNode?'node:'+element.dataset.energyPathLayoutNode:element?.dataset.energyPathBridgeRatio?'ratio:'+element.dataset.energyPathBridgeRatio:element?.dataset.energyPathLinkHit?'edge:'+element.dataset.energyPathLinkHit:'';
 const activeKey=()=>key(document.activeElement);
 const targets=controls.map(element=>{const r=element.getBoundingClientRect(),kind=key(element).split(':')[0];return {key:key(element),id:element.dataset.energyPathLayoutNode||element.dataset.energyExplanationEdge,kind,x:r.x+r.width/2,y:r.y+r.height/2};});
 const geometryElements=[...host.querySelectorAll('[data-energy-path-canvas],[data-energy-path-layout-node],[data-energy-path-ribbon],[data-energy-path-bar],[data-energy-path-hit-layer],[data-energy-path-link-hit],[data-energy-path-bridge-ratio]')];
 const geometry=element=>['d','x','y','width','height','viewBox'].map(name=>element.getAttribute(name));
 const geometryValues=geometryElements.map(geometry);
 const events=[];for(const type of ['keydown','keyup'])document.addEventListener(type,event=>events.push({key:event.key,trusted:event.isTrusted}),true);
 const visible=element=>{if(!element)return false;const r=element.getBoundingClientRect(),style=getComputedStyle(element);return r.width>0&&r.height>0&&r.bottom>0&&r.top<innerHeight&&r.right>0&&r.left<innerWidth&&style.visibility!=='hidden'&&style.display!=='none';};
 const unchanged=()=>host.querySelector('[data-energy-path-canvas]')===canvas&&JSON.stringify(state.simulationResult)===original&&JSON.stringify(__epath201Calls)===counts&&analyze===0&&run===0&&context===JSON.stringify([state.simulationEnergyScopeKind,state.simulationEnergyZoneName,state.simulationEnergyPeriod,state.simulationEnergyService])&&geometryElements.every((element,index)=>element.isConnected&&JSON.stringify(geometry(element))===JSON.stringify(geometryValues[index]))&&controls.every((element,index)=>element.isConnected&&key(element)===targets[index].key);
 const focused=(expected,selection)=>activeKey()===expected&&state.simulationEnergySelection===selection&&visible(document.activeElement)&&Boolean(document.activeElement.getAttribute('aria-label'))&&unchanged();
 const selected=(id,kind)=>state.simulationEnergySelection===id&&document.activeElement.getAttribute('aria-pressed')==='true'&&Boolean(kind==='node'?host.querySelector('[data-energy-path-inspector="'+CSS.escape(id)+'"]'):host.querySelector('[data-energy-path-link-inspector="'+CSS.escape(id)+'"]'))&&host.querySelectorAll('[data-energy-path-detail-section]').length===5&&unchanged();
 const focusReadability=()=>{const element=document.activeElement,id=element.dataset.energyPathLayoutNode,bars=[...host.querySelectorAll('[data-energy-path-bar]')].filter(bar=>bar.dataset.energyPathBar===id);return element.matches(':focus-visible')&&Number(getComputedStyle(element).opacity)===1&&element.dataset.energyPathFocus==='dimmed'&&bars.length>0&&bars.every(bar=>Number(getComputedStyle(bar).opacity)===.25)&&state.simulationEnergySelection.includes('internal.lighting')&&unchanged();};
 const tooltipVisible=()=>{const element=document.activeElement,tooltip=document.getElementById(element.getAttribute('aria-describedby'));return element.dataset.energyPathBridgeRatio&&visible(tooltip)&&tooltip.getAttribute('role')==='tooltip'&&tooltip.textContent===element.getAttribute('aria-label')&&tooltip.textContent.includes('kWh')&&state.simulationEnergySelection===''&&unchanged();};
 const nodeStages=controls.filter(element=>element.dataset.energyPathLayoutNode).map(element=>element.closest('[data-energy-path-stage]').dataset.energyPathStage);
 const stageOrder=[...new Set(nodeStages)],kinds=targets.map(target=>target.kind),kindOrder=[...new Set(kinds)];
 const initialValid=innerWidth===1600&&innerHeight===900&&JSON.stringify(stageOrder)===JSON.stringify(['driver','load','end_use','carrier'])&&JSON.stringify(kindOrder)===JSON.stringify(['node','ratio','edge'])&&targets.filter(target=>target.kind==='ratio').length===2&&new Set(targets.map(target=>target.key)).size===targets.length&&__epath201Calls.layout>0&&__epath201Calls.ribbons>0&&__epath201Calls.projection>0;
 const proof=()=>({active:activeKey(),selection:state.simulationEnergySelection,calls:__epath201Calls,initialCounts:counts,unchanged:unchanged(),visible:visible(document.activeElement),aria:document.activeElement?.getAttribute('aria-label'),pressed:document.activeElement?.getAttribute('aria-pressed'),detailSections:host.querySelectorAll('[data-energy-path-detail-section]').length,targets:targets.length,analyze,run,events:events.slice(-6)});
 globalThis.__epath201={targets,events,initialValid,focused,selected,focusReadability,tooltipVisible,unchanged,activeKey,proof};
 document.body.dataset.epath201Status='ready';
}catch(error){document.body.dataset.epath201Status='failed';document.getElementById('epath201-result').textContent=error.stack||String(error);}
</script>`
