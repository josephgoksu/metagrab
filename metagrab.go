package metagrab

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"golang.org/x/sync/errgroup"
)

// Link holds extracted metadata for a single URL.
type Link struct {
	URL          string            `json:"url"`
	Title        string            `json:"title,omitempty"`
	Description  string            `json:"description,omitempty"`
	OpenGraph    map[string]string `json:"open_graph,omitempty"`
	Twitter      map[string]string `json:"twitter,omitempty"`
	Content      string            `json:"content,omitempty"`
	Favicon      string            `json:"favicon,omitempty"`
	CanonicalURL string            `json:"canonical_url,omitempty"`
	Image        string            `json:"image,omitempty"`
	SiteName     string            `json:"site_name,omitempty"`
	Author       string            `json:"author,omitempty"`
	PublishedAt  string            `json:"published_at,omitempty"`
	JSONLD       []map[string]any  `json:"json_ld,omitempty"`
	ContentType  string            `json:"content_type,omitempty"`
	OEmbed       *OEmbedData       `json:"oembed,omitempty"`
}

// Field is a bitmask for selecting which metadata to extract.
//
// Field is 16 bits wide; bits 8-15 are reserved for future fields.
type Field uint16

const (
	TitleField       Field = 1 << iota // 1
	URLField                           // 2
	MetaField                          // 4
	DescriptionField                   // 8
	ContentField                       // 16
	FaviconField                       // 32
	CanonicalField                     // 64
	JSONLDField                        // 128

	MetadataFields = TitleField | URLField | MetaField | DescriptionField // 15
	PreviewFields  = MetadataFields | FaviconField | CanonicalField       // 111
	RichFields     = PreviewFields | JSONLDField                          // 239
	AllFields      = RichFields | ContentField                            // 255
)

// Has reports whether f contains the flag x.
func (f Field) Has(x Field) bool { return f&x != 0 }

// ErrorCode identifies a machine-readable failure category.
type ErrorCode string

const (
	ErrorCodeInvalidURL       ErrorCode = "invalid_url"
	ErrorCodeURLDenied        ErrorCode = "url_denied"
	ErrorCodeRequestBuild     ErrorCode = "request_build_failed"
	ErrorCodeNetwork          ErrorCode = "network_error"
	ErrorCodeCanceled         ErrorCode = "context_canceled"
	ErrorCodeDeadlineExceeded ErrorCode = "deadline_exceeded"
	ErrorCodeHTTPStatus       ErrorCode = "http_status_error"
	ErrorCodeReadBody         ErrorCode = "read_body_error"
	ErrorCodeBodyTooLarge     ErrorCode = "body_too_large"
	ErrorCodeContentType      ErrorCode = "unsupported_content_type"
	ErrorCodeParseHTML        ErrorCode = "parse_html_error"
)

// FetchError is a structured error returned by Fetch/FetchBulk methods.
type FetchError struct {
	Code       ErrorCode `json:"code"`
	URL        string    `json:"url,omitempty"`
	StatusCode int       `json:"status_code,omitempty"`
	Message    string    `json:"message"`

	cause error
}

func (e *FetchError) Error() string {
	if e == nil {
		return "<nil>"
	}
	msg := e.Message
	if msg == "" {
		msg = "metadata fetch failed"
	}
	if e.StatusCode > 0 {
		msg = fmt.Sprintf("%s (status=%d)", msg, e.StatusCode)
	}
	if e.URL != "" {
		msg = fmt.Sprintf("%s: %s", msg, e.URL)
	}
	if e.cause != nil {
		msg = fmt.Sprintf("%s: %v", msg, e.cause)
	}
	if e.Code != "" {
		msg = fmt.Sprintf("%s: %s", e.Code, msg)
	}
	return msg
}

