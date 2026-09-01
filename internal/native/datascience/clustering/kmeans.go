package clustering

import (
	"math"
	"math/rand"
)

type Point struct {
	Entry []float64
}

func (p Point) distanceTo(o Point) float64 {
	return math.Sqrt(p.distanceSqTo(o))
}

func (p Point) distanceSqTo(o Point) float64 {
	sum := 0.0
	for i := range p.Entry {
		d := p.Entry[i] - o.Entry[i]
		sum += d * d
	}
	return sum
}

func clonePoint(p Point) Point {
	entry := make([]float64, len(p.Entry))
	copy(entry, p.Entry)
	return Point{Entry: entry}
}

type KMeansResult struct {
	Centers     []Point
	Assignments []int
	Iterations  int
}

func kmeansPlusPlusInit(data []Point, k int, rng *rand.Rand) []Point {
	centers := make([]Point, 0, k)
	centers = append(centers, clonePoint(data[rng.Intn(len(data))]))

	dist2 := make([]float64, len(data))
	for len(centers) < k {
		sum := 0.0
		for i, p := range data {
			d := nearestDistSq(p, centers)
			dist2[i] = d
			sum += d
		}
		if sum == 0 {
			centers = append(centers, clonePoint(data[rng.Intn(len(data))]))
			continue
		}
		target := rng.Float64() * sum
		acc := 0.0
		chosen := len(data) - 1
		for i := range data {
			acc += dist2[i]
			if acc >= target {
				chosen = i
				break
			}
		}
		centers = append(centers, clonePoint(data[chosen]))
	}
	return centers
}

func nearestDistSq(p Point, centers []Point) float64 {
	best := math.MaxFloat64
	for i := range centers {
		if d := p.distanceSqTo(centers[i]); d < best {
			best = d
		}
	}
	return best
}

func KMeans(data []Point, k, maxIter int, delta float64, rng *rand.Rand) KMeansResult {
	centers := kmeansPlusPlusInit(data, k, rng)
	dims := len(data[0].Entry)
	assign := make([]int, len(data))

	iters := 0
	for ; iters < maxIter; iters++ {
		for i := range data {
			best, bestDist := 0, math.MaxFloat64
			for c := range centers {
				if d := data[i].distanceSqTo(centers[c]); d < bestDist {
					bestDist = d
					best = c
				}
			}
			assign[i] = best
		}

		sums := make([][]float64, k)
		counts := make([]int, k)
		for c := range sums {
			sums[c] = make([]float64, dims)
		}
		for i := range data {
			c := assign[i]
			counts[c]++
			for d := range dims {
				sums[c][d] += data[i].Entry[d]
			}
		}

		maxMove := 0.0
		for c := range centers {
			if counts[c] == 0 {
				continue
			}
			nc := make([]float64, dims)
			for d := range dims {
				nc[d] = sums[c][d] / float64(counts[c])
			}
			np := Point{Entry: nc}
			if move := centers[c].distanceTo(np); move > maxMove {
				maxMove = move
			}
			centers[c] = np
		}

		if maxMove <= delta {
			iters++
			break
		}
	}

	return KMeansResult{Centers: centers, Assignments: assign, Iterations: iters}
}
