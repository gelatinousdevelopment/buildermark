//go:build !embed_frontend

package handler

import "io/fs"

func embeddedFrontendFS() fs.FS {
	return nil
}
