package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
)

type cspNonceKey struct{}

var (
	scriptOpenTagRe = regexp.MustCompile(`(?i)<script\b([^>]*)>`)
	scriptNonceAttr = regexp.MustCompile(`(?i)\bnonce\s*=`)
	cspNonceMetaRe  = regexp.MustCompile(`(?i)<meta[^>]+property\s*=\s*["']csp-nonce["'][^>]*>`)
	headOpenTagRe   = regexp.MustCompile(`(?i)<head\b[^>]*>`)
)

func newCSPNonce() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func withCSPNonce(ctx context.Context, nonce string) context.Context {
	return context.WithValue(ctx, cspNonceKey{}, nonce)
}

func cspNonceFromContext(ctx context.Context) (string, bool) {
	nonce, ok := ctx.Value(cspNonceKey{}).(string)
	if !ok || nonce == "" {
		return "", false
	}
	return nonce, true
}

func isFrontendHTMLRequest(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	accept := r.Header.Get("Accept")
	if accept == "" {
		return true
	}
	return strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isFrontendHTMLRequest(r) {
			nonce, err := newCSPNonce()
			if err != nil {
				log.Printf("error generating CSP nonce: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			r = r.WithContext(withCSPNonce(r.Context(), nonce))
			w.Header().Set("Content-Security-Policy", buildCSPHeader(nonce))
		}
		next.ServeHTTP(w, r)
	})
}

func buildCSPHeader(nonce string) string {
	return fmt.Sprintf(
		"default-src 'none'; base-uri 'none'; frame-ancestors 'none'; object-src 'none'; form-action 'self'; script-src 'self' 'nonce-%s'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self' ws://localhost:* ws://127.0.0.1:* wss://localhost:* wss://127.0.0.1:*; manifest-src 'self'; frame-src 'none'; worker-src 'none'",
		nonce,
	)
}

func injectNonceIntoHTML(html, nonce string) string {
	if nonce == "" {
		return html
	}

	withMeta := html
	if !cspNonceMetaRe.MatchString(withMeta) {
		meta := `<meta property="csp-nonce" nonce="` + nonce + `">`
		loc := headOpenTagRe.FindStringIndex(withMeta)
		if loc != nil {
			insert := loc[1]
			withMeta = withMeta[:insert] + "\n        " + meta + withMeta[insert:]
		}
	}

	return scriptOpenTagRe.ReplaceAllStringFunc(withMeta, func(tag string) string {
		if scriptNonceAttr.MatchString(tag) {
			return tag
		}
		return strings.TrimSuffix(tag, ">") + ` nonce="` + nonce + `">`
	})
}
