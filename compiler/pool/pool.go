package pool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// WorkerPool implements an enterprise-grade goroutine pool with auto-scaling.
type WorkerPool struct {
	minWorkers     int
	maxWorkers     int
	currentWorkers int64
	idleWorkers    int64
	nextWorkerID   int64 // monotonic ID generator for workers created via scaleUp

	tasks        chan Task
	results      chan Result
	workers      map[int]*Worker
	workersMutex sync.RWMutex

	// totalTaskDuration accumulates nanoseconds across all completed tasks
	// so updateStatistics can compute AverageTaskTime without holding a lock
	// on the hot path.
	totalTaskDuration int64

	// statsMutex guards the non-atomic fields of stats (AverageTaskTime,
	// WorkerUtilization, QueueDepth). The counter fields (TasksProcessed,
	// TasksQueued) are updated atomically and don't need the mutex.
	stats      PoolStatistics
	statsMutex sync.RWMutex

	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

type Task struct {
	ID       string
	Function func() any
	Priority int
	Timeout  time.Duration
}

type Result struct {
	TaskID   string
	Data     any
	Error    error
	Duration time.Duration
}

type Worker struct {
	id       int
	pool     *WorkerPool
	lastUsed time.Time
	tasks    chan Task
	quit     chan bool
}

type PoolStatistics struct {
	TasksProcessed    int64
	TasksQueued       int64
	AverageTaskTime   time.Duration
	WorkerUtilization float64
	QueueDepth        int64
}

// NewWorkerPool creates a new worker pool with auto-scaling.
func NewWorkerPool(minWorkers, maxWorkers, queueSize int) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())

	pool := &WorkerPool{
		minWorkers: minWorkers,
		maxWorkers: maxWorkers,
		tasks:      make(chan Task, queueSize),
		results:    make(chan Result, queueSize),
		workers:    make(map[int]*Worker),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Initial workers take IDs 0..minWorkers-1; scaleUp picks up from there.
	for i := range minWorkers {
		pool.startWorker(i)
	}
	atomic.StoreInt64(&pool.nextWorkerID, int64(minWorkers))

	go pool.monitor()

	return pool
}

// Submit submits a task to the worker pool.
func (p *WorkerPool) Submit(task Task) error {
	select {
	case p.tasks <- task:
		atomic.AddInt64(&p.stats.TasksQueued, 1)
		return nil
	case <-p.ctx.Done():
		return p.ctx.Err()
	default:
		// Queue is full — try to scale up, then retry with a short deadline.
		if p.shouldScaleUp() {
			p.scaleUp()
		}

		select {
		case p.tasks <- task:
			atomic.AddInt64(&p.stats.TasksQueued, 1)
			return nil
		case <-p.ctx.Done():
			return p.ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return fmt.Errorf("task queue full, unable to submit task")
		}
	}
}

func (p *WorkerPool) startWorker(id int) {
	worker := &Worker{
		id:       id,
		pool:     p,
		lastUsed: time.Now(),
		tasks:    make(chan Task, 1),
		quit:     make(chan bool, 1), // buffered so scaleDown's send is non-blocking
	}

	p.workersMutex.Lock()
	p.workers[id] = worker
	atomic.AddInt64(&p.currentWorkers, 1)
	p.workersMutex.Unlock()

	go worker.run()
}

func (w *Worker) run() {
	defer func() {
		atomic.AddInt64(&w.pool.currentWorkers, -1)
		w.pool.workersMutex.Lock()
		delete(w.pool.workers, w.id)
		w.pool.workersMutex.Unlock()
	}()

	// Each iteration the worker is "idle" while it waits in select. Every
	// path out of select decrements idleWorkers — the original code only
	// decremented on the task branch, which let the counter grow without
	// bound across timeout iterations and broke the scaling predicates.
	for {
		atomic.AddInt64(&w.pool.idleWorkers, 1)

		select {
		case task := <-w.pool.tasks:
			atomic.AddInt64(&w.pool.idleWorkers, -1)
			w.processTask(task)

		case <-w.quit:
			atomic.AddInt64(&w.pool.idleWorkers, -1)
			return

		case <-w.pool.ctx.Done():
			atomic.AddInt64(&w.pool.idleWorkers, -1)
			return

		case <-time.After(30 * time.Second):
			atomic.AddInt64(&w.pool.idleWorkers, -1)
			if w.pool.shouldScaleDown() {
				return
			}
		}
	}
}

func (w *Worker) processTask(task Task) {
	start := time.Now()
	w.lastUsed = start

	var result Result
	result.TaskID = task.ID

	if task.Timeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), task.Timeout)
		defer cancel()

		done := make(chan struct{})
		go func() {
			defer close(done)
			result.Data = task.Function()
		}()

		select {
		case <-done:
		case <-ctx.Done():
			result.Error = fmt.Errorf("task timeout after %v", task.Timeout)
			// Note: the inner goroutine continues running until task.Function
			// returns on its own. Go can't preempt goroutines, so callers
			// should make Function honor a context if cancellation matters.
		}
	} else {
		result.Data = task.Function()
	}

	result.Duration = time.Since(start)
	atomic.AddInt64(&w.pool.stats.TasksProcessed, 1)
	atomic.AddInt64(&w.pool.totalTaskDuration, int64(result.Duration))

	select {
	case w.pool.results <- result:
	default:
		// Results channel full — drop. Consumers should drain via Results().
	}
}

func (p *WorkerPool) monitor() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.updateStatistics()
			p.autoScale()

		case <-p.ctx.Done():
			return
		}
	}
}

