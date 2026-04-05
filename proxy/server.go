package main

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func buildServer(cfg Config, remote *RemoteClient, cache *RecallCache) *server.MCPServer {
	hooks := &server.Hooks{}
	hooks.AddOnRegisterSession(func(_ context.Context, _ server.ClientSession) {
		cache.TriggerAutoRecall()
	})

	s := server.NewMCPServer("memory", "1.0.0",
		server.WithToolCapabilities(false),
		server.WithHooks(hooks),
	)

	registerTools(s, cfg, remote, cache)
	return s
}

func registerTools(s *server.MCPServer, cfg Config, remote *RemoteClient, cache *RecallCache) {
	defaultNS := cfg.Namespaces[0]
	categories := []string{"decision", "pattern", "fact", "task", "preference"}

	// ── recall ──────────────────────────────────────────────────────────
	recallOpts := []mcp.ToolOption{
		mcp.WithDescription(
			"Search memories for this project. Returns pinned items first, then FTS matches or recent items. " +
				"Context is auto-loaded at session start — use this only when you need to search for something specific.",
		),
		mcp.WithString("query", mcp.Description("Search terms. Omit to get pinned + recent items.")),
		mcp.WithString("category", mcp.Description("Filter by category"), mcp.Enum(categories...)),
		mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
	}
	if cfg.MultiRepo {
		recallOpts = append(recallOpts, mcp.WithString("namespace",
			mcp.Description("Project namespace"),
			mcp.Enum(cfg.Namespaces...),
		))
	}
	s.AddTool(mcp.NewTool("recall", recallOpts...), makeRecallHandler(remote, cache, defaultNS))

	// ── remember ────────────────────────────────────────────────────────
	rememberOpts := []mcp.ToolOption{
		mcp.WithDescription(
			"Store a memory. Use for decisions (with rationale), patterns, debugging insights, or cross-session task state. " +
				"Store 'why' not 'what' — the code is the source of truth for 'what'.",
		),
		mcp.WithString("content", mcp.Required(), mcp.Description("The memory content. Be concise but include rationale.")),
		mcp.WithString("category", mcp.Required(), mcp.Description("Type of memory"), mcp.Enum(categories...)),
		mcp.WithBoolean("pinned", mcp.Description("Pin to always appear at top of recall results. Use sparingly.")),
	}
	if cfg.MultiRepo {
		rememberOpts = append(rememberOpts, mcp.WithString("namespace",
			mcp.Description("Project namespace"),
			mcp.Enum(cfg.Namespaces...),
		))
	}
	s.AddTool(mcp.NewTool("remember", rememberOpts...), makeRememberHandler(remote, defaultNS))

	// ── forget ──────────────────────────────────────────────────────────
	s.AddTool(
		mcp.NewTool("forget",
			mcp.WithDescription("Delete a memory by ID. Use to clean up stale decisions or completed tasks."),
			mcp.WithString("id", mcp.Required(), mcp.Description("Memory ID from recall results")),
		),
		makeForgetHandler(remote),
	)
}

// resolveNamespace extracts namespace from request args or returns the default.
func resolveNamespace(req mcp.CallToolRequest, defaultNS string) string {
	if ns := req.GetString("namespace", ""); ns != "" {
		return ns
	}
	return defaultNS
}

func makeRecallHandler(remote *RemoteClient, cache *RecallCache, defaultNS string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns := resolveNamespace(req, defaultNS)
		query := req.GetString("query", "")

		// Serve cached auto-recall for the first no-query call
		if query == "" {
			if cached, ok := cache.Get(ns); ok {
				return cached, nil
			}
		}

		args := map[string]any{
			"namespace": ns,
			"limit":     20,
		}
		if query != "" {
			args["query"] = query
		}
		if cat := req.GetString("category", ""); cat != "" {
			args["category"] = cat
		}
		if limit, ok := req.GetArguments()["limit"]; ok {
			args["limit"] = limit
		}

		result, err := remote.CallTool(ctx, "recall", args)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return result, nil
	}
}

func makeRememberHandler(remote *RemoteClient, defaultNS string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ns := resolveNamespace(req, defaultNS)

		content, err := req.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		category, err := req.RequireString("category")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		args := map[string]any{
			"namespace": ns,
			"content":   content,
			"category":  category,
		}
		if pinned, ok := req.GetArguments()["pinned"]; ok {
			args["pinned"] = pinned
		}

		result, err := remote.CallTool(ctx, "remember", args)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return result, nil
	}
}

func makeForgetHandler(remote *RemoteClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		result, err := remote.CallTool(ctx, "forget", map[string]any{"id": id})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return result, nil
	}
}
