.PHONY: generate build test

generate:
	@echo "Generating public APIs, schemas, and deepcopy methods..."
	@go run ./cmd/orlop-gen
	@echo "Done!"

build:
	go build ./...

test:
	go test ./...