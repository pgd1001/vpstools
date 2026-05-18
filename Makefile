.PHONY: dev-up dev-down build test vet lint generate clean

dev-up:
	docker compose -f deploy/docker-compose/docker-compose.yml up -d

dev-down:
	docker compose -f deploy/docker-compose/docker-compose.yml down

build: cli api runner

cli:
	go build -o bin/vps.exe ./apps/cli

api:
	go build -o bin/api.exe ./apps/api

runner:
	go build -o bin/runner.exe ./apps/runner

test:
	go test ./... -count=1 -timeout 30s

vet:
	go vet ./...

lint:
	golangci-lint run ./...

generate:
	buf generate
	sqlc generate

clean:
	rm -f svrtools.db vps.exe api.exe runner.exe

.PHONY: cli api runner
