package asm

import "unsafe"

//go:nosplit
//go:noinline
func FastMemcpy(dst, src unsafe.Pointer, n uintptr) {
	// Custom assembly implementation for x86_64
	if n < 32 {
		// Use simple loop for small copies
		fastMemcpySmall(dst, src, n)
		return
	}

	// Use SIMD instructions for larger copies
	fastMemcpyLarge(dst, src, n)
}

//go:noescape
func fastMemcpySmall(dst, src unsafe.Pointer, n uintptr)

//go:noescape
func fastMemcpyLarge(dst, src unsafe.Pointer, n uintptr)

// Assembly implementations in separate .s file:
/*
// fastcopy_amd64.s

#include "textflag.h"

// func fastMemcpySmall(dst, src unsafe.Pointer, n uintptr)
TEXT ·fastMemcpySmall(SB), NOSPLIT, $0-24
    MOVQ dst+0(FP), DI
    MOVQ src+8(FP), SI
    MOVQ n+16(FP), CX
    REP; MOVSB
    RET

// func fastMemcpyLarge(dst, src unsafe.Pointer, n uintptr)
TEXT ·fastMemcpyLarge(SB), NOSPLIT, $0-24
    MOVQ dst+0(FP), DI
    MOVQ src+8(FP), SI
    MOVQ n+16(FP), CX

    // Align to 32-byte boundary
    MOVQ DI, AX
    ANDQ $31, AX
    JZ aligned
    MOVQ $32, DX
    SUBQ AX, DX
    SUBQ DX, CX
    REP; MOVSB

aligned:
    // Use AVX2 for bulk copy
    SHRQ $5, CX // Divide by 32
    JZ remainder

avx_loop:
    VMOVDQU (SI), Y0
    VMOVDQU Y0, (DI)
    ADDQ $32, SI
    ADDQ $32, DI
    LOOP avx_loop

remainder:
    MOVQ n+16(FP), CX
    ANDQ $31, CX
    REP; MOVSB
    RET
*/
