package main

import "testing"

func TestRun_Help(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"no args", []string{}, 0},
		{"help", []string{"help"}, 0},
		{"-h", []string{"-h"}, 0},
		{"--help", []string{"--help"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(tt.args)
			if got != tt.want {
				t.Errorf("run(%v) = %d, want %d", tt.args, got, tt.want)
			}
		})
	}
}

func TestRun_Version(t *testing.T) {
	tests := []struct {
		args []string
		want int
	}{
		{[]string{"version"}, 0},
		{[]string{"-v"}, 0},
		{[]string{"--version"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.args[0], func(t *testing.T) {
			got := run(tt.args)
			if got != tt.want {
				t.Errorf("run(%v) = %d, want %d", tt.args, got, tt.want)
			}
		})
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	got := run([]string{"nonexistent"})
	if got != 1 {
		t.Errorf("run([nonexistent]) = %d, want 1", got)
	}
}

func TestRun_Status(t *testing.T) {
	got := run([]string{"status"})
	if got != 0 {
		t.Errorf("run([status]) = %d, want 0", got)
	}
}

func TestRun_ServiceNoArgs(t *testing.T) {
	got := run([]string{"service"})
	if got != 1 {
		t.Errorf("run([service]) = %d, want 1", got)
	}
}

func TestRun_UpdateNoArgs(t *testing.T) {
	got := run([]string{"update"})
	if got != 1 {
		t.Errorf("run([update]) = %d, want 1", got)
	}
}

func TestRun_UpdateModeNoArgs(t *testing.T) {
	got := run([]string{"update", "mode"})
	if got != 0 {
		t.Errorf("run([update mode]) = %d, want 0", got)
	}
}
