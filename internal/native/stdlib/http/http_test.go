package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	nativehttp "github.com/hilthontt/luascript/internal/native/stdlib/http"
	"github.com/hilthontt/luascript/internal/vm"
)

func runHTTP(t *testing.T, base, src string) *vm.VM {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	v := vm.New()
	nativehttp.RegisterHttpPreload(v)
	v.Globals.Set("BASE", base)
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v\nsource:\n%s", err, src)
	}
	return v
}

func eq(t *testing.T, v *vm.VM, name string, want vm.Value) {
	t.Helper()
	if got := v.Globals.Get(name); !vm.Equal(got, want) {
		t.Errorf("%s = %v (%T), want %v (%T)", name, got, got, want, want)
	}
}

func TestPostJSONSetsBodyAndContentType(t *testing.T) {
	var gotType, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotType = r.Header.Get("Content-Type")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	runHTTP(t, srv.URL, `
		local http = require("http")
		http.post(BASE, nil, { json = { name = "ada", n = 3 } })
	`)
	if gotType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotType)
	}
	if gotBody != `{"n":3,"name":"ada"}` && gotBody != `{"name":"ada","n":3}` {
		t.Errorf("body = %q, want the table encoded as a JSON object", gotBody)
	}
}

func TestPostFormSetsBodyAndContentType(t *testing.T) {
	var gotType, gotField string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotType = r.Header.Get("Content-Type")
		_ = r.ParseForm()
		gotField = r.PostFormValue("name")
	}))
	defer srv.Close()

	runHTTP(t, srv.URL, `
		local http = require("http")
		http.post(BASE, nil, { form = { name = "ada" } })
	`)
	if gotType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want form encoding", gotType)
	}
	if gotField != "ada" {
		t.Errorf("form field name = %q, want ada", gotField)
	}
}

func TestExplicitContentTypeWins(t *testing.T) {
	var gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotType = r.Header.Get("Content-Type")
	}))
	defer srv.Close()

	runHTTP(t, srv.URL, `
		local http = require("http")
		http.post(BASE, nil, {
			json = { a = 1 },
			headers = { ["Content-Type"] = "application/vnd.custom+json" },
		})
	`)
	if gotType != "application/vnd.custom+json" {
		t.Errorf("Content-Type = %q, want the explicit header to win", gotType)
	}
}

func TestHeadersRawKeepsMultipleValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "a=1; Path=/")
		w.Header().Add("Set-Cookie", "b=2; Path=/")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	v := runHTTP(t, srv.URL, `
		local http = require("http")
		local res = http.get(BASE)
		n = #res.headers_raw["Set-Cookie"]
		first = res.headers_raw["Set-Cookie"][1]
		second = res.headers_raw["Set-Cookie"][2]
		joined = res.headers["Set-Cookie"]
	`)
	eq(t, v, "n", int64(2))
	eq(t, v, "first", "a=1; Path=/")
	eq(t, v, "second", "b=2; Path=/")
	eq(t, v, "joined", "a=1; Path=/, b=2; Path=/")
}

func TestFollowRedirectsOff(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dest", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("arrived"))
	})
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dest", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	v := runHTTP(t, srv.URL, `
		local http = require("http")
		local followed = http.get(BASE .. "/start")
		followedStatus = followed.status
		followedBody = followed.body

		local stopped = http.get(BASE .. "/start", { follow_redirects = false })
		stoppedStatus = stopped.status
		location = stopped.headers["Location"]
	`)
	eq(t, v, "followedStatus", int64(200))
	eq(t, v, "followedBody", "arrived")
	eq(t, v, "stoppedStatus", int64(302))
	eq(t, v, "location", "/dest")
}

func TestResponseURLReflectsRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/dest", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dest", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	v := runHTTP(t, srv.URL, `
		local http = require("http")
		finalURL = http.get(BASE .. "/start").url
	`)
	got, _ := v.Globals.Get("finalURL").(string)
	if got != srv.URL+"/dest" {
		t.Errorf("res.url = %q, want the post-redirect URL %q", got, srv.URL+"/dest")
	}
}

func TestBasicAuth(t *testing.T) {
	var user, pass string
	var ok bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok = r.BasicAuth()
	}))
	defer srv.Close()

	runHTTP(t, srv.URL, `
		local http = require("http")
		http.get(BASE, { username = "ada", password = "s3cret" })
	`)
	if !ok || user != "ada" || pass != "s3cret" {
		t.Errorf("basic auth = (%q, %q, ok=%v), want (ada, s3cret, true)", user, pass, ok)
	}
}

func TestJSONBodyEncodesArrays(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
	}))
	defer srv.Close()

	runHTTP(t, srv.URL, `
		local http = require("http")
		http.post(BASE, nil, { json = {1, 2, 3} })
	`)
	if gotBody != "[1,2,3]" {
		t.Errorf("body = %q, want [1,2,3]", gotBody)
	}
}
