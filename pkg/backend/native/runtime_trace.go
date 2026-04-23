package native

const nativeTraceBillion = 1_000_000_000

func (s *EmitState) emitRuntimeClockNowNS() {
	s.addRuntimeFunc("__rt_clock_now_ns", 0)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	emitSubRspImm8(&s.Code, 16)

	emitMovRegImm32(&s.Code, RDI, 1)
	emitLeaRegMemRbp(&s.Code, RSI, -16)
	emitMovRegImm32(&s.Code, RAX, 228)
	emitSyscall(&s.Code)

	emitMovRegMemBaseDisp(&s.Code, RAX, RBP, -16)
	emitMovRegImm32(&s.Code, RDX, nativeTraceBillion)
	emitImulRegReg(&s.Code, RAX, RDX)
	emitMovRegMemBaseDisp(&s.Code, RDX, RBP, -8)
	emitAddRegReg(&s.Code, RAX, RDX)

	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

func (s *EmitState) emitRuntimeTraceEnter() {
	s.addRuntimeFunc("__rt_trace_enter", 1)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)

	enterPrefix := s.addStringLiteral("bak.trace event=enter fn=")
	enterDepth := s.addStringLiteral(" depth=")
	enterTail := s.addStringLiteral(" thread=0\n")

	emitMovRegReg(&s.Code, R12, RDI)
	s.emitDataAddr(s.ensureTraceDepthData())
	emitMovRegReg(&s.Code, R13, RAX)
	emitMovRegMem(&s.Code, RAX, R13)

	s.emitDataAddr(enterPrefix)
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_print_str",
		},
	)

	emitMovRegReg(&s.Code, RDI, R12)
	callSite = emitCallRel32(&s.Code, 0)
	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_print_str",
		},
	)

	s.emitDataAddr(enterDepth)
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite = emitCallRel32(&s.Code, 0)
	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_print_str",
		},
	)

	emitMovRegMem(&s.Code, R14, R13)
	emitMovRegReg(&s.Code, RDI, R14)
	callSite = emitCallRel32(&s.Code, 0)
	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_print_int",
		},
	)

	s.emitDataAddr(enterTail)
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite = emitCallRel32(&s.Code, 0)
	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_print_str",
		},
	)

	emitMovRegMem(&s.Code, RAX, R13)
	emitAddRegImm32(&s.Code, RAX, 1)
	emitMovMemReg(&s.Code, R13, RAX)

	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)
	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}

func (s *EmitState) emitRuntimeTraceExit() {
	s.addRuntimeFunc("__rt_trace_exit", 3)
	emitPushReg(&s.Code, RBP)
	emitMovRegReg(&s.Code, RBP, RSP)
	emitPushReg(&s.Code, R12)
	emitPushReg(&s.Code, R13)
	emitPushReg(&s.Code, R14)
	emitPushReg(&s.Code, R15)

	exitPrefix := s.addStringLiteral("bak.trace event=exit fn=")
	exitDepth := s.addStringLiteral(" depth=")
	exitStatus := s.addStringLiteral(" thread=0 status=")
	exitDuration := s.addStringLiteral(" duration_ns=")
	exitNewline := s.addStringLiteral("\n")

	emitMovRegReg(&s.Code, R12, RDI)
	emitMovRegReg(&s.Code, R13, RSI)
	emitMovRegReg(&s.Code, R14, RDX)

	s.emitDataAddr(s.ensureTraceDepthData())
	emitMovRegReg(&s.Code, RDI, RAX)
	emitMovRegMem(&s.Code, RAX, RDI)
	emitSubRegImm32(&s.Code, RAX, 1)
	emitMovMemReg(&s.Code, RDI, RAX)
	emitMovRegReg(&s.Code, R15, RAX)

	s.emitDataAddr(exitPrefix)
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_print_str",
		},
	)

	emitMovRegReg(&s.Code, RDI, R12)
	callSite = emitCallRel32(&s.Code, 0)
	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_print_str",
		},
	)

	s.emitDataAddr(exitDepth)
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite = emitCallRel32(&s.Code, 0)
	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_print_str",
		},
	)

	emitMovRegReg(&s.Code, RDI, R15)
	callSite = emitCallRel32(&s.Code, 0)
	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_print_int",
		},
	)

	s.emitDataAddr(exitStatus)
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite = emitCallRel32(&s.Code, 0)
	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_print_str",
		},
	)

	emitMovRegReg(&s.Code, RDI, R13)
	callSite = emitCallRel32(&s.Code, 0)
	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_print_str",
		},
	)

	s.emitDataAddr(exitDuration)
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite = emitCallRel32(&s.Code, 0)
	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_print_str",
		},
	)

	emitMovRegReg(&s.Code, RDI, R14)
	callSite = emitCallRel32(&s.Code, 0)
	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_print_int",
		},
	)

	s.emitDataAddr(exitNewline)
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite = emitCallRel32(&s.Code, 0)
	s.CallPatches = append(
		s.CallPatches,
		CallPatch{
			ImmOffset: callSite,
			Target:    "__rt_print_str",
		},
	)

	emitPopReg(&s.Code, R15)
	emitPopReg(&s.Code, R14)
	emitPopReg(&s.Code, R13)
	emitPopReg(&s.Code, R12)
	emitMovRegReg(&s.Code, RSP, RBP)
	emitPopReg(&s.Code, RBP)
	emitRet(&s.Code)
}
