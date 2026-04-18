package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/baxromumarov/bak/pkg/lexer"
	"github.com/baxromumarov/bak/pkg/parser"
	"github.com/baxromumarov/bak/pkg/typechecker"
)

type diag struct {
	code string
	msg  string
	line int
	col  int
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	passDir := filepath.Join(root, "tests", "pass")
	failDir := filepath.Join(root, "tests", "fail")

	passFiles, err := collectBakFiles(passDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	failFiles, err := collectBakFiles(failDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var failed bool

	for _, path := range passFiles {
		goDiags, err := goTypecheck(path)
		if err != nil {
			fail("pass", path, err.Error())
			failed = true
			continue
		}
		bakcDiags, err := bakcTypecheck(root, path)
		if err != nil {
			fail("pass", path, err.Error())
			failed = true
			continue
		}

		if len(goDiags) != 0 || len(bakcDiags) != 0 {
			fail("pass", path, fmt.Sprintf("expected no errors; go=%d bakc=%d", len(goDiags), len(bakcDiags)))
			failed = true
		}
	}

	for _, path := range failFiles {
		expCode, expPrefix, err := readExpect(path)
		if err != nil {
			fail("fail", path, err.Error())
			failed = true
			continue
		}

		goDiags, err := goTypecheck(path)
		if err != nil {
			fail("fail", path, err.Error())
			failed = true
			continue
		}
		bakcDiags, err := bakcTypecheck(root, path)
		if err != nil {
			fail("fail", path, err.Error())
			failed = true
			continue
		}

		goMatch, ok := findGoMatch(goDiags, expPrefix)
		if !ok {
			fail("fail", path, "go typechecker did not emit expected message prefix")
			failed = true
			continue
		}
		bakcMatch, ok := findBakcMatch(bakcDiags, expCode, expPrefix)
		if !ok {
			fail("fail", path, "bakc did not emit expected code/message prefix")
			failed = true
			continue
		}
		if goMatch.line != bakcMatch.line || goMatch.col != bakcMatch.col {
			fail("fail", path, fmt.Sprintf("span mismatch: go=%d:%d bakc=%d:%d",
				goMatch.line, goMatch.col, bakcMatch.line, bakcMatch.col))
			failed = true
		}
	}

	if failed {
		os.Exit(1)
	}
	fmt.Println("bakc parity tests: all green")
}

func collectBakFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		if strings.HasSuffix(ent.Name(), ".bak") {
			out = append(out, filepath.Join(dir, ent.Name()))
		}
	}
	return out, nil
}

func goTypecheck(path string) ([]diag, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p := parser.New(lexer.New(string(source)))
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return nil, fmt.Errorf("parse errors: %s", strings.Join(p.Errors(), "; "))
	}
	tc := typechecker.NewWithPath(path)
	errs := tc.Check(program)
	return parseGoDiagnostics(errs), nil
}

func parseGoDiagnostics(errs []string) []diag {
	var out []diag
	// regex to strip ANSI color escape sequences
	ansiRe := regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	for _, e := range errs {
		lines := strings.Split(e, "\n")
		if len(lines) < 1 {
			continue
		}
		// Extract message after the closing bracket "]: ", e.g. "ERROR [E0100]: message"
		header := strings.TrimSpace(lines[0])
		// remove ANSI codes
		header = ansiRe.ReplaceAllString(header, "")
		msg := header
		if _, after, ok := strings.Cut(header, "]: "); ok {
			msg = strings.TrimSpace(after)
		}

		// Find a line that contains the location arrow "-->"
		var locLine string
		if len(lines) > 1 {
			for i := 1; i < len(lines); i++ {
				l := lines[i]
				l = strings.TrimSpace(ansiRe.ReplaceAllString(l, ""))
				if strings.Contains(l, "-->") {
					locLine = l
					break
				}
			}
		}
		line, col := parseGoLineCol(locLine)
		out = append(out, diag{msg: msg, line: line, col: col})
	}
	return out
}

func parseGoLineCol(line string) (int, int) {
	if line == "" {
		return 0, 0
	}
	// Locate the --> marker and then parse the trailing file:line:col
	_, after, ok := strings.Cut(line, "-->")
	if !ok {
		// Try to fallback to any digits in the string
		return 0, 0
	}

	after = strings.TrimSpace(after)
	// after should be like: file:path:line:col or file:line:col
	// Split by ':' and take last two parts as line and col
	parts := strings.Split(after, ":")
	if len(parts) < 2 {
		return 0, 0
	}
	// Last is col, second last is line
	colStr := strings.TrimSpace(parts[len(parts)-1])
	lineStr := strings.TrimSpace(parts[len(parts)-2])
	ln, _ := strconv.Atoi(lineStr)
	col, _ := strconv.Atoi(colStr)
	return ln, col
}

func bakcTypecheck(root, path string) ([]diag, error) {
	var cmd *exec.Cmd
	if _, err := os.Stat(filepath.Join(root, "bak")); err == nil {
		cmd = exec.Command(filepath.Join(root, "bak"), "src/compiler/cmd/bakc/main.bak", path)
	} else {
		cmd = exec.Command("go", "run", "./cmd/bak", "src/compiler/cmd/bakc/main.bak", path)
	}
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return nil, err
	}
	return parseBakcDiagnostics(output), nil
}

func parseBakcDiagnostics(out []byte) []diag {
	var diags []diag
	scanner := bufio.NewScanner(bytes.NewReader(out))
	var current *diag
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "error[") || strings.HasPrefix(line, "warning[") {
			codeEnd := strings.Index(line, "]")
			if codeEnd > 6 {
				code := line[len("error["):codeEnd]
				msg := strings.TrimSpace(line[codeEnd+2:])
				current = &diag{code: code, msg: msg}
				diags = append(diags, *current)
			}
			continue
		}
		if strings.HasPrefix(line, "-->") && current != nil {
			colon := strings.LastIndex(line, ":")
			if colon > 0 {
				before := line[:colon]
				last := strings.LastIndex(before, ":")
				if last > 0 {
					lineNum, _ := strconv.Atoi(strings.TrimSpace(before[last+1:]))
					colNum, _ := strconv.Atoi(strings.TrimSpace(line[colon+1:]))
					// Only set the primary location if it hasn't been set yet
					if current.line == 0 {
						current.line = lineNum
						current.col = colNum
						diags[len(diags)-1] = *current
					}
				}
			}
		}
	}
	return diags
}

func readExpect(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "// expect:"); ok {
			rest := strings.TrimSpace(after)
			parts := strings.SplitN(rest, " ", 2)
			if len(parts) != 2 {
				return "", "", fmt.Errorf("invalid expect format")
			}
			return parts[0], parts[1], nil
		}
	}
	return "", "", fmt.Errorf("missing expect directive")
}

func findGoMatch(diags []diag, prefix string) (diag, bool) {
	for _, d := range diags {
		if strings.HasPrefix(d.msg, prefix) {
			return d, true
		}
	}
	return diag{}, false
}

func findBakcMatch(diags []diag, code, prefix string) (diag, bool) {
	for _, d := range diags {
		if d.code == code && strings.HasPrefix(d.msg, prefix) {
			return d, true
		}
	}
	return diag{}, false
}

func fail(kind, path, msg string) {
	fmt.Printf("%s %s: %s\n", kind, filepath.Base(path), msg)
}
