package config

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorYellow = "\033[33m"
)

var FormatLevel = func(i interface{}) string {
	level, ok := i.(string)
	if !ok {
		return "[unknown]   "
	}

	label := fmt.Sprintf("[%s]", strings.ToUpper(level))

	padded := fmt.Sprintf("%-7s", label)

	return padded
}

var FormatLevelColored = func(i interface{}) string {
	level, ok := i.(string)
	if !ok {
		return "[unknown]   "
	}

	var color string
	switch level {
	case zerolog.LevelWarnValue:
		color = ColorYellow
	case zerolog.LevelErrorValue, zerolog.LevelFatalValue, zerolog.LevelPanicValue:
		color = ColorRed
	default:
		color = ""
	}

	// label: [level]
	label := fmt.Sprintf("[%s]", strings.ToUpper(level))

	// pad right to fixed width (e.g. 7 chars total)
	padded := fmt.Sprintf("%-7s", label)

	if color != "" {
		return fmt.Sprintf("%s%s%s", color, padded, ColorReset)
	}
	return padded
}

func InitLogger(cfg *Config) {
	slog.Info("Initializing logger", "level", cfg.Log.Level, "format", cfg.Log.Format, "colored", cfg.Log.Colored)

	zerolog.TimeFieldFormat = time.RFC3339Nano
	zerolog.TimestampFieldName = "time"
	zerolog.LevelFieldName = "level"

	var output io.Writer
	formatter := FormatLevel
	if cfg.Log.Colored {
		formatter = FormatLevelColored
	}

	if cfg.Log.Format == "json" {
		// Structured JSON output
		output = os.Stdout

	} else {
		// Human-friendly console output
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339Nano,
			PartsOrder: []string{
				zerolog.TimestampFieldName,
				zerolog.LevelFieldName,
				zerolog.MessageFieldName,
			},
			FormatLevel: formatter,
			FormatMessage: func(i interface{}) string {
				return fmt.Sprint(i)
			},
		}
	}

	globalLogger := zerolog.New(output).With().Timestamp().Logger()

	l, err := zerolog.ParseLevel(cfg.Log.Level)
	if err == nil {
		zerolog.SetGlobalLevel(l)
	}

	log.Logger = globalLogger
}

type ChiZerologFormatter struct{}

func (f *ChiZerologFormatter) NewLogEntry(r *http.Request) middleware.LogEntry {
	l := log.With().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Str("remote", r.RemoteAddr).
		Str("userAgent", r.UserAgent()).
		Logger()

	l.Info().Msg("request started")

	return &ChiZerologEntry{logger: l}
}

type ChiZerologEntry struct {
	logger zerolog.Logger
}

func (e *ChiZerologEntry) Write(status, bytes int, _ http.Header, elapsed time.Duration, _ interface{}) {
	e.logger.Info().
		Int("status", status).
		Int("bytes", bytes).
		Dur("duration", elapsed).
		Msg("request completed")
}

func (e *ChiZerologEntry) Panic(v interface{}, stack []byte) {
	e.logger.Error().
		Interface("panic", v).
		Bytes("stack", stack).
		Msg("panic occurred")
}
