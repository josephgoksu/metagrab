package metagrab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestOEmbedProviderMatch(t *testing.T) {
	tests := []struct {
		rawURL   string
		wantName string
	}{
		{"https://www.youtube.com/watch?v=abc", "YouTube"},
		{"https://youtu.be/abc", "YouTube"},
		{"https://vimeo.com/12345", "Vimeo"},
		{"https://open.spotify.com/track/abc", "Spotify"},
		{"https://codepen.io/user/pen/abc", "CodePen"},
		{"https://twitter.com/user/status/123", "Twitter/X"},
		{"https://x.com/user/status/123", "Twitter/X"},
		{"https://www.figma.com/file/abc", "Figma"},
		{"https://unknown-site.example.com/page", ""},
	}

	for _, tt := range tests {
		u, err := url.Parse(tt.rawURL)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", tt.rawURL, err)
		}
		got := matchOEmbedProvider(u)
		if tt.wantName == "" {
			if got != nil {
				t.Errorf("matchOEmbedProvider(%q) = %q, want nil", tt.rawURL, got.name)
			}
			continue
		}
		if got == nil {
			t.Errorf("matchOEmbedProvider(%q) = nil, want %q", tt.rawURL, tt.wantName)
			continue
		}
		if got.name != tt.wantName {
			t.Errorf("matchOEmbedProvider(%q).name = %q, want %q", tt.rawURL, got.name, tt.wantName)
		}
	}
}

func TestOEmbedNotFetchedWithoutMetaField(t *testing.T) {
	// Temporarily add a test provider so we can verify oEmbed is skipped.
	origRegistry := make([]oembedProvider, len(oembedRegistry))
	copy(origRegistry, oembedRegistry)
	defer func() { oembedRegistry = origRegistry }()

	oembedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("oEmbed endpoint should not be called without MetaField")
	}))
	defer oembedSrv.Close()

	pageSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>Test</title></head><body></body></html>`)
	}))
	defer pageSrv.Close()

	// Register the page server as a known provider so it would trigger oEmbed if MetaField were set.
	pageURL, _ := url.Parse(pageSrv.URL)
	oembedRegistry = append(oembedRegistry, oembedProvider{
		name:     "TestProvider",
		hosts:    []string{pageURL.Hostname()},
		endpoint: oembedSrv.URL + "?url={url}",
	})

	// Only request TitleField — no MetaField. oEmbed must NOT be fetched.
	link, err := Fetch(context.Background(), pageSrv.URL, TitleField)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.OEmbed != nil {
		t.Error("OEmbed should be nil without MetaField")
	}
}

func TestOEmbedUnknownProviderSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>Unknown Site</title></head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, MetadataFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.OEmbed != nil {
		t.Error("OEmbed should be nil for unknown provider")
	}
}

func TestOEmbedDataPopulated(t *testing.T) {
	// Fake oEmbed endpoint
	oembedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := OEmbedData{
			Type:         "video",
			Title:        "Test Video",
			AuthorName:   "Test Author",
			ProviderName: "TestProvider",
			ThumbnailURL: "https://example.com/thumb.jpg",
			Width:        1920,
			Height:       1080,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	}))
	defer oembedSrv.Close()

	// Temporarily add a test provider that matches our test server.
	// We save and restore the registry to avoid affecting other tests.
	origRegistry := make([]oembedProvider, len(oembedRegistry))
	copy(origRegistry, oembedRegistry)
	defer func() { oembedRegistry = origRegistry }()

	pageSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>Page</title></head><body></body></html>`)
	}))
	defer pageSrv.Close()

	// Parse the page server URL to get its host.
	pageURL, _ := url.Parse(pageSrv.URL)

	oembedRegistry = append(oembedRegistry, oembedProvider{
		name:     "TestProvider",
		hosts:    []string{pageURL.Hostname()},
		endpoint: oembedSrv.URL + "?url={url}",
	})

	link, err := Fetch(context.Background(), pageSrv.URL, MetadataFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.OEmbed == nil {
		t.Fatal("OEmbed should be populated")
	}
	if link.OEmbed.Type != "video" {
		t.Errorf("OEmbed.Type = %q, want %q", link.OEmbed.Type, "video")
	}
	if link.OEmbed.Title != "Test Video" {
		t.Errorf("OEmbed.Title = %q, want %q", link.OEmbed.Title, "Test Video")
	}
	if link.OEmbed.AuthorName != "Test Author" {
		t.Errorf("OEmbed.AuthorName = %q, want %q", link.OEmbed.AuthorName, "Test Author")
	}
	if link.OEmbed.ThumbnailURL != "https://example.com/thumb.jpg" {
		t.Errorf("OEmbed.ThumbnailURL = %q", link.OEmbed.ThumbnailURL)
	}
}

func TestOEmbedDoesNotPolluteOpenGraph(t *testing.T) {
	oembedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := OEmbedData{
			Type:         "video",
			Title:        "oEmbed Title",
			AuthorName:   "Channel Name",
			ProviderName: "TestTube",
			ThumbnailURL: "https://example.com/thumb.jpg",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	}))
	defer oembedSrv.Close()

	origRegistry := make([]oembedProvider, len(oembedRegistry))
	copy(origRegistry, oembedRegistry)
	defer func() { oembedRegistry = origRegistry }()

	pageSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>Page Title</title></head><body></body></html>`)
	}))
	defer pageSrv.Close()

	pageURL, _ := url.Parse(pageSrv.URL)
	oembedRegistry = append(oembedRegistry, oembedProvider{
		name:     "TestProvider",
		hosts:    []string{pageURL.Hostname()},
		endpoint: oembedSrv.URL + "?url={url}",
	})

	link, err := Fetch(context.Background(), pageSrv.URL, MetadataFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// Title not overwritten — page already has one.
	if link.Title != "Page Title" {
		t.Errorf("Title = %q, want %q", link.Title, "Page Title")
	}
	// SiteName from the oEmbed provider, Author from the oEmbed author.
	if link.SiteName != "TestTube" || link.Author != "Channel Name" {
		t.Errorf("SiteName = %q, Author = %q; want TestTube, Channel Name", link.SiteName, link.Author)
	}
	if link.Image != "https://example.com/thumb.jpg" {
		t.Errorf("Image = %q, want the oEmbed thumbnail", link.Image)
	}
	// oEmbed data accessible via its own field.
	if link.OEmbed.ThumbnailURL != "https://example.com/thumb.jpg" {
		t.Errorf("OEmbed.ThumbnailURL = %q", link.OEmbed.ThumbnailURL)
	}
	// OpenGraph map must NOT contain synthetic entries from oEmbed.
	if link.OpenGraph["og:title"] != "" {
		t.Errorf("og:title should be empty (oEmbed must not pollute OpenGraph), got %q", link.OpenGraph["og:title"])
	}
	if link.OpenGraph["og:image"] != "" {
		t.Errorf("og:image should be empty (oEmbed must not pollute OpenGraph), got %q", link.OpenGraph["og:image"])
	}
	if link.OpenGraph["og:site_name"] != "" {
		t.Errorf("og:site_name should be empty (oEmbed must not pollute OpenGraph), got %q", link.OpenGraph["og:site_name"])
	}
}

func TestOEmbedYouTubeBackwardCompat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	// This test verifies that YouTube URLs still get oEmbed data.
	// It requires network access.
	link, err := Fetch(context.Background(), "https://www.youtube.com/watch?v=dQw4w9WgXcQ", MetadataFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.Title == "" {
		t.Error("expected non-empty title for YouTube video")
	}
	if link.OEmbed == nil {
		t.Error("expected OEmbed data for YouTube video")
	}
}
