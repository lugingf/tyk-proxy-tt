package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

type Token struct {
	APIKey        string    `json:"api_key"`
	RateLimit     int       `json:"rate_limit"`
	ExpiresAt     time.Time `json:"expires_at"`
	AllowedRoutes []string  `json:"allowed_routes"`
}

var (
	ErrNotFound = errors.New("token not found")
	ErrExpired  = errors.New("token expired")
	ErrInvalid  = errors.New("token record invalid")
)

type Store struct {
	rdcl   redis.UniversalClient
	prefix string

	// for tests
	now func() time.Time
}

type Options struct {
	Prefix string

	// for tests
	Now func() time.Time
}

func NewStore(rdcl redis.UniversalClient, pfx string) *Store {
	if pfx == "" {
		pfx = "token:"
	}

	return &Store{
		rdcl:   rdcl,
		prefix: pfx,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func (s *Store) WithOptions(opts *Options) {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	s.now = now
}

func (s *Store) key(apiKey string) string {
	return s.prefix + apiKey
}

func (s *Store) Upsert(ctx context.Context, t Token) error {
	if t.APIKey == "" {
		return fmt.Errorf("%w: empty api_key", ErrInvalid)
	}

	if t.RateLimit <= 0 {
		return fmt.Errorf("%w: rate_limit must be > 0", ErrInvalid)
	}

	if t.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: expires_at is required", ErrInvalid)
	}

	now := s.now()
	if !t.ExpiresAt.After(now) {
		return ErrExpired
	}

	ar, err := json.Marshal(t.AllowedRoutes)
	if err != nil {
		return fmt.Errorf("%w: allowed_routes marshal: %v", ErrInvalid, err)
	}

	key := s.key(t.APIKey)

	pipe := s.rdcl.TxPipeline()
	pipe.HSet(ctx, key, map[string]any{
		"api_key":        t.APIKey,
		"rate_limit":     strconv.Itoa(t.RateLimit),
		"expires_at":     t.ExpiresAt.UTC().Format(time.RFC3339),
		"allowed_routes": string(ar), // JSON array
	})

	pipe.ExpireAt(ctx, key, t.ExpiresAt.UTC()) // auto-expire
	_, err = pipe.Exec(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (s *Store) GetToken(ctx context.Context, apiKey string) (Token, error) {
	if apiKey == "" {
		return Token{}, fmt.Errorf("%w: empty api_key", ErrInvalid)
	}

	key := s.key(apiKey)
	log.Debug().Str("key", key).Msg("getting token")

	m, err := s.rdcl.HGetAll(ctx, key).Result()
	if err != nil {
		return Token{}, err
	}
	if len(m) == 0 {
		return Token{}, ErrNotFound
	}

	t, err := decodeToken(apiKey, m)
	if err != nil {
		return Token{}, err
	}

	if !t.ExpiresAt.After(s.now()) {
		// best-effort cleanup
		_, _ = s.rdcl.Del(ctx, key).Result()
		return Token{}, ErrExpired
	}

	return t, nil
}

func (s *Store) Delete(ctx context.Context, apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("%w: empty api_key", ErrInvalid)
	}
	_, err := s.rdcl.Del(ctx, s.key(apiKey)).Result()
	return err
}

func decodeToken(apiKey string, m map[string]string) (Token, error) {
	var t Token

	// unbelivable case, but still let's protect ourselves
	t.APIKey = apiKey
	if v := m["api_key"]; v != "" {
		t.APIKey = v
	}

	rls := m["rate_limit"]
	if rls == "" {
		return Token{}, fmt.Errorf("%w: missing rate_limit", ErrInvalid)
	}

	rl, err := strconv.Atoi(rls)
	if err != nil || rl <= 0 {
		return Token{}, fmt.Errorf("%w: invalid rate_limit", ErrInvalid)
	}
	t.RateLimit = rl

	exps := m["expires_at"]
	if exps == "" {
		return Token{}, fmt.Errorf("%w: missing expires_at", ErrInvalid)
	}

	exp, err := time.Parse(time.RFC3339, exps)
	if err != nil {
		return Token{}, fmt.Errorf("%w: invalid expires_at: %v", ErrInvalid, err)
	}

	t.ExpiresAt = exp.UTC()

	routes := m["allowed_routes"]
	if routes == "" {
		t.AllowedRoutes = nil
		return t, nil
	}

	if err := json.Unmarshal([]byte(routes), &t.AllowedRoutes); err != nil {
		return Token{}, fmt.Errorf("%w: invalid allowed_routes: %v", ErrInvalid, err)
	}

	return t, nil
}
