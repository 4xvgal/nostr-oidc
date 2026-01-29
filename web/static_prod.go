package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/dist/** static/js/** static/css/**
var embeddedStatic embed.FS

func getStaticFileSystem() http.FileSystem {
	fsys, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		panic(err)
	}
	return http.FS(fsys)
}
