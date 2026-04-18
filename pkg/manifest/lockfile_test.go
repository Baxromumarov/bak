package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLockfileRoundTripPreservesChecksum(t *testing.T) {
	dir := t.TempDir()
	lock := NewLockfile()
	lock.AddPackage("demo", LockedPackage{
		Name:     "demo",
		Version:  "1.2.3",
		Source:   "github.com/acme/demo",
		Commit:   "abcdef1234567890",
		Checksum: "deadbeef",
		Path:     ".bak-cache/pkg/demo-1234",
	})

	path := filepath.Join(dir, "bak.lock")
	if err := lock.Save(path); err != nil {
		t.Fatalf("saving lockfile: %v", err)
	}

	loaded, err := LoadLockfile(path)
	if err != nil {
		t.Fatalf("loading lockfile: %v", err)
	}

	pkg, ok := loaded.GetPackage("demo")
	if !ok {
		t.Fatalf("expected package to be present after reload")
	}
	if pkg.Checksum != "deadbeef" {
		t.Fatalf("expected checksum to round-trip, got %q", pkg.Checksum)
	}
}

func TestLoadLockfileRejectsMalformedContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bak.lock")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write lockfile: %v", err)
	}

	_, err := LoadLockfile(path)
	if err == nil {
		t.Fatalf("expected malformed json to fail")
	}
}

func TestLoadLockfileRejectsInvalidStructure(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantErr  string
	}{
		{
			name: "zero version",
			contents: `{
  "version": 0,
  "packages": {}
}`,
			wantErr: "lockfile version must be greater than zero",
		},
		{
			name: "mismatched package name",
			contents: `{
  "version": 1,
  "packages": {
    "demo": {
      "name": "other",
      "source": "github.com/acme/demo",
      "path": ".bak-cache/pkg/demo"
    }
  }
}`,
			wantErr: `lockfile package "demo" has mismatched name "other"`,
		},
		{
			name: "empty source",
			contents: `{
  "version": 1,
  "packages": {
    "demo": {
      "name": "demo",
      "source": "",
      "path": ".bak-cache/pkg/demo"
    }
  }
}`,
			wantErr: `lockfile package "demo" has empty source`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "bak.lock")
			if err := os.WriteFile(path, []byte(tt.contents), 0o644); err != nil {
				t.Fatalf("write lockfile: %v", err)
			}
			_, err := LoadLockfile(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
