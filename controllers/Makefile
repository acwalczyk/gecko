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
	# Must be run from the repo root: make -C controllers docker-build (or cd .. && docker build)
	cd .. && docker build -f controllers/Dockerfile -t $(IMAGE):latest .
