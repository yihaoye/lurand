package lurand

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/bits"
	"sync/atomic"
)

const (
	// MaxPermutationSize is the largest domain a Permutation supports (2^62).
	MaxPermutationSize = 1 << 62

	// MinKeySize is the minimum secret length accepted by NewPermutation.
	MinKeySize = 16

	// prpRounds is the number of Feistel rounds. Luby–Rackoff needs 4 for a strong PRP
	// with an ideal round function; 10 matches NIST FF1 and leaves a security margin.
	prpRounds = 10
)

// Compile-time check that prpRounds is even: the alternating half widths only return to
// (u, v) after an even number of rounds, otherwise encrypt would not be a permutation of [0, 2^b).
var _ = [1]struct{}{}[prpRounds%2]

// ErrExhausted is returned once every number of the domain has been handed out.
var ErrExhausted = errors.New("No more numbers available")

// Permutation is a keyed pseudo-random permutation (PRP) over [0, n).
//
// It is a 10-round alternating Feistel network on b = ceil(log2 n) bits with SipHash-2-4
// as round function, restricted to [0, n) by cycle-walking (Black & Rogaway, 2002):
// if the output falls outside [0, n), it is encrypted again until it lands inside.
// Because a Feistel network is a bijection for any round function, and cycle-walking
// a bijection keeps it one, At is a bijection on [0, n) for every key.
//
// Without the key, the order is computationally indistinguishable from a random
// permutation, as long as n is not tiny (see README: below about 10^6 the Feistel
// security bound gets weak and an in-memory shuffle such as LUR is the better choice).
//
// A Permutation holds no state besides its key: O(1) space for any n. It is safe for
// concurrent use.
type Permutation struct {
	n      uint64
	u, v   uint   // bit widths of the two Feistel halves, u = b/2, v = b-u
	k0, k1 uint64 // SipHash key derived from the secret
}

// NewPermutation creates the permutation of [0, n) selected by key. The same (n, key)
// always yields the same permutation. key must be at least MinKeySize bytes of secret,
// high-entropy data, e.g. from GenerateKey.
func NewPermutation(n uint64, key []byte) (*Permutation, error) {
	if n == 0 || n > MaxPermutationSize {
		return nil, errors.New("lurand: permutation size must be in [1, 2^62]")
	}
	if len(key) < MinKeySize {
		return nil, errors.New("lurand: key must be at least 16 bytes")
	}
	sum := sha256.Sum256(key)
	b := uint(bits.Len64(n - 1)) // smallest b with 2^b >= n, so 2^b < 2n: cycle-walking takes < 2 steps on average
	return &Permutation{
		n:  n,
		u:  b / 2,
		v:  b - b/2,
		k0: binary.LittleEndian.Uint64(sum[:8]),
		k1: binary.LittleEndian.Uint64(sum[8:16]),
	}, nil
}

// GenerateKey returns a random 32-byte key from crypto/rand.
func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// Size returns n, the size of the domain [0, n).
func (p *Permutation) Size() uint64 { return p.n }

// At returns the element at position i of the permutation. It panics if i >= n.
func (p *Permutation) At(i uint64) uint64 {
	if i >= p.n {
		panic("lurand: permutation index out of range")
	}
	x := p.encrypt(i)
	for x >= p.n {
		x = p.encrypt(x)
	}
	return x
}

// Index is the inverse of At: it returns the position i such that At(i) == x.
// It panics if x >= n. Useful to check whether a value was already handed out
// (Index(x) < number of values issued so far).
func (p *Permutation) Index(x uint64) uint64 {
	if x >= p.n {
		panic("lurand: permutation value out of range")
	}
	i := p.decrypt(x)
	for i >= p.n {
		i = p.decrypt(i)
	}
	return i
}

// roundFunc is the Feistel round function F(round, half). The domain size is mixed in
// so that the same key gives unrelated permutations for different n.
func (p *Permutation) roundFunc(round int, half uint64) uint64 {
	return sipHash24(p.k0, p.k1, p.n, half|uint64(round)<<56)
}

