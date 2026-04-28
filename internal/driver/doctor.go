package driver

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/strfmt"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

const VERSION = "dev"

// RunDoctor runs doctor checks and returns an error when checks fail.
func RunDoctor(w io.Writer, root string) error {
	ok := true
	check := func(status, name, detail string) {
		_, _ = strfmt.Fprint(w, "[", status, "] ", name)
		if detail != "" {
			_, _ = strfmt.Fprint(w, " - ", detail)
		}
		_, _ = strfmt.Fprintln(w)
		if status == "fail" {
			ok = false
		}
	}

	_, _ = strfmt.Fprintln(w, "Bak doctor")
	_, _ = strfmt.Fprintln(w, "version: ", VERSION)
	_, _ = strfmt.Fprintln(w, "host: ", runtime.GOOS, "/", runtime.GOARCH)

	absRoot, err := filepath.Abs(root)
	if err != nil {
		check("fail", "workspace", err.Error())
	} else {
		check("ok", "workspace", absRoot)
	}

	for _, tool := range []string{"bak", "bakfmt", "baklint", "bakcheck", "bak-lsp"} {
		if toolPath, source, err := resolveDoctorToolPath(root, tool); err != nil {
			check("warn", "tool "+tool, "not found in PATH or ./bin")
		} else {
			version := doctorToolVersion(toolPath)
			check("ok", "tool "+tool, source+": "+toolPath+" ("+version+")")
		}
	}

	if goPath, err := exec.LookPath("go"); err == nil {
		check("ok", "go toolchain", goPath)
	} else {
		check("warn", "go toolchain", "not found in PATH; building from source will fail")
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
		if err := doctorSmokeCheckBakFile(filepath.Join(root, "examples", "hello.bak")); err != nil {
			check("warn", "examples/hello.bak smoke", err.Error())
		} else {
			check("ok", "examples/hello.bak smoke", "parse/typecheck passed")
		}
	}

	if !ok {
		return fmt.Errorf("doctor checks failed")
	}
	return nil
}

func resolveDoctorToolPath(root, tool string) (string, string, error) {
	if toolPath, err := exec.LookPath(tool); err == nil {
		return toolPath, "PATH", nil
	}
	localPath := filepath.Join(root, "bin", tool)
	if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
		return localPath, "local", nil
	}
	return "", "", fmt.Errorf("tool %s not found", tool)
}

func doctorToolVersion(path string) string {
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

func doctorSmokeCheckBakFile(path string) error {
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
