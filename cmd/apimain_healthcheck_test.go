package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/lib/cache"
	"github.com/sirupsen/logrus"
)

func newTestLogger() (*logrus.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	log := logrus.New()
	log.SetOutput(buf)
	log.SetLevel(logrus.DebugLevel)
	return log, buf
}

// TestInitCacheWithFallback_RedisUnreachable_FallsBackToMemory
// 验证 redis 不可达时 initCacheWithFallback 退化到内存缓存,且日志含 fallback 关键字。
func TestInitCacheWithFallback_RedisUnreachable_FallsBackToMemory(t *testing.T) {
	log, buf := newTestLogger()
	cfg := &config.Config{}
	cfg.Cache.Type = cache.TypeRedis
	cfg.Cache.RedisAddr = "127.0.0.1:1" // 不存在的端口

	c := initCacheWithFallback(cfg, log)

	if _, ok := c.(*cache.MemoryCache); !ok {
		t.Fatalf("expected fallback to *MemoryCache, got %T", c)
	}
	if !strings.Contains(buf.String(), "fallback") {
		t.Fatalf("expected log to mention fallback, got: %s", buf.String())
	}
}

// TestInitCacheWithFallback_EmptyType_DefaultsToMemory
// 验证 cache.type=="" (老配置文件) 时返回内存缓存,而非 nil(修复历史 panic)。
func TestInitCacheWithFallback_EmptyType_DefaultsToMemory(t *testing.T) {
	log, _ := newTestLogger()
	cfg := &config.Config{}
	cfg.Cache.Type = ""

	c := initCacheWithFallback(cfg, log)
	if c == nil {
		t.Fatalf("expected non-nil cache for empty cache.type, got nil")
	}
	if _, ok := c.(*cache.MemoryCache); !ok {
		t.Fatalf("expected *MemoryCache for empty cache.type, got %T", c)
	}
}

// TestInitCacheWithFallback_TypeFile_NoFallback
// 验证 cache.type=file 走 FileCache,且不被误判为不可达。
func TestInitCacheWithFallback_TypeFile_NoFallback(t *testing.T) {
	log, _ := newTestLogger()
	cfg := &config.Config{}
	cfg.Cache.Type = cache.TypeFile
	cfg.Cache.FileDir = t.TempDir()

	c := initCacheWithFallback(cfg, log)
	if _, ok := c.(*cache.FileCache); !ok {
		t.Fatalf("expected *FileCache, got %T", c)
	}
}

// TestInitCacheWithFallback_TypeMemory_ReturnsMemoryCache
func TestInitCacheWithFallback_TypeMemory_ReturnsMemoryCache(t *testing.T) {
	log, _ := newTestLogger()
	cfg := &config.Config{}
	cfg.Cache.Type = cache.TypeMem

	c := initCacheWithFallback(cfg, log)
	if _, ok := c.(*cache.MemoryCache); !ok {
		t.Fatalf("expected *MemoryCache, got %T", c)
	}
}
