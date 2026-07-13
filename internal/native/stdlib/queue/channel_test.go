package queue

import (
	"sync"
	"testing"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

func TestChannelSendReceive(t *testing.T) {
	c := NewChannel(2)
	if r := c.Send("a", 0); r != OK {
		t.Fatalf("Send = %v, want OK", r)
	}
	if c.Len() != 1 || c.Cap() != 2 {
		t.Fatalf("len/cap = %d/%d, want 1/2", c.Len(), c.Cap())
	}
	v, r := c.Receive(0)
	if r != OK || v != "a" {
		t.Fatalf("Receive = (%v, %v), want (a, OK)", v, r)
	}
}

func TestChannelTrySendFull(t *testing.T) {
	c := NewChannel(1)
	c.Send("a", 0)
	if r := c.Send("b", 0); r != Timeout {
		t.Fatalf("try-send on a full channel = %v, want Timeout (reported as \"full\" to Lua)", r)
	}
}

func TestChannelTryReceiveEmpty(t *testing.T) {
	c := NewChannel(1)
	if _, r := c.Receive(0); r != Timeout {
		t.Fatalf("try-receive on an empty channel = %v, want Timeout", r)
	}
}

// TestChannelClosedDrainsBufferFirst is the core close-semantics guarantee:
// values already in the buffer when Close lands must still be receivable, and
// only once they run out does the receiver see Closed. Selecting over the data
// channel and the done channel together would let Go pick either ready case at
// random and lose buffered values.
func TestChannelClosedDrainsBufferFirst(t *testing.T) {
	c := NewChannel(4)
	c.Send("a", 0)
	c.Send("b", 0)
	c.Close()

	for _, want := range []string{"a", "b"} {
		v, r := c.Receive(-1)
		if r != OK || v != want {
			t.Fatalf("Receive = (%v, %v), want (%s, OK) — a closed channel must drain first", v, r, want)
		}
	}
	if _, r := c.Receive(-1); r != Closed {
		t.Fatalf("Receive on a drained closed channel = %v, want Closed", r)
	}
}

// TestChannelSendOnClosed: a send after close reports Closed. Crucially it does
// NOT panic — the data channel is never closed, so there is no
// "send on closed channel" window for a racing sender to fall into.
func TestChannelSendOnClosed(t *testing.T) {
	c := NewChannel(1)
	c.Close()
	if r := c.Send("a", -1); r != Closed {
		t.Fatalf("Send after Close = %v, want Closed", r)
	}
	if r := c.Send("a", 0); r != Closed {
		t.Fatalf("try-send after Close = %v, want Closed (not Timeout — retrying can never succeed)", r)
	}
}

func TestChannelDoubleCloseIsNoop(t *testing.T) {
	c := NewChannel(0)
	c.Close()
	c.Close() // sync.Once: must not panic
	if !c.IsClosed() {
		t.Fatal("IsClosed = false after Close")
	}
}

// TestChannelCloseUnblocksReceiver: a receiver parked forever must wake on
// Close rather than hang.
func TestChannelCloseUnblocksReceiver(t *testing.T) {
	c := NewChannel(0)
	done := make(chan Result, 1)
	go func() {
		_, r := c.Receive(-1)
		done <- r
	}()

	time.Sleep(10 * time.Millisecond) // let the receiver park
	c.Close()

	select {
	case r := <-done:
		if r != Closed {
			t.Fatalf("parked receiver woke with %v, want Closed", r)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not wake the parked receiver")
	}
}

func TestChannelReceiveTimeout(t *testing.T) {
	c := NewChannel(0)
	start := time.Now()
	if _, r := c.Receive(30 * time.Millisecond); r != Timeout {
		t.Fatalf("Receive = %v, want Timeout", r)
	}
	if elapsed := time.Since(start); elapsed < 25*time.Millisecond {
		t.Fatalf("Receive returned after %v, want to have waited ~30ms", elapsed)
	}
}

func TestChannelSendTimeout(t *testing.T) {
	c := NewChannel(1)
	c.Send("a", 0)
	if r := c.Send("b", 20*time.Millisecond); r != Timeout {
		t.Fatalf("Send on a full channel = %v, want Timeout", r)
	}
}

// TestChannelUnbufferedHandoff: an unbuffered channel is a rendezvous.
func TestChannelUnbufferedHandoff(t *testing.T) {
	c := NewChannel(0)
	go func() { c.Send(int64(42), -1) }()

	v, r := c.Receive(time.Second)
	if r != OK || v != int64(42) {
		t.Fatalf("Receive = (%v, %v), want (42, OK)", v, r)
	}
}

// TestChannelConcurrentSendersAndClose hammers the send/close race that a
// naive close(c.ch) implementation would panic on. Run with -race.
func TestChannelConcurrentSendersAndClose(t *testing.T) {
	c := NewChannel(8)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Send(int64(j), 0) // must return Closed, never panic
			}
		}()
	}
	// Drain concurrently so senders make progress.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 400; i++ {
			c.Receive(0)
		}
	}()

	time.Sleep(2 * time.Millisecond)
	c.Close()
	wg.Wait()
}

// TestChannelCarriesLuaValues checks the FFI rule: what goes in comes out
// identically, with integers staying int64.
func TestChannelCarriesLuaValues(t *testing.T) {
	c := NewChannel(4)
	tbl := vm.NewTable(0, 1)
	tbl.Set("k", "v")

	for _, want := range []vm.Value{int64(7), 1.5, "s", true, tbl} {
		c.Send(want, 0)
		got, r := c.Receive(0)
		if r != OK {
			t.Fatalf("Receive(%v) = %v, want OK", want, r)
		}
		if got != want {
			t.Fatalf("round-trip = %#v, want %#v", got, want)
		}
	}
}
