package store

import (
	"sync"
	"testing"
	"uuid"
)

func TestCreateUsesV7(t *testing.T) {
	s := New(8)
	id := s.Create()
	u, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("parse inbox id: %v", err)
	}
	if v := u[6] >> 4; v != 7 {
		t.Fatalf("uuid version = %d, want 7", v)
	}
	if !s.Has(id) {
		t.Fatal("created inbox missing")
	}
}

func TestRingEvictsOldest(t *testing.T) {
	s := New(3)
	id := s.Create()
	for i := range 5 {
		if _, err := s.Append(id, Record{Method: "POST", Path: "/i/" + id, Query: string(rune('a' + i))}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.List(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Query != "c" || got[2].Query != "e" {
		t.Fatalf("ring contents = %+v", got)
	}
}

func TestAppendUnknownInbox(t *testing.T) {
	s := New(8)
	if _, err := s.Append("missing", Record{}); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestConcurrentAppend(t *testing.T) {
	const n = 64
	s := New(n)
	id := s.Create()
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			if _, err := s.Append(id, Record{Method: "POST", Body: []byte("x")}); err != nil {
				t.Errorf("append: %v", err)
			}
		}()
	}
	wg.Wait()
	got, err := s.List(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("len = %d, want %d", len(got), n)
	}
}
