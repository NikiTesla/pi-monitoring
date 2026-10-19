package httpcheck

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/NikiTesla/pi-monitoring/internal/config"
)

// Reuse connections between checks. There is no cookie jar or shared auth state.
var transport = &http.Transport{
	Proxy:        http.ProxyFromEnvironment,
	DialContext:  (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	MaxIdleConns: 32, MaxIdleConnsPerHost: 2, IdleConnTimeout: 90 * time.Second,
	TLSHandshakeTimeout:    5 * time.Second,
	MaxResponseHeaderBytes: 64 << 10,
}

func newHTTPClient(check config.HTTPCheck) http.Client {
	crossedOrigin := false
	return http.Client{Transport: transport, CheckRedirect: func(next *http.Request, via []*http.Request) error {
		if !check.FollowRedirects {
			return http.ErrUseLastResponse
		}
		if len(via) >= 10 {
			return errors.New("redirect limit reached")
		}
		if next.URL.Scheme != "http" && next.URL.Scheme != "https" {
			return errors.New("unsupported redirect")
		}
		if next.URL.User != nil {
			return errors.New("redirect credentials are unsupported")
		}
		// The default client only strips a few sensitive headers. Strip every
		// configured header across origins, including application-specific secrets.
		crossedOrigin = crossedOrigin || !sameOrigin(next, via[len(via)-1])
		if crossedOrigin {
			// Go generates Referer from the previous URL, including its query.
			// Query parameters can contain secrets even without configured headers.
			next.Header.Del("Referer")
			for name := range check.Headers {
				next.Header.Del(name)
			}
		}
		return nil
	}}
}

func sameOrigin(first, second *http.Request) bool {
	effectivePort := func(request *http.Request) string {
		explicitPort := request.URL.Port()
		if explicitPort == "" {
			if request.URL.Scheme == "https" {
				return "443"
			}
			return "80"
		}
		return explicitPort
	}
	return strings.EqualFold(first.URL.Scheme, second.URL.Scheme) && strings.EqualFold(first.URL.Hostname(), second.URL.Hostname()) && effectivePort(first) == effectivePort(second)
}
