# Implementation Plan: Deploying OpenGuard to LocalStack

This plan outlines the steps to migrate OpenGuard's cloud-dependent components to [LocalStack](https://localstack.cloud/), enabling a fully self-contained local AWS-like environment for development and testing.

## Overview
Currently, OpenGuard uses MinIO for S3-compatible storage. This plan will replace MinIO with LocalStack S3 and introduce LocalStack Secrets Manager for sensitive configuration (JWT keys, MFA secrets).

## Phase 1: Infrastructure Updates

### 1.1 Modify `infra/docker/docker-compose.yml`
Replace `minio` and `minio-init` with `localstack`.

```yaml
  localstack:
    container_name: openguard-localstack
    image: localstack/localstack:latest
    ports:
      - "127.0.0.1:4566:4566"            # LocalStack Gateway
      - "127.0.0.1:4510-4559:4510-4559"  # external services port range
    environment:
      - SERVICES=s3,secretsmanager,sqs,sns,lambda
      - DEBUG=${DEBUG:-0}
      - DOCKER_HOST=unix:///var/run/docker.sock
      - AWS_DEFAULT_REGION=us-east-1
    volumes:
      - "${LOCALSTACK_VOLUME_DIR:-./volume}:/var/lib/localstack"
      - "/var/run/docker.sock:/var/run/docker.sock"
      - "./localstack-init:/etc/localstack/init/ready.d" # Initialization scripts
```

### 1.2 Create Initialization Scripts
Create `infra/docker/localstack-init/01_init.sh` to set up buckets and secrets.

```bash
#!/bin/bash
awslocal s3 mb s3://compliance-reports
awslocal secretsmanager create-secret --name openguard/jwt-keys --secret-string '[{"kid":"k1","secret":"super-secret-key-at-least-32-chars","algorithm":"HS256","status":"active"}]'
```

## Phase 2: Service Configuration

### 2.1 Update `compliance` Service
Point the `S3_ENDPOINT` to LocalStack in `infra/docker/docker-compose.yml`.

```yaml
  compliance:
    environment:
      - S3_ENDPOINT=http://localstack:4566
      - S3_ACCESS_KEY=test
      - S3_SECRET_KEY=test
      - S3_REGION=us-east-1
      - S3_BUCKET=compliance-reports
```

### 2.2 Update `iam` Service (Enhancement)
Modify `services/iam/main.go` to optionally fetch secrets from Secrets Manager if `USE_AWS_SECRETS_MANAGER=true`.

1.  Add `github.com/aws/aws-sdk-go-v2/service/secretsmanager` dependency.
2.  Implement a secret provider that defaults to env vars but can use AWS.

## Phase 3: SDK & Testing

### 3.1 Verify Presigned URLs
LocalStack's presigned URLs might need `S3_HOSTNAME_EXTERNAL` configuration in `docker-compose.yml` to be accessible from the host (browser/Angular dashboard).

### 3.2 Update Acceptance Tests
Ensure `make test-acceptance` passes with the new LocalStack backend.

## Phase 4: Documentation
- Update `README.md` with LocalStack setup instructions.
- Document how to use `awslocal` CLI to inspect LocalStack state.

## Next Steps
1.  Add `localstack` to `docker-compose.yml`.
2.  Create the initialization script.
3.  Update the `compliance` service environment variables.
4.  (Optional) Implement Secrets Manager integration in the `iam` service.
