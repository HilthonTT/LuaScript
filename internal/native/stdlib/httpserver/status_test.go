package httpserver

import (
	"net/http"
	"testing"

	"github.com/hilthontt/luascript/v3/internal/vm"
)

// A status net/http rejects made WriteHeader panic inside the connection
// goroutine: the client got an empty reply and the log a stack trace.
func TestNormalizeRejectsInvalidStatus(t *testing.T) {
	for _, status := range []int64{0, 99, 1000, -1} {
		tbl := vm.NewTable(0, 1)
		tbl.Set("status", status)
		got := normalize(tbl)
		if got.status != http.StatusInternalServerError {
			t.Errorf("status %d normalized to %d, want 500", status, got.status)
		}
	}
	tbl := vm.NewTable(0, 1)
	tbl.Set("status", int64(204))
	if got := normalize(tbl); got.status != 204 {
		t.Errorf("status 204 normalized to %d", got.status)
	}
}
