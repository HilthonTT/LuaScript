package bytecode

// localTable tracks Lua locals for a single function. Lua locals are
// function-scoped slots with block-scoped lifetime: opening a block records
// the next-free-slot, closing it releases all slots above that mark for
// reuse. Names within a single scope shadow names above.
//
// The `parent` link points at the enclosing *function's* localTable (not the
// enclosing block) so identifiers can walk up to find captured upvalues.
type localTable struct {
	parent   *localTable
	scopes   []scopeFrame
	nextSlot int
	maxSlot  int // high-water mark; stored on the function proto
}

type scopeFrame struct {
	baseSlot int            // value of nextSlot when this scope opened
	bindings []localBinding // locals declared inside this scope
}

type localBinding struct {
	Name string
	Slot int
}

func newLocalTable(parent *localTable) *localTable {
	lt := &localTable{parent: parent}
	lt.openScope() // every function has at least one root scope
	return lt
}

// openScope is called when entering a Lua block (`do ... end`, function body,
// loop body, if branch, etc.).
func (lt *localTable) openScope() {
	lt.scopes = append(lt.scopes, scopeFrame{baseSlot: lt.nextSlot})
}

// closeScope releases every local declared in the innermost scope.
func (lt *localTable) closeScope() {
	if len(lt.scopes) == 0 {
		return
	}
	top := lt.scopes[len(lt.scopes)-1]
	lt.nextSlot = top.baseSlot
	lt.scopes = lt.scopes[:len(lt.scopes)-1]
}

// define adds a new local in the innermost scope and returns its slot.
// Re-declaration in the same scope is allowed in Lua (the new binding
// shadows the old one for the rest of the scope) but reuses a fresh slot.
func (lt *localTable) define(name string) int {
	slot := lt.nextSlot
	lt.nextSlot++
	if lt.nextSlot > lt.maxSlot {
		lt.maxSlot = lt.nextSlot
	}
	idx := len(lt.scopes) - 1
	lt.scopes[idx].bindings = append(lt.scopes[idx].bindings, localBinding{Name: name, Slot: slot})
	return slot
}

// lookupLocal searches innermost-to-outermost for a binding visible in this
// function. It does NOT walk into the parent function.
func (lt *localTable) lookupLocal(name string) (slot int, ok bool) {
	for s := len(lt.scopes) - 1; s >= 0; s-- {
		bs := lt.scopes[s].bindings
		for b := len(bs) - 1; b >= 0; b-- {
			if bs[b].Name == name {
				return bs[b].Slot, true
			}
		}
	}
	return 0, false
}
