package pkgmgr

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/baxromumarov/bak/pkg/manifest"
)

// GetCwd returns the current working directory.
func GetCwd() (string, error) {
	return os.Getwd()
}

// FetchAndCacheLockedPackage clones a git repo, resolves the requested
// version (tag or "latest"), computes a directory checksum, and installs
// the package into the cache directory atomically.
func FetchAndCacheLockedPackage(cacheDir, pkgName, fullPath, gitURL, requestedVersion string) (manifest.LockedPackage, error) {
	tmpDir, cleanup, err := makePackageTempDir(cacheDir)
	if err != nil {
		return manifest.LockedPackage{}, err
	}
	defer cleanup()

	if err := gitClone(gitURL, tmpDir); err != nil {
		return manifest.LockedPackage{}, err
	}
	commitHash, resolvedVersion, err := resolveVersion(tmpDir, requestedVersion)
	if err != nil {
		return manifest.LockedPackage{}, err
	}
	if err := gitRun(tmpDir, "checkout", commitHash); err != nil {
		return manifest.LockedPackage{}, fmt.Errorf("checking out commit %s: %w", commitHash, err)
	}
	if err := removeGitMetadata(tmpDir); err != nil {
		return manifest.LockedPackage{}, err
	}
	checksum, err := DirectoryChecksum(tmpDir)
	if err != nil {
		return manifest.LockedPackage{}, err
	}
	lockedPkg := manifest.LockedPackage{
		Name:     pkgName,
		Version:  resolvedVersion,
		Source:   fullPath,
		Commit:   commitHash,
		Checksum: checksum,
		Path:     packageCachePath(cacheDir, pkgName, fullPath, commitHash),
	}
	if err := replaceDirAtomically(tmpDir, lockedPkg.Path); err != nil {
		return manifest.LockedPackage{}, err
	}
	cleanup = func() {}
	return lockedPkg, nil
}

// ShortCommit returns a short commit string.
func ShortCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// LockfileExists returns true if bak.lock exists at the given path (dir).
func LockfileExists(path string) bool {
	_, err := os.Stat(filepath.Join(path, "bak.lock"))
	return err == nil
}

// ValidateFrozenLockfile ensures bak.lock matches bak.toml constraints.
func ValidateFrozenLockfile(root string, lock *manifest.Lockfile) error {
	m, err := manifest.LoadFromDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("loading bak.toml for frozen lockfile validation: %w", err)
	}
	for name, dep := range m.Dependencies {
		if strings.TrimSpace(dep.Path) != "" {
			continue
		}
		pkg, ok := lock.Packages[name]
		if !ok {
			return fmt.Errorf("bak.lock is missing dependency %q required by bak.toml", name)
		}
		if dep.Git != "" && pkg.Source != dep.Git {
			return fmt.Errorf("bak.lock dependency %q points to %q, but bak.toml requires %q", name, pkg.Source, dep.Git)
		}
		if expected := strings.TrimSpace(dep.Version); expected != "" && !FrozenLockfileVersionMatches(expected, pkg.Version) {
			return fmt.Errorf("bak.lock dependency %q is version %q, but bak.toml requires %q", name, pkg.Version, expected)
		}
	}
	return nil
}

// FrozenLockfileVersionMatches compares version strings with a permissive v-prefix rule.
func FrozenLockfileVersionMatches(expected, actual string) bool {
	expected = strings.TrimSpace(expected)
	actual = strings.TrimSpace(actual)

	if expected == "" || actual == "" {
		return expected == actual
	}
	if expected == actual {
		return true
	}
	if expected == "latest" || actual == "latest" {
		return expected == actual
	}
	return strings.TrimPrefix(expected, "v") == strings.TrimPrefix(actual, "v")
}

