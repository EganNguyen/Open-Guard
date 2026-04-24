.PHONY: dev test lint build migrate seed load-test certs help generate generate-phase-5

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
	@echo "  generate   - Run opencode to generate all phases"
	@echo "  phase5     - Run opencode to generate Phase 5 (Detectors)"

check-env:
	@test -f .env || (echo "ERROR: .env missing. Copy .env.example to .env and fill in values." && exit 1)
	@grep -q "AUDIT_SECRET_KEY=$$" .env && echo "ERROR: AUDIT_SECRET_KEY not set in .env" && exit 1 || true
	@grep -q "DATABASE_URL=$$" .env && echo "ERROR: DATABASE_URL not set in .env" && exit 1 || true
	@grep -q "JWT_KEYS=$$" .env && echo "ERROR: JWT_KEYS not set in .env" && exit 1 || true

dev: check-env
	cd infra/docker && docker compose up -d

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
	@echo "Running load tests..."
	@k6 run tests/load/auth-login.js --env BASE_URL=http://localhost:8080
	@k6 run tests/load/policy-eval.js --env BASE_URL=http://localhost:8083
	@k6 run tests/load/event-ingest.js --env BASE_URL=http://localhost:8083
	@k6 run tests/load/audit-query.js --env BASE_URL=http://localhost:8084
	@k6 run tests/load/scim-users.js --env BASE_URL=http://localhost:8080
	@k6 run tests/load/compliance.js --env BASE_URL=http://localhost:8085

certs:
	@echo "Generating certificates..."
	./scripts/gen-mtls-certs.sh

generate:
	opencode run .opencode/opencode.manifest.yaml "Generate all code defined in the manifest"

phase5:
	opencode run .opencode/phase5-detectors.yaml "Generate all detectors and logic defined in this spec"

geo-db:
	./scripts/download-geolite2.sh
