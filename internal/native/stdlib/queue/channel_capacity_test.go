package queue_test

import (
	"strings"
	"testing"
)

// queue.channel preallocates its buffer; a capacity of 1e12 used to be a
// fatal out-of-memory (the process died, pcall could not catch it).
func TestChannelRejectsHugeCapacity(t *testing.T) {
	msg := runQueueErr(t, `
		local queue = require("queue")
		queue.channel(1000000000000)
	`)
	if !strings.Contains(msg, "capacity must be at most") {
		t.Fatalf("got %q, want a capacity error", msg)
	}
}
