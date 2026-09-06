package vm

import (
	"fmt"
	"strings"

	"github.com/hilthontt/luascript/internal/compiler/bytecode"
)

func (v *VM) exec(entryDepth int) {
	if v.frames[entryDepth-1].Closure.Proto.HasTry() {
		for v.execCatching(entryDepth) {
		}
		return
	}
	for len(v.frames) >= entryDepth {
		f := v.frames[len(v.frames)-1]
		if f.IP >= len(f.Closure.Proto.Instructions) {
			v.unwindFrame(f, nil)
			continue
		}
		ins := f.Closure.Proto.Instructions[f.IP]
		f.IP++
		v.dispatch(f, ins)
	}
}

func (v *VM) execCatching(entryDepth int) (resume bool) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if isCloseSignal(r) {
			panic(r)
		}
		if !v.dispatchToHandler(entryDepth, r) {
			panic(r)
		}
		resume = true
	}()
	v.execLoop(entryDepth)
	return false
}

func (v *VM) dispatchToHandler(entryDepth int, r any) bool {
	if len(v.frames) < entryDepth {
		return false
	}
	f := v.frames[entryDepth-1]
	if len(f.handlers) == 0 {
		return false
	}
	h := f.handlers[len(f.handlers)-1]
	f.handlers = f.handlers[:len(f.handlers)-1]

	caught := v.errorValue(r)

	for i := len(v.frames) - 1; i >= entryDepth; i-- {
		if len(v.frames[i].tbc) > 0 {
			v.closeAllTBCSafely(v.frames[i], caught)
		}
		if len(v.frames[i].Deferred) > 0 {
			v.runDeferredSafely(v.frames[i])
		}
	}
	if len(f.tbc) > h.tbcTop {
		v.closeTBCSafely(f, len(f.tbc)-h.tbcTop, caught)
	}
	v.closeUpvaluesAbove(h.stackTop)
	v.frames = v.frames[:entryDepth]
	v.Stack = v.Stack[:h.stackTop]
	v.callMarks = v.callMarks[:h.markDepth]

	f.IP = h.catchIP
	v.push(caught)
	return true
}

func (v *VM) execLoop(entryDepth int) {
	for len(v.frames) >= entryDepth {
		f := v.frames[len(v.frames)-1]
		if f.IP >= len(f.Closure.Proto.Instructions) {
			v.unwindFrame(f, nil)
			continue
		}
		ins := f.Closure.Proto.Instructions[f.IP]
		f.IP++
		v.dispatch(f, ins)
	}
}

