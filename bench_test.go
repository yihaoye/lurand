package lurand

import (
	"fmt"
	"testing"
)

// Side-by-side benchmarks of the generators. Run with:
//
//	go test -run '^$' -bench 'Compare' -benchmem

// BenchmarkCompareNext measures the cost of one number on a single goroutine.
func BenchmarkCompareNext(b *testing.B) {
	b.Run("LUR", func(b *testing.B) {
		g := New_(int32(b.N))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = g.Int31n()
		}
	})
	b.Run("PRP", func(b *testing.B) {
		g, _ := NewPRP(PRPConfig{Max: uint64(b.N), Key: testKey})
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = g.Next()
		}
	})
	b.Run("PRP-max-2^62", func(b *testing.B) {
		g, _ := NewPRP(PRPConfig{Max: MaxPermutationSize, Key: testKey})
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = g.Next()
		}
	})
}

// BenchmarkCompareParallel measures throughput with GOMAXPROCS goroutines sharing one generator.
func BenchmarkCompareParallel(b *testing.B) {
	b.Run("LUR", func(b *testing.B) {
		g := New_(int32(b.N))
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = g.Int31n()
			}
		})
	})
	b.Run("PRP", func(b *testing.B) {
		g, _ := NewPRP(PRPConfig{Max: uint64(b.N), Key: testKey})
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, _ = g.Next()
			}
		})
	})
}

// BenchmarkCompareMemory reports the memory a generator holds for a given range (B/op of construction).
func BenchmarkCompareMemory(b *testing.B) {
	for _, n := range []int{1_000_000, 10_000_000, 100_000_000} {
		b.Run(fmt.Sprintf("LUR/N=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = New_(int32(n))
			}
		})
		b.Run(fmt.Sprintf("PRP/N=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = NewPRP(PRPConfig{Max: uint64(n), Key: testKey})
			}
		})
	}
}
