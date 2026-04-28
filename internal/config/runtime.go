package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/baxromumarov/bak/pkg/manifest"
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

func ResolveProjectFeatureState(cliFeatures []string) (*manifest.Manifest, []string, error) {
	m, err := manifest.LoadFromDir(".")
	if err != nil {
		if os.IsNotExist(err) {
			features, resolveErr := ResolveProjectFeaturesByLanguageMode(manifest.LanguageModeFrozen, nil, cliFeatures)
			if resolveErr != nil {
				return nil, nil, resolveErr
			}
			return nil, features, nil
		}
		return nil, nil, err
	}

	features, err := ResolveProjectFeaturesByLanguageMode(m.LanguageMode, m.Features, cliFeatures)
	if err != nil {
		return nil, nil, err
	}
	return m, features, nil
}

func LoadProjectRuntimePermissions(base runtimecap.Permissions, cliFeatures []string) runtimecap.Permissions {
	m, features, err := ResolveProjectFeatureState(cliFeatures)
	if err != nil {
		runtimecap.SetCurrentFeatures(nil)
		fmt.Fprintf(os.Stderr, "Error loading runtime permissions from bak.toml: %v\n", err)
		os.Exit(1)
	}
	runtimecap.SetCurrentFeatures(features)
	if m == nil || m.Permissions == nil {
		return base
	}

	permissions := runtimecap.Permissions{
		AllowExec:     m.Permissions.AllowExec,
		AllowNet:      m.Permissions.AllowNet,
		AllowFSMutate: m.Permissions.AllowFSMutate,
		ExecMaxOutput: m.Permissions.ExecMaxOutput,
	}
	if m.Permissions.ExecTimeout != "" {
		dur, err := time.ParseDuration(m.Permissions.ExecTimeout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading runtime permissions from bak.toml: %v\n", fmt.Errorf("invalid permissions.exec_timeout %q: %w", m.Permissions.ExecTimeout, err))
			os.Exit(1)
		}
		permissions.ExecTimeout = dur
	}
	return mergeRuntimePermissions(base, permissions)
}

func ResolveProjectFeaturesByLanguageMode(languageMode string, manifestFeatures []string, cliFeatures []string) ([]string, error) {
	mode := manifest.NormalizeLanguageMode(languageMode)
	merged := mergeFeatureLists(manifestFeatures, cliFeatures)

	switch mode {
	case manifest.LanguageModeExperimental:
		return merged, nil
	case manifest.LanguageModeFrozen:
		manifestExperimental := pickExperimentalFeatures(manifestFeatures)
		if len(manifestExperimental) > 0 {
			return nil, fmt.Errorf(
				"bak.toml has language_mode=%q but enables experimental features: %s\n\nTo opt in, add this to bak.toml:\n%s",
				mode,
				strings.Join(manifestExperimental, ", "),
				languageModeOptInSnippet(),
			)
		}
		cliExperimental := pickExperimentalFeatures(cliFeatures)
		if len(cliExperimental) > 0 {
			return nil, fmt.Errorf(
				"language_mode=%q blocks CLI experimental features (%s)\n\nTo opt in, add this to bak.toml:\n%s",
				mode,
				strings.Join(cliExperimental, ", "),
				languageModeOptInSnippet(),
			)
		}
		return merged, nil
	default:
		return nil, fmt.Errorf("unknown language_mode %q", mode)
	}
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

func pickExperimentalFeatures(features []string) []string {
	experimental := make([]string, 0, len(features))
	for _, feature := range features {
		if runtimecap.IsKnownExperimentalFeature(feature) {
			experimental = append(experimental, feature)
		}
	}
	return experimental
}

func languageModeOptInSnippet() string {
	return "language_mode = \"experimental\""
}

func mergeRuntimePermissions(base, extra runtimecap.Permissions) runtimecap.Permissions {
	base.AllowExec = base.AllowExec || extra.AllowExec
	base.AllowNet = base.AllowNet || extra.AllowNet
	base.AllowFSMutate = base.AllowFSMutate || extra.AllowFSMutate
	if base.ExecTimeout <= 0 {
		base.ExecTimeout = extra.ExecTimeout
	}
	if base.ExecMaxOutput <= 0 {
		base.ExecMaxOutput = extra.ExecMaxOutput
	}
	return base
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
