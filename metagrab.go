package metagrab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
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
	SiteName     string            `json:"site_name,omitempty"`
	Author       string            `json:"author,omitempty"`
	PublishedAt  string            `json:"published_at,omitempty"`
	JSONLD       []map[string]any  `json:"json_ld,omitempty"`
	ContentType  string            `json:"content_type,omitempty"`
	OEmbed       *OEmbedData       `json:"oembed,omitempty"`
}

// Field is a bitmask for selecting which metadata to extract.
type Field uint8

const (
	TitleField       Field = 1 << iota // 1
	URLField                           // 2
	MetaField                          // 4
	DescriptionField                   // 8
	ContentField                       // 16
	FaviconField                       // 32
	CanonicalField                     // 64
	JSONLDField                        // 128

	MetadataFields = TitleField | URLField | MetaField | DescriptionField                          // 15
	PreviewFields  = MetadataFields | FaviconField | CanonicalField                                // 111
	RichFields     = PreviewFields | JSONLDField                                                    // 239
	AllFields      = RichFields | ContentField                                                      // 255
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
	defaultTimeout     = 10 * time.Second
	defaultUserAgent   = "metagrab/2.0 (+https://github.com/josephgoksu/metagrab)"
	defaultMaxBodySize = 2 * 1024 * 1024 // 2 MB
	defaultConcurrency = 10
	defaultMaxURLLen   = 2048
	defaultRetryDelay  = 250 * time.Millisecond
)

var errBodyTooLarge = errors.New("response body exceeds configured limit")

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
}

