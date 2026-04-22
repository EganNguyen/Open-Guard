package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"
	"github.com/openguard/control-plane/pkg/middleware"
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
	parsedURL, _ := url.Parse(targetURL)
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
		log.Info("Proxying request", zap.String("path", r.URL.Path), zap.String("target", targetURL))
		proxy.ServeHTTP(w, r)
	}
}
