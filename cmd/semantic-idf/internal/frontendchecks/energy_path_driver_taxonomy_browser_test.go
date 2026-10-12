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

func TestEnergyPathAllServiceDriverProjectionBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser Energy Path driver projection harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/energy-path-driver-taxonomy", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(writer, energyPathDriverTaxonomyHarnessHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--no-sandbox",
		"--no-first-run",
		"--no-default-browser-check",
		"--virtual-time-budget=10000",
		"--user-data-dir="+t.TempDir(),
		"--dump-dom",
		server.URL+"/energy-path-driver-taxonomy",
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("Energy Path driver projection browser harness timed out:\n%s", output)
	}
	if err != nil {
		t.Fatalf("Energy Path driver projection browser harness failed: %v\n%s", err, output)
	}
	if document := string(output); !strings.Contains(document, `data-energy-path-driver-taxonomy-status="passed"`) {
		t.Fatalf("Energy Path driver projection contract failed in browser:\n%s", document)
	}

}

const energyPathDriverTaxonomyHarnessHTML = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Energy Path driver taxonomy harness</title></head>
<body data-energy-path-driver-taxonomy-status="pending"><pre id="result">pending</pre>
<script type="module">
const assert = (condition, message) => { if (!condition) throw new Error(message); };
try {
  const module = await import("/src/js/views/energy-path-view.js");
  const categories = [
    ["surface.exterior_walls", "Wall heat exchange"],
    ["surface.roofs", "Roof heat exchange"],
    ["surface.ground_floors", "Ground / floor heat exchange"],
    ["surface.windows_doors", "Window / door heat exchange"],
    ["surface.interzone", "Interzone surfaces"],
    ["air.infiltration", "Infiltration"],
    ["air.mechanical_ventilation", "Mechanical ventilation"],
    ["air.interzone", "Interzone air"],
    ["internal.people", "People"],
    ["internal.lighting", "Lighting"],
    ["internal.equipment", "Equipment"],
    ["balance.storage_other", "Other / storage"],
  ];
  const nodes = [
    { id: "load.cooling.building", level: "load", label: "Cooling load", value: 100, serviceKind: "cooling" },
    { id: "load.heating.building", level: "load", label: "Heating load", value: 100, serviceKind: "heating" },
    { id: "driver.zero.cooling.building", level: "driver", label: "Zero driver trap", value: 0, driverCategory: "internal.people", serviceKind: "cooling" },
  ];
  const links = [];
  for (const service of ["cooling", "heating"]) {
    categories.forEach(([category, label], index) => {
      const value = category === "internal.equipment" ? 1 : index + 2;
      const id = "driver." + category + "." + service + ".building";
      nodes.push({
        id, level: "driver", kind: "driver." + category, driverCategory: category,
        label, value, rawValue: value, allocatedValue: value, serviceKind: service,
        sourceIds: [category + "." + service], relatedEntityIds: ["entity." + category + "." + service],
      });
      links.push({
        id: "link." + category + "." + service, fromId: id, toId: "load." + service + ".building",
        relation: "driver_to_load", fromValue: value, toValue: value, serviceKind: service,
        sourceIds: [category + "." + service],
      });
    });
  }
  const explanation = {
    schema: module.ENERGY_PATH_SCHEMA_V2,
    scope: { kind: "building", aggregationBasis: "model_total" },
    nodes, links, periods: [],
  };
  const allState = {
    simulationEnergyScopeKind: "building", simulationEnergyPeriod: "annual",
    simulationEnergyService: "all", simulationEnergySelection: "",
  };
  const graph = module.energyPathGraphForState(explanation, allState);
  const drivers = graph.nodes.filter((node) => node.level === "driver" && Number(node.value));
  assert(drivers.length === categories.length - 2, "Internal sources must share one Internal gains driver while all other categories remain: " + drivers.length);
  assert(drivers.every((node) => node.id.includes(".all.building")), "All-services drivers were not projected to virtual all-service IDs");
  const wall = drivers.find((node) => node.driverCategory === "surface.exterior_walls");
  assert(wall && wall.value === 4, "Cooling/heating wall values were not merged");
  assert(wall.sourceIds.includes("surface.exterior_walls.cooling") && wall.sourceIds.includes("surface.exterior_walls.heating"), "Merged wall source provenance was lost");
  assert(wall.relatedEntityIds.length === 2, "Merged wall related entities were lost");
  const internal = drivers.find((node) => node.driverCategory === "internal.gains");
  assert(internal && internal.value === 44 && internal.groupedMembers.length === 6, "Internal gains lost its People/Lighting/Equipment members or quantities");
  assert(drivers.some((node) => node.driverCategory === "balance.storage_other"), "Original Other / storage node is missing");
  const nodeIDs = new Set(graph.nodes.map((node) => node.id));
  assert(graph.links.every((link) => nodeIDs.has(link.fromId) && nodeIDs.has(link.toId)), "Projected graph has a dangling link endpoint");
  assert(graph.links.some((link) => link.toId === "load.cooling.building") && graph.links.some((link) => link.toId === "load.heating.building"), "Projected drivers did not retain both service branches");

  const html = module.renderEnergyPathView(explanation, allState);
  assert(!html.includes("Zero driver trap"), "Zero-value driver was rendered");
  assert(html.indexOf("Wall heat exchange") < html.indexOf("Roof heat exchange"), "Driver display order is not canonical");
  assert(html.indexOf("Roof heat exchange") < html.indexOf("Ground / floor heat exchange"), "Driver display order is not canonical");

  const cooling = module.energyPathGraphForState(explanation, { ...allState, simulationEnergyService: "cooling" });
  assert(cooling.nodes.some((node) => node.id === "driver.surface.exterior_walls.cooling.building"), "Single-service view lost the canonical service node");

  const { energyPathProjectDriverBreakdown } = await import("/src/js/energy-path-driver-presentation.js");
  const parent = {id:"driver.balance.cooling.building", level:"driver", driverCategory:"balance.storage_other", value:100,
    serviceKind:"cooling", unit:"kWh", sourceIds:["air","surface","unresolved"],
    driverBreakdown:[{component:"air_storage",value:30,rawValue:3,effectiveValue:6,sourceIds:["air"]},
      {component:"surface_balance",value:20,rawValue:2,effectiveValue:4,sourceIds:["surface"]},
      {component:"unresolved_balance",value:50,rawValue:5,effectiveValue:10,sourceIds:["unresolved"]}]};
  const load = {id:"load.cooling.building",level:"load",serviceKind:"cooling",value:100,unit:"kWh"};
  const parentLink = {id:"parent-link",fromId:parent.id,toId:load.id,relation:"driver_to_load",fromValue:100,toValue:100,fromUnit:"kWh",toUnit:"kWh"};
  const inputs = ["people","lighting","equipment.electric","equipment.gas","surface_storage","solar_absorbed","internal_gains"].map((kind,index)=>
    ({id:"input."+kind,level:"input",scaleDomain:"boundary",kind:"input."+kind,value:index+1,signedValue:index+1,unit:"kWh"}));
  const original = JSON.stringify({nodes:[parent,load,...inputs],links:[parentLink]});
  const explained = {...explanation,nodes:[parent,load,...inputs],links:[parentLink]};
  const detailed = module.energyPathGraphForState(explained,allState);
  const components = detailed.nodes.filter(node=>node.level==="driver");
  assert(components.length===3 && components.reduce((sum,node)=>sum+node.value,0)===100,"Evidence-backed residual children do not conserve parent allocation");
  assert(components.find(node=>node.driverCategory==="balance.air_storage")?.value===30,"Air storage was mixed with unresolved balance");
  assert(detailed.links.reduce((sum,link)=>sum+link.fromValue,0)===100 && detailed.links.reduce((sum,link)=>sum+link.toValue,0)===100,"Breakdown changed service budget");
  const boundary = detailed.nodes.filter(node=>node.level==="input");
  assert(boundary.length===3 && boundary.find(node=>node.kind==="input.equipment")?.value===7,"People/Lighting/Equipment source hierarchy is incorrect");
  assert(!boundary.some(node=>["input.surface_storage","input.solar_absorbed","input.internal_gains"].includes(node.kind)),"Building response leaked into Boundary Inputs");
  assert(JSON.stringify({nodes:[parent,load,...inputs],links:[parentLink]})===original,"Presentation mutated saved source evidence");
  const invalid = energyPathProjectDriverBreakdown([{...parent,driverBreakdown:[{component:"air_storage",value:99}]},load],[parentLink]);
  assert(invalid.nodes[0].id===parent.id && invalid.links[0].fromValue===100,"Incomplete breakdown fabricated a split");
  const zeroParent={...parent,value:0,driverBreakdown:parent.driverBreakdown.map(part=>({...part,value:0}))};
  const zero=energyPathProjectDriverBreakdown([zeroParent,load],[{...parentLink,fromValue:0,toValue:0}]);
  assert(zero.nodes.filter(node=>node.level==="driver").length===3 && zero.links.every(link=>link.fromValue===0 && link.toValue===0),"Known-zero components disappeared or acquired invalid link values");
  const {energyPathMonthlyChartSeries}=await import("/src/js/energy-path-chart.js");
  const selected=components.find(node=>node.driverCategory==="balance.air_storage");
  const monthZero=module.energyPathGraphForState({...explained,nodes:[zeroParent,load],links:[{...parentLink,fromValue:0,toValue:0}]},allState);
  const zeroSeries=energyPathMonthlyChartSeries(selected,"node",[monthZero]);
  assert(zeroSeries[0].points[0].value===0 && zeroSeries[0].points[1].value===null,"Known-zero monthly component became missing or filled an absent month");
  const directionalExplanation = (gain, loss) => {
    const directionalNodes = [
      ...["gain", "loss"].map((direction, index) => ({id:"driver.surface.exterior_walls." + (index ? "heating" : "cooling") + ".building." + direction,
        level:"driver",kind:"driver.surface.exterior_walls",driverCategory:"surface.exterior_walls",label:"Wall heat exchange",
        serviceKind:index ? "heating" : "cooling",heatDirection:direction,heatDirectionBasis:"native_interval",unit:"kWh",scaleDomain:"thermal",
        value:index ? 2 : 4,allocatedValue:index ? 2 : 4,rawValue:index ? loss : gain,effectiveValue:index ? loss : gain,
        signedValue:index ? -loss : gain,sourceIds:["wall." + direction]})),
      {id:"load.cooling.building",level:"load",serviceKind:"cooling",value:4,unit:"kWh",scaleDomain:"thermal"},
      {id:"load.heating.building",level:"load",serviceKind:"heating",value:2,unit:"kWh",scaleDomain:"thermal"},
      ...["gain", "loss"].map((direction, index) => ({id:"input.convection." + direction,level:"input",kind:"input.exterior_convection",
        label:"Exterior convection",heatDirection:direction,heatDirectionBasis:"native_interval",value:index ? loss : gain,
        signedValue:index ? -loss : gain,rawValue:index ? -loss : gain,effectiveValue:index ? -loss : gain,unit:"kWh",scaleDomain:"boundary",sourceIds:["outside." + direction]})),
      {id:"driver.balance.storage_other.heating.building.loss",level:"driver",kind:"driver.balance.storage_other",driverCategory:"balance.storage_other",
        label:"Storage",heatDirection:"loss",heatDirectionBasis:"native_interval",serviceKind:"heating",value:0,allocatedValue:0,rawValue:5,effectiveValue:5,
        signedValue:-5,unit:"kWh",scaleDomain:"thermal",sourceIds:["air.loss"],driverBreakdown:[{component:"air_storage",value:0,rawValue:5,effectiveValue:5,sourceIds:["air.loss"]}]},
    ];
    const directionalLinks = directionalNodes.filter(node => node.level === "driver" && node.value > 0).map(node => ({
      id:"link." + node.id,fromId:node.id,toId:"load." + node.serviceKind + ".building",relation:"driver_to_load",serviceKind:node.serviceKind,
      fromValue:node.value,toValue:node.value,fromUnit:"kWh",toUnit:"kWh",sourceIds:node.sourceIds}));
    return {...explanation,nodes:directionalNodes,links:directionalLinks,periods:[{id:"M1",nodes:directionalNodes.map(node => ({...node,period:"M1"})),links:directionalLinks.map(link => ({...link,period:"M1"}))}]};
  };
  const firstDirectional = directionalExplanation(10, 5), secondDirectional = directionalExplanation(110, 105);
  const firstGraph = module.energyPathGraphForState(firstDirectional, allState), secondGraph = module.energyPathGraphForState(secondDirectional, allState);
  const wallDirections = firstGraph.nodes.filter(node => node.driverCategory === "surface.exterior_walls");
  assert(wallDirections.length === 2 && wallDirections.every(node => node.id.endsWith(".all.building")), "Gain/loss cards merged or directional ID suffix corrupted scope");
  const gainNamedScope = module.energyPathMergeAllServiceDrivers([{...firstDirectional.nodes[0],id:"driver.surface.exterior_walls.cooling.gain",zoneName:"gain",heatDirectionBasis:"native_direction"}],[]);
  assert(gainNamedScope.nodes[0].id.endsWith(".all.gain"), "A legacy native-direction Zone named gain was mistaken for an appended direction suffix");
  const nativeDirectionScope = module.energyPathMergeAllServiceDrivers([{...firstDirectional.nodes[0],heatDirectionBasis:"native_direction"}],[],{kind:"building"});
  assert(nativeDirectionScope.nodes[0].id.endsWith(".all.building"), "New native-direction suffix replaced the explicit Building scope");
  const lossNamedScope = module.energyPathMergeAllServiceDrivers([{...firstDirectional.nodes[1],id:"driver.surface.exterior_walls.heating.loss.loss",zoneName:"loss",heatDirectionBasis:"native_direction"}],[],{kind:"zone",zoneName:"loss"});
  assert(lossNamedScope.nodes[0].id.endsWith(".all.loss"), "New directional suffix corrupted a Zone literally named loss");
  assert(wallDirections.find(node => node.heatDirection === "gain").effectiveValue === 10 && wallDirections.find(node => node.heatDirection === "loss").effectiveValue === 5, "Directional gross wall quantities were netted");
  assert(secondGraph.nodes.find(node => node.driverCategory === "surface.exterior_walls" && node.heatDirection === "gain").effectiveValue === 110, "Equal net heat erased distinct gross exchange");
  const inputDirections = firstGraph.nodes.filter(node => node.level === "input");
  assert(inputDirections.length === 2 && inputDirections.some(node => node.signedValue === 10) && inputDirections.some(node => node.signedValue === -5), "Boundary input grouping cancelled opposite directions");
  const storedAir = firstGraph.nodes.find(node => node.driverCategory === "balance.air_storage");
  assert(storedAir?.label === "Zone-air heat storage" && storedAir.heatDirection === "loss" && storedAir.value === 0 && storedAir.effectiveValue === 5, "Storage physical direction or zero-allocation observation was lost");
  assert(firstGraph.links.reduce((sum, link) => sum + link.fromValue, 0) === 6, "Gross directional observations changed the allocated load budget");
  const directionalScene = module.prepareEnergyPathScene(firstDirectional, {...allState});
  assert(directionalScene.visibleNodes.some(node => node.id === storedAir.id) && !directionalScene.drawing.ribbons.some(link => link.fromId === storedAir.id), "Observed storage without a load allocation disappeared or acquired a fabricated ribbon");
  assert(directionalScene.layout.nodes.filter(node => node.level === "driver").every(node => node.height >= 64), "Directional quantities were compressed into unreadable card rows");
  const directionalHTML = module.renderEnergyPathView(firstDirectional, {...allState});
  const directionalHost = document.createElement("div");directionalHost.innerHTML = directionalHTML;
  assert(directionalHost.querySelector('[data-energy-path-gross-heat="10"]') && directionalHost.querySelector('[data-energy-path-gross-heat="-5"]') && directionalHost.querySelector('[data-energy-path-load-contribution="4"]'), "Cards do not distinguish physical heat exchange from load contribution");
  assert(!directionalHost.querySelector("[data-energy-path-net-only]"), "Proven directional observations were labelled net-only");
  const monthlyDirectional = module.energyPathGraphForState(firstDirectional, {...allState,simulationEnergyPeriod:"M1"});
  const lossDriver = wallDirections.find(node => node.heatDirection === "loss");
  const lossSeries = energyPathMonthlyChartSeries(lossDriver,"node",[monthlyDirectional]);
  assert(lossSeries.length === 2 && lossSeries[0].points[0].value === -5 && lossSeries[1].points[0].value === 2 && lossSeries[0].points[1].value === null, "Monthly chart mixed directions, allocation, or absent observations");
  const observedZeroMonth = {...firstDirectional,periods:[{id:"M2",nodes:firstDirectional.nodes.map(node=>({...node,period:"M2",value:0,allocatedValue:0,rawValue:0,effectiveValue:0,signedValue:0,...(node.driverBreakdown ? {driverBreakdown:node.driverBreakdown.map(part=>({...part,value:0,rawValue:0,effectiveValue:0}))} : {})})),links:firstDirectional.links.map(link=>({...link,period:"M2",fromValue:0,toValue:0}))}]};
  const zeroDirectionalGraph = module.energyPathGraphForState(observedZeroMonth,{...allState,simulationEnergyPeriod:"M2"});
  const zeroDirectionalSeries = energyPathMonthlyChartSeries(lossDriver,"node",[monthlyDirectional,zeroDirectionalGraph]);
  assert(zeroDirectionalSeries[0].points[1].value === 0 && zeroDirectionalSeries[1].points[1].value === 0 && zeroDirectionalSeries[0].points[2].value === null && zeroDirectionalSeries[1].points[2].value === null, "A fully observed zero directional month became a gap, or an absent month became zero");
  const {energyPathHourlyChartSeries} = await import("/src/js/energy-path-chart.js");
  assert(energyPathHourlyChartSeries(lossDriver,[{id:"wall.loss",name:"Native net",hourlyEnergy:{basis:"reported_source",unit:"kWh",values:[7]}}],allState,["01-01 01:00"]).length === 0, "Native hourly net masqueraded as directional gross heat");
  const nativeLoss = {...lossDriver,heatDirectionBasis:"native_direction",sourceIds:["native-loss"]};
  const nativeLossSource = {id:"native-loss",name:"Zone Mixing Sensible Heat Loss Energy",keyValue:"Office",multiplierApplication:"already_model_total",hourlyEnergy:{basis:"reported_source",unit:"kWh",values:[7]}};
  assert(energyPathHourlyChartSeries(nativeLoss,[nativeLossSource],allState,["01-01 01:00"])[0]?.points[0].value === -7, "Already separate native Hourly heat loss was removed or its direction reversed");
  assert(energyPathHourlyChartSeries(nativeLoss,[{...nativeLossSource,hourlyEnergy:{...nativeLossSource.hourlyEnergy,values:[-7]}}],allState,["01-01 01:00"]).length === 0, "Conflicting native directional Hourly sign was silently clipped or inverted");
  const host=document.createElement("div");host.innerHTML=module.renderEnergyPathView(explained,allState);
  assert(host.querySelector("[data-energy-path-net-only]"), "Legacy net-only results lack a disclosure");
  assert(host.querySelectorAll("[data-energy-path-unit]").length===1 && !host.querySelector("[data-energy-path-stage] header span"),"Units must appear once, without per-stage domain subtitles");
  assert(!host.querySelector("[data-energy-path-divider], [data-energy-path-envelope-context], [data-energy-path-context-metrics]"),"Obsolete dividers or utility context remain");
  const zoned={...explained,availableZones:["Office","building"],zoneResults:["Office","building"].map(zoneName=>({...explained,scope:{kind:"zone",zoneName},periods:[]}))};
  host.innerHTML=module.renderEnergyPathControls(zoned,allState);
  const picker=host.querySelector("[data-simulation-energy-scope]");
  assert(picker.options[0].value==="building" && picker.options[0].textContent==="All" && picker.options.length===3,"One-step scope options do not expose All and every Zone");
  assert(!host.querySelector("[data-simulation-energy-zone-name]"),"Two-step Zone control remains");
  picker.value="zone:building";const switched={...allState};
  assert(module.updateEnergyPathControlState({target:picker,type:"change"},switched,zoned).render && switched.simulationEnergyScopeKind==="zone" && switched.simulationEnergyZoneName==="building","Zone named building collided with All scope");
  document.body.dataset.energyPathDriverTaxonomyStatus = "passed";
  document.getElementById("result").textContent = "passed";
} catch (error) {
  document.body.dataset.energyPathDriverTaxonomyStatus = "failed";
  document.getElementById("result").textContent = String(error?.stack || error);
}
</script></body></html>`
