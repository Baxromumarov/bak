// Command build_release creates reproducible Bak toolchain archives.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/baxromumarov/bak/internal/buildinfo"
)

type target struct {
	OS   string
	Arch string
}

type tool struct {
	Name    string
	Package string
}

type archiveInfo struct {
	Name   string `json:"name"`
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	SHA256 string `json:"sha256"`
}

type releaseManifest struct {
	SchemaVersion     int           `json:"schemaVersion"`
	Product           string        `json:"product"`
	ToolchainVersion  string        `json:"toolchainVersion"`
	Commit            string        `json:"commit"`
	BuiltAt           string        `json:"builtAt"`
	LanguageVersion   string        `json:"languageVersion"`
	LanguageStatus    string        `json:"languageStatus"`
	ProductionBackend string        `json:"productionBackend"`
	VMStatus          string        `json:"vmStatus"`
	NativeStatus      string        `json:"nativeStatus"`
	NativeTarget      string        `json:"nativeTarget"`
	Archives          []archiveInfo `json:"archives,omitempty"`
}

var releaseTools = []tool{
	{Name: "bak", Package: "./cmd/bak"},
	{Name: "bakfmt", Package: "./cmd/bakfmt"},
	{Name: "baklint", Package: "./cmd/baklint"},
	{Name: "bakcheck", Package: "./cmd/bakcheck"},
	{Name: "bak-lsp", Package: "./lsp"},
}

var defaultTargets = []target{
	{OS: "linux", Arch: "amd64"},
	{OS: "linux", Arch: "arm64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "darwin", Arch: "arm64"},
	{OS: "windows", Arch: "amd64"},
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "build_release:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		outDir    = flag.String("out", "dist", "release output directory")
		version   = flag.String("version", "dev", "toolchain version")
		commit    = flag.String("commit", "unknown", "source commit")
		builtAt   = flag.String("date", "1980-01-01T00:00:00Z", "reproducible RFC3339 build time")
		targetCSV = flag.String("targets", "", "comma-separated GOOS/GOARCH targets")
	)
	flag.Parse()

	stamp, err := time.Parse(time.RFC3339, *builtAt)
	if err != nil {
		return fmt.Errorf("parse -date: %w", err)
	}
	if stamp.Year() < 1980 {
		stamp = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	stamp = stamp.UTC().Truncate(2 * time.Second)

	targets, err := parseTargets(*targetCSV)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "bak-release-")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	manifest := releaseManifest{
		SchemaVersion:     1,
		Product:           "bak",
		ToolchainVersion:  *version,
		Commit:            *commit,
		BuiltAt:           stamp.Format(time.RFC3339),
		LanguageVersion:   buildinfo.LanguageVersion,
		LanguageStatus:    buildinfo.LanguageStatus,
		ProductionBackend: buildinfo.ProductionBackend,
		VMStatus:          buildinfo.VMStatus,
		NativeStatus:      buildinfo.NativeStatus,
		NativeTarget:      buildinfo.NativeTarget,
	}

	for _, target := range targets {
		info, err := buildArchive(tempDir, *outDir, *version, *commit, stamp, target, manifest)
		if err != nil {
			return err
		}
		manifest.Archives = append(manifest.Archives, info)
		fmt.Printf("built %s\n", filepath.Join(*outDir, info.Name))
	}

	manifestBytes, err := marshalJSON(manifest)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*outDir, "release-manifest.json"), manifestBytes, 0o644); err != nil {
		return fmt.Errorf("write release manifest: %w", err)
	}

	var checksums strings.Builder
	for _, archive := range manifest.Archives {
		fmt.Fprintf(&checksums, "%s  %s\n", archive.SHA256, archive.Name)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "checksums.txt"), []byte(checksums.String()), 0o644); err != nil {
		return fmt.Errorf("write checksums: %w", err)
	}
	return nil
}

func parseTargets(value string) ([]target, error) {
	if strings.TrimSpace(value) == "" {
		return defaultTargets, nil
	}
	var targets []target
	seen := make(map[string]bool)
	for _, raw := range strings.Split(value, ",") {
		parts := strings.Split(strings.TrimSpace(raw), "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid target %q; want GOOS/GOARCH", raw)
		}
		key := parts[0] + "/" + parts[1]
		if !seen[key] {
			targets = append(targets, target{OS: parts[0], Arch: parts[1]})
			seen[key] = true
		}
	}
	return targets, nil
}

