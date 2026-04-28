package pkgmgr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/manifest"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

func ResolveDoctorToolPath(root, tool string) (string, string, error) {
	if toolPath, err := exec.LookPath(tool); err == nil {
		return toolPath, "PATH", nil
	}
	localPath := filepath.Join(root, "bin", tool)
	if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
		return localPath, "local", nil
	}
	return "", "", fmt.Errorf("tool %s not found", tool)
}

func DoctorToolVersion(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "version unknown"
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return "version unknown"
	}
	if idx := strings.IndexByte(trimmed, '\n'); idx >= 0 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}
	if trimmed == "" {
		return "version unknown"
	}
	return trimmed
}

func DoctorSmokeCheckBakFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read failed: %w", err)
	}

	l := lexer.New(string(data))
	p := parser.New(l)
	p.SetFilename(path)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return fmt.Errorf("parse failed (%s)", p.Errors()[0])
	}

	tc := typechecker.NewWithPath(path)
	tc.SetSuppressUnused(true)
	typeErrors := tc.Check(program)
	if len(typeErrors) > 0 && hasFatalTypeErrorsLocal(tc) {
		return fmt.Errorf("typecheck failed (%s)", typeErrors[0])
	}
	return nil
}

type DoctorCacheCheck struct {
	Status string
	Name   string
	Detail string
}

func DoctorLockCacheChecks(root string, lock *manifest.Lockfile) []DoctorCacheCheck {
	results := make([]DoctorCacheCheck, 0)
	if lock == nil || len(lock.Packages) == 0 {
		return results
	}

	cacheRoot := filepath.Join(root, ".bak-cache", "pkg")
	if _, err := os.Stat(cacheRoot); errors.Is(err, os.ErrNotExist) {
		results = append(results, DoctorCacheCheck{
			Status: "warn",
			Name:   "lock cache",
			Detail: "cache directory .bak-cache/pkg is missing; run 'bak install' to populate dependency cache",
		})
		return results
	}

	if err := verifyLockCacheChecksums(root, lock); err != nil {
		results = append(results, DoctorCacheCheck{Status: "fail", Name: "lock cache checksums", Detail: err.Error()})
	} else {
		results = append(results, DoctorCacheCheck{Status: "ok", Name: "lock cache checksums", Detail: "cache entries match bak.lock checksums for available packages"})
	}
	return results
}

func verifyLockCacheChecksums(root string, lock *manifest.Lockfile) error {
	var problems []string
	for _, name := range SortedLockedPackageNames(lock) {
		pkg := lock.Packages[name]
		targetPath := pkg.Path
		if targetPath == "" {
			problems = append(problems, fmt.Sprintf("%s has empty cache path; run 'bak install' to normalize bak.lock", name))
			continue
		}
		if !filepath.IsAbs(targetPath) {
			targetPath = filepath.Join(root, targetPath)
		}
		info, err := os.Stat(targetPath)
		if errors.Is(err, os.ErrNotExist) {
			problems = append(problems, fmt.Sprintf("%s cache is missing at %s; run 'bak install'", name, pkg.Path))
			continue
		}
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s cache check failed at %s: %v", name, pkg.Path, err))
			continue
		}
		if !info.IsDir() {
			problems = append(problems, fmt.Sprintf("%s cache path %s is not a directory", name, pkg.Path))
			continue
		}
		if strings.TrimSpace(pkg.Checksum) == "" {
			problems = append(problems, fmt.Sprintf("%s has empty checksum in bak.lock; run 'bak install' to refresh lock metadata", name))
			continue
		}
		checksum, err := DirectoryChecksum(targetPath)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s checksum read failed: %v", name, err))
			continue
		}
		if checksum != pkg.Checksum {
			problems = append(problems, fmt.Sprintf("%s checksum mismatch (lock=%s cache=%s); run 'bak install' to repair cache", name, pkg.Checksum, checksum))
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func hasFatalTypeErrorsLocal(tc *typechecker.TypeChecker) bool {
	if tc == nil {
		return false
	}
	for _, typeErr := range tc.GetErrors() {
		if typeErr.Tier == typechecker.TierFatal {
			return true
		}
	}
	return false
}
