FROM golang:1.25-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /metagrab ./examples/standalone

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /metagrab /usr/local/bin/metagrab
EXPOSE 8080
ENTRYPOINT ["metagrab"]
