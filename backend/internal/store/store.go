// Package store provides durable persistence for sandbox entities.
//
// Entities are stored as JSON documents grouped into collections. Two
// backends exist: an in-memory store (optionally snapshotted to a JSON file)
// and a PostgreSQL store (one JSONB table). The document model keeps the
// sandbox schema flexible while test personas and API profiles evolve.
package store

import (
	"encoding/json"
	"errors"
	"sort"
)

var ErrNotFound = errors.New("not found")

type Store interface {
	Put(coll, id string, v any) error
	// Get unmarshals the record into v; returns ErrNotFound if absent.
	Get(coll, id string, v any) error
	Delete(coll, id string) error
	// Scan calls fn for each record in insertion order.
	Scan(coll string, fn func(id string, raw []byte) error) error
	Count(coll string) (int, error)
	Clear(coll string) error
	// Trim keeps only the newest max records of a collection.
	Trim(coll string, max int) error
	Ping() error
	Close() error
}

// List loads every record of coll that matches keep (nil keeps all).
func List[T any](s Store, coll string, keep func(*T) bool) ([]T, error) {
	var out []T
	err := s.Scan(coll, func(_ string, raw []byte) error {
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil // skip corrupt records rather than failing the listing
		}
		if keep == nil || keep(&v) {
			out = append(out, v)
		}
		return nil
	})
	return out, err
}

// ListSorted is List followed by a stable sort.
func ListSorted[T any](s Store, coll string, keep func(*T) bool, less func(a, b *T) bool) ([]T, error) {
	out, err := List(s, coll, keep)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return less(&out[i], &out[j]) })
	return out, nil
}

// GetT is a typed Get.
func GetT[T any](s Store, coll, id string) (*T, error) {
	var v T
	if err := s.Get(coll, id, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
