package lurand

import (
	"math/rand"
	"sync"
	"time"
)

const (
	ONE_MILLION = 1_000_000 // default max
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

	rnd *rand.Rand
	mu  sync.Mutex
}

// New init time complexity O(1)
func New() *LUR {
	return &LUR{
		mapping: make([]int32, ONE_MILLION),
		max:     ONE_MILLION,
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func New_(max int32) *LUR {
	if max <= 0 || max > 100*ONE_MILLION {
		panic("Invalid max setting")
	}
	return &LUR{
		mapping: make([]int32, max),
		max:     max,
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Intn time complexity O(1), space complexity O(N)
func (r *LUR) Int31n() int32 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.max <= 0 {
		panic("No more numbers available")
	}

	key := r.rnd.Int31n(r.max)
	val := r.mapping[key]
	rep := r.mapping[r.max-1] // replace
	if rep == 0 {
		rep = r.max - 1
	}
	r.mapping[key] = rep
	r.max--
	if val == 0 {
		val = key
	}
	return val
}
