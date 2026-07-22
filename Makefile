BINARY_NAME=bnet-change-detector
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags="-s -w -X main.version=$(VERSION)"

.PHONY: all build test clean cross-compile docker-build

all: test build

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/bnet-change-detector

test:
	go test -v ./...

clean:
	rm -rf bin/

cross-compile:
	@mkdir -p bin/
	# Windows x64
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-windows-amd64.exe ./cmd/bnet-change-detector
	# Windows ARM64
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-windows-arm64.exe ./cmd/bnet-change-detector
	# Linux x64
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/bnet-change-detector
	# Linux ARM64
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/bnet-change-detector
	# Linux ARMv7
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-armv7 ./cmd/bnet-change-detector

docker-build:
	docker build -t $(BINARY_NAME):latest .
