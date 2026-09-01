package classification

import "github.com/hilthontt/luascript/internal/vm"

func RegisterClassificationPreload(v *vm.VM) {
	vm.RegisterPreload(v, "classification", classificationLoader)
}

func classificationLoader(_ *vm.VM, _ []vm.Value) []vm.Value {
	mod := vm.NewTable(0, 6)
	mod.Set("VERSION", "0.1.0")
	mod.Set("naivebayes", &vm.GoFunc{Name: "classification.naivebayes", Fn: newNaiveBayes})
	mod.Set("knn", &vm.GoFunc{Name: "classification.knn", Fn: newKNNObject})
	mod.Set("perceptron", &vm.GoFunc{Name: "classification.perceptron", Fn: newPerceptronObject})
	mod.Set("logistic", &vm.GoFunc{Name: "classification.logistic", Fn: newLogisticObject})
	mod.Set("svm", &vm.GoFunc{Name: "classification.svm", Fn: newSVMObject})
	return []vm.Value{mod}
}

func newNaiveBayes(_ *vm.VM, args []vm.Value) []vm.Value {
	tfidf := false
	end := len(args)
	if end > 0 {
		if t, ok := args[end-1].(*vm.Table); ok {
			if b, ok := t.Get("tfidf").(bool); ok {
				tfidf = b
			}
			end--
		}
	}

	classes := make([]Class, 0, end)
	for i := 0; i < end; i++ {
		classes = append(classes, Class(vm.StringArg("classification.naivebayes", i+1, args)))
	}
	if len(classes) < 2 {
		panic(vm.Errorf("classification.naivebayes: provide at least two class names"))
	}

	var clf *Classifier
	if tfidf {
		clf = NewClassifierTfIdf(classes...)
	} else {
		clf = NewClassifier(classes...)
	}
	return []vm.Value{newBayesObject(clf)}
}

func newBayesObject(clf *Classifier) *vm.Table {
	methods := vm.NewTable(0, 6)

	methods.Set("learn", &vm.GoFunc{Name: "naivebayes:learn", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		doc := stringList("naivebayes:learn", 2, a)
		class := Class(vm.StringArg("naivebayes:learn", 3, a))
		clf.Learn(doc, class)
		return nil
	}})

	methods.Set("convert", &vm.GoFunc{Name: "naivebayes:convert", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		clf.ConvertTermsFreqToTfIdf()
		return nil
	}})

	methods.Set("classify", &vm.GoFunc{Name: "naivebayes:classify", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		doc := stringList("naivebayes:classify", 2, a)
		cls, scores, strict := clf.Classify(doc)
		return []vm.Value{string(cls), floatsToTable(scores), strict}
	}})

	methods.Set("classifyProb", &vm.GoFunc{Name: "naivebayes:classifyProb", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		doc := stringList("naivebayes:classifyProb", 2, a)
		cls, probs, strict := clf.ClassifyProb(doc)
		return []vm.Value{string(cls), floatsToTable(probs), strict}
	}})

	methods.Set("classes", &vm.GoFunc{Name: "naivebayes:classes", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		out := vm.NewTable(len(clf.Classes), 0)
		for i, c := range clf.Classes {
			out.Set(int64(i+1), string(c))
		}
		return []vm.Value{out}
	}})

	methods.Set("learned", &vm.GoFunc{Name: "naivebayes:learned", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(clf.Learned())}
	}})

	return withMethods(methods)
}

func newKNNObject(_ *vm.VM, args []vm.Value) []vm.Value {
	k := int(vm.OptInt("classification.knn", 1, args, 3))
	model := NewKNN(k)
	return []vm.Value{newNumericObject("knn", model.Fit, model.Predict, nil)}
}

func newPerceptronObject(_ *vm.VM, args []vm.Value) []vm.Value {
	opts := optTable(args, 1)
	lr := optFloat(opts, "lr", 0.1)
	epochs := int(optInt(opts, "epochs", 50))
	model := NewPerceptron(lr, epochs)
	return []vm.Value{newNumericObject("perceptron", model.Fit, model.Predict, nil)}
}

func newLogisticObject(_ *vm.VM, args []vm.Value) []vm.Value {
	opts := optTable(args, 1)
	lr := optFloat(opts, "lr", 0.1)
	epochs := int(optInt(opts, "epochs", 200))
	model := NewLogisticRegression(lr, epochs)
	return []vm.Value{newNumericObject("logistic", model.Fit, model.Predict, model.PredictProba)}
}

func newSVMObject(_ *vm.VM, args []vm.Value) []vm.Value {
	opts := optTable(args, 1)
	model := NewSVM(SVMConfig{
		Kernel:  ParseKernel(optString(opts, "kernel", "rbf")),
		C:       optFloat(opts, "C", 1.0),
		Gamma:   optFloat(opts, "gamma", 0.5),
		Coef0:   optFloat(opts, "coef0", 0),
		Degree:  int(optInt(opts, "degree", 3)),
		Tol:     optFloat(opts, "tol", 1e-3),
		MaxIter: int(optInt(opts, "maxIter", 100)),
		Seed:    optInt(opts, "seed", 1),
	})

	methods := vm.NewTable(0, 4)

	width := -1

	checkQuery := func(fn string, x []float64) {
		if width < 0 {
			panic(vm.Errorf("%s: model is not fitted yet", fn))
		}
		if len(x) != width {
			panic(vm.Errorf("%s: expected %d features, got %d", fn, width, len(x)))
		}
	}

	methods.Set("fit", &vm.GoFunc{Name: "svm:fit", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		features := tableToMatrix("svm:fit", vm.TableArg("svm:fit", 2, a))
		labels := stringList("svm:fit", 3, a)
		if len(features) != len(labels) {
			panic(vm.Errorf("svm:fit: #features (%d) must equal #labels (%d)", len(features), len(labels)))
		}
		if len(features) < 2 {
			panic(vm.Errorf("svm:fit: training set needs at least 2 samples, got %d", len(features)))
		}
		model.Fit(features, labels)
		width = len(features[0])
		return nil
	}})

	methods.Set("predict", &vm.GoFunc{Name: "svm:predict", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		x := floatList("svm:predict", 2, a)
		checkQuery("svm:predict", x)
		return []vm.Value{model.Predict(x)}
	}})

	methods.Set("decision_function", &vm.GoFunc{Name: "svm:decision_function", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		x := floatList("svm:decision_function", 2, a)
		checkQuery("svm:decision_function", x)
		return []vm.Value{model.DecisionFunction(x)}
	}})

	methods.Set("support_vectors", &vm.GoFunc{Name: "svm:support_vectors", Fn: func(_ *vm.VM, _ []vm.Value) []vm.Value {
		return []vm.Value{int64(model.SupportVectorCount())}
	}})

	return []vm.Value{withMethods(methods)}
}

