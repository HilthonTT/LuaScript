package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Theme colors, picked to sit on the website's ink background.
var (
	cBg       = rgb(0x0f0d17)
	cChrome   = rgb(0x17142a)
	cDivider  = rgb(0x262140)
	cText     = rgb(0xe4e1f0)
	cDim      = rgb(0x7d7895)
	cGutter   = rgb(0x4a4560)
	cPurple   = rgb(0xb79cff)
	cMagenta  = rgb(0xd38cff)
	cCyan     = rgb(0x67e8f9)
	cGreen    = rgb(0x86efac)
	cOK       = rgb(0x4ade80)
	cRed      = rgb(0xf87171)
	cYellow   = rgb(0xfcd34d)
	cOrange   = rgb(0xfdba74)
	cBlue     = rgb(0x93c5fd)
	cType     = rgb(0x7dd3fc)
	cCursor   = rgb(0xc4b5fd)
	cDotRed   = rgb(0xff5f57)
	cDotAmber = rgb(0xfebc2e)
	cDotGreen = rgb(0x28c840)
)

func rgb(v uint32) color.RGBA {
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}
}

// Span is a run of styled text on one line.
type Span struct {
	Text string
	FG   color.RGBA
	Bold bool
}

// Line is one row of styled text.
type Line []Span

func plain(s string, c color.RGBA) Line { return Line{{Text: s, FG: c}} }

func (l Line) text() string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.Text)
	}
	return b.String()
}

const (
	fontSize   = 15
	lineHeight = 22
	padX       = 22
	chromeH    = 38
)

type fonts struct {
	regular, bold font.Face
	advance       int
}

func loadFonts() *fonts {
	parse := func(ttf []byte) font.Face {
		f, err := opentype.Parse(ttf)
		check(err)
		face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: fontSize, DPI: 72, Hinting: font.HintingFull})
		check(err)
		return face
	}
	reg := parse(gomono.TTF)
	adv, _ := reg.GlyphAdvance('M')
	return &fonts{regular: reg, bold: parse(gomonobold.TTF), advance: adv.Ceil()}
}

// Window is a terminal-ish window: a title bar, an optional editor pane on
// top and a terminal pane below.
type Window struct {
	W, H  int
	Title string
	f     *fonts

	Editor *Editor
	Term   *Term
}

// Editor shows a source file with line numbers and a caret.
type Editor struct {
	File   string
	Lines  []string
	Rows   int
	Row    int // caret position
	Col    int
	Caret  bool
	Marked int // 1-based line drawn with an error underline, 0 for none
}

// Term is a scrolling terminal.
type Term struct {
	Lines []Line
	Rows  int
	Caret bool
}

func (t *Term) Print(l Line) { t.Lines = append(t.Lines, l) }

// Append adds text to the last line (creating one if needed).
func (t *Term) Append(s Span) {
	if len(t.Lines) == 0 {
		t.Lines = append(t.Lines, Line{})
	}
	t.Lines[len(t.Lines)-1] = append(t.Lines[len(t.Lines)-1], s)
}

func (t *Term) visible() []Line {
	if len(t.Lines) > t.Rows {
		return t.Lines[len(t.Lines)-t.Rows:]
	}
	return t.Lines
}

func (w *Window) drawText(dst *image.RGBA, x, y int, s Span) int {
	face := w.f.regular
	if s.Bold {
		face = w.f.bold
	}
	d := font.Drawer{Dst: dst, Src: image.NewUniform(s.FG), Face: face, Dot: fixed.P(x, y)}
	for _, r := range s.Text {
		if r == '\t' {
			x += 4 * w.f.advance
			continue
		}
		// Box-drawing and block glyphs are missing from Go Mono, so draw
		// the few the REPL banner uses as cell-filling rectangles.
		if drawBlock(dst, r, x, y, w.f.advance, s.FG) {
			x += w.f.advance
			continue
		}
		d.Dot = fixed.P(x, y)
		d.DrawString(string(r))
		x += w.f.advance
	}
	return x
}

func drawBlock(dst *image.RGBA, r rune, x, baseline, adv int, c color.RGBA) bool {
	top := baseline - lineHeight + 6
	bot := top + lineHeight
	midX, midY := x+adv/2, top+lineHeight/2
	fill := func(x0, y0, x1, y1 int) {
		draw.Draw(dst, image.Rect(x0, y0, x1, y1), image.NewUniform(c), image.Point{}, draw.Src)
	}
	switch r {
	case '█':
		fill(x, top, x+adv, bot)
	case '═':
		fill(x, midY-3, x+adv, midY-2)
		fill(x, midY+1, x+adv, midY+2)
	case '║':
		fill(midX-3, top, midX-2, bot)
		fill(midX+1, top, midX+2, bot)
	case '╗':
		fill(x, midY-3, midX+2, midY-2)
		fill(x, midY+1, midX-2, midY+2)
		fill(midX+1, midY-3, midX+2, bot)
		fill(midX-3, midY+1, midX-2, bot)
	case '╔':
		fill(midX-3, midY-3, x+adv, midY-2)
		fill(midX+1, midY+1, x+adv, midY+2)
		fill(midX-3, midY-3, midX-2, bot)
		fill(midX+1, midY+1, midX+2, bot)
	case '╝':
		fill(x, midY+1, midX+2, midY+2)
		fill(x, midY-3, midX-2, midY-2)
		fill(midX+1, top, midX+2, midY+2)
		fill(midX-3, top, midX-2, midY-2)
	case '╚':
		fill(midX-3, midY+1, x+adv, midY+2)
		fill(midX+1, midY-3, x+adv, midY-2)
		fill(midX-3, top, midX-2, midY+2)
		fill(midX+1, top, midX+2, midY-2)
	default:
		return false
	}
	return true
}

