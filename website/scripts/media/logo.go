package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

func savePNG(img image.Image, path string) {
	f, err := os.Create(path)
	check(err)
	defer f.Close()
	check(png.Encode(f, img))
}

// rasterSVG draws an SVG file into dst at rect r.
func rasterSVG(svg []byte, dst *image.RGBA, r image.Rectangle) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(svg))
	check(err)
	icon.SetTarget(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	icon.Draw(rasterx.NewDasher(w, h, rasterx.NewScannerGV(w, h, dst, dst.Bounds())), 1)
}

// renderLogos rasterizes public/logo.svg into the PNG sizes the site and
// README use, plus the 1200x630 social card.
func renderLogos() {
	svg, err := os.ReadFile(filepath.Join(outDir, "logo.svg"))
	check(err)

	for _, s := range []struct {
		name string
		size int
	}{{"logo.png", 512}, {"apple-touch-icon.png", 180}} {
		img := image.NewRGBA(image.Rect(0, 0, s.size, s.size))
		if s.name == "apple-touch-icon.png" {
			// iOS does not honor transparency: put the mark on ink.
			draw.Draw(img, img.Bounds(), image.NewUniform(rgb(0x0b0912)), image.Point{}, draw.Src)
			pad := s.size / 8
			rasterSVG(svg, img, image.Rect(pad, pad, s.size-pad, s.size-pad))
		} else {
			rasterSVG(svg, img, img.Bounds())
		}
		savePNG(img, filepath.Join(outDir, s.name))
	}

	// Social card.
	const W, H = 1200, 630
	card := image.NewRGBA(image.Rect(0, 0, W, H))
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			// A soft violet glow in the upper left over near-black ink.
			dx, dy := float64(x-260)/900, float64(y-200)/600
			t := 1 - (dx*dx + dy*dy)
			if t < 0 {
				t = 0
			}
			card.SetRGBA(x, y, blend(rgb(0x07060b), rgb(0x2a1660), t*t*0.9))
		}
	}
	rasterSVG(svg, card, image.Rect(110, 175, 390, 455))
	title := face(gobold.TTF, 104)
	sub := face(goregular.TTF, 34)
	text(card, title, 440, 300, "Lua", rgb(0xf7f7fb))
	adv := font.MeasureString(title, "Lua").Ceil()
	text(card, title, 440+adv, 300, "Script", rgb(0xb79cff))
	text(card, sub, 446, 365, "Lua 5.4 with gradual types,", rgb(0xcfcbe0))
	text(card, sub, 446, 412, "compiled to a fast stack VM in Go.", rgb(0xcfcbe0))
	savePNG(card, filepath.Join(outDir, "og.png"))
}

func face(ttf []byte, size float64) font.Face {
	f, err := opentype.Parse(ttf)
	check(err)
	fc, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	check(err)
	return fc
}

func text(dst *image.RGBA, f font.Face, x, y int, s string, c color.RGBA) {
	(&font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y)}).DrawString(s)
}
