package manifest

import (
	"path/filepath"
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
