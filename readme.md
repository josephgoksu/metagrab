<img src="./assets/logo.png" alt="metagrab" height="250" width="250" align="right" style="margin-left: 10px;" />

# metagrab

[![Go Reference](https://pkg.go.dev/badge/github.com/josephgoksu/metagrab.svg)](https://pkg.go.dev/github.com/josephgoksu/metagrab)
[![Go Report Card](https://goreportcard.com/badge/github.com/josephgoksu/metagrab)](https://goreportcard.com/report/github.com/josephgoksu/metagrab)

Fast, focused URL metadata extraction for Go. Extracts titles, descriptions, Open Graph, Twitter Cards, favicons, canonical URLs, author/date metadata, JSON-LD structured data, and oEmbed from any URL — with built-in retries, SSRF protection, and a deploy-ready HTTP adapter.

Requires **Go 1.26+**. Non-UTF-8 pages (for example Shift_JIS or windows-1251) are decoded to UTF-8.

## Install

```bash
go get github.com/josephgoksu/metagrab
```

## Quick start

Extract a link preview (title, description, OG, Twitter, favicon, canonical URL):

```go
link, err := metagrab.Fetch(ctx, "https://example.com", metagrab.PreviewFields)
if err != nil {
    log.Fatal(err) // *metagrab.FetchError with machine-readable Code
}
fmt.Println(link.Title)
fmt.Println(link.Favicon)
fmt.Println(link.OpenGraph["og:image"])
```

Include JSON-LD structured data:

```go
link, err := metagrab.Fetch(ctx, "https://example.com", metagrab.RichFields)
// link.JSONLD contains parsed JSON-LD blocks
// link.ContentType has the first @type (e.g. "Article", "Person")
```

## Field bitmask

Control what gets extracted per request. Use presets or combine individual flags:

| Preset           | Value | What it extracts                         |
| ---------------- | ----- | ---------------------------------------- |
| `MetadataFields` | 15    | Title + URL + OG/Twitter + Description   |
| `PreviewFields`  | 111   | MetadataFields + Favicon + Canonical URL |
| `RichFields`     | 239   | PreviewFields + JSON-LD                  |
| `AllFields`      | 255   | RichFields + full HTML body              |

Individual flags for custom combinations:

| Flag               | Value | Extracts                                                                                       |
| ------------------ | ----- | ---------------------------------------------------------------------------------------------- |
| `TitleField`       | 1     | `<title>` tag (falls back to hostname)                                                         |
| `URLField`         | 2     | Validated URL only (no network request)                                                        |
| `MetaField`        | 4     | Open Graph, Twitter Cards, oEmbed, elevated fields (author, site_name, published_at)           |
| `DescriptionField` | 8     | `<meta name="description">`                                                                    |
| `ContentField`     | 16    | Full HTML body content                                                                         |
| `FaviconField`     | 32    | Best favicon URL (`apple-touch-icon` > `icon` > `shortcut icon`, falls back to `/favicon.ico`) |
| `CanonicalField`   | 64    | `<link rel="canonical">` URL                                                                   |
| `JSONLDField`      | 128   | `<script type="application/ld+json">` blocks + `ContentType` from `@type`                      |

Example — metadata + favicon only (no canonical):

```go
mask := metagrab.MetadataFields | metagrab.FaviconField // 47
link, err := metagrab.Fetch(ctx, url, mask)
```

> **Note:** When mask is `0`, `Fetch` defaults to `AllFields` (255). The `httphandler` HTTP adapter defaults to `MetadataFields` (15) when the request omits `fields`.

## Link struct

| Field          | Type                | Source             | Description                                                                    |
| -------------- | ------------------- | ------------------ | ------------------------------------------------------------------------------ |
| `URL`          | `string`            | `URLField`         | Validated URL                                                                  |
| `Title`        | `string`            | `TitleField`       | Page title (falls back to hostname if missing)                                 |
| `Description`  | `string`            | `DescriptionField` | Meta description                                                               |
| `OpenGraph`    | `map[string]string` | `MetaField`        | All `og:*` tags                                                                |
| `Twitter`      | `map[string]string` | `MetaField`        | All `twitter:*` tags                                                           |
| `Content`      | `string`            | `ContentField`     | Full HTML body                                                                 |
| `Favicon`      | `string`            | `FaviconField`     | Best favicon URL                                                               |
| `CanonicalURL` | `string`            | `CanonicalField`   | Canonical URL                                                                  |
| `Image`        | `string`            | `MetaField`        | Best preview image, absolute (`og:image` > `twitter:image` > oEmbed thumbnail) |
| `SiteName`     | `string`            | `MetaField`        | From `og:site_name`                                                            |
| `Author`       | `string`            | `MetaField`        | From `article:author` or `<meta name="author">`                                |
| `PublishedAt`  | `string`            | `MetaField`        | From `article:published_time` or `<meta name="date">` (raw string, not parsed) |
| `JSONLD`       | `[]map[string]any`  | `JSONLDField`      | Parsed JSON-LD blocks (max 10 per page, 256 KB per block)                      |
| `ContentType`  | `string`            | `JSONLDField`      | First `@type` from JSON-LD (e.g. `"Article"`, `"Person"`)                      |
| `OEmbed`       | `*OEmbedData`       | `MetaField`        | oEmbed data (only when URL matches a known provider)                           |

## Bulk fetch

`FetchBulkResults` fetches multiple URLs concurrently and returns per-URL results (no single failure aborts the batch):

```go
results := metagrab.FetchBulkResults(ctx, urls, metagrab.PreviewFields)
for _, r := range results {
    if r.Error != nil {
        log.Printf("skip %s: %s", r.URL, r.Error.Code)
        continue
    }
    fmt.Println(r.Link.Title)
}
```

## Client options

Create a client with custom configuration:

```go
client := metagrab.NewClient(
    metagrab.WithTimeout(5 * time.Second),
    metagrab.WithRetries(2),
    metagrab.WithConcurrency(20),
    metagrab.WithDenyPrivateNetworks(), // SSRF protection
)
link, err := client.Fetch(ctx, url, metagrab.PreviewFields)
```

| Option            | Default        | Description                                                             |
| ----------------- | -------------- | ----------------------------------------------------------------------- |
| `WithTimeout`     | 10s            | HTTP client timeout (ignored when `WithHTTPClient` is set)              |
| `WithRetries`     | 0              | Retry on 429/502/503/504 with exponential backoff                       |
| `WithRetryDelay`  | 250ms          | Base delay between retries                                              |
| `WithMaxRetryWait` | 30s           | Upper bound for any retry wait, including `Retry-After`                 |
| `WithMaxRedirects` | 5             | Redirects to follow (0 = do not follow)                                 |
| `WithDenyPrivateNetworks` | off    | Block private/reserved addresses at dial time (see SSRF protection)     |
| `WithConcurrency` | 10             | Max parallel fetches for bulk operations                                |
| `WithMaxBodySize` | 2 MB           | Response body size limit                                                |
| `WithURLPolicy`   | nil            | URL allow/deny hook, run before the request and on every redirect       |
| `WithUserAgent`   | `metagrab/1`   | User-Agent header                                                       |
| `WithHTTPClient`  | —              | Bring your own `*http.Client` (timeout and transport managed by caller) |

Package-level functions (`metagrab.Fetch`, `metagrab.FetchBulkResults`) use a default client with the defaults above.

## Error handling

All errors are `*FetchError` with a machine-readable `Code`:

```go
link, err := metagrab.Fetch(ctx, url, metagrab.PreviewFields)
if err != nil {
    var fe *metagrab.FetchError
    if errors.As(err, &fe) {
        switch fe.Code {
        case metagrab.ErrorCodeInvalidURL:
            // bad URL format
        case metagrab.ErrorCodeURLDenied:
            // blocked by URL policy
        case metagrab.ErrorCodeHTTPStatus:
            log.Printf("HTTP %d for %s", fe.StatusCode, fe.URL)
        case metagrab.ErrorCodeCanceled:
            // context was canceled
        }
    }
}
```

| Code                       | Meaning                                                          |
| -------------------------- | ---------------------------------------------------------------- |
| `invalid_url`              | Malformed URL, unsupported scheme, or URL too long (>2048 chars) |
| `url_denied`               | Blocked by URL policy                                            |
| `request_build_failed`     | Could not construct HTTP request                                 |
| `network_error`            | Connection failed                                                |
| `context_canceled`         | Context was canceled                                             |
| `deadline_exceeded`        | Context deadline exceeded                                        |
| `http_status_error`        | Non-2xx HTTP status (after retries)                              |
| `read_body_error`          | Failed to read response body                                     |
| `body_too_large`           | Response body exceeded `MaxBodySize`                             |
| `unsupported_content_type` | Response is not HTML (e.g. JSON, PDF)                            |
| `parse_html_error`         | Failed to parse HTML                                             |

## SSRF protection

If URLs come from users, turn on `WithDenyPrivateNetworks()`:

```go
client := metagrab.NewClient(
    metagrab.WithDenyPrivateNetworks(),
)
```

The check runs in the dialer, after DNS resolution, on every connection. That covers redirects and DNS rebinding. It blocks loopback, RFC 1918, link-local (including cloud metadata at `169.254.169.254`), CGNAT, multicast, documentation, NAT64, IPv6 ULA and IPv4-mapped IPv6 forms of all of these. Blocked requests fail with `url_denied`, and the error wraps `metagrab.ErrPrivateAddress`. Proxy environment variables are ignored in this mode, because a proxy would hide the real target address.

`DenyPrivateIPs()` is still available as a `URLPolicy`. It resolves the host once before the request, so on its own it does not stop DNS rebinding. `IsPublicAddr(netip.Addr)` is exported for your own transports.

If you pass your own client with `WithHTTPClient`, these protections are yours to configure.

## oEmbed support

When `MetaField` is set and the URL matches a known provider, metagrab fetches oEmbed data and backfills empty `Title` and `SiteName` fields. The full oEmbed response is available via `link.OEmbed`:

```go
link, _ := metagrab.Fetch(ctx, "https://www.youtube.com/watch?v=dQw4w9WgXcQ", metagrab.PreviewFields)
fmt.Println(link.OEmbed.Type)         // "video"
fmt.Println(link.OEmbed.AuthorName)   // "Rick Astley"
fmt.Println(link.OEmbed.ThumbnailURL) // thumbnail URL
```

`OEmbedData` fields: `Type`, `Title`, `AuthorName`, `AuthorURL`, `ProviderName`, `ProviderURL`, `ThumbnailURL`, `HTML`, `Width`, `Height`.

Supported providers (23):

| Category | Providers                                 |
| -------- | ----------------------------------------- |
| Video    | YouTube, Vimeo, Dailymotion, TikTok       |
| Audio    | Spotify, SoundCloud                       |
| Social   | Twitter/X, Reddit                         |
| Code     | CodePen, CodeSandbox, JSFiddle, Replit    |
| Design   | Figma                                     |
| Docs     | SlideShare, Speaker Deck                  |
| Media    | Flickr, Giphy, Imgur                      |
| Other    | Loom, Miro, Kickstarter, Mixcloud, Scribd |

## HTTP service

The `httphandler` sub-package wraps a `metagrab.Client` as a standard `http.Handler`:

```go
import "github.com/josephgoksu/metagrab/httphandler"

client := metagrab.NewClient(
    metagrab.WithDenyPrivateNetworks(),
)
h := httphandler.New(client,
    httphandler.WithAPIKey(os.Getenv("API_KEY")),
)
http.ListenAndServe(":8080", h)
```

### Endpoints

**`POST /fetch`** — single URL

```json
{ "url": "https://example.com", "fields": 111 }
```

Returns the `Link` object directly. `fields` defaults to `15` (MetadataFields) when omitted.

Errors return a `FetchError` JSON body. Status codes: `400` for invalid or denied URLs, `422` for non-HTML targets, `504` for timeouts, and `502` for every other upstream failure. A non-2xx response from the target is also `502`, with the target's status in `status_code`.

**`POST /fetch-bulk`** — batch (up to 100 URLs)

```json
{ "urls": ["https://a.com", "https://b.com"], "fields": 111 }
```

Returns `[Link, ...]`. Failed URLs return a Link with only the `url` field populated.

**`GET /health`** — health check

Returns `{"status": "ok"}`. This endpoint does not require the API key, so load balancers can call it.

### Handler options

| Option                 | Default | Description                              |
| ---------------------- | ------- | ---------------------------------------- |
| `WithAPIKey`           | —       | Enable `X-API-Key` header authentication |
| `WithRequestBodyLimit` | 64 KB   | Max request body size                    |
| `WithMaxBulkURLs`      | 100     | Max URLs per `/fetch-bulk` request       |

Deployment examples in `examples/`:

| Runtime              | Directory                                                          | Notes                    |
| -------------------- | ------------------------------------------------------------------ | ------------------------ |
| Standalone           | [`examples/standalone/`](examples/standalone/)                     | ~25 lines                |
| AWS Lambda           | [`examples/lambda/`](examples/lambda/)                             | `provided.al2023`, arm64 |
| Cloudflare Container | [`examples/cloudflare-container/`](examples/cloudflare-container/) | Full Docker on CF edge   |

## CLI

```bash
brew install --cask josephgoksu/tap/metagrab
# or
go install github.com/josephgoksu/metagrab/cmd/metagrab@latest
```

Usage:

```bash
metagrab https://example.com                        # preview (default)
metagrab -fields=metadata https://example.com       # title + OG + description only
metagrab -fields=rich https://example.com           # preview + JSON-LD
metagrab -fields=all https://example.com            # everything including HTML body
metagrab -retries=2 -timeout=5s https://example.com # with retries and custom timeout
metagrab https://a.com https://b.com https://c.com  # bulk fetch (concurrent)
metagrab version
```

| Flag           | Default   | Description                                                      |
| -------------- | --------- | ---------------------------------------------------------------- |
| `-fields`      | `preview` | Preset (`metadata`, `preview`, `rich`, `all`) or numeric bitmask |
| `-timeout`     | `10s`     | HTTP timeout per request                                         |
| `-retries`     | `0`       | Retry attempts for 429/502/503/504                               |
| `-retry-delay` | `250ms`   | Base delay between retries                                       |
| `-concurrency` | `10`      | Parallel fetches when several URLs are given                     |
| `-compact`     | `false`   | One-line JSON output                                             |
| `-quiet`       | `false`   | Do not print timing to stderr                                    |

Exit codes: `0` ok, `1` usage error, `2` one or more URLs failed, `3` internal error.

### Run the HTTP API

```bash
metagrab serve -addr :8080 -api-key "$METAGRAB_API_KEY"
# or
docker run --rm -p 8080:8080 -e METAGRAB_API_KEY=secret ghcr.io/josephgoksu/metagrab
```

`serve` blocks private networks by default (`-allow-private-networks` turns that off for internal use), logs JSON to stderr, sets server timeouts and drains in-flight requests on `SIGTERM`. It reads `METAGRAB_ADDR`, `PORT` and `METAGRAB_API_KEY` from the environment.

## Testing

```bash
go test ./... -short        # Unit tests only (no network, ~0.6s)
go test ./... -race -short  # With race detector
go test ./...               # All tests including network
go test -tags=integration   # Integration tests against real URLs
```

## More

- [MIGRATION.md](./MIGRATION.md) — v1 → v2 breaking changes
- [ARCHITECTURE.md](./ARCHITECTURE.md) — Scope, boundaries, and deployment architecture
