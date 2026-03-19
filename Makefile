.PHONY: dev test lint build migrate seed clean

# Start all infrastructure and services for local development
dev:
	docker compose -f infra/docker/docker-compose.yml -f infra/docker/docker-compose.dev.yml up -d
	@echo "Infrastructure and services started."

# Run all tests across the workspace
test:
	go test ./services/gateway/... ./services/iam/... ./shared/...

# Run tests with race detector
test-race:
	go test -race ./services/gateway/... ./services/iam/... ./shared/...

# Run integration tests
test-integration:
	go test -tags=integration ./services/gateway/... ./services/iam/... ./shared/...

# Lint all Go code
lint:
	golangci-lint run ./services/gateway/... ./services/iam/... ./shared/...

# Build all services
build:
	go build -o bin/gateway ./services/gateway
	go build -o bin/iam ./services/iam

# Run database migrations
migrate:
	bash scripts/migrate.sh

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
