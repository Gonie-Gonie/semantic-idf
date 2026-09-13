package frontendchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Synthetic DOM wheel events do not perform native scrolling. Exercise the
// compositor's real scroll chaining over the rendered topology instead.
func TestHVACInspectionTopologyTrustedWheelBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("native HVAC result scrolling")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/scroll", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, hvacInspectionScrollHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	browser := epath201FreshBrowser(t, ctx, chrome)
	browser.call("Page.navigate", map[string]any{"url": server.URL + "/scroll"}, nil)
	for deadline := time.Now().Add(10 * time.Second); ; {
		if string(browser.evaluate(`Boolean(window.hvacScroll)`)) == "true" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("HVAC scroll fixture did not load: %s", browser.evaluate(`document.body.innerText`))
		}
		time.Sleep(20 * time.Millisecond)
	}
	type scrollState struct {
		Top, Left, InnerTop, X, Y float64
		Width, ContentWidth       float64
	}
	read := func() scrollState {
		var value scrollState
		if err := json.Unmarshal(browser.evaluate(`hvacScroll.measure()`), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	wheel := func(dx, dy float64) {
		at := read()
		browser.call("Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": at.X, "y": at.Y, "deltaX": dx, "deltaY": dy}, nil)
		browser.evaluate(`hvacScroll.settle()`)
	}
	for _, width := range []int{900, 700} {
		browser.evaluate(fmt.Sprintf(`hvacScroll.render(%d); hvacScroll.settle()`, width))
		before := read()
		wheel(0, 280)
		down := read()
		if down.Top < before.Top+180 || down.InnerTop != 0 {
			t.Fatalf("width %d swallowed vertical wheel above the lower graphs: before=%+v after=%+v", width, before, down)
		}
		wheel(0, 280)
		second := read()
		if second.Top < down.Top+180 {
			t.Fatalf("width %d bounced or stalled during continued downward scrolling: %+v -> %+v", width, down, second)
		}
		wheel(0, -220)
		up := read()
		if up.Top > second.Top-120 {
			t.Fatalf("width %d swallowed upward scrolling: %+v -> %+v", width, second, up)
		}
		wheel(320, 0)
		horizontal := read()
		if horizontal.Left < up.Left+160 || horizontal.Top != up.Top {
			t.Fatalf("width %d lost horizontal diagram panning or moved the result vertically: %+v -> %+v", width, up, horizontal)
		}
		if string(browser.evaluate(`window.scrollY===0 && document.documentElement.scrollTop===0 && hvacScroll.sameDiagram()`)) != "true" {
			t.Fatal("scrolling moved the app document or replaced the topology")
		}
		t.Logf("width %d: result scroll %.0f -> %.0f -> %.0f -> %.0f; horizontal %.0f", width, before.Top, down.Top, second.Top, up.Top, horizontal.Left)
	}
}

const hvacInspectionScrollHTML = `<!doctype html><html data-theme="dark"><head><meta charset="utf-8"><link rel="stylesheet" href="/src/styles.css">
<style>body{display:block;overflow:hidden;padding:20px}#pane{height:650px;width:900px;flex:none}#topology{min-width:0}.below-graphs{height:900px}</style></head><body>
<main id="pane" class="simulation-pane has-simulation-result"><section class="simulation-section"><h3>HVAC Loops</h3><div class="simulation-hvac-loop-results"><section class="simulation-hvac-loop-result"><div id="topology"></div><section class="below-graphs"><h4>Node graphs</h4></section></section></div></section></main>
<script type="module">
try{
 const {renderHVACInspectionTopology:renderTopology}=await import('/src/js/views/hvac-inspection-topology.js');
 const loop={id:'air',name:'VAV_1',type:'AirLoopHVAC',supplySide:{inletNode:'Inlet',outletNode:'Outlet',branches:[{name:'Main',components:[{objectType:'Fan:VariableVolume',objectName:'Supply fan',inletNode:'Inlet',outletNode:'Outlet',exists:true}]}]},demandSide:{}};
 const metrics=[{id:'temperature',label:'Temperature',value:17.27,unit:'C'},{id:'flow',label:'Mass flow',value:3.72,unit:'kg/s'},{id:'setpoint',label:'Setpoint temperature',value:12.8,unit:'C'},{id:'relativeHumidity',label:'Relative humidity',value:50.52,unit:'%'}];
 const nodes=['Inlet','Outlet',...Array.from({length:16},(_,i)=>'Water circuit point '+i)].map(name=>({id:name,name,metrics}));
 const pane=document.getElementById('pane'),mount=document.getElementById('topology');let diagram;
 const viewport=()=>mount.querySelector('[data-hvac-inspect-topology-viewport]');
 const render=width=>{pane.style.width=width+'px';mount.innerHTML=renderTopology({loop,nodes});diagram=mount.querySelector('svg');pane.scrollTop=0;};
 window.hvacScroll={render,sameDiagram:()=>diagram===mount.querySelector('svg'),settle:async()=>{for(let i=0;i<20;i++)await new Promise(resolve=>requestAnimationFrame(resolve));},measure:()=>{
  const v=viewport(),r=v.getBoundingClientRect(),p=pane.getBoundingClientRect();return {top:pane.scrollTop,left:v.scrollLeft,innerTop:v.scrollTop,width:v.clientWidth,contentWidth:v.scrollWidth,x:Math.min(r.right,p.right)-80,y:Math.max(r.top,p.top)+Math.min(220,(Math.min(r.bottom,p.bottom)-Math.max(r.top,p.top))/2)};
 }};render(900);
}catch(error){document.body.textContent=error.stack;}
</script></body></html>`
