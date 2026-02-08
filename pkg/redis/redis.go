package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	*redis.Client
}

func NewRedis(ctx context.Context, addr string) (*Redis, error) {
	rd := redis.NewClient(&redis.Options{Addr: addr})
	st := rd.Ping(ctx)
	if st.Err() != nil {
		return nil, st.Err()
	}

	return &Redis{rd}, nil
}
