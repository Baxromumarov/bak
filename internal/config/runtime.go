package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/strfmt"
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

func ParseExperimentalFeatures(args []string) ([]string, []string, error) {
	features := []string{}
	rest := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--experimental":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--experimental requires a feature list")
			}
			parsed, err := parseExperimentalFeatureList(args[i+1])
			if err != nil {
				return nil, nil, err
			}
			features = append(features, parsed...)
			i++
		case strings.HasPrefix(arg, "--experimental="):
			parsed, err := parseExperimentalFeatureList(strings.TrimPrefix(arg, "--experimental="))
			if err != nil {
				return nil, nil, err
			}
			features = append(features, parsed...)
		default:
			rest = append(rest, arg)
		}
	}

	return mergeFeatureLists(nil, features), rest, nil
}

func ResolveProjectFeatureState(cliFeatures []string) ([]string, error) {
	return mergeFeatureLists(nil, cliFeatures), nil
}

func LoadProjectRuntimePermissions(base runtimecap.Permissions, cliFeatures []string) runtimecap.Permissions {
	features, err := ResolveProjectFeatureState(cliFeatures)
	if err != nil {
		runtimecap.SetCurrentFeatures(nil)
		_, _ = strfmt.Fprintln(os.Stderr, "Error resolving runtime feature flags: ", err)
		return base
	}
	runtimecap.SetCurrentFeatures(features)
	return base
}

func mergeFeatureLists(base []string, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	merged := make([]string, 0, len(base)+len(extra))
	for _, feature := range append(append([]string(nil), base...), extra...) {
		feature = strings.TrimSpace(feature)
		if feature == "" {
			continue
		}
		if _, ok := seen[feature]; ok {
			continue
		}
		seen[feature] = struct{}{}
		merged = append(merged, feature)
	}
	sort.Strings(merged)
	return merged
}

func parseExperimentalFeatureList(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	features := make([]string, 0, len(parts))
	for _, part := range parts {
		feature, err := canonicalExperimentalFeature(part)
		if err != nil {
			return nil, err
		}
		features = append(features, feature)
	}
	return features, nil
}

func canonicalExperimentalFeature(name string) (string, error) {
	switch strings.TrimSpace(name) {
	case "unsafe", runtimecap.ExperimentalFeatureUnsafe:
		return runtimecap.ExperimentalFeatureUnsafe, nil
	case "user-generics", runtimecap.ExperimentalFeatureUserGenerics:
		return runtimecap.ExperimentalFeatureUserGenerics, nil
	default:
		return "", fmt.Errorf("unknown experimental feature %q (expected one of: unsafe, user-generics)", name)
	}
}