// Unwrap returns the underlying error.
func (e *FetchError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// BulkResult contains per-URL results for partial-success batch fetches.
type BulkResult struct {
	URL   string      `json:"url"`
	Link  *Link       `json:"link,omitempty"`
	Error *FetchError `json:"error,omitempty"`
}

// URLPolicy can block/allow URLs before network I/O.
// Return nil to allow the URL, or an error to block it.
type URLPolicy func(*url.URL) error

const (
	defaultTimeout      = 10 * time.Second
	defaultUserAgent    = "metagrab/1 (+https://github.com/josephgoksu/metagrab)"
	defaultMaxBodySize  = 2 * 1024 * 1024 // 2 MB
	defaultConcurrency  = 10
	defaultMaxURLLen    = 2048
	defaultRetryDelay   = 250 * time.Millisecond
	defaultMaxRetryWait = 30 * time.Second
	defaultMaxRedirects = 5
	maxDrainBytes       = 64 * 1024
	maxDrainWait        = 100 * time.Millisecond
)

var (
	errBodyTooLarge   = errors.New("response body exceeds configured limit")
	errRedirectDenied = errors.New("redirect rejected")
)

// Client fetches and parses HTML metadata.
type Client struct {
	httpClient  *http.Client
	userAgent   string
	maxBodySize int64
	concurrency int
	timeout     time.Duration
	urlPolicy   URLPolicy
	maxRetries  int
	retryDelay  time.Duration

	maxRetryWait time.Duration
	maxRedirects int
	denyPrivate  bool
}

// NewClient creates a Client with the given options applied.
func NewClient(opts ...Option) *Client {
	c := &Client{
		userAgent:   defaultUserAgent,
		maxBodySize: defaultMaxBodySize,
		concurrency: defaultConcurrency,
		timeout:     defaultTimeout,
		retryDelay:  defaultRetryDelay,

		maxRetryWait: defaultMaxRetryWait,
		maxRedirects: defaultMaxRedirects,
	}

	for _, o := range opts {
		o(c)
	}

	if c.timeout <= 0 {
		c.timeout = defaultTimeout
	}
	if c.maxBodySize <= 0 {
		c.maxBodySize = defaultMaxBodySize
	}
	if c.concurrency <= 0 {
		c.concurrency = defaultConcurrency
	}
	if c.userAgent == "" {
		c.userAgent = defaultUserAgent
	}
	if c.retryDelay <= 0 {
		c.retryDelay = defaultRetryDelay
	}

	if c.maxRetryWait <= 0 {
		c.maxRetryWait = defaultMaxRetryWait
	}

	if c.httpClient == nil {
		c.httpClient = c.newHTTPClient()
	}

	return c
}

func (c *Client) newHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if c.denyPrivate {
		dialer.Control = dialControlDenyPrivate
		// Through a proxy the dialer would only see the proxy's address,
		// which silently disables the check. WithDenyPrivateNetworks
		// documents that HTTP(S)_PROXY is ignored.
		transport.Proxy = nil
	}
	transport.DialContext = dialer.DialContext
	return &http.Client{
		Timeout:       c.timeout,
		Transport:     transport,
		CheckRedirect: c.checkRedirect,
	}
}

// checkRedirect applies the redirect limit, the scheme rules and the URL
// policy to every redirect target.
func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > c.maxRedirects {
		// Hand the last 3xx back to the caller instead of following it.
		return http.ErrUseLastResponse
	}
	if err := checkURL(req.URL); err != nil {
		return fmt.Errorf("%w: %v", errRedirectDenied, err)
	}
	if c.urlPolicy != nil {
		if err := c.urlPolicy(req.URL); err != nil {
			return fmt.Errorf("%w: %v", errRedirectDenied, err)
		}
	}
	return nil
}

// Package-level singleton.
var defaultClient = NewClient()

// Fetch extracts metadata from a single URL using the default client.
func Fetch(ctx context.Context, rawURL string, mask Field) (*Link, error) {
	return defaultClient.Fetch(ctx, rawURL, mask)
}

// FetchBulk fetches links in batch and fails if any item fails.
//
// Deprecated: use FetchBulkResults for partial-success behavior.
func FetchBulk(ctx context.Context, urls []string, mask Field) ([]*Link, error) {
	return defaultClient.FetchBulk(ctx, urls, mask)
}

// FetchBulkResults extracts metadata from multiple URLs and returns per-item errors.
func FetchBulkResults(ctx context.Context, urls []string, mask Field) []BulkResult {
	return defaultClient.FetchBulkResults(ctx, urls, mask)
}

