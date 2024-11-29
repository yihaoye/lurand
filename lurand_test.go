package lurand

import (
	"sync"
	"testing"
)

func TestFunctions(t *testing.T) {
	t.Run("default max succeed", func(t *testing.T) {
		rg := New()
		dedup := make(map[int32]bool)
		for i := 0; i < ONE_MILLION; i++ {
			num := rg.Int31n()
			if dedup[num] {
				t.Errorf("%d: duplicate num: %d", i, num)
				return
			}
			dedup[num] = true
		}
		if len(dedup) != ONE_MILLION {
			t.Errorf("len(dedup) != ONE_MILLION: %d != %d", len(dedup), ONE_MILLION)
			return
		}
	})

	t.Run("default max failed", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				if r != "No more numbers available" {
					t.Errorf("Unexpected panic message: %v", r)
				}
			} else {
				t.Errorf("Expected a panic, but none occurred")
			}
		}()

		rg := New()
		dedup := make(map[int32]bool)
		for i := 0; i < ONE_MILLION+1; i++ {
			num := rg.Int31n()
			if dedup[num] {
				t.Errorf("%d: duplicate num: %d", i, num)
				return
			}
			dedup[num] = true
		}
	})

	t.Run("custom max parallel succeed", func(t *testing.T) {
		rg := New_(1_500_000)
		dedup := sync.Map{}
		var wg sync.WaitGroup
		concurrentWorkers := 100
		numbersPerWorker := 15_000

		for i := 0; i < concurrentWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for j := 0; j < numbersPerWorker; j++ {
					num := rg.Int31n()
					if _, loaded := dedup.LoadOrStore(num, true); loaded {
						t.Errorf("worker %d: duplicate num: %d", workerID, num)
						return
					}
				}
			}(i)
		}

		wg.Wait()

		totalNumbers := concurrentWorkers * numbersPerWorker
		count := 0
		dedup.Range(func(_, _ interface{}) bool {
			count++
			return true
		})
		if count != totalNumbers {
			t.Errorf("expected %d unique numbers, but got %d", totalNumbers, count)
		}
	})
}

// goos: darwin
// goarch: amd64
// cpu: 2.50GHz
func BenchmarkTest(b *testing.B) {
	// BenchmarkTest/New_-8         	68782362	        77.63 ns/op	       4 B/op	       0 allocs/op
	b.Run("New_", func(b *testing.B) {
		rg := New_(int32(b.N))
		for i := 0; i < b.N; i++ {
			_ = rg.Int31n()
		}
	})

	// BenchmarkTest/Parallel-8     	 6146419	       218.4 ns/op	       4 B/op	       0 allocs/op
	b.Run("Parallel", func(b *testing.B) {
		rg := New_(int32(b.N))
		b.RunParallel(func(pb *testing.PB) {
			for i := 0; pb.Next(); i++ {
				_ = rg.Int31n()
			}
		})
	})
}
