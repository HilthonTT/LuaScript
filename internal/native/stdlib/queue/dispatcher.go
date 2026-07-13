package queue

import (
	"container/heap"
	"errors"
	"sync"
	"time"
)

// Errors returned by Submit.
var (
	ErrFull    = errors.New("queue is full")
	ErrStopped = errors.New("queue is stopped")
)

// Metrics is a snapshot of a dispatcher's counters.
//
// It carries no mutex: the dispatcher's own lock guards the live copy, and
// Snapshot returns this struct by value. (Embedding a sync.Mutex here and
// returning *d.metrics — the shape this file used to have — copies the lock,
// which `go vet`'s copylocks check rejects and which silently hands callers a
// mutex in whatever state it happened to be in.)
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

// AvgWait returns the mean time a processed job spent waiting to start.
func (m Metrics) AvgWait() time.Duration {
	if m.Processed == 0 {
		return 0
	}
	return m.TotalWait / time.Duration(m.Processed)
}

// AvgExec returns the mean time a processed job spent running.
func (m Metrics) AvgExec() time.Duration {
	if m.Processed == 0 {
		return 0
	}
	return m.TotalExec / time.Duration(m.Processed)
}

// Outcome is what Complete decided about a finished attempt.
type Outcome int

const (
	Succeeded Outcome = iota
	Failed
	Retried // failed, but re-queued for another attempt
)

// Dispatcher is a priority + delay scheduler. It is safe to submit to from any
// goroutine; it never runs a job itself. Callers pull work with NextDue and
// report the result with Complete.
//
// Jobs are ordered by priority (higher first), FIFO within a priority. A job
// with a delay is parked in a separate heap until it comes due.
type Dispatcher struct {
	mu      sync.Mutex
	ready   readyHeap
	delayed delayHeap
	seq     uint64
	stopped bool
	metrics Metrics

	// capacity caps ready+delayed. 0 means unbounded.
	capacity int

	// wake carries a single coalesced "state changed" signal to a pump parked
	// in NextDue's wait. Buffered (cap 1) and only ever sent to under a
	// non-blocking select, so no producer can ever block on it — and it is
	// never closed, so a Submit racing a Stop cannot panic.
	wake chan struct{}
}

// NewDispatcher builds an empty dispatcher. capacity bounds the number of
// pending jobs (ready + delayed); 0 leaves it unbounded.
func NewDispatcher(capacity int) *Dispatcher {
	if capacity < 0 {
		capacity = 0
	}
	return &Dispatcher{
		capacity: capacity,
		wake:     make(chan struct{}, 1),
	}
}

// Submit queues a job. It returns ErrStopped if the dispatcher has been
// stopped, or ErrFull if capacity is exhausted.
//
// Unlike the channel-fed design this replaces, Submit pushes straight onto the
// heap under the lock. There is no receiver goroutine and no channel to close,
// which removes both the send-on-a-closed-channel panic that a Submit racing a
// Shutdown used to hit and the unbounded backlog that could build up behind a
// nominally "full" buffer.
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

// push files a job into the ready or delayed heap. Caller holds d.mu.
func (d *Dispatcher) push(j *Job) {
	d.seq++
	j.seq = d.seq
	if j.ReadyAt.After(time.Now()) {
		heap.Push(&d.delayed, j)
		return
	}
	heap.Push(&d.ready, j)
}

// signal nudges a parked pump. Non-blocking: if a wake is already pending the
// pump has not consumed it yet, and one wake is all it needs. Caller holds d.mu.
func (d *Dispatcher) signal() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// Wake is the channel a pump parks on while waiting for the next job to come
// due. It fires on Submit and on Stop.
func (d *Dispatcher) Wake() <-chan struct{} { return d.wake }

// NextDue returns the highest-priority job eligible to run at `now`.
//
// When no job is eligible, ok is false and `wait` says what the caller should
// do: a positive duration is how long until the earliest delayed job comes due
// (park that long, or until Wake fires); zero means there is nothing pending
// at all — the queue is drained, or stopped.
func (d *Dispatcher) NextDue(now time.Time) (j *Job, wait time.Duration, ok bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return nil, 0, false
	}

	// Promote every delayed job that has come due. They re-enter the ready
	// heap keeping their original seq, so a delayed job that has been waiting
	// stays ahead of same-priority jobs submitted after it came due.
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

// Complete records the result of an attempt and decides whether to retry.
//
// `wait` is how long the job sat before starting and `exec` how long it ran;
// both feed the metrics. A failed job with attempts left is pushed back with
// its backoff applied and reported as Retried.
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

	// A retry is only worth queueing if the dispatcher is still live —
	// otherwise it would sit in the heap forever, and Submit's own ErrStopped
	// guard would have rejected it anyway.
	if j.Attempts <= j.Retries && !d.stopped {
		d.metrics.Retried++
		j.ReadyAt = time.Now().Add(j.Backoff)
		d.push(j) // fresh seq: a retry goes to the back of its priority class
		d.signal()
		return Retried
	}

	d.metrics.Failed++
	return Failed
}

// MarkExpired records a job dropped for missing its start deadline.
func (d *Dispatcher) MarkExpired() {
	d.mu.Lock()
	d.metrics.Expired++
	d.mu.Unlock()
}

// Stop makes the dispatcher reject new work and tells any parked pump to
// return. Pending jobs are left in place (Clear drops them); Stop is
// idempotent and safe to call from a running job.
func (d *Dispatcher) Stop() {
	d.mu.Lock()
	d.stopped = true
	d.signal()
	d.mu.Unlock()
}

// Stopped reports whether Stop has been called.
func (d *Dispatcher) Stopped() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stopped
}

// Len returns the number of pending jobs (ready + delayed).
func (d *Dispatcher) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.ready) + len(d.delayed)
}

// Clear drops every pending job and returns how many were dropped.
func (d *Dispatcher) Clear() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	n := len(d.ready) + len(d.delayed)
	d.ready = nil
	d.delayed = nil
	d.metrics.Dropped += int64(n)
	return n
}

// Snapshot returns a copy of the current metrics.
func (d *Dispatcher) Snapshot() Metrics {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.metrics
}
