package vm

// To-be-closed variables (`local x <close> = v`, Lua 5.4 §3.3.8).
//
// A `<close>` declaration registers its value with the current call frame; the
// value's __close metamethod runs when the declaring block is left, whether
// that happens by falling off the end, by break/continue, by return, or by an
// error unwinding through the frame. __close receives the value itself plus the
// error that caused the exit (nil on a normal exit).

// markTBC registers val as a to-be-closed value of f. false and nil are ignored,
// as in Lua; anything else must have a __close metamethod.
func (v *VM) markTBC(f *CallFrame, val Value, name string) {
	if val == nil || val == false {
		return
	}
	if v.getMetamethod(val, "__close") == nil {
		panic(Errorf("variable '%s' got a non-closable value (%s)", name, TypeName(val)))
	}
	f.tbc = append(f.tbc, val)
}

// closeTBC closes the innermost n to-be-closed values of f, most recent first.
func (v *VM) closeTBC(f *CallFrame, n int, errVal Value) {
	if n > len(f.tbc) {
		n = len(f.tbc)
	}
	for ; n > 0; n-- {
		last := len(f.tbc) - 1
		val := f.tbc[last]
		f.tbc = f.tbc[:last]
		if mm := v.getMetamethod(val, "__close"); mm != nil {
			v.CallValue(mm, []Value{val, errVal}, 0)
		}
	}
}

// closeAllTBCSafely closes every remaining to-be-closed value of f while an
// error is already unwinding, so a failing __close cannot derail the unwind.
func (v *VM) closeAllTBCSafely(f *CallFrame, errVal Value) {
	v.closeTBCSafely(f, len(f.tbc), errVal)
}

// closeTBCSafely closes the innermost n to-be-closed values of f during an
// unwind, swallowing errors raised by __close itself.
func (v *VM) closeTBCSafely(f *CallFrame, n int, errVal Value) {
	if n > len(f.tbc) {
		n = len(f.tbc)
	}
	for ; n > 0; n-- {
		last := len(f.tbc) - 1
		val := f.tbc[last]
		f.tbc = f.tbc[:last]
		mm := v.getMetamethod(val, "__close")
		if mm == nil {
			continue
		}
		fd, st := len(v.frames), len(v.Stack)
		func() {
			defer func() {
				if r := recover(); r != nil {
					if isCloseSignal(r) {
						panic(r)
					}
					v.closeUpvaluesAbove(st)
					v.frames = v.frames[:fd]
					v.Stack = v.Stack[:st]
				}
			}()
			v.CallValue(mm, []Value{val, errVal}, 0)
		}()
	}
}

// closePendingTBC runs the __close metamethods still owed by every live frame,
// used when an error escapes the VM entirely and no handler will unwind them.
func (v *VM) closePendingTBC(errVal Value) {
	for i := len(v.frames) - 1; i >= 0; i-- {
		if len(v.frames[i].tbc) > 0 {
			v.closeAllTBCSafely(v.frames[i], errVal)
		}
	}
}
