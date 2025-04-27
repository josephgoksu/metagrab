package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/josephgoksu/metagrab"
)

func main() {
	start := time.Now()
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: grabber <url>")
		os.Exit(1)
	}
	url := os.Args[1]

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	link, err := metagrab.Fetch(ctx, url, metagrab.AllFields)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(2)
	}

	if err := json.NewEncoder(os.Stdout).Encode(link); err != nil {
		fmt.Fprintln(os.Stderr, "JSON error:", err)
		os.Exit(3)
	}

	fmt.Fprintf(os.Stderr, "Took %s\n", time.Since(start))
}
