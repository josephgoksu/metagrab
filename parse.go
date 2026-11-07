package metagrab

import (
	"bytes"
	"errors"
	"io"

	"golang.org/x/net/html"
)

// Parse extracts metadata from an HTML document you already have, such as a
// cached page or a crawler's response body, using the default client's
// limits. pageURL is the document's URL: it is used to resolve relative
// links and for the title and favicon fallbacks. Parse makes no network
// requests, so oEmbed data is never fetched.
func Parse(r io.Reader, pageURL string, mask Field) (*Link, error) {
	return defaultClient.Parse(r, pageURL, mask)
}

// Parse extracts metadata from an HTML document. See the package-level Parse.
// The client's max body size applies. The encoding is detected from a BOM or
// a <meta charset>; undeclared documents are read as UTF-8.
func (c *Client) Parse(r io.Reader, pageURL string, mask Field) (*Link, error) {
	if mask == 0 {
		mask = AllFields
	}
	u, err := validateURL(pageURL)
	if err != nil {
		return nil, newFetchError(ErrorCodeInvalidURL, pageURL, "invalid URL", err)
	}
	link := &Link{URL: u.String()}
	if mask == URLField {
		return link, nil
	}
	if ferr := c.parseInto(r, link, link.URL, "", mask); ferr != nil {
		ferr.URL = link.URL
		return nil, ferr
	}
	if mask.Has(FaviconField) && link.Favicon == "" {
		link.Favicon = resolveURL(link.URL, "/favicon.ico")
	}
	if mask.Has(TitleField) && link.Title == "" {
		link.Title = u.Hostname()
	}
	return link, nil
}

// parseInto reads an HTML document into link. With ContentField it buffers
// and parses the whole document (up to maxBodySize); otherwise it streams
// and stops as early as the mask allows.
func (c *Client) parseInto(r io.Reader, link *Link, baseURL, contentType string, mask Field) *FetchError {
	if mask.Has(ContentField) {
		rawBytes, err := readBodyWithLimit(r, c.maxBodySize)
		if err != nil {
			if errors.Is(err, errBodyTooLarge) {
				return newFetchError(ErrorCodeBodyTooLarge, "", "response body exceeded max size", err)
			}
			return classifyContextOrReadError("", "failed to read response body", err)
		}
		bodyBytes := decodeToUTF8(rawBytes, contentType)

		doc, err := html.Parse(bytes.NewReader(bodyBytes))
		if err != nil {
			return newFetchError(ErrorCodeParseHTML, "", "failed to parse HTML", err)
		}
		extractMetadata(doc, link, baseURL, mask)
		link.Content = string(bodyBytes)
		return nil
	}

	ur := utf8Reader(io.LimitReader(r, c.maxBodySize), contentType)
	if err := extractMetadataFromHead(ur, link, baseURL, mask); err != nil {
		return classifyContextOrReadError("", "failed to read HTML head", err)
	}
	return nil
}
