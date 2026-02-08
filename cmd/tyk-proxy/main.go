package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/pkg/errors"
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

var (
	envOverrides *bool
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cfgPath := preStart()
	if cfgPath == "version" {
		return
	}

	cfg, err := config.ReadConfig(cfgPath, envOverrides)
	if err != nil || cfg.Application.Port == 0 {
		slog.Error("Failed to load configuration", "error", err)
		return
	}

	config.InitLogger(cfg)
	log.Info().Str("level", cfg.Log.Level).Msg("Logger initialized")

	mtx := metrics.GetMetrics()
	go runMetrics(ctx, cfg.Monitoring)

	verifier := auth.NewJWTVerifier(auth.KeySet{
		ExpectedAlg: cfg.Application.Token.Algorithm,
		DefaultKey:  []byte(cfg.Application.Token.JWTSecret),
	})

	rd, err := redis.NewRedis(ctx, cfg.Redis.Addr)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis")
	}

	defer rd.Close()

	rateStore := rs.NewStore(rd, rs.Options{
		Prefix: "req_limit:",
	})

	limiter := rate.NewRateLimit(rateStore)
	hndStore := store.NewStore(rd, "token:")
	authMdlw := auth.New(hndStore, limiter, verifier)

	hnd := handler.NewHandler(cfg.Application.TargetHost, authMdlw, rd)

	st := cfg.ServerTimeouts
	srv := http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Application.Port),
		Handler: handler.GetRouter(hnd, mtx),

		ReadHeaderTimeout: st.ReadHeaderTimeout,
		ReadTimeout:       st.ReadTimeout,
		WriteTimeout:      st.WriteTimeout,
		IdleTimeout:       st.IdleTimeout,
	}

	go func() {
		log.Info().Int("port", cfg.Application.Port).Msg("Server starting")

		err := srv.ListenAndServe()
		if err != nil {
			log.Error().Err(err).Msg("Service closed with error")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("Context done signal received")

	tctx, tcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer tcancel()

	if err := srv.Shutdown(tctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	log.Info().Msg("Tyk Proxy Service gracefully shutdown")
}

func runMetrics(ctx context.Context, cfg config.Monitoring) {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)

	server := &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", cfg.Port),
		Handler:           r,
		ReadHeaderTimeout: time.Second * 3,
	}

	log.Info().Msgf("Listening metrics on %s", server.Addr)

	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msgf("Error when shutting down server %v", err)
		}
	}()

	log.Info().Msg("Metric server initialized")
	<-ctx.Done()
}

func preStart() string {
	configPath := flag.String("config", "", "path to config file")
	showVersion := flag.Bool("version", false, "show version")

	envOverrides = flag.Bool("env", false, "override json config values by ENV vars")

	flag.Parse()

	if showVersion != nil && *showVersion {
		fmt.Printf("version: %s", version.GetVersion())
		return "version"
	}

	if configPath == nil {
		temp := ""
		configPath = &temp
	}

	return *configPath
}