// SortedLockedPackageNames returns sorted names from a lockfile.
func SortedLockedPackageNames(lock *manifest.Lockfile) []string {
	if lock == nil {
		return nil
	}
	names := make([]string, 0, len(lock.Packages))
	for k := range lock.Packages {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func packageCachePath(cacheDir, name, source, commit string) string {
	safe := sanitizeCacheName(name)
	if safe == "" {
		safe = "pkg"
	}
	return filepath.Join(cacheDir, safe+"-"+packageCacheKey(source, commit))
}

// PackageCachePath is exported wrapper.
func PackageCachePath(cacheDir, name, source, commit string) string {
	return packageCachePath(cacheDir, name, source, commit)
}

// EnsureLockedPackage ensures the package is present in cache and returns checksum.
func EnsureLockedPackage(cacheDir string, pkg manifest.LockedPackage, opts Options) (string, error) {
	targetPath := pkg.Path
	if targetPath == "" {
		targetPath = packageCachePath(cacheDir, pkg.Name, pkg.Source, pkg.Commit)
	}

	if _, err := os.Stat(targetPath); err == nil {
		checksum, err := DirectoryChecksum(targetPath)
		if err != nil {
			return "", err
		}
		if pkg.Checksum == "" || checksum == pkg.Checksum {
			return checksum, nil
		}
		if opts.Offline {
			return "", fmt.Errorf("cached package checksum mismatch for %s and offline mode is enabled", pkg.Name)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	} else if opts.Offline {
		return "", fmt.Errorf("package %s is not cached at %s", pkg.Name, targetPath)
	}

	gitURL := sourceToGitURL(pkg.Source)
	tmpDir, cleanup, err := makePackageTempDir(cacheDir)
	if err != nil {
		return "", err
	}
	defer cleanup()

	if err := gitClone(gitURL, tmpDir); err != nil {
		return "", err
	}
	if pkg.Commit != "" {
		if err := gitRun(tmpDir, "checkout", pkg.Commit); err != nil {
			return "", fmt.Errorf("checking out %s: %w", pkg.Commit, err)
		}
	}
	if err := removeGitMetadata(tmpDir); err != nil {
		return "", err
	}
	checksum, err := DirectoryChecksum(tmpDir)
	if err != nil {
		return "", err
	}
	if pkg.Checksum != "" && checksum != pkg.Checksum {
		return "", fmt.Errorf("checksum mismatch for %s: expected %s, got %s", pkg.Name, pkg.Checksum, checksum)
	}
	if err := replaceDirAtomically(tmpDir, targetPath); err != nil {
		return "", err
	}
	cleanup = func() {}
	return checksum, nil
}

func makePackageTempDir(cacheDir string) (string, func(), error) {
	tmpRoot := filepath.Join(cacheDir, ".tmp")
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		return "", nil, err
	}
	tmpDir, err := os.MkdirTemp(tmpRoot, "pkg-*")
	if err != nil {
		return "", nil, err
	}
	return tmpDir, func() { _ = os.RemoveAll(tmpDir) }, nil
}

func gitClone(gitURL, dest string) error {
	return gitRun("", "clone", gitURL, dest)
}

func gitRun(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func gitOutput(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return out, nil
}

func removeGitMetadata(dir string) error {
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		if err := os.RemoveAll(gitDir); err != nil {
			return fmt.Errorf("removing git metadata: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func replaceDirAtomically(srcDir, destDir string) error {
	if err := os.MkdirAll(filepath.Dir(destDir), 0o755); err != nil {
		return err
	}
	backupDir := destDir + ".bak-old"
	_ = os.RemoveAll(backupDir)

	destExists := false
	if _, err := os.Stat(destDir); err == nil {
		destExists = true
		if err := os.Rename(destDir, backupDir); err != nil {
			return fmt.Errorf("moving existing cache aside: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.Rename(srcDir, destDir); err != nil {
		if destExists {
			_ = os.Rename(backupDir, destDir)
		}
		return fmt.Errorf("installing cache directory: %w", err)
	}

	if destExists {
		_ = os.RemoveAll(backupDir)
	}
	return nil
}

// DirectoryChecksum computes a stable checksum for a directory tree.
func DirectoryChecksum(root string) (string, error) {
	h := sha256.New()
	var files []string
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(files)
	for _, rel := range files {
		if _, err := io.WriteString(h, rel+"\x00"); err != nil {
			return "", err
		}
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return "", err
		}
		if _, err := h.Write(data); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func resolveVersion(repoDir, version string) (string, string, error) {
	if version == "latest" {
		out, err := gitOutput(repoDir, "rev-parse", "HEAD")
		if err != nil {
			return "", "", err
		}
		return strings.TrimSpace(string(out)), "latest", nil
	}

	candidates := []string{version, "v" + version}
	for _, tag := range candidates {
		if _, err := gitOutput(repoDir, "rev-parse", tag); err == nil {
			out, _ := gitOutput(repoDir, "rev-parse", tag)
			return strings.TrimSpace(string(out)), tag, nil
		}
	}

	return "", "", fmt.Errorf("version tag '%s' not found", version)
}

func sourceToGitURL(source string) string {
	if strings.Contains(source, "://") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "git@") {
		return source
	}
	return "https://" + strings.TrimSuffix(source, ".git") + ".git"
}

func packageCacheKey(source, commit string) string {
	sum := sha256.Sum256([]byte(source + "\n" + commit))
	return hex.EncodeToString(sum[:8])
}

func sanitizeCacheName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}
