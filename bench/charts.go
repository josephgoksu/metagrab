package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

// Colors from the reference data-viz palette: one accent for metagrab,
// a recessive gray for every other library (emphasis, not identity).
const svgStyle = `<style>
  .surface{fill:#fcfcfb} .title{fill:#0b0b0b;font:600 15px system-ui,-apple-system,Segoe UI,sans-serif}
  .sub{fill:#52514e;font:12px system-ui,-apple-system,Segoe UI,sans-serif}
  .panel{fill:#0b0b0b;font:600 13px system-ui,-apple-system,Segoe UI,sans-serif}
  .lbl{fill:#52514e;font:12px system-ui,-apple-system,Segoe UI,sans-serif}
  .lbl.hi{fill:#0b0b0b;font-weight:600}
  .val{fill:#52514e;font:12px ui-monospace,SFMono-Regular,Menlo,monospace}
  .val.hi{fill:#0b0b0b;font-weight:600}
  .bar{fill:#b5b3ab} .bar.hi{fill:#2a78d6} .axis{stroke:#c3c2b7;stroke-width:1}
  @media (prefers-color-scheme: dark){
    .surface{fill:#1a1a19} .title,.panel,.lbl.hi,.val.hi{fill:#f0efec} .sub,.lbl,.val{fill:#c3c2b7}
    .bar{fill:#5c5b56} .bar.hi{fill:#3987e5} .axis{stroke:#383835}
  }
</style>`

type bar struct {
	label string
	value float64
	text  string
	hi    bool
}

type panel struct {
	title string
	bars  []bar
}

// smallMultiples draws one horizontal bar chart per panel in a 2-column grid.
// Each panel has its own scale; values are labelled at the bar end.
func smallMultiples(title, subtitle string, panels []panel) string {
	const (
		width    = 820
		colW     = 400
		labelW   = 150
		barMax   = 170
		rowH     = 24
		barH     = 14
		top      = 64
		panelGap = 28
	)
	rows := 0
	for _, p := range panels {
		rows = max(rows, len(p.bars))
	}
	panelH := 26 + rows*rowH
	gridRows := (len(panels) + 1) / 2
	height := top + gridRows*(panelH+panelGap)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">`, width, height, width, height, html.EscapeString(title))
	b.WriteString(svgStyle)
	fmt.Fprintf(&b, `<rect class="surface" width="%d" height="%d" rx="8"/>`, width, height)
	fmt.Fprintf(&b, `<text class="title" x="16" y="28">%s</text>`, html.EscapeString(title))
	fmt.Fprintf(&b, `<text class="sub" x="16" y="47">%s</text>`, html.EscapeString(subtitle))

	for i, p := range panels {
		x0 := 16 + (i%2)*colW
		y0 := top + (i/2)*(panelH+panelGap)
		fmt.Fprintf(&b, `<text class="panel" x="%d" y="%d">%s</text>`, x0, y0+14, html.EscapeString(p.title))
		maxV := 0.0
		for _, br := range p.bars {
			maxV = max(maxV, br.value)
		}
		bx := x0 + labelW
		fmt.Fprintf(&b, `<line class="axis" x1="%d" y1="%d" x2="%d" y2="%d"/>`, bx, y0+24, bx, y0+24+len(p.bars)*rowH-6)
		for j, br := range p.bars {
			y := y0 + 26 + j*rowH
			cls := ""
			if br.hi {
				cls = " hi"
			}
			w := 2.0
			if maxV > 0 {
				w = max(2, br.value/maxV*barMax)
			}
			fmt.Fprintf(&b, `<text class="lbl%s" x="%d" y="%d" text-anchor="end">%s</text>`, cls, bx-8, y+barH-3, html.EscapeString(br.label))
			// Square at the axis, 4px rounded at the data end.
			fmt.Fprintf(&b, `<path class="bar%s" d="M%d %d h%.1f a4 4 0 0 1 4 4 v%d a4 4 0 0 1 -4 4 h-%.1f z"><title>%s: %s</title></path>`,
				cls, bx, y, max(0, w-4), barH-8, max(0, w-4), html.EscapeString(br.label), html.EscapeString(br.text))
			fmt.Fprintf(&b, `<text class="val%s" x="%.1f" y="%d">%s</text>`, cls, float64(bx)+w+6, y+barH-3, html.EscapeString(br.text))
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func fmtDuration(ns float64) string {
	switch {
	case ns >= 1e6:
		return fmt.Sprintf("%.2f ms", ns/1e6)
	default:
		return fmt.Sprintf("%.0f µs", ns/1e3)
	}
}

func fmtBytes(n float64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.2f MB", n/(1<<20))
	default:
		return fmt.Sprintf("%.0f KB", n/(1<<10))
	}
}

func lookup(rep Report, mode, page, lib string) (Measurement, bool) {
	for _, m := range rep.Results {
		if m.Mode == mode && m.Page == page && m.Lib == lib && m.Error == "" {
			return m, true
		}
	}
	return Measurement{}, false
}

func buildPanels(rep Report, mode string, value func(Measurement) float64, format func(float64) string) []panel {
	var panels []panel
	for _, p := range rep.Pages {
		pn := panel{title: fmt.Sprintf("%s · %s", pageTitle(p.Name), fmtBytes(float64(p.Bytes)))}
		for _, lib := range Libs() {
			m, ok := lookup(rep, mode, p.Name, lib.Name)
			if !ok {
				continue
			}
			v := value(m)
			pn.bars = append(pn.bars, bar{label: lib.Name, value: v, text: format(v), hi: strings.HasPrefix(lib.Name, "metagrab")})
		}
		panels = append(panels, pn)
	}
	return panels
}

func pageTitle(name string) string {
	return map[string]string{"small": "Small blog post", "medium": "Typical page", "large": "News article", "huge": "Shop product page"}[name]
}

func writeCharts(rep Report, dir string) error {
	charts := map[string]string{
		"fetch-time.svg": smallMultiples(
			"Time to fetch one link preview (lower is better)",
			"Local HTTP server, median of runs. metagrab = PreviewFields, rich = PreviewFields + JSON-LD.",
			buildPanels(rep, "fetch", func(m Measurement) float64 { return m.NsPerOp }, fmtDuration)),
		"bytes-read.svg": smallMultiples(
			"Response bytes read per preview (lower is better)",
			"metagrab stops after </head>; rich keeps scanning <body> for JSON-LD only.",
			buildPanels(rep, "fetch", func(m Measurement) float64 { return float64(m.ReadPerOp) }, fmtBytes)),
		"parse-memory.svg": smallMultiples(
			"Memory allocated to parse one page (lower is better)",
			"HTML already in memory; no network. Garbage the Go runtime must collect per page.",
			buildPanels(rep, "parse", func(m Measurement) float64 { return float64(m.BytesOp) }, fmtBytes)),
		"parse-time.svg": smallMultiples(
			"Time to parse one page already in memory (lower is better)",
			"CPU only, no network. goscraper is missing: it can only parse pages it downloads.",
			buildPanels(rep, "parse", func(m Measurement) float64 { return m.NsPerOp }, fmtDuration)),
	}
	for name, svg := range charts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(svg+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}
