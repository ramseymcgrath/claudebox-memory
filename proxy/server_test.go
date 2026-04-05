package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestResolveNamespace_Default(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	ns := resolveNamespace(req, "fallback")
	if ns != "fallback" {
		t.Errorf("expected fallback, got %q", ns)
	}
}

func TestResolveNamespace_Explicit(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"namespace": "custom",
	}

	ns := resolveNamespace(req, "fallback")
	if ns != "custom" {
		t.Errorf("expected custom, got %q", ns)
	}
}

func TestBuildServer_SingleRepo(t *testing.T) {
	cfg := Config{
		RemoteURL:  "https://example.com/mcp",
		AuthToken:  "test-token",
		Namespaces: []string{"myproject"},
		MultiRepo:  false,
	}

	remote := NewRemoteClient(cfg)
	cache := NewRecallCache(remote, cfg)
	s := buildServer(cfg, remote, cache)

	msg := mustMarshal(t, mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      mcp.NewRequestId(1),
		Request: mcp.Request{Method: "tools/list"},
	})

	result := s.HandleMessage(context.Background(), msg)

	resp, ok := result.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("expected JSONRPCResponse, got %T", result)
	}

	toolsResult, ok := resp.Result.(mcp.ListToolsResult)
	if !ok {
		t.Fatalf("expected ListToolsResult, got %T", resp.Result)
	}

	if len(toolsResult.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(toolsResult.Tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range toolsResult.Tools {
		toolNames[tool.Name] = true
	}

	for _, name := range []string{"recall", "remember", "forget"} {
		if !toolNames[name] {
			t.Errorf("missing tool %q", name)
		}
	}

	// In single-repo mode, tools should NOT have a namespace parameter
	for _, tool := range toolsResult.Tools {
		if tool.Name == "recall" || tool.Name == "remember" {
			props := schemaProperties(t, tool)
			if _, hasNS := props["namespace"]; hasNS {
				t.Errorf("tool %q should not expose namespace in single-repo mode", tool.Name)
			}
		}
	}
}

func TestBuildServer_MultiRepo(t *testing.T) {
	cfg := Config{
		RemoteURL:  "https://example.com/mcp",
		AuthToken:  "test-token",
		Namespaces: []string{"repo-a", "repo-b"},
		MultiRepo:  true,
	}

	remote := NewRemoteClient(cfg)
	cache := NewRecallCache(remote, cfg)
	s := buildServer(cfg, remote, cache)

	msg := mustMarshal(t, mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      mcp.NewRequestId(2),
		Request: mcp.Request{Method: "tools/list"},
	})

	result := s.HandleMessage(context.Background(), msg)
	resp := result.(mcp.JSONRPCResponse)
	toolsResult := resp.Result.(mcp.ListToolsResult)

	for _, tool := range toolsResult.Tools {
		if tool.Name == "recall" || tool.Name == "remember" {
			props := schemaProperties(t, tool)
			if _, hasNS := props["namespace"]; !hasNS {
				t.Errorf("tool %q should expose namespace in multi-repo mode", tool.Name)
			}
		}
	}
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// schemaProperties extracts the properties map from a tool's InputSchema.
func schemaProperties(t *testing.T, tool mcp.Tool) map[string]any {
	t.Helper()
	// InputSchema.Properties may be a map or need re-marshaling
	b, err := json.Marshal(tool.InputSchema.Properties)
	if err != nil {
		t.Fatalf("marshal schema properties: %v", err)
	}
	var props map[string]any
	if err := json.Unmarshal(b, &props); err != nil {
		t.Fatalf("unmarshal schema properties: %v", err)
	}
	return props
}
