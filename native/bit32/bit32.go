// Package bit32 implements the Lua 5.2-style bit32 library for 32-bit
// unsigned integer arithmetic. All operands are coerced to uint32 (sign
// bit dropped, high bits masked) before the operation; results are
// returned as int64 in the [0, 2^32) range, matching Lua's bit32
// semantics where every result is a normalized unsigned 32-bit value.
//
// Lua 5.3+ folded these into native integer bitops on int64, but bit32
// remains widely used for fixed-width work (hashing, network protocols,
// embedded targets). The module is preloaded; use `require("bit32")`.
package bit32

import (
	"math/bits"

	"github.com/hilthontt/sakura-lang/vm"
)

// RegisterBit32Preload installs the bit32 module at package.preload.
func RegisterBit32Preload(v *vm.VM) {
	vm.RegisterPreload(v, "bit32", loader)
}

func loader(_ *vm.VM, _ []vm.Value) []vm.Value {
	return []vm.Value{newBit32()}
}

// u32 coerces a Lua value to a uint32 the way Lua 5.2 does: any number
// is first converted to int64, then masked to 32 bits. Float operands
// that aren't integer-valued raise the same error Lua does.
func u32(name string, idx int, args []vm.Value) uint32 {
	n := vm.IntArg(name, idx, args)
	return uint32(n & 0xFFFFFFFF)
}

// foldBinary folds an n-ary identity-bearing op over all args, masked to
// 32 bits. Used by band/bor/bxor.
func foldBinary(name string, args []vm.Value, identity uint32, op func(a, b uint32) uint32) int64 {
	acc := identity
	for i := range args {
		acc = op(acc, u32(name, i+1, args))
	}
	return int64(acc)
}

func newBit32() *vm.Table {
	m := vm.NewTable(0, 1)
	methods := vm.NewTable(0, 14)
	add := func(name string, fn func(*vm.VM, []vm.Value) []vm.Value) {
		methods.Set(name, &vm.GoFunc{Name: "bit32." + name, Fn: fn})
	}

	add("band", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{foldBinary("band", args, 0xFFFFFFFF, func(a, b uint32) uint32 { return a & b })}
	})
	add("bor", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{foldBinary("bor", args, 0, func(a, b uint32) uint32 { return a | b })}
	})
	add("bxor", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{foldBinary("bxor", args, 0, func(a, b uint32) uint32 { return a ^ b })}
	})
	add("bnot", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{int64(^u32("bnot", 1, args))}
	})
	add("btest", func(_ *vm.VM, args []vm.Value) []vm.Value {
		acc := uint32(0xFFFFFFFF)
		for i := range args {
			acc &= u32("btest", i+1, args)
		}
		return []vm.Value{acc != 0}
	})

	// Shifts. Lua semantics: a shift by ≥32 produces 0; negative shifts
	// reverse direction. arshift fills with the sign bit.
	add("lshift", func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := u32("lshift", 1, args)
		n := vm.IntArg("lshift", 2, args)
		return []vm.Value{int64(shiftLeft(x, int(n)))}
	})
	add("rshift", func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := u32("rshift", 1, args)
		n := vm.IntArg("rshift", 2, args)
		return []vm.Value{int64(shiftRight(x, int(n)))}
	})
	add("arshift", func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := u32("arshift", 1, args)
		n := vm.IntArg("arshift", 2, args)
		return []vm.Value{int64(arithShiftRight(x, int(n)))}
	})
	add("lrotate", func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := u32("lrotate", 1, args)
		n := int(vm.IntArg("lrotate", 2, args))
		// math/bits handles negative rotation counts correctly.
		return []vm.Value{int64(bits.RotateLeft32(x, n))}
	})
	add("rrotate", func(_ *vm.VM, args []vm.Value) []vm.Value {
		x := u32("rrotate", 1, args)
		n := int(vm.IntArg("rrotate", 2, args))
		return []vm.Value{int64(bits.RotateLeft32(x, -n))}
	})

	// extract / replace operate on a contiguous bit field.
	add("extract", func(_ *vm.VM, args []vm.Value) []vm.Value {
		n := u32("extract", 1, args)
		field := int(vm.IntArg("extract", 2, args))
		width := 1
		if len(args) >= 3 {
			width = int(vm.IntArg("extract", 3, args))
		}
		checkField(field, width)
		mask := uint32((1<<width)-1) << field
		return []vm.Value{int64((n & mask) >> field)}
	})
	add("replace", func(_ *vm.VM, args []vm.Value) []vm.Value {
		n := u32("replace", 1, args)
		v := u32("replace", 2, args)
		field := int(vm.IntArg("replace", 3, args))
		width := 1
		if len(args) >= 4 {
			width = int(vm.IntArg("replace", 4, args))
		}
		checkField(field, width)
		mask := uint32((1<<width)-1) << field
		return []vm.Value{int64((n &^ mask) | ((v << field) & mask))}
	})

	// bswap reverses byte order — not in Lua's bit32 but trivially useful
	// for the same audience.
	add("bswap", func(_ *vm.VM, args []vm.Value) []vm.Value {
		return []vm.Value{int64(bits.ReverseBytes32(u32("bswap", 1, args)))}
	})

	mt := vm.NewTable(0, 1)
	mt.Set("__index", methods)
	m.SetMetatable(mt)
	return m
}

func shiftLeft(x uint32, n int) uint32 {
	if n < 0 {
		return shiftRight(x, -n)
	}
	if n >= 32 {
		return 0
	}
	return x << uint(n)
}

func shiftRight(x uint32, n int) uint32 {
	if n < 0 {
		return shiftLeft(x, -n)
	}
	if n >= 32 {
		return 0
	}
	return x >> uint(n)
}

func arithShiftRight(x uint32, n int) uint32 {
	if n < 0 {
		return shiftLeft(x, -n)
	}
	if n >= 32 {
		// Sign-extend a 32-bit value: all ones if sign bit set, else 0.
		if x&0x80000000 != 0 {
			return 0xFFFFFFFF
		}
		return 0
	}
	return uint32(int32(x) >> uint(n))
}

func checkField(field, width int) {
	if field < 0 || width < 1 || field+width > 32 {
		panic(vm.Errorf("bit32: field out of range (field=%d, width=%d, must satisfy 0 <= field, 1 <= width, field+width <= 32)", field, width))
	}
}
