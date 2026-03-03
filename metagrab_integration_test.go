//go:build integration
// +build integration

package metagrab

import (
	"context"
	"testing"
	"time"
)

func TestFetchRealLinksIntegration(t *testing.T) {
	links := []string{
		"https://josephgoksu.com/",
		"https://github.com/galeone/tfgo",
		"https://markwise.app",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, rawURL := range links {
		t.Run(rawURL, func(t *testing.T) {
			link, err := Fetch(ctx, rawURL, AllFields)
			if err != nil {
				t.Fatalf("Fetch(%s): %v", rawURL, err)
			}
			if link.Title == "" {
				t.Errorf("Title is empty for %s", rawURL)
			}
			t.Logf("URL=%s Title=%q Desc=%q OG=%d Twitter=%d", link.URL, link.Title, link.Description, len(link.OpenGraph), len(link.Twitter))
		})
	}
}
