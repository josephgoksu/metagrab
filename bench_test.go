package metagrab

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// BenchmarkParse benchmarks HTML parsing without network overhead.
func BenchmarkParse(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head>
			<title>Benchmark Page</title>
			<meta name="description" content="A page for benchmarking">
			<meta property="og:title" content="OG Bench">
			<meta property="og:description" content="OG desc">
			<meta property="og:image" content="https://example.com/img.png">
			<meta property="og:url" content="https://example.com">
			<meta name="twitter:card" content="summary_large_image">
			<meta name="twitter:title" content="Twitter Bench">
		</head><body><p>Some body content here.</p></body></html>`)
	}))
	defer srv.Close()

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Fetch(ctx, srv.URL, MetadataFields)
		if err != nil {
			b.Fatalf("Fetch failed: %v", err)
		}
	}
}

// BenchmarkFetchBulk benchmarks real network fetches — skip in -short mode.
func BenchmarkFetchBulk(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping real network benchmark in -short mode")
	}

	links := []string{
		"https://josephgoksu.com/",
		"https://josephgoksu.com/blog/notes-on-zero-trust-architecture-on-aws",
		"https://github.com/galeone/tfgo",
		"https://markwise.app",
	}

	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		_, err := FetchBulk(ctx, links, MetadataFields)
		if err != nil {
			b.Fatalf("FetchBulk failed: %v", err)
		}
	}
}
