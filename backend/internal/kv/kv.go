// Package kv stores short-lived state: OTPs, authorization requests, codes,
// tokens and sessions. Backed by Redis in shared deployments, or an
// in-process map for single-binary local runs.
package kv

import (
	"encoding/json"
	"errors"
	"time"
)

var ErrNotFound = errors.New("kv: not found")

type KV interface {
	Set(key string, val []byte, ttl time.Duration) error
	// SetNX sets the key only if absent; reports whether it was set.
	SetNX(key string, val []byte, ttl time.Duration) (bool, error)
	Get(key string) ([]byte, error)
	Del(keys ...string) error
	// Incr increments an integer counter, applying ttl when it is created.
	Incr(key string, ttl time.Duration) (int64, error)
	// Keys lists keys with the given prefix.
	Keys(prefix string) ([]string, error)
	Ping() error
	Close() error
}

func SetJSON(k KV, key string, v any, ttl time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return k.Set(key, b, ttl)
}

func GetJSON(k KV, key string, v any) error {
	b, err := k.Get(key)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// DelPrefix deletes every key with the given prefix.
func DelPrefix(k KV, prefix string) (int, error) {
	keys, err := k.Keys(prefix)
	if err != nil || len(keys) == 0 {
		return 0, err
	}
	for i := 0; i < len(keys); i += 500 {
		end := min(i+500, len(keys))
		if err := k.Del(keys[i:end]...); err != nil {
			return i, err
		}
	}
	return len(keys), nil
}
