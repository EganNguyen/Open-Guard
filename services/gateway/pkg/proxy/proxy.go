package proxy

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/sony/gobreaker"
	"github.com/openguard/shared/resilience"
)

type CircuitBreakerTransport struct {
	breaker   *gobreaker.CircuitBreaker
	transport http.RoundTripper
	timeout   time.Duration
}

func (c *CircuitBreakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return resilience.Call(req.Context(), c.breaker, c.timeout, func(ctx context.Context) (*http.Response, error) {
		return c.transport.RoundTrip(req.WithContext(ctx))
	})
}

// NewReverseProxy creates a resilient reverse proxy with Circuit Breaker and mTLS.
func NewReverseProxy(target string, logger *slog.Logger, cb *gobreaker.CircuitBreaker, tlsCfg *tls.Config) (*httputil.ReverseProxy, error) {
	upstream, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(upstream)

	// Wrap standard transport in a resilient circuit breaker
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	if tlsCfg != nil {
		baseTransport.TLSClientConfig = tlsCfg
	}
	proxy.Transport = &CircuitBreakerTransport{
		breaker:   cb,
		transport: baseTransport,
		timeout:   10 * time.Second, // Timeout per backend request
	}

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
		
		code := "SERVICE_UNAVAILABLE"
		status := http.StatusServiceUnavailable
		if err.Error() == resilience.ErrCircuitOpen.Error() {
			code = "CIRCUIT_OPEN"
		}
		
		http.Error(w, `{"error":{"code":"`+code+`","message":"Upstream service is currently unavailable"}}`, status)
	}

	return proxy, nil
}
