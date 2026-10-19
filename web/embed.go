// Package web embeds the built frontend (web/dist) so a single netcfg
// binary serves both the API and the management UI.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist returns the frontend assets rooted at the dist directory.
func Dist() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
