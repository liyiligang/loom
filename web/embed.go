package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var assets embed.FS

func Assets() fs.FS {
	files, err := fs.Sub(assets, "dist")
	if err != nil {
		panic(err)
	}
	return files
}
