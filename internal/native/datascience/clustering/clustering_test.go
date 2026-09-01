package clustering

import (
	"math/rand"
	"testing"
)

func twoBlobs() []Point {
	return []Point{
		{Entry: []float64{1, 1}},
		{Entry: []float64{1.5, 2}},
		{Entry: []float64{1.3, 1.4}},
		{Entry: []float64{8, 8}},
		{Entry: []float64{9, 8.5}},
		{Entry: []float64{8.5, 9}},
	}
}

func sameCluster(t *testing.T, labels []int, groupA, groupB []int) {
	t.Helper()
	a := labels[groupA[0]]
	for _, i := range groupA {
		if labels[i] != a {
			t.Fatalf("expected group A to share a label, got %v", labels)
		}
	}
	b := labels[groupB[0]]
	for _, i := range groupB {
		if labels[i] != b {
			t.Fatalf("expected group B to share a label, got %v", labels)
		}
	}
	if a == b {
		t.Fatalf("expected the two groups in different clusters, got %v", labels)
	}
}

func TestKMeansSeparatesBlobs(t *testing.T) {
	res := KMeans(twoBlobs(), 2, 100, 1e-6, rand.New(rand.NewSource(1)))
	if len(res.Centers) != 2 {
		t.Fatalf("expected 2 centers, got %d", len(res.Centers))
	}
	sameCluster(t, res.Assignments, []int{0, 1, 2}, []int{3, 4, 5})
}

func TestDBSCANFindsTwoClustersNoNoise(t *testing.T) {
	labels := DBSCAN(twoBlobs(), 2.0, 2)
	for i, l := range labels {
		if l == Noise {
			t.Fatalf("point %d unexpectedly labelled noise: %v", i, labels)
		}
	}
	sameCluster(t, labels, []int{0, 1, 2}, []int{3, 4, 5})
}

func TestDBSCANFlagsNoise(t *testing.T) {
	data := append(twoBlobs(), Point{Entry: []float64{100, 100}})
	labels := DBSCAN(data, 2.0, 2)
	if labels[len(labels)-1] != Noise {
		t.Fatalf("expected far point to be noise, got %v", labels)
	}
}

func TestHierarchicalLinkages(t *testing.T) {
	for _, name := range []string{"single", "complete", "average"} {
		labels := Hierarchical(twoBlobs(), 2, ParseLinkage(name))
		sameCluster(t, labels, []int{0, 1, 2}, []int{3, 4, 5})
	}
}

func TestMeanShiftDiscoversTwoModes(t *testing.T) {
	res := MeanShift(twoBlobs(), 3.0, 100)
	if len(res.Centers) != 2 {
		t.Fatalf("expected 2 modes, got %d: %v", len(res.Centers), res.Centers)
	}
	sameCluster(t, res.Assignments, []int{0, 1, 2}, []int{3, 4, 5})
}

func TestKMeansEmptyClusterDoesNotNaN(t *testing.T) {
	data := []Point{
		{Entry: []float64{1, 1}},
		{Entry: []float64{1, 1}},
		{Entry: []float64{1, 1}},
	}
	res := KMeans(data, 2, 50, 1e-6, rand.New(rand.NewSource(2)))
	for _, c := range res.Centers {
		for _, v := range c.Entry {
			if v != v {
				t.Fatalf("center contains NaN: %v", res.Centers)
			}
		}
	}
}
