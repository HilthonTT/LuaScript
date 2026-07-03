// Package plot is a require()-able host module that turns numeric data into
// standalone SVG charts — the "see your data" half of a data-science loop.
// It has no external dependencies: charts are rendered by hand to an SVG
// string that any browser or image viewer can open, and can be written
// straight to a .svg file.
//
// The central object is a Figure, which accumulates one or more series and
// renders them onto shared, auto-ranged axes:
//
//	local plot = require("plot")
//	local fig = plot.figure({ title = "demo", xlabel = "x", ylabel = "y" })
//	fig:line({1,2,3,4}, {1,4,9,16}, { label = "sq", color = "#e15759" })
//	fig:scatter({1,2,3,4}, {2,3,5,7}, { label = "obs" })
//	fig:save("demo.svg")
//
// Convenience one-liners build a single-series figure directly:
//
//	plot.line({1,2,3}, {2,4,6}):save("line.svg")
//	plot.histogram({1,1,2,3,3,3,4}, 4):save("hist.svg")
//
// Charts render on an explicit white background with dark axes so they read
// correctly regardless of the viewer's theme.
package plot

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/hilthontt/luascript/vm"
)

// RegisterPlotPreload installs the loader under package.preload.
func RegisterPlotPreload(v *vm.VM) {
	vm.RegisterPreload(v, "plot", plotLoader)
}

// palette is a categorical, reasonably colorblind-aware series color cycle.
var palette = []string{
	"#4e79a7", "#f28e2b", "#e15759", "#76b7b2",
	"#59a14f", "#edc948", "#b07aa1", "#ff9da7",
}

type series struct {
	kind  string // "line" | "scatter" | "bar"
	xs    []float64
	ys    []float64
	label string
	color string
}

type figure struct {
	title, xlabel, ylabel string
	width, height         int
	series                []*series
	xcats                 []string // categorical x tick labels (bar charts)
}

func newFigure() *figure { return &figure{width: 640, height: 400} }

func (f *figure) nextColor() string { return palette[len(f.series)%len(palette)] }

// ---------------------------------------------------------------------------
// Loader + Figure wrapping
// ---------------------------------------------------------------------------

const figKey = "\x00plotfig"

var (
	figMeta    *vm.Table
	figMethods *vm.Table
	metaOnce   sync.Once
)

func plotLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	m := vm.NewTable(0, 8)
	set := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		m.Set(name, &vm.GoFunc{Name: "plot." + name, Fn: fn})
	}

	set("figure", func(_ *vm.VM, args []vm.Value) []vm.Value {
		f := newFigure()
		if opts, ok := argAt(args, 1).(*vm.Table); ok {
			applyFigureOpts(f, opts)
		}
		return []vm.Value{wrap(f)}
	})

	// One-liner constructors: build a single-series figure and return it so
	// callers can immediately :save() or :to_svg().
	oneShot := func(name, kind string) {
		set(name, func(_ *vm.VM, args []vm.Value) []vm.Value {
			f := newFigure()
			addXYSeries(f, "plot."+name, kind, args, 1)
			return []vm.Value{wrap(f)}
		})
	}
	oneShot("line", "line")
	oneShot("scatter", "scatter")
	set("bar", func(_ *vm.VM, args []vm.Value) []vm.Value {
		f := newFigure()
		addBarSeries(f, "plot.bar", args, 1)
		return []vm.Value{wrap(f)}
	})
	set("histogram", func(_ *vm.VM, args []vm.Value) []vm.Value {
		f := newFigure()
		addHistogram(f, "plot.histogram", args, 1)
		return []vm.Value{wrap(f)}
	})

	m.Set("VERSION", "0.1.0")
	return []vm.Value{m}
}

func wrap(f *figure) *vm.Table {
	metaOnce.Do(buildMeta)
	t := vm.NewTable(0, 1)
	t.Set(figKey, f)
	t.SetMetatable(figMeta)
	return t
}

func selfFig(site string, args []vm.Value) *figure {
	if len(args) == 0 {
		panic(vm.Errorf("%s: called without a receiver (use fig:method())", site))
	}
	t, ok := args[0].(*vm.Table)
	if !ok {
		panic(vm.Errorf("%s: receiver is not a figure", site))
	}
	f, ok := t.Get(figKey).(*figure)
	if !ok {
		panic(vm.Errorf("%s: receiver is not a figure", site))
	}
	return f
}

