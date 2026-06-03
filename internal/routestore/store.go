// Package routestore persists scenic route results in Redis using gob encoding.
package routestore

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/orofarne/scenic-routing-mcp/internal/scenic"
)

// ErrNotFound is returned when a route UUID is unknown or expired.
var ErrNotFound = errors.New("route not found or expired")

type entry struct {
	Params scenic.Params
	Result scenic.Result
}

// Store saves and loads route entries in Redis.
type Store struct {
	rdb *redis.Client
	ttl time.Duration
}

// New creates a Store connected to redisAddr (host:port) with the given TTL.
func New(redisAddr string, ttl time.Duration) *Store {
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	return &Store{rdb: rdb, ttl: ttl}
}

// Save encodes params+result via gob, stores them in Redis, and returns a new UUID.
func (s *Store) Save(ctx context.Context, params scenic.Params, result *scenic.Result) (string, error) {
	id := uuid.New().String()
	e := entry{Params: params, Result: *result}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(e); err != nil {
		return "", fmt.Errorf("routestore encode: %w", err)
	}
	if err := s.rdb.Set(ctx, "route:"+id, buf.Bytes(), s.ttl).Err(); err != nil {
		return "", fmt.Errorf("routestore save: %w", err)
	}
	return id, nil
}

// Load retrieves and decodes the entry for the given UUID.
// Returns ErrNotFound if the key is absent or expired.
func (s *Store) Load(ctx context.Context, id string) (scenic.Params, *scenic.Result, error) {
	data, err := s.rdb.Get(ctx, "route:"+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return scenic.Params{}, nil, ErrNotFound
	}
	if err != nil {
		return scenic.Params{}, nil, fmt.Errorf("routestore load: %w", err)
	}
	var e entry
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&e); err != nil {
		return scenic.Params{}, nil, fmt.Errorf("routestore decode: %w", err)
	}
	return e.Params, &e.Result, nil
}
