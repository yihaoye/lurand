package lurand

import "math/bits"

// sipHash24 computes SipHash-2-4 (Aumasson & Bernstein, 2012) with key (k0, k1) over the
// 16-byte message whose little-endian encoding is m0 || m1. It is specialised to that fixed
// length so the Feistel round function needs no byte buffers or allocations.
func sipHash24(k0, k1, m0, m1 uint64) uint64 {
	v0 := k0 ^ 0x736f6d6570736575
	v1 := k1 ^ 0x646f72616e646f6d
	v2 := k0 ^ 0x6c7967656e657261
	v3 := k1 ^ 0x7465646279746573

	round := func() {
		v0 += v1
		v1 = bits.RotateLeft64(v1, 13)
		v1 ^= v0
		v0 = bits.RotateLeft64(v0, 32)
		v2 += v3
		v3 = bits.RotateLeft64(v3, 16)
		v3 ^= v2
		v0 += v3
		v3 = bits.RotateLeft64(v3, 21)
		v3 ^= v0
		v2 += v1
		v1 = bits.RotateLeft64(v1, 17)
		v1 ^= v2
		v2 = bits.RotateLeft64(v2, 32)
	}
	compress := func(m uint64) {
		v3 ^= m
		round()
		round()
		v0 ^= m
	}

	compress(m0)
	compress(m1)
	compress(16 << 56) // final block: message length (16) in the top byte, no tail bytes

	v2 ^= 0xff
	round()
	round()
	round()
	round()
	return v0 ^ v1 ^ v2 ^ v3
}
