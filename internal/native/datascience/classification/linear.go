package classification

import "math"

// Binary linear classifiers. Both map a two-class problem onto numeric
// labels, learn a weight vector + bias, and classify by the sign (or
// sigmoid) of w·x + b. The first label seen during Fit becomes the
// negative class, the second the positive class.

// classMapper records the two string classes in first-seen order and maps
// between them and numeric targets.
type classMapper struct {
	neg, pos string
	set      bool
}

func (c *classMapper) observe(label string) {
	if !c.set {
		c.neg = label
		c.set = true
		return
	}
	if c.pos == "" && label != c.neg {
		c.pos = label
	}
}

func (c *classMapper) target(label string) float64 {
	if label == c.pos {
		return 1
	}
	return 0
}

func (c *classMapper) label(positive bool) string {
	if positive {
		return c.pos
	}
	return c.neg
}

// Perceptron is the classic online linear classifier. It performs the
// perceptron update (w += lr·(y−ŷ)·x) over the data for a number of
// epochs. Converges to a separating hyperplane when the data is linearly
// separable; otherwise it just stops after the epoch budget.
type Perceptron struct {
	weights []float64
	bias    float64
	lr      float64
	epochs  int
	classes classMapper
}

// NewPerceptron builds a perceptron with the given learning rate and epoch
// budget. Sensible defaults are applied for non-positive values.
func NewPerceptron(lr float64, epochs int) *Perceptron {
	if lr <= 0 {
		lr = 0.1
	}
	if epochs <= 0 {
		epochs = 50
	}
	return &Perceptron{lr: lr, epochs: epochs}
}

// Fit trains the perceptron on the labelled feature rows.
func (p *Perceptron) Fit(features [][]float64, labels []string) {
	for _, l := range labels {
		p.classes.observe(l)
	}
	dims := len(features[0])
	p.weights = make([]float64, dims)

	for e := 0; e < p.epochs; e++ {
		for i := range features {
			y := p.classes.target(labels[i])
			pred := 0.0
			if p.score(features[i]) >= 0 {
				pred = 1
			}
			err := y - pred
			if err == 0 {
				continue
			}
			for d := range p.weights {
				p.weights[d] += p.lr * err * features[i][d]
			}
			p.bias += p.lr * err
		}
	}
}

func (p *Perceptron) score(x []float64) float64 {
	s := p.bias
	for d := range p.weights {
		s += p.weights[d] * x[d]
	}
	return s
}

// Predict returns the predicted class label for x.
func (p *Perceptron) Predict(x []float64) string {
	return p.classes.label(p.score(x) >= 0)
}

// LogisticRegression is a binary classifier trained by batch gradient
// descent on the log-loss. Unlike the perceptron it also yields calibrated
// class probabilities via the logistic (sigmoid) link.
type LogisticRegression struct {
	weights []float64
	bias    float64
	lr      float64
	epochs  int
	classes classMapper
}

// NewLogisticRegression builds a logistic-regression model with the given
// learning rate and epoch budget, applying defaults for non-positive values.
func NewLogisticRegression(lr float64, epochs int) *LogisticRegression {
	if lr <= 0 {
		lr = 0.1
	}
	if epochs <= 0 {
		epochs = 200
	}
	return &LogisticRegression{lr: lr, epochs: epochs}
}

// Fit trains the model with full-batch gradient descent.
func (m *LogisticRegression) Fit(features [][]float64, labels []string) {
	for _, l := range labels {
		m.classes.observe(l)
	}
	n := len(features)
	dims := len(features[0])
	m.weights = make([]float64, dims)

	for e := 0; e < m.epochs; e++ {
		gradW := make([]float64, dims)
		gradB := 0.0
		for i := range features {
			pred := m.prob(features[i])
			err := pred - m.classes.target(labels[i])
			for d := range gradW {
				gradW[d] += err * features[i][d]
			}
			gradB += err
		}
		scale := m.lr / float64(n)
		for d := range m.weights {
			m.weights[d] -= scale * gradW[d]
		}
		m.bias -= scale * gradB
	}
}

func (m *LogisticRegression) prob(x []float64) float64 {
	z := m.bias
	for d := range m.weights {
		z += m.weights[d] * x[d]
	}
	return 1.0 / (1.0 + math.Exp(-z))
}

// Predict returns the predicted class label for x.
func (m *LogisticRegression) Predict(x []float64) string {
	return m.classes.label(m.prob(x) >= 0.5)
}

// PredictProba returns P(positive class | x) in [0, 1].
func (m *LogisticRegression) PredictProba(x []float64) float64 {
	return m.prob(x)
}
