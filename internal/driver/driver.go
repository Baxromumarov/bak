package driver

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/backend/native"
	"github.com/baxromumarov/bak/pkg/bytecodejson"
	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/manifest"

	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/trace"
	"github.com/baxromumarov/bak/pkg/typechecker"
	"github.com/baxromumarov/bak/pkg/vm"

	"github.com/baxromumarov/bak/internal/config"
	"github.com/baxromumarov/bak/internal/pkgmgr"
	testpkg "github.com/baxromumarov/bak/internal/test"
)

// RunFile executes a Bak source file; traceEnabled chooses VM path when true.
func RunFile(filename string, scriptArgs []string, permissions runtimecap.Permissions, cliFeatures []string, traceEnabled bool) error {
	// For now, don't attempt to load project-specific runtime permissions
	runtimecap.SetCurrentFeatures(nil)
	// avoid reading file or creating environment here; RunFile delegates to RunFileVM
	restorePermissions := runtimecap.SetCurrent(permissions)
	defer restorePermissions()
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	if len(scriptArgs) > 0 {
		os.Args = append([]string{filename}, scriptArgs...)
	} else {
		os.Args = []string{filename}
	}

	// Use VM path for execution (avoid relying on package-main helpers)
	return RunFileVM(filename, scriptArgs, traceEnabled, permissions, cliFeatures)
}

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

// GetPackageWithOptions exposes package fetching using an exported options type.
func GetPackageWithOptions(pkgArg string, opts PackageOptions) error {
	return GetPackage(pkgArg, packageCommandOptions{Offline: opts.Offline, FrozenLockfile: opts.FrozenLockfile})
}

// InstallDependenciesWithOptions exposes dependency installation using an exported options type.
func InstallDependenciesWithOptions(opts PackageOptions) error {
	return InstallDependencies(packageCommandOptions{Offline: opts.Offline, FrozenLockfile: opts.FrozenLockfile})
}

// injectPrelude is a best-effort no-op prelude injector for the driver.
// The real prelude injection lives in cmd/bak/utils.go (package main).
func injectPrelude(program *ast.Program) []string {
	return nil
}

func hasFatalTypeErrors(tc *typechecker.TypeChecker) bool {
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

// RunFileVM runs the VM for a source file.
func RunFileVM(filename string, scriptArgs []string, traceEnabled bool, permissions runtimecap.Permissions, cliFeatures []string) error {
	// For now, don't attempt to load project-specific runtime permissions
	runtimecap.SetCurrentFeatures(nil)
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read failed: %w", err)
	}

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	if len(scriptArgs) > 0 {
		os.Args = append([]string{filename}, scriptArgs...)
	} else {
		os.Args = []string{filename}
	}

	// Parse the source code
	l := lexer.New(string(data))
	p := parser.New(l)
	p.SetFilename(filename)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		return fmt.Errorf("parse failed: %s", p.Errors()[0])
	}

	// Inject Prelude (best-effort; driver provides a no-op helper)
	for _, w := range injectPrelude(program) {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	// Type check
	tc := typechecker.NewWithPath(filename)
	typeErrors := tc.Check(program)
	if len(typeErrors) > 0 {
		if hasFatalTypeErrors(tc) {
			return fmt.Errorf("typecheck failed: %s", typeErrors[0])
		}
		return fmt.Errorf("typecheck warnings: %v", typeErrors)
	}

	// Compile to bytecode
	c := compiler.New()
	module, compileErr := c.Compile(program)
	if compileErr != nil {
		return fmt.Errorf("compilation error: %w", compileErr)
	}

	// Run the VM
	v := vm.NewWithPermissions(module, permissions)
	v.SetTracer(trace.New(traceEnabled, os.Stderr))
	_, vmErr := v.Run()
	if vmErr != nil {
		return fmt.Errorf("runtime error: %w", vmErr)
	}
	return nil
}

