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

func TestHVACInspectionChartsBrowser(t *testing.T) {
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
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/chart").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-hvac-chart-status="passed"`) {
		t.Fatalf("HVAC inspection chart acceptance failed: %v\n%s", err, output)
	}
}

const hvacInspectionChartsHTML = `<!doctype html><html><head><link rel="stylesheet" href="/src/styles/hvac-inspection-charts.css"></head><body><div id="mount" style="width:720px"></div><pre id="result"></pre>
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
 check(Number.parseFloat(getComputedStyle(mount.querySelector('.hvac-chart-tick')).fontSize)>=15,'chart ticks are not readable');
 check(mount.scrollWidth===720,'responsive chart overflowed regular inspection panel');
 mount.innerHTML=render({series:[flow,{...temp,unit:'kg/s'}]});check(!mount.querySelector('[data-hvac-chart-axis="right"]'),'same-unit traces received separate Y axes');
 check(mount.querySelector('[data-hvac-chart-axis="left"]').textContent==='Value (kg/s)','shared-unit axis incorrectly names only one of different properties');
 mount.innerHTML=render({series:[{...flow,points:[point(0,1,'01/21 01:00:00'),point(1,2,'01/21 02:00:00'),point(2,3,'01/21 03:00:00')]}]});
 for(const label of mount.querySelectorAll('[data-hvac-chart-tick="x"]')){const bounds=label.getBBox();check(bounds.x>=0&&bounds.x+bounds.width<=1000,'full time tick clipped outside graph SVG at actual half-panel width');}
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
 document.body.dataset.hvacChartStatus='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.hvacChartStatus='failed';document.getElementById('result').textContent=error.stack;}
</script></body></html>`
