package epinput

import (
	"fmt"
	"os"
	"path/filepath"
)

type ReferenceSet struct {
	Version     VersionInfo `json:"version"`
	IDDFound    bool        `json:"iddFound"`
	IDDPath     string      `json:"iddPath,omitempty"`
	SchemaFound bool        `json:"schemaFound"`
	SchemaPath  string      `json:"schemaPath,omitempty"`
}

func ResolveReferences(repoRoot string, version VersionInfo) ReferenceSet {
	refs := ReferenceSet{Version: version}
	if !version.Known {
		return refs
	}

	versionDir := fmt.Sprintf("%d.%d", version.Major, version.Minor)
	root := filepath.Join(repoRoot, "resources", "energyplus", versionDir)
	iddPath := filepath.Join(root, "Energy+.idd")
	schemaPath := filepath.Join(root, "Energy+.schema.epJSON")

	if fileExists(iddPath) {
		refs.IDDFound = true
		refs.IDDPath = iddPath
	}
	if fileExists(schemaPath) {
		refs.SchemaFound = true
		refs.SchemaPath = schemaPath
	}
	return refs
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
