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

func TestEPATH123AutomaticOtherGroupingBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser grouping verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath123", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath123OtherGroupingHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath123").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH123 browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath123-status="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH123 grouping contract failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH123 grouping contract failed:\n%s", output)
	}
}

const epath123OtherGroupingHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH123 Other grouping</title></head>
<body data-epath123-status="pending"><div id="mount"></div><pre id="result">pending</pre>
<script type="module">
const failures = [];
const check = (condition, message) => { if (!condition) failures.push(message); };
const sorted = (items = []) => [...items].sort();
const equalSet = (a,b) => JSON.stringify(sorted(a)) === JSON.stringify(sorted(b));
const close = (a,b) => Math.abs(a-b) < 1e-8;
const endUse = (token,value,extra={}) => ({ id:"end_use."+token+".building", level:"end_use", kind:"end_use."+token, endUse:token, label:token, value, rawValue:value, effectiveValue:value, allocatedValue:value, unit:"kWh", scaleDomain:"site", period:"annual", sourceIds:["source."+token], ...extra });
const carrier = (token,value) => ({ id:"carrier."+token+".building", level:"carrier", kind:"carrier."+token, carrier:token, label:token, value, unit:"kWh", scaleDomain:"site" });
const branch = (node, carrierToken="electricity") => ({ id:"link."+node.id, fromId:node.id, toId:"carrier."+carrierToken+".building", relation:"direct_end_use_to_carrier", basis:"reported_meter", fromValue:node.value, toValue:node.value, fromUnit:"kWh", toUnit:"kWh", sourceIds:[...node.sourceIds] });
const endUses = [endUse("cooling",90),endUse("lighting",.99),endUse("equipment",1),endUse("water_systems",1.01),endUse("refrigeration",.98),endUse("other",6.02)];
const nodes = [...endUses,carrier("electricity",99.02),carrier("natural_gas",.98)];
const links = endUses.map(node => branch(node,node.endUse === "refrigeration" ? "natural_gas" : "electricity"));
const rawSnapshot = JSON.stringify({nodes,links});
const snapshot = (graph) => JSON.stringify({
  nodes: graph.nodes.map(n => ({id:n.id,value:n.value,sourceIds:sorted(n.sourceIds),originalNodeIds:sorted(n.originalNodeIds),memberIds:sorted((n.groupedMembers||[]).map(m=>m.id))})).sort((a,b)=>a.id.localeCompare(b.id)),
  links: graph.links.map(l => ({from:l.fromId,to:l.toId,relation:l.relation,fromValue:l.fromValue,toValue:l.toValue,sourceIds:sorted(l.sourceIds)})).sort((a,b)=>JSON.stringify(a).localeCompare(JSON.stringify(b))),
});
try {
  const view = await import("/src/js/views/energy-path-view.js");
  const grouped = view.energyPathGroupSmallNodes(nodes,links);
  const other = grouped.nodes.find(node => node.level === "end_use" && node.endUse === "other");
  check(other?.automaticOther && close(other.value,7.99), "strict <1% grouping did not combine .99/.98 with existing Other6.02");
  check(!grouped.nodes.some(node => node.id === "end_use.lighting.building" || node.id === "end_use.refrigeration.building"), "eligible tiny nodes escaped automatic grouping");
  check(grouped.nodes.some(node => node.id === "end_use.equipment.building" && node.value === 1), "exact1% was incorrectly grouped");
  check(grouped.nodes.some(node => node.id === "end_use.water_systems.building" && node.value === 1.01), "above1% was incorrectly grouped");
  check(grouped.nodes.some(node => node.id === "end_use.cooling.building"), "protected HVAC end use was grouped");
  const wantedMembers = ["end_use.lighting.building","end_use.refrigeration.building","end_use.other.building"];
  check(equalSet(other?.originalNodeIds,wantedMembers), "Other lost original node IDs");
  check(equalSet((other?.groupedMembers||[]).map(member=>member.id),wantedMembers), "Other lost full inspector member snapshots");
  check(close((other?.groupedMembers||[]).reduce((sum,m)=>sum+m.value,0),other?.value), "Other member values do not close to group value");
  check(equalSet(other?.sourceIds,["source.lighting","source.refrigeration","source.other"]), "Other source union lost or contaminated provenance");
  const otherBranches = grouped.links.filter(link=>link.fromId===other?.id);
  const electric = otherBranches.find(link=>link.toId==="carrier.electricity.building");
  const gas = otherBranches.find(link=>link.toId==="carrier.natural_gas.building");
  check(otherBranches.length===2,"grouped Other retained duplicate links for the same carrier branch");
  check(close(electric?.fromValue,7.01) && close(electric?.toValue,7.01) && equalSet(electric?.sourceIds,["source.lighting","source.other"]), "electric Other split lost exact value or branch-local sources");
  check(close(gas?.fromValue,.98) && close(gas?.toValue,.98) && equalSet(gas?.sourceIds,["source.refrigeration"]), "gas Other split inherited unrelated electricity source");
  check(snapshot(grouped)===snapshot(view.energyPathGroupSmallNodes([...nodes].reverse(),[...links].reverse())), "grouping depends on input order");
  check(JSON.stringify({nodes,links})===rawSnapshot, "grouping mutated original node/link payload");
  const realistic=[endUse("cooling",98,{serviceKind:"cooling"}),endUse("lighting",.4),endUse("equipment",.3),endUse("other",.3),carrier("electricity",99)];
  const realisticResult=view.energyPathGroupSmallNodes(realistic,realistic.filter(n=>n.level==="end_use").map(n=>branch(n)));
  const realisticOther=realisticResult.nodes.find(n=>n.automaticOther&&n.endUse==="other");
  check(realisticOther&&close(realisticOther.value,1)&&equalSet(realisticOther.originalNodeIds,["end_use.lighting.building","end_use.equipment.building","end_use.other.building"]),"direct-use percentage ignored protected HVAC energy with explicit cooling serviceKind instead of using selected site-stage total");
  const realisticBranches=realisticResult.links.filter(link=>link.fromId===realisticOther?.id);
  check(realisticBranches.length===1&&close(realisticBranches[0].fromValue,1)&&close(realisticBranches[0].toValue,1)&&equalSet(realisticBranches[0].sourceIds,["source.lighting","source.equipment","source.other"]),"existing Other plus same-carrier tiny members must produce one coherent split with exact summed value and source union");
  check(realisticResult.nodes.some(n=>n.id==="end_use.cooling.building"&&n.serviceKind==="cooling"),"stage denominator inclusion incorrectly merged the HVAC service bucket");
  const floating=[endUse("cooling",98.70000000000002),endUse("lighting",1),endUse("equipment",.1),endUse("water_systems",.1),endUse("refrigeration",.1)];
  const floatingResult=view.energyPathGroupSmallNodes(floating,[]);
  check(floatingResult.nodes.some(n=>n.id==="end_use.lighting.building"&&n.value===1),"floating-point stage-sum noise grouped an exactly1% category");

  const protectedCategories=["surface.exterior_walls","surface.roofs","surface.ground_floors","surface.windows_doors","air.infiltration","air.mechanical_ventilation"];
  const protectedNodes=protectedCategories.map((category,index)=>({id:"driver."+category+".cooling.building",level:"driver",driverCategory:category,serviceKind:"cooling",value:.1,unit:"kWh",scaleDomain:"thermal",sourceIds:["protected."+index]}));
  protectedNodes.push({id:"driver.internal.people.cooling.building",level:"driver",driverCategory:"internal.people",serviceKind:"cooling",value:99.4,unit:"kWh",scaleDomain:"thermal"});
  protectedNodes.push(...["cooling","heating"].map(service=>({id:"load."+service+".building",level:"load",serviceKind:service,value:.001,unit:"kWh",scaleDomain:"thermal"})));
  protectedNodes.push(endUse("cooling",.001),endUse("heating",.001),endUse("equipment",99.998),carrier("electricity",.001),carrier("natural_gas",99.999));
  const protectedResult=view.energyPathGroupSmallNodes(protectedNodes,[]);
  for(const id of [...protectedCategories.map(c=>"driver."+c+".cooling.building"),"load.cooling.building","load.heating.building","end_use.cooling.building","end_use.heating.building","carrier.electricity.building","carrier.natural_gas.building"]){
    check(protectedResult.nodes.some(node=>node.id===id),"protected taxonomy node was grouped: "+id);
  }
  const correspondenceDriver={id:"driver.internal.lighting.cooling.building",level:"driver",driverCategory:"internal.lighting",value:.99,unit:"kWh",scaleDomain:"thermal"};
  const correspondence={id:"correspondence",fromId:correspondenceDriver.id,toId:"end_use.lighting.building",relation:"source_correspondence",fromValue:.99,toValue:.99,fromUnit:"kWh",toUnit:"kWh",sourceIds:["source.lighting"]};
  const corresponding=view.energyPathGroupSmallNodes([...nodes,correspondenceDriver],[...links,correspondence]);
  check(corresponding.nodes.some(n=>n.id===correspondenceDriver.id)&&corresponding.nodes.some(n=>n.id==="end_use.lighting.building"),"correspondence endpoints lost separate identities");
  check(corresponding.links.some(l=>l.fromId===correspondenceDriver.id&&l.toId==="end_use.lighting.building"&&l.relation==="source_correspondence"),"correspondence relation rewritten through Other");

  const bucketNodes=[];
  for(const [zoneName,period,serviceKind] of [["Alpha","M1","cooling"],["Beta","M1","cooling"],["Alpha","M2","cooling"],["Alpha","M1","heating"]]){
    const suffix=[zoneName,period,serviceKind].join("_");
    for(const [category,value] of [["surface.exterior_walls",99.5],["internal.people",.2],["internal.lighting",.3]]){
      bucketNodes.push({id:"driver."+category+"."+suffix,level:"driver",driverCategory:category,zoneName,period,serviceKind,scaleDomain:"thermal",unit:"kWh",value,sourceIds:[category+"."+suffix]});
    }
  }
  const buckets=view.energyPathGroupSmallNodes(bucketNodes,[]).nodes.filter(n=>n.automaticOther);
  check(buckets.length===4,"different zone/month/service buckets were merged or lost");
  for(const group of buckets){
    check(close(group.value,.5),"group crossed context buckets: wrong total");
    check((group.groupedMembers||[]).every(m=>m.zoneName===group.zoneName&&m.period===group.period&&m.serviceKind===group.serviceKind&&m.scaleDomain===group.scaleDomain),"group combined incompatible scope/month/service/domain");
  }
  const laneNodes=[endUse("cooling",99.6),endUse("lighting",.1),endUse("refrigeration",.1),endUse("fans",.1),endUse("pumps",.1),carrier("electricity",100)];
  const laneResult=view.energyPathGroupSmallNodes(laneNodes,laneNodes.filter(n=>n.level==="end_use").map(n=>branch(n)));
  check(laneResult.nodes.some(n=>n.endUse==="fans")&&laneResult.nodes.some(n=>n.endUse==="pumps"),"lower auxiliary lane was absorbed into direct-use Other");
  check(laneResult.nodes.filter(n=>n.automaticOther).every(n=>(n.groupedMembers||[]).every(m=>!["fans","pumps"].includes(m.endUse))),"Other inspector members mixed auxiliary/direct lanes");
  const fansAndPumps=view.energyPathProjectEndUsePresentation([endUse("fans",2),endUse("pumps",3),carrier("electricity",5)],[branch(endUse("fans",2)),branch(endUse("pumps",3))]).nodes.find(n=>n.endUse==="fans_pumps");
  check(fansAndPumps?.value===5&&equalSet(fansAndPumps.originalNodeIds,["end_use.fans.building","end_use.pumps.building"]),"existing fans/pumps presentation lost original node IDs");
  check(equalSet((fansAndPumps?.groupedMembers||[]).map(m=>m.id),["end_use.fans.building","end_use.pumps.building"]),"existing fans/pumps presentation lost member snapshots");
  const serviceDrivers=["cooling","heating"].map((service,index)=>({id:"driver.internal.people."+service+".building",level:"driver",driverCategory:"internal.people",serviceKind:service,value:index+2,allocatedValue:index+2,unit:"kWh",scaleDomain:"thermal",sourceIds:["people."+service]}));
  const mergedPeople=view.energyPathMergeAllServiceDrivers(serviceDrivers,[]).nodes.find(n=>n.driverCategory==="internal.people");
  check(mergedPeople?.value===5&&equalSet(mergedPeople.originalNodeIds,serviceDrivers.map(n=>n.id)),"All-service driver projection lost service-specific original IDs");
  check(equalSet((mergedPeople?.groupedMembers||[]).map(m=>m.id),serviceDrivers.map(n=>n.id)),"All-service driver projection lost original allocation members");

  const explanation={schema:"semantic-idf.energy-explanation/v2",scope:{kind:"building",aggregationBasis:"model_total"},nodes,links,sources:endUses.map(n=>({id:n.sourceIds[0],name:n.label,sourceType:"sql_meter",isMeter:true,normalizedUnit:"kWh"}))};
  const explanationBefore=JSON.stringify(explanation);
  const state={simulationEnergyScopeKind:"building",simulationEnergyPeriod:"annual",simulationEnergyService:"all"};
  const graph=view.energyPathGraphForState(explanation,{...state});
  const renderedOther=graph.nodes.find(n=>n.automaticOther&&n.level==="end_use");
  check(renderedOther && equalSet(renderedOther.originalNodeIds,wantedMembers),"standard projections lost original IDs before rendering");
  document.getElementById("mount").innerHTML=view.renderEnergyPathView(explanation,{...state,simulationEnergySelection:renderedOther?.id});
  const beforeCount=document.querySelectorAll('[data-energy-path-stage] [data-energy-explanation-node]').length;
  const details=document.querySelector('details[data-energy-path-group-members]');
  check(details && !details.open && details.querySelector('summary')?.textContent.includes("Expand"),"Other inspector lacks collapsed native Expand details");
  details?.querySelector("summary")?.click();
  check(details?.open,"native Expand did not open inspector members");
  check(details?.querySelectorAll('[data-energy-path-group-member]').length===3,"Expand did not show every original grouped member");
  check(document.querySelectorAll('[data-energy-path-stage] [data-energy-explanation-node]').length===beforeCount,"Expand increased graph node count");
  document.getElementById("mount").innerHTML=view.renderEnergyPathView(explanation,{...state,simulationEnergySelection:"end_use.lighting.building"});
  check(document.querySelector('[data-energy-path-stage] [aria-pressed="true"]')?.dataset.energyExplanationNode===renderedOther?.id,"original grouped member selection did not resolve to its visible Other node");
  check(document.querySelector('details[data-energy-path-group-members]'),"selecting an original grouped ID did not open the containing inspector");
  check(!document.querySelector('[data-energy-node-limit], [data-energy-path-node-limit]'),"grouping introduced node-limit control");
  check(JSON.stringify(explanation)===explanationBefore,"render/projection mutated original export payload");
  if(failures.length)throw new Error(failures.join(" | "));
  document.body.dataset.epath123Status="passed";
  document.getElementById("result").textContent="passed";
}catch(error){document.body.dataset.epath123Status="failed";document.getElementById("result").textContent=String(error?.stack||error);}
</script></body></html>`
