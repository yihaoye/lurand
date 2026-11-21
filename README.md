# LuRand
Large-scale unique random number generators for Go. Every number in `[0, max)` is returned exactly once (or exactly `k` times), in random order, in `O(1)` time per number and safe for concurrent use.

LuRand offers two strategies with different trade-offs. Each has an in-process version and a Redis-backed version:

| | `LUR` / `CacheLUR` | `PRP` / `CachePRP` |
|---|---|---|
| Algorithm | Sparse Fisher–Yates shuffle | Keyed pseudo-random permutation (Feistel + cycle-walking) |
| Space | `O(max·k)`: 4 bytes per number up front (`LUR`), or one hash field per moved slot (`CacheLUR`) | **`O(1)`**: a key and a counter |
| Time per number | `O(1)`, ~55 ns | `O(1)`, ~230–370 ns |
| Order | Uniformly random (up to the quality of `math/rand`) | Computationally indistinguishable from random to anyone without the key |
| Max range | `max·k ≤ 10^8` (`LUR`), `< 2^31` (`CacheLUR`) | `max·k ≤ 2^62` |
| Distributed | Lua script on one Redis key | Shared Redis counter; each instance reserves blocks of positions |
| Best for | Small to medium ranges, or when you need true uniform shuffling | Huge ranges (10^9 and up), low memory, many instances, or when a leaked RNG state must not reveal past and future numbers |

