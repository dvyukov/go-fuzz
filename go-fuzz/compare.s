#include "textflag.h"

// func compareCoverBody(base, cur *byte) (bool, bool)
TEXT ·compareCoverBody(SB), NOSPLIT, $0-18
	MOVQ	base+0(FP), SI
	MOVQ	cur+8(FP), DI
	MOVQ	$0, AX   // loop counter
	MOVQ	$0, R10  // newCounter
	MOVQ	$0, R11  // newCover
	MOVQ	$1, R12  // const 1
	BYTE	$0x90	 // nop
	BYTE	$0x90
	BYTE	$0x90
	BYTE	$0x90
	BYTE	$0x90
	BYTE	$0x90
	BYTE	$0x90
loop:
	MOVBQZX 0(SI)(AX*1), R8
	MOVBQZX 0(DI)(AX*1), R9
	MOVB	R8, R13
	ORB	R9, R13
	JNZ	non_zero
	ADDQ	$1, AX
	// CMPQ	$65536, AX
	BYTE	$0x48; BYTE $0x3d; BYTE $0x00; BYTE $0x00; BYTE $0x01; BYTE $0x00
	JNE	loop
	JMP	done
non_zero:
	TESTB	R8, R8
	JZ	new_cover
	CMPB	R8, R9
	// CMOVQB	R12, R10
	BYTE	$0x4d; BYTE $0x0f; BYTE $0x42; BYTE $0xd4
	ADDQ	$1, AX
	// CMPQ	$65536, AX
	BYTE	$0x48; BYTE $0x3d; BYTE $0x00; BYTE $0x00; BYTE $0x01; BYTE $0x00
	JNE	loop
	JMP	done
new_cover:
	MOVB	$1, R10
	MOVB	$1, R11
done:
	MOVB	R11, newCover+16(FP)
	MOVB	R10, newCounter+17(FP)
	RET
