package logging

import (
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
)

// Middleware stashes a request-scoped child logger on both echo.Context
// (via c.Set) and the underlying context.Context (via WithLogger). Handlers
// read via From(c); code below reads via FromCtx(ctx).
//
// Emits one INFO line per request at completion with status + latency,
// or ERROR when the handler returned an error.
func Middleware(base *slog.Logger) echo.MiddlewareFunc {
	if base == nil {
		base = slog.Default()
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			child := base.With(
				slog.String("method", req.Method),
				slog.String("path", req.URL.Path),
			)
			c.Set(echoKey, child)
			c.SetRequest(req.WithContext(WithLogger(req.Context(), child)))

			started := time.Now()
			err := next(c)
			latencyMs := time.Since(started).Milliseconds()

			attrs := []any{
				slog.Int("status", c.Response().Status),
				slog.Int64("latency_ms", latencyMs),
				slog.Int64("bytes_out", c.Response().Size),
			}
			if err != nil {
				attrs = append(attrs, slog.String("error", err.Error()))
				child.LogAttrs(req.Context(), slog.LevelError, "request", toSlogAttrs(attrs)...)
			} else {
				child.LogAttrs(req.Context(), slog.LevelInfo, "request", toSlogAttrs(attrs)...)
			}
			return err
		}
	}
}

// toSlogAttrs bridges the []any-of-slog.Attr shape used above into the
// slog.Attr slice LogAttrs wants — keeps the call site readable.
func toSlogAttrs(items []any) []slog.Attr {
	out := make([]slog.Attr, 0, len(items))
	for _, item := range items {
		if attr, ok := item.(slog.Attr); ok {
			out = append(out, attr)
		}
	}
	return out
}
