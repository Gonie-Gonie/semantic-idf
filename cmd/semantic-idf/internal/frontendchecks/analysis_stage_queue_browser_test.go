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

func TestAnalysisStageQueueBrowserHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping headless-browser analysis queue harness in short mode")
	}
	chrome := phaseHChromeExecutable()
	if chrome == "" {
		t.Skip("Chrome/Chromium/Edge is not installed")
	}
	mux := http.NewServeMux()
	mux.Handle("/src/", http.StripPrefix("/src/", http.FileServer(http.Dir(repoPath("frontend/src")))))
	mux.HandleFunc("/queue", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, analysisStageQueueHarnessHTML)
	})
	workspace := strings.Replace(readTestFile(t, "frontend/src/index.html"), "<head>", `<head><base href="/src/">`+analysisQueueWorkspaceSetupHTML, 1)
	mux.HandleFunc("/queue-workspace", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, workspace)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, chrome,
		"--headless=new", "--disable-gpu", "--disable-dev-shm-usage", "--no-sandbox", "--no-first-run", "--no-default-browser-check",
		"--virtual-time-budget=10000", "--user-data-dir="+t.TempDir(), "--dump-dom", server.URL+"/queue")
	output, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), `data-queue-status="passed"`) {
		t.Fatalf("analysis queue harness failed: %v\n%s", err, output)
	}
}

const analysisStageQueueHarnessHTML = `<!doctype html>
<html><body data-queue-status="pending"><pre id="result"></pre><script type="module">
const check = (ok, message) => { if (!ok) throw new Error(message); };
const deferred = () => { let resolve; const promise = new Promise(r => resolve = r); return { promise, resolve }; };
try {
  const { createAnalysisStageQueue } = await import('/src/js/analysis-stage-queue.js');
  const queue = createAnalysisStageQueue(['profile', 'hvac', 'geometry'], { analysisKey: 'input' });
  const gates = { profile: deferred(), hvac: deferred(), geometry: deferred() };
  const started = [];
  const running = queue.run(async stage => { started.push(stage); await gates[stage].promise; return stage; }, 2);
  check(started.join(',') === 'profile,hvac' && queue.running.size === 2, 'worker concurrency changed');
  gates.hvac.resolve();
  await Promise.resolve(); await Promise.resolve();
  check(started.join(',') === 'profile,hvac,geometry', 'available worker did not start pending stage');
  gates.geometry.resolve(); gates.profile.resolve();
  check((await running).join(',') === 'profile,hvac,geometry', 'completion order changed result order');
  check(queue.running.size === 0 && queue.completed.size === 3, 'queue did not settle');

  const priority = createAnalysisStageQueue(['profile', 'hvac', 'geometry']);
  check(priority.prioritize('geometry'), 'pending priority failed');
  const priorityOrder = [];
  await priority.run(stage => { priorityOrder.push(stage); return stage; }, 1);
  check(priorityOrder.join(',') === 'geometry,profile,hvac', 'priority changed');

  let current = true;
  const canceled = createAnalysisStageQueue(['profile', 'hvac', 'geometry'], { shouldContinue: () => current });
  const stopGate = deferred(); const canceledStarts = [];
  const stopping = canceled.run(async stage => { canceledStarts.push(stage); await stopGate.promise; }, 2);
  current = false; stopGate.resolve(); await stopping;
  check(canceledStarts.join(',') === 'profile,hvac' && canceled.pending.length === 1, 'stale queue launched extra work');
  const alreadyStale = createAnalysisStageQueue(['profile'], { shouldContinue: () => false });
  await alreadyStale.run(() => { throw new Error('stale work started'); }, 2);

  const failed = createAnalysisStageQueue(['profile', 'hvac', 'geometry']);
  const failGate = deferred(); const failure = new Error('expected stage failure');
  const failedStarts = []; let settled = false;
  const failing = failed.run(async stage => {
    failedStarts.push(stage);
    if (stage === 'profile') throw failure;
    await failGate.promise;
  }, 2).then(() => { throw new Error('failure lost'); }, error => { check(error === failure, 'wrong error'); settled = true; });
  await Promise.resolve(); await Promise.resolve();
  check(!settled && failedStarts.join(',') === 'profile,hvac', 'failure did not wait for running work');
  failGate.resolve(); await failing;
  check(failed.pending.length === 1 && failed.running.size === 0, 'failure launched extra work');

  // Exercise the actual startup, index DOM, action pipeline and active-tab
  // renderer, including an unfinished HVAC stage restored from the workspace.
  const frame = document.createElement('iframe'); frame.src = '/queue-workspace'; document.body.append(frame);
  const waitFor = async (predicate, message) => {
    for (let attempt=0; attempt<300; attempt++) { if (predicate()) return; await new Promise(resolve => setTimeout(resolve,20)); }
    throw new Error(message + ': ' + frame.contentDocument?.querySelector('#runtimeStatus')?.textContent);
  };
  await waitFor(() => frame.contentWindow.__queueWorkspace && frame.contentWindow.__analysisStageStarts.includes('hvac'), 'restored HVAC workspace did not reach its queued stages');
  const win = frame.contentWindow, doc = frame.contentDocument, {state,actions,navigation} = win.__queueWorkspace;
  const verifyPending = () => {
    check(state.activeResultTab === 'hvac' && !state.analysisReady.hvac, 'HVAC was not the active pending tab');
    check(Boolean(doc.querySelector('#hvacSummary .status-loading')), 'pending HVAC placeholder missing');
    check(Boolean(doc.querySelector('#textObjectView .text-object-head')), 'quick analysis did not render editable input while HVAC was pending');
    check(doc.querySelector('#runtimeStatus').classList.contains('status-loading'), 'pending renderer interrupted the analysis');
  };
  const finishStages = async () => {
    for (const stage of ['hvac','profile','geometry']) {
      await waitFor(() => win.__analysisStageWaiters[stage], 'stage was not launched: '+stage);
      win.__analysisStageWaiters[stage]();
    }
    await waitFor(() => state.analysisStage === 'complete', 'queued analysis did not finish');
    check(state.analysisReady.hvac && state.analysisReady.profile && state.geometryReady, 'stage readiness did not settle');
    check(!doc.querySelector('#hvacSummary .status-loading') && Boolean(doc.querySelector('#hvacSummary .hvac-navigator')), 'ready HVAC view did not replace the pending placeholder');
    check(Boolean(doc.querySelector('#textObjectView .text-object-head')), 'completed analysis lost the input view');
    check(win.__queueWorkspaceErrors.length === 0, win.__queueWorkspaceErrors.join('\n'));
    check(!doc.querySelector('#runtimeStatus').textContent.includes('Cannot set properties'), 'analysis ended in an error status');
  };
  verifyPending();
  navigation.switchResultTab('profile', {recordHistory:false});
  check(Boolean(doc.querySelector('#profileGraph .status-loading')), 'other pending result tab did not render');
  navigation.switchResultTab('hvac', {recordHistory:false}); verifyPending();
  await finishStages();

  // Reanalyzing another opened document must work with HVAC still selected.
  win.__analysisStageStarts.length = 0; win.__analysisStageWaiters = {};
  win.__queueWorkspace.setDocumentText('Version,25.1;\nBuilding,Reopened;\n');
  actions.registerLoadedDocument(state.documentText, {filename:'reopened.idf'});
  const reanalysis = actions.analyze();
  await waitFor(() => win.__analysisStageStarts.includes('hvac'), 'reopened HVAC input did not reach queued stages');
  verifyPending(); await finishStages(); await reanalysis;
  frame.remove();
  document.body.dataset.queueStatus = 'passed';
  document.getElementById('result').textContent = 'concurrency, order, priority, stale input, failure, restored HVAC startup, pending input, and reopened HVAC analysis passed';
} catch (error) {
  document.body.dataset.queueStatus = 'failed'; document.getElementById('result').textContent = error.stack || String(error);
}
</script></body></html>`

