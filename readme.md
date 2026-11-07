<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/banner-dark.svg">
  <img src="assets/banner-light.svg" alt="metagrab: link previews for Go" width="820">
</picture>

[![CI](https://github.com/josephgoksu/metagrab/actions/workflows/ci.yml/badge.svg)](https://github.com/josephgoksu/metagrab/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/josephgoksu/metagrab.svg)](https://pkg.go.dev/github.com/josephgoksu/metagrab)
[![Go Report Card](https://goreportcard.com/badge/github.com/josephgoksu/metagrab)](https://goreportcard.com/report/github.com/josephgoksu/metagrab)
[![Release](https://img.shields.io/github/v/release/josephgoksu/metagrab?sort=semver)](https://github.com/josephgoksu/metagrab/releases)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

metagrab turns a URL into a link preview: title, description, image, favicon, canonical URL, author, date, Open Graph, Twitter Cards, JSON-LD and oEmbed. Use it as a Go library, a CLI, or a ready-made HTTP API. **[metagrab.josephgoksu.com](https://metagrab.josephgoksu.com)**

- **Fast.** It stops reading at `</head>` when it can. On a 1.8 MB product page it reads 445 KB and returns in 1.84 ms, 3.9x to 10.8x faster than the other Go libraries we tested. [Benchmarks](#benchmarks)
- **Safe to expose.** `WithDenyPrivateNetworks()` blocks private and cloud-metadata addresses on every connection, including redirects and DNS rebinding. Retries, timeouts and body limits are built in.
- **Complete.** One call gives you what a preview card needs, with absolute URLs, the best favicon, charset decoding, and oEmbed for 23 providers.
- **Small.** Only `golang.org/x` dependencies.

## Install

```bash
go get github.com/josephgoksu/metagrab                     # library
brew install --cask josephgoksu/tap/metagrab              # CLI (macOS, Linux)
go install github.com/josephgoksu/metagrab/cmd/metagrab@latest
docker run --rm -p 8080:8080 ghcr.io/josephgoksu/metagrab  # HTTP API
```

Requires Go 1.26 or newer.

## Quick start

```go
link, err := metagrab.Fetch(ctx, "https://go.dev", metagrab.PreviewFields)
if err != nil {
    log.Fatal(err) // *metagrab.FetchError with a machine-readable Code
}
fmt.Println(link.Title, link.Description)
fmt.Println(link.Image)   // absolute URL, ready for an <img>
fmt.Println(link.Favicon) // best icon, falls back to /favicon.ico
```

Already have the HTML? `metagrab.Parse(r, pageURL, mask)` extracts without a request. Add JSON-LD with `metagrab.RichFields`. For URLs from users, create a client with `metagrab.WithDenyPrivateNetworks()`.

## Benchmarks

Time to fetch one link preview from a local server (median, lower is better). Full method, raw numbers and memory charts: [docs/benchmarks](docs/benchmarks/README.md).

![Fetch time per link preview](docs/benchmarks/fetch-time.svg)

![Bytes read per link preview](docs/benchmarks/bytes-read.svg)

| Page | metagrab | metagrab (rich) | Fastest other | Slowest other |
| --- | ---: | ---: | ---: | ---: |
| Small blog post (38 KB) | **280 µs** | 312 µs | 489 µs (badoux/goscraper) | 909 µs (goquery (hand-rolled)) |
| Typical page (154 KB) | **627 µs** | 550 µs | 1.11 ms (badoux/goscraper) | 2.42 ms (goquery (hand-rolled)) |
| News article (699 KB) | **1.12 ms** | 1.36 ms | 3.28 ms (badoux/goscraper) | 7.60 ms (goquery (hand-rolled)) |
| Shop product page (1.8 MB) | **1.84 ms** | 2.22 ms | 7.09 ms (badoux/goscraper) | 19.84 ms (goquery (hand-rolled)) |

`metagrab (rich)` adds JSON-LD, so it scans the whole body. The libraries do not extract the same fields; see the [feature comparison](docs/benchmarks/README.md#features). Reproduce with `cd bench && go run .`

## CLI and HTTP API

```bash
metagrab https://go.dev                         # JSON link preview
metagrab -fields=rich https://a.com https://b.com
metagrab serve -api-key "$METAGRAB_API_KEY"     # POST /fetch, POST /fetch-bulk, GET /health
```

`serve` blocks private networks by default. Same API in Docker: `docker run -p 8080:8080 ghcr.io/josephgoksu/metagrab`.

## Documentation

- [Reference](docs/reference.md): field masks, `Link` fields, client options, error codes, SSRF protection, oEmbed providers, HTTP API, CLI flags, production checklist
- [pkg.go.dev](https://pkg.go.dev/github.com/josephgoksu/metagrab): API docs and runnable examples
- [Website](https://metagrab.josephgoksu.com): how it works, with an interactive demo
- [Benchmarks](docs/benchmarks/README.md): method, raw numbers, memory charts, feature comparison
- [Architecture](ARCHITECTURE.md) and [Changelog](CHANGELOG.md)

## Contributing

Issues and pull requests are welcome; see [CONTRIBUTING.md](CONTRIBUTING.md). Report security issues privately ([SECURITY.md](SECURITY.md)).

```bash
go test -short -race ./...
```

metagrab follows [semantic versioning](https://semver.org). Licensed under [Apache 2.0](LICENSE).
