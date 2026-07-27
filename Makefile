.PHONY: dev-up dev-down build test vet lint generate clean backup backup-run backup-verify backup-restore runbook-validate release-check

BACKUP ?= ./backups/latest
DB_PATH ?= ./svrtools.db
ARTIFACTS_DIR ?= ./data/artifacts

ifeq ($(OS),Windows_NT)
RELEASE_VALIDATE = powershell -NoProfile -ExecutionPolicy Bypass -File scripts/validate-release.ps1 -DistDir dist
RELEASE_LAYOUT_VALIDATE = powershell -NoProfile -ExecutionPolicy Bypass -File scripts/validate-release-layout.ps1 -DistDirectory dist
else
RELEASE_VALIDATE = sh ./scripts/validate-release.sh dist
RELEASE_LAYOUT_VALIDATE = sh ./scripts/validate-release-layout.sh dist
endif

dev-up:
	docker compose -f deploy/docker-compose/docker-compose.yml up -d

dev-down:
	docker compose -f deploy/docker-compose/docker-compose.yml down

build: cli api runner backup

cli:
	go build -o bin/vps.exe ./apps/cli

api:
	go build -o bin/api.exe ./apps/api

runner:
	go build -o bin/runner.exe ./apps/runner

backup:
	go build -o bin/backup.exe ./apps/api/cmd/backup

test:
	go test ./... -count=1 -timeout 30s

vet:
	go vet ./...

lint:
	golangci-lint run ./...

generate:
	buf generate
	sqlc generate

backup-run:
	go run ./apps/api/cmd/backup

backup-verify:
	go run ./apps/api/cmd/backup -mode verify -input "$(BACKUP)"

backup-restore:
	go run ./apps/api/cmd/backup -mode restore -input "$(BACKUP)" -db "$(DB_PATH)" -artifacts "$(ARTIFACTS_DIR)"

runbook-validate:
	go test ./runbooks -count=1

release-check:
	goreleaser check
	goreleaser release --snapshot --clean
	$(RELEASE_LAYOUT_VALIDATE)
	$(RELEASE_VALIDATE)

clean:
	rm -f svrtools.db vps.exe api.exe runner.exe

.PHONY: cli api runner
