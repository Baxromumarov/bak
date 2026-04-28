package driver

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/baxromumarov/bak/internal/pkgmgr"
	"github.com/baxromumarov/bak/pkg/manifest"
)

// RunDoctor runs doctor checks and returns an error when checks fail.
func RunDoctor(w io.Writer, root string) error {
	ok := true
	check := func(status, name, detail string) {
		fmt.Fprintf(w, "[%s] %s", status, name)
		if detail != "" {
			fmt.Fprintf(w, " - %s", detail)
		}
		fmt.Fprintln(w)
		if status == "fail" {
			ok = false
		}
	}

	fmt.Fprintf(w, "Bak doctor\n")
	fmt.Fprintf(w, "version: %s\n", VERSION)
	fmt.Fprintf(w, "host: %s/%s\n", runtime.GOOS, runtime.GOARCH)

	absRoot, err := filepath.Abs(root)
	if err != nil {
		check("fail", "workspace", err.Error())
	} else {
		check("ok", "workspace", absRoot)
	}

	for _, tool := range []string{"bak", "bakfmt", "baklint", "bakcheck", "bak-lsp"} {
		if toolPath, source, err := pkgmgr.ResolveDoctorToolPath(root, tool); err != nil {
			check("warn", "tool "+tool, "not found in PATH or ./bin")
		} else {
			version := pkgmgr.DoctorToolVersion(toolPath)
			check("ok", "tool "+tool, source+": "+toolPath+" ("+version+")")
		}
	}

	if goPath, err := exec.LookPath("go"); err == nil {
		check("ok", "go toolchain", goPath)
	} else {
		check("warn", "go toolchain", "not found in PATH; building from source will fail")
	}

	var loadedManifest *manifest.Manifest
	hasManifest := false
	manifestPath := filepath.Join(root, "bak.toml")
	if _, err := os.Stat(manifestPath); err == nil {
		m, err := manifest.Load(manifestPath)
		if err != nil {
			check("fail", "bak.toml", err.Error())
		} else {
			loadedManifest = m
			hasManifest = true
			mode := m.LanguageMode
			if mode == "" {
				mode = manifest.LanguageModeFrozen
			}
			check("ok", "bak.toml", "language_mode="+mode)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		check("warn", "bak.toml", "not found; commands default to frozen language mode")
	} else {
		check("fail", "bak.toml", err.Error())
	}

	requiredStdlib := []string{
		"src/std/result.bak",
		"src/std/collections/vec.bak",
		"src/std/strings/strings.bak",
		"src/std/fs/fs.bak",
		"src/std/os/os.bak",
	}
	for _, rel := range requiredStdlib {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			check("fail", rel, "missing or unreadable")
		} else {
			check("ok", rel, "")
		}
	}

	if _, err := os.Stat(filepath.Join(root, "examples", "hello.bak")); err != nil {
		check("warn", "examples/hello.bak", "missing; example smoke checks may fail")
	} else {
		check("ok", "examples/hello.bak", "")
		if err := pkgmgr.DoctorSmokeCheckBakFile(filepath.Join(root, "examples", "hello.bak")); err != nil {
			check("warn", "examples/hello.bak smoke", err.Error())
		} else {
			check("ok", "examples/hello.bak smoke", "parse/typecheck passed")
		}
	}

	if _, err := os.Stat(filepath.Join(root, "bak.lock")); err == nil {
		lock, err := manifest.LoadLockfileFromDir(root)
		if err != nil {
			check("fail", "bak.lock", err.Error())
		} else {
			check("ok", "bak.lock", fmt.Sprintf("%d locked package(s)", len(lock.Packages)))
			if hasManifest {
				if err := manifest.ValidateLockfileIntegrity(lock, loadedManifest); err != nil {
					check("fail", "bak.lock integrity", err.Error())
				} else {
					check("ok", "bak.lock integrity", "manifest-aligned dependency set")
				}
				if err := pkgmgr.ValidateFrozenLockfile(root, lock); err != nil {
					check("fail", "manifest/lock coherence", err.Error())
				} else {
					check("ok", "manifest/lock coherence", "bak.lock matches bak.toml dependency constraints")
				}
			}
			cacheChecks := pkgmgr.DoctorLockCacheChecks(root, lock)
			for _, c := range cacheChecks {
				check(c.Status, c.Name, c.Detail)
			}
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if hasManifest && len(loadedManifest.Dependencies) > 0 {
			check("warn", "bak.lock", "missing with declared dependencies; run 'bak install' or 'bak get <pkg>'")
		} else {
			check("warn", "bak.lock", "not found; dependency installs are not pinned")
		}
	} else {
		check("fail", "bak.lock", err.Error())
	}

	if !ok {
		return fmt.Errorf("doctor checks failed")
	}
	return nil
}
