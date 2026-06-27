package cache

import (
	"context"
	"github.com/go-redis/redis/v8"
	"time"
)

var ctx = context.Background()

type RedisCache struct {
	rdb *redis.Client
}

func RedisCacheInit(conf *redis.Options) *RedisCache {
	c := &RedisCache{}
	c.rdb = redis.NewClient(conf)
	return c
}

func (c *RedisCache) Get(key string, value interface{}) error {
	data, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return err
	}
	err1 := DecodeValue(data, value)
	return err1
}

func (c *RedisCache) Set(key string, value interface{}, exp int) error {
	str, err := EncodeValue(value)
	if err != nil {
		return err
	}
	if exp <= 0 {
		exp = MaxTimeOut
	}
	_, err1 := c.rdb.Set(ctx, key, str, time.Duration(exp)*time.Second).Result()
	return err1
}

func (c *RedisCache) Gc() error {
	return nil
}

// Ping 探测 redis 连通性,用于启动期 healthcheck 与 /metrics 后端可达性检查。
func (c *RedisCache) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

func NewRedis(conf *redis.Options) *RedisCache {
	cache := RedisCacheInit(conf)
	return cache
}
