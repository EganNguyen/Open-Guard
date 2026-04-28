# Open-Guard Deployment Guide & Strategies

This guide outlines the deployment architecture, testing strategies, and production rollout for the Open-Guard security control plane.

## 1. Core Architecture: "Beside, Not In Front"
Open-Guard uses a high-performance, fail-closed architecture. The control plane is deployed "beside" your applications rather than as a proxy, ensuring zero latency impact for cached policy decisions.

- **Fail-Closed:** If the control plane is unreachable, SDKs are configured to deny access after a 60s TTL (configurable).
- **Communication:** Services communicate via **mTLS** with certificates stored in AWS Secrets Manager.
- **Audit:** Exactly-once audit via Transactional Outbox -> Kafka -> ClickHouse.

---

## 2. LocalStack Testing (Mock Infrastructure)
For CI/CD and local development, we use LocalStack Community Edition to simulate the AWS environment.

### Prerequisites
- Docker & Docker Compose
- [awslocal](https://github.com/localstack/awscli-local) and [tflocal](https://github.com/localstack/terraform-local)

### Deployment Steps
```bash
# 1. Start LocalStack
make dev

# 2. Bootstrap the mock backend (S3/DynamoDB for Terraform State)
export INFRA_MODE=localstack
./infra/environments/prod/bootstrap.sh us-east-1 localstack

# 3. Deploy Simulation Infrastructure
cd infra/environments/prod
tflocal init
tflocal apply -var="is_localstack=true" -var="domain_name=openguard.test"
```

**What it simulates:** VPC, S3 (Mock Registry), IAM, KMS, DynamoDB, and Core Networking.  
**What it skips:** Pro-only features like MSK Serverless, Redis Serverless, and WAF are skipped via the `is_localstack` toggle to ensure the pipeline passes in Community Edition.

---

## 3. Production Deployment (Real AWS)
Transitioning to production requires zero code changes—only a change in the variables passed to Terraform.

### Steps
1. **Configure Credentials:** Ensure `aws configure` points to your production account.
2. **Bootstrap Backend:**
   ```bash
   ./infra/environments/prod/bootstrap.sh <your-region> prod
   ```
3. **Deploy Full Stack:**
   ```bash
   cd infra/environments/prod
   terraform init
   terraform apply -var="is_localstack=false" -var="domain_name=yourcompany.com"
   ```

**Result:** Terraform provisions the full managed stack: Aurora Serverless v2, MSK Serverless (Kafka), ElastiCache Serverless (Redis), and the ECS Fargate cluster.

---

## 4. ECS Fargate vs. Kubernetes (K8s)

| Feature | **ECS Fargate (Primary)** | **Kubernetes (Optional)** |
|---|---|---|
| **Complexity** | **Low** - No node management. | **High** - Requires cluster management. |
| **Maintenance** | AWS handles patching/scaling. | You handle patching/autoscaling. |
| **Best For** | High-security control planes. | Organizations already on K8s. |

### ECS Fargate (Standard)
Open-Guard is optimized for Fargate to minimize the security surface area. Services find each other using **AWS Cloud Map** (`openguard.local`).

### Kubernetes (Optional)
If your organization standardizes on K8s, use the Helm charts in `infra/k8s/helm/openguard`. Ensure you have an Ingress Controller (Nginx/ALB) and the `external-secrets` operator installed to sync mTLS certs.

---

## 5. Continuous Deployment
The project includes a robust GitHub Actions pipeline in `.github/workflows/production-deploy.yml` that:
1. Builds Docker images for all microservices.
2. Runs LocalStack simulation tests.
3. Automatically applies Terraform changes to the mock environment.

---

## 6. Troubleshooting
- **mTLS Errors:** Check `infra/certs/` to ensure certificates are not expired.
- **LocalStack 501:** If you see "Not Implemented", ensure `is_localstack=true` is set to skip Pro features.
- **Database Migrations:** Triggered via the IAM service management endpoint: `POST /mgmt/migrate`.
