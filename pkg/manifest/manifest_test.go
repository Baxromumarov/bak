package manifest

import (
	"path/filepath"
	"testing"
)

func TestManifestPermissionRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bak.toml")

	m := DefaultManifest("demo")
	m.Permissions = &RuntimePermissions{
		AllowExec:     true,
		AllowNet:      true,
		AllowFSMutate: true,
		ExecTimeout:   "5s",
		ExecMaxOutput: 4096,
	}

	if err := m.Save(path); err != nil {
		t.Fatalf("saving manifest: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("loading manifest: %v", err)
	}
	if loaded.Permissions == nil {
		t.Fatalf("expected permissions to round-trip")
	}
	if !loaded.Permissions.AllowExec || !loaded.Permissions.AllowNet || !loaded.Permissions.AllowFSMutate {
		t.Fatalf("unexpected permissions: %+v", loaded.Permissions)
	}
	if loaded.Permissions.ExecTimeout != "5s" {
		t.Fatalf("unexpected exec timeout: %q", loaded.Permissions.ExecTimeout)
	}
	if loaded.Permissions.ExecMaxOutput != 4096 {
		t.Fatalf("unexpected exec max output: %d", loaded.Permissions.ExecMaxOutput)
	}
}
