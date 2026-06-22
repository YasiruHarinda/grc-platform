BINARY := bin/server

.PHONY: all setup vet test build clean

all: build

setup:
	git -C "$(shell git rev-parse --show-toplevel)" config core.hooksPath .githooks

vet:
	go vet ./...

test: vet
	go test -race ./...

build: test
	mkdir -p bin
	go build -o $(BINARY) ./cmd/server

clean:
	rm -rf bin
