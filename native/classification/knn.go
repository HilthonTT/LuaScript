package classification

import (
	"math"
	"sort"
)

// KNN is a lazy k-nearest-neighbours classifier. It stores the training
// set verbatim and, at predict time, votes among the k closest samples by
// Euclidean distance. Works on numeric feature vectors with arbitrary
// string labels.
type KNN struct {
	k        int
	features [][]float64
	labels   []string
}

// NewKNN returns a KNN classifier that votes over the k nearest samples.
// k is clamped to at least 1.
func NewKNN(k int) *KNN {
	if k < 1 {
		k = 1
	}
	return &KNN{k: k}
}

// Fit stores the training samples. Each feature row must match the
// dimensionality of the others; labels[i] is the class of features[i].
func (m *KNN) Fit(features [][]float64, labels []string) {
	m.features = features
	m.labels = labels
}

type neighbor struct {
	dist  float64
	label string
}

// Predict returns the majority label among the k nearest training samples.
// Ties are broken by the smallest summed distance for the tied labels.
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
	weight := make(map[string]float64) // tie-break: total proximity
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
