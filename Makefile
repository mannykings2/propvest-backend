.PHONY: run build up down tidy test

run:
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

up:
	docker compose up -d

down:
	docker compose down

tidy:
	go mod tidy

test:
	go test ./... -race -cover