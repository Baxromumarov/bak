// bak - A statically typed, compiled systems programming language
// focused on explicitness, predictability, and developer productivity.
//
// Usage:
//
//	bak <file.bak>           Run a bak program
//	bak                      Start the REPL
//	bak new <name>           Create a new project
//	bak init <name>          Alias for bak new
//	bak --version            Show version information
//	bak --help               Show help
package main

import (
	"bufio"
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
	"strconv"
	"strings"
	"time"

	"github.com/baxromumarov/bak/pkg/ast"
	"github.com/baxromumarov/bak/pkg/backend/native"
	"github.com/baxromumarov/bak/pkg/bytecodejson"
	"github.com/baxromumarov/bak/pkg/compiler"
	"github.com/baxromumarov/bak/pkg/diagnostics"
	"github.com/baxromumarov/bak/pkg/evaluator"
	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/manifest"
	"github.com/baxromumarov/bak/pkg/object"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/runtimecap"
	"github.com/baxromumarov/bak/pkg/trace"
	"github.com/baxromumarov/bak/pkg/typechecker"
	"github.com/baxromumarov/bak/pkg/vm"
)

const VERSION = "1.0.0"

const LOGO = `
  ____          _    
 |  _ \        | |   
 | |_) |  __ _ | | __
 |  _ <  / _' || |/ /
 | |_) || (_| ||   < 
 |____/  \__,_||_|\_\
                     
`

type packageCommandOptions struct {
	Offline        bool
	FrozenLockfile bool
}

