// Copyright 2026 Certen Protocol
//
// Keccak-256 (the pre-NIST padding Ethereum uses for block hashes), so that the service can check that a settlement header it is
// about to carry in the `finality` member hashes to the block hash of the leg. It is not evidence and verifies nothing for a reader:
// a verifier re-checks keccak256(header) == blockHash itself. The module has no dependency that provides it.

package server

import "encoding/binary"

var keccakRC = [24]uint64{
	0x0000000000000001, 0x0000000000008082, 0x800000000000808a, 0x8000000080008000, 0x000000000000808b, 0x0000000080000001,
	0x8000000080008081, 0x8000000000008009, 0x000000000000008a, 0x0000000000000088, 0x0000000080008009, 0x000000008000000a,
	0x000000008000808b, 0x800000000000008b, 0x8000000000008089, 0x8000000000008003, 0x8000000000008002, 0x8000000000000080,
	0x000000000000800a, 0x800000008000000a, 0x8000000080008081, 0x8000000000008080, 0x0000000080000001, 0x8000000080008008,
}

var keccakRot = [24]uint{1, 3, 6, 10, 15, 21, 28, 36, 45, 55, 2, 14, 27, 41, 56, 8, 25, 43, 62, 18, 39, 61, 20, 44}
var keccakPi = [24]int{10, 7, 11, 17, 18, 3, 5, 16, 8, 21, 24, 4, 15, 23, 19, 13, 12, 2, 20, 14, 22, 9, 6, 1}

func keccakF1600(a *[25]uint64) {
	var bc [5]uint64
	for round := 0; round < 24; round++ {
		for i := 0; i < 5; i++ {
			bc[i] = a[i] ^ a[i+5] ^ a[i+10] ^ a[i+15] ^ a[i+20]
		}
		for i := 0; i < 5; i++ {
			t := bc[(i+4)%5] ^ (bc[(i+1)%5]<<1 | bc[(i+1)%5]>>63)
			for j := 0; j < 25; j += 5 {
				a[j+i] ^= t
			}
		}
		t := a[1]
		for i := 0; i < 24; i++ {
			j := keccakPi[i]
			b := a[j]
			a[j] = t<<keccakRot[i] | t>>(64-keccakRot[i])
			t = b
		}
		for j := 0; j < 25; j += 5 {
			for i := 0; i < 5; i++ {
				bc[i] = a[j+i]
			}
			for i := 0; i < 5; i++ {
				a[j+i] ^= ^bc[(i+1)%5] & bc[(i+2)%5]
			}
		}
		a[0] ^= keccakRC[round]
	}
}

// keccak256 is Keccak-256 with the original 0x01 domain padding (not SHA3-256).
func keccak256(data []byte) [32]byte {
	const rate = 136
	var a [25]uint64
	absorb := func(block []byte) {
		for i := 0; i < rate/8; i++ {
			a[i] ^= binary.LittleEndian.Uint64(block[8*i:])
		}
		keccakF1600(&a)
	}
	for len(data) >= rate {
		absorb(data[:rate])
		data = data[rate:]
	}
	last := make([]byte, rate)
	copy(last, data)
	last[len(data)] ^= 0x01
	last[rate-1] ^= 0x80
	absorb(last)
	var out [32]byte
	for i := 0; i < 4; i++ {
		binary.LittleEndian.PutUint64(out[8*i:], a[i])
	}
	return out
}