func newNumericObject(
	name string,
	fit func([][]float64, []string),
	predict func([]float64) string,
	predictProba func([]float64) float64,
) *vm.Table {
	methods := vm.NewTable(0, 3)

	width := -1

	checkQuery := func(fn string, x []float64) {
		if width < 0 {
			panic(vm.Errorf("%s: model is not fitted yet", fn))
		}
		if len(x) != width {
			panic(vm.Errorf("%s: expected %d features, got %d", fn, width, len(x)))
		}
	}

	methods.Set("fit", &vm.GoFunc{Name: name + ":fit", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		features := tableToMatrix(name+":fit", vm.TableArg(name+":fit", 2, a))
		labels := stringList(name+":fit", 3, a)
		if len(features) != len(labels) {
			panic(vm.Errorf("%s:fit: #features (%d) must equal #labels (%d)", name, len(features), len(labels)))
		}
		if len(features) == 0 {
			panic(vm.Errorf("%s:fit: training set is empty", name))
		}
		fit(features, labels)
		width = len(features[0])
		return nil
	}})

	methods.Set("predict", &vm.GoFunc{Name: name + ":predict", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
		x := floatList(name+":predict", 2, a)
		checkQuery(name+":predict", x)
		return []vm.Value{predict(x)}
	}})

	if predictProba != nil {
		methods.Set("predict_proba", &vm.GoFunc{Name: name + ":predict_proba", Fn: func(_ *vm.VM, a []vm.Value) []vm.Value {
			x := floatList(name+":predict_proba", 2, a)
			checkQuery(name+":predict_proba", x)
			return []vm.Value{predictProba(x)}
		}})
	}

	return withMethods(methods)
}

func withMethods(methods *vm.Table) *vm.Table {
	obj := vm.NewTable(0, 1)
	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	obj.SetMetatable(mt)
	return obj
}

func stringList(name string, n int, args []vm.Value) []string {
	t := vm.TableArg(name, n, args)
	count := int(t.Len())
	out := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		s, ok := t.Get(int64(i)).(string)
		if !ok {
			panic(vm.Errorf("%s: element #%d must be a string", name, i))
		}
		out = append(out, s)
	}
	return out
}

func floatList(name string, n int, args []vm.Value) []float64 {
	t := vm.TableArg(name, n, args)
	count := int(t.Len())
	out := make([]float64, 0, count)
	for i := 1; i <= count; i++ {
		f, ok := vm.ToFloat(t.Get(int64(i)))
		if !ok {
			panic(vm.Errorf("%s: element #%d must be a number", name, i))
		}
		out = append(out, f)
	}
	return out
}

func tableToMatrix(name string, t *vm.Table) [][]float64 {
	rows := int(t.Len())
	out := make([][]float64, 0, rows)
	width := -1
	for i := 1; i <= rows; i++ {
		row, ok := t.Get(int64(i)).(*vm.Table)
		if !ok {
			panic(vm.Errorf("%s: row #%d must be an array of numbers", name, i))
		}
		m := int(row.Len())
		if width == -1 {
			width = m
		} else if m != width {
			panic(vm.Errorf("%s: every row must have the same width (row 1 has %d, row %d has %d)", name, width, i, m))
		}
		vals := make([]float64, m)
		for j := 1; j <= m; j++ {
			f, ok := vm.ToFloat(row.Get(int64(j)))
			if !ok {
				panic(vm.Errorf("%s: element [%d][%d] must be a number", name, i, j))
			}
			vals[j-1] = f
		}
		out = append(out, vals)
	}
	return out
}

func floatsToTable(xs []float64) *vm.Table {
	out := vm.NewTable(len(xs), 0)
	for i, x := range xs {
		out.Set(int64(i+1), x)
	}
	return out
}

func optTable(args []vm.Value, n int) *vm.Table {
	if n < 1 || n > len(args) || args[n-1] == nil {
		return nil
	}
	if t, ok := args[n-1].(*vm.Table); ok {
		return t
	}
	return nil
}

func optFloat(opts *vm.Table, key string, dflt float64) float64 {
	if opts == nil {
		return dflt
	}
	if f, ok := vm.ToFloat(opts.Get(key)); ok {
		return f
	}
	return dflt
}

func optInt(opts *vm.Table, key string, dflt int64) int64 {
	if opts == nil {
		return dflt
	}
	if i, ok := vm.ToInteger(opts.Get(key)); ok {
		return i
	}
	return dflt
}

func optString(opts *vm.Table, key, dflt string) string {
	if opts == nil {
		return dflt
	}
	if s, ok := opts.Get(key).(string); ok {
		return s
	}
	return dflt
}
