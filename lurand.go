package lurand

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ONE_MILLION = 1_000_000 // default max
	ONE_BILLION = 1_000_000_000
)

var rPool = sync.Pool{
	New: func() interface{} {
		return rand.New(rand.NewSource(time.Now().UnixNano()))
	},
}

// Use a map[int]int to implement large-scale unique random number generation.
// For example, start by generating random numbers between 0 and 1,000,000.
// Suppose the random number generated is 999. If the map does not have this number as a key,
// return this number and store the current last available number (e.g., 999,999) in this key-value pair,
// i.e., <999: 999,999>. Then, the next random number should be generated between 0 and 999,998 (decrement max by 1).
// Similarly, if the next random number's key exists in the map, return its value,
// and overwrite it with the last available number, while decrementing max by 1.
// This is similar to the Fisher–Yates Shuffle algorithm.
type LUR struct {
	mapping []int32 // mapping random number to current available number
	max     int32   // current available number range
}

// New init time complexity O(1)
func New() *LUR {
	return &LUR{
		mapping: make([]int32, ONE_MILLION),
		max:     ONE_MILLION,
	}
}

func New_(max int32) *LUR {
	if max <= 0 || max > (ONE_BILLION/10) {
		panic("Invalid max value")
	}
	return &LUR{
		mapping: make([]int32, max),
		max:     max,
	}
}

// Int31n time complexity O(1), space complexity O(N)
func (r *LUR) Int31n() int32 {
	max := atomic.AddInt32(&r.max, -1)
	if max < 0 {
		panic("No more numbers available")
	}
	if max == 0 {
		return r.mapping[max]
	}

	rep := r.mapping[max] // replace
	if rep == 0 {
		rep = max
	}

	rnd := rPool.Get().(*rand.Rand)
	defer rPool.Put(rnd)

	for {
		key := rnd.Int31n(atomic.LoadInt32(&r.max))
		val := atomic.LoadInt32(&r.mapping[key])
		if val == 0 {
			val = key
		}
		if atomic.CompareAndSwapInt32(&r.mapping[key], val, rep) && key < atomic.LoadInt32(&r.max) {
			return val
		}
	}
}
