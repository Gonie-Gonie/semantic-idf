// Package frontend provides the embedded static assets used by the desktop app.
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:src
var embedded embed.FS

// Assets exposes frontend/src as the asset filesystem root.
var Assets = mustAssets()

func mustAssets() fs.FS {
	assets, err := fs.Sub(embedded, "src")
	if err != nil {
		panic(err)
	}
	return assets
}
