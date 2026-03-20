.PHONY: dev test lint build migrate seed clean

# Generate mTLS certificates for internal services


# Start all infrastructure, backend and frontend services for local development
dev:
	docker compose --env-file .env -f services/docker-compose.yml -f infra/docker/docker-compose.yml up -d
	@echo "All infrastructure and services started."

# Run all tests across the workspace
test-unit:
	go test ./services/gateway/... ./services/iam/... ./services/policy/... ./shared/...

# Run tests with race detector
test-race:
	go test -race ./services/gateway/... ./services/iam/... ./services/policy/... ./shared/...

# Run integration tests
test-integration:
	go test -tags=integration ./services/gateway/... ./services/iam/... ./services/policy/... ./shared/...

# Lint all Go code
lint:
	golangci-lint run ./services/gateway/... ./services/iam/... ./services/policy/... ./shared/...

# Build all services
build:
	go build -o bin/gateway ./services/gateway
	go build -o bin/iam ./services/iam
	go build -o bin/policy ./services/policy

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