func buildMeta() {
	figMethods = vm.NewTable(0, 12)
	set := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		figMethods.Set(name, &vm.GoFunc{Name: "figure:" + name, Fn: fn})
	}

	// Series builders return the receiver so calls chain: fig:line(...):scatter(...).
	set("line", func(_ *vm.VM, args []vm.Value) []vm.Value {
		f := selfFig("figure:line", args)
		addXYSeries(f, "figure:line", "line", args, 2)
		return []vm.Value{args[0]}
	})
	set("scatter", func(_ *vm.VM, args []vm.Value) []vm.Value {
		f := selfFig("figure:scatter", args)
		addXYSeries(f, "figure:scatter", "scatter", args, 2)
		return []vm.Value{args[0]}
	})
	set("bar", func(_ *vm.VM, args []vm.Value) []vm.Value {
		f := selfFig("figure:bar", args)
		addBarSeries(f, "figure:bar", args, 2)
		return []vm.Value{args[0]}
	})
	set("histogram", func(_ *vm.VM, args []vm.Value) []vm.Value {
		f := selfFig("figure:histogram", args)
		addHistogram(f, "figure:histogram", args, 2)
		return []vm.Value{args[0]}
	})

	// Chainable setters.
	setter := func(name string, apply func(*figure, vm.Value)) {
		set(name, func(_ *vm.VM, args []vm.Value) []vm.Value {
			apply(selfFig("figure:"+name, args), argAt(args, 2))
			return []vm.Value{args[0]}
		})
	}
	setter("title", func(f *figure, v vm.Value) { f.title = asString(v) })
	setter("xlabel", func(f *figure, v vm.Value) { f.xlabel = asString(v) })
	setter("ylabel", func(f *figure, v vm.Value) { f.ylabel = asString(v) })
	set("size", func(_ *vm.VM, args []vm.Value) []vm.Value {
		f := selfFig("figure:size", args)
		f.width = int(vm.IntArg("figure:size", 2, args))
		f.height = int(vm.IntArg("figure:size", 3, args))
		return []vm.Value{args[0]}
	})

	set("to_svg", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{selfFig("figure:to_svg", args).renderSVG()}
	})
	set("save", func(_ *vm.VM, args []vm.Value) []vm.Value {
		f := selfFig("figure:save", args)
		path := vm.StringArg("figure:save", 2, args)
		if err := os.WriteFile(path, []byte(f.renderSVG()), 0o644); err != nil {
			panic(vm.Errorf("figure:save: %s", err.Error()))
		}
		return []vm.Value{args[0]}
	})

	figMeta = vm.NewTable(0, 2)
	figMeta.Set("__index", figMethods)
	figMeta.Set("__tostring", &vm.GoFunc{Name: "figure:__tostring", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		f := selfFig("figure:__tostring", args)
		return []vm.Value{fmt.Sprintf("Figure(%q, %d series, %dx%d)", f.title, len(f.series), f.width, f.height)}
	}})
}

// ---------------------------------------------------------------------------
// Argument parsing / series construction
// ---------------------------------------------------------------------------

func argAt(args []vm.Value, i int) vm.Value {
	if i < 1 || i > len(args) {
		return nil
	}
	return args[i-1]
}

func applyFigureOpts(f *figure, opts *vm.Table) {
	if s, ok := opts.Get("title").(string); ok {
		f.title = s
	}
	if s, ok := opts.Get("xlabel").(string); ok {
		f.xlabel = s
	}
	if s, ok := opts.Get("ylabel").(string); ok {
		f.ylabel = s
	}
	if w, ok := vm.ToInteger(opts.Get("width")); ok {
		f.width = int(w)
	}
	if h, ok := vm.ToInteger(opts.Get("height")); ok {
		f.height = int(h)
	}
}

// numsFromTable reads the 1..n numeric portion of a Lua array.
func numsFromTable(site string, t *vm.Table) []float64 {
	n := int(t.Len())
	out := make([]float64, n)
	for i := 1; i <= n; i++ {
		f, ok := vm.ToFloat(t.Get(int64(i)))
		if !ok {
			panic(vm.Errorf("%s: element %d is not a number", site, i))
		}
		out[i-1] = f
	}
	return out
}

