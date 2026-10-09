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
	browser := epath201FreshBrowser(t, ctx, chrome)
	browser.call("Page.enable", map[string]any{}, nil)
	browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 1600, "height": 1000, "deviceScaleFactor": 1, "mobile": false}, nil)
	browser.call("Page.navigate", map[string]any{"url": server.URL + "/chart"}, nil)
	for deadline := time.Now().Add(30 * time.Second); ; {
		status := string(browser.evaluate(`document.body?.dataset.componentChartStatus`))
		if status == `"passed"` {
			t.Log(string(browser.evaluate(`document.getElementById('result').textContent`)))
			if os.Getenv("ENERGY_PATH_COMPONENT_REVIEW") != "" {
				directory, err := os.MkdirTemp(os.Getenv("ENERGY_PATH_COMPONENT_REVIEW"), "component-chart-review-")
				if err != nil {
					t.Fatal(err)
				}
				for _, capture := range []struct {
					frequency, theme, language string
					font                       int
				}{{"monthly", "dark", "ko", 18}, {"hourly", "light", "en", 12}} {
					browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 1200, "height": 700, "deviceScaleFactor": 1, "mobile": false}, nil)
					browser.evaluate(fmt.Sprintf(`(async()=>{window.energyPathComponentReview(%q,%q,%q,%d);await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));})()`, capture.frequency, capture.theme, capture.language, capture.font))
					var screenshot struct {
						Data string `json:"data"`
					}
					browser.call("Page.captureScreenshot", map[string]any{"format": "png", "fromSurface": true, "captureBeyondViewport": false}, &screenshot)
					data, err := base64.StdEncoding.DecodeString(screenshot.Data)
					if err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(directory, capture.frequency+"-"+capture.theme+"-"+capture.language+".png")
					if err := os.WriteFile(path, data, 0o644); err != nil {
						t.Fatal(err)
					}
					t.Logf("temporary native component chart screenshot: %s", path)
				}
			}
			return
		}
		if status == `"failed"` || time.Now().After(deadline) {
			t.Fatalf("Energy component chart browser %s: %s", status, browser.evaluate(`document.getElementById('result')?.textContent`))
		}
		time.Sleep(25 * time.Millisecond)
	}
}

