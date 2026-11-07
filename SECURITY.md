# Security policy

## Supported versions

Only the latest minor release gets security fixes.

## Reporting a vulnerability

Please do not open a public issue. Use GitHub's
[private vulnerability reporting](https://github.com/josephgoksu/metagrab/security/advisories/new)
for this repository. You should get a reply within 5 working days.

## Running metagrab safely

metagrab fetches URLs, so when those URLs come from users it is an SSRF risk.

- Use `metagrab.WithDenyPrivateNetworks()` (on by default in `metagrab serve`).
  It checks every connection after DNS resolution, including redirects.
- If you pass your own `http.Client`, you are responsible for the same checks.
- Set an API key (`METAGRAB_API_KEY`) and rate-limit the HTTP API at your edge.
- Treat everything metagrab returns as untrusted input. oEmbed `html` and
  extracted strings can contain markup: escape them before rendering.
