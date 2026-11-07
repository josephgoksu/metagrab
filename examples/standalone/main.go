// Standalone HTTP server example for metagrab.
//
// Run:
//
//	API_KEY=secret go run .
//	curl -H 'X-API-Key: secret' -d '{"url":"https://example.com","fields":15}' http://localhost:8080/fetch
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/josephgoksu/metagrab"
	"github.com/josephgoksu/metagrab/httphandler"
)

func main() {
	client := metagrab.NewClient(
		metagrab.WithDenyPrivateNetworks(),
	)

	var opts []httphandler.HandlerOption
	if key := os.Getenv("API_KEY"); key != "" {
		opts = append(opts, httphandler.WithAPIKey(key))
	}

	h := httphandler.New(client, opts...)

	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	log.Printf("metagrab listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, h))
}
