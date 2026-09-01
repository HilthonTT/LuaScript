package vm

type Upvalue struct {
	Stack  *[]Value
	Index  int
	Closed Value
}

func (u *Upvalue) Get() Value {
	if u.Stack != nil {
		return (*u.Stack)[u.Index]
	}
	return u.Closed
}

func (u *Upvalue) Set(v Value) {
	if u.Stack != nil {
		(*u.Stack)[u.Index] = v
		return
	}
	u.Closed = v
}
