package timex

import (
	"math"
	"strings"
	"time"

	osNative "github.com/hilthontt/luascript/internal/native/stdlib/os"
	"github.com/hilthontt/luascript/internal/vm"
)

func RegisterTimePreload(v *vm.VM) {
	vm.RegisterPreload(v, "time", timeLoader)
}

func timeLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := newTime()
	mod.Set("VERSION", "0.1.0")
	mod.Set("RFC3339", time.RFC3339)
	mod.Set("DATE", "2006-01-02")
	mod.Set("DATETIME", "2006-01-02 15:04:05")
	mod.Set("KITCHEN", time.Kitchen)
	return []vm.Value{mod}
}

func newTime() *vm.Table {
	m := vm.NewTable(0, 5)
	methods := vm.NewTable(0, 8)

	start := time.Now()

	methods.Set("now", &vm.GoFunc{Name: "time:now", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{float64(time.Now().UnixNano()) / 1e9}
	}})

	methods.Set("now_ms", &vm.GoFunc{Name: "time:now_ms", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{time.Now().UnixMilli()}
	}})

	methods.Set("clock", &vm.GoFunc{Name: "time:clock", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{time.Since(start).Seconds()}
	}})

	methods.Set("sleep", &vm.GoFunc{Name: "time:sleep", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		secs := vm.FloatArg("time.sleep", 1, args)
		if secs > 0 {
			time.Sleep(time.Duration(secs * float64(time.Second)))
		}
		return nil
	}})

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
		d.Set("wday", int64(t.Weekday())+1)
		d.Set("yday", int64(t.YearDay()))
		d.Set("isdst", isDST(t))
		return []vm.Value{d}
	}})

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

	methods.Set("parse", &vm.GoFunc{Name: "time:parse", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		layout := vm.StringArg("time.parse", 1, args)
		str := vm.StringArg("time.parse", 2, args)
		t, err := time.ParseInLocation(layout, str, time.Local)
		if err != nil {
			panic(vm.Errorf("time.parse: %s", err.Error()))
		}
		return []vm.Value{t.Unix()}
	}})

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

func unixFromValue(site string, args []vm.Value, idx int) time.Time {
	if f, ok := args[idx-1].(float64); ok {
		sec, frac := math.Modf(f)
		return time.Unix(int64(sec), int64(frac*1e9))
	}
	return time.Unix(vm.IntArg(site, idx, args), 0)
}

func isDST(t time.Time) bool {
	jan := time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, t.Location())
	_, tOffset := t.Zone()
	_, janOffset := jan.Zone()
	return tOffset != janOffset
}
