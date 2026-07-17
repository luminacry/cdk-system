package webhook

import (
	"net"
	"testing"
)

func TestForbiddenIPRejectsNonPublicNetworks(t *testing.T) {
	for _, raw := range []string{
		"0.0.0.0",
		"127.0.0.1",
		"10.0.0.1",
		"169.254.169.254",
		"224.0.0.1",
		"::",
		"::1",
		"fc00::1",
		"ff02::1",
	} {
		if !isForbiddenIP(net.ParseIP(raw)) {
			t.Errorf("isForbiddenIP(%q) = false, want true", raw)
		}
	}
	if isForbiddenIP(net.ParseIP("8.8.8.8")) {
		t.Error("public unicast address was rejected")
	}
}

func TestTruncatePreservesUTF8(t *testing.T) {
	if got := truncate("兑换成功", 2); got != "兑换" {
		t.Fatalf("truncate() = %q, want %q", got, "兑换")
	}
}
