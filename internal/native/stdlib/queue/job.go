package queue

import (
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

// Job is one unit of work held by a Dispatcher. It is pure data: nothing in
// this file or in dispatcher.go ever *runs* a job. Execution happens in
// queue.go's pump, on the VM goroutine — see the package doc for why.
type Job struct {
	ID       string
	Priority int64
	Fn       vm.Value   // callable: *vm.Closure or *vm.GoFunc
	Args     []vm.Value // passed to Fn on every attempt
	Payload  vm.Value   // opaque; carried along for the script's benefit

	Enqueued time.Time     // when the job first entered the queue
	ReadyAt  time.Time     // job is not eligible to run before this instant
	Timeout  time.Duration // 0 = none; see Expired
	Attempts int           // completed attempts so far
	Retries  int           // how many *re*-attempts a failure may trigger
	Backoff  time.Duration // delay before a retry becomes ready

	// seq is a monotonic per-dispatcher counter used to break priority ties
	// in FIFO order. The obvious alternative — comparing Enqueued — is wrong:
	// time.Now()'s resolution is coarse (~1-15ms on Windows), so a burst of
	// submissions lands on identical timestamps and their relative order
	// becomes arbitrary. seq is exact by construction.
	seq uint64

	// index is the job's position in whichever heap currently holds it,
	// maintained by that heap's Swap/Push/Pop. A job is in at most one heap
	// at a time, so one field serves both.
	index int
}

// Expired reports whether the job's start deadline has passed.
//
// Timeout is a deadline on *starting*, not on running: a job that has sat in
// the queue past its deadline is dropped unrun. It deliberately does NOT
// abort a job already in progress — the VM is single-threaded and has no
// interrupt hooks, so there is no way to preempt a Lua function mid-call. A
// "cancel" that let the job keep running (and keep mutating VM state) while
// the caller was told it had been cancelled would be a lie, so the module
// enforces only the half of the contract it can actually honour: shedding
// stale work before it starts.
func (j *Job) Expired(now time.Time) bool {
	if j.Timeout <= 0 {
		return false
	}
	return now.After(j.ReadyAt.Add(j.Timeout))
}

// readyHeap orders runnable jobs by (priority desc, seq asc): a max-heap on
// priority, FIFO within a priority class. Implements heap.Interface.
type readyHeap []*Job

func (h readyHeap) Len() int { return len(h) }

func (h readyHeap) Less(i, j int) bool {
	if h[i].Priority != h[j].Priority {
		return h[i].Priority > h[j].Priority
	}
	return h[i].seq < h[j].seq
}

func (h readyHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *readyHeap) Push(x any) {
	j := x.(*Job)
	j.index = len(*h)
	*h = append(*h, j)
}

func (h *readyHeap) Pop() any {
	old := *h
	n := len(old)
	j := old[n-1]
	old[n-1] = nil // let the job be collected once the caller drops it
	j.index = -1
	*h = old[:n-1]
	return j
}

// delayHeap orders not-yet-eligible jobs by ReadyAt ascending, so the
// dispatcher can find the next job to come due in O(1) and promote it in
// O(log n).
//
// Delayed jobs need a heap of their own rather than a ReadyAt term in
// readyHeap's Less: with one heap, a delayed high-priority job would sit at
// the root and mask every runnable low-priority job behind it, so a pump that
// only ever looks at the root would stall until the delay elapsed.
type delayHeap []*Job

func (h delayHeap) Len() int { return len(h) }

func (h delayHeap) Less(i, j int) bool {
	if !h[i].ReadyAt.Equal(h[j].ReadyAt) {
		return h[i].ReadyAt.Before(h[j].ReadyAt)
	}
	return h[i].seq < h[j].seq
}

func (h delayHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *delayHeap) Push(x any) {
	j := x.(*Job)
	j.index = len(*h)
	*h = append(*h, j)
}

func (h *delayHeap) Pop() any {
	old := *h
	n := len(old)
	j := old[n-1]
	old[n-1] = nil
	j.index = -1
	*h = old[:n-1]
	return j
}
