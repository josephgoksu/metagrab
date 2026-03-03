# Migration Guide: v1 → v2

## Breaking changes

### 1. Metadata shape: `Meta` → `OpenGraph` + `Twitter`

v1 stored all meta tags in a single `Meta map[string]string` field. v2 separates them into two maps:

```go
// v1
type Link struct {
    Meta map[string]string `json:"meta"`
}

// v2
type Link struct {
    OpenGraph map[string]string `json:"open_graph,omitempty"`
    Twitter   map[string]string `json:"twitter,omitempty"`
}
```

**Action:** Replace `link.Meta["og:title"]` with `link.OpenGraph["og:title"]`, and `link.Meta["twitter:image"]` with `link.Twitter["twitter:image"]`.

### 2. Bulk fetching: `FetchBulk` → `FetchBulkResults`

`FetchBulk` is deprecated. It fails the entire batch if any single URL fails:

```go
// v1 — all-or-nothing
links, err := metagrab.FetchBulk(ctx, urls, mask)
```

v2 introduces `FetchBulkResults` which returns per-URL errors without aborting the batch:

```go
// v2 — partial success (recommended)
results := metagrab.FetchBulkResults(ctx, urls, mask)
for _, r := range results {
    if r.Error != nil {
        log.Printf("%s failed: %s", r.URL, r.Error.Code)
        continue
    }
    fmt.Println(r.Link.Title)
}
```

`FetchBulk` still works but will not receive new features.

### 3. Structured errors: `FetchError` with `ErrorCode`

v1 returned plain `error` values. v2 returns `*FetchError` with machine-readable codes:

```go
link, err := metagrab.Fetch(ctx, url, mask)
if err != nil {
    var fe *metagrab.FetchError
    if errors.As(err, &fe) {
        switch fe.Code {
        case metagrab.ErrorCodeURLDenied:
            // blocked by URL policy
        case metagrab.ErrorCodeHTTPStatus:
            log.Printf("HTTP %d for %s", fe.StatusCode, fe.URL)
        }
    }
}
```

See the full list of error codes in the [readme](./readme.md#quick-start) or in `metagrab.go`.

### 4. Client type with functional options

v1 used package-level functions with implicit global state. v2 introduces an explicit `Client` type:

```go
// v1 — package-level defaults
link, err := metagrab.Fetch(ctx, url, metagrab.AllFields)

// v2 — configurable client (recommended)
client := metagrab.NewClient(
    metagrab.WithTimeout(5 * time.Second),
    metagrab.WithRetries(2),
    metagrab.WithURLPolicy(metagrab.DenyPrivateIPs()),
)
link, err := client.Fetch(ctx, url, metagrab.AllFields)
```

Package-level `Fetch`, `FetchBulk`, and `FetchBulkResults` functions still work using a default client, so this change is backwards-compatible for simple use cases.

### 5. URL policy hook

v2 adds `WithURLPolicy` for blocking URLs before any network I/O. The built-in `DenyPrivateIPs()` policy prevents SSRF by rejecting private/reserved IP ranges:

```go
client := metagrab.NewClient(
    metagrab.WithURLPolicy(metagrab.DenyPrivateIPs()),
)
```

Custom policies implement `func(*url.URL) error`. Return `nil` to allow, or an error to block.

## New in v2

- `httphandler` sub-package — shared HTTP adapter for deploying as Lambda, CF Container, or standalone server
- `DenyPrivateIPs()` — built-in SSRF protection policy
- `BulkResult` type with per-URL error reporting
- Retry support with exponential backoff and `Retry-After` header respect
- Head-only HTML parsing optimization for metadata-only requests
- Favicon extraction with priority ranking (`apple-touch-icon` > `icon` > `shortcut icon`, `/favicon.ico` fallback)
- Canonical URL extraction from `<link rel="canonical">`
- Elevated fields: `SiteName`, `Author`, `PublishedAt` from OG/meta tags
- JSON-LD structured data extraction with `ContentType` from `@type`
- oEmbed support for 23 providers (YouTube, Vimeo, Twitter/X, Spotify, Figma, and more)
- CLI tool with field presets (`-fields=metadata|preview|rich|all`)