// seriesOpts pulls the label/color from an optional opts table at the given
// position and returns them (color defaulting to the figure's next color).
func seriesOpts(f *figure, args []vm.Value, pos int) (label, color string) {
	color = f.nextColor()
	if opts, ok := argAt(args, pos).(*vm.Table); ok {
		if s, ok := opts.Get("label").(string); ok {
			label = s
		}
		if s, ok := opts.Get("color").(string); ok {
			color = s
		}
	}
	return
}

// addXYSeries handles line/scatter. Accepts (xs, ys[, opts]) or (ys[, opts]),
// where a lone array becomes ys plotted against 1..n. `base` is the 1-based
// position of the first data argument (2 for methods, 1 for module funcs).
func addXYSeries(f *figure, site, kind string, args []vm.Value, base int) {
	first := vm.TableArg(site, base, args)
	var xs, ys []float64
	optPos := base + 1
	if second, ok := argAt(args, base+1).(*vm.Table); ok {
		xs = numsFromTable(site, first)
		ys = numsFromTable(site, second)
		optPos = base + 2
	} else {
		ys = numsFromTable(site, first)
		xs = make([]float64, len(ys))
		for i := range xs {
			xs[i] = float64(i + 1)
		}
	}
	if len(xs) != len(ys) {
		panic(vm.Errorf("%s: x and y have different lengths (%d vs %d)", site, len(xs), len(ys)))
	}
	label, color := seriesOpts(f, args, optPos)
	f.series = append(f.series, &series{kind: kind, xs: xs, ys: ys, label: label, color: color})
}

// addBarSeries handles bars. The first argument may be an array of category
// strings (categorical x) or numbers (positional x); the second is the
// heights.
func addBarSeries(f *figure, site string, args []vm.Value, base int) {
	first := vm.TableArg(site, base, args)
	var ys []float64
	var xs []float64
	optPos := base + 1
	if second, ok := argAt(args, base+1).(*vm.Table); ok {
		// (labels_or_x, heights)
		ys = numsFromTable(site, second)
		if cats, ok := stringsFromTable(first); ok {
			f.xcats = cats
			xs = positions(len(ys))
		} else {
			xs = numsFromTable(site, first)
		}
		optPos = base + 2
	} else {
		// (heights) only
		ys = numsFromTable(site, first)
		xs = positions(len(ys))
	}
	if len(xs) != len(ys) {
		panic(vm.Errorf("%s: labels and heights have different lengths (%d vs %d)", site, len(xs), len(ys)))
	}
	label, color := seriesOpts(f, args, optPos)
	f.series = append(f.series, &series{kind: "bar", xs: xs, ys: ys, label: label, color: color})
}

// addHistogram bins raw values into `bins` equal-width buckets and adds a bar
// series of counts positioned at bin centers.
func addHistogram(f *figure, site string, args []vm.Value, base int) {
	vals := numsFromTable(site, vm.TableArg(site, base, args))
	bins := 10
	if b := argAt(args, base+1); b != nil {
		if bi, ok := vm.ToInteger(b); ok {
			bins = int(bi)
		}
	}
	if bins < 1 {
		panic(vm.Errorf("%s: bins must be >= 1", site))
	}
	if len(vals) == 0 {
		f.series = append(f.series, &series{kind: "bar", color: f.nextColor()})
		return
	}
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	if lo == hi {
		hi = lo + 1
	}
	w := (hi - lo) / float64(bins)
	counts := make([]float64, bins)
	centers := make([]float64, bins)
	for i := 0; i < bins; i++ {
		centers[i] = lo + w*(float64(i)+0.5)
	}
	for _, v := range vals {
		idx := int((v - lo) / w)
		if idx >= bins {
			idx = bins - 1 // include the right edge in the last bin
		}
		if idx < 0 {
			idx = 0
		}
		counts[idx]++
	}
	label, color := seriesOpts(f, args, base+2)
	f.series = append(f.series, &series{kind: "bar", xs: centers, ys: counts, label: label, color: color})
}

func positions(n int) []float64 {
	xs := make([]float64, n)
	for i := range xs {
		xs[i] = float64(i + 1)
	}
	return xs
}

func stringsFromTable(t *vm.Table) ([]string, bool) {
	n := int(t.Len())
	if n == 0 {
		return nil, false
	}
	out := make([]string, n)
	for i := 1; i <= n; i++ {
		s, ok := t.Get(int64(i)).(string)
		if !ok {
			return nil, false
		}
		out[i-1] = s
	}
	return out, true
}

