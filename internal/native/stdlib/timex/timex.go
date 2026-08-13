// Package timex provides the `time` native module: wall-clock and
// monotonic time, sleeping, and date formatting/parsing.
//
// The Go package is named `timex` (not `time`) to avoid shadowing the
// standard library import within this file; the module is still exposed
// to scripts as `require("time")`.
package timex

import (
	"math"
	"strings"
	"time"

	osNative "github.com/hilthontt/luascript/internal/native/stdlib/os"
	"github.com/hilthontt/luascript/internal/vm"
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

	// time.date(unix?, utc?) -> table { year, month, day, hour, min, sec,
	// wday, yday, isdst }. Defaults to the current local time; pass utc = true
	// for the UTC breakdown of the same instant.
	methods.Set("date", &vm.GoFunc{Name: "time:date", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		t := time.Now()
		if len(args) >= 1 && args[0] != nil {
			t = unixFromValue("time.date", args, 1)
		}
		if len(args) >= 2 && vm.IsTruthy(args[1]) {
			t = t.UTC()
		}
		d := vm.NewTable(0, 9)
		d.Set("year", int64(t.Year()))
		d.Set("month", int64(t.Month()))
		d.Set("day", int64(t.Day()))
		d.Set("hour", int64(t.Hour()))
		d.Set("min", int64(t.Minute()))
		d.Set("sec", int64(t.Second()))
		// wday: 1 = Sunday .. 7 = Saturday, matching Lua's os.date("*t").
		d.Set("wday", int64(t.Weekday())+1)
		d.Set("yday", int64(t.YearDay()))
		// isdst completes the os.date("*t") shape. Go has no direct accessor,
		// so it is inferred by comparing this instant's zone offset with the
		// one in force in January, which is standard time in every zone that
		// observes DST.
		d.Set("isdst", isDST(t))
		return []vm.Value{d}
	}})

	// time.format(unix, layout?, utc?) -> string. layout defaults to RFC3339.
	//
	// A layout containing a '%' is treated as strftime, so time.format(t,
	// "%Y-%m-%d") works. Go's reference-time layouts ("2006-01-02") still
	// work and remain the default; requiring them was a leak of the host
	// language into the script surface, since nothing about that spelling is
	// guessable.
	methods.Set("format", &vm.GoFunc{Name: "time:format", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		t := unixFromValue("time.format", args, 1)
		layout := vm.OptString("time.format", 2, args, time.RFC3339)
		if len(args) >= 3 && vm.IsTruthy(args[2]) {
			t = t.UTC()
		}
		if strings.ContainsRune(layout, '%') {
			return []vm.Value{osNative.Strftime(layout, t)}
		}
		return []vm.Value{t.Format(layout)}
	}})

	// time.parse(layout, str) -> unix seconds. A malformed string raises.
	//
	// Parsed in the local zone (time.ParseInLocation, not time.Parse) so that
	// parse and date/format agree. time.Parse defaults a zone-less layout to
	// UTC while date/format render local, which silently shifted every
	// round-trip through this module by the host's UTC offset. A layout that
	// does carry an explicit zone still wins — ParseInLocation only supplies
	// the default.
	methods.Set("parse", &vm.GoFunc{Name: "time:parse", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		layout := vm.StringArg("time.parse", 1, args)
		str := vm.StringArg("time.parse", 2, args)
		t, err := time.ParseInLocation(layout, str, time.Local)
		if err != nil {
			panic(vm.Errorf("time.parse: %s", err.Error()))
		}
		return []vm.Value{t.Unix()}
	}})

	// time.parse_utc(layout, str) -> unix seconds, reading a zone-less layout
	// as UTC. time.parse reads it as local; a timestamp that is known to be
	// UTC (a log line, an API field) needs the other one, and appending a "Z"
	// to make RFC3339 do it is not always possible.
	methods.Set("parse_utc", &vm.GoFunc{Name: "time:parse_utc", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		layout := vm.StringArg("time.parse_utc", 1, args)
		str := vm.StringArg("time.parse_utc", 2, args)
		t, err := time.Parse(layout, str)
		if err != nil {
			panic(vm.Errorf("time.parse_utc: %s", err.Error()))
		}
		return []vm.Value{t.Unix()}
	}})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

// unixFromValue reads a unix timestamp argument, accepting the fractional
// seconds time.now() returns as well as whole seconds. Truncating a float
// argument to int64 (as IntArg would) silently discarded the sub-second part
// that now() had just produced.
func unixFromValue(site string, args []vm.Value, idx int) time.Time {
	if f, ok := args[idx-1].(float64); ok {
		sec, frac := math.Modf(f)
		return time.Unix(int64(sec), int64(frac*1e9))
	}
	return time.Unix(vm.IntArg(site, idx, args), 0)
}

// isDST reports whether t is in daylight saving time in its own zone. Go
// exposes no direct accessor, so this compares t's UTC offset with January's
// in the same location — standard time everywhere that observes DST. Zones
// with no DST give equal offsets and correctly report false.
func isDST(t time.Time) bool {
	jan := time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, t.Location())
	_, tOffset := t.Zone()
	_, janOffset := jan.Zone()
	return tOffset != janOffset
}
