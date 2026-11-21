package lurand

import (
	"math/rand"
	"sync"
	"time"
)

const (
	ONE_MILLION = 1_000_000 // default max

	// maxLURSize caps max*k for the in-memory LUR (~400MB of int32 slots).
	maxLURSize = 100 * ONE_MILLION
)

// Use a map[int32]int32 to implement large-scale unique random number generation.
// For example, start by generating random numbers between 0 and 1,000,000.
// Suppose the random number generated is 999. If the map does not have this number as a key,
// return this number and store the current last available number (e.g., 999,999) in this key-value pair,
// i.e., <999: 999,999>. Then, the next random number should be generated between 0 and 999,998 (decrement max by 1).
// Similarly, if the next random number's key exists in the map, return its value,
// and overwrite it with the last available number, while decrementing max by 1.
// This is similar to the Fisher–Yates Shuffle algorithm.
type LUR struct {
	// mapping random number to current available number, apply array instead of map for better performance,
	// also array could throw panic if the number is out of range initially
	mapping []int32

	// current available number range
	max int32

	// max duplicate times, default set to 1
	k int32

	rnd *rand.Rand
	mu  sync.Mutex
}

// New creates a generator over [0, 1_000_000).
// Init allocates the whole mapping up front: time and space O(max*k).
func New() *LUR {
	return New__(ONE_MILLION, 1)
}

func New_(max int32) *LUR {
	return New__(max, 1)
}

func New__(max int32, k int32) *LUR {
	if k <= 0 {
		panic("Invalid k setting")
	}
	// multiply in int64 so that max*k cannot overflow int32 and bypass the cap
	if max <= 0 || int64(max)*int64(k) > maxLURSize {
		panic("Invalid max setting")
	}
	max = max * k
	return &LUR{
		mapping: make([]int32, max),
		max:     max,
		k:       k,
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Int31n returns the next unique number. Time O(1); the generator holds O(max*k) space.
func (r *LUR) Int31n() int32 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.max <= 0 {
		panic("No more numbers available")
	}

	key := r.rnd.Int31n(r.max)
	val := r.mapping[key]
	if val == 0 {
		val = key
	}
	rep := r.mapping[r.max-1] // replace
	if rep == 0 {
		rep = r.max - 1
	}
	r.mapping[key] = rep
	r.max--
	return val / r.k
}
