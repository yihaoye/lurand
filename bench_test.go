package lurand

import (
	"fmt"
	"testing"
)

// Run with:
//
//	go test -run '^$' -bench . -benchmem

// BenchmarkNext measures the cost of one number on a single goroutine.
func BenchmarkNext(b *testing.B) {
	b.Run("Max=b.N", func(b *testing.B) {
		g, _ := NewPRP(PRPConfig{Max: uint64(b.N), Key: testKey})
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = g.Next()
		}
	})
	b.Run("Max=2^62", func(b *testing.B) {
		g, _ := NewPRP(PRPConfig{Max: MaxPermutationSize, Key: testKey})
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = g.Next()
		}
	})
}

// BenchmarkNextParallel measures throughput with GOMAXPROCS goroutines sharing one generator.
func BenchmarkNextParallel(b *testing.B) {
	g, _ := NewPRP(PRPConfig{Max: MaxPermutationSize, Key: testKey})
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = g.Next()
		}
	})
}

// BenchmarkNew reports the memory a generator holds (B/op of construction), independent of Max.
func BenchmarkNew(b *testing.B) {
	for _, n := range []uint64{1_000_000, 1_000_000_000_000, MaxPermutationSize} {
		b.Run(fmt.Sprintf("Max=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = NewPRP(PRPConfig{Max: n, Key: testKey})
			}
		})
	}
}