// width returns the bit width of the half that is replaced in the given round.
// Halves alternate between u and v bits (as in NIST FF1), which lets b be odd.
func (p *Permutation) width(round int) uint {
	if round%2 == 0 {
		return p.u
	}
	return p.v
}

// encrypt applies the Feistel network on [0, 2^b). State (a, c) starts with a = high u bits
// and c = low v bits; each round: a, c = c, (a XOR F(c)) mod 2^width. After an even number
// of rounds the widths are back to (u, v).
func (p *Permutation) encrypt(x uint64) uint64 {
	a, c := x>>p.v, x&mask(p.v)
	for r := 0; r < prpRounds; r++ {
		a, c = c, (a^p.roundFunc(r, c))&mask(p.width(r))
	}
	return a<<p.v | c
}

// decrypt undoes encrypt round by round: given (a', c') = (c, (a XOR F(c)) mod 2^w),
// recover c = a' and a = (c' XOR F(a')) mod 2^w.
func (p *Permutation) decrypt(y uint64) uint64 {
	a, c := y>>p.v, y&mask(p.v)
	for r := prpRounds - 1; r >= 0; r-- {
		a, c = (c^p.roundFunc(r, a))&mask(p.width(r)), a
	}
	return a<<p.v | c
}

func mask(w uint) uint64 { return 1<<w - 1 }

// PRPConfig configures a PRP generator.
type PRPConfig struct {
	// Max: numbers are drawn from [0, Max).
	Max uint64
	// K: each number is returned exactly K times (0 means 1). Max*K must not exceed MaxPermutationSize.
	K uint64
	// Key: secret selecting the order, at least MinKeySize bytes. Keep it stable for the lifetime
	// of the sequence and keep it secret: whoever has it can compute every number.
	Key []byte
}

func (c PRPConfig) build() (perm *Permutation, k, total uint64, err error) {
	k = c.K
	if k == 0 {
		k = 1
	}
	if c.Max == 0 {
		return nil, 0, 0, errors.New("lurand: Max must be > 0")
	}
	hi, total := bits.Mul64(c.Max, k)
	if hi != 0 || total > MaxPermutationSize {
		return nil, 0, 0, errors.New("lurand: Max*K exceeds 2^62")
	}
	perm, err = NewPermutation(total, c.Key)
	return perm, k, total, err
}

// PRP generates unique random numbers in [0, Max) (each exactly K times) in O(1) time
// and O(1) space, independent of Max. Its only state is a counter: the i-th call returns
// Permutation.At(i) / K. It is safe for concurrent use.
type PRP struct {
	next  uint64 // first field: 64-bit aligned for atomic access on 32-bit platforms
	perm  *Permutation
	k     uint64
	total uint64
}

// NewPRP creates a generator starting at position 0.
func NewPRP(cfg PRPConfig) (*PRP, error) {
	return NewPRPAt(cfg, 0)
}

// NewPRPAt creates a generator that resumes at position pos (e.g. a value saved from
// Position before a restart). With the same cfg it continues the same sequence.
func NewPRPAt(cfg PRPConfig, pos uint64) (*PRP, error) {
	perm, k, total, err := cfg.build()
	if err != nil {
		return nil, err
	}
	if pos > total {
		return nil, errors.New("lurand: position out of range")
	}
	return &PRP{next: pos, perm: perm, k: k, total: total}, nil
}

// Next returns the next number, or ErrExhausted once all Max*K values have been returned.
func (g *PRP) Next() (uint64, error) {
	i := atomic.AddUint64(&g.next, 1) - 1
	if i >= g.total {
		atomic.StoreUint64(&g.next, g.total) // keep the counter from creeping towards overflow
		return 0, ErrExhausted
	}
	return g.perm.At(i) / g.k, nil
}

// Position returns how many values have been handed out. Persist it to resume with NewPRPAt.
func (g *PRP) Position() uint64 {
	if pos := atomic.LoadUint64(&g.next); pos < g.total {
		return pos
	}
	return g.total
}

// Remaining returns how many values can still be handed out.
func (g *PRP) Remaining() uint64 { return g.total - g.Position() }
