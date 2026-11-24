package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type record struct {
	seq uint64
	raw []byte
}

// Memory is a thread-safe in-memory store. When file is non-empty, the full
// state is snapshotted to disk (debounced) and reloaded on start.
type Memory struct {
	mu       sync.RWMutex
	data     map[string]map[string]*record
	seq      uint64
	file     string
	dirty    chan struct{}
	done     chan struct{}
	finished chan struct{}
}

func NewMemory(file string) (*Memory, error) {
	m := &Memory{data: map[string]map[string]*record{}, file: file, dirty: make(chan struct{}, 1), done: make(chan struct{}), finished: make(chan struct{})}
	if file != "" {
		if err := m.load(); err != nil {
			return nil, err
		}
		go m.flusher()
	}
	return m, nil
}

type snapshot struct {
	Seq  uint64                                `json:"seq"`
	Data map[string]map[string]json.RawMessage `json:"data"`
	Ord  map[string]map[string]uint64          `json:"ord"`
}

func (m *Memory) load() error {
	b, err := os.ReadFile(m.file)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var s snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	m.seq = s.Seq
	for c, recs := range s.Data {
		m.data[c] = map[string]*record{}
		for id, raw := range recs {
			m.data[c][id] = &record{seq: s.Ord[c][id], raw: raw}
		}
	}
	return nil
}

func (m *Memory) flusher() {
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	pending := false
	for {
		select {
		case <-m.dirty:
			pending = true
		case <-t.C:
			if pending {
				_ = m.flush()
				pending = false
			}
		case <-m.done:
			_ = m.flush() // final flush happens here only, never concurrently
			close(m.finished)
			return
		}
	}
}

func (m *Memory) flush() error {
	m.mu.RLock()
	s := snapshot{Seq: m.seq, Data: map[string]map[string]json.RawMessage{}, Ord: map[string]map[string]uint64{}}
	for c, recs := range m.data {
		s.Data[c] = map[string]json.RawMessage{}
		s.Ord[c] = map[string]uint64{}
		for id, r := range recs {
			s.Data[c][id] = r.raw
			s.Ord[c][id] = r.seq
		}
	}
	m.mu.RUnlock()
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(m.file), 0o755)
	tmp := m.file + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.file)
}

func (m *Memory) touch() {
	if m.file == "" {
		return
	}
	select {
	case m.dirty <- struct{}{}:
	default:
	}
}

func (m *Memory) Put(coll, id string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	m.mu.Lock()
	c := m.data[coll]
	if c == nil {
		c = map[string]*record{}
		m.data[coll] = c
	}
	if r, ok := c[id]; ok {
		r.raw = raw
	} else {
		m.seq++
		c[id] = &record{seq: m.seq, raw: raw}
	}
	m.mu.Unlock()
	m.touch()
	return nil
}

func (m *Memory) Get(coll, id string, v any) error {
	m.mu.RLock()
	r, ok := m.data[coll][id]
	var raw []byte
	if ok {
		raw = r.raw
	}
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	return json.Unmarshal(raw, v)
}

func (m *Memory) Delete(coll, id string) error {
	m.mu.Lock()
	delete(m.data[coll], id)
	m.mu.Unlock()
	m.touch()
	return nil
}

func (m *Memory) Scan(coll string, fn func(id string, raw []byte) error) error {
	type kv struct {
		id  string
		seq uint64
		raw []byte
	}
	m.mu.RLock()
	items := make([]kv, 0, len(m.data[coll]))
	for id, r := range m.data[coll] {
		items = append(items, kv{id, r.seq, r.raw})
	}
	m.mu.RUnlock()
	sortBy(items, func(a, b kv) bool { return a.seq < b.seq })
	for _, it := range items {
		if err := fn(it.id, it.raw); err != nil {
			return err
		}
	}
	return nil
}

func (m *Memory) Count(coll string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data[coll]), nil
}

func (m *Memory) Clear(coll string) error {
	m.mu.Lock()
	delete(m.data, coll)
	m.mu.Unlock()
	m.touch()
	return nil
}

func (m *Memory) Trim(coll string, max int) error {
	m.mu.Lock()
	c := m.data[coll]
	if len(c) > max {
		type kv struct {
			id  string
			seq uint64
		}
		items := make([]kv, 0, len(c))
		for id, r := range c {
			items = append(items, kv{id, r.seq})
		}
		sortBy(items, func(a, b kv) bool { return a.seq < b.seq })
		for _, it := range items[:len(items)-max] {
			delete(c, it.id)
		}
	}
	m.mu.Unlock()
	m.touch()
	return nil
}

func (m *Memory) Ping() error { return nil }

func (m *Memory) Close() error {
	if m.file != "" {
		close(m.done)
		<-m.finished
	}
	return nil
}
