# §5 — Environment & Configuration

---

## 5.1 `.env.example` (canonical — every variable required unless marked optional)

```dotenv
# ── App ──────────────────────────────────────────────────────────────
APP_ENV=development                    # development | staging | production
LOG_LEVEL=info                         # debug | info | warn | error
LOG_FORMAT=json                        # json | text (use json in non-dev)

# ── Control Plane ────────────────────────────────────────────────────
CONTROL_PLANE_PORT=8080
CONTROL_PLANE_API_KEY_SALT=change-me-32-bytes-hex
CONTROL_PLANE_WEBHOOK_SIGNING_SECRET=change-me-32-bytes-hex
CONTROL_PLANE_POLICY_CACHE_TTL_SECONDS=60
CONTROL_PLANE_EVENT_INGEST_MAX_BATCH=500
CONTROL_PLANE_RATE_LIMIT_CONNECTOR=1000          # req/min per connector_id
CONTROL_PLANE_TENANT_QUOTA_RPM=5000              # req/min per org_id (all connectors)
CONTROL_PLANE_CONNECTOR_CACHE_TTL_SECONDS=30     # Redis TTL for connector auth cache
CONTROL_PLANE_MTLS_CERT_FILE=/certs/control-plane.crt
CONTROL_PLANE_MTLS_KEY_FILE=/certs/control-plane.key
CONTROL_PLANE_MTLS_CA_FILE=/certs/ca.crt

# ── Connector Registry ───────────────────────────────────────────────
CONNECTOR_REGISTRY_PORT=8090
CONNECTOR_REGISTRY_MTLS_CERT_FILE=/certs/connector-registry.crt
CONNECTOR_REGISTRY_MTLS_KEY_FILE=/certs/connector-registry.key
CONNECTOR_REGISTRY_MTLS_CA_FILE=/certs/ca.crt

# ── Webhook Delivery ─────────────────────────────────────────────────
WEBHOOK_DELIVERY_PORT=8091
WEBHOOK_MAX_ATTEMPTS=5
WEBHOOK_BACKOFF_BASE_MS=1000
WEBHOOK_BACKOFF_MAX_MS=60000
WEBHOOK_DELIVERY_TIMEOUT_MS=5000
WEBHOOK_DELIVERY_MTLS_CERT_FILE=/certs/webhook-delivery.crt
WEBHOOK_DELIVERY_MTLS_KEY_FILE=/certs/webhook-delivery.key
WEBHOOK_DELIVERY_MTLS_CA_FILE=/certs/ca.crt

# ── SDK ──────────────────────────────────────────────────────────────
SDK_CONTROL_PLANE_URL=https://api.openguard.example.com
SDK_POLICY_CACHE_TTL_SECONDS=60
SDK_POLICY_EVALUATE_TIMEOUT_MS=100
SDK_EVENT_BATCH_SIZE=100
SDK_EVENT_FLUSH_INTERVAL_MS=2000
SDK_OFFLINE_RETRY_LIMIT=500              # Max events buffered locally when breaker is open

# ── IAM ──────────────────────────────────────────────────────────────
IAM_PORT=8081
IAM_JWT_KEYS_JSON=[{"kid":"k1","secret":"change-me","algorithm":"HS256","status":"active"}]
IAM_JWT_EXPIRY_SECONDS=900             # 15 minutes
IAM_REFRESH_TOKEN_EXPIRY_DAYS=30
IAM_SAML_ENTITY_ID=https://openguard.example.com
IAM_SAML_IDP_METADATA_URL=https://idp.example.com/metadata
IAM_OIDC_ISSUER=https://accounts.example.com
IAM_OIDC_CLIENT_ID=openguard
IAM_OIDC_CLIENT_SECRET=change-me
IAM_SCIM_TOKENS_JSON=[{"token":"scim-t1","org_id":"00000000-0000-0000-0000-000000000000"}]
IAM_MFA_TOTP_ISSUER=OpenGuard
IAM_MFA_ENCRYPTION_KEY_JSON=[{"kid":"mk1","key":"base64-encoded-32-bytes","status":"active"}]
IAM_WEBAUTHN_RPID=openguard.example.com
IAM_WEBAUTHN_RPORIGIN=https://openguard.example.com
IAM_MTLS_CERT_FILE=/certs/iam.crt
IAM_MTLS_KEY_FILE=/certs/iam.key
IAM_MTLS_CA_FILE=/certs/ca.crt
IAM_BCRYPT_WORKER_COUNT=8                # Default: NumCPU / 2

# ── Policy ───────────────────────────────────────────────────────────
POLICY_PORT=8082
POLICY_CACHE_TTL_SECONDS=30
POLICY_MTLS_CERT_FILE=/certs/policy.crt
POLICY_MTLS_KEY_FILE=/certs/policy.key
POLICY_MTLS_CA_FILE=/certs/ca.crt

# ── Threat ───────────────────────────────────────────────────────────
THREAT_PORT=8083
THREAT_ANOMALY_WINDOW_MINUTES=60
THREAT_MAX_FAILED_LOGINS=10
THREAT_GEO_CHANGE_THRESHOLD_KM=500
THREAT_MAXMIND_DB_PATH=/data/GeoLite2-City.mmdb
THREAT_MTLS_CERT_FILE=/certs/threat.crt
THREAT_MTLS_KEY_FILE=/certs/threat.key
THREAT_MTLS_CA_FILE=/certs/ca.crt

# ── Audit ────────────────────────────────────────────────────────────
AUDIT_PORT=8084
AUDIT_RETENTION_DAYS=730
AUDIT_HASH_CHAIN_SECRET=change-me-32-bytes-hex
AUDIT_BULK_INSERT_MAX_DOCS=500
AUDIT_BULK_INSERT_FLUSH_MS=1000
AUDIT_MTLS_CERT_FILE=/certs/audit.crt
AUDIT_MTLS_KEY_FILE=/certs/audit.key
AUDIT_MTLS_CA_FILE=/certs/ca.crt

# ── Alerting ─────────────────────────────────────────────────────────
ALERTING_PORT=8085
ALERTING_SLACK_WEBHOOK_URL=            # optional
ALERTING_SMTP_HOST=smtp.example.com
ALERTING_SMTP_PORT=587
ALERTING_SMTP_USER=
ALERTING_SMTP_PASS=
ALERTING_SIEM_WEBHOOK_URL=             # optional
ALERTING_SIEM_WEBHOOK_HMAC_SECRET=change-me
ALERTING_MTLS_CERT_FILE=/certs/alerting.crt
ALERTING_MTLS_KEY_FILE=/certs/alerting.key
ALERTING_MTLS_CA_FILE=/certs/ca.crt

# ── Compliance ───────────────────────────────────────────────────────
COMPLIANCE_PORT=8086
COMPLIANCE_REPORT_MAX_CONCURRENT=10
COMPLIANCE_MTLS_CERT_FILE=/certs/compliance.crt
COMPLIANCE_MTLS_KEY_FILE=/certs/compliance.key
COMPLIANCE_MTLS_CA_FILE=/certs/ca.crt

# ── DLP ──────────────────────────────────────────────────────────────
DLP_PORT=8087
DLP_ENTROPY_THRESHOLD=4.5
DLP_MIN_CREDENTIAL_LENGTH=24
DLP_SYNC_BLOCK_TIMEOUT_MS=30
DLP_POLICY_CACHE_TTL_SECONDS=60
DLP_MTLS_CERT_FILE=/certs/dlp.crt
DLP_MTLS_KEY_FILE=/certs/dlp.key
DLP_MTLS_CA_FILE=/certs/ca.crt

# ── Outbox Relay ─────────────────────────────────────────────────────
OUTBOX_RELAY_TOTAL_INSTANCES=1
OUTBOX_RELAY_INSTANCE_INDEX=0
OUTBOX_MAX_LAG_SECONDS=10
SAGA_STEP_TIMEOUT_SECONDS=30

# ── Webhook Delivery ──────────────────────────────────────────────────
WEBHOOK_IP_REVALIDATE_INTERVAL_SECONDS=300

# ── PostgreSQL ───────────────────────────────────────────────────────
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=openguard_app
POSTGRES_PASSWORD=change-me
POSTGRES_DB=openguard
POSTGRES_SSLMODE=verify-full
POSTGRES_SSLROOTCERT=/certs/postgres-ca.crt
POSTGRES_POOL_MIN_CONNS=5
POSTGRES_POOL_MAX_CONNS=25
POSTGRES_OUTBOX_USER=openguard_outbox
POSTGRES_OUTBOX_PASSWORD=change-me
POSTGRES_MIGRATE_USER=openguard_migrate
POSTGRES_MIGRATE_PASSWORD=change-me

# ── MongoDB ──────────────────────────────────────────────────────────
MONGO_URI_PRIMARY=mongodb://localhost:27017
MONGO_URI_SECONDARY=mongodb://localhost:27018
MONGO_DB=openguard
MONGO_AUTH_SOURCE=admin
MONGO_TLS_CA_FILE=/certs/mongo-ca.crt
MONGO_WRITE_POOL_MAX=10
MONGO_READ_POOL_MAX=30

# ── Redis ────────────────────────────────────────────────────────────
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=change-me
REDIS_DB=0
REDIS_TLS_CERT_FILE=/certs/redis.crt
REDIS_POOL_SIZE=20

# ── Kafka ────────────────────────────────────────────────────────────
KAFKA_BROKERS=localhost:9092
KAFKA_CLIENT_ID=openguard
KAFKA_TLS_CA_FILE=/certs/kafka-ca.crt
KAFKA_SASL_USER=openguard
KAFKA_SASL_PASSWORD=change-me
KAFKA_PRODUCER_IDEMPOTENT=true

# ── ClickHouse ───────────────────────────────────────────────────────
CLICKHOUSE_ADDR=localhost:9000
CLICKHOUSE_USER=openguard
CLICKHOUSE_PASSWORD=change-me
CLICKHOUSE_DB=openguard
CLICKHOUSE_TLS_CA_FILE=/certs/clickhouse-ca.crt

# ── Circuit Breakers ─────────────────────────────────────────────────
CB_POLICY_TIMEOUT_MS=50
CB_POLICY_FAILURE_THRESHOLD=5
CB_POLICY_OPEN_DURATION_MS=10000
CB_IAM_TIMEOUT_MS=200
CB_IAM_FAILURE_THRESHOLD=5
CB_IAM_OPEN_DURATION_MS=15000

# ── OpenTelemetry ────────────────────────────────────────────────────
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
OTEL_SAMPLING_RATE=0.1

# ── Org Lifecycle ────────────────────────────────────────────────────
ORG_DATA_RETENTION_DAYS=2555           # 7 years (compliance baseline)
ORG_OFFBOARDING_GRACE_PERIOD_DAYS=30

# ── Frontend ─────────────────────────────────────────────────────────
NEXT_PUBLIC_API_URL=http://localhost:8080
NEXTAUTH_URL=http://localhost:3000
NEXTAUTH_SECRET=change-me
```

---

## 5.2 Config Loading Pattern

```go
// shared/config/config.go
package config

import (
    "encoding/json"
    "fmt"
    "os"
    "strconv"
    "time"
)

func Must(key string) string {
    v := os.Getenv(key)
    if v == "" {
        panic(fmt.Sprintf("required env var %q not set", key))
    }
    return v
}

func Default(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}

func MustInt(key string) int {
    v := Must(key)
    n, err := strconv.Atoi(v)
    if err != nil {
        panic(fmt.Sprintf("env var %q must be int, got %q", key, v))
    }
    return n
}

func DefaultInt(key string, fallback int) int {
    v := os.Getenv(key)
    if v == "" {
        return fallback
    }
    n, err := strconv.Atoi(v)
    if err != nil {
        panic(fmt.Sprintf("env var %q must be int, got %q", key, v))
    }
    return n
}

func MustDuration(key string) time.Duration {
    return time.Duration(MustInt(key)) * time.Millisecond
}

func MustJSON(key string, dest any) {
    v := Must(key)
    if err := json.Unmarshal([]byte(v), dest); err != nil {
        panic(fmt.Sprintf("env var %q is not valid JSON: %v", key, err))
    }
}
```
