package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunNotificationsGetDefault(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := RunNotificationsGet(&buf, dir); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "on" {
		t.Fatalf("default = %q, want %q", got, "on")
	}
}

func TestRunNotificationsSetOff(t *testing.T) {
	dir := t.TempDir()
	if err := RunNotificationsSet(dir, false); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := RunNotificationsGet(&buf, dir); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "off" {
		t.Fatalf("after set off = %q, want %q", got, "off")
	}
}

func TestRunNotificationsRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Set off.
	if err := RunNotificationsSet(dir, false); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := RunNotificationsGet(&buf, dir); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "off" {
		t.Fatalf("got %q, want %q", got, "off")
	}

	// Set on.
	if err := RunNotificationsSet(dir, true); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := RunNotificationsGet(&buf, dir); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "on" {
		t.Fatalf("got %q, want %q", got, "on")
	}
}
