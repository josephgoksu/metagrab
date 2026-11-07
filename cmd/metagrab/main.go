// Command metagrab fetches link-preview metadata from URLs, or serves the
// metagrab HTTP API.
//
//	metagrab [flags] <url> [url...]   print metadata as JSON
//	metagrab serve [flags]            run the HTTP API
//	metagrab version                  print the version
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/josephgoksu/metagrab"
)

// Exit codes.
const (
	exitOK       = 0
	exitUsage    = 1
	exitFetch    = 2
	exitInternal = 3
)

var fieldPresets = map[string]metagrab.Field{
	"metadata": metagrab.MetadataFields, // 15
	"preview":  metagrab.PreviewFields,  // 111: adds favicon + canonical
	"rich":     metagrab.RichFields,     // 239: adds JSON-LD
	"all":      metagrab.AllFields,      // 255: adds full body content
}

func parseFields(s string) (metagrab.Field, error) {
	if f, ok := fieldPresets[strings.ToLower(strings.TrimSpace(s))]; ok {
		return f, nil
	}
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 16)
	if err != nil || n == 0 {
		names := make([]string, 0, len(fieldPresets))
		for k := range fieldPresets {
			names = append(names, k)
		}
		sort.Strings(names)
		return 0, fmt.Errorf("unknown fields %q (use %s, or a numeric bitmask such as 111)", s, strings.Join(names, ", "))
	}
	return metagrab.Field(n), nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "serve":
			return runServe(ctx, args[1:], stderr)
		case "version", "--version", "-version":
			fmt.Fprintln(stdout, versionString())
			return exitOK
		}
	}
	return runFetch(ctx, args, stdout, stderr)
}

func runFetch(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("metagrab", flag.ContinueOnError)
	fs.SetOutput(stderr)
	timeout := fs.Duration("timeout", 10*time.Second, "HTTP timeout per request")
	fields := fs.String("fields", "preview", "field preset (metadata, preview, rich, all) or numeric bitmask")
	retries := fs.Int("retries", 0, "retry attempts for retryable statuses (429/502/503/504)")
	retryDelay := fs.Duration("retry-delay", 250*time.Millisecond, "base delay between retries")
	concurrency := fs.Int("concurrency", 10, "max parallel fetches when several URLs are given")
	compact := fs.Bool("compact", false, "print compact JSON (one line)")
	quiet := fs.Bool("quiet", false, "do not print timing to stderr")
	fs.Usage = func() {
		fmt.Fprintf(stderr, `Usage:
  metagrab [flags] <url> [url...]   print metadata as JSON
  metagrab serve [flags]            run the HTTP API (metagrab serve -h)
  metagrab version                  print the version

Field presets:
  metadata  title, description, Open Graph, Twitter Cards, image
  preview   metadata + favicon + canonical URL (default)
  rich      preview + JSON-LD structured data
  all       rich + full page HTML

Flags:
`)
		fs.PrintDefaults()
		fmt.Fprintf(stderr, `
Exit codes: 0 ok, 1 usage error, 2 one or more URLs failed, 3 internal error.
`)
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	urls := fs.Args()
	if len(urls) == 0 {
		fs.Usage()
		return exitUsage
	}

	mask, err := parseFields(*fields)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitUsage
	}

	start := time.Now()
	client := metagrab.NewClient(
		metagrab.WithTimeout(*timeout),
		metagrab.WithRetries(*retries),
		metagrab.WithRetryDelay(*retryDelay),
		metagrab.WithConcurrency(*concurrency),
	)

	enc := json.NewEncoder(stdout)
	if !*compact {
		enc.SetIndent("", "  ")
	}

	code := exitOK
	if len(urls) == 1 {
		link, err := client.Fetch(ctx, urls[0], mask)
		if err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return exitFetch
		}
		if err := enc.Encode(link); err != nil {
			fmt.Fprintln(stderr, "error: encode JSON:", err)
			return exitInternal
		}
	} else {
		results := client.FetchBulkResults(ctx, urls, mask)
		if err := enc.Encode(results); err != nil {
			fmt.Fprintln(stderr, "error: encode JSON:", err)
			return exitInternal
		}
		failed := 0
		for _, r := range results {
			if r.Error != nil {
				failed++
			}
		}
		if failed > 0 {
			fmt.Fprintf(stderr, "%d of %d URLs failed\n", failed, len(results))
			code = exitFetch
		}
	}

	if !*quiet {
		fmt.Fprintf(stderr, "took %s\n", time.Since(start).Round(time.Millisecond))
	}
	return code
}
