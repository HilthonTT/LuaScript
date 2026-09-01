package classification

import "math"

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

type Perceptron struct {
	weights []float64
	bias    float64
	lr      float64
	epochs  int
	classes classMapper
}

func NewPerceptron(lr float64, epochs int) *Perceptron {
	if lr <= 0 {
		lr = 0.1
	}
	if epochs <= 0 {
		epochs = 50
	}
	return &Perceptron{lr: lr, epochs: epochs}
}

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

func (p *Perceptron) Predict(x []float64) string {
	return p.classes.label(p.score(x) >= 0)
}

type LogisticRegression struct {
	weights []float64
	bias    float64
	lr      float64
	epochs  int
	classes classMapper
}

func NewLogisticRegression(lr float64, epochs int) *LogisticRegression {
	if lr <= 0 {
		lr = 0.1
	}
	if epochs <= 0 {
		epochs = 200
	}
	return &LogisticRegression{lr: lr, epochs: epochs}
}

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

func (m *LogisticRegression) Predict(x []float64) string {
	return m.classes.label(m.prob(x) >= 0.5)
}

func (m *LogisticRegression) PredictProba(x []float64) float64 {
	return m.prob(x)
}
