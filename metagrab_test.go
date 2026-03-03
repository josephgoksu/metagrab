package metagrab

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBasicHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<title>Test Page</title>
			<meta name="description" content="A test page">
			<meta property="og:title" content="OG Title">
			<meta property="og:image" content="https://example.com/img.png">
			<meta name="twitter:card" content="summary">
		</head><body>Hello</body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, AllFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if link.Title != "Test Page" {
		t.Errorf("Title = %q, want %q", link.Title, "Test Page")
	}
	if link.Description != "A test page" {
		t.Errorf("Description = %q, want %q", link.Description, "A test page")
	}
	if link.OpenGraph["og:title"] != "OG Title" {
		t.Errorf("og:title = %q, want %q", link.OpenGraph["og:title"], "OG Title")
	}
	if link.Twitter["twitter:card"] != "summary" {
		t.Errorf("twitter:card = %q, want %q", link.Twitter["twitter:card"], "summary")
	}
	if !strings.Contains(link.Content, "Hello") {
		t.Errorf("Content should contain body text, got %q", link.Content)
	}
}

func TestReversedMetaAttributes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<meta content="Reversed Desc" name="description">
			<meta content="Reversed OG" property="og:title">
		</head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, AllFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.Description != "Reversed Desc" {
		t.Errorf("Description = %q, want %q", link.Description, "Reversed Desc")
	}
	if link.OpenGraph["og:title"] != "Reversed OG" {
		t.Errorf("og:title = %q, want %q", link.OpenGraph["og:title"], "Reversed OG")
	}
}

func TestTitleFallbackToOGWithoutMetaField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><meta property="og:title" content="OG fallback"></head></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, TitleField)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.Title != "OG fallback" {
		t.Errorf("Title = %q, want %q", link.Title, "OG fallback")
	}
	if link.OpenGraph != nil {
		t.Fatalf("OpenGraph should be nil when MetaField is not requested")
	}
}

func TestDescriptionFallbackToOGWithoutMetaField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><meta property="og:description" content="OG description"></head></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, DescriptionField)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.Description != "OG description" {
		t.Errorf("Description = %q, want %q", link.Description, "OG description")
	}
}

func TestMissingTitleFallsBackToHostname(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head></head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, TitleField)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.Title != "127.0.0.1" {
		t.Errorf("Title = %q, want hostname fallback", link.Title)
	}
}

func TestDefaultMaskUsesAllFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>Mask default</title><meta property="og:title" content="OG"></head></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, 0)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.Title == "" {
		t.Fatal("expected title when mask=0")
	}
	if link.OpenGraph == nil || link.OpenGraph["og:title"] != "OG" {
		t.Fatal("expected OpenGraph when mask=0")
	}
}

func TestMetadataFieldsExcludeContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>Meta Only</title></head><body>body text</body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, MetadataFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.Title != "Meta Only" {
		t.Fatalf("title = %q, want %q", link.Title, "Meta Only")
	}
	if link.Content != "" {
		t.Fatalf("content should be empty in MetadataFields mode, got len=%d", len(link.Content))
	}
}

func TestMetadataFieldsHeadParserExtractsMetaTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<meta property="og:title" content="OG Fast Path">
			<meta property="og:description" content="OG Desc">
			<meta name="twitter:card" content="summary_large_image">
		</head><body>ignored</body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, MetadataFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.Title != "OG Fast Path" {
		t.Fatalf("Title = %q, want %q", link.Title, "OG Fast Path")
	}
	if link.Description != "OG Desc" {
		t.Fatalf("Description = %q, want %q", link.Description, "OG Desc")
	}
	if link.OpenGraph["og:title"] != "OG Fast Path" {
		t.Fatalf("og:title mismatch")
	}
	if link.Twitter["twitter:card"] != "summary_large_image" {
		t.Fatalf("twitter:card mismatch")
	}
}

