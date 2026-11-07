FROM golang:1.27-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /metagrab ./examples/standalone

FROM alpine:3.24
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 metagrab
COPY --from=build /metagrab /usr/local/bin/metagrab
USER metagrab
EXPOSE 8080
ENTRYPOINT ["metagrab"]
