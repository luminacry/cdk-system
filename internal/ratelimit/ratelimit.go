package ratelimit

import (
	"context"
	"crypto/rand"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/rueidis"
)

var slidingWindowScript = rueidis.NewLuaScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window_start = tonumber(ARGV[2])
local max_requests = tonumber(ARGV[3])
local ttl_ms = tonumber(ARGV[4])
local member = ARGV[5]

redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)
if redis.call('ZCARD', key) >= max_requests then
    return 0
end

redis.call('ZADD', key, now, member)
redis.call('PEXPIRE', key, ttl_ms)
return 1
`)

// Limiter implements a Redis-backed sliding window rate limiter.
type Limiter struct {
	client  rueidis.Client
	window  time.Duration
	maxReqs int
	eval    scriptEvaluator
}

type scriptEvaluator func(context.Context, rueidis.Client, []string, []string) (int64, error)

func evalSlidingWindow(ctx context.Context, client rueidis.Client, keys, args []string) (int64, error) {
	return slidingWindowScript.Exec(ctx, client, keys, args).AsInt64()
}

// NewLimiter creates a new rate limiter.
func NewLimiter(client rueidis.Client, maxReqs int, window time.Duration) *Limiter {
	return &Limiter{
		client:  client,
		window:  window,
		maxReqs: maxReqs,
		eval:    evalSlidingWindow,
	}
}

// Allow checks if a request is allowed under the rate limit for the given key.
// It returns true if allowed, false if rate limited, and an error if Redis fails.
func (l *Limiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now().UnixMilli()
	windowMillis := l.window.Milliseconds()
	if windowMillis <= 0 {
		return false, fmt.Errorf("rate limit window must be positive")
	}
	redisKey := fmt.Sprintf("ratelimit:%s", key)
	member := fmt.Sprintf("%d:%s", now, randomMember())
	result, err := l.eval(ctx, l.client, []string{redisKey}, []string{
		strconv.FormatInt(now, 10),
		strconv.FormatInt(now-windowMillis, 10),
		strconv.Itoa(l.maxReqs),
		strconv.FormatInt(windowMillis, 10),
		member,
	})
	if err != nil {
		return false, fmt.Errorf("redis rate limit script: %w", err)
	}
	return result == 1, nil
}

func randomMember() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
