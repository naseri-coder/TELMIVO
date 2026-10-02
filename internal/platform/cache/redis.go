package cache

import (
	"context"
	"github.com/redis/go-redis/v9"
	"time"
)

type Client struct{ Redis *redis.Client }

func Open(raw string) (*Client, error) {
	o, e := redis.ParseURL(raw)
	if e != nil {
		return nil, e
	}
	o.DialTimeout = 3 * time.Second
	o.ReadTimeout = 2 * time.Second
	o.WriteTimeout = 2 * time.Second
	return &Client{Redis: redis.NewClient(o)}, nil
}
func (c *Client) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	n, e := c.Redis.Eval(ctx, "local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('PEXPIRE',KEYS[1],ARGV[1]) end; return n", []string{"telmivo:rate:" + key}, window.Milliseconds()).Int64()
	return n <= limit, e
}
