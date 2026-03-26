.PHONY: build clean test lint integration-test

build:
	CGO_ENABLED=1 go build -o build/worker ./cmd/worker

clean:
	rm -rf build/

test:
	CGO_ENABLED=1 go test ./...

lint:
	go vet ./...

integration-test:
	CGO_ENABLED=1 go test -tags integration -v -timeout 120s ./internal/parser/...
