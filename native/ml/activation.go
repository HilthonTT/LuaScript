package ml

import "math"

// Mode denotes interference mode
type Mode int

const (
	// ModeDefault is unspecified mode
	ModeDefault Mode = iota
	// ModeMultiClass is for one-hot encoded classification, applies softmax output layer
	ModeMultiClass // ModeRegression is regression, applies linear output layer
	ModeRegression
	// ModeBinary is binary classification, applies sigmoid output layer
	ModeBinary
	// ModeMultiLabel is for multilabel classification, applies sigmoid output layer
	ModeMultiLabel
)

// ActivationType is represents a neuron activation function
type ActivationType int

const (
	// ActivationNone is no activation
	ActivationNone ActivationType = iota
	// ActivationSigmoid is a sigmoid activation
	ActivationSigmoid
	// ActivationTanh is hyperbolic activation
	ActivationTanh
	// ActivationReLU is rectified linear unit activation
	ActivationReLU
	// ActivationLinear is linear activation
	ActivationLinear
	// ActivationSoftmax is a softmax activation (per layer)
	ActivationSoftmax
)

// Differentiable is an activation function and its first order derivative,
// where the latter is expressed as a function of the former for efficiency
type Differentiable interface {
	F(float64) float64
	Df(float64) float64
}

// OutputActivation returns activation corresponding to prediction mode
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

// GetActivation returns the concrete activation given an ActivationType
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

// Sigmoid is a logistic activator in the special case of a = 1
type Sigmoid struct{}

// F is Sigmoid(x)
func (a Sigmoid) F(x float64) float64 { return Logistic(x, 1) }

// Df is Sigmoid'(y), where y = Sigmoid(x)
func (a Sigmoid) Df(y float64) float64 { return y * (1 - y) }

// Logistic is the logistic function
func Logistic(x, a float64) float64 {
	return 1 / (1 + math.Exp(-a*x))
}

// Tanh is a hyperbolic activator
type Tanh struct{}

// F is Tanh(x)
func (a Tanh) F(x float64) float64 {
	return (1 - math.Exp(-2*x)) / (1 + math.Exp(-2*x))
}

// Df is Tanh'(y), where y = Tanh(x)
func (a Tanh) Df(y float64) float64 {
	return 1 - math.Pow(y, 2)
}

// ReLU is a rectified linear unit activator
type ReLU struct{}

// F is ReLU(x)
func (a ReLU) F(x float64) float64 {
	return math.Max(x, 0)
}

// Df is ReLU'(y), where y = ReLU(x)
func (a ReLU) Df(y float64) float64 {
	if y > 0 {
		return 1
	}
	return 0
}

// Linear is a linear activator
type Linear struct{}

// F is the identity function
func (a Linear) F(x float64) float64 {
	return x
}

// Df is constant
func (a Linear) Df(x float64) float64 {
	return 1
}
