package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"

	"tyk-proxy/internal/auth"
	"tyk-proxy/internal/config"
	"tyk-proxy/internal/handler"
	"tyk-proxy/internal/metrics"
	rate "tyk-proxy/internal/ratelimit/service"
	rs "tyk-proxy/internal/ratelimit/store"
	"tyk-proxy/internal/store"
	"tyk-proxy/pkg/redis"
	"tyk-proxy/pkg/version"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := preStart()
	if opts.showVersion {
		fmt.Printf("version: %s\n", version.GetVersion())
		return
	}

	cfg, err := config.ReadConfig(opts.configPath, &opts.envOverrides)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	if err := cfg.ValidateAndNormalize(); err != nil {
		slog.Error("Invalid configuration", "error", err)
		os.Exit(1)
	}

	config.InitLogger(cfg)
	log.Info().Str("level", cfg.Log.Level).Msg("Logger initialized")

	mtx := metrics.GetMetrics()

	verifier := auth.NewJWTVerifier(auth.KeySet{
		ExpectedAlg: cfg.Application.Token.Algorithm,
		DefaultKey:  []byte(cfg.Application.Token.JWTSecret),
	})

	redisConnectCtx, redisConnectCancel := context.WithTimeout(ctx, 5*time.Second)
	defer redisConnectCancel()

	rd, err := redis.NewRedis(redisConnectCtx, cfg.Redis.Addr)
	if err != nil {
		log.Error().Err(err).Msg("Failed to connect to Redis")
		os.Exit(1)
	}
	defer rd.Close()

	rateStore := rs.NewStore(rd, rs.Options{Prefix: "req_limit:"})
	limiter := rate.NewRateLimit(rateStore)
	hndStore := store.NewStore(rd, "token:")
	authMdlw := auth.New(hndStore, limiter, verifier)
	hnd := handler.NewHandler(cfg.Application.TargetHost, authMdlw, rd)

	st := cfg.ServerTimeouts
	mainSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Application.Port),
		Handler:           handler.GetRouter(hnd, mtx),
		ReadHeaderTimeout: st.ReadHeaderTimeout,
		ReadTimeout:       st.ReadTimeout,
		WriteTimeout:      st.WriteTimeout,
		IdleTimeout:       st.IdleTimeout,
	}

	metricsSrv := newMetricsServer(cfg.Monitoring)

	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	startServer("main", mainSrv, errCh, &wg)
	if metricsSrv != nil {
		startServer("metrics", metricsSrv, errCh, &wg)
	} else {
		log.Info().Msg("Metrics server is disabled")
	}

	select {
	case <-ctx.Done():
		log.Info().Msg("Shutdown signal received")
	case err := <-errCh:
		log.Error().Err(err).Msg("Server exited unexpectedly")
		stop()
	}

	tctx, tcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer tcancel()

	shutdownServer(tctx, "main", mainSrv)
	if metricsSrv != nil {
		shutdownServer(tctx, "metrics", metricsSrv)
	}

	wg.Wait()
	log.Info().Msg("Tyk Proxy Service gracefully shutdown")
}

func newMetricsServer(cfg config.Monitoring) *http.Server {
	if cfg.Port == 0 {
		return nil
	}

	host := cfg.IP
	if host == "" {
		host = "0.0.0.0"
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)

	return &http.Server{
		Addr:              fmt.Sprintf("%s:%d", host, cfg.Port),
		Handler:           r,
		ReadHeaderTimeout: 3 * time.Second,
	}
}

func startServer(name string, srv *http.Server, errCh chan<- error, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Info().Str("server", name).Str("addr", srv.Addr).Msg("Server starting")

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case errCh <- fmt.Errorf("%s server: %w", name, err):
			default:
			}
		}
	}()
}

func shutdownServer(ctx context.Context, name string, srv *http.Server) {
	if srv == nil {
		return
	}

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Str("server", name).Msg("Server forced to shutdown")
	}
}

type startOptions struct {
	configPath   string
	envOverrides bool
	showVersion  bool
}

func preStart() startOptions {
	configPath := flag.String("config", "", "path to config file")
	showVersion := flag.Bool("version", false, "show version")
	envOverrides := flag.Bool("env", false, "override json config values by ENV vars")

	flag.Parse()

	opts := startOptions{}
	if configPath != nil {
		opts.configPath = *configPath
	}
	if envOverrides != nil {
		opts.envOverrides = *envOverrides
	}
	if showVersion != nil {
		opts.showVersion = *showVersion
	}

	return opts
}
