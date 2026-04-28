package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/baxromumarov/bak/internal/config"
	"github.com/baxromumarov/bak/internal/driver"
	testpkg "github.com/baxromumarov/bak/internal/test"
	"github.com/baxromumarov/bak/pkg/manifest"
	"github.com/baxromumarov/bak/pkg/runtimecap"
)

// Minimal compatibility wrappers that forward to internal packages.

type packageCommandOptions struct {
	Offline        bool
	FrozenLockfile bool
}

type testCommandOptions struct {
	RunPattern     string
	PackageFilters map[string]struct{}
}

func parseRuntimePermissions(args []string) (runtimecap.Permissions, []string, error) {
	return config.ParseRuntimePermissions(args)
}

func parseExperimentalFeatures(args []string) ([]string, []string, error) {
	return config.ParseExperimentalFeatures(args)
}

func stripTraceFlag(args []string) ([]string, bool) {
	return config.StripTraceFlag(args)
}

func findMainBak(dir string) string {
	p := filepath.Join(dir, "main.bak")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func startREPL(permissions runtimecap.Permissions, features []string) {
	fmt.Fprintln(os.Stdout, "bak REPL (stub) — not implemented in test environment")
}

func runFile(
	filename string,
	scriptArgs []string,
	permissions runtimecap.Permissions,
	features []string,
) error {
	return driver.RunFile(
		filename,
		scriptArgs,
		permissions,
		features,
		false,
	)
}

func splitBytecodeArgs(args []string) (modulePath string, programArgs []string, explicitArgs bool, profileEnabled bool, err error) {
	if len(args) == 0 {
		return "", nil, false, false, fmt.Errorf("missing bytecode module path")
	}
	return args[0], args[1:], true, false, nil
}

func runBytecodeFile(modulePath string, programArgs []string, profileEnabled bool, traceEnabled bool, permissions runtimecap.Permissions, features []string) error {
	return driver.RunFileVM(modulePath, programArgs, traceEnabled, permissions, features)
}

func runFileVM(filename string, scriptArgs []string, traceEnabled bool, permissions runtimecap.Permissions, features []string) error {
	return driver.RunFileVM(filename, scriptArgs, traceEnabled, permissions, features)
}

func checkFile(filename string, features []string) error {
	return driver.CheckFile(filename, features)
}

type testFunctionInfo struct {
	name  string
	arity int
}

type testFileRunResult struct {
	Executed bool
	Passed   bool
}

func filterTestsByNamePattern(tests []testFunctionInfo, runPattern string) []testFunctionInfo {
	if runPattern == "" {
		return tests
	}
	filtered := make([]testFunctionInfo, 0, len(tests))
	for _, t := range tests {
		if strings.Contains(t.name, runPattern) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func filterTestFilesByPackage(files []string, packageFilters map[string]struct{}) ([]string, []error) {
	if len(packageFilters) == 0 {
		return files, nil
	}
	filterSet := make(map[string]struct{}, len(packageFilters))
	for name := range packageFilters {
		filterSet[name] = struct{}{}
	}

	filtered := make([]string, 0, len(files))
	errs := make([]error, 0)
	for _, file := range files {
		pkgName, err := packageNameFromFile(file)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
			continue
		}
		if _, ok := filterSet[pkgName]; ok {
			filtered = append(filtered, file)
		}
	}
	return filtered, errs
}

func packageNameFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
	}
	return "", fmt.Errorf("missing package declaration")
}

func buildFile(filename string, output string, native bool, traceEnabled bool, permissions runtimecap.Permissions, features []string) error {
	return driver.BuildFile(filename, output, native, traceEnabled, permissions, features)
}

func runTests(targets []string, permissions runtimecap.Permissions, features []string, opts testCommandOptions) error {
	var pf map[string]struct{}
	if len(opts.PackageFilters) > 0 {
		pf = opts.PackageFilters
	}
	return driver.RunTests(targets, permissions, features, pf, opts.RunPattern)
}

