package ml

import "math"

type Mode int

const (
	ModeDefault Mode = iota
	ModeMultiClass
	ModeRegression
	ModeBinary
	ModeMultiLabel
)

type ActivationType int

const (
	ActivationNone ActivationType = iota
	ActivationSigmoid
	ActivationTanh
	ActivationReLU
	ActivationLinear
	ActivationSoftmax
)

type Differentiable interface {
	F(float64) float64
	Df(float64) float64
}

func OutputActivation(c Mode) ActivationType {
	switch c {
	case ModeMultiClass:
		return ActivationSoftmax
	case ModeRegression:
		return ActivationLinear
	case ModeBinary, ModeMultiLabel:
		return ActivationSigmoid
	}
	return ActivationNone
}

func GetActivation(act ActivationType) Differentiable {
	switch act {
	case ActivationSigmoid:
		return Sigmoid{}
	case ActivationTanh:
		return Tanh{}
	case ActivationReLU:
		return ReLU{}
	case ActivationLinear:
		return Linear{}
	case ActivationSoftmax:
		return Linear{}
	}
	return Linear{}
}

type Sigmoid struct{}

func (a Sigmoid) F(x float64) float64 { return Logistic(x, 1) }

func (a Sigmoid) Df(y float64) float64 { return y * (1 - y) }

func Logistic(x, a float64) float64 {
	return 1 / (1 + math.Exp(-a*x))
}

type Tanh struct{}

func (a Tanh) F(x float64) float64 {
	return math.Tanh(x)
}

func (a Tanh) Df(y float64) float64 {
	return 1 - y*y
}

type ReLU struct{}

func (a ReLU) F(x float64) float64 {
	return math.Max(x, 0)
}

func (a ReLU) Df(y float64) float64 {
	if y > 0 {
		return 1
	}
	return 0
}

type Linear struct{}

func (a Linear) F(x float64) float64 {
	return x
}

func (a Linear) Df(x float64) float64 {
	return 1
}