func buildArchive(tempDir, outDir, version, commit string, stamp time.Time, target target, base releaseManifest) (archiveInfo, error) {
	rootName := fmt.Sprintf("bak-%s-%s-%s", sanitizeVersion(version), target.OS, target.Arch)
	stageDir := filepath.Join(tempDir, rootName)
	if err := os.MkdirAll(filepath.Join(stageDir, "bin"), 0o755); err != nil {
		return archiveInfo{}, fmt.Errorf("create stage directory: %w", err)
	}

	date := stamp.Format(time.RFC3339)
	ldflags := strings.Join([]string{
		"-s", "-w",
		"-X", "github.com/baxromumarov/bak/internal/buildinfo.Version=" + version,
		"-X", "github.com/baxromumarov/bak/internal/buildinfo.Commit=" + commit,
		"-X", "github.com/baxromumarov/bak/internal/buildinfo.Date=" + date,
	}, " ")

	for _, releaseTool := range releaseTools {
		name := releaseTool.Name
		if target.OS == "windows" {
			name += ".exe"
		}
		output := filepath.Join(stageDir, "bin", name)
		cmd := exec.Command("go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false", "-ldflags", ldflags, "-o", output, releaseTool.Package)
		cmd.Env = withBuildEnv(os.Environ(), target)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return archiveInfo{}, fmt.Errorf("build %s for %s/%s: %w", releaseTool.Name, target.OS, target.Arch, err)
		}
	}

	for _, doc := range []string{"README.md", "docs/SUPPORT_MATRIX_V0.1.md", "docs/CORE_LANGUAGE_SPEC.md", "docs/STDLIB_STABLE_V0.1.md", "docs/TRUST_MODEL.md"} {
		dst := filepath.Join(stageDir, doc)
		if err := copyFile(doc, dst); err != nil {
			return archiveInfo{}, err
		}
	}

	base.Archives = nil
	archiveManifest, err := marshalJSON(base)
	if err != nil {
		return archiveInfo{}, err
	}
	if err := os.WriteFile(filepath.Join(stageDir, "manifest.json"), archiveManifest, 0o644); err != nil {
		return archiveInfo{}, fmt.Errorf("write archive manifest: %w", err)
	}

	archiveName := rootName + ".zip"
	archivePath := filepath.Join(outDir, archiveName)
	if err := zipTree(archivePath, stageDir, rootName, stamp); err != nil {
		return archiveInfo{}, err
	}
	digest, err := fileSHA256(archivePath)
	if err != nil {
		return archiveInfo{}, err
	}
	return archiveInfo{Name: archiveName, OS: target.OS, Arch: target.Arch, SHA256: digest}, nil
}

func withBuildEnv(env []string, target target) []string {
	out := make([]string, 0, len(env)+3)
	for _, item := range env {
		if strings.HasPrefix(item, "GOOS=") || strings.HasPrefix(item, "GOARCH=") || strings.HasPrefix(item, "CGO_ENABLED=") {
			continue
		}
		out = append(out, item)
	}
	return append(out, "GOOS="+target.OS, "GOARCH="+target.Arch, "CGO_ENABLED=0")
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read release file %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create release documentation directory: %w", err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write release file %s: %w", dst, err)
	}
	return nil
}

func zipTree(archivePath, sourceDir, rootName string, stamp time.Time) error {
	file, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	zw := zip.NewWriter(file)

	var paths []string
	err = filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err == nil {
		sort.Strings(paths)
		for _, path := range paths {
			rel, relErr := filepath.Rel(sourceDir, path)
			if relErr != nil {
				err = relErr
				break
			}
			mode := os.FileMode(0o644)
			if strings.HasPrefix(filepath.ToSlash(rel), "bin/") {
				mode = 0o755
			}
			header := &zip.FileHeader{
				Name:     rootName + "/" + filepath.ToSlash(rel),
				Method:   zip.Deflate,
				Modified: stamp,
			}
			header.SetMode(mode)
			writer, createErr := zw.CreateHeader(header)
			if createErr != nil {
				err = createErr
				break
			}
			input, openErr := os.Open(path)
			if openErr != nil {
				err = openErr
				break
			}
			_, copyErr := io.Copy(writer, input)
			closeErr := input.Close()
			if copyErr != nil {
				err = copyErr
				break
			}
			if closeErr != nil {
				err = closeErr
				break
			}
		}
	}
	closeZipErr := zw.Close()
	closeFileErr := file.Close()
	if err != nil {
		return fmt.Errorf("create archive contents: %w", err)
	}
	if closeZipErr != nil {
		return fmt.Errorf("finalize archive: %w", closeZipErr)
	}
	if closeFileErr != nil {
		return fmt.Errorf("close archive: %w", closeFileErr)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func marshalJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal release metadata: %w", err)
	}
	return append(data, '\n'), nil
}

func sanitizeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	if version == "" {
		return "dev"
	}
	return strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(version)
}
