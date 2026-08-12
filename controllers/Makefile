.PHONY: build test lint

BINARY_NAME=gecko-controllers

build:
	go build -o bin/$(BINARY_NAME) .

test:
	go test ./...

lint:
	golangci-lint run ./...