const energyPathComponentChartHTML = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/src/styles.css"></head><body class="guide-page"><div id="mount" style="width:720px;margin:20px"></div><pre id="result" hidden></pre>
<script type="module">
const check=(value,message)=>{if(!value)throw new Error(message);};
const tick=()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
try {
 const {energyPathMonthlyChartSeries:monthly,energyPathHourlyChartSeries:hourlyModel,renderEnergyPathComponentChart:render}=await import('/src/js/energy-path-chart.js');
 const {setLanguage}=await import('/src/js/i18n.js');setLanguage('en');
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
 check([...mount.querySelectorAll('[data-energy-path-chart-value]')].map(mark=>mark.dataset.energyPathChartValue).join(',')==='50,0','monthly intensity formatting changed raw chart values or drew missing months');
 check(mount.querySelectorAll('option').length===2&&mount.querySelector('[data-energy-path-chart-frequency]').value==='monthly','chart frequency choices incorrect');
 check(mount.querySelector('rect title')?.textContent.includes('5.00 kWh/m²'),'monthly intensity lost precise unit/denominator');
 check([...mount.querySelectorAll('rect title')].some(node=>node.textContent.includes('0.00 kWh/m²')),'reported zero disappeared');
 check(!mount.querySelector('[data-energy-path-detail-section]')&&!mount.textContent.includes('Calculation / allocation basis'),'old inspector details remain');
 mount.innerHTML=render({item,frequency:'hourly',hourly:hours,display});
 check([...mount.querySelectorAll('[data-energy-path-chart-value]')].map(mark=>mark.dataset.energyPathChartValue).join(',')==='4,0','hourly intensity formatting changed native source values or known zero');
 check(mount.querySelector('[data-energy-path-chart-axis="y"]')?.textContent==='Measured energy (kWh/m²)'&&mount.querySelector('[data-energy-path-chart-axis="x"]')?.textContent==='Date & hour'&&mount.querySelector('circle title')?.textContent.includes('0.40 kWh/m²'),'hourly curve lost proper axes or intensity');
 check(!mount.textContent.includes('Office')&&!mount.textContent.includes('Cooling energy')&&!mount.querySelector('p'),'chart lists source objects, raw variable names or old explanatory text');
 mount.innerHTML=render({item,frequency:'hourly',hourly:[],display});check(mount.querySelector('[data-energy-path-chart-empty]')&&!mount.querySelector('svg'),'missing hourly data fabricated a chart');
 mount.innerHTML=render({item,monthly:months,display:{perArea:true,areaM2:null}});check(mount.querySelector('[data-energy-path-chart-empty]'),'missing area produced a false intensity');
 const appearance=[],conversion={...link,label:'Cooling demand from occupied perimeter rooms and adjoining internal zones'},appearanceHours=hourly({sourceIds:['gain','loss']},[gain,loss]);
 const markGeometry=()=>JSON.stringify([...mount.querySelectorAll('[data-energy-path-chart-value],.energy-path-chart-line')].map(mark=>[mark.dataset.energyPathChartValue,...['d','x','y','width','height','cx','cy'].map(name=>mark.getAttribute(name))]));
 const physicalFont=element=>parseFloat(getComputedStyle(element).fontSize)*element.getScreenCTM().a;
 const boundsCheck=label=>{
  const svg=mount.querySelector('svg'),viewBox=svg.viewBox.baseVal;
  for(const text of svg.querySelectorAll('text')){const bounds=text.getBBox();check(bounds.x>=-.1&&bounds.y>=-.1&&bounds.x+bounds.width<=viewBox.width+.1&&bounds.y+bounds.height<=viewBox.height+.1,'component chart text clips outside SVG '+JSON.stringify({label,text:text.textContent,x:bounds.x,y:bounds.y,width:bounds.width,height:bounds.height}));}
  const ticks=[...svg.querySelectorAll('[data-energy-path-chart-tick="x"]')].map(text=>text.getBoundingClientRect());
  check(ticks.every((box,index)=>!index||ticks[index-1].right+2<=box.left),'component chart date/month labels overlap '+label);
 };
 for(const frequency of['monthly','hourly'])for(const [language,theme,font,width]of[['en','light',11,320],['ko','dark',12,720],['en','dark',18,350],['ko','light',18,1423]]){
  setLanguage(language);mount.innerHTML=render({item:conversion,kind:'link',frequency,monthly:links,hourly:appearanceHours,display});
  const mark=mount.querySelector('[data-energy-path-chart-value]'),geometry=markGeometry();document.documentElement.dataset.theme=theme;document.documentElement.style.setProperty('--graph-label-font-size',font+'px');mount.style.width=width+'px';await tick();
  const panel=mount.querySelector('[data-energy-path-component-chart]'),plot=panel.querySelector('.energy-path-chart-plot'),family=getComputedStyle(document.body).fontFamily,caption=panel.querySelector('header strong'),control=panel.querySelector('select'),legend=panel.querySelector('.energy-path-chart-legend');
  check(plot&&getComputedStyle(plot).overflowX==='auto'&&panel.getBoundingClientRect().width<=width+.1&&mount.scrollWidth<=width+1,'component plot lost loaded CSS or scrolls the narrow page instead of its own chart '+JSON.stringify({frequency,width}));
  check(mount.querySelector('[data-energy-path-chart-value]')===mark&&markGeometry()===geometry,'component chart font/theme/width changed source observations or numeric mark geometry');
  for(const element of panel.querySelectorAll('.energy-path-chart-tick,.energy-path-chart-axis-label'))check(Math.abs(physicalFont(element)-(Math.max(12,font)+(element.classList.contains('energy-path-chart-axis-label')?1:0)))<.06,'component chart physical font ignores Settings '+JSON.stringify({frequency,language,font,width,text:element.textContent,size:physicalFont(element)}));
  for(const element of[caption,control,legend,panel.querySelector('.energy-path-chart-tick'),panel.querySelector('.energy-path-chart-axis-label')])check(getComputedStyle(element).fontFamily===family,'component chart font family differs from app family');
  check(parseFloat(getComputedStyle(caption).fontSize)>parseFloat(getComputedStyle(control).fontSize)&&Number(getComputedStyle(caption).fontWeight)>Number(getComputedStyle(control).fontWeight)&&parseFloat(getComputedStyle(legend).fontSize)===Math.max(12,font),'component title/control/legend typography hierarchy is inconsistent');
  boundsCheck(JSON.stringify({frequency,language,theme,font,width}));
  check(frequency!=='monthly'||panel.querySelectorAll('[data-energy-path-chart-tick="x"]').length===12,'monthly appearance correction discarded original month labels');
  appearance.push({frequency,language,theme,font,width});
 }
 const largeMonthly=links.map(trace=>({...trace,points:trace.points.map(point=>({...point,value:point.value===null?null:point.value*1000000}))}));
 mount.innerHTML=render({item:conversion,kind:'link',monthly:largeMonthly,display});mount.style.width='350px';await tick();boundsCheck('large reported monthly totals');check([...mount.querySelectorAll('[data-energy-path-chart-value]')].map(mark=>mark.dataset.energyPathChartValue).join(',')==='40000000,10000000','large monthly axis padding changed actual reported values');
 window.energyPathComponentReview=(frequency,theme,language,font)=>{
  setLanguage(language);document.documentElement.dataset.theme=theme;document.documentElement.style.setProperty('--graph-label-font-size',font+'px');mount.style.width='1140px';
  const previewMonths=monthly(link,'link',[42,38,35,28,24,30,47,55,43,31,37,46].map(value=>({links:[{...link,fromValue:value,toValue:value/4}]}))),labels=Array.from({length:24},(_value,index)=>'01-01 '+String(index+1).padStart(2,'0')+':00');
  const previewSources=[gain,loss].map((source,trace)=>({...source,hourlyEnergy:{...source.hourlyEnergy,values:labels.map((_label,index)=>Math.round((3+trace+2*Math.sin(index/4+trace))*100)/100)}}));
  mount.innerHTML=render({item:conversion,kind:'link',frequency,monthly:previewMonths,hourly:hourly({sourceIds:['gain','loss']},previewSources,{},labels),display});
 };
 check(JSON.stringify({item,graphs})===original&&JSON.stringify(sources)===sourceJSON,'chart changed original observations or accounting data');
 document.body.dataset.componentChartStatus='passed';document.getElementById('result').textContent=JSON.stringify({status:'passed',appearance});
}catch(error){document.body.dataset.componentChartStatus='failed';document.getElementById('result').textContent=error.stack;}
</script></body></html>`
