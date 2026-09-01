package clustering

import "math"

type MeanShiftResult struct {
	Centers     []Point
	Assignments []int
	Iterations  int
}

func MeanShift(data []Point, bandwidth float64, maxIter int) MeanShiftResult {
	n := len(data)
	dims := len(data[0].Entry)

	shifted := make([]Point, n)
	for i := range data {
		shifted[i] = clonePoint(data[i])
	}

	twoH2 := 2 * bandwidth * bandwidth
	iters := 0
	for ; iters < maxIter; iters++ {
		next := make([]Point, n)
		maxMove := 0.0
		for i := range shifted {
			num := make([]float64, dims)
			den := 0.0
			for j := range data {
				d2 := shifted[i].distanceSqTo(data[j])
				w := math.Exp(-d2 / twoH2)
				den += w
				for d := range dims {
					num[d] += w * data[j].Entry[d]
				}
			}
			nc := make([]float64, dims)
			for d := range dims {
				nc[d] = num[d] / den
			}
			np := Point{Entry: nc}
			if move := shifted[i].distanceTo(np); move > maxMove {
				maxMove = move
			}
			next[i] = np
		}
		shifted = next
		if maxMove < 1e-5 {
			iters++
			break
		}
	}

	mergeRadius := bandwidth / 2
	var centers []Point
	labels := make([]int, n)
	for i := range shifted {
		found := -1
		for c := range centers {
			if shifted[i].distanceTo(centers[c]) < mergeRadius {
				found = c
				break
			}
		}
		if found < 0 {
			centers = append(centers, shifted[i])
			found = len(centers) - 1
		}
		labels[i] = found + 1
	}

	return MeanShiftResult{Centers: centers, Assignments: labels, Iterations: iters}
}
