// Package logger constructs the application's *zap.Logger.
//
// Why Zap? It's a structured, leveled logger (fields, not string formatting),
// comparable to Serilog in .NET or Logback+structured-args in Java. Structured
// logging means you attach typed key/value fields (zap.String, zap.Int,
// zap.Duration) instead of interpolating into a message — so logs stay
// machine-queryable.
//
// We expose two flavors:
//   - development: human-readable, colored console output.
//   - production:  JSON, one object per line, ready for log aggregation.
package logger

import (
	"fmt"

	"go.uber.org/zap"
)

// New builds a *zap.Logger appropriate for the given environment ("development"
// or anything else => production JSON). The caller owns the logger's lifecycle
// and should `defer logger.Sync()` to flush buffered entries on shutdown.
func New(env string) (*zap.Logger, error) {
	var (
		l   *zap.Logger
		err error
	)

	switch env {
	case "development":
		// Console encoder, DebugLevel, caller info — friendly for local dev.
		l, err = zap.NewDevelopment()
	default:
		// JSON encoder, InfoLevel, sampling — suited to production ingestion.
		l, err = zap.NewProduction()
	}
	if err != nil {
		// %w wraps the underlying error so callers can errors.Is/As it later.
		return nil, fmt.Errorf("logger: build %q logger: %w", env, err)
	}

	return l, nil
}