// Fetch extracts metadata from a single URL.
func (c *Client) Fetch(ctx context.Context, rawURL string, mask Field) (*Link, error) {
	if mask == 0 {
		mask = AllFields
	}

	parsedURL, err := validateURL(rawURL)
	if err != nil {
		return nil, newFetchError(ErrorCodeInvalidURL, rawURL, "invalid URL", err)
	}

	if c.urlPolicy != nil {
		if err := c.urlPolicy(parsedURL); err != nil {
			return nil, newFetchError(ErrorCodeURLDenied, parsedURL.String(), "URL rejected by policy", err)
		}
	}

	link := &Link{URL: parsedURL.String()}
	if mask == URLField {
		return link, nil
	}

	// reqCtx lets the deferred drain below give up on a body that trickles
	// or never ends, without touching the caller's ctx.
	reqCtx, cancelReq := context.WithCancel(ctx)
	defer cancelReq()

	var resp *http.Response
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, parsedURL.String(), nil)
		if err != nil {
			return nil, newFetchError(ErrorCodeRequestBuild, parsedURL.String(), "failed to build request", err)
		}
		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")

		resp, err = c.httpClient.Do(req)
		if err != nil {
			return nil, classifyRequestError(parsedURL.String(), err)
		}
		if !isRetryableStatus(resp.StatusCode) || attempt >= c.maxRetries {
			break
		}
		delay := c.retryDelay * time.Duration(1<<min(attempt, 16))
		if d, ok := parseRetryAfter(resp.Header.Get("Retry-After")); ok {
			delay = d
		}
		delay = min(delay, c.maxRetryWait)
		_ = resp.Body.Close()
		if err := sleepWithContext(ctx, delay); err != nil {
			return nil, classifyContextOrNetworkError(parsedURL.String(), "retry wait interrupted", err)
		}
	}
	defer func() {
		// Read a little of what is left so the connection can go back to the
		// pool; a body closed early forces a new TCP+TLS handshake next time.
		// Larger leftovers are cheaper to drop than to download, and the
		// drain is time-bounded too: a server that streams slowly or never
		// finishes must not hold the result back until the timeout.
		stop := time.AfterFunc(maxDrainWait, cancelReq)
		_, _ = io.CopyN(io.Discard, resp.Body, maxDrainBytes)
		stop.Stop()
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &FetchError{
			Code:       ErrorCodeHTTPStatus,
			URL:        parsedURL.String(),
			StatusCode: resp.StatusCode,
			Message:    "unexpected HTTP status",
		}
	}

	contentType := resp.Header.Get("Content-Type")
	if err := validateContentType(contentType); err != nil {
		return nil, newFetchError(ErrorCodeContentType, parsedURL.String(), "unsupported content type", err)
	}

	// Relative URLs (canonical, favicon, image) resolve against the final
	// URL after redirects, not the requested one.
	baseURL := parsedURL.String()
	if resp.Request != nil && resp.Request.URL != nil {
		baseURL = resp.Request.URL.String()
	}

	if ferr := c.parseInto(resp.Body, link, baseURL, contentType, mask); ferr != nil {
		ferr.URL = parsedURL.String()
		return nil, ferr
	}

	if mask.Has(FaviconField) && link.Favicon == "" {
		link.Favicon = resolveURL(baseURL, "/favicon.ico")
	}

	if mask.Has(MetaField) {
		if provider := matchOEmbedProvider(parsedURL); provider != nil {
			if data, err := c.fetchOEmbed(ctx, parsedURL.String(), provider); err == nil {
				link.OEmbed = data
				if mask.Has(TitleField) && link.Title == "" && data.Title != "" {
					link.Title = data.Title
				}
				if data.ProviderName != "" && link.SiteName == "" {
					link.SiteName = data.ProviderName
				}
				if data.AuthorName != "" && link.Author == "" {
					link.Author = data.AuthorName
				}
				if link.Image == "" && data.ThumbnailURL != "" {
					link.Image = data.ThumbnailURL
				}
			}
		}
	}

	// The hostname fallback runs last so an oEmbed title can fill the gap.
	if mask.Has(TitleField) && link.Title == "" {
		link.Title = parsedURL.Hostname()
	}

	return link, nil
}

