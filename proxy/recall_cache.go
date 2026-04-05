package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// RecallCache stores auto-recalled results per namespace.
// Results are served once per namespace, then subsequent calls go to the remote.
type RecallCache struct {
	remote *RemoteClient
	cfg    Config

	mu       sync.Mutex
	data     map[string]*mcp.CallToolResult
	consumed map[string]bool
}

func NewRecallCache(remote *RemoteClient, cfg Config) *RecallCache {
	return &RecallCache{
		remote:   remote,
		cfg:      cfg,
		data:     make(map[string]*mcp.CallToolResult),
		consumed: make(map[string]bool),
	}
}

// Get returns the cached recall result for a namespace if available and not yet consumed.
func (c *RecallCache) Get(namespace string) (*mcp.CallToolResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.consumed[namespace] {
		return nil, false
	}

	result, ok := c.data[namespace]
	if ok {
		c.consumed[namespace] = true
	}
	return result, ok
}

// TriggerAutoRecall fires a background recall for each configured namespace.
// Called from the OnRegisterSession hook.
func (c *RecallCache) TriggerAutoRecall() {
	for _, ns := range c.cfg.Namespaces {
		go c.fetchNamespace(ns)
	}
}

func (c *RecallCache) fetchNamespace(ns string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := c.remote.CallTool(ctx, "recall", map[string]any{
		"namespace": ns,
		"limit":     20,
	})
	if err != nil {
		log.Printf("auto-recall failed for namespace %q: %v", ns, err)
		return
	}

	c.mu.Lock()
	c.data[ns] = result
	c.mu.Unlock()

	log.Printf("auto-recall cached %d content blocks for namespace %q", len(result.Content), ns)
}
