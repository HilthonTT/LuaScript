// Package bonsai grows procedural ASCII bonsai trees in the terminal.
//
// It is a tcell-based port of cbonsai (https://gitlab.com/jallbrit/cbonsai).
// The public surface is intentionally small: build an Options value and call
// Run. The package owns the screen lifetime; on return the terminal is
// restored.
package bonsai

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/gdamore/tcell/v3"
)

// Pot kinds accepted by Options.Pot.
const (
	PotBig   = 1
	PotSmall = 2
)

// Alignment values accepted by Options.Align.
const (
	AlignCenter = int(center)
	AlignLeft   = int(left)
	AlignRight  = int(right)
)

// Options configures a single Run. Zero values pick cbonsai-compatible
// defaults inside Run, so callers can pass an empty Options{} for "just
// grow a tree".
type Options struct {
	Seed         int64         // 0 -> time.Now().UnixNano()
	Life         int           // 0 -> 32, clamped to [1,127]
	Multiplier   int           // 0 -> 5, clamped to [0,20]
	Pot          int           // 0 -> PotBig
	Align        int           // 0 -> AlignCenter
	Leaves       []string      // nil -> []string{"&"}
	Message      string        // optional message box
	MsgX, MsgY   int           // upper-left of message; defaults 4,2
	BaseX, BaseY int           // overrides for pot position; 0 = auto-center
	Print        bool          // dump tree to stdout and exit
	Live         bool          // animate growth step-by-step
	Step         time.Duration // delay between live steps; 0 -> 33ms
	Infinite     bool          // keep regrowing until quit
	Wait         time.Duration // delay between trees in infinite mode; 0 -> 4s
	Screensaver  bool          // implies Live + Infinite, exits on any keypress
}

// applyDefaults fills zero-valued fields with cbonsai defaults and clamps
// out-of-range numerics.
func (o *Options) applyDefaults() {
	if o.Seed == 0 {
		o.Seed = time.Now().UnixNano()
	}
	if o.Life == 0 {
		o.Life = 32
	}
	if o.Life < 1 {
		o.Life = 1
	}
	if o.Life > 127 {
		o.Life = 127
	}
	if o.Multiplier == 0 {
		o.Multiplier = 5
	}
	if o.Multiplier < 0 {
		o.Multiplier = 0
	}
	if o.Multiplier > 20 {
		o.Multiplier = 20
	}
	if o.Pot == 0 {
		o.Pot = PotBig
	}
	if o.Align == 0 {
		o.Align = AlignCenter
	}
	if len(o.Leaves) == 0 {
		o.Leaves = []string{"&"}
	}
	if o.Step == 0 {
		o.Step = 33 * time.Millisecond
	}
	if o.Wait == 0 {
		o.Wait = 4 * time.Second
	}
	if o.MsgX == 0 {
		o.MsgX = 4
	}
	if o.MsgY == 0 {
		o.MsgY = 2
	}
	if o.Screensaver {
		o.Live = true
		o.Infinite = true
	}
}

// Run grows a tree according to o. It blocks until the user quits (Esc /
// Ctrl-C / Ctrl-D) or, in non-infinite non-print mode, until any keypress
// after the first tree finishes drawing. On return, the terminal is restored.
func Run(o Options) error {
	o.applyDefaults()

	rng = rand.New(rand.NewSource(o.Seed))

	pot, err := selectPot(o.Pot)
	if err != nil {
		return err
	}

	sc, err := newScreen()
	if err != nil {
		return err
	}
	sc.active = true

	var capturedTree string
	cleanup := func() {
		sc.active = false
		sc.Fini()
		if o.Print && capturedTree != "" {
			fmt.Println(capturedTree)
		}
	}
	defer cleanup()

	shutdown := make(chan struct{})

	// Drawing goroutine.
	go func() {
		defer func() {
			// Swallow panics here so the event loop still gets a chance
			// to run Fini via the deferred cleanup. We surface the panic
			// after restoring the terminal.
			if r := recover(); r != nil {
				evQuit(sc)
				panic(r)
			}
		}()

		for {
			if !sc.active {
				return
			}

			sc.Clear()

			px, py := pot.ulPos(sc)
			switch o.Align {
			case AlignLeft:
				sw, _ := sc.Size()
				px -= sw / 4
			case AlignRight:
				sw, _ := sc.Size()
				px += sw / 4
			}
			if o.BaseX != 0 {
				px = o.BaseX
			}
			if o.BaseY != 0 {
				py = o.BaseY
			}

			pot.draw(sc, px, py)

			if drawErr := drawTree(sc, o); drawErr != nil {
				evQuit(sc)
				return
			}

			if o.Message != "" {
				sc.drawMessage(o.Message, o.MsgX, o.MsgY)
			}

			evDrawn(sc)

			if o.Print {
				capturedTree = captureScreen(sc)
				evQuit(sc)
				<-shutdown
				return
			}

			if o.Infinite {
				select {
				case <-shutdown:
					return
				case <-time.After(o.Wait):
				}
			} else {
				<-shutdown
				return
			}
		}
	}()

	// Event loop on the calling goroutine.
	for {
		ev := <-sc.EventQ()
		switch ev := ev.(type) {
		case *tcell.EventResize:
			sc.Sync()
		case *tcell.EventKey:
			if o.Screensaver {
				close(shutdown)
				return nil
			}
			switch ev.Key() {
			case tcell.KeyEscape, tcell.KeyCtrlC, tcell.KeyCtrlD:
				close(shutdown)
				return nil
			}
		case *eventDrawn:
			sc.Show()
		case *eventQuit:
			close(shutdown)
			return nil
		case nil:
			// PollEvent returns nil when the screen is finalized.
			return nil
		}
	}
}

func selectPot(kind int) (Pot, error) {
	switch kind {
	case PotBig:
		return bigPot, nil
	case PotSmall:
		return smallPot, nil
	default:
		return Pot{}, fmt.Errorf("bonsai: unknown pot %d (use PotBig=1 or PotSmall=2)", kind)
	}
}

// captureScreen reads the rendered cell grid back into a single string,
// trimming all-blank lines above the tree.
func captureScreen(sc *screen) string {
	w, h := sc.Size()
	lines := make([]string, 0, h)
	for y := 0; y < h; y++ {
		var sb strings.Builder
		for x := 0; x < w; x++ {
			s, _, width := sc.Get(x, y)
			if s == "" {
				sb.WriteRune(' ')
			} else {
				sb.WriteString(s)
			}
			if width > 1 {
				x += width - 1
			}
		}
		s := strings.TrimRight(sb.String(), " ")
		if s != "" {
			lines = append(lines, s)
		}
	}
	return strings.Join(lines, "\n")
}
