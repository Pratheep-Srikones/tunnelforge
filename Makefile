.PHONY: all build build-server build-agent test clean

VERSION ?= 0.1.0

all: build

build: build-server build-agent

build-server:
	@echo "Building TunnelForge Server..."
	go build -ldflags="-s -w" -o bin/tunnelforge-server ./server

build-agent:
	@echo "Building TunnelForge Agent CLI..."
	go build -ldflags="-s -w" -o bin/forge ./agent/forge

build-cross-agent:
	@echo "Cross-compiling TunnelForge Agent CLI binaries..."
	mkdir -p bin/dist
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o bin/dist/forge-darwin-arm64 ./agent/forge
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o bin/dist/forge-darwin-amd64 ./agent/forge
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/dist/forge-linux-amd64 ./agent/forge
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/dist/forge-linux-arm64 ./agent/forge
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/dist/forge-windows-amd64.exe ./agent/forge

test:
	@echo "Running all test suites..."
	go test -v -count=1 ./...

lint:
	@echo "Running go vet..."
	go vet ./...

clean:
	rm -rf bin/
