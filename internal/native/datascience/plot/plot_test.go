package plot

import (
	"math"
	"strings"
	"testing"
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
	// Step should be a 1-2-5 multiple; for [0,10]/5 that is 2.
	if math.Abs((ticks[1]-ticks[0])-2) > 1e-9 {
		t.Fatalf("unexpected step %v (ticks %v)", ticks[1]-ticks[0], ticks)
	}
}

func TestDataBoundsIncludesZeroForBars(t *testing.T) {
	f := newFigure()
	f.series = append(f.series, &series{kind: "bar", xs: []float64{1, 2}, ys: []float64{5, 7}})
	_, _, ylo, yhi := f.dataBounds()
	if ylo != 0 {
		t.Fatalf("bar y-lower should include 0, got %v", ylo)
	}
	if yhi != 7 {
		t.Fatalf("y-upper: got %v want 7", yhi)
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
