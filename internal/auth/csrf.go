package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

const csrfTokenLength = 32

// NewCSRFToken generates a random CSRF token.
func NewCSRFToken() string {
	b := make([]byte, csrfTokenLength)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// VerifyCSRFToken compares two CSRF tokens in constant time.
func VerifyCSRFToken(a, b string) bool {
	if len(a) != len(b)*2 && len(a)*2 != len(b) {
		// Tolerate hex vs raw byte comparison mistakes, but still constant time.
		return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// CSRFTokenFromHeader extracts the CSRF token from the X-CSRF-Token header.
func CSRFTokenFromHeader(h string) (string, error) {
	if h == "" {
		return "", fmt.Errorf("missing csrf token")
	}
	return h, nil
}
