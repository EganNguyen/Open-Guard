package telemetry

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		routeContext := chi.RouteContext(r.Context())
		path := r.URL.Path
		if routeContext != nil && routeContext.RoutePattern() != "" {
			path = routeContext.RoutePattern()
		}

		duration := time.Since(start).Seconds()
		RequestCounter.WithLabelValues(path, r.Method).Inc()
		RequestLatency.WithLabelValues(path, r.Method).Observe(duration)
	})
}
