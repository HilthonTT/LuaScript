package std

import "github.com/hilthontt/luascript/internal/native/constraints"

type BTreeNode[T constraints.Ordered] struct {
	keys     []T
	children []*BTreeNode[T]
	numKeys  int
	isLeaf   bool
}

type BTree[T constraints.Ordered] struct {
	root    *BTreeNode[T]
	maxKeys int
}

func NewBTreeNode[T constraints.Ordered](maxKeys int, isLeaf bool) *BTreeNode[T] {
	if maxKeys <= 0 {
		panic("BTree maxKeys cannot be zero")
	}
	return &BTreeNode[T]{
		keys:     make([]T, maxKeys),
		children: make([]*BTreeNode[T], maxKeys+1),
		isLeaf:   isLeaf,
	}
}

func (node *BTreeNode[T]) Verify(tree *BTree[T]) {
	minKeys := minKeys(tree.maxKeys)
	if node != tree.root && node.numKeys < minKeys {
		panic("node has too few keys")
	} else if node.numKeys > tree.maxKeys {
		panic("node has too many keys")
	}
}

func (node *BTreeNode[T]) IsFull(maxKeys int) bool {
	return node.numKeys == maxKeys
}

func (node *BTreeNode[T]) Search(key T) bool {
	i := 0
	for ; i < node.numKeys; i++ {
		if key == node.keys[i] {
			return true
		}
		if key < node.keys[i] {
			break
		}
	}
	if node.isLeaf {
		return false
	}
	return node.children[i].Search(key)
}

func (tree *BTree[T]) Search(key T) bool {
	if tree.root == nil {
		return false
	}
	return tree.root.Search(key)
}

func (node *BTreeNode[T]) InsertKeyChild(key T, child *BTreeNode[T]) {
	i := node.numKeys
	node.children[i+1] = node.children[i]
	for ; i > 0; i-- {
		if key > node.keys[i-1] {
			node.keys[i] = key
			node.children[i] = child
			break
		}
		node.keys[i] = node.keys[i-1]
		node.children[i] = node.children[i-1]
	}
	if i == 0 {
		node.keys[0] = key
		node.children[0] = child
	}
	node.numKeys++
}

func (node *BTreeNode[T]) Append(key T, child *BTreeNode[T]) {
	node.keys[node.numKeys] = key
	node.children[node.numKeys+1] = child
	node.numKeys++
}

func (node *BTreeNode[T]) Concat(other *BTreeNode[T], idx int) {
	for i := 0; i < other.numKeys-idx; i++ {
		node.keys[node.numKeys+i] = other.keys[i+idx]
		node.children[node.numKeys+i+1] = other.children[i+idx+1]
	}
	node.numKeys += other.numKeys - idx
}

func (parent *BTreeNode[T]) Split(idx int, maxKeys int) {
	child := parent.children[idx]
	midKeyIndex := maxKeys / 2
	rightChild := NewBTreeNode[T](maxKeys, child.isLeaf)
	rightChild.Concat(child, midKeyIndex+1)
	rightChild.children[0] = child.children[midKeyIndex+1]

	child.numKeys = midKeyIndex

	for i := parent.numKeys; i > idx; i-- {
		parent.keys[i] = parent.keys[i-1]
		parent.children[i+1] = parent.children[i]
	}
	parent.keys[idx] = child.keys[midKeyIndex]
	parent.children[idx] = child
	parent.children[idx+1] = rightChild
	parent.numKeys += 1
}

func (node *BTreeNode[T]) InsertNonFull(tree *BTree[T], key T) {
	node.Verify(tree)
	if node.IsFull(tree.maxKeys) {
		panic("Called InsertNonFull() with a full node")
	}

	if node.isLeaf {
		node.InsertKeyChild(key, nil)
		return
	}

	i := 0
	for ; i < node.numKeys; i++ {
		if key < node.keys[i] {
			break
		}
	}

	if node.children[i].IsFull(tree.maxKeys) {
		node.Split(i, tree.maxKeys)
		if key > node.keys[i] {
			i++
		}
	}
	node.children[i].InsertNonFull(tree, key)
}

