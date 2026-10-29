package simulation

import (
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func resultBuildAwait[T any](t *testing.T, channel <-chan T) T {
	t.Helper()
	select {
	case value := <-channel:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("result build did not make progress")
		var zero T
		return zero
	}
}

func resultBuildCapturePanic(work func()) (returned bool, value any) {
	defer func() { value = recover() }()
	work()
	return true, nil
}

func TestResultBuildExecutorBoundedWork(t *testing.T) {
	previous := runtime.GOMAXPROCS(4)
	t.Cleanup(func() { runtime.GOMAXPROCS(previous) })
	t.Run("overlap and original slots", func(t *testing.T) {
		executor := newResultBuildExecutor(100)
		gate := make(chan struct{})
		var releaseOnce sync.Once
		release := func() { releaseOnce.Do(func() { close(gate) }) }
		defer release()
		entered, done := make(chan int, 7), make(chan struct{})
		var active, maximum atomic.Int32
		counts := make([]atomic.Int32, 7)
		values := make([]int, len(counts))
		go func() {
			defer close(done)
			executor.forEach(len(counts), func(index int) {
				current := active.Add(1)
				defer active.Add(-1)
				for prior := maximum.Load(); current > prior && !maximum.CompareAndSwap(prior, current); prior = maximum.Load() {
				}
				entered <- index
				<-gate
				counts[index].Add(1)
				values[index] = index * index
			})
		}()
		first := map[int]bool{}
		for range 4 {
			first[resultBuildAwait(t, entered)] = true
		}
		if !first[0] || len(first) != 4 || maximum.Load() != 4 || executor.start(func() {}) != nil {
			t.Fatal("caller and three workers did not overlap within the executor limit", first, maximum.Load())
		}
		release()
		resultBuildAwait(t, done)
		for index, value := range values {
			if counts[index].Load() != 1 || value != index*index {
				t.Fatalf("index %d executed %d times with value %d", index, counts[index].Load(), value)
			}
		}
		if maximum.Load() > 4 {
			t.Fatal("result build exceeded four participants")
		}
	})
	t.Run("global budget and failed acquisition releases local permit", func(t *testing.T) {
		first, second := newResultBuildExecutor(4), newResultBuildExecutor(4)
		gate := make(chan struct{})
		var releaseOnce sync.Once
		release := func() { releaseOnce.Do(func() { close(gate) }) }
		defer release()
		var tasks []*resultBuildTask
		for _, executor := range []*resultBuildExecutor{first, first, second} {
			task := executor.start(func() { <-gate })
			if task == nil {
				t.Fatal("available process worker permit was rejected")
			}
			tasks = append(tasks, task)
		}
		if second.start(func() {}) != nil || len(second.permits) != 1 {
			t.Fatal("process worker budget exceeded or rejected task retained local permit")
		}
		release()
		for _, task := range tasks {
			task.wait()
		}
		if task := second.start(func() {}); task == nil {
			t.Fatal("completed tasks did not return permits")
		} else {
			task.wait()
		}
	})
	t.Run("nested saturation stays live", func(t *testing.T) {
		executor := newResultBuildExecutor(4)
		gate, done := make(chan struct{}), make(chan struct{})
		var entered, completed atomic.Int32
		go func() {
			defer close(done)
			executor.forEach(4, func(int) {
				if entered.Add(1) == 4 {
					close(gate)
				}
				<-gate
				executor.forEach(5, func(int) { completed.Add(1) })
			})
		}()
		resultBuildAwait(t, done)
		if completed.Load() != 20 {
			t.Fatal("nested inline fallback lost work", completed.Load())
		}
	})
	t.Run("nil and one processor stay sequential", func(t *testing.T) {
		prior := runtime.GOMAXPROCS(1)
		defer runtime.GOMAXPROCS(prior)
		for _, executor := range []*resultBuildExecutor{nil, newResultBuildExecutor(4), newResultBuildExecutor(0)} {
			if executor.start(func() {}) != nil {
				t.Fatal("sequential executor launched a worker")
			}
			var indexes []int
			executor.forEach(4, func(index int) { indexes = append(indexes, index) })
			if !reflect.DeepEqual(indexes, []int{0, 1, 2, 3}) {
				t.Fatal("sequential execution order changed", indexes)
			}
		}
	})
}

func TestResultBuildExecutorPanicsJoinAndPropagate(t *testing.T) {
	previous := runtime.GOMAXPROCS(4)
	t.Cleanup(func() { runtime.GOMAXPROCS(previous) })
	for _, callerPanics := range []bool{false, true} {
		name := "worker panic joins all tasks"
		if callerPanics {
			name = "caller panic takes precedence after joining"
		}
		t.Run(name, func(t *testing.T) {
			executor := newResultBuildExecutor(3)
			gate := make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(gate) }) }
			defer release()
			started, caught := make(chan int, 3), make(chan any, 1)
			var finished atomic.Bool
			callerPanic, workerPanic := new(int), new(int)
			go func() {
				_, value := resultBuildCapturePanic(func() {
					executor.forEach(3, func(index int) {
						started <- index
						switch index {
						case 0:
							if callerPanics {
								panic(callerPanic)
							}
						case 1:
							panic(workerPanic)
						case 2:
							<-gate
							finished.Store(true)
						}
					})
				})
				caught <- value
			}()
			for range 3 {
				resultBuildAwait(t, started)
			}
			select {
			case value := <-caught:
				t.Fatal("panic escaped before every task joined", value)
			default:
			}
			release()
			want := workerPanic
			if callerPanics {
				want = callerPanic
			}
			if value := resultBuildAwait(t, caught); value != want || !finished.Load() {
				t.Fatal("wrong panic or unfinished worker", value, want, finished.Load())
			}
		})
	}
	t.Run("nil panic is not mistaken for Goexit", func(t *testing.T) {
		task := newResultBuildExecutor(2).start(func() { panic(nil) })
		returned, _ := resultBuildCapturePanic(task.wait)
		if returned || !task.panicked || !task.returned {
			t.Fatal("nil panic was lost or treated as Goexit")
		}
	})
	t.Run("worker Goexit reaches waiter", func(t *testing.T) {
		task := newResultBuildExecutor(2).start(runtime.Goexit)
		done := make(chan struct{})
		var returned atomic.Bool
		go func() {
			defer close(done)
			task.wait()
			returned.Store(true)
		}()
		resultBuildAwait(t, done)
		if returned.Load() || task.returned || task.panicked {
			t.Fatal("worker Goexit was swallowed or converted to panic")
		}
	})
	t.Run("caller Goexit joins workers", func(t *testing.T) {
		gate, entered, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
		var releaseOnce sync.Once
		release := func() { releaseOnce.Do(func() { close(gate) }) }
		defer release()
		var finished, returned atomic.Bool
		go func() {
			defer close(done)
			newResultBuildExecutor(2).forEach(2, func(index int) {
				if index == 0 {
					runtime.Goexit()
				}
				close(entered)
				<-gate
				finished.Store(true)
			})
			returned.Store(true)
		}()
		resultBuildAwait(t, entered)
		select {
		case <-done:
			t.Fatal("caller Goexit abandoned a worker")
		default:
		}
		release()
		resultBuildAwait(t, done)
		if returned.Load() || !finished.Load() {
			t.Fatal("caller Goexit was swallowed or worker not joined")
		}
	})
}
