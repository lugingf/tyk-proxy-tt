package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"

	"tyk-proxy/internal/store"
)

type fakeVerifier struct {
	parseFn   func(tokenString string) (*Claims, error)
	calls     int
	lastToken string
}

func (f *fakeVerifier) Parse(tokenString string) (*Claims, error) {
	f.calls++
	f.lastToken = tokenString
	return f.parseFn(tokenString)
}

type fakeTokenStore struct {
	getFn   func(ctx context.Context, key string) (store.Token, error)
	calls   int
	lastKey string
	lastCtx context.Context
}

func (f *fakeTokenStore) GetToken(ctx context.Context, key string) (store.Token, error) {
	f.calls++
	f.lastCtx = ctx
	f.lastKey = key
	return f.getFn(ctx, key)
}

type fakeLimiter struct {
	allowFn   func(ctx context.Context, key string, limit int) (bool, error)
	calls     int
	lastKey   string
	lastLimit int
	lastCtx   context.Context
}

func (f *fakeLimiter) Allow(ctx context.Context, key string, limit int) (bool, error) {
	f.calls++
	f.lastCtx = ctx
	f.lastKey = key
	f.lastLimit = limit
	return f.allowFn(ctx, key, limit)
}

func newClaims(apiKey string, exp time.Time, routes []string) *Claims {
	return &Claims{
		APIKey:        apiKey,
		AllowedRoutes: routes,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
}

func TestAuthorizationMiddlewareService_extractBearer(t *testing.T) {
	m := &AuthorizationMiddlewareService{}
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"", "", false},
		{"Bearer", "", false},
		{"Bearer   ", "", false},
		{"Token xxx", "", false},
		{"Bearer xxx", "xxx", true},
		{"bearer xxx", "xxx", true},
		{"  Bearer   xxx  ", "xxx", true},
	}

	for _, tt := range tests {
		got, ok := m.extractBearer(tt.in)
		if ok != tt.ok || got != tt.want {
			t.Fatalf("extractBearer(%q) => (%q,%v), want (%q,%v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestAuthorizationMiddlewareService_isAllowedPath(t *testing.T) {
	m := &AuthorizationMiddlewareService{}
	tests := []struct {
		path     string
		patterns []string
		want     bool
	}{
		{"/api/v1/users/1", []string{"/api/v1/users/*"}, true},
		{"/api/v1/users", []string{"/api/v1/users/*"}, false}, // prefix is "/api/v1/users/" after trim "*"
		{"/api/v1/users", []string{"/api/v1/users*"}, true},
		{"/api/v1/products/42", []string{"/api/v1/users/*"}, false},
		{"/anything", []string{"*"}, true},
		{"", []string{"*"}, false},
	}

	for _, tt := range tests {
		got := m.isAllowedPath(tt.path, tt.patterns)
		if got != tt.want {
			t.Fatalf("isAllowedPath(%q,%v) => %v, want %v", tt.path, tt.patterns, got, tt.want)
		}
	}
}

func TestAuthMiddleware_MissingAuthorization_401(t *testing.T) {
	fs := &fakeTokenStore{getFn: func(ctx context.Context, key string) (store.Token, error) {
		return store.Token{}, nil
	}}
	fl := &fakeLimiter{allowFn: func(ctx context.Context, key string, limit int) (bool, error) {
		return true, nil
	}}
	fv := &fakeVerifier{parseFn: func(tokenString string) (*Claims, error) {
		return newClaims("k", time.Now().Add(time.Hour), []string{"/api/v1/*"}), nil
	}}

	mw := New(fs, fl, fv)

	nextCalled := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled++
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/test", nil)
	rr := httptest.NewRecorder()

	mw.Handler(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusUnauthorized)
	}
	if rr.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("expected WWW-Authenticate header")
	}
	if nextCalled != 0 {
		t.Fatalf("next should not be called")
	}
	if fv.calls != 0 || fs.calls != 0 || fl.calls != 0 {
		t.Fatalf("no dependencies should be called; verifier=%d store=%d limiter=%d", fv.calls, fs.calls, fl.calls)
	}
}

func TestAuthMiddleware_InvalidBearer_401(t *testing.T) {
	fs := &fakeTokenStore{getFn: func(ctx context.Context, key string) (store.Token, error) { return store.Token{}, nil }}
	fl := &fakeLimiter{allowFn: func(ctx context.Context, key string, limit int) (bool, error) { return true, nil }}
	fv := &fakeVerifier{parseFn: func(tokenString string) (*Claims, error) { return nil, nil }}

	mw := New(fs, fl, fv)

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/test", nil)
	req.Header.Set("Authorization", "Token abc")
	rr := httptest.NewRecorder()

	mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next must not be called")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusUnauthorized)
	}
	if fv.calls != 0 || fs.calls != 0 || fl.calls != 0 {
		t.Fatalf("no dependencies should be called; verifier=%d store=%d limiter=%d", fv.calls, fs.calls, fl.calls)
	}
}

