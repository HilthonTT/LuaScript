// Package luaml is the require()-able host module that exposes the ml neural-
// network engine (native/ml + native/ml/training) to LuaScript as `ml`. It
// lives in its own package — rather than alongside the engine — because it
// imports both ml and ml/training, and training already imports ml; putting the
// loader in package ml would create an import cycle.
//
// Lua surface:
//
//	local ml = require("ml")
//
//	local net = ml.new({
//	  inputs     = 2,
//	  layout     = { 4, 1 },        -- hidden(4) -> output(1)
//	  activation = "tanh",          -- sigmoid | tanh | relu | linear
//	  mode       = "binary",        -- default | binary | multiclass | regression | multilabel
//	  bias       = true,
//	  -- optional: loss, weight = { kind = "normal", stddev = 0.5 }
//	})
//
//	net:train({
//	  data       = {{ input = {0,0}, response = {0} }, ... },
//	  iterations = 2000,
//	  solver     = { kind = "adam", lr = 0.05 },   -- or { kind = "sgd", lr=.1, momentum=.9 }
//	  verbosity  = 500,                            -- print every N epochs (0 = silent)
//	  -- optional: validation = {...}, batch = { size = 16, parallelism = 4 }
//	})
//
//	local out = net:predict({1, 0})   -- -> array of output values
//	local blob = net:marshal()        -- JSON string; ml.load(blob) restores it
//
// Every network method is colon-called, so the receiver is args[0] and the
// first real argument is at 1-based position 2 (matching the rest of the
// native modules).
package luaml

import (
	"os"

	"github.com/hilthontt/luascript/internal/native/datascience/ml"
	"github.com/hilthontt/luascript/internal/native/datascience/ml/training"
	"github.com/hilthontt/luascript/internal/vm"
)

// RegisterMLPreload installs the loader under package.preload as "ml".
func RegisterMLPreload(v *vm.VM) {
	vm.RegisterPreload(v, "ml", mlLoader)
}

func mlLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	m := vm.NewTable(0, 4)
	m.Set("VERSION", "0.1.0")

	m.Set("new", &vm.GoFunc{Name: "ml.new", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		cfg := parseConfig("ml.new", vm.TableArg("ml.new", 1, args))
		return []vm.Value{wrapNet(ml.NewNeural(cfg))}
	}})

	m.Set("load", &vm.GoFunc{Name: "ml.load", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		blob := vm.StringArg("ml.load", 1, args)
		n, err := ml.Unmarshal([]byte(blob))
		if err != nil {
			panic(vm.Errorf("ml.load: %s", err.Error()))
		}
		return []vm.Value{wrapNet(n)}
	}})

	m.Set("load_file", &vm.GoFunc{Name: "ml.load_file", Fn: func(_ *vm.VM, args []vm.Value) []vm.Value {
		path := vm.StringArg("ml.load_file", 1, args)
		blob, err := os.ReadFile(path)
		if err != nil {
			panic(vm.Errorf("ml.load_file: %s", err.Error()))
		}
		n, err := ml.Unmarshal(blob)
		if err != nil {
			panic(vm.Errorf("ml.load_file: %s", err.Error()))
		}
		return []vm.Value{wrapNet(n)}
	}})

	return []vm.Value{m}
}

// ---------------------------------------------------------------------------
// Network object
// ---------------------------------------------------------------------------

