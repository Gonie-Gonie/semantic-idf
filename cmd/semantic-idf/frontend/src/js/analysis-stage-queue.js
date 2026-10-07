// A queue belongs to one input snapshot. Cancel pending work when that snapshot
// changes; already-running backend requests are allowed to settle.
export function createAnalysisStageQueue(stages, options = {}) {
  return {
    analysisKey: options.analysisKey || "",
    pending: stages.map((stage, index) => ({ stage, index })),
    running: new Set(),
    completed: new Set(),
    prioritize(stage) {
      const index = this.pending.findIndex((task) => task.stage === stage);
      if (index <= 0) {
        return false;
      }
      const [task] = this.pending.splice(index, 1);
      this.pending.unshift(task);
      return true;
    },
    async run(worker, concurrency) {
      const results = [];
      let failed = false;
      const runNext = async () => {
        while (!failed && (options.shouldContinue?.() ?? true)) {
          const task = this.pending.shift();
          if (!task) {
            return;
          }
          this.running.add(task.stage);
          try {
            results[task.index] = await worker(task.stage);
          } catch (error) {
            failed = true;
            throw error;
          } finally {
            this.running.delete(task.stage);
            this.completed.add(task.stage);
          }
        }
      };
      const workers = Array.from({ length: Math.min(concurrency, stages.length) }, runNext);
      const settled = await Promise.allSettled(workers);
      const rejected = settled.find((result) => result.status === "rejected");
      if (rejected) {
        throw rejected.reason;
      }
      return results;
    },
  };
}
