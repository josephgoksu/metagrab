package metagrab

import (
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

type Link struct {
	URL         string
	Title       string
	Description string
	Meta        map[string]string // og:title, twitter:*, etc.
	Content     string            // <body> text, optional
}

// tiny-integer bitmask
type Field uint8

const (
	TitleField       Field = 1 << iota // 1
	URLField                           // 2
	MetaField                          // 4
	DescriptionField                   // 8
	ContentField                       // 16
	AllFields        = TitleField | URLField | MetaField | DescriptionField | ContentField
)

// one byte, zero allocations, o(1) checks
// it's the idiomatic way to let callers choose which colums to hydrate without building clunky option structs
func (f Field) Has(x Field) bool { return f&x != 0 }

// --------- core client -------------

type client struct {
	http *http.Client
	pool sync.Pool
}

func newClient() *client {
	tr := &http.Transport{
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 64,
		IdleConnTimeout:     90 * time.Second,
	}

	return &client{
		http: &http.Client{
			Timeout:   10 * time.Second,
			Transport: tr,
		},
		pool: sync.Pool{
			New: func() any { return make([]byte, 0, 4096) }, // 4kb buffer
		},
	}
}

var c = newClient() // package-level singleton

// -----------

func Fetch(ctx context.Context, url string, mask Field) (*Link, error) {
	return c.fetch(ctx, url, mask)
}

func FetchBulk(ctx context.Context, urls []string, mask Field) ([]*Link, error) {
	return c.fetchBulk(ctx, urls, mask)
}

// ----- implementation -----

func (c *client) fetch(ctx context.Context, url string, mask Field) (*Link, error) {
	if url == "" {
		return nil, errors.New("empty URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	buf := c.pool.Get().([]byte)[:0]
	defer c.pool.Put(buf[:cap(buf)])

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)

	l := &Link{URL: url}

	if mask.Has(ContentField) {
		l.Content = html
	}
	if mask.Has(TitleField) {
		l.Title = first(html, `<title.*?>([^<]+)`)
	}
	if mask.Has(DescriptionField) {
		l.Description = first(html, `<meta\s+name=["']description["']\s+content=["']([^"']+)`)
	}
	if mask.Has(MetaField) {
		l.Meta = make(map[string]string)
		kvs := all(html, `<meta\s+(?:property|name)=["']([^"']+)["']\s+content=["']([^"']+)`)
		for i := 0; i < len(kvs); i += 2 {
			l.Meta[kvs[i]] = kvs[i+1]
		}
	}

	return l, nil
}

func (c *client) fetchBulk(ctx context.Context, urls []string, mask Field) ([]*Link, error) {
	g, gctx := errgroup.WithContext(ctx)
	out := make([]*Link, len(urls))
	for i, u := range urls {
		i, u := i, u
		g.Go(func() error {
			l, err := c.fetch(gctx, u, mask)
			if err != nil {
				return err
			}
			out[i] = l
			return nil
		})
	}
	return out, g.Wait()
}

// ---- helpers -----

func first(s, re string) string {
	r := regexp.MustCompile(re)
	if m := r.FindStringSubmatch(s); len(m) == 2 {
		return m[1]
	}
	return ""
}

func all(s, re string) []string {
	r := regexp.MustCompile(re)
	raw := r.FindAllStringSubmatch(s, -1)
	flat := make([]string, 0, len(raw)*2)
	for _, m := range raw {
		if len(m) == 3 {
			flat = append(flat, m[1], m[2])
		}
	}
	return flat
}
