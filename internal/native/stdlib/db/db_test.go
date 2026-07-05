package db_test

import (
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	native_db "github.com/hilthontt/luascript/internal/native/stdlib/db"
	"github.com/hilthontt/luascript/internal/vm"
)

// ---------------------------------------------------------------------------
// Fake sql driver — a minimal in-memory key/value store implemented behind
// the database/sql/driver interfaces. Avoids pulling a real DB dependency
// (sqlite, etc.) into the test suite. The "schema" is one column "key" and
// one column "value"; SQL is parsed as a tiny custom DSL:
//
//   "set k v"   → store/update key k = v
//   "del k"     → remove key k
//   "all"       → return every row sorted by insertion order
//   "get k"     → return zero or one row
//
// Bind parameters are not used by the tests, but the interface implementations
// accept and ignore them so the helper stays generic.
// ---------------------------------------------------------------------------

type fakeDriver struct{}

type fakeConn struct {
	store *fakeStore
}

type fakeStore struct {
	mu     sync.Mutex
	keys   []string
	values map[string]string
}

func newFakeStore() *fakeStore {
	return &fakeStore{values: make(map[string]string)}
}

type fakeStmt struct {
	conn  *fakeConn
	query string
}

type fakeResult struct {
	affected int64
}

func (r fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (r fakeResult) RowsAffected() (int64, error) { return r.affected, nil }

type fakeRows struct {
	cols []string
	rows [][]driver.Value
	idx  int
}

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

// Shared store keyed by dataSourceName so multiple connections in one test
// hit the same data. fakeDriver.Open looks it up here.
var (
	storesMu sync.Mutex
	stores   = map[string]*fakeStore{}
)

func storeFor(name string) *fakeStore {
	storesMu.Lock()
	defer storesMu.Unlock()
	if s, ok := stores[name]; ok {
		return s
	}
	s := newFakeStore()
	stores[name] = s
	return s
}

func (fakeDriver) Open(name string) (driver.Conn, error) {
	return &fakeConn{store: storeFor(name)}, nil
}

func (c *fakeConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeStmt{conn: c, query: query}, nil
}
func (c *fakeConn) Close() error              { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) { return nil, nil }

// Ping is optional but database/sql calls it when handle.Ping is invoked.
func (c *fakeConn) Ping() error { return nil }

func (s *fakeStmt) Close() error  { return nil }
func (s *fakeStmt) NumInput() int { return -1 } // variadic / unknown

func (s *fakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	parts := strings.Fields(s.query)
	switch parts[0] {
	case "set":
		s.conn.store.mu.Lock()
		defer s.conn.store.mu.Unlock()
		if _, ok := s.conn.store.values[parts[1]]; !ok {
			s.conn.store.keys = append(s.conn.store.keys, parts[1])
		}
		s.conn.store.values[parts[1]] = parts[2]
		return fakeResult{affected: 1}, nil
	case "del":
		s.conn.store.mu.Lock()
		defer s.conn.store.mu.Unlock()
		if _, ok := s.conn.store.values[parts[1]]; !ok {
			return fakeResult{affected: 0}, nil
		}
		delete(s.conn.store.values, parts[1])
		// Trim keys slice — O(n) but fine for tests.
		for i, k := range s.conn.store.keys {
			if k == parts[1] {
				s.conn.store.keys = append(s.conn.store.keys[:i], s.conn.store.keys[i+1:]...)
				break
			}
		}
		return fakeResult{affected: 1}, nil
	}
	return fakeResult{}, nil
}

func (s *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	parts := strings.Fields(s.query)
	s.conn.store.mu.Lock()
	defer s.conn.store.mu.Unlock()
	rows := &fakeRows{cols: []string{"key", "value"}}
	switch parts[0] {
	case "all":
		for _, k := range s.conn.store.keys {
			rows.rows = append(rows.rows, []driver.Value{k, s.conn.store.values[k]})
		}
	case "get":
		if v, ok := s.conn.store.values[parts[1]]; ok {
			rows.rows = append(rows.rows, []driver.Value{parts[1], v})
		}
	}
	return rows, nil
}

func init() {
	sql.Register("luascripttest", fakeDriver{})
}

// ---------------------------------------------------------------------------
// VM harness
// ---------------------------------------------------------------------------

func runDB(t *testing.T, src string) *vm.VM {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	v := vm.New()
	native_db.RegisterDBPreload(v)
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v\nsource:\n%s", err, src)
	}
	return v
}

func runDBErr(t *testing.T, src string) string {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	v := vm.New()
	native_db.RegisterDBPreload(v)
	e := v.Run(chunks[0])
	if e == nil {
		t.Fatalf("expected runtime error; got success")
	}
	return e.Error()
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestModuleExposesOpenAndVersion(t *testing.T) {
	v := runDB(t, `
		local db = require("db")
		ver = db.VERSION
		fn = type(db.open)
	`)
	if got := v.Globals.Get("ver"); got != "0.1.0" {
		t.Errorf("VERSION = %v, want 0.1.0", got)
	}
	if got := v.Globals.Get("fn"); got != "function" {
		t.Errorf("type(db.open) = %v, want function", got)
	}
}

func TestExecReturnsRowCount(t *testing.T) {
	v := runDB(t, `
		local db = require("db")
		local c = db.open("luascripttest", "exec1")
		n = c:exec("set foo bar")
		c:close()
	`)
	if got, _ := v.Globals.Get("n").(int64); got != 1 {
		t.Errorf("exec rows = %v, want 1", v.Globals.Get("n"))
	}
}

func TestQueryReturnsRows(t *testing.T) {
	v := runDB(t, `
		local db = require("db")
		local c = db.open("luascripttest", "query1")
		c:exec("set a 1")
		c:exec("set b 2")
		local rows = c:query("all")
		n = #rows
		first_k = rows[1].key
		first_v = rows[1].value
		c:close()
	`)
	if got, _ := v.Globals.Get("n").(int64); got != 2 {
		t.Errorf("row count = %v, want 2", v.Globals.Get("n"))
	}
	if got := v.Globals.Get("first_k"); got != "a" {
		t.Errorf("first key = %v, want a", got)
	}
	if got := v.Globals.Get("first_v"); got != "1" {
		t.Errorf("first value = %v, want 1", got)
	}
}

func TestPingAndClose(t *testing.T) {
	v := runDB(t, `
		local db = require("db")
		local c = db.open("luascripttest", "ping1")
		c:ping()
		c:close()
		r = "ok"
	`)
	if got := v.Globals.Get("r"); got != "ok" {
		t.Errorf("r = %v, want ok", got)
	}
}

func TestOpenRejectsNonStringDriver(t *testing.T) {
	// table can't coerce to string — triggers StringArg's error path.
	msg := runDBErr(t, `
		local db = require("db")
		db.open({}, "")
	`)
	if !strings.Contains(msg, "string expected") {
		t.Errorf("error = %q, want it to mention 'string expected'", msg)
	}
}

func TestOpenRejectsUnknownDriver(t *testing.T) {
	msg := runDBErr(t, `
		local db = require("db")
		db.open("definitely_not_registered", "")
	`)
	if msg == "" {
		t.Errorf("expected an error")
	}
}
