package webhook

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config controls webhook delivery behavior.
type Config struct {
	AllowedDomains   []string
	MaxAttempts      int
	InitialDelay     time.Duration
	MaxDelay         time.Duration
	BackoffFactor    float64
	RequestTimeout   time.Duration
	MaxResponseBytes int64
}

// DefaultConfig returns a safe default webhook configuration.
func DefaultConfig() Config {
	return Config{
		AllowedDomains:   []string{},
		MaxAttempts:      10,
		InitialDelay:     5 * time.Second,
		MaxDelay:         1 * time.Hour,
		BackoffFactor:    2.0,
		RequestTimeout:   8 * time.Second,
		MaxResponseBytes: 4096,
	}
}

// ValidateURL checks that a webhook URL is safe to call.
func ValidateURL(rawURL string, allowedDomains []string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	if u.Scheme != "https" {
		return fmt.Errorf("only https webhook urls are allowed")
	}

	if u.Hostname() == "" {
		return fmt.Errorf("missing host")
	}

	if u.User != nil {
		return fmt.Errorf("url must not contain credentials")
	}

	// Check custom port.
	if u.Port() != "" && u.Port() != "443" {
		return fmt.Errorf("custom ports are not allowed")
	}

	// Domain allowlist.
	if len(allowedDomains) > 0 {
		allowed := false
		host := strings.ToLower(u.Hostname())
		for _, d := range allowedDomains {
			if host == strings.ToLower(d) || strings.HasSuffix(host, "."+strings.ToLower(d)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("domain not in allowlist")
		}
	}

	// Resolve and check IPs.
	ips, err := net.LookupIP(u.Hostname())
	if err != nil {
		return fmt.Errorf("dns lookup failed: %w", err)
	}
	for _, ip := range ips {
		if isForbiddenIP(ip) {
			return fmt.Errorf("forbidden ip: %s", ip)
		}
	}

	return nil
}

func isForbiddenIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	// Cloud metadata IPs.
	if ip.Equal(net.ParseIP("169.254.169.254")) {
		return true
	}
	return false
}

// SafeTransport returns an http.Transport that validates resolved IPs before connecting.
func SafeTransport(timeout time.Duration) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.LookupIP(host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if isForbiddenIP(ip) {
					return nil, fmt.Errorf("forbidden ip: %s", ip)
				}
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("no ips resolved for %s", host)
			}
			// Use the first allowed IP.
			dialer := &net.Dialer{Timeout: timeout}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// NoRedirect returns an error when a redirect is encountered.
func NoRedirect(req *http.Request, via []*http.Request) error {
	return fmt.Errorf("redirects are not allowed")
}
