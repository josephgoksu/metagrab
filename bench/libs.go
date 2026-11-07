package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/badoux/goscraper"
	dyatlov "github.com/dyatlov/go-opengraph/opengraph"
	"github.com/josephgoksu/metagrab"
	otiai10 "github.com/otiai10/opengraph/v2"
)

// Result is the common shape every library's output is mapped to.
type Result struct {
	Title, Description, Image, Favicon, Canonical string
	SiteName, Author, Published, TwitterCard      string
	JSONLD                                        int
}

// Lib is one contender. Parse is nil when the library has no way to parse a
// document it did not download itself.
type Lib struct {
	Name   string
	Module string
	Parse  func(r io.Reader, pageURL string) (Result, error)
	Fetch  func(hc *http.Client, pageURL string) (Result, error)
}

func fromLink(l *metagrab.Link) Result {
	return Result{
		Title: l.Title, Description: l.Description, Image: l.Image, Favicon: l.Favicon,
		Canonical: l.CanonicalURL, SiteName: l.SiteName, Author: l.Author,
		Published: l.PublishedAt, TwitterCard: l.Twitter["twitter:card"], JSONLD: len(l.JSONLD),
	}
}

func metagrabLib(name string, mask metagrab.Field) Lib {
	clients := sync.Map{} // one client per *http.Client, like real usage
	client := func(hc *http.Client) *metagrab.Client {
		if c, ok := clients.Load(hc); ok {
			return c.(*metagrab.Client)
		}
		c, _ := clients.LoadOrStore(hc, metagrab.NewClient(metagrab.WithHTTPClient(hc)))
		return c.(*metagrab.Client)
	}
	return Lib{
		Name:   name,
		Module: "github.com/josephgoksu/metagrab",
		Parse: func(r io.Reader, u string) (Result, error) {
			l, err := metagrab.Parse(r, u, mask)
			if err != nil {
				return Result{}, err
			}
			return fromLink(l), nil
		},
		Fetch: func(hc *http.Client, u string) (Result, error) {
			l, err := client(hc).Fetch(context.Background(), u, mask)
			if err != nil {
				return Result{}, err
			}
			return fromLink(l), nil
		},
	}
}

func otiai10Parse(r io.Reader, u string) (Result, error) {
	og := otiai10.New(u)
	if err := og.Parse(r); err != nil {
		return Result{}, err
	}
	_ = og.ToAbs()
	res := Result{Title: og.Title, Description: og.Description, SiteName: og.SiteName, Favicon: og.Favicon.URL}
	if len(og.Image) > 0 {
		res.Image = og.Image[0].URL
	}
	return res, nil
}

func dyatlovParse(r io.Reader, _ string) (Result, error) {
	og := dyatlov.NewOpenGraph()
	if err := og.ProcessHTML(r); err != nil {
		return Result{}, err
	}
	res := Result{Title: og.Title, Description: og.Description, SiteName: og.SiteName}
	if len(og.Images) > 0 {
		res.Image = og.Images[0].URL
	}
	if og.Article != nil {
		if og.Article.PublishedTime != nil {
			res.Published = og.Article.PublishedTime.Format("2006-01-02T15:04:05Z07:00")
		}
		if len(og.Article.Authors) > 0 {
			res.Author = og.Article.Authors[0]
		}
	}
	return res, nil
}

// goqueryParse is the "write it yourself" baseline most projects start with.
func goqueryParse(r io.Reader, _ string) (Result, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return Result{}, err
	}
	meta := func(sel string) string { v, _ := doc.Find(sel).First().Attr("content"); return v }
	attr := func(sel, a string) string { v, _ := doc.Find(sel).First().Attr(a); return v }
	return Result{
		Title:       strings.TrimSpace(doc.Find("title").First().Text()),
		Description: meta(`meta[name="description"]`),
		Image:       meta(`meta[property="og:image"]`),
		Favicon:     attr(`link[rel~="icon"]`, "href"),
		Canonical:   attr(`link[rel="canonical"]`, "href"),
		SiteName:    meta(`meta[property="og:site_name"]`),
		Author:      meta(`meta[property="article:author"]`),
		Published:   meta(`meta[property="article:published_time"]`),
		TwitterCard: meta(`meta[name="twitter:card"]`),
		JSONLD:      doc.Find(`script[type="application/ld+json"]`).Length(),
	}, nil
}

// fetchWith downloads with hc and hands the body to parse, the way you would
// wire a parse-only library into a real service.
func fetchWith(parse func(io.Reader, string) (Result, error)) func(*http.Client, string) (Result, error) {
	return func(hc *http.Client, u string) (Result, error) {
		resp, err := hc.Get(u)
		if err != nil {
			return Result{}, err
		}
		defer resp.Body.Close()
		return parse(resp.Body, u)
	}
}

var goscraperMu sync.Mutex

func goscraperFetch(hc *http.Client, u string) (Result, error) {
	// goscraper always uses http.DefaultClient; swap its transport so the
	// byte counter sees the traffic.
	goscraperMu.Lock()
	defer goscraperMu.Unlock()
	old := http.DefaultClient.Transport
	http.DefaultClient.Transport = hc.Transport
	defer func() { http.DefaultClient.Transport = old }()

	d, err := goscraper.Scrape(u, 5)
	if err != nil {
		return Result{}, err
	}
	res := Result{Title: d.Preview.Title, Description: d.Preview.Description, Favicon: d.Preview.Icon, SiteName: d.Preview.Name}
	if len(d.Preview.Images) > 0 {
		res.Image = d.Preview.Images[0]
	}
	return res, nil
}

// Libs returns every contender, metagrab first.
func Libs() []Lib {
	return []Lib{
		metagrabLib("metagrab", metagrab.PreviewFields),
		metagrabLib("metagrab (rich)", metagrab.RichFields),
		{Name: "otiai10/opengraph", Module: "github.com/otiai10/opengraph/v2", Parse: otiai10Parse, Fetch: fetchWith(otiai10Parse)},
		{Name: "dyatlov/go-opengraph", Module: "github.com/dyatlov/go-opengraph/opengraph", Parse: dyatlovParse, Fetch: fetchWith(dyatlovParse)},
		{Name: "goquery (hand-rolled)", Module: "github.com/PuerkitoBio/goquery", Parse: goqueryParse, Fetch: fetchWith(goqueryParse)},
		{Name: "badoux/goscraper", Module: "github.com/badoux/goscraper", Fetch: goscraperFetch},
	}
}

func reader(b []byte) io.Reader { return bytes.NewReader(b) }
