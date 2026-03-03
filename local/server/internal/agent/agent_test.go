package agent

import (
	"testing"
	"time"
)

func TestStartupScanWindow(t *testing.T) {
	tests := []struct {
		name     string
		latestMs int64
		wantMin  time.Duration
		wantMax  time.Duration
	}{
		{
			name:     "zero returns DefaultScanWindow",
			latestMs: 0,
			wantMin:  DefaultScanWindow,
			wantMax:  DefaultScanWindow,
		},
		{
			name:     "negative returns DefaultScanWindow",
			latestMs: -1,
			wantMin:  DefaultScanWindow,
			wantMax:  DefaultScanWindow,
		},
		{
			name:     "recent timestamp returns small window",
			latestMs: time.Now().Add(-30 * time.Second).UnixMilli(),
			wantMin:  time.Minute, // floored at 1 min
			wantMax:  6 * time.Minute,
		},
		{
			name:     "10 minutes ago",
			latestMs: time.Now().Add(-10 * time.Minute).UnixMilli(),
			wantMin:  14 * time.Minute, // ~10m + 5m buffer, allow some slack
			wantMax:  16 * time.Minute,
		},
		{
			name:     "very old timestamp capped at DefaultScanWindow",
			latestMs: time.Now().Add(-200 * 24 * time.Hour).UnixMilli(),
			wantMin:  DefaultScanWindow,
			wantMax:  DefaultScanWindow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StartupScanWindow(tt.latestMs)
			if got < tt.wantMin {
				t.Errorf("StartupScanWindow(%d) = %s, want >= %s", tt.latestMs, got, tt.wantMin)
			}
			if got > tt.wantMax {
				t.Errorf("StartupScanWindow(%d) = %s, want <= %s", tt.latestMs, got, tt.wantMax)
			}
		})
	}
}
