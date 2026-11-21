package lurand

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

// redisClient connects to $LURAND_REDIS_ADDR (default localhost:6379) and skips the test if unreachable.
func redisClient(tb testing.TB, poolSize int) *redis.Client {
	tb.Helper()
	addr := os.Getenv("LURAND_REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	client := redis.NewClient(&redis.Options{Addr: addr, PoolSize: poolSize})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		tb.Skipf("redis not available at %s: %v", addr, err)
	}
	tb.Cleanup(func() { _ = client.Close() })
	return client
}

// cleanKeys deletes keys before (and after) a test so reruns are independent.
func cleanKeys(tb testing.TB, client *redis.Client, keys ...string) {
	tb.Helper()
	ctx := context.Background()
	client.Del(ctx, keys...)
	tb.Cleanup(func() { client.Del(ctx, keys...) })
}

func TestCacheDBFunctions(t *testing.T) {
	client := redisClient(t, 10)
	ctx := context.Background()
	max := 10_000
	cleanKeys(t, client, "ftest_", "ftest2_", "p_ftest_")

	t.Run("default max succeed", func(t *testing.T) {
		rg := NewCacheLUR_(ctx, client, "ftest_", int32(max), 60)
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

	t.Run("default max failed", func(t *testing.T) {
		rg := NewCacheLUR_(ctx, client, "ftest2_", 1000, 60)
		var err error
		for i := 0; i <= 1000; i++ {
			_, err = rg.Int31n(ctx)
			if err != nil {
				break
			}
		}
		if err == nil || err.Error() != errNoMore {
			t.Errorf("expected a used up error, but none occurred")
		}
	})

	t.Run("custom max parallel succeed", func(t *testing.T) {
		rg := NewCacheLUR_(ctx, client, "p_ftest_", int32(max), 60)
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

func TestCacheDBRegressions(t *testing.T) {
	client := redisClient(t, 10)
	ctx := context.Background()

	t.Run("ttl 0 means no expiry", func(t *testing.T) {
		cleanKeys(t, client, "reg_ttl0")
		rg := NewCacheLUR_(ctx, client, "reg_ttl0", 100, 0)
		for i := 0; i < 100; i++ {
			if _, err := rg.Int31n(ctx); err != nil {
				t.Fatalf("call %d: unexpected error: %v", i, err)
			}
		}
		if ttl := client.TTL(ctx, "reg_ttl0").Val(); ttl != -1 {
			t.Errorf("expected no expiry, got ttl %v", ttl)
		}
	})

	t.Run("keys with shared prefix do not collide", func(t *testing.T) {
		cleanKeys(t, client, "reg_a", "reg_a1")
		a := NewCacheLUR_(ctx, client, "reg_a", 20, 60)
		_ = NewCacheLUR_(ctx, client, "reg_a1", 999, 60)
		seen := map[int32]bool{}
		for i := 0; i < 20; i++ {
			n, err := a.Int31n(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if n < 0 || n >= 20 || seen[n] {
				t.Fatalf("bad or duplicate value %d", n)
			}
			seen[n] = true
		}
	})

	t.Run("sliding ttl never yields duplicates", func(t *testing.T) {
		if testing.Short() {
			t.Skip("slow")
		}
		cleanKeys(t, client, "reg_expire")
		rg := NewCacheLUR_(ctx, client, "reg_expire", 1000, 2)
		seen := map[int32]bool{}
		for i := 0; i < 1000; i++ {
			n, err := rg.Int31n(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if seen[n] {
				t.Fatalf("call %d: duplicate value %d", i, n)
			}
			seen[n] = true
			if i == 300 || i == 600 {
				time.Sleep(1500 * time.Millisecond) // total run > ttl, but each gap < ttl
			}
		}
	})

	t.Run("expired state is reported, not silently restarted", func(t *testing.T) {
		if testing.Short() {
			t.Skip("slow")
		}
		cleanKeys(t, client, "reg_gone")
		rg := NewCacheLUR_(ctx, client, "reg_gone", 10, 1)
		time.Sleep(1100 * time.Millisecond)
		if _, err := rg.Int31n(ctx); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected key missing error, got %v", err)
		}
	})

	t.Run("max*k overflow is rejected", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Errorf("expected panic")
			}
		}()
		NewCacheLUR__(ctx, client, "reg_overflow", 1<<20, 1<<12, 0)
	})

	t.Run("open existing state and continue", func(t *testing.T) {
		cleanKeys(t, client, "reg_open")
		first := NewCacheLUR__(ctx, client, "reg_open", 50, 2, 0)
		seen := map[int32]int{}
		for i := 0; i < 40; i++ {
			n, _ := first.Int31n(ctx)
			seen[n]++
		}
		second, err := OpenCacheLUR(ctx, client, "reg_open", 2, 0)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 60; i++ {
			n, err := second.Int31n(ctx)
			if err != nil {
				t.Fatal(err)
			}
			seen[n]++
		}
		for n := int32(0); n < 50; n++ {
			if seen[n] != 2 {
				t.Errorf("num %d seen %d times, want 2", n, seen[n])
			}
		}
		if _, err := second.Int31n(ctx); err == nil || err.Error() != errNoMore {
			t.Errorf("expected exhaustion, got %v", err)
		}
		if n := client.HLen(ctx, "reg_open").Val(); n != 1 {
			t.Errorf("exhausted state should only keep the counter field, has %d fields", n)
		}
		if _, err := OpenCacheLUR(ctx, client, "reg_open_missing", 1, 0); err == nil {
			t.Errorf("expected error opening missing key")
		}
	})
}

func BenchmarkCacheTest(b *testing.B) {
	client := redisClient(b, 20)
	ctx := context.Background()

	// BenchmarkCacheTest/NewCacheLUR_-8         	    3476	    377372 ns/op	     278 B/op	       7 allocs/op
	b.Run("NewCacheLUR_", func(b *testing.B) {
		cleanKeys(b, client, "btest_")
		rg := NewCacheLUR_(ctx, client, "btest_", int32(b.N), 60)
		for i := 0; i < b.N; i++ {
			_, _ = rg.Int31n(ctx)
		}
	})

	// BenchmarkCacheTest/ParallelCache-8        	   11744	    111448 ns/op	     279 B/op	       7 allocs/op
	b.Run("ParallelCache", func(b *testing.B) {
		cleanKeys(b, client, "p_btest_")
		rg := NewCacheLUR_(ctx, client, "p_btest_", int32(b.N), 60)
		b.RunParallel(func(pb *testing.PB) {
			for i := 0; pb.Next(); i++ {
				_, _ = rg.Int31n(ctx)
			}
		})
	})
}
