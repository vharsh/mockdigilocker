package kv

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Redis is a small RESP2 client covering exactly the commands the sandbox
// needs, with a fixed-size connection pool. It keeps the server free of
// third-party dependencies.
type Redis struct {
	addr, pass string
	db         int
	pool       chan *rconn
	timeout    time.Duration
}

type rconn struct {
	c net.Conn
	r *bufio.Reader
}

type redisError string

func (e redisError) Error() string { return "redis: " + string(e) }

func NewRedis(addr, pass string, db int) (*Redis, error) {
	r := &Redis{addr: addr, pass: pass, db: db, pool: make(chan *rconn, 16), timeout: 5 * time.Second}
	var err error
	for i := 0; i < 30; i++ { // wait for the redis container to come up
		if err = r.Ping(); err == nil {
			return r, nil
		}
		time.Sleep(time.Second)
	}
	return nil, fmt.Errorf("redis %s: %w", addr, err)
}

func (r *Redis) dial() (*rconn, error) {
	c, err := net.DialTimeout("tcp", r.addr, r.timeout)
	if err != nil {
		return nil, err
	}
	rc := &rconn{c: c, r: bufio.NewReader(c)}
	if r.pass != "" {
		if _, err := rc.do(r.timeout, "AUTH", r.pass); err != nil {
			c.Close()
			return nil, err
		}
	}
	if r.db != 0 {
		if _, err := rc.do(r.timeout, "SELECT", strconv.Itoa(r.db)); err != nil {
			c.Close()
			return nil, err
		}
	}
	return rc, nil
}

func (r *Redis) do(args ...string) (any, error) {
	var rc *rconn
	select {
	case rc = <-r.pool:
	default:
		var err error
		if rc, err = r.dial(); err != nil {
			return nil, err
		}
	}
	v, err := rc.do(r.timeout, args...)
	var re redisError
	if err != nil && !errors.As(err, &re) {
		rc.c.Close() // network/protocol error: drop the connection
		return nil, err
	}
	select {
	case r.pool <- rc:
	default:
		rc.c.Close()
	}
	return v, err
}

func (rc *rconn) do(timeout time.Duration, args ...string) (any, error) {
	_ = rc.c.SetDeadline(time.Now().Add(timeout))
	var sb strings.Builder
	fmt.Fprintf(&sb, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&sb, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := io.WriteString(rc.c, sb.String()); err != nil {
		return nil, err
	}
	return readReply(rc.r)
}

func readReply(r *bufio.Reader) (any, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(line, "\r\n")
	if line == "" {
		return nil, errors.New("redis: empty reply")
	}
	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return nil, redisError(line[1:])
	case ':':
		return strconv.ParseInt(line[1:], 10, 64)
	case '$':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return buf[:n], nil
	case '*':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		out := make([]any, n)
		for i := range out {
			if out[i], err = readReply(r); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("redis: unexpected reply %q", line)
}

func ttlArgs(ttl time.Duration) []string {
	if ttl <= 0 {
		return nil
	}
	ms := ttl.Milliseconds()
	if ms < 1 {
		ms = 1
	}
	return []string{"PX", strconv.FormatInt(ms, 10)}
}

func (r *Redis) Set(key string, val []byte, ttl time.Duration) error {
	_, err := r.do(append([]string{"SET", key, string(val)}, ttlArgs(ttl)...)...)
	return err
}

func (r *Redis) SetNX(key string, val []byte, ttl time.Duration) (bool, error) {
	v, err := r.do(append([]string{"SET", key, string(val), "NX"}, ttlArgs(ttl)...)...)
	if err != nil {
		return false, err
	}
	return v != nil, nil
}

func (r *Redis) Get(key string) ([]byte, error) {
	v, err := r.do("GET", key)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, ErrNotFound
	}
	return v.([]byte), nil
}

func (r *Redis) Del(keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	_, err := r.do(append([]string{"DEL"}, keys...)...)
	return err
}

func (r *Redis) Incr(key string, ttl time.Duration) (int64, error) {
	v, err := r.do("INCR", key)
	if err != nil {
		return 0, err
	}
	n := v.(int64)
	if n == 1 && ttl > 0 {
		_, err = r.do("PEXPIRE", key, strconv.FormatInt(ttl.Milliseconds(), 10))
	}
	return n, err
}

func (r *Redis) Keys(prefix string) ([]string, error) {
	var out []string
	cursor := "0"
	pattern := escapeGlob(prefix) + "*"
	for {
		v, err := r.do("SCAN", cursor, "MATCH", pattern, "COUNT", "1000")
		if err != nil {
			return nil, err
		}
		arr, ok := v.([]any)
		if !ok || len(arr) != 2 {
			return nil, errors.New("redis: bad SCAN reply")
		}
		cursor = string(arr[0].([]byte))
		for _, k := range arr[1].([]any) {
			out = append(out, string(k.([]byte)))
		}
		if cursor == "0" {
			return out, nil
		}
	}
}

func escapeGlob(s string) string {
	var b strings.Builder
	for _, c := range s {
		if strings.ContainsRune(`*?[]\`, c) {
			b.WriteByte('\\')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func (r *Redis) Ping() error {
	_, err := r.do("PING")
	return err
}

func (r *Redis) Close() error {
	for {
		select {
		case rc := <-r.pool:
			rc.c.Close()
		default:
			return nil
		}
	}
}
