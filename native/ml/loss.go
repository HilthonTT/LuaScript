package ml

import "math"

// LossType represents a loss function
type LossType int

const (
	// LossNone signifies unspecified loss
	LossNone LossType = iota
	// LossCrossEntropy is cross entropy loss
	LossCrossEntropy
	// LossBinaryCrossEntropy is the special case of binary cross entropy loss
	LossBinaryCrossEntropy
	// LossMeanSquared is MSE
	LossMeanSquared
)

// Loss is satisfied by loss functions
type Loss interface {
	F(estimate, ideal [][]float64) float64
	Df(estimate, ideal, activation float64) float64
}

func (l LossType) String() string {
	switch l {
	case LossCrossEntropy:
		return "CE"
	case LossBinaryCrossEntropy:
		return "BinCE"
	case LossMeanSquared:
		return "MSE"
	default:
		return "N/A"
	}
}

// GetLoss returns a loss function given a LossType
func GetLoss(loss LossType) Loss {
	switch loss {
	case LossCrossEntropy:
		return CrossEntropy{}
	case LossMeanSquared:
		return MeanSquared{}
	case LossBinaryCrossEntropy:
		return BinaryCrossEntropy{}
	}
	return CrossEntropy{}
}

// CrossEntropy is CE loss
type CrossEntropy struct{}

// F is CE(...)
func (l CrossEntropy) F(estimate, ideal [][]float64) float64 {
	var sum float64

	for i := range estimate {
		ce := 0.0
		for j := range estimate[i] {
			ce += ideal[i][j] * math.Log(estimate[i][j])
		}

		sum += ce
	}

	return sum / float64(len(estimate))
}

// Df is CE'(...)
func (l CrossEntropy) Df(estimate, ideal, activation float64) float64 {
	return estimate - ideal
}

// MeanSquared in MSE loss
type MeanSquared struct{}

// F is CE(...)
func (l MeanSquared) F(estimate, ideal [][]float64) float64 {
	var sum float64
	for i := range estimate {
		for j := range estimate[i] {
			d := estimate[i][j] - ideal[i][j]
			sum += d * d
		}
	}
	return sum / float64(len(estimate)*len(estimate[0]))
}

// Df is CE'(...)
func (l MeanSquared) Df(estimate, ideal, activation float64) float64 {
	return activation * (estimate - ideal)
}

// BinaryCrossEntropy is binary CE loss
type BinaryCrossEntropy struct{}

// F is CE(...)
func (l BinaryCrossEntropy) F(estimate, ideal [][]float64) float64 {
	epsilon := 1e-16
	var sum float64
	for i := range estimate {
		ce := 0.0
		for j := range estimate[i] {
			ce += ideal[i][j]*math.Log(estimate[i][j]+epsilon) + (1.0-ideal[i][j])*math.Log(1.0-estimate[i][j]+epsilon)
		}
		sum -= ce
	}
	return sum / float64(len(estimate))
}

// Df is CE'(...)
func (l BinaryCrossEntropy) Df(estimate, ideal, activation float64) float64 {
	return estimate - ideal
}