func TestURLFieldSkipsNetwork(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		fmt.Fprint(w, `<html><head><title>Should not be fetched</title></head></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, URLField)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.URL != srv.URL {
		t.Errorf("URL = %q, want %q", link.URL, srv.URL)
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("expected no network call for URLField, got %d", got)
	}
}

func TestInvalidURLError(t *testing.T) {
	_, err := Fetch(context.Background(), "ftp://example.com", AllFields)
	if err == nil {
		t.Fatal("expected error")
	}
	assertFetchErrorCode(t, err, ErrorCodeInvalidURL)
}

func TestURLWithUserInfoRejected(t *testing.T) {
	_, err := Fetch(context.Background(), "https://user:pass@example.com", URLField)
	if err == nil {
		t.Fatal("expected error")
	}
	assertFetchErrorCode(t, err, ErrorCodeInvalidURL)
}

func TestTooLongURLRejected(t *testing.T) {
	tooLong := "https://example.com/" + strings.Repeat("a", 2050)
	_, err := Fetch(context.Background(), tooLong, URLField)
	if err == nil {
		t.Fatal("expected error")
	}
	assertFetchErrorCode(t, err, ErrorCodeInvalidURL)
}

func TestNon2xxStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL, AllFields)
	if err == nil {
		t.Fatal("expected error")
	}

	ferr := assertFetchErrorCode(t, err, ErrorCodeHTTPStatus)
	if ferr.StatusCode != http.StatusForbidden {
		t.Fatalf("StatusCode = %d, want %d", ferr.StatusCode, http.StatusForbidden)
	}
}

func TestUnsupportedContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL, AllFields)
	if err == nil {
		t.Fatal("expected unsupported content type error")
	}
	assertFetchErrorCode(t, err, ErrorCodeContentType)
}

func TestRetry429ThenSuccess(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `<html><head><title>Recovered</title></head></html>`)
	}))
	defer srv.Close()

	client := NewClient(WithRetries(1), WithRetryDelay(1*time.Millisecond))
	link, err := client.Fetch(context.Background(), srv.URL, TitleField)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.Title != "Recovered" {
		t.Fatalf("Title = %q, want %q", link.Title, "Recovered")
	}
	if hits.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", hits.Load())
	}
}

func TestNoRetryOn403(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := NewClient(WithRetries(3), WithRetryDelay(1*time.Millisecond))
	_, err := client.Fetch(context.Background(), srv.URL, TitleField)
	if err == nil {
		t.Fatal("expected error")
	}
	assertFetchErrorCode(t, err, ErrorCodeHTTPStatus)
	if hits.Load() != 1 {
		t.Fatalf("expected single attempt for 403, got %d", hits.Load())
	}
}

func TestURLPolicyDenied(t *testing.T) {
	policy := func(u *url.URL) error {
		if strings.Contains(u.Host, "example.com") {
			return errors.New("host is blocked")
		}
		return nil
	}
	client := NewClient(WithURLPolicy(policy))

	_, err := client.Fetch(context.Background(), "https://example.com", URLField)
	if err == nil {
		t.Fatal("expected policy error")
	}
	assertFetchErrorCode(t, err, ErrorCodeURLDenied)
}

func TestCanceledContextReturnsStructuredError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>never reached</title></head></html>`)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Fetch(ctx, srv.URL, AllFields)
	if err == nil {
		t.Fatal("expected canceled error")
	}

	ferr := assertFetchErrorCode(t, err, ErrorCodeCanceled)
	if !errors.Is(ferr, context.Canceled) {
		t.Fatalf("expected wrapped context.Canceled, got %v", ferr)
	}
}

func TestFetchBulkResultsPartialSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>OK</title></head></html>`)
	}))
	defer srv.Close()

	urls := []string{srv.URL, "ftp://bad"}
	results := FetchBulkResults(context.Background(), urls, TitleField)
	if len(results) != 2 {
		t.Fatalf("results length = %d, want 2", len(results))
	}

	if results[0].Error != nil || results[0].Link == nil {
		t.Fatalf("expected success for first result, got %+v", results[0])
	}
	if results[0].Link.Title != "OK" {
		t.Fatalf("title = %q, want %q", results[0].Link.Title, "OK")
	}
	if results[1].Error == nil {
		t.Fatal("expected second result to fail")
	}
	if results[1].Error.Code != ErrorCodeInvalidURL {
		t.Fatalf("code = %q, want %q", results[1].Error.Code, ErrorCodeInvalidURL)
	}
}

func TestFetchBulkStrictFailsWhenAnyFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>OK</title></head></html>`)
	}))
	defer srv.Close()

	urls := []string{srv.URL, "ftp://bad"}
	_, err := FetchBulk(context.Background(), urls, TitleField)
	if err == nil {
		t.Fatal("expected strict bulk failure")
	}
	assertFetchErrorCode(t, err, ErrorCodeInvalidURL)
}

func TestFetchBulkStrictFailFastAvoidsLaterNetworkCalls(t *testing.T) {
	var hits atomic.Int32
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(200 * time.Millisecond)
		fmt.Fprint(w, `<html><head><title>slow</title></head></html>`)
	}))
	defer slow.Close()

	client := NewClient(WithConcurrency(1))
	urls := []string{"ftp://bad-url", slow.URL}

	start := time.Now()
	_, err := client.FetchBulk(context.Background(), urls, TitleField)
	if err == nil {
		t.Fatal("expected strict bulk failure")
	}
	assertFetchErrorCode(t, err, ErrorCodeInvalidURL)
	if hits.Load() != 0 {
		t.Fatalf("expected no request to later URL, got %d hits", hits.Load())
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("strict fail-fast took too long: %s", elapsed)
	}
}

func TestBoundedConcurrency(t *testing.T) {
	var active atomic.Int32
	var maxSeen atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := active.Add(1)
		for {
			old := maxSeen.Load()
			if cur <= old || maxSeen.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		active.Add(-1)
		fmt.Fprint(w, `<html><head><title>OK</title></head></html>`)
	}))
	defer srv.Close()

	limit := 3
	client := NewClient(WithConcurrency(limit))
	urls := make([]string, 20)
	for i := range urls {
		urls[i] = srv.URL
	}

	results := client.FetchBulkResults(context.Background(), urls, TitleField)
	for i, r := range results {
		if r.Error != nil {
			t.Fatalf("result[%d] failed: %v", i, r.Error)
		}
	}

	if peak := maxSeen.Load(); peak > int32(limit) {
		t.Errorf("max concurrent = %d, want <= %d", peak, limit)
	}
}

func TestBodySizeLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>Big Page</title></head><body>`)
		_, _ = w.Write(make([]byte, 512*1024))
		fmt.Fprint(w, `</body></html>`)
	}))
	defer srv.Close()

	client := NewClient(WithMaxBodySize(1024))
	_, err := client.Fetch(context.Background(), srv.URL, AllFields)
	if err != nil {
		assertFetchErrorCode(t, err, ErrorCodeBodyTooLarge)
		return
	}
	t.Fatal("expected body_too_large error")
}

func TestLargeBodyMetadataModeSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>Large Page</title></head><body>`)
		_, _ = w.Write(make([]byte, 512*1024))
		fmt.Fprint(w, `</body></html>`)
	}))
	defer srv.Close()

	client := NewClient(WithMaxBodySize(1024))
	link, err := client.Fetch(context.Background(), srv.URL, MetadataFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.Title != "Large Page" {
		t.Errorf("Title = %q, want %q", link.Title, "Large Page")
	}
	if link.Content != "" {
		t.Errorf("Content should be empty in MetadataFields mode")
	}
}

