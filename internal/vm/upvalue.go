package vm

// Upvalue is a captured local. While the enclosing function is still on the
// stack the upvalue is "open" and reads/writes the live stack slot through
// `Stack[Index]`. When the enclosing function returns, the VM closes the
// upvalue: it copies the value into Closed and clears Stack. Subsequent
// accesses then go through Closed.
//
// Closing happens inside VM.closeUpvaluesAbove, called on every Return whose
// frame contains open upvalues.
type Upvalue struct {
	Stack  *[]Value // non-nil while open
	Index  int      // stack slot while open
	Closed Value    // value while closed (Stack==nil)
}

// Get reads the upvalue's current value.
func (u *Upvalue) Get() Value {
	if u.Stack != nil {
		return (*u.Stack)[u.Index]
	}
	return u.Closed
}

// Set stores v into the upvalue.
func (u *Upvalue) Set(v Value) {
	if u.Stack != nil {
		(*u.Stack)[u.Index] = v
		return
	}
	u.Closed = v
}
