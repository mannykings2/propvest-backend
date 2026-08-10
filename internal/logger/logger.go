// Package logger provides the application's structured, JSON logging.
//
// WHY THIS PACKAGE EXISTS
// -----------------------
// The engineering docs (06-Engineering/6.1 and 6.2) mandate STRUCTURED JSON logs
// with fields like request_id, user_id, method, path, status, duration_ms, and
// error. The original code used the standard library `log.Printf`, which only
// produces free-form text lines that are hard to search, filter, and aggregate.
//
// We use Go's built-in `log/slog` (standard library, no third-party dependency).
// slog emits key/value structured logs and has a ready-made JSON handler. In
// development we switch to a human-readable text handler so local output stays
// easy to read.
//
// HOW TO USE IT
// -------------
//	logger.Init("development")                       // once, at startup
//	logger.Info("user registered", "user_id", id)     // anywhere
//	logger.Error("db failed", "error", err)
//
// The package keeps a single process-wide *slog.Logger. That is the simplest
// design that satisfies the docs; services call the package-level helpers
// instead of threading a logger through every constructor. If we later want
// per-request loggers (with request_id pre-bound), FromContext/WithContext give
// us that without changing call sites.
package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// log is the process-wide structured logger. It is set by Init and defaults to
// a JSON logger writing to stdout so that even code paths that run before Init
// (or in tests) never nil-panic.
var log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

// ctxKey is a private type for context keys so we never collide with other
// packages that also store values in a context.
type ctxKey struct{}

var loggerKey = ctxKey{}

// Init configures the global logger for the given environment.
//
//   - "production": JSON handler at INFO level (machine-readable, aggregatable).
//   - anything else (development/test): text handler at DEBUG level (readable).
//
// Call this once from main() as early as possible, right after loading config.
func Init(appEnv string) {
	level := slog.LevelDebug
	if strings.EqualFold(appEnv, "production") {
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if strings.EqualFold(appEnv, "production") {
		// Production: structured JSON so log aggregators (Loki/ELK) can parse it.
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		// Development: text is friendlier for a human reading a terminal.
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	log = slog.New(handler)
	// Also route the standard library's global slog default to the same handler
	// so any stray slog.Info calls elsewhere are consistent.
	slog.SetDefault(log)
}

// L returns the global logger. Prefer the package-level helpers (Info/Error/…)
// for call sites; use L() when you need the *slog.Logger itself (e.g. to pass
// to another library).
func L() *slog.Logger {
	return log
}

// WithContext stores a (usually request-scoped) logger in the context so
// downstream code can retrieve it with FromContext. The HTTP logging middleware
// uses this to attach a logger that already carries the request_id.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// FromContext returns the logger stored in ctx, or the global logger if none is
// present. This never returns nil.
func FromContext(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok && l != nil {
			return l
		}
	}
	return log
}

// The following are thin convenience wrappers so callers can write
// logger.Info(...) instead of logger.L().Info(...). They keep call sites short
// and make it trivial to swap the backing implementation later.

func Debug(msg string, args ...any) { log.Debug(msg, args...) }
func Info(msg string, args ...any)  { log.Info(msg, args...) }
func Warn(msg string, args ...any)  { log.Warn(msg, args...) }
func Error(msg string, args ...any) { log.Error(msg, args...) }
