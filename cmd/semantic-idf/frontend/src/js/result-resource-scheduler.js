// One bounded queue per named result. Foreground requests promote the same
// promise; prefetch never creates a second request or an unbounded Promise.all.
export function createResultResourceScheduler(load, { concurrency = 2, inFlightBytes = 8 * 1024 * 1024 } = {}) {
  const tasks = new Map();
  let active = 0, bytes = 0, sequence = 0, disposed = false;
  function pump() {
    if (disposed) return;
    const pending = [...tasks.values()].filter(task => task.status === "queued")
      .sort((a, b) => a.priority - b.priority || a.sequence - b.sequence);
    while (active < concurrency && pending.length) {
      const task = pending.shift();
      if (active && bytes + task.bytes > inFlightBytes) break;
      task.status = "loading"; active++; bytes += task.bytes;
      Promise.resolve().then(() => load(task.resource)).then(value => {
        task.status = "ready"; task.resolve(value);
      }, error => {
        task.status = "failed"; task.reject(error);
      }).finally(() => { active--; bytes -= task.bytes; pump(); });
    }
  }
  return {
    request(resource, priority = 0) {
      if (disposed) return Promise.reject(new DOMException("Result released", "AbortError"));
      const previous = tasks.get(resource.id);
      // A speculative read can fail while the app is idle. One later explicit
      // request may retry that read; foreground failures are never looped.
      if (previous && !(previous.status === "failed" && previous.priority > 0 && priority === 0)) {
        previous.priority = Math.min(previous.priority, priority); pump(); return previous.promise;
      }
      const task = { resource, bytes: resource.byteLength || 0, priority, sequence: sequence++, status: "queued" };
      task.promise = new Promise((resolve, reject) => { task.resolve = resolve; task.reject = reject; });
      // Prefetch and cancelled owners can leave no consumer; retain the failure
      // for a later explicit request without creating an unhandled rejection.
      task.promise.catch(() => {});
      tasks.set(resource.id, task); pump(); return task.promise;
    },
    status(id) { return tasks.get(id)?.status || "idle"; },
    dispose() {
      disposed = true;
      for (const task of tasks.values()) if (task.status === "queued") task.reject(new DOMException("Result released", "AbortError"));
      tasks.clear();
    },
  };
}
