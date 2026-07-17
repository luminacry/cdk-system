package redis

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/redis/rueidis"
)

// NewClient creates a rueidis Redis client from a Redis URL.
func NewClient(redisURL string) (rueidis.Client, error) {
	u, err := url.Parse(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}

	opts := rueidis.ClientOption{
		InitAddress:  []string{fmt.Sprintf("%s:%s", u.Hostname(), defaultPort(u.Port(), "6379"))},
		DisableCache: true,
	}

	if u.User != nil {
		if password, ok := u.User.Password(); ok {
			opts.Password = password
		}
		if dbStr := u.User.Username(); dbStr != "" {
			if db, err := strconv.Atoi(dbStr); err == nil {
				opts.SelectDB = db
			}
		}
	}

	client, err := rueidis.NewClient(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return client, nil
}

func defaultPort(port, fallback string) string {
	if port == "" {
		return fallback
	}
	return port
}
