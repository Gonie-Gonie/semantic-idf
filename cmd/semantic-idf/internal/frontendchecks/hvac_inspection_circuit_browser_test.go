package frontendchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

// Exercise real Go -> JSON -> browser topology; a hand-written zoneName fixture
// would conceal the missing backend field that originally broke zone anchors.
func TestHVACInspectionExecutedCircuitBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("HVAC circuit and zone-anchor browser acceptance")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	input := repoPath("frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf")
	data, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := idf.Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	report := idf.AnalyzeHVAC(doc)
	series := []simulation.SimulationSeries{}
	seen := map[string]bool{}
	// Only these fixture nodes are temperature control points. All other nodes
	// still have requested setpoint outputs, containing EnergyPlus's unset flag.
	setpoints := map[string]float64{
		"HEATSYS1 SUPPLY EQUIPMENT OUTLET NODE": 82.2, "HEATSYS1 SUPPLY OUTLET NODE": 82.2,
		"COOLSYS1 SUPPLY EQUIPMENT OUTLET NODE 1": 6.7,
		"VAV_5_OA-VAV_5_COOLCNODE":                12.8, "VAV_5_COOLC-VAV_5_HEATCNODE": 12.8,
	}
	add := func(key string, temperature, flow float64) {
		if key == "" || seen[strings.ToUpper(key)] {
			return
		}
		seen[strings.ToUpper(key)] = true
		setpoint, controlled := setpoints[strings.ToUpper(key)]
		if !controlled {
			setpoint = -999
		}
		for _, property := range []struct {
			name, unit string
			value      float64
		}{
			{"System Node Temperature", "C", temperature}, {"System Node Mass Flow Rate", "kg/s", flow},
			{"System Node Relative Humidity", "%", 98.65}, {"System Node Setpoint Temperature", "C", setpoint},
		} {
			next := property.value + 1
			if property.name == "System Node Setpoint Temperature" {
				next = setpoint
				if !controlled {
					next = -998.9999999999999
				}
			}
			series = append(series, simulation.SimulationSeries{KeyValue: strings.ToUpper(key), Name: property.name,
				File: "eplusout.sql", Column: strings.ToUpper(key) + ":" + property.name + " [" + property.unit + "](Hourly)", ReportingFrequency: "Hourly",
				Points: []simulation.SimulationPoint{{X: 1, Label: "01-01 01:00", Value: property.value}, {X: 2, Label: "01-01 02:00", Value: next}},
			})
		}
	}
	add("VAV_5_CoolCDemand Inlet Node", 6.7, .65)
	add("VAV_5_CoolCDemand Outlet Node", 11.3, .65)
	add("VAV_5_OA-VAV_5_CoolCNode", 27.14, 3.14)
	add("VAV_5_CoolC-VAV_5_HeatCNode", 12.34, 3.14)
	add("CoolSys1 Pump-CoolSys1 ChillerNode 1", 12, 1)
	add("CoolSys1 Supply Equipment Outlet Node 1", 7, 1)
	add("CoolSys1 Chiller Water Inlet Node 1", 30, 2)
	add("CoolSys1 Chiller Water Outlet Node 1", 35, 2)
	for _, loop := range report.Loops {
		add(loop.SupplySide.InletNode, 22, 1)
		if loop.Name == "HeatSys1" {
			for _, side := range []idf.HVACLoopSide{loop.SupplySide, loop.DemandSide} {
				add(side.InletNode, 75.1, 2.62)
				add(side.OutletNode, 49.25, 2.62)
				for _, branch := range side.Branches {
					for _, component := range branch.Components {
						add(component.InletNode, 82.2, .13)
						add(component.OutletNode, 49.98, .13)
						name := ""
						switch {
						case strings.HasPrefix(component.ObjectType, "Coil:Heating"):
							name = "Heating Coil Heating Rate"
						case strings.HasPrefix(component.ObjectType, "Boiler:"):
							name = "Boiler Heating Rate"
						case strings.HasPrefix(component.ObjectType, "Pump:"):
							name = "Pump Electricity Rate"
						}
						if name != "" {
							series = append(series, simulation.SimulationSeries{KeyValue: strings.ToUpper(component.ObjectName), Name: name, File: "eplusout.sql", Column: component.ObjectName + ":" + name + " [W](Hourly)", ReportingFrequency: "Hourly", Points: []simulation.SimulationPoint{{X: 1, Label: "01-01 01:00", Value: 20800}, {X: 2, Label: "01-01 02:00", Value: 0}}})
						}
					}
				}
			}
		}
	}
	for _, zone := range report.ZoneRelations {
		if zone.ZoneName != "Core_top" {
			continue
		}
		for _, name := range append(append([]string(nil), zone.Nodes.InletNodes...), zone.Nodes.ReturnNodes...) {
			add(name, 21, .5)
		}
	}
	bundle := simulation.BuildPurposeResultBundle(&simulation.SimulationRunResult{InputPath: input, Series: series}, simulation.SimulationPurposeRequest{Purposes: []simulation.SimulationPurposeID{simulation.SimulationPurposeHVACLoopCheck}})
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/fixture", func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(bundle.HVACLoops) })
	mux.HandleFunc("/circuit", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, hvacInspectionCircuitHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/circuit").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-hvac-circuit-status="passed"`) {
		if _, detail, ok := strings.Cut(string(output), `<pre id="result">`); ok {
			message, _, _ := strings.Cut(detail, "</pre>")
			t.Fatalf("HVAC circuit/zone contract failed: %v\n%s", err, message)
		}
		t.Fatalf("HVAC circuit/zone contract failed: %v\n%s", err, output)
	}
	if screenshot := os.Getenv("HVAC_INSPECTION_SCREENSHOT"); screenshot != "" {
		output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--window-size=1460,1800", "--force-device-scale-factor=1", "--user-data-dir="+t.TempDir(), "--screenshot="+screenshot, server.URL+"/circuit?review=HeatSys1").CombinedOutput()
		if err != nil {
			t.Fatalf("HVAC screenshot: %v\n%s", err, output)
		}
	}
}