func fillRect(dst *image.RGBA, r image.Rectangle, c color.RGBA) {
	draw.Draw(dst, r, image.NewUniform(c), image.Point{}, draw.Src)
}

func fillCircle(dst *image.RGBA, cx, cy, rad int, c color.RGBA) {
	// Supersampled so the traffic lights are not jagged.
	for y := cy - rad - 1; y <= cy+rad+1; y++ {
		for x := cx - rad - 1; x <= cx+rad+1; x++ {
			hits := 0
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					dx := float64(x) + (float64(sx)+0.5)/4 - float64(cx) - 0.5
					dy := float64(y) + (float64(sy)+0.5)/4 - float64(cy) - 0.5
					if dx*dx+dy*dy <= float64(rad*rad) {
						hits++
					}
				}
			}
			if hits > 0 {
				bg := dst.RGBAAt(x, y)
				dst.SetRGBA(x, y, blend(bg, c, float64(hits)/16))
			}
		}
	}
}

func blend(a, b color.RGBA, t float64) color.RGBA {
	m := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t + 0.5) }
	return color.RGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 0xff}
}

// Render draws the whole window.
func (w *Window) Render() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w.W, w.H))
	fillRect(img, img.Bounds(), cBg)

	// Title bar.
	fillRect(img, image.Rect(0, 0, w.W, chromeH), cChrome)
	fillRect(img, image.Rect(0, chromeH-1, w.W, chromeH), cDivider)
	for i, c := range []color.RGBA{cDotRed, cDotAmber, cDotGreen} {
		fillCircle(img, 22+i*20, chromeH/2, 6, c)
	}
	tw := utf8.RuneCountInString(w.Title) * w.f.advance
	w.drawText(img, (w.W-tw)/2, chromeH/2+5, Span{Text: w.Title, FG: cDim})

	y := chromeH
	if e := w.Editor; e != nil {
		// Tab strip with the file name.
		tabH := 32
		fillRect(img, image.Rect(0, y, w.W, y+tabH), cChrome)
		tabW := (utf8.RuneCountInString(e.File)+4)*w.f.advance + 8
		fillRect(img, image.Rect(0, y, tabW, y+tabH), cBg)
		fillRect(img, image.Rect(0, y, tabW, y+2), cPurple)
		w.drawText(img, padX, y+tabH/2+5, Span{Text: "●", FG: cPurple})
		w.drawText(img, padX+2*w.f.advance, y+tabH/2+5, Span{Text: e.File, FG: cText})
		y += tabH + 10

		first := 0
		if e.Row >= e.Rows {
			first = e.Row - e.Rows + 1
		}
		gutter := padX + 3*w.f.advance
		for i := 0; i < e.Rows; i++ {
			n := first + i
			base := y + (i+1)*lineHeight - 6
			if n >= len(e.Lines) {
				continue
			}
			num := itoa(n + 1)
			numColor := cGutter
			if e.Caret && n == e.Row {
				fillRect(img, image.Rect(0, base-lineHeight+6, w.W, base+6), rgb(0x16132a))
				numColor = cDim
			}
			w.drawText(img, padX+(2-len(num))*w.f.advance, base, Span{Text: num, FG: numColor})
			x := gutter + w.f.advance
			for _, s := range highlight(e.Lines[n]) {
				x = w.drawText(img, x, base, s)
			}
			if e.Marked == n+1 {
				w.squiggle(img, gutter+w.f.advance, base+3, utf8.RuneCountInString(e.Lines[n]))
			}
			if e.Caret && n == e.Row {
				cx := gutter + w.f.advance + e.Col*w.f.advance
				fillRect(img, image.Rect(cx, base-lineHeight+9, cx+2, base+4), cCursor)
			}
		}
		y += e.Rows*lineHeight + 12

		// Terminal header.
		fillRect(img, image.Rect(0, y, w.W, y+1), cDivider)
		w.drawText(img, padX, y+22, Span{Text: "TERMINAL", FG: cDim, Bold: true})
		y += 30
	}

	if t := w.Term; t != nil {
		y += 8
		vis := t.visible()
		for i, l := range vis {
			base := y + (i+1)*lineHeight - 6
			x := padX
			for _, s := range l {
				x = w.drawText(img, x, base, s)
			}
			if t.Caret && i == len(vis)-1 {
				fillRect(img, image.Rect(x, base-lineHeight+8, x+w.f.advance, base+5), cCursor)
			}
		}
	}
	return img
}

