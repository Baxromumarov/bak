package driver

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/baxromumarov/bak/internal/pkgmgr"
	"github.com/baxromumarov/bak/pkg/manifest"
)

// packageCommandOptions mirrors the CLI struct used by commands for package operations.
type packageCommandOptions struct {
	Offline        bool
	FrozenLockfile bool
}

// PackageOptions is an exported, simple options struct for package commands.
type PackageOptions struct {
	Offline        bool
	FrozenLockfile bool
}

const VERSION = "dev"

// GetPackageWithOptions exposes package fetching using an exported options type.
func GetPackageWithOptions(pkgArg string, opts PackageOptions) error {
	return GetPackage(pkgArg, packageCommandOptions{Offline: opts.Offline, FrozenLockfile: opts.FrozenLockfile})
}

// InstallDependenciesWithOptions exposes dependency installation using an exported options type.
func InstallDependenciesWithOptions(opts PackageOptions) error {
	return InstallDependencies(packageCommandOptions{Offline: opts.Offline, FrozenLockfile: opts.FrozenLockfile})
}

// GetPackage fetches and installs a package from a Git repository.
func GetPackage(pkgArg string, opts packageCommandOptions) error {
	if opts.FrozenLockfile {
		return fmt.Errorf("'bak get' cannot be used with --frozen-lockfile because it updates bak.lock")
	}
	if opts.Offline {
		return fmt.Errorf("'bak get --offline' cannot resolve a new dependency without network access")
	}

	m, err := manifest.LoadFromDir(".")
	if err != nil {
		cwd, cwdErr := pkgmgr.GetCwd()
		if cwdErr != nil {
			return fmt.Errorf("get cwd: %w", cwdErr)
		}
		m = manifest.DefaultManifest(filepath.Base(cwd))
	}

	pkgPath := pkgArg
	requestedVersion := "latest"
	if strings.Contains(pkgArg, "@") {
		parts := strings.SplitN(pkgArg, "@", 2)
		pkgPath = parts[0]
		requestedVersion = parts[1]
	}

	fullPath := pkgPath
	isExplicitURL := strings.Contains(pkgPath, "://") || strings.HasPrefix(pkgPath, "/") || strings.HasPrefix(pkgPath, "git@")
	if !isExplicitURL && !strings.Contains(pkgPath, ".") {
		fullPath = "github.com/" + pkgPath
	}

	if err := manifest.ValidateSourceAllowed(fullPath, m.TrustedSources); err != nil {
		return fmt.Errorf("validate source: %w", err)
	}

	parts := strings.Split(strings.TrimSuffix(fullPath, ".git"), "/")
	pkgName := parts[len(parts)-1]
	if pkgName == "" && len(parts) > 1 {
		pkgName = parts[len(parts)-2]
	}

	cacheDir := ".bak-cache/pkg"
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	gitURL := fullPath
	if !isExplicitURL {
		gitURL = "https://" + fullPath + ".git"
	}

	fmt.Printf("Fetching %s (%s)...\n", fullPath, requestedVersion)
	lockedPkg, err := pkgmgr.FetchAndCacheLockedPackage(cacheDir, pkgName, fullPath, gitURL, requestedVersion)
	if err != nil {
		return fmt.Errorf("fetch package: %w", err)
	}

	m.AddDependency(pkgName, manifest.Dependency{Git: fullPath, Version: requestedVersion})
	if err := m.SaveToDir("."); err != nil {
		return fmt.Errorf("save manifest: %w", err)
	}

	lock, _ := manifest.LoadLockfileFromDir(".")
	lock.AddPackage(pkgName, lockedPkg)
	if err := lock.SaveToDir("."); err != nil {
		return fmt.Errorf("save lock: %w", err)
	}

	fmt.Printf("Added %s @ %s (%s)\n", pkgName, lockedPkg.Version, pkgmgr.ShortCommit(lockedPkg.Commit))
	return nil
}

// InstallDependencies installs packages from bak.lock
func InstallDependencies(opts packageCommandOptions) error {
	if !pkgmgr.LockfileExists(".") {
		return fmt.Errorf("bak.lock not found; run 'bak get' to add dependencies first")
	}

	lock, err := manifest.LoadLockfileFromDir(".")
	if err != nil {
		return fmt.Errorf("load lockfile: %w", err)
	}

	m, err := manifest.LoadFromDir(".")
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	if err := manifest.ValidateLockfileIntegrity(lock, m); err != nil {
		return fmt.Errorf("lockfile integrity: %w", err)
	}

	if opts.FrozenLockfile {
		if err := pkgmgr.ValidateFrozenLockfile(".", lock); err != nil {
			return fmt.Errorf("frozen lockfile validation: %w", err)
		}
	}

	cacheDir := ".bak-cache/pkg"
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	if opts.Offline {
		if info, err := os.Stat(cacheDir); errors.Is(err, os.ErrNotExist) || (err == nil && !info.IsDir()) {
			return fmt.Errorf("offline mode requested but cache dir %s is missing", cacheDir)
		}
	}

	names := pkgmgr.SortedLockedPackageNames(lock)
	lockDirty := false
	for _, name := range names {
		pkg := lock.Packages[name]
		if err := manifest.ValidateSourceAllowed(pkg.Source, m.TrustedSources); err != nil {
			return fmt.Errorf("install %s: %w", pkg.Name, err)
		}
		normalizedPath := pkgmgr.PackageCachePath(cacheDir, pkg.Name, pkg.Source, pkg.Commit)
		if pkg.Path == "" || pkg.Path != normalizedPath {
			pkg.Path = normalizedPath
			lockDirty = true
		}
		checksum, err := pkgmgr.EnsureLockedPackage(cacheDir, pkg, pkgmgr.Options{Offline: opts.Offline, FrozenLockfile: opts.FrozenLockfile})
		if err != nil {
			return fmt.Errorf("install %s: %w", pkg.Name, err)
		}
		if checksum != "" && pkg.Checksum != checksum {
			pkg.Checksum = checksum
			lockDirty = true
		}
		lock.Packages[name] = pkg
		fmt.Printf("Installed %s @ %s (%s)\n", pkg.Name, pkg.Version, pkgmgr.ShortCommit(pkg.Commit))
	}
	if lockDirty && !opts.FrozenLockfile {
		if err := lock.SaveToDir("."); err != nil {
			return fmt.Errorf("save lockfile: %w", err)
		}
	}
	fmt.Println("Done.")
	return nil
}