Use `LUR` for small ranges. Below about 10^6, `PRP` is not recommended (see [Limits](#limits-and-security-notes)).

## Install
`go get github.com/yihaoye/lurand`

## Usage

### LUR (in memory, sparse Fisher–Yates)
```go
// [0..1_000_000)
r1 := lurand.New()
num := r1.Int31n() // e.g. 876783

// [0..{custom})
r2 := lurand.New_(10)
for i := 0; i < 10; i++ {
    num := r2.Int31n() // e.g. 4, 8, 3, 0, 9, 1, 6, 2, 5, 7
}
r2.Int31n() // panic: No more numbers available

// each number at most k times (k = 3 here)
r3 := lurand.New__(4, 3)
for i := 0; i < 12; i++ {
    num := r3.Int31n() // e.g. 0, 3, 2, 0, 3, 1, 1, 2, 2, 0, 1, 3
}
```

### PRP (in memory, O(1) space)
```go
key, _ := lurand.GenerateKey() // do this once, store it as a secret, reuse it

cfg := lurand.PRPConfig{
    Max: 1_000_000_000_000, // 10^12 numbers, still O(1) memory
    K:   1,                 // optional: return every number exactly K times
    Key: key,
}
g, err := lurand.NewPRP(cfg)
num, err := g.Next() // err == lurand.ErrExhausted once all numbers are used

// resume after a restart: persist g.Position(), then
g, err = lurand.NewPRPAt(cfg, savedPosition)
```

The underlying `Permutation` is also usable on its own, and it is invertible:
```go
p, _ := lurand.NewPermutation(1_000_000, key)
code := p.At(42)  // the number at position 42
p.Index(code)     // 42: find a value's position without any lookup table
```

### CachePRP (distributed, O(1) Redis memory)
All instances that use the same `CounterKey`, `Max`, `K` and `Key` share one sequence. Redis stores only a counter and a config fingerprint, no matter how large `Max` is.
```go
client := redis.NewClient(&redis.Options{Addr: "localhost:6379"}) // or a ClusterClient
g, err := lurand.NewCachePRP(ctx, lurand.CachePRPConfig{
    PRPConfig:  lurand.PRPConfig{Max: 1_000_000_000_000, Key: key},
    Client:     client,
    CounterKey: "coupon:seq",
    Batch:      100, // positions reserved per Redis round trip
})
num, err := g.Next(ctx)
```
- Each instance reserves `Batch` positions with one `INCRBY` and computes the numbers locally. Larger batches mean fewer round trips.
- If an instance crashes, the rest of its block is skipped: those numbers are never handed out, but no number is ever handed out twice.
- Starting an instance with a different `Max`, `K` or `Key` on the same `CounterKey` returns an error instead of silently producing duplicates.

### CacheLUR (distributed sparse Fisher–Yates)
```go
r1 := lurand.NewCacheLUR_(ctx, client, "my-generator", 1_000_000, 0) // last param: ttl in seconds, 0 = no expiry
num, err := r1.Int31n(ctx)

// after a restart, attach to the existing state instead of creating a new one
r1, err = lurand.OpenCacheLUR(ctx, client, "my-generator", 1, 0)
```
- All state of one generator lives in a single Redis hash. A TTL applies to the whole state and is refreshed on every call.
- The script only touches the key it declares, so it also works on Redis Cluster.
- Redis memory grows with the number of slots that have been moved, up to `O(max·k)`. For huge ranges use `CachePRP`.

## How it works

### LUR: sparse Fisher–Yates
Picture a virtual array `[0, 1, ..., max-1]`. To draw a number:
1. Pick a random index `i` in `[0, max)` and return the value at `i`.
2. Move the last element into slot `i` and shrink `max` by one.

Only slots that have been moved need to be stored; every other slot implicitly holds its own index. This is the classic Fisher–Yates (Durstenfeld) shuffle performed lazily, one step per call. `LUR` stores the slots in a preallocated `[]int32`, which is fast but `O(max)` from the start. `CacheLUR` stores them sparsely in a Redis hash.

### Why exact uniform sampling cannot beat O(N)
After `m` of `N` numbers have been drawn, the generator must know which `m` are used, and there are `C(N, m)` possibilities. Any exact sampler therefore needs at least `log2 C(N, m)` bits of state, about `N` bits at `m = N/2`. Probabilistic structures such as Bloom filters do not escape this bound. They also give false positives, so some numbers would never be returned, and rejection sampling slows down as the range fills up.

The way out is to relax "uniformly random" to "computationally indistinguishable from uniformly random": that is what a pseudo-random permutation provides.

### PRP: Feistel network + cycle-walking
`PRP` returns `Permutation.At(0), At(1), At(2), ...`. `At` is a keyed bijection on `[0, n)`, so distinct positions always give distinct numbers, and the only state is the counter.

- **Feistel network.** Split `x` into two halves `(a, c)` and repeat `a, c = c, a XOR F(c)`. Each round can be undone (`c = a'`, `a = c' XOR F(a')`), so the network is a bijection no matter what `F` is. `F` only has to be a good keyed pseudo-random function; LuRand uses SipHash-2-4. By the Luby–Rackoff theorem, 4 rounds with a secure `F` give a strong pseudo-random permutation. LuRand uses 10 rounds, as NIST FF1 does. The two halves alternate between `floor(b/2)` and `ceil(b/2)` bits, so any bit width `b` works.
- **Cycle-walking.** The network permutes `[0, 2^b)` with `b = ceil(log2 n)`. If the result is `>= n`, it is encrypted again until it lands in `[0, n)` (Black & Rogaway, 2002). This always terminates, because following the permutation's cycle from `x < n` returns to `x`. The result is still a bijection on `[0, n)`. Since `2^b < 2n`, it takes fewer than 2 encryptions on average.
- **K duplicates.** The domain is `[0, Max·K)` and each result is divided by `K`.

## Limits and security notes
- **Small domains.** The Feistel security bound degrades as the domain shrinks: it is roughly `q² / 2^(b/2)` for `q` observed outputs. NIST requires at least 10^6 for FF1, and FF3 was revised after attacks on small domains. Below about 10^6, use `LUR`: memory is small there anyway, and its order is truly uniform.
- **The key is the secret.** Anyone who knows the key (and `Max`, `K`) can compute the whole sequence and its inverse. Generate it with `GenerateKey`, store it like any other secret, and never change it for a running sequence.
- **Not a certified FPE.** The construction follows FF1's structure but is not FF1. If you need a standardized, audited cipher (for example for card numbers), use an FF1 implementation. In both `LUR` and `PRP`, the order is random but not cryptographically uniform in the strict sense: `LUR` relies on `math/rand`, and `PRP` selects one of 2^128 keyed permutations rather than one of all `N!`.
- **Panics.** The `LUR`/`CacheLUR` constructors panic on invalid settings, and `LUR.Int31n` panics when the range is exhausted (kept for backward compatibility). The `PRP` APIs return errors instead.

## Benchmarks
Intel Xeon @ 2.10GHz, 4 cores, Go 1.24, Redis 7.0 on localhost. Run with `go test -run '^$' -bench Compare -benchmem`.

| Benchmark | LUR / CacheLUR | PRP / CachePRP |
|---|---|---|
| One number, single goroutine | 55 ns | 370 ns (`Max` just above a power of two, so ~1.7 Feistel passes); 232 ns (`Max = 2^62`, no cycle-walking) |
| One number, 4 goroutines sharing a generator | 170 ns | 101 ns (lock-free counter) |
| Memory held, `Max = 10^6` | 4.0 MB | 80 B |
| Memory held, `Max = 10^7` | 40 MB | 80 B |
| Memory held, `Max = 10^8` | 400 MB | 80 B |
| Redis, 4 goroutines | 32 µs (`CacheLUR`, one script call per number) | 16 µs (`Batch = 1`); 0.44 µs (`Batch = 100`) |

## Testing
```bash
go test -race ./...
```
- **Redis.** Redis tests use `$LURAND_REDIS_ADDR` (default `localhost:6379`) and are skipped if no server is reachable. To run them locally:
  ```bash
  docker run -d --name lurand-redis -p 6379:6379 redis:7
  ```
- **What is covered:**
  - bijection on many domain sizes (odd and even bit widths, sizes just above a power of two), and `At`/`Index` round trips up to 2^62
  - key and domain separation
  - a chi-square uniformity test over 20,000 keys, which fails for 1–2 Feistel rounds and passes for 10
  - the official SipHash-2-4 test vector
  - concurrency, resume, multi-instance Redis use and crash recovery
  - regression tests for the `CacheLUR` bugs listed below

## Changelog
- **`CacheLUR` storage format changed.** State now lives in a single Redis hash, which is not compatible with keys created by earlier versions. This fixes:
  - duplicate numbers when some mapping keys expired before the counter
  - `ttl = 0` failing on every call
  - collisions between generators whose names share a prefix (`"a"` + `"1"` vs `"a1"`)
  - keys that were not declared to the Lua script, which breaks Redis Cluster rules
- **No more Lua seeding.** The random index is now chosen by the Go client, so the `math.randomseed` call from earlier examples is no longer needed.
- **Overflow check.** `max·k` is validated in 64-bit arithmetic. Previously the int32 product could overflow and bypass the size limit.
- **New APIs:** `OpenCacheLUR`, `Permutation`, `PRP`, `CachePRP`.

## References
- R. A. Fisher, F. Yates (1938); R. Durstenfeld, *Algorithm 235: Random permutation*, CACM 1964.
- M. Luby, C. Rackoff, *How to construct pseudorandom permutations from pseudorandom functions*, SIAM J. Comput. 1988.
- J. Black, P. Rogaway, *Ciphers with arbitrary finite domains*, CT-RSA 2002 (cycle-walking).
- NIST SP 800-38G Rev. 1, *Methods for Format-Preserving Encryption* (FF1).
- J.-P. Aumasson, D. J. Bernstein, *SipHash: a fast short-input PRF*, INDOCRYPT 2012.
- Related uses: masscan's BlackRock cipher for randomized scan order; ZMap's cyclic-group address iteration.
