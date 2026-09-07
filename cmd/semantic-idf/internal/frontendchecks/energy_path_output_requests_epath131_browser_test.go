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

func TestEPATH131OutputRequestIdentityBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless browser output-request identity verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath131-resolver", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, epath131OutputRequestIdentityHTML)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath131-resolver").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH131 resolver browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath131-resolver="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="result">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH131 resolver failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH131 resolver failed:\n%s", output)
	}
}

const epath131OutputRequestIdentityHTML = `<!doctype html><html><head><meta charset="utf-8"><title>EPATH131 request identity</title></head>
<body data-epath131-resolver="pending"><pre id="result">pending</pre><script type="module">
const failures=[];
const check=(condition,message)=>{if(!condition)failures.push(message);};
try {
  const {resolveEnergyPathOutputRequest:resolve,energyPathOutputRequestKey:key,energyPathOutputRequestFields:fields}=await import('/src/js/energy-path-output-requests.js');
  const monthly={objectType:'Output:Variable',keyValue:'Office',variableName:'Zone Cooling Energy',reportingFrequency:'Monthly',objectIndex:1};
  const hourly={...monthly,reportingFrequency:'Hourly',objectIndex:0};
  const wildcard={...monthly,keyValue:'*',objectIndex:2};
  const source={id:'cooling',sourceType:'sql_report_data',name:'Zone Cooling Energy',keyValue:'Office',reportingFrequency:'monthly',objectIndex:0};
  const inputs=[hourly,monthly,wildcard];
  const before=JSON.stringify({source,inputs});
  let result=resolve(source,inputs);
  check(result.status==='exact'&&result.request===monthly&&result.requestIndex===1,'stale hourly objectIndex overrode monthly source identity');
  check(resolve(source,[wildcard,monthly]).request===monthly,'unique exact key did not outrank wildcard request');
  check(resolve(source,[hourly,wildcard]).request===wildcard,'truthful single wildcard request was not resolved');
  const duplicate={...monthly,objectIndex:3};
  for(const missingIndex of [null,undefined,'',false]){
    result=resolve({...source,objectIndex:missingIndex},[{...monthly,objectIndex:0},duplicate]);
    check(result.status==='ambiguous'&&result.request===null&&result.requestIndex===-1&&result.candidates.length===2,'missing/non-index value became IDF index0 or first duplicate won');
  }
  check(resolve({...source,objectIndex:3},[monthly,duplicate]).request===duplicate,'valid index could not distinguish identical exact requests');
  const unknownFrequency=resolve({...source,reportingFrequency:''},[monthly]);
  check(unknownFrequency.status==='unavailable'&&unknownFrequency.request===null&&unknownFrequency.candidates[0]===monthly,'missing source frequency fabricated an exact request');
  check(resolve(source,[{...monthly,reportingFrequency:''}]).status==='unavailable','missing request frequency was treated as exact');
  check(resolve({...source,keyValue:''},[monthly]).status==='unavailable','missing observed key acquired an exact zone request');
  for(const blankKey of ['',undefined]){
    result=resolve(source,[{...monthly,keyValue:blankKey}]);
    check(result.status==='unavailable'&&result.request===null,'absent request key silently became an explicit wildcard');
  }
  const meter={objectType:'Output:Meter',keyValue:'Electricity:Cooling',reportingFrequency:'MONTHLY'};
  const meterSource={id:'meter',isMeter:true,name:'Cooling:Electricity',reportingFrequency:'Monthly'};
  check(resolve(meterSource,[meter]).request===meter,'carrier/end-use meter-order alias was not accepted');
  check(resolve({...meterSource,name:'Heating:Electricity'},[meter]).status==='unavailable','meter alias matching crossed end-use identity');
  const fieldsMeter={objectType:'Output:Meter',fields:[{name:'Key Name',value:'Electricity:Cooling'},{name:'Reporting Frequency',value:'Monthly'}]};
  check(resolve(meterSource,[fieldsMeter]).request===fieldsMeter,'meter Key Name field fallback was lost');
  const fieldsVariable={objectType:'Output:Variable',fields:[{name:'Key Value',value:'Office'},{name:'Variable Name',value:'Zone Cooling Energy'},{name:'Reporting Frequency',value:'Monthly'}]};
  check(resolve(source,[fieldsVariable]).request===fieldsVariable,'variable named-field fallback was lost');
  check(JSON.stringify(fields(fieldsVariable))===JSON.stringify({objectType:'Output:Variable',keyValue:'Office',variableName:'Zone Cooling Energy',reportingFrequency:'Monthly'}),'display helper diverged from original-case field-only variable request identity');
  check(fields(fieldsMeter).keyValue==='Electricity:Cooling'&&fields(fieldsMeter).reportingFrequency==='Monthly','display helper lost meter Key Name/frequency identity');
  check(resolve({...source,objectType:'Output:Meter',sourceType:'sql_variable',isMeter:false},[meter]).status==='unavailable','contradictory sql_variable/type evidence produced a meter jump');
  check(resolve({...meterSource,sourceType:'sql_meter',objectType:'Output:Variable'},[monthly]).status==='unavailable','contradictory sql_meter/type evidence produced a variable jump');
  check(resolve(source,[{...monthly,objectType:'Output:Meter'}]).status==='unavailable','stale index bypassed output type');
  check(resolve({...meterSource,sourceType:'sql_tabular'},[meter]).status==='tabular','SQL tabular alias falsely acquired a direct meter request');
  check(resolve({...meterSource,sourceType:'derived_formula'},[meter]).status==='derived','derived formula falsely acquired a direct meter request');
  const derived={...meterSource,inputSourceIds:['meter']};
  result=resolve(derived,[meter],[meterSource]);
  check(result.status==='derived'&&result.request===null&&result.candidates.length===0,'derived input traversal selected an input request as its own');
  check(resolve('meter',[meter],[meterSource]).request===meter,'explicit input source ID lookup was lost');
  check(resolve(source,[]).status==='unavailable','missing run plan acquired an invented request');
  check(JSON.stringify({source,inputs})===before,'resolver mutated source or output plan');
  check(key(monthly,0)===key({...monthly},0),'focus token is not deterministic');
  check(key(monthly,0)!==key(monthly,1),'duplicate request rows collide in focus token');
  check(key(monthly,0)!==key(hourly,0),'different frequency identities collide in focus token');
  check(/^[a-zA-Z0-9-]+$/.test(key({...monthly,keyValue:'서울 "Office" <&>'},0)),'focus token contains unsafe DOM identity characters');
  if(failures.length)throw new Error(failures.join(' | '));
  document.body.dataset.epath131Resolver='passed';document.getElementById('result').textContent='passed';
}catch(error){document.body.dataset.epath131Resolver='failed';document.getElementById('result').textContent=String(error?.stack||error);}
</script></body></html>`
