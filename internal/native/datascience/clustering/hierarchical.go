package clustering

import "math"

// Linkage selects how the distance between two clusters is measured during
// agglomerative merging.
type Linkage int

const (
	// SingleLinkage uses the closest pair of members (min distance).
	SingleLinkage Linkage = iota
	// CompleteLinkage uses the farthest pair of members (max distance).
	CompleteLinkage
	// AverageLinkage uses the mean over all cross-cluster member pairs.
	AverageLinkage
)

// ParseLinkage maps a string name to a Linkage, defaulting to average.
func ParseLinkage(s string) Linkage {
	switch s {
	case "single":
		return SingleLinkage
	case "complete":
		return CompleteLinkage
	default:
		return AverageLinkage
	}
}

// Hierarchical performs bottom-up agglomerative clustering: every point
// starts as its own cluster and the two closest clusters are merged
// repeatedly until exactly k clusters remain. The pairwise point-distance
// matrix is precomputed once; linkage decides how cluster-to-cluster
// distance is derived from it.
//
// This is an O(n^3) implementation suited to small/medium inputs. Returned
// labels are 1-based cluster ids.
func Hierarchical(data []Point, k int, linkage Linkage) []int {
	n := len(data)
	if k < 1 {
		k = 1
	}

	// Precompute pairwise squared distances; linkage comparisons only need
	// the ordering, so squared distances are sufficient and cheaper.
	dist := make([][]float64, n)
	for i := range n {
		dist[i] = make([]float64, n)
		for j := range n {
			dist[i][j] = data[i].distanceSqTo(data[j])
		}
	}

	members := make([][]int, n)
	active := make([]bool, n)
	for i := range n {
		members[i] = []int{i}
		active[i] = true
	}

	count := n
	for count > k {
		bestA, bestB := -1, -1
		bestD := math.MaxFloat64
		for a := range n {
			if !active[a] {
				continue
			}
			for b := a + 1; b < n; b++ {
				if !active[b] {
					continue
				}
				if d := clusterDistance(dist, members[a], members[b], linkage); d < bestD {
					bestD = d
					bestA, bestB = a, b
				}
			}
		}
		if bestA < 0 {
			break
		}
		members[bestA] = append(members[bestA], members[bestB]...)
		active[bestB] = false
		count--
	}

	labels := make([]int, n)
	cid := 0
	for a := range n {
		if !active[a] {
			continue
		}
		cid++
		for _, idx := range members[a] {
			labels[idx] = cid
		}
	}
	return labels
}

func clusterDistance(dist [][]float64, a, b []int, linkage Linkage) float64 {
	switch linkage {
	case SingleLinkage:
		best := math.MaxFloat64
		for _, i := range a {
			for _, j := range b {
				if dist[i][j] < best {
					best = dist[i][j]
				}
			}
		}
		return best
	case CompleteLinkage:
		best := 0.0
		for _, i := range a {
			for _, j := range b {
				if dist[i][j] > best {
					best = dist[i][j]
				}
			}
		}
		return best
	default: // AverageLinkage
		sum := 0.0
		for _, i := range a {
			for _, j := range b {
				sum += dist[i][j]
			}
		}
		return sum / float64(len(a)*len(b))
	}
}
