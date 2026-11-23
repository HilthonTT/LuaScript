package classification

import (
	"strings"
	"testing"

	"github.com/hilthontt/luascript/v3/internal/vm"
)

// Learning into a class the classifier was not built with dereferenced a nil
// map entry.
func TestNaiveBayesLearnUnknownClass(t *testing.T) {
	obj := newNaiveBayes(nil, []vm.Value{"spam", "ham"})[0].(*vm.Table)
	learn := obj.Metatable().Get("__index").(*vm.Table).Get("learn").(*vm.GoFunc)
	doc := vm.NewTable(1, 0)
	doc.Append("hello")
	defer func() {
		r := recover()
		e, ok := r.(vm.LuaError)
		if !ok || !strings.Contains(string(e), "unknown class") {
			t.Fatalf("learn into unknown class panicked with %v (%T), want an unknown-class error", r, r)
		}
	}()
	learn.Fn(nil, []vm.Value{obj, doc, "eggs"})
}