func main() {
	args := os.Args[1:]
	var scriptArgs []string

	// Scan for -- separator first
	dashIndex := -1
	for i, arg := range args {
		if arg == "--" {
			dashIndex = i
			break
		}
	}

	var interpreterArgs []string
	if dashIndex >= 0 {
		interpreterArgs = args[:dashIndex]
		scriptArgs = args[dashIndex+1:]
	} else {
		interpreterArgs = args
	}

	permissions, interpreterArgs, err := parseRuntimePermissions(interpreterArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	experimentalFeatures, interpreterArgs, err := parseExperimentalFeatures(interpreterArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	interpreterArgs, traceEnabled := stripTraceFlag(interpreterArgs)

	if len(interpreterArgs) == 0 {
		startREPL(permissions, experimentalFeatures)
		return
	}

	// Check for --vm flag and separator --
	useVM := false
	var filename string

	for _, arg := range interpreterArgs {
		if arg == "--vm" {
			useVM = true
		} else if strings.HasSuffix(arg, ".bak") && filename == "" {
			filename = arg
		}
	}

	// Update args to be processed by switch
	args = interpreterArgs
	if len(args) == 0 {
		startREPL(permissions, experimentalFeatures)
		return
	}

	// Support simple subcommands: run/check/build/new/init
	switch args[0] {
	case "run":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: 'run' requires a file argument")
			os.Exit(1)
		}
		if traceEnabled {
			runFileVM(args[1], scriptArgs, traceEnabled, permissions, experimentalFeatures)
			return
		}
		runFile(args[1], scriptArgs, permissions, experimentalFeatures)
		return
	case "check":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: 'check' requires a file argument")
			os.Exit(1)
		}
		checkFile(args[1], experimentalFeatures)
		return
	case "native":
		// Parse native args: bak native <file.bak> -o <output>
		outputFile := ""
		sourceFile := ""
		for i := 1; i < len(args); i++ {
			if args[i] == "-o" && i+1 < len(args) {
				outputFile = args[i+1]
				i++
			} else if !strings.HasPrefix(args[i], "-") {
				sourceFile = args[i]
			}
		}
		if sourceFile == "" {
			fmt.Fprintln(os.Stderr, "Error: 'native' requires a file argument")
			os.Exit(1)
		}
		buildFile(sourceFile, outputFile, true, traceEnabled, permissions, experimentalFeatures)
		return
	case "build":
		// bak build works like go build: produces a native executable by default.
		// Usage: bak build [-o output] <file.bak|.>
		outputFile := ""
		sourceFile := ""
		for i := 1; i < len(args); i++ {
			if args[i] == "-o" && i+1 < len(args) {
				outputFile = args[i+1]
				i++
			} else if args[i] == "--native" {
				// accepted for backward compat but native is now default
				continue
			} else if !strings.HasPrefix(args[i], "-") {
				sourceFile = args[i]
			}
		}
		if sourceFile == "" || sourceFile == "." {
			// Like `go build .` - find main.bak in current directory
			sourceFile = findMainBak(".")
			if sourceFile == "" {
				fmt.Fprintln(os.Stderr, "Error: no main.bak found in current directory")
				os.Exit(1)
			}
		}
		if outputFile == "" {
			// Derive output name from source file, like go build
			base := filepath.Base(sourceFile)
			outputFile = strings.TrimSuffix(base, ".bak")
			if outputFile == base {
				outputFile = "a.out"
			}
		}
		buildFile(sourceFile, outputFile, true, traceEnabled, permissions, experimentalFeatures)
		return
	case "new", "init":
		projectName := "my-project"
		if len(args) >= 2 {
			projectName = args[1]
		}
		if err := initProject(projectName); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	case "get":
		opts, rest, err := parsePackageCommandOptions(args[1:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if len(rest) != 1 {
			fmt.Fprintln(os.Stderr, "Error: 'get' requires exactly one package argument")
			os.Exit(1)
		}
		getPackage(rest[0], opts)
		return
	case "install":
		opts, rest, err := parsePackageCommandOptions(args[1:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if len(rest) != 0 {
			fmt.Fprintln(os.Stderr, "Error: 'install' does not accept positional arguments")
			os.Exit(1)
		}
		installDependencies(opts)
		return
	case "test":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: 'test' requires a file or directory argument")
			os.Exit(1)
		}
		runTests(args[1], permissions, experimentalFeatures)
		return

	case "doc":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: 'doc' requires a file argument")
			os.Exit(1)
		}
		generateDocs(args[1])
		return
	case "--version", "-v":
		fmt.Printf("bak version %s\n", VERSION)
		return
	case "--help", "-h":
		printHelp()
		return
	case "--bc":
		modulePath, programArgs, explicitArgs, profileEnabled, err := splitBytecodeArgs(args)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if !explicitArgs {
			programArgs = append([]string{modulePath}, programArgs...)
		}
		runBytecodeFile(modulePath, programArgs, profileEnabled, traceEnabled, permissions, experimentalFeatures)
	case "--vm":
		if len(args) > 1 && strings.HasSuffix(args[1], ".bak") {
			runFileVM(args[1], scriptArgs, traceEnabled, permissions, experimentalFeatures)
		} else {
			fmt.Fprintf(os.Stderr, "Error: --vm requires a .bak file\n")
			os.Exit(1)
		}
	default:
		if filename != "" {
			if useVM || traceEnabled {
				runFileVM(filename, scriptArgs, traceEnabled, permissions, experimentalFeatures)
			} else {
				runFile(filename, scriptArgs, permissions, experimentalFeatures)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error: unknown argument or file type: %s\n", args[0])
			fmt.Fprintf(os.Stderr, "Use 'bak --help' for usage information.\n")
			os.Exit(1)
		}
	}
}

func printHelp() {
	fmt.Println("bak - A systems programming language focused on explicitness")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  bak build [flags] <file.bak|.>   Compile to native executable (like go build)")
	fmt.Println("  bak run <file.bak>               Interpret a bak program")
	fmt.Println("  bak check <file.bak>             Typecheck only")
	fmt.Println("  bak test <file|dir>              Run tests (like go test)")
	fmt.Println("  bak init [name]                  Initialize a new project")
	fmt.Println("  bak get [flags] <pkg[@ver]>      Add a dependency and pin it in bak.lock")
	fmt.Println("  bak install [flags]              Install dependencies from bak.lock")
	fmt.Println("  bak                              Start the REPL")
	fmt.Println()
	fmt.Println("Build flags:")
	fmt.Println("  -o <file>              Output file name")
	fmt.Println("  --trace                Enable built-in function tracing")
	fmt.Println()
	fmt.Println("Runtime permission flags:")
	fmt.Println("  --allow-exec           Allow subprocess execution via os.exec")
	fmt.Println("  --allow-net            Allow network and database access")
	fmt.Println("  --allow-fs-mutate      Allow destructive filesystem operations")
	fmt.Println("  --allow-all            Allow all dangerous runtime capabilities")
	fmt.Println("  --exec-timeout <dur>   Limit os.exec runtime; default 30s; direct exec only")
	fmt.Println("  --exec-max-output-bytes <n>")
	fmt.Println("                         Limit captured os.exec stdout/stderr; default 1048576")
	fmt.Println()
	fmt.Println("Dependency flags:")
	fmt.Println("  --offline              Use only cached packages; do not fetch from git")
	fmt.Println("  --frozen-lockfile      Refuse operations that would change bak.lock")
	fmt.Println()
	fmt.Println("Experimental language flags:")
	fmt.Println("  --experimental <list>  Enable experimental features outside frozen v0.1: unsafe, box, user-generics, traits")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  bak --allow-exec run main.bak    Run with subprocess access enabled")
	fmt.Println("  bak --allow-exec --exec-timeout 2s run main.bak")
	fmt.Println("  bak --allow-all --vm main.bak    Run on the VM with all capabilities")
	fmt.Println("  bak build main.bak               Build main.bak -> ./main")
	fmt.Println("  bak build -o myapp main.bak      Build main.bak -> ./myapp")
	fmt.Println("  bak build .                      Build main.bak in current directory")
	fmt.Println("  bak run main.bak                 Interpret main.bak")
	fmt.Println("  bak --experimental=box run main.bak")
	fmt.Println("  bak get github.com/u/repo@1.2.3 Add a versioned dependency")
	fmt.Println("  bak install --offline            Install from cache only")
	fmt.Println()
	fmt.Println("For more information, visit: https://github.com/baxromumarov/bak")
}

func runFile(filename string, scriptArgs []string, permissions runtimecap.Permissions, cliFeatures []string) {
	permissions = loadProjectRuntimePermissions(permissions, cliFeatures)
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		os.Exit(1)
	}

	env := object.NewEnvironment()
	restorePermissions := runtimecap.SetCurrent(permissions)
	defer restorePermissions()
	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
	}()

	// Set os.Args for the script (builtins use this)
	// Preserve os.Args[0] as executable name (or use filename)
	// We can't easily change os.Args[0] of the host process without weirdness,
	// but we can replace the slice.
	// The convention is [program_name, arg1, arg2...]
	if len(scriptArgs) > 0 {
		os.Args = append([]string{filename}, scriptArgs...)
	} else {
		// If no script args, just expose filename
		os.Args = []string{filename}
	}

	result := run(string(data), env, filename)

	if result != nil {
		switch result.Type() {
		case object.ERROR_OBJ, object.PANIC_OBJ:
			fmt.Fprintf(os.Stderr, "%s\n", result.Inspect())
			os.Exit(1)
		}
	}
}

func runFileVM(filename string, scriptArgs []string, traceEnabled bool, permissions runtimecap.Permissions, cliFeatures []string) {
	permissions = loadProjectRuntimePermissions(permissions, cliFeatures)
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		os.Exit(1)
	}

	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
	}()
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
		printParserErrors(p.Diagnostics())
		os.Exit(1)
	}

	// Inject Prelude
	for _, w := range injectPrelude(program) {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	// Type check
	tc := typechecker.NewWithPath(filename)
	typeErrors := tc.Check(program)
	if len(typeErrors) > 0 {
		fmt.Fprintf(os.Stderr, "Type errors:\n")
		for _, msg := range typeErrors {
			fmt.Fprintf(os.Stderr, "  %s\n", msg)
		}
		if tc.HasErrors() {
			os.Exit(1)
		}
	}

	// Compile to bytecode
	c := compiler.New()
	module, compileErr := c.Compile(program)
	if compileErr != nil {
		fmt.Fprintf(os.Stderr, "Compilation error: %s\n", compileErr)
		os.Exit(1)
	}

	// Run the VM
	v := vm.NewWithPermissions(module, permissions)
	v.SetTracer(trace.New(traceEnabled, os.Stderr))
	_, vmErr := v.Run()
	if vmErr != nil {
		fmt.Fprintf(os.Stderr, "Runtime error: %s\n", vmErr)
		os.Exit(1)
	}
}

