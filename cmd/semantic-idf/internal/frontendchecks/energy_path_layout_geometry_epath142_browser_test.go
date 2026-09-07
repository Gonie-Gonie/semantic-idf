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

func TestEPATH142PureLayoutGeometryBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser pure layout verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath142-layout-geometry", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath142PureLayoutGeometryHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath142-layout-geometry").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH142 pure layout browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath142-layout-geometry="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH142 pure layout failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH142 pure layout failed:\n%s", output)
	}
}

const epath142PureLayoutGeometryHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH142 pure layout</title></head>
<body data-epath142-layout-geometry="pending"><pre id="result">pending</pre><script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
const freeze=value=>{if(value&&typeof value==='object'){Object.values(value).forEach(freeze);Object.freeze(value);}return value;};
try {
  const {energyPathLayout:layout}=await import('/src/js/energy-path-layout.js');
  const driverCategories=['envelope.exterior_wall','envelope.roof','envelope.ground_floor','envelope.window_conduction','envelope.window_solar','envelope.interzone','envelope.internal_mass','envelope.other_side','air.infiltration','air.ventilation','internal.people','balance.storage_other'];
  const drivers=driverCategories.map((driverCategory,index)=>({id:'driver.'+index,level:'driver',driverCategory,label:driverCategory,value:index+1,unit:'kWh thermal',scaleDomain:'thermal',sourceIds:['source.'+index]}));
  const main=['cooling','heating'].flatMap(service=>[
    {id:'load.'+service,level:'load',serviceKind:service,value:100,scaleDomain:'thermal'},
    {id:'end_use.'+service,level:'end_use',endUse:service,serviceKind:service,value:25,scaleDomain:'site'},
  ]);
  const categories=['fans_pumps','hvac_auxiliaries','lighting','equipment','water_systems','refrigeration','other','storage_charge'];
  const direct=categories.map((endUse,index)=>({id:'end_use.'+endUse,level:'end_use',endUse,value:index+1,serviceKind:index<2?'cooling':'all',scaleDomain:'site'}));
  const residual={id:'residual.electricity',level:'residual',presentationLevel:'end_use',presentationKind:'unclassified_energy',value:5,scaleDomain:'site'};
  const carriers=['electricity','natural_gas'].map(carrier=>({id:'carrier.'+carrier,level:'carrier',carrier,value:200,scaleDomain:'site'}));
  const nodes=freeze([...drivers,...main,...direct,residual,...carriers,{id:'support.production',level:'support',value:20}]);
  const links=freeze([
    ...drivers.map((node,index)=>({id:'driver-link.'+index,fromId:node.id,toId:'load.cooling',relation:'driver_to_load',fromValue:index+1,toValue:index+1})),
    ...['cooling','heating'].map(service=>({id:'conversion.'+service,fromId:'load.'+service,toId:'end_use.'+service,relation:'load_to_end_use',fromValue:100,toValue:25})),
    ...[...main.filter(node=>node.level==='end_use'),...direct].map(node=>({id:'carrier-link.'+node.id,fromId:node.id,toId:'carrier.electricity',relation:node.endUse==='cooling'?'end_use_to_carrier':'direct_end_use_to_carrier'})),
    {id:'residual-link',fromId:residual.id,toId:'carrier.electricity',relation:'residual'},
    {id:'counterpart',fromId:'driver.10',toId:'end_use.lighting',relation:'source_correspondence'},
    {id:'forced-load',fromId:'load.cooling',toId:'end_use.fans_pumps',relation:'load_to_end_use'},
    {id:'reverse',fromId:'carrier.electricity',toId:'end_use.cooling',relation:'end_use_to_carrier'},
    {id:'support-link',fromId:'support.production',toId:'carrier.electricity',relation:'support_supply'},
  ]);
  const before=JSON.stringify({nodes,links});
  const result=layout(nodes,links,{width:1000,height:420});
  check(JSON.stringify(result.columns.map(item=>item.level))==='["driver","load","end_use","carrier"]','four fixed columns lost canonical direction');
  check(result.width===1000&&result.height===420&&result.dividerX===500,'plot/divider geometry drifted');
  const byID=new Map(result.nodes.map(node=>[node.id,node]));
  const band=result.lanes.find(lane=>lane.id==='direct');
  check(band.startLevel==='end_use'&&band.y>0&&band.labelHeight===20,'direct lane fabricated a driver/load stage or omitted caption space');
  check(result.nodes.length===nodes.length-1&&result.nodes.filter(node=>node.level==='carrier').length===2,'layout hid real nodes or cloned carriers perlane');
  check(drivers.every(node=>byID.get(node.id)?.lane==='thermal'&&byID.get(node.id).height>=28),'twelve drivers did not fit fullheight readable rows');
  check(main.every(node=>byID.get(node.id)?.lane==='main'&&byID.get(node.id).y+byID.get(node.id).height<=band.y),'thermal conversions escaped main lane');
  check(direct.every(node=>byID.get(node.id)?.lane==='direct'&&byID.get(node.id).y>=band.y+band.labelHeight),'direct category or cooling-allocatedfan went through thermal lane');
  check(result.nodes.filter(node=>node.lane==='direct').length===9&&result.nodes.filter(node=>node.lane==='direct').every(node=>node.height>=30),'nine direct/residual cards shrank text rows before whitespace');
  check(result.nodes.filter(node=>node.lane==='main').every(node=>node.height>=52),'dense direct lane took space required by load/HVAC badges');
  check(byID.get(residual.id)?.level==='end_use'&&byID.get(residual.id)?.lane==='direct'&&byID.get(residual.id)?.node===residual,'presented residual lost original identity or lowerlane');
  check(carriers.every(node=>byID.get(node.id)?.lane==='shared'),'carrier geometry duplicated into main/direct lanes');
  for(const node of result.nodes){
    check(node.node===nodes.find(original=>original.id===node.id),'layout replaced original node object');
    check([node.x,node.y,node.width,node.height,node.anchorY].every(Number.isFinite)&&node.width>0&&node.height>0,'non-finite/negative geometry');
    check(node.x>=0&&node.y>=0&&node.x+node.width<=result.width+1e-8&&node.y+node.height<=result.height+1e-8,'node escaped bounded plot');
    check(node.labelBox.maxLines===2&&node.labelBox.width<=node.width&&node.labelBox.height<=node.height,'labelbox lost two-line limit');
  }
  check(result.links.every(link=>links.includes(link.link)&&byID.has(link.fromId)&&byID.has(link.toId)&&link.fromAnchor.x<link.toAnchor.x),'flow anchors fabricated records or reversed direction');
  check(!result.links.some(link=>['counterpart','forced-load','reverse','support-link'].includes(link.id)),'non-flow correspondence/directload/support entered main flow geometry');
  check(result.links.some(link=>link.id==='residual-link'),'vetted positive unclassified residual lost carrier geometry');
  for(const link of result.links){const from=byID.get(link.fromId),to=byID.get(link.toId);check(link.fromAnchor.x===from.x+from.width&&link.toAnchor.x===to.x&&link.fromAnchor.y===from.anchorY&&link.toAnchor.y===to.anchorY,'link anchor disagrees with existing node geometry');}
  for(const width of [320,760,1000,1600]){
    const resized=layout(nodes,links,{width,height:420});
    check(resized.width===width&&resized.nodes.every(node=>node.x>=0&&node.x+node.width<=width+1e-8),'responsive width depended on a horizontal minimum');
  }
  const shape=value=>value.nodes.map(({id,level,lane,x,y,width,height})=>({id,level,lane,x,y,width,height}));
  const zoneNodes=nodes.map(node=>({...node,zoneName:'Office',value:node.value*3}));
  check(JSON.stringify(shape(layout(zoneNodes,links)))===JSON.stringify(shape(result)),'scope/multiplier changed structural geometry');
  check(JSON.stringify(shape(layout(nodes,links)))===JSON.stringify(shape(result)),'repeat layout is nondeterministic');
  const reversedDrivers=layout([...drivers].reverse(),[]).nodes.map(node=>node.id);
  check(JSON.stringify(reversedDrivers)===JSON.stringify([...drivers].reverse().map(node=>node.id)),'142 helper silently introduced value/taxonomy sorting');
  const duplicated=layout([...nodes,carriers[0]],links);
  check(duplicated.nodes.filter(node=>node.id===carriers[0].id).length===1,'duplicate carrier identity produced duplicate visualnode');
  const twoResidualNodes=[...nodes.filter(node=>node.id!=='end_use.storage_charge'),{...residual,id:'residual.natural_gas'}];
  check(layout(twoResidualNodes,links).nodes.filter(node=>node.lane==='direct').every(node=>node.height>=30),'seven direct categories plus two carrier residuals lost readable rows');
  const onlyDirect=layout(direct,[]);
  check(onlyDirect.lanes.find(lane=>lane.id==='main').height===0&&onlyDirect.nodes.every(node=>node.lane==='direct'),'direct-only graph fabricated thermal lane');
  const noDirect=layout(main,[]);
  check(noDirect.lanes.find(lane=>lane.id==='direct').height===0&&noDirect.nodes.every(node=>node.lane==='main'),'thermal-only graph reserved an emptydirect lane');
  const empty=layout(null,null,{width:NaN,height:-1});
  check(empty.width===1000&&empty.height===420&&empty.columns.length===4&&empty.nodes.length===0,'empty/invaliddimensions fabricated nodes or unsafe geometry');
  check(!('fromWidth' in result.links[0])&&!('energyScale' in result),'142 prematurely synthesized quantitative143 ribbon widths');
  check(JSON.stringify({nodes,links})===before,'geometry mutated source values/IDs/links');
  if(failures.length)throw new Error(failures.join(' | '));
  document.body.dataset.epath142LayoutGeometry='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.epath142LayoutGeometry='failed';document.getElementById('result').textContent=String(error?.stack||error);}
</script></body></html>`
