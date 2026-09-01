package http

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

func RegisterHttpPreload(v *vm.VM) {
	vm.RegisterPreload(v, "http", httpLoader)
}

func httpLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newHttp()
	mod.Set("VERSION", "0.1.0")

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

	addShortcut(methods, "get", http.MethodGet, false)
	addShortcut(methods, "delete", http.MethodDelete, false)
	addShortcut(methods, "head", http.MethodHead, false)
	addShortcut(methods, "options", http.MethodOptions, false)
	addShortcut(methods, "post", http.MethodPost, true)
	addShortcut(methods, "put", http.MethodPut, true)
	addShortcut(methods, "patch", http.MethodPatch, true)

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

	methods.Set("new_client", &vm.GoFunc{Name: "http:new_client", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		var clientOpts *vm.Table
		if len(args) >= 1 && args[0] != nil {
			clientOpts = vm.TableArg("http.new_client", 1, args)
		}
		return []vm.Value{newClient(clientOpts)}
	}})

	methods.Set("encode_url", &vm.GoFunc{Name: "http:encode_url", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		t := vm.TableArg("http.encode_url", 1, args)
		return []vm.Value{encodeQuery(t)}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	o.SetMetatable(mt)
	return o
}

func addShortcut(methods *vm.Table, name, method string, hasBody bool) {
	site := "http." + name
	methods.Set(name, &vm.GoFunc{Name: "http:" + name, Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		rawURL := vm.StringArg(site, 1, args)
		var body string
		optsIdx := 2
		if hasBody {
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

func newClient(opts *vm.Table) *vm.Table {
	hc := &http.Client{Timeout: defaultTimeout}
	var baseURL string
	var defaultHeaders *vm.Table
	if opts != nil {
		applyTimeout(opts, hc)
		applyRedirectPolicy(opts, hc)
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
		if body == "" {
			if b, ok := opts.Get("body").(string); ok {
				body = b
			}
		}
	}

	autoType := ""
	if opts != nil && body == "" {
		if j := opts.Get("json"); j != nil {
			encoded, err := json.Marshal(vmToJSON(j, 0))
			if err != nil {
				panic(vm.Errorf("%s: encoding opts.json: %s", site, err.Error()))
			}
			body = string(encoded)
			autoType = "application/json"
		} else if f, ok := opts.Get("form").(*vm.Table); ok && f != nil {
			body = encodeQuery(f)
			autoType = "application/x-www-form-urlencoded"
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
		if u, ok := opts.Get("username").(string); ok && u != "" {
			pw, _ := opts.Get("password").(string)
			req.SetBasicAuth(u, pw)
		}
	}
	if autoType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", autoType)
	}

	resp, err := client.Do(req)
	if err != nil {
		panic(vm.Errorf("%s: %s", site, err.Error()))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		panic(vm.Errorf("%s: reading response body: %s", site, err.Error()))
	}
	if int64(len(respBody)) > maxResponseBytes {
		panic(vm.Errorf("%s: response body exceeds %d bytes", site, int64(maxResponseBytes)))
	}

	r := vm.NewTable(0, 5)
	r.Set("status", int64(resp.StatusCode))
	r.Set("status_text", resp.Status)
	r.Set("body", string(respBody))
	r.Set("ok", resp.StatusCode >= 200 && resp.StatusCode < 300)

	headers := vm.NewTable(0, len(resp.Header))
	rawHeaders := vm.NewTable(0, len(resp.Header))
	for k, vs := range resp.Header {
		headers.Set(k, strings.Join(vs, ", "))
		list := vm.NewTable(len(vs), 0)
		for _, one := range vs {
			list.Append(one)
		}
		rawHeaders.Set(k, list)
	}
	r.Set("headers", headers)
	r.Set("headers_raw", rawHeaders)

	if resp.Request != nil && resp.Request.URL != nil {
		r.Set("url", resp.Request.URL.String())
	} else {
		r.Set("url", rawURL)
	}

	return r
}

func vmToJSON(v vm.Value, depth int) any {
	if depth > 1000 {
		panic(vm.Errorf("http: request body nesting too deep (cyclic reference?)"))
	}
	switch x := v.(type) {
	case nil:
		return nil
	case bool:
		return x
	case int64:
		return x
	case float64:
		return x
	case string:
		return x
	case *vm.Table:
		if n := x.Len(); n > 0 && isPureArray(x, n) {
			arr := make([]any, 0, n)
			for i := int64(1); i <= n; i++ {
				arr = append(arr, vmToJSON(x.Get(i), depth+1))
			}
			return arr
		}
		obj := map[string]any{}
		var key vm.Value
		for {
			var val vm.Value
			key, val = x.Next(key)
			if key == nil {
				break
			}
			switch k := key.(type) {
			case string:
				obj[k] = vmToJSON(val, depth+1)
			case int64:
				obj[strconv.FormatInt(k, 10)] = vmToJSON(val, depth+1)
			}
		}
		return obj
	}
	panic(vm.Errorf("http: cannot encode a %s value as JSON", vm.TypeName(v)))
}

func isPureArray(t *vm.Table, n int64) bool {
	for i := int64(1); i <= n; i++ {
		if t.Get(i) == nil {
			return false
		}
	}
	count := int64(0)
	var key vm.Value
	for {
		key, _ = t.Next(key)
		if key == nil {
			break
		}
		count++
		if count > n {
			return false
		}
	}
	return count == n
}

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

const (
	defaultTimeout = 30 * time.Second

	maxResponseBytes = 64 << 20
)

func clientFromTimeout(opts *vm.Table) *http.Client {
	hc := &http.Client{Timeout: defaultTimeout}
	if opts != nil {
		applyTimeout(opts, hc)
		applyRedirectPolicy(opts, hc)
	}
	return hc
}

func applyRedirectPolicy(opts *vm.Table, hc *http.Client) {
	follow, isBool := opts.Get("follow_redirects").(bool)
	if !isBool || follow {
		return
	}
	hc.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
}

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

func mergeRequestOpts(defaultHeaders *vm.Table, perReq *vm.Table) *vm.Table {
	if defaultHeaders == nil {
		return perReq
	}

	merged := vm.NewTable(0, 4)
	headers := vm.NewTable(0, 8)

	var dk vm.Value
	for {
		var dv vm.Value
		dk, dv = defaultHeaders.Next(dk)
		if dk == nil {
			break
		}
		headers.Set(dk, dv)
	}

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