const analysisQueueWorkspaceSetupHTML = `<script>
window.__queueWorkspaceErrors = [];
window.addEventListener('error', event => window.__queueWorkspaceErrors.push(event.error?.stack || event.message));
window.addEventListener('unhandledrejection', event => window.__queueWorkspaceErrors.push(event.reason?.stack || String(event.reason)));
sessionStorage.setItem('idfAnalyzer.currentDocument', JSON.stringify({schemaVersion:4,text:'Version,25.1;\nBuilding,Restored;\n',filename:'restored.idf',activeInputView:'text',activeResultTab:'hvac'}));
window.__analysisStageStarts = []; window.__analysisStageWaiters = {};
const queueObject = {index:0,sourceIndex:0,type:'Version',name:'25.1',fields:[{key:'version_identifier',comment:'Version Identifier',value:'25.1'}]};
window.go = {main:{App:{
  GetSettings: async () => ({settings:{appearance:{language:'en',theme:'dark',defaultInputView:'text',analysisTabOrder:['hvac','metrics','topology','profile','simulation']}}}),
  GetAppInfo: async () => ({name:'SemanticIDF',version:'test'}),
  GetSimulationEnvironment: async () => ({weatherFolders:[],installations:[],defaultWorkerCount:1}),
  SetStorageInputPath: async () => {},
  AnalyzeInputQuickText: async () => ({report:{metrics:{categories:[]},objects:[queueObject]},model:{format:'idf',version:{raw:'25.1'},objects:[queueObject]},epjson:'{}'}),
  AnalyzeInputStageText: async (text,stage) => {
    window.__analysisStageStarts.push(stage);
    await new Promise(resolve => { window.__analysisStageWaiters[stage] = resolve; });
    return {report:{profile:{},hvac:{loops:[]},geometry:{}}};
  },
}}};
</script><script type="module">
import {state,setDocumentText} from '/src/js/state.js';
import * as actions from '/src/js/actions.js';
import * as navigation from '/src/js/navigation.js';
window.__queueWorkspace = {state,setDocumentText,actions,navigation};
</script>`
