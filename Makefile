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
	go work sync
	go test -v -race ./sdk/... ./shared/... \
		./services/iam/... ./services/policy/... ./services/alerting/... \
		./services/threat/... ./services/audit/... ./services/compliance/... \
		./services/control-plane/... ./services/dlp/... \
		./services/connector-registry/... ./services/webhook-delivery/...
	# cd web && npm test


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
	@k6 run tests/load/kafka-throughput.js --env KAFKA_BROKERS=localhost:9092

test-integration:
	go test -v ./tests/integration/...

certs:
	@echo "Generating certificates..."
	./scripts/gen-mtls-certs.sh

generate:
	opencode run .opencode/opencode.manifest.yaml "Generate all code defined in the manifest"

phase5:
	opencode run .opencode/phase5-detectors.yaml "Generate all detectors and logic defined in this spec"

localstack-up: certs
	@echo "Building Microservices..."
	docker build -t openguard/iam:latest -f services/iam/Dockerfile .
	docker build -t openguard/policy:latest -f services/policy/Dockerfile .
	docker build -t openguard/audit:latest -f services/audit/Dockerfile .
	docker build -t openguard/threat:latest -f services/threat/Dockerfile .
	docker build -t openguard/alerting:latest -f services/alerting/Dockerfile .
	docker build -t openguard/webhook-delivery:latest -f services/webhook-delivery/Dockerfile .
	docker build -t openguard/compliance:latest -f services/compliance/Dockerfile .
	docker build -t openguard/dlp:latest -f services/dlp/Dockerfile .
	docker build -t openguard/connector-registry:latest -f services/connector-registry/Dockerfile .
	docker build -t openguard/control-plane:latest -f services/control-plane/Dockerfile .
	@echo "Building Connected App & Dashboard..."
	docker build -t openguard/example-app:latest -f examples/task-management-app/backend/Dockerfile .
	docker build -t openguard/dashboard:latest -f web/Dockerfile .
	@echo "Starting LocalStack Pro on openguard-net..."
	-docker network create openguard-net
	LOCALSTACK_AUTH_TOKEN=$(LOCALSTACK_AUTH_TOKEN) localstack start -d
	@echo "Waiting for Data Tier Readiness..."
	chmod +x deploy/localstack/scripts/wait-for-infra.sh
	./deploy/localstack/scripts/wait-for-infra.sh
	@echo "Waiting for LocalStack API..."
	localstack wait -t 30
	@echo "Connecting LocalStack to openguard-net..."
	-docker network connect openguard-net localstack-main
	@echo "Bridging Data Tier to openguard-net..."
	-docker network connect openguard-net docker-postgres-1
	-docker network connect openguard-net docker-redis-1
	-docker network connect openguard-net docker-kafka-1
	-docker network connect openguard-net docker-mongo-primary-1
	-docker network connect openguard-net docker-clickhouse-1
	@echo "Provisioning AWS Resources & Syncing Certs..."
	export AWS_ACCESS_KEY_ID=test && export AWS_SECRET_ACCESS_KEY=test && export AWS_DEFAULT_REGION=us-east-1 && \
	INFRA_MODE=localstack ./deploy/production/bootstrap.sh us-east-1 localstack
	export PATH=$$PATH:/Users/nguyenhoangtuan/Library/Python/3.9/bin && \
	cd deploy/localstack/terraform && tflocal init && tflocal apply -auto-approve
	@echo "Deploying Full Stack via ECS Shim..."
	chmod +x deploy/localstack/scripts/run-ecs-shim.sh
	-docker rm -f $$(docker ps -aqf "name=openguard-")
	./deploy/localstack/scripts/run-ecs-shim.sh iam openguard/iam:latest 8081
	./deploy/localstack/scripts/run-ecs-shim.sh policy openguard/policy:latest 8082
	./deploy/localstack/scripts/run-ecs-shim.sh audit openguard/audit:latest 8083
	./deploy/localstack/scripts/run-ecs-shim.sh threat openguard/threat:latest 8084
	./deploy/localstack/scripts/run-ecs-shim.sh alerting openguard/alerting:latest 8085
	./deploy/localstack/scripts/run-ecs-shim.sh webhook openguard/webhook-delivery:latest 8086
	./deploy/localstack/scripts/run-ecs-shim.sh compliance openguard/compliance:latest 8087
	./deploy/localstack/scripts/run-ecs-shim.sh dlp openguard/dlp:latest 8088
	./deploy/localstack/scripts/run-ecs-shim.sh registry openguard/connector-registry:latest 8089
	./deploy/localstack/scripts/run-ecs-shim.sh control-plane openguard/control-plane:latest 8080
	./deploy/localstack/scripts/run-ecs-shim.sh example-app openguard/example-app:latest 3005
	./deploy/localstack/scripts/run-ecs-shim.sh dashboard openguard/dashboard:latest 4200
	@echo "Waiting for IAM service health..."
	until [ "$$(docker inspect -f '{{.State.Health.Status}}' openguard-iam 2>/dev/null)" == "healthy" ] || [ "$$(docker inspect -f '{{.State.Running}}' openguard-iam 2>/dev/null)" == "true" ]; do \
		echo "Waiting for openguard-iam to be running..."; \
		sleep 2; \
	done
	@echo "Small delay for migration readiness..."
	sleep 5
	@echo "Initializing Data (Seeding)..."
	-docker exec openguard-iam env SEED_DB=true /usr/local/bin/service
	@echo "Deployment Complete."



localstack-down:
	@echo "Stopping Tunnels and Containers..."
	-pkill instatunnel
	-docker rm -f $$(docker ps -aqf "name=openguard-")
	localstack stop

public-url:
	@echo "🚀 Starting InstaTunnel public tunnel (openguard-dev.instatunnel.io)..."
	instatunnel tunnel 4200 --subdomain openguard-dev > /dev/null &
	@echo "🌐 Public URL is: https://openguard-dev.instatunnel.io"


