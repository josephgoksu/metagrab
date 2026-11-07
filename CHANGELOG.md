# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-11-07

First tagged release. Before this, metagrab was only available as untagged
pseudo-versions (`v0.0.0-2026...`).

### Added

- `Parse` and `Client.Parse`: extract metadata from HTML you already have.
- `Link.Image`: best absolute preview image (`og:image`, then `twitter:image`, then the oEmbed thumbnail).
- `WithDenyPrivateNetworks`: dial-time SSRF protection that also covers redirects and DNS rebinding. `IsPublicAddr` and `ErrPrivateAddress` are exported. While it is on, `HTTP_PROXY`/`HTTPS_PROXY` are ignored. `IsPublicAddr` checks the IPv4 address inside a NAT64 address (`64:ff9b::/96`), so DNS64 networks keep working.
- `WithMaxRedirects` (default 5) and `WithMaxRetryWait` (default 30s).
- Charset decoding for non-UTF-8 pages.
- CLI: `metagrab serve` (HTTP API), `metagrab version`, `-concurrency`, `-compact`, `-quiet`.
- Release artifacts: binaries for Linux, macOS and Windows, a Homebrew cask, and a Docker image at `ghcr.io/josephgoksu/metagrab`.
- Benchmarks against other Go libraries in `bench/` and [docs/benchmarks](docs/benchmarks/README.md).

### Changed

- `RichFields` now finds JSON-LD anywhere in the document, using a fast scanner for the body (about 3x faster than tokenizing it).
- `Field` is now `uint16`, which leaves room for new fields without a breaking change.
- The URL policy runs on every redirect target, not only the first URL.
- Relative URLs resolve against the final URL after redirects.
- The first value of a repeated tag (such as `og:image`) wins.
- Favicon `rel` values are parsed as token lists; `mask-icon` is ignored.
- oEmbed fills `SiteName` from the provider name (for example "YouTube") and `Author` from the author name. Before, the author name went into `SiteName`.
- Fetch keeps the connection alive when little of the body is left to read and it arrives quickly.
- `ContentField` buffers the body but extracts metadata with the same streaming tokenizer as the other masks (no DOM parse), so the mask never changes what is found.
- Default User-Agent is `metagrab/1 (+https://github.com/josephgoksu/metagrab)`.
- `httphandler`: upstream non-2xx responses return `502` (upstream status in `status_code`), non-HTML returns `422`, and `/health` does not need the API key.
- Minimum Go version is 1.26.

### Fixed

- Redirects to private addresses bypassed `DenyPrivateIPs`.
- A large `Retry-After` header could stall a request for hours.
- `go install .../cmd@latest` installed a binary named `cmd`. The CLI is now at `cmd/metagrab`.
- An SVG `<title>` or a `<meta>` in `<body>` could override head metadata.
- Pages in other encodings (for example windows-1251 or Shift_JIS) came back garbled.

### Upgrading from a pseudo-version

| Before | Now |
| --- | --- |
| `go install github.com/josephgoksu/metagrab/cmd@latest` | `go install github.com/josephgoksu/metagrab/cmd/metagrab@latest` |
| `metagrab.WithURLPolicy(metagrab.DenyPrivateIPs())` for SSRF | `metagrab.WithDenyPrivateNetworks()` |
| `metagrab.Field(uint8(x))` | `metagrab.Field(x)` (now `uint16`) |
| Handler returned the upstream status (for example `404`) | Handler returns `502`; read `status_code` in the body |
| `link.Meta["og:title"]` (2025 API) | `link.OpenGraph["og:title"]`, `link.Twitter["twitter:card"]` |
| `FetchBulk` (fails the whole batch) | `FetchBulkResults` (per-URL errors) |