func asString(v vm.Value) string {
	if s, ok := v.(string); ok {
		return s
	}
	return vm.ToString(v)
}

// ---------------------------------------------------------------------------
// SVG rendering
// ---------------------------------------------------------------------------

func (f *figure) dataBounds() (xlo, xhi, ylo, yhi float64) {
	xlo, ylo = math.Inf(1), math.Inf(1)
	xhi, yhi = math.Inf(-1), math.Inf(-1)
	any := false
	for _, s := range f.series {
		for i := range s.xs {
			any = true
			xlo, xhi = math.Min(xlo, s.xs[i]), math.Max(xhi, s.xs[i])
			ylo, yhi = math.Min(ylo, s.ys[i]), math.Max(yhi, s.ys[i])
		}
	}
	if !any {
		return 0, 1, 0, 1
	}
	// Bars are drawn from the y baseline, so always include 0.
	if f.hasBars() {
		ylo = math.Min(ylo, 0)
		yhi = math.Max(yhi, 0)
	}
	if xlo == xhi {
		xlo, xhi = xlo-1, xhi+1
	}
	if ylo == yhi {
		ylo, yhi = ylo-1, yhi+1
	}
	return
}

func (f *figure) hasBars() bool {
	for _, s := range f.series {
		if s.kind == "bar" {
			return true
		}
	}
	return false
}

func (f *figure) barSeriesCount() int {
	n := 0
	for _, s := range f.series {
		if s.kind == "bar" {
			n++
		}
	}
	return n
}

func (f *figure) renderSVG() string {
	W, H := f.width, f.height
	// Margins: leave room for title, axis labels, and tick labels.
	mL, mR, mT, mB := 60, 20, 20, 44
	if f.title != "" {
		mT = 44
	}
	if f.ylabel != "" {
		mL = 72
	}
	// Room for a legend on the right if any series is labeled.
	legend := f.legendEntries()
	if len(legend) > 0 {
		mR = 130
	}
	plotW := W - mL - mR
	plotH := H - mT - mB
	xlo, xhi, ylo, yhi := f.dataBounds()

	sx := func(x float64) float64 { return float64(mL) + (x-xlo)/(xhi-xlo)*float64(plotW) }
	sy := func(y float64) float64 { return float64(mT) + (1-(y-ylo)/(yhi-ylo))*float64(plotH) }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" font-family="sans-serif">`, W, H, W, H)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#ffffff"/>`, W, H)

	// Gridlines + tick labels.
	xticks := niceTicks(xlo, xhi, 6)
	yticks := niceTicks(ylo, yhi, 6)
	for _, ty := range yticks {
		py := sy(ty)
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#e6e6e6" stroke-width="1"/>`,
			float64(mL), py, float64(mL+plotW), py)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="11" fill="#333" text-anchor="end" dominant-baseline="middle">%s</text>`,
			float64(mL-6), py, fmtTick(ty))
	}
	// X ticks: categorical labels for bar-only figures, else numeric.
	if f.xcats != nil {
		for i, lbl := range f.xcats {
			px := sx(float64(i + 1))
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="11" fill="#333" text-anchor="middle">%s</text>`,
				px, float64(mT+plotH+16), xmlEscape(lbl))
		}
	} else {
		for _, tx := range xticks {
			px := sx(tx)
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#e6e6e6" stroke-width="1"/>`,
				px, float64(mT), px, float64(mT+plotH))
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="11" fill="#333" text-anchor="middle">%s</text>`,
				px, float64(mT+plotH+16), fmtTick(tx))
		}
	}

	// Axes frame.
	fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="none" stroke="#333" stroke-width="1"/>`,
		mL, mT, plotW, plotH)
	// Zero line, if 0 is within the y range and not on the frame.
	if ylo < 0 && yhi > 0 {
		p0 := sy(0)
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#999" stroke-width="1"/>`,
			float64(mL), p0, float64(mL+plotW), p0)
	}

	// Series.
	barIdx := 0
	barTotal := f.barSeriesCount()
	baseline := sy(math.Max(0, ylo))
	for _, s := range f.series {
		switch s.kind {
		case "line":
			if len(s.xs) == 0 {
				continue
			}
			var pts strings.Builder
			for i := range s.xs {
				if i > 0 {
					pts.WriteByte(' ')
				}
				fmt.Fprintf(&pts, "%.2f,%.2f", sx(s.xs[i]), sy(s.ys[i]))
			}
			fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="2"/>`, pts.String(), s.color)
		case "scatter":
			for i := range s.xs {
				fmt.Fprintf(&b, `<circle cx="%.2f" cy="%.2f" r="3.5" fill="%s"/>`, sx(s.xs[i]), sy(s.ys[i]), s.color)
			}
		case "bar":
			// Group bars from different series side by side within each slot.
			slot := f.slotWidth(plotW)
			groupW := slot * 0.8
			barW := groupW / math.Max(1, float64(barTotal))
			for i := range s.xs {
				cx := sx(s.xs[i])
				x := cx - groupW/2 + float64(barIdx)*barW
				y := sy(s.ys[i])
				top, h := y, baseline-y
				if h < 0 {
					top, h = baseline, -h
				}
				fmt.Fprintf(&b, `<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" fill="%s"/>`,
					x, top, math.Max(1, barW-1), h, s.color)
			}
			barIdx++
		}
	}

	// Title & axis labels.
	if f.title != "" {
		fmt.Fprintf(&b, `<text x="%.1f" y="24" font-size="15" font-weight="bold" fill="#111" text-anchor="middle">%s</text>`,
			float64(mL+plotW)/2+float64(mL)/2, xmlEscape(f.title))
	}
	if f.xlabel != "" {
		fmt.Fprintf(&b, `<text x="%.1f" y="%d" font-size="12" fill="#111" text-anchor="middle">%s</text>`,
			float64(mL)+float64(plotW)/2, H-8, xmlEscape(f.xlabel))
	}
	if f.ylabel != "" {
		cy := float64(mT) + float64(plotH)/2
		fmt.Fprintf(&b, `<text x="16" y="%.1f" font-size="12" fill="#111" text-anchor="middle" transform="rotate(-90 16 %.1f)">%s</text>`,
			cy, cy, xmlEscape(f.ylabel))
	}

	// Legend.
	if len(legend) > 0 {
		lx := mL + plotW + 14
		ly := mT + 4
		for i, e := range legend {
			y := ly + i*18
			fmt.Fprintf(&b, `<rect x="%d" y="%d" width="12" height="12" fill="%s"/>`, lx, y, e.color)
			fmt.Fprintf(&b, `<text x="%d" y="%d" font-size="11" fill="#111" dominant-baseline="middle">%s</text>`,
				lx+18, y+6, xmlEscape(e.label))
		}
	}

	b.WriteString(`</svg>`)
	return b.String()
}

