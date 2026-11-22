# LuRand
Large-scale unique random number generator for Go, with no dependencies. Every number in `[0, max)` is returned exactly once (or exactly `k` times), in random order, in `O(1)` time and `O(1)` space, no matter how large `max` is (up to 2^62). It is safe for concurrent use, resumable, and can be split across instances without coordination.

It works by walking a keyed pseudo-random permutation (a Feistel network with cycle-walking): the `i`-th number is computed from `i` and a secret key, so nothing has to be stored except a counter.

## Install
`go get github.com/yihaoye/lurand`

## Usage

### Basic
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

### Why exact uniform sampling cannot beat O(N)
After `m` of `N` numbers have been drawn, the generator must know which `m` are used, and there are `C(N, m)` possibilities. Any exact sampler therefore needs at least `log2 C(N, m)` bits of state, about `N` bits at `m = N/2`. Probabilistic structures such as Bloom filters do not escape this bound. They also give false positives, so some numbers would never be returned, and rejection sampling slows down as the range fills up.

The way out is to relax "uniformly random" to "computationally indistinguishable from uniformly random": that is what a pseudo-random permutation provides.

### PRP: Feistel network + cycle-walking
`PRP` returns `Permutation.At(0), At(1), At(2), ...`. `At` is a keyed bijection on `[0, n)`, so distinct positions always give distinct numbers, and the only state is the counter.

- **Feistel network.** Split `x` into two halves `(a, c)` and repeat `a, c = c, a XOR F(c)`. Each round can be undone (`c = a'`, `a = c' XOR F(a')`), so the network is a bijection no matter what `F` is. `F` only has to be a good keyed pseudo-random function; LuRand uses SipHash-2-4. By the Luby–Rackoff theorem, 4 rounds with a secure `F` give a strong pseudo-random permutation. LuRand uses 10 rounds, as NIST FF1 does. The two halves alternate between `floor(b/2)` and `ceil(b/2)` bits, so any bit width `b` works.
- **Cycle-walking.** The network permutes `[0, 2^b)` with `b = ceil(log2 n)`. If the result is `>= n`, it is encrypted again until it lands in `[0, n)` (Black & Rogaway, 2002). This always terminates, because following the permutation's cycle from `x < n` returns to `x`. The result is still a bijection on `[0, n)`. Since `2^b < 2n`, it takes fewer than 2 encryptions on average.
- **K duplicates.** The domain is `[0, Max·K)` and each result is divided by `K`.

## Limits and security notes
- **Small domains.** The output is a correct permutation for any `n`, but the Feistel security bound degrades as the domain shrinks: it is roughly `q² / 2^(b/2)` for `q` observed outputs. NIST requires at least 10^6 for FF1, and FF3 was revised after attacks on small domains. Below about 10^6, if unpredictability matters, shuffle in memory instead with a Fisher–Yates shuffle driven by `crypto/rand` (`math/rand`'s `rand.Perm` is fine when it does not): memory is small there anyway.
- **The key is the secret.** Anyone who knows the key (and `Max`, `K`) can compute the whole sequence and its inverse. Generate it with `GenerateKey`, store it like any other secret, and never change it for a running sequence.
- **Not a certified FPE.** The construction follows FF1's structure but is not FF1. If you need a standardized, audited cipher (for example for card numbers), use an FF1 implementation. The order is random but not uniform over all `N!` permutations: the key selects one of 2^128 of them.
## Benchmarks
Apple Silicon (Go amd64 under Rosetta), 8 cores. Run with `go test -run '^$' -bench . -benchmem`.

| Benchmark | Result |
|---|---|
| One number, single goroutine | ~430 ns (`Max` just above a power of two, so ~1.7 Feistel passes); ~370 ns (`Max = 2^62`, no cycle-walking) |
| One number, 8 goroutines sharing a generator | ~100 ns (lock-free counter) |
| Memory held, any `Max` (10^6, 10^12, 2^62) | 96 B |
| Allocations per number | 0 |

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

## References
- M. Luby, C. Rackoff, *How to construct pseudorandom permutations from pseudorandom functions*, SIAM J. Comput. 1988.
- J. Black, P. Rogaway, *Ciphers with arbitrary finite domains*, CT-RSA 2002 (cycle-walking).
- NIST SP 800-38G Rev. 1, *Methods for Format-Preserving Encryption* (FF1).
- J.-P. Aumasson, D. J. Bernstein, *SipHash: a fast short-input PRF*, INDOCRYPT 2012.
- Related uses: masscan's BlackRock cipher for randomized scan order; ZMap's cyclic-group address iteration.
