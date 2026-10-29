package simulation

import "runtime"

const maxResultBuildParallelism = 4

// Callers always make progress inline. These nonblocking permits bound extra
// workers across nested builds and concurrent simulation results alike.
var resultBuildWorkerPermits = make(chan struct{}, maxResultBuildParallelism-1)

type resultBuildExecutor struct {
	permits chan struct{}
}

type resultBuildTask struct {
	done       chan struct{}
	returned   bool
	panicked   bool
	panicValue any
}

func newResultBuildExecutor(parallelism int) *resultBuildExecutor {
	parallelism = min(parallelism, maxResultBuildParallelism, runtime.GOMAXPROCS(0))
	return &resultBuildExecutor{permits: make(chan struct{}, max(0, parallelism-1))}
}

// start never waits for capacity. The caller owns the inline fallback when it
// returns nil, and must join every accepted task even while unwinding a panic.
func (executor *resultBuildExecutor) start(work func()) *resultBuildTask {
	if executor == nil {
		return nil
	}
	select {
	case executor.permits <- struct{}{}:
	default:
		return nil
	}
	select {
	case resultBuildWorkerPermits <- struct{}{}:
	default:
		<-executor.permits
		return nil
	}
	task := &resultBuildTask{done: make(chan struct{})}
	go func() {
		defer func() {
			<-resultBuildWorkerPermits
			<-executor.permits
			close(task.done)
		}()
		workReturned := false
		func() {
			defer func() {
				if !workReturned {
					task.panicValue = recover()
				}
			}()
			work()
			workReturned = true
		}()
		// A recovered panic, including panic(nil), returns through this point.
		// Goexit unwinds the whole goroutine instead and leaves returned false.
		task.panicked = !workReturned
		task.returned = true
	}()
	return task
}

func (task *resultBuildTask) wait() {
	if task == nil {
		return
	}
	<-task.done
	if task.panicked {
		panic(task.panicValue)
	}
	if !task.returned {
		runtime.Goexit()
	}
}

// forEach gives index zero to the caller. Accepted workers write only their
// original index slots; the caller executes unsubmitted indexes in order.
func (executor *resultBuildExecutor) forEach(count int, work func(int)) {
	if count <= 0 {
		return
	}
	if executor == nil || cap(executor.permits) == 0 {
		for index := range count {
			work(index)
		}
		return
	}
	var workers []*resultBuildTask
	var inline []int
	callerReturned := false
	defer func() {
		for _, task := range workers {
			<-task.done
		}
		// Do not recover the caller: its panic(nil) or Goexit has precedence
		// over worker failures, after every worker has released its ownership.
		if callerReturned {
			for _, task := range workers {
				task.wait()
			}
		}
	}()
	for index := 1; index < count; index++ {
		if task := executor.start(func() { work(index) }); task != nil {
			workers = append(workers, task)
		} else {
			inline = append(inline, index)
		}
	}
	work(0)
	for _, index := range inline {
		work(index)
	}
	callerReturned = true
}
