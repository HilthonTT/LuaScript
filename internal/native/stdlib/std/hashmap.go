package std

import (
	"fmt"
	"hash/fnv"
)

var defaultCapacity uint64 = 1 << 10

type node struct {
	key   any
	value any
	next  *node
}

type HashMap struct {
	buckets  []*node
	size     uint64
	capacity uint64
}

func DefaultHashMap() *HashMap {
	return &HashMap{
		capacity: defaultCapacity,
		buckets:  make([]*node, defaultCapacity),
	}
}

func NewHashMap(size, capacity uint64) *HashMap {
	return &HashMap{
		buckets:  make([]*node, capacity),
		size:     size,
		capacity: capacity,
	}
}

func (hm *HashMap) Get(key any) any {
	node := hm.getNodeByKey(key)
	if node != nil {
		return node.value
	}
	return nil
}

func (hm *HashMap) Put(key, value any) {
	index := hm.hash(key)
	if hm.buckets[index] == nil {
		hm.buckets[index] = &node{key: key, value: value}
		// size is incremented once, unconditionally, after this if/else.
	} else {
		current := hm.buckets[index]
		for {
			if current.key == key {
				current.value = value
				return
			}
			if current.next == nil {
				break
			}
			current = current.next
		}
		current.next = &node{key: key, value: value}
	}
	hm.size++
	if float64(hm.size)/float64(hm.capacity) > 0.75 {
		hm.resize()
	}
}

func (hm *HashMap) Contains(key any) bool {
	return hm.getNodeByKey(key) != nil
}

func (hm *HashMap) getNodeByKey(key any) *node {
	index := hm.hash(key)
	current := hm.buckets[index]
	for current != nil {
		if current.key == key {
			return current
		}
		current = current.next
	}
	return nil
}

func (hm *HashMap) resize() {
	oldBuckets := hm.buckets
	hm.capacity <<= 1
	hm.size = 0
	hm.buckets = make([]*node, hm.capacity)

	for _, bucket := range oldBuckets {
		for bucket != nil {
			hm.Put(bucket.key, bucket.value)
			bucket = bucket.next
		}
	}
}

func (hm *HashMap) hash(key any) uint64 {
	h := fnv.New64a()
	_, err := fmt.Fprintf(h, "%v", key)
	if err != nil {
		panic(err)
	}
	hashValue := h.Sum64()
	return (hm.capacity - 1) & (hashValue >> 16)
}
