package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestComfortInspectionBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("Comfort graph and indicator browser regression")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/comfort", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, comfortInspectionBrowserHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/comfort").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-comfort-inspection-status="passed"`) {
		diagnostic := regexp.MustCompile(`(?s)<pre id="result"[^>]*>(.*?)</pre>`).FindSubmatch(output)
		if len(diagnostic) > 1 {
			t.Fatalf("Comfort inspection browser: %v\n%s", err, diagnostic[1])
		}
		t.Fatalf("Comfort inspection browser: %v\n%s", err, output)
	}
}

const comfortInspectionBrowserHTML = `<!doctype html><html data-theme="dark"><head><link rel="stylesheet" href="/src/styles.css"></head><body style="display:block;overflow:auto"><div id="mount" style="max-width:1100px"></div><pre id="result"></pre>
<script type="module">
const check=(condition,message)=>{if(!condition)throw Error(message);};
try {
 const {renderComfortInspection:render,handleComfortInspectionEvent:handle,renderComfortInspectionReport:renderReport}=await import('/src/js/views/comfort-inspection-view.js');
 const {prepareComfortInspection:prepare}=await import('/src/js/comfort-inspection-data.js');
 const close=(left,right)=>Math.abs(left-right)<1e-7;
 const drawn=trace=>{
  const svg=trace.closest('svg[data-hvac-chart-domain-low]'),axis=svg.querySelector('[data-hvac-chart-axis="'+trace.dataset.hvacChartSeriesAxis+'"]'),cursor=svg.querySelector('[data-hvac-chart-frame]');
  const low=Number(axis.dataset.hvacChartYLow),high=Number(axis.dataset.hvacChartYHigh),top=Number(cursor.getAttribute('y1')),bottom=Number(cursor.getAttribute('y2')),first=Number(svg.dataset.hvacChartDomainLow),last=Number(svg.dataset.hvacChartDomainHigh),left=Number(svg.dataset.hvacChartPlotLeft),width=Number(svg.dataset.hvacChartPlotWidth);
  return [...trace.querySelector('path.hvac-chart-line').getAttribute('d').matchAll(/([ML])([^,\s]+),([^\s]+)/g)].map(match=>({command:match[1],x:first+(Number(match[2])-left)/width*(last-first),value:high-(Number(match[3])-top)/(bottom-top)*(high-low)}));
 };
 const traceFor=(container,metric)=>container.querySelector('[data-hvac-chart-series="'+CSS.escape(metric.id)+'"]');
 const nativeVertices=(trace,expected)=>{const vertices=drawn(trace);return vertices.length===expected.length&&vertices.every((point,index)=>close(point.x,expected[index].x)&&close(point.value,expected[index].value));};
 const points=values=>values.map((value,x)=>({x,value,label:'01/21 0'+(x+1)+':00:00'}));
 const metric=(name,unit,values,keyValue)=>({name,unit,keyValue,source:'eplusout.sql',reportingFrequency:'Hourly',min:-999,max:999,average:999,points:points(values)});
 const zoneA={zoneName:'Zone A',metrics:[
  metric('Zone Mean Air Temperature','C',[20,null,24]),
  metric('Zone Thermostat Heating Setpoint Temperature','C',[19,19,19]),
  metric('Zone Thermostat Cooling Setpoint Temperature','C',[26,26,26]),
  metric('Zone Thermal Comfort Fanger Model PMV','',[0,null,0.8],'People A'),
  metric('Zone Thermal Comfort Fanger Model PPD','%',[5,null,18],'People A'),
  metric('Zone Air Relative Humidity','%',[40,45,50]),
  metric('Zone Air System Sensible Heating Rate','W',[1100,1200,1300]),
  metric('Zone Air System Sensible Cooling Energy','J',[3600000,7200000,10800000]),
 ]};
 const zoneB={zoneName:'Zone B',metrics:[metric('Zone Mean Air Temperature','C',[30,30,30]),metric('Zone Thermal Comfort Fanger Model PMV','',[-1,-1,-1],'People B')]};
 const fixture={zones:[zoneA,zoneB],buildingMetrics:[],unmetHours:[
  {zoneName:'Zone A',scope:'zone',metric:'Time Setpoint Not Met During Occupied Heating',value:2,unit:'hr',source:'eplusout.sql',report:'Comfort source report',table:'Raw source table'},
  {zoneName:'Zone B',scope:'zone',metric:'Time Setpoint Not Met During Occupied Heating',value:3,unit:'hr'},
  {zoneName:'Entire Facility',scope:'building',metric:'Time Setpoint Not Met During Occupied Heating',value:7.5,unit:'hr'},
 ],completeness:[{variable:'Zone Thermal Comfort Fanger Model PMV',status:'partial',source:'eplusout.sql'}]};
 const original=JSON.stringify(fixture),mount=document.getElementById('mount'),ui={};
 mount.innerHTML=render(fixture,ui);
 mount.addEventListener('change',event=>handle(event,mount,fixture,ui));
 const select=value=>{const input=mount.querySelector('[data-comfort-inspect-zone]');check(input&&[...input.options].some(option=>option.value===value),'missing Comfort scope '+value);input.value=value;input.dispatchEvent(new Event('change',{bubbles:true}));};
 const chart=kind=>mount.querySelector('[data-comfort-inspect-chart="'+kind+'"]');
 const indicator=id=>mount.querySelector('[data-comfort-inspect-metric="'+id+'"]');
 const average=id=>{const item=indicator(id);return Number((item?.querySelector('[data-comfort-inspect-value]')||item)?.dataset.comfortInspectValue);};
 select('Zone A');
 check(average('temperature')===22&&average('pmv')===0.4&&average('ppd')===11.5,'zone indicators used stale summaries or treated missing samples as zero');
 const temperature=chart('temperature'),pmv=chart('pmv'),ppd=chart('ppd');
 const zoneAModel=prepare(fixture).zones.find(zone=>zone.name==='Zone A'),zoneMetric=kind=>zoneAModel.metrics.find(metric=>metric.kind===kind);
 check(temperature?.querySelectorAll('[data-hvac-chart-series]').length===3&&['temperature','heatingSetpoint','coolingSetpoint'].every(kind=>nativeVertices(traceFor(temperature,zoneMetric(kind)),zoneMetric(kind).points.filter(point=>point.value!==null))),'T/Tset graph omitted native zone values or combined separate properties');
 check([...temperature.querySelectorAll('[data-hvac-chart-series]')].flatMap(drawn).every(point=>!close(point.value,30)),'selected zone temperature graph mixed another zone');
 const pmvTrace=pmv?.querySelector('[data-hvac-chart-series]');
 check(pmvTrace&&!pmvTrace.querySelector('circle')&&nativeVertices(pmvTrace,zoneMetric('pmv').points.filter(point=>point.value!==null))&&drawn(pmvTrace).length===2&&close(drawn(pmvTrace)[0].value,0)&&zoneMetric('pmv').points[1].value===null,'PMV line graph lost real zero or fabricated a missing observation');
 check((pmvTrace.querySelector('path').getAttribute('d').match(/M/g)||[]).length===2,'PMV graph connected across missing data');
 check(ppd?.querySelector('[data-hvac-chart-axis="left"]').textContent.includes('%')&&nativeVertices(traceFor(ppd,zoneMetric('ppd')),zoneMetric('ppd').points.filter(point=>point.value!==null))&&nativeVertices(traceFor(chart('humidity'),zoneMetric('relativeHumidity')),zoneMetric('relativeHumidity').points),'PPD/RH graphs lost their native values or units');
 for(const kind of ['temperature','pmv','ppd','humidity']){
  const item=chart(kind);check(item?.querySelector('[data-hvac-chart-axis="x"]')&&item.querySelector('[data-hvac-chart-axis="left"]')&&item.querySelectorAll('.hvac-chart-grid').length>3,'Comfort '+kind+' graph lacks labeled axes and grid');
 }
 const clean=()=>check(!mount.querySelector('table, dl, [data-semantic-id], [data-semantic-ref], [data-hvac-graph-key]')&&!['eplusout.sql','Comfort source report','Raw source table','Related model entities','Source data','Heating Rate','Cooling Energy','3600000'].some(text=>mount.textContent.includes(text)),'Comfort contains energy, raw data details, tables or linked model objects');
 clean();
 select('Zone B');check(average('temperature')===30&&average('pmv')===-1&&[...chart('temperature').querySelectorAll('[data-hvac-chart-series]')].flatMap(drawn).every(point=>close(point.value,30)),'zone picker failed to isolate the next zone');
 select('__building__');
 const reported=[...mount.querySelectorAll('[data-comfort-inspect-hours]')].map(item=>Number(item.dataset.comfortInspectHours));
 check(reported.includes(7.5)&&!reported.includes(5)&&!reported.includes(2)&&!reported.includes(3),'building indicator summed zone hours or lost the directly reported building value');
 check(!indicator('pmv')&&!indicator('temperature')&&!chart('pmv')&&!chart('temperature'),'building view fabricated PMV/temperature by averaging zones');
 clean();
 select('Zone A');check(average('temperature')===22&&average('pmv')===0.4,'returning to the zone changed its metrics');
 check(JSON.stringify(fixture)===original,'Comfort rendering or scope selection mutated returned results');
 const missing={zones:[{zoneName:'Missing zone',metrics:[metric('Zone Mean Air Temperature','C',[null,null,null])]}]};
 mount.innerHTML=render(missing,{});
 check(!mount.querySelector('[data-hvac-chart-series] circle,path.hvac-chart-line')&&!mount.querySelector('[data-comfort-inspect-value="0"], [data-comfort-inspect-value="999"]'),'all-missing comfort data manufactured a value or reused stale summary');
 const report=document.createElement('div');report.innerHTML=renderReport(fixture);
 check(report.querySelector('[data-comfort-inspect-chart] svg')&&!report.querySelector('table,[data-hvac-chart-series] circle')&&!report.textContent.includes('eplusout.sql')&&nativeVertices(traceFor(report,zoneMetric('pmv')),zoneMetric('pmv').points.filter(point=>point.value!==null)),'Comfort report restored source tables/markers or lost native comfort observations');
 check(!mount.querySelector('[data-hvac-chart-series] circle')&&!report.querySelector('[data-hvac-chart-series] circle'),'Comfort screens/reports retained point markers');
 const annualTimes=new Map(),firstHour=Date.UTC(2001,0,1,1)/3600000;
 const annualPoints=value=>Array.from({length:8760},(_,index)=>{
  const day=new Date(Date.UTC(2001,0,1+Math.floor(index/24))),label='2001-'+String(day.getUTCMonth()+1).padStart(2,'0')+'-'+String(day.getUTCDate()).padStart(2,'0')+' '+String(index%24+1).padStart(2,'0')+':00';
  annualTimes.set(label,firstHour+index);return {x:34*Math.floor(index/4)+[7,14,23,30][index%4],label,value:value(index)};
 });
 const annual={zones:[{zoneName:'Annual',metrics:[
  {...metric('Zone Thermal Comfort Fanger Model PMV','',[],'Annual People'),points:annualPoints(index=>index%21===0?0:(index%21-10)/10)},
  {...metric('Zone Thermal Comfort Fanger Model PPD','%',[],'Annual People'),points:annualPoints(index=>5+index%31)},
 ]}]};
 const assertAnnual=(captured,segments,message)=>{
  const before=JSON.stringify(captured),prepared=prepare(captured).zones[0];mount.innerHTML=render(captured,{});
  check(!mount.querySelector('[data-hvac-chart-series] circle'),'annual Comfort trace retained point markers');
  for(const kind of ['pmv','ppd']){
   const projected=prepared.metrics.find(metric=>metric.kind===kind),source=captured.zones[0].metrics.find(metric=>metric.name.endsWith(kind.toUpperCase())),trace=traceFor(mount,projected),vertices=drawn(trace),native=new Map(projected.points.map(point=>[point.x,point]));
   check(projected.points.length===source.points.length&&projected.points.every((point,index)=>point.x===annualTimes.get(source.points[index].label)&&point.key===String(source.points[index].x)&&point.label===source.points[index].label&&Object.is(point.value,source.points[index].value)),message+': native calendar/key or complete source observations changed for '+kind);
   check(vertices.length>2&&vertices.filter(point=>point.command==='M').length===segments&&vertices.every(point=>{const observed=native.get(Math.round(point.x));return observed&&close(point.x,observed.x)&&observed.value!==null&&close(point.value,observed.value);}),message+': line used SQL row spacing, invented values or crossed a real gap for '+kind);
   check(close(vertices[0].x,projected.points[0].x)&&close(vertices.at(-1).x,projected.points.at(-1).x)&&close(vertices[0].value,projected.points[0].value)&&close(vertices.at(-1).value,projected.points.at(-1).value),message+': sampled geometry lost native endpoints');
  }
  check(JSON.stringify(captured)===before,message+': rendering mutated original raw SQL rows/labels/values');return prepared;
 };
 const annualModel=assertAnnual(annual,1,'continuous annual hourly observations');
 check(annualModel.metrics.find(metric=>metric.kind==='pmv').points[0].value===0&&annualModel.metrics.every(metric=>metric.points.every((point,index)=>index===0||point.x-metric.points[index-1].x===1)),'annual SQL row-ID changes created false calendar gaps or lost a real zero');
 const omitted=structuredClone(annual);for(const metric of omitted.zones[0].metrics)metric.points.splice(4000,1);assertAnnual(omitted,2,'omitted native hourly observation');
 const explicitNull=structuredClone(annual);for(const metric of explicitNull.zones[0].metrics)metric.points[4000].value=null;assertAnnual(explicitNull,2,'explicit null hourly observation');
 const legacy=structuredClone(annual);for(const metric of legacy.zones[0].metrics){delete metric.reportingFrequency;metric.points=metric.points.slice(0,72);}legacy.zones[0].metrics[1].reportingFrequency='';assertAnnual(legacy,1,'legacy empty-frequency calendar observations');
 const annualReport=document.createElement('div');annualReport.innerHTML=renderReport(annual);check(!annualReport.querySelector('[data-hvac-chart-series] circle')&&annualModel.metrics.every(metric=>drawn(traceFor(annualReport,metric)).filter(point=>point.command==='M').length===1),'annual Comfort report restored markers or false gaps');
 for(const [name,labels]of [
  ['invalid',['2001-01-01 01:00','not a date','2001-01-01 03:00']],
  ['invalid-day',['2001-02-28 01:00','2001-02-30 02:00','2001-03-01 03:00']],
  ['reversed',['2001-01-01 03:00','2001-01-01 02:00','2001-01-01 01:00']],
  ['repeated',['2001-01-01 01:00','2001-01-01 01:00','2001-01-01 03:00']],
  ['year-rollover',['12-31 23:00','01-01 00:00','01-01 01:00']],
 ]){
  const captured={zones:[{zoneName:name,metrics:[{...metric('Zone Thermal Comfort Fanger Model PMV','',[]),points:labels.map((label,index)=>({x:[7,14,23][index],label,value:[0,-.5,.8][index]}))}]}]},before=JSON.stringify(captured),projected=prepare(captured).zones[0].metrics[0];mount.innerHTML=render(captured,{});
  check(projected.points.length===3&&projected.points.every((point,index)=>point.x===[7,14,23][index]&&point.value===[0,-.5,.8][index]&&point.label===labels[index])&&nativeVertices(traceFor(mount,projected),projected.points)&&JSON.stringify(captured)===before,'unvalidated '+name+' labels invented a calendar, chose an ambiguous observation or changed original native values');
 }
 const category={zones:[{zoneName:'Mixed category clock',metrics:[metric('Zone Mean Air Temperature','C',[20,21,22]),metric('Zone Thermostat Heating Setpoint Temperature','C',[19,19,19]),metric('Zone Thermal Comfort Fanger Model PMV','',[0,.1,.2])]}]};
 for(const metric of category.zones[0].metrics)metric.points.forEach((point,index)=>{point.x=[7,14,23][index];point.label='2001-01-01 0'+(index+1)+':00';});category.zones[0].metrics[1].points[1].label='invalid';const categoryBefore=JSON.stringify(category),categoryModel=prepare(category).zones[0];mount.innerHTML=render(category,{});
 check(categoryModel.metrics.filter(metric=>metric.category==='temperature').every(metric=>metric.points.every((point,index)=>point.x===[7,14,23][index]))&&categoryModel.metrics.find(metric=>metric.kind==='pmv').points.every((point,index)=>point.x===firstHour+index)&&JSON.stringify(category)===categoryBefore,'one category mixed raw SQL rows with calendar coordinates or invalidated an independent valid comfort category');
 const leap={zones:[{zoneName:'Yearless January',metrics:[metric('Zone Mean Air Temperature','C',[20,21])]}, {zoneName:'Leap context',metrics:[metric('Zone Thermal Comfort Fanger Model PMV','',[0,.2])]}]};leap.zones[0].metrics[0].points.forEach((point,index)=>point.label='01-01 0'+(index+1)+':00');leap.zones[1].metrics[0].points.forEach((point,index)=>point.label='2024-02-29 0'+(index+1)+':00');const leapBefore=JSON.stringify(leap),leapModel=prepare(leap);
 check(leapModel.zones[0].metrics[0].points[0].x===Date.UTC(2024,0,1,1)/3600000&&leapModel.zones[1].metrics[0].points[0].x===Date.UTC(2024,1,29,1)/3600000&&JSON.stringify(leap)===leapBefore,'Comfort result did not share its unique explicit year/leap context across zones');
 const yearlessLeap=structuredClone(leap);for(const point of yearlessLeap.zones[1].metrics[0].points)point.label=point.label.slice(5);check(prepare(yearlessLeap).zones[0].metrics[0].points[0].x===Date.UTC(2000,0,1,1)/3600000,'yearless Feb29 did not establish one leap-year context for the whole result');
 const ambiguousYear=structuredClone(leap);ambiguousYear.zones.push({zoneName:'Another explicit year',metrics:[{...metric('Zone Air Relative Humidity','%',[40,45]),points:[{x:7,label:'2025-01-01 01:00',value:40},{x:14,label:'2025-01-01 02:00',value:45}]}]});check(prepare(ambiguousYear).zones[0].metrics[0].points.every((point,index)=>point.x===index),'yearless labels amidst multiple explicit years acquired an invented calendar year');
 const sparse={zones:[{zoneName:'Sparse Hourly',metrics:[{...metric('Zone Thermal Comfort Fanger Model PMV','',[]),points:[{x:7,label:'2001-01-01 01:00',value:0},{x:14,label:'2001-01-01 03:00',value:.8}]}]}]},sparseMetric=prepare(sparse).zones[0].metrics[0];mount.innerHTML=render(sparse,{});check(drawn(traceFor(mount,sparseMetric)).filter(point=>point.command==='M').length===2,'explicit Hourly cadence connected across a real missing hour when every reported interval was sparse');
 const mixedCase=structuredClone(sparse);mixedCase.zones[0].metrics[0].reportingFrequency=' hOuRlY ';const mixedCaseBefore=JSON.stringify(mixedCase),mixedCaseMetric=prepare(mixedCase).zones[0].metrics[0];mount.innerHTML=render(mixedCase,{});check(drawn(traceFor(mount,mixedCaseMetric)).filter(point=>point.command==='M').length===2&&JSON.stringify(mixedCase)===mixedCaseBefore,'exact native Hourly spelling/whitespace lost cadence authority or was rewritten in returned metadata');
 const duplicated={zones:[{zoneName:'Duplicate Native Row',metrics:[{...metric('Zone Thermal Comfort Fanger Model PMV','',[]),points:[{x:7,label:'2001-01-01 01:00',value:0},{x:14,label:'2001-01-01 02:00',value:.5},{x:14,label:'2001-01-01 02:00',value:.7},{x:23,label:'2001-01-01 03:00',value:1}]}]}]},duplicateBefore=JSON.stringify(duplicated),duplicateMetric=prepare(duplicated).zones[0].metrics[0];mount.innerHTML=render(duplicated,{});check(duplicateMetric.points.length===3&&duplicateMetric.points[1].value===null&&nativeVertices(traceFor(mount,duplicateMetric),duplicateMetric.points.filter(point=>point.value!==null))&&drawn(traceFor(mount,duplicateMetric)).filter(point=>point.command==='M').length===2&&JSON.stringify(duplicated)===duplicateBefore,'duplicate native SQL observation was selected/interpolated or its real gap disappeared');
 const comparison=await import('/src/js/simulation-comparison-data.js'),comparisonView=await import('/src/js/views/simulation-comparison-view.js');
 const reads={},nativeValues={};
 const native=(id,key,name,unit,cadence,values,labels=['01-01 01:00','01-01 02:00','01-01 03:00'],rows=[7,14,23],file='eplusout.sql')=>{
  const original=values.map((value,index)=>({x:rows[index],label:labels[index],value}));nativeValues[id]=original;reads[id]=0;
  const source={sourceId:id,keyValue:key,name,file,column:key+':'+name+' ['+unit+']('+cadence+')',reportingFrequency:cadence};Object.defineProperty(source,'points',{enumerable:true,get(){reads[id]++;return original;}});return source;
 };
 const outside=native('weather','Environment','Site Outdoor Air Drybulb Temperature','C','Hourly',[-5,0,8]),rh=native('weather-rh','Environment','Site Outdoor Air Relative Humidity','%','Hourly',[30,45,60]),inside=native('inside','ROOM','Zone Mean Air Temperature','C','Hourly',[20,0,22],undefined,[900,930,960]),unrelated=native('unrelated','Other node','System Node Temperature','C','Hourly',[99,99,99]);
 const request=(source,purposes=['basic_energy','zone_heat_flow','comfort_check'])=>({objectType:'Output:Variable',keyValue:source.keyValue,variableName:source.name,reportingFrequency:source.reportingFrequency,purposeIds:purposes});
 let comfortReads=0,heatReads=0,heatLabels=0,energyReads=0;const comfortPoints=nativeValues.inside.map(point=>({...point})),heatAxis=['01-01 01:00','01-01 02:00','01-01 03:00'];
 const fullMetric={name:inside.name,keyValue:'ROOM',source:'eplusout.sql',unit:'C',reportingFrequency:'Hourly'};Object.defineProperty(fullMetric,'points',{get(){comfortReads++;return comfortPoints;}});
 const heatZone={name:'ROOM',observed:[[true,false,true]],temperature:[20,0,22],temperatureObserved:[true,true,true]};Object.defineProperty(heatZone,'values',{get(){heatReads++;return [[1000,0,5000]];}});
 const heat={sourceFile:'eplusout.sql',unit:'W',temperatureUnit:'C',frameCount:3,categories:[{id:'internalConvective',label:'Internal gains',variableName:'Zone Air Heat Balance Internal Convective Heat Gain Rate'}],zones:[heatZone]};Object.defineProperty(heat,'labels',{get(){heatLabels++;return heatAxis;}});
 const hourly={unit:'kWh',basis:'reported_source'};Object.defineProperty(hourly,'values',{get(){energyReads++;return [0,2,4];}});
 const relevant={series:[outside,rh,inside,unrelated],purposeRunPlan:{outputObjects:[request(outside),request(rh),request(inside,['comfort_check']),request(unrelated,['hvac_loop_check']),{objectType:'Output:Variable',keyValue:'*',variableName:heat.categories[0].variableName,reportingFrequency:'Hourly',purposeIds:['zone_heat_flow']}]},purposeResults:{comfort:{zones:[{zoneName:'ROOM',metrics:[fullMetric]}]},zoneHeatFlow:heat,energyExplanation:{hourlyLabels:heatAxis,sources:[{id:'energy-native',sourceFile:'eplusout.sql',name:'Electricity',keyValue:'Building',hourlyEnergy:hourly}]}}};
 const nativeBefore=JSON.stringify(nativeValues),context={simulationComfortZone:'ROOM',simulationEnergyPeriod:'M2'},model=comparison.prepareSimulationComparison(relevant,'comfort',context),temperatureProperty=model.properties.find(property=>property.category==='comfort'&&property.kind==='temperature'),weatherProperty=model.properties.find(property=>property.seriesId==='weather'),comparisonUI={rows:[{entity:temperatureProperty.entityId,property:temperatureProperty.id},{entity:weatherProperty.entityId,property:weatherProperty.id}]};
 for(const view of ['comfort','zone_heat_flow','energy'])comparison.prepareSimulationComparison(relevant,view,context);
 const selectedIDs=comparison.selectedSimulationComparisonSeriesIDs(relevant,'comfort',comparisonUI,context);
 check(Object.values(reads).every(value=>value===0)&&comfortReads===0&&heatReads===0&&heatLabels===0&&energyReads===0&&selectedIDs.sort().join(',')==='inside,weather'&&!model.properties.some(property=>property.seriesId==='unrelated')&&model.properties.filter(property=>property.seriesId==='inside').length===1,'comparison metadata catalog/source selection read unrelated lazy columns, mixed purposes or duplicated native source identity');
 const comparisonMount=document.createElement('div');document.body.append(comparisonMount);comparisonMount.innerHTML=comparisonView.renderSimulationComparison(relevant,'comfort',comparisonUI,context);
 for(const type of ['change','input','click','pointerover','pointerout','focusin','focusout'])comparisonMount.addEventListener(type,event=>comparisonView.handleSimulationComparisonEvent(event,comparisonMount,relevant,'comfort',comparisonUI,context));
 check(comfortReads===1&&reads.weather===1&&reads.inside===0&&reads['weather-rh']===0&&reads.unrelated===0&&heatReads===0&&heatLabels===0&&energyReads===0&&!comparisonMount.querySelector('[data-hvac-chart-series] circle'),'comparison rendering loaded unselected sources/purposes or retained line markers');
 const mode=comparisonMount.querySelector('[data-simulation-comparison-mode]');mode.value='scatter';mode.dispatchEvent(new Event('change',{bubbles:true}));
 check(comparisonMount.querySelectorAll('[data-hvac-chart-scatter] circle').length===3&&comparisonMount.querySelector('[data-hvac-chart-x-value="0"][data-hvac-chart-y-value="0"]'),'comparison scatter used SQL row IDs instead of actual equal-time Hourly observations or lost zero');
 const pairs=(result,view,properties,ctx={})=>{const ui={mode:'scatter',rows:properties.map(property=>({entity:property.entityId,property:property.id}))};comparisonMount.innerHTML=comparisonView.renderSimulationComparison(result,view,ui,ctx);return comparisonMount.querySelectorAll('[data-hvac-chart-scatter] circle').length;};
 const monthlyA=native('month-a','Building','Monthly A','J','Monthly',[0,3600000,7200000],['Jan','Feb','Mar']),monthlyB=native('month-b','Building','Monthly B','J','Monthly',[18000000,0,3600000],['Jan','Feb','Mar'],[101,202,303]),monthly={series:[monthlyA,monthlyB],purposeRunPlan:{outputObjects:[request(monthlyA),request(monthlyB)]}},monthlyModel=comparison.prepareSimulationComparison(monthly,'energy',{});
 check(pairs(monthly,'energy',monthlyModel.properties)===3&&[...comparisonMount.querySelectorAll('[data-hvac-chart-scatter] circle')].some(point=>close(Number(point.dataset.hvacChartXValue),1)&&Number(point.dataset.hvacChartYValue)===0),'native Monthly variables did not pair by the same actual month or lost J→kWh/zero values');
 const timestep=native('step','ROOM','Timestep value','C','Timestep',[1,2,3]);timestep.nativeIntervals=nativeValues.step.map(point=>({timeIndex:point.x,environmentPeriodIndex:3,year:2017,simulationDays:1,intervalMinutes:15}));const cadenceResult={series:[outside,timestep],purposeRunPlan:{outputObjects:[request(outside),request(timestep)]}},roomScope={simulationComfortZone:'ROOM'},cadenceModel=comparison.prepareSimulationComparison(cadenceResult,'comfort',roomScope);
 check(cadenceModel.properties.length===2&&pairs(cadenceResult,'comfort',cadenceModel.properties,roomScope)===0,'same endpoint Hourly averages and Timestep observations were incorrectly paired');
 const duplicate=native('duplicate','ROOM','Duplicate time','C','Hourly',[1,2,3],['01-01 01:00','01-01 01:00','01-01 03:00']),duplicates={series:[outside,duplicate],purposeRunPlan:{outputObjects:[request(outside),request(duplicate)]}},duplicateModel=comparison.prepareSimulationComparison(duplicates,'comfort',roomScope);
 check(duplicateModel.properties.length===2&&pairs(duplicates,'comfort',duplicateModel.properties,roomScope)===1,'ambiguous duplicate observation time selected a value or borrowed its nearest neighbor');
 const otherFile=native('other-file','ROOM','Other file','C','Hourly',[1,2,3],undefined,undefined,'other.sql'),fileResult={series:[outside,otherFile],purposeRunPlan:{outputObjects:[request(outside),request(otherFile)]}},fileModel=comparison.prepareSimulationComparison(fileResult,'comfort',roomScope);
 check(fileModel.properties.length===2&&pairs(fileResult,'comfort',fileModel.properties,roomScope)===0,'comparison paired independent output files just because timestamps happened to match');
 const hfContext={simulationHeatFlowSelectedZone:'ROOM',simulationHeatFlowFrameIndex:0,simulationHeatFlowRangeStart:0,simulationHeatFlowRangeEnd:2,simulationEnergyPeriod:'M2'},hfModel=comparison.prepareSimulationComparison(relevant,'zone_heat_flow',hfContext),heatProperty=hfModel.properties.find(property=>property.kind==='internalConvective'),hfWeather=hfModel.properties.find(property=>property.seriesId==='weather'),hfUI={rows:[{entity:heatProperty.entityId,property:heatProperty.id},{entity:hfWeather.entityId,property:hfWeather.id}]};
 comparisonMount.innerHTML=comparisonView.renderSimulationComparison(relevant,'zone_heat_flow',hfUI,hfContext);const heatTrace=comparison.simulationComparisonTrace(hfModel,heatProperty);check(heatTrace.points.map(point=>point.value).join(',')==='1,,5','HeatFlow comparison lost the observation mask, W→kW scaling or leaked Energy period filtering');
 const originalComparisonPath=comparisonMount.querySelector('path.hvac-chart-line');comparisonView.updateSimulationComparisonFrame(comparisonMount,relevant,'zone_heat_flow',hfUI,{...hfContext,simulationHeatFlowFrameIndex:2});check(comparisonMount.querySelector('path.hvac-chart-line')===originalComparisonPath&&Number(comparisonMount.querySelector('[data-hvac-chart-frame]').dataset.hvacChartFrame)===Date.UTC(2001,0,1,3)/3600000,'comparison frame-only update recreated paths or used an array/SQL row index instead of its actual timestamp');
 const reused=comparison.prepareSimulationComparison(relevant,'zone_heat_flow',{...hfContext,simulationHeatFlowRangeStart:1,simulationHeatFlowFrameIndex:1});check(reused===hfModel&&reused.context.simulationHeatFlowFrameIndex===1&&comparison.simulationComparisonTrace(reused,heatProperty).points.length===2,'frame/range changes retained a stale cursor context or rebuilt unbounded metadata catalogs');
 const scoped={...relevant,purposeResults:{...relevant.purposeResults,comfort:{zones:[{zoneName:'ROOM',metrics:[fullMetric]},{zoneName:'OTHER',metrics:[{name:inside.name,unit:'C',source:'eplusout.sql',reportingFrequency:'Hourly',points:points([30,31,32])}]},{zoneName:'Building',metrics:[{name:inside.name,unit:'C',source:'eplusout.sql',reportingFrequency:'Hourly',points:points([40,41,42])}]}]}}},scopeUI={};comparison.selectedSimulationComparisonSeriesIDs(scoped,'comfort',scopeUI,roomScope);const otherModel=comparison.prepareSimulationComparison(scoped,'comfort',{simulationComfortZone:'OTHER'});comparison.normalizeSimulationComparison(otherModel,scopeUI);check(!otherModel.entities.some(entity=>entity.name==='ROOM')&&otherModel.properties.find(property=>property.id===scopeUI.rows[0].property)?.entityId===otherModel.entities.find(entity=>entity.name==='OTHER').id,'scope change retained an unrelated old Zone or failed to select its new related variable');
 const buildingModel=comparison.prepareSimulationComparison(scoped,'comfort',{simulationComfortZone:'__building__'}),buildingUI={};comparison.normalizeSimulationComparison(buildingModel,buildingUI);check(buildingModel.entities.every(entity=>entity.category==='environment')&&buildingUI.rows[0].property&&!buildingUI.rows[1].property,'Building without reported building observations silently selected a Zone or invented an aggregate');
 check(JSON.stringify(Object.fromEntries(['weather','weather-rh','inside','unrelated'].map(id=>[id,nativeValues[id]])))===nativeBefore&&heatZone.observed[0].join(',')==='true,false,true','comparison mutated original native source observations/masks');
 document.body.dataset.comfortInspectionStatus='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.comfortInspectionStatus='failed';document.getElementById('result').textContent=error.stack;}
</script></body></html>`