func TestNewClientOptions(t *testing.T) {
	client := NewClient(
		WithTimeout(5*time.Second),
		WithUserAgent("test-agent/1.0"),
		WithMaxBodySize(1024),
		WithConcurrency(2),
		WithURLPolicy(func(*url.URL) error { return nil }),
	)

	if client.userAgent != "test-agent/1.0" {
		t.Errorf("userAgent = %q", client.userAgent)
	}
	if client.maxBodySize != 1024 {
		t.Errorf("maxBodySize = %d", client.maxBodySize)
	}
	if client.concurrency != 2 {
		t.Errorf("concurrency = %d", client.concurrency)
	}
	if client.urlPolicy == nil {
		t.Fatal("urlPolicy should not be nil")
	}
	if client.httpClient == nil {
		t.Fatal("httpClient should not be nil")
	}
}

func TestUserAgentSent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		fmt.Fprint(w, `<html><head><title>UA Test</title></head></html>`)
	}))
	defer srv.Close()

	ua := "custom-bot/3.0"
	client := NewClient(WithUserAgent(ua))
	_, err := client.Fetch(context.Background(), srv.URL, TitleField)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotUA != ua {
		t.Errorf("User-Agent = %q, want %q", gotUA, ua)
	}
}

func assertFetchErrorCode(t *testing.T, err error, code ErrorCode) *FetchError {
	t.Helper()
	var ferr *FetchError
	if !errors.As(err, &ferr) {
		t.Fatalf("expected *FetchError, got %T (%v)", err, err)
	}
	if ferr.Code != code {
		t.Fatalf("code = %q, want %q", ferr.Code, code)
	}
	return ferr
}

// ---------- Step 1: Favicon + Canonical URL ----------

func TestFaviconExtraction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<link rel="shortcut icon" href="/shortcut.ico">
			<link rel="icon" href="/icon.png" sizes="32x32">
			<link rel="apple-touch-icon" href="/apple.png" sizes="180x180">
		</head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, PreviewFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// apple-touch-icon has highest priority
	want := srv.URL + "/apple.png"
	if link.Favicon != want {
		t.Errorf("Favicon = %q, want %q", link.Favicon, want)
	}
}

func TestFaviconRelativeURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<link rel="icon" href="assets/fav.png">
		</head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, PreviewFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	want := srv.URL + "/assets/fav.png"
	if link.Favicon != want {
		t.Errorf("Favicon = %q, want %q", link.Favicon, want)
	}
}

func TestFaviconFallbackToRoot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>No Favicon Link</title></head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, PreviewFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	want := srv.URL + "/favicon.ico"
	if link.Favicon != want {
		t.Errorf("Favicon = %q, want %q", link.Favicon, want)
	}
}

func TestCanonicalURLExtraction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<link rel="canonical" href="https://example.com/canonical-page">
		</head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, PreviewFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	want := "https://example.com/canonical-page"
	if link.CanonicalURL != want {
		t.Errorf("CanonicalURL = %q, want %q", link.CanonicalURL, want)
	}
}

func TestCanonicalURLRelativeResolution(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<link rel="canonical" href="/page">
		</head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, PreviewFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	want := srv.URL + "/page"
	if link.CanonicalURL != want {
		t.Errorf("CanonicalURL = %q, want %q", link.CanonicalURL, want)
	}
}

func TestMetadataFieldsExcludeNewFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<title>Meta Only</title>
			<link rel="icon" href="/icon.png">
			<link rel="canonical" href="https://example.com/canon">
			<script type="application/ld+json">{"@type":"Article"}</script>
		</head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, MetadataFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.Favicon != "" {
		t.Errorf("Favicon should be empty with MetadataFields, got %q", link.Favicon)
	}
	if link.CanonicalURL != "" {
		t.Errorf("CanonicalURL should be empty with MetadataFields, got %q", link.CanonicalURL)
	}
	if link.JSONLD != nil {
		t.Errorf("JSONLD should be nil with MetadataFields, got %v", link.JSONLD)
	}
	if link.Title != "Meta Only" {
		t.Errorf("Title = %q, want %q", link.Title, "Meta Only")
	}
}

// ---------- Step 2: Elevated Fields ----------

