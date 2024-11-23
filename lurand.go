package lurand

import (
	"math/rand"
	"sync"
	"time"
)

const defaultMax = 1_000_000 // default set to 1 million

// Use a map[int]int to implement large-scale unique random number generation.
// For example, start by generating random numbers between 0 and 1,000,000.
// Suppose the random number generated is 999. If the map does not have this number as a key,
// return this number and store the current last available number (e.g., 1,000,000) in this key-value pair,
// i.e., <999: 1000000>. Then, the next random number should be generated between 0 and 999,999 (decrement max by 1).
// Similarly, if the next random number's key exists in the map, return its value,
// and overwrite it with the last available number, while decrementing max by 1.
// This is similar to the Fisher–Yates Shuffle algorithm.
type LUR struct {
	mapping map[int]int // mapping random number to current available number
	max     int         // current available number range
	rnd     *rand.Rand
	mu      sync.Mutex
}

// New init time complexity O(1)
func New() *LUR {
	return &LUR{
		mapping: make(map[int]int),
		max:     defaultMax,
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func New_(max int) *LUR {
	return &LUR{
		mapping: make(map[int]int),
		max:     max,
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Intn time complexity O(1), space complexity O(N)
func (r *LUR) Intn() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.max <= 0 {
		panic("No more numbers available")
	}
	delete(r.mapping, r.max) // optimize memory usage

	key := r.rnd.Intn(r.max)
	val, ok1 := r.mapping[key]
	rep, ok2 := r.mapping[r.max-1] // replace
	if !ok2 {
		rep = r.max - 1
	}
	r.mapping[key] = rep
	r.max--
	if !ok1 {
		val = key
	}
	return val
}
