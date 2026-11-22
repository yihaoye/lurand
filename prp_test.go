package lurand

import (
	"encoding/binary"
	"errors"
	"math"
	"math/rand"
	"sync"
	"testing"
)

var testKey = []byte("0123456789abcdef-test-key")

func mustPerm(t testing.TB, n uint64, key []byte) *Permutation {
	t.Helper()
	p, err := NewPermutation(n, key)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSipHashVector(t *testing.T) {
	// Official SipHash-2-4 test vector: key = 00..0f, message = 00..0f (16 bytes).
	var key, msg [16]byte
	for i := range key {
		key[i], msg[i] = byte(i), byte(i)
	}
	got := sipHash24(
		binary.LittleEndian.Uint64(key[:8]), binary.LittleEndian.Uint64(key[8:]),
		binary.LittleEndian.Uint64(msg[:8]), binary.LittleEndian.Uint64(msg[8:]),
	)
	if want := uint64(0x3f2acc7f57c29bdb); got != want {
		t.Fatalf("siphash = %016x, want %016x", got, want)
	}
}

func TestPermutationIsBijection(t *testing.T) {
	// powers of two, odd/even bit widths, and sizes just above a power of two (worst case for cycle-walking)
	sizes := []uint64{1, 2, 3, 4, 5, 7, 8, 9, 10, 100, 255, 256, 257, 1000, 4095, 65536, 65537, 1_000_003}
	for _, n := range sizes {
		p := mustPerm(t, n, testKey)
		seen := make([]bool, n)
		for i := uint64(0); i < n; i++ {
			x := p.At(i)
			if x >= n {
				t.Fatalf("n=%d: At(%d)=%d out of range", n, i, x)
			}
			if seen[x] {
				t.Fatalf("n=%d: At(%d)=%d repeated", n, i, x)
			}
			seen[x] = true
			if j := p.Index(x); j != i {
				t.Fatalf("n=%d: Index(At(%d)) = %d", n, i, j)
			}
		}
	}
}

func TestPermutationLargeDomain(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, n := range []uint64{1<<40 + 12345, 1<<61 + 1, MaxPermutationSize} {
		p := mustPerm(t, n, testKey)
		for j := 0; j < 10_000; j++ {
			i := r.Uint64() % n
			x := p.At(i)
			if x >= n || p.Index(x) != i {
				t.Fatalf("n=%d: round trip failed for %d", n, i)
			}
		}
	}
}

func TestPermutationDeterministicAndKeyed(t *testing.T) {
	const n = 1_000_000
	a := mustPerm(t, n, testKey)
	b := mustPerm(t, n, testKey)
	c := mustPerm(t, n, []byte("another-16-byte-key!"))
	d := mustPerm(t, n+1, testKey)
	sameAB, sameAC, sameAD := 0, 0, 0
	for i := uint64(0); i < 1000; i++ {
		if a.At(i) == b.At(i) {
			sameAB++
		}
		if a.At(i) == c.At(i) {
			sameAC++
		}
		if a.At(i) == d.At(i) {
			sameAD++
		}
	}
	if sameAB != 1000 {
		t.Errorf("same key must give same permutation, %d/1000 equal", sameAB)
	}
	// unrelated permutations agree at a position with probability ~1/n
	if sameAC > 2 || sameAD > 2 {
		t.Errorf("different key or size should give unrelated permutations: %d, %d equal", sameAC, sameAD)
	}
}

func TestPermutationInvalid(t *testing.T) {
	if _, err := NewPermutation(0, testKey); err == nil {
		t.Error("n=0 accepted")
	}
	if _, err := NewPermutation(MaxPermutationSize+1, testKey); err == nil {
		t.Error("n>2^62 accepted")
	}
	if _, err := NewPermutation(10, []byte("short")); err == nil {
		t.Error("short key accepted")
	}
	p := mustPerm(t, 10, testKey)
	for name, f := range map[string]func(){"At": func() { p.At(10) }, "Index": func() { p.Index(10) }} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s(n) did not panic", name)
				}
			}()
			f()
		}()
	}
}

