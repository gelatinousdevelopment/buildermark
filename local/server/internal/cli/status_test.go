package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckStatus_NoServer(t *testing.T) {
	result := CheckStatus("http://127.0.0.1:0", "/tmp/cfg", "/tmp/db")
	if result.Running {
		t.Error("CheckStatus() Running = true, want false when no server")
	}
}

func TestCheckStatus_RunningServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	result := CheckStatus(ts.URL, "/tmp/cfg", "/tmp/db")
	if !result.Running {
		t.Error("CheckStatus() Running = false, want true with running server")
	}
}

func TestPrintStatus(t *testing.T) {
	tests := []struct {
		name   string
		status StatusResult
		want   []string
	}{
		{
			name: "running",
			status: StatusResult{
				Running:   true,
				URL:       "http://localhost:55022",
				ConfigDir: "/home/user/.buildermark",
				DBPath:    "/home/user/.buildermark/local.db",
				Version:   "1.0.0",
			},
			want: []string{"running", "http://localhost:55022", "/home/user/.buildermark", "local.db", "1.0.0"},
		},
		{
			name: "stopped",
			status: StatusResult{
				Running:   false,
				URL:       "http://localhost:55022",
				ConfigDir: "/home/user/.buildermark",
				DBPath:    "/home/user/.buildermark/local.db",
				Version:   "dev",
			},
			want: []string{"stopped", "http://localhost:55022"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			PrintStatus(&buf, tt.status)
			got := buf.String()
			for _, s := range tt.want {
				if !strings.Contains(got, s) {
					t.Errorf("PrintStatus() output missing %q:\n%s", s, got)
				}
			}
		})
	}
}