func checkFile(filename string, cliFeatures []string) {
	loadProjectRuntimePermissions(runtimecap.Permissions{}, cliFeatures)
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		os.Exit(1)
	}

	l := lexer.New(string(data))
	p := parser.New(l)
	p.SetFilename(filename)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		printParserErrors(p.Diagnostics())
		os.Exit(1)
	}

	tc := typechecker.NewWithPath(filename)
	typeErrors := tc.Check(program)
	if len(typeErrors) > 0 {
		fmt.Fprintf(os.Stderr, "Type errors:\n")
		for _, msg := range typeErrors {
			fmt.Fprintf(os.Stderr, "  %s\n", msg)
		}
		os.Exit(1)
	}

	fmt.Println("Typecheck: OK")
}

func buildFile(filename string, outputFile string, nativeBuild bool, traceEnabled bool, permissions runtimecap.Permissions, cliFeatures []string) {
	permissions = loadProjectRuntimePermissions(permissions, cliFeatures)
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		os.Exit(1)
	}

	l := lexer.New(string(data))
	p := parser.New(l)
	p.SetFilename(filename)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		printParserErrors(p.Diagnostics())
		os.Exit(1)
	}

	// Type check
	tc := typechecker.NewWithPath(filename)
	typeErrors := tc.Check(program)
	if len(typeErrors) > 0 {
		fmt.Fprintf(os.Stderr, "Type errors:\n")
		for _, msg := range typeErrors {
			fmt.Fprintf(os.Stderr, "  %s\n", msg)
		}
		if tc.HasErrors() {
			os.Exit(1)
		}
	}

	if nativeBuild {
		exe, err := native.BuildExecutableWithOptions(program, native.BuildOptions{
			Permissions:  permissions,
			TraceEnabled: traceEnabled,
			MainPath:     filename,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Native build error: %s\n", err)
			os.Exit(1)
		}

		if outputFile == "" {
			outputFile = "a.out"
		}

		err = os.WriteFile(outputFile, exe, 0755)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output: %s\n", err)
			os.Exit(1)
		}

		fmt.Printf("Built native: %s\n", outputFile)
		return
	}

	// Compile
	c := compiler.New()
	module, compileErr := c.Compile(program)
	if compileErr != nil {
		fmt.Fprintf(os.Stderr, "Compilation error: %s\n", compileErr)
		os.Exit(1)
	}

	// Determine output path
	if outputFile == "" {
		// Default: replace .bak with .json
		if strings.HasSuffix(filename, ".bak") {
			outputFile = filename[:len(filename)-4] + ".json"
		} else {
			outputFile = filename + ".json"
		}
	}

	// Serialize to JSON
	jsonData, err := bytecodejson.Serialize(module)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Serialization error: %s\n", err)
		os.Exit(1)
	}

	// Write output
	err = os.WriteFile(outputFile, jsonData, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Built: %s\n", outputFile)
}

// runTests runs tests for a file or directory. If given a directory it
// discovers *_test.bak files (fallback to all .bak) and runs each.
func runTests(path string, permissions runtimecap.Permissions, cliFeatures []string) {
	permissions = loadProjectRuntimePermissions(permissions, cliFeatures)
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error stating path: %s\n", err)
		os.Exit(1)
	}
	if info.IsDir() {
		var files []string
		walkErr := filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasSuffix(p, "_test.bak") {
				files = append(files, p)
			}
			return nil
		})
		if walkErr != nil {
			fmt.Fprintf(os.Stderr, "Error walking directory %s: %s\n", path, walkErr)
			os.Exit(1)
		}
		if len(files) == 0 {
			walkErr = filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					return nil
				}
				if strings.HasSuffix(p, ".bak") {
					files = append(files, p)
				}
				return nil
			})
			if walkErr != nil {
				fmt.Fprintf(os.Stderr, "Error walking directory %s: %s\n", path, walkErr)
				os.Exit(1)
			}
		}
		sort.Strings(files)
		if len(files) == 0 {
			fmt.Fprintf(os.Stderr, "No .bak files found in %s\n", path)
			os.Exit(1)
		}
		var failed int = 0
		for _, f := range files {
			if !runTestFile(f, permissions) {
				failed = failed + 1
			}
		}
		if failed != 0 {
			os.Exit(1)
		}
		return
	}
	if !runTestFile(path, permissions) {
		os.Exit(1)
	}
}

