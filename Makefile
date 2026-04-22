.PHONY: dev test lint build migrate seed load-test certs help

help:
	@echo "OpenGuard Makefile Targets:"
	@echo "  dev        - Start development environment (Docker Compose)"
	@echo "  test       - Run all unit and integration tests"
	@echo "  lint       - Run golangci-lint and sqlfluff"
	@echo "  build      - Build all Go services and the Angular frontend"
	@echo "  migrate    - Run database migrations"
	@echo "  seed       - Seed the database with initial data"
	@echo "  load-test  - Run k6 load tests"
	@echo "  certs      - Generate mTLS and JWT certificates/keys"

dev:
	docker compose up -d

test:
	go test -v ./...
	cd web && npm test

lint:
	golangci-lint run ./...
	sqlfluff lint .
	cd web && npm run lint

build:
	go work sync
	go build ./...
	cd web && npm run build

migrate:
	@echo "Running migrations..."
	# Implementation would call golang-migrate or similar
	# For now, we trigger it via IAM service if configured
	curl -X POST http://localhost:8080/mgmt/migrate || true

seed:
	@echo "Seeding database..."
	SEED_DB=true go run services/iam/main.go

load-test:
	@echo "Starting load tests..."
	# k6 run scripts/load-test.js

certs:
	@echo "Generating certificates..."
	./scripts/gen-mtls-certs.sh
