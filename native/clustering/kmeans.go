package clustering

import (
	"math"
	"math/rand"
)

// Point is a single observation in feature space. Entry holds the
// coordinates; every Point fed to an algorithm is expected to share the
// same dimensionality.
type Point struct {
	Entry []float64
}

// distanceTo returns the Euclidean distance between two points. Callers
// that only need to compare distances should prefer distanceSqTo, which
// skips the square root.
func (p Point) distanceTo(o Point) float64 {
	return math.Sqrt(p.distanceSqTo(o))
}

// distanceSqTo returns the squared Euclidean distance. Used on the hot
// assignment paths where the monotonic ordering of distances is all that
// matters, so the sqrt can be elided.
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

// KMeansResult bundles the converged cluster centers with the per-point
// cluster assignment. Assignments are 0-based indices into Centers.
type KMeansResult struct {
	Centers     []Point
	Assignments []int
	Iterations  int
}

// kmeansPlusPlusInit seeds k centers using the k-means++ strategy: the
// first center is uniform-random, and each subsequent center is drawn
// with probability proportional to its squared distance from the nearest
// already-chosen center. This spreads the seeds out and dramatically cuts
// the number of iterations (and bad local minima) versus pure-random init.
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
		// All remaining points coincide with existing centers; fall back
		// to a uniform pick so we still return k distinct-ish seeds.
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

// KMeans partitions data into k clusters using Lloyd's algorithm with
// k-means++ seeding. It iterates until no center moves more than delta or
// maxIter is reached. Empty clusters keep their previous center rather
// than collapsing to NaN.
func KMeans(data []Point, k, maxIter int, delta float64, rng *rand.Rand) KMeansResult {
	centers := kmeansPlusPlusInit(data, k, rng)
	dims := len(data[0].Entry)
	assign := make([]int, len(data))

	iters := 0
	for ; iters < maxIter; iters++ {
		// Assignment step: nearest center wins.
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

		// Update step: each center becomes the mean of its members.
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
				continue // keep the stale center; re-seeding is overkill here
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
