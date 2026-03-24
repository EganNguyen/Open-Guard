.PHONY: dev test lint build migrate seed clean

# Generate mTLS certificates for internal services
	bash scripts/gen-mtls-certs.sh

# Start all infrastructure, backend and frontend services for local development
dev:
	docker compose --env-file .env -f services/docker-compose.yml -f infra/docker/docker-compose.yml up -d
	@echo "All infrastructure and services started."

# Build all backend services with docker
build-backend:
	docker compose -f services/docker-compose.yml build iam gateway policy

# Build all frontend services with docker
build-frontend:
	docker compose -f services/docker-compose.yml build web


# Run all tests across the workspace
test-unit:
	go test -cover ./services/gateway/... ./services/iam/... ./services/policy/... ./services/audit/... ./shared/...

# Run tests with race detector
test-race:
	go test -cover -race ./services/gateway/... ./services/iam/... ./services/policy/... ./services/audit/... ./shared/...

# Run integration tests
test-integration:
	go test -tags=integration ./services/gateway/... ./services/iam/... ./services/policy/... ./services/audit/... ./shared/...

# Lint all Go code
lint:
	golangci-lint run ./services/gateway/... ./services/iam/... ./services/policy/... ./shared/...

# Run database migrations
migrate:
	@if [ -f .env ]; then export $$(cat .env | grep -v '^#' | xargs); fi; bash scripts/migrate.sh

# Seed the database with sample data
seed:
	bash scripts/seed.sh

# Remove build artifacts
clean:
	rm -rf bin/

# Start only infrastructure (no app services)
infra-up:
	docker compose -f infra/docker/docker-compose.yml up -d

# Stop infrastructure
infra-down:
	docker compose -f infra/docker/docker-compose.yml down
