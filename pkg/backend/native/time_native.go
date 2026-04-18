package native

import "github.com/baxromumarov/bak/pkg/ast"

const (
	nanosPerMillisecond = 1000000
)

func (s *EmitState) ensureTimeCounterData() int {
	if s.TimeCounterDataIndex < 0 {
		s.TimeCounterDataIndex = s.addDataItem(make([]byte, 8), 8)
	}
	return s.TimeCounterDataIndex
}

func (s *EmitState) emitTimeNow(clockID int32) error {
	_ = clockID
	idx := s.ensureTimeCounterData()
	s.emitDataAddr(idx)
	emitMovRegMem(&s.Code, RCX, RAX) // current monotonic counter
	emitMovRegReg(&s.Code, RDX, RCX)
	emitAddRegImm32(&s.Code, RDX, 1)
	emitMovMemReg(&s.Code, RAX, RDX)
	emitMovRegReg(&s.Code, RAX, RCX)
	return nil
}

func (s *EmitState) emitSleepMs(expr ast.Expression) error {
	if err := s.emitExpression(expr); err != nil {
		return err
	}

	// Save the requested delay in milliseconds and advance the counter deterministically.
	emitMovRegReg(&s.Code, RDX, RAX)
	idx := s.ensureTimeCounterData()
	s.emitDataAddr(idx)
	emitMovRegReg(&s.Code, RSI, RAX) // save counter address
	emitMovRegMem(&s.Code, RCX, RSI) // current counter value
	emitMovRegReg(&s.Code, RAX, RDX) // restore ms
	emitMovRegImm32(&s.Code, RDX, nanosPerMillisecond)
	emitImulRegReg(&s.Code, RAX, RDX)
	emitAddRegReg(&s.Code, RAX, RCX)
	emitMovMemReg(&s.Code, RSI, RAX)

	return nil
}