func (w *Window) squiggle(img *image.RGBA, x, y, cols int) {
	for i := 0; i < cols*w.f.advance; i++ {
		dy := 0
		if (i/2)%2 == 1 {
			dy = 1
		}
		img.SetRGBA(x+i, y+dy, cRed)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// Recorder turns a sequence of window states into an animated GIF. Only the
// changed rectangle of each frame is stored, so typing costs a few bytes.
type Recorder struct {
	win    *Window
	pal    color.Palette
	cache  map[color.RGBA]uint8
	frames []*image.Paletted
	delays []int
	prev   *image.Paletted
}

func NewRecorder(w *Window) *Recorder {
	return &Recorder{win: w, pal: buildPalette(), cache: map[color.RGBA]uint8{}}
}

// buildPalette spans every theme color blended onto the two backgrounds, so
// antialiased glyph edges quantize cleanly without dithering.
func buildPalette() color.Palette {
	fgs := []color.RGBA{cText, cDim, cGutter, cPurple, cMagenta, cCyan, cGreen, cOK, cRed,
		cYellow, cOrange, cBlue, cType, cCursor, cDotRed, cDotAmber, cDotGreen, cDivider}
	bgs := []color.RGBA{cBg, cChrome, rgb(0x16132a)}
	seen := map[color.RGBA]bool{}
	var pal color.Palette
	add := func(c color.RGBA) {
		if !seen[c] && len(pal) < 256 {
			seen[c] = true
			pal = append(pal, c)
		}
	}
	for _, c := range bgs {
		add(c)
	}
	for _, c := range fgs {
		add(c)
	}
	steps := []float64{0.12, 0.25, 0.38, 0.5, 0.62, 0.75, 0.88}
	for _, bg := range bgs[:2] {
		for _, fg := range fgs {
			for _, t := range steps {
				add(blend(bg, fg, t))
			}
		}
	}
	for _, fg := range []color.RGBA{cText, cDim, cGutter, cCursor} {
		for _, t := range []float64{0.25, 0.5, 0.75} {
			add(blend(rgb(0x16132a), fg, t))
		}
	}
	return pal
}

func (r *Recorder) quantize(img *image.RGBA) *image.Paletted {
	p := image.NewPaletted(img.Bounds(), r.pal)
	for i := 0; i < len(img.Pix); i += 4 {
		c := color.RGBA{img.Pix[i], img.Pix[i+1], img.Pix[i+2], 0xff}
		idx, ok := r.cache[c]
		if !ok {
			idx = uint8(r.pal.Index(c))
			r.cache[c] = idx
		}
		p.Pix[i/4] = idx
	}
	return p
}

// Snap records the current window state, shown for ms milliseconds.
func (r *Recorder) Snap(ms int) {
	cur := r.quantize(r.win.Render())
	delay := (ms + 5) / 10
	if r.prev == nil {
		r.frames = append(r.frames, cur)
		r.delays = append(r.delays, delay)
		r.prev = cur
		return
	}
	box, changed := diffBox(r.prev, cur)
	r.prev = cur
	if !changed {
		r.delays[len(r.delays)-1] += delay
		return
	}
	r.frames = append(r.frames, cur.SubImage(box).(*image.Paletted))
	r.delays = append(r.delays, delay)
}

// Hold extends the current frame.
func (r *Recorder) Hold(ms int) { r.Snap(ms) }

func diffBox(a, b *image.Paletted) (image.Rectangle, bool) {
	w, h := a.Rect.Dx(), a.Rect.Dy()
	minX, minY, maxX, maxY := w, h, -1, -1
	for y := 0; y < h; y++ {
		row := y * a.Stride
		for x := 0; x < w; x++ {
			if a.Pix[row+x] != b.Pix[row+x] {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				maxY = y
			}
		}
	}
	if maxX < 0 {
		return image.Rectangle{}, false
	}
	return image.Rect(minX, minY, maxX+1, maxY+1), true
}

func (r *Recorder) Save(path string) {
	disposal := make([]byte, len(r.frames))
	for i := range disposal {
		disposal[i] = gif.DisposalNone
	}
	f, err := os.Create(path)
	check(err)
	defer f.Close()
	check(gif.EncodeAll(f, &gif.GIF{
		Image:     r.frames,
		Delay:     r.delays,
		Disposal:  disposal,
		LoopCount: 0,
		Config:    image.Config{ColorModel: r.pal, Width: r.win.W, Height: r.win.H},
	}))
}
