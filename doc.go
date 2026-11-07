// Package metagrab extracts link-preview metadata from web pages: title,
// description, Open Graph, Twitter Cards, preview image, favicon, canonical
// URL, author and date, JSON-LD and oEmbed.
//
// Fetch downloads a page and returns a [Link]. A [Field] bitmask selects what
// to extract, and the presets cover the common cases:
//
//   - [MetadataFields]: title, description, Open Graph, Twitter, image
//   - [PreviewFields]: MetadataFields plus favicon and canonical URL
//   - [RichFields]: PreviewFields plus JSON-LD
//   - [AllFields]: RichFields plus the full HTML
//
// Unless the mask asks for JSON-LD or the full HTML, metagrab stops reading
// at the end of <head>, so large pages cost little more than small ones.
//
// Use [Parse] when you already have the HTML.
//
// When URLs come from users, create the client with
// [WithDenyPrivateNetworks] to block requests to private networks,
// including through redirects and DNS rebinding.
//
// Every error is a [*FetchError] with a machine-readable [ErrorCode].
//
// The httphandler sub-package serves the same features over HTTP, and
// cmd/metagrab is a CLI that can also run that server.
package metagrab
