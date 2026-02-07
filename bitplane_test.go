package main

import (
	"encoding/binary"
	"testing"
)

func TestBitPlaneRoundTrip(t *testing.T) {
	src := make([]byte, 512)
	// Fill with known int16 values
	for i := 0; i < 256; i++ {
		binary.LittleEndian.PutUint16(src[i*2:], uint16(int16(i*7-900)))
	}

	bp := make([]byte, 512)
	toBitPlanes(bp, src)

	dst := make([]byte, 512)
	fromBitPlanes(dst, bp)

	for i := 0; i < 512; i++ {
		if src[i] != dst[i] {
			t.Fatalf("mismatch at byte %d: got %d, want %d", i, dst[i], src[i])
		}
	}
}

func TestBitPlaneZeros(t *testing.T) {
	src := make([]byte, 512) // all zeros
	bp := make([]byte, 512)
	toBitPlanes(bp, src)

	// All bit planes should be zero
	for i := 0; i < 512; i++ {
		if bp[i] != 0 {
			t.Fatalf("expected all zeros in bit planes, got %d at byte %d", bp[i], i)
		}
	}
}

func TestBitPlaneMSBFirst(t *testing.T) {
	src := make([]byte, 512)
	// Set coefficient 0 to value 1 (only LSB set)
	binary.LittleEndian.PutUint16(src[0:2], 1)

	bp := make([]byte, 512)
	toBitPlanes(bp, src)

	// Bit 0 (LSB) should be in the last plane (offset 15*32 = 480)
	if bp[480] != 1 {
		t.Fatalf("LSB plane: expected 1 at offset 480, got %d", bp[480])
	}
	// MSB plane (offset 0) should be empty
	for i := 0; i < 32; i++ {
		if bp[i] != 0 {
			t.Fatalf("MSB plane should be zero, got %d at byte %d", bp[i], i)
		}
	}
}
