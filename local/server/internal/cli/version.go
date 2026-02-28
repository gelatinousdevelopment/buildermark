package cli

import (
	"fmt"
	"io"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// RunVersion prints the version string to w.
func RunVersion(w io.Writer) {
	fmt.Fprintf(w, "buildermark %s\n", Version)
}