// TestPermutationUniformity checks, over many random keys, that every position maps to
// every value equally often (chi-square test on the n×n count table).
func TestPermutationUniformity(t *testing.T) {
	for _, n := range []uint64{8, 10} { // 10 exercises cycle-walking
		const keys = 20_000
		counts := make([][]float64, n)
		for i := range counts {
			counts[i] = make([]float64, n)
		}
		r := rand.New(rand.NewSource(42))
		key := make([]byte, 16)
		for j := 0; j < keys; j++ {
			r.Read(key)
			p := mustPerm(t, n, key)
			for i := uint64(0); i < n; i++ {
				counts[i][p.At(i)]++
			}
		}
		expected := float64(keys) / float64(n)
		chi2 := 0.0
		for i := range counts {
			for _, c := range counts[i] {
				chi2 += (c - expected) * (c - expected) / expected
			}
		}
		df := float64((n - 1) * (n - 1))
		if crit := chi2Critical(df, 3.29); chi2 > crit { // p ≈ 0.0005
			t.Errorf("n=%d: chi2=%.1f exceeds critical %.1f (df=%.0f)", n, chi2, crit, df)
		}
	}
}

// chi2Critical approximates the chi-square quantile for df degrees of freedom at
// standard normal quantile z (Wilson–Hilferty).
func chi2Critical(df, z float64) float64 {
	a := 2 / (9 * df)
	return df * math.Pow(1-a+z*math.Sqrt(a), 3)
}

func TestPRP(t *testing.T) {
	t.Run("exhaust with k duplicates", func(t *testing.T) {
		g, err := NewPRP(PRPConfig{Max: 30_000, K: 3, Key: testKey})
		if err != nil {
			t.Fatal(err)
		}
		counts := make([]int, 30_000)
		for i := 0; i < 90_000; i++ {
			n, err := g.Next()
			if err != nil {
				t.Fatalf("call %d: %v", i, err)
			}
			counts[n]++
		}
		for n, c := range counts {
			if c != 3 {
				t.Fatalf("num %d returned %d times, want 3", n, c)
			}
		}
		if _, err := g.Next(); !errors.Is(err, ErrExhausted) {
			t.Fatalf("expected ErrExhausted, got %v", err)
		}
		if g.Position() != 90_000 || g.Remaining() != 0 {
			t.Fatalf("position %d remaining %d", g.Position(), g.Remaining())
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		const max = 200_000
		g, _ := NewPRP(PRPConfig{Max: max, Key: testKey})
		var seen sync.Map
		var wg sync.WaitGroup
		for w := 0; w < 50; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					n, err := g.Next()
					if err != nil {
						return
					}
					if _, dup := seen.LoadOrStore(n, true); dup {
						t.Errorf("duplicate %d", n)
						return
					}
				}
			}()
		}
		wg.Wait()
		count := 0
		seen.Range(func(_, _ interface{}) bool { count++; return true })
		if count != max {
			t.Fatalf("got %d unique numbers, want %d", count, max)
		}
	})

	t.Run("resume from position", func(t *testing.T) {
		cfg := PRPConfig{Max: 1000, Key: testKey}
		ref, _ := NewPRP(cfg)
		var want []uint64
		for i := 0; i < 1000; i++ {
			n, _ := ref.Next()
			want = append(want, n)
		}
		first, _ := NewPRP(cfg)
		for i := 0; i < 400; i++ {
			first.Next()
		}
		resumed, err := NewPRPAt(cfg, first.Position())
		if err != nil {
			t.Fatal(err)
		}
		for i := 400; i < 1000; i++ {
			if n, _ := resumed.Next(); n != want[i] {
				t.Fatalf("position %d: got %d, want %d", i, n, want[i])
			}
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		bad := []PRPConfig{
			{Max: 0, Key: testKey},
			{Max: 10, Key: []byte("short")},
			{Max: 1 << 40, K: 1 << 30, Key: testKey}, // above 2^62
			{Max: 1 << 63, K: 1 << 2, Key: testKey},  // overflows uint64
		}
		for i, cfg := range bad {
			if _, err := NewPRP(cfg); err == nil {
				t.Errorf("config %d accepted", i)
			}
		}
		if _, err := NewPRPAt(PRPConfig{Max: 10, Key: testKey}, 11); err == nil {
			t.Error("position beyond total accepted")
		}
	})
}
