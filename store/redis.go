package store

import (
	"context"
	"fmt"
	redis2 "github.com/go-redis/redis/v8"
	"time"
)

type redis struct {
	*redis2.Client
}

func NewRedis(client *redis2.Client) *redis {
	return &redis{client}
}

func (r *redis) Del(ctx context.Context, key string) error {
	fmt.Print("del key:", key, "\n")
	return r.Client.Del(ctx, key).Err()
}

func (r *redis) Set(ctx context.Context, key string, val interface{}, ttl time.Duration) error {
	fmt.Print("set key:", key, val, "\n")
	return r.Client.Set(ctx, key, val, ttl).Err()
}

func (r *redis) Get(ctx context.Context, key string, val interface{}) error {
	fmt.Print("get key:", key, "\n")
	return r.Client.Get(ctx, key).Scan(val)
}
