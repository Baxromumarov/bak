// Package buildinfo contains version and support-profile metadata shared by
// every Bak command. Release builds override Version, Commit, and Date through
// linker flags; the language and backend support levels are frozen source
// constants for the v0.1 line.
package buildinfo

import (
	"encoding/json"
	"fmt"
	"io"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

const (
	LanguageVersion   = "0.1"
	LanguageStatus    = "frozen"
	ProductionBackend = "vm"
	VMStatus          = "stable"
	NativeStatus      = "preview"
	NativeTarget      = "linux/amd64"
)

// Info is the machine-readable identity and support profile of a Bak tool.
type Info struct {
	Tool              string `json:"tool"`
	Version           string `json:"version"`
	Commit            string `json:"commit"`
	BuiltAt           string `json:"builtAt"`
	LanguageVersion   string `json:"languageVersion"`
	LanguageStatus    string `json:"languageStatus"`
	ProductionBackend string `json:"productionBackend"`
	VMStatus          string `json:"vmStatus"`
	NativeStatus      string `json:"nativeStatus"`
	NativeTarget      string `json:"nativeTarget"`
}

// Current returns the build identity shared by every binary in a release.
func Current(tool string) Info {
	return Info{
		Tool:              tool,
		Version:           Version,
		Commit:            Commit,
		BuiltAt:           Date,
		LanguageVersion:   LanguageVersion,
		LanguageStatus:    LanguageStatus,
		ProductionBackend: ProductionBackend,
		VMStatus:          VMStatus,
		NativeStatus:      NativeStatus,
		NativeTarget:      NativeTarget,
	}
}

// WriteVersion prints either human-readable build details or a stable JSON
// object suitable for release and editor integration checks.
func WriteVersion(w io.Writer, tool string, jsonMode bool) error {
	info := Current(tool)
	if jsonMode {
		encoder := json.NewEncoder(w)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(info)
	}

	if _, err := fmt.Fprintf(w, "%s %s\n", tool, Version); err != nil {
		return err
	}
	if Commit != "" && Commit != "unknown" {
		if _, err := fmt.Fprintf(w, "commit %s\n", Commit); err != nil {
			return err
		}
	}
	if Date != "" && Date != "unknown" {
		if _, err := fmt.Fprintf(w, "built %s\n", Date); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "language v%s (%s)\n", LanguageVersion, LanguageStatus); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "production backend: %s (%s)\n", ProductionBackend, VMStatus); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "native backend: %s (%s)\n", NativeStatus, NativeTarget)
	return err
}
