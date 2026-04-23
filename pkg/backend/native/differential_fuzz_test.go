package native

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baxromumarov/bak/pkg/runtimecap"
)

func TestDifferentialParitySeeds(t *testing.T) {
	for i := 0; i < 12; i++ {
		i := i
		t.Run(fmt.Sprintf("seed_%02d", i), func(t *testing.T) {
			seed := differentialSeed(i)
			source := buildDifferentialProgram(seed)
			assertDifferentialParityFromSource(t, source)
		})
	}
}

func FuzzEvaluatorVMNativeDifferential(f *testing.F) {
	for i := 0; i < 8; i++ {
		f.Add(differentialSeed(i))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}
		if len(data) > 64 {
			data = data[:64]
		}
		source := buildDifferentialProgram(data)
		assertDifferentialParityFromSource(t, source)
	})
}

func assertDifferentialParityFromSource(t *testing.T, source string) {
	t.Helper()

	sourcePath := filepath.Join(t.TempDir(), "differential_parity.bak")
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatalf("writing generated source: %v", err)
	}

	perms := runtimecap.Permissions{}
	evaluatorExit, evaluatorOut := runEvaluatorProgramFromFileWithOutput(t, sourcePath, perms)
	vmExit, vmOut := runVMProgramFromFileWithOutput(t, sourcePath, perms)
	nativeExit, nativeOut := runNativeProgramFromFileWithOutput(t, sourcePath, perms)

	if evaluatorExit != vmExit {
		t.Fatalf("evaluator/VM exit mismatch: evaluator=%d vm=%d\nsource:\n%s", evaluatorExit, vmExit, source)
	}
	if vmExit != nativeExit {
		t.Fatalf("VM/native exit mismatch: vm=%d native=%d\nsource:\n%s", vmExit, nativeExit, source)
	}

	if evaluatorOut != vmOut {
		t.Fatalf("evaluator/VM output mismatch\n--- evaluator ---\n%s\n--- vm ---\n%s\nsource:\n%s", evaluatorOut, vmOut, source)
	}
	if vmOut != nativeOut {
		t.Fatalf("VM/native output mismatch\n--- vm ---\n%s\n--- native ---\n%s\nsource:\n%s", vmOut, nativeOut, source)
	}
}

func buildDifferentialProgram(data []byte) string {
	if len(data) == 0 {
		data = []byte{0}
	}
	if len(data) > 48 {
		data = data[:48]
	}

	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("func main() -> (int) {\n")
	b.WriteString("    mut var v: Vec<int, _> = Vec.new()\n")
	b.WriteString("    var s: string = \"abc\"\n")
	b.WriteString("    mut var score: int = 0\n")
	b.WriteString("    score = score + (v.len() - v.len())\n")
	b.WriteString("    score = score + (s.len() - s.len())\n")

	for i, by := range data {
		idx := int(by%7) - 1 // [-1..5]
		val := int(by%20) + 1
		switch by % 8 {
		case 0:
			fmt.Fprintf(&b, "    v.push(%d)\n", val)
			fmt.Fprintf(&b, "    println(\"op%d:push\")\n", i)
			b.WriteString("    score = score + 1\n")
		case 1:
			fmt.Fprintf(&b, "    switch v.pop() {\n")
			fmt.Fprintf(&b, "        case Ok(x) { println(\"op%d:pop:ok\"); score = score + x }\n", i)
			fmt.Fprintf(&b, "        case Err(err) { println(\"op%d:pop:err\"); score = score + err.len() }\n", i)
			b.WriteString("    }\n")
		case 2:
			fmt.Fprintf(&b, "    switch v.first() {\n")
			fmt.Fprintf(&b, "        case Ok(x) { println(\"op%d:first:ok\"); score = score + x }\n", i)
			fmt.Fprintf(&b, "        case Err(err) { println(\"op%d:first:err\"); score = score + err.len() }\n", i)
			b.WriteString("    }\n")
		case 3:
			fmt.Fprintf(&b, "    switch v.last() {\n")
			fmt.Fprintf(&b, "        case Ok(x) { println(\"op%d:last:ok\"); score = score + x }\n", i)
			fmt.Fprintf(&b, "        case Err(err) { println(\"op%d:last:err\"); score = score + err.len() }\n", i)
			b.WriteString("    }\n")
		case 4:
			fmt.Fprintf(&b, "    switch v.get(%d) {\n", idx)
			fmt.Fprintf(&b, "        case Ok(x) { println(\"op%d:get:ok\"); score = score + x }\n", i)
			fmt.Fprintf(&b, "        case Err(err) { println(\"op%d:get:err\"); score = score + err.len() }\n", i)
			b.WriteString("    }\n")
		case 5:
			fmt.Fprintf(&b, "    switch v.remove(%d) {\n", idx)
			fmt.Fprintf(&b, "        case Ok(x) { println(\"op%d:remove:ok\"); score = score + x }\n", i)
			fmt.Fprintf(&b, "        case Err(err) { println(\"op%d:remove:err\"); score = score + err.len() }\n", i)
			b.WriteString("    }\n")
		case 6:
			fmt.Fprintf(&b, "    switch s.get(%d) {\n", idx)
			fmt.Fprintf(&b, "        case Ok(ch) {\n")
			fmt.Fprintf(&b, "            println(\"op%d:string:get:ok\")\n", i)
			b.WriteString("            if ch == char('a') { score = score + 1 } else { if ch == char('b') { score = score + 2 } else { score = score + 3 } }\n")
			b.WriteString("        }\n")
			fmt.Fprintf(&b, "        case Err(err) { println(\"op%d:string:get:err\"); score = score + err.len() }\n", i)
			b.WriteString("    }\n")
		default:
			fmt.Fprintf(&b, "    var r%d: Result<int, string> = v.get(%d)\n", i, idx)
			fmt.Fprintf(&b, "    if r%d.isOk() {\n", i)
			fmt.Fprintf(&b, "        println(\"op%d:result:isOk\")\n", i)
			fmt.Fprintf(&b, "        score = score + r%d.unwrap()\n", i)
			b.WriteString("    } else {\n")
			fmt.Fprintf(&b, "        println(\"op%d:result:isErr\")\n", i)
			fmt.Fprintf(&b, "        score = score + r%d.unwrapErr().len()\n", i)
			b.WriteString("    }\n")
		}
	}

	b.WriteString("    println(\"final\")\n")
	b.WriteString("    mut var exit_code: int = score % 200\n")
	b.WriteString("    if exit_code < 0 { exit_code = 0 - exit_code }\n")
	b.WriteString("    return exit_code\n")
	b.WriteString("}\n")
	return b.String()
}

func differentialSeed(i int) []byte {
	// Deterministic xorshift64* stream.
	x := uint64(i+1) * 0x9E3779B97F4A7C15
	out := make([]byte, 24)
	for idx := range out {
		x ^= x >> 12
		x ^= x << 25
		x ^= x >> 27
		x *= 2685821657736338717
		out[idx] = byte(x >> 56)
	}
	return out
}
