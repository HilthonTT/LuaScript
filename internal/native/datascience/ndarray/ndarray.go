package ndarray

import (
	"github.com/hilthontt/luascript/internal/vm"
)

func RegisterNDArrayPreload(v *vm.VM) {
	vm.RegisterPreload(v, "ndarray", ndLoader)
}

type ndarray struct {
	data  []float64
	shape []int
}

const maxNDElems = 1 << 26

func newND(shape []int) *ndarray {
	n := 1
	for _, d := range shape {
		if d < 0 {
			panic(vm.Errorf("ndarray: negative dimension %d", d))
		}
		if d > 0 && n > maxNDElems/d {
			panic(vm.Errorf("ndarray: shape exceeds %d elements", maxNDElems))
		}
		n *= d
	}
	return &ndarray{data: make([]float64, n), shape: append([]int(nil), shape...)}
}

func scalarND(x float64) *ndarray { return &ndarray{data: []float64{x}, shape: []int{}} }

func (a *ndarray) ndim() int { return len(a.shape) }
func (a *ndarray) size() int { return len(a.data) }
