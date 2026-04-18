// Package manifest provides parsing and writing of bak.toml manifest files.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Manifest represents the bak.toml package manifest.
type Manifest struct {
	Package        PackageInfo           `toml:"package"`
	Dependencies   map[string]Dependency `toml:"dependencies"`
	Permissions    *RuntimePermissions   `toml:"permissions,omitempty"`
	TrustedSources []string              `toml:"trusted_sources,omitempty"`
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
	return &m, nil
}

// LoadFromDir loads the bak.toml from a directory.
func LoadFromDir(dir string) (*Manifest, error) {
	return Load(filepath.Join(dir, "bak.toml"))
}

// Save writes the manifest to a file.
func (m *Manifest) Save(path string) error {
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
