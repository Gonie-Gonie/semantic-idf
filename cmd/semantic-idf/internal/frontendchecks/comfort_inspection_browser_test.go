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
 const points=values=>values.map((value,x)=>({x,value,label:'01/21 0'+(x+1)+':00:00'}));
 const metric=(name,unit,values,keyValue)=>({name,unit,keyValue,source:'eplusout.sql',min:-999,max:999,average:999,points:points(values)});
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
 check(temperature?.querySelectorAll('[data-hvac-chart-series]').length===3&&[19,20,24,26].every(value=>temperature.querySelector('[data-hvac-chart-value="'+value+'"]')),'T/Tset graph omitted zone values or combined separate properties');
 check(!temperature.querySelector('[data-hvac-chart-value="30"]'),'selected zone temperature graph mixed another zone');
 const pmvTrace=pmv?.querySelector('[data-hvac-chart-series]');
 check(pmvTrace?.querySelectorAll('circle').length===2&&pmvTrace.querySelector('[data-hvac-chart-time="0"][data-hvac-chart-value="0"]')&&!pmvTrace.querySelector('[data-hvac-chart-time="1"]'),'PMV graph lost real zero or fabricated missing observation');
 check((pmvTrace.querySelector('path').getAttribute('d').match(/M/g)||[]).length===2,'PMV graph connected across missing data');
 check(ppd?.querySelector('[data-hvac-chart-axis="left"]').textContent.includes('%')&&chart('humidity')?.querySelector('[data-hvac-chart-value="45"]'),'PPD/RH graphs lost their values or units');
 for(const kind of ['temperature','pmv','ppd','humidity']){
  const item=chart(kind);check(item?.querySelector('[data-hvac-chart-axis="x"]')&&item.querySelector('[data-hvac-chart-axis="left"]')&&item.querySelectorAll('.hvac-chart-grid').length>3,'Comfort '+kind+' graph lacks labeled axes and grid');
 }
 const clean=()=>check(!mount.querySelector('table, dl, [data-semantic-id], [data-semantic-ref], [data-hvac-graph-key]')&&!['eplusout.sql','Comfort source report','Raw source table','Related model entities','Source data','Heating Rate','Cooling Energy','3600000'].some(text=>mount.textContent.includes(text)),'Comfort contains energy, raw data details, tables or linked model objects');
 clean();
 select('Zone B');check(average('temperature')===30&&average('pmv')===-1&&!chart('temperature').querySelector('[data-hvac-chart-value="20"]'),'zone picker failed to isolate the next zone');
 select('__building__');
 const reported=[...mount.querySelectorAll('[data-comfort-inspect-hours]')].map(item=>Number(item.dataset.comfortInspectHours));
 check(reported.includes(7.5)&&!reported.includes(5)&&!reported.includes(2)&&!reported.includes(3),'building indicator summed zone hours or lost the directly reported building value');
 check(!indicator('pmv')&&!indicator('temperature')&&!chart('pmv')&&!chart('temperature'),'building view fabricated PMV/temperature by averaging zones');
 clean();
 select('Zone A');check(average('temperature')===22&&average('pmv')===0.4,'returning to the zone changed its metrics');
 check(JSON.stringify(fixture)===original,'Comfort rendering or scope selection mutated returned results');
 const missing={zones:[{zoneName:'Missing zone',metrics:[metric('Zone Mean Air Temperature','C',[null,null,null])]}]};
 mount.innerHTML=render(missing,{});
 check(!mount.querySelector('[data-hvac-chart-series] circle')&&!mount.querySelector('[data-comfort-inspect-value="0"], [data-comfort-inspect-value="999"]'),'all-missing comfort data manufactured a value or reused stale summary');
 const report=document.createElement('div');report.innerHTML=renderReport(fixture);
 check(report.querySelector('[data-comfort-inspect-chart] svg')&&!report.querySelector('table')&&!report.textContent.includes('eplusout.sql'),'Comfort report restored source tables or omitted comfort graphs');
 document.body.dataset.comfortInspectionStatus='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.comfortInspectionStatus='failed';document.getElementById('result').textContent=error.stack;}
</script></body></html>`
