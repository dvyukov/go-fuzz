#include "textflag.h"

// func compareCoverBody(base, cur *byte) (bool, bool)
TEXT ·compareCoverBody(SB), NOSPLIT, $0-18
	MOVQ	base+0(FP), SI
	MOVQ	cur+8(FP), DI
	MOVQ	$65535, AX   // loop counter
	MOVQ	$0, R10  // newCounter
	MOVQ	$0, R11  // newCover
	MOVQ	$1, R12  // const 1
	BYTE	$0x90	 // nop
	BYTE	$0x90
loop:
	MOVBQZX 0(DI)(AX*1), R9
	TESTB	R9, R9
	JNZ	non_zero
continue:
	SUBQ	$1, AX
	JNS	loop
	JMP	done
non_zero:
	MOVBQZX 0(SI)(AX*1), R8
	CMPB	R8, R9
	JB	interesting
	JMP	continue
interesting:
	TESTB	R8, R8
	JZ	new_cover
	MOVQ	R12, R10
	JMP	continue
new_cover:
	MOVB	$1, R10
	MOVB	$1, R11
done:
	MOVB	R11, newCover+16(FP)
	MOVB	R10, newCounter+17(FP)
	RET



