.PHONY: run build test tidy docker-up docker-down docker-logs lint

run:
	air

build:
	go build -o bin/hashvault ./cmd/api

test:
	go test ./... -v -race -count=1

tidy:
	go mod tidy

lint:
	golangci-lint run ./...

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f

docker-reset:
	docker compose down -v

.DEFAULT_GOAL := run
