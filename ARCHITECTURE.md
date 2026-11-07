# Architecture

## Scope of this repository

`metagrab` is a Go library.

In-scope:
- Metadata fetching/parsing primitives (title, description, OG, Twitter Cards, favicon, canonical URL, author/date, JSON-LD, oEmbed)
- Opt-in field bitmask for controlling extraction scope (`uint16`, 8 bits used)
- oEmbed provider registry with 23 compiled-in providers
- Public Go API (`Fetch`, `Parse`, `FetchBulkResults`, deprecated `FetchBulk`)
- SSRF protection (`WithDenyPrivateNetworks`, `DenyPrivateIPs`, `IsPublicAddr`)
- `httphandler/` — shared HTTP adapter (`http.Handler`)
- Library tests and benchmarks; comparison benchmarks in `bench/` (separate module)
- CLI in `cmd/metagrab` with field presets, retries, bulk fetch and `serve`
- Deployment examples in `examples/` (standalone, Lambda, CF Container)

Out-of-scope:
- Production deployment infrastructure (Terraform, CDK, etc.)
- Caching and rate limiting (do these at the edge or in your service)

## Why this boundary exists

The library and its HTTP adapter (`httphandler/`) are pure Go with zero deployment-specific dependencies. Deployment configuration (Dockerfiles, wrangler configs, Worker scripts) lives in `examples/` as reference implementations, not production infrastructure.

This separation protects:
- API stability and versioning discipline
- Faster test cycles (no Docker/Wrangler needed for `go test`)
- Clean dependency graph (library has only `golang.org/x/net`, `golang.org/x/sync` and `golang.org/x/text`)
- Reuse across multiple runtimes (VPS, Lambda, CF Containers, any Go HTTP server)

## Field bitmask capacity

The `Field` type is `uint16`. Bits 0-7 are allocated:
- Bits 0-4: Title, URL, Meta, Description, Content
- Bits 5-7: Favicon, Canonical, JSON-LD

Bits 8-15 are free, so new fields can be added without a breaking change.

## Deployment reference architecture

Recommended architecture using `httphandler/`:

```
Client → Edge proxy (CF Worker / API Gateway) → metagrab httphandler (standalone / Lambda / CF Container)
```

Responsibilities split:
- Edge proxy: TLS termination, caching, rate limiting, request shaping
- `httphandler`: auth (`X-API-Key`), request validation, JSON wire format
- `metagrab.Client`: SSRF protection (`WithDenyPrivateNetworks`), retries/backoff, fetch/parse metadata

See `examples/` for concrete deployment references.

## Security posture

The library ships `WithDenyPrivateNetworks()` (dial-time SSRF protection that also covers redirects and DNS rebinding) and `httphandler` provides `X-API-Key` auth and request body limits out of the box.

Production deployments should additionally implement:
- Abuse controls (rate limiting, quotas)
- Structured logs and reason-coded failures

## Extraction paths

| Mask | Path | Reads |
| --- | --- | --- |
| Without `JSONLDField` or `ContentField` | Streaming tokenizer (`extractMetadataFromHead`) | Until `</head>` or `<body>` |
| With `JSONLDField` | Tokenizer for the head, then `scanJSONLD` for the body | Whole document, but the body is only scanned for JSON-LD scripts |
| With `ContentField` | Same tokenizer and scanner, run over the buffered body | Whole document, buffered |

All paths run `extractMetadataFromHead` and share the `extractor` type, so they apply the same rules. Inside `<body>`, only JSON-LD is collected. There is no DOM parse: `html.Parse` moves `<meta>` tags that follow a stray element in `<head>` into `<body>`, which would make results depend on the mask.
