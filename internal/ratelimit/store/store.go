package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Store struct {
	rdcl   redis.UniversalClient
	prefix string

	// for tests
	now func() time.Time
}

type Options struct {
	Prefix string
	Now    func() time.Time
}

func NewStore(rdcl redis.UniversalClient, opts Options) *Store {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	pfx := opts.Prefix
	if pfx == "" {
		pfx = "rate_count:"
	}
	return &Store{
		rdcl:   rdcl,
		prefix: pfx,
		now:    now,
	}
}

func (s *Store) counterKey(key string, window time.Duration) string {
	ws := windowStart(s.now(), window).Unix()
	return fmt.Sprintf("%s%s:%d", s.prefix, key, ws)
}

var incrScript = redis.NewScript(`
	local c = redis.call("INCR", KEYS[1])
	if c == 1 then
	  redis.call("PEXPIRE", KEYS[1], ARGV[1])
	end
	return c
`)

func (s *Store) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	if window <= 0 {
		return 0, errors.New("store: window must be > 0")
	}

	k := s.counterKey(key, window)

	ttlMs := window.Milliseconds() + 1000

	v, err := incrScript.Run(ctx, s.rdcl, []string{k}, ttlMs).Int64()
	if err != nil {
		return 0, err
	}

	return v, nil
}

func windowStart(t time.Time, window time.Duration) time.Time {
	sec := int64(window.Seconds())
	if sec <= 0 {
		return t.UTC()
	}

	u := t.Unix()
	ws := (u / sec) * sec
	return time.Unix(ws, 0).UTC()
}