func (tree *BTree[T]) Insert(key T) {
	if tree.root == nil {
		tree.root = NewBTreeNode[T](tree.maxKeys, true)
		tree.root.keys[0] = key
		tree.root.numKeys = 1
		return
	}

	if tree.root.IsFull(tree.maxKeys) {
		newRoot := NewBTreeNode[T](tree.maxKeys, false)
		newRoot.numKeys = 0
		newRoot.children[0] = tree.root
		newRoot.Split(0, tree.maxKeys)
		tree.root = newRoot
	}
	tree.root.InsertNonFull(tree, key)
}

func (node *BTreeNode[T]) DeleteIthKey(i int) {
	if i >= node.numKeys {
		panic("deleting out of bounds key")
	}
	for j := i; j < node.numKeys-1; j++ {
		node.keys[j] = node.keys[j+1]
		node.children[j+1] = node.children[j+2]
	}
	node.numKeys--
}

func (node *BTreeNode[T]) Merge(idx int) {
	if node.isLeaf {
		panic("cannot merge when leaf node is parent")
	}
	left := node.children[idx]
	right := node.children[idx+1]
	left.Append(node.keys[idx], right.children[0])
	left.Concat(right, 0)
	node.DeleteIthKey(idx)
}

func (node *BTreeNode[T]) Min() T {
	if node.isLeaf {
		return node.keys[0]
	}
	return node.children[0].Min()
}

func (node *BTreeNode[T]) Max() T {
	if node.isLeaf {
		return node.keys[node.numKeys-1]
	}
	return node.children[node.numKeys].Max()
}

func (node *BTreeNode[T]) Delete(tree *BTree[T], key T) {
	node.Verify(tree)
	if node.isLeaf {
		for i := 0; i < node.numKeys; i++ {
			if key == node.keys[i] {
				node.DeleteIthKey(i)
				return
			}
		}
		return
	}

	minKeys := minKeys(tree.maxKeys)
	i := 0
	for ; i < node.numKeys; i++ {
		if key == node.keys[i] {
			left := node.children[i]
			right := node.children[i+1]
			if left.numKeys > minKeys {
				replacementKey := left.Max()
				node.keys[i] = replacementKey
				left.Delete(tree, replacementKey)
			} else if right.numKeys > minKeys {
				replacementKey := right.Min()
				node.keys[i] = replacementKey
				right.Delete(tree, replacementKey)
			} else {
				if left.numKeys != minKeys || right.numKeys != minKeys {
					panic("nodes should not have less than the minimum number of keys")
				}
				node.Merge(i)
				left.Delete(tree, key)
			}
			return
		}

		if key < node.keys[i] {
			break
		}
	}

	child := node.children[i]
	if child.numKeys == minKeys {
		if i > 0 && node.children[i-1].numKeys > minKeys {
			left := node.children[i-1]
			child.InsertKeyChild(node.keys[i-1], left.children[left.numKeys])
			node.keys[i-1] = left.keys[left.numKeys-1]
			left.numKeys--
		} else if i < node.numKeys && node.children[i+1].numKeys > minKeys {
			right := node.children[i+1]
			child.Append(node.keys[i], right.children[0])
			node.keys[i] = right.keys[0]
			right.children[0] = right.children[1]
			right.DeleteIthKey(0)
		} else {
			if i == 0 {
				node.Merge(i)
			} else {
				node.Merge(i - 1)
				child = node.children[i-1]
			}
		}
	}
	if child.numKeys == minKeys {
		panic("cannot delete key from node with minimum number of keys")
	}
	child.Delete(tree, key)
}

func (tree *BTree[T]) Delete(key T) {
	if tree.root == nil {
		return
	}
	tree.root.Delete(tree, key)
	if tree.root.numKeys == 0 {
		tree.root = tree.root.children[0]
	}
}

func minKeys(maxKeys int) int {
	return (maxKeys - 1) / 2
}
