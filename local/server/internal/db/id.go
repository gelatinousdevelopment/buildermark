package db

import (
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// newID returns a secure URL-friendly NanoID (21 chars by default).
func newID() string {
	return gonanoid.Must()
}
