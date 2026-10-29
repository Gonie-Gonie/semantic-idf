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
					font, width                int
				}{{"monthly", "dark", "ko", 12, 740}, {"monthly", "dark", "ko", 18, 740}, {"hourly", "light", "en", 12, 1140}, {"outlier", "dark", "en", 12, 740}} {
					browser.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 1200, "height": 700, "deviceScaleFactor": 1, "mobile": false}, nil)
					browser.evaluate(fmt.Sprintf(`(async()=>{window.energyPathComponentReview(%q,%q,%q,%d,%d);await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));})()`, capture.frequency, capture.theme, capture.language, capture.font, capture.width))
					var screenshot struct {
						Data string `json:"data"`
					}
					browser.call("Page.captureScreenshot", map[string]any{"format": "png", "fromSurface": true, "captureBeyondViewport": false}, &screenshot)
					data, err := base64.StdEncoding.DecodeString(screenshot.Data)
					if err != nil {
						t.Fatal(err)
					}
					path := filepath.Join(directory, fmt.Sprintf("%s-%s-%s-%d-%d.png", capture.frequency, capture.theme, capture.language, capture.font, capture.width))
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
 const {energyPathMonthlyChartSeries:monthly,energyPathHourlyChartSeries:hourlyModel,energyPathLinkRatioChartSeries:ratioModel,renderEnergyPathComponentChart:render}=await import('/src/js/energy-path-chart.js');
 const {energyPathConversionRatio:validateRatio}=await import('/src/js/views/energy-path-view.js');
 const {connectTimeSeriesCharts}=await import('/src/js/time-series-chart.js');
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
 const pair={...link,relation:'load_to_end_use',serviceKind:'cooling',ratioKind:'coefficient_of_performance',ratio:4,fromId:'load.cooling',toId:'end_use.cooling',fromValue:100,toValue:25};
 const pairGraph={nodes:[{id:pair.fromId,level:'load',serviceKind:'cooling'},{id:pair.toId,level:'end_use',endUse:'cooling'},{id:'carrier.electricity',level:'carrier',carrier:'electricity'}],links:[pair,{id:'carrier-branch',fromId:pair.toId,toId:'carrier.electricity',relation:'end_use_to_carrier'}]};
 const ratioOptions={from:'Cooling load',to:'Cooling equipment',ratioForLink:(record,graph)=>validateRatio(record,graph.nodes,graph.links)};
 const actualRatio=validateRatio(pair,pairGraph.nodes,pairGraph.links),pairMonths=monthly(pair,'link',[pairGraph,{...pairGraph,links:[{...pair,fromValue:30,toValue:10,ratio:3}]},{...pairGraph,links:[{...pair,fromValue:0,toValue:10,ratio:0}]},{...pairGraph,links:[{...pair,toValue:0,ratio:0}]}],ratioOptions);
 const actualSeries=ratioModel(pair,pairMonths,actualRatio)[0];
 check(actualSeries.label==='COP'&&actualSeries.points[0].value===4&&actualSeries.points[1].value===3&&actualSeries.points[2].value===null&&actualSeries.points[3].value===null&&actualSeries.points[11].value===null,'COP chart invented monthly ratio evidence, zero-denominator values or missing-month observations');
 check(actualSeries.points[2].unavailableReason==='unverified_pair'&&actualSeries.points[3].unavailableReason==='zero_denominator'&&actualSeries.points[11].unavailableReason==='missing_pair','ratio chart collapsed unverified boundaries, actual zero denominators and missing observations into one reason');
 const smallDenominatorMonths=[{id:'fromValue',label:'Cooling load',unit:'kWh',points:[{x:0,label:'Jan',value:4,ratioKind:actualRatio.kind},{x:1,label:'Feb',value:1.8503,ratioKind:actualRatio.kind},{x:2,label:'Mar',value:10,ratioKind:actualRatio.kind}]},{id:'toValue',label:'Cooling equipment',unit:'kWh',points:[{x:0,label:'Jan',value:1,ratioKind:actualRatio.kind},{x:1,label:'Feb',value:.0001,ratioKind:actualRatio.kind},{x:2,label:'Mar',value:2,ratioKind:actualRatio.kind}]}];
 const smallDenominatorRatio=ratioModel(pair,smallDenominatorMonths,actualRatio)[0];
 check(smallDenominatorRatio.points[1].value===18503&&smallDenominatorRatio.points[1].toValue===.0001&&smallDenominatorRatio.points[1].unavailableReason==='','tiny but nonzero denominator was capped, discarded or replaced with a zero-denominator gap');
 check(pairMonths[0].label==='Cooling load'&&pairMonths[1].label==='Cooling equipment','paired quantity legends lost actual endpoint names');
 const references={id:'reference',relation:'input_to_driver',fromUnit:'kWh',toUnit:'kWh',fromValue:-20,toValue:5,ratioKind:'coefficient_of_performance'};
 const referenceMonths=monthly(references,'link',[{links:[references]},{links:[{...references,fromValue:0,toValue:5}]},{links:[{...references,fromValue:9,toValue:0}]}]);
 const referenceRatio=ratioModel(references,referenceMonths,actualRatio)[0];
 check(referenceRatio.label==='Reference quantity ratio'&&referenceRatio.ratioKind==='quantity_ratio'&&referenceRatio.points[0].value===-4&&referenceRatio.points[1].value===0&&referenceRatio.points[2].value===null,'boundary reference ratio lost sign/known zero or acquired equipment COP');
 const merged={id:'merged',fromUnit:'kWh',toUnit:'kWh',groupedMembers:[{id:'small'},{id:'large'}]};
 const summedRatio=ratioModel(merged,monthly(merged,'link',[{links:[{id:'small',fromValue:10,toValue:5},{id:'large',fromValue:90,toValue:20}]}]))[0];
 check(summedRatio.points[0].value===4,'grouped ratio averaged individual ratios instead of dividing summed paired quantities');
 const convertedUnits=ratioModel(pair,[{id:'fromValue',unit:'MWh',points:[{x:0,label:'Jan',value:1}]},{id:'toValue',unit:'kWh',points:[{x:0,label:'Jan',value:250}]}])[0];
 check(convertedUnits.points[0].value===4,'quantity ratio divided incompatible energy-unit magnitudes');
 const correspondenceLinks=[{id:'cooling-correspondence',relation:'source_correspondence',fromId:'lighting.cooling',toId:'lighting.site',fromValue:8,toValue:10},{id:'heating-correspondence',relation:'source_correspondence',fromId:'lighting.heating',toId:'lighting.site',fromValue:5,toValue:10}],correspondence={id:'grouped-correspondence',relation:'source_correspondence',fromUnit:'kWh',toUnit:'kWh',groupedMembers:correspondenceLinks};
 const correspondenceMonths=monthly(correspondence,'link',[{relations:correspondenceLinks},{relations:[correspondenceLinks[0],{...correspondenceLinks[1],toValue:11}]},{relations:[{...correspondence,groupedMembers:[...correspondenceLinks,{id:'unrelated-correspondence',fromId:'equipment.cooling',toId:'lighting.site',fromValue:99,toValue:10}]}]}]);
 check(correspondenceMonths[0].points[0].value===13&&correspondenceMonths[1].points[0].value===10&&ratioModel(correspondence,correspondenceMonths)[0].points[0].value===1.3,'service correspondences counted a repeated direct-use endpoint twice or used the whole grouped Internal gains value');
 check(correspondenceMonths[1].points[1].value===null&&correspondenceMonths.every(trace=>trace.points[2].value===null),'conflicting correspondence endpoint values or an unrelated group member produced a quantity/ratio');
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
 mount.innerHTML=render({item:pair,kind:'link',monthly:pairMonths,display,endpointLabels:ratioOptions,conversionRatio:actualRatio});
 check(mount.querySelector('[data-energy-path-current-ratio]')?.textContent==='4.00'&&[...mount.querySelectorAll('[data-energy-path-chart-ratio]')].map(mark=>mark.dataset.energyPathChartRatio).join(',')==='4,3','selected relation omitted current ratio or plotted source quantities as COP');
 check(mount.querySelector('[data-energy-path-chart-panel="ratio"] svg')?.dataset.energyPathChartScale==='linear'&&!mount.querySelector('[data-energy-path-chart-scale-note]'),'ordinary COP values acquired an undisclosed or unnecessary nonlinear scale');
 check(mount.querySelector('[data-energy-path-chart-gap="zero_denominator"] title')?.textContent.includes('denominator is zero')&&mount.querySelector('[data-energy-path-chart-gap="unverified_pair"] title')?.textContent.includes('not verified')&&mount.querySelector('[data-energy-path-chart-gap="missing_pair"] title')?.textContent.includes('missing')&&mount.querySelector('[data-energy-path-chart-gap-note]')?.textContent.includes('not zero'),'unavailable ratio points disappeared or their reason was shown as numeric zero');
 const ratioGeometry=mount.querySelector('[data-energy-path-chart-ratio]').getAttribute('cy');
 mount.innerHTML=render({item:pair,kind:'link',monthly:pairMonths,display:{perArea:true,areaM2:1000},conversionRatio:actualRatio});
 check(mount.querySelector('[data-energy-path-current-ratio]')?.textContent==='4.00'&&mount.querySelector('[data-energy-path-chart-ratio]')?.getAttribute('cy')===ratioGeometry,'area normalization changed the dimensionless ratio');
 mount.innerHTML=render({item:pair,kind:'link',monthly:smallDenominatorMonths,display,conversionRatio:actualRatio});
 const outlierPanel=mount.querySelector('[data-energy-path-chart-panel="ratio"]'),outlierMarks=[...outlierPanel.querySelectorAll('[data-energy-path-chart-ratio]')];
 check(outlierPanel.querySelector('svg')?.dataset.energyPathChartScale==='linear'&&outlierPanel.querySelector('svg')?.dataset.energyPathChartYMax==='5'&&mount.querySelector('[data-energy-path-chart-panel="quantities"] svg')?.dataset.energyPathChartScale==='linear'&&outlierMarks.map(mark=>mark.dataset.energyPathChartRatio).join(',')==='4,18503,5','large-ratio display changed raw observations, flattened ordinary values or changed the endpoint energy axis');
 check(outlierPanel.querySelector('[data-energy-path-chart-scale-note]')?.textContent.includes('10× the median')&&outlierPanel.querySelector('[data-energy-path-chart-denominator-note]')?.textContent.includes('fan/pump')&&outlierMarks[1].dataset.energyPathChartOutlier==='upper'&&outlierMarks[1].querySelector('title')?.textContent.includes('Cooling equipment: 1e-5 kWh/m²')&&outlierPanel.querySelector('[data-energy-path-chart-outlier-summary]')?.textContent.includes('Feb: 18,503.00'),'large ratio lacks a disclosed linear outlier policy, break marker, actual-value summary or precise tiny denominator');
 check(!outlierPanel.querySelector('.energy-path-chart-line').getAttribute('d').includes('L'),'ordinary ratio line connected through an excluded spike');
 const outlierBaseline=Number(outlierPanel.querySelector('.energy-path-chart-axis').getAttribute('y1'));
 check(outlierBaseline-Number(outlierMarks[0].getAttribute('cy'))>20,'large monthly ratio flattened all ordinary COP values to the baseline');
 const ratioMarkGeometry=()=>[...mount.querySelectorAll('[data-energy-path-chart-ratio]')].map(mark=>mark.getAttribute('cy')||mark.getAttribute('transform')).join(',');
 const outlierGeometry=ratioMarkGeometry();
 mount.innerHTML=render({item:pair,kind:'link',monthly:smallDenominatorMonths,display:{perArea:true,areaM2:1000},conversionRatio:actualRatio});
 check(ratioMarkGeometry()===outlierGeometry,'area normalization changed the linear ratio geometry or off-scale marker');
 const area100Monthly=smallDenominatorMonths.map(trace=>({...trace,points:trace.points.map(point=>({...point,value:point.value*10}))}));
 mount.innerHTML=render({item:pair,kind:'link',monthly:area100Monthly,display:{perArea:true,areaM2:100},conversionRatio:actualRatio});
 check(mount.querySelector('[data-energy-path-chart-ratio="18503"] title')?.textContent.includes('Cooling equipment: 1e-5 kWh/m²')&&mount.querySelector('[data-energy-path-chart-denominator-note]')?.textContent.includes('not the equipment\'s rated COP'),'ratio tooltip lost scoped intensity or large delivered-load ratio was presented as rated equipment COP');
 for(const areaM2 of[null,0]){
  mount.innerHTML=render({item:pair,kind:'link',monthly:smallDenominatorMonths,display:{perArea:true,areaM2},conversionRatio:actualRatio});
  const tooltip=mount.querySelector('[data-energy-path-chart-ratio="18503"] title')?.textContent;
  check(tooltip?.includes('Cooling equipment: — kWh/m²')&&!tooltip.includes('1e-4 kWh/m²')&&ratioMarkGeometry()===outlierGeometry,'missing/zero floor area relabelled raw tiny energy as intensity or changed dimensionless ratio geometry');
 }
 const missingEndpointMonths=smallDenominatorMonths.map(trace=>({...trace,points:trace.points.map(point=>({...point,value:trace.id==='toValue'&&point.x===1?null:point.value}))}));
 mount.innerHTML=render({item:pair,kind:'link',monthly:missingEndpointMonths,display,conversionRatio:actualRatio});
 check(mount.querySelector('[data-energy-path-chart-gap="missing_pair"]')?.getAttribute('aria-label').includes('Cooling equipment has no monthly observation'),'ratio gap did not identify the missing endpoint');
 const signedMonths=[{id:'fromValue',label:'Input',unit:'kWh',points:[{x:0,label:'Jan',value:-4},{x:1,label:'Feb',value:0},{x:2,label:'Mar',value:18503}]},{id:'toValue',label:'Driver',unit:'kWh',points:[{x:0,label:'Jan',value:1},{x:1,label:'Feb',value:1},{x:2,label:'Mar',value:1}]}];
 mount.innerHTML=render({item:references,kind:'link',monthly:signedMonths,display});
 check([...mount.querySelectorAll('[data-energy-path-chart-ratio]')].map(mark=>mark.dataset.energyPathChartRatio).join(',')==='-4,0,18503'&&mount.querySelector('[data-energy-path-chart-panel="ratio"] svg')?.dataset.energyPathChartScale==='linear'&&!mount.querySelector('[data-energy-path-chart-gap]')&&!mount.querySelector('[data-energy-path-chart-outlier]'),'insufficient observations triggered automatic outlier classification or lost signed values/known zero');
 const negativeSpike=signedMonths.map(trace=>({...trace,points:[...trace.points.slice(0,2),{x:2,label:'Mar',value:trace.id==='fromValue'?5:1},{x:3,label:'Apr',value:trace.id==='fromValue'?-18503:1}]}));
 mount.innerHTML=render({item:references,kind:'link',monthly:negativeSpike,display});
 check([...mount.querySelectorAll('[data-energy-path-chart-ratio]')].map(mark=>mark.dataset.energyPathChartRatio).join(',')==='-4,0,5,-18503'&&mount.querySelector('[data-energy-path-chart-outlier]')?.dataset.energyPathChartOutlier==='lower'&&mount.querySelector('[data-energy-path-chart-panel="ratio"] svg')?.dataset.energyPathChartYMin==='-4'&&mount.querySelector('[data-energy-path-chart-panel="ratio"] svg')?.dataset.energyPathChartYMax==='5'&&!mount.querySelector('[data-energy-path-chart-gap]'),'negative off-scale spike removed an observation, changed its sign or confused real zero with a missing ratio');
 const measuredRatios=[488.79,18503,48.55,30.91,17.25,8.54,6.59,7.41,11.67,31.93,97.58,145.97];
 const measuredMonths=smallDenominatorMonths.map(trace=>({...trace,points:measuredRatios.map((value,x)=>({x,label:['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'][x],value:trace.id==='fromValue'?value:1,ratioKind:actualRatio.kind}))}));
 mount.innerHTML=render({item:pair,kind:'link',monthly:measuredMonths,display,conversionRatio:actualRatio});
 check([...mount.querySelectorAll('[data-energy-path-chart-ratio]')].map(mark=>Number(mark.dataset.energyPathChartRatio)).join(',')===measuredRatios.join(',')&&[...mount.querySelectorAll('[data-energy-path-chart-outlier]')].map(mark=>mark.dataset.energyPathChartRatio).join(',')==='488.79,18503'&&mount.querySelector('[data-energy-path-chart-panel="ratio"] svg')?.dataset.energyPathChartYMax==='145.97','actual monthly COP pattern lost its chronological observations or failed to exclude only the two spikes from the linear axis range');
 connectTimeSeriesCharts(mount);
 const wheelZoom=async svg=>{const box=svg.getBoundingClientRect(),event=new WheelEvent('wheel',{bubbles:true,cancelable:true,deltaY:-360,clientX:box.left+box.width/2});svg.dispatchEvent(event);await tick();check(event.defaultPrevented,'time-series wheel escaped to page scrolling');};
 const ratioSVG=mount.querySelector('[data-energy-path-chart-panel="ratio"] svg'),quantitySVG=mount.querySelector('[data-energy-path-chart-panel="quantities"] svg'),summary=mount.querySelector('.energy-path-chart-quantities'),monthLegend=mount.querySelector('.energy-path-chart-legend'),nativeMonths=JSON.stringify(measuredMonths);
 const ratioY=[ratioSVG.dataset.energyPathChartYMin,ratioSVG.dataset.energyPathChartYMax],quantityRange=[quantitySVG.dataset.timeSeriesLow,quantitySVG.dataset.timeSeriesHigh],ratioDomain=[ratioSVG.dataset.timeSeriesLow,ratioSVG.dataset.timeSeriesHigh];
 await wheelZoom(ratioSVG);
 check(mount.querySelector('[data-energy-path-chart-panel="ratio"] svg')===ratioSVG&&Number(ratioSVG.dataset.timeSeriesHigh)-Number(ratioSVG.dataset.timeSeriesLow)<12,'monthly wheel zoom replaced the SVG or failed to narrow its time axis');
 check(JSON.stringify([ratioSVG.dataset.energyPathChartYMin,ratioSVG.dataset.energyPathChartYMax])===JSON.stringify(ratioY)&&ratioY[1]==='145.97','monthly wheel zoom changed the fixed linear Y range or reclassified off-scale COP observations');
 check(JSON.stringify([quantitySVG.dataset.timeSeriesLow,quantitySVG.dataset.timeSeriesHigh])===JSON.stringify(quantityRange)&&mount.querySelector('.energy-path-chart-quantities')===summary&&mount.querySelector('.energy-path-chart-legend')===monthLegend&&JSON.stringify(measuredMonths)===nativeMonths,'monthly zoom changed its sibling chart, full summary, legend or original observations');
 ratioSVG.dispatchEvent(new MouseEvent('dblclick',{bubbles:true,cancelable:true}));await tick();
 check(JSON.stringify([ratioSVG.dataset.timeSeriesLow,ratioSVG.dataset.timeSeriesHigh])===JSON.stringify(ratioDomain),'monthly double-click did not restore the original horizontal extent');
 const zoomHours=[{id:'native',label:'Reported cooling',unit:'kWh',points:Array.from({length:24},(_,x)=>({x,label:'01-01 '+String(x+1).padStart(2,'0')+':00',value:x===0?100:x===12?null:x%6}))}],nativeHoursJSON=JSON.stringify(zoomHours);
 mount.innerHTML=render({item:pair,frequency:'hourly',hourly:zoomHours,display});await tick();
 const hourlySVG=mount.querySelector('svg[data-time-series-chart]'),hourlyY=[hourlySVG.dataset.energyPathChartYMin,hourlySVG.dataset.energyPathChartYMax],hourlyLegend=mount.querySelector('.energy-path-chart-legend');
 await wheelZoom(hourlySVG);
 check(Number(hourlySVG.dataset.timeSeriesHigh)-Number(hourlySVG.dataset.timeSeriesLow)<23&&JSON.stringify([hourlySVG.dataset.energyPathChartYMin,hourlySVG.dataset.energyPathChartYMax])===JSON.stringify(hourlyY),'hourly wheel zoom changed Y bounds when the peak left the horizontal viewport');
 check((hourlySVG.querySelector('.energy-path-chart-line').getAttribute('d').match(/M/g)||[]).length===2&&mount.querySelector('.energy-path-chart-legend')===hourlyLegend&&JSON.stringify(zoomHours)===nativeHoursJSON,'hourly zoom filled a true missing-value gap, replaced the legend or changed native observations');
 for(const count of[2,3]){
  const clustered=[{id:'sparse-native',label:'Reported hourly energy',unit:'kWh',points:[...Array.from({length:count},(_,x)=>({x,label:'01-01 0'+(x+1)+':00',value:x+1})),{x:744,label:'02-01 01:00',value:4}]}],nativeCluster=JSON.stringify(clustered);
  mount.innerHTML=render({item:pair,frequency:'hourly',hourly:clustered,display});await tick();const svg=mount.querySelector('svg[data-time-series-chart]'),beforeY=[svg.dataset.energyPathChartYMin,svg.dataset.energyPathChartYMax],box=svg.getBoundingClientRect(),view=svg.viewBox.baseVal;
  const left=Number(svg.dataset.timeSeriesPlotLeft),width=Number(svg.dataset.timeSeriesPlotWidth),event=new WheelEvent('wheel',{bubbles:true,cancelable:true,deltaY:-360,clientX:box.left+left/view.width*box.width});
  svg.dispatchEvent(event);await tick();const ticks=[...svg.querySelectorAll('[data-energy-path-chart-tick="x"]')];
  check(event.defaultPrevented&&Number(svg.dataset.timeSeriesHigh)<744&&ticks.length===1&&ticks[0].getAttribute('text-anchor')==='start','Energy hourly time labels overlap when zoom exposes two/three clustered observations beside a month-long gap');
  for(const label of ticks){const bounds=label.getBBox();check(bounds.x>=left-.1&&bounds.x+bounds.width<=left+width+.1,'Energy hourly sparse timestamp clips outside the visible plot');}
  check(JSON.stringify([svg.dataset.energyPathChartYMin,svg.dataset.energyPathChartYMax])===JSON.stringify(beforeY)&&JSON.stringify(clustered)===nativeCluster,'Energy sparse label layout rescaled Y or changed original observations');
 }
 mount.innerHTML=render({item:references,kind:'link',monthly:referenceMonths,display,conversionRatio:actualRatio});
 check(mount.querySelector('[data-energy-path-reference-note]')?.textContent.includes('not a conversion efficiency')&&mount.querySelector('[data-energy-path-chart-panel="ratio"] h4')?.textContent==='Reference quantity ratio','reference relation ratio lacks physical-boundary explanation');
 mount.innerHTML=render({item:pair,kind:'link',frequency:'hourly',hourly:hours,display,conversionRatio:actualRatio});
 check(mount.querySelector('[data-energy-path-ratio-empty]')&&!mount.querySelector('[data-energy-path-chart-ratio]'),'native source traces acquired an unproven Hourly endpoint ratio');
 const appearance=[],conversion={...link,label:'Cooling demand from occupied perimeter rooms and adjoining internal zones'},appearanceHours=hourly({sourceIds:['gain','loss']},[gain,loss]);
 const markGeometry=()=>JSON.stringify([...mount.querySelectorAll('[data-energy-path-chart-value],.energy-path-chart-line')].map(mark=>[mark.dataset.energyPathChartValue,...['d','x','y','width','height','cx','cy'].map(name=>mark.getAttribute(name))]));
 const physicalFont=element=>parseFloat(getComputedStyle(element).fontSize)*element.getScreenCTM().a;
 const boundsCheck=label=>{
  for(const svg of mount.querySelectorAll('svg')){const viewBox=svg.viewBox.baseVal;
  for(const text of svg.querySelectorAll('text')){const bounds=text.getBBox();check(bounds.x>=-.1&&bounds.y>=-.1&&bounds.x+bounds.width<=viewBox.width+.1&&bounds.y+bounds.height<=viewBox.height+.1,'component chart text clips outside SVG '+JSON.stringify({label,text:text.textContent,x:bounds.x,y:bounds.y,width:bounds.width,height:bounds.height}));}
  const ticks=[...svg.querySelectorAll('[data-energy-path-chart-tick="x"]')].map(text=>text.getBoundingClientRect());
  check(ticks.every((box,index)=>!index||ticks[index-1].right+2<=box.left),'component chart date/month labels overlap '+label);}
 };
 for(const frequency of['monthly','hourly'])for(const [language,theme,font,width]of[['en','light',11,320],['ko','dark',12,740],['en','light',12,500],['en','dark',18,350],['ko','light',18,1423],['en','dark',12,1423],['ko','light',18,2400]]){
  setLanguage(language);mount.innerHTML=render({item:conversion,kind:'link',frequency,monthly:links,hourly:appearanceHours,display});
  const mark=mount.querySelector('[data-energy-path-chart-value]'),geometry=markGeometry();document.documentElement.dataset.theme=theme;document.documentElement.style.setProperty('--graph-label-font-size',font+'px');mount.style.width=width+'px';await tick();
  const panel=mount.querySelector('[data-energy-path-component-chart]'),plot=panel.querySelector('.energy-path-chart-plot'),family=getComputedStyle(document.body).fontFamily,caption=panel.querySelector('header strong'),control=panel.querySelector('select'),legend=panel.querySelector('.energy-path-chart-legend');
  check(plot&&getComputedStyle(plot).overflowX==='auto'&&panel.getBoundingClientRect().width<=width+.1&&mount.scrollWidth<=width+1,'component plot lost loaded CSS or scrolls the narrow page instead of its own chart '+JSON.stringify({frequency,width}));
  check(mount.querySelector('[data-energy-path-chart-value]')===mark&&markGeometry()===geometry,'component chart font/theme/width changed source observations or numeric mark geometry');
  for(const element of panel.querySelectorAll('.energy-path-chart-tick,.energy-path-chart-axis-label'))check(Math.abs(physicalFont(element)-(Math.max(12,font)+(element.classList.contains('energy-path-chart-axis-label')?1:0)))<.06,'component chart physical font ignores Settings '+JSON.stringify({frequency,language,font,width,text:element.textContent,size:physicalFont(element)}));
  for(const element of[caption,control,legend,panel.querySelector('.energy-path-chart-tick'),panel.querySelector('.energy-path-chart-axis-label')])check(getComputedStyle(element).fontFamily===family,'component chart font family differs from app family');
  check(parseFloat(getComputedStyle(caption).fontSize)>parseFloat(getComputedStyle(control).fontSize)&&Number(getComputedStyle(caption).fontWeight)>Number(getComputedStyle(control).fontWeight)&&parseFloat(getComputedStyle(legend).fontSize)===Math.max(12,font),'component title/control/legend typography hierarchy is inconsistent');
  boundsCheck(JSON.stringify({frequency,language,theme,font,width}));
  check(frequency!=='monthly'||panel.querySelector('[data-energy-path-chart-panel="quantities"]').querySelectorAll('[data-energy-path-chart-tick="x"]').length===12,'monthly appearance correction discarded original month labels');
  const quantityPanel=panel.querySelector('[data-energy-path-chart-panel="quantities"]'),ratioPanel=panel.querySelector('[data-energy-path-chart-panel="ratio"]');
  const twoColumns=width>=2400||font<=12&&width>=1423;
  check(ratioPanel&&(twoColumns?Math.abs(quantityPanel.getBoundingClientRect().top-ratioPanel.getBoundingClientRect().top)<1:ratioPanel.getBoundingClientRect().top>=quantityPanel.getBoundingClientRect().bottom),'relation plots do not wrap at a readable width for the configured font '+JSON.stringify({frequency,width,font}));
  appearance.push({frequency,language,theme,font,width});
 }
 const largeMonthly=links.map(trace=>({...trace,points:trace.points.map(point=>({...point,value:point.value===null?null:point.value*1000000}))}));
 mount.innerHTML=render({item:conversion,kind:'link',monthly:largeMonthly,display});mount.style.width='350px';await tick();boundsCheck('large reported monthly totals');check([...mount.querySelectorAll('[data-energy-path-chart-value]')].map(mark=>mark.dataset.energyPathChartValue).join(',')==='40000000,10000000','large monthly axis padding changed actual reported values');
 window.energyPathComponentReview=(frequency,theme,language,font,width=740)=>{
  setLanguage(language);document.documentElement.dataset.theme=theme;document.documentElement.style.setProperty('--graph-label-font-size',font+'px');mount.style.width=width+'px';
  const reviewLabels={...ratioOptions,from:language==='ko'?'냉방부하':'Cooling load',to:language==='ko'?'냉방기기 에너지':'Cooling equipment energy'};
  const previewGraphs=[42,38,35,28,24,30,47,55,43,31,37,46].map((value,index)=>{const ratio=frequency==='outlier'?measuredRatios[index]:3+index%5*.4,toValue=value/ratio;return {...pairGraph,links:[{...pair,fromValue:value,toValue,ratio}]};});
  const previewMonths=monthly(pair,'link',previewGraphs,reviewLabels),labels=Array.from({length:24},(_value,index)=>'01-01 '+String(index+1).padStart(2,'0')+':00');
  const fromValue=previewGraphs.reduce((sum,graph)=>sum+graph.links[0].fromValue,0),toValue=previewGraphs.reduce((sum,graph)=>sum+graph.links[0].toValue,0),previewItem={...pair,fromValue,toValue,ratio:fromValue/toValue};
  const previewSources=[gain,loss].map((source,trace)=>({...source,hourlyEnergy:{...source.hourlyEnergy,values:labels.map((_label,index)=>Math.round((3+trace+2*Math.sin(index/4+trace))*100)/100)}}));
  mount.innerHTML=render({item:previewItem,label:reviewLabels.from+' → '+reviewLabels.to,kind:'link',frequency:frequency==='outlier'?'monthly':frequency,monthly:previewMonths,hourly:hourly({sourceIds:['gain','loss']},previewSources,{},labels),display,endpointLabels:reviewLabels,conversionRatio:validateRatio(previewItem,pairGraph.nodes,pairGraph.links)});
 };
 check(JSON.stringify({item,graphs})===original&&JSON.stringify(sources)===sourceJSON,'chart changed original observations or accounting data');
 document.body.dataset.componentChartStatus='passed';document.getElementById('result').textContent=JSON.stringify({status:'passed',appearance});
}catch(error){document.body.dataset.componentChartStatus='failed';document.getElementById('result').textContent=error.stack;}
</script></body></html>`
