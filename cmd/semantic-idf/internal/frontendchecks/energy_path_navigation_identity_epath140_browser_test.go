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

func TestEPATH140SeriesNavigationIdentityBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser series identity verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath140-navigation-identity", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath140SeriesNavigationIdentityHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath140-navigation-identity").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH140 series identity browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath140-navigation-identity="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH140 series identity failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH140 series identity failed:\n%s", output)
	}
}

const epath140SeriesNavigationIdentityHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH140 series identity</title></head>
<body data-epath140-navigation-identity="pending"><pre id="result">pending</pre><script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
try {
  const {energyPathSeriesID:id,resolveEnergyPathSeriesCandidates:resolve,energyPathSeriesPeriodRange:range}=await import('/src/js/energy-path-navigation.js');
  const source={id:'sql-rdd-10',sourceFile:'run/eplusout.sql',sourceType:'sql_variable',name:'Zone Cooling Energy',keyValue:'Office',reportingFrequency:'Monthly',isMeter:false};
  const monthly={sourceId:source.id,file:'eplusout.sql',column:'Office:Zone Cooling Energy [J]',name:source.name,keyValue:source.keyValue,isMeter:false,reportingFrequency:'Monthly',points:[{label:'01-31 24:00',value:10},{label:'02-28 24:00',value:20}]};
  const hourly={...monthly,sourceId:'sql-rdd-20',reportingFrequency:'Hourly'};
  const node={sourceIds:[source.id]};
  const before=JSON.stringify({source,monthly,hourly,node});
  let candidates=resolve(node,[source],[hourly,monthly]);
  check(candidates.length===1&&candidates[0].status==='exact'&&candidates[0].series===monthly,'duplicate name/key/file selected hourly observation instead of dictionary identity');
  check(id(monthly)==='eplusout.sql::Office:Zone Cooling Energy [J]::sql-rdd-10','provenance-qualified identity changed');
  check(id({...monthly,sourceId:''})==='eplusout.sql::Office:Zone Cooling Energy [J]','legacy file/column identity changed');
  check(id(null)==='::','missing legacy identity threw');
  check(resolve(node,[source],[{...monthly,reportingFrequency:'Hourly'}]).length===0,'known frequency contradiction bypassed exact source ID');
  check(resolve(node,[source],[{...monthly,reportingFrequency:''}]).length===0,'missing observation frequency was inferred');
  check(resolve(node,[source],[{...monthly,file:'another.sql'}]).length===0,'known source file mismatch accepted');
  check(resolve(node,[{...source,sourceFile:'run-a/eplusout.sql'}],[{...monthly,file:'run-b/eplusout.sql'}]).length===0,'conflicting full paths collapsed to common basename');
  check(resolve(node,[source],[{...monthly,keyValue:'Other zone'}]).length===0,'typed zone contradiction bypassed dictionary ID');
  check(resolve(node,[source],[{...monthly,isMeter:true}]).length===0,'typed meter contradiction bypassed dictionary ID');
  check(resolve(node,[{...source,reportingFrequency:''}],[monthly])[0]?.status==='exact','exact dictionary ID could not retain actual observation frequency');
  check(resolve(node,[{...source,reportingFrequency:''}],[{...monthly,sourceId:''}]).length===0,'unqualified missing source frequency fabricated exact identity');
  candidates=resolve(node,[source],[monthly,{...monthly}]);
  check(candidates.length===1&&candidates[0].status==='ambiguous','duplicate exact observation silently selected the first item');
  const hourlySource={...source,id:hourly.sourceId,reportingFrequency:'Hourly'};
  candidates=resolve({sourceIds:[source.id,hourlySource.id]},[source,hourlySource],[monthly,hourly]);
  check(candidates.length===2&&candidates.every(item=>item.status==='exact'),'distinct source observations lost explicit exact choices');
  const derived={id:'derived',sourceType:'derived_formula',inputSourceIds:['cycle',source.id]};
  const cycle={id:'cycle',sourceType:'derived_formula',inputSourceIds:['derived']};
  candidates=resolve({sourceIds:['derived']},[derived,cycle,source],[monthly]);
  check(candidates.length===1&&candidates[0].source===source,'recursive derivation did not resolve leaf source or terminate cycle');
  check(resolve(node,[{...source,sourceType:'sql_tabular'}],[monthly]).length===0,'tabular row fabricated a raw series jump');
  const meterSource={id:'sql-rdd-30',sourceType:'sql_meter',isMeter:true,objectType:'Output:Meter:MeterFileOnly',name:'Cooling:Electricity',reportingFrequency:'Monthly'};
  const meterSeries={...monthly,sourceId:meterSource.id,column:'Electricity:Cooling [J]',name:'Electricity:Cooling',keyValue:'',isMeter:true};
  check(resolve({sourceIds:[meterSource.id]},[meterSource],[meterSeries])[0]?.status==='exact','truthful meter alias/Output subtype prevented dictionary-series navigation');
  check(JSON.stringify({source,monthly,hourly,node})===before,'series resolver mutated original source or observations');
  check(JSON.stringify(range(monthly,'annual'))===JSON.stringify({start:0,end:-1}),'annual range no longer covers original points');
  check(JSON.stringify(range(monthly,'M2'))===JSON.stringify({start:1,end:1}),'monthly range inferred point index instead of actual February label');
  check(range(monthly,'M3')===null,'missing month invented index range');
  const points=(labels)=>({points:labels.map((label,index)=>({label,value:index}))});
  check(JSON.stringify(range(points(['2024-01-31T23:00:00Z','2024-02-29T23:00:00Z','2024-03-31T23:00:00Z']),'M2'))===JSON.stringify({start:1,end:1}),'ISO leap calendar range was lost');
  check(range(points(['01-31 24:00','02-28 24:00','01-31 24:00']),'M1')===null,'noncontiguous month merged separate intervals');
  check(range(points(['2023-01-31','2024-01-31']),'M1')===null,'multi-year month silently merged years');
  check(range(points(['01-31 24:00','01-31 24:00']),'M1')===null,'duplicate month timestamp acquired exact range');
  check(range(points(['2023-02-29']),'M2')===null,'invalid non-leap date acquired month range');
  check(range(points(['01-31 24:30']),'M1')===null,'invalid end-of-day time acquired month range');
  check(range(points(['M1','M2']),'M2')===null,'unqualified labels acquired point-index month inference');
  for(const empty of [{},null,{points:[]}]) check(range(empty,'annual')===null&&range(empty,'M1')===null,'empty series enabled a blank chart');
  for(const bad of [null,false,true,'', '1',NaN,Infinity,undefined]) {
    const item={points:[{label:'01-31 24:00',value:bad}]};
    check(range(item,'annual')===null&&range(item,'M1')===null,'non-reported value coerced to a valid annual/monthly range: '+String(bad));
  }
  check(range({points:[{label:'01-31 24:00',value:null},{label:'02-28 24:00',value:0}]},'M2')?.start===1,'invalid value outside selected month blocked a valid reported zero');
  if(failures.length)throw new Error(failures.join(' | '));
  document.body.dataset.epath140NavigationIdentity='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.epath140NavigationIdentity='failed';document.getElementById('result').textContent=String(error?.stack||error);}
</script></body></html>`
