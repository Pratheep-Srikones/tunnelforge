package capture

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestNewRingBuffer(t *testing.T) {
	rbDefault := NewRingBuffer(0)
	if rbDefault.defaultSize != DefaultMaxRequests {
		t.Fatalf("expected default size %d, got %d", DefaultMaxRequests, rbDefault.defaultSize)
	}

	rbNegative := NewRingBuffer(-10)
	if rbNegative.defaultSize != DefaultMaxRequests {
		t.Fatalf("expected default size %d for negative input, got %d", DefaultMaxRequests, rbNegative.defaultSize)
	}

	rbCustom := NewRingBuffer(500)
	if rbCustom.defaultSize != 500 {
		t.Fatalf("expected custom size 500, got %d", rbCustom.defaultSize)
	}
}

func TestRingBuffer_PushAndGet(t *testing.T) {
	rb := NewRingBuffer(5)

	entry := &RequestEntry{
		ID:             "req-1",
		Subdomain:      "api",
		Timestamp:      time.Now(),
		Method:         http.MethodGet,
		URL:            "/users",
		ResponseStatus: http.StatusOK,
	}

	err := rb.Push("api", entry)
	if err != nil {
		t.Fatalf("unexpected push error: %v", err)
	}

	got, err := rb.Get("api", "req-1")
	if err != nil {
		t.Fatalf("unexpected get error: %v", err)
	}
	if got.ID != "req-1" || got.Method != http.MethodGet || got.URL != "/users" {
		t.Fatalf("unexpected entry retrieved: %+v", got)
	}

	// Not found
	_, err = rb.Get("api", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent ID, got nil")
	}

	// Unknown subdomain
	_, err = rb.Get("unknown", "req-1")
	if err == nil {
		t.Fatal("expected error for unknown subdomain, got nil")
	}
}

