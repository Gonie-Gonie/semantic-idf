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

func TestEnergyPathComponentChartDataBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless Energy component chart data acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/chart", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathComponentChartHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/chart").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-component-chart-status="passed"`) {
		t.Fatalf("Energy component chart acceptance failed: %v\n%s", err, output)
	}
}

const energyPathComponentChartHTML = `<!doctype html><html><body><div id="mount"></div><pre id="result"></pre>
<script type="module">
const check=(value,message)=>{if(!value)throw new Error(message);};
try {
 const {energyPathMonthlyChartSeries:monthly,energyPathHourlyChartSeries:hourlyModel,renderEnergyPathComponentChart:render}=await import('/src/js/energy-path-chart.js');
 const hourlyLabels=['01-01 01:00','01-01 02:00','02-01 01:00'];
 const hourly=(item,sources,state={},labels=hourlyLabels)=>hourlyModel(item,sources,state,labels);
 const item={id:'group',label:'Other',unit:'kWh',value:99999,allocationApplied:true,sourceIds:['derived'],groupedMembers:[{id:'a'},{id:'b'}]};
 const graphs=[{nodes:[{id:'a',value:20},{id:'b',value:30}]},{nodes:[{id:'changed-other',groupedMembers:[{id:'a',value:0},{id:'b',value:0},{id:'unrelated',value:500}]}]},{nodes:[{id:'a',value:9}]}];
 const original=JSON.stringify({item,graphs});
 const months=monthly(item,'node',graphs);
 check(months[0].points.length===12&&months[0].points[0].value===50&&months[0].points[1].value===0,'monthly chart lost selected original group members or reported zero');
 check(months[0].points[2].value===null&&months[0].points[11].value===null,'missing monthly members were replaced with partial/annual totals');
 const link={id:'conversion',fromUnit:'kWh thermal',toUnit:'kWh site'};
 const links=monthly(link,'link',[{links:[{...link,fromValue:40,toValue:10}]}]);
 check(links.length===2&&links[0].points[0].value===40&&links[1].points[0].value===10,'conversion chart conflated thermal and site values');
 const sources=[{id:'derived',inputSourceIds:['hourly','hourly','missing'],allocationFactor:.05},{id:'hourly',name:'Cooling energy',keyValue:'Office',multiplierApplication:'requires_zone_multiplier',effectiveMultiplier:2,hourlyEnergy:{basis:'reported_source',unit:'kWh',values:[2,0,9]}},{id:'missing',name:'No canonical hourly data'}];
 const sourceJSON=JSON.stringify(sources),hours=hourly(item,sources,{simulationEnergyPeriod:'M1'});
 check(hours.length===1&&hours[0].points.length===2&&hours[0].points[0].value===4&&hours[0].points[1].value===0,'hourly source tracing lost identities, real zero, month filter or proven multiplier');
 check(hours[0].points[0].value!==item.value*.05,'hourly chart fabricated annual allocation');
 const siblingSources=[{id:'monthly-a',name:'Cooling energy',keyValue:'Office',reportingFrequency:'Monthly',multiplierApplication:'requires_zone_multiplier',effectiveMultiplier:2},{id:'monthly-b',name:'Cooling energy',keyValue:'Office',reportingFrequency:'Monthly',multiplierApplication:'requires_zone_multiplier',effectiveMultiplier:2},sources[1]];
 const siblings=hourly({sourceIds:['monthly-a','monthly-b']},siblingSources,{simulationEnergyPeriod:'M1'});
 check(siblings.length===1&&siblings[0].id==='hourly'&&siblings[0].points[0].value===4,'separate Monthly dictionary references failed exact Hourly sibling matching/deduplication');
 check(hourly({sourceIds:['monthly-a']},[...siblingSources,{...sources[1],id:'duplicate-hourly'}]).length===0,'duplicate Hourly identities were silently chosen or combined');
 check(hourly(item,[{id:'derived',inputSourceIds:['only-monthly']},{id:'only-monthly',rawValue:999,reportingFrequency:'Monthly'}]).length===0,'monthly scalar became an hourly curve');
 const duplicate=structuredClone(sources);duplicate[1].hourlyEnergy.values.push(2);
 check(hourly(item,duplicate,{},[...hourlyLabels,hourlyLabels[0]]).length===0,'ambiguous repeated hourly calendars were silently combined');
 check(hourlyModel(item,sources).length===0,'compact hourly values acquired a fabricated calendar');
 const native=structuredClone(sources);delete native[1].multiplierApplication;delete native[1].effectiveMultiplier;
 const nativeHours=hourly(item,native,{simulationEnergyPeriod:'M1'});check(nativeHours[0].points[0].value===2,'unknown multiplier acquired a guessed contribution');
 const second={...structuredClone(sources[1]),id:'hourly-lab',keyValue:'Private laboratory object'};second.hourlyEnergy.values=[3,5,7];
 const grouped=hourly({sourceIds:['hourly','hourly-lab']},[sources[1],second],{simulationEnergyPeriod:'M1'});
 check(grouped.length===1&&grouped[0].points[0].value===10&&grouped[0].points[1].value===10,'homogeneous metric aggregation lost proven multipliers or aligned the wrong shared hours');
 const incomplete=structuredClone(second);incomplete.hourlyEnergy.values.pop();
 check(hourly({sourceIds:['hourly','hourly-lab']},[sources[1],incomplete]).length===0,'incomplete shared-axis values became partial hourly totals');
 const nullValue=structuredClone(second);nullValue.hourlyEnergy.values[1]=null;
 check(hourly({sourceIds:['hourly','hourly-lab']},[sources[1],nullValue]).length===0,'missing shared-axis value became zero or a partial hourly total');
 const missing={id:'monthly-missing',name:sources[1].name,keyValue:'Missing zone',reportingFrequency:'Monthly'};
 check(hourly({sourceIds:['hourly','monthly-missing']},[sources[1],missing]).length===0,'missing homogeneous source became a partial displayed total');
 const gain={...structuredClone(sources[1]),id:'gain',name:'Zone Mixing Sensible Heat Gain Energy'},loss={...structuredClone(sources[1]),id:'loss',name:'Zone Mixing Sensible Heat Loss Energy'};
 const directions=hourly({sourceIds:['gain','loss']},[gain,loss],{simulationEnergyPeriod:'M1'});check(directions.length===2&&directions[0].label!==directions[1].label,'different measurement directions were combined');
 const calendar=structuredClone(sources);calendar[1].hourlyEnergy.values=[1,2];
 const nonLeap=hourly(item,calendar,{},['02-28 24:00','03-01 01:00']);check(nonLeap[0].points[1].x-nonLeap[0].points[0].x===1,'non-leap February introduced an artificial missing day');
 const mount=document.getElementById('mount'),display={perArea:true,areaM2:10};
 mount.innerHTML=render({item,monthly:months,display});
 check(mount.querySelectorAll('option').length===2&&mount.querySelector('[data-energy-path-chart-frequency]').value==='monthly','chart frequency choices incorrect');
 check(mount.querySelector('rect title')?.textContent.includes('5.00 kWh/m²'),'monthly intensity lost precise unit/denominator');
 check([...mount.querySelectorAll('rect title')].some(node=>node.textContent.includes('0.00 kWh/m²')),'reported zero disappeared');
 check(!mount.querySelector('[data-energy-path-detail-section]')&&!mount.textContent.includes('Calculation / allocation basis'),'old inspector details remain');
 mount.innerHTML=render({item,frequency:'hourly',hourly:hours,display});
 check(mount.querySelector('[data-energy-path-chart-axis="y"]')?.textContent==='Measured energy (kWh/m²)'&&mount.querySelector('[data-energy-path-chart-axis="x"]')?.textContent==='Date & hour'&&mount.querySelector('circle title')?.textContent.includes('0.40 kWh/m²'),'hourly curve lost proper axes or intensity');
 check(!mount.textContent.includes('Office')&&!mount.textContent.includes('Cooling energy')&&!mount.querySelector('p'),'chart lists source objects, raw variable names or old explanatory text');
 mount.innerHTML=render({item,frequency:'hourly',hourly:[],display});check(mount.querySelector('[data-energy-path-chart-empty]')&&!mount.querySelector('svg'),'missing hourly data fabricated a chart');
 mount.innerHTML=render({item,monthly:months,display:{perArea:true,areaM2:null}});check(mount.querySelector('[data-energy-path-chart-empty]'),'missing area produced a false intensity');
 check(JSON.stringify({item,graphs})===original&&JSON.stringify(sources)===sourceJSON,'chart changed original observations or accounting data');
 document.body.dataset.componentChartStatus='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.componentChartStatus='failed';document.getElementById('result').textContent=error.stack;}
</script></body></html>`
