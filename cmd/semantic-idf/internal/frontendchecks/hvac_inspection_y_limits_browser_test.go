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

func TestHVACInspectionYLimitsBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless HVAC basic graph Y-limit interaction acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/limits", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, hvacInspectionYLimitsHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/limits").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-hvac-y-limits-status="passed"`) {
		t.Fatalf("HVAC basic graph Y limits failed: %v\n%s", err, output)
	}
}

const hvacInspectionYLimitsHTML = `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="/src/styles.css"><style>body{display:block;overflow:auto;height:auto}#mount{width:720px}</style></head><body><div id="mount"></div><pre id="result"></pre><script type="module">
const check=(v,m)=>{if(!v)throw Error(m)},paint=()=>new Promise(resolve=>setTimeout(resolve,60));
try{
 const {renderHVACInspection:render,handleHVACInspectionEvent:handle}=await import('/src/js/views/hvac-inspection-view.js');
 const {setLanguage}=await import('/src/js/i18n.js');setLanguage('en');
 const series=(key,name,unit,values)=>({keyValue:key,name,file:'eplusout.sql',reportingFrequency:'Hourly',column:key+':'+name+' ['+unit+']',points:values.map((value,x)=>({x,value,label:'01-01 0'+(x+1)+':00'}))});
 const loop={name:'First',series:[
  series('NODE A','System Node Mass Flow Rate','kg/s',[0,5,10]),series('NODE A','System Node Temperature','C',[10,15,20]),series('NODE A','System Node Relative Humidity','%',[30,50,70]),
  series('NODE B','System Node Mass Flow Rate','kg/s',[2,4,6]),series('NODE B','System Node Temperature','C',[30,40,50]),series('NODE B','System Node Humidity Ratio','kg/kg',[.010,.012,.014]),
 ],components:[{componentName:'FAN',componentType:'Fan:ConstantVolume',series:[series('FAN','Fan Electricity Rate','W',[100,200,300])]}]};
 const original=JSON.stringify(loop),mount=document.getElementById('mount'),ui={};let activeLoop=loop,activeUI=ui;
 for(const type of ['input','change','click'])mount.addEventListener(type,event=>handle(event,mount,activeLoop,activeUI));
 mount.innerHTML=render(loop,ui);
 const plot=kind=>mount.querySelector('[data-hvac-inspect-basic-plot="'+kind+'"]');
 const ranges=kind=>[...mount.querySelectorAll('[data-hvac-inspect-y-range="'+kind+'"]')];
 const range=(kind,unit)=>ranges(kind).find(item=>item.dataset.hvacInspectYUnit===unit);
 const input=(kind,unit,bound)=>range(kind,unit).querySelector('[data-hvac-inspect-y-bound="'+bound+'"]');
 const axis=(kind,unit)=>[...plot(kind).querySelectorAll('[data-hvac-chart-y-low]')].find(item=>item.dataset.hvacChartUnit===unit);
 // Native release/change commits the queued plot immediately; this avoids
 // relying on compositor frames while Chrome advances its virtual test clock.
 const set=(kind,unit,bound,value,type='input',commit=true)=>{const item=input(kind,unit,bound);item.value=String(value);item.dispatchEvent(new Event(type,{bubbles:true}));if(type==='input'&&commit){check(input(kind,unit,bound)===item,'input replaced the captured range handle');item.dispatchEvent(new Event('change',{bubbles:true}));}};
 for(const kind of ['flow','temperature','humidity']){
  check(ranges(kind).length===(kind==='humidity'?2:1),'three basic graphs do not have separate per-unit range controls: '+kind);
  for(const group of ranges(kind))check(group.querySelectorAll('input[type="range"]').length===2&&[...group.querySelectorAll('input')].every(item=>item.getAttribute('aria-label')&&item.getAttribute('aria-valuetext')),'missing accessible lower/upper native handle');
  const legend=plot(kind).querySelector('.hvac-chart-legend'),items=[...legend.querySelectorAll('[data-hvac-chart-legend]')];
  check(legend.classList.contains('is-vertical')&&items.map(item=>item.textContent).join(',')==='NODE A,NODE B','basic legend repeats property/unit or is not node-only vertical');
  const labels=items.map(item=>item.querySelector('span').getBoundingClientRect()),swatches=items.map(item=>item.querySelector('i').getBoundingClientRect());
  check(labels[0].left===labels[1].left&&swatches[0].left===swatches[1].left&&labels[1].top>=labels[0].bottom,'legend rows/swatch labels are not vertically aligned');
 }
 const originalNodeSVG=plot('flow').querySelector('svg'),originalNodePath=plot('flow').querySelector('path'),originalNodeGeometry=originalNodePath.getAttribute('d'),originalNodeHandle=input('flow','kg/s','high');
 const cards=()=>[...mount.querySelectorAll('[data-hvac-inspect-basic-chart]')],rect=item=>item.getBoundingClientRect(),screenFont=item=>parseFloat(getComputedStyle(item).fontSize)*Math.hypot(item.getScreenCTM().a,item.getScreenCTM().b);
 for(const [width,columns] of [[360,1],[720,1],[1280,2],[1920,3],[3840,3]]){
  mount.style.width=width+'px';
  const boxes=cards().map(rect),firstRow=boxes.filter(box=>Math.abs(box.top-boxes[0].top)<.1);
  check(firstRow.length===columns,'node chart columns do not follow available panel width: '+JSON.stringify({width,columns,boxes:boxes.map(box=>({x:box.x,y:box.y,width:box.width}))}));
  for(const card of cards()){
   const svg=card.querySelector('.hvac-chart-svg'),scroll=card.querySelector('.hvac-chart-scroll');
   check(rect(svg).height<=340.1&&rect(scroll).width<=1000.1,'wide node SVG became an oversized tall plot at '+width);
   check(rect(card).width<=width&&rect(scroll).left>=rect(card).left-.1&&rect(scroll).right<=rect(card).right+.1&&getComputedStyle(scroll).overflowX==='auto','node plot lost its contained horizontal scrolling at '+width);
   check(Math.abs(screenFont(svg.querySelector('.hvac-chart-tick'))-11)<.05,'responsive node plot changed physical label size at '+width);
  }
  check(rect(mount.querySelector('[data-hvac-inspect-custom-chart] .hvac-chart-svg')).height<=340.1,'custom node plot grew without a useful height bound at '+width);
  check(plot('flow').querySelector('svg')===originalNodeSVG&&plot('flow').querySelector('path')===originalNodePath&&originalNodePath.getAttribute('d')===originalNodeGeometry&&input('flow','kg/s','high')===originalNodeHandle,'resizing rebuilt a node plot or changed source geometry/controls');
 }
 document.documentElement.style.setProperty('--graph-label-font-size','18px');mount.style.width='1920px';
 check(cards().filter(card=>Math.abs(rect(card).top-rect(cards()[0]).top)<.1).length===2&&cards().every(card=>Math.abs(screenFont(card.querySelector('.hvac-chart-tick'))-18)<.05),'larger configured labels did not retain usable node chart columns/font size');
 document.documentElement.style.removeProperty('--graph-label-font-size');
 mount.style.width='3200px';
 for(const kind of ['flow','temperature','humidity']){
  const items=[...plot(kind).querySelectorAll('[data-hvac-chart-legend]')],labels=items.map(item=>item.querySelector('span').getBoundingClientRect()),swatches=items.map(item=>item.querySelector('i').getBoundingClientRect());
  check(labels[0].top===labels[1].top&&labels[1].left>labels[0].right,'wide graph did not use multiple legend columns');
  check(labels.every((item,index)=>Math.abs(item.left-swatches[index].left-25)<.1),'wide legend labels/swatches lost fixed alignment');
 }
 mount.style.width='720px';
 check(!mount.querySelector('[data-hvac-inspect-custom-chart] [data-hvac-inspect-y-bound]')&&!mount.querySelector('[data-hvac-inspect-custom-chart] .is-vertical'),'basic controls or node-only legends changed custom graphs');
 const customSVG=mount.querySelector('[data-hvac-inspect-custom-chart] svg'),frame=mount.querySelector('[data-simulation-hvac-frame]');
 const low=input('flow','kg/s','low'),high=input('flow','kg/s','high'),autoLow=Number(axis('flow','kg/s').dataset.hvacChartYLow),autoHigh=Number(axis('flow','kg/s').dataset.hvacChartYHigh),min=low.min,max=low.max;
 check(Number(min)<autoLow&&Number(max)>autoHigh,'slider domain has no headroom to expand automatic limits');
 const linePoints=group=>[...group.querySelector('path').getAttribute('d').matchAll(/([ML])([^,\s]+),([^\s]+)/g)].map(match=>({x:Number(match[2]),y:Number(match[3])}));
 const flowValues=()=>{const bounds=axis('flow','kg/s'),bottom=Number(bounds.dataset.hvacChartYLow),top=Number(bounds.dataset.hvacChartYHigh);return [...plot('flow').querySelectorAll('[data-hvac-chart-series]')].flatMap(group=>linePoints(group).map(point=>top-(point.y-44)/215*(top-bottom)));};
 const valuesBefore=flowValues();
 check(!plot('flow').querySelector('circle')&&valuesBefore.length===6&&valuesBefore.every((value,index)=>Math.abs(value-[0,5,10,2,4,6][index])<1e-9),'marker-free flow lines lost source values or zero');
 set('flow','kg/s','high',5);await paint();
 check(input('flow','kg/s','low')===low&&input('flow','kg/s','high')===high,'Y handle input replaced native ranges and lost drag capture');
 check(Number(axis('flow','kg/s').dataset.hvacChartYHigh)===ui.basicYLimits.flow['kg/s'].high&&Number(axis('flow','kg/s').dataset.hvacChartYHigh)<autoHigh,'upper handle did not change plotted Y limit');
 check(plot('flow').querySelector('[data-hvac-chart-clipped-marks]')?.getAttribute('overflow')==='hidden'&&linePoints(plot('flow').querySelector('[data-hvac-chart-series]')).at(-1).y<44,'out-of-range line observation was clamped/deleted or not clipped to plot');
 const valuesAfter=flowValues();check(valuesAfter.length===valuesBefore.length&&valuesAfter.every((value,index)=>Math.abs(value-valuesBefore[index])<1e-9),'manual extent changed rendered source values');
 const flowSaved=JSON.stringify(ui.basicYLimits.flow),tempAuto=axis('temperature','°C').dataset.hvacChartYHigh;
 set('humidity','%','high',55);await paint();
 const ratioAuto=axis('humidity','g/kg').dataset.hvacChartYHigh;
 check(axis('humidity','%').dataset.hvacChartYHigh===String(ui.basicYLimits.humidity['%'].high)&&!ui.basicYLimits.humidity['g/kg'],'RH limit applied to humidity-ratio scale');
 set('humidity','g/kg','low',11);await paint();
 check(axis('humidity','g/kg').dataset.hvacChartYHigh===ratioAuto&&axis('humidity','g/kg').dataset.hvacChartYLow===String(ui.basicYLimits.humidity['g/kg'].low),'mixed humidity unit handles changed the wrong Y axis: '+JSON.stringify({axis:{...axis('humidity','g/kg').dataset},ratioAuto,saved:ui.basicYLimits.humidity['g/kg']}));
 check(JSON.stringify(ui.basicYLimits.flow)===flowSaved&&axis('temperature','°C').dataset.hvacChartYHigh===tempAuto,'separate basic graph ranges contaminated another metric');
 set('temperature','°C','low',18);await paint();
 const stored=JSON.stringify(ui.basicYLimits),flowSVG=plot('flow').querySelector('svg');
 frame.value='2';frame.dispatchEvent(new Event('input',{bubbles:true}));
 check(JSON.stringify(ui.basicYLimits)===stored&&input('flow','kg/s','low')===low&&plot('flow').querySelector('svg')===flowSVG,'frame change reset limits, handle or graph DOM');
 check(mount.querySelector('[data-hvac-inspect-custom-chart] svg')===customSVG,'basic controls changed custom chart');
 const nodeBColor=plot('flow').querySelectorAll('[data-hvac-chart-legend] i')[1].style.getPropertyValue('--hvac-chart-color');
 mount.querySelector('[data-hvac-inspect-node-visible="node:node a"]').click();
 check(JSON.stringify(ui.basicYLimits)===stored&&input('flow','kg/s','low').min===min&&input('flow','kg/s','low').max===max,'node visibility reset manual limits or shifted fixed handle domain');
 check(plot('flow').querySelector('[data-hvac-chart-legend] i').style.getPropertyValue('--hvac-chart-color')===nodeBColor,'node visibility changed the remaining node color');
 check([...range('humidity','%').querySelectorAll('input')].every(item=>item.disabled)&&!input('humidity','g/kg','low').disabled,'hidden-unit handles did not disable independently');
 mount.querySelector('[data-hvac-inspect-node-visible="node:node b"]').click();
 check(plot('flow').querySelector('[data-hvac-chart-empty]')&&[...range('flow','kg/s').querySelectorAll('input')].every(item=>item.disabled),'all-hidden graph manufactured values or left active handles');
 mount.querySelector('[data-hvac-inspect-node-visible="node:node a"]').click();
 set('flow','kg/s','low',Number(input('flow','kg/s','low').max),'change');
 check(ui.basicYLimits.flow['kg/s'].low<ui.basicYLimits.flow['kg/s'].high,'lower handle crossed upper handle');
 set('flow','kg/s','high',Number(input('flow','kg/s','high').min),'change');
 check(ui.basicYLimits.flow['kg/s'].low<ui.basicYLimits.flow['kg/s'].high,'upper handle crossed lower handle');
 const temperatureSaved=JSON.stringify(ui.basicYLimits.temperature),humiditySaved=JSON.stringify(ui.basicYLimits.humidity);
 mount.querySelector('[data-hvac-inspect-y-reset="flow"]').click();
 check(!ui.basicYLimits.flow&&Number(axis('flow','kg/s').dataset.hvacChartYLow)===autoLow&&Number(axis('flow','kg/s').dataset.hvacChartYHigh)===autoHigh,'Auto did not restore full observed extent');
 check(JSON.stringify(ui.basicYLimits.temperature)===temperatureSaved&&JSON.stringify(ui.basicYLimits.humidity)===humiditySaved,'one Auto button reset another graph');
 set('flow','kg/s','high',7,'input',false);
 const second={name:'Second',series:[series('OTHER NODE','System Node Mass Flow Rate','kg/s',[100,200,300])]},secondUI={};activeLoop=second;activeUI=secondUI;mount.innerHTML=render(second,secondUI);await paint();
 check(plot('flow').textContent.includes('OTHER NODE')&&!plot('flow').textContent.includes('NODE A')&&Number(axis('flow','kg/s').dataset.hvacChartYHigh)>300,'queued range redraw leaked old loop data into new loop');
 activeLoop=loop;activeUI=ui;mount.innerHTML=render(loop,ui);
 check(Number(axis('flow','kg/s').dataset.hvacChartYHigh)===ui.basicYLimits.flow['kg/s'].high&&Number(axis('flow','kg/s').dataset.hvacChartYHigh)<10,'loop state round-trip lost manual limits');
 const empty={name:'Empty',series:[series('EMPTY','System Node Temperature','C',[null,null,null])]},emptyUI={};activeLoop=empty;activeUI=emptyUI;mount.innerHTML=render(empty,emptyUI);
 check(!mount.querySelector('[data-hvac-inspect-y-bound]')&&plot('temperature').querySelector('[data-hvac-chart-empty]'),'all-null source manufactured an adjustable scale');
 check(JSON.stringify(loop)===original,'graph UI mutated original loop source data');
 document.body.dataset.hvacYLimitsStatus='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.hvacYLimitsStatus='failed';document.getElementById('result').textContent=error.stack;}
</script></body></html>`