func TestRingBuffer_List(t *testing.T) {
	rb := NewRingBuffer(10)

	// List on empty/unseen subdomain should return empty slice, no error
	list, err := rb.List("empty", 0)
	if err != nil {
		t.Fatalf("expected no error for empty subdomain, got: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(list))
	}

	// Push 5 items
	for i := 1; i <= 5; i++ {
		_ = rb.Push("api", &RequestEntry{
			ID:     fmt.Sprintf("req-%d", i),
			Method: http.MethodGet,
			URL:    fmt.Sprintf("/item/%d", i),
		})
	}

	// List all (limit = 0)
	all, err := rb.List("api", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("expected 5 items, got %d", len(all))
	}
	if all[0].ID != "req-1" || all[4].ID != "req-5" {
		t.Fatalf("unexpected ordering: first=%s, last=%s", all[0].ID, all[4].ID)
	}

	// List with limit=2 (should return the 2 most recent)
	recent, err := rb.List("api", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2 items, got %d", len(recent))
	}
	if recent[0].ID != "req-4" || recent[1].ID != "req-5" {
		t.Fatalf("expected req-4 and req-5, got %s and %s", recent[0].ID, recent[1].ID)
	}

	// List with limit exceeding total
	over, err := rb.List("api", 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(over) != 5 {
		t.Fatalf("expected 5 items, got %d", len(over))
	}
}

func TestRingBuffer_Eviction(t *testing.T) {
	rb := NewRingBuffer(3) // capacity = 3

	for i := 1; i <= 5; i++ {
		err := rb.Push("api", &RequestEntry{
			ID:     fmt.Sprintf("req-%d", i),
			Method: http.MethodGet,
		})
		if err != nil {
			t.Fatalf("push error: %v", err)
		}
	}

	entries, err := rb.List("api", 0)
	if err != nil {
		t.Fatalf("list error: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries after eviction, got %d", len(entries))
	}

	// Oldest entries (req-1 and req-2) should be evicted; req-3, req-4, req-5 retained
	expectedIDs := []string{"req-3", "req-4", "req-5"}
	for i, exp := range expectedIDs {
		if entries[i].ID != exp {
			t.Fatalf("index %d: expected %s, got %s", i, exp, entries[i].ID)
		}
	}

	// Verify req-1 and req-2 are no longer retrievable
	if _, err := rb.Get("api", "req-1"); err == nil {
		t.Fatal("expected req-1 to be evicted, but was found")
	}
	if _, err := rb.Get("api", "req-2"); err == nil {
		t.Fatal("expected req-2 to be evicted, but was found")
	}
	// Verify req-5 is retrievable
	if _, err := rb.Get("api", "req-5"); err != nil {
		t.Fatalf("expected req-5 to be found, got error: %v", err)
	}
}

func TestRingBuffer_RemoveAndClear(t *testing.T) {
	rb := NewRingBuffer(5)

	_ = rb.Push("api", &RequestEntry{ID: "req-1"})
	_ = rb.Push("api", &RequestEntry{ID: "req-2"})
	_ = rb.Push("api", &RequestEntry{ID: "req-3"})

	// Remove middle element
	err := rb.Remove("api", "req-2")
	if err != nil {
		t.Fatalf("unexpected remove error: %v", err)
	}

	entries, err := rb.List("api", 0)
	if err != nil {
		t.Fatalf("unexpected list error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].ID != "req-1" || entries[1].ID != "req-3" {
		t.Fatalf("expected [req-1, req-3], got [%s, %s]", entries[0].ID, entries[1].ID)
	}

	// Remove nonexistent element
	err = rb.Remove("api", "nonexistent")
	if err == nil {
		t.Fatal("expected error removing nonexistent element, got nil")
	}

	// Clear subdomain
	err = rb.Clear("api")
	if err != nil {
		t.Fatalf("unexpected clear error: %v", err)
	}

	entries, err = rb.List("api", 0)
	if err != nil {
		t.Fatalf("unexpected list error after clear: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries after clear, got %d", len(entries))
	}
}

func TestRingBuffer_SubdomainIsolation(t *testing.T) {
	rb := NewRingBuffer(10)

	_ = rb.Push("api", &RequestEntry{ID: "api-1"})
	_ = rb.Push("web", &RequestEntry{ID: "web-1"})

	apiEntries, _ := rb.List("api", 0)
	webEntries, _ := rb.List("web", 0)

	if len(apiEntries) != 1 || apiEntries[0].ID != "api-1" {
		t.Fatalf("unexpected api entries: %+v", apiEntries)
	}
	if len(webEntries) != 1 || webEntries[0].ID != "web-1" {
		t.Fatalf("unexpected web entries: %+v", webEntries)
	}

	// Clear api only
	_ = rb.Clear("api")
	apiAfter, _ := rb.List("api", 0)
	webAfter, _ := rb.List("web", 0)

	if len(apiAfter) != 0 {
		t.Fatalf("expected api to be empty, got %d", len(apiAfter))
	}
	if len(webAfter) != 1 || webAfter[0].ID != "web-1" {
		t.Fatalf("expected web to be untouched, got %+v", webAfter)
	}
}

func TestRingBuffer_ValidationErrors(t *testing.T) {
	rb := NewRingBuffer(5)

	if err := rb.Push("", &RequestEntry{ID: "1"}); err == nil {
		t.Fatal("expected error on empty subdomain, got nil")
	}

	if err := rb.Push("api", nil); err == nil {
		t.Fatal("expected error on nil entry, got nil")
	}
}

func TestRingBuffer_ConcurrentAccess(t *testing.T) {
	rb := NewRingBuffer(20)
	subdomain := "concurrency-test"
	iterations := 100
	workers := 10

	var wg sync.WaitGroup

	// Writers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = rb.Push(subdomain, &RequestEntry{
					ID:        fmt.Sprintf("w%d-%d", workerID, i),
					Timestamp: time.Now(),
					Method:    http.MethodPost,
				})
			}
		}(w)
	}

	// Readers
	for r := 0; r < workers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_, _ = rb.List(subdomain, 5)
				_, _ = rb.Get(subdomain, "w0-0")
			}
		}()
	}

	wg.Wait()

	entries, err := rb.List(subdomain, 0)
	if err != nil {
		t.Fatalf("unexpected list error after concurrency: %v", err)
	}
	if len(entries) > 20 {
		t.Fatalf("expected at most 20 entries due to ring buffer capacity, got %d", len(entries))
	}
}
