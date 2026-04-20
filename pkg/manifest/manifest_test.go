package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestManifestPermissionRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bak.toml")

	m := DefaultManifest("demo")
	m.LanguageMode = LanguageModeExperimental
	m.Features = []string{"experimental-cfg", "fast-path"}
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
	if loaded.LanguageMode != LanguageModeExperimental {
		t.Fatalf("unexpected language mode: %q", loaded.LanguageMode)
	}
	if !reflect.DeepEqual(loaded.Features, []string{"experimental-cfg", "fast-path"}) {
		t.Fatalf("unexpected features: %#v", loaded.Features)
	}
}

func TestDefaultManifestLanguageModeIsFrozen(t *testing.T) {
	m := DefaultManifest("demo")
	if m.LanguageMode != LanguageModeFrozen {
		t.Fatalf("unexpected default language mode: %q", m.LanguageMode)
	}
}

func TestValidateSourceAllowed(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		trusted []string
		wantErr bool
	}{
		{"empty allowlist allows all", "github.com/evil/pkg", nil, false},
		{"exact match", "github.com/acme/pkg", []string{"github.com/acme/pkg"}, false},
		{"wildcard match", "github.com/acme/lib", []string{"github.com/acme/*"}, false},
		{"wildcard prefix only", "github.com/acme", []string{"github.com/acme/*"}, false},
		{"no match", "github.com/evil/pkg", []string{"github.com/acme/*"}, true},
		{"multiple patterns match", "github.com/acme/pkg", []string{"github.com/other/*", "github.com/acme/*"}, false},
		{"multiple patterns no match", "github.com/evil/pkg", []string{"github.com/other/*", "github.com/acme/*"}, true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSourceAllowed(tt.source, tt.trusted)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for source %q with trusted %v", tt.source, tt.trusted)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateLockfileIntegrity(t *testing.T) {
	m := DefaultManifest("demo")
	m.AddDependency("foo", Dependency{Git: "github.com/acme/foo"})

	validLock := NewLockfile()
	validLock.AddPackage("foo", LockedPackage{
		Name:     "foo",
		Source:   "github.com/acme/foo",
		Commit:   "abc123",
		Checksum: "sha256:deadbeef",
		Path:     ".bak-cache/pkg/foo-abc123",
	})

	if err := ValidateLockfileIntegrity(validLock, m); err != nil {
		t.Fatalf("expected valid lockfile to pass, got: %v", err)
	}

	// Missing commit
	badCommit := NewLockfile()
	badCommit.AddPackage("foo", LockedPackage{
		Name:     "foo",
		Source:   "github.com/acme/foo",
		Commit:   "",
		Checksum: "sha256:deadbeef",
		Path:     ".bak-cache/pkg/foo-abc123",
	})
	if err := ValidateLockfileIntegrity(badCommit, m); err == nil {
		t.Fatalf("expected error for empty commit")
	}

	// Missing checksum
	badChecksum := NewLockfile()
	badChecksum.AddPackage("foo", LockedPackage{
		Name:     "foo",
		Source:   "github.com/acme/foo",
		Commit:   "abc123",
		Checksum: "",
		Path:     ".bak-cache/pkg/foo-abc123",
	})
	if err := ValidateLockfileIntegrity(badChecksum, m); err == nil {
		t.Fatalf("expected error for empty checksum")
	}

	// Orphaned package
	orphan := NewLockfile()
	orphan.AddPackage("bar", LockedPackage{
		Name:     "bar",
		Source:   "github.com/acme/bar",
		Commit:   "def456",
		Checksum: "sha256:cafebabe",
		Path:     ".bak-cache/pkg/bar-def456",
	})
	if err := ValidateLockfileIntegrity(orphan, m); err == nil {
		t.Fatalf("expected error for orphaned package")
	}
}

func TestLoadManifestRejectsInvalidDependencyShapes(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantErr  string
	}{
		{
			name: "git and path",
			contents: `
[package]
name = "demo"
version = "0.1.0"

[dependencies.bad]
git = "github.com/acme/bad"
path = "../bad"
`,
			wantErr: `dependency "bad" cannot set both git and path`,
		},
		{
			name: "missing source",
			contents: `
[package]
name = "demo"
version = "0.1.0"

[dependencies.bad]
version = "1.0.0"
`,
			wantErr: `dependency "bad" must set either git or path`,
		},
		{
			name: "path with version",
			contents: `
[package]
name = "demo"
version = "0.1.0"

[dependencies.bad]
path = "../bad"
version = "1.0.0"
`,
			wantErr: `dependency "bad" cannot set version for a local path dependency`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bak.toml")
			if err := os.WriteFile(path, []byte(strings.TrimSpace(tt.contents)), 0o644); err != nil {
				t.Fatalf("write manifest: %v", err)
			}
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestLoadManifestRejectsInvalidPermissions(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantErr  string
	}{
		{
			name: "invalid timeout",
			contents: `
[package]
name = "demo"
version = "0.1.0"

[permissions]
exec_timeout = "later"
`,
			wantErr: `invalid permissions.exec_timeout`,
		},
		{
			name: "negative output",
			contents: `
[package]
name = "demo"
version = "0.1.0"

[permissions]
exec_max_output_bytes = -1
`,
			wantErr: `permissions.exec_max_output_bytes must be greater than or equal to zero`,
		},
		{
			name: "blank trusted source",
			contents: `
trusted_sources = ["  "]

[package]
name = "demo"
version = "0.1.0"
`,
			wantErr: `trusted_sources entries must not be empty`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bak.toml")
			if err := os.WriteFile(path, []byte(strings.TrimSpace(tt.contents)), 0o644); err != nil {
				t.Fatalf("write manifest: %v", err)
			}
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestLoadManifestRejectsInvalidFeatures(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantErr  string
	}{
		{
			name: "blank feature",
			contents: `
features = ["  "]

[package]
name = "demo"
version = "0.1.0"
`,
			wantErr: `features entries must not be empty`,
		},
		{
			name: "duplicate feature",
			contents: `
features = ["fast-path", "fast-path"]

[package]
name = "demo"
version = "0.1.0"
`,
			wantErr: `duplicate feature "fast-path" in bak.toml`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bak.toml")
			if err := os.WriteFile(path, []byte(strings.TrimSpace(tt.contents)), 0o644); err != nil {
				t.Fatalf("write manifest: %v", err)
			}
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestLoadManifestRejectsInvalidLanguageMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bak.toml")
	contents := `
language_mode = "unstable"

[package]
name = "demo"
version = "0.1.0"
`
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), `language_mode "unstable" is invalid`) {
		t.Fatalf("expected invalid language mode error, got %v", err)
	}
}
