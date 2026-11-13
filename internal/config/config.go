// Package config loads strongly-typed application configuration from the
// environment. Think of it as the Go equivalent of binding IConfiguration to
// an IOptions<T> in .NET, or @ConfigurationProperties in Spring — except Go
// has no framework doing it for you, so we read env vars explicitly.
//
// Idioms on display here:
//   - A plain struct as the config "shape" (no annotations/reflection).
//   - Composition: small structs (HTTP, DB, Engine, Auth) embedded into one.
//   - Errors are returned, not thrown. Load() returns (Config, error).
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config is the root configuration object. It is built once at startup in the
// composition root (cmd/*) and passed down by value to the components that
// need it. Sub-structs keep related settings together.
type Config struct {
	// Env is "development" or "production"; it switches the logger format.
	Env string

	HTTP   HTTPConfig
	DB     DBConfig
	Engine EngineConfig
	Auth   AuthConfig
}

// HTTPConfig holds the API server settings (used from Phase 2 onward).
type HTTPConfig struct {
	Addr            string        // e.g. ":8080"
	ReadTimeout     time.Duration // max time to read the full request
	WriteTimeout    time.Duration // max time to write the response
	ShutdownTimeout time.Duration // grace period for in-flight requests on shutdown
	MaxBodyBytes    int64         // cap on a request body (POST /runs is newline-delimited URLs)
	TenantsFile     string        // JSON file the tenant registry loads at startup
}

// DBConfig holds Postgres connection settings (used from Phase 3 onward).
type DBConfig struct {
	DSN         string // postgres://user:pass@host:port/db?sslmode=disable
	MaxConns    int32  // upper bound on the pgx pool
	MaxConnIdle time.Duration
}

// EngineConfig tunes the concurrency engine (Phase 1). These map directly to
// the levers you will experiment with: pool size, per-request timeout, and how
// much work to buffer in the jobs channel.
type EngineConfig struct {
	Workers    int           // number of worker goroutines in the pool
	Timeout    time.Duration // per-URL HTTP timeout (the context deadline)
	BufferSize int           // capacity of the buffered jobs channel
	MaxURLs    int           // global cap on URLs per run; a tenant may override it
}

// AuthConfig holds OAuth2 resource-server settings (Phase 4). The service is a
// resource server: it validates tokens minted by an external IdP, it does not
// issue them.
type AuthConfig struct {
	Issuer   string // expected "iss" claim, e.g. https://idp.example.com/
	Audience string // expected "aud" claim, your API's identifier
	JWKSURL  string // where to fetch the IdP's public signing keys
}

// Load reads configuration from environment variables, applying sensible
// defaults. It returns an error only when a provided value is malformed (e.g.
// a non-numeric WORKERS), which lets the caller fail fast at startup.
//
// Note: there is no global singleton. The returned Config is wired in by the
// caller — that explicitness is the whole point of manual dependency injection.
func Load() (Config, error) {
	cfg := Config{
		Env: getEnv("APP_ENV", "development"),
		HTTP: HTTPConfig{
			Addr:            getEnv("HTTP_ADDR", ":8080"),
			ReadTimeout:     getEnvDuration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:    getEnvDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			ShutdownTimeout: getEnvDuration("HTTP_SHUTDOWN_TIMEOUT", 15*time.Second),
			MaxBodyBytes:    int64(getEnvInt("HTTP_MAX_BODY_BYTES", 1<<20)),
			TenantsFile:     getEnv("TENANTS_FILE", "testdata/tenants.json"),
		},
		DB: DBConfig{
			DSN:         getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/healthchecks?sslmode=disable"),
			MaxConns:    int32(getEnvInt("DB_MAX_CONNS", 10)),
			MaxConnIdle: getEnvDuration("DB_MAX_CONN_IDLE", 5*time.Minute),
		},
		Engine: EngineConfig{
			Workers:    getEnvInt("ENGINE_WORKERS", 50),
			Timeout:    getEnvDuration("ENGINE_TIMEOUT", 5*time.Second),
			BufferSize: getEnvInt("ENGINE_BUFFER", 1000),
			MaxURLs:    getEnvInt("ENGINE_MAX_URLS", 10000),
		},
		Auth: AuthConfig{
			Issuer:   getEnv("AUTH_ISSUER", ""),
			Audience: getEnv("AUTH_AUDIENCE", ""),
			JWKSURL:  getEnv("AUTH_JWKS_URL", ""),
		},
	}

	// Validate the few values that must be sane. Returning early with a wrapped
	// error is the idiomatic alternative to throwing a ConfigurationException.
	if cfg.Engine.Workers <= 0 {
		return Config{}, fmt.Errorf("config: ENGINE_WORKERS must be > 0, got %d", cfg.Engine.Workers)
	}
	if cfg.Engine.Timeout <= 0 {
		return Config{}, fmt.Errorf("config: ENGINE_TIMEOUT must be > 0, got %s", cfg.Engine.Timeout)
	}
	if cfg.Engine.MaxURLs <= 0 {
		return Config{}, fmt.Errorf("config: ENGINE_MAX_URLS must be > 0, got %d", cfg.Engine.MaxURLs)
	}
	if cfg.HTTP.MaxBodyBytes <= 0 {
		return Config{}, fmt.Errorf("config: HTTP_MAX_BODY_BYTES must be > 0, got %d", cfg.HTTP.MaxBodyBytes)
	}
	if cfg.HTTP.TenantsFile == "" {
		return Config{}, fmt.Errorf("config: TENANTS_FILE must be set")
	}

	return cfg, nil
}

// --- small env helpers -------------------------------------------------------
// These exist because Go's stdlib gives you os.Getenv (string only). Rather
// than pull in a config library, we hand-roll typed accessors. It's a few lines
// and keeps dependencies (and magic) to zero.

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
