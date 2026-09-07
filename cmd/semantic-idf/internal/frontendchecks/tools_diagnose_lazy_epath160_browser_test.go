package frontendchecks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestEPATH160BatchStartupDefersDiagnoseBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("headless-browser lazy Diagnose activation verification")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	page, err := os.ReadFile(repoPath("frontend/src/tools.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := strings.Replace(string(page), `<script type="module" src="./js/tools.js"></script>`, toolsDiagnoseHarnessSetup+epath160LazyDiagnoseSetup+`<script type="module" src="/src/js/tools.js"></script>`+epath160LazyDiagnoseAssertions, 1)
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/epath160-lazy-diagnose", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, html)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check", "--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/epath160-lazy-diagnose#batch-simulation").CombinedOutput()
	if err != nil {
		t.Fatalf("EPATH160 lazy Diagnose browser: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `data-epath160-lazy-diagnose="passed"`) {
		document := string(output)
		if start := strings.Index(document, `<pre id="epath160LazyDiagnoseResult">`); start >= 0 {
			if end := strings.Index(document[start:], "</pre>"); end >= 0 {
				t.Fatalf("EPATH160 lazy Diagnose failed: %s", document[start:start+end])
			}
		}
		t.Fatalf("EPATH160 lazy Diagnose failed:\n%s", output)
	}
}

const epath160LazyDiagnoseSetup = `<script>
mainWorkspaceSnapshot.simulationResultRef={textHash:'a'.repeat(64),runId:'previous-main-run'};
sessionStorage.setItem('idfAnalyzer.currentDocument',JSON.stringify(mainWorkspaceSnapshot));
window.epath160DiagnoseCalls={analyze:0,scan:0};
for(const [name,key] of [['AnalyzeInputDiagnosticsText','analyze'],['ScanCleanupText','scan']]){
  const original=window.go.main.App[name];
  window.go.main.App[name]=async(...args)=>{window.epath160DiagnoseCalls[key]++;return original(...args);};
}
</script>`

const epath160LazyDiagnoseAssertions = `<script>
(() => {
  const result=document.createElement('pre');result.id='epath160LazyDiagnoseResult';document.body.append(result);
  const failures=[],check=(value,message)=>{if(!value)failures.push(message);};
  const waitFor=predicate=>new Promise((resolve,reject)=>{
    const start=performance.now(),timer=setInterval(()=>{if(predicate()){clearInterval(timer);resolve();}else if(performance.now()-start>8000){clearInterval(timer);reject(Error('Timed out waiting for lazy Diagnose'));}},20);
  });
  const tab=name=>document.querySelector('[data-tools-tab="'+name+'"]');
  (async()=>{
    await waitFor(()=>tab('batch-simulation')?.classList.contains('active'));
    check(window.epath160DiagnoseCalls.analyze===0&&window.epath160DiagnoseCalls.scan===0,'Batch startup analyzed/scanned hidden Diagnose input');
    check(document.querySelector('#diagnoseFilename').textContent==='current.idf','lazy input hydration lost the restored filename');
    check(JSON.stringify(JSON.parse(sessionStorage.getItem('idfAnalyzer.currentDocument')))===JSON.stringify(mainWorkspaceSnapshot),'Batch startup mutated the Energy workspace snapshot');
    tab('batch-metrics').click();tab('batch-simulation').click();
    check(window.epath160DiagnoseCalls.analyze===0&&window.epath160DiagnoseCalls.scan===0,'switching Batch pages activated Diagnose');
    tab('diagnose').click();tab('diagnose').click();tab('batch-simulation').click();tab('diagnose').click();
    await waitFor(()=>document.querySelector('#diagnoseList').textContent.includes('Broken reference')&&!document.querySelector('#diagnoseRefresh').disabled);
    check(window.epath160DiagnoseCalls.analyze===1&&window.epath160DiagnoseCalls.scan===1,'first Diagnose activation did not analyze/scan exactly once');
    check(document.querySelector('#diagnoseCandidates').textContent.includes('Unused Schedule'),'first Diagnose activation lost actual cleanup candidates');
    tab('batch-simulation').click();tab('diagnose').click();
    check(window.epath160DiagnoseCalls.analyze===1&&window.epath160DiagnoseCalls.scan===1,'reopening unchanged Diagnose needlessly rescanned');
    document.querySelector('#diagnoseRefresh').click();
    await waitFor(()=>window.epath160DiagnoseCalls.analyze===2&&window.epath160DiagnoseCalls.scan===2&&!document.querySelector('#diagnoseRefresh').disabled);
    document.querySelector('#diagnoseApply').click();
    await waitFor(()=>window.epath160DiagnoseCalls.analyze===3&&window.epath160DiagnoseCalls.scan===3&&!document.querySelector('#diagnoseRefresh').disabled);
    const applied=JSON.parse(sessionStorage.getItem('idfAnalyzer.currentDocument'));
    check(applied.text==='Version, 24.2;\n'&&applied.simulationResultRef===null,'Diagnose apply retained a result locator for invalidated input');
    // A replacement can have identical text but a different physical file.
    // It must still drop the previous run locator when replacing the workspace.
    applied.simulationResultRef={textHash:'b'.repeat(64),runId:'same-text-previous-file'};
    sessionStorage.setItem('idfAnalyzer.currentDocument',JSON.stringify(applied));
    window.go.main.App.OpenInputFile=async()=>({canceled:false,text:applied.text,filename:'identical-text.idf',path:'C:/another/identical-text.idf'});
    document.querySelector('#diagnoseSelectInput').click();
    await waitFor(()=>document.querySelector('#diagnoseFilename').textContent==='identical-text.idf'&&window.epath160DiagnoseCalls.analyze===4&&window.epath160DiagnoseCalls.scan===4&&!document.querySelector('#diagnoseRefresh').disabled);
    const replacement=JSON.parse(sessionStorage.getItem('idfAnalyzer.currentDocument'));
    check(replacement.text===applied.text&&replacement.path==='C:/another/identical-text.idf'&&replacement.simulationResultRef===null,'same-text different-file replacement retained the old run locator');
    if(failures.length)throw Error(failures.join('\n'));
    document.body.dataset.epath160LazyDiagnose='passed';result.textContent='PASS '+JSON.stringify(window.epath160DiagnoseCalls);
  })().catch(error=>{document.body.dataset.epath160LazyDiagnose='failed';result.textContent=error.stack||String(error);});
})();
</script>`