// wrapNet exposes a *ml.Neural as a Lua object. Each method closes over n, so
// it has direct access to the Go network without round-tripping through the
// table — the same pattern the dataframe module uses.
func wrapNet(n *ml.Neural) *vm.Table {
	methods := vm.NewTable(0, 8)
	set := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		methods.Set(name, &vm.GoFunc{Name: "ml.net:" + name, Fn: fn})
	}

	set("predict", func(_ *vm.VM, args []vm.Value) []vm.Value {
		input := floatArray("net:predict", vm.TableArg("net:predict", 2, args))
		if len(input) != n.Config.Inputs {
			panic(vm.Errorf("net:predict: expected %d inputs, got %d", n.Config.Inputs, len(input)))
		}
		return []vm.Value{floatSliceToTable(n.Predict(input))}
	})

	set("train", func(_ *vm.VM, args []vm.Value) []vm.Value {
		trainNet(n, vm.TableArg("net:train", 2, args))
		return []vm.Value{args[0]} // return self, so calls can chain
	})

	set("num_weights", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(n.NumWeights())}
	})

	set("weights", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{weights3DToTable(n.Weights())}
	})

	set("marshal", func(_ *vm.VM, _ []vm.Value) []vm.Value {
		b, err := n.Marshal()
		if err != nil {
			panic(vm.Errorf("net:marshal: %s", err.Error()))
		}
		return []vm.Value{string(b)}
	})

	set("save", func(_ *vm.VM, args []vm.Value) []vm.Value {
		path := vm.StringArg("net:save", 2, args)
		b, err := n.Marshal()
		if err != nil {
			panic(vm.Errorf("net:save: %s", err.Error()))
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			panic(vm.Errorf("net:save: %s", err.Error()))
		}
		return nil
	})

	t := vm.NewTable(0, 0)
	mt := vm.NewTable(0, 2)
	mt.Set("__index", methods)
	mt.Set("__tostring", &vm.GoFunc{Name: "ml.net:__tostring", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{n.String()}
	}})
	t.SetMetatable(mt)
	return t
}

// trainNet parses a training-options table and runs a full training session in
// place. An online trainer is used by default; supplying a `batch` table
// switches to the parallelized batch trainer.
func trainNet(n *ml.Neural, opts *vm.Table) {
	iterations := int(intField("net:train", opts, "iterations", 1000))
	verbosity := int(intField("net:train", opts, "verbosity", 0))

	dataT, ok := tableField(opts, "data")
	if !ok {
		panic(vm.Errorf("net:train: option 'data' (array of {input=..., response=...}) is required"))
	}
	data := parseExamples("net:train.data", dataT)

	var validation training.Examples
	if vt, ok := tableField(opts, "validation"); ok {
		validation = parseExamples("net:train.validation", vt)
	}

	// Validate example shapes against the network before training starts:
	// a short response would otherwise index out of range deep inside the
	// trainer — on a worker goroutine for the batch trainer, where the panic
	// is unrecoverable and kills the whole process.
	inSize := n.Config.Inputs
	outSize := n.Config.Layout[len(n.Config.Layout)-1]
	checkShapes := func(site string, ex training.Examples) {
		for i, e := range ex {
			if len(e.Input) != inSize {
				panic(vm.Errorf("%s: example %d input has %d values, network expects %d",
					site, i+1, len(e.Input), inSize))
			}
			if len(e.Response) != outSize {
				panic(vm.Errorf("%s: example %d response has %d values, network expects %d",
					site, i+1, len(e.Response), outSize))
			}
		}
	}
	checkShapes("net:train.data", data)
	checkShapes("net:train.validation", validation)

	solver := parseSolver(opts.Get("solver"))

	var trainer training.Trainer
	if bt, ok := tableField(opts, "batch"); ok {
		size := int(intField("net:train.batch", bt, "size", 16))
		parallelism := int(intField("net:train.batch", bt, "parallelism", 1))
		if size < 1 {
			panic(vm.Errorf("net:train.batch: 'size' must be a positive integer, got %d", size))
		}
		if parallelism < 1 {
			panic(vm.Errorf("net:train.batch: 'parallelism' must be a positive integer, got %d", parallelism))
		}
		trainer = training.NewBatchTrainer(solver, verbosity, size, parallelism)
	} else {
		trainer = training.NewTrainer(solver, verbosity)
	}

	trainer.Train(n, data, validation, iterations)
}

// ---------------------------------------------------------------------------
// Config / option parsing
// ---------------------------------------------------------------------------

