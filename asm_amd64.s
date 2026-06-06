//go:build !go1.26

#include "go_asm.h"
#include "go_tls.h"
#include "textflag.h"

TEXT ·GetG(SB), NOSPLIT, $0-8
	get_tls(CX)
	MOVQ	g(CX), AX
	MOVQ	AX, ret+0(FP)
	RET
