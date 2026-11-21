package lurand

import (
	"context"
	"fmt"
	"testing"
)

// Side-by-side benchmarks of the generators. Run with:
//
//	go test -run '^$' -bench 'Compare' -benchmem
//
// Redis benchmarks are skipped when no server is reachable (see LURAND_REDIS_ADDR).

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

// BenchmarkCompareRedis measures the distributed generators against a Redis server.
func BenchmarkCompareRedis(b *testing.B) {
	client := redisClient(b, 64)
	ctx := context.Background()

	b.Run("CacheLUR", func(b *testing.B) {
		cleanKeys(b, client, "bench_lur")
		g := NewCacheLUR_(ctx, client, "bench_lur", int32(b.N), 0)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_, _ = g.Int31n(ctx)
			}
		})
	})
	for _, batch := range []uint64{1, 100} {
		b.Run(fmt.Sprintf("CachePRP/batch=%d", batch), func(b *testing.B) {
			cleanKeys(b, client, "bench_prp", "bench_prp:cfg")
			g, err := NewCachePRP(ctx, CachePRPConfig{
				PRPConfig:  PRPConfig{Max: uint64(b.N), Key: testKey},
				Client:     client,
				CounterKey: "bench_prp",
				Batch:      batch,
			})
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _ = g.Next(ctx)
				}
			})
		})
	}
}
