package observability

import (
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
)

// TrustedProxyMiddleware replaces chi's RealIP with trusted-proxy aware extraction.
func TrustedProxyMiddleware(trusted []string) func(http.Handler) http.Handler {
	trustedNets := make([]*net.IPNet, 0, len(trusted))
	for _, cidr := range trusted {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			trustedNets = append(trustedNets, ipNet)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := extractClientIP(r, trustedNets)
			r = r.WithContext(WithRequestID(r.Context(), middleware.GetReqID(r.Context())))
			r.RemoteAddr = clientIP
			next.ServeHTTP(w, r)
		})
	}
}

func extractClientIP(r *http.Request, trusted []*net.IPNet) string {
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteHost = r.RemoteAddr
	}
	remoteIP := net.ParseIP(remoteHost)
	if remoteIP == nil {
		return remoteHost
	}

	// Only trust X-Forwarded-For if the immediate peer is trusted.
	if !isTrusted(remoteIP, trusted) {
		return remoteHost
	}

	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return remoteHost
	}

	// X-Forwarded-For is client, proxy1, proxy2, ...; use the rightmost trusted entry.
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ipStr := strings.TrimSpace(parts[i])
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if isTrusted(ip, trusted) {
			continue
		}
		return ipStr
	}
	return remoteHost
}

func isTrusted(ip net.IP, trusted []*net.IPNet) bool {
	for _, n := range trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
