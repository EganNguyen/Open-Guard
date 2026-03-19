package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// NewReverseProxy creates a reverse proxy to the given upstream URL.
// It preserves the original request path relative to the strip prefix.
func NewReverseProxy(target string, logger *slog.Logger) (*httputil.ReverseProxy, error) {
	upstream, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(upstream)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = upstream.Host
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("proxy error",
			"upstream", target,
			"path", r.URL.Path,
			"error", err,
		)
		http.Error(w, `{"error":{"code":"SERVICE_UNAVAILABLE","message":"Upstream service is unavailable"}}`,
			http.StatusBadGateway)
	}

	return proxy, nil
}
