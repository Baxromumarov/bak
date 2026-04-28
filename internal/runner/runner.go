package runner

import (
	"github.com/baxromumarov/bak/internal/pipeline"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

// RunVM executes a pipeline through the VM runner.
func RunVM(
	p *pipeline.Pipeline,
	scriptArgs []string,
	permissions runtimecap.Permissions,
	traceEnabled bool,
) error {
	return p.RunVM(scriptArgs, permissions, traceEnabled)
}

// BuildNative builds a native executable from a pipeline.
func BuildNative(
	p *pipeline.Pipeline,
	outputFile string,
	permissions runtimecap.Permissions,
	traceEnabled bool,
) (
	string,
	error,
) {
	return p.BuildNative(outputFile, permissions, traceEnabled)
}
