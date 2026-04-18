package vm

import (
	"bytes"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/trace"
)

func TestVMTraceEmitsEnterAndExit(t *testing.T) {
	src := `package main

trace func work(value int) -> (int) {
	return value + 1
}

func main() -> (int) {
	return work(41)
}
`

	module := compileModule(t, src)
	var buf bytes.Buffer
	v := New(module)
	v.SetTracer(trace.New(true, &buf))

	val, err := v.Run()
	if err != nil {
		t.Fatalf("unexpected VM error: %v", err)
	}
	if val.AsInt != 42 {
		t.Fatalf("unexpected VM result: got %d want 42", val.AsInt)
	}

	output := buf.String()
	if !strings.Contains(output, "bak.trace event=enter fn=work depth=1 thread=0") {
		t.Fatalf("missing trace enter event:\n%s", output)
	}
	if !strings.Contains(output, "bak.trace event=exit fn=work depth=1 thread=0 status=ok duration_ns=") {
		t.Fatalf("missing trace exit event:\n%s", output)
	}
}

func TestVMTraceMarksPanicExit(t *testing.T) {
	src := `package main

trace func boom() -> (void) {
	panic("boom")
}

func main() -> (void) {
	boom()
}
`

	module := compileModule(t, src)
	var buf bytes.Buffer
	v := New(module)
	v.SetTracer(trace.New(true, &buf))

	if _, err := v.Run(); err == nil {
		t.Fatalf("expected VM panic")
	}

	output := buf.String()
	if !strings.Contains(output, "bak.trace event=exit fn=boom depth=1 thread=0 status=panic duration_ns=") {
		t.Fatalf("missing trace panic exit event:\n%s", output)
	}
}
