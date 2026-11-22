package lurand_test

import (
	"fmt"

	"github.com/yihaoye/lurand"
)

func ExamplePRP() {
	// In production use lurand.GenerateKey() once and store the key as a secret.
	key := []byte("an example key, keep yours secret")

	g, err := lurand.NewPRP(lurand.PRPConfig{Max: 10, Key: key})
	if err != nil {
		panic(err)
	}
	for {
		n, err := g.Next()
		if err == lurand.ErrExhausted {
			break
		}
		fmt.Print(n, " ")
	}
	fmt.Println()
	// Output: 9 8 6 4 1 5 0 2 7 3
}

func ExamplePRP_duplicates() {
	key := []byte("an example key, keep yours secret")

	// every number in [0, 4) exactly 3 times
	g, _ := lurand.NewPRP(lurand.PRPConfig{Max: 4, K: 3, Key: key})
	for i := 0; i < 12; i++ {
		n, _ := g.Next()
		fmt.Print(n, " ")
	}
	fmt.Println()
	// Output: 0 2 1 0 2 2 1 3 3 3 0 1
}

func ExamplePRP_resume() {
	key := []byte("an example key, keep yours secret")
	cfg := lurand.PRPConfig{Max: 1_000_000_000_000, Key: key} // 10^12 numbers, still O(1) memory

	g, _ := lurand.NewPRP(cfg)
	a, _ := g.Next()
	pos := g.Position() // persist this (e.g. on shutdown) ...

	g2, _ := lurand.NewPRPAt(cfg, pos) // ... and continue the same sequence later
	b, _ := g2.Next()
	fmt.Println(a != b, g2.Position())
	// Output: true 2
}

func ExampleNewPRPShard() {
	key := []byte("an example key, keep yours secret")
	cfg := lurand.PRPConfig{Max: 10, Key: key}

	// three instances, e.g. shard = pod ordinal; together they hand out [0, 10) exactly once
	for shard := uint64(0); shard < 3; shard++ {
		g, _ := lurand.NewPRPShard(cfg, shard, 3)
		var nums []uint64
		for {
			n, err := g.Next()
			if err == lurand.ErrExhausted {
				break
			}
			nums = append(nums, n)
		}
		fmt.Println(nums)
	}
	// Output:
	// [9 4 0 3]
	// [8 1 2]
	// [6 5 7]
}

func ExamplePermutation_Index() {
	key := []byte("an example key, keep yours secret")
	p, _ := lurand.NewPermutation(1_000_000, key)

	code := p.At(42)                   // the 43rd number handed out
	fmt.Println(p.Index(code) == 42)   // recover its position without any lookup table
	fmt.Println(p.Index(code) < 1_000) // e.g. "was this code issued among the first 1000?"
	// Output:
	// true
	// true
}
