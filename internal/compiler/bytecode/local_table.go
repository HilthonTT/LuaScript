package bytecode

type localTable struct {
	parent   *localTable
	scopes   []scopeFrame
	nextSlot int
	maxSlot  int
}

type scopeFrame struct {
	baseSlot int
	bindings []localBinding
}

type localBinding struct {
	Name string
	Slot int
}

func newLocalTable(parent *localTable) *localTable {
	lt := &localTable{parent: parent}
	lt.openScope()
	return lt
}

func (lt *localTable) openScope() {
	lt.scopes = append(lt.scopes, scopeFrame{baseSlot: lt.nextSlot})
}

func (lt *localTable) closeScope() {
	if len(lt.scopes) == 0 {
		return
	}
	top := lt.scopes[len(lt.scopes)-1]
	lt.nextSlot = top.baseSlot
	lt.scopes = lt.scopes[:len(lt.scopes)-1]
}

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