func TestAuthMiddleware_VerifierError_401(t *testing.T) {
	fv := &fakeVerifier{parseFn: func(tokenString string) (*Claims, error) {
		return nil, errors.New("bad jwt")
	}}
	fs := &fakeTokenStore{getFn: func(ctx context.Context, key string) (store.Token, error) {
		return store.Token{}, nil
	}}
	fl := &fakeLimiter{allowFn: func(ctx context.Context, key string, limit int) (bool, error) { return true, nil }}

	mw := New(fs, fl, fv)

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer abc.def.ghi")
	rr := httptest.NewRecorder()

	mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next must not be called")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusUnauthorized)
	}
	if fv.calls != 1 {
		t.Fatalf("verifier calls=%d want=1", fv.calls)
	}
	if fs.calls != 0 || fl.calls != 0 {
		t.Fatalf("store/limiter should not be called; store=%d limiter=%d", fs.calls, fl.calls)
	}
}

func TestAuthMiddleware_MissingAPIKey_401(t *testing.T) {
	fv := &fakeVerifier{parseFn: func(tokenString string) (*Claims, error) {
		return newClaims("", time.Now().Add(time.Hour), []string{"/api/v1/*"}), nil
	}}
	fs := &fakeTokenStore{getFn: func(ctx context.Context, key string) (store.Token, error) { return store.Token{}, nil }}
	fl := &fakeLimiter{allowFn: func(ctx context.Context, key string, limit int) (bool, error) { return true, nil }}

	mw := New(fs, fl, fv)

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()

	mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next must not be called")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusUnauthorized)
	}
	if fs.calls != 0 || fl.calls != 0 {
		t.Fatalf("store/limiter should not be called; store=%d limiter=%d", fs.calls, fl.calls)
	}
}

func TestAuthMiddleware_ExpiredToken_401(t *testing.T) {
	now := time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC)

	fv := &fakeVerifier{parseFn: func(tokenString string) (*Claims, error) {
		return newClaims("k1", now.Add(-time.Second), []string{"/api/v1/*"}), nil
	}}
	fs := &fakeTokenStore{getFn: func(ctx context.Context, key string) (store.Token, error) { return store.Token{}, nil }}
	fl := &fakeLimiter{allowFn: func(ctx context.Context, key string, limit int) (bool, error) { return true, nil }}

	mw := New(fs, fl, fv)
	mw.WithOptions(&Options{Now: func() time.Time { return now }})

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()

	mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next must not be called")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusUnauthorized)
	}
	if fs.calls != 0 || fl.calls != 0 {
		t.Fatalf("store/limiter should not be called on expired; store=%d limiter=%d", fs.calls, fl.calls)
	}
}

func TestAuthMiddleware_PathNotAllowed_403(t *testing.T) {
	now := time.Now().UTC()

	fv := &fakeVerifier{parseFn: func(tokenString string) (*Claims, error) {
		return newClaims("k1", now.Add(time.Hour), []string{"/api/v1/users/*"}), nil
	}}
	fs := &fakeTokenStore{getFn: func(ctx context.Context, key string) (store.Token, error) {
		return store.Token{RateLimit: 10}, nil
	}}
	fl := &fakeLimiter{allowFn: func(ctx context.Context, key string, limit int) (bool, error) {
		return true, nil
	}}

	mw := New(fs, fl, fv)
	mw.WithOptions(&Options{Now: func() time.Time { return now }})

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/products/1", nil)
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()

	mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next must not be called")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusForbidden)
	}
	if rr.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("WWW-Authenticate should not be set for 403")
	}
	if fs.calls != 0 || fl.calls != 0 {
		t.Fatalf("store/limiter should not be called when path forbidden; store=%d limiter=%d", fs.calls, fl.calls)
	}
}

func TestAuthMiddleware_StoreNotFound_401(t *testing.T) {
	now := time.Now().UTC()

	fv := &fakeVerifier{parseFn: func(tokenString string) (*Claims, error) {
		return newClaims("k1", now.Add(time.Hour), []string{"/api/v1/*"}), nil
	}}
	fs := &fakeTokenStore{getFn: func(ctx context.Context, key string) (store.Token, error) {
		return store.Token{}, store.ErrNotFound
	}}
	fl := &fakeLimiter{allowFn: func(ctx context.Context, key string, limit int) (bool, error) {
		return true, nil
	}}

	mw := New(fs, fl, fv)
	mw.WithOptions(&Options{Now: func() time.Time { return now }})

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()

	mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next must not be called")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusUnauthorized)
	}
	if fs.calls != 1 {
		t.Fatalf("store calls=%d want=1", fs.calls)
	}
	if fl.calls != 0 {
		t.Fatalf("limiter should not be called")
	}
}

