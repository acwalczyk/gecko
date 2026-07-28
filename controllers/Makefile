.PHONY: build test lint docker-build

BINARY_NAME=gecko-controllers
IMAGE=quay.io/gcphcp/gecko-controllers

build:
	go build -o bin/$(BINARY_NAME) .

test:
	go test ./...

lint:
	golangci-lint run ./...

docker-build:
	docker build -t $(IMAGE):latest .
