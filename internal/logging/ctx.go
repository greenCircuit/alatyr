package logging

import (
	"context"
	"log/slog"

	"github.com/labstack/echo/v4"
)

// ctxKey type-safe key for stashing *slog.Logger on context.Context.
type ctxKey struct{}

// echoKey the string key used on echo.Context. Kept in one place so
// middleware and From() cannot drift.
const echoKey = "log"

// From pulls the request-scoped logger stashed by Middleware. Falls back
// to slog.Default() if nothing is on the context — safer than a panic
// when a handler runs outside the middleware chain (tests, background).
func From(c echo.Context) *slog.Logger {
	if c == nil {
		return slog.Default()
	}
	if logger, ok := c.Get(echoKey).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}

// FromCtx same as From, but takes a plain context.Context. Used by code
// paths below the handler layer that only get ctx (store.PolicyIssues,
// store.IsNodesReachable, mesh sources).
func FromCtx(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return slog.Default()
	}
	if logger, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}

// WithLogger stashes logger on ctx. Used by middleware and by any caller
// that wants downstream code to see a logger via FromCtx.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}
