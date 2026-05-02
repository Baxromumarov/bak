package runtimecap

import (
	"slices"
	"sort"
	"strings"
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

const (
	ExperimentalFeatureUnsafe       = "experimental-unsafe"
	ExperimentalFeatureUserGenerics = "experimental-user-generics"
)

func KnownExperimentalFeatures() []string {
	return []string{
		ExperimentalFeatureUnsafe,
		ExperimentalFeatureUserGenerics,
	}
}

func IsKnownExperimentalFeature(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	return slices.Contains(KnownExperimentalFeatures(), name)
}

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

	featureMu       sync.RWMutex
	currentFeatures []string
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

func normalizeFeatures(features []string) []string {
	seen := make(map[string]struct{}, len(features))
	normalized := make([]string, 0, len(features))
	
	for _, feature := range features {
		feature = strings.TrimSpace(feature)
		if feature == "" {
			continue
		}
		if _, ok := seen[feature]; ok {
			continue
		}
		seen[feature] = struct{}{}
		normalized = append(normalized, feature)
	}
	
	sort.Strings(normalized)

	return normalized
}

func CurrentFeatures() []string {
	featureMu.RLock()
	defer featureMu.RUnlock()

	return append([]string(nil), currentFeatures...)
}

func CurrentFeatureEnabled(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}

	featureMu.RLock()
	defer featureMu.RUnlock()

	idx := sort.SearchStrings(currentFeatures, name)

	return idx < len(currentFeatures) && currentFeatures[idx] == name
}

func SetCurrentFeatures(features []string) func() {
	featureMu.Lock()
	prev := append([]string(nil), currentFeatures...)

	currentFeatures = normalizeFeatures(features)

	featureMu.Unlock()

	return func() {
		featureMu.Lock()
		currentFeatures = prev
		featureMu.Unlock()
	}
}
