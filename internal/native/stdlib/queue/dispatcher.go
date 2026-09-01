package queue

import (
	"container/heap"
	"errors"
	"sync"
	"time"
)

var (
	ErrFull    = errors.New("queue is full")
	ErrStopped = errors.New("queue is stopped")
)

type Metrics struct {
	Enqueued  int64
	Processed int64
	Succeeded int64
	Failed    int64
	Retried   int64
	Expired   int64
	Dropped   int64

	TotalWait time.Duration
	TotalExec time.Duration
	MaxWait   time.Duration
	MaxExec   time.Duration
}

func (m Metrics) AvgWait() time.Duration {
	if m.Processed == 0 {
		return 0
	}
	return m.TotalWait / time.Duration(m.Processed)
}

func (m Metrics) AvgExec() time.Duration {
	if m.Processed == 0 {
		return 0
	}
	return m.TotalExec / time.Duration(m.Processed)
}

type Outcome int

const (
	Succeeded Outcome = iota
	Failed
	Retried
)

type Dispatcher struct {
	mu      sync.Mutex
	ready   readyHeap
	delayed delayHeap
	seq     uint64
	stopped bool
	metrics Metrics

	capacity int

	wake chan struct{}
}

func NewDispatcher(capacity int) *Dispatcher {
	if capacity < 0 {
		capacity = 0
	}
	return &Dispatcher{
		capacity: capacity,
		wake:     make(chan struct{}, 1),
	}
}

func (d *Dispatcher) Submit(j *Job) error {
	if j == nil {
		return errors.New("nil job")
	}
	now := time.Now()

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return ErrStopped
	}
	if d.capacity > 0 && len(d.ready)+len(d.delayed) >= d.capacity {
		d.metrics.Dropped++
		return ErrFull
	}

	if j.Enqueued.IsZero() {
		j.Enqueued = now
	}
	if j.ReadyAt.IsZero() {
		j.ReadyAt = now
	}
	d.push(j)
	d.metrics.Enqueued++
	d.signal()
	return nil
}

func (d *Dispatcher) push(j *Job) {
	d.seq++
	j.seq = d.seq
	if j.ReadyAt.After(time.Now()) {
		heap.Push(&d.delayed, j)
		return
	}
	heap.Push(&d.ready, j)
}

func (d *Dispatcher) signal() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *Dispatcher) Wake() <-chan struct{} { return d.wake }

func (d *Dispatcher) NextDue(now time.Time) (j *Job, wait time.Duration, ok bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return nil, 0, false
	}

	for len(d.delayed) > 0 && !d.delayed[0].ReadyAt.After(now) {
		heap.Push(&d.ready, heap.Pop(&d.delayed).(*Job))
	}

	if len(d.ready) > 0 {
		return heap.Pop(&d.ready).(*Job), 0, true
	}
	if len(d.delayed) > 0 {
		return nil, d.delayed[0].ReadyAt.Sub(now), false
	}
	return nil, 0, false
}

func (d *Dispatcher) Complete(j *Job, failed bool, wait, exec time.Duration) Outcome {
	d.mu.Lock()
	defer d.mu.Unlock()

	j.Attempts++
	d.metrics.Processed++
	d.metrics.TotalWait += wait
	d.metrics.TotalExec += exec
	if wait > d.metrics.MaxWait {
		d.metrics.MaxWait = wait
	}
	if exec > d.metrics.MaxExec {
		d.metrics.MaxExec = exec
	}

	if !failed {
		d.metrics.Succeeded++
		return Succeeded
	}

	if j.Attempts <= j.Retries && !d.stopped {
		d.metrics.Retried++
		j.ReadyAt = time.Now().Add(j.Backoff)
		d.push(j)
		d.signal()
		return Retried
	}

	d.metrics.Failed++
	return Failed
}

func (d *Dispatcher) MarkExpired() {
	d.mu.Lock()
	d.metrics.Expired++
	d.mu.Unlock()
}

func (d *Dispatcher) Stop() {
	d.mu.Lock()
	d.stopped = true
	d.signal()
	d.mu.Unlock()
}

func (d *Dispatcher) Stopped() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stopped
}

func (d *Dispatcher) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.ready) + len(d.delayed)
}

func (d *Dispatcher) Clear() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	n := len(d.ready) + len(d.delayed)
	d.ready = nil
	d.delayed = nil
	d.metrics.Dropped += int64(n)
	return n
}

func (d *Dispatcher) Snapshot() Metrics {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.metrics
}
