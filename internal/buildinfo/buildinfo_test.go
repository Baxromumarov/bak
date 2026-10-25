package buildinfo

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestFrozenV01SupportProfile(t *testing.T) {
	if LanguageVersion != "0.1" || LanguageStatus != "frozen" {
		t.Fatalf("unexpected language contract: version=%q status=%q", LanguageVersion, LanguageStatus)
	}
	if ProductionBackend != "vm" || VMStatus != "stable" {
		t.Fatalf("unexpected production backend contract: backend=%q status=%q", ProductionBackend, VMStatus)
	}
	if NativeStatus != "preview" || NativeTarget != "linux/amd64" {
		t.Fatalf("unexpected native support contract: status=%q target=%q", NativeStatus, NativeTarget)
	}
}

func TestWriteVersionJSON(t *testing.T) {
	var out bytes.Buffer
	if err := WriteVersion(&out, "bakfmt", true); err != nil {
		t.Fatal(err)
	}
	var info Info
	if err := json.Unmarshal(out.Bytes(), &info); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if info.Tool != "bakfmt" || info.LanguageVersion != "0.1" || info.ProductionBackend != "vm" {
		t.Fatalf("unexpected version payload: %+v", info)
	}
}

func TestWriteVersionHuman(t *testing.T) {
	var out bytes.Buffer
	if err := WriteVersion(&out, "bak-lsp", false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"bak-lsp dev", "language v0.1 (frozen)", "production backend: vm (stable)", "native backend: preview (linux/amd64)"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("version output missing %q:\n%s", want, out.String())
		}
	}
}
