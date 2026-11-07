package metagrab_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/josephgoksu/metagrab"
)

func ExampleFetch() {
	link, err := metagrab.Fetch(context.Background(), "https://go.dev", metagrab.PreviewFields)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(link.Title, link.Image, link.Favicon)
}

func ExampleParse() {
	html := `<html><head>
		<title>Gophers</title>
		<meta property="og:image" content="/img/gopher.png">
		<link rel="apple-touch-icon" href="/touch.png">
	</head></html>`

	link, err := metagrab.Parse(strings.NewReader(html), "https://example.com/blog/post", metagrab.PreviewFields)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(link.Title)
	fmt.Println(link.Image)
	fmt.Println(link.Favicon)
	// Output:
	// Gophers
	// https://example.com/img/gopher.png
	// https://example.com/touch.png
}

func ExampleNewClient() {
	client := metagrab.NewClient(
		metagrab.WithTimeout(5*time.Second),
		metagrab.WithRetries(2),
		metagrab.WithDenyPrivateNetworks(), // URLs come from users
	)
	link, err := client.Fetch(context.Background(), "https://example.com", metagrab.MetadataFields)
	if err != nil {
		var fe *metagrab.FetchError
		if errors.As(err, &fe) && fe.Code == metagrab.ErrorCodeURLDenied {
			log.Printf("blocked: %v", err)
		}
		return
	}
	fmt.Println(link.Title)
}

func ExampleClient_FetchBulkResults() {
	client := metagrab.NewClient(metagrab.WithConcurrency(20))
	urls := []string{"https://go.dev", "https://pkg.go.dev"}
	for _, r := range client.FetchBulkResults(context.Background(), urls, metagrab.PreviewFields) {
		if r.Error != nil {
			log.Printf("%s: %s", r.URL, r.Error.Code)
			continue
		}
		fmt.Println(r.Link.Title)
	}
}
