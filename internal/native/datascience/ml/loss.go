package ml

import "math"

type LossType int

const (
	LossNone LossType = iota
	LossCrossEntropy
	LossBinaryCrossEntropy
	LossMeanSquared
)

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

type CrossEntropy struct{}

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

func (l CrossEntropy) Df(estimate, ideal, activation float64) float64 {
	return estimate - ideal
}

type MeanSquared struct{}

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

func (l MeanSquared) Df(estimate, ideal, activation float64) float64 {
	return activation * (estimate - ideal)
}

type BinaryCrossEntropy struct{}

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

func (l BinaryCrossEntropy) Df(estimate, ideal, activation float64) float64 {
	return estimate - ideal
}
