package vm

import (
	"fmt"

	"github.com/baxromumarov/bak/pkg/compiler"
)

// runtimeLimit is a Bak runtime failure recovered at the VM boundary.
// It is not a host panic and must not escape Run.
type runtimeLimit struct {
	err error
}

func (vm *VM) stackOverflowError() error {
	return fmt.Errorf("runtime error: stack overflow: exceeded maximum stack depth of %d", StackMax)
}

func (vm *VM) frameOverflowError() error {
	return fmt.Errorf("runtime error: frame overflow: exceeded maximum call depth of %d", FramesMax)
}

// pushValue writes one stack slot or returns a Bak stack-overflow error.
func (vm *VM) pushValue(v compiler.Value) error {
	if vm.sp < 0 || vm.sp >= StackMax {
		return vm.stackOverflowError()
	}
	vm.stack[vm.sp] = v
	vm.sp++
	return nil
}

func (vm *VM) localSlot(frame *CallFrame, slot int) (int, error) {
	if frame == nil || slot < 0 {
		return 0, fmt.Errorf("runtime error: local slot out of range")
	}
	idx := frame.base + slot
	if idx < 0 || idx >= StackMax {
		return 0, fmt.Errorf("runtime error: local slot out of range")
	}
	return idx, nil
}
