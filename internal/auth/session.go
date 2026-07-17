package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/rueidis"
)

const (
	sessionKeyPrefix = "session:"
)

// Session represents an administrator session stored in Redis.
type Session struct {
	AdminID        int64     `json:"admin_id"`
	Username       string    `json:"username"`
	SessionVersion int64     `json:"session_version"`
	CreatedAt      time.Time `json:"created_at"`
}

// SessionStore manages administrator sessions backed by Redis.
type SessionStore struct {
	client rueidis.Client
	ttl    time.Duration
}

// NewSessionStore creates a new session store.
func NewSessionStore(client rueidis.Client, ttl time.Duration) *SessionStore {
	return &SessionStore{client: client, ttl: ttl}
}

// Create generates a new session ID and stores the session in Redis.
func (s *SessionStore) Create(ctx context.Context, sess Session) (string, error) {
	id := newSessionID()
	data, err := json.Marshal(sess)
	if err != nil {
		return "", fmt.Errorf("marshal session: %w", err)
	}

	resp := s.client.Do(ctx, s.client.B().Set().
		Key(sessionKeyPrefix+id).
		Value(rueidis.BinaryString(data)).
		ExSeconds(int64(s.ttl.Seconds())).
		Build())
	if err := resp.Error(); err != nil {
		return "", fmt.Errorf("redis set: %w", err)
	}
	return id, nil
}

// Get retrieves a session by ID from Redis.
func (s *SessionStore) Get(ctx context.Context, id string) (*Session, error) {
	resp := s.client.Do(ctx, s.client.B().Get().Key(sessionKeyPrefix+id).Build())
	data, err := resp.AsBytes()
	if rueidis.IsRedisNil(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return &sess, nil
}

// Delete removes a session from Redis.
func (s *SessionStore) Delete(ctx context.Context, id string) error {
	resp := s.client.Do(ctx, s.client.B().Del().Key(sessionKeyPrefix+id).Build())
	if err := resp.Error(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}
	return nil
}

// Refresh extends the TTL of an existing session.
func (s *SessionStore) Refresh(ctx context.Context, id string) error {
	resp := s.client.Do(ctx, s.client.B().Expire().Key(sessionKeyPrefix+id).Seconds(int64(s.ttl.Seconds())).Build())
	if err := resp.Error(); err != nil {
		return fmt.Errorf("redis expire: %w", err)
	}
	return nil
}

func newSessionID() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
