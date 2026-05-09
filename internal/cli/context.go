package cli

import "github.com/baxromumarov/bak/pkg/runtimecap"

// Context carries command execution state shared across all CLI commands.
type Context struct {
	Permissions  runtimecap.Permissions
	Trace        bool
	DebugEscapes bool
	WorkingDir   string
	ScriptArgs   []string
}
