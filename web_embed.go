package embshift

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist
var webDistFS embed.FS

// DistFS returns the embedded filesystem rooted at the Vite dist directory.
func DistFS() (fs.FS, error) {
	return fs.Sub(webDistFS, "web/dist")
}
