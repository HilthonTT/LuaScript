package pool

import (
	"sync"
	"sync/atomic"
)

// TypedPool provides type-safe object pooling with automatic sizing
type TypedPool[T any] struct {
	pools []sync.Pool
	sizes []int
	stats PoolStats
}

type PoolStats struct {
	Gets      int64
	Puts      int64
	Allocated int64
	Reused    int64
}

func NewTypedPool[T any](sizes []int) *TypedPool[T] {
	p := &TypedPool[T]{
		pools: make([]sync.Pool, 0),
		sizes: sizes,
	}

	for i, size := range sizes {
		size := size // capture loop variable
		p.pools[i] = sync.Pool{
			New: func() any {
				slice := make([]T, 0, size)
				atomic.AddInt64(&p.stats.Allocated, 1)
				return &slice
			},
		}
	}

	return p
}

// Get retrieves a slice with the closest matching capacity
func (p *TypedPool[T]) Get(minSize int) []T {
	poolIndex := p.findBestPool(minSize)
	if poolIndex == -1 {
		// Size too large for any pool, allocate directly
		atomic.AddInt64(&p.stats.Gets, 1)
		atomic.AddInt64(&p.stats.Allocated, 1)
		return make([]T, 0, minSize)
	}

	atomic.AddInt64(&p.stats.Gets, 1)
	atomic.AddInt64(&p.stats.Reused, 1)

	slice := p.pools[poolIndex].Get().(*[]T)
	*slice = (*slice)[:0] // Reset length but keep capacity
	return *slice
}

// Put returns a slice to the appropriate pool
func (p *TypedPool[T]) Put(slice []T) {
	if cap(slice) == 0 {
		return
	}

	poolIndex := p.findPoolForCapacity(cap(slice))
	if poolIndex == -1 {
		return // Capacity doesn't match any pool
	}

	atomic.AddInt64(&p.stats.Puts, 1)

	// Clear references to prevent memory leaks
	for i := range slice {
		var zero T
		slice[i] = zero
	}

	slice = slice[:0] // Reset length
	p.pools[poolIndex].Put(&slice)
}

func (p *TypedPool[T]) findBestPool(minSize int) int {
	for i, size := range p.sizes {
		if size >= minSize {
			return i
		}
	}
	return -1
}

func (p *TypedPool[T]) findPoolForCapacity(capacity int) int {
	for i, size := range p.sizes {
		if size == capacity {
			return i
		}
	}
	return -1
}
