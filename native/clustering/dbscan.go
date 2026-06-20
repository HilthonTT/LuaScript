package clustering

// DBSCAN labels. Cluster ids start at 1; Noise marks points that fall in
// no dense region, and undefined is the internal "not yet visited" state.
const (
	Noise     = 0
	undefined = -1
)

// DBSCAN performs density-based spatial clustering. A point is a core
// point if at least minPts points (including itself) lie within eps;
// clusters grow by transitively absorbing the neighbourhoods of core
// points. Points reachable from no core point are labelled Noise (0).
//
// Returned labels are 1-based cluster ids, with 0 reserved for noise.
// Unlike k-means, DBSCAN discovers the cluster count on its own and finds
// arbitrarily shaped clusters.
func DBSCAN(data []Point, eps float64, minPts int) []int {
	n := len(data)
	labels := make([]int, n)
	for i := range labels {
		labels[i] = undefined
	}

	eps2 := eps * eps
	cluster := 0
	for i := range n {
		if labels[i] != undefined {
			continue
		}
		neighbors := regionQuery(data, i, eps2)
		if len(neighbors) < minPts {
			labels[i] = Noise
			continue
		}

		cluster++
		labels[i] = cluster

		// Seed set; grows as we discover further core points. Indexing
		// rather than ranging because we append while iterating.
		seeds := make([]int, len(neighbors))
		copy(seeds, neighbors)
		for s := 0; s < len(seeds); s++ {
			q := seeds[s]
			if labels[q] == Noise {
				labels[q] = cluster // border point: density-reachable, not core
			}
			if labels[q] != undefined {
				continue
			}
			labels[q] = cluster
			qn := regionQuery(data, q, eps2)
			if len(qn) >= minPts {
				seeds = append(seeds, qn...)
			}
		}
	}
	return labels
}

// regionQuery returns the indices of every point within eps (eps2 is the
// squared radius) of data[idx], including idx itself.
func regionQuery(data []Point, idx int, eps2 float64) []int {
	var out []int
	p := data[idx]
	for j := range data {
		if p.distanceSqTo(data[j]) <= eps2 {
			out = append(out, j)
		}
	}
	return out
}
