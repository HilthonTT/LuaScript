package classification

import "math"

type SVMKernel int

const (
	KernelLinear SVMKernel = iota
	KernelRBF
	KernelPoly
)

func ParseKernel(s string) SVMKernel {
	switch s {
	case "linear":
		return KernelLinear
	case "poly":
		return KernelPoly
	default:
		return KernelRBF
	}
}

type SVM struct {
	kernel  SVMKernel
	gamma   float64
	coef0   float64
	degree  int
	c       float64
	tol     float64
	maxIter int

	sv      [][]float64
	svY     []float64
	svAlpha []float64
	bias    float64
	classes classMapper

	rng *lcg
}

type SVMConfig struct {
	Kernel  SVMKernel
	Gamma   float64
	Coef0   float64
	Degree  int
	C       float64
	Tol     float64
	MaxIter int
	Seed    int64
}

func NewSVM(cfg SVMConfig) *SVM {
	if cfg.C <= 0 {
		cfg.C = 1.0
	}
	if cfg.Gamma <= 0 {
		cfg.Gamma = 0.5
	}
	if cfg.Degree <= 0 {
		cfg.Degree = 3
	}
	if cfg.Tol <= 0 {
		cfg.Tol = 1e-3
	}
	if cfg.MaxIter <= 0 {
		cfg.MaxIter = 100
	}
	return &SVM{
		kernel:  cfg.Kernel,
		gamma:   cfg.Gamma,
		coef0:   cfg.Coef0,
		degree:  cfg.Degree,
		c:       cfg.C,
		tol:     cfg.Tol,
		maxIter: cfg.MaxIter,
		rng:     newLCG(cfg.Seed),
	}
}

func (s *SVM) kernelFn(a, b []float64) float64 {
	switch s.kernel {
	case KernelLinear:
		return dot(a, b)
	case KernelPoly:
		return math.Pow(s.gamma*dot(a, b)+s.coef0, float64(s.degree))
	default:
		sum := 0.0
		for i := range a {
			d := a[i] - b[i]
			sum += d * d
		}
		return math.Exp(-s.gamma * sum)
	}
}

func dot(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

func (s *SVM) Fit(features [][]float64, labels []string) {
	for _, l := range labels {
		s.classes.observe(l)
	}

	n := len(features)
	y := make([]float64, n)
	for i, l := range labels {
		if l == s.classes.pos {
			y[i] = 1
		} else {
			y[i] = -1
		}
	}

	gram := make([][]float64, n)
	for i := range n {
		gram[i] = make([]float64, n)
		for j := range n {
			gram[i][j] = s.kernelFn(features[i], features[j])
		}
	}

	alpha := make([]float64, n)
	b := 0.0

	decision := func(k int) float64 {
		sum := b
		for i := range n {
			if alpha[i] != 0 {
				sum += alpha[i] * y[i] * gram[i][k]
			}
		}
		return sum
	}

	passes := 0
	for passes < s.maxIter {
		changed := 0
		for i := range n {
			ei := decision(i) - y[i]
			if (y[i]*ei < -s.tol && alpha[i] < s.c) || (y[i]*ei > s.tol && alpha[i] > 0) {
				j := s.rng.intn(n - 1)
				if j >= i {
					j++
				}
				ej := decision(j) - y[j]

				ai, aj := alpha[i], alpha[j]

				var lo, hi float64
				if y[i] != y[j] {
					lo = math.Max(0, aj-ai)
					hi = math.Min(s.c, s.c+aj-ai)
				} else {
					lo = math.Max(0, ai+aj-s.c)
					hi = math.Min(s.c, ai+aj)
				}
				if lo == hi {
					continue
				}

				eta := 2*gram[i][j] - gram[i][i] - gram[j][j]
				if eta >= 0 {
					continue
				}

				aj -= y[j] * (ei - ej) / eta
				aj = clamp(aj, lo, hi)
				if math.Abs(aj-alpha[j]) < 1e-5 {
					continue
				}
				ai += y[i] * y[j] * (alpha[j] - aj)

				b1 := b - ei - y[i]*(ai-alpha[i])*gram[i][i] - y[j]*(aj-alpha[j])*gram[i][j]
				b2 := b - ej - y[i]*(ai-alpha[i])*gram[i][j] - y[j]*(aj-alpha[j])*gram[j][j]
				switch {
				case ai > 0 && ai < s.c:
					b = b1
				case aj > 0 && aj < s.c:
					b = b2
				default:
					b = (b1 + b2) / 2
				}

				alpha[i], alpha[j] = ai, aj
				changed++
			}
		}
		if changed == 0 {
			passes++
		} else {
			passes = 0
		}
	}

	s.sv = s.sv[:0]
	s.svY = s.svY[:0]
	s.svAlpha = s.svAlpha[:0]
	for i := range n {
		if alpha[i] > 1e-8 {
			s.sv = append(s.sv, features[i])
			s.svY = append(s.svY, y[i])
			s.svAlpha = append(s.svAlpha, alpha[i])
		}
	}
	s.bias = b
}

func (s *SVM) score(x []float64) float64 {
	sum := s.bias
	for i := range s.sv {
		sum += s.svAlpha[i] * s.svY[i] * s.kernelFn(s.sv[i], x)
	}
	return sum
}

func (s *SVM) Predict(x []float64) string {
	return s.classes.label(s.score(x) >= 0)
}

func (s *SVM) DecisionFunction(x []float64) float64 {
	return s.score(x)
}

func (s *SVM) SupportVectorCount() int {
	return len(s.sv)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

type lcg struct{ state uint64 }

func newLCG(seed int64) *lcg {
	if seed == 0 {
		seed = 1
	}
	return &lcg{state: uint64(seed)}
}

func (g *lcg) next() uint64 {
	g.state = g.state*6364136223846793005 + 1442695040888963407
	return g.state
}

func (g *lcg) intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(g.next() >> 33 % uint64(n))
}
