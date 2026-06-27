package cache

import (
	"context"
	"testing"
	"time"
)

func TestMemoryCache_Ping_Nil(t *testing.T) {
	c := NewMemoryCache(0)
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("memory cache Ping should be nil, got %v", err)
	}
}

func TestFileCache_Ping_Nil(t *testing.T) {
	c := NewFileCache()
	c.SetDir(t.TempDir())
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("file cache Ping should be nil, got %v", err)
	}
}

func TestSimpleCache_Ping_Nil(t *testing.T) {
	s := NewSimpleCache()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := s.Ping(ctx); err != nil {
		t.Fatalf("simple cache Ping should be nil, got %v", err)
	}
}
