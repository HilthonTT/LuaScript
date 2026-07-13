package queue

import (
	"sync"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

// Channel is a Go channel carrying Lua values, usable from any goroutine.
//
// The underlying data channel is never closed. Closing it would make a Send
// racing a Close panic ("send on closed channel") with no way for the sender
// to defend itself — the check-then-send window is unclosable. Instead Close
// closes a separate `done` channel, and Send/Receive select on it. That keeps
// Go's observable close semantics (senders fail, receivers drain the buffer
// first and only then see the close) without the panic.
type Channel struct {
	ch   chan vm.Value
	done chan struct{}

	once sync.Once // guards Close, so a double close is a no-op, not a panic
	cap  int
}

// Result distinguishes the three ways a blocking channel op can end.
type Result int

const (
	// OK means the value was sent / received.
	OK Result = iota
	// Closed means the channel was closed (and, for a receive, drained).
	Closed
	// Timeout means the deadline elapsed first.
	Timeout
)

func (r Result) String() string {
	switch r {
	case OK:
		return "ok"
	case Closed:
		return "closed"
	default:
		return "timeout"
	}
}

// NewChannel builds a channel with the given buffer capacity. capacity 0 is a
// Go-style unbuffered channel: a send blocks until a receiver takes the value.
func NewChannel(capacity int) *Channel {
	if capacity < 0 {
		capacity = 0
	}
	return &Channel{
		ch:   make(chan vm.Value, capacity),
		done: make(chan struct{}),
		cap:  capacity,
	}
}

// Send blocks until the value is taken (or buffered). A negative timeout waits
// forever; zero makes it a non-blocking try.
//
// A send that genuinely races a concurrent Close may still be delivered — the
// check-then-send window cannot be closed without a lock on the hot path, and
// Go's own answer to that race (panic) is not one a script should have to
// survive. Every non-racing send on a closed channel reports Closed.
func (c *Channel) Send(v vm.Value, timeout time.Duration) Result {
	// Checked before the buffer, not after: a closed channel refuses a value
	// even when it has room for it.
	if c.IsClosed() {
		return Closed
	}

	select {
	case c.ch <- v:
		return OK
	default:
	}

	if timeout == 0 {
		// Distinguish "full" from "closed since we looked": a caller retrying
		// a try_send would otherwise spin forever on a channel that can never
		// accept another value.
		select {
		case <-c.done:
			return Closed
		default:
			return Timeout
		}
	}

	var timer <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timer = t.C
	}

	select {
	case c.ch <- v:
		return OK
	case <-c.done:
		return Closed
	case <-timer:
		return Timeout
	}
}

// Receive blocks until a value arrives. A negative timeout waits forever; zero
// makes it a non-blocking try.
func (c *Channel) Receive(timeout time.Duration) (vm.Value, Result) {
	// Buffered values outrank the close signal — after Close, a receiver must
	// still drain what was already sent, exactly as a real Go channel does.
	// Trying the buffer first (rather than selecting over ch and done
	// together, where Go would pick a ready case at random) is what guarantees
	// that ordering.
	select {
	case v := <-c.ch:
		return v, OK
	default:
	}
	if timeout == 0 {
		select {
		case <-c.done:
			return nil, Closed
		default:
			return nil, Timeout
		}
	}

	var timer <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timer = t.C
	}

	select {
	case v := <-c.ch:
		return v, OK
	case <-c.done:
		// A value may have landed in the buffer between the drain attempt
		// above and Close; hand it over rather than dropping it.
		select {
		case v := <-c.ch:
			return v, OK
		default:
			return nil, Closed
		}
	case <-timer:
		return nil, Timeout
	}
}

// Close closes the channel. Idempotent. Buffered values remain receivable.
func (c *Channel) Close() {
	c.once.Do(func() { close(c.done) })
}

// IsClosed reports whether Close has been called.
func (c *Channel) IsClosed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// Len is the number of buffered values; Cap the buffer size.
func (c *Channel) Len() int { return len(c.ch) }
func (c *Channel) Cap() int { return c.cap }
