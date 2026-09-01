package db_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLiteRoundTrip(t *testing.T) {
	v := runDB(t, `
		local db = require("db")
		local c = db.open("sqlite", ":memory:")
		c:exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT, score REAL)")
		c:exec("INSERT INTO t (id, name, score) VALUES (?, ?, ?)", 1, "ada", 99.5)
		c:exec("INSERT INTO t (id, name, score) VALUES (?, ?, ?)", 2, "grace", 87.25)

		local rows = c:query("SELECT id, name, score FROM t ORDER BY id")
		count      = #rows
		first_id   = rows[1].id
		first_name = rows[1].name
		first_score= rows[1].score
		id_type    = type(rows[1].id)
		score_type = type(rows[1].score)
		c:close()
	`)

	if got, _ := v.Globals.Get("count").(int64); got != 2 {
		t.Errorf("row count = %v, want 2", v.Globals.Get("count"))
	}
	if got := v.Globals.Get("first_name"); got != "ada" {
		t.Errorf("name = %v, want ada", got)
	}
	if got := v.Globals.Get("id_type"); got != "number" {
		t.Errorf("type(id) = %v, want number", got)
	}
	if got := v.Globals.Get("score_type"); got != "number" {
		t.Errorf("type(score) = %v, want number", got)
	}
	if got, _ := v.Globals.Get("first_score").(float64); got != 99.5 {
		t.Errorf("score = %v, want 99.5", v.Globals.Get("first_score"))
	}
}

func TestSQLiteInMemoryPoolIsPinned(t *testing.T) {
	v := runDB(t, `
		local db = require("db")
		local c = db.open("sqlite", ":memory:")
		c:exec("CREATE TABLE t (n INTEGER)")
		-- Enough separate statements to cycle the pool if it had room to.
		for i = 1, 25 do
			c:exec("INSERT INTO t (n) VALUES (?)", i)
		end
		local rows = c:query("SELECT count(*) AS n FROM t")
		total = rows[1].n
		c:close()
	`)
	if got, _ := v.Globals.Get("total").(int64); got != 25 {
		t.Errorf("row count = %v, want 25 — the in-memory pool was not pinned", v.Globals.Get("total"))
	}
}

func TestSQLiteFileBacked(t *testing.T) {
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "app.db"))
	v := runDB(t, `
		local db = require("db")
		local c = db.open("sqlite", "`+path+`")
		c:exec("CREATE TABLE t (n INTEGER)")
		c:exec("INSERT INTO t (n) VALUES (?)", 7)
		c:close()

		-- Reopen: a file-backed database must survive the connection.
		local c2 = db.open("sqlite", "`+path+`")
		local rows = c2:query("SELECT n FROM t")
		n = rows[1].n
		c2:close()
	`)
	if got, _ := v.Globals.Get("n").(int64); got != 7 {
		t.Errorf("n = %v, want 7", v.Globals.Get("n"))
	}
}

func TestExecReturnsLastInsertID(t *testing.T) {
	v := runDB(t, `
		local db = require("db")
		local c = db.open("sqlite", ":memory:")
		c:exec("CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT)")
		rows1, id1 = c:exec("INSERT INTO t (name) VALUES (?)", "a")
		rows2, id2 = c:exec("INSERT INTO t (name) VALUES (?)", "b")
		c:close()
	`)
	if got, _ := v.Globals.Get("rows1").(int64); got != 1 {
		t.Errorf("rows affected = %v, want 1", v.Globals.Get("rows1"))
	}
	if got, _ := v.Globals.Get("id1").(int64); got != 1 {
		t.Errorf("first insert id = %v, want 1", v.Globals.Get("id1"))
	}
	if got, _ := v.Globals.Get("id2").(int64); got != 2 {
		t.Errorf("second insert id = %v, want 2", v.Globals.Get("id2"))
	}
}

