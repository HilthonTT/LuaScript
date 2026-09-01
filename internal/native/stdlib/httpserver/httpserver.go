package httpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

const (
	maxBodyBytes = 8 << 20

	jobChBuffer = 64

	stopGraceTimeout = 5 * time.Second
)

func RegisterHTTPServerPreload(v *vm.VM) {
	vm.RegisterPreload(v, "httpserver", httpServerLoader)
}

func httpServerLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := vm.NewTable(0, 2)
	mod.Set("VERSION", "0.1.0")

	methods := vm.NewTable(0, 1)
	methods.Set("new", &vm.GoFunc{Name: "httpserver:new", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{newServer()}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	mod.SetMetatable(mt)
	return []vm.Value{mod}
}

type job struct {
	method string
	path   string
	req    *vm.Table
	reply  chan resp
}

type resp struct {
	status  int
	body    string
	headers map[string]string
}

func newServer() *vm.Table {
	routes := map[string]vm.Value{}
	var notFound vm.Value
	var srv *http.Server

	o := vm.NewTable(0, 1)
	methods := vm.NewTable(0, 12)

	methods.Set("route", &vm.GoFunc{Name: "server:route", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("server:route", 1, a)
		method := vm.StringArg("server:route", 2, a)
		path := vm.StringArg("server:route", 3, a)
		handler := funcArg("server:route", 4, a)
		routes[strings.ToUpper(method)+" "+path] = handler
		return nil
	}})

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

	methods.Set("set_not_found", &vm.GoFunc{Name: "server:set_not_found", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("server:set_not_found", 1, a)
		notFound = funcArg("server:set_not_found", 2, a)
		return nil
	}})

	methods.Set("listen", &vm.GoFunc{Name: "server:listen", Fn: func(machine *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("server:listen", 1, a)
		addr := vm.StringArg("server:listen", 2, a)

		jobCh := make(chan job, jobChBuffer)
		errCh := make(chan error, 1)

		handler := func(w http.ResponseWriter, r *http.Request) {
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
			Addr:              addr,
			Handler:           http.HandlerFunc(handler),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		go func() {
			errCh <- srv.ListenAndServe()
		}()

		for {
			select {
			case err := <-errCh:
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

	methods.Set("stop", &vm.GoFunc{Name: "server:stop", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		_ = vm.TableArg("server:stop", 1, a)
		if srv == nil {
			return nil
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

func dispatch(machine *vm.VM, handler, notFound vm.Value, req *vm.Table) (out resp) {
	if handler == nil {
		if notFound == nil {
			return resp{status: http.StatusNotFound, body: "404 page not found\n",
				headers: map[string]string{"Content-Type": "text/plain; charset=utf-8"}}
		}
		handler = notFound
	}

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
		return resp{
			status: http.StatusInternalServerError,
			body:   fmt.Sprintf("server handler must return a string or table, got %s\n", vm.TypeName(result)),
			headers: map[string]string{
				"Content-Type": "text/plain; charset=utf-8",
			},
		}
	}
}

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
