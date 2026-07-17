package ratelimit

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/redis/rueidis"
)

func TestAllowExecutesSingleAtomicScript(t *testing.T) {
	var calls int
	limiter := &Limiter{
		window:  time.Minute,
		maxReqs: 3,
		eval: func(_ context.Context, _ rueidis.Client, keys, args []string) (int64, error) {
			calls++
			if len(keys) != 1 || keys[0] != "ratelimit:login:127.0.0.1" {
				t.Fatalf("keys = %v", keys)
			}
			if len(args) != 5 {
				t.Fatalf("args length = %d, want 5", len(args))
			}
			now, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			start, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			if now-start != time.Minute.Milliseconds() {
				t.Fatalf("window = %dms, want %dms", now-start, time.Minute.Milliseconds())
			}
			if args[2] != "3" || args[3] != "60000" || !strings.HasPrefix(args[4], args[0]+":") {
				t.Fatalf("unexpected script args: %v", args)
			}
			return 1, nil
		},
	}

	allowed, err := limiter.Allow(context.Background(), "login:127.0.0.1")
	if err != nil || !allowed {
		t.Fatalf("Allow() = %v, %v; want true, nil", allowed, err)
	}
	if calls != 1 {
		t.Fatalf("script calls = %d, want 1", calls)
	}
}

func TestAllowHandlesDeniedAndScriptError(t *testing.T) {
	limiter := &Limiter{
		window:  time.Second,
		maxReqs: 1,
		eval: func(context.Context, rueidis.Client, []string, []string) (int64, error) {
			return 0, nil
		},
	}
	allowed, err := limiter.Allow(context.Background(), "key")
	if err != nil || allowed {
		t.Fatalf("denied Allow() = %v, %v; want false, nil", allowed, err)
	}

	redisErr := errors.New("redis unavailable")
	limiter.eval = func(context.Context, rueidis.Client, []string, []string) (int64, error) {
		return 0, redisErr
	}
	allowed, err = limiter.Allow(context.Background(), "key")
	if allowed || !errors.Is(err, redisErr) {
		t.Fatalf("error Allow() = %v, %v; want false and wrapped Redis error", allowed, err)
	}
}

func TestAllowRejectsNonPositiveWindow(t *testing.T) {
	limiter := &Limiter{window: 0, maxReqs: 1}
	allowed, err := limiter.Allow(context.Background(), "key")
	if allowed || err == nil {
		t.Fatalf("Allow() = %v, %v; want false and validation error", allowed, err)
	}
}
