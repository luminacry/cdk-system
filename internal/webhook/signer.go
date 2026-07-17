package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Sign signs a webhook payload with the given secret using HMAC-SHA256.
func Sign(payload []byte, secret string) string {
	if secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature verifies a webhook signature.
func VerifySignature(payload []byte, secret, signature string) (bool, error) {
	expected := Sign(payload, secret)
	if expected == "" {
		return false, fmt.Errorf("no secret configured")
	}
	return hmac.Equal([]byte(expected), []byte(signature)), nil
}