func parseConfig(site string, t *vm.Table) *ml.Config {
	inputs := int(intField(site, t, "inputs", 0))
	if inputs <= 0 {
		panic(vm.Errorf("%s: 'inputs' must be a positive integer", site))
	}

	layoutT, ok := tableField(t, "layout")
	if !ok {
		panic(vm.Errorf("%s: 'layout' (array of layer sizes) is required", site))
	}
	layout := intArray(site+".layout", layoutT)
	if len(layout) == 0 {
		panic(vm.Errorf("%s: 'layout' must list at least one layer", site))
	}

	c := &ml.Config{
		Inputs:     inputs,
		Layout:     layout,
		Activation: activationFromString(site, stringField(t, "activation", "sigmoid")),
		Mode:       modeFromString(site, stringField(t, "mode", "default")),
		Bias:       boolField(t, "bias", false),
	}
	if l := stringField(t, "loss", ""); l != "" {
		c.Loss = lossFromString(site, l)
	}
	if wt, ok := tableField(t, "weight"); ok {
		c.Weight = weightFromTable(site, wt)
	}
	return c
}

func parseSolver(v vm.Value) training.Solver {
	if v == nil {
		return training.NewAdam(0, 0, 0, 0) // library defaults
	}
	t, ok := v.(*vm.Table)
	if !ok {
		panic(vm.Errorf("net:train: 'solver' must be a table, got %s", vm.TypeName(v)))
	}
	switch kind := stringField(t, "kind", "adam"); kind {
	case "adam":
		return training.NewAdam(
			floatField(t, "lr", 0),
			floatField(t, "beta", 0),
			floatField(t, "beta2", 0),
			floatField(t, "epsilon", 0),
		)
	case "sgd":
		return training.NewSGD(
			floatField(t, "lr", 0),
			floatField(t, "momentum", 0),
			floatField(t, "decay", 0),
			boolField(t, "nesterov", false),
		)
	default:
		panic(vm.Errorf("net:train: unknown solver kind %q (use \"adam\" or \"sgd\")", kind))
	}
}

func weightFromTable(site string, t *vm.Table) ml.WeightInitializer {
	stddev := floatField(t, "stddev", 1.0)
	mean := floatField(t, "mean", 0.0)
	switch kind := stringField(t, "kind", "uniform"); kind {
	case "uniform":
		return ml.NewUniform(stddev, mean)
	case "normal":
		return ml.NewNormal(stddev, mean)
	default:
		panic(vm.Errorf("%s: unknown weight kind %q (use \"uniform\" or \"normal\")", site, kind))
	}
}

func activationFromString(site, s string) ml.ActivationType {
	switch s {
	case "sigmoid":
		return ml.ActivationSigmoid
	case "tanh":
		return ml.ActivationTanh
	case "relu":
		return ml.ActivationReLU
	case "linear":
		return ml.ActivationLinear
	case "softmax":
		return ml.ActivationSoftmax
	default:
		panic(vm.Errorf("%s: unknown activation %q (sigmoid|tanh|relu|linear|softmax)", site, s))
	}
}

func modeFromString(site, s string) ml.Mode {
	switch s {
	case "default":
		return ml.ModeDefault
	case "multiclass":
		return ml.ModeMultiClass
	case "regression":
		return ml.ModeRegression
	case "binary":
		return ml.ModeBinary
	case "multilabel":
		return ml.ModeMultiLabel
	default:
		panic(vm.Errorf("%s: unknown mode %q (default|multiclass|regression|binary|multilabel)", site, s))
	}
}

func lossFromString(site, s string) ml.LossType {
	switch s {
	case "crossentropy", "cross_entropy":
		return ml.LossCrossEntropy
	case "binary_crossentropy", "binary_cross_entropy":
		return ml.LossBinaryCrossEntropy
	case "mse", "mean_squared":
		return ml.LossMeanSquared
	default:
		panic(vm.Errorf("%s: unknown loss %q (crossentropy|binary_crossentropy|mse)", site, s))
	}
}

