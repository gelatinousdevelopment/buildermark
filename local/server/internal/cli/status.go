package cli

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// StatusResult holds the result of a server status check.
type StatusResult struct {
	Running    bool
	URL        string
	ConfigDir  string
	DBPath     string
	Version    string
}

// CheckStatus checks whether the server is running by hitting its health endpoint.
func CheckStatus(serverURL string, configDir string, dbPath string) StatusResult {
	result := StatusResult{
		URL:       serverURL,
		ConfigDir: configDir,
		DBPath:    dbPath,
		Version:   Version,
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(serverURL + "/api/v1/settings")
	if err != nil {
		return result
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		result.Running = true
	}
	return result
}

// PrintStatus writes the status to w in a human-readable format.
func PrintStatus(w io.Writer, s StatusResult) {
	if s.Running {
		fmt.Fprintln(w, "Server:     running")
	} else {
		fmt.Fprintln(w, "Server:     stopped")
	}
	fmt.Fprintf(w, "URL:        %s\n", s.URL)
	fmt.Fprintf(w, "Config:     %s\n", s.ConfigDir)
	fmt.Fprintf(w, "Database:   %s\n", s.DBPath)
	fmt.Fprintf(w, "Version:    %s\n", s.Version)
}
