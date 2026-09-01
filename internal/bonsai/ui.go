package bonsai

import (
	"strings"

	"github.com/gdamore/tcell/v3"
	"github.com/mattn/go-runewidth"
)

type screen struct {
	tcell.Screen
	x, y   int
	active bool
}

func (sc *screen) draw(text string, style tcell.Style) {
	w, h := sc.Size()

	for _, r := range []rune(text) {
		rw := runewidth.RuneWidth(r)

		if sc.x+rw >= w || sc.y >= h {
			continue
		}

		sc.put(r, style)
	}
}

func (sc *screen) drawMessage(msg string, x int, y int) {
	sc.x = x
	sc.y = y
	sc.draw("+"+strings.Repeat("-", len(msg)+2)+"+", styleGray)
	sc.x = x
	sc.y = y + 1
	sc.draw("| ", styleGray)
	sc.draw(msg, styleDefault)
	sc.draw(" |", styleGray)
	sc.x = x
	sc.y = y + 2
	sc.draw("+"+strings.Repeat("-", len(msg)+2)+"+", styleGray)
}

func (sc *screen) put(r rune, style tcell.Style) {
	if !sc.active {
		return
	}

	w := runewidth.RuneWidth(r)

	if w < 1 {
		return
	}

	sc.SetContent(sc.x, sc.y, r, nil, style)
	sc.x = sc.x + w
}

func newScreen() (*screen, error) {
	tsc, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}

	if err := tsc.Init(); err != nil {
		return nil, err
	}

	tsc.DisablePaste()
	tsc.DisableMouse()

	tsc.SetStyle(tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset))

	tsc.Clear()

	return &screen{Screen: tsc}, nil
}