func (v *VM) dispatch(f *CallFrame, ins *bytecode.Instruction) {
	switch ins.Opcode {

	case bytecode.LoadNil:
		v.pushNils(int(ins.A))
	case bytecode.LoadTrue:
		v.push(true)
	case bytecode.LoadFalse:
		v.push(false)
	case bytecode.LoadInt:
		v.push(ins.BoxedAny)
	case bytecode.LoadFloat:
		v.push(ins.BoxedAny)
	case bytecode.LoadString:
		v.push(ins.BoxedAny)
	case bytecode.LoadVararg:
		count := int(ins.A)
		va := f.Varargs
		switch {
		case count < 0:
			v.Stack = append(v.Stack, va...)
		case count <= len(va):
			v.Stack = append(v.Stack, va[:count]...)
		default:
			v.Stack = append(v.Stack, va...)
			v.pushNils(count - len(va))
		}
	case bytecode.Closure:
		proto := f.Closure.Proto.Protos[ins.A]
		v.push(v.makeClosure(f, proto))

	case bytecode.GetLocal:
		v.push(*v.localAt(f, int(ins.A)))
	case bytecode.SetLocal:
		*v.localAt(f, int(ins.A)) = v.pop()
	case bytecode.GetUpvalue:
		v.push(f.Closure.Upvalues[ins.A].Get())
	case bytecode.SetUpvalue:
		f.Closure.Upvalues[ins.A].Set(v.pop())
	case bytecode.GetGlobal:
		if ins.CacheGen() == v.Globals.gen {
			v.push(ins.CacheVal())
		} else {
			val := v.Globals.Get(ins.StrA)
			ins.SetCache(v.Globals.gen, val)
			v.push(val)
		}
	case bytecode.SetGlobal:
		v.Globals.Set(ins.StrA, v.pop())

	case bytecode.NewTable:
		v.push(NewTable(int(ins.A), int(ins.B)))
	case bytecode.GetTable:
		key := v.pop()
		obj := v.pop()
		v.push(v.indexMM(obj, key))
	case bytecode.SetTable:
		val := v.pop()
		key := v.pop()
		obj := v.pop()
		v.newIndexMM(obj, key, val)
	case bytecode.GetField:
		obj := v.pop()
		v.push(v.indexMM(obj, ins.StrA))
	case bytecode.SetField:
		val := v.pop()
		obj := v.pop()
		v.newIndexMM(obj, ins.StrA, val)
	case bytecode.Self:
		obj := v.Stack[len(v.Stack)-1]
		v.Stack[len(v.Stack)-1] = v.indexMM(obj, ins.StrA)
		v.push(obj)
	case bytecode.SetList:
		count := int(ins.A)
		offset := int(ins.B)
		var valuesStart int
		if count < 0 {
			if len(v.callMarks) == 0 {
				panic("vm: SetList with count=-1 but no MarkArgs mark on stack")
			}
			valuesStart = v.callMarks[len(v.callMarks)-1]
			v.callMarks = v.callMarks[:len(v.callMarks)-1]
			count = len(v.Stack) - valuesStart
		} else {
			valuesStart = len(v.Stack) - count
		}
		t := v.Stack[valuesStart-1].(*Table)
		for i := 0; i < count; i++ {
			t.Set(int64(offset+i+1), v.Stack[valuesStart+i])
		}
		v.popN(count)

	case bytecode.Add:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(internInt(ai + bi))
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af + bf)
				return
			}
		}
		v.setTop2(v.arithMM(a, b, "+", metaAdd))
	case bytecode.Sub:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(internInt(ai - bi))
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af - bf)
				return
			}
		}
		v.setTop2(v.arithMM(a, b, "-", metaSub))
	case bytecode.Mul:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(internInt(ai * bi))
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af * bf)
				return
			}
		}
		v.setTop2(v.arithMM(a, b, "*", metaMul))
	case bytecode.Div:
		a, b := v.peek2()
		v.setTop2(v.arithDivMM(a, b))
	case bytecode.FloorDiv:
		a, b := v.peek2()
		v.setTop2(v.arithFloorDivMM(a, b))
	case bytecode.Mod:
		a, b := v.peek2()
		v.setTop2(v.arithMM(a, b, "%", metaMod))
	case bytecode.Pow:
		a, b := v.peek2()
		v.setTop2(v.arithPowMM(a, b))
	case bytecode.Neg:
		v.push(v.arithNegMM(v.pop()))

	case bytecode.BitAnd:
		a, b := v.peek2()
		v.setTop2(v.bitwiseMM(a, b, bitAnd, metaBAnd))
	case bytecode.BitOr:
		a, b := v.peek2()
		v.setTop2(v.bitwiseMM(a, b, bitOr, metaBOr))
	case bytecode.BitXor:
		a, b := v.peek2()
		v.setTop2(v.bitwiseMM(a, b, bitXor, metaBXor))
	case bytecode.Shl:
		a, b := v.peek2()
		v.setTop2(v.bitwiseMM(a, b, shl, metaShl))
	case bytecode.Shr:
		a, b := v.peek2()
		v.setTop2(v.bitwiseMM(a, b, shr, metaShr))
	case bytecode.BitNot:
		v.push(v.bitNotMM(v.pop()))

	case bytecode.Concat:
		count := int(ins.A)
		start := len(v.Stack) - count
		allPlain := true
		size := 0
		for i := start; i < len(v.Stack); i++ {
			if s, ok := v.Stack[i].(string); ok {
				size += len(s)
				continue
			}
			if !isStringOrNumber(v.Stack[i]) {
				allPlain = false
				break
			}
			size += 16
		}
		if allPlain {
			var b strings.Builder
			b.Grow(size)
			for i := start; i < len(v.Stack); i++ {
				b.WriteString(concatOne(v.Stack[i]))
			}
			v.popN(count)
			v.push(b.String())
			return
		}
		acc := v.Stack[len(v.Stack)-1]
		for i := count - 2; i >= 0; i-- {
			acc = v.concatMM(v.Stack[start+i], acc)
		}
		v.popN(count)
		v.push(acc)
	case bytecode.Len:
		v.push(v.lenMM(v.pop()))

	case bytecode.Eq:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(ai == bi)
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af == bf)
				return
			}
		}
		if as, ok := a.(string); ok {
			if bs, ok := b.(string); ok {
				v.setTop2(as == bs)
				return
			}
		}
		v.setTop2(v.equalMM(a, b))
	case bytecode.NotEq:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(ai != bi)
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af != bf)
				return
			}
		}
		if as, ok := a.(string); ok {
			if bs, ok := b.(string); ok {
				v.setTop2(as != bs)
				return
			}
		}
		v.setTop2(!v.equalMM(a, b))
	case bytecode.Lt:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(ai < bi)
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af < bf)
				return
			}
		}
		v.setTop2(v.lessMM(a, b))
	case bytecode.Le:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(ai <= bi)
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af <= bf)
				return
			}
		}
		v.setTop2(v.lessOrEqualMM(a, b))
	case bytecode.Gt:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(ai > bi)
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af > bf)
				return
			}
		}
		v.setTop2(v.lessMM(b, a))
	case bytecode.Ge:
		a, b := v.peek2()
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				v.setTop2(ai >= bi)
				return
			}
		}
		if af, ok := a.(float64); ok {
			if bf, ok := b.(float64); ok {
				v.setTop2(af >= bf)
				return
			}
		}
		v.setTop2(v.lessOrEqualMM(b, a))

	case bytecode.Not:
		v.push(!IsTruthy(v.pop()))

	case bytecode.Jump:
		f.IP = int(ins.A)
	case bytecode.JumpIfFalse:
		x := v.pop()
		if !IsTruthy(x) {
			f.IP = int(ins.A)
		}
	case bytecode.JumpIfTrue:
		x := v.pop()
		if IsTruthy(x) {
			f.IP = int(ins.A)
		}
	case bytecode.JumpIfFalseKeep:
		x := v.Stack[len(v.Stack)-1]
		if !IsTruthy(x) {
			f.IP = int(ins.A)
		} else {
			v.pop()
		}
	case bytecode.JumpIfTrueKeep:
		x := v.Stack[len(v.Stack)-1]
		if IsTruthy(x) {
			f.IP = int(ins.A)
		} else {
			v.pop()
		}

	case bytecode.MarkArgs:
		v.callMarks = append(v.callMarks, len(v.Stack))
	case bytecode.CloseUpvalues:
		v.closeUpvaluesAbove(f.Base + int(ins.A))
	case bytecode.Call:
		v.doCall(int(ins.A), int(ins.B))
	case bytecode.Return:
		v.doReturn(f, int(ins.A))

	case bytecode.ForPrep:
		v.forPrep(f, int(ins.A), int(ins.B))
	case bytecode.ForLoop:
		v.forLoop(f, int(ins.A), int(ins.B))

	case bytecode.TForCall:
		v.tForCall(f, int(ins.A), int(ins.B))
	case bytecode.TForLoop:
		v.tForLoop(f, int(ins.A), int(ins.B))

	case bytecode.Pop:
		v.popN(int(ins.A))
	case bytecode.Dup:
		v.push(v.Stack[len(v.Stack)-1])

	case bytecode.Leave:
		v.doReturn(f, 0)
	case bytecode.Defer:
		d := v.pop()
		cl, ok := d.(*Closure)
		if !ok {
			panic(Errorf("defer: expected a function, got %s", TypeName(d)))
		}
		f.Deferred = append(f.Deferred, cl)

	case bytecode.MarkTBC:
		v.markTBC(f, *v.localAt(f, int(ins.A)), ins.StrA)
	case bytecode.CloseTBC:
		v.closeTBC(f, int(ins.A), nil)

	case bytecode.Try:
		f.handlers = append(f.handlers, tryHandler{
			catchIP:   int(ins.A),
			stackTop:  len(v.Stack),
			markDepth: len(v.callMarks),
			tbcTop:    len(f.tbc),
		})
	case bytecode.EndTry:
		f.handlers = f.handlers[:len(f.handlers)-int(ins.A)]
	case bytecode.Throw:
		panic(luaError{value: v.pop()})

	default:
		panic(fmt.Sprintf("vm: unknown opcode %d (%s)", ins.Opcode, bytecode.InstructionNameTable[ins.Opcode]))
	}
}
