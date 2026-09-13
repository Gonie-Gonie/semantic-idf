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

func TestHVACInspectionDataBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless HVAC data alignment acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/data", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, hvacInspectionDataHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/data").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-hvac-data-status="passed"`) {
		t.Fatalf("HVAC data alignment failed: %v\n%s", err, output)
	}
}

const hvacInspectionDataHTML = `<!doctype html><body><pre id="result"></pre><script type="module">
const check=(v,m)=>{if(!v)throw Error(m)};
try {
 const {prepareHVACInspection:prepare,hvacInspectionTrace:trace,hvacInspectionSnapshots:snapshot,hvacInspectionBasicProperties:basic}=await import('/src/js/hvac-inspection-data.js');
 const point=(x,value,label)=>({x,value,label:label||'01-01 '+String(x+1).padStart(2,'0')+':00'});
 const series=(name,unit,points,key='NODE A',file='eplusout.sql')=>({name,keyValue:key,file,column:key+':'+name+' ['+unit+']',reportingFrequency:'Hourly',points});
 const temp=series('System Node Temperature','C',[point(0,20),point(1,21),point(2,22)]);
 const flow=series('System Node Mass Flow Rate','kg/s',[point(0,0),point(2,3)]);
 const humidity=series('System Node Humidity Ratio','kgWater/kgDryAir',[point(0,.008),point(1,.009),point(2,.010)]);
 const rh=series('System Node Relative Humidity','%',[point(0,40),point(1,45),point(2,50)]);
 const setpoint=series('System Node Setpoint Temperature','C',[point(0,21),point(1,-999),point(2,23)]);
 const power=series('Fan Electricity Rate','W',[point(0,0),point(1,400),point(2,null)],'FAN');
 const loop={name:'Main loop',series:[temp,flow,humidity,rh,setpoint],components:[{componentName:'FAN',componentType:'Fan:VariableVolume',inletNodes:['NODE A'],outletNodes:['NODE B'],series:[power]}]};
 const original=JSON.stringify(loop),model=prepare(loop),node=model.entities.find(e=>e.kind==='node'),fan=model.entities.find(e=>e.kind==='component');
 check(model.frames.length===3&&model.frames[2].x-model.frames[0].x===2,'actual hourly calendar not retained');
 const flowProperty=node.properties.find(p=>p.kind==='flow'),flowTrace=trace(model,flowProperty);
 check(flowTrace.points.map(p=>p.value).join(',')==='0,,3','missing middle sample shifted or acquired zero/previous value');
 check(snapshot(model,1).nodes[0].metrics.find(m=>m.id==='flow').value===null,'snapshot clamped missing sample');
 check(snapshot(model,0).nodes[0].active,'reported zero point lost frame highlighting');
 check(snapshot(model,0).components[0].status==='off'&&snapshot(model,1).components[0].status==='on'&&snapshot(model,2).components[0].status==='unknown','equipment state fabricated from missing/zero power');
 check(trace(model,fan.properties[0]).unit==='kW'&&trace(model,fan.properties[0]).points[1].value===.4,'power unit/value disagrees');
 check(fan.inletNodes[0]==='NODE A'&&fan.outletNodes[0]==='NODE B','exact nested device ports lost');
 const ratio=trace(model,node.properties.find(p=>p.kind==='humidity'));
 check(ratio.unit==='g/kg'&&ratio.points[0].value===8,'humidity ratio scaling mislabeled');
 check(basic(model,'humidity')[0].kind==='relativeHumidity','reported RH should be preferred in default humidity graph');
 check(snapshot(model,0).nodes[0].metrics.length===4&&snapshot(model,0).nodes[0].metrics.some(m=>m.id==='setpoint')&&!snapshot(model,0).nodes[0].metrics.some(m=>m.id==='humidity'),'redundant humidity hid setpoint');
 check(snapshot(model,1).nodes[0].metrics.find(m=>m.id==='setpoint').value===null,'unset setpoint sentinel shown as temperature');
 check(basic(model,'flow',{[node.id]:false}).length===0,'node toggle retained hidden trace');
 const shifted=structuredClone(loop); shifted.series.push(series('System Node Enthalpy','J/kg',[point(101,42000,'01-01 02:00')],'NODE A','eplusout.csv'));
 const shiftedModel=prepare(shifted),enthalpy=shiftedModel.entities[0].properties.find(p=>p.kind==='enthalpy');
 check(trace(shiftedModel,enthalpy).points.map(p=>p.value).join(',')===',42,','cross-file data joined raw row number instead of unique timestamp');
 const ambiguous=structuredClone(shifted); ambiguous.series.at(-1).points.push(point(102,43000,'01-01 02:00'));
 const ambiguousModel=prepare(ambiguous);
 check(trace(ambiguousModel,ambiguousModel.entities[0].properties.find(p=>p.kind==='enthalpy')).points.every(p=>p.value===null),'ambiguous cross-file calendar silently joined');
 const repeated=structuredClone(loop); repeated.series[1].points.push(point(0,4));
 const repeatedModel=prepare(repeated);
 check(trace(repeatedModel,repeatedModel.entities[0].properties.find(p=>p.kind==='flow')).points[0].value===null,'duplicate same-file time silently chosen');
 const mixed=structuredClone(loop); mixed.series.push({...temp,reportingFrequency:'Monthly',points:[point(9,100)]});
 check(prepare(mixed).frames.length===3,'Monthly observations mixed into frame timeline');
 const fuel={...structuredClone(loop),components:[{componentName:'GAS COIL',componentType:'Coil:Heating:Fuel',series:[series('Heating Coil Electricity Rate','W',[point(0,0)],'GAS COIL'),series('Heating Coil Heating Rate','W',[point(0,10000)],'GAS COIL')]}]};
 check(snapshot(prepare(fuel),0).components[0].status==='on','positive fuel heating output marked Off from zero ancillary electric power');
 const blank={name:'Empty',series:[],components:[]};check(prepare(blank).frames.length===0,'empty result invented frames');
 for(const loopType of ['PlantLoop','CondenserLoop']){
  const water={...loop,loopType},waterModel=prepare(water);
  check(waterModel.waterLoop&&basic(waterModel,'humidity').length===0,'water loop exposes default humidity graph data');
  check(!waterModel.entities.filter(e=>e.kind==='node').some(e=>e.properties.some(p=>/humidity/i.test(p.name))),'water loop offers humidity in custom property selector');
  check(snapshot(waterModel,0).nodes[0].metrics.length===3&&snapshot(waterModel,0).nodes[0].metrics.find(m=>m.id==='temperature').value===20,'water snapshot retained RH or lost temperature/flow/setpoint');
 }
 const {setLanguage}=await import('/src/js/i18n.js');setLanguage('ko');
 check(prepare(loop)!==model&&prepare(loop).entities[0].properties.find(p=>p.kind==='temperature').label!==node.properties.find(p=>p.kind==='temperature').label,'language change retained cached labels');setLanguage('en');
 check(JSON.stringify(loop)===original,'inspection mutated original result');
 document.body.dataset.hvacDataStatus='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.hvacDataStatus='failed';document.getElementById('result').textContent=error.stack;}
</script>`