func (p *WorkerPool) shouldScaleUp() bool {
	queueDepth := int64(len(p.tasks))
	currentWorkers := atomic.LoadInt64(&p.currentWorkers)
	idleWorkers := atomic.LoadInt64(&p.idleWorkers)

	return queueDepth > currentWorkers*2 &&
		idleWorkers < currentWorkers/4 &&
		currentWorkers < int64(p.maxWorkers)
}

func (p *WorkerPool) shouldScaleDown() bool {
	currentWorkers := atomic.LoadInt64(&p.currentWorkers)
	idleWorkers := atomic.LoadInt64(&p.idleWorkers)

	return idleWorkers > currentWorkers/2 &&
		currentWorkers > int64(p.minWorkers)
}

// scaleUp adds one worker if we are below maxWorkers. The map mutation and
// currentWorkers increment happen under the same lock so a concurrent
// scaleUp can't race past the maxWorkers check. The worker goroutine is
// started after the lock is released to keep the critical section small.
func (p *WorkerPool) scaleUp() {
	p.workersMutex.Lock()
	if len(p.workers) >= p.maxWorkers {
		p.workersMutex.Unlock()
		return
	}

	id := int(atomic.AddInt64(&p.nextWorkerID, 1) - 1)
	worker := &Worker{
		id:       id,
		pool:     p,
		lastUsed: time.Now(),
		tasks:    make(chan Task, 1),
		quit:     make(chan bool, 1),
	}
	p.workers[id] = worker
	atomic.AddInt64(&p.currentWorkers, 1)
	p.workersMutex.Unlock()

	go worker.run()
}

// scaleDown signals one worker to exit if we are above minWorkers. The
// buffered quit channel makes the send non-blocking even when the target
// worker is busy executing a task — it'll pick up the signal on its next
// trip through the select.
//
// The 30s-idle timeout path in Worker.run() also handles graceful
// shrinkage; this is just an immediate nudge from autoScale().
func (p *WorkerPool) scaleDown() {
	p.workersMutex.RLock()
	if len(p.workers) <= p.minWorkers {
		p.workersMutex.RUnlock()
		return
	}

	// Pick any worker. We deliberately don't do LRU here: Worker.lastUsed
	// is written without synchronization and reading it for comparison
	// across workers would race with processTask.
	var victim *Worker
	for _, w := range p.workers {
		victim = w
		break
	}
	p.workersMutex.RUnlock()

	if victim != nil {
		select {
		case victim.quit <- true:
		default:
			// quit buffer already holds a signal — that worker is on its
			// way out, no further action needed.
		}
	}
}

// updateStatistics refreshes the derived metrics on the stats struct.
// Counter fields stay atomic and always up to date; derived fields are
// recomputed here under statsMutex so Stats() can return a coherent
// snapshot.
func (p *WorkerPool) updateStatistics() {
	processed := atomic.LoadInt64(&p.stats.TasksProcessed)
	totalNs := atomic.LoadInt64(&p.totalTaskDuration)
	current := atomic.LoadInt64(&p.currentWorkers)
	idle := atomic.LoadInt64(&p.idleWorkers)
	queueDepth := int64(len(p.tasks))

	var avg time.Duration
	if processed > 0 {
		avg = time.Duration(totalNs / processed)
	}

	var util float64
	if current > 0 {
		busy := current - idle
		if busy < 0 {
			// idleWorkers can briefly read higher than currentWorkers
			// because they're separate atomics — clamp.
			busy = 0
		}
		util = float64(busy) / float64(current)
	}

	p.statsMutex.Lock()
	p.stats.AverageTaskTime = avg
	p.stats.WorkerUtilization = util
	p.stats.QueueDepth = queueDepth
	p.statsMutex.Unlock()
}

// autoScale runs once per monitor tick. It moves the pool size in at most
// one direction per tick to dampen oscillation around the thresholds.
func (p *WorkerPool) autoScale() {
	switch {
	case p.shouldScaleUp():
		p.scaleUp()
	case p.shouldScaleDown():
		p.scaleDown()
	}
}

// Stats returns a snapshot of pool statistics. Safe to call concurrently
// with Submit and task execution.
func (p *WorkerPool) Stats() PoolStatistics {
	p.statsMutex.RLock()
	defer p.statsMutex.RUnlock()
	return PoolStatistics{
		TasksProcessed:    atomic.LoadInt64(&p.stats.TasksProcessed),
		TasksQueued:       atomic.LoadInt64(&p.stats.TasksQueued),
		AverageTaskTime:   p.stats.AverageTaskTime,
		WorkerUtilization: p.stats.WorkerUtilization,
		QueueDepth:        int64(len(p.tasks)),
	}
}

// Results returns the receive end of the results channel. Consumers must
// drain it; if it fills up, processTask drops new results to avoid
// blocking workers.
func (p *WorkerPool) Results() <-chan Result {
	return p.results
}

// Shutdown cancels the pool context and waits for all workers to exit,
// up to the given timeout. Tasks still sitting in the queue may be
// dropped — workers select randomly between p.tasks and ctx.Done() once
// the context is cancelled. Callers that need to drain should stop
// submitting and wait until Stats().QueueDepth == 0 before calling
// Shutdown.
func (p *WorkerPool) Shutdown(timeout time.Duration) error {
	p.closeOnce.Do(func() {
		p.cancel()
	})

	deadline := time.After(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if atomic.LoadInt64(&p.currentWorkers) == 0 {
			return nil
		}
		select {
		case <-deadline:
			return fmt.Errorf("shutdown timed out with %d workers still running",
				atomic.LoadInt64(&p.currentWorkers))
		case <-ticker.C:
		}
	}
}
