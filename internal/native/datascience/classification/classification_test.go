package classification

import "testing"

func numericData() ([][]float64, []string) {
	features := [][]float64{
		{1, 1}, {1.5, 2}, {2, 1.5},
		{8, 8}, {9, 8.5}, {8.5, 9},
	}
	labels := []string{"A", "A", "A", "B", "B", "B"}
	return features, labels
}

func TestKNNPredicts(t *testing.T) {
	f, l := numericData()
	m := NewKNN(3)
	m.Fit(f, l)
	if got := m.Predict([]float64{1.2, 1.3}); got != "A" {
		t.Fatalf("expected A, got %s", got)
	}
	if got := m.Predict([]float64{8.7, 8.2}); got != "B" {
		t.Fatalf("expected B, got %s", got)
	}
}

func TestPerceptronSeparates(t *testing.T) {
	f, l := numericData()
	m := NewPerceptron(0.1, 100)
	m.Fit(f, l)
	if got := m.Predict([]float64{2, 2}); got != "A" {
		t.Fatalf("expected A, got %s", got)
	}
	if got := m.Predict([]float64{9, 9}); got != "B" {
		t.Fatalf("expected B, got %s", got)
	}
}

func TestLogisticRegressionProbabilities(t *testing.T) {
	f, l := numericData()
	m := NewLogisticRegression(0.5, 1000)
	m.Fit(f, l)
	if got := m.Predict([]float64{1, 1.5}); got != "A" {
		t.Fatalf("expected A, got %s", got)
	}
	if got := m.Predict([]float64{8, 8.5}); got != "B" {
		t.Fatalf("expected B, got %s", got)
	}
	if p := m.PredictProba([]float64{9, 9}); p < 0.5 {
		t.Fatalf("expected P(positive) > 0.5 for B-like point, got %f", p)
	}
}

func TestSVMLinearSeparates(t *testing.T) {
	f, l := numericData()
	m := NewSVM(SVMConfig{Kernel: KernelLinear, C: 1.0, MaxIter: 200, Seed: 1})
	m.Fit(f, l)
	if m.SupportVectorCount() == 0 {
		t.Fatal("expected at least one support vector")
	}
	if got := m.Predict([]float64{2, 2}); got != "A" {
		t.Fatalf("expected A, got %s", got)
	}
	if got := m.Predict([]float64{9, 9}); got != "B" {
		t.Fatalf("expected B, got %s", got)
	}
}

func TestSVMRBFHandlesNonLinear(t *testing.T) {
	features := [][]float64{
		{0, 0}, {0.1, 0.1}, {1, 1}, {0.9, 1.1},
		{0, 1}, {0.1, 0.9}, {1, 0}, {1.1, 0.1},
	}
	labels := []string{"in", "in", "in", "in", "out", "out", "out", "out"}
	m := NewSVM(SVMConfig{Kernel: KernelRBF, Gamma: 2.0, C: 10.0, MaxIter: 300, Seed: 1})
	m.Fit(features, labels)

	if got := m.Predict([]float64{0.05, 0.05}); got != "in" {
		t.Fatalf("expected in, got %s", got)
	}
	if got := m.Predict([]float64{0.95, 0.05}); got != "out" {
		t.Fatalf("expected out, got %s", got)
	}
}

func TestNaiveBayesClassifies(t *testing.T) {
	c := NewClassifier("spam", "ham")
	c.Learn([]string{"buy", "cheap", "pills"}, "spam")
	c.Learn([]string{"cheap", "loan", "offer"}, "spam")
	c.Learn([]string{"lunch", "meeting", "tomorrow"}, "ham")
	c.Learn([]string{"see", "you", "lunch"}, "ham")

	if cls, _, _ := c.Classify([]string{"cheap", "buy"}); cls != "spam" {
		t.Fatalf("expected spam, got %s", cls)
	}
	if cls, _, _ := c.Classify([]string{"lunch", "meeting"}); cls != "ham" {
		t.Fatalf("expected ham, got %s", cls)
	}
}
