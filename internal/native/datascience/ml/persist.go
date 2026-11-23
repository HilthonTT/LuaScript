package ml

import (
	"encoding/json"
	"errors"
	"fmt"
)

type Dump struct {
	Config  *Config
	Weights [][][]float64
}

func (n *Neural) ApplyWeights(weights [][][]float64) {
	for i, l := range n.Layers {
		for j := range l.Neurons {
			for k := range l.Neurons[j].In {
				n.Layers[i].Neurons[j].In[k].Weight = weights[i][j][k]
			}
		}
	}
}

func (n Neural) Weights() [][][]float64 {
	weights := make([][][]float64, len(n.Layers))
	for i, l := range n.Layers {
		weights[i] = make([][]float64, len(l.Neurons))
		for j, n := range l.Neurons {
			weights[i][j] = make([]float64, len(n.In))
			for k, in := range n.In {
				weights[i][j][k] = in.Weight
			}
		}
	}
	return weights
}

func (n Neural) Dump() *Dump {
	return &Dump{
		Config:  n.Config,
		Weights: n.Weights(),
	}
}

// MaxWeights caps how many synapses a network may have. A serialized model is
// data, so its layer sizes must not be able to request an allocation large
// enough to take the process down.
const MaxWeights = 1 << 24

// Validate reports whether c describes a network that can be built: positive
// sizes, known enum values and a bounded number of weights.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("missing config")
	}
	if c.Inputs <= 0 {
		return fmt.Errorf("inputs must be a positive integer, got %d", c.Inputs)
	}
	if len(c.Layout) == 0 {
		return errors.New("layout must list at least one layer")
	}
	if c.Activation < ActivationNone || c.Activation > ActivationSoftmax {
		return fmt.Errorf("unknown activation %d", c.Activation)
	}
	if c.Mode < ModeDefault || c.Mode > ModeMultiLabel {
		return fmt.Errorf("unknown mode %d", c.Mode)
	}
	if c.Loss < LossNone || c.Loss > LossMeanSquared {
		return fmt.Errorf("unknown loss %d", c.Loss)
	}
	total, prev := 0, c.Inputs
	for i, n := range c.Layout {
		if n <= 0 {
			return fmt.Errorf("layer %d size must be a positive integer, got %d", i+1, n)
		}
		// prev and n are both positive; checking before multiplying keeps the
		// running total from overflowing.
		if n > MaxWeights || prev > (MaxWeights-total)/n {
			return fmt.Errorf("network exceeds %d weights", MaxWeights)
		}
		total += prev * n
		if c.Bias {
			if total > MaxWeights-n {
				return fmt.Errorf("network exceeds %d weights", MaxWeights)
			}
			total += n
		}
		prev = n
	}
	return nil
}

func FromDump(dump *Dump) (*Neural, error) {
	if err := dump.Config.Validate(); err != nil {
		return nil, err
	}
	n := NewNeural(dump.Config)
	if len(dump.Weights) != len(n.Layers) {
		return nil, fmt.Errorf("weights have %d layers, config has %d", len(dump.Weights), len(n.Layers))
	}
	for i, l := range n.Layers {
		if len(dump.Weights[i]) != len(l.Neurons) {
			return nil, fmt.Errorf("weights layer %d has %d neurons, config has %d", i+1, len(dump.Weights[i]), len(l.Neurons))
		}
		for j, neuron := range l.Neurons {
			if len(dump.Weights[i][j]) != len(neuron.In) {
				return nil, fmt.Errorf("weights layer %d neuron %d has %d inputs, want %d", i+1, j+1, len(dump.Weights[i][j]), len(neuron.In))
			}
		}
	}
	n.ApplyWeights(dump.Weights)

	return n, nil
}

func (n Neural) Marshal() ([]byte, error) {
	return json.Marshal(n.Dump())
}

func Unmarshal(bytes []byte) (*Neural, error) {
	var dump Dump
	if err := json.Unmarshal(bytes, &dump); err != nil {
		return nil, err
	}
	return FromDump(&dump)
}
