# Build Stage
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

WORKDIR /src

# Copy go module files
COPY go.mod ./
RUN go mod download

# Copy source code
COPY . .

ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -ldflags="-s -w -X main.version=docker" -o /bnet-change-detector ./cmd/bnet-change-detector

# Runtime Stage (Alpine base for Docker & Podman)
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bnet-change-detector /usr/local/bin/bnet-change-detector

USER nobody:nobody

ENTRYPOINT ["/usr/local/bin/bnet-change-detector"]
CMD ["--mode=loop", "--interval=60s"]
