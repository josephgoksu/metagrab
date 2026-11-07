package metagrab

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Micro-benchmarks for the library itself. For the comparison with other
// libraries, see bench/ and docs/benchmarks.

var benchPage = `<html><head>
	<title>Benchmark Page</title>
	<meta name="description" content="A page for benchmarking">
	<meta property="og:title" content="OG Bench">
	<meta property="og:description" content="OG desc">
	<meta property="og:image" content="https://example.com/img.png">
	<meta property="og:url" content="https://example.com">
	<meta name="twitter:card" content="summary_large_image">
	<link rel="icon" href="/favicon.png">
	<link rel="canonical" href="https://example.com/">
	<script type="application/ld+json">{"@type":"Article"}</script>
</head><body>` + strings.Repeat(`<p>Some body content here. <a href="/x">link</a></p>`, 2000) + `</body></html>`

func BenchmarkParse(b *testing.B) {
	for _, tc := range []struct {
		name string
		mask Field
	}{{"metadata", MetadataFields}, {"preview", PreviewFields}, {"rich", RichFields}, {"all", AllFields}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(benchPage)))
			for b.Loop() {
				if _, err := Parse(strings.NewReader(benchPage), "https://example.com/", tc.mask); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkFetch(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, benchPage)
	}))
	defer srv.Close()
	c := NewClient()
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := c.Fetch(ctx, srv.URL, PreviewFields); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFetchBulkResults(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, benchPage)
	}))
	defer srv.Close()
	urls := make([]string, 50)
	for i := range urls {
		urls[i] = fmt.Sprintf("%s/%d", srv.URL, i)
	}
	c := NewClient(WithConcurrency(10))
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		for _, r := range c.FetchBulkResults(ctx, urls, PreviewFields) {
			if r.Error != nil {
				b.Fatal(r.Error)
			}
		}
	}
}
