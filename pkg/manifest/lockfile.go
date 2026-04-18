// Package manifest provides lockfile handling.
package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	return &l, nil
}

// LoadLockfileFromDir loads the bak.lock from a directory.
func LoadLockfileFromDir(dir string) (*Lockfile, error) {
	return LoadLockfile(filepath.Join(dir, "bak.lock"))
}

// Save writes the lockfile to a file.
func (l *Lockfile) Save(path string) error {
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

// GetPackage retrieves a locked package by name.
func (l *Lockfile) GetPackage(name string) (LockedPackage, bool) {
	pkg, exists := l.Packages[name]
	return pkg, exists
}
