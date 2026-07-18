package redeem

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeCode(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"abcd-1234", "ABCD1234"},
		{"  ABcd  ", "ABCD"},
		{"aBcD-EF12_34", "ABCDEF1234"},
		{"", ""},
	}

	for _, c := range cases {
		got := NormalizeCode(c.input)
		if got != c.expected {
			t.Errorf("NormalizeCode(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func TestNormalizeUserKey(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"PlayerOne", "playerone"},
		{"  Player One  ", "player one"},
		{"\tPlayer\n", "player"},
	}

	for _, c := range cases {
		got := NormalizeUserKey(c.input)
		if got != c.expected {
			t.Errorf("NormalizeUserKey(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func TestDisplayCode(t *testing.T) {
	cases := []struct {
		code, prefix, expected string
	}{
		{"PREFIXABCDEFGH", "PREFIX", "PREFIX-ABCD-EFGH"},
		{"ABCDEFGH", "", "ABCD-EFGH"},
		{"ABC", "", "ABC"},
	}

	for _, c := range cases {
		got := DisplayCode(c.code, c.prefix)
		if got != c.expected {
			t.Errorf("DisplayCode(%q, %q) = %q, want %q", c.code, c.prefix, got, c.expected)
		}
	}
}

func TestRedeemRejectsInvalidInputBeforeDatabaseAccess(t *testing.T) {
	service := NewService(nil, nil)
	tests := []RedeemRequest{
		{UserID: "   ", Code: "ABCD1234"},
		{UserID: "user@example.com", Code: strings.Repeat("A", MaxCodeLength+1)},
		{UserID: "user@example.com", Code: "ABCD1234", IdempotencyKey: strings.Repeat("k", MaxIdempotencyKeyLength+1)},
		{UserID: "user@example.com", Code: "ABCD1234", IdempotencyKey: " key-with-space "},
	}
	for _, req := range tests {
		resp, err := service.Redeem(context.Background(), req)
		if err != nil || resp == nil || resp.Result != ResultInvalidInput {
			t.Fatalf("Redeem(%+v) = %+v, %v; want invalid_input", req, resp, err)
		}
	}
}

func TestBatchRedeemRejectsInvalidInputBeforeDatabaseAccess(t *testing.T) {
	service := NewService(nil, nil)
	tests := []BatchRedeemRequest{
		{UserID: "", Codes: []string{"ABCD1234"}},
		{UserID: "user@example.com"},
		{UserID: "user@example.com", Codes: make([]string, MaxBatchCodes+1)},
		{UserID: "user@example.com", Codes: []string{"ABCD1234"}, IdempotencyKey: strings.Repeat("k", MaxIdempotencyKeyLength+1)},
		{UserID: "user@example.com", Codes: []string{"ABCD1234"}, IdempotencyKey: " key-with-space "},
	}
	for _, req := range tests {
		resp, err := service.BatchRedeem(context.Background(), req)
		if err != nil || resp == nil || resp.OK {
			t.Fatalf("BatchRedeem(%+v) = %+v, %v; want validation failure", req, resp, err)
		}
	}
}
