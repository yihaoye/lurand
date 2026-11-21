package lurand

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"runtime"
	"sync"

	"github.com/go-redis/redis/v8"
)

// CachePRPConfig configures a CachePRP.
type CachePRPConfig struct {
	PRPConfig

	// Client is any go-redis client (single node, cluster, ring, ...).
	Client redis.Cmdable
	// CounterKey is the Redis key holding the shared position counter.
	// All instances that must never return the same number use the same CounterKey and PRPConfig.
	CounterKey string
	// Batch is how many positions an instance reserves per Redis round trip (0 means 1).
	// Larger batches mean fewer round trips; positions reserved by an instance that dies are
	// skipped (never handed out), but never duplicated.
	Batch uint64
}

// CachePRP is a distributed PRP generator: all instances share one Redis counter and
// compute numbers locally from it, so Redis stores two small keys regardless of Max
// (no mapping, no TTL). Uniqueness across instances holds as long as they share the
// same CounterKey, Max, K and Key; a fingerprint stored next to the counter rejects
// instances started with a different Max, K or Key.
type CachePRP struct {
	perm       *Permutation
	k, total   uint64
	client     redis.Cmdable
	counterKey string
	batch      uint64

	mu       sync.Mutex
	spans    []span // reserved but not yet used positions
	inflight int    // refills between INCRBY and handing their leftover span to spans
}

// span is a reserved range of positions [cur, end).
type span struct{ cur, end uint64 }

// NewCachePRP creates a distributed generator. The first instance initialises the
// fingerprint; later instances (or restarts) with the same config attach to it.
func NewCachePRP(ctx context.Context, cfg CachePRPConfig) (*CachePRP, error) {
	perm, k, total, err := cfg.build()
	if err != nil {
		return nil, err
	}
	if cfg.Client == nil || cfg.CounterKey == "" {
		return nil, errors.New("lurand: Client and CounterKey are required")
	}
	batch := cfg.Batch
	if batch == 0 {
		batch = 1
	}
	if batch > total {
		batch = total
	}

	fp := fingerprint(cfg.Key, cfg.Max, k)
	fpKey := cfg.CounterKey + ":cfg"
	if _, err := cfg.Client.SetNX(ctx, fpKey, fp, 0).Result(); err != nil {
		return nil, err
	}
	stored, err := cfg.Client.Get(ctx, fpKey).Result()
	if err != nil {
		return nil, err
	}
	if stored != fp {
		return nil, errors.New("lurand: CounterKey is in use with a different Max, K or Key")
	}

	return &CachePRP{
		perm:       perm,
		k:          k,
		total:      total,
		client:     cfg.Client,
		counterKey: cfg.CounterKey,
		batch:      batch,
	}, nil
}

// fingerprint identifies (Key, Max, K) without revealing Key.
func fingerprint(key []byte, max, k uint64) string {
	h := sha256.New()
	h.Write([]byte("lurand/cacheprp/v1\x00"))
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[:8], max)
	binary.LittleEndian.PutUint64(buf[8:], k)
	h.Write(buf[:])
	h.Write(key)
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// Next returns the next number, or ErrExhausted once every position has been reserved
// and this instance has none left. The Redis round trip happens outside the lock, so
// concurrent callers refill in parallel.
func (g *CachePRP) Next(ctx context.Context) (uint64, error) {
	if i, ok := g.take(); ok {
		return g.perm.At(i) / g.k, nil
	}

	g.mu.Lock()
	g.inflight++
	g.mu.Unlock()

	end, err := g.client.IncrBy(ctx, g.counterKey, int64(g.batch)).Result()
	start := uint64(end) - g.batch

	g.mu.Lock()
	g.inflight--
	if err == nil && start < g.total {
		if stop := min64(uint64(end), g.total); start+1 < stop {
			g.spans = append(g.spans, span{cur: start + 1, end: stop})
		}
	}
	g.mu.Unlock()

	switch {
	case err != nil:
		return 0, err
	case start < g.total:
		return g.perm.At(start) / g.k, nil
	}

	// The shared counter is exhausted, but another goroutine's refill may still be adding a span.
	for {
		g.mu.Lock()
		if n := len(g.spans); n > 0 {
			i := g.popLocked(n)
			g.mu.Unlock()
			return g.perm.At(i) / g.k, nil
		}
		pending := g.inflight
		g.mu.Unlock()
		if pending == 0 {
			return 0, ErrExhausted
		}
		runtime.Gosched()
	}
}

// take hands out a locally reserved position, if any.
func (g *CachePRP) take() (uint64, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if n := len(g.spans); n > 0 {
		return g.popLocked(n), true
	}
	return 0, false
}

func (g *CachePRP) popLocked(n int) uint64 {
	s := &g.spans[n-1]
	i := s.cur
	s.cur++
	if s.cur == s.end {
		g.spans = g.spans[:n-1]
	}
	return i
}

func min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
