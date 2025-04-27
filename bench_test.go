package metagrab

import (
	"context"
	"testing"
)

func BenchmarkFetchBulk(b *testing.B) {
	links := []string{
		"https://josephgoksu.com/",
		"https://josephgoksu.com/blog/notes-on-zero-trust-architecture-on-aws",
		"https://github.com/galeone/tfgo",
		"https://markwise.app",
	}

	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		_, err := FetchBulk(ctx, links, AllFields)
		if err != nil {
			b.Fatalf("FetchBulk failed: %v", err)
		}
	}
}