func getPackageNoExit(arg string, opts packageCommandOptions) error {
	return driver.GetPackageWithOptions(arg, driver.PackageOptions{Offline: opts.Offline, FrozenLockfile: opts.FrozenLockfile})
}

func installDependenciesNoExit(opts packageCommandOptions) error {
	return driver.InstallDependenciesWithOptions(driver.PackageOptions{Offline: opts.Offline, FrozenLockfile: opts.FrozenLockfile})
}

func parsePackageCommandOptions(args []string) (packageCommandOptions, []string, error) {
	var out packageCommandOptions
	var rest []string
	for _, a := range args {
		switch a {
		case "--offline":
			out.Offline = true
		case "--frozen-lockfile":
			out.FrozenLockfile = true
		default:
			if strings.HasPrefix(a, "-") {
				return packageCommandOptions{}, nil, fmt.Errorf("unknown dependency flag: %s", a)
			}
			rest = append(rest, a)
		}
	}
	return out, rest, nil
}

func parseTestCommandOptions(args []string) (testCommandOptions, []string, error) {
	var out testCommandOptions
	out.PackageFilters = make(map[string]struct{})
	rest := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--run" {
			if i+1 >= len(args) {
				return testCommandOptions{}, nil, fmt.Errorf("--run requires a pattern")
			}
			out.RunPattern = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(a, "--package=") {
			val := strings.TrimPrefix(a, "--package=")
			for _, p := range strings.Split(val, ",") {
				out.PackageFilters[p] = struct{}{}
			}
			continue
		}
		if a == "--package" {
			if i+1 >= len(args) {
				return testCommandOptions{}, nil, fmt.Errorf("--package requires a comma-separated list of packages")
			}
			for _, p := range strings.Split(args[i+1], ",") {
				out.PackageFilters[p] = struct{}{}
			}
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			return testCommandOptions{}, nil, fmt.Errorf("unknown test flag: %s", a)
		}
		rest = append(rest, a)
	}
	return out, rest, nil
}

func initProject(name string) error {
	if err := os.MkdirAll(name, 0o755); err != nil {
		return err
	}
	m := manifest.DefaultManifest(name)
	// Ensure package name is filesystem-safe (replace '-' with '_')
	base := filepath.Base(name)
	m.Package.Name = strings.ReplaceAll(base, "-", "_")
	m.LanguageMode = manifest.LanguageModeFrozen
	if err := m.SaveToDir(name); err != nil {
		return err
	}
	// Ensure README and .gitignore exist with helpful defaults
	readme := "# demo project\n\nUse `bak new` to scaffold new projects.\nThis project uses language_mode = \"frozen\" by default.\n"
	_ = os.WriteFile(filepath.Join(name, "README.md"), []byte(readme), 0o644)
	gitignore := ".bak-cache/\n# build outputs\na.out\n"
	_ = os.WriteFile(filepath.Join(name, ".gitignore"), []byte(gitignore), 0o644)
	return nil
}

func resolveProjectFeaturesByLanguageMode(languageMode string, manifestFeatures []string, cliFeatures []string) ([]string, error) {
	return config.ResolveProjectFeaturesByLanguageMode(languageMode, manifestFeatures, cliFeatures)
}

func resolveProjectFeatureState(cliFeatures []string) (*manifest.Manifest, []string, error) {
	return config.ResolveProjectFeatureState(cliFeatures)
}

func collectTestFilesForTargets(paths []string) ([]string, []error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}

	seen := make(map[string]struct{})
	files := make([]string, 0)
	pathErrors := make([]error, 0)

	for _, path := range paths {
		targetFiles, err := collectTestFiles(path)
		if err != nil {
			pathErrors = append(pathErrors, fmt.Errorf("%s: %w", path, err))
			continue
		}
		if len(targetFiles) == 0 {
			pathErrors = append(pathErrors, fmt.Errorf("%s: no .bak files found", path))
			continue
		}
		for _, file := range targetFiles {
			clean := filepath.Clean(file)
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			files = append(files, clean)
		}
	}

	sort.Strings(files)
	return files, pathErrors
}

func collectTestFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{path}, nil
	}

	var testFiles []string
	var bakFiles []string
	walkErr := filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, "_test.bak") {
			testFiles = append(testFiles, p)
			return nil
		}
		if strings.HasSuffix(p, ".bak") {
			bakFiles = append(bakFiles, p)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	if len(testFiles) > 0 {
		sort.Strings(testFiles)
		return testFiles, nil
	}

	sort.Strings(bakFiles)
	return bakFiles, nil
}

func loadRuntimePermissionsFromManifest(dir string) (runtimecap.Permissions, error) {
	m, err := manifest.LoadFromDir(dir)
	if err != nil {
		return runtimecap.Permissions{}, err
	}
	if m == nil || m.Permissions == nil {
		return runtimecap.Permissions{}, nil
	}
	p := runtimecap.Permissions{
		AllowExec:     m.Permissions.AllowExec,
		AllowNet:      m.Permissions.AllowNet,
		AllowFSMutate: m.Permissions.AllowFSMutate,
		ExecMaxOutput: m.Permissions.ExecMaxOutput,
	}
	if m.Permissions.ExecTimeout != "" {
		d, err := time.ParseDuration(m.Permissions.ExecTimeout)
		if err != nil {
			return runtimecap.Permissions{}, fmt.Errorf("invalid permissions.exec_timeout %q: %w", m.Permissions.ExecTimeout, err)
		}
		p.ExecTimeout = d
	}
	return p, nil
}

func mergeRuntimePermissions(base, extra runtimecap.Permissions) runtimecap.Permissions {
	base.AllowExec = base.AllowExec || extra.AllowExec
	base.AllowNet = base.AllowNet || extra.AllowNet
	base.AllowFSMutate = base.AllowFSMutate || extra.AllowFSMutate
	if base.ExecTimeout <= 0 {
		base.ExecTimeout = extra.ExecTimeout
	}

	if base.ExecMaxOutput <= 0 {
		base.ExecMaxOutput = extra.ExecMaxOutput
	}

	return base
}

func runTestFile(filename string, permissions runtimecap.Permissions, runPattern string) testFileRunResult {
	opts := testpkg.Options{Targets: []string{filename}, RunPattern: runPattern}
	err := testpkg.Run([]string{filename}, permissions, nil, opts)
	if err != nil {
		return testFileRunResult{Executed: true, Passed: false}
	}
	return testFileRunResult{Executed: true, Passed: true}
}

func explainDiagnosticCode(w io.Writer, code string) bool {
	catalog := map[string]string{
		"E0001": "missing package",
		"E0100": "use of moved value",
		"E0101": "borrow after move",
		"E0200": "mutability required",
		"E0300": "type mismatch",
	}
	c := strings.ToUpper(code)
	if title, ok := catalog[c]; ok {
		fmt.Fprintf(w, "%s: %s\n\n", c, title)
		fmt.Fprintf(w, "Description: %s\n", title)
		return true
	}
	fmt.Fprintf(w, "Unknown diagnostic code: %s\n\n", strings.ToUpper(code))
	fmt.Fprintln(w, "Try: bak explain --list for available codes")
	return false
}

func printDiagnosticCodeList(w io.Writer) {
	catalog := map[string]string{
		"E0001": "missing package",
		"E0100": "use of moved value",
		"E0101": "borrow after move",
		"E0200": "mutability required",
		"E0300": "type mismatch",
	}
	fmt.Fprintln(w, "Known diagnostic codes")
	// Print in sorted order for stable output
	keys := make([]string, 0, len(catalog))
	for k := range catalog {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "%s - %s\n", k, catalog[k])
	}
}

func runDoctor(w io.Writer, root string) bool {
	if err := driver.RunDoctor(w, root); err != nil {
		return false
	}
	return true
}
