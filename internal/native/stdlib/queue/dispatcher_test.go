package queue

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// drainIDs pops every currently-due job and returns their IDs in pop order.
func drainIDs(t *testing.T, d *Dispatcher) []string {
	t.Helper()
	var got []string
	for {
		j, _, ok := d.NextDue(time.Now())
		if !ok {
			return got
		}
		got = append(got, j.ID)
		d.Complete(j, false, 0, 0)
	}
}

func mustSubmit(t *testing.T, d *Dispatcher, j *Job) {
	t.Helper()
	if err := d.Submit(j); err != nil {
		t.Fatalf("Submit(%s): %v", j.ID, err)
	}
}

// TestPriorityThenFIFO is the regression guard for the seq tiebreak. The old
// heap broke priority ties on Created (a time.Time), and time.Now()'s
// resolution is coarse enough (~1-15ms on Windows) that a burst of same-tick
// submissions ordered arbitrarily. Equal-priority jobs must come out in
// submission order, every time.
func TestPriorityThenFIFO(t *testing.T) {
	d := NewDispatcher(0)
	mustSubmit(t, d, &Job{ID: "lo-1", Priority: 0})
	mustSubmit(t, d, &Job{ID: "hi-1", Priority: 10})
	mustSubmit(t, d, &Job{ID: "lo-2", Priority: 0})
	mustSubmit(t, d, &Job{ID: "hi-2", Priority: 10})
	mustSubmit(t, d, &Job{ID: "mid", Priority: 5})

	want := []string{"hi-1", "hi-2", "mid", "lo-1", "lo-2"}
	got := drainIDs(t, d)
	if len(got) != len(want) {
		t.Fatalf("drained %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pop order = %v, want %v", got, want)
		}
	}
}

// TestDelayedJobDoesNotMaskReadyWork is why delayed jobs live in a heap of
// their own. A high-priority job parked on a delay must not sit at the root of
// the ready heap starving the runnable low-priority work behind it.
func TestDelayedJobDoesNotMaskReadyWork(t *testing.T) {
	d := NewDispatcher(0)
	mustSubmit(t, d, &Job{ID: "delayed-hi", Priority: 100, ReadyAt: time.Now().Add(time.Hour)})
	mustSubmit(t, d, &Job{ID: "ready-lo", Priority: 0})

	j, _, ok := d.NextDue(time.Now())
	if !ok {
		t.Fatal("NextDue returned nothing; the delayed job masked the ready one")
	}
	if j.ID != "ready-lo" {
		t.Fatalf("got %q, want ready-lo", j.ID)
	}
	d.Complete(j, false, 0, 0)

	// Nothing else is due, but a delayed job is pending: the pump must be told
	// to wait, not to quit.
	_, wait, ok := d.NextDue(time.Now())
	if ok {
		t.Fatal("delayed job came due an hour early")
	}
	if wait <= 0 {
		t.Fatalf("wait = %v, want a positive duration so :run parks instead of returning", wait)
	}
}

// TestDelayedJobPromotedWhenDue covers the delayed -> ready promotion.
func TestDelayedJobPromotedWhenDue(t *testing.T) {
	d := NewDispatcher(0)
	start := time.Now()
	mustSubmit(t, d, &Job{ID: "soon", ReadyAt: start.Add(20 * time.Millisecond)})

	if _, _, ok := d.NextDue(start); ok {
		t.Fatal("job ran before its ReadyAt")
	}
	j, _, ok := d.NextDue(start.Add(21 * time.Millisecond))
	if !ok {
		t.Fatal("job never came due")
	}
	if j.ID != "soon" {
		t.Fatalf("got %q, want soon", j.ID)
	}
}

// TestCapacityRejectsAndCounts checks backpressure: a bounded queue refuses
// work with ErrFull rather than growing without limit.
func TestCapacityRejectsAndCounts(t *testing.T) {
	d := NewDispatcher(2)
	mustSubmit(t, d, &Job{ID: "a"})
	mustSubmit(t, d, &Job{ID: "b"})

	err := d.Submit(&Job{ID: "c"})
	if !errors.Is(err, ErrFull) {
		t.Fatalf("Submit over capacity = %v, want ErrFull", err)
	}
	if d.Len() != 2 {
		t.Fatalf("Len = %d, want 2 (the rejected job must not be stored)", d.Len())
	}
	if m := d.Snapshot(); m.Dropped != 1 || m.Enqueued != 2 {
		t.Fatalf("metrics = %+v, want Dropped=1 Enqueued=2", m)
	}
}

// TestRetryRequeuesWithBackoff covers the failure path: a job with retries left
// comes back, and only counts as Failed once the attempts run out.
func TestRetryRequeuesWithBackoff(t *testing.T) {
	d := NewDispatcher(0)
	mustSubmit(t, d, &Job{ID: "flaky", Retries: 2})

	// Attempt 1 and 2 fail -> Retried; the job returns to the queue.
	for i := 1; i <= 2; i++ {
		j, _, ok := d.NextDue(time.Now())
		if !ok {
			t.Fatalf("attempt %d: job was not requeued", i)
		}
		if got := d.Complete(j, true, 0, 0); got != Retried {
			t.Fatalf("attempt %d outcome = %v, want Retried", i, got)
		}
	}

	// Attempt 3 exhausts the retries -> Failed, and it does not come back.
	j, _, ok := d.NextDue(time.Now())
	if !ok {
		t.Fatal("attempt 3: job was not requeued")
	}
	if got := d.Complete(j, true, 0, 0); got != Failed {
		t.Fatalf("attempt 3 outcome = %v, want Failed", got)
	}
	if d.Len() != 0 {
		t.Fatalf("Len = %d, want 0: a job out of retries must not requeue", d.Len())
	}

	m := d.Snapshot()
	if m.Retried != 2 || m.Failed != 1 || m.Processed != 3 || m.Succeeded != 0 {
		t.Fatalf("metrics = %+v, want Retried=2 Failed=1 Processed=3", m)
	}
}

