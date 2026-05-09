package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/baxromumarov/bak/pkg/runtimecap"
)

func StripTraceFlag(args []string) ([]string, bool) {
	filtered := make([]string, 0, len(args))
	traceEnabled := false
	for _, arg := range args {
		if arg == "--trace" {
			traceEnabled = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered, traceEnabled
}

func StripDebugEscapesFlag(args []string) ([]string, bool) {
	filtered := make([]string, 0, len(args))
	debugEscapes := false
	for _, arg := range args {
		if arg == "--debug-escapes" {
			debugEscapes = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered, debugEscapes
}

func ParseRuntimePermissions(args []string) (runtimecap.Permissions, []string, error) {
	var permissions runtimecap.Permissions
	var rest []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case runtimecap.FlagAllowExec:
			permissions.AllowExec = true
		case runtimecap.FlagAllowNet:
			permissions.AllowNet = true
		case runtimecap.FlagAllowFSMutate:
			permissions.AllowFSMutate = true
		case runtimecap.FlagAllowAll:
			permissions = runtimecap.AllPermissions()
		case runtimecap.FlagExecTimeout:
			i++
			if i >= len(args) {
				return runtimecap.Permissions{}, nil, fmt.Errorf("%s requires a duration value", runtimecap.FlagExecTimeout)
			}
			dur, err := time.ParseDuration(args[i])
			if err != nil {
				return runtimecap.Permissions{}, nil, fmt.Errorf("invalid %s value %q: %w", runtimecap.FlagExecTimeout, args[i], err)
			}
			if dur <= 0 {
				return runtimecap.Permissions{}, nil, fmt.Errorf("%s must be greater than zero", runtimecap.FlagExecTimeout)
			}
			permissions.ExecTimeout = dur
		case runtimecap.FlagExecMaxOutput:
			i++
			if i >= len(args) {
				return runtimecap.Permissions{}, nil, fmt.Errorf("%s requires a byte limit", runtimecap.FlagExecMaxOutput)
			}
			limit, err := strconv.ParseInt(args[i], 10, 64)
			if err != nil {
				return runtimecap.Permissions{}, nil, fmt.Errorf("invalid %s value %q: %w", runtimecap.FlagExecMaxOutput, args[i], err)
			}
			if limit <= 0 {
				return runtimecap.Permissions{}, nil, fmt.Errorf("%s must be greater than zero", runtimecap.FlagExecMaxOutput)
			}
			permissions.ExecMaxOutput = limit
		default:
			if value, ok := strings.CutPrefix(arg, runtimecap.FlagExecTimeout+"="); ok {
				dur, err := time.ParseDuration(value)
				if err != nil {
					return runtimecap.Permissions{}, nil, fmt.Errorf("invalid %s value %q: %w", runtimecap.FlagExecTimeout, value, err)
				}
				if dur <= 0 {
					return runtimecap.Permissions{}, nil, fmt.Errorf("%s must be greater than zero", runtimecap.FlagExecTimeout)
				}
				permissions.ExecTimeout = dur
				continue
			}
			if value, ok := strings.CutPrefix(arg, runtimecap.FlagExecMaxOutput+"="); ok {
				limit, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return runtimecap.Permissions{}, nil, fmt.Errorf("invalid %s value %q: %w", runtimecap.FlagExecMaxOutput, value, err)
				}
				if limit <= 0 {
					return runtimecap.Permissions{}, nil, fmt.Errorf("%s must be greater than zero", runtimecap.FlagExecMaxOutput)
				}
				permissions.ExecMaxOutput = limit
				continue
			}
			rest = append(rest, arg)
		}
	}

	return permissions, rest, nil
}

func LoadProjectRuntimePermissions(base runtimecap.Permissions) runtimecap.Permissions {
	return base
}
