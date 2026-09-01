package queue

import (
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

type Job struct {
	ID       string
	Priority int64
	Fn       vm.Value
	Args     []vm.Value
	Payload  vm.Value

	Enqueued time.Time
	ReadyAt  time.Time
	Timeout  time.Duration
	Attempts int
	Retries  int
	Backoff  time.Duration

	seq uint64

	index int
}

func (j *Job) Expired(now time.Time) bool {
	if j.Timeout <= 0 {
		return false
	}
	return now.After(j.ReadyAt.Add(j.Timeout))
}

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
	old[n-1] = nil
	j.index = -1
	*h = old[:n-1]
	return j
}

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
