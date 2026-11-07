package main

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"strings"
)

// basePlaceholder is replaced with the page's real base URL, so the
// canonical URL is absolute and equal to the URL being fetched, as on most
// real sites.
const basePlaceholder = "__BASE__"

// Page is one synthetic document in the benchmark corpus.
type Page struct {
	Name     string // short id used in charts
	Label    string // human description
	HTML     []byte // with basePlaceholder; use WithBase
	HeadSize int    // bytes up to and including </head>
}

// profile describes the shape of a page. Sizes are modelled on the
// HTTP Archive distribution of HTML document sizes: the median page is
// ~30 KB of HTML, large news and shop pages reach 0.5-2 MB because of
// inline scripts, styles and JSON state.
type profile struct {
	name, label   string
	inlineCSS     int // bytes of inline <style> in <head>
	inlineJS      int // bytes of inline <script> in <head>
	bodyBlocks    int // number of article/product blocks in <body>
	bodyScriptKB  int // inline JSON state at the end of <body>
	jsonLDInBody  bool
	extraHeadTags int
}

var profiles = []profile{
	{name: "small", label: "Small blog post", inlineCSS: 4_000, inlineJS: 1_000, bodyBlocks: 40, extraHeadTags: 10},
	{name: "medium", label: "Typical page", inlineCSS: 20_000, inlineJS: 15_000, bodyBlocks: 150, bodyScriptKB: 20, extraHeadTags: 30},
	{name: "large", label: "News article", inlineCSS: 60_000, inlineJS: 90_000, bodyBlocks: 600, bodyScriptKB: 200, jsonLDInBody: true, extraHeadTags: 80},
	{name: "huge", label: "Shop product page", inlineCSS: 120_000, inlineJS: 200_000, bodyBlocks: 1500, bodyScriptKB: 700, jsonLDInBody: true, extraHeadTags: 120},
}

var words = strings.Fields(`lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod
tempor incididunt ut labore et dolore magna aliqua enim ad minim veniam quis nostrud exercitation
ullamco laboris nisi aliquip ex ea commodo consequat duis aute irure in reprehenderit voluptate`)

func sentence(r *rand.Rand, n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteString(words[r.IntN(len(words))])
	}
	return sb.String()
}

func filler(r *rand.Rand, n int, line func(int) string) string {
	var sb strings.Builder
	for i := 0; sb.Len() < n; i++ {
		sb.WriteString(line(i))
		sb.WriteByte('\n')
	}
	_ = r
	return sb.String()
}

// Corpus builds the deterministic benchmark pages.
func Corpus() []Page {
	pages := make([]Page, 0, len(profiles))
	for i, p := range profiles {
		r := rand.New(rand.NewPCG(uint64(i+1), 42))
		var b strings.Builder
		b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + sentence(r, 8) + ` | Example Site</title>
<meta name="description" content="` + sentence(r, 25) + `">
<link rel="canonical" href="` + basePlaceholder + `/` + p.name + `">
<meta property="og:type" content="article">
<meta property="og:title" content="` + sentence(r, 8) + `">
<meta property="og:description" content="` + sentence(r, 25) + `">
<meta property="og:image" content="/images/cover-1200x630.jpg">
<meta property="og:image:width" content="1200"><meta property="og:image:height" content="630">
<meta property="og:url" content="` + basePlaceholder + `/` + p.name + `">
<meta property="og:site_name" content="Example Site">
<meta property="article:author" content="Jane Doe">
<meta property="article:published_time" content="2026-10-01T09:00:00Z">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:site" content="@example">
<meta name="twitter:image" content="https://cdn.example.com/cover.jpg">
<link rel="icon" href="/favicon-32.png" sizes="32x32">
<link rel="icon" href="/favicon-16.png" sizes="16x16">
<link rel="apple-touch-icon" href="/apple-touch-icon.png" sizes="180x180">
`)
		for j := 0; j < p.extraHeadTags; j++ {
			switch j % 4 {
			case 0:
				fmt.Fprintf(&b, `<link rel="preload" href="/static/chunk-%d.js" as="script">`+"\n", j)
			case 1:
				fmt.Fprintf(&b, `<meta name="x-tracking-%d" content="%s">`+"\n", j, sentence(r, 4))
			case 2:
				fmt.Fprintf(&b, `<link rel="stylesheet" href="/static/style-%d.css">`+"\n", j)
			default:
				fmt.Fprintf(&b, `<link rel="alternate" hreflang="x-%d" href="/x-%d/">`+"\n", j, j)
			}
		}
		b.WriteString("<style>\n" + filler(r, p.inlineCSS, func(k int) string {
			return fmt.Sprintf(".c%d{margin:%dpx;padding:%dpx;color:#%06x}", k, k%32, k%16, k*2654435761%0xffffff)
		}) + "</style>\n")
		b.WriteString("<script>\n" + filler(r, p.inlineJS, func(k int) string {
			return fmt.Sprintf("window.__cfg%d={id:%d,label:%q,enabled:%t};", k, k, sentence(r, 3), k%2 == 0)
		}) + "</script>\n")
		if !p.jsonLDInBody {
			b.WriteString(`<script type="application/ld+json">{"@context":"https://schema.org","@type":"Article","headline":"` + sentence(r, 6) + `","author":{"@type":"Person","name":"Jane Doe"}}</script>` + "\n")
		}
		b.WriteString("</head>\n")
		head := b.Len()

		b.WriteString(`<body><header><nav>`)
		for j := 0; j < 20; j++ {
			fmt.Fprintf(&b, `<a href="/section/%d">%s</a>`, j, sentence(r, 2))
		}
		b.WriteString(`</nav></header><main>`)
		for j := 0; j < p.bodyBlocks; j++ {
			fmt.Fprintf(&b, `<section class="c%d"><h2>%s</h2><p>%s</p><img src="/img/%d.jpg" alt="%s" loading="lazy"><p>%s <a href="/link/%d">%s</a></p></section>`+"\n",
				j%50, sentence(r, 6), sentence(r, 60), j, sentence(r, 3), sentence(r, 30), j, sentence(r, 2))
		}
		b.WriteString(`</main>`)
		if p.jsonLDInBody {
			b.WriteString(`<script type="application/ld+json">{"@context":"https://schema.org","@type":"Product","name":"` + sentence(r, 4) + `","offers":{"@type":"Offer","price":"19.99","priceCurrency":"GBP"}}</script>` + "\n")
		}
		if p.bodyScriptKB > 0 {
			b.WriteString(`<script id="__STATE__" type="application/json">[`)
			for k := 0; b.Len()-head < p.bodyBlocks*600+p.bodyScriptKB*1000; k++ {
				fmt.Fprintf(&b, `{"id":%d,"t":%q},`, k, sentence(r, 8))
			}
			b.WriteString(`{}]</script>`)
		}
		b.WriteString("</body></html>\n")
		pages = append(pages, Page{Name: p.name, Label: p.label, HTML: []byte(b.String()), HeadSize: head})
	}
	return pages
}

// WithBase returns the page HTML with its base URL filled in.
func (p Page) WithBase(base string) []byte {
	return bytes.ReplaceAll(p.HTML, []byte(basePlaceholder), []byte(base))
}
