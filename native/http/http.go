package http

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hilthontt/sakura-lang/vm"
)

// RegisterHttpPreload installs the `http` module under package.preload.
// Mirrors the pattern of native/json and native/os — `require("http")`
// triggers httpLoader on first use.
func RegisterHttpPreload(v *vm.VM) {
	vm.RegisterPreload(v, "http", httpLoader)
}

func httpLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newHttp()
	mod.Set("VERSION", "0.1.0")

	// HTTP method constants.
	mod.Set("MethodGet", http.MethodGet)
	mod.Set("MethodPost", http.MethodPost)
	mod.Set("MethodPut", http.MethodPut)
	mod.Set("MethodPatch", http.MethodPatch)
	mod.Set("MethodDelete", http.MethodDelete)
	mod.Set("MethodHead", http.MethodHead)
	mod.Set("MethodOptions", http.MethodOptions)
	mod.Set("MethodTrace", http.MethodTrace)

	return []vm.Value{mod}
}

func newHttp() *vm.Table {
	o := vm.NewTable(0, 31)
	methods := vm.NewTable(0, 10)

	// Module-level shortcuts use `.` call style: http.get(url[, opts]).
	// Arg index 1 = url; no implicit self. Body-bearing methods take
	// the body string at index 2 and opts at index 3.
	addShortcut(methods, "get", http.MethodGet, false)
	addShortcut(methods, "delete", http.MethodDelete, false)
	addShortcut(methods, "head", http.MethodHead, false)
	addShortcut(methods, "options", http.MethodOptions, false)
	addShortcut(methods, "post", http.MethodPost, true)
	addShortcut(methods, "put", http.MethodPut, true)
	addShortcut(methods, "patch", http.MethodPatch, true)

	// http.request{ method=, url=, body=, headers=, query=, timeout= }
	// Full-surface entry point. `timeout` is seconds (int or float).
	methods.Set("request", &vm.GoFunc{Name: "http:request", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		opts := vm.TableArg("http.request", 1, args)
		method, _ := opts.Get("method").(string)
		if method == "" {
			method = http.MethodGet
		}
		u, _ := opts.Get("url").(string)
		if u == "" {
			panic(vm.Errorf("http.request: 'url' is required"))
		}
		body, _ := opts.Get("body").(string)
		client := clientFromTimeout(opts)
		return []vm.Value{doRequest(client, method, u, body, opts)}
	}})

	// http.new_client{ timeout=, base_url=, headers= } -> client object.
	// Headers passed here are sent on every request unless overridden.
	methods.Set("new_client", &vm.GoFunc{Name: "http:new_client", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		var clientOpts *vm.Table
		if len(args) >= 1 && args[0] != nil {
			clientOpts = vm.TableArg("http.new_client", 1, args)
		}
		return []vm.Value{newClient(clientOpts)}
	}})

	// http.encode_url(table) -> "k=v&k=v" with proper percent-encoding.
	// Repeats values when a key maps to an array-style table.
	methods.Set("encode_url", &vm.GoFunc{Name: "http:encode_url", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		t := vm.TableArg("http.encode_url", 1, args)
		return []vm.Value{encodeQuery(t)}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	o.SetMetatable(mt)
	return o
}

// addShortcut wires one of get/post/put/etc. onto the module's methods
// table. hasBody controls whether arg #2 is a body string (post/put/patch)
// or the opts table (get/delete/head/options).
func addShortcut(methods *vm.Table, name, method string, hasBody bool) {
	site := "http." + name
	methods.Set(name, &vm.GoFunc{Name: "http:" + name, Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		rawURL := vm.StringArg(site, 1, args)
		var body string
		optsIdx := 2
		if hasBody {
			// Body slot is optional-but-typed: nil/absent -> "", string -> body,
			// anything else -> bad-arg panic with the standard shape.
			if len(args) >= 2 && args[1] != nil {
				s, ok := args[1].(string)
				if !ok {
					panic(vm.Errorf("bad argument #2 to '%s' (string or nil expected, got %s)", site, vm.TypeName(args[1])))
				}
				body = s
			}
			optsIdx = 3
		}
		var opts *vm.Table
		if len(args) >= optsIdx && args[optsIdx-1] != nil {
			opts = vm.TableArg(site, optsIdx, args)
		}
		client := clientFromTimeout(opts)
		return []vm.Value{doRequest(client, method, rawURL, body, opts)}
	}})
}

