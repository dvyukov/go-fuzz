#include "textflag.h"

// func compareCoverBody(base, cur *byte) (bool, bool)
TEXT ·compareCoverBody(SB), NOSPLIT, $0-17
	MOVQ	base+0(FP), SI
	MOVQ	cur+8(FP), DI
	MOVQ	$65535, AX	// loop counter
	MOVQ	$0, R10		// newCover
	BYTE	$0x90		// nop
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
	JB	new_cover
	JMP	continue
new_cover:
	MOVB	$1, R10
done:
	MOVB	R10, ret+16(FP)
	RET