// FetchBulk fetches links in batch and fails if any item fails.
//
// Deprecated: use FetchBulkResults for partial-success behavior.
func (c *Client) FetchBulk(ctx context.Context, urls []string, mask Field) ([]*Link, error) {
	if mask == 0 {
		mask = AllFields
	}

	out := make([]*Link, len(urls))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(c.concurrency)

	for i, rawURL := range urls {
		g.Go(func() error {
			link, err := c.Fetch(gctx, rawURL, mask)
			if err != nil {
				return err
			}
			out[i] = link
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

// FetchBulkResults extracts metadata from multiple URLs with bounded concurrency.
func (c *Client) FetchBulkResults(ctx context.Context, urls []string, mask Field) []BulkResult {
	if mask == 0 {
		mask = AllFields
	}

	results := make([]BulkResult, len(urls))
	if len(urls) == 0 {
		return results
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(c.concurrency)

	for i, rawURL := range urls {
		g.Go(func() error {
			link, err := c.Fetch(gctx, rawURL, mask)
			if err != nil {
				results[i] = BulkResult{URL: rawURL, Error: toFetchError(rawURL, err)}
				return nil
			}
			results[i] = BulkResult{URL: link.URL, Link: link}
			return nil
		})
	}

	_ = g.Wait()
	return results
}

// extractor accumulates metadata from the head tokenizer and the JSON-LD
// body scanner.
type extractor struct {
	link        *Link
	base        string
	mask        Field
	bestFav     faviconCandidate
	jsonLDCount int
}

func (e *extractor) title(text string) {
	if e.mask.Has(TitleField) && e.link.Title == "" {
		e.link.Title = collapseSpace(text)
	}
}

func (e *extractor) linkTag(rel, href, sizes string) {
	href = strings.TrimSpace(href)
	if href == "" {
		return
	}
	tokens := strings.Fields(strings.ToLower(rel))

	if e.mask.Has(CanonicalField) && e.link.CanonicalURL == "" && slices.Contains(tokens, "canonical") {
		e.link.CanonicalURL = resolveURL(e.base, href)
	}

	if e.mask.Has(FaviconField) {
		if prio := faviconPriority(tokens); prio > 0 {
			cand := faviconCandidate{href: resolveURL(e.base, href), priority: prio, size: parseSizes(sizes)}
			if cand.betterThan(e.bestFav) {
				e.bestFav = cand
			}
		}
	}
}

// wantJSONLD reports whether more JSON-LD blocks should be collected.
func (e *extractor) wantJSONLD() bool {
	return e.mask.Has(JSONLDField) && e.jsonLDCount < maxJSONLDBlocks
}

func (e *extractor) jsonLD(raw string) {
	blocks := parseJSONLDAll(raw)
	if len(blocks) == 0 {
		return
	}
	if room := maxJSONLDBlocks - e.jsonLDCount; len(blocks) > room {
		blocks = blocks[:room]
	}
	e.link.JSONLD = append(e.link.JSONLD, blocks...)
	e.jsonLDCount += len(blocks)
}

func (e *extractor) finish() {
	link := e.link
	if e.bestFav.href != "" {
		link.Favicon = e.bestFav.href
	}
	if e.mask.Has(JSONLDField) && len(link.JSONLD) > 0 && link.ContentType == "" {
		link.ContentType = extractContentType(link.JSONLD)
	}
	if e.mask.Has(MetaField) && link.Image == "" {
		for _, v := range []string{
			link.OpenGraph["og:image"],
			link.OpenGraph["og:image:secure_url"],
			link.OpenGraph["og:image:url"],
			link.Twitter["twitter:image"],
			link.Twitter["twitter:image:src"],
		} {
			if v = strings.TrimSpace(v); v != "" {
				link.Image = resolveURL(e.base, v)
				break
			}
		}
	}
}

func isLDJSONType(v string) bool {
	return strings.EqualFold(strings.TrimSpace(v), "application/ld+json")
}

func applyMetaTag(name, property, content string, link *Link, mask Field) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	key := property
	if key == "" {
		key = name
	}
	lowerKey := strings.ToLower(strings.TrimSpace(key))
	if lowerKey == "" {
		return
	}

	if mask.Has(DescriptionField) && lowerKey == "description" && link.Description == "" {
		link.Description = content
	}

	if strings.HasPrefix(lowerKey, "og:") {
		if mask.Has(TitleField) && lowerKey == "og:title" && link.Title == "" {
			link.Title = content
		}
		if mask.Has(DescriptionField) && lowerKey == "og:description" && link.Description == "" {
			link.Description = content
		}
		if mask.Has(MetaField) {
			if link.OpenGraph == nil {
				link.OpenGraph = make(map[string]string)
			}
			if _, seen := link.OpenGraph[lowerKey]; !seen {
				// Keep the first value: for repeated tags such as og:image,
				// the first one is the primary.
				link.OpenGraph[lowerKey] = content
			}

			if lowerKey == "og:site_name" && link.SiteName == "" {
				link.SiteName = content
			}
		}
	}

	if mask.Has(MetaField) && strings.HasPrefix(lowerKey, "article:") {
		if link.OpenGraph == nil {
			link.OpenGraph = make(map[string]string)
		}
		if _, seen := link.OpenGraph[lowerKey]; !seen {
			link.OpenGraph[lowerKey] = content
		}

		switch lowerKey {
		case "article:author":
			if link.Author == "" {
				link.Author = content
			}
		case "article:published_time":
			if link.PublishedAt == "" {
				link.PublishedAt = content
			}
		}
	}

	if mask.Has(MetaField) && strings.HasPrefix(lowerKey, "twitter:") {
		if link.Twitter == nil {
			link.Twitter = make(map[string]string)
		}
		if _, seen := link.Twitter[lowerKey]; !seen {
			link.Twitter[lowerKey] = content
		}
	}

	// Standard meta name fallbacks for author and date.
	if mask.Has(MetaField) {
		switch lowerKey {
		case "author":
			if link.Author == "" {
				link.Author = content
			}
		case "date", "pubdate":
			if link.PublishedAt == "" {
				link.PublishedAt = content
			}
		}
	}
}

// extractMetadataFromHead streams the document with a tokenizer and stops at
// <body> or </head>. When JSON-LD is requested, the rest of the document is
// handed to scanJSONLD, which only looks for JSON-LD scripts.
// r must already be size-limited.
func extractMetadataFromHead(r io.Reader, link *Link, base string, mask Field) error {
	z := html.NewTokenizer(r)
	e := &extractor{link: link, base: base, mask: mask}

	// attrs returns the values of up to three attributes of the current tag,
	// in the order of keys. The first occurrence of an attribute wins, as in
	// browsers. The tokenizer already lower-cases attribute keys.
	attrs := func(hasAttr bool, keys ...string) (vals [3]string) {
		var seen [3]bool
		for hasAttr {
			k, v, more := z.TagAttr()
			for i, want := range keys {
				if !seen[i] && string(k) == want {
					vals[i], seen[i] = string(v), true
				}
			}
			hasAttr = more
		}
		return vals
	}

	// scanRest hands everything the tokenizer has not consumed to the
	// JSON-LD body scanner, when JSON-LD is requested.
	scanRest := func() error {
		if !mask.Has(JSONLDField) {
			return nil
		}
		return scanJSONLD(io.MultiReader(bytes.NewReader(z.Buffered()), r), e)
	}

loop:
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if err := z.Err(); err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			break loop
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) { // the tokenizer lower-cases tag names
			case "body":
				if err := scanRest(); err != nil {
					return err
				}
				break loop
			case "title":
				if tt == html.StartTagToken {
					var sb strings.Builder
					for z.Next() == html.TextToken {
						sb.Write(z.Text())
					}
					e.title(sb.String())
				}
			case "meta":
				a := attrs(hasAttr, "name", "property", "content")
				applyMetaTag(a[0], a[1], a[2], link, mask)
			case "link":
				a := attrs(hasAttr, "rel", "href", "sizes")
				e.linkTag(a[0], a[1], a[2])
			case "script":
				if !e.wantJSONLD() {
					continue
				}
				a := attrs(hasAttr, "type")
				if !isLDJSONType(a[0]) || tt != html.StartTagToken {
					continue
				}
				var sb strings.Builder
				for z.Next() == html.TextToken {
					sb.Write(z.Text())
				}
				e.jsonLD(sb.String())
			}
		case html.EndTagToken:
			if name, _ := z.TagName(); string(name) == "head" {
				if err := scanRest(); err != nil {
					return err
				}
				break loop
			}
		}
	}

	e.finish()
	return nil
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// sniffLen is how many leading bytes are examined for a <meta charset>.
const sniffLen = 1024

// utf8Reader wraps r so it yields UTF-8. The encoding comes from a BOM,
// the Content-Type charset, or a <meta charset> in the first 1024 bytes.
// Without any declaration the body is assumed to be UTF-8, the default for
// the modern web (x/net/html/charset would guess windows-1252, which garbles
// undeclared UTF-8 pages).
func utf8Reader(r io.Reader, contentType string) io.Reader {
	br := bufio.NewReaderSize(r, 4096)
	head := peekHead(br)
	enc, name, certain := charset.DetermineEncoding(head, contentType)
	if name == "utf-8" {
		return br
	}
	// DetermineEncoding reports both "nothing declared" and a <meta> that
	// declares windows-1252 (or iso-8859-1, us-ascii, ...) as an uncertain
	// windows-1252. Only the first means UTF-8.
	if !certain && name == "windows-1252" && !declaresWindows1252(head) {
		return br
	}
	return enc.NewDecoder().Reader(br)
}

// peekHead returns up to sniffLen bytes from br without consuming them. It
// stops early once </head> or <body has arrived, so a small page whose
// server keeps the response open does not block until the timeout.
func peekHead(br *bufio.Reader) []byte {
	for {
		n := br.Buffered()
		if n >= sniffLen {
			head, _ := br.Peek(sniffLen)
			return head
		}
		head, err := br.Peek(n + 1) // waits for at least one more read
		if err != nil {
			return head
		}
		head, _ = br.Peek(min(br.Buffered(), sniffLen))
		lower := bytes.ToLower(head)
		if bytes.Contains(lower, []byte("</head")) || bytes.Contains(lower, []byte("<body")) {
			return head
		}
	}
}

// declaresWindows1252 reports whether head has a <meta charset> or a
// <meta http-equiv="Content-Type"> whose label maps to windows-1252.
func declaresWindows1252(head []byte) bool {
	is1252 := func(label string) bool {
		_, name := charset.Lookup(strings.TrimSpace(label))
		return name == "windows-1252"
	}
	z := html.NewTokenizer(bytes.NewReader(head))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return false
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			if string(name) != "meta" {
				continue
			}
			var isContentType bool
			var label string
			for hasAttr {
				k, v, more := z.TagAttr()
				switch string(k) {
				case "charset":
					return is1252(string(v))
				case "http-equiv":
					isContentType = strings.EqualFold(strings.TrimSpace(string(v)), "content-type")
				case "content":
					if _, params, err := mime.ParseMediaType(string(v)); err == nil {
						label = params["charset"]
					}
				}
				hasAttr = more
			}
			if isContentType && label != "" {
				return is1252(label)
			}
		}
	}
}

