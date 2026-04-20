// Package manifest provides parsing and writing of bak.toml manifest files.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Manifest represents the bak.toml package manifest.
type Manifest struct {
	Package        PackageInfo           `toml:"package"`
	LanguageMode   string                `toml:"language_mode,omitempty"`
	Dependencies   map[string]Dependency `toml:"dependencies"`
	Features       []string              `toml:"features,omitempty"`
	Permissions    *RuntimePermissions   `toml:"permissions,omitempty"`
	TrustedSources []string              `toml:"trusted_sources,omitempty"`
}

const (
	LanguageModeFrozen       = "frozen"
	LanguageModeExperimental = "experimental"
)

func NormalizeLanguageMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return LanguageModeFrozen
	}
	return mode
}

func IsKnownLanguageMode(mode string) bool {
	switch NormalizeLanguageMode(mode) {
	case LanguageModeFrozen, LanguageModeExperimental:
		return true
	default:
		return false
	}
}

// PackageInfo contains package metadata.
type PackageInfo struct {
	Name    string   `toml:"name"`
	Version string   `toml:"version"`
	Authors []string `toml:"authors,omitempty"`
}

// Dependency represents a single dependency.
type Dependency struct {
	// Git-based dependency
	Git     string `toml:"git,omitempty"`
	Version string `toml:"version,omitempty"`
	Branch  string `toml:"branch,omitempty"`

	// Local path dependency
	Path string `toml:"path,omitempty"`
}

// RuntimePermissions describes the dangerous runtime capabilities a project requests.
type RuntimePermissions struct {
	AllowExec     bool   `toml:"allow_exec,omitempty"`
	AllowNet      bool   `toml:"allow_net,omitempty"`
	AllowFSMutate bool   `toml:"allow_fs_mutate,omitempty"`
	ExecTimeout   string `toml:"exec_timeout,omitempty"`
	ExecMaxOutput int64  `toml:"exec_max_output_bytes,omitempty"`
}

// DefaultManifest creates a new manifest with default values.
func DefaultManifest(name string) *Manifest {
	return &Manifest{
		Package: PackageInfo{
			Name:    name,
			Version: "0.1.0",
			Authors: []string{},
		},
		LanguageMode: LanguageModeFrozen,
		Dependencies: make(map[string]Dependency),
		Permissions:  nil,
	}
}

