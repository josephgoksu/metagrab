// AWS Lambda example for metagrab.
//
// Build:
//
//	GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bootstrap .
//	zip function.zip bootstrap
//
// Deploy:
//
//	aws lambda create-function \
//	  --function-name metagrab \
//	  --runtime provided.al2023 \
//	  --architectures arm64 \
//	  --handler bootstrap \
//	  --zip-file fileb://function.zip \
//	  --role arn:aws:iam::ACCOUNT:role/lambda-role \
//	  --environment 'Variables={API_KEY=your-secret-key}'
package main

import (
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"
	"github.com/josephgoksu/metagrab"
	"github.com/josephgoksu/metagrab/httphandler"
)

func main() {
	client := metagrab.NewClient(
		metagrab.WithURLPolicy(metagrab.DenyPrivateIPs()),
	)

	h := httphandler.New(client,
		httphandler.WithAPIKey(os.Getenv("API_KEY")),
	)

	lambda.Start(httpadapter.New(h).ProxyWithContext)
}
