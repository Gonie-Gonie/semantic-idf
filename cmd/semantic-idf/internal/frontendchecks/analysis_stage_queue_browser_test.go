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
  document.body.dataset.queueStatus = 'passed';
  document.getElementById('result').textContent = 'concurrency, order, priority, stale input, and failure passed';
} catch (error) {
  document.body.dataset.queueStatus = 'failed'; document.getElementById('result').textContent = error.stack || String(error);
}
</script></body></html>`
