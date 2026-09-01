package std

type Node struct {
	children map[rune]*Node
	isLeaf   bool
}

func NewNode() *Node {
	n := &Node{}
	n.children = make(map[rune]*Node)
	n.isLeaf = false
	return n
}

func (n *Node) insert(s string) {
	curr := n
	for _, c := range s {
		next, ok := curr.children[c]
		if !ok {
			next = NewNode()
			curr.children[c] = next
		}
		curr = next
	}
	curr.isLeaf = true
}

func (n *Node) Insert(s ...string) {
	for _, ss := range s {
		n.insert(ss)
	}
}

func (n *Node) Find(s string) bool {
	next, ok := n, false
	for _, c := range s {
		next, ok = next.children[c]
		if !ok {
			return false
		}
	}
	return next.isLeaf
}

func (n *Node) Capacity() int {
	r := 0
	for _, c := range n.children {
		r += c.Capacity()
	}
	return 1 + r
}

func (n *Node) Size() int {
	r := 0
	for _, c := range n.children {
		r += c.Size()
	}
	if n.isLeaf {
		r++
	}
	return r
}

func (n *Node) remove(s string) {
	if len(s) == 0 {
		return
	}

	next, ok := n, false
	for _, c := range s {
		next, ok = next.children[c]
		if !ok {
			return
		}
	}
	next.isLeaf = false
}

func (n *Node) Remove(s ...string) {
	for _, ss := range s {
		n.remove(ss)
	}
}

func (n *Node) Compact() (remove bool) {
	for r, c := range n.children {
		if c.Compact() {
			delete(n.children, r)
		}
	}
	return !n.isLeaf && len(n.children) == 0
}
