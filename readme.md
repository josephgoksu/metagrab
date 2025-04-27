<img src="./assets/logo.png" alt="metagrab" height="250" width="250" align="right" />

# metagrab

Fast, lightweight metadata scraper for URLs.
Written in Go. Perfect for embedding into Node.js, CLI tools, or microservices.

- Fetches Title, Description, OpenGraph, Twitter meta tags
- Tiny binary, ultra-fast execution
- Smart field selection (bitmask-powered)
- Ready for high-concurrency scraping

It might be useful for link preview generation, SEO crawlers, social sharing, AI agents

## Build

```bash
go build -o metagrab cmd/main.go
```

## Usage

```bash
./metagrab https://example.com
```

It's open source, so you can build it yourself
