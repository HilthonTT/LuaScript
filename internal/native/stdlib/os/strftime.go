package os

import (
	"fmt"
	"strings"
	"time"

	"github.com/hilthontt/luascript/internal/vm"
)

// tableInt fetches an integer field from t with a default fallback. Used
// by os.time to read the year/month/day/... fields out of a calendar
// table without panicking when a field is missing.
func tableInt(t *vm.Table, key string, def int64) int64 {
	v := t.Get(key)
	if v == nil {
		return def
	}
	if n, ok := vm.ToInteger(v); ok {
		return n
	}
	return def
}

// strftime renders t against a C-strftime-style format. Only the subset
// of conversions used by Lua scripts is implemented; unrecognised codes
// pass through unchanged so the user notices ("the %Q didn't expand") in
// a single place rather than getting a silent empty string.
//
// Supported codes (matching POSIX strftime where reasonable):
//
//	%Y  4-digit year                  %y  2-digit year
//	%m  month (01-12)                 %B  full month name
//	%b  abbrev month name             %d  day of month (01-31)
//	%e  day of month, space-padded    %j  day of year (001-366)
//	%H  hour (00-23)                  %I  hour (01-12)
//	%M  minute (00-59)                %S  second (00-59)
//	%p  AM/PM                         %A  full weekday name
//	%a  abbrev weekday name           %w  weekday (0-6, Sun=0)
//	%u  ISO weekday (1-7, Mon=1)      %Z  timezone name
//	%z  timezone offset (+/-HHMM)     %c  default date+time
//	%x  default date                  %X  default time
//	%%  literal %
func strftime(format string, t time.Time) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' || i+1 >= len(format) {
			b.WriteByte(c)
			continue
		}
		i++
		switch format[i] {
		case 'Y':
			fmt.Fprintf(&b, "%04d", t.Year())
		case 'y':
			fmt.Fprintf(&b, "%02d", t.Year()%100)
		case 'm':
			fmt.Fprintf(&b, "%02d", int(t.Month()))
		case 'B':
			b.WriteString(t.Month().String())
		case 'b', 'h':
			b.WriteString(t.Month().String()[:3])
		case 'd':
			fmt.Fprintf(&b, "%02d", t.Day())
		case 'e':
			fmt.Fprintf(&b, "%2d", t.Day())
		case 'j':
			fmt.Fprintf(&b, "%03d", t.YearDay())
		case 'H':
			fmt.Fprintf(&b, "%02d", t.Hour())
		case 'I':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			fmt.Fprintf(&b, "%02d", h)
		case 'M':
			fmt.Fprintf(&b, "%02d", t.Minute())
		case 'S':
			fmt.Fprintf(&b, "%02d", t.Second())
		case 'p':
			if t.Hour() < 12 {
				b.WriteString("AM")
			} else {
				b.WriteString("PM")
			}
		case 'A':
			b.WriteString(t.Weekday().String())
		case 'a':
			b.WriteString(t.Weekday().String()[:3])
		case 'w':
			fmt.Fprintf(&b, "%d", int(t.Weekday()))
		case 'u':
			d := int(t.Weekday())
			if d == 0 {
				d = 7
			}
			fmt.Fprintf(&b, "%d", d)
		case 'Z':
			b.WriteString(t.Format("MST"))
		case 'z':
			b.WriteString(t.Format("-0700"))
		case 'c':
			// "Mon Jan  2 15:04:05 2006" — POSIX default.
			b.WriteString(t.Format("Mon Jan _2 15:04:05 2006"))
		case 'x':
			b.WriteString(t.Format("01/02/06"))
		case 'X':
			b.WriteString(t.Format("15:04:05"))
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case '%':
			b.WriteByte('%')
		default:
			// Unknown code: preserve verbatim so the script sees the typo.
			b.WriteByte('%')
			b.WriteByte(format[i])
		}
	}
	return b.String()
}
