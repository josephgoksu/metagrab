# AGENTS.md

Guidance for coding agents and contributors working in this repository.

## What this is

`metagrab` is a Go library (plus a CLI and an HTTP adapter) that extracts
link-preview metadata from URLs: title, description, Open Graph, Twitter
Cards, favicon, canonical URL, JSON-LD and oEmbed.

## Layout

| Path | What it holds |
| --- | --- |
| `metagrab.go` | `Client`, `Fetch`, bulk fetch, extraction (streaming head tokenizer), charset detection, errors |
| `parse.go` | `Parse` for HTML you already have; shared `parseInto` |
| `jsonld_scan.go` | Fast body scanner for JSON-LD scripts (used by `RichFields`) |
| `policy.go` | SSRF protection: `IsPublicAddr`, `DenyPrivateIPs`, dial-time check |
| `options.go` | Functional options |
| `oembed.go` | oEmbed provider registry and fetch |
| `httphandler/` | `http.Handler` exposing `POST /fetch`, `POST /fetch-bulk`, `GET /health` |
| `cmd/metagrab/` | CLI and `metagrab serve` |
| `bench/` | Separate module: comparison benchmarks and chart generator |
| `docs/benchmarks/` | Generated results and SVG charts (do not edit by hand) |
| `examples/` | Lambda, Cloudflare Container and standalone server examples |

## Commands

```bash
go test -short -race ./...          # unit tests, no network
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
go test -tags=integration ./...     # hits real websites
cd bench && go run .                # regenerate docs/benchmarks (takes a few minutes)
```

CI runs gofmt, vet, race tests on the two supported Go releases, staticcheck,
govulncheck, a GoReleaser snapshot and a Docker build.

## Rules

- The library depends only on `golang.org/x/net`, `golang.org/x/sync` and
  `golang.org/x/text`. Do not add dependencies to the root module. Comparison
  libraries belong in `bench/go.mod`.
- Every mask must give the same metadata: `ContentField` buffers the body
  but runs the same streaming extractor, so do not add a separate DOM-based
  path. Add a test that runs both `ContentField` and non-`ContentField` masks
  when you change extraction.
- Tests must not use the network unless they are behind the `integration`
  build tag. Use `httptest`.
- Every error returned to callers is a `*FetchError` with an `ErrorCode`.
- Anything that fetches URLs must keep working with `WithDenyPrivateNetworks`.
- Public API changes need a `CHANGELOG.md` entry. Removing or changing an
  exported identifier is a breaking change after v1.0.0.
- Commits use Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`).
- Benchmark claims in the README must come from `docs/benchmarks/results.json`.

## Releasing

Push a `vX.Y.Z` tag. `.github/workflows/release.yml` runs GoReleaser
(archives, checksums, Homebrew cask in `josephgoksu/homebrew-tap`), pushes a
multi-arch image to `ghcr.io/josephgoksu/metagrab`, and asks the Go module
proxy to fetch the version so pkg.go.dev lists it. The cask needs the
`HOMEBREW_TAP_TOKEN` repository secret.
