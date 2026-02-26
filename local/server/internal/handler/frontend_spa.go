package handler

import (
	"io/fs"
	"net/http"
	"strings"
)

// spaFileServer serves an embedded SPA filesystem. Static files are served
// directly; all other GET requests fall back to the SPA's fallback HTML page
// (200.html from @sveltejs/adapter-static) for client-side routing.
func spaFileServer(fsys fs.FS, fallbackFile string) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only serve GET/HEAD requests from the SPA
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "."
		}

		// Try to open the file. If it exists, serve it directly.
		f, err := fsys.Open(path)
		if err == nil {
			f.Close()
			// Inject CSP nonce for HTML files
			if strings.HasSuffix(path, ".html") {
				serveHTMLWithNonce(w, r, fsys, path)
				return
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// File not found — serve the SPA fallback page for client-side routing
		serveHTMLWithNonce(w, r, fsys, fallbackFile)
	})
}

func serveHTMLWithNonce(w http.ResponseWriter, r *http.Request, fsys fs.FS, path string) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	html := string(data)
	if nonce, ok := cspNonceFromContext(r.Context()); ok {
		html = injectNonceIntoHTML(html, nonce)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}
