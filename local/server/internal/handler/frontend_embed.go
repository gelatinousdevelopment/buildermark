//go:build embed_frontend

package handler

import (
	"embed"
	"io/fs"
)

//go:embed frontend_dist/*
var frontendDist embed.FS

func embeddedFrontendFS() fs.FS {
	sub, err := fs.Sub(frontendDist, "frontend_dist")
	if err != nil {
		panic("frontend_dist embed subtree: " + err.Error())
	}
	return sub
}