const hvacInspectionCircuitHTML = `<!doctype html><html data-theme="dark"><head><meta charset="utf-8"><link rel="stylesheet" href="/src/styles.css"><style>body{display:block;overflow:auto;height:auto}#mount{width:1400px}</style></head><body><div id="mount"></div><pre id="result"></pre><script type="module">
const check=(value,message)=>{if(!value)throw Error(message)};
try{
 const {renderHVACInspection:render}=await import('/src/js/views/hvac-inspection-view.js');
 const {prepareHVACInspection:prepare,hvacInspectionBasicProperties:basic,hvacInspectionTrace:trace}=await import('/src/js/hvac-inspection-data.js');
 const {buildHVACLoopDiagramLayout:layout}=await import('/src/js/views/hvac-views.js');
 const loops=await (await fetch('/fixture')).json(),original=JSON.stringify(loops),mount=document.getElementById('mount');
 const find=name=>loops.find(loop=>loop.name===name);
 const point=name=>[...mount.querySelectorAll('.node[data-hvac-inspect-point-name]')].find(item=>item.dataset.hvacInspectPointName.toLowerCase()===name.toLowerCase());
 for(const name of ['CoolSys1','HeatSys1','TowerWaterSys','SWHSys1']){
  const loop=find(name);mount.innerHTML=render(loop,{});
  check(mount.querySelectorAll('[data-hvac-inspect-basic-chart]').length===2&&!mount.querySelector('[data-hvac-inspect-basic-chart="humidity"]'),name+' retained humidity chart');
  check(!mount.querySelector('[data-hvac-inspect-metric="relativeHumidity"],[data-hvac-inspect-metric="humidity"]'),name+' shows humidity on water');
  check(mount.querySelector('[data-hvac-inspect-basic-plot="temperature"] svg')&&mount.querySelector('[data-hvac-inspect-basic-plot="flow"] svg'),name+' lost water graphs');
 }
 const cool=find('CoolSys1'),coolModel=prepare(cool);mount.innerHTML=render(cool,{});
 for(const [name,value] of [['VAV_5_CoolCDemand Inlet Node',6.7],['VAV_5_CoolCDemand Outlet Node',11.3]]){
  check(point(name)?.classList.contains('anchored')&&point(name).querySelector('.hvac-inspect-node-ring'),name+' is not on the water coil port');
  check(point(name).querySelector('[data-hvac-inspect-metric="temperature"] .hvac-inspect-metric-value').textContent===value.toFixed(2)+' °C',name+' shows air temperature or inherited a setpoint');
  check(point(name).querySelector('[data-hvac-inspect-metric="flow"] .hvac-inspect-metric-value').textContent==='0.65 kg/s',name+' shows air mass flow');
  const property=basic(coolModel,'temperature').find(p=>p.entity.name.toLowerCase()===name.toLowerCase());
  check(trace(coolModel,property).points.map(p=>p.value).join(',')===[value,value+1].join(','),'graph differs from water snapshot');
 }
 check(!point('VAV_5_OA-VAV_5_CoolCNode')&&!point('VAV_5_CoolC-VAV_5_HeatCNode')&&!point('CoolSys1 Chiller Water Inlet Node 1'),'water diagram includes another circuit');
 const tower=find('TowerWaterSys');mount.innerHTML=render(tower,{});
 check(point('CoolSys1 Chiller Water Inlet Node 1')?.classList.contains('anchored')&&!point('CoolSys1 Pump-CoolSys1 ChillerNode 1'),'condenser circuit replaced by chilled water');
 const air=find('VAV_5');mount.innerHTML=render(air,{});
 check(mount.querySelector('[data-hvac-inspect-basic-chart="humidity"] svg')&&point('VAV_5_OA-VAV_5_CoolCNode')?.textContent.includes('27.14 >set 12.80 °C'),'air loop lost its temperature or RH');
 check(!point('VAV_5_CoolCDemand Inlet Node'),'air loop includes coil water points');
 const zoneLoop=loops.find(loop=>loop.loopType==='AirLoopHVAC'&&loop.topology.relatedZones.includes('Core_top'));mount.innerHTML=render(zoneLoop,{});
 const zoneNodes=zoneLoop.topology.demandGraph.nodes.filter(node=>node.zoneName==='Core_top'&&['zone_inlet','zone_return'].includes(node.role));
 check(zoneNodes.length===2,'Go result omitted Core_top inlet/return zone ownership');
 const zone=layout(zoneLoop.topology).anchors.find(anchor=>anchor.kind==='zone'&&anchor.name==='Core_top');
 for(const node of zoneNodes){
  const item=point(node.nodeName),ring=item?.querySelector('.hvac-inspect-node-ring');
  check(item?.classList.contains('anchored')&&ring,'actual Go zone port is detached: '+node.nodeName);
  check(Number(ring.getAttribute('cx'))===zone.x+(node.role==='zone_inlet'?42:-42),'zone port attached to the wrong icon or side');
 }
 check(JSON.stringify(loops)===original,'render mutated executed results');
 const heat=find('HeatSys1');mount.innerHTML=render(heat,{});
 const annotations=[...mount.querySelectorAll('.hvac-inspect-annotation')].map(item=>({name:item.parentElement.dataset.hvacInspectPointName,rect:item.getBoundingClientRect()}));
 check(annotations.length>45,'dense heating-loop fixture lacks observed branch ports/equipment');
 for(let i=0;i<annotations.length;i++)for(let j=i+1;j<annotations.length;j++){
  const a=annotations[i],b=annotations[j];check(Math.min(a.rect.right,b.rect.right)-Math.max(a.rect.left,b.rect.left)<=1||Math.min(a.rect.bottom,b.rect.bottom)-Math.max(a.rect.top,b.rect.top)<=1,'dense loop text overlaps: '+a.name+' / '+b.name);
 }
 const demandRows=[...mount.querySelectorAll('.node.anchored.demand .hvac-inspect-node-ring')].map(item=>Number(item.getAttribute('cy'))),rows=[...new Set(demandRows)].sort((a,b)=>a-b);
 check(rows.length>10&&rows.slice(1).every((y,index)=>y-rows[index]<=260),'parallel equipment lines retain excessive vertical spacing: '+JSON.stringify(rows));
 const setpointNodes=['HEATSYS1 SUPPLY EQUIPMENT OUTLET NODE','HEATSYS1 SUPPLY OUTLET NODE'];
 check(heat.nodeSummaries.filter(node=>node.hasSetpoint).length===2,'Go summary marked every requested setpoint output as a control point');
 for(const frameIndex of [0,1]){
  mount.innerHTML=render(heat,{frameIndex});
  const comparisons=[...mount.querySelectorAll('[data-hvac-setpoint-state]')];
  check(comparisons.length===2&&comparisons.every(item=>setpointNodes.includes(item.closest('[data-hvac-inspect-point-name]').dataset.hvacInspectPointName.toUpperCase())),'unset SQL flag or loop setpoint spread to other nodes');
  check(comparisons.every(item=>item.dataset.hvacSetpointState==='unmet'&&getComputedStyle(item).fill==='rgb(255, 123, 114)'),'unmet heating setpoints are not red');
 }
 if(!new URLSearchParams(location.search).has('review'))mount.innerHTML=render(zoneLoop,{});
 document.body.dataset.hvacCircuitStatus='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.hvacCircuitStatus='failed';document.getElementById('result').textContent=error.stack;}
</script>`
