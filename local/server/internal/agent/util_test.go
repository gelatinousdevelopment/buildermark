package agent

import "testing"

func TestTitleFromPlanPrompt(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "standard plan prompt",
			input: "Implement the following plan:\n# Plan: Fix and Enhance Git Worktree Detection\nsome details here",
			want:  "Fix and Enhance Git Worktree Detection",
		},
		{
			name:  "generic heading",
			input: "Implement the following plan:\n# Some Title\nmore details",
			want:  "Some Title",
		},
		{
			name:  "blank line before heading",
			input: "Implement the following plan:\n\n# Plan: My Title\ndetails",
			want:  "My Title",
		},
		{
			name:  "non-plan prompt",
			input: "Please fix the bug in auth",
			want:  "",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "plan prefix but no heading",
			input: "Implement the following plan:\nNo heading here",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TitleFromPlanPrompt(tt.input)
			if got != tt.want {
				t.Errorf("TitleFromPlanPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}