// decodeToUTF8 converts a full body to UTF-8 (same rules as utf8Reader).
func decodeToUTF8(raw []byte, contentType string) []byte {
	enc, name, certain := charset.DetermineEncoding(raw, contentType)
	if name == "utf-8" || (!certain && utf8.Valid(raw)) {
		return raw
	}
	out, err := enc.NewDecoder().Bytes(raw)
	if err != nil {
		return raw
	}
	return out
}

// resolveURL resolves ref against base. Returns ref unchanged if parsing fails.
func resolveURL(base, ref string) string {
	if ref == "" {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

// faviconPriority scores lower-cased rel tokens for favicon preference.
func faviconPriority(tokens []string) int {
	switch {
	case slices.Contains(tokens, "apple-touch-icon"), slices.Contains(tokens, "apple-touch-icon-precomposed"):
		return 3
	case slices.Contains(tokens, "icon") && slices.Contains(tokens, "shortcut"):
		return 1
	case slices.Contains(tokens, "icon"):
		return 2
	default:
		return 0
	}
}

// parseSizes returns the largest width in a sizes attribute such as
// "180x180" or "16x16 32x32". "any" and malformed values return 0.
func parseSizes(sizes string) int {
	best := 0
	for _, s := range strings.Fields(strings.ToLower(sizes)) {
		w, _, ok := strings.Cut(s, "x")
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(w); err == nil && n > best {
			best = n
		}
	}
	return best
}

type faviconCandidate struct {
	href     string
	priority int
	size     int
}

func (c faviconCandidate) betterThan(other faviconCandidate) bool {
	if c.priority != other.priority {
		return c.priority > other.priority
	}
	return c.size > other.size
}

const (
	maxJSONLDBlocks = 10
	maxJSONLDSize   = 256 * 1024
)

// parseJSONLDAll parses a JSON-LD script body. Handles both single objects and arrays.
func parseJSONLDAll(raw string) []map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxJSONLDSize {
		return nil
	}

	// Try as single object first.
	var single map[string]any
	if err := json.Unmarshal([]byte(raw), &single); err == nil {
		return []map[string]any{single}
	}

	// Try as array.
	var arr []map[string]any
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return arr
	}

	return nil
}

