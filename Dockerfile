# syntax=docker/dockerfile:1
# metagrab HTTP API image.
#   docker build -t metagrab .
#   docker run --rm -p 8080:8080 -e METAGRAB_API_KEY=secret metagrab
FROM --platform=$BUILDPLATFORM golang:1.27-alpine3.24 AS build
ARG TARGETOS TARGETARCH VERSION=dev COMMIT=
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/metagrab ./cmd/metagrab

# distroless/static ships CA certificates and runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/metagrab /usr/local/bin/metagrab
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/metagrab"]
CMD ["serve"]