// slotWidth is the horizontal pixel span allotted to one bar category.
func (f *figure) slotWidth(plotW int) float64 {
	// Count distinct bar x positions (assume all bar series share them).
	n := 0
	for _, s := range f.series {
		if s.kind == "bar" && len(s.xs) > n {
			n = len(s.xs)
		}
	}
	if n < 1 {
		n = 1
	}
	return float64(plotW) / float64(n+1)
}

type legendEntry struct{ label, color string }

func (f *figure) legendEntries() []legendEntry {
	var out []legendEntry
	for _, s := range f.series {
		if s.label != "" {
			out = append(out, legendEntry{s.label, s.color})
		}
	}
	return out
}

// niceTicks returns rounded tick positions spanning [lo, hi] with about
// `want` intervals, using the 1-2-5 step progression.
func niceTicks(lo, hi float64, want int) []float64 {
	if hi <= lo || want < 1 {
		return []float64{lo}
	}
	raw := (hi - lo) / float64(want)
	mag := math.Pow(10, math.Floor(math.Log10(raw)))
	norm := raw / mag
	var step float64
	switch {
	case norm < 1.5:
		step = 1
	case norm < 3:
		step = 2
	case norm < 7:
		step = 5
	default:
		step = 10
	}
	step *= mag
	start := math.Ceil(lo/step) * step
	var ticks []float64
	for t := start; t <= hi+step*1e-9; t += step {
		ticks = append(ticks, t)
	}
	return ticks
}

func fmtTick(x float64) string {
	if x == 0 {
		return "0"
	}
	if x == math.Trunc(x) && math.Abs(x) < 1e15 {
		return strconv.FormatInt(int64(x), 10)
	}
	return strconv.FormatFloat(x, 'g', 4, 64)
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
