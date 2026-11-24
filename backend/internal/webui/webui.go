// Package webui embeds the built React application (frontend/ → dist/).
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built UI rooted at dist/.
func FS() fs.FS {
	sub, _ := fs.Sub(dist, "dist")
	return sub
}
