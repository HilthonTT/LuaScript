package clustering

import "math"

type Linkage int

const (
	SingleLinkage Linkage = iota
	CompleteLinkage
	AverageLinkage
)

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

func Hierarchical(data []Point, k int, linkage Linkage) []int {
	n := len(data)
	if k < 1 {
		k = 1
	}

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
	default:
		sum := 0.0
		for _, i := range a {
			for _, j := range b {
				sum += dist[i][j]
			}
		}
		return sum / float64(len(a)*len(b))
	}
}
