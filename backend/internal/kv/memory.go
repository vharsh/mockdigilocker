package kv

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

type entry struct {
	val []byte
	exp time.Time // zero = no expiry
}

// Memory is an in-process KV with TTL support.
type Memory struct {
	mu   sync.Mutex
	m    map[string]entry
	now  func() time.Time
	stop chan struct{}
}

func NewMemory() *Memory {
	k := &Memory{m: map[string]entry{}, now: time.Now, stop: make(chan struct{})}
	go k.janitor()
	return k
}

func (k *Memory) janitor() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			k.mu.Lock()
			now := k.now()
			for key, e := range k.m {
				if !e.exp.IsZero() && now.After(e.exp) {
					delete(k.m, key)
				}
			}
			k.mu.Unlock()
		case <-k.stop:
			return
		}
	}
}

func (k *Memory) live(key string) (entry, bool) {
	e, ok := k.m[key]
	if ok && !e.exp.IsZero() && k.now().After(e.exp) {
		delete(k.m, key)
		return entry{}, false
	}
	return e, ok
}

func (k *Memory) expiry(ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}
	return k.now().Add(ttl)
}

func (k *Memory) Set(key string, val []byte, ttl time.Duration) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[key] = entry{val: append([]byte(nil), val...), exp: k.expiry(ttl)}
	return nil
}

func (k *Memory) SetNX(key string, val []byte, ttl time.Duration) (bool, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, ok := k.live(key); ok {
		return false, nil
	}
	k.m[key] = entry{val: append([]byte(nil), val...), exp: k.expiry(ttl)}
	return true, nil
}

func (k *Memory) Get(key string) ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	e, ok := k.live(key)
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), e.val...), nil
}

func (k *Memory) Del(keys ...string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, key := range keys {
		delete(k.m, key)
	}
	return nil
}

func (k *Memory) Incr(key string, ttl time.Duration) (int64, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	e, ok := k.live(key)
	var n int64
	if ok {
		n, _ = strconv.ParseInt(string(e.val), 10, 64)
	} else {
		e.exp = k.expiry(ttl)
	}
	n++
	e.val = []byte(strconv.FormatInt(n, 10))
	k.m[key] = e
	return n, nil
}

func (k *Memory) Keys(prefix string) ([]string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	var out []string
	for key := range k.m {
		if strings.HasPrefix(key, prefix) {
			if _, ok := k.live(key); ok {
				out = append(out, key)
			}
		}
	}
	return out, nil
}

func (k *Memory) Ping() error { return nil }

func (k *Memory) Close() error {
	close(k.stop)
	return nil
}
