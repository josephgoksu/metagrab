package metagrab

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// OEmbedData holds structured data from an oEmbed endpoint.
type OEmbedData struct {
	Type         string `json:"type"`
	Title        string `json:"title,omitempty"`
	AuthorName   string `json:"author_name,omitempty"`
	AuthorURL    string `json:"author_url,omitempty"`
	ProviderName string `json:"provider_name,omitempty"`
	ProviderURL  string `json:"provider_url,omitempty"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	HTML         string `json:"html,omitempty"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
}

type oembedProvider struct {
	name     string
	hosts    []string // exact hostname matches (lowercase)
	endpoint string   // oEmbed endpoint URL with {url} placeholder
}

const maxOEmbedBody = 64 * 1024

// oembedRegistry is the compiled-in list of supported oEmbed providers.
var oembedRegistry = []oembedProvider{
	// Video
	{
		name:     "YouTube",
		hosts:    []string{"www.youtube.com", "youtube.com", "youtu.be", "m.youtube.com"},
		endpoint: "https://www.youtube.com/oembed?url={url}&format=json",
	},
	{
		name:     "Vimeo",
		hosts:    []string{"vimeo.com", "www.vimeo.com"},
		endpoint: "https://vimeo.com/api/oembed.json?url={url}",
	},
	{
		name:     "Dailymotion",
		hosts:    []string{"www.dailymotion.com", "dailymotion.com"},
		endpoint: "https://www.dailymotion.com/services/oembed?url={url}&format=json",
	},
	{
		name:     "TikTok",
		hosts:    []string{"www.tiktok.com", "tiktok.com"},
		endpoint: "https://www.tiktok.com/oembed?url={url}",
	},
	// Audio
	{
		name:     "Spotify",
		hosts:    []string{"open.spotify.com"},
		endpoint: "https://open.spotify.com/oembed?url={url}",
	},
	{
		name:     "SoundCloud",
		hosts:    []string{"soundcloud.com", "www.soundcloud.com"},
		endpoint: "https://soundcloud.com/oembed?url={url}&format=json",
	},

	// Social
	{
		name:     "Twitter/X",
		hosts:    []string{"twitter.com", "www.twitter.com", "x.com", "www.x.com"},
		endpoint: "https://publish.twitter.com/oembed?url={url}",
	},
	{
		name:     "Reddit",
		hosts:    []string{"www.reddit.com", "reddit.com", "old.reddit.com"},
		endpoint: "https://www.reddit.com/oembed?url={url}",
	},

	// Code
	{
		name:     "CodePen",
		hosts:    []string{"codepen.io", "www.codepen.io"},
		endpoint: "https://codepen.io/api/oembed?url={url}&format=json",
	},
	{
		name:     "CodeSandbox",
		hosts:    []string{"codesandbox.io", "www.codesandbox.io"},
		endpoint: "https://codesandbox.io/oembed?url={url}",
	},
	{
		name:     "JSFiddle",
		hosts:    []string{"jsfiddle.net", "www.jsfiddle.net"},
		endpoint: "https://jsfiddle.net/oembed?url={url}&format=json",
	},
	{
		name:     "Replit",
		hosts:    []string{"replit.com", "www.replit.com", "repl.it"},
		endpoint: "https://replit.com/data/oembed?url={url}",
	},

	// Design
	{
		name:     "Figma",
		hosts:    []string{"www.figma.com", "figma.com"},
		endpoint: "https://www.figma.com/api/oembed?url={url}",
	},
	// Docs & Presentations
	{
		name:     "SlideShare",
		hosts:    []string{"www.slideshare.net", "slideshare.net"},
		endpoint: "https://www.slideshare.net/api/oembed/2?url={url}&format=json",
	},
	{
		name:     "Speaker Deck",
		hosts:    []string{"speakerdeck.com", "www.speakerdeck.com"},
		endpoint: "https://speakerdeck.com/oembed.json?url={url}",
	},

	// Media
	{
		name:     "Flickr",
		hosts:    []string{"www.flickr.com", "flickr.com", "flic.kr"},
		endpoint: "https://www.flickr.com/services/oembed?url={url}&format=json",
	},
	{
		name:     "Giphy",
		hosts:    []string{"giphy.com", "www.giphy.com", "media.giphy.com"},
		endpoint: "https://giphy.com/services/oembed?url={url}",
	},
	{
		name:     "Imgur",
		hosts:    []string{"imgur.com", "www.imgur.com", "i.imgur.com"},
		endpoint: "https://api.imgur.com/oembed.json?url={url}",
	},

	// Other
	{
		name:     "Loom",
		hosts:    []string{"www.loom.com", "loom.com"},
		endpoint: "https://www.loom.com/v1/oembed?url={url}",
	},
	{
		name:     "Miro",
		hosts:    []string{"miro.com", "www.miro.com"},
		endpoint: "https://miro.com/api/v1/oembed?url={url}",
	},
	{
		name:     "Kickstarter",
		hosts:    []string{"www.kickstarter.com", "kickstarter.com"},
		endpoint: "https://www.kickstarter.com/services/oembed?url={url}",
	},
	{
		name:     "Mixcloud",
		hosts:    []string{"www.mixcloud.com", "mixcloud.com"},
		endpoint: "https://www.mixcloud.com/oembed/?url={url}&format=json",
	},
	{
		name:     "Scribd",
		hosts:    []string{"www.scribd.com", "scribd.com"},
		endpoint: "https://www.scribd.com/services/oembed?url={url}&format=json",
	},
}

// matchOEmbedProvider returns the first provider matching the URL, or nil.
func matchOEmbedProvider(u *url.URL) *oembedProvider {
	host := strings.ToLower(u.Hostname())
	for i := range oembedRegistry {
		for _, h := range oembedRegistry[i].hosts {
			if host == h {
				return &oembedRegistry[i]
			}
		}
	}
	return nil
}

// fetchOEmbed fetches oEmbed data from the provider endpoint.
func (c *Client) fetchOEmbed(ctx context.Context, rawURL string, provider *oembedProvider) (*OEmbedData, error) {
	endpoint := strings.Replace(provider.endpoint, "{url}", url.QueryEscape(rawURL), 1)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &FetchError{
			Code:       ErrorCodeHTTPStatus,
			URL:        endpoint,
			StatusCode: resp.StatusCode,
			Message:    "oEmbed endpoint returned non-200",
		}
	}

	var data OEmbedData
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxOEmbedBody)).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
}
