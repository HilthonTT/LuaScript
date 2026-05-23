// Package timex provides the `time` native module: wall-clock and
// monotonic time, sleeping, and date formatting/parsing.
//
// The Go package is named `timex` (not `time`) to avoid shadowing the
// standard library import within this file; the module is still exposed
// to scripts as `require("time")`.
package timex

import (
	"time"

	"github.com/hilthontt/sakura-lang/vm"
)

// RegisterTimePreload installs the `time` module under package.preload.
func RegisterTimePreload(v *vm.VM) {
	vm.RegisterPreload(v, "time", timeLoader)
}

func timeLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newTime()
	mod.Set("VERSION", "0.1.0")
	// Common layout strings, handy for time.format / time.parse.
	mod.Set("RFC3339", time.RFC3339)
	mod.Set("DATE", "2006-01-02")
	mod.Set("DATETIME", "2006-01-02 15:04:05")
	mod.Set("KITCHEN", time.Kitchen)
	return []vm.Value{mod}
}

func newTime() *vm.Table {
	m := vm.NewTable(0, 5)
	methods := vm.NewTable(0, 8)

	// start anchors the monotonic clock at module-load time.
	start := time.Now()

	// time.now() -> unix seconds as a float (sub-second precision).
	methods.Set("now", &vm.GoFunc{Name: "time:now", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{float64(time.Now().UnixNano()) / 1e9}
	}})

	// time.now_ms() -> unix milliseconds as an integer.
	methods.Set("now_ms", &vm.GoFunc{Name: "time:now_ms", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{time.Now().UnixMilli()}
	}})

	// time.clock() -> monotonic seconds elapsed since the module loaded.
	methods.Set("clock", &vm.GoFunc{Name: "time:clock", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{time.Since(start).Seconds()}
	}})

	// time.sleep(seconds) -> blocks the calling thread.
	methods.Set("sleep", &vm.GoFunc{Name: "time:sleep", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		secs := vm.FloatArg("time.sleep", 1, args)
		if secs > 0 {
			time.Sleep(time.Duration(secs * float64(time.Second)))
		}
		return nil
	}})

	// time.date(unix?) -> table { year, month, day, hour, min, sec, wday, yday }.
	// Defaults to the current local time when no argument is given.
	methods.Set("date", &vm.GoFunc{Name: "time:date", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		t := time.Now()
		if len(args) >= 1 && args[0] != nil {
			t = time.Unix(vm.IntArg("time.date", 1, args), 0)
		}
		d := vm.NewTable(0, 8)
		d.Set("year", int64(t.Year()))
		d.Set("month", int64(t.Month()))
		d.Set("day", int64(t.Day()))
		d.Set("hour", int64(t.Hour()))
		d.Set("min", int64(t.Minute()))
		d.Set("sec", int64(t.Second()))
		// wday: 1 = Sunday .. 7 = Saturday, matching Lua's os.date("*t").
		d.Set("wday", int64(t.Weekday())+1)
		d.Set("yday", int64(t.YearDay()))
		return []vm.Value{d}
	}})

	// time.format(unix, layout?) -> string. layout defaults to RFC3339.
	methods.Set("format", &vm.GoFunc{Name: "time:format", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		unix := vm.IntArg("time.format", 1, args)
		layout := vm.OptString("time.format", 2, args, time.RFC3339)
		return []vm.Value{time.Unix(unix, 0).Format(layout)}
	}})

	// time.parse(layout, str) -> unix seconds. A malformed string raises.
	methods.Set("parse", &vm.GoFunc{Name: "time:parse", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		layout := vm.StringArg("time.parse", 1, args)
		str := vm.StringArg("time.parse", 2, args)
		t, err := time.Parse(layout, str)
		if err != nil {
			panic(vm.Errorf("time.parse: %s", err.Error()))
		}
		return []vm.Value{t.Unix()}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}