// Load reads and parses a bak.toml file.
func Load(path string) (*Manifest, error) {
	var m Manifest
	if _, err := toml.DecodeFile(path, &m); err != nil {
		return nil, err
	}
	if m.Dependencies == nil {
		m.Dependencies = make(map[string]Dependency)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// LoadFromDir loads the bak.toml from a directory.
func LoadFromDir(dir string) (*Manifest, error) {
	return Load(filepath.Join(dir, "bak.toml"))
}

// Save writes the manifest to a file.
func (m *Manifest) Save(path string) error {
	if err := m.Validate(); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	return encoder.Encode(m)
}

// SaveToDir saves the bak.toml to a directory.
func (m *Manifest) SaveToDir(dir string) error {
	return m.Save(filepath.Join(dir, "bak.toml"))
}

// AddDependency adds a dependency to the manifest.
func (m *Manifest) AddDependency(name string, dep Dependency) {
	m.Dependencies[name] = dep
}

// Validate performs structural and semantic validation on a manifest.
func (m *Manifest) Validate() error {
	if m == nil {
		return fmt.Errorf("manifest is nil")
	}
	if strings.TrimSpace(m.Package.Name) == "" {
		return fmt.Errorf("package.name is required")
	}
	if strings.TrimSpace(m.Package.Version) == "" {
		return fmt.Errorf("package.version is required")
	}
	m.LanguageMode = NormalizeLanguageMode(m.LanguageMode)
	if !IsKnownLanguageMode(m.LanguageMode) {
		return fmt.Errorf("language_mode %q is invalid (expected one of: %s, %s)", m.LanguageMode, LanguageModeFrozen, LanguageModeExperimental)
	}
	if err := validateFeatureList(m.Features); err != nil {
		return err
	}

	depNames := make([]string, 0, len(m.Dependencies))
	for name := range m.Dependencies {
		depNames = append(depNames, name)
	}
	sort.Strings(depNames)
	for _, name := range depNames {
		dep := m.Dependencies[name]
		hasGit := strings.TrimSpace(dep.Git) != ""
		hasPath := strings.TrimSpace(dep.Path) != ""
		switch {
		case hasGit && hasPath:
			return fmt.Errorf("dependency %q cannot set both git and path", name)
		case !hasGit && !hasPath:
			return fmt.Errorf("dependency %q must set either git or path", name)
		}
		if hasPath && strings.TrimSpace(dep.Version) != "" {
			return fmt.Errorf("dependency %q cannot set version for a local path dependency", name)
		}
		if hasPath && strings.TrimSpace(dep.Branch) != "" {
			return fmt.Errorf("dependency %q cannot set branch for a local path dependency", name)
		}
	}

	if m.Permissions != nil {
		if m.Permissions.ExecTimeout != "" {
			dur, err := time.ParseDuration(m.Permissions.ExecTimeout)
			if err != nil {
				return fmt.Errorf("invalid permissions.exec_timeout %q: %w", m.Permissions.ExecTimeout, err)
			}
			if dur <= 0 {
				return fmt.Errorf("permissions.exec_timeout must be greater than zero")
			}
		}
		if m.Permissions.ExecMaxOutput < 0 {
			return fmt.Errorf("permissions.exec_max_output_bytes must be greater than or equal to zero")
		}
	}

	for _, source := range m.TrustedSources {
		source = strings.TrimSpace(source)
		if source == "" {
			return fmt.Errorf("trusted_sources entries must not be empty")
		}
		if strings.Contains(source, " ") {
			return fmt.Errorf("trusted_sources entry %q must not contain spaces", source)
		}
	}
	return nil
}

func validateFeatureList(features []string) error {
	seen := make(map[string]struct{}, len(features))
	for _, feature := range features {
		trimmed := strings.TrimSpace(feature)
		if trimmed == "" {
			return fmt.Errorf("features entries must not be empty")
		}
		if _, ok := seen[trimmed]; ok {
			return fmt.Errorf("duplicate feature %q in bak.toml", trimmed)
		}
		seen[trimmed] = struct{}{}
	}
	return nil
}

// HasDependency checks if a dependency exists.
func (m *Manifest) HasDependency(name string) bool {
	_, exists := m.Dependencies[name]
	return exists
}

// ValidateSourceAllowed checks if a package source matches the trusted source list.
// An empty trusted list means all sources are allowed (backward compatible).
// Trusted entries can be exact matches or wildcard prefixes ending with "/*".
func ValidateSourceAllowed(source string, trusted []string) error {
	if len(trusted) == 0 {
		return nil
	}
	for _, pattern := range trusted {
		if pattern == source {
			return nil
		}
		if strings.HasSuffix(pattern, "/*") {
			prefix := strings.TrimSuffix(pattern, "/*")
			if strings.HasPrefix(source, prefix+"/") || source == prefix {
				return nil
			}
		}
	}
	return fmt.Errorf("source %q is not in the trusted_sources allowlist", source)
}

// ValidateLockfileIntegrity performs structural integrity checks on a lockfile.
// It verifies that every locked package has a non-empty commit and checksum,
// and that there are no orphaned packages not present in the manifest.
func ValidateLockfileIntegrity(lock *Lockfile, m *Manifest) error {
	if lock == nil {
		return fmt.Errorf("lockfile is nil")
	}
	var problems []string
	for name, pkg := range lock.Packages {
		if pkg.Commit == "" {
			problems = append(problems, fmt.Sprintf("package %q has empty commit", name))
		}
		if pkg.Checksum == "" {
			problems = append(problems, fmt.Sprintf("package %q has empty checksum", name))
		}
		if m != nil && !m.HasDependency(name) {
			problems = append(problems, fmt.Sprintf("package %q exists in lockfile but not in manifest", name))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("lockfile integrity issues: %s", strings.Join(problems, "; "))
	}
	return nil
}
