package clustering

const (
	Noise     = 0
	undefined = -1
)

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

		seeds := make([]int, len(neighbors))
		copy(seeds, neighbors)
		for s := 0; s < len(seeds); s++ {
			q := seeds[s]
			if labels[q] == Noise {
				labels[q] = cluster
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
