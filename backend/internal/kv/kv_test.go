package kv

import (
	"os"
	"testing"
	"time"
)

func testKV(t *testing.T, k KV) {
	if err := k.Set("t:a", []byte("1"), 0); err != nil {
		t.Fatal(err)
	}
	if b, _ := k.Get("t:a"); string(b) != "1" {
		t.Fatal("get")
	}
	if ok, _ := k.SetNX("t:a", []byte("2"), 0); ok {
		t.Fatal("setnx overwrote")
	}
	if n, _ := k.Incr("t:n", time.Minute); n != 1 {
		t.Fatal("incr")
	}
	if n, _ := k.Incr("t:n", time.Minute); n != 2 {
		t.Fatal("incr 2")
	}
	_ = k.Set("t:exp", []byte("x"), 30*time.Millisecond)
	time.Sleep(80 * time.Millisecond)
	if _, err := k.Get("t:exp"); err != ErrNotFound {
		t.Fatal("ttl not applied")
	}
	keys, _ := k.Keys("t:")
	if len(keys) != 2 {
		t.Fatalf("keys %v", keys)
	}
	if n, _ := DelPrefix(k, "t:"); n != 2 {
		t.Fatal("delprefix")
	}
	if err := SetJSON(k, "t:j", map[string]int{"a": 1}, time.Minute); err != nil {
		t.Fatal(err)
	}
	var m map[string]int
	if GetJSON(k, "t:j", &m) != nil || m["a"] != 1 {
		t.Fatal("json")
	}
	_ = k.Del("t:j")
}

func TestMemoryKV(t *testing.T) { testKV(t, NewMemory()) }

func TestRedisKV(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set TEST_REDIS_ADDR to run against Redis")
	}
	r, err := NewRedis(addr, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	_, _ = DelPrefix(r, "t:")
	testKV(t, r)
}
