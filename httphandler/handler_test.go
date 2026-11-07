package httphandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/josephgoksu/metagrab"
)

func newTestHandler(apiKey string) *Handler {
	client := metagrab.NewClient(metagrab.WithTimeout(5 * time.Second))
	return New(client, WithAPIKey(apiKey))
}

func TestHealth(t *testing.T) {
	h := newTestHandler("")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected status=ok, got %q", resp.Status)
	}
}

func TestAuthRejection(t *testing.T) {
	h := newTestHandler("secret-key")

	do := func(key string) int {
		req := httptest.NewRequest(http.MethodPost, "/fetch", strings.NewReader(`{}`))
		if key != "" {
			req.Header.Set("X-API-Key", key)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := do(""); code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without key, got %d", code)
	}
	if code := do("wrong-key"); code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong key, got %d", code)
	}
	// Correct key passes auth; the empty body then fails validation.
	if code := do("secret-key"); code != http.StatusBadRequest {
		t.Fatalf("expected 400 with correct key, got %d", code)
	}
}

func TestHealthSkipsAuth(t *testing.T) {
	h := newTestHandler("secret-key")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected /health to be open, got %d", rec.Code)
	}
}

func TestUpstreamStatusMapsTo502(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer upstream.Close()

	h := newTestHandler("")
	body := fmt.Sprintf(`{"url":%q}`, upstream.URL)
	req := httptest.NewRequest(http.MethodPost, "/fetch", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	var fe metagrab.FetchError
	if err := json.Unmarshal(rec.Body.Bytes(), &fe); err != nil {
		t.Fatal(err)
	}
	if fe.Code != metagrab.ErrorCodeHTTPStatus || fe.StatusCode != http.StatusUnauthorized {
		t.Fatalf("body = %+v, want http_status_error with status_code 401", fe)
	}
}

func TestNoAuthWhenKeyEmpty(t *testing.T) {
	h := newTestHandler("")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when no apiKey configured, got %d", rec.Code)
	}
}

func TestFetchMissingURL(t *testing.T) {
	h := newTestHandler("")
	body, _ := json.Marshal(map[string]any{"fields": 15})
	req := httptest.NewRequest(http.MethodPost, "/fetch", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	var resp errorResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Code != "bad_request" {
		t.Fatalf("expected code=bad_request, got %q", resp.Code)
	}
}

func TestFetchMalformedJSON(t *testing.T) {
	h := newTestHandler("")
	req := httptest.NewRequest(http.MethodPost, "/fetch", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestFetchInvalidURL(t *testing.T) {
	h := newTestHandler("")
	body, _ := json.Marshal(map[string]any{"url": "ftp://invalid", "fields": 15})
	req := httptest.NewRequest(http.MethodPost, "/fetch", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid URL scheme, got %d", rec.Code)
	}
}

func TestFetchBulkEmptyURLs(t *testing.T) {
	h := newTestHandler("")
	body, _ := json.Marshal(map[string]any{"urls": []string{}, "fields": 15})
	req := httptest.NewRequest(http.MethodPost, "/fetch-bulk", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty urls, got %d", rec.Code)
	}
}

func TestFetchBulkMalformedJSON(t *testing.T) {
	h := newTestHandler("")
	req := httptest.NewRequest(http.MethodPost, "/fetch-bulk", bytes.NewReader([]byte("{bad")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed json, got %d", rec.Code)
	}
}

func TestFetchWithUpstreamServer(t *testing.T) {
	// Uses httptest.NewServer (local loopback) — no external network needed.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>Test Page</title><meta property="og:title" content="OG Title"/></head><body></body></html>`))
	}))
	defer upstream.Close()

	client := metagrab.NewClient()
	h := New(client)

	body, _ := json.Marshal(map[string]any{"url": upstream.URL, "fields": 15})
	req := httptest.NewRequest(http.MethodPost, "/fetch", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var link metagrab.Link
	if err := json.NewDecoder(rec.Body).Decode(&link); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if link.Title != "Test Page" {
		t.Fatalf("expected title 'Test Page', got %q", link.Title)
	}
	if link.OpenGraph["og:title"] != "OG Title" {
		t.Fatalf("expected og:title 'OG Title', got %q", link.OpenGraph["og:title"])
	}
}

func TestFetchBulkWithUpstreamServer(t *testing.T) {
	// Uses httptest.NewServer (local loopback) — no external network needed.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><title>Bulk Test</title></head><body></body></html>`))
	}))
	defer upstream.Close()

	client := metagrab.NewClient()
	h := New(client)

	body, _ := json.Marshal(map[string]any{
		"urls":   []string{upstream.URL, upstream.URL},
		"fields": 15,
	})
	req := httptest.NewRequest(http.MethodPost, "/fetch-bulk", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Response is flat []Link for wire-compatibility with workerLink contract
	var links []metagrab.Link
	if err := json.NewDecoder(rec.Body).Decode(&links); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 results, got %d", len(links))
	}
	for i, l := range links {
		if l.Title != "Bulk Test" {
			t.Fatalf("result[%d] title: got %q, want %q", i, l.Title, "Bulk Test")
		}
		if l.URL == "" {
			t.Fatalf("result[%d] URL is empty", i)
		}
	}
}
