// Copyright 2019 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

#include "textflag.h"

// ·compareCoverBodySSE2 compares every corresponding byte of base and cur, and
// reports whether cur has any entries bigger than base.
// func ·compareCoverBodySSE2(base, cur *byte) bool
TEXT ·compareCoverBodySSE2(SB), NOSPLIT, $0-17
	MOVQ	base+0(FP), SI
	MOVQ	cur+8(FP), DI
	XORQ	CX, CX	// loop counter
	XORQ	R10, R10	// ret

	// Fill X0 with 128.
	MOVL	$128, AX
	MOVD	AX, X0
	PUNPCKLBW X0, X0
	PUNPCKLBW X0, X0
	PSHUFL $0, X0, X0

	// Align loop.
	// There is not enough control to align to 16 bytes;
	// the function itself might be at any 2 byte offset.
	// But we can at least get the loop offset to be even.
	BYTE    $0x90
loop:
	MOVOU	(SI)(CX*1), X1
	MOVOU	(DI)(CX*1), X2
	// Add -128 to each byte.
	// This lets us use signed comparison below to implement unsigned comparison.
	PSUBB	X0, X1
	PSUBB	X0, X2
	// Compare each byte
	PCMPGTB	X1, X2 // X2 > X1
	// Extract top bit of each elem.
	PMOVMSKB X2, AX
	// If any bits were set, then some elem of X2 (cur) was bigger than some elem of X1 (base).
	TESTL	AX, AX
	JNZ	yes
	LEAQ	16(CX), CX	// CX += 16
	BTL	$16, CX	// have we reached 65536 (CoverSize)?
	JCS	ret
	JMP	loop
yes:
	MOVQ	$1, R10
ret:
	MOVB	R10, ret+16(FP)
	RET

// compareCoverBodyAVX2 compares every corresponding byte of base and cur, and
// reports whether cur has any entries bigger than base.
// func ·compareCoverBodyAVX2(base, cur *byte) bool
TEXT ·compareCoverBodyAVX2(SB), NOSPLIT, $0-17
	MOVQ	base+0(FP), SI
	MOVQ	cur+8(FP), DI
	XORQ	CX, CX	// loop counter
	XORQ	R10, R10	// ret
	MOVL	$128, AX
	MOVD	AX, X0
	VPBROADCASTB	X0, Y0

	// align loop.
	// See comment in compareCoverBodySSE2.
	BYTE    $0x90
loop:
	VMOVDQU	(SI)(CX*1), Y1
	VMOVDQU	(DI)(CX*1), Y2
	VPSUBB	Y0, Y1, Y3
	VPSUBB	Y0, Y2, Y4
	VPCMPGTB	Y3, Y4, Y5
	// Extract top bit of each elem.
	VPMOVMSKB Y5, AX
	// If any bits were set, then some elem of X2 (cur) was bigger than some elem of X1 (base).
	TESTL	AX, AX
	JNZ	yes
	LEAQ	32(CX), CX
	BTL	$16, CX	// have we reached 65536 (CoverSize)?
	JCS	ret
	JMP	loop
yes:
	MOVQ	$1, R10
ret:
	VZEROUPPER
	MOVB	R10, ret+16(FP)
	RET
