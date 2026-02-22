package config

import "testing"

func TestValidateAndNormalize_AppliesTimeoutDefaults(t *testing.T) {
	cfg := &Config{
		Application: Application{
			TargetHost: "http://example.com",
			Port:       8080,
			Token: Token{
				JWTSecret: "secret",
				Algorithm: "HS256",
			},
		},
		Redis: Redis{Addr: "localhost:6379"},
	}

	if err := cfg.ValidateAndNormalize(); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}

	if cfg.ServerTimeouts.ReadHeaderTimeout <= 0 {
		t.Fatalf("ReadHeaderTimeout should be defaulted")
	}
	if cfg.ServerTimeouts.ReadTimeout <= 0 {
		t.Fatalf("ReadTimeout should be defaulted")
	}
	if cfg.ServerTimeouts.WriteTimeout <= 0 {
		t.Fatalf("WriteTimeout should be defaulted")
	}
	if cfg.ServerTimeouts.IdleTimeout <= 0 {
		t.Fatalf("IdleTimeout should be defaulted")
	}
}

func TestValidateAndNormalize_InvalidPort(t *testing.T) {
	cfg := &Config{
		Application: Application{
			TargetHost: "http://example.com",
			Port:       0,
			Token: Token{
				JWTSecret: "secret",
				Algorithm: "HS256",
			},
		},
		Redis: Redis{Addr: "localhost:6379"},
	}

	if err := cfg.ValidateAndNormalize(); err == nil {
		t.Fatalf("expected error for invalid port")
	}
}

func TestValidateAndNormalize_UnsupportedAlgorithm(t *testing.T) {
	cfg := &Config{
		Application: Application{
			TargetHost: "http://example.com",
			Port:       8080,
			Token: Token{
				JWTSecret: "secret",
				Algorithm: "RS256",
			},
		},
		Redis: Redis{Addr: "localhost:6379"},
	}

	if err := cfg.ValidateAndNormalize(); err == nil {
		t.Fatalf("expected error for unsupported algorithm")
	}
}
