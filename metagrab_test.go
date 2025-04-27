package metagrab

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestFetchRealLinksWithTiming(t *testing.T) {
	links := []string{
		"https://josephgoksu.com/",
		"https://josephgoksu.com/blog/notes-on-zero-trust-architecture-on-aws",
		"https://github.com/galeone/tfgo",
		"https://markwise.app",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	type result struct {
		link   *Link
		status int
		dur    time.Duration
		err    error
	}

	results := make([]result, len(links))

	for i, url := range links {
		start := time.Now()
		l, status, err := fetchWithStatus(ctx, url, AllFields)
		results[i] = result{
			link:   l,
			status: status,
			dur:    time.Since(start),
			err:    err,
		}
	}

	var total time.Duration
	for i, r := range results {
		if r.err != nil {
			t.Errorf("[%d] failed: %v", i, r.err)
			continue
		}
		fmt.Printf("\n--- Link %d ---\n", i+1)
		fmt.Printf("URL: %s\n", r.link.URL)
		fmt.Printf("Status: %d\n", r.status)
		fmt.Printf("Duration: %s\n", r.dur)
		fmt.Printf("Title: %s\n", r.link.Title)
		fmt.Printf("Description: %s\n", r.link.Description)
		fmt.Printf("Meta tags: %d\n", len(r.link.Meta))
		fmt.Printf("Content size: %d bytes\n", len(r.link.Content))
		total += r.dur
	}

	fmt.Printf("\nTotal time: %s\n", total)
	fmt.Printf("Average per link: %s\n", total/time.Duration(len(links)))
}

// New helper to get status code
func fetchWithStatus(ctx context.Context, url string, mask Field) (*Link, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	html := string(body)
	l := &Link{URL: url}
	if mask.Has(ContentField) {
		l.Content = html
	}
	if mask.Has(TitleField) {
		l.Title = first(html, `<title.*?>([^<]+)`)
	}
	if mask.Has(DescriptionField) {
		l.Description = first(html, `<meta\s+name=["']description["']\s+content=["']([^"']+)`)
	}
	if mask.Has(MetaField) {
		l.Meta = make(map[string]string)
		kvs := all(html, `<meta\s+(?:property|name)=["']([^"']+)["']\s+content=["']([^"']+)`)
		for i := 0; i < len(kvs); i += 2 {
			l.Meta[kvs[i]] = kvs[i+1]
		}
	}

	return l, resp.StatusCode, nil
}
