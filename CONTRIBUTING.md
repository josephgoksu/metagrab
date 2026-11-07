# Contributing

Thanks for helping. Bug reports, fixes and new oEmbed providers are all welcome.

## Before you start

- For a bug, open an issue with the URL (or a minimal HTML sample), the field
  mask, what you expected and what you got.
- For a new feature, open an issue first so we can agree on the API.

## Development

```bash
git clone https://github.com/josephgoksu/metagrab
cd metagrab
go test -short -race ./...
```

Go 1.26 or newer is required. Read [AGENTS.md](AGENTS.md) for the layout and
the rules the code follows (no new root dependencies, no network in unit
tests, both parser paths must agree).

## Pull requests

- Keep each PR to one change, with tests.
- Run `gofmt`, `go vet ./...` and `go test -short -race ./...`.
- Use a Conventional Commit title, for example `fix: resolve relative og:image`.
- Add a line to `CHANGELOG.md` under **Unreleased** for user-visible changes.

## Adding an oEmbed provider

Add an entry to `oembedRegistry` in `oembed.go` with exact host names and the
provider's documented endpoint, and add a case to `TestOEmbedProviderMatch`.
