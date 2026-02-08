package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"tyk-proxy/internal/store"

	"github.com/rs/zerolog/log"
)

type Token struct {
	APIKey        string    `json:"api_key"`
	RateLimit     int       `json:"rate_limit"`
	ExpiresAt     time.Time `json:"expires_at"`
	AllowedRoutes []string  `json:"allowed_routes"`
}

type tokenStore interface {
	GetToken(ctx context.Context, key string) (store.Token, error)
}

type limiter interface {
	Allow(ctx context.Context, key string, limit int) (bool, error)
}

// verifier verifies and parses JWT.
type verifier interface {
	Parse(tokenString string) (*Claims, error)
}

type AuthorizationMiddlewareService struct {
	store    tokenStore
	limiter  limiter
	verifier verifier
	now      func() time.Time
}

type Options struct {
	Now func() time.Time
}

func New(store tokenStore, limiter limiter, verifier verifier) *AuthorizationMiddlewareService {
	return &AuthorizationMiddlewareService{
		store:    store,
		limiter:  limiter,
		verifier: verifier,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (m *AuthorizationMiddlewareService) WithOptions(opts *Options) {
	if opts == nil {
		opts = &Options{}
	}

	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	m.now = now
}

func (m *AuthorizationMiddlewareService) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Debug().Msg("authenticating starting")

		jwtStr, ok := m.extractBearer(r.Header.Get("Authorization"))
		if !ok {
			m.unauthorized(w, "missing bearer token")
			return
		}

		log.Debug().Msg("getting claims from token")
		claims, err := m.verifier.Parse(jwtStr)
		if err != nil {
			m.unauthorized(w, "invalid token: "+err.Error())
			return
		}

		log.Debug().Msg("token parsed successfully")

		if claims.APIKey == "" {
			m.unauthorized(w, "missing api_key claim")
			return
		}

		exp, err := claims.GetExpirationTime()
		if err != nil || exp == nil || !exp.Time.After(m.now()) {
			m.unauthorized(w, "token expired")
			return
		}

		if len(claims.AllowedRoutes) > 0 {
			if !m.isAllowedPath(r.URL.Path, claims.AllowedRoutes) {
				log.Debug().Str("path", r.URL.Path).Msg("path not allowed")
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}

		tok, err := m.store.GetToken(r.Context(), claims.APIKey)
		if err != nil {
			log.Debug().Err(err).Str("api_key", claims.APIKey).Msg("token not found in store")
			m.unauthorized(w, "unknown token: "+err.Error())
			return
		}

		limit := tok.RateLimit
		log.Debug().Int("limit", limit).Str("api", claims.APIKey).Msg("rate limit")

		if limit <= 0 {
			m.unauthorized(w, "token disabled")
			return
		}

		allowed, err := m.limiter.Allow(r.Context(), claims.APIKey, limit)
		if err != nil {
			http.Error(w, "Rate limiter error", http.StatusInternalServerError)
			return
		}

		if !allowed {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		log.Debug().
			Int("limit", limit).
			Bool("all", allowed).
			Str("api", claims.APIKey).
			Msg("access allowed")

		ctx := WithClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *AuthorizationMiddlewareService) extractBearer(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}

	parts := strings.SplitN(v, " ", 2)
	if len(parts) != 2 {
		return "", false
	}

	if !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}

	t := strings.TrimSpace(parts[1])
	return t, t != ""
}

func (m *AuthorizationMiddlewareService) unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="api"`)
	log.Info().Msg(msg)

	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

func (m *AuthorizationMiddlewareService) isAllowedPath(path string, patterns []string) bool {
	if path == "" {
		return false
	}
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "*" {
			return true
		}
		// "/api/v1/users/*" or "/api/v1/users*"
		if strings.HasSuffix(p, "*") {
			prefix := strings.TrimSuffix(p, "*")
			if strings.HasPrefix(path, prefix) {
				return true
			}
			continue
		}
		// exact match
		if path == p {
			return true
		}
	}
	return false
}

//
// Context: store claims for downstream
//

type ctxKeyClaims struct{}

func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, ctxKeyClaims{}, c)
}

func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	v := ctx.Value(ctxKeyClaims{})
	if v == nil {
		return nil, false
	}
	c, ok := v.(*Claims)
	return c, ok
}