// extractContentType extracts the first @type value from JSON-LD blocks.
func extractContentType(blocks []map[string]any) string {
	for _, block := range blocks {
		t, ok := block["@type"]
		if !ok {
			continue
		}
		switch v := t.(type) {
		case string:
			return v
		case []any:
			if len(v) > 0 {
				if s, ok := v[0].(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

func validateURL(rawURL string) (*url.URL, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, errors.New("URL is empty")
	}
	if len(trimmed) > defaultMaxURLLen {
		return nil, fmt.Errorf("URL exceeds %d characters", defaultMaxURLLen)
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, err
	}
	if err := checkURL(u); err != nil {
		return nil, err
	}
	return u, nil
}

// checkURL applies the scheme, host and userinfo rules. Redirect targets get
// these checks but not the input length cap: SSO and consent redirects often
// carry long query strings.
func checkURL(u *url.URL) error {
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported scheme %q", u.Scheme)
	}

	if u.Hostname() == "" {
		return errors.New("URL host is empty")
	}
	if u.User != nil {
		return errors.New("URL userinfo is not allowed")
	}
	return nil
}

func validateContentType(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return err
	}
	switch strings.ToLower(mediaType) {
	case "text/html", "application/xhtml+xml":
		return nil
	default:
		return fmt.Errorf("got %q", mediaType)
	}
}

func readBodyWithLimit(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return io.ReadAll(r)
	}
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errBodyTooLarge
	}
	return b, nil
}

