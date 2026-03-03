# Architecture

## Scope of this repository

`metagrab` is a Go library.

In-scope:
- Metadata fetching/parsing primitives (title, description, OG, Twitter Cards, favicon, canonical URL, author/date, JSON-LD, oEmbed)
- Opt-in field bitmask for controlling extraction scope (all 8 bits of `uint8` allocated)
- oEmbed provider registry with ~30 compiled-in providers
- Public Go API (`Fetch`, `FetchBulk`, `FetchBulkResults`)
- SSRF protection helpers (`DenyPrivateIPs`)
- `httphandler/` — shared HTTP adapter (`http.Handler`)
- Library tests and benchmarks
- CLI tool in `cmd/` with field presets, retries, and bulk fetch
- Deployment examples in `examples/` (standalone, Lambda, CF Container)

Out-of-scope:
- Production deployment infrastructure (Terraform, CDK, etc.)
- CI/CD pipelines for deployed services

## Why this boundary exists

The library and its HTTP adapter (`httphandler/`) are pure Go with zero deployment-specific dependencies. Deployment configuration (Dockerfiles, wrangler configs, Worker scripts) lives in `examples/` as reference implementations, not production infrastructure.

This separation protects:
- API stability and versioning discipline
- Faster test cycles (no Docker/Wrangler needed for `go test`)
- Clean dependency graph (library has only `golang.org/x/net` and `golang.org/x/sync`)
- Reuse across multiple runtimes (VPS, Lambda, CF Containers, any Go HTTP server)

## Field bitmask capacity

The `Field` type is `uint8`. All 8 bits are allocated:
- Bits 0-4: original fields (Title, URL, Meta, Description, Content)
- Bits 5-7: v2.x additions (Favicon, Canonical, JSON-LD)

Future fields require bumping `Field` to `uint16` (v3 breaking change). This is acceptable because the 8 current fields cover the complete link preview use case.

## Deployment reference architecture

Recommended architecture using `httphandler/`:

```
Client → Edge proxy (CF Worker / API Gateway) → metagrab httphandler (standalone / Lambda / CF Container)
```

Responsibilities split:
- Edge proxy: TLS termination, caching, rate limiting, request shaping
- `httphandler`: auth (`X-API-Key`), request validation, JSON wire format
- `metagrab.Client`: SSRF policy (`DenyPrivateIPs`), retries/backoff, fetch/parse metadata

See `examples/` for concrete deployment references.

## Security posture

The library ships `DenyPrivateIPs()` for SSRF protection and `httphandler` provides `X-API-Key` auth and request body limits out of the box.

Production deployments should additionally implement:
- Abuse controls (rate limiting, quotas)
- Structured logs and reason-coded failures
- Custom `net.Dialer` for DNS-rebinding hardening (see `policy.go` godoc)