func TestNullBecomesNil(t *testing.T) {
	v := runDB(t, `
		local db = require("db")
		local c = db.open("sqlite", ":memory:")
		c:exec("CREATE TABLE t (a TEXT, b INTEGER)")
		c:exec("INSERT INTO t (a, b) VALUES (?, ?)", "set", nil)
		local rows = c:query("SELECT a, b FROM t")
		a = rows[1].a
		b_is_nil = rows[1].b == nil
		c:close()
	`)
	if got := v.Globals.Get("a"); got != "set" {
		t.Errorf("a = %v, want 'set'", got)
	}
	if got := v.Globals.Get("b_is_nil"); got != true {
		t.Errorf("NULL column = %v, want nil", v.Globals.Get("b_is_nil"))
	}
}

func TestBindParametersAreNotInterpolated(t *testing.T) {
	v := runDB(t, `
		local db = require("db")
		local c = db.open("sqlite", ":memory:")
		c:exec("CREATE TABLE t (name TEXT)")
		c:exec("INSERT INTO t (name) VALUES (?)", "O'Brien; DROP TABLE t; --")
		local rows = c:query("SELECT name FROM t")
		count = #rows
		name  = rows[1].name
		c:close()
	`)
	if got, _ := v.Globals.Get("count").(int64); got != 1 {
		t.Fatalf("row count = %v, want 1 — the table may have been dropped", v.Globals.Get("count"))
	}
	if got := v.Globals.Get("name"); got != "O'Brien; DROP TABLE t; --" {
		t.Errorf("name = %v, want the literal string back", got)
	}
}

func TestConnectionReportsItsDriverAndPlaceholder(t *testing.T) {
	v := runDB(t, `
		local db = require("db")
		local c = db.open("sqlite", ":memory:")
		driver = c.driver
		ph     = c:placeholder(1)
		c:close()
	`)
	got, _ := v.Globals.Get("driver").(string)
	if !strings.HasPrefix(got, "sqlite") {
		t.Errorf("conn.driver = %v, want a sqlite driver", v.Globals.Get("driver"))
	}
	if got := v.Globals.Get("ph"); got != "?" {
		t.Errorf("conn:placeholder(1) = %v, want ?", got)
	}
}

func TestDriversListsWhatIsCompiledIn(t *testing.T) {
	v := runDB(t, `
		local db = require("db")
		names = table.concat(db.drivers(), ",")
	`)
	list, _ := v.Globals.Get("names").(string)
	for _, want := range []string{"postgres", "mysql", "sqlserver", "sqlite"} {
		if !strings.Contains(list, want) {
			t.Errorf("db.drivers() = %q, want it to include %q", list, want)
		}
	}
}

func TestPlaceholderModuleFunction(t *testing.T) {
	v := runDB(t, `
		local db = require("db")
		pg  = db.placeholder("postgres", 2)
		my  = db.placeholder("mysql", 2)
		ms  = db.placeholder("sqlserver", 2)
		pg1 = db.placeholder("pg")
	`)
	for name, want := range map[string]string{"pg": "$2", "my": "?", "ms": "@p2", "pg1": "$1"} {
		if got := v.Globals.Get(name); got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
}

func TestOpenResolvesAliases(t *testing.T) {
	runDB(t, `
		local db = require("db")
		local c = db.open("sqlite3", ":memory:")
		c:ping()
		c:close()
	`)
}

func TestQueryErrorIsCatchable(t *testing.T) {
	v := runDB(t, `
		local db = require("db")
		local c = db.open("sqlite", ":memory:")
		ok, err = pcall(function() return c:query("SELECT * FROM nope") end)
		msg = tostring(err)
		c:close()
	`)
	if got := v.Globals.Get("ok"); got != false {
		t.Errorf("pcall ok = %v, want false", got)
	}
	msg, _ := v.Globals.Get("msg").(string)
	if !strings.Contains(msg, "db:query") {
		t.Errorf("error = %q, want it to name the failing method", msg)
	}
}