func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func parseRetryAfter(header string) (time.Duration, bool) {
	h := strings.TrimSpace(header)
	if h == "" {
		return 0, false
	}
	if sec, err := time.ParseDuration(h + "s"); err == nil && sec > 0 {
		return sec, true
	}
	if when, err := http.ParseTime(h); err == nil {
		d := time.Until(when)
		if d > 0 {
			return d, true
		}
	}
	return 0, false
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func classifyContextOrNetworkError(rawURL, message string, err error) *FetchError {
	switch {
	case errors.Is(err, context.Canceled):
		return newFetchError(ErrorCodeCanceled, rawURL, message, err)
	case errors.Is(err, context.DeadlineExceeded):
		return newFetchError(ErrorCodeDeadlineExceeded, rawURL, message, err)
	default:
		return newFetchError(ErrorCodeNetwork, rawURL, message, err)
	}
}

// classifyRequestError maps an error from http.Client.Do to a FetchError.
func classifyRequestError(rawURL string, err error) *FetchError {
	switch {
	case errors.Is(err, errRedirectDenied):
		return newFetchError(ErrorCodeURLDenied, rawURL, "redirect rejected", err)
	case errors.Is(err, ErrPrivateAddress):
		return newFetchError(ErrorCodeURLDenied, rawURL, "address rejected by private network policy", err)
	default:
		return classifyContextOrNetworkError(rawURL, "request failed", err)
	}
}

func classifyContextOrReadError(rawURL, message string, err error) *FetchError {
	switch {
	case errors.Is(err, context.Canceled):
		return newFetchError(ErrorCodeCanceled, rawURL, message, err)
	case errors.Is(err, context.DeadlineExceeded):
		return newFetchError(ErrorCodeDeadlineExceeded, rawURL, message, err)
	default:
		return newFetchError(ErrorCodeReadBody, rawURL, message, err)
	}
}

func toFetchError(rawURL string, err error) *FetchError {
	var ferr *FetchError
	if errors.As(err, &ferr) {
		return ferr
	}
	return newFetchError(ErrorCodeNetwork, rawURL, "fetch failed", err)
}

func newFetchError(code ErrorCode, rawURL, message string, cause error) *FetchError {
	return &FetchError{
		Code:    code,
		URL:     rawURL,
		Message: message,
		cause:   cause,
	}
}
