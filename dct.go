package main

import "encoding/binary"

// HEVC 16-point integer DCT basis matrix.
var dct16 = [16][16]int32{
	{64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64},
	{90, 87, 80, 70, 57, 43, 25, 9, -9, -25, -43, -57, -70, -80, -87, -90},
	{89, 75, 50, 18, -18, -50, -75, -89, -89, -75, -50, -18, 18, 50, 75, 89},
	{87, 57, 9, -43, -80, -90, -70, -25, 25, 70, 90, 80, 43, -9, -57, -87},
	{83, 36, -36, -83, -83, -36, 36, 83, 83, 36, -36, -83, -83, -36, 36, 83},
	{80, 9, -70, -87, -25, 57, 90, 43, -43, -90, -57, 25, 87, 70, -9, -80},
	{75, -18, -89, -50, 50, 89, 18, -75, -75, 18, 89, 50, -50, -89, -18, 75},
	{70, -43, -87, 9, 90, 25, -80, -57, 57, 80, -25, -90, -9, 87, 43, -70},
	{64, -64, -64, 64, 64, -64, -64, 64, 64, -64, -64, 64, 64, -64, -64, 64},
	{57, -80, -25, 90, -9, -87, 43, 70, -70, -43, 87, 9, -90, 25, 80, -57},
	{50, -89, 18, 75, -75, -18, 89, -50, -50, 89, -18, -75, 75, 18, -89, 50},
	{43, -90, 57, 25, -87, 70, 9, -80, 80, -9, -70, 87, -25, -57, 90, -43},
	{36, -83, 83, -36, -36, 83, -83, 36, 36, -83, 83, -36, -36, 83, -83, 36},
	{25, -70, 90, -80, 43, 9, -57, 87, -87, 57, -9, -43, 80, -90, 70, -25},
	{18, -50, 75, -89, 89, -75, 50, -18, -18, 50, -75, 89, -89, 75, -50, 18},
	{9, -25, 43, -57, 70, -80, 87, -90, 90, -87, 80, -70, 57, -43, 25, -9},
}

const (
	// HEVC shift values for 16-point transform, 8-bit input.
	// Row:  log2(16) + bitDepth - 9 = 4 + 8 - 9 = 3
	// Col:  log2(16) + 6            = 4 + 6     = 10
	dctShiftRow = 3
	dctShiftCol = 10
)

const N = 16

// forwardDCT applies a 2D 16-point integer DCT to a 16x16 block.
//
//	src: 256 bytes  — uint8 pixels, row-major
//	dst: 512 bytes  — int16 coefficients, little-endian, row-major
//
// The input is level-shifted by -128 before the transform. No quantization is applied.
func forwardDCT(dst, src []byte) {
	var tmp [N][N]int32

	// Row transform
	for y := 0; y < N; y++ {
		row := src[y*N : y*N+N]
		for k := 0; k < N; k++ {
			var sum int32
			basis := &dct16[k]
			for n := 0; n < N; n++ {
				sum += basis[n] * (int32(row[n]) - 128)
			}
			tmp[y][k] = (sum + (1 << (dctShiftRow - 1))) >> dctShiftRow
		}
	}

	// Column transform
	for x := 0; x < N; x++ {
		for k := 0; k < N; k++ {
			var sum int32
			basis := &dct16[k]
			for n := 0; n < N; n++ {
				sum += basis[n] * tmp[n][x]
			}
			coeff := (sum + (1 << (dctShiftCol - 1))) >> dctShiftCol
			binary.LittleEndian.PutUint16(dst[(k*N+x)*2:], uint16(int16(coeff)))
		}
	}
}
