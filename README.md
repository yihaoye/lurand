# LuRand
Large-scale unique random number generators for Go, with no dependencies. Every number in `[0, max)` is returned exactly once (or exactly `k` times), in random order, in `O(1)` time per number. Both generators are safe for concurrent use.

LuRand offers two strategies with different trade-offs:

| | `LUR` | `PRP` |
|---|---|---|
| Algorithm | Sparse Fisher–Yates shuffle | Keyed pseudo-random permutation (Feistel + cycle-walking) |
| Space | `O(max·k)`: 4 bytes per number, allocated up front | **`O(1)`**: a key and a counter |
| Time per number | `O(1)`, ~55 ns | `O(1)`, ~230–370 ns |
| Order | Uniformly random (up to the quality of `math/rand`) | Computationally indistinguishable from random to anyone without the key |
| Max range | `max·k ≤ 10^8` | `max·k ≤ 2^62` |
| Best for | Small to medium ranges, or when you need true uniform shuffling | Huge ranges (10^9 and up), low memory, resumable or multi-instance sequences |

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

### PRP (O(1) space)
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

### Restarts and multiple instances
The sequence itself is pure math: the same `(Max, K, Key)` always gives the same order, and the `i`-th number is `Permutation.At(i) / K`. The only state is **which position comes next**. Uniqueness depends on never using the same position twice, so:

- **Single process, across restarts:** persist `Position()` (a file, a database row, ...) and resume with `NewPRPAt`. To avoid a write per number, reserve ahead: save `pos + 1000`, hand out positions up to it, then save the next block. A crash only skips the unused rest of a block; it never repeats a number.
- **A fixed number of instances:** use `NewPRPShard`. Shard `j` of `n` takes positions `j, j+n, j+2n, ...`, so the shards together cover the sequence exactly once with no coordination at all. Each shard persists its own `Position()` and resumes with `NewPRPShardAt`. `n` must stay the same for the lifetime of the sequence, and each shard must run on at most one instance at a time.
  ```go
  g, err := lurand.NewPRPShard(cfg, podOrdinal, 3) // same cfg and shard count on every instance
  num, err := g.Next()
  ```
- **A dynamic number of instances:** reserve blocks of positions from any atomic counter you already have (a database sequence, Redis `INCRBY`, etcd, ...) and call `Permutation.At(pos) / K` directly.

## How it works

### LUR: sparse Fisher–Yates
Picture a virtual array `[0, 1, ..., max-1]`. To draw a number:
1. Pick a random index `i` in `[0, max)` and return the value at `i`.
2. Move the last element into slot `i` and shrink `max` by one.

This is the classic Fisher–Yates (Durstenfeld) shuffle performed lazily, one step per call. `LUR` stores the slots in a preallocated `[]int32`, which is fast but `O(max)` from the start.

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
- **Panics.** The `LUR` constructors panic on invalid settings, and `LUR.Int31n` panics when the range is exhausted (kept for backward compatibility). The `PRP` APIs return errors instead.

## Benchmarks
Intel Xeon @ 2.10GHz, 4 cores, Go 1.24. Run with `go test -run '^$' -bench Compare -benchmem`.

| Benchmark | LUR | PRP |
|---|---|---|
| One number, single goroutine | 55 ns | 370 ns (`Max` just above a power of two, so ~1.7 Feistel passes); 232 ns (`Max = 2^62`, no cycle-walking) |
| One number, 4 goroutines sharing a generator | 170 ns | 101 ns (lock-free counter) |
| Memory held, `Max = 10^6` | 4.0 MB | 80 B |
| Memory held, `Max = 10^7` | 40 MB | 80 B |
| Memory held, `Max = 10^8` | 400 MB | 80 B |

## Testing
```bash
go test -race ./...
```
What is covered:
- bijection on many domain sizes (odd and even bit widths, sizes just above a power of two), and `At`/`Index` round trips up to 2^62
- key and domain separation
- a chi-square uniformity test over 20,000 keys, which fails for 1–2 Feistel rounds and passes for 10
- the official SipHash-2-4 test vector
- sharding (shards partition the sequence exactly), concurrency and resume

## Changelog
- **Redis support removed.** `CacheLUR` and its `go-redis` dependency are gone; the module now has no dependencies. With `PRP`, a distributed generator only needs a shared position counter, see [Restarts and multiple instances](#restarts-and-multiple-instances).
- **Overflow check.** `max·k` is validated in 64-bit arithmetic. Previously the int32 product could overflow and bypass the size limit.
- **New APIs:** `Permutation`, `PRP` (`NewPRP`, `NewPRPAt`, `NewPRPShard`, `NewPRPShardAt`).

## References
- R. A. Fisher, F. Yates (1938); R. Durstenfeld, *Algorithm 235: Random permutation*, CACM 1964.
- M. Luby, C. Rackoff, *How to construct pseudorandom permutations from pseudorandom functions*, SIAM J. Comput. 1988.
- J. Black, P. Rogaway, *Ciphers with arbitrary finite domains*, CT-RSA 2002 (cycle-walking).
- NIST SP 800-38G Rev. 1, *Methods for Format-Preserving Encryption* (FF1).
- J.-P. Aumasson, D. J. Bernstein, *SipHash: a fast short-input PRF*, INDOCRYPT 2012.
- Related uses: masscan's BlackRock cipher for randomized scan order; ZMap's cyclic-group address iteration.
