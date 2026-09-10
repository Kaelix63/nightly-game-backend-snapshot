package snapshotstore

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPresignSnapshotPutRetriesRateLimit(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization header was not set")
		}
		if requests == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		var body PresignPutRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Op != "put" || body.ExpiresSeconds != 900 || body.IdempotencyKey != "night-2026-08-03" {
			t.Errorf("unexpected body: %+v", body)
		}
		_, _ = io.WriteString(w, `{"ok":true,"data":{"url":"https://signed.example/upload"},"error":null,"metadata":{}}`)
	}))
	defer server.Close()

	client, err := NewClient("test-key")
	if err != nil {
		t.Fatal(err)
	}
	client.baseURL = server.URL
	client.httpClient = server.Client()
	client.sleep = func(context.Context, time.Duration) error { return nil }

	result, err := client.PresignSnapshotPut(context.Background(), "game-backups", "snapshot.json", 42, "night-2026-08-03")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	if result.URL != "https://signed.example/upload" {
		t.Fatalf("url = %q", result.URL)
	}
}
