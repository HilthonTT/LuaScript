package clustering

import "math"

// MeanShiftResult bundles the discovered modes (cluster centers) with the
// per-point assignment. Like DBSCAN, mean shift discovers the number of
// clusters on its own — here it is governed by the bandwidth.
type MeanShiftResult struct {
	Centers     []Point
	Assignments []int
	Iterations  int
}

// MeanShift is a centroid-seeking, non-parametric clusterer. Every point
// is iteratively shifted toward the weighted mean of its neighbourhood
// (using a Gaussian kernel parameterised by bandwidth) until convergence;
// points that settle on the same mode form a cluster.
//
// Smaller bandwidths yield more, tighter clusters. Returned assignments
// are 1-based cluster ids.
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

	// Collapse converged points into modes: a point joins an existing mode
	// if it lies within half a bandwidth of it, otherwise it founds a new one.
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