// runTestFile compiles a single .bak file and, if it contains a
// `run_all_tests` function, sets that as the entrypoint and executes it.
func runTestFile(filename string, permissions runtimecap.Permissions) bool {
	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		return false
	}

	src := string(data)

	// Parse original source to discover tests and their arity.
	l := lexer.New(src)
	p := parser.New(l)
	origProgram := p.ParseProgram()

	if len(p.Errors()) != 0 {
		fmt.Fprintf(os.Stderr, "Parse errors in %s:\n", filename)
		for _, msg := range p.Errors() {
			fmt.Fprintf(os.Stderr, "  %s\n", msg)
		}
		return false
	}

	type testInfo struct {
		name  string
		arity int
	}
	var tests []testInfo
	for _, stmt := range origProgram.Statements {
		if fn, ok := stmt.(*ast.FunctionDecl); ok {
			if strings.HasPrefix(fn.Name.Value, "test_") {
				tests = append(tests, testInfo{name: fn.Name.Value, arity: len(fn.Parameters)})
			}
		}
	}

	if len(tests) == 0 {
		fmt.Fprintf(os.Stderr, "No test functions found in %s\n", filename)
		return false
	}

	// Build a wrapper appended to the source that runs discovered tests.
	var b strings.Builder
	b.WriteString(src)
	b.WriteString("\n\n// GENERATED test runner\n")
	b.WriteString("pub func run_all_tests() -> (void) {\n")
	b.WriteString(fmt.Sprintf("    test.set_prefix(\"%s\")\n", filename))
	b.WriteString("    mut var results: Vec<test.TestResult, _> = Vec.new()\n")
	for i, name := range tests {
		idx := i + 1
		testLabel := filename + ":" + name.name
		if name.arity == 0 {
			b.WriteString(fmt.Sprintf("    %s()\n", name.name))
			b.WriteString(fmt.Sprintf("    var lr%d: Option<test.TestResult> = test.take_last_result()\n", idx))
			b.WriteString("    switch lr" + fmt.Sprintf("%d", idx) + " {\n")
			b.WriteString(fmt.Sprintf("        case Some(r%d) { results.push(r%d) }\n", idx, idx))
			b.WriteString(fmt.Sprintf("        case None { results.push(test.fail_result(\"%s\", \"test did not call t.finish()\")) }\n", testLabel))
			b.WriteString("    }\n")
		} else {
			b.WriteString(fmt.Sprintf("    mut var t%d: test.T = test.new(\"%s\")\n", idx, testLabel))
			b.WriteString(fmt.Sprintf("    %s(&mut t%d)\n", name.name, idx))
			b.WriteString(fmt.Sprintf("    var r%d: test.TestResult = test.finish(&t%d)\n", idx, idx))
			b.WriteString(fmt.Sprintf("    results.push(r%d)\n", idx))
		}
	}
	b.WriteString("    test.run_tests(results)\n")
	b.WriteString("    test.clear_prefix()\n")
	b.WriteString("}\n")

	combined := b.String()

	// Parse combined source
	l = lexer.New(combined)
	p = parser.New(l)
	program := p.ParseProgram()

	// Inject prelude
	for _, w := range injectPrelude(program) {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	if len(p.Errors()) != 0 {
		fmt.Fprintf(os.Stderr, "Parse errors in generated runner for %s:\n", filename)
		for _, msg := range p.Errors() {
			fmt.Fprintf(os.Stderr, "  %s\n", msg)
		}
		return false
	}

	// Type check combined program
	tc := typechecker.NewWithPath(filename)
	tc.SetSuppressUnused(true)
	typeErrors := tc.Check(program)
	if len(typeErrors) > 0 {
		fmt.Fprintf(os.Stderr, "Type errors in generated runner for %s:\n", filename)
		for _, msg := range typeErrors {
			fmt.Fprintf(os.Stderr, "  %s\n", msg)
		}
		return false
	}

	// Compile to bytecode for VM execution
	c := compiler.New()
	module, err := c.Compile(program)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Compilation error in %s: %s\n", filename, err)
		return false
	}

	// Find and set run_all_tests as entry point
	runIndex := -1
	for i, fn := range module.Functions {
		if fn.Name == "run_all_tests" {
			runIndex = i
			break
		}
	}

	if runIndex < 0 {
		fmt.Fprintf(os.Stderr, "Error: run_all_tests not found in %s\n", filename)
		return false
	}

	// If we have an init function, create a wrapper entry to run init then tests.
	initIndex := -1
	for i, fn := range module.Functions {
		if fn.Name == "__bak_init" {
			initIndex = i
		}
	}
	if initIndex >= 0 {
		entryFn := &compiler.FunctionObj{
			Name:      "__bak_test_entry",
			Arity:     0,
			Code:      []byte{},
			Constants: []compiler.Value{},
			SourceMap: make(map[int]compiler.SourcePos),
		}
		emit := func(op compiler.Opcode) {
			entryFn.Code = append(entryFn.Code, byte(op))
		}
		emitShort := func(val int) {
			entryFn.Code = append(entryFn.Code, byte(val>>8), byte(val))
		}
		emit(compiler.OP_GET_FUNC)
		emitShort(initIndex)
		emit(compiler.OP_CALL)
		entryFn.Code = append(entryFn.Code, 0)
		emit(compiler.OP_POP)
		emit(compiler.OP_GET_FUNC)
		emitShort(runIndex)
		emit(compiler.OP_CALL)
		entryFn.Code = append(entryFn.Code, 0)
		emit(compiler.OP_RETURN)
		entryFn.NumLocals = 0
		entryIndex := module.AddFunction(entryFn)
		module.EntryPoint = entryIndex
	} else {
		module.EntryPoint = runIndex
	}

	// Run on VM
	v := vm.NewWithPermissions(module, permissions)
	_, err = v.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Test failure in %s: %s\n", filename, err)
		return false
	}

	return true
}

func splitBytecodeArgs(args []string) (string, []string, bool, bool, error) {
	if len(args) < 2 {
		return "", nil, false, false, fmt.Errorf("--bc requires a .json module path")
	}

	profileEnabled := false
	modulePath := ""
	var programArgs []string
	explicitArgs := false

	i := 1
	for i < len(args) {
		arg := args[i]
		if arg == "--profile" {
			profileEnabled = true
			i++
			continue
		}
		if arg == "--" {
			programArgs = args[i+1:]
			explicitArgs = true
			break
		}
		if modulePath == "" {
			modulePath = arg
		} else {
			programArgs = args[i:]
			break
		}
		i++
	}

	if modulePath == "" {
		return "", nil, false, false, fmt.Errorf("--bc requires a .json module path")
	}
	return modulePath, programArgs, explicitArgs, profileEnabled, nil
}