// newClient builds a stateful client table. base_url + default headers
// are captured in the closure so each method invocation merges them with
// per-request opts.
func newClient(opts *vm.Table) *vm.Table {
	hc := &http.Client{}
	var baseURL string
	var defaultHeaders *vm.Table
	if opts != nil {
		applyTimeout(opts, hc)
		if s, ok := opts.Get("base_url").(string); ok {
			baseURL = s
		}
		if h, ok := opts.Get("headers").(*vm.Table); ok {
			defaultHeaders = h
		}
	}

	c := vm.NewTable(0, 1)
	methods := vm.NewTable(0, 8)

	addClientMethod := func(name, method string, hasBody bool) {
		site := "client:" + name
		methods.Set(name, &vm.GoFunc{Name: site, Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
			// `:` call style — arg 1 = self, arg 2 = path, then body/opts.
			_ = vm.TableArg(site, 1, a)
			path := vm.StringArg(site, 2, a)
			var body string
			optsIdx := 3
			if hasBody {
				if len(a) >= 3 && a[2] != nil {
					s, ok := a[2].(string)
					if !ok {
						panic(vm.Errorf("bad argument #3 to '%s' (string or nil expected, got %s)", site, vm.TypeName(a[2])))
					}
					body = s
				}
				optsIdx = 4
			}
			var perReq *vm.Table
			if len(a) >= optsIdx && a[optsIdx-1] != nil {
				perReq = vm.TableArg(site, optsIdx, a)
			}
			fullURL := joinURL(baseURL, path)
			merged := mergeRequestOpts(defaultHeaders, perReq)
			return []vm.Value{doRequest(hc, method, fullURL, body, merged)}
		}})
	}
	addClientMethod("get", http.MethodGet, false)
	addClientMethod("delete", http.MethodDelete, false)
	addClientMethod("head", http.MethodHead, false)
	addClientMethod("options", http.MethodOptions, false)
	addClientMethod("post", http.MethodPost, true)
	addClientMethod("put", http.MethodPut, true)
	addClientMethod("patch", http.MethodPatch, true)

	// client:request{ ... } — same shape as http.request but with the
	// client's base_url/headers merged in.
	methods.Set("request", &vm.GoFunc{Name: "client:request", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("client:request", 1, a)
		opts := vm.TableArg("client:request", 2, a)
		method, _ := opts.Get("method").(string)
		if method == "" {
			method = http.MethodGet
		}
		u, _ := opts.Get("url").(string)
		if u == "" {
			panic(vm.Errorf("client:request: 'url' is required"))
		}
		body, _ := opts.Get("body").(string)
		fullURL := joinURL(baseURL, u)
		merged := mergeRequestOpts(defaultHeaders, opts)
		return []vm.Value{doRequest(hc, method, fullURL, body, merged)}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	c.SetMetatable(mt)
	return c
}

// doRequest is the single execution path for every method on the module
// and on client objects. It builds the request, applies headers/query,
// reads the response fully, and returns the response table.
func doRequest(client *http.Client, method, rawURL, body string, opts *vm.Table) *vm.Table {
	site := "http." + strings.ToLower(method)

	if opts != nil {
		if q, ok := opts.Get("query").(*vm.Table); ok && q != nil {
			qs := encodeQuery(q)
			if qs != "" {
				if strings.Contains(rawURL, "?") {
					rawURL += "&" + qs
				} else {
					rawURL += "?" + qs
				}
			}
		}
		// `body` on opts is a fallback for shortcut methods that took no
		// body arg (get/delete/head). For post/put/patch the positional
		// body arg has already been threaded through.
		if body == "" {
			if b, ok := opts.Get("body").(string); ok {
				body = b
			}
		}
	}

	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}

	req, err := http.NewRequest(method, rawURL, reader)
	if err != nil {
		panic(vm.Errorf("%s: %s", site, err.Error()))
	}

	if opts != nil {
		if h, ok := opts.Get("headers").(*vm.Table); ok && h != nil {
			applyHeaders(req, h)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		panic(vm.Errorf("%s: %s", site, err.Error()))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(vm.Errorf("%s: reading response body: %s", site, err.Error()))
	}

	r := vm.NewTable(0, 5)
	r.Set("status", int64(resp.StatusCode))
	r.Set("status_text", resp.Status)
	r.Set("body", string(respBody))
	r.Set("ok", resp.StatusCode >= 200 && resp.StatusCode < 300)

	headers := vm.NewTable(0, len(resp.Header))
	for k, vs := range resp.Header {
		// Multi-valued headers (e.g. Set-Cookie) are joined with ", " to
		// keep the response table shape flat. Scripts that need the raw
		// list can call http.request and inspect with a custom helper —
		// out of scope for v1.
		headers.Set(k, strings.Join(vs, ", "))
	}
	r.Set("headers", headers)

	return r
}

// applyHeaders copies string→string entries from a Lua table into req.Header.
// Non-string keys/values are silently skipped — mirrors json.encode's
// approach to non-string keys for compatibility with dynamic Lua tables.
func applyHeaders(req *http.Request, h *vm.Table) {
	var k vm.Value
	for {
		var v vm.Value
		k, v = h.Next(k)
		if k == nil {
			break
		}
		ks, ok := k.(string)
		if !ok {
			continue
		}
		vs, ok := v.(string)
		if !ok {
			continue
		}
		req.Header.Set(ks, vs)
	}
}

// encodeQuery turns a Lua table into a URL-encoded query string. Array-
// style table values become repeated query params (?tag=a&tag=b).
func encodeQuery(t *vm.Table) string {
	values := url.Values{}
	var k vm.Value
	for {
		var v vm.Value
		k, v = t.Next(k)
		if k == nil {
			break
		}
		ks, ok := k.(string)
		if !ok {
			continue
		}
		switch x := v.(type) {
		case string:
			values.Add(ks, x)
		case int64:
			values.Add(ks, strconv.FormatInt(x, 10))
		case float64:
			values.Add(ks, strconv.FormatFloat(x, 'g', -1, 64))
		case bool:
			if x {
				values.Add(ks, "true")
			} else {
				values.Add(ks, "false")
			}
		case *vm.Table:
			n := x.Len()
			for i := int64(1); i <= n; i++ {
				switch iv := x.Get(i).(type) {
				case string:
					values.Add(ks, iv)
				case int64:
					values.Add(ks, strconv.FormatInt(iv, 10))
				case float64:
					values.Add(ks, strconv.FormatFloat(iv, 'g', -1, 64))
				}
			}
		}
	}
	return values.Encode()
}

// clientFromTimeout returns http.DefaultClient when no timeout is set, or
// a new *http.Client with the timeout applied. Both reuse the default
// Transport (and thus its connection pool) since Client.Transport is nil.
func clientFromTimeout(opts *vm.Table) *http.Client {
	if opts == nil {
		return http.DefaultClient
	}
	hc := &http.Client{}
	if applyTimeout(opts, hc) {
		return hc
	}
	return http.DefaultClient
}

// applyTimeout reads opts.timeout (seconds, int or float) onto hc.
// Returns true if a positive timeout was applied.
func applyTimeout(opts *vm.Table, hc *http.Client) bool {
	switch t := opts.Get("timeout").(type) {
	case int64:
		if t > 0 {
			hc.Timeout = time.Duration(t) * time.Second
			return true
		}
	case float64:
		if t > 0 {
			hc.Timeout = time.Duration(t * float64(time.Second))
			return true
		}
	}
	return false
}

// joinURL stitches a base_url to a per-request path. Absolute URLs in the
// path slot bypass the base entirely so callers can override on a case-
// by-case basis. Trailing-slash / leading-slash collisions are normalized
// to a single `/`.
func joinURL(base, path string) string {
	if base == "" {
		return path
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	switch {
	case strings.HasSuffix(base, "/") && strings.HasPrefix(path, "/"):
		return base + path[1:]
	case !strings.HasSuffix(base, "/") && path != "" && !strings.HasPrefix(path, "/"):
		return base + "/" + path
	default:
		return base + path
	}
}

// mergeRequestOpts overlays per-request opts on top of the client's
// default headers. Per-request headers win on conflict; non-header keys
// (body/query/timeout/method/url) come straight from perReq. If there
// are no defaultHeaders, perReq is returned as-is to avoid a needless
// allocation.
func mergeRequestOpts(defaultHeaders *vm.Table, perReq *vm.Table) *vm.Table {
	if defaultHeaders == nil {
		return perReq
	}

	merged := vm.NewTable(0, 4)
	headers := vm.NewTable(0, 8)

	// Seed with defaults.
	var dk vm.Value
	for {
		var dv vm.Value
		dk, dv = defaultHeaders.Next(dk)
		if dk == nil {
			break
		}
		headers.Set(dk, dv)
	}

	// Overlay per-request options. The "headers" key is special-cased:
	// rather than replacing the table wholesale, individual entries
	// overlay onto the seeded defaults so callers can add a single
	// auth header without redeclaring the rest.
	if perReq != nil {
		var k vm.Value
		for {
			var v vm.Value
			k, v = perReq.Next(k)
			if k == nil {
				break
			}
			if ks, ok := k.(string); ok && ks == "headers" {
				if h, ok := v.(*vm.Table); ok {
					var hk vm.Value
					for {
						var hv vm.Value
						hk, hv = h.Next(hk)
						if hk == nil {
							break
						}
						headers.Set(hk, hv)
					}
				}
				continue
			}
			merged.Set(k, v)
		}
	}
	merged.Set("headers", headers)
	return merged
}
