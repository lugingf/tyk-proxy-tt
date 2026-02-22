package handler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"tyk-proxy/internal/auth"
	"tyk-proxy/internal/config"
	mp "tyk-proxy/internal/metrics"
)

const maxBodyBytes int64 = 10 << 20 // 10 MiB // TODO to config

type Proxy struct {
	target string
	authMw *auth.AuthorizationMiddlewareService

	rdcl redis.UniversalClient
}

func NewHandler(target string, authMw *auth.AuthorizationMiddlewareService, rdcl redis.UniversalClient) *Proxy {
	return &Proxy{target: target, authMw: authMw, rdcl: rdcl}
}

func setRequestIDHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rid := middleware.GetReqID(r.Context()); rid != "" {
			w.Header().Set(middleware.RequestIDHeader, rid)
			w.Header().Set("X-Request-ID", rid)
		}

		next.ServeHTTP(w, r)
	})
}

func GetRouter(h *Proxy, metrics *mp.Metrics) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.CleanPath)
	r.Use(middleware.RequestID)
	r.Use(setRequestIDHeader)
	r.Use(middleware.RequestSize(maxBodyBytes))
	r.Use(middleware.RequestLogger(&config.ChiZerologFormatter{}))
	r.Use(middleware.Recoverer)
	r.Use(metrics.MetricsMiddleware)

	r.Get("/health", h.Health())
	r.Get("/ready", h.Ready())

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(h.authMw.Handler)
		r.Handle("/*", h.Handler(h.target))
	})

	return r
}

func (h *Proxy) Handler(targetURL string) http.HandlerFunc {
	target, err := url.Parse(targetURL)
	if err != nil || target.Scheme == "" || target.Host == "" {
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "invalid upstream target", http.StatusInternalServerError)
		}
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = newUpstreamTransport()
	proxy.FlushInterval = 100 * time.Millisecond

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		var mbe *http.MaxBytesError
		if errors.As(e, &mbe) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		if errors.Is(e, context.DeadlineExceeded) {
			http.Error(w, "gateway timeout", http.StatusGatewayTimeout)
			return
		}

		http.Error(w, "bad gateway", http.StatusBadGateway)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if rid := middleware.GetReqID(r.Context()); rid != "" {
			r.Header.Set("X-Request-ID", rid)
		}
		r.Host = target.Host
		proxy.ServeHTTP(w, r)
	}
}

func newUpstreamTransport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
	}
}
