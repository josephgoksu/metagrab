# metagrab — Cloudflare Container

Deploy metagrab as a [Cloudflare Container](https://developers.cloudflare.com/containers/) (public beta since June 2025). This runs a full Go binary on Cloudflare's edge — no WASM, no TinyGo constraints.

## How it works

1. **Dockerfile** builds the Go binary using the `examples/standalone` entrypoint
2. **Worker** (`worker/index.ts`) fronts the container using a `Container` Durable Object class
3. Requests hit the Worker, which routes them to a container instance
4. Container sleeps after 5 minutes of inactivity to save costs

## Prerequisites

- [Wrangler CLI](https://developers.cloudflare.com/workers/wrangler/) v3.60+
- Docker running locally (for image build)
- Cloudflare Workers Paid plan ($5/mo base)

## Setup

```bash
cd examples/cloudflare-container

# Install worker dependencies
npm install @cloudflare/containers

# Set the API key secret
wrangler secret put API_KEY
```

## Deploy

```bash
# Docker must be running — wrangler builds the image locally
wrangler deploy
```

Wrangler builds the Dockerfile targeting `linux/amd64`, pushes the image to Cloudflare's registry, and deploys the Worker + Container binding.

## Test

```bash
curl -X POST https://metagrab.<your-subdomain>.workers.dev/fetch \
  -H 'Content-Type: application/json' \
  -H 'X-API-Key: your-secret-key' \
  -d '{"url":"https://example.com","fields":15}'

curl https://metagrab.<your-subdomain>.workers.dev/health
```

## Instance types

| Type | RAM | vCPU | Use case |
|------|-----|------|----------|
| `dev` | 256 MB | 1/16 | HTML parsing (default) |
| `basic` | 1 GB | 1/4 | Medium workloads |
| `standard` | 4 GB | 1/2 | Heavy processing |

`dev` is more than enough for metadata extraction.

## Cost estimate

- Workers Paid plan: $5/mo base (includes 10M requests)
- Container compute: billed per GB-s of active time
- With `sleepAfter: "5m"`, idle containers cost nothing
- Low-volume usage fits comfortably in the $5/mo base plan
