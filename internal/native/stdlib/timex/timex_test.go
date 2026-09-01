package timex_test

import (
	"testing"

	"github.com/hilthontt/luascript/internal/compiler"
	"github.com/hilthontt/luascript/internal/compiler/parser"
	"github.com/hilthontt/luascript/internal/native/stdlib/timex"
	"github.com/hilthontt/luascript/internal/vm"
)

func runTime(t *testing.T, src string) *vm.VM {
	t.Helper()
	chunks, err := compiler.CompileToInstructions(src, parser.NormalMode)
	if err != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", err, src)
	}
	v := vm.New()
	timex.RegisterTimePreload(v)
	if err := v.Run(chunks[0]); err != nil {
		t.Fatalf("vm error: %v\nsource:\n%s", err, src)
	}
	return v
}

func TestParseFormatRoundTripsInOneZone(t *testing.T) {
	v := runTime(t, `
		local time = require("time")
		local stamp = "2024-03-15 08:30:00"
		local unix = time.parse(time.DATETIME, stamp)
		back = time.format(unix, time.DATETIME)
	`)
	if got := v.Globals.Get("back"); !vm.Equal(got, "2024-03-15 08:30:00") {
		t.Errorf("parse->format round trip = %v, want the original stamp back", got)
	}
}

func TestParseDateAgreeOnZone(t *testing.T) {
	v := runTime(t, `
		local time = require("time")
		local unix = time.parse(time.DATETIME, "2024-03-15 08:30:00")
		local d = time.date(unix)
		year = d.year
		month = d.month
		day = d.day
		hour = d.hour
		min = d.min
	`)
	for _, c := range []struct {
		name string
		want int64
	}{
		{"year", 2024}, {"month", 3}, {"day", 15}, {"hour", 8}, {"min", 30},
	} {
		if got := v.Globals.Get(c.name); !vm.Equal(got, c.want) {
			t.Errorf("date().%s = %v, want %d", c.name, got, c.want)
		}
	}
}

func TestParseHonoursExplicitZone(t *testing.T) {
	v := runTime(t, `
		local time = require("time")
		a = time.parse(time.RFC3339, "2024-03-15T08:30:00Z")
		b = time.parse(time.RFC3339, "2024-03-15T09:30:00+01:00")
	`)
	a, b := v.Globals.Get("a"), v.Globals.Get("b")
	if !vm.Equal(a, b) {
		t.Errorf("same instant in two zones parsed to %v and %v; want equal", a, b)
	}
}

func TestClockAndNowAreSane(t *testing.T) {
	v := runTime(t, `
		local time = require("time")
		now = time.now()
		ms = time.now_ms()
		c = time.clock()
	`)
	if got, ok := v.Globals.Get("now").(float64); !ok || got < 1.6e9 {
		t.Errorf("time.now() = %v, want unix seconds past 2020", v.Globals.Get("now"))
	}
	if got, ok := v.Globals.Get("ms").(int64); !ok || got < 1.6e12 {
		t.Errorf("time.now_ms() = %v, want unix millis past 2020", v.Globals.Get("ms"))
	}
	if got, ok := v.Globals.Get("c").(float64); !ok || got < 0 {
		t.Errorf("time.clock() = %v, want a non-negative elapsed value", v.Globals.Get("c"))
	}
}

func TestFormatAcceptsStrftime(t *testing.T) {
	v := runTime(t, `
		local time = require("time")
		local unix = time.parse(time.DATETIME, "2024-03-15 08:30:00")
		strf = time.format(unix, "%Y-%m-%d %H:%M:%S")
		goLayout = time.format(unix, time.DATETIME)
	`)
	if got := v.Globals.Get("strf"); !vm.Equal(got, "2024-03-15 08:30:00") {
		t.Errorf("strftime layout gave %v", got)
	}
	if got := v.Globals.Get("goLayout"); !vm.Equal(got, "2024-03-15 08:30:00") {
		t.Errorf("Go layout gave %v", got)
	}
}

func TestFormatAcceptsFractionalTimestamps(t *testing.T) {
	v := runTime(t, `
		local time = require("time")
		local unix = time.parse(time.DATETIME, "2024-03-15 08:30:00") + 0.75
		s = time.format(unix, "%Y-%m-%d %H:%M:%S")
		local d = time.date(unix)
		sec = d.sec
	`)
	if got := v.Globals.Get("s"); !vm.Equal(got, "2024-03-15 08:30:00") {
		t.Errorf("fractional timestamp formatted as %v, want the same second", got)
	}
	if got := v.Globals.Get("sec"); !vm.Equal(got, int64(0)) {
		t.Errorf("date().sec = %v, want 0", got)
	}
}

func TestParseUTCDiffersFromLocalByOffset(t *testing.T) {
	v := runTime(t, `
		local time = require("time")
		localUnix = time.parse(time.DATETIME, "2024-03-15 08:30:00")
		utcUnix = time.parse_utc(time.DATETIME, "2024-03-15 08:30:00")
		roundTrip = time.format(utcUnix, time.DATETIME, true)
	`)
	if got := v.Globals.Get("roundTrip"); !vm.Equal(got, "2024-03-15 08:30:00") {
		t.Errorf("parse_utc -> format(utc) = %v, want the original stamp", got)
	}
	l, lok := v.Globals.Get("localUnix").(int64)
	u, uok := v.Globals.Get("utcUnix").(int64)
	if !lok || !uok {
		t.Fatalf("expected integer timestamps, got %T and %T",
			v.Globals.Get("localUnix"), v.Globals.Get("utcUnix"))
	}
	if (u-l)%60 != 0 {
		t.Errorf("local and UTC parses differ by %d seconds, not a whole number of minutes", u-l)
	}
}

func TestDateUTCFlag(t *testing.T) {
	v := runTime(t, `
		local time = require("time")
		local unix = time.parse_utc(time.DATETIME, "2024-06-15 12:00:00")
		local d = time.date(unix, true)
		year, month, day, hour = d.year, d.month, d.day, d.hour
		hasIsdst = d.isdst ~= nil
	`)
	for _, c := range []struct {
		name string
		want int64
	}{{"year", 2024}, {"month", 6}, {"day", 15}, {"hour", 12}} {
		if got := v.Globals.Get(c.name); !vm.Equal(got, c.want) {
			t.Errorf("date(utc).%s = %v, want %d", c.name, got, c.want)
		}
	}
	if got := v.Globals.Get("hasIsdst"); !vm.Equal(got, true) {
		t.Error("date() has no isdst field")
	}
}

func TestIsDSTFalseInUTC(t *testing.T) {
	v := runTime(t, `
		local time = require("time")
		local summer = time.parse_utc(time.DATETIME, "2024-07-15 12:00:00")
		local winter = time.parse_utc(time.DATETIME, "2024-01-15 12:00:00")
		summerDST = time.date(summer, true).isdst
		winterDST = time.date(winter, true).isdst
	`)
	for _, name := range []string{"summerDST", "winterDST"} {
		if got := v.Globals.Get(name); !vm.Equal(got, false) {
			t.Errorf("%s = %v, want false (UTC has no DST)", name, got)
		}
	}
}
