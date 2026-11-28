.PHONY: build test

build:
	go build -o bin/ltc-timeline ./cmd/ltc-timeline

test:
	go test ./...
