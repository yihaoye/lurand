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
// return this number and store the current last available number (e.g., 999,999) in this key-value pair,
// i.e., <999: 999,999>. Then, the next random number should be generated between 0 and 999,998 (decrement max by 1).
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
	if max <= 0 {
		panic("Max must be greater than 0")
	}
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

// LUR32 is a 32-bit version of LUR
type LUR32 struct {
	mapping map[int32]int32
	max     int32
	rnd     *rand.Rand
	mu      sync.Mutex
}

func New32() *LUR32 {
	return &LUR32{
		mapping: make(map[int32]int32),
		max:     int32(defaultMax),
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func New32_(max int32) *LUR32 {
	if max <= 0 {
		panic("Max must be greater than 0")
	}
	return &LUR32{
		mapping: make(map[int32]int32),
		max:     max,
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (r *LUR32) Int31n() int32 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.max <= 0 {
		panic("No more numbers available")
	}
	delete(r.mapping, r.max)

	key := r.rnd.Int31n(r.max)
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

// LUR64 is a 64-bit version of LUR
type LUR64 struct {
	mapping map[int64]int64
	max     int64
	rnd     *rand.Rand
	mu      sync.Mutex
}

func New64() *LUR64 {
	return &LUR64{
		mapping: make(map[int64]int64),
		max:     defaultMax,
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func New64_(max int64) *LUR64 {
	if max <= 0 {
		panic("Max must be greater than 0")
	}
	return &LUR64{
		mapping: make(map[int64]int64),
		max:     max,
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (r *LUR64) Int63n() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.max <= 0 {
		panic("No more numbers available")
	}
	delete(r.mapping, r.max)

	key := r.rnd.Int63n(r.max)
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