// NewClient creates a Client with the given options applied.
func NewClient(opts ...Option) *Client {
	c := &Client{
		userAgent:   defaultUserAgent,
		maxBodySize: defaultMaxBodySize,
		concurrency: defaultConcurrency,
		timeout:     defaultTimeout,
		retryDelay:  defaultRetryDelay,
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

	if c.httpClient == nil {
		c.httpClient = &http.Client{
			Timeout: c.timeout,
			Transport: &http.Transport{
				MaxIdleConns:        256,
				MaxIdleConnsPerHost: 64,
				IdleConnTimeout:     90 * time.Second,
			},
		}
	}

	return c
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

	var resp *http.Response
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
		if err != nil {
			return nil, newFetchError(ErrorCodeRequestBuild, parsedURL.String(), "failed to build request", err)
		}
		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")

		resp, err = c.httpClient.Do(req)
		if err != nil {
			return nil, classifyContextOrNetworkError(parsedURL.String(), "request failed", err)
		}
		if !isRetryableStatus(resp.StatusCode) || attempt >= c.maxRetries {
			break
		}
		delay := c.retryDelay
		if d, ok := parseRetryAfter(resp.Header.Get("Retry-After")); ok {
			delay = d
		} else {
			delay = c.retryDelay * time.Duration(1<<attempt)
		}
		resp.Body.Close()
		if err := sleepWithContext(ctx, delay); err != nil {
			return nil, classifyContextOrNetworkError(parsedURL.String(), "retry wait interrupted", err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &FetchError{
			Code:       ErrorCodeHTTPStatus,
			URL:        parsedURL.String(),
			StatusCode: resp.StatusCode,
			Message:    "unexpected HTTP status",
		}
	}

	if err := validateContentType(resp.Header.Get("Content-Type")); err != nil {
		return nil, newFetchError(ErrorCodeContentType, parsedURL.String(), "unsupported content type", err)
	}

	if mask.Has(ContentField) {
		bodyBytes, err := readBodyWithLimit(resp.Body, c.maxBodySize)
		if err != nil {
			if errors.Is(err, errBodyTooLarge) {
				return nil, newFetchError(ErrorCodeBodyTooLarge, parsedURL.String(), "response body exceeded max size", err)
			}
			return nil, classifyContextOrReadError(parsedURL.String(), "failed to read response body", err)
		}

		doc, err := html.Parse(bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, newFetchError(ErrorCodeParseHTML, parsedURL.String(), "failed to parse HTML", err)
		}
		c.extractMetadata(doc, link, mask)
		link.Content = string(bodyBytes)
	} else {
		if err := c.extractMetadataFromHead(resp.Body, link, mask); err != nil {
			return nil, newFetchError(ErrorCodeParseHTML, parsedURL.String(), "failed to parse HTML head", err)
		}
	}

	if mask.Has(TitleField) && link.Title == "" {
		link.Title = parsedURL.Hostname()
	}

	if mask.Has(FaviconField) && link.Favicon == "" {
		link.Favicon = parsedURL.Scheme + "://" + parsedURL.Host + "/favicon.ico"
	}

	if mask.Has(MetaField) {
		if provider := matchOEmbedProvider(parsedURL); provider != nil {
			if data, err := c.fetchOEmbed(ctx, parsedURL.String(), provider); err == nil {
				link.OEmbed = data
				if mask.Has(TitleField) && link.Title == "" && data.Title != "" {
					link.Title = data.Title
				}
				if data.AuthorName != "" && link.SiteName == "" {
					link.SiteName = data.AuthorName
				}
			}
		}
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
		i, rawURL := i, rawURL
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
		i, rawURL := i, rawURL
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

func (c *Client) extractMetadata(n *html.Node, link *Link, mask Field) {
	if n == nil {
		return
	}

	var bestFav faviconCandidate
	var jsonLDCount int

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "title":
				if mask.Has(TitleField) && link.Title == "" {
					link.Title = textContent(node)
				}
			case "meta":
				c.processMeta(node, link, mask)
			case "link":
				c.processLink(node, link, mask, &bestFav)
			case "script":
				if mask.Has(JSONLDField) && jsonLDCount < maxJSONLDBlocks {
					jsonLDCount += c.processScript(node, link)
				}
			case "body":
				// JSON-LD can appear in body, so only skip body traversal when we don't need it.
				if !mask.Has(JSONLDField) {
					return
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)

	if bestFav.href != "" {
		link.Favicon = bestFav.href
	}
	if mask.Has(JSONLDField) && len(link.JSONLD) > 0 && link.ContentType == "" {
		link.ContentType = extractContentType(link.JSONLD)
	}
}

func (c *Client) processLink(node *html.Node, link *Link, mask Field, bestFav *faviconCandidate) {
	var rel, href, sizes string
	for _, a := range node.Attr {
		switch strings.ToLower(a.Key) {
		case "rel":
			rel = a.Val
		case "href":
			href = a.Val
		case "sizes":
			sizes = a.Val
		}
	}
	if href == "" {
		return
	}

	lowerRel := strings.ToLower(strings.TrimSpace(rel))

	if mask.Has(CanonicalField) && lowerRel == "canonical" && link.CanonicalURL == "" {
		link.CanonicalURL = resolveURL(link.URL, href)
	}

	if mask.Has(FaviconField) {
		prio := faviconPriority(rel)
		if prio > 0 {
			resolved := resolveURL(link.URL, href)
			cand := faviconCandidate{href: resolved, priority: prio, size: parseSizes(sizes)}
			if cand.betterThan(*bestFav) {
				*bestFav = cand
			}
		}
	}
}

func (c *Client) processScript(node *html.Node, link *Link) int {
	isLDJSON := false
	for _, a := range node.Attr {
		if strings.ToLower(a.Key) == "type" && strings.ToLower(strings.TrimSpace(a.Val)) == "application/ld+json" {
			isLDJSON = true
			break
		}
	}
	if !isLDJSON {
		return 0
	}

	raw := textContent(node)
	blocks := parseJSONLDAll(raw)
	if len(blocks) == 0 {
		return 0
	}
	link.JSONLD = append(link.JSONLD, blocks...)
	return len(blocks)
}

func (c *Client) processMeta(node *html.Node, link *Link, mask Field) {
	var name, property, content string
	for _, a := range node.Attr {
		k := strings.ToLower(a.Key)
		switch k {
		case "name":
			name = a.Val
		case "property":
			property = a.Val
		case "content":
			content = a.Val
		}
	}
	if content == "" {
		return
	}
	c.applyMetaTag(name, property, content, link, mask)
}

func (c *Client) applyMetaTag(name, property, content string, link *Link, mask Field) {
	key := property
	if key == "" {
		key = name
	}
	if key == "" {
		return
	}

	lowerKey := strings.ToLower(key)

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
			link.OpenGraph[lowerKey] = content

			if lowerKey == "og:site_name" && link.SiteName == "" {
				link.SiteName = content
			}
		}
	}

	if mask.Has(MetaField) && strings.HasPrefix(lowerKey, "article:") {
		if link.OpenGraph == nil {
			link.OpenGraph = make(map[string]string)
		}
		link.OpenGraph[lowerKey] = content

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
		link.Twitter[lowerKey] = content
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

func (c *Client) extractMetadataFromHead(r io.Reader, link *Link, mask Field) error {
	limited := io.LimitReader(r, c.maxBodySize)
	z := html.NewTokenizer(limited)

	var bestFav faviconCandidate
	var jsonLDCount int

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if err := z.Err(); err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			goto done
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := strings.ToLower(string(name))

			switch tag {
			case "body":
				goto done
			case "title":
				if mask.Has(TitleField) && link.Title == "" {
					if z.Next() == html.TextToken {
						link.Title = strings.TrimSpace(string(z.Text()))
					}
				}
			case "meta":
				var metaName, metaProperty, metaContent string
				for hasAttr {
					k, v, more := z.TagAttr()
					key := strings.ToLower(string(k))
					val := string(v)
					switch key {
					case "name":
						metaName = val
					case "property":
						metaProperty = val
					case "content":
						metaContent = val
					}
					hasAttr = more
				}
				if metaContent != "" {
					c.applyMetaTag(metaName, metaProperty, metaContent, link, mask)
				}
			case "link":
				var rel, href, sizes string
				for hasAttr {
					k, v, more := z.TagAttr()
					key := strings.ToLower(string(k))
					val := string(v)
					switch key {
					case "rel":
						rel = val
					case "href":
						href = val
					case "sizes":
						sizes = val
					}
					hasAttr = more
				}
				if href != "" {
					lowerRel := strings.ToLower(strings.TrimSpace(rel))
					if mask.Has(CanonicalField) && lowerRel == "canonical" && link.CanonicalURL == "" {
						link.CanonicalURL = resolveURL(link.URL, href)
					}
					if mask.Has(FaviconField) {
						prio := faviconPriority(rel)
						if prio > 0 {
							resolved := resolveURL(link.URL, href)
							cand := faviconCandidate{href: resolved, priority: prio, size: parseSizes(sizes)}
							if cand.betterThan(bestFav) {
								bestFav = cand
							}
						}
					}
				}
			case "script":
				if mask.Has(JSONLDField) && jsonLDCount < maxJSONLDBlocks {
					var isLDJSON bool
					for hasAttr {
						k, v, more := z.TagAttr()
						if strings.ToLower(string(k)) == "type" && strings.ToLower(strings.TrimSpace(string(v))) == "application/ld+json" {
							isLDJSON = true
						}
						hasAttr = more
					}
					if isLDJSON {
						// Read text content until </script>.
						var sb strings.Builder
						for {
							next := z.Next()
							if next == html.ErrorToken || next == html.EndTagToken {
								break
							}
							if next == html.TextToken {
								sb.Write(z.Text())
							}
						}
						blocks := parseJSONLDAll(sb.String())
						if len(blocks) > 0 {
							link.JSONLD = append(link.JSONLD, blocks...)
							jsonLDCount += len(blocks)
						}
					}
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if strings.EqualFold(string(name), "head") {
				goto done
			}
		}
	}

done:
	if bestFav.href != "" {
		link.Favicon = bestFav.href
	}
	if mask.Has(JSONLDField) && len(link.JSONLD) > 0 && link.ContentType == "" {
		link.ContentType = extractContentType(link.JSONLD)
	}
	return nil
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

// faviconPriority scores a link rel value for favicon preference.
func faviconPriority(rel string) int {
	switch strings.ToLower(strings.TrimSpace(rel)) {
	case "apple-touch-icon", "apple-touch-icon-precomposed":
		return 3
	case "icon":
		return 2
	case "shortcut icon":
		return 1
	default:
		return 0
	}
}

// parseSizes extracts the width from a sizes attribute like "180x180".
func parseSizes(sizes string) int {
	sizes = strings.TrimSpace(sizes)
	if sizes == "" || strings.EqualFold(sizes, "any") {
		return 0
	}
	parts := strings.SplitN(strings.ToLower(sizes), "x", 2)
	if len(parts) != 2 {
		return 0
	}
	w, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	return w
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

func textContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(sb.String())
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

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}

	if u.Hostname() == "" {
		return nil, errors.New("URL host is empty")
	}
	if u.User != nil {
		return nil, errors.New("URL userinfo is not allowed")
	}

	return u, nil
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
