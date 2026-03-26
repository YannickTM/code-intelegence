.PHONY: test-unit test-integration build lint sqlc-generate backend-up backend-down

test-unit:
	cd backend-api && go test ./internal/...

test-integration:
	cd backend-api && go test -tags integration ./tests/integration/... -v -count=1

build:
	cd backend-api && $(MAKE) build
	cd backend-worker && $(MAKE) build

lint:
	cd backend-api && go vet ./...

sqlc-generate:
	sqlc generate

backend-up:
	docker compose -f docker-compose.backend.yaml up -d --build

backend-down:
	docker compose -f docker-compose.backend.yaml down
