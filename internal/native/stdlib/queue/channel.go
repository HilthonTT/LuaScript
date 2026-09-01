package queue

import (
	"sync"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

type Channel struct {
	ch   chan vm.Value
	done chan struct{}

	once sync.Once
	cap  int
}

type Result int

const (
	OK Result = iota
	Closed
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

func (c *Channel) Send(v vm.Value, timeout time.Duration) Result {
	if c.IsClosed() {
		return Closed
	}

	select {
	case c.ch <- v:
		return OK
	default:
	}

	if timeout == 0 {
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

func (c *Channel) Receive(timeout time.Duration) (vm.Value, Result) {
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

func (c *Channel) Close() {
	c.once.Do(func() { close(c.done) })
}

func (c *Channel) IsClosed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *Channel) Len() int { return len(c.ch) }
func (c *Channel) Cap() int { return c.cap }