func TestElevatedFieldsFromOG(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<meta property="og:site_name" content="My Site">
			<meta property="article:author" content="Jane Doe">
			<meta property="article:published_time" content="2025-01-15T10:00:00Z">
		</head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, MetadataFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.SiteName != "My Site" {
		t.Errorf("SiteName = %q, want %q", link.SiteName, "My Site")
	}
	if link.Author != "Jane Doe" {
		t.Errorf("Author = %q, want %q", link.Author, "Jane Doe")
	}
	if link.PublishedAt != "2025-01-15T10:00:00Z" {
		t.Errorf("PublishedAt = %q, want %q", link.PublishedAt, "2025-01-15T10:00:00Z")
	}
}

func TestAuthorFromMetaName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<meta name="author" content="John Smith">
		</head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, MetadataFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.Author != "John Smith" {
		t.Errorf("Author = %q, want %q", link.Author, "John Smith")
	}
}

func TestOGAuthorPrecedenceOverMetaName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<meta property="article:author" content="OG Author">
			<meta name="author" content="Meta Author">
		</head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, MetadataFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.Author != "OG Author" {
		t.Errorf("Author = %q, want %q (article:author should take precedence)", link.Author, "OG Author")
	}
}

func TestElevatedFieldsRequireMetaField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<meta property="og:site_name" content="My Site">
			<meta property="article:author" content="Jane Doe">
			<meta name="author" content="Fallback">
		</head><body></body></html>`)
	}))
	defer srv.Close()

	// Only request Title — no MetaField
	link, err := Fetch(context.Background(), srv.URL, TitleField)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.SiteName != "" {
		t.Errorf("SiteName should be empty without MetaField, got %q", link.SiteName)
	}
	if link.Author != "" {
		t.Errorf("Author should be empty without MetaField, got %q", link.Author)
	}
}

// ---------- Step 3: JSON-LD ----------

func TestJSONLDExtraction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<script type="application/ld+json">
			{"@type": "Article", "@context": "https://schema.org", "name": "Test Article"}
			</script>
		</head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, MetadataFields|JSONLDField)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(link.JSONLD) != 1 {
		t.Fatalf("JSONLD length = %d, want 1", len(link.JSONLD))
	}
	if link.JSONLD[0]["@type"] != "Article" {
		t.Errorf("JSONLD @type = %v, want Article", link.JSONLD[0]["@type"])
	}
	if link.ContentType != "Article" {
		t.Errorf("ContentType = %q, want %q", link.ContentType, "Article")
	}
}

func TestJSONLDNotExtractedWithoutFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<script type="application/ld+json">
			{"@type": "Article"}
			</script>
		</head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, MetadataFields)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if link.JSONLD != nil {
		t.Errorf("JSONLD should be nil without JSONLDField, got %v", link.JSONLD)
	}
	if link.ContentType != "" {
		t.Errorf("ContentType should be empty without JSONLDField, got %q", link.ContentType)
	}
}

func TestMultipleJSONLDBlocks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<script type="application/ld+json">{"@type": "Article"}</script>
			<script type="application/ld+json">{"@type": "BreadcrumbList"}</script>
		</head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, MetadataFields|JSONLDField)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(link.JSONLD) != 2 {
		t.Fatalf("JSONLD length = %d, want 2", len(link.JSONLD))
	}
	if link.ContentType != "Article" {
		t.Errorf("ContentType = %q, want %q (first block wins)", link.ContentType, "Article")
	}
}

func TestMalformedJSONLDIgnored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<script type="application/ld+json">{invalid json!!!}</script>
			<script type="application/ld+json">{"@type": "Product"}</script>
		</head><body></body></html>`)
	}))
	defer srv.Close()

	link, err := Fetch(context.Background(), srv.URL, MetadataFields|JSONLDField)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(link.JSONLD) != 1 {
		t.Fatalf("JSONLD length = %d, want 1 (malformed block should be skipped)", len(link.JSONLD))
	}
	if link.ContentType != "Product" {
		t.Errorf("ContentType = %q, want %q", link.ContentType, "Product")
	}
}
