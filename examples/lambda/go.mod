module github.com/josephgoksu/metagrab/examples/lambda

go 1.26.0

require (
	github.com/aws/aws-lambda-go v1.55.1
	github.com/awslabs/aws-lambda-go-api-proxy v0.16.2
	github.com/josephgoksu/metagrab v0.0.0
)

require (
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

// For local development, point to the repo root.
// Remove this line when using a published version of metagrab.
replace github.com/josephgoksu/metagrab => ../..
