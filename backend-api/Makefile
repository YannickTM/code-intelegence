.PHONY: build clean test lint

build:
	go build -o build/api ./cmd/api

clean:
	rm -rf build/

test:
	go test ./internal/...

lint:
	go vet ./...