func runBytecodeFile(filename string, programArgs []string, profileEnabled bool, traceEnabled bool, permissions runtimecap.Permissions, cliFeatures []string) {
	permissions = loadProjectRuntimePermissions(permissions, cliFeatures)
	oldArgs := os.Args
	if len(programArgs) > 0 {
		os.Args = append([]string{oldArgs[0]}, programArgs...)
	} else {
		os.Args = []string{oldArgs[0]}
	}
	defer func() {
		os.Args = oldArgs
	}()

	module, err := bytecodejson.LoadModuleFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading bytecode module: %s\n", err)
		os.Exit(1)
	}

	v := vm.NewWithProfileAndPermissions(module, profileEnabled, permissions)
	v.SetTracer(trace.New(traceEnabled, os.Stderr))
	_, vmErr := v.Run()
	if profileEnabled {
		v.PrintProfile()
	}
	if vmErr != nil {
		fmt.Fprintf(os.Stderr, "Runtime error: %s\n", vmErr)
		os.Exit(1)
	}
}

func stripTraceFlag(args []string) ([]string, bool) {
	filtered := make([]string, 0, len(args))
	traceEnabled := false
	for _, arg := range args {
		if arg == "--trace" {
			traceEnabled = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered, traceEnabled
}

func startREPL(permissions runtimecap.Permissions, cliFeatures []string) {
	permissions = loadProjectRuntimePermissions(permissions, cliFeatures)
	fmt.Print(LOGO)
	fmt.Printf("bak v%s - Interactive Mode\n", VERSION)
	fmt.Println("Type 'exit' or 'quit' to exit, 'help' for help.")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	env := object.NewEnvironment()
	restorePermissions := runtimecap.SetCurrent(permissions)
	defer restorePermissions()

	for {
		fmt.Print("bak> ")
		scanned := scanner.Scan()
		if !scanned {
			return
		}

		line := strings.TrimSpace(scanner.Text())

		switch line {
		case "exit", "quit":
			fmt.Println("Goodbye!")
			return
		case "help":
			printREPLHelp()
			continue
		case "clear":
			env = object.NewEnvironment()
			fmt.Println("Environment cleared.")
			continue
		case "":
			continue
		}

		result := run(line, env, "<repl>")
		if result != nil {
			// Don't print VOID for statements that don't produce values
			if result.Type() != object.VOID_OBJ {
				fmt.Println(result.Inspect())
			}
		}
	}
}

func parseRuntimePermissions(args []string) (runtimecap.Permissions, []string, error) {
	var permissions runtimecap.Permissions
	var rest []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case runtimecap.FlagAllowExec:
			permissions.AllowExec = true
		case runtimecap.FlagAllowNet:
			permissions.AllowNet = true
		case runtimecap.FlagAllowFSMutate:
			permissions.AllowFSMutate = true
		case runtimecap.FlagAllowAll:
			permissions = runtimecap.AllPermissions()
		case runtimecap.FlagExecTimeout:
			i++
			if i >= len(args) {
				return runtimecap.Permissions{}, nil, fmt.Errorf("%s requires a duration value", runtimecap.FlagExecTimeout)
			}
			dur, err := time.ParseDuration(args[i])
			if err != nil {
				return runtimecap.Permissions{}, nil, fmt.Errorf("invalid %s value %q: %w", runtimecap.FlagExecTimeout, args[i], err)
			}
			if dur <= 0 {
				return runtimecap.Permissions{}, nil, fmt.Errorf("%s must be greater than zero", runtimecap.FlagExecTimeout)
			}
			permissions.ExecTimeout = dur
		case runtimecap.FlagExecMaxOutput:
			i++
			if i >= len(args) {
				return runtimecap.Permissions{}, nil, fmt.Errorf("%s requires a byte limit", runtimecap.FlagExecMaxOutput)
			}
			limit, err := strconv.ParseInt(args[i], 10, 64)
			if err != nil {
				return runtimecap.Permissions{}, nil, fmt.Errorf("invalid %s value %q: %w", runtimecap.FlagExecMaxOutput, args[i], err)
			}
			if limit <= 0 {
				return runtimecap.Permissions{}, nil, fmt.Errorf("%s must be greater than zero", runtimecap.FlagExecMaxOutput)
			}
			permissions.ExecMaxOutput = limit
		default:
			if value, ok := strings.CutPrefix(arg, runtimecap.FlagExecTimeout+"="); ok {
				dur, err := time.ParseDuration(value)
				if err != nil {
					return runtimecap.Permissions{}, nil, fmt.Errorf("invalid %s value %q: %w", runtimecap.FlagExecTimeout, value, err)
				}
				if dur <= 0 {
					return runtimecap.Permissions{}, nil, fmt.Errorf("%s must be greater than zero", runtimecap.FlagExecTimeout)
				}
				permissions.ExecTimeout = dur
				continue
			}
			if value, ok := strings.CutPrefix(arg, runtimecap.FlagExecMaxOutput+"="); ok {
				limit, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return runtimecap.Permissions{}, nil, fmt.Errorf("invalid %s value %q: %w", runtimecap.FlagExecMaxOutput, value, err)
				}
				if limit <= 0 {
					return runtimecap.Permissions{}, nil, fmt.Errorf("%s must be greater than zero", runtimecap.FlagExecMaxOutput)
				}
				permissions.ExecMaxOutput = limit
				continue
			}
			rest = append(rest, arg)
		}
	}

	return permissions, rest, nil
}

func parseExperimentalFeatures(args []string) ([]string, []string, error) {
	features := []string{}
	rest := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--experimental":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--experimental requires a feature list")
			}
			parsed, err := parseExperimentalFeatureList(args[i+1])
			if err != nil {
				return nil, nil, err
			}
			features = append(features, parsed...)
			i++
		case strings.HasPrefix(arg, "--experimental="):
			parsed, err := parseExperimentalFeatureList(strings.TrimPrefix(arg, "--experimental="))
			if err != nil {
				return nil, nil, err
			}
			features = append(features, parsed...)
		default:
			rest = append(rest, arg)
		}
	}

	return mergeFeatureLists(nil, features), rest, nil
}