// ---------------------------------------------------------------------------
// Lua marshalling helpers
// ---------------------------------------------------------------------------

// parseExamples reads an array of { input = {...}, response = {...} } records
// into training.Examples.
func parseExamples(site string, t *vm.Table) training.Examples {
	n := int(t.Len())
	ex := make(training.Examples, n)
	for i := 1; i <= n; i++ {
		row, ok := t.Get(int64(i)).(*vm.Table)
		if !ok {
			panic(vm.Errorf("%s: example %d must be a table {input=..., response=...}", site, i))
		}
		in, ok := row.Get("input").(*vm.Table)
		if !ok {
			panic(vm.Errorf("%s: example %d is missing an 'input' array", site, i))
		}
		resp, ok := row.Get("response").(*vm.Table)
		if !ok {
			panic(vm.Errorf("%s: example %d is missing a 'response' array", site, i))
		}
		ex[i-1] = training.Example{
			Input:    floatArray(site, in),
			Response: floatArray(site, resp),
		}
	}
	return ex
}

// floatArray reads the 1..n array part of t into a []float64, promoting ints.
func floatArray(site string, t *vm.Table) []float64 {
	n := int(t.Len())
	out := make([]float64, n)
	for i := 1; i <= n; i++ {
		f, ok := vm.ToFloat(t.Get(int64(i)))
		if !ok {
			panic(vm.Errorf("%s: array element %d must be a number", site, i))
		}
		out[i-1] = f
	}
	return out
}

// intArray reads the 1..n array part of t into a []int.
func intArray(site string, t *vm.Table) []int {
	n := int(t.Len())
	out := make([]int, n)
	for i := 1; i <= n; i++ {
		v, ok := vm.ToInteger(t.Get(int64(i)))
		if !ok {
			panic(vm.Errorf("%s: array element %d must be an integer", site, i))
		}
		out[i-1] = int(v)
	}
	return out
}

func floatSliceToTable(xs []float64) *vm.Table {
	t := vm.NewTable(len(xs), 0)
	for i, x := range xs {
		t.Set(int64(i+1), x)
	}
	return t
}

// weights3DToTable boxes the network's [layer][neuron][weight] slice into
// nested Lua arrays.
func weights3DToTable(w [][][]float64) *vm.Table {
	layers := vm.NewTable(len(w), 0)
	for i, layer := range w {
		lt := vm.NewTable(len(layer), 0)
		for j, neuron := range layer {
			lt.Set(int64(j+1), floatSliceToTable(neuron))
		}
		layers.Set(int64(i+1), lt)
	}
	return layers
}

// ---------------------------------------------------------------------------
// Table-field readers (config/option tables are keyed by string)
// ---------------------------------------------------------------------------

func intField(site string, t *vm.Table, key string, dflt int64) int64 {
	v := t.Get(key)
	if v == nil {
		return dflt
	}
	i, ok := vm.ToInteger(v)
	if !ok {
		panic(vm.Errorf("%s: field %q must be an integer", site, key))
	}
	return i
}

func floatField(t *vm.Table, key string, dflt float64) float64 {
	v := t.Get(key)
	if v == nil {
		return dflt
	}
	f, ok := vm.ToFloat(v)
	if !ok {
		panic(vm.Errorf("field %q must be a number", key))
	}
	return f
}

func stringField(t *vm.Table, key, dflt string) string {
	if s, ok := t.Get(key).(string); ok {
		return s
	}
	return dflt
}

func boolField(t *vm.Table, key string, dflt bool) bool {
	v := t.Get(key)
	if v == nil {
		return dflt
	}
	return vm.IsTruthy(v)
}

func tableField(t *vm.Table, key string) (*vm.Table, bool) {
	tbl, ok := t.Get(key).(*vm.Table)
	return tbl, ok
}
