package ndarray

import (
	"math"
	"strconv"
	"strings"
)

const edgeItems = 3

func fmtNum(x float64) string {
	if x == math.Trunc(x) && !math.IsInf(x, 0) && math.Abs(x) < 1e15 {
		return strconv.FormatInt(int64(x), 10)
	}
	return strconv.FormatFloat(x, 'g', 6, 64)
}

func (a *ndarray) render() string {
	if a.ndim() == 0 {
		return "ndarray(" + fmtNum(a.data[0]) + ")"
	}
	width := 0
	for _, x := range a.data {
		if l := len(fmtNum(x)); l > width {
			width = l
		}
	}
	strides := rowStrides(a.shape)
	var render func(axis, base int) string
	render = func(axis, base int) string {
		n := a.shape[axis]
		if axis == len(a.shape)-1 {
			var sb strings.Builder
			sb.WriteByte('[')
			writeCell := func(k int) {
				s := fmtNum(a.data[base+k*strides[axis]])
				sb.WriteString(strings.Repeat(" ", width-len(s)))
				sb.WriteString(s)
			}
			if n > 2*edgeItems {
				for k := 0; k < edgeItems; k++ {
					if k > 0 {
						sb.WriteByte(' ')
					}
					writeCell(k)
				}
				sb.WriteString(" ... ")
				for k := n - edgeItems; k < n; k++ {
					if k > n-edgeItems {
						sb.WriteByte(' ')
					}
					writeCell(k)
				}
			} else {
				for k := 0; k < n; k++ {
					if k > 0 {
						sb.WriteByte(' ')
					}
					writeCell(k)
				}
			}
			sb.WriteByte(']')
			return sb.String()
		}
		indent := strings.Repeat(" ", axis+1)
		sep := ",\n" + strings.Repeat("\n", len(a.shape)-axis-2) + indent
		rows := func(k int) string { return render(axis+1, base+k*strides[axis]) }
		var parts []string
		if n > 2*edgeItems {
			for k := 0; k < edgeItems; k++ {
				parts = append(parts, rows(k))
			}
			parts = append(parts, "...")
			for k := n - edgeItems; k < n; k++ {
				parts = append(parts, rows(k))
			}
		} else {
			for k := 0; k < n; k++ {
				parts = append(parts, rows(k))
			}
		}
		return "[" + strings.Join(parts, sep) + "]"
	}
	return render(0, 0)
}