func parseExperimentalFeatureList(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	features := make([]string, 0, len(parts))
	for _, part := range parts {
		feature, err := canonicalExperimentalFeature(part)
		if err != nil {
			return nil, err
		}
		features = append(features, feature)
	}
	return features, nil
}

func canonicalExperimentalFeature(name string) (string, error) {
	switch strings.TrimSpace(name) {
	case "unsafe", runtimecap.ExperimentalFeatureUnsafe:
		return runtimecap.ExperimentalFeatureUnsafe, nil
	case "box", runtimecap.ExperimentalFeatureBox:
		return runtimecap.ExperimentalFeatureBox, nil
	case "user-generics", runtimecap.ExperimentalFeatureUserGenerics:
		return runtimecap.ExperimentalFeatureUserGenerics, nil
	case "traits", runtimecap.ExperimentalFeatureTraits:
		return runtimecap.ExperimentalFeatureTraits, nil
	default:
		return "", fmt.Errorf("unknown experimental feature %q (expected one of: unsafe, box, user-generics, traits)", name)
	}
}

func mergeFeatureLists(base []string, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	merged := make([]string, 0, len(base)+len(extra))
	for _, feature := range append(append([]string(nil), base...), extra...) {
		feature = strings.TrimSpace(feature)
		if feature == "" {
			continue
		}
		if _, ok := seen[feature]; ok {
			continue
		}
		seen[feature] = struct{}{}
		merged = append(merged, feature)
	}
	sort.Strings(merged)
	return merged
}

func loadProjectRuntimePermissions(base runtimecap.Permissions, cliFeatures []string) runtimecap.Permissions {
	m, err := manifest.LoadFromDir(".")
	if err != nil {
		if os.IsNotExist(err) {
			runtimecap.SetCurrentFeatures(cliFeatures)
			return base
		}
		runtimecap.SetCurrentFeatures(cliFeatures)
		fmt.Fprintf(os.Stderr, "Error loading runtime permissions from bak.toml: %v\n", err)
		os.Exit(1)
	}
	runtimecap.SetCurrentFeatures(mergeFeatureLists(m.Features, cliFeatures))
	if m == nil || m.Permissions == nil {
		return base
	}

	permissions := runtimecap.Permissions{
		AllowExec:     m.Permissions.AllowExec,
		AllowNet:      m.Permissions.AllowNet,
		AllowFSMutate: m.Permissions.AllowFSMutate,
		ExecMaxOutput: m.Permissions.ExecMaxOutput,
	}
	if m.Permissions.ExecTimeout != "" {
		dur, err := time.ParseDuration(m.Permissions.ExecTimeout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading runtime permissions from bak.toml: %v\n", fmt.Errorf("invalid permissions.exec_timeout %q: %w", m.Permissions.ExecTimeout, err))
			os.Exit(1)
		}
		permissions.ExecTimeout = dur
	}
	return mergeRuntimePermissions(base, permissions)
}

func loadRuntimePermissionsFromManifest(dir string) (runtimecap.Permissions, error) {
	m, err := manifest.LoadFromDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return runtimecap.Permissions{}, nil
		}
		return runtimecap.Permissions{}, err
	}
	if m == nil || m.Permissions == nil {
		return runtimecap.Permissions{}, nil
	}

	permissions := runtimecap.Permissions{
		AllowExec:     m.Permissions.AllowExec,
		AllowNet:      m.Permissions.AllowNet,
		AllowFSMutate: m.Permissions.AllowFSMutate,
		ExecMaxOutput: m.Permissions.ExecMaxOutput,
	}
	if m.Permissions.ExecTimeout != "" {
		dur, err := time.ParseDuration(m.Permissions.ExecTimeout)
		if err != nil {
			return runtimecap.Permissions{}, fmt.Errorf("invalid permissions.exec_timeout %q: %w", m.Permissions.ExecTimeout, err)
		}
		permissions.ExecTimeout = dur
	}
	return permissions, nil
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

func printREPLHelp() {
	fmt.Println("bak REPL Commands:")
	fmt.Println("  exit, quit    Exit the REPL")
	fmt.Println("  clear         Clear the environment")
	fmt.Println("  help          Show this help message")
	fmt.Println()
	fmt.Println("Example code:")
	fmt.Println("  var x int = 10")
	fmt.Println("  mut var y int = 20")
	fmt.Println("  println(x + y)")
	fmt.Println()
	fmt.Println("  func add(a int, b int) -> (int) { return a + b }")
	fmt.Println("  add(5, 3)")
}