// CheckFile typechecks a file and returns an error on failure.
func CheckFile(filename string, cliFeatures []string) error {
	config.LoadProjectRuntimePermissions(runtimecap.Permissions{}, cliFeatures)
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read failed: %w", err)
	}

	l := lexer.New(string(data))
	p := parser.New(l)
	p.SetFilename(filename)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		return fmt.Errorf("parse failed: %s", p.Errors()[0])
	}

	tc := typechecker.NewWithPath(filename)
	typeErrors := tc.Check(program)
	if len(typeErrors) > 0 {
		if hasFatalTypeErrors(tc) {
			return fmt.Errorf("typecheck failed: %s", typeErrors[0])
		}
		return fmt.Errorf("typecheck warnings: %v", typeErrors)
	}

	return nil
}

// BuildFile builds either native or bytecode output and returns errors.
func BuildFile(filename string, outputFile string, nativeBuild bool, traceEnabled bool, permissions runtimecap.Permissions, cliFeatures []string) error {
	permissions = config.LoadProjectRuntimePermissions(permissions, cliFeatures)
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read failed: %w", err)
	}

	l := lexer.New(string(data))
	p := parser.New(l)
	p.SetFilename(filename)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		return fmt.Errorf("parse failed: %s", p.Errors()[0])
	}

	// Type check
	tc := typechecker.NewWithPath(filename)
	typeErrors := tc.Check(program)
	if len(typeErrors) > 0 {
		if tc.HasErrors() {
			return fmt.Errorf("typecheck failed: %s", typeErrors[0])
		}
		return fmt.Errorf("typecheck warnings: %v", typeErrors)
	}

	if nativeBuild {
		exe, err := native.BuildExecutableWithOptions(program, native.BuildOptions{
			Permissions:  permissions,
			TraceEnabled: traceEnabled,
			MainPath:     filename,
		})
		if err != nil {
			return fmt.Errorf("native build error: %w", err)
		}

		if outputFile == "" {
			outputFile = "a.out"
		}

		if err := os.WriteFile(outputFile, exe, 0755); err != nil {
			return fmt.Errorf("write failed: %w", err)
		}

		fmt.Printf("Built native: %s\n", outputFile)
		return nil
	}

	// Compile
	c := compiler.New()
	module, compileErr := c.Compile(program)
	if compileErr != nil {
		return fmt.Errorf("compilation error: %w", compileErr)
	}

	// Determine output path
	if outputFile == "" {
		if strings.HasSuffix(filename, ".bak") {
			outputFile = filename[:len(filename)-4] + ".json"
		} else {
			outputFile = filename + ".json"
		}
	}

	jsonData, err := bytecodejson.Serialize(module)
	if err != nil {
		return fmt.Errorf("serialization error: %w", err)
	}

	if err := os.WriteFile(outputFile, jsonData, 0644); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	fmt.Printf("Built: %s\n", outputFile)
	return nil
}

// RunTests runs test files and returns an error on failures.
func RunTests(paths []string, permissions runtimecap.Permissions, cliFeatures []string, packageFilters map[string]struct{}, runPattern string) error {
	// Delegate discovery & execution to the internal/test package (AST-based runner).
	opts := testpkg.Options{Targets: paths, RunPattern: runPattern}
	if len(packageFilters) > 0 {
		pf := make([]string, 0, len(packageFilters))
		for k := range packageFilters {
			pf = append(pf, k)
		}
		opts.PackageFilters = pf
	}
	return testpkg.Run(paths, config.LoadProjectRuntimePermissions(permissions, cliFeatures), cliFeatures, opts)
}

const VERSION = "dev"

// GetPackage fetches and installs a package from a Git repository.
func GetPackage(pkgArg string, opts packageCommandOptions) error {
	if opts.FrozenLockfile {
		return fmt.Errorf("'bak get' cannot be used with --frozen-lockfile because it updates bak.lock")
	}
	if opts.Offline {
		return fmt.Errorf("'bak get --offline' cannot resolve a new dependency without network access")
	}

	// Load or create manifest
	m, err := manifest.LoadFromDir(".")
	if err != nil {
		cwd, cwdErr := pkgmgr.GetCwd()
		if cwdErr != nil {
			return fmt.Errorf("get cwd: %w", cwdErr)
		}
		m = manifest.DefaultManifest(filepath.Base(cwd))
	}

	// Parse version: package@version
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
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
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

	m.AddDependency(pkgName, manifest.Dependency{
		Git:     fullPath,
		Version: requestedVersion,
	})

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
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
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
