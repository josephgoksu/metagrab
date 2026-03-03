package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/josephgoksu/metagrab"
)

var fieldPresets = map[string]metagrab.Field{
	"metadata": metagrab.MetadataFields, // 15
	"preview":  metagrab.PreviewFields,  // 111 — adds favicon + canonical
	"rich":     metagrab.RichFields,     // 239 — adds JSON-LD
	"all":      metagrab.AllFields,      // 255 — adds full body content
}

func parseFields(s string) (metagrab.Field, error) {
	if f, ok := fieldPresets[strings.ToLower(s)]; ok {
		return f, nil
	}
	n, err := strconv.ParseUint(s, 10, 8)
	if err != nil {
		names := make([]string, 0, len(fieldPresets))
		for k := range fieldPresets {
			names = append(names, k)
		}
		return 0, fmt.Errorf("unknown fields %q (use: %s, or a number 0-255)", s, strings.Join(names, ", "))
	}
	return metagrab.Field(n), nil
}

func main() {
	timeout := flag.Duration("timeout", 10*time.Second, "HTTP timeout per request")
	fields := flag.String("fields", "preview", "field preset (metadata, preview, rich, all) or numeric bitmask 0-255")
	retries := flag.Int("retries", 0, "retry attempts for retryable statuses (429/502/503/504)")
	retryDelay := flag.Duration("retry-delay", 250*time.Millisecond, "base delay between retries")
	flag.Parse()

	urls := flag.Args()
	if len(urls) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: metagrab [-fields=preview] [-timeout=10s] <url> [url...]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Field presets:")
		fmt.Fprintln(os.Stderr, "  metadata  title, description, open graph, twitter cards")
		fmt.Fprintln(os.Stderr, "  preview   metadata + favicon + canonical URL (default)")
		fmt.Fprintln(os.Stderr, "  rich      preview + JSON-LD structured data")
		fmt.Fprintln(os.Stderr, "  all       rich + full page content")
		os.Exit(1)
	}

	mask, err := parseFields(*fields)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	start := time.Now()
	client := metagrab.NewClient(
		metagrab.WithTimeout(*timeout),
		metagrab.WithRetries(*retries),
		metagrab.WithRetryDelay(*retryDelay),
	)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	if len(urls) == 1 {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout+time.Second)
		defer cancel()
		link, err := client.Fetch(ctx, urls[0], mask)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(2)
		}
		if err := enc.Encode(link); err != nil {
			fmt.Fprintln(os.Stderr, "JSON error:", err)
			os.Exit(3)
		}
	} else {
		results := client.FetchBulkResults(context.Background(), urls, mask)
		if err := enc.Encode(results); err != nil {
			fmt.Fprintln(os.Stderr, "JSON error:", err)
			os.Exit(3)
		}

		for _, r := range results {
			if r.Error != nil {
				fmt.Fprintln(os.Stderr, "One or more URLs failed")
				os.Exit(2)
			}
		}
	}

	fmt.Fprintf(os.Stderr, "Took %s\n", time.Since(start))
}