func run(input string, env *object.Environment, filename string) object.Object {
	l := lexer.New(input)
	p := parser.New(l)
	p.SetFilename(filename)

	program := p.ParseProgram()

	if len(p.Errors()) != 0 {
		printParserErrors(p.Diagnostics())
		return nil
	}

	// Inject prelude (HashMap, etc)
	for _, w := range injectPrelude(program) {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	// Run static type checker before evaluation
	tc := typechecker.NewWithPath(filename)
	typeErrors := tc.Check(program)
	if len(typeErrors) != 0 {
		printTypeErrors(typeErrors)
		if tc.HasErrors() {
			return nil
		}
	}

	evaluator.ResetState()
	return evaluator.Eval(program, env)
}

func printParserErrors(diags []diagnostics.Diagnostic) {
	fmt.Fprintln(os.Stderr, "Parser errors:")
	for _, d := range diags {
		fmt.Fprint(os.Stderr, d.Format())
		fmt.Fprintln(os.Stderr)
	}
}

func printTypeErrors(errors []string) {
	fmt.Fprintln(os.Stderr, "Type errors:")
	for _, msg := range errors {
		fmt.Fprintf(os.Stderr, "  %s\n", msg)
	}
}

// initProject creates a new Bak project with bak.toml and starter files.
func initProject(name string) error {
	projectDir := filepath.Clean(name)
	projectTitle := projectDisplayName(projectDir)
	packageName := sanitizeProjectName(projectTitle)

	fmt.Printf("Initializing project %q...\n", projectDir)

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("create project directory: %w", err)
	}

	srcDir := filepath.Join(projectDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return fmt.Errorf("create src directory: %w", err)
	}

	mainPath := filepath.Join(srcDir, "main.bak")
	mainContent := "package main\n\nfunc main() -> (void) {\n    println(\"Hello, world!\")\n}\n"
	if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
		return fmt.Errorf("create main.bak: %w", err)
	}

	readmePath := filepath.Join(projectDir, "README.md")
	readmeContent := fmt.Sprintf("# %s\n\nStarter Bak project generated with bak new.\n\n## Layout\n\n- src/main.bak: entry point\n- bak.toml: package metadata and permissions\n- .gitignore: local build and cache ignores\n\n## Run\n\n```bash\nbak run src/main.bak\n```\n\n## Format and lint\n\n```bash\nbakfmt src\nbaklint src\n```\n", projectTitle)
	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		return fmt.Errorf("create README.md: %w", err)
	}

	gitIgnorePath := filepath.Join(projectDir, ".gitignore")
	gitIgnoreContent := ".bak-cache/\n*.bakc\na.out\nbin/\nbuild/\n"
	if err := os.WriteFile(gitIgnorePath, []byte(gitIgnoreContent), 0644); err != nil {
		return fmt.Errorf("create .gitignore: %w", err)
	}

	m := manifest.DefaultManifest(packageName)
	if err := m.SaveToDir(projectDir); err != nil {
		return fmt.Errorf("create bak.toml: %w", err)
	}

	fmt.Printf("Project %q created!\n", projectDir)
	fmt.Println("Created files:")
	fmt.Println("  bak.toml")
	fmt.Println("  README.md")
	fmt.Println("  src/main.bak")
	fmt.Println("  .gitignore")
	fmt.Println()
	fmt.Printf("To run: cd %s && bak run src/main.bak\n", projectDir)
	return nil
}

func projectDisplayName(path string) string {
	base := filepath.Base(filepath.Clean(path))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "my-project"
	}
	return base
}

func sanitizeProjectName(name string) string {
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			lastUnderscore = false
		case r >= '0' && r <= '9':
			if builder.Len() == 0 {
				builder.WriteString("project_")
			}
			builder.WriteRune(r)
			lastUnderscore = false
		default:
			if builder.Len() > 0 && !lastUnderscore {
				builder.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "project"
	}
	if result[0] >= '0' && result[0] <= '9' {
		result = "project_" + result
	}
	return result
}

func parsePackageCommandOptions(args []string) (packageCommandOptions, []string, error) {
	var opts packageCommandOptions
	var rest []string
	for _, arg := range args {
		switch arg {
		case "--offline":
			opts.Offline = true
		case "--frozen-lockfile":
			opts.FrozenLockfile = true
		default:
			if strings.HasPrefix(arg, "-") {
				return packageCommandOptions{}, nil, fmt.Errorf("unknown dependency flag: %s", arg)
			}
			rest = append(rest, arg)
		}
	}
	return opts, rest, nil
}

// getPackage fetches and installs a package from a Git repository.
func getPackage(pkgArg string, opts packageCommandOptions) {
	if opts.FrozenLockfile {
		fmt.Fprintln(os.Stderr, "Error: 'bak get' cannot be used with --frozen-lockfile because it updates bak.lock")
		os.Exit(1)
	}
	if opts.Offline {
		fmt.Fprintln(os.Stderr, "Error: 'bak get --offline' cannot resolve a new dependency without network access")
		os.Exit(1)
	}

	// Load or create manifest
	m, err := manifest.LoadFromDir(".")
	if err != nil {
		// Create new manifest if not exists
		cwd, cwdErr := getCwd()
		if cwdErr != nil {
			fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", cwdErr)
			os.Exit(1)
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

	// Parse package path
	// Format: github.com/user/repo or user/repo (assumes github.com)
	fullPath := pkgPath
	isExplicitURL := strings.Contains(pkgPath, "://") || strings.HasPrefix(pkgPath, "/") || strings.HasPrefix(pkgPath, "git@")

	if !isExplicitURL && !strings.Contains(pkgPath, ".") {
		fullPath = "github.com/" + pkgPath
	}

	// Validate against trusted source allowlist
	if err := manifest.ValidateSourceAllowed(fullPath, m.TrustedSources); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Package name is the last part of the path (ignoring .git extension if present)
	parts := strings.Split(strings.TrimSuffix(fullPath, ".git"), "/")
	pkgName := parts[len(parts)-1]
	if pkgName == "" && len(parts) > 1 {
		pkgName = parts[len(parts)-2]
	}

	// Create cache directory
	cacheDir := ".bak-cache/pkg"
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating cache directory: %v\n", err)
		os.Exit(1)
	}

	gitURL := fullPath
	if !isExplicitURL {
		gitURL = "https://" + fullPath + ".git"
	}

	fmt.Printf("Fetching %s (%s)...\n", fullPath, requestedVersion)
	lockedPkg, err := fetchAndCacheLockedPackage(cacheDir, pkgName, fullPath, gitURL, requestedVersion)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching package: %v\n", err)
		os.Exit(1)
	}

	// Add to manifest
	m.AddDependency(pkgName, manifest.Dependency{
		Git:     fullPath,
		Version: requestedVersion,
	})

	// Save manifest
	if err := m.SaveToDir("."); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving bak.toml: %v\n", err)
		os.Exit(1)
	}

	// Create/update lockfile
	lock, _ := manifest.LoadLockfileFromDir(".")
	lock.AddPackage(pkgName, lockedPkg)
	if err := lock.SaveToDir("."); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving bak.lock: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Added %s @ %s (%s)\n", pkgName, lockedPkg.Version, shortCommit(lockedPkg.Commit))
}

