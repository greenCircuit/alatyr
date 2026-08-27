package metrics

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// Middleware records alatyr_http_requests_total + alatyr_http_request_duration_seconds
// per Echo request. Uses c.Path() (the route pattern) as the path label —
// never c.Request().URL, which would embed IDs and query params and blow up
// cardinality on the first paginated call.
func (r *Recorder) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			started := time.Now()
			err := next(c)
			route := c.Path()
			if route == "" {
				// No route matched (404 on unregistered path). Bucket all of them
				// under one label so a scanner probing random URLs can't create
				// unbounded series.
				route = "unmatched"
			}
			code := strconv.Itoa(c.Response().Status)
			r.httpRequests.WithLabelValues(route, code).Inc()
			r.httpRequestDur.WithLabelValues(route).Observe(time.Since(started).Seconds())
			return err
		}
	}
}
