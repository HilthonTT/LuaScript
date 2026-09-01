package classification

import (
	"math"
	"sort"
)

type KNN struct {
	k        int
	features [][]float64
	labels   []string
}

func NewKNN(k int) *KNN {
	if k < 1 {
		k = 1
	}
	return &KNN{k: k}
}

func (m *KNN) Fit(features [][]float64, labels []string) {
	m.features = features
	m.labels = labels
}

type neighbor struct {
	dist  float64
	label string
}

func (m *KNN) Predict(x []float64) string {
	neighbors := make([]neighbor, len(m.features))
	for i := range m.features {
		neighbors[i] = neighbor{dist: euclidean(x, m.features[i]), label: m.labels[i]}
	}
	sort.Slice(neighbors, func(a, b int) bool {
		return neighbors[a].dist < neighbors[b].dist
	})

	k := min(m.k, len(neighbors))

	votes := make(map[string]int)
	weight := make(map[string]float64)
	for i := range k {
		votes[neighbors[i].label]++
		weight[neighbors[i].label] += 1.0 / (1.0 + neighbors[i].dist)
	}

	best := ""
	bestVotes := -1
	bestWeight := -1.0
	for label, count := range votes {
		if count > bestVotes || (count == bestVotes && weight[label] > bestWeight) {
			best = label
			bestVotes = count
			bestWeight = weight[label]
		}
	}
	return best
}

func euclidean(a, b []float64) float64 {
	sum := 0.0
	for i := range a {
		d := a[i] - b[i]
		sum += d * d
	}
	return math.Sqrt(sum)
}
