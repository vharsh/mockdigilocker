package store

import (
	"path/filepath"
	"testing"
)

type rec struct {
	ID string `json:"id"`
	N  int    `json:"n"`
}

func TestMemoryStoreAndSnapshot(t *testing.T) {
	f := filepath.Join(t.TempDir(), "data.json")
	m, err := NewMemory(f)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		_ = m.Put("c", string(rune('a'+i)), rec{ID: string(rune('a' + i)), N: i})
	}
	_ = m.Put("c", "a", rec{ID: "a", N: 100}) // update keeps insertion order
	list, _ := List[rec](m, "c", nil)
	if len(list) != 10 || list[0].N != 100 {
		t.Fatalf("list %v", list)
	}
	_ = m.Trim("c", 3)
	list, _ = List[rec](m, "c", nil)
	if len(list) != 3 || list[0].ID != "h" {
		t.Fatalf("trim kept %v", list)
	}
	if err := m.Get("c", "a", &rec{}); err != ErrNotFound {
		t.Fatal("expected not found")
	}
	_ = m.Close()
	m2, _ := NewMemory(f)
	list, _ = List[rec](m2, "c", func(r *rec) bool { return r.N > 7 })
	if len(list) != 2 {
		t.Fatalf("snapshot reload: %v", list)
	}
	_ = m2.Close()
}
