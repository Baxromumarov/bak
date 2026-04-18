package native

import "github.com/baxromumarov/bak/pkg/ast"

func (s *EmitState) traceFunctionName(fd *ast.FunctionDecl) string {
	if fd == nil || fd.Name == nil {
		return s.CurrentFunc
	}
	name := fd.Name.Value
	if s.CurrentModule != "" && s.CurrentModule != "main" && !stringsContainsDot(name) {
		return s.CurrentModule + "." + name
	}
	return name
}

func stringsContainsDot(name string) bool {
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			return true
		}
	}
	return false
}

func (s *EmitState) ensureTraceDepthData() int {
	if s.TraceDepthDataIndex < 0 {
		s.TraceDepthDataIndex = s.addDataItem(make([]byte, 8), 8)
	}
	return s.TraceDepthDataIndex
}

func (s *EmitState) emitTraceEnter() {
	if !s.CurrentFunctionTraced {
		return
	}
	fnIdx := s.addStringLiteral(s.CurrentFunctionTraceName)
	s.emitDataAddr(fnIdx)
	emitMovRegReg(&s.Code, RDI, RAX)
	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{
		ImmOffset: callSite,
		Target:    "__rt_trace_enter",
	})
}

func (s *EmitState) emitTraceExit(status string) {
	if !s.CurrentFunctionTraced {
		return
	}
	traceStartOffset, ok := s.resolveLocal("__trace_start")
	if !ok {
		return
	}
	fnIdx := s.addStringLiteral(s.CurrentFunctionTraceName)
	statusIdx := s.addStringLiteral(status)

	emitPushReg(&s.Code, RAX)
	emitPushReg(&s.Code, RDX)

	callSite := emitCallRel32(&s.Code, 0)
	s.CallPatches = append(s.CallPatches, CallPatch{
		ImmOffset: callSite,
		Target:    "__rt_clock_now_ns",
	})

	emitMovRegMemRbp(&s.Code, RCX, traceStartOffset)
	emitSubRegReg(&s.Code, RAX, RCX)
	emitMovRegReg(&s.Code, RDX, RAX)

	s.emitDataAddr(statusIdx)
	emitMovRegReg(&s.Code, RSI, RAX)
	s.emitDataAddr(fnIdx)
	emitMovRegReg(&s.Code, RDI, RAX)

	callSite = emitCallRel32(&s.Code, 0)

	s.CallPatches = append(s.CallPatches, CallPatch{
		ImmOffset: callSite,
		Target:    "__rt_trace_exit",
	})

	emitPopReg(&s.Code, RDX)
	emitPopReg(&s.Code, RAX)
}