func TestAuthMiddleware_StoreBackendError_503(t *testing.T) {
	now := time.Now().UTC()

	fv := &fakeVerifier{parseFn: func(tokenString string) (*Claims, error) {
		return newClaims("k1", now.Add(time.Hour), []string{"/api/v1/*"}), nil
	}}
	fs := &fakeTokenStore{getFn: func(ctx context.Context, key string) (store.Token, error) {
		return store.Token{}, errors.New("redis unavailable")
	}}
	fl := &fakeLimiter{allowFn: func(ctx context.Context, key string, limit int) (bool, error) {
		return true, nil
	}}

	mw := New(fs, fl, fv)
	mw.WithOptions(&Options{Now: func() time.Time { return now }})

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()

	mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next must not be called")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusServiceUnavailable)
	}
	if fs.calls != 1 {
		t.Fatalf("store calls=%d want=1", fs.calls)
	}
	if fl.calls != 0 {
		t.Fatalf("limiter should not be called")
	}
}

func TestAuthMiddleware_TokenDisabled_401(t *testing.T) {
	now := time.Now().UTC()

	fv := &fakeVerifier{parseFn: func(tokenString string) (*Claims, error) {
		return newClaims("k1", now.Add(time.Hour), []string{"/api/v1/*"}), nil
	}}
	fs := &fakeTokenStore{getFn: func(ctx context.Context, key string) (store.Token, error) {
		return store.Token{RateLimit: 0}, nil
	}}
	fl := &fakeLimiter{allowFn: func(ctx context.Context, key string, limit int) (bool, error) {
		return true, nil
	}}

	mw := New(fs, fl, fv)
	mw.WithOptions(&Options{Now: func() time.Time { return now }})

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()

	mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next must not be called")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusUnauthorized)
	}
	if fl.calls != 0 {
		t.Fatalf("limiter should not be called when token disabled")
	}
}

func TestAuthMiddleware_LimiterError_500(t *testing.T) {
	now := time.Now().UTC()

	fv := &fakeVerifier{parseFn: func(tokenString string) (*Claims, error) {
		return newClaims("k1", now.Add(time.Hour), []string{"/api/v1/*"}), nil
	}}
	fs := &fakeTokenStore{getFn: func(ctx context.Context, key string) (store.Token, error) {
		return store.Token{RateLimit: 5}, nil
	}}
	fl := &fakeLimiter{allowFn: func(ctx context.Context, key string, limit int) (bool, error) {
		return false, errors.New("redis down")
	}}

	mw := New(fs, fl, fv)
	mw.WithOptions(&Options{Now: func() time.Time { return now }})

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()

	mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next must not be called")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusInternalServerError)
	}
}

func TestAuthMiddleware_TooManyRequests_429(t *testing.T) {
	now := time.Now().UTC()

	fv := &fakeVerifier{parseFn: func(tokenString string) (*Claims, error) {
		return newClaims("k1", now.Add(time.Hour), []string{"/api/v1/*"}), nil
	}}
	fs := &fakeTokenStore{getFn: func(ctx context.Context, key string) (store.Token, error) {
		return store.Token{RateLimit: 5}, nil
	}}
	fl := &fakeLimiter{allowFn: func(ctx context.Context, key string, limit int) (bool, error) {
		return false, nil
	}}

	mw := New(fs, fl, fv)
	mw.WithOptions(&Options{Now: func() time.Time { return now }})

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	rr := httptest.NewRecorder()

	mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next must not be called")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusTooManyRequests)
	}
}

func TestAuthMiddleware_Success_PassesClaimsInContext(t *testing.T) {
	now := time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC)

	fv := &fakeVerifier{parseFn: func(tokenString string) (*Claims, error) {
		return newClaims("k1", now.Add(10*time.Minute), []string{"/api/v1/*"}), nil
	}}
	fs := &fakeTokenStore{getFn: func(ctx context.Context, key string) (store.Token, error) {
		return store.Token{RateLimit: 7}, nil
	}}
	fl := &fakeLimiter{allowFn: func(ctx context.Context, key string, limit int) (bool, error) {
		return true, nil
	}}

	mw := New(fs, fl, fv)
	mw.WithOptions(&Options{Now: func() time.Time { return now }})

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := ClaimsFromContext(r.Context())
		if !ok || c == nil || c.APIKey != "k1" {
			t.Fatalf("expected claims in context with api_key=k1; got ok=%v claims=%v", ok, c)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "http://example/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer good.jwt.token")
	rr := httptest.NewRecorder()

	mw.Handler(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusOK)
	}
	if fv.calls != 1 {
		t.Fatalf("verifier calls=%d want=1", fv.calls)
	}
	if fs.calls != 1 || fs.lastKey != "k1" {
		t.Fatalf("store calls=%d lastKey=%q want calls=1 lastKey=k1", fs.calls, fs.lastKey)
	}
	if fl.calls != 1 || fl.lastKey != "k1" || fl.lastLimit != 7 {
		t.Fatalf("limiter calls=%d lastKey=%q lastLimit=%d want calls=1 lastKey=k1 lastLimit=7",
			fl.calls, fl.lastKey, fl.lastLimit)
	}
}
