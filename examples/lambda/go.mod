module github.com/josephgoksu/metagrab/examples/lambda

go 1.25.0

require (
	github.com/aws/aws-lambda-go v1.47.0
	github.com/awslabs/aws-lambda-go-api-proxy v0.16.2
	github.com/josephgoksu/metagrab v0.0.0
)

// For local development, point to the repo root.
// Remove this line when using a published version of metagrab.
replace github.com/josephgoksu/metagrab => ../..
