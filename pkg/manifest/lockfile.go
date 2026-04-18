// Package manifest provides lockfile handling.
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Lockfile represents the bak.lock file with resolved dependencies.
type Lockfile struct {
	Version  int                      `json:"version"`
	Packages map[string]LockedPackage `json:"packages"`
}

// LockedPackage represents a resolved dependency.
type LockedPackage struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Source   string `json:"source"`             // git URL or local path
	Commit   string `json:"commit,omitempty"`   // git commit hash
	Checksum string `json:"checksum,omitempty"` // content checksum of cached package files
	Path     string `json:"path"`               // path in .bak-cache
}

// NewLockfile creates a new empty lockfile.
func NewLockfile() *Lockfile {
	return &Lockfile{
		Version:  1,
		Packages: make(map[string]LockedPackage),
	}
}

// LoadLockfile reads and parses a bak.lock file.
func LoadLockfile(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewLockfile(), nil
		}
		return nil, err
	}

	var l Lockfile
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, err
	}
	if l.Packages == nil {
		l.Packages = make(map[string]LockedPackage)
	}
	if err := l.Validate(); err != nil {
		return nil, err
	}
	return &l, nil
}

// LoadLockfileFromDir loads the bak.lock from a directory.
func LoadLockfileFromDir(dir string) (*Lockfile, error) {
	return LoadLockfile(filepath.Join(dir, "bak.lock"))
}

// Save writes the lockfile to a file.
func (l *Lockfile) Save(path string) error {
	if err := l.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SaveToDir saves the bak.lock to a directory.
func (l *Lockfile) SaveToDir(dir string) error {
	return l.Save(filepath.Join(dir, "bak.lock"))
}

// AddPackage adds a resolved package to the lockfile.
func (l *Lockfile) AddPackage(name string, pkg LockedPackage) {
	l.Packages[name] = pkg
}

// Validate performs structural validation on a lockfile.
func (l *Lockfile) Validate() error {
	if l == nil {
		return fmt.Errorf("lockfile is nil")
	}
	if l.Version <= 0 {
		return fmt.Errorf("lockfile version must be greater than zero")
	}

	names := make([]string, 0, len(l.Packages))
	for name := range l.Packages {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		pkg := l.Packages[name]
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("lockfile package names must not be empty")
		}
		if pkg.Name != "" && pkg.Name != name {
			return fmt.Errorf("lockfile package %q has mismatched name %q", name, pkg.Name)
		}
		if strings.TrimSpace(pkg.Source) == "" {
			return fmt.Errorf("lockfile package %q has empty source", name)
		}
		if strings.TrimSpace(pkg.Path) == "" {
			return fmt.Errorf("lockfile package %q has empty path", name)
		}
	}
	return nil
}

// GetPackage retrieves a locked package by name.
func (l *Lockfile) GetPackage(name string) (LockedPackage, bool) {
	pkg, exists := l.Packages[name]
	return pkg, exists
}