// TestRetryBackoffDelaysRequeue: a backoff must park the retry, not make it
// instantly runnable again (which would spin the pump).
func TestRetryBackoffDelaysRequeue(t *testing.T) {
	d := NewDispatcher(0)
	mustSubmit(t, d, &Job{ID: "slow-retry", Retries: 1, Backoff: time.Hour})

	j, _, _ := d.NextDue(time.Now())
	if got := d.Complete(j, true, 0, 0); got != Retried {
		t.Fatalf("outcome = %v, want Retried", got)
	}
	if _, wait, ok := d.NextDue(time.Now()); ok || wait <= 0 {
		t.Fatalf("retry was immediately runnable (ok=%v wait=%v); backoff was ignored", ok, wait)
	}
}

// TestExpiredJob covers the start deadline.
func TestExpiredJob(t *testing.T) {
	j := &Job{ID: "stale", Timeout: 10 * time.Millisecond, ReadyAt: time.Now()}
	if j.Expired(time.Now()) {
		t.Fatal("job expired immediately")
	}
	if !j.Expired(time.Now().Add(50 * time.Millisecond)) {
		t.Fatal("job past its deadline did not expire")
	}

	// A job with no timeout never expires.
	forever := &Job{ID: "patient", ReadyAt: time.Now()}
	if forever.Expired(time.Now().Add(24 * time.Hour)) {
		t.Fatal("a job with no timeout must never expire")
	}
}

// TestStopRejectsAndDrainsNothing: after Stop, Submit fails and NextDue reports
// "nothing pending" so a parked :run returns instead of hanging.
func TestStopRejectsAndDrainsNothing(t *testing.T) {
	d := NewDispatcher(0)
	mustSubmit(t, d, &Job{ID: "a"})
	d.Stop()

	if err := d.Submit(&Job{ID: "b"}); !errors.Is(err, ErrStopped) {
		t.Fatalf("Submit after Stop = %v, want ErrStopped", err)
	}
	if _, wait, ok := d.NextDue(time.Now()); ok || wait != 0 {
		t.Fatalf("NextDue after Stop = (ok=%v wait=%v), want (false, 0) so the pump returns", ok, wait)
	}
	// Stop is idempotent — the old Shutdown closed a channel and panicked on
	// the second call.
	d.Stop()
}

// TestStopIsSafeFromAnyGoroutine exercises Submit/Stop/NextDue concurrently.
// The design this replaces closed the job channel inside Shutdown, so a Submit
// racing a Shutdown panicked with "send on closed channel". Run with -race.
func TestStopIsSafeFromAnyGoroutine(t *testing.T) {
	d := NewDispatcher(0)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				// Submit must return ErrStopped, never panic.
				_ = d.Submit(&Job{ID: "x"})
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			d.NextDue(time.Now())
		}
	}()

	d.Stop()
	wg.Wait()
}

// TestClearDropsPending covers :clear().
func TestClearDropsPending(t *testing.T) {
	d := NewDispatcher(0)
	mustSubmit(t, d, &Job{ID: "a"})
	mustSubmit(t, d, &Job{ID: "b", ReadyAt: time.Now().Add(time.Hour)})

	if n := d.Clear(); n != 2 {
		t.Fatalf("Clear = %d, want 2 (both the ready and the delayed job)", n)
	}
	if d.Len() != 0 {
		t.Fatalf("Len after Clear = %d, want 0", d.Len())
	}
	if m := d.Snapshot(); m.Dropped != 2 {
		t.Fatalf("Dropped = %d, want 2", m.Dropped)
	}
}

// TestMetricsAverages covers the divide-by-zero guard and the averages.
func TestMetricsAverages(t *testing.T) {
	d := NewDispatcher(0)
	if m := d.Snapshot(); m.AvgWait() != 0 || m.AvgExec() != 0 {
		t.Fatal("averages on an empty dispatcher must be 0, not a division by zero")
	}

	mustSubmit(t, d, &Job{ID: "a"})
	j, _, _ := d.NextDue(time.Now())
	d.Complete(j, false, 10*time.Millisecond, 4*time.Millisecond)
	mustSubmit(t, d, &Job{ID: "b"})
	j, _, _ = d.NextDue(time.Now())
	d.Complete(j, false, 30*time.Millisecond, 8*time.Millisecond)

	m := d.Snapshot()
	if m.AvgWait() != 20*time.Millisecond {
		t.Errorf("AvgWait = %v, want 20ms", m.AvgWait())
	}
	if m.AvgExec() != 6*time.Millisecond {
		t.Errorf("AvgExec = %v, want 6ms", m.AvgExec())
	}
	if m.MaxWait != 30*time.Millisecond || m.MaxExec != 8*time.Millisecond {
		t.Errorf("Max = (%v, %v), want (30ms, 8ms)", m.MaxWait, m.MaxExec)
	}
}
