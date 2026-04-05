package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// RemoteClient wraps an MCP StreamableHTTP client to the remote Worker.
// Uses mutex-based lazy init so transient failures can retry.
type RemoteClient struct {
	cfg   Config
	mu    sync.Mutex
	c     *client.Client
	ready bool
}

func NewRemoteClient(cfg Config) *RemoteClient {
	return &RemoteClient{cfg: cfg}
}

func (r *RemoteClient) ensureInit(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ready {
		return nil
	}

	headers := map[string]string{
		"x-service-auth": r.cfg.AuthToken,
	}
	for k, v := range r.cfg.ExtraHeaders {
		headers[k] = v
	}

	c, err := client.NewStreamableHttpClient(r.cfg.RemoteURL,
		transport.WithHTTPHeaders(headers),
		transport.WithHTTPTimeout(10*time.Second),
	)
	if err != nil {
		return fmt.Errorf("failed to create transport: %w", err)
	}

	if err := c.Start(ctx); err != nil {
		c.Close()
		return fmt.Errorf("failed to start client: %w", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "memory-mcp-proxy",
		Version: "1.0.0",
	}

	if _, err := c.Initialize(ctx, initReq); err != nil {
		c.Close()
		return fmt.Errorf("failed to initialize remote: %w", err)
	}

	r.c = c
	r.ready = true
	return nil
}

// CallTool forwards a tool call to the remote Worker.
func (r *RemoteClient) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	if err := r.ensureInit(ctx); err != nil {
		return nil, err
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := r.c.CallTool(ctx, req)
	if err != nil {
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "Unauthorized") {
			return nil, fmt.Errorf("authentication failed — check MEMORY_MCP_TOKEN (%w)", err)
		}
		return nil, fmt.Errorf("remote call to %q failed: %w", name, err)
	}
	return result, nil
}

// Close shuts down the underlying client if initialized.
func (r *RemoteClient) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ready && r.c != nil {
		r.c.Close()
		r.ready = false
	}
}
