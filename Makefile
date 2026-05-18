.PHONY: dev-up dev-down migrate seed test lint generate cli api runner

dev-up:
	docker compose -f deploy/docker-compose/docker-compose.yml up -d

dev-down:
	docker compose -f deploy/docker-compose/docker-compose.yml down

migrate:
	go run apps/api/cmd/migrate/main.go -- up

seed:
	go run apps/api/cmd/seed/main.go

test:
	go test ./...

lint:
	golangci-lint run ./...

generate:
	buf generate

cli:
	go build -o bin/vps ./apps/cli

api:
	go build -o bin/api ./apps/api

runner:
	go build -o bin/runner ./apps/runner
