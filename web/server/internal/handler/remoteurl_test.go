package handler

import "testing"

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		want   parsedRemote
		wantOK bool
	}{
		{
			name:   "https github",
			raw:    "https://github.com/owner/repo.git",
			want:   parsedRemote{Domain: "github.com", Owner: "owner", Repo: "repo"},
			wantOK: true,
		},
		{
			name:   "https without .git",
			raw:    "https://github.com/owner/repo",
			want:   parsedRemote{Domain: "github.com", Owner: "owner", Repo: "repo"},
			wantOK: true,
		},
		{
			name:   "ssh scp-style",
			raw:    "git@github.com:owner/repo.git",
			want:   parsedRemote{Domain: "github.com", Owner: "owner", Repo: "repo"},
			wantOK: true,
		},
		{
			name:   "ssh protocol",
			raw:    "ssh://git@github.com/owner/repo.git",
			want:   parsedRemote{Domain: "github.com", Owner: "owner", Repo: "repo"},
			wantOK: true,
		},
		{
			name:   "gitlab https",
			raw:    "https://gitlab.com/owner/repo.git",
			want:   parsedRemote{Domain: "gitlab.com", Owner: "owner", Repo: "repo"},
			wantOK: true,
		},
		{
			name:   "bitbucket ssh",
			raw:    "git@bitbucket.org:owner/repo.git",
			want:   parsedRemote{Domain: "bitbucket.org", Owner: "owner", Repo: "repo"},
			wantOK: true,
		},
		{
			name:   "custom domain",
			raw:    "https://git.example.com/owner/repo.git",
			want:   parsedRemote{Domain: "git.example.com", Owner: "owner", Repo: "repo"},
			wantOK: true,
		},
		{
			name:   "gitlab subgroup",
			raw:    "https://gitlab.com/group/subgroup/repo.git",
			want:   parsedRemote{Domain: "gitlab.com", Owner: "group/subgroup", Repo: "repo"},
			wantOK: true,
		},
		{name: "empty", raw: "", wantOK: false},
		{name: "just a word", raw: "foobar", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseRemoteURL(tt.raw)
			if ok != tt.wantOK {
				t.Fatalf("parseRemoteURL(%q) ok = %v, want %v", tt.raw, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got != tt.want {
				t.Errorf("parseRemoteURL(%q) = %+v, want %+v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestRemoteURL(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		want   string
	}{
		{
			name:   "github https",
			remote: "https://github.com/owner/repo.git",
			want:   "https://github.com/owner/repo",
		},
		{
			name:   "github ssh",
			remote: "git@github.com:owner/repo.git",
			want:   "https://github.com/owner/repo",
		},
		{
			name:   "gitlab https",
			remote: "https://gitlab.com/owner/repo.git",
			want:   "https://gitlab.com/owner/repo",
		},
		{
			name:   "gitlab subgroup",
			remote: "https://gitlab.com/group/subgroup/repo.git",
			want:   "https://gitlab.com/group/subgroup/repo",
		},
		{
			name:   "bitbucket",
			remote: "https://bitbucket.org/owner/repo.git",
			want:   "https://bitbucket.org/owner/repo",
		},
		{
			name:   "custom domain",
			remote: "https://gitea.example.com/owner/repo.git",
			want:   "https://gitea.example.com/owner/repo",
		},
		{
			name:   "empty remote",
			remote: "",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := remoteURL(tt.remote)
			if got != tt.want {
				t.Errorf("remoteURL(%q) = %q, want %q", tt.remote, got, tt.want)
			}
		})
	}
}

func TestCommitURL(t *testing.T) {
	hash := "abc123def456"

	tests := []struct {
		name   string
		remote string
		want   string
	}{
		{
			name:   "github https",
			remote: "https://github.com/owner/repo.git",
			want:   "https://github.com/owner/repo/commit/" + hash,
		},
		{
			name:   "github ssh",
			remote: "git@github.com:owner/repo.git",
			want:   "https://github.com/owner/repo/commit/" + hash,
		},
		{
			name:   "gitlab https",
			remote: "https://gitlab.com/owner/repo.git",
			want:   "https://gitlab.com/owner/repo/-/commit/" + hash,
		},
		{
			name:   "codeberg",
			remote: "https://codeberg.org/owner/repo.git",
			want:   "https://codeberg.org/owner/repo/commit/" + hash,
		},
		{
			name:   "bitbucket",
			remote: "https://bitbucket.org/owner/repo.git",
			want:   "https://bitbucket.org/owner/repo/commits/" + hash,
		},
		{
			name:   "unknown domain defaults to /commit/",
			remote: "https://gitea.example.com/owner/repo.git",
			want:   "https://gitea.example.com/owner/repo/commit/" + hash,
		},
		{
			name:   "empty remote",
			remote: "",
			want:   "",
		},
		{
			name:   "gitlab subgroup",
			remote: "https://gitlab.com/group/subgroup/repo.git",
			want:   "https://gitlab.com/group/subgroup/repo/-/commit/" + hash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commitURL(tt.remote, hash)
			if got != tt.want {
				t.Errorf("commitURL(%q, %q) = %q, want %q", tt.remote, hash, got, tt.want)
			}
		})
	}
}
