package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/server"
)

// Config holds resolved startup configuration.
type Config struct {
	RemoteURL    string
	AuthToken    string
	ExtraHeaders map[string]string
	Namespaces   []string
	MultiRepo    bool
}

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	token := os.Getenv("MEMORY_MCP_TOKEN")
	if token == "" {
		log.Fatal("MEMORY_MCP_TOKEN is required")
	}

	remoteURL := os.Getenv("MEMORY_MCP_URL")
	if remoteURL == "" {
		log.Fatal("MEMORY_MCP_URL is required")
	}

	namespaces, err := detectNamespaces()
	if err != nil {
		log.Fatalf("namespace detection failed: %v", err)
	}

	extraHeaders := parseExtraHeaders(os.Getenv("MEMORY_MCP_EXTRA_HEADERS"))

	cfg := Config{
		RemoteURL:    remoteURL,
		AuthToken:    token,
		ExtraHeaders: extraHeaders,
		Namespaces:   namespaces,
		MultiRepo:    len(namespaces) > 1,
	}

	log.Printf("memory-mcp-proxy: namespaces=%v multi=%v url=%s", cfg.Namespaces, cfg.MultiRepo, cfg.RemoteURL)

	remote := NewRemoteClient(cfg)
	defer remote.Close()

	cache := NewRecallCache(remote, cfg)
	s := buildServer(cfg, remote, cache)

	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("stdio server error: %v", err)
	}
}

// detectNamespaces resolves project namespace(s) in priority order:
// 1. MEMORY_NAMESPACE env var
// 2. Git remote origin → repo name
// 3. /repos/* subdirectories (multi-repo mode)
// 4. basename of /workspace
func detectNamespaces() ([]string, error) {
	// 1. Explicit env var
	if ns := os.Getenv("MEMORY_NAMESPACE"); ns != "" {
		return []string{ns}, nil
	}

	// 2. Git remote
	if name, err := repoNameFromGit("/workspace"); err == nil && name != "" {
		return []string{name}, nil
	}

	// 3. Multi-repo scan
	if entries, err := os.ReadDir("/repos"); err == nil {
		var repos []string
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				repos = append(repos, e.Name())
			}
		}
		if len(repos) > 0 {
			return repos, nil
		}
	}

	// 4. Basename fallback
	if info, err := os.Stat("/workspace"); err == nil && info.IsDir() {
		name := filepath.Base("/workspace")
		if name != "" && name != "." && name != "/" {
			return []string{name}, nil
		}
	}

	return nil, fmt.Errorf("could not detect namespace: no MEMORY_NAMESPACE, no git remote at /workspace, no repos at /repos, and /workspace does not exist")
}

// repoNameFromGit extracts the repo name from the git remote URL.
// Handles both SSH (git@host:owner/repo.git) and HTTPS (https://host/owner/repo.git).
func repoNameFromGit(dir string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", err
	}

	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", err
	}

	return extractRepoName(strings.TrimSpace(string(out))), nil
}

// parseExtraHeaders parses comma-separated "Name: Value" pairs.
// e.g. "X-CF-Bypass: secret123, X-Other: val"
func parseExtraHeaders(raw string) map[string]string {
	headers := make(map[string]string)
	if raw == "" {
		return headers
	}
	for _, pair := range strings.Split(raw, ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(pair), ":")
		if ok {
			headers[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	return headers
}

func extractRepoName(gitURL string) string {
	gitURL = strings.TrimSuffix(gitURL, ".git")

	// Last path segment works for both SSH and HTTPS URLs
	if idx := strings.LastIndex(gitURL, "/"); idx >= 0 {
		return gitURL[idx+1:]
	}
	// SSH with colon separator: git@host:owner/repo
	if idx := strings.LastIndex(gitURL, ":"); idx >= 0 {
		return gitURL[idx+1:]
	}
	return gitURL
}
