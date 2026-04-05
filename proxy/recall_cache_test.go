package main

import (
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestRecallCache_GetReturnsData(t *testing.T) {
	cache := &RecallCache{
		data:     make(map[string]*mcp.CallToolResult),
		consumed: make(map[string]bool),
	}

	expected := mcp.NewToolResultText("cached memories")
	cache.data["myproject"] = expected

	result, ok := cache.Get("myproject")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if result != expected {
		t.Error("returned result does not match cached data")
	}
}

func TestRecallCache_GetMarksConsumed(t *testing.T) {
	cache := &RecallCache{
		data:     make(map[string]*mcp.CallToolResult),
		consumed: make(map[string]bool),
	}

	cache.data["myproject"] = mcp.NewToolResultText("data")

	// First call returns data
	_, ok := cache.Get("myproject")
	if !ok {
		t.Fatal("first Get should return data")
	}

	// Second call returns miss (consumed)
	_, ok = cache.Get("myproject")
	if ok {
		t.Error("second Get should return false (consumed)")
	}
}

func TestRecallCache_GetMissForUnknownNamespace(t *testing.T) {
	cache := &RecallCache{
		data:     make(map[string]*mcp.CallToolResult),
		consumed: make(map[string]bool),
	}

	_, ok := cache.Get("nonexistent")
	if ok {
		t.Error("expected cache miss for unknown namespace")
	}
}

func TestRecallCache_GetMissWhenAlreadyConsumed(t *testing.T) {
	cache := &RecallCache{
		data:     make(map[string]*mcp.CallToolResult),
		consumed: make(map[string]bool),
	}

	cache.data["proj"] = mcp.NewToolResultText("data")
	cache.consumed["proj"] = true

	_, ok := cache.Get("proj")
	if ok {
		t.Error("expected miss when already consumed")
	}
}

func TestRecallCache_ConcurrentAccess(t *testing.T) {
	cache := &RecallCache{
		data:     make(map[string]*mcp.CallToolResult),
		consumed: make(map[string]bool),
	}

	cache.data["proj"] = mcp.NewToolResultText("data")

	var wg sync.WaitGroup
	hits := make(chan bool, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok := cache.Get("proj")
			hits <- ok
		}()
	}

	wg.Wait()
	close(hits)

	hitCount := 0
	for ok := range hits {
		if ok {
			hitCount++
		}
	}

	if hitCount != 1 {
		t.Errorf("expected exactly 1 cache hit across concurrent callers, got %d", hitCount)
	}
}