// resolveVersion finds the commit hash for a requested version (tag or "latest").
func resolveVersion(repoDir, version string) (string, string, error) {
	if version == "latest" {
		out, err := gitOutput(repoDir, "rev-parse", "HEAD")
		if err != nil {
			return "", "", err
		}
		return strings.TrimSpace(string(out)), "latest", nil
	}

	// Try to find a tag matching the version (vX.Y.Z or X.Y.Z)
	// Naive implementation: exact match or with 'v' prefix
	candidates := []string{version, "v" + version}
	for _, tag := range candidates {
		if _, err := gitOutput(repoDir, "rev-parse", tag); err == nil {
			out, _ := gitOutput(repoDir, "rev-parse", tag)
			return strings.TrimSpace(string(out)), tag, nil
		}
	}

	return "", "", fmt.Errorf("version tag '%s' not found", version)
}

// installDependencies installs packages from bak.lock
func installDependencies(opts packageCommandOptions) {
	if !lockfileExists(".") {
		fmt.Fprintln(os.Stderr, "Error loading bak.lock: not found. Run 'bak get' to add dependencies first.")
		os.Exit(1)
	}

	lock, err := manifest.LoadLockfileFromDir(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading bak.lock: %v. Run 'bak get' to add dependencies first.\n", err)
		os.Exit(1)
	}

	m, err := manifest.LoadFromDir(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading bak.toml: %v\n", err)
		os.Exit(1)
	}

	// Validate lockfile integrity
	if err := manifest.ValidateLockfileIntegrity(lock, m); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if opts.FrozenLockfile {
		if err := validateFrozenLockfile(".", lock); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	cacheDir := ".bak-cache/pkg"
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating cache directory: %v\n", err)
		os.Exit(1)
	}

	names := sortedLockedPackageNames(lock)
	lockDirty := false
	for _, name := range names {
		pkg := lock.Packages[name]
		// Validate source against trusted allowlist
		if err := manifest.ValidateSourceAllowed(pkg.Source, m.TrustedSources); err != nil {
			fmt.Fprintf(os.Stderr, "Error installing %s: %v\n", pkg.Name, err)
			os.Exit(1)
		}
		normalizedPath := packageCachePath(cacheDir, pkg.Name, pkg.Source, pkg.Commit)
		if pkg.Path == "" || pkg.Path != normalizedPath {
			pkg.Path = normalizedPath
			lockDirty = true
		}
		checksum, err := ensureLockedPackage(cacheDir, pkg, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error installing %s: %v\n", pkg.Name, err)
			os.Exit(1)
		}
		if checksum != "" && pkg.Checksum != checksum {
			pkg.Checksum = checksum
			lockDirty = true
		}
		lock.Packages[name] = pkg
		fmt.Printf("Installed %s @ %s (%s)\n", pkg.Name, pkg.Version, shortCommit(pkg.Commit))
	}
	if lockDirty && !opts.FrozenLockfile {
		if err := lock.SaveToDir("."); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving bak.lock: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Println("Done.")
}

func lockfileExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "bak.lock"))
	return err == nil
}

func validateFrozenLockfile(dir string, lock *manifest.Lockfile) error {
	m, err := manifest.LoadFromDir(dir)
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
		if expected := strings.TrimSpace(dep.Version); expected != "" && !frozenLockfileVersionMatches(expected, pkg.Version) {
			return fmt.Errorf("bak.lock dependency %q is version %q, but bak.toml requires %q", name, pkg.Version, expected)
		}
	}
	return nil
}

func frozenLockfileVersionMatches(expected, actual string) bool {
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

func sortedLockedPackageNames(lock *manifest.Lockfile) []string {
	names := make([]string, 0, len(lock.Packages))
	for name := range lock.Packages {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func fetchAndCacheLockedPackage(cacheDir, pkgName, fullPath, gitURL, requestedVersion string) (manifest.LockedPackage, error) {
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
	checksum, err := directoryChecksum(tmpDir)
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

func ensureLockedPackage(cacheDir string, pkg manifest.LockedPackage, opts packageCommandOptions) (string, error) {
	targetPath := pkg.Path
	if targetPath == "" {
		targetPath = packageCachePath(cacheDir, pkg.Name, pkg.Source, pkg.Commit)
	}

	if _, err := os.Stat(targetPath); err == nil {
		checksum, err := directoryChecksum(targetPath)
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
	checksum, err := directoryChecksum(tmpDir)
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

func sourceToGitURL(source string) string {
	if strings.Contains(source, "://") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, "git@") {
		return source
	}
	return "https://" + strings.TrimSuffix(source, ".git") + ".git"
}

func packageCachePath(cacheDir, pkgName, source, commit string) string {
	safeName := sanitizeCacheName(pkgName)
	if safeName == "" {
		safeName = "pkg"
	}
	return filepath.Join(cacheDir, safeName+"-"+packageCacheKey(source, commit))
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

func makePackageTempDir(cacheDir string) (string, func(), error) {
	tmpRoot := filepath.Join(cacheDir, ".tmp")
	if err := os.MkdirAll(tmpRoot, 0755); err != nil {
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
	if err := os.MkdirAll(filepath.Dir(destDir), 0755); err != nil {
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

func directoryChecksum(root string) (string, error) {
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

func shortCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

func findMainBak(dir string) string {
	// Look for main.bak in the given directory
	mainPath := filepath.Join(dir, "main.bak")
	if _, err := os.Stat(mainPath); err == nil {
		return mainPath
	}
	// Also try looking for any .bak file with "package main" in the directory
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".bak") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		if strings.Contains(content, "package main") && strings.Contains(content, "func main()") {
			return path
		}
	}
	return ""
}

func getCwd() (string, error) {
	return os.Getwd()
}
