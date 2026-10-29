package config

import (
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Config
	}{
		{
			name: "production defaults",
			env:  map[string]string{"DATABASE_URL": "postgres://db", "CORS_ALLOWED_ORIGINS": "https://app.example"},
			want: Config{
				DatabaseURL:          "postgres://db",
				HTTPAddr:             ":8080",
				AppEnv:               "production",
				CORSAllowedOrigins:   []string{"https://app.example"},
				GraphQLMaxDepth:      10,
				GraphQLMaxComplexity: 200,
				ShutdownTimeout:      15 * time.Second,
				LogLevel:             slog.LevelInfo,
			},
		},
		{
			name: "development defaults to wildcard origin",
			env:  map[string]string{"DATABASE_URL": "postgres://db", "APP_ENV": "development"},
			want: Config{
				DatabaseURL:          "postgres://db",
				HTTPAddr:             ":8080",
				AppEnv:               "development",
				CORSAllowedOrigins:   []string{"*"},
				GraphQLMaxDepth:      10,
				GraphQLMaxComplexity: 200,
				ShutdownTimeout:      15 * time.Second,
				LogLevel:             slog.LevelInfo,
			},
		},
		{
			name: "all values set",
			env: map[string]string{
				"DATABASE_URL":           "postgres://db",
				"HTTP_ADDR":              "127.0.0.1:9000",
				"APP_ENV":                "development",
				"CORS_ALLOWED_ORIGINS":   " http://a.test , ,http://b.test ",
				"GRAPHQL_MAX_DEPTH":      "5",
				"GRAPHQL_MAX_COMPLEXITY": "50",
				"SHUTDOWN_TIMEOUT":       "3s",
				"LOG_LEVEL":              "debug",
			},
			want: Config{
				DatabaseURL:          "postgres://db",
				HTTPAddr:             "127.0.0.1:9000",
				AppEnv:               "development",
				CORSAllowedOrigins:   []string{"http://a.test", "http://b.test"},
				GraphQLMaxDepth:      5,
				GraphQLMaxComplexity: 50,
				ShutdownTimeout:      3 * time.Second,
				LogLevel:             slog.LevelDebug,
			},
		},
		{
			name: "wildcard accepted in development",
			env:  map[string]string{"DATABASE_URL": "postgres://db", "APP_ENV": "development", "CORS_ALLOWED_ORIGINS": "*"},
			want: Config{
				DatabaseURL:          "postgres://db",
				HTTPAddr:             ":8080",
				AppEnv:               "development",
				CORSAllowedOrigins:   []string{"*"},
				GraphQLMaxDepth:      10,
				GraphQLMaxComplexity: 200,
				ShutdownTimeout:      15 * time.Second,
				LogLevel:             slog.LevelInfo,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Load(env(tt.env))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLoadLogLevels(t *testing.T) {
	tests := []struct {
		raw  string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := Load(env(map[string]string{"DATABASE_URL": "x", "APP_ENV": "development", "LOG_LEVEL": tt.raw}))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got.LogLevel != tt.want {
				t.Errorf("LogLevel = %v, want %v", got.LogLevel, tt.want)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	valid := map[string]string{"DATABASE_URL": "postgres://db", "CORS_ALLOWED_ORIGINS": "https://app.example"}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for key, val := range valid {
			m[key] = val
		}
		m[k] = v
		return m
	}
	tests := []struct {
		name     string
		env      map[string]string
		wantMsgs []string
	}{
		{"missing DATABASE_URL", with("DATABASE_URL", ""), []string{"DATABASE_URL is required"}},
		{"invalid APP_ENV", with("APP_ENV", "staging"), []string{"APP_ENV must be"}},
		{"missing CORS in production", with("CORS_ALLOWED_ORIGINS", ""), []string{"CORS_ALLOWED_ORIGINS is required in production"}},
		{"blank CORS entries in production", with("CORS_ALLOWED_ORIGINS", " , "), []string{"CORS_ALLOWED_ORIGINS is required in production"}},
		{"wildcard CORS in production", with("CORS_ALLOWED_ORIGINS", "https://a.example,*"), []string{"must not contain * in production"}},
		{"non-numeric depth", with("GRAPHQL_MAX_DEPTH", "ten"), []string{"GRAPHQL_MAX_DEPTH must be a positive integer"}},
		{"zero depth", with("GRAPHQL_MAX_DEPTH", "0"), []string{"GRAPHQL_MAX_DEPTH must be a positive integer"}},
		{"negative complexity", with("GRAPHQL_MAX_COMPLEXITY", "-1"), []string{"GRAPHQL_MAX_COMPLEXITY must be a positive integer"}},
		{"invalid timeout", with("SHUTDOWN_TIMEOUT", "soon"), []string{"SHUTDOWN_TIMEOUT must be a positive duration"}},
		{"zero timeout", with("SHUTDOWN_TIMEOUT", "0s"), []string{"SHUTDOWN_TIMEOUT must be a positive duration"}},
		{"negative timeout", with("SHUTDOWN_TIMEOUT", "-5s"), []string{"SHUTDOWN_TIMEOUT must be a positive duration"}},
		{"invalid log level", with("LOG_LEVEL", "verbose"), []string{"LOG_LEVEL must be one of"}},
		{
			name: "multiple errors reported together",
			env: map[string]string{
				"GRAPHQL_MAX_DEPTH": "x",
				"LOG_LEVEL":         "loud",
			},
			wantMsgs: []string{
				"DATABASE_URL is required",
				"CORS_ALLOWED_ORIGINS is required in production",
				"GRAPHQL_MAX_DEPTH must be a positive integer",
				"LOG_LEVEL must be one of",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(env(tt.env))
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}
			for _, msg := range tt.wantMsgs {
				if !strings.Contains(err.Error(), msg) {
					t.Errorf("error %q does not contain %q", err, msg)
				}
			}
		})
	}
}
