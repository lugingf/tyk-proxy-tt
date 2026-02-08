package service

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

type store interface {
	Incr(ctx context.Context, key string, window time.Duration) (int64, error)
}

type RateLimit struct {
	store  store
	window time.Duration
}

type Options struct {
	Window time.Duration
}

func NewRateLimit(s store) *RateLimit {
	return NewRateLimitWithOptions(s, Options{})
}

func NewRateLimitWithOptions(s store, opts Options) *RateLimit {
	w := opts.Window
	if w <= 0 {
		w = time.Minute // N requests per minute by default
	}

	return &RateLimit{
		store:  s,
		window: w,
	}
}

func (rl *RateLimit) Allow(ctx context.Context, key string, limit int) (bool, error) {
	if key == "" {
		return false, errors.New("rate limit: empty key")
	}

	if limit <= 0 {
		return false, errors.New("rate limit: limit must be > 0")
	}

	n, err := rl.store.Incr(ctx, key, rl.window)
	log.Debug().Str("key", key).Int64("n", n).Msg("current rate")

	if err != nil {
		return false, errors.Wrap(err, "rate limit: failed to increment counter")
	}

	return n <= int64(limit), nil
}
