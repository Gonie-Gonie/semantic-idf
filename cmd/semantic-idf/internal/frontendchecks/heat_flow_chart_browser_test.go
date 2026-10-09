package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHeatFlowChartBrowser(t *testing.T) {
	for _, path := range []string{"frontend/src/js/views/heat-flow-chart.js", "frontend/src/styles/heat-flow-chart.css", "frontend/src/js/heat-flow-data.js"} {
		_ = readTestFile(t, path)
	}
	if testing.Short() {
		t.Skip("headless Heat-Flow chart acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/heat-flow-chart", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, heatFlowChartHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	browser := epath201FreshBrowser(t, ctx, chrome)
	browser.call("Page.navigate", map[string]any{"url": server.URL + "/heat-flow-chart"}, nil)
	for deadline := time.Now().Add(30 * time.Second); ; {
		status := string(browser.evaluate(`document.body?.dataset.heatFlowChartStatus`))
		if status == `"passed"` {
			t.Log(string(browser.evaluate(`document.getElementById('result').textContent`)))
			return
		}
		if status == `"failed"` || time.Now().After(deadline) {
			t.Fatalf("Heat-Flow chart browser %s: %s", status, browser.evaluate(`document.getElementById('result')?.textContent`))
		}
		time.Sleep(25 * time.Millisecond)
	}
}

const heatFlowChartHTML = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/src/styles.css"><link rel="stylesheet" href="/src/styles/heat-flow-chart.css"></head><body class="guide-page"><div id="mount" style="width:720px"></div><pre id="result"></pre>
<script type="module">
const check=(value,message)=>{if(!value)throw new Error(message);};
try {
 const {renderHeatFlowStackChart:render,heatFlowChartFrameFromRatio:frameFromRatio}=await import('/src/js/views/heat-flow-chart.js');
 const {setLanguage}=await import('/src/js/i18n.js');
 const mount=document.getElementById('mount');
 const ids=['internalConvective','surfaceConvection','interzoneAir','outdoorAir','systemAir','systemConvective','airStorage','deviation'];
 const colors=['#f59e0b','#ef4444','#a855f7','#14b8a6','#3b82f6','#64748b','#e5e7eb','#94a3b8'];
 const dataset={frameCount:5,labels:['01/01 01:00:00','01/01 02:00:00','01/01 03:00:00','01/01 04:00:00','01/01 05:00:00'],categories:ids.map((id,index)=>({id,label:id,color:colors[index]}))};
 const zone={name:'Test zone',values:[[25600,0,null,0.125,100],[1100,-4000,2000,0,0],[500,-500,0,0,0],[1000,-1000,0,0,0],[-24600,-30000,-10000,0,-100],[0,0,0,0,0],[2200,null,800,0,0],[-100,0,500,0,0]]};
 const before=JSON.stringify({dataset,zone});
 mount.innerHTML=render(dataset,zone,0);
 check(mount.querySelectorAll('[data-heatflow-chart-panel]').length===3,'local, exchange and diagnostic panels missing');
 check(mount.querySelector('.heatflow-chart-heading h4').textContent.includes(zone.name),'history loses selected-zone identity');
 const local=mount.querySelector('[data-heatflow-chart-panel="local"]'),exchange=mount.querySelector('[data-heatflow-chart-panel="exchange"]'),diagnostic=mount.querySelector('[data-heatflow-chart-panel="diagnostic"]');
 check(local.querySelectorAll('[data-heatflow-category]').length===3&&exchange.querySelectorAll('[data-heatflow-category]').length===3,'local/exchange categories not separated');
 check(!local.querySelector('[data-heatflow-category="surfaceConvection"]')&&!exchange.querySelector('[data-heatflow-category="systemAir"]'),'local and boundary exchange mixed');
 check(!mount.querySelector('[data-heatflow-category="airStorage"], [data-heatflow-category="deviation"]')&&diagnostic.querySelectorAll('[data-heatflow-diagnostic]').length===2,'storage or deviation included in gain/loss stack');
 check(new Set([...mount.querySelectorAll('svg')].map(svg=>svg.dataset.heatflowChartExtent)).size===1&&local.querySelector('svg').dataset.heatflowChartExtent==='50000','panels do not share exact numeric scale');
 const internal=local.querySelector('[data-heatflow-category="internalConvective"]'),cooling=local.querySelector('[data-heatflow-category="systemAir"]');
 check(internal.getAttribute('d').startsWith('M174.4,132H182.4V82.848H174.4Z'),'positive observation glyph has incorrect zero/scale');
 check(cooling.getAttribute('d').startsWith('M174.4,132H182.4V179.232H174.4Z'),'negative observation glyph has incorrect zero/scale');
 check((internal.getAttribute('d').match(/M/g)||[]).length===3&&(cooling.getAttribute('d').match(/M/g)||[]).length===3,'missing group frame rendered as zero or unknown stack baseline');
 check(internal.dataset.heatflowObservedCount==='4'&&internal.dataset.heatflowObservedMin==='0','known zero discarded or null manufactured as observation');
 const storage=diagnostic.querySelector('[data-heatflow-diagnostic="airStorage"]'),storageGeometry=storage.getAttribute('d');
 check((storage.getAttribute('d').match(/M/g)||[]).length===2,'storage path joined across missing observation');
 check(storage.dataset.heatflowObservedMin==='0'&&storage.dataset.heatflowObservedMax==='2200','diagnostic values/sign altered');
 check(local.querySelector('[data-heatflow-chart-legend="internalConvective"]').title.includes('25,600.000000 W'),'current legend loses exact numeric detail');
 check(local.querySelector('[data-heatflow-chart-legend="internalConvective"] strong').textContent==='+25.600 kW'&&local.querySelector('[data-heatflow-chart-legend="systemAir"] strong').textContent==='-24.600 kW','legend current gains/losses do not retain signed kW precision');
 check(mount.querySelectorAll('[data-heatflow-chart="1"]').length===3&&mount.querySelectorAll('[data-heatflow-chart-frame="0"]').length===3,'interactive frame hit/cursor missing');
 const inheritedFamily=getComputedStyle(document.body).fontFamily,screenFont=element=>parseFloat(getComputedStyle(element).fontSize)*element.getScreenCTM().a;
 const originalPath=internal,originalGeometry=internal.getAttribute('d');
 const appearance=[];
 for(const font of [11,18])for(const width of [350,720,1100,1423,1600,1800,2000,2800]){
  document.documentElement.style.setProperty('--graph-label-font-size',font+'px');mount.style.width=width+'px';
  await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
  for(const panel of mount.querySelectorAll('[data-heatflow-chart-panel]')){
   const tick=panel.querySelector('.heatflow-chart-tick'),axis=panel.querySelector('.heatflow-chart-axis-label');
   check(Math.abs(screenFont(tick)-Math.max(12,font))<.02&&Math.abs(screenFont(axis)-(Math.max(12,font)+1))<.02,'physical chart font does not follow Settings/readability floor at '+JSON.stringify({font,width,tick:screenFont(tick),axis:screenFont(axis)}));
   check(getComputedStyle(tick).fontFamily===inheritedFamily&&getComputedStyle(axis).fontFamily===inheritedFamily,'chart font differs from app family');
   for(const label of panel.querySelectorAll('[data-heatflow-chart-tick]')){const box=label.getBBox();check(box.x>=0&&box.x+box.width<=760,'axis tick clips outside SVG at '+JSON.stringify({font,width,text:label.textContent,x:box.x,labelWidth:box.width}));}
   const xLabels=[...panel.querySelectorAll('[data-heatflow-chart-tick="x"]')].map(label=>label.getBoundingClientRect());
   check(xLabels[0].right<xLabels[1].left,'full endpoint timestamps overlap');
  }
  check(mount.scrollWidth===width,'chart overflow leaked outside panel at '+width);
  const localBox=local.getBoundingClientRect(),exchangeBox=exchange.getBoundingClientRect(),diagnosticBox=diagnostic.getBoundingClientRect();
  if(font===11&&(width===1423||width===1600))check(Math.abs(localBox.top-exchangeBox.top)<.01&&diagnosticBox.top>localBox.bottom&&Math.abs(diagnosticBox.width-(localBox.width+exchangeBox.width+14))<.02,'default-font two-column layout does not give diagnostics a full separate row');
  if(font===18&&width===1423)check(exchangeBox.top>localBox.bottom&&diagnosticBox.top>exchangeBox.bottom,'large-font cards do not wrap into a full readable column');
  if(font===18&&width===1800)check(Math.abs(localBox.top-exchangeBox.top)<.01&&diagnosticBox.top>localBox.bottom&&Math.abs(diagnosticBox.width-(localBox.width+exchangeBox.width+14))<.02,'large-font two-column layout does not give diagnostics a full separate row');
  if((font===11&&width===2000)||(font===18&&width===2800))check(Math.abs(localBox.top-exchangeBox.top)<.01&&Math.abs(localBox.top-diagnosticBox.top)<.01,'very wide layout does not show three readable chart cards');
  if(width===1423)for(const scroll of mount.querySelectorAll('.heatflow-chart-scroll'))check(scroll.scrollWidth===scroll.clientWidth,'1423px workspace requires horizontal chart scrolling at font '+font);
  check(local.querySelector('[data-heatflow-category="internalConvective"]')===originalPath&&originalPath.getAttribute('d')===originalGeometry,'font/layout update changed observations');
  appearance.push({font,width,tickFont:screenFont(local.querySelector('.heatflow-chart-tick'))});
 }
 document.documentElement.dataset.theme='dark';await new Promise(resolve=>requestAnimationFrame(resolve));
 check(getComputedStyle(storage).stroke==='rgb(241, 245, 249)'&&getComputedStyle(diagnostic.querySelector('[data-heatflow-chart-legend="airStorage"] i')).borderTopColor===getComputedStyle(storage).stroke,'dark diagnostics line/legend contrast differ');
 check(getComputedStyle(storage).strokeDasharray!==getComputedStyle(diagnostic.querySelector('[data-heatflow-diagnostic="deviation"]')).strokeDasharray,'storage and deviation styles indistinguishable');
 check(storage.getAttribute('d')===storageGeometry,'theme change altered numeric geometry');
 mount.innerHTML=render(dataset,zone,2);check(mount.querySelector('.heatflow-chart-missing')&&mount.querySelectorAll('[data-heatflow-chart-frame="2"]').length===3,'missing current observation status lost frame');
 mount.innerHTML=render(dataset,zone,3,{start:2,end:4});check([...mount.querySelectorAll('svg')].every(svg=>svg.dataset.heatflowChartStart==='2'&&svg.dataset.heatflowChartEnd==='4'),'requested visible range ignored');
 check(mount.querySelector('[data-heatflow-category="internalConvective"]').dataset.heatflowObservedMax==='100','range extrema include hidden frame');
 check(mount.querySelector('[data-heatflow-chart-legend="internalConvective"]').title.includes('0.125000 W'),'fractional source value rounded away');
 const irregular={...dataset,frameCount:4,labels:['01/01 01:00:00','01/01 02:00:00','01/01 08:00:00','01/01 09:00:00']},irregularZone={name:'Irregular',values:ids.map((_id,index)=>[1,1,1,1].map(value=>index===4?-value:value))};
 mount.innerHTML=render(irregular,irregularZone,2);
 check([...mount.querySelectorAll('svg')].every(svg=>svg.dataset.heatflowChartTimeMode==='elapsed'),'strictly increasing timestamps did not use elapsed-time spacing');
 const irregularGlyphs=mount.querySelector('[data-heatflow-category="internalConvective"]').getAttribute('d').match(/M[^Z]+Z/g).map(glyph=>{const numbers=glyph.match(/-?\d+(?:\.\d+)?/g).map(Number);return {left:numbers[0],right:numbers[2],center:(numbers[0]+numbers[2])/2};});
 check(Math.abs((irregularGlyphs[2].center-irregularGlyphs[1].center)/(irregularGlyphs[1].center-irregularGlyphs[0].center)-6)<.0001,'six-hour missing interval collapsed into one frame step');
 check(irregularGlyphs.every(glyph=>glyph.right-glyph.left<=8.000001)&&irregularGlyphs[2].left>irregularGlyphs[1].right+300,'observation bars filled an unobserved time interval');
 check(frameFromRatio(irregular,{start:0,end:3},.49)===1&&frameFromRatio(irregular,{start:0,end:3},.51)===2,'pointer selection did not select nearest actual observation across a gap');
 check((mount.querySelector('[data-heatflow-diagnostic="airStorage"]').getAttribute('d').match(/M/g)||[]).length===2,'diagnostic line invented continuity over missing six-hour interval');
 const sampled={...irregular,labels:['01/01 01:00:00','01/01 14:00:00','01/02 03:00:00','01/02 16:00:00']};mount.innerHTML=render(sampled,irregularZone,1);
 const sampledGlyphs=mount.querySelector('[data-heatflow-category="internalConvective"]').getAttribute('d').match(/M[^Z]+Z/g).map(glyph=>glyph.match(/-?\d+(?:\.\d+)?/g).map(Number));
 check(sampledGlyphs.every(glyph=>glyph[2]-glyph[0]<16)&&sampledGlyphs[1][0]-sampledGlyphs[0][2]>180,'stride-sampled observations were shown as continuous thirteen-hour rates');
 check((mount.querySelector('[data-heatflow-diagnostic="airStorage"]').getAttribute('d').match(/M/g)||[]).length===4,'stride-sampled diagnostic observations were joined over unobserved hours');
 const isolated={name:'Sparse',values:ids.map((_id,index)=>index===6?[2200,null,0,null]:[0,0,0,0])};mount.innerHTML=render(sampled,isolated,2);
 const isolatedStorage=mount.querySelector('[data-heatflow-diagnostic="airStorage"]'),isolatedPath=isolatedStorage.getAttribute('d');
 check((isolatedPath.match(/h1L/g)||[]).length===2&&!isolatedPath.includes('l0,0'),'isolated diagnostic observations do not paint real short segments');
 check(isolatedStorage.getTotalLength()>=2,'isolated diagnostic glyphs have no paintable native SVG stroke length');
 check(isolatedPath.includes(',132h1L')&&isolatedStorage.dataset.heatflowObservedMin==='0'&&isolatedStorage.dataset.heatflowObservedCount==='2','reported zero diagnostic point is absent');
 check(mount.querySelector('[data-heatflow-chart-legend="airStorage"] strong').textContent==='0.000 kW','known-zero diagnostic legend does not display its current value');
 const designDay={...irregular,labels:['07/21 01:00:00','07/21 02:00:00','01/21 01:00:00','01/21 02:00:00']};mount.innerHTML=render(designDay,irregularZone,2);
 check([...mount.querySelectorAll('svg')].every(svg=>svg.dataset.heatflowChartTimeMode==='ordinal')&&mount.querySelector('.heatflow-chart-sequence-note')&&mount.textContent.includes('Recorded frame sequence'),'backward DesignDay timestamps were guessed/reordered into elapsed time');
 check(frameFromRatio(designDay,{start:0,end:3},.62)===2&&frameFromRatio(designDay,{start:2,end:3},0)===2&&frameFromRatio(designDay,{start:2,end:3},1)===3,'ordinal pointer mapping or restricted range incorrect');
 const duplicate={...irregular,labels:['01/01 01:00:00','01/01 01:00:00','01/01 08:00:00','01/01 09:00:00']};mount.innerHTML=render(duplicate,irregularZone,0);check(mount.querySelector('svg').dataset.heatflowChartTimeMode==='ordinal','duplicate timestamps were arbitrarily chosen');
 const leap={...irregular,labels:['02/28 24:00:00','02/29 01:00:00','02/29 24:00:00','03/01 01:00:00']};mount.innerHTML=render(leap,irregularZone,0);check(mount.querySelector('svg').dataset.heatflowChartTimeMode==='elapsed','valid leap-day/24:00 timestamps lost exact calendar spacing');
 const masked={...zone,observed:zone.values.map((values,index)=>values.map((_value,frame)=>!(index===0&&frame===0)))};
 mount.innerHTML=render(dataset,masked,0);check(mount.querySelector('.heatflow-chart-missing')&&mount.querySelector('[data-heatflow-category="internalConvective"]').dataset.heatflowObservedCount==='3','explicit absent mask became known numeric value');
 check(mount.querySelector('[data-heatflow-chart-legend="internalConvective"] strong').dataset.heatflowLegendValue===''&&mount.querySelector('[data-heatflow-chart-legend="internalConvective"] strong').textContent!=='0.000 kW','missing current legend value became known zero');
 const count=24000,large={...dataset,frameCount:count,labels:Array.from({length:count},(_,frame)=>'Hour '+frame)},largeZone={name:'Large',values:ids.map((_id,index)=>Array.from({length:count},(_,frame)=>index===0?(frame===12003?9000000:frame===17998?-7000000:3):index===4?-2:0))};
 mount.innerHTML=render(large,largeZone,17611);
 const largePath=mount.querySelector('[data-heatflow-category="internalConvective"]');
 check(largePath.dataset.heatflowObservedCount===String(count)&&largePath.dataset.heatflowObservedMin==='-7000000'&&largePath.dataset.heatflowObservedMax==='9000000','large chart removed observed extrema');
 check((largePath.getAttribute('d').match(/M/g)||[]).length===count,'large chart dropped supplied frames');
 check(mount.querySelectorAll('path').length===8&&mount.querySelectorAll('*').length<170,'full source frames create unbounded DOM elements');
 check(mount.querySelectorAll('[data-heatflow-chart-frame="17611"]').length===3,'large current observed frame disappeared');
 setLanguage('ko');mount.innerHTML=render(dataset,zone,0);check(!mount.querySelector('[data-heatflow-chart-panel="local"]').textContent.includes('Local zone gains / losses'),'Korean chart headings did not use app language');
 setLanguage('en');const unsafe={frameCount:1,labels:['<img src=x onerror=alert(1)>'],categories:[{id:'custom',label:'<script>unsafe<\/script>',color:'#f59e0b'}]};mount.innerHTML=render(unsafe,{name:'<button>zone</button>',values:[[1]]},0);
 check(!mount.querySelector('img,script,button')&&mount.textContent.includes('<img'),'chart labels were not escaped');
 check(JSON.stringify({dataset,zone})===before,'chart mutated source data');
 document.body.dataset.heatFlowChartStatus='passed';document.getElementById('result').textContent=JSON.stringify({status:'passed',appearance});
}catch(error){document.body.dataset.heatFlowChartStatus='failed';document.getElementById('result').textContent=error.stack;}
</script></body></html>`
