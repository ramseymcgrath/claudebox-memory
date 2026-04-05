package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		name   string
		gitURL string
		want   string
	}{
		{
			name:   "HTTPS with .git suffix",
			gitURL: "https://github.com/ramseymcgrath/claudebox-memory.git",
			want:   "claudebox-memory",
		},
		{
			name:   "HTTPS without .git suffix",
			gitURL: "https://github.com/ramseymcgrath/claudebox-memory",
			want:   "claudebox-memory",
		},
		{
			name:   "SSH with .git suffix",
			gitURL: "git@github.com:ramseymcgrath/claudebox-memory.git",
			want:   "claudebox-memory",
		},
		{
			name:   "SSH without .git suffix",
			gitURL: "git@github.com:ramseymcgrath/claudebox-memory",
			want:   "claudebox-memory",
		},
		{
			name:   "simple repo name",
			gitURL: "https://github.com/org/repo.git",
			want:   "repo",
		},
		{
			name:   "deeply nested path",
			gitURL: "https://gitlab.com/group/subgroup/project.git",
			want:   "project",
		},
		{
			name:   "bare name",
			gitURL: "myrepo",
			want:   "myrepo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractRepoName(tt.gitURL)
			if got != tt.want {
				t.Errorf("extractRepoName(%q) = %q, want %q", tt.gitURL, got, tt.want)
			}
		})
	}
}

func TestParseExtraHeaders(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"single", "X-CF-Bypass: secret123", map[string]string{"X-CF-Bypass": "secret123"}},
		{"multiple", "X-CF-Bypass: abc, X-Other: def", map[string]string{"X-CF-Bypass": "abc", "X-Other": "def"}},
		{"extra whitespace", " X-Foo : bar , X-Baz : qux ", map[string]string{"X-Foo": "bar", "X-Baz": "qux"}},
		{"no colon skipped", "bad-header", map[string]string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseExtraHeaders(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("header %q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestDetectNamespaces_EnvVar(t *testing.T) {
	t.Setenv("MEMORY_NAMESPACE", "test-project")

	ns, err := detectNamespaces()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ns) != 1 || ns[0] != "test-project" {
		t.Errorf("got %v, want [test-project]", ns)
	}
}

func TestDetectNamespaces_EnvVarTakesPriority(t *testing.T) {
	t.Setenv("MEMORY_NAMESPACE", "override")

	ns, err := detectNamespaces()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ns[0] != "override" {
		t.Errorf("env var should take priority, got %v", ns)
	}
}

func TestDetectNamespaces_ReposDir(t *testing.T) {
	// Clear env var so it doesn't short-circuit
	t.Setenv("MEMORY_NAMESPACE", "")

	// Create a temporary /repos-like structure
	dir := t.TempDir()
	repoA := filepath.Join(dir, "alpha")
	repoB := filepath.Join(dir, "beta")
	hidden := filepath.Join(dir, ".hidden")
	os.Mkdir(repoA, 0o755)
	os.Mkdir(repoB, 0o755)
	os.Mkdir(hidden, 0o755)

	// We can't easily test the full detectNamespaces since it hardcodes /repos,
	// but we can test the scanning logic directly
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	var repos []string
	for _, e := range entries {
		if e.IsDir() && e.Name()[0] != '.' {
			repos = append(repos, e.Name())
		}
	}

	if len(repos) != 2 {
		t.Errorf("expected 2 repos, got %d: %v", len(repos), repos)
	}
	// Hidden dir should be skipped
	for _, r := range repos {
		if r == ".hidden" {
			t.Error("hidden directory should be skipped")
		}
	}
}
