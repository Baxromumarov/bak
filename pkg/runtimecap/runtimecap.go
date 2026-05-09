package runtimecap

import (
	"sync"
	"time"

	"github.com/baxromumarov/bak/pkg/strfmt"
)

const (
	FlagAllowExec     = "--allow-exec"
	FlagAllowNet      = "--allow-net"
	FlagAllowFSMutate = "--allow-fs-mutate"
	FlagAllowAll      = "--allow-all"
	FlagExecTimeout   = "--exec-timeout"
	FlagExecMaxOutput = "--exec-max-output-bytes"
)

const (
	DefaultExecTimeout        = 30 * time.Second
	DefaultExecMaxOutputBytes = 1 << 20
)

type Permissions struct {
	AllowExec     bool
	AllowNet      bool
	AllowFSMutate bool
	ExecTimeout   time.Duration
	ExecMaxOutput int64
}

func AllPermissions() Permissions {
	return Permissions{
		AllowExec:     true,
		AllowNet:      true,
		AllowFSMutate: true,
	}
}

func PermissionError(op string, flag string) string {
	return strfmt.Named(
		"permission denied: {op} requires {flag}",
		"Op", op,
		"Flag", flag,
	)
}

func (p Permissions) EffectiveExecTimeout() time.Duration {
	if p.ExecTimeout > 0 {
		return p.ExecTimeout
	}
	return DefaultExecTimeout
}

func (p Permissions) EffectiveExecMaxOutputBytes() int64 {
	if p.ExecMaxOutput > 0 {
		return p.ExecMaxOutput
	}
	return DefaultExecMaxOutputBytes
}

var (
	currentMu sync.RWMutex
	current   Permissions
)

func Current() Permissions {
	currentMu.RLock()
	defer currentMu.RUnlock()
	return current
}

func SetCurrent(p Permissions) func() {
	currentMu.Lock()
	prev := current
	current = p
	currentMu.Unlock()

	return func() {
		currentMu.Lock()
		current = prev
		currentMu.Unlock()
	}
}
