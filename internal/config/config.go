// Package config loads and validates the service configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

const (
	envDevelopment = "development"
	envProduction  = "production"
)

// Config is the validated service configuration.
type Config struct {
	DatabaseURL          string
	HTTPAddr             string
	AppEnv               string
	CORSAllowedOrigins   []string
	GraphQLMaxDepth      int
	GraphQLMaxComplexity int
	ShutdownTimeout      time.Duration
	LogLevel             slog.Level
}

// Development reports whether the service runs in the development environment.
func (c Config) Development() bool {
	return c.AppEnv == envDevelopment
}

// Load reads the configuration through getenv and validates it. Every problem found is
// reported in the returned error, not only the first one.
func Load(getenv func(string) string) (Config, error) {
	var errs []error
	cfg := Config{
		DatabaseURL: strings.TrimSpace(getenv("DATABASE_URL")),
		HTTPAddr:    valueOr(getenv("HTTP_ADDR"), ":8080"),
		AppEnv:      valueOr(getenv("APP_ENV"), envProduction),
	}

	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}

	if cfg.AppEnv != envDevelopment && cfg.AppEnv != envProduction {
		errs = append(errs, fmt.Errorf("APP_ENV must be %q or %q, got %q", envDevelopment, envProduction, cfg.AppEnv))
	}

	origins, err := parseOrigins(getenv("CORS_ALLOWED_ORIGINS"), cfg.AppEnv == envDevelopment)
	if err != nil {
		errs = append(errs, err)
	}
	cfg.CORSAllowedOrigins = origins

	if cfg.GraphQLMaxDepth, err = positiveInt(getenv, "GRAPHQL_MAX_DEPTH", 10); err != nil {
		errs = append(errs, err)
	}
	if cfg.GraphQLMaxComplexity, err = positiveInt(getenv, "GRAPHQL_MAX_COMPLEXITY", 200); err != nil {
		errs = append(errs, err)
	}

	if cfg.ShutdownTimeout, err = parseTimeout(getenv("SHUTDOWN_TIMEOUT")); err != nil {
		errs = append(errs, err)
	}

	if cfg.LogLevel, err = parseLevel(getenv("LOG_LEVEL")); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return cfg, nil
}

func valueOr(v, def string) string {
	if v = strings.TrimSpace(v); v == "" {
		return def
	}
	return v
}

func parseOrigins(raw string, development bool) ([]string, error) {
	var origins []string
	for o := range strings.SplitSeq(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	if len(origins) == 0 {
		if development {
			return []string{"*"}, nil
		}
		return nil, errors.New("CORS_ALLOWED_ORIGINS is required in production")
	}
	if !development {
		for _, o := range origins {
			if o == "*" {
				return nil, errors.New("CORS_ALLOWED_ORIGINS must not contain * in production")
			}
		}
	}
	return origins, nil
}

func positiveInt(getenv func(string) string, name string, def int) (int, error) {
	raw := strings.TrimSpace(getenv(name))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer, got %q", name, raw)
	}
	return n, nil
}

func parseTimeout(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 15 * time.Second, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("SHUTDOWN_TIMEOUT must be a positive duration such as 15s, got %q", raw)
	}
	return d, nil
}

func parseLevel(raw string) (slog.Level, error) {
	switch strings.TrimSpace(raw) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error, got %q", raw)
	}
}
