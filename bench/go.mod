module github.com/josephgoksu/metagrab/bench

go 1.26.0

replace github.com/josephgoksu/metagrab => ../

require (
	github.com/PuerkitoBio/goquery v1.13.0
	github.com/badoux/goscraper v0.0.0-20190827161153-36995ce6b19f
	github.com/dyatlov/go-opengraph/opengraph v0.0.0-20220524092352-606d7b1e5f8a
	github.com/josephgoksu/metagrab v0.0.0-20260303082529-292a408eb0c4
	github.com/otiai10/opengraph/v2 v2.2.0
	golang.org/x/text v0.43.0
)

require (
	github.com/andybalholm/cascadia v1.3.4 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
)
