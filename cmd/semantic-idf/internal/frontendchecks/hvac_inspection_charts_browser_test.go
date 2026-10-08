package frontendchecks

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHVACInspectionChartsBrowser(t *testing.T) {
	for _, path := range []string{"frontend/src/js/hvac-inspection-charts.js", "frontend/src/styles/hvac-inspection-charts.css", "frontend/src/styles/base.css", "frontend/src/styles/profile.css", "frontend/src/styles/simulation.css"} {
		_ = readTestFile(t, path)
	}
	if testing.Short() {
		t.Skip("headless HVAC inspection chart acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/chart", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, hvacInspectionChartsHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	browser := epath201FreshBrowser(t, ctx, chrome)
	browser.call("Page.enable", map[string]any{}, nil)
	browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 1600, "height": 1000, "deviceScaleFactor": 1, "mobile": false}, nil)
	ready := func(url string) {
		browser.call("Page.navigate", map[string]any{"url": url}, nil)
		for deadline := time.Now().Add(10 * time.Second); ; {
			status := string(browser.evaluate(`document.body?.dataset.hvacChartStatus`))
			if status == `"passed"` {
				return
			}
			if status == `"failed"` || time.Now().After(deadline) {
				t.Fatalf("HVAC chart browser %s: %s", status, browser.evaluate(`document.getElementById('result')?.textContent`))
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	ready(server.URL + "/chart")
	t.Log(string(browser.evaluate(`document.getElementById('result').textContent`)))
	if os.Getenv("HVAC_CHART_REVIEW") != "" {
		ready(server.URL + "/chart?review=1")
		directory := t.TempDir()
		for _, viewport := range []struct {
			name  string
			width int
		}{{"desktop", 1600}, {"mobile", 390}} {
			browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": viewport.width, "height": 1000, "deviceScaleFactor": 1, "mobile": false}, nil)
			browser.evaluate(`new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))`)
			var screenshot struct {
				Data string `json:"data"`
			}
			browser.call("Page.captureScreenshot", map[string]any{"format": "png", "fromSurface": true, "captureBeyondViewport": false}, &screenshot)
			data, err := base64.StdEncoding.DecodeString(screenshot.Data)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, viewport.name+".png")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			t.Logf("temporary chart review screenshot: %s", path)
		}
		time.Sleep(45 * time.Second) // Optional local review only; t.TempDir removes both captures.
	}
}

const hvacInspectionChartsHTML = `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/src/styles.css"></head><body class="guide-page"><div id="reference" style="position:absolute;visibility:hidden"><span class="profile-axis-tick">12</span><svg width="100" height="30" viewBox="0 0 100 30"><text class="simulation-axis" x="0" y="20">12</text><path class="simulation-line" d="M0,0 L1,1"/></svg></div><div id="mount" style="width:720px"></div><pre id="result"></pre>
<script type="module">
const check=(value,message)=>{if(!value)throw new Error(message);};
try {
 const {renderHVACInspectionChart:render,updateHVACInspectionChartFrame:updateFrame}=await import('/src/js/hvac-inspection-charts.js');
 const mount=document.getElementById('mount');
 const point=(x,value,label='Hour '+x)=>({x,value,label});
 const flow={id:'flow',label:'Supply node · Flow rate',axisLabel:'Flow rate',unit:'kg/s',interval:1,points:[point(0,0),point(1,2),point(2,null),point(3,4),point(4,Infinity),point(5,3),point(8,1)]};
 const temp={id:'temperature',label:'Supply node · Temperature',axisLabel:'Temperature',unit:'°C',points:[point(0,10),point(1,20),point(2,25),point(3,30),point(5,32),point(8,35)]};
 const humidity={id:'humidity',label:'Supply node · Humidity',unit:'%',points:[point(0,40),point(1,50)]};
 const before=JSON.stringify({flow,temp,humidity});
 mount.innerHTML=render({series:[flow,temp],frameKey:3,title:'Loop state'});
 check(mount.querySelector('svg[role="img"]')?.getAttribute('aria-label')==='Loop state','chart missing accessible title');
 check(mount.querySelector('[data-hvac-chart-axis="left"]').textContent==='Flow rate (kg/s)'&&mount.querySelector('[data-hvac-chart-axis="right"]').textContent==='Temperature (°C)','independent Y axes lost property/unit labels');
 check(mount.querySelector('[data-hvac-chart-axis="x"]').textContent==='Date & time'&&mount.querySelectorAll('.hvac-chart-grid').length>5,'time axis/grid missing');
 check(mount.querySelector('[data-hvac-chart-series="temperature"]').dataset.hvacChartSeriesAxis==='right','temperature uses flow scale');
 const flowGroup=mount.querySelector('[data-hvac-chart-series="flow"]'),path=flowGroup.querySelector('path').getAttribute('d');
 check((path.match(/M/g)||[]).length===4&&(path.match(/L/g)||[]).length===1,'null, nonfinite or absent time interval was connected');
 check(flowGroup.querySelector('[data-hvac-chart-value="0"]')&&!flowGroup.querySelector('[data-hvac-chart-value="null"]'),'reported zero or missing value handling incorrect');
 const circles=[...flowGroup.querySelectorAll('circle')],first=Number(circles[0].getAttribute('cx')),second=Number(circles[1].getAttribute('cx')),last=Number(circles.at(-1).getAttribute('cx'));
 check(Math.abs((last-first)/(second-first)-8)<.001,'observed time spacing collapsed to consecutive indices');
 check(mount.querySelector('[data-hvac-chart-frame="3"]')&&flowGroup.querySelector('[data-hvac-chart-time="3"].is-frame'),'current snapshot marker missing');
 const originalSVG=mount.querySelector('svg'),originalPath=flowGroup.querySelector('path');
 updateFrame(mount,1);check(mount.querySelector('svg')===originalSVG&&flowGroup.querySelector('path')===originalPath&&mount.querySelector('[data-hvac-chart-frame="1"]')&&flowGroup.querySelector('[data-hvac-chart-time="1"].is-frame')&&!flowGroup.querySelector('[data-hvac-chart-time="3"].is-frame'),'frame update redrew graph or did not move exact observed highlight');
 updateFrame(mount,100);check(mount.querySelector('[data-hvac-chart-frame-marker]').style.display==='none'&&!mount.querySelector('.is-frame'),'out-of-domain frame was clamped to invented point');
 updateFrame(mount.querySelector('section'),0);check(mount.querySelector('[data-hvac-chart-frame-marker]').style.display===''&&flowGroup.querySelector('[data-hvac-chart-time="0"].is-frame'),'frame zero or chart-root container unsupported');
 check(!mount.querySelector('table')&&!mount.textContent.includes('Source data'),'chart contains source/provenance details');
 const inheritedFamily=getComputedStyle(document.body).fontFamily;
 check(getComputedStyle(mount.querySelector('.hvac-chart-tick')).fontFamily===inheritedFamily&&getComputedStyle(mount.querySelector('.hvac-chart-axis-label')).fontFamily===inheritedFamily,'HVAC tick/title font overrides the app family');
 const screenFont=element=>parseFloat(getComputedStyle(element).fontSize)*element.getScreenCTM().a;
 check(Math.abs(screenFont(mount.querySelector('.hvac-chart-tick'))-parseFloat(getComputedStyle(document.querySelector('.profile-axis-tick')).fontSize))<.02,'HVAC physical tick font does not match Profile at default panel width');
 check(Math.abs(screenFont(mount.querySelector('.hvac-chart-tick'))-screenFont(document.querySelector('.simulation-axis')))<.02,'HVAC and Simulation default physical tick font differ');
 check(getComputedStyle(mount.querySelector('.hvac-chart-tick')).fontWeight==='400'&&getComputedStyle(mount.querySelector('.hvac-chart-axis-label')).fontWeight==='600','tick/title hierarchy changed');
 const appearance=[];
 for(const font of [11,18])for(const width of [350,720,1100]){
  document.documentElement.style.setProperty('--graph-label-font-size',font+'px');mount.style.width=width+'px';
  await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
  const tick=mount.querySelector('.hvac-chart-tick'),axis=mount.querySelector('.hvac-chart-axis-label'),legend=mount.querySelector('.hvac-chart-legend'),svg=mount.querySelector('svg');
  check(Math.abs(screenFont(tick)-font)<.02&&Math.abs(screenFont(axis)-(font+1))<.02,'resized/configured SVG font differs from app setting at '+JSON.stringify({font,width,tick:screenFont(tick),axis:screenFont(axis)}));
  check(parseFloat(getComputedStyle(legend).fontSize)===font+1&&getComputedStyle(legend).fontFamily===inheritedFamily,'legend does not share inherited font/settings');
  check(mount.querySelector('svg')===originalSVG&&flowGroup.querySelector('path')===originalPath&&originalPath.getAttribute('d')===path,'font/width change rebuilt or altered data geometry');
  check(mount.scrollWidth===width,'chart scroller leaked outside panel at '+width);
  for(const label of mount.querySelectorAll('[data-hvac-chart-tick="left"], [data-hvac-chart-tick="right"], [data-hvac-chart-tick="x"]')){const box=label.getBBox();check(box.x>=0&&box.x+box.width<=1000,'configured tick clips outside SVG at '+JSON.stringify({font,width,text:label.textContent,box:{x:box.x,width:box.width}}));}
  appearance.push({font,width,screenFont:screenFont(tick),svgWidth:svg.getBoundingClientRect().width});
 }
 document.documentElement.style.setProperty('--graph-label-font-size','11px');mount.style.width='720px';await new Promise(resolve=>requestAnimationFrame(resolve));
 const lightStroke=getComputedStyle(originalPath).stroke,legendSwatch=mount.querySelector('[data-hvac-chart-legend="flow"] i');
 check(lightStroke===getComputedStyle(legendSwatch).backgroundColor&&lightStroke===getComputedStyle(document.querySelector('.simulation-line')).stroke,'line/legend do not share app primary color');
 document.documentElement.dataset.theme='dark';await new Promise(resolve=>requestAnimationFrame(resolve));
 check(getComputedStyle(originalPath).stroke===getComputedStyle(legendSwatch).backgroundColor&&getComputedStyle(originalPath).stroke!==lightStroke,'line/legend do not adapt together to dark theme');
 check(getComputedStyle(mount.querySelector('.hvac-chart-tick')).fill===getComputedStyle(document.querySelector('.profile-axis-tick')).color,'dark HVAC tick color differs from Profile');
 check(originalPath.getAttribute('d')===path&&mount.querySelector('[data-hvac-chart-frame="0"]'),'theme change changed observations or frame');
 delete document.documentElement.dataset.theme;
 check(mount.scrollWidth===720,'responsive chart overflowed regular inspection panel');
 mount.innerHTML=render({series:[flow,{...temp,unit:'kg/s'}]});check(!mount.querySelector('[data-hvac-chart-axis="right"]'),'same-unit traces received separate Y axes');
 check(mount.querySelector('[data-hvac-chart-axis="left"]').textContent==='Value (kg/s)','shared-unit axis incorrectly names only one of different properties');
 mount.innerHTML=render({series:[{...flow,points:[point(0,1,'01/21 01:00:00'),point(1,2,'01/21 02:00:00'),point(2,3,'01/21 03:00:00')]}]});
 for(const label of mount.querySelectorAll('[data-hvac-chart-tick="x"]')){const bounds=label.getBBox();check(bounds.x>=0&&bounds.x+bounds.width<=1000,'full time tick clipped outside graph SVG at actual half-panel width');}
 document.documentElement.style.setProperty('--graph-label-font-size','18px');mount.style.width='350px';
 const irregular=[point(0,1,'01/21 01:00:00'),point(1,2,'01/21 02:00:00'),point(7,3,'01/21 08:00:00'),point(8,4,'01/21 09:00:00')];
 mount.innerHTML=render({series:[{...flow,points:irregular}],frameKey:7});await new Promise(resolve=>requestAnimationFrame(resolve));
 const tickBounds=[...mount.querySelectorAll('[data-hvac-chart-tick="x"]')].map(item=>item.getBoundingClientRect());
 check(tickBounds.every((bounds,index)=>index===0||bounds.left>tickBounds[index-1].right),'irregular full timestamp labels overlap at maximum font/mobile width');
 check(mount.querySelectorAll('[data-hvac-chart-series] circle').length===4&&mount.querySelector('[data-hvac-chart-time="7"].is-frame'),'axis label spacing removed actual observations or selected frame');
 const regular=Array.from({length:5},(_,x)=>point(x,x+1,'01/21 0'+(x+1)+':00:00'));
 mount.innerHTML=render({series:[{...flow,points:regular},{...temp,points:regular.map(item=>({...item,value:item.value+20}))}],frameKey:2});await new Promise(resolve=>requestAnimationFrame(resolve));
 const regularTickBounds=[...mount.querySelectorAll('[data-hvac-chart-tick="x"]')].map(item=>item.getBoundingClientRect());
 check(regularTickBounds.every((bounds,index)=>index===0||bounds.left>regularTickBounds[index-1].right),'regular full timestamp labels overlap between endpoint and centered tick at maximum font/mobile dual-axis width');
 check(mount.querySelectorAll('[data-hvac-chart-series] circle').length===10&&mount.querySelectorAll('[data-hvac-chart-time="2"].is-frame').length===2&&mount.querySelector('[data-hvac-chart-frame="2"]'),'regular dual-axis label spacing removed observations or frame');
 check(mount.querySelector('[data-hvac-chart-tick="x"]').textContent===regular[0].label&&[...mount.querySelectorAll('[data-hvac-chart-tick="x"]')].at(-1).textContent===regular.at(-1).label,'regular full timestamp endpoints disappeared');
 document.documentElement.style.setProperty('--graph-label-font-size','11px');mount.style.width='720px';
 mount.innerHTML=render({series:[flow,temp,humidity]});check(mount.querySelector('[data-hvac-chart-empty]')&&!mount.querySelector('svg'),'three incompatible units were drawn on false shared scale');
 mount.innerHTML=render({series:[{...flow,points:[point(0,null),point(1,NaN)]}]});check(mount.querySelector('[data-hvac-chart-empty]'),'all missing values manufactured chart');
 mount.innerHTML=render({series:[flow],mode:'scatter'});check(mount.querySelector('[data-hvac-chart-empty]'),'scatter accepted fewer than two properties');
 mount.innerHTML=render({series:[flow,temp,humidity],mode:'scatter'});check(mount.querySelector('[data-hvac-chart-empty]'),'scatter accepted more than two properties');
 mount.innerHTML=render({series:[flow,temp],mode:'scatter',frameKey:3});
 check(mount.querySelectorAll('[data-hvac-chart-scatter] circle').length===5,'scatter paired missing/invalid observations or dropped zero');
 check(mount.querySelector('[data-hvac-chart-axis="x"]').textContent==='Supply node · Flow rate (kg/s)'&&mount.querySelector('[data-hvac-chart-axis="left"]').textContent==='Supply node · Temperature (°C)','scatter axes do not identify compared properties and units');
 check(mount.querySelector('[data-hvac-chart-x-value="0"][data-hvac-chart-y-value="10"]')&&mount.querySelector('[data-hvac-chart-time="3"].is-frame'),'scatter lost matching zero value/frame observation');
 check(!mount.querySelector('path')&&!mount.querySelector('[data-hvac-chart-axis="right"]'),'scatter connected observations or created extra Y axis');
 updateFrame(mount,1);check(mount.querySelector('[data-hvac-chart-time="1"].is-frame')&&!mount.querySelector('[data-hvac-chart-time="3"].is-frame'),'scatter frame update did not move exact matching observation');
 mount.innerHTML=render({series:[{...flow,points:[point(0,1,'Jan 1')]},{...temp,points:[point(0,22,'Feb 1')]}],mode:'scatter'});check(mount.querySelector('[data-hvac-chart-empty]'),'different observation labels at same index were falsely paired');
 mount.innerHTML=render({series:[{...flow,points:[{...point(0,1),key:'run-a|0'}]},{...temp,points:[{...point(0,22),key:'run-b|0'}]}],mode:'scatter'});check(mount.querySelector('[data-hvac-chart-empty]'),'different files/runs at same time/index were falsely paired');
 mount.innerHTML=render({series:[{...flow,points:[{...point(0,1,'Display A'),key:'weather|0'}]},{...temp,points:[{...point(0,22,'Display B'),key:'weather|0'}]}],mode:'scatter'});check(mount.querySelectorAll('[data-hvac-chart-scatter] circle').length===1,'exact common observation identity depended on display formatting');
 mount.innerHTML=render({series:[{...flow,points:[point(0,1),point(0,2)]},{...temp,points:[point(0,22)]}],mode:'scatter'});check(mount.querySelector('[data-hvac-chart-empty]'),'ambiguous duplicate time observation silently chosen');
 const large=Array.from({length:24000},(_,x)=>point(x,x===12003?9000:x===17998?-7000:x%2?3:4));large[11999].value=null;large[12000].value=null;
 mount.innerHTML=render({series:[{...flow,points:large}],frameKey:17611});
 check(mount.querySelectorAll('circle').length<=2403,'large chart generates unbounded SVG geometry');
 check(mount.querySelector('[data-hvac-chart-value="9000"]')&&mount.querySelector('[data-hvac-chart-value="-7000"]'),'large chart sampling removed observed extrema');
 check((mount.querySelector('path').getAttribute('d').match(/M/g)||[]).length===2,'sampling connected across explicit missing observations');
 check(mount.querySelector('[data-hvac-chart-time="17611"].is-frame'),'sampling dropped current observed frame');
 mount.innerHTML=render({series:[{...flow,label:'<img src=x onerror=alert(1)>',unit:'<script>',points:[point(0,1,'<b>time</b>')]}],title:'<button>unsafe</button>'});
 check(!mount.querySelector('img,script,button,b')&&mount.textContent.includes('<img'),'graph labels were not escaped');
 check(JSON.stringify({flow,temp,humidity})===before,'chart mutated source observations');
 if(new URLSearchParams(location.search).has('review')){
  mount.style.cssText='width:min(1100px,calc(100vw - 40px));margin:20px auto';
  mount.innerHTML=render({series:[flow,temp],frameKey:3,title:'HVAC node trends'})+'<div style="height:20px"></div>'+render({series:[flow,temp],mode:'scatter',frameKey:3,title:'Flow rate / temperature'});
  document.getElementById('result').hidden=true;
 }
 document.body.dataset.hvacChartStatus='passed';document.getElementById('result').textContent=JSON.stringify({status:'passed',appearance});
}catch(error){document.body.dataset.hvacChartStatus='failed';document.getElementById('result').textContent=error.stack;}
</script></body></html>`
