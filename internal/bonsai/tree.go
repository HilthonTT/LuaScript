package bonsai

import (
	"math/rand"
	"time"
)

type align int

const (
	center align = iota + 1
	left
	right
)

type branch int

const (
	trunk branch = iota
	shootLeft
	shootRight
	dying
	dead
)

type counters struct {
	branches     int
	shoots       int
	shootCounter int
}

var rng *rand.Rand

func drawTree(sc *screen, o Options) error {
	c := &counters{
		shootCounter: rng.Intn(3),
	}
	growBranch(sc, o, c, sc.x, sc.y, trunk, o.Life)
	return nil
}

func growBranch(sc *screen, o Options, c *counters, x, y int, kind branch, life int) {
	c.branches++
	age := 0
	maxLife := life

	for life > 0 {
		life--
		age = maxLife - life

		dx, dy := setDeltas(kind, life, age, o.Multiplier)

		_, h := sc.Size()
		if dy > 0 && y > h-2 {
			dy--
		}

		if life < 3 {
			growBranch(sc, o, c, x, y, dead, life)
		} else if kind == trunk && life < (o.Multiplier+2) {
			growBranch(sc, o, c, x, y, dying, life)
		} else if (kind == shootLeft || kind == shootRight) && life < (o.Multiplier+2) {
			growBranch(sc, o, c, x, y, dying, life)
		} else if kind == trunk && (rng.Intn(3) == 0 || (life%o.Multiplier == 0)) {
			if rng.Intn(8) == 0 && life > 7 {
				c.shoots++
				c.shootCounter++
				shootKind := shootLeft
				if c.shootCounter%2 == 0 {
					shootKind = shootRight
				}
				growBranch(sc, o, c, x, y, shootKind, life+rng.Intn(5)-2)
			}
		}

		if c.branches > 1024 {
			return
		}

		x += dx
		y += dy

		w, h2 := sc.Size()
		if x < 0 || x >= w || y < 0 || y >= h2 {
			return
		}

		sc.x = x
		sc.y = y
		glyph := chooseString(kind, dx, dy, life, o.Leaves)
		sc.draw(glyph, chooseColor(kind))

		if o.Live {
			evDrawn(sc)
			time.Sleep(o.Step)
		}
	}
}

func setDeltas(kind branch, life, age, multiplier int) (dx, dy int) {
	switch kind {
	case trunk:
		if age <= 2 || life < 4 {
			dy = 0
			dx = rng.Intn(3) - 1
			return
		}
		if age < (multiplier*3) && age%(multiplier+1) == 0 {
			dy = -1
			dx = rng.Intn(3) - 1
			return
		}
		switch rng.Intn(10) {
		case 0:
			dx, dy = -2, 0
		case 1, 2:
			dx, dy = -1, -1
		case 3:
			dx, dy = 0, -1
		case 4, 5, 6:
			dx, dy = 1, -1
		case 7:
			dx, dy = 2, 0
		case 8:
			dx, dy = -1, 0
		default:
			dx, dy = 1, 0
		}
	case shootLeft:
		switch rng.Intn(10) {
		case 0, 1:
			dx, dy = -2, 0
		case 2, 3, 4, 5:
			dx, dy = -1, 0
		case 6, 7, 8:
			dx, dy = 0, -1
		default:
			dx, dy = 1, 0
		}
	case shootRight:
		switch rng.Intn(10) {
		case 0, 1:
			dx, dy = 2, 0
		case 2, 3, 4, 5:
			dx, dy = 1, 0
		case 6, 7, 8:
			dx, dy = 0, -1
		default:
			dx, dy = -1, 0
		}
	case dying:
		switch rng.Intn(10) {
		case 0, 1:
			dy = -1
			dx = rng.Intn(3) - 1
		case 2:
			dx, dy = -2, 0
		case 3:
			dx, dy = 2, 0
		default:
			dx = rng.Intn(3) - 1
			dy = 0
		}
	case dead:
		dx = rng.Intn(5) - 2
		dy = rng.Intn(3) - 1
	}
	return
}

func chooseString(kind branch, dx, dy int, life int, leaves []string) string {
	if kind == dying || kind == dead {
		if len(leaves) == 0 {
			return "&"
		}
		return leaves[rng.Intn(len(leaves))]
	}

	switch {
	case dy == 0 && dx < 0:
		if kind == shootLeft {
			return "\\_"
		}
		return "\\"
	case dy == 0 && dx > 0:
		if kind == shootRight {
			return "_/"
		}
		return "/"
	case dy < 0 && dx < 0:
		return "\\"
	case dy < 0 && dx == 0:
		i := rng.Intn(3)
		return "/|\\"[i : i+1]
	case dy < 0 && dx > 0:
		return "/"
	case dy > 0:
		i := rng.Intn(3)
		return "/|\\"[i : i+1]
	}
	if life < 4 {
		return "~"
	}
	return "|"
}
