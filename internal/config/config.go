package config

import (
	"log/slog"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/pkg/errors"
)

type Config struct {
	Application    Application    `json:"application"`
	ServerTimeouts ServerTimeouts `json:"server_timeouts"`
	Redis          Redis          `json:"redis"`
	Log            Log            `json:"log"`
	Monitoring     Monitoring     `json:"monitoring"`
}

type Application struct {
	TargetHost string `json:"target_host"`
	Port       int    `json:"port"`
	Token      Token  `json:"token"`
}

type Token struct {
	JWTSecret string `json:"jwt_secret"` // II+NZDtODCTp0eAGX0/3HNdaExOf+M1uesFHdN+IFcTD774aaeJrJIOMS4aYhi+l
	Algorithm string `json:"algorithm"`  // HS256
}
type ServerTimeouts struct {
	ReadHeaderTimeout time.Duration `json:"readHeaderTimeout,omitempty"`
	ReadTimeout       time.Duration `json:"readTimeout,omitempty"`
	WriteTimeout      time.Duration `json:"writeTimeout,omitempty"`
	IdleTimeout       time.Duration `json:"idleTimeout,omitempty"`
}

type Log struct {
	Level   string `json:"level"`
	Format  string `json:"format"`
	Colored bool   `json:"colored"`
}

type Monitoring struct {
	IP     string `json:"ip"`
	Scheme string `json:"scheme"`
	Port   int    `json:"port"`
}

type Redis struct {
	Addr string `json:"addr"`
}

const servicePrefix = "TYK_PROX_"

// ReadConfig loads config from JSON file and environment variables.
// envOverrides controls priority:
//   - true  => JSON loaded first, then ENV (ENV has higher priority)
//   - false => ENV loaded first, then JSON (JSON has higher priority)
func ReadConfig(configPath string, envOverrides *bool) (*Config, error) {
	k := koanf.New(".")

	slog.Info("Starting service. Reading config", "config_path", configPath, "env_overrides", *envOverrides)

	if configPath == "" {
		slog.Info("No config path provided, loading from env only")
		if err := loadEnv(k); err != nil {
			return nil, errors.Wrap(err, "fatal error loading config from env")
		}
	} else {
		if *envOverrides {
			slog.Info("Override with env variables")
			// JSON (low prior) -> ENV (high prior)
			if err := loadFile(k, configPath); err != nil {
				return nil, err
			}
			if err := loadEnv(k); err != nil {
				return nil, errors.Wrap(err, "fatal error loading config from env")
			}
		} else {
			slog.Info("json config has priority over env variables")
			// ENV (low prior) -> JSON (high prior)
			if err := loadEnv(k); err != nil {
				return nil, errors.Wrap(err, "fatal error loading config from env")
			}
			if err := loadFile(k, configPath); err != nil {
				return nil, err
			}
		}
	}

	slog.Info("Unmarshaling config fields into struct")
	var cfg Config
	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{
		Tag:       "json",
		FlatPaths: false,
	}); err != nil {
		return nil, errors.Wrap(err, "failure to unmarshal config fields into struct")
	}

	slog.Info("Config loaded successfully")
	return &cfg, nil
}

func loadEnv(k *koanf.Koanf) error {
	return k.Load(env.Provider(servicePrefix, ".", func(s string) string {
		// has to cut it by itself, but didn't work
		s = strings.TrimPrefix(s, servicePrefix)
		
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, "__", ".")
		return s
	}), nil)
}

func loadFile(k *koanf.Koanf, cfgPath string) error {
	if cfgPath == "" {
		slog.Warn("No config path")
		return nil
	}

	if err := k.Load(file.Provider(cfgPath), json.Parser()); err != nil {
		return errors.Wrap(err, "fatal error loading config from file")
	}

	return nil
}
