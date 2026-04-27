# AWS + DevOps Mastery Roadmap
## From Beginner to Production Architect

> **Anchor Project:** [OpenGuard](https://github.com/openguard/openguard) — a production-grade, self-hostable security control plane (Go + Angular + Kafka + PostgreSQL + MongoDB). Every phase of this roadmap is illustrated using patterns from OpenGuard's architecture. Clone it. Deploy it. Break it. Fix it.

---

## How to Use This Roadmap

This roadmap is **vertical, not horizontal**. Instead of touching 40 services shallowly, you go deep on the ~15 services that appear in 90% of real production systems. Each topic covers:

- **The concept** — precise, not paraphrased
- **Why it matters in production** — where you'll feel the pain if you skip it
- **Real-world architecture** — how OpenGuard or similar systems use it
- **Common mistakes** — what senior engineers have learned the hard way
- **Hands-on project** — concrete things to build, not tutorials to watch

**Time estimate:** 6–12 months if you're building real systems alongside this. Reading alone won't do it.

---

## Roadmap Phases

| Phase | Level | Focus | Duration |
|-------|-------|--------|----------|
| [Phase 1](#phase-1-foundations) | Beginner | Linux, Networking, AWS basics, IAM, VPC | 6–8 weeks |
| [Phase 2](#phase-2-compute-and-data) | Intermediate | EC2, S3, RDS, DynamoDB, containers | 6–8 weeks |
| [Phase 3](#phase-3-devops-and-ci-cd) | Intermediate | Docker, CI/CD, GitHub Actions, IaC | 6–8 weeks |
| [Phase 4](#phase-4-distributed-systems) | Advanced | ECS, EKS, Lambda, Kafka, microservices | 8–10 weeks |
| [Phase 5](#phase-5-production-mastery) | Production | Observability, security, cost, DR | 6–8 weeks |

---

# Phase 1: Foundations

## 1.1 Networking Fundamentals (Before Any AWS)

**You cannot debug a broken VPC if you don't understand what a VPC is abstracting.**

### DNS

DNS is a distributed key-value store that maps names to IP addresses. The resolution chain is: your stub resolver → recursive resolver (e.g. 8.8.8.8) → root nameserver → TLD nameserver (`.com`) → authoritative nameserver for the domain.

**Why it matters in production:** DNS TTL causes stale routing during deployments. If your TTL is 3600 seconds and you change an A record, traffic can take an hour to shift. AWS Route 53's health checks + failover records depend on DNS propagation behavior.

**In OpenGuard:** mTLS certificates are generated per-service using `scripts/gen-mtls-certs.sh`. The CN (Common Name) and SAN (Subject Alternative Name) fields must match the service's DNS name for TLS to succeed. When services run on Kubernetes, those names are `service-name.namespace.svc.cluster.local`.

**Common mistake:** Setting a 5-minute TTL on a record you need to change in an emergency, then wondering why traffic still goes to the old IP 90 minutes later. During incident resolution, TTLs are not your friend.

### HTTP/HTTPS and TLS

HTTP is a request-response protocol over TCP. HTTPS is HTTP over TLS. TLS does two things: encrypts traffic and authenticates the server (and optionally the client, which is mTLS).

The TLS handshake:
1. Client sends `ClientHello` (supported TLS versions, cipher suites)
2. Server responds with `ServerHello` + its certificate
3. Client verifies the cert against trusted CAs
4. Key exchange happens (ECDHE in TLS 1.3)
5. Symmetric encryption begins

**mTLS** adds step 2.5: the server also requests and verifies the client's certificate. This is how OpenGuard's internal services authenticate each other — not via API keys, but via mutual certificate verification.

**Why it matters:** Every microservice system you build should use mTLS internally. Service mesh tools like Istio/Linkerd automate this. Without it, a compromised internal service can impersonate any other.

**Common mistake:** Using self-signed certificates in production without proper certificate pinning or a CA hierarchy. The result is TLS_ERROR at 3am during an incident.

### TCP and Load Balancing

TCP is connection-oriented. Connections have state. Load balancers work at two layers:
- **Layer 4 (TCP):** Routes by IP/port, doesn't read HTTP headers. AWS NLB.
- **Layer 7 (HTTP):** Reads HTTP headers, enables path-based routing, sticky sessions, WebSocket upgrades. AWS ALB.

**When to use which:** NLB for raw throughput (millions of connections/sec, gRPC, mTLS passthrough). ALB for HTTP routing, authentication offloading, and header manipulation.

**OpenGuard uses:** ALB in front of the control plane with path-based routing to different microservices (`/v1/policy/*` → policy service, `/v1/events/*` → connector-registry).

---

## 1.2 AWS IAM — Identity and Access Management

**IAM is the foundation. Everything else depends on it. Get this wrong and you're one misconfiguration away from a breach.**

### Core Concepts

**IAM Principals:** Who is making requests.
- **Users:** Human identities with long-lived credentials. Avoid in production; use roles.
- **Roles:** Assumed identities with temporary credentials (STS tokens expire in 1–12 hours). EC2 instances, Lambda functions, ECS tasks, CI/CD pipelines, and cross-account access all use roles.
- **Service Accounts:** Kubernetes service accounts mapped to IAM roles via IRSA (IAM Roles for Service Accounts).

**IAM Policies:** What actions are allowed or denied.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject"
      ],
      "Resource": "arn:aws:s3:::openguard-audit-exports/*",
      "Condition": {
        "StringEquals": {
          "s3:prefix": ["org-123/"]
        }
      }
    }
  ]
}
```

**Policy types (in order of priority):**
1. Service Control Policies (SCP) — applied at AWS Organizations level, can only deny
2. Permission Boundaries — cap what a role can do, used to delegate IAM management safely
3. Identity-based policies — attached to users/roles
4. Resource-based policies — attached to resources (S3 bucket policies, Lambda resource policies)
5. Session policies — scoped down when assuming a role
6. VPC Endpoint policies — restrict what can flow through a VPC endpoint

**Evaluation logic:** Explicit Deny > SCP > Permission Boundary > Identity Policy > Resource Policy. An implicit deny if nothing allows.

### Least Privilege in Practice

Don't think "what does this service need?" Think "what is the minimum set of actions on the minimum set of resources for this exact use case?"

```json
// BAD — wildcard everything
{
  "Effect": "Allow",
  "Action": "s3:*",
  "Resource": "*"
}

// GOOD — scoped to the exact bucket and prefix
{
  "Effect": "Allow",
  "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
  "Resource": "arn:aws:s3:::openguard-compliance-reports/org-*/reports/*"
}
```

**Why conditions matter:** Use `aws:RequestedRegion` to prevent resource creation outside approved regions. Use `aws:MultiFactorAuthPresent` to require MFA for sensitive actions like deleting production data.

### IAM Anti-Patterns

1. **Static access keys on EC2:** Never put `AWS_ACCESS_KEY_ID` in an EC2 environment variable. Use instance metadata (IMDS) + IAM roles. The instance role is fetched automatically by every AWS SDK.
2. **Shared IAM users for CI/CD:** Each GitHub Actions workflow, each service, each environment gets its own role with its own policy.
3. **`AdministratorAccess` on anything that's not an IAM admin bootstrap role.** If your Lambda has admin access and it's exposed to user input, you have a privilege escalation vulnerability.
4. **Not enabling CloudTrail:** Every IAM API call should be logged. You need this for incident investigation and compliance.

### OpenGuard IAM Architecture

OpenGuard runs as an ECS service. The ECS task role grants:
- `secretsmanager:GetSecretValue` on specific secret ARNs (JWT signing keys, DB passwords)
- `s3:PutObject` on the compliance export bucket
- `ses:SendEmail` for alert notifications
- No EC2 permissions, no IAM permissions, no billing permissions

Each microservice has its own task role. The `dlp` service has no S3 access. The `compliance` service has S3 read/write but not SES. This is least privilege at the service boundary.

### Hands-On Project

1. Create an AWS account. Enable MFA on the root account immediately.
2. Create an IAM user for yourself with `AdministratorAccess`, but set a permission boundary that denies `iam:CreateUser` and `iam:DeleteUser`.
3. Create a role for a hypothetical "audit-export" Lambda: it can only `s3:PutObject` on `arn:aws:s3:::my-audit-bucket/exports/*` and `logs:CreateLogGroup`/`logs:PutLogEvents` for CloudWatch.
4. Test your policy with the IAM Policy Simulator.
5. Enable CloudTrail and trigger some API calls. Find them in CloudTrail logs.

---

## 1.3 VPC — Virtual Private Cloud

**A VPC is your private network on AWS. Almost everything you deploy lives inside one.**

### The Mental Model

Think of a VPC as a data center you define in software. You get:
- A CIDR block (e.g., `10.0.0.0/16`) — 65,536 IP addresses
- Divided into subnets across Availability Zones
- Controlled by route tables, NACLs, and Security Groups

### Subnets: Public vs Private

**Public subnet:** Has a route to an Internet Gateway. Resources here can have public IPs and receive inbound internet traffic.

**Private subnet:** No route to the internet. Resources here cannot be reached from the internet directly. They can reach the internet via a NAT Gateway (for outbound only).

**The standard 3-tier VPC architecture:**

```
Internet
    │
    ▼
Internet Gateway (IGW)
    │
    ▼
┌─────────────────────────────────────────────┐
│  Public Subnets (AZ-a, AZ-b, AZ-c)         │
│  ┌──────────────┐   ┌──────────────────┐    │
│  │ ALB          │   │ NAT Gateway      │    │
│  │ (public-facing│  │ (one per AZ for  │    │
│  │  load balancer│  │  HA egress)      │    │
│  └──────────────┘   └──────────────────┘    │
└─────────────────────────────────────────────┘
    │                       │
    ▼ (private traffic)     ▼ (outbound egress)
┌─────────────────────────────────────────────┐
│  Private Subnets (AZ-a, AZ-b, AZ-c)         │
│  ┌──────────────┐   ┌──────────────────┐    │
│  │ ECS Services │   │ EC2 (App Tier)   │    │
│  │ Lambda       │   │ EKS Nodes        │    │
│  └──────────────┘   └──────────────────┘    │
└─────────────────────────────────────────────┘
    │
    ▼
┌─────────────────────────────────────────────┐
│  Isolated Subnets (AZ-a, AZ-b, AZ-c)        │
│  ┌──────────────┐   ┌──────────────────┐    │
│  │ RDS (Primary)│   │ ElastiCache      │    │
│  │ RDS (Replica)│   │ (Redis)          │    │
│  └──────────────┘   └──────────────────┘    │
└─────────────────────────────────────────────┘
```

### Security Groups vs NACLs

**Security Groups** are stateful firewalls attached to ENIs (Elastic Network Interfaces). If you allow inbound traffic on port 443, the response traffic is automatically allowed outbound. They operate at the instance level and support allow rules only.

**NACLs (Network ACLs)** are stateless firewalls at the subnet level. Both inbound AND outbound rules must explicitly allow traffic. Rules are evaluated in order by rule number. Default NACL allows everything.

**When to use which:** Use Security Groups for everything. Use NACLs as a coarse subnet-level backstop for blocking entire IP ranges (e.g., block a malicious ASN).

```
# Security Group for OpenGuard Policy Service
Inbound:
  - TCP 8082 from ALB Security Group     # Only the ALB can reach this
  - TCP 8082 from Control Plane SG        # Internal mTLS calls
Outbound:
  - TCP 5432 to RDS Security Group        # PostgreSQL
  - TCP 6379 to Redis Security Group      # Redis
  - TCP 443 to 0.0.0.0/0                 # AWS API calls (Secrets Manager, etc.)

# Security Group for RDS
Inbound:
  - TCP 5432 from Policy Service SG only
  - TCP 5432 from IAM Service SG only
  - TCP 5432 from Bastion SG             # Admin access only
Outbound:
  - None
```

### VPC Endpoints

VPC Endpoints allow your private-subnet resources to call AWS services (S3, Secrets Manager, SQS) without traversing the public internet. Two types:

- **Gateway Endpoints:** For S3 and DynamoDB only. Free. Added to route tables.
- **Interface Endpoints (PrivateLink):** For everything else. Creates an ENI in your subnet with a private IP. Costs $7.20/month per AZ per service.

**Why it matters:** An ECS task in a private subnet calling `secretsmanager:GetSecretValue` without a VPC endpoint either needs a NAT Gateway (costs money, goes through the internet) or fails. Use Interface Endpoints for Secrets Manager, ECR, CloudWatch Logs, and STS.

### NAT Gateway

NAT Gateway provides outbound internet access for private subnet resources. It must be in a public subnet. For high availability, deploy one NAT Gateway per AZ and configure each AZ's private subnet route table to use the NAT Gateway in that same AZ.

**Cost:** $0.045/hour (~$32/month) + $0.045/GB processed. For high-traffic systems, this adds up fast. A service processing 1TB/month of outbound traffic spends ~$45 just on NAT data transfer.

**NAT Gateway anti-pattern:** Single NAT Gateway for all AZs. If that AZ has an outage, all private subnet egress fails. Always deploy one per AZ for production.

### Hands-On Project

1. Create a VPC with CIDR `10.0.0.0/16` across 3 AZs using the AWS Console, then redo it with Terraform.
2. Deploy a public-facing ALB in public subnets.
3. Deploy an EC2 instance in a private subnet with no public IP.
4. Prove the EC2 can reach the internet via NAT Gateway (`curl https://api.ipify.org`).
5. Block outbound port 80 on the EC2's Security Group. Confirm `curl http://example.com` fails.
6. Add a VPC Endpoint for S3. Confirm traffic to S3 no longer goes through NAT Gateway (check VPC Flow Logs).

---

# Phase 2: Compute and Data

## 2.1 EC2 and Auto Scaling

**EC2 is the foundation of AWS compute. Even if you use ECS or Lambda, understanding EC2 deeply makes you better at both.**

### Instance Selection Framework

| Dimension | Questions to Ask |
|-----------|-----------------|
| CPU type | General (m-series), compute (c-series), memory (r-series), GPU (p/g-series), burstable (t-series) |
| Architecture | x86_64 vs ARM (Graviton) — Graviton is 20-40% cheaper for equivalent workloads |
| Storage | Does your workload need NVMe instance store (ephemeral, fast) or EBS-backed persistence? |
| Network | Is your workload network-intensive? Check "Network Performance" in the instance type table |

**OpenGuard Sizing Example:**
- IAM service (bcrypt-heavy, CPU-bound): `c7g.xlarge` (4 vCPU, 8GB RAM, Graviton3)
- Policy service (memory + network): `m7g.large` (2 vCPU, 8GB RAM)
- Kafka brokers (disk-heavy): `r7gd.xlarge` (4 vCPU, 32GB RAM, local NVMe)

### Auto Scaling Groups

An ASG maintains a fleet of EC2 instances. It replaces unhealthy instances and scales in/out based on policies.

**Three scaling policy types:**
1. **Target Tracking:** "Keep CPU at 60%." AWS adds/removes instances automatically. Easiest to use, correct 90% of the time.
2. **Step Scaling:** "At 70% CPU add 1 instance, at 90% add 3." More control, more to get wrong.
3. **Scheduled Scaling:** "Every weekday at 8am, set min capacity to 10." For predictable traffic patterns.

**Launch Template vs Launch Configuration:** Always use Launch Templates. They support versioning, mixed instance policies (on-demand + Spot), and are required for the latest features.

**Instance warm-up and cooldown:** After scaling out, instances need time to register with the load balancer and start serving traffic. Set `DefaultInstanceWarmup` to match your application startup time. Without it, the ASG might trigger another scale-out event before the first new instances are healthy.

**Common mistake:** Scale-out triggers that are too aggressive (CPU > 50% for 1 minute) cause thrashing — you scale out, CPU drops, instances terminate, CPU rises again. Use longer evaluation periods (5-10 minutes) and set a scale-in cooldown.

### EC2 Spot Instances

Spot instances are excess AWS capacity sold at up to 90% discount. They can be interrupted with a 2-minute warning.

**When to use:** Stateless workloads that can be interrupted and restarted — batch processing, CI/CD build agents, data processing. Never for stateful primaries.

**OpenGuard's approach:** Compliance report generation (Phase 6) runs as a background batch job on Spot instances. If interrupted, the job retries from a checkpoint stored in S3. The main API services run on on-demand instances.

---

## 2.2 S3 — Simple Storage Service

**S3 is not just object storage. It's a building block for data lakes, backup, artifact distribution, and event-driven architectures.**

### Storage Classes and Cost

| Class | Use Case | Price (per GB/month) |
|-------|----------|---------------------|
| Standard | Frequently accessed data | $0.023 |
| Intelligent-Tiering | Unknown access patterns | $0.023 (automatic cost optimization) |
| Standard-IA | Infrequent access, millisecond retrieval | $0.0125 |
| Glacier Instant Retrieval | Archive, millisecond retrieval | $0.004 |
| Glacier Flexible Retrieval | Archive, 1-12 hour retrieval | $0.0036 |
| Deep Archive | Long-term archive, 12-48 hour retrieval | $0.00099 |

**Lifecycle policies** automatically transition objects between classes:

```json
{
  "Rules": [{
    "ID": "audit-export-lifecycle",
    "Status": "Enabled",
    "Filter": {"Prefix": "audit-exports/"},
    "Transitions": [
      {"Days": 30, "StorageClass": "STANDARD_IA"},
      {"Days": 90, "StorageClass": "GLACIER_IR"},
      {"Days": 365, "StorageClass": "DEEP_ARCHIVE"}
    ],
    "Expiration": {"Days": 2555}
  }]
}
```

OpenGuard's audit log has a 2-year retention requirement. Compliance exports go to S3 with this exact lifecycle policy — Standard for the first month (engineers actively reference recent exports), IA at 30 days, Glacier at 90 days.

### S3 Security

**Block Public Access:** Enable at the account level. Never make buckets public unless you're intentionally hosting a static website.

**Bucket policies vs ACLs:** Disable ACLs (Object Ownership = Bucket owner enforced). Use bucket policies for cross-account access. Use IAM policies for same-account access.

**Encryption:** Server-side encryption is enabled by default since 2023. For compliance (HIPAA, FINTRAC), use SSE-KMS with a customer-managed CMK so you control key rotation and can prove it.

**S3 Access Logs vs CloudTrail S3 data events:** Access logs capture every request (useful for debugging). CloudTrail S3 data events capture `GetObject`/`PutObject` with the IAM principal who made the call (useful for auditing). Use both for a regulated workload.

**Pre-signed URLs:** Let clients upload/download directly without going through your application server. OpenGuard uses them for compliance report downloads — the compliance service generates a pre-signed URL (15-minute expiry), returns it to the client, and the client downloads directly from S3. Your application server handles zero bytes of that download.

### S3 Anti-Patterns

1. **Using S3 as a message queue:** S3 doesn't guarantee ordering, has no visibility timeout, and polling costs money. Use SQS.
2. **Storing secrets in S3:** Use Secrets Manager. S3 has no automatic rotation or audit trail for secret access.
3. **Forgetting to enable versioning on critical buckets:** Without versioning, a `DeleteObject` is permanent.
4. **Hot partitions:** If all your keys start with the same prefix (e.g., dates: `2025/01/01/...`), S3 performance suffers. Randomize prefixes or use hex hash prefixes.

---

## 2.3 RDS vs DynamoDB — When to Use Which

**This is one of the most consequential architectural decisions you'll make. Get it wrong and you're refactoring your data layer 18 months later.**

### The Decision Framework

```
Does your data have complex relationships requiring JOINs?
  YES → RDS (PostgreSQL)
  NO → Continue

Do you need ACID transactions across multiple items?
  YES → RDS or DynamoDB Transactions (limited to 25 items)
  NO → Continue

Is your access pattern well-defined and key-based?
  YES → DynamoDB
  NO → Continue

Do you need full-text search, aggregations, or ad-hoc queries?
  YES → RDS (or add OpenSearch)
  NO → DynamoDB
```

### RDS / Aurora PostgreSQL

**Aurora PostgreSQL** is AWS's re-engineered PostgreSQL-compatible database. The storage layer is separated from the compute layer — 6 copies of data across 3 AZs, automatic healing. Read replicas share the same storage. Failover takes 30 seconds instead of the 5+ minutes for standard RDS Multi-AZ.

**OpenGuard's PostgreSQL schema (partial):**

```sql
-- Multi-tenant with Row-Level Security
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id),
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- RLS policy: each service session can only see its own org's data
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
CREATE POLICY rls_users ON users
    USING (org_id = NULLIF(current_setting('app.org_id', true), '')::UUID);

-- The application sets this before any query:
-- SET LOCAL app.org_id = '11111111-...';
```

**Why this matters:** PostgreSQL RLS is enforced at the database level. Even if your application code has a bug that forgets to add a `WHERE org_id = ?` clause, the data for other tenants is still invisible. This is defense in depth for multi-tenancy.

**Connection pooling:** PostgreSQL has a process-per-connection model. Each connection costs ~5MB RAM. 1,000 connections = 5GB RAM just for connection overhead. Use **PgBouncer** (or RDS Proxy) to pool connections. OpenGuard runs PgBouncer as a sidecar.

### DynamoDB

DynamoDB is a fully managed key-value/document store. Performance at any scale — single-digit millisecond reads and writes. No connection pooling. No servers to manage. No schema migrations.

**The access pattern trap:** DynamoDB forces you to design your data model around your access patterns, not the other way around. Get your partition key wrong and you get hot partitions (all traffic on one shard) and throttling.

**When DynamoDB shines in AWS:**
- Session storage (user sessions keyed by session ID)
- Rate limiting counters
- Feature flags (keyed by feature name)
- Time-series data with a time-based sort key

**When DynamoDB fails:**
- Complex reporting queries (you can't `GROUP BY`)
- Many-to-many relationships
- Full-text search
- Anything that would require a `JOIN`

**OpenGuard uses PostgreSQL for everything transactional** (IAM, Policy, Connectors) because the data is relational and requires ACID transactions. It uses Redis (not DynamoDB) for caching because the workload is read-heavy and Redis has sub-millisecond performance for in-process cache scenarios.

---

## 2.4 Secrets Manager and Parameter Store

**Secrets management is not optional. Hardcoded credentials are how companies end up in breach notifications.**

### Secrets Manager

AWS Secrets Manager stores, retrieves, and automatically rotates secrets. Supports automatic rotation for RDS, Redshift, and DocumentDB via built-in Lambda functions. For custom secrets, you write the rotation Lambda.

**How OpenGuard uses it:**

```go
// services/iam/internal/config/config.go
func LoadJWTKeys(ctx context.Context, sm *secretsmanager.Client) ([]JWTKey, error) {
    out, err := sm.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
        SecretId: aws.String("openguard/iam/jwt-keys"),
    })
    if err != nil {
        return nil, fmt.Errorf("loading JWT keys: %w", err)
    }
    var keys []JWTKey
    return keys, json.Unmarshal([]byte(*out.SecretString), &keys)
}
```

The JWT keys JSON includes multiple keys with `kid` (key ID) and `status` fields:

```json
[
  {"kid": "k2", "secret": "<new>", "algorithm": "HS256", "status": "active"},
  {"kid": "k1", "secret": "<old>", "algorithm": "HS256", "status": "rotating"}
]
```

During rotation, both keys are valid. Tokens signed with `kid: k1` still verify. After all existing tokens expire, `k1` is removed. Zero-downtime secret rotation.

### Parameter Store

AWS Systems Manager Parameter Store is simpler and cheaper than Secrets Manager. Use it for non-secret configuration values.

| Use Secrets Manager for | Use Parameter Store for |
|------------------------|------------------------|
| Database passwords | Feature flags |
| API keys with rotation | App configuration |
| JWT signing secrets | Environment variables |
| OAuth client secrets | Non-sensitive parameters |

**Parameter Store tiers:**
- **Standard:** Free, 4KB max size, no automatic rotation
- **Advanced:** $0.05/parameter/month, 8KB max, supports parameter policies (expiry notifications)

**Common mistake:** Putting secrets in Parameter Store Standard because it's free. You lose automatic rotation and fine-grained audit logging. Use Secrets Manager for anything that should rotate.

### Hands-On Project

1. Store OpenGuard's JWT signing key in Secrets Manager.
2. Create an IAM role for the IAM service that can only call `secretsmanager:GetSecretValue` on that specific secret ARN.
3. Write a Go/Python/Node script that fetches the secret and decodes it.
4. Manually rotate the secret (update the value). Confirm your application picks up the new value without restarting.
5. Set up CloudTrail to alert when anyone calls `GetSecretValue` on that secret.

---

# Phase 3: DevOps and CI/CD

## 3.1 Docker — Container Fundamentals

**Docker is not just a packaging tool. A poorly designed Dockerfile becomes a security vulnerability, a slow CI pipeline, and an operations nightmare.**

### Image Layers and Caching

Every `RUN`, `COPY`, and `ADD` instruction creates a new layer. Docker caches layers. If a layer hasn't changed, Docker reuses the cached version.

**Cache-busting mistake:**

```dockerfile
# BAD — copies code before installing dependencies
# Any code change = reinstall all deps
COPY . .
RUN go mod download

# GOOD — install dependencies first, copy code second
# Code changes don't bust the dependency cache
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /app/server .
```

### Multi-Stage Builds

Build stages are discarded at the end. Only the final stage becomes your image.

```dockerfile
# OpenGuard IAM service Dockerfile
# Stage 1: Build
FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \  # Strip debug info — smaller binary
    -o /app/iam \
    ./services/iam/cmd/server

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian12  # No shell, no package manager, minimal attack surface
COPY --from=builder /app/iam /app/iam
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/app/iam"]
```

The final image is ~20MB instead of ~1GB. It contains no shell, no package manager, and no build tools. If an attacker gets code execution, their blast radius is severely limited.

### Container Security Essentials

1. **Never run as root:** `USER nonroot:nonroot` in your Dockerfile
2. **Read-only filesystem:** `docker run --read-only` or `securityContext.readOnlyRootFilesystem: true` in Kubernetes
3. **No new privileges:** `--security-opt=no-new-privileges:true`
4. **Scan images:** Use `trivy image my-image:latest` in CI. OpenGuard's CI blocks releases with CRITICAL or HIGH vulnerabilities.
5. **Pin base image digests:** `FROM golang:1.22-alpine@sha256:abc123...` instead of `FROM golang:1.22-alpine`. Tags are mutable; digests are not.

### Docker Anti-Patterns

1. **`latest` tag in production:** If the base image updates and breaks your app, you don't know when it happened.
2. **Secrets in Dockerfile:** `ENV DB_PASSWORD=supersecret` bakes the secret into every layer. Use Docker secrets or environment injection at runtime.
3. **One massive image for all environments:** Build once, promote the same image through dev → staging → prod. Different environments get different configs via environment variables, not different images.
4. **Not using `.dockerignore`:** `COPY . .` copies your `node_modules`, `.git`, local secrets, and test data into the image.

---

## 3.2 CI/CD with GitHub Actions

**A CI/CD pipeline is not just automation. It's your enforcement mechanism for code quality, security, and deployment standards.**

### OpenGuard's Production CI Pipeline

OpenGuard's `.github/workflows/ci.yml` runs on every push to `main` and every PR. It demonstrates a real production pipeline:

```yaml
name: Full Production Pipeline

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  lint-and-format:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'
          cache: true  # Caches Go module downloads

      - name: Run Go Lint
        run: golangci-lint run ./...
      
      # OpenGuard-specific: catch unsafe JWT usage
      - name: Check for unsafe AuthJWT usage
        run: |
          if grep -rn "\.AuthJWT(" services/ | grep -v "WithBlocklist"; then
            echo "ERROR: Use AuthJWTWithBlocklist. JWT without blocklist = no revocation."
            exit 1
          fi

      - name: Run SQL Lint
        run: sqlfluff lint .

  security-scan:
    needs: lint-and-format
    steps:
      - name: govulncheck
        run: govulncheck ./...
      
      - name: trivy
        uses: aquasecurity/trivy-action@master
        with:
          scan-type: 'fs'
          severity: 'CRITICAL,HIGH'
          exit-code: '1'  # Fail the build

  test:
    needs: lint-and-format
    steps:
      - name: Unit Tests
        run: go test ./... -race -count=1 -coverprofile=coverage.out -timeout 5m
      
      - name: Coverage Gate
        run: |
          coverage=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
          if (( $(echo "$coverage < 70" | bc -l) )); then
            echo "Coverage $coverage% < 70%"
            exit 1
          fi

  build-and-push:
    needs: [test, security-scan]
    steps:
      - name: Build and push Docker image
        uses: docker/build-push-action@v5
        with:
          context: .
          push: ${{ github.ref == 'refs/heads/main' }}  # Push only on main
          tags: |
            ${{ env.ECR_REGISTRY }}/openguard-iam:${{ github.sha }}
            ${{ env.ECR_REGISTRY }}/openguard-iam:latest
          cache-from: type=gha  # GitHub Actions cache for Docker layers
          cache-to: type=gha,mode=max

  deploy-staging:
    needs: build-and-push
    if: github.ref == 'refs/heads/main'
    environment: staging
    steps:
      - name: Deploy to ECS
        run: |
          aws ecs update-service \
            --cluster openguard-staging \
            --service iam \
            --force-new-deployment
```

### GitHub Actions Security

**OIDC for AWS authentication (no static keys):**

```yaml
permissions:
  id-token: write
  contents: read

steps:
  - name: Configure AWS Credentials
    uses: aws-actions/configure-aws-credentials@v4
    with:
      role-to-assume: arn:aws:iam::123456789:role/github-actions-deploy
      aws-region: us-east-1
      # No access keys — GitHub generates a short-lived OIDC token
```

The IAM role's trust policy allows only your specific repository and branch:

```json
{
  "Condition": {
    "StringEquals": {
      "token.actions.githubusercontent.com:repo": "myorg/openguard",
      "token.actions.githubusercontent.com:ref": "refs/heads/main"
    }
  }
}
```

**Secret scanning:** Use `trufflesecurity/trufflehog-actions-scan` to detect accidentally committed secrets in PRs.

**Dependency review:** Use `actions/dependency-review-action` to block PRs that introduce vulnerable npm/pip/go dependencies.

### Deployment Strategies

**Blue/Green Deployment:**
- Two identical environments (blue = current, green = new)
- Route 100% traffic to blue, deploy to green, test green, switch all traffic to green
- Rollback = switch traffic back to blue (instant)
- AWS CodeDeploy supports this for ECS. ALB weighted target groups make it controllable.

**Canary Deployment:**
- Route 5% of traffic to new version, 95% to old
- Monitor error rates, latency for both
- Gradually increase new version percentage
- Roll back if metrics degrade
- AWS ALB weighted target groups: `BlueWeight: 95, GreenWeight: 5`

**Rolling Deployment (ECS default):**
- Replace instances one by one
- Less infrastructure overhead than blue/green
- Slower rollback (need to redeploy, not just switch traffic)

**For OpenGuard:** Blue/Green for the IAM and Policy services (revenue-critical). Rolling for the compliance and DLP services (batch/async workloads).

---

## 3.3 Infrastructure as Code — Terraform

**If you can't recreate your infrastructure from code in under an hour, you're one disaster away from losing everything.**

### Terraform Fundamentals

Terraform uses HCL (HashiCorp Configuration Language) to declare infrastructure. The core workflow:

```bash
terraform init    # Download providers, initialize backend
terraform plan    # Show what will change (always review this)
terraform apply   # Execute the changes
terraform destroy # Tear everything down
```

**State:** Terraform tracks resource state in a `.tfstate` file. In production, this file lives in an S3 bucket with DynamoDB for locking (prevents two people from running `apply` simultaneously).

```hcl
terraform {
  backend "s3" {
    bucket         = "openguard-terraform-state"
    key            = "production/iam-service.tfstate"
    region         = "us-east-1"
    dynamodb_table = "terraform-state-lock"
    encrypt        = true
  }
}
```

### OpenGuard Infrastructure in Terraform

```hcl
# VPC Module
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.0"

  name = "openguard-production"
  cidr = "10.0.0.0/16"

  azs             = ["us-east-1a", "us-east-1b", "us-east-1c"]
  private_subnets = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  public_subnets  = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]
  database_subnets = ["10.0.201.0/24", "10.0.202.0/24", "10.0.203.0/24"]

  enable_nat_gateway     = true
  one_nat_gateway_per_az = true  # HA: one per AZ, not shared
  enable_vpn_gateway     = false

  enable_flow_log                      = true
  create_flow_log_cloudwatch_iam_role  = true
  flow_log_destination_type            = "s3"
  flow_log_destination_arn             = aws_s3_bucket.vpc_flow_logs.arn
}

# ECS Cluster
resource "aws_ecs_cluster" "openguard" {
  name = "openguard-${var.environment}"

  setting {
    name  = "containerInsights"
    value = "enabled"  # CloudWatch Container Insights
  }
}

# IAM Task Role
resource "aws_iam_role" "iam_service_task" {
  name = "openguard-iam-task-${var.environment}"

  assume_role_policy = jsonencode({
    Statement = [{
      Effect = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy" "iam_service_secrets" {
  role = aws_iam_role.iam_service_task.id
  policy = jsonencode({
    Statement = [{
      Effect   = "Allow"
      Action   = ["secretsmanager:GetSecretValue"]
      Resource = [
        aws_secretsmanager_secret.jwt_keys.arn,
        aws_secretsmanager_secret.mfa_encryption_keys.arn
      ]
    }]
  })
}
```

### Terraform Anti-Patterns

1. **No state locking:** Two concurrent `apply` operations will corrupt your state file. Always use DynamoDB for locking.
2. **`terraform apply` directly in CI without plan review:** Use `-out=plan.tfplan` to save the plan, review it, then apply only that plan.
3. **Hardcoded secrets in `.tf` files:** Use `variable` with no default, and pass values via environment variables (`TF_VAR_db_password`). Better: reference Secrets Manager directly from ECS task definitions.
4. **One giant `main.tf` for everything:** Organize by service. OpenGuard has `infra/terraform/vpc/`, `infra/terraform/ecs/`, `infra/terraform/rds/`, etc.
5. **Forgetting to version-lock providers:** `version = "~> 5.0"` prevents surprise breaking changes from provider updates.

### Hands-On Project

1. Write Terraform to create the 3-tier VPC from Phase 1.3.
2. Add an Aurora PostgreSQL cluster in the database subnets.
3. Add an ECS cluster with a simple "hello world" ECS service.
4. Set up remote state in S3 + DynamoDB.
5. Create a CI pipeline that runs `terraform plan` on every PR and posts the plan output as a PR comment (use `infracost` for cost estimation).
6. Implement `terraform destroy` protection via S3 bucket lifecycle rules and DeletionPolicy.

---

# Phase 4: Distributed Systems and Orchestration

## 4.1 ECS vs EKS vs Lambda — The Real Trade-offs

**This is where most teams make their biggest mistake: choosing based on hype instead of requirements.**

### Decision Framework

```
Is your function short-lived (< 15 min), event-driven, and rarely called?
  → Lambda

Do you need simple container orchestration without Kubernetes complexity?
  → ECS (Fargate for serverless, EC2 for control)

Do you need advanced Kubernetes features: service mesh, custom operators,
complex scheduling, multi-tenancy, or your team already knows K8s?
  → EKS
```

### Lambda: Real Strengths and Real Limits

**Lambda excels at:**
- Event-driven triggers (S3 events, SQS messages, API Gateway)
- Cron-like scheduled tasks
- Glue code between AWS services
- Functions that handle variable, spiky traffic (0 to 10,000 invocations in seconds)

**Lambda struggles with:**
- Cold starts: A new Lambda container takes 100ms–2s to initialize. For a Java Lambda with Spring, cold starts are 5–15 seconds.
- Long-running processes (15-minute max)
- Stateful connections (no persistent DB connections — use RDS Proxy)
- Containers larger than 10GB
- Complex deployments with 50+ functions

**OpenGuard's Lambda use case:** The compliance report generation scheduler. A CloudWatch Events rule triggers a Lambda every hour that scans for pending report requests in PostgreSQL and enqueues them as SQS messages for the compliance ECS service to process. The Lambda runs for < 1 second and costs essentially nothing.

### ECS: The Pragmatic Choice

ECS is AWS's container orchestration service. Two launch types:

**Fargate:** Serverless containers. AWS manages the underlying EC2. You define CPU and memory, pay per second of usage. No patching, no capacity planning. Slightly more expensive per vCPU-hour but zero operational overhead.

**EC2:** You manage the EC2 instances. More control (GPU instances, custom AMIs, host-level monitoring). Required if you need instance store NVMe or specific instance families.

**OpenGuard on ECS Fargate:**
- Each service runs as a separate ECS Service
- Each service has a Task Definition with the Docker image, CPU/memory, environment variables (from Secrets Manager), and the task role
- Services run in private subnets, behind an internal ALB (not public-facing)
- The only public ALB is in front of the control-plane service
- ECS Service Auto Scaling triggers on CPU and request count metrics from CloudWatch

```hcl
resource "aws_ecs_task_definition" "iam_service" {
  family                   = "openguard-iam"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"  # Required for Fargate
  cpu                      = 2048      # 2 vCPU (bcrypt is CPU-hungry)
  memory                   = 4096      # 4 GB

  task_role_arn      = aws_iam_role.iam_service_task.arn
  execution_role_arn = aws_iam_role.ecs_execution.arn  # For ECR pull and CloudWatch

  container_definitions = jsonencode([{
    name  = "iam"
    image = "${aws_ecr_repository.iam.repository_url}:${var.image_tag}"
    portMappings = [{ containerPort = 8081 }]
    
    secrets = [
      {
        name      = "IAM_JWT_KEYS_JSON"
        valueFrom = aws_secretsmanager_secret.jwt_keys.arn
      }
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"  = "/openguard/iam"
        "awslogs-region" = "us-east-1"
        "awslogs-stream-prefix" = "ecs"
      }
    }

    healthCheck = {
      command     = ["CMD-SHELL", "curl -f http://localhost:8081/health || exit 1"]
      interval    = 30
      timeout     = 5
      retries     = 3
      startPeriod = 60
    }
  }])
}
```

### EKS: When Kubernetes is Worth It

Kubernetes is the right choice when:
- Your team already runs K8s on-prem and EKS is parity
- You need fine-grained resource management (CPU limits, QoS classes)
- You need a service mesh (Istio, Linkerd) for automatic mTLS
- You're running 50+ microservices and need custom scheduling
- You need custom operators for stateful workloads (database operators, Kafka operators)

**The operational cost is real:** EKS control plane is $0.10/hour ($72/month). That's before EC2 nodes, NAT, load balancers, and the 2–4 engineers who need Kubernetes expertise. For a 5-service application, ECS + Fargate is almost always the better choice.

**OpenGuard's EKS config (from Helm chart):**

```yaml
# infra/k8s/values.production.yaml
iam:
  replicaCount: 6
  resources:
    requests:
      cpu: "2"
      memory: "1Gi"
    limits:
      cpu: "4"
      memory: "2Gi"
  hpa:
    targetCPUUtilizationPercentage: 60  # Bcrypt saturates CPU

policy:
  replicaCount: 4
  resources:
    requests:
      cpu: "1"
      memory: "512Mi"
```

**Key Kubernetes production concerns:**

1. **Resource requests and limits:** Requests determine scheduling. Limits cap usage. A pod without requests can steal resources from others. A pod without limits can OOM the node.
2. **Pod Disruption Budgets (PDB):** `minAvailable: 2` ensures at least 2 replicas survive during node drains. Without PDB, a rolling update or node maintenance can take your service to 0.
3. **Readiness vs Liveness probes:** Readiness: is the pod ready to serve traffic? Liveness: is the pod still alive (restart if not)? Don't use the same endpoint for both.
4. **RBAC:** ServiceAccounts, Roles, and RoleBindings. Default service account has too much access. Create dedicated ServiceAccounts with minimal permissions.
5. **Secrets management:** Kubernetes Secrets are base64-encoded, not encrypted (at rest, by default). Use External Secrets Operator + AWS Secrets Manager to sync secrets into Kubernetes without storing them in etcd.

---

## 4.2 Kafka — Event-Driven Architecture

**Kafka is not a message queue. It's a distributed commit log. Understanding this distinction changes how you design systems with it.**

### The Core Concepts

**Topics** are append-only logs. Messages are not deleted when consumed — they persist until the retention period expires.

**Partitions** are the unit of parallelism. A topic with 12 partitions can have 12 consumers reading in parallel. Partition count determines max consumer parallelism.

**Consumer Groups** allow multiple independent consumers of the same topic. Each group maintains its own offset. The audit service and the threat service both consume from `data.access` — they get their own offsets and don't interfere with each other.

**Offsets** are the consumer's position in a partition. Committing an offset means "I've processed everything up to here." **Manual offset commit** is essential for exactly-once processing guarantees.

### OpenGuard's Kafka Architecture

OpenGuard uses 11 Kafka topics across its pipeline:

```
Connected App → POST /v1/events/ingest → connector-registry
    → Outbox (PostgreSQL) → Relay → kafka: data.access
        → threat service (anomaly scoring) → kafka: threat.alerts
            → alerting service → kafka: webhook.delivery
                → webhook-delivery service → external SIEM
        → audit service → MongoDB (hash-chained audit log)

IAM service → Transactional Outbox → kafka: auth.events
    → audit service → MongoDB
    → threat service (brute force detection)
```

**The Transactional Outbox Pattern** (critical for exactly-once audit):

```go
// WRONG — crash between these two = lost audit event
func CreateUser(tx *sql.Tx, user User) error {
    _, err := tx.Exec("INSERT INTO users ...")
    if err != nil { return err }
    
    // If process crashes here, user exists but event is lost
    kafka.Publish("auth.events", UserCreatedEvent{...})
    return nil
}

// CORRECT — both commit atomically or neither does
func CreateUser(tx *sql.Tx, user User) error {
    _, err := tx.Exec("INSERT INTO users ...")
    if err != nil { return err }
    
    // Outbox record in SAME transaction
    _, err = tx.Exec(
        "INSERT INTO outbox_records (topic, payload) VALUES ($1, $2)",
        "auth.events",
        json.Marshal(UserCreatedEvent{...}),
    )
    return err
    // Relay process reads committed outbox records and publishes to Kafka
    // If relay crashes, it replays from the last committed outbox record
}
```

### Kafka Production Concerns

**Replication factor of 3:** Data is replicated across 3 brokers. With `acks=all` (producer waits for all ISR replicas to acknowledge), you tolerate 2 broker failures before losing data.

**Consumer lag:** The difference between the latest offset and the consumer's committed offset. Alert on `consumer_lag > 10,000` for the audit trail topic — it means events are piling up faster than they're being processed.

**Dead Letter Queue (DLQ):** If a consumer can't process a message after N retries, it writes the message to a DLQ topic (`audit.trail.dlq`). Operators investigate and replay manually. OpenGuard has DLQ topics for the outbox relay and webhook delivery.

**Partition key design:** Messages with the same partition key go to the same partition, in order. For the audit trail, use `org_id` as the partition key — all events for one org stay ordered on one partition, making hash chain verification easier.

---

## 4.3 Kubernetes Core Concepts for Production

**Kubernetes is not just about running containers. It's about declaring desired state and letting the control plane reconcile reality to match it.**

### The Reconciliation Loop

Every Kubernetes controller runs a loop:
1. Observe current state
2. Compute desired state from specs
3. If they differ, take action
4. Repeat

This is why Kubernetes is self-healing. If a pod crashes, the ReplicaSet controller notices the actual count differs from desired count and creates a new pod.

### Essential Objects

**Deployment:** Manages ReplicaSets, which manage Pods. Handles rolling updates.

**Service:** Stable network endpoint for pods. Pods die and get new IPs; the Service IP is stable. Three types:
- `ClusterIP`: internal only (default)
- `NodePort`: exposed on each node's IP
- `LoadBalancer`: creates an AWS NLB/ALB

**Ingress:** HTTP/HTTPS routing rules managed by an Ingress Controller (AWS Load Balancer Controller for ALB).

**ConfigMap:** Non-sensitive configuration (app configs, feature flags).

**Secret:** Sensitive data (passwords, tokens). Encrypted at rest in etcd when configured with an encryption provider.

**HorizontalPodAutoscaler (HPA):** Scales deployments based on CPU, memory, or custom metrics.

**PodDisruptionBudget (PDB):** Guarantees minimum availability during voluntary disruptions (node drains, rolling updates).

### OpenGuard on EKS — Key Configuration

```yaml
# Deployment for the Policy Service
apiVersion: apps/v1
kind: Deployment
metadata:
  name: policy-service
  namespace: openguard
spec:
  replicas: 4
  selector:
    matchLabels:
      app: policy-service
  template:
    metadata:
      labels:
        app: policy-service
    spec:
      serviceAccountName: policy-service  # IRSA for Secrets Manager access
      securityContext:
        runAsNonRoot: true
        runAsUser: 10001
        readOnlyRootFilesystem: true
      containers:
        - name: policy
          image: 123456.dkr.ecr.us-east-1.amazonaws.com/openguard-policy:v1.2.3
          resources:
            requests:
              cpu: "1"
              memory: "512Mi"
            limits:
              cpu: "2"
              memory: "1Gi"
          readinessProbe:
            httpGet:
              path: /health/ready
              port: 8082
            initialDelaySeconds: 10
            periodSeconds: 5
          livenessProbe:
            httpGet:
              path: /health/live
              port: 8082
            initialDelaySeconds: 30
            periodSeconds: 15
      topologySpreadConstraints:  # Spread pods across AZs
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: DoNotSchedule
          labelSelector:
            matchLabels:
              app: policy-service
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: policy-service-pdb
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: policy-service
```

---

# Phase 5: Production Mastery

## 5.1 Observability — CloudWatch, X-Ray, and OpenTelemetry

**You can't fix what you can't see. Observability is not monitoring. Monitoring tells you when something is wrong. Observability tells you why.**

### The Three Pillars

**Metrics:** Aggregated numerical data over time. CPU %, request rates, error rates, p99 latency. Answer: "Is the system healthy?"

**Logs:** Discrete events with context. JSON structured logs with request ID, user ID, duration, error code. Answer: "What happened?"

**Traces:** End-to-end request flow across services. Answer: "Where did the time go and why did this request fail?"

### CloudWatch

**Metrics:** Every AWS service publishes metrics to CloudWatch. You pay $0.30/metric/month for custom metrics.

**Alarms:** React to metric thresholds. OpenGuard's alerting rules:

```hcl
resource "aws_cloudwatch_metric_alarm" "policy_p99_latency" {
  alarm_name          = "policy-service-p99-latency"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 3
  threshold           = 30  # 30ms — the SLO
  alarm_actions       = [aws_sns_topic.pagerduty.arn]
  
  metric_name = "openguard_policy_evaluation_duration_p99"
  namespace   = "OpenGuard"
  statistic   = "p99"
  period      = 60
}
```

**Structured Logging to CloudWatch Logs Insights:**

```go
// OpenGuard uses structured JSON logging
slog.Info("policy evaluated",
    slog.String("request_id", r.Header.Get("X-Request-ID")),
    slog.String("org_id", req.OrgID),
    slog.String("effect", decision.Effect),
    slog.String("cache_hit", decision.CacheLayer),
    slog.Duration("duration", time.Since(start)),
)
```

Query in CloudWatch Logs Insights:

```
fields @timestamp, effect, cache_hit, duration
| filter org_id = "11111111-..."
| stats avg(duration), p99(duration) by cache_hit
| sort @timestamp desc
```

**Log Insights anti-pattern:** Logging everything including request bodies. You'll accidentally log PII, secrets (passwords in form submissions), and you'll pay for terabytes of log storage. Log metadata (request ID, user ID, action, result) not payload.

### AWS X-Ray (Distributed Tracing)

X-Ray traces requests as they flow through your system. Each service adds a segment to the trace. You can see exactly how long each service took and where errors occurred.

**OpenTelemetry is the better choice for new systems.** OTEL is vendor-neutral, standard, and works with Jaeger, Grafana Tempo, Honeycomb, and Datadog. X-Ray is AWS-only.

OpenGuard uses OTEL with Jaeger (or Grafana Tempo in production):

```go
// shared/telemetry/tracer.go
func InitTracer(serviceName string) (func(), error) {
    exporter, err := otlptracehttp.New(
        context.Background(),
        otlptracehttp.WithEndpoint(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
        otlptracehttp.WithInsecure(), // mTLS handled by service mesh
    )
    
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceNameKey.String(serviceName),
            semconv.ServiceVersionKey.String(version.Version),
        )),
    )
    
    otel.SetTracerProvider(tp)
    return func() { tp.Shutdown(context.Background()) }, nil
}
```

### The Four Golden Signals (SRE Principle)

Monitor these four metrics for every service:

1. **Latency:** How long requests take. Track p50, p95, p99. Don't use averages — a p99 of 5 seconds hidden by a p50 of 10ms means 1 in 100 users has a terrible experience.
2. **Traffic:** Requests per second. Establish baseline; alert on unexpected spikes (DDoS, runaway client).
3. **Errors:** Error rate (HTTP 5xx). Separate 4xx (client errors, not your fault) from 5xx (server errors, your fault).
4. **Saturation:** How full is your system? CPU %, memory %, disk %, connection pool %, queue depth.

**OpenGuard's SLOs** (from the spec):
- Policy evaluation p99 < 30ms (cache miss), p99 < 5ms (Redis cached)
- Auth token p99 < 150ms
- Audit log ingestion p99 < 2s end-to-end (Kafka → MongoDB)

SLOs are enforced in CI: Phase 8 of OpenGuard's build process runs k6 load tests. The pipeline fails if any SLO is breached. A release doesn't ship until every SLO is green.

---

## 5.2 Security Best Practices — Zero Trust in Production

**Zero trust means: never trust, always verify. No implicit trust based on network location.**

### Zero Trust Principles Applied to OpenGuard

1. **Verify every request, even internal:** Services authenticate to each other via mTLS certificates, not by being on the same VPC. A compromised internal service cannot impersonate another.

2. **Least privilege everywhere:** Each ECS task has its own IAM role. Each service has its own DB user with table-level grants. The DLP service cannot read the IAM users table.

3. **Assume breach:** Design so that compromising one service limits the blast radius. If the webhook-delivery service is compromised, the attacker cannot access the audit log, cannot create users, cannot evaluate policies. They can only deliver webhooks (and the SSRF guard prevents them from calling internal IP addresses).

4. **Log and audit everything:** Every API call, every policy evaluation, every secret access, every IAM action. OpenGuard's audit trail is cryptographically hash-chained — you can detect if anyone deletes or modifies audit records.

### SSRF Protection (Critical for Webhook Systems)

OpenGuard's webhook delivery service sends HTTP requests to URLs provided by customers. Without SSRF protection, an attacker registers `http://169.254.169.254/latest/meta-data/iam/security-credentials/` as a webhook endpoint and receives AWS IAM credentials.

```go
// shared/middleware/ssrf_guard.go
func resolveAndValidateWebhookURL(rawURL string) error {
    u, _ := url.Parse(rawURL)
    if u.Scheme != "https" {
        return errors.New("webhook URL must use HTTPS")
    }
    
    ips, _ := net.LookupHost(u.Hostname())
    for _, ip := range ips {
        parsed := net.ParseIP(ip)
        if parsed.IsLoopback()          { return fmt.Errorf("SSRF: loopback %s", ip) }
        if parsed.IsPrivate()           { return fmt.Errorf("SSRF: private %s", ip) }
        if parsed.IsLinkLocalUnicast()  { return fmt.Errorf("SSRF: link-local %s", ip) }
        // 169.254.169.254 (AWS metadata) is link-local, blocked above
    }
    return nil
}
```

**DNS rebinding protection:** Validate the IP at registration time AND at delivery time. An attacker can make DNS resolve to a public IP at registration, then change it to `10.0.0.1` at delivery time. Re-resolve on every delivery.

### Security Headers

Every HTTP response from OpenGuard includes:

```
Strict-Transport-Security: max-age=63072000; includeSubDomains; preload
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Content-Security-Policy: default-src 'none'
Referrer-Policy: no-referrer
```

**CSP for an API:** `default-src 'none'` is correct for an API — it serves JSON, not HTML/CSS/JS. For a web app, CSP is more complex but critical for XSS prevention.

### Secret Rotation Without Downtime

OpenGuard supports rolling key rotation for JWT signing keys, MFA encryption keys, and connector API keys. The pattern:

1. Add new key with `status: active`
2. Keep old key with `status: rotating`
3. Deploy new config (both keys valid simultaneously)
4. Wait for all tokens signed by old key to expire (TTL = 15 minutes for JWTs)
5. Remove old key
6. Deploy updated config

This pattern works because token verification checks the `kid` (key ID) header and picks the matching key. Multiple valid keys coexist.

**AWS Secrets Manager rotation Lambda:** For database passwords, Secrets Manager calls your Lambda, which:
1. Generates a new password
2. Updates the database user's password
3. Updates the secret value in Secrets Manager
4. Tests the new credentials
5. Marks the rotation as successful

---

## 5.3 Cost Optimization Strategies

**AWS bills can spiral. Cost optimization is an engineering discipline, not an accounting exercise.**

### The Biggest Cost Levers

**Compute:**
- Use Graviton (ARM) instances: 20-40% cheaper for equivalent workloads. OpenGuard's Go services compile to ARM with `GOARCH=arm64`.
- Spot instances for batch workloads: 70-90% cheaper than on-demand.
- Right-size: A `c7g.4xlarge` running at 10% CPU should be a `c7g.large`. CloudWatch metrics tell you actual utilization.
- Savings Plans: 1-year commitment = 40% discount. 3-year = 60%. Use Compute Savings Plans (not EC2 instance SPs) for flexibility across ECS, EKS, Lambda.

**Data Transfer:**
- VPC Endpoints for S3, Secrets Manager, ECR: eliminates NAT Gateway data transfer charges.
- Same-AZ data transfer is free. Cross-AZ is $0.01/GB each way. Design services to be co-located when possible.
- CloudFront in front of S3 for public assets: reduces S3 GET request costs and origin data transfer.

**Storage:**
- S3 Intelligent-Tiering for objects with unknown access patterns.
- EBS volumes: Delete unattached volumes. Snapshot lifecycle policies to expire old snapshots.
- CloudWatch Logs: Set retention periods. Default is infinite retention at $0.03/GB/month.

**Database:**
- Aurora Serverless v2 for dev/staging: scales to 0 when idle. Production: provisioned for predictable latency.
- DynamoDB on-demand pricing for unpredictable traffic, provisioned + auto-scaling for steady-state.
- RDS read replicas only if you're actually using them. An unused replica costs the same as a primary.

### Cost Visibility

Set up AWS Cost Explorer alerts. Tag every resource with `Environment`, `Service`, and `Team`. Without tags, you can't tell which team or service is driving costs.

```hcl
resource "aws_ecs_service" "iam" {
  tags = {
    Environment = var.environment
    Service     = "iam"
    Team        = "platform"
    CostCenter  = "security-infrastructure"
  }
}
```

Use AWS Cost Anomaly Detection to alert when any cost category spikes unexpectedly. It found an accidentally-left-on RDS cluster that cost $1,200 in its first month.

---

## 5.4 Disaster Recovery

**DR is not about having backups. It's about having tested, automated recovery procedures with measured RTO and RPO.**

### OpenGuard's DR Architecture

| Component | Backup Method | RPO | RTO |
|-----------|--------------|-----|-----|
| PostgreSQL (IAM, Policy) | Aurora PITR (continuous WAL) | 5 min | 30 min |
| MongoDB (Audit Log) | Atlas continuous backup | 1 hour | 2 hours |
| ClickHouse (Compliance) | Daily snapshots to S3 | 24 hours | 4 hours |
| Redis | Ephemeral — no backup | 0 (in-memory) | 5 min (Sentinel failover) |
| Kafka | Replication factor 3 | 0 | 15 min |

**PostgreSQL recovery procedure:**
```bash
# Restore Aurora cluster to a point in time
aws rds restore-db-cluster-to-point-in-time \
  --source-db-cluster-identifier openguard-production \
  --db-cluster-identifier openguard-restored \
  --restore-to-time 2025-01-15T14:30:00Z \
  --vpc-security-group-ids sg-xxxxx \
  --db-subnet-group-name openguard-db-subnet-group

# Validate before switching traffic
psql $RESTORED_DATABASE_URL -c "SELECT count(*) FROM users WHERE created_at > NOW() - INTERVAL '1 hour'"
```

**Audit chain verification after restore:**
```bash
# Hash chain integrity check — detects tampered or missing records
scripts/verify-audit-chain.sh --org-id $ORG_ID --from 2025-01-01
# Output: verified 142,857 events, last_seq=142857, integrity=ok
# If tampered: integrity=FAILED, gap_at_seq=98765
```

**The drill schedule that actually works:**
- Monthly: PostgreSQL restore to staging, run full acceptance test suite
- Quarterly: Redis Sentinel failover test, Kafka partition leadership failover
- Bi-annual: Full region failover drill with stopwatch

**The mistake teams make:** DR plans that have never been tested. A backup that can't be restored is not a backup. Put DR drills on the engineering calendar.

---

## 5.5 Production Architecture: Putting It All Together

**The OpenGuard Production Architecture on AWS**

```
┌─────────────────────────────────────────────────────────────────────┐
│  Internet                                                           │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Route 53                                                           │
│  api.openguard.example.com → ALB (with health checks + failover)   │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Public Subnets (us-east-1a, 1b, 1c)                               │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │  ALB (Application Load Balancer)                               │ │
│  │  - ACM certificate (TLS termination)                          │ │
│  │  - WAF (rate limiting, IP reputation, SQL injection)          │ │
│  │  - Path routing: /v1/* → Control Plane TG                     │ │
│  │  - /auth/* → IAM Service TG                                   │ │
│  └────────────────────────────────────────────────────────────────┘ │
│  ┌──────────────────┐                                               │
│  │  NAT Gateways    │ (one per AZ)                                 │
│  │  Elastic IPs     │                                               │
│  └──────────────────┘                                               │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ▼ (private traffic only)
┌─────────────────────────────────────────────────────────────────────┐
│  Private Subnets (us-east-1a, 1b, 1c)                              │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  ECS Fargate Services (or EKS Pods)                         │   │
│  │  control-plane (2 tasks) ─mTLS→ iam (6 tasks)              │   │
│  │                          ─mTLS→ policy (4 tasks)           │   │
│  │                          ─mTLS→ connector-registry (3 tasks│   │
│  │                          ─mTLS→ audit (2 tasks)            │   │
│  │                          ─mTLS→ threat (2 tasks)           │   │
│  │                          ─mTLS→ alerting (2 tasks)         │   │
│  │                          ─mTLS→ compliance (2 tasks)       │   │
│  │                          ─mTLS→ dlp (2 tasks)              │   │
│  └─────────────────────────────────────────────────────────────┘   │
│  ┌───────────────────────────────┐                                  │
│  │  MSK (Managed Kafka)          │                                  │
│  │  3 brokers, replication=3    │                                  │
│  │  11 topics configured        │                                  │
│  └───────────────────────────────┘                                  │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Isolated Subnets (us-east-1a, 1b, 1c)                             │
│  ┌──────────────────────┐  ┌────────────────────┐                  │
│  │ Aurora PostgreSQL    │  │ DocumentDB/Atlas   │                  │
│  │ (Multi-AZ, r7g.xl)  │  │ MongoDB            │                  │
│  │ + PgBouncer pooler  │  │ (Audit Log)        │                  │
│  └──────────────────────┘  └────────────────────┘                  │
│  ┌──────────────────────┐  ┌────────────────────┐                  │
│  │ ElastiCache Redis    │  │ ClickHouse         │                  │
│  │ (Cluster mode, 6    │  │ (Compliance        │                  │
│  │  nodes across 3 AZ) │  │  Analytics)        │                  │
│  └──────────────────────┘  └────────────────────┘                  │
└─────────────────────────────────────────────────────────────────────┘

VPC Endpoints (private DNS): Secrets Manager, ECR, CloudWatch Logs, S3, STS
S3 Buckets: compliance-reports, audit-exports, vpc-flow-logs, terraform-state
CloudWatch: Metrics, Logs, Alarms → SNS → PagerDuty
X-Ray / OTEL: Distributed traces → Grafana Tempo
```

---

# Hands-On Projects by Phase

## Beginner Projects

**Project 1: OpenGuard Local Setup**
1. Clone OpenGuard, copy `.env.example` to `.env`
2. `make certs` to generate mTLS certificates
3. `make dev` to start all services via Docker Compose
4. Access the dashboard at `http://localhost:4200`
5. Register a connector, create a policy, push an audit event
6. Watch the event flow through Kafka into the audit log

**Project 2: VPC and IAM from Scratch**
Build the 3-tier VPC architecture in Terraform. Add IAM roles for EC2, ECS, and Lambda. Test the roles with AWS CLI. Break something (misconfigure a security group) and fix it using VPC Flow Logs.

## Intermediate Projects

**Project 3: Deploy OpenGuard to ECS Fargate**
1. Push Docker images to ECR
2. Write Terraform for ECS cluster, task definitions, services
3. Set up ALB with path-based routing
4. Configure Secrets Manager for JWT keys
5. Set up CloudWatch dashboards for the four golden signals
6. Run k6 load tests, see where the system breaks

**Project 4: CI/CD Pipeline for OpenGuard**
Implement the full GitHub Actions pipeline: lint → security scan → test → build → push to ECR → deploy to staging. Add OIDC authentication (no static keys). Add cost estimation with Infracost.

## Advanced Projects

**Project 5: Kubernetes Deployment with EKS**
1. Deploy OpenGuard on EKS using the Helm chart
2. Configure IRSA for Secrets Manager access
3. Set up Cluster Autoscaler and HPA for all services
4. Add Pod Disruption Budgets
5. Install Istio for automatic mTLS between services
6. Configure External Secrets Operator to sync from AWS Secrets Manager

**Project 6: Chaos Engineering**
1. Kill the Redis cluster (simulate ElastiCache failure). Does policy evaluation fail closed as expected?
2. Terminate the primary PostgreSQL node. Measure actual failover time.
3. Kill the Kafka leader for `audit.trail`. Verify no events are lost.
4. Kill 2/3 of the IAM ECS tasks. Verify the remaining tasks handle the load.
5. Disconnect the NAT Gateway. Verify services can still reach Secrets Manager via VPC Endpoint.

## Production Mastery Project

**Project 7: Full Observability Stack**
1. Set up CloudWatch dashboards for all four golden signals per service
2. Create SLO burn rate alarms (30-day error budget, alerting when burn rate > 1x for 1 hour)
3. Instrument Go services with OTEL, ship traces to Grafana Tempo
4. Set up structured logging with correlation IDs flowing from ALB request ID → every service → every log line
5. Create a runbook for every alarm: what it means, what to check, how to fix it

---

# Decision-Making Quick Reference

## ECS vs EKS vs Lambda

| Scenario | Recommendation |
|----------|---------------|
| New team, 3-10 services, AWS-first | ECS Fargate |
| Existing Kubernetes expertise | EKS |
| Event-driven, < 15min runtime | Lambda |
| Need GPU, custom instances | ECS EC2 |
| 50+ services, service mesh required | EKS + Istio |
| Mobile backend, API Gateway | Lambda + API Gateway |

## RDS vs DynamoDB

| Scenario | Recommendation |
|----------|---------------|
| Complex queries, JOINs, reporting | PostgreSQL (Aurora) |
| Session storage, feature flags | DynamoDB |
| Financial transactions, ACID required | PostgreSQL |
| IoT time-series, billions of writes | DynamoDB + TTL |
| Multi-tenant SaaS with RLS | PostgreSQL |
| Simple key-value at any scale | DynamoDB |

## Secrets Manager vs Parameter Store

| Scenario | Recommendation |
|----------|---------------|
| Database passwords, API keys, tokens | Secrets Manager |
| Automatic rotation required | Secrets Manager |
| Application config, feature flags | Parameter Store |
| High volume of reads (> 10K/month) | Parameter Store (cheaper) |
| Audit trail for every secret access | Secrets Manager |

## S3 Storage Class

| Access Pattern | Storage Class |
|----------------|--------------|
| Accessed daily | Standard |
| Unknown/unpredictable | Intelligent-Tiering |
| Accessed < once/month, latency tolerated | Standard-IA |
| Archives, accessed rarely, instant needed | Glacier Instant Retrieval |
| Archives, can wait hours | Glacier Flexible Retrieval |
| 7-year compliance archive | Glacier Deep Archive |

---

# What You Should Be Able to Do After This Roadmap

**Phase 1 Complete:** You can design a secure VPC, write IAM policies with correct least privilege, and explain DNS, TCP, and TLS without handwaving.

**Phase 2 Complete:** You can choose the right compute, storage, and database for a given workload. You can explain why OpenGuard uses PostgreSQL with RLS instead of DynamoDB.

**Phase 3 Complete:** You can write a production Dockerfile, a GitHub Actions pipeline with OIDC and security scanning, and Terraform code with remote state and proper module structure.

**Phase 4 Complete:** You can design a microservice system with Kafka, deploy it on ECS or EKS, and explain the trade-offs between every choice.

**Phase 5 Complete:** You can instrument a service for observability, respond to incidents using structured logs and distributed traces, implement zero-trust security patterns, and run DR drills that actually validate recovery.

**Production Mastery:** Given a system like OpenGuard, you can independently deploy it to AWS, harden it, observe it, optimize its cost, and recover it from any failure — in under four hours.

---

*This roadmap was built around OpenGuard (commit `dd13d0e`) — an open-source Go + Angular + Kafka security control plane that implements every pattern described here in production-quality code. The best way to learn is to deploy real systems, break them deliberately, and rebuild them better.*