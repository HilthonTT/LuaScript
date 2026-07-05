// Package httpserver provides the `httpserver` native module — an HTTP
// *server* to complement the client-only `http` module.
//
// Concurrency model: the.lsc VM is single-threaded and not safe for
// concurrent access, but net/http dispatches every request on its own
// goroutine. This module bridges the two with a serialized design:
//
//   - `server:listen(addr)` BLOCKS on the calling (VM) goroutine and runs
//     a loop that is the *only* place Lua handlers are ever invoked.
//   - net/http's per-request goroutines merely marshal the request into a
//     Lua table, push a job onto a channel, and block waiting for the
//     reply. They never touch the VM.
//
// As a result every handler runs serially on the VM goroutine — simple
// and race-free, at the cost of no request-level parallelism.
package httpserver

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

const (
	// maxBodyBytes caps the per-request body size. Larger requests get a
	// 413 and the job is never queued for the VM. Picked to cover typical
	// form posts and small uploads; a `:max_body_bytes(n)` setter is
	// intentionally deferred until a script needs it.
	maxBodyBytes = 8 << 20 // 8 MiB

	// jobChBuffer absorbs short request bursts so per-request goroutines
	// don't block on the channel send. Handlers still serialize on the VM
	// goroutine — this just smooths the wake-up cost.
	jobChBuffer = 64

	// stopGraceTimeout bounds how long :stop will wait for in-flight
	// handlers to drain before forcibly closing the server.
	stopGraceTimeout = 5 * time.Second
)

// RegisterHTTPServerPreload installs the `httpserver` module under
// package.preload.
func RegisterHTTPServerPreload(v *vm.VM) {
	vm.RegisterPreload(v, "httpserver", httpServerLoader)
}

func httpServerLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := vm.NewTable(0, 2)
	mod.Set("VERSION", "0.1.0")

	methods := vm.NewTable(0, 1)
	// httpserver.new() -> a fresh server object.
	methods.Set("new", &vm.GoFunc{Name: "httpserver:new", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{newServer()}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	mod.SetMetatable(mt)
	return []vm.Value{mod}
}

// job is one request handed from a net/http goroutine to the VM loop.
type job struct {
	method string
	path   string
	req    *vm.Table
	reply  chan resp
}

// resp is the normalized result the VM loop sends back to the waiting
// net/http goroutine.
type resp struct {
	status  int
	body    string
	headers map[string]string
}

// newServer builds a stateful server object. The route table, the
// optional not-found handler, the job channel, and the *http.Server
// pointer are captured in the method closures so no raw Go handle is
// exposed to script space.
//
// Concurrency invariant: routes, notFound, and srv are all written
// AND read on the VM goroutine. :route / :get / … / :set_not_found
// mutate them, :listen reads routes/notFound in its for-select loop,
// and :stop reads srv. The only cross-goroutine touch is :stop ->
// srv.Shutdown, which is itself goroutine-safe per net/http's contract.
func newServer() *vm.Table {
	routes := map[string]vm.Value{}
	var notFound vm.Value
	var srv *http.Server

	o := vm.NewTable(0, 1)
	methods := vm.NewTable(0, 12)

	// :route(method, path, handler) — register a handler. handler may be
	// any callable (a.lsc function or a host function).
	methods.Set("route", &vm.GoFunc{Name: "server:route", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("server:route", 1, a)
		method := vm.StringArg("server:route", 2, a)
		path := vm.StringArg("server:route", 3, a)
		handler := funcArg("server:route", 4, a)
		routes[strings.ToUpper(method)+" "+path] = handler
		return nil
	}})

	// Method sugar: :get/:post/... (path, handler).
	addVerb := func(name, method string) {
		site := "server:" + name
		methods.Set(name, &vm.GoFunc{Name: site, Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
			_ = vm.TableArg(site, 1, a)
			path := vm.StringArg(site, 2, a)
			handler := funcArg(site, 3, a)
			routes[method+" "+path] = handler
			return nil
		}})
	}
	addVerb("get", http.MethodGet)
	addVerb("post", http.MethodPost)
	addVerb("put", http.MethodPut)
	addVerb("patch", http.MethodPatch)
	addVerb("delete", http.MethodDelete)
	addVerb("head", http.MethodHead)
	addVerb("options", http.MethodOptions)

	// :set_not_found(handler) — custom handler for unmatched routes.
	methods.Set("set_not_found", &vm.GoFunc{Name: "server:set_not_found", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("server:set_not_found", 1, a)
		notFound = funcArg("server:set_not_found", 2, a)
		return nil
	}})

	// :listen(addr) — BLOCKS. Starts the HTTP server and runs the
	// serialization loop. See the package doc for the concurrency model.
	// Returns cleanly when :stop is called (or when ListenAndServe
	// returns http.ErrServerClosed); panics only on genuine bind errors.
	methods.Set("listen", &vm.GoFunc{Name: "server:listen", Fn: func(machine *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("server:listen", 1, a)
		addr := vm.StringArg("server:listen", 2, a)

		jobCh := make(chan job, jobChBuffer)
		errCh := make(chan error, 1)

		handler := func(w http.ResponseWriter, r *http.Request) {
			// MaxBytesReader caps the body AND surfaces a typed error on
			// overflow so we can return 413 cleanly without queueing a
			// half-read body to the VM.
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
			body, err := io.ReadAll(r.Body)
			if err != nil {
				var maxErr *http.MaxBytesError
				if errors.As(err, &maxErr) {
					http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
					return
				}
				http.Error(w, "bad request body", http.StatusBadRequest)
				return
			}
			j := job{
				method: r.Method,
				path:   r.URL.Path,
				req:    buildRequest(r, string(body)),
				reply:  make(chan resp, 1),
			}
			jobCh <- j
			res := <-j.reply
			for k, v := range res.headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(res.status)
			io.WriteString(w, res.body)
		}

		srv = &http.Server{
			Addr:    addr,
			Handler: http.HandlerFunc(handler),
			// Timeouts bound how long a single (possibly malicious) client
			// can hold a connection. Without them a handful of slow-loris
			// clients dribbling headers one byte at a time would pin
			// connections open and, because handlers serialize on the VM
			// goroutine, starve all legitimate traffic.
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		go func() {
			errCh <- srv.ListenAndServe()
		}()

		// The loop: the sole place Lua handlers run.
		for {
			select {
			case err := <-errCh:
				// Clean shutdown via :stop (or any external Shutdown
				// call) surfaces as ErrServerClosed; return normally.
				if errors.Is(err, http.ErrServerClosed) {
					return nil
				}
				panic(vm.Errorf("server:listen: %s", err.Error()))
			case j := <-jobCh:
				h := routes[j.method+" "+j.path]
				j.reply <- dispatch(machine, h, notFound, j.req)
			}
		}
	}})

	// :stop() — request graceful shutdown. Safe to call from inside a
	// handler: Shutdown is dispatched on a background goroutine so the
	// current handler can finish, the :listen loop drains any queued
	// jobs, and ListenAndServe's ErrServerClosed unblocks the loop.
	methods.Set("stop", &vm.GoFunc{Name: "server:stop", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("server:stop", 1, a)
		if srv == nil {
			return nil // :listen never started; nothing to do.
		}
		go func(s *http.Server) {
			ctx, cancel := context.WithTimeout(context.Background(), stopGraceTimeout)
			defer cancel()
			_ = s.Shutdown(ctx)
		}(srv)
		return nil
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	o.SetMetatable(mt)
	return o
}

// dispatch invokes the matched handler (or the not-found handler / a
// default 404) on the VM goroutine and normalizes its return value into
// a resp. A handler panic is recovered and turned into a 500 so a single
// bad request can never take the server down.
func dispatch(machine *vm.VM, handler, notFound vm.Value, req *vm.Table) (out resp) {
	if handler == nil {
		if notFound == nil {
			return resp{status: http.StatusNotFound, body: "404 page not found\n",
				headers: map[string]string{"Content-Type": "text/plain; charset=utf-8"}}
		}
		handler = notFound
	}

	// Route through SafeCall, not CallValue: a handler panic (a Lua error(),
	// a bad-arg panic, a nil index, …) must not leave the shared VM's
	// stack/frames/upvalues dirty for the next request. SafeCall recovers and
	// fully unwinds the VM, so one bad request can neither crash the server
	// nor corrupt subsequent handlers.
	results, errVal, failed := machine.SafeCall(handler, []vm.Value{req})
	if failed {
		msg := vm.ToString(errVal)
		return resp{status: http.StatusInternalServerError, body: msg + "\n",
			headers: map[string]string{"Content-Type": "text/plain; charset=utf-8"}}
	}
	var result vm.Value
	if len(results) > 0 {
		result = results[0]
	}
	return normalize(result)
}

// normalize turns a handler's return value into a resp. A string becomes
// a 200 text/plain body; a table is read for status/body/headers; nil
// (or no return) becomes an empty 200.
func normalize(result vm.Value) resp {
	switch v := result.(type) {
	case nil:
		return resp{status: http.StatusOK, headers: map[string]string{}}
	case string:
		return resp{status: http.StatusOK, body: v,
			headers: map[string]string{"Content-Type": "text/plain; charset=utf-8"}}
	case *vm.Table:
		out := resp{status: http.StatusOK, headers: map[string]string{}}
		if s, ok := v.Get("status").(int64); ok {
			out.status = int(s)
		}
		if b, ok := v.Get("body").(string); ok {
			out.body = b
		}
		if h, ok := v.Get("headers").(*vm.Table); ok {
			var k vm.Value
			for {
				var hv vm.Value
				k, hv = h.Next(k)
				if k == nil {
					break
				}
				ks, kok := k.(string)
				vs, vok := hv.(string)
				if kok && vok {
					out.headers[ks] = vs
				}
			}
		}
		return out
	default:
		panic(vm.Errorf("server handler must return a string or table, got %s", vm.TypeName(result)))
	}
}

// buildRequest marshals an *http.Request into the Lua request table that
// handlers receive as their single argument.
func buildRequest(r *http.Request, body string) *vm.Table {
	t := vm.NewTable(0, 7)
	t.Set("method", r.Method)
	t.Set("path", r.URL.Path)
	t.Set("query", r.URL.RawQuery)
	t.Set("body", body)
	t.Set("host", r.Host)
	t.Set("remote_addr", r.RemoteAddr)

	headers := vm.NewTable(0, len(r.Header))
	for k, vs := range r.Header {
		headers.Set(k, strings.Join(vs, ", "))
	}
	t.Set("headers", headers)
	return t
}

// funcArg validates that arg n is a callable (a.lsc *Closure or a host
// *GoFunc) — vm has no combined helper, and ClosureArg would reject
// *GoFunc handlers.
func funcArg(name string, n int, args []vm.Value) vm.Value {
	if n < 1 || n > len(args) {
		panic(vm.Errorf("bad argument #%d to '%s' (function expected)", n, name))
	}
	switch args[n-1].(type) {
	case *vm.Closure, *vm.GoFunc:
		return args[n-1]
	}
	panic(vm.Errorf("bad argument #%d to '%s' (function expected, got %s)", n, name, vm.TypeName(args[n-1])))
}
