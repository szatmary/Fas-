package main

import "encoding/binary"

// toBitPlanes rearranges 256 int16 coefficients (512 bytes, little-endian)
// into 16 bit planes of 32 bytes each, MSB (bit 15) first, LSB (bit 0) last.
//
// This concentrates high-order bits (mostly zeros for small DCT coefficients)
// together and low-order bits (noisy) together, producing long runs of zeros
// in the leading planes.
//
//	src: 512 bytes — 256 × int16 LE
//	dst: 512 bytes — 16 × 32-byte bit planes (plane 15 at offset 0, plane 0 at offset 480)
func toBitPlanes(dst, src []byte) {
	// Clear destination
	for i := range dst[:512] {
		dst[i] = 0
	}

	for i := 0; i < 256; i++ {
		val := binary.LittleEndian.Uint16(src[i*2:])
		byteIdx := i / 8
		bitIdx := uint(i % 8)
		for b := 0; b < 16; b++ {
			if val&(1<<uint(b)) != 0 {
				planeOffset := (15 - b) * 32 // MSB first
				dst[planeOffset+byteIdx] |= 1 << bitIdx
			}
		}
	}
}

// fromBitPlanes reverses toBitPlanes: reconstructs 256 int16 coefficients
// from 16 bit planes.
//
//	src: 512 bytes — 16 × 32-byte bit planes (MSB first)
//	dst: 512 bytes — 256 × int16 LE
func fromBitPlanes(dst, src []byte) {
	for i := range dst[:512] {
		dst[i] = 0
	}

	for b := 0; b < 16; b++ {
		planeOffset := (15 - b) * 32
		mask := uint16(1) << uint(b)
		for i := 0; i < 256; i++ {
			if src[planeOffset+i/8]&(1<<uint(i%8)) != 0 {
				val := binary.LittleEndian.Uint16(dst[i*2:])
				val |= mask
				binary.LittleEndian.PutUint16(dst[i*2:], val)
			}
		}
	}
}
