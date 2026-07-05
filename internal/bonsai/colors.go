package bonsai

import (
	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
)

var (
	styleDefault   = tcell.StyleDefault
	styleGreen     = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(color.Green)
	styleGreenBold = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(color.Green).Bold(true)
	styleWhiteBold = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(color.White).Bold(true)
	styleBrown     = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(color.XTerm94)
	styleBrownBold = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(color.XTerm94).Bold(true)
	styleGray      = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(color.Gray)
)

func chooseColor(kind branch) tcell.Style {
	switch kind {
	case dying:
		if rng.Intn(10) == 0 {
			return styleGreenBold
		}
		return styleGreen

	case dead:
		if rng.Intn(3) == 0 {
			return styleGreenBold
		}
		return styleGreen

	default:
		if rng.Intn(2) == 0 {
			return styleBrownBold
		}
		return styleBrown
	}
}
