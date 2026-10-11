package plot

import (
	"math"
	"strings"
	"testing"

	"github.com/hilthontt/luascript/internal/vm"
)

func TestNiceTicks(t *testing.T) {
	ticks := niceTicks(0, 10, 5)
	if len(ticks) < 2 {
		t.Fatalf("expected several ticks, got %v", ticks)
	}
	for i := 1; i < len(ticks); i++ {
		if ticks[i] <= ticks[i-1] {
			t.Fatalf("ticks not increasing: %v", ticks)
		}
	}
	if math.Abs((ticks[1]-ticks[0])-2) > 1e-9 {
		t.Fatalf("unexpected step %v (ticks %v)", ticks[1]-ticks[0], ticks)
	}
}

func TestDataBoundsIncludesZeroForBars(t *testing.T) {
	f := newFigure()
	f.series = append(f.series, &series{kind: "bar", xs: []float64{1, 2}, ys: []float64{5, 7}})
	xlo, xhi, ylo, yhi := f.dataBounds()
	if ylo != 0 {
		t.Fatalf("bar y-lower should include 0, got %v", ylo)
	}
	// 7 plus 5% headroom.
	if math.Abs(yhi-7.35) > 1e-9 {
		t.Fatalf("y-upper: got %v want 7.35", yhi)
	}
	// Half a slot either side keeps the outer bars inside the frame.
	if xlo != 0.5 || xhi != 2.5 {
		t.Fatalf("x bounds: got [%v, %v] want [0.5, 2.5]", xlo, xhi)
	}
}

func TestHistogramBinsFillFrame(t *testing.T) {
	f := newFigure()
	f.series = append(f.series, &series{kind: "bar", hist: true,
		xs: []float64{5, 15, 25}, ys: []float64{3, 3, 3}})
	xlo, xhi, _, yhi := f.dataBounds()
	if xlo != 0 || xhi != 30 {
		t.Fatalf("histogram x bounds: got [%v, %v] want the bin edges [0, 30]", xlo, xhi)
	}
	if yhi <= 3 {
		t.Fatalf("equal-count bins need headroom above them, got y-upper %v", yhi)
	}
}

func TestSeriesOptsAcceptsLabelString(t *testing.T) {
	f := newFigure()
	label, color := seriesOpts(f, []vm.Value{"growth"}, 1) // argAt is 1-based
	if label != "growth" || color == "" {
		t.Fatalf("got label %q color %q, want label \"growth\" and a palette color", label, color)
	}
}

func TestScatterPointsInsideFrame(t *testing.T) {
	f := newFigure()
	f.series = append(f.series, &series{kind: "scatter", xs: []float64{1, 20}, ys: []float64{1, 400}})
	xlo, xhi, ylo, yhi := f.dataBounds()
	if !(xlo < 1 && xhi > 20 && ylo < 1 && yhi > 400) {
		t.Fatalf("points should not sit on the frame: x [%v, %v] y [%v, %v]", xlo, xhi, ylo, yhi)
	}
}

func TestRenderSVGWellFormedish(t *testing.T) {
	f := newFigure()
	f.title = "t"
	f.xlabel = "x"
	f.ylabel = "y"
	f.series = append(f.series,
		&series{kind: "line", xs: []float64{1, 2, 3}, ys: []float64{1, 4, 9}, label: "sq", color: "#4e79a7"},
		&series{kind: "scatter", xs: []float64{1, 2, 3}, ys: []float64{2, 3, 5}, label: "obs", color: "#f28e2b"},
	)
	svg := f.renderSVG()
	for _, want := range []string{"<svg", "</svg>", "<polyline", "<circle", "sq", "obs", ">t<"} {
		if !strings.Contains(svg, want) {
			t.Fatalf("SVG missing %q:\n%s", want, svg)
		}
	}
	if strings.Count(svg, "<svg") != 1 || strings.Count(svg, "</svg>") != 1 {
		t.Fatalf("malformed svg envelope")
	}
}

func TestXMLEscape(t *testing.T) {
	if got := xmlEscape(`a<b>&"c`); got != `a&lt;b&gt;&amp;&quot;c` {
		t.Fatalf("escape: %q", got)
	}
}
