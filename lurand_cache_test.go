package lurand

import (
	"context"
	"sync"
	"testing"
)

func TestCacheDBFunctions(t *testing.T) {
	InitCache_("localhost:6379", 60)
	ctx := context.Background()
	max := 10_000

	t.Run("default max succeed", func(t *testing.T) {
		rg := NewCacheLUR_(ctx, "ftest_", int32(max))
		dedup := make(map[int32]bool)
		for i := 0; i < max; i++ {
			num, err := rg.Int31n(ctx)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if dedup[num] {
				t.Errorf("%d: duplicate num: %d", i, num)
				return
			}
			dedup[num] = true
		}
		if len(dedup) != max {
			t.Errorf("len(dedup) != max: %d != %d", len(dedup), max)
			return
		}
	})

	t.Run("custom max parallel succeed", func(t *testing.T) {
		rg := NewCacheLUR_(ctx, "p_ftest_", int32(max))
		dedup := sync.Map{}
		var wg sync.WaitGroup
		concurrentWorkers := 100
		numbersPerWorker := max / concurrentWorkers

		for i := 0; i < concurrentWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for j := 0; j < numbersPerWorker; j++ {
					num, err := rg.Int31n(ctx)
					if err != nil {
						t.Errorf("worker %d: unexpected error: %v", workerID, err)
						return
					}
					if _, loaded := dedup.LoadOrStore(num, true); loaded {
						t.Errorf("worker %d: duplicate num: %d", workerID, num)
						return
					}
				}
			}(i)
		}
		wg.Wait()

		count := 0
		dedup.Range(func(_, _ interface{}) bool {
			count++
			return true
		})
		if count != max {
			t.Errorf("expected %d unique numbers, but got %d", max, count)
		}
	})
}

func BenchmarkCacheTest(b *testing.B) {
	InitCache_("localhost:6379", 60)
	ctx := context.Background()

	// BenchmarkCacheTest/NewCacheLUR_-8         	    3597	    413351 ns/op	     278 B/op	       7 allocs/op
	b.Run("NewCacheLUR_", func(b *testing.B) {
		rg := NewCacheLUR_(ctx, "btest_", int32(b.N))
		for i := 0; i < b.N; i++ {
			_, _ = rg.Int31n(ctx)
		}
	})

	// BenchmarkCacheTest/ParallelCache-8        	   11744	    111448 ns/op	     279 B/op	       7 allocs/op
	b.Run("ParallelCache", func(b *testing.B) {
		rg := NewCacheLUR_(ctx, "p_btest_", int32(b.N))
		b.RunParallel(func(pb *testing.PB) {
			for i := 0; pb.Next(); i++ {
				_, _ = rg.Int31n(ctx)
			}
		})
	})
}
