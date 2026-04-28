package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/openguard/control-plane/pkg/middleware"
	"github.com/sony/gobreaker"
)

type CircuitBreakerTransport struct {
	cb *gobreaker.CircuitBreaker
	rt http.RoundTripper
}

func (t *CircuitBreakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.cb.Execute(func() (interface{}, error) {
		return t.rt.RoundTrip(req)
	})
	if err != nil {
		return nil, err
	}
	return resp.(*http.Response), nil
}

func NewProxy(targetURL string, cb *gobreaker.CircuitBreaker) http.HandlerFunc {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		panic(fmt.Sprintf("invalid proxy target URL %q: %v", targetURL, err))
	}
	if parsedURL.Host == "" {
		panic(fmt.Sprintf("proxy target URL %q has empty host", targetURL))
	}
	proxy := httputil.NewSingleHostReverseProxy(parsedURL)

	// Default transport wrapped with circuit breaker
	if cb != nil {
		proxy.Transport = &CircuitBreakerTransport{
			cb: cb,
			rt: http.DefaultTransport,
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		log := middleware.GetLogger(r.Context())
		log.Info("Proxying request", "path", r.URL.Path, "target", targetURL)
		proxy.ServeHTTP(w, r)
	}
}
