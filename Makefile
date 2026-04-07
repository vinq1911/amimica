VERSION ?= 0.1.0-dev

.PHONY: all build test lint vet bench clean

## all: Run lint, vet, test, and build.
all: lint vet test build

## build: Compile the amimica binary to bin/amimica.
build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/amimica ./cmd/amimica

## test: Run all tests with race detection.
test:
	go test ./... -race -count=1

## lint: Run golangci-lint on all packages.
lint:
	golangci-lint run ./...

## vet: Run go vet on all packages.
vet:
	go vet ./...

## bench: Run all benchmarks.
bench:
	go test -bench=. -benchmem ./...

## clean: Remove the bin/ directory.
clean:
	rm -rf bin/
