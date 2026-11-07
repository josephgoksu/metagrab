# metagrab — AWS Lambda

Deploy metagrab as an AWS Lambda function using the `provided.al2023` runtime.

## Prerequisites

- Go 1.26+
- AWS CLI configured
- IAM role with Lambda execution permissions

## Build

```bash
cd examples/lambda
go mod tidy
GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bootstrap .
zip function.zip bootstrap
```

## Deploy

```bash
# Create function
aws lambda create-function \
  --function-name metagrab \
  --runtime provided.al2023 \
  --architectures arm64 \
  --handler bootstrap \
  --zip-file fileb://function.zip \
  --role arn:aws:iam::ACCOUNT_ID:role/lambda-execution-role \
  --environment 'Variables={API_KEY=your-secret-key}' \
  --timeout 30 \
  --memory-size 256

# Update code
aws lambda update-function-code \
  --function-name metagrab \
  --zip-file fileb://function.zip
```

## Add Function URL (optional)

```bash
aws lambda create-function-url-config \
  --function-name metagrab \
  --auth-type NONE
```

Or use API Gateway for more control over routing, throttling, and auth.

## Test

```bash
FUNCTION_URL=$(aws lambda get-function-url-config --function-name metagrab --query 'FunctionUrl' --output text)

curl -X POST "$FUNCTION_URL/fetch" \
  -H 'Content-Type: application/json' \
  -H 'X-API-Key: your-secret-key' \
  -d '{"url":"https://example.com","fields":15}'
```

## Cost

Lambda free tier includes 1M requests/month and 400,000 GB-seconds. At 256 MB memory, that's ~1.5M invocations free. Beyond free tier, ~$0.20 per million requests.
