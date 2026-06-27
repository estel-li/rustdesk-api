package cache

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"reflect"
	"testing"
	"time"
)

func TestRedisSet(t *testing.T) {
	//rc := New("redis")
	rc := RedisCacheInit(&redis.Options{
		Addr:     "192.168.1.168:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	err := rc.Set("123", "ddd", 0)
	if err != nil {
		fmt.Println(err.Error())
		t.Fatalf("写入失败")
	}
}

func TestRedisGet(t *testing.T) {
	rc := RedisCacheInit(&redis.Options{
		Addr:     "192.168.1.168:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	err := rc.Set("123", "451156", 300)
	if err != nil {
		t.Fatalf("写入失败")
	}
	res := ""
	err = rc.Get("123", &res)
	if err != nil {
		t.Fatalf("读取失败")
	}
	fmt.Println("res", res)
}

func TestRedisGetJson(t *testing.T) {
	rc := RedisCacheInit(&redis.Options{
		Addr:     "192.168.1.168:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	type r struct {
		Aa string `json:"a"`
		B  string `json:"c"`
	}
	old := &r{
		Aa: "ab", B: "cdc",
	}
	err := rc.Set("1233", old, 300)
	if err != nil {
		t.Fatalf("写入失败")
	}

	res := &r{}
	err2 := rc.Get("1233", res)
	if err2 != nil {
		t.Fatalf("读取失败")
	}
	if !reflect.DeepEqual(res, old) {
		t.Fatalf("读取错误")
	}
	fmt.Println(res, res.Aa)
}

func BenchmarkRSet(b *testing.B) {
	rc := RedisCacheInit(&redis.Options{
		Addr:     "192.168.1.168:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rc.Set("123", "{dsv}", 1000)
	}
}

func BenchmarkRGet(b *testing.B) {
	rc := RedisCacheInit(&redis.Options{
		Addr:     "192.168.1.168:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	b.ResetTimer()
	v := ""
	for i := 0; i < b.N; i++ {
		rc.Get("123", &v)
	}
}

// TestRedisCache_Ping_ReturnsErrorOnBadAddr 校验 RedisCache.Ping 能在 redis 不可达时返回错误,
// 这是 cmd/apimain.go fallback 到内存缓存的依据。
func TestRedisCache_Ping_ReturnsErrorOnBadAddr(t *testing.T) {
	rc := RedisCacheInit(&redis.Options{
		Addr:        "127.0.0.1:1", // 不存在的端口
		DialTimeout: 200 * time.Millisecond,
		ReadTimeout: 200 * time.Millisecond,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := rc.Ping(ctx); err == nil {
		t.Fatalf("expected Ping to fail on unreachable redis, got nil")
	}
}
