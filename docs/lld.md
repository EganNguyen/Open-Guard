# Open-Guard — Low-Level Design (LLD)

> 6-step LLD/OOD framework applied to Open-Guard's domain model.

---

## STEP 1: The Setup — Clarify Requirements

| Aspect | Answer |
|--------|--------|
| **Actors** | `Admin` (dashboard), `User` (via SDK), `System` (services, Kafka consumers), `External` (SIEM, Webhook targets) |
| **Functional** | Auth/IAM, Policy eval, Threat detection, DLP scanning, Audit trail, Compliance reports, Alerting, Webhook delivery |
| **Non-Functional** | mTLS encrypted, RLS multi-tenant, fail-closed (60s TTL), async audit (Transactional Outbox), CQRS, circuit breakers |
| **Scale** | Distributed microservices — event-driven via Kafka, horizontal scaling |

---

## STEP 2: Structure — Define Entities

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                        CORE DOMAIN ENTITIES                                      │
├──────────────────────────────────────────────────────────────────────────────────┤
│                                                                                   │
│  ┌────────────────────────┐    ┌────────────────────────┐                         │
│  │         User           │    │      Organization      │                         │
│  ├────────────────────────┤    ├────────────────────────┤                         │
│  │ ID, OrgID              │    │ ID, Name, Slug         │                         │
│  │ Email, PasswordHash    │    │ CreatedAt, UpdatedAt   │                         │
│  │ DisplayName, Role      │    └────────────────────────┘                         │
│  │ Status, FailedLogin    │            1   has  N                                │
│  │ LockedUntil            │◄───────────────                                         │
│  │ MFAEnabled, MFAMethod  │                                                       │
│  │ SCIMExternalID         │                                                       │
│  │ Version (optimistic)   │                                                       │
│  │ CreatedAt, UpdatedAt   │                                                       │
│  └────────────────────────┘                                                       │
│                                                                                   │
│  ┌────────────────────────┐    ┌────────────────────────┐                         │
│  │        Policy          │    │      Assignment        │                         │
│  ├────────────────────────┤    ├────────────────────────┤                         │
│  │ ID, OrgID              │    │ ID, OrgID              │                         │
│  │ Name, Description      │1   │ PolicyID ──────────────│──── FK                   │
│  │ Logic (json.RawMessage)│◄───│ SubjectID (user/group) │                         │
│  │ Version                │    │ SubjectType             │                         │
│  │ CreatedAt, UpdatedAt   │    │ CreatedAt               │                         │
│  └────────────────────────┘    └────────────────────────┘                         │
│                                                                                   │
│  ┌────────────────────────┐    ┌────────────────────────┐                         │
│  │       Connector        │    │     SAMLProvider       │                         │
│  ├────────────────────────┤    ├────────────────────────┤                         │
│  │ ID, OrgID*             │    │ ID, OrgID              │                         │
│  │ Name, ClientSecret     │    │ EntityID, SSOURL       │                         │
│  │ RedirectURIs           │    │ SLOURL, MetadataXML    │                         │
│  └────────────────────────┘    │ SPCertPEM, SPKeyPEM    │                         │
│                                │ AttributeMap (json)    │                         │
│  ┌────────────────────────┐    │ Enabled                 │                         │
│  │    RefreshToken        │    └────────────────────────┘                         │
│  ├────────────────────────┤                                                       │
│  │ ID, OrgID, UserID      │    ┌────────────────────────┐                         │
│  │ FamilyID (uuid.UUID)   │    │  WebAuthnCredential    │                         │
│  │ ExpiresAt, Revoked     │    ├────────────────────────┤                         │
│  └────────────────────────┘    │ CredentialID, PublicKey│                         │
│                                │ AttestationType        │                         │
│                                │ SignCount              │                         │
│                                └────────────────────────┘                         │
│                                                                                   │
│  ┌────────────────────────┐    ┌────────────────────────┐                         │
│  │    DLPPolicy           │    │     DLPFinding         │                         │
│  ├────────────────────────┤    ├────────────────────────┤                         │
│  │ ID, OrgID, Name        │1   │ ID, OrgID, EventID     │                         │
│  │ Rules []string         │◄───│ PolicyID ──────────────│─── FK                   │
│  │ Action (audit|block)   │    │ FindingType, Action    │                         │
│  │ Enabled                │    │ Confidence, Matched    │                         │
│  │ CreatedAt              │    │ RedactedValue          │                         │
│  └────────────────────────┘    │ CreatedAt              │                         │
│                                └────────────────────────┘                         │
│                                                                                   │
│  ┌────────────────────────┐    ┌────────────────────────┐                         │
│  │   ThreatAlert          │    │      AuditEvent        │                         │
│  ├────────────────────────┤    ├────────────────────────┤                         │
│  │ ID (ObjectID), OrgID   │    │ map[string]interface{} │                         │
│  │ UserID, Detector       │    │ (schema-less in Mongo) │                         │
│  │ Score, Severity        │    │  + timestamp           │                         │
│  │ Status (open|ack|res)  │    │  + sequence            │                         │
│  │ CreatedAt, ResolvedAt  │    │  + integrity_hash      │                         │
│  │ MTTR, Metadata (map)   │    └────────────────────────┘                         │
│  └────────────────────────┘                                                       │
│                                                                                   │
│  ┌────────────────────────┐    ┌────────────────────────┐                         │
│  │    AlertingAlert       │    │    WebhookDelivery     │                         │
│  ├────────────────────────┤    ├────────────────────────┤                         │
│  │ ID, OrgID, Type        │    │ ID, OrgID, ConnectorID │                         │
│  │ Severity (typed enum)  │    │ EventID, TargetURL     │                         │
│  │ Status (typed enum)    │1   │ Payload (json.RawMsg)  │                         │
│  │ RiskScore, DetectorID  │◄───│ Attempts, Status       │                         │
│  │ RawEvent, SagaSteps    │    │ LastError, NextRetryAt │                         │
│  │ CreatedAt, AckAt, Res  │    └────────────────────────┘                         │
│  └────────────────────────┘                                                       │
│                                                                                   │
│  ┌────────────────────────┐    ┌────────────────────────┐                         │
│  │  ComplianceReport      │    │   ConnectorRegistry    │                         │
│  ├────────────────────────┤    ├────────────────────────┤                         │
│  │ ID, OrgID, Framework   │    │ ID, OrgID, Name        │                         │
│  │ Status, S3Key, S3SigKey│    │ ClientSecret, APIKey   │                         │
│  │ ErrorMsg               │    │ RedirectURIs, Status   │                         │
│  │ CreatedAt, UpdatedAt   │    │ CreatedAt, UpdatedAt   │                         │
│  └────────────────────────┘    └────────────────────────┘                         │
│                                                                                   │
└──────────────────────────────────────────────────────────────────────────────────┘
```

---

## STEP 3: Interface — Define APIs / Behaviors

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                         SERVICE INTERFACE LAYER                                    │
├──────────────────────────────────────────────────────────────────────────────────┤
│                                                                                   │
│  ┌── IAM Service ────────────────────────────────────────────────────────────┐    │
│  │  RegisterUser(req) → (User, error)                                         │    │
│  │  Login(email, pwd) → (TokenResponse, error)                                │    │
│  │  IssueTokens(req) → (TokenResponse, error)                                 │    │
│  │  ValidateSession(jti) → (User, error)                                      │    │
│  │  ProvisionSCIM(user) → error                                               │    │
│  │  OffboardOrg(orgID) → error                                                │    │
│  └────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                   │
│  ┌── Policy Service ─────────────────────────────────────────────────────────┐    │
│  │  Evaluate(EvaluateRequest) → EvaluateResponse (allow/deny)                 │    │
│  │  CreatePolicy(Policy) → Policy                                             │    │
│  │  AssignPolicy(policyID, subjectID) → Assignment                            │    │
│  └────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                   │
│  ┌── Threat Service ─────────────────────────────────────────────────────────┐    │
│  │  ListAlerts(orgID, filter) → []Alert                                        │    │
│  │  AcknowledgeAlert(id) → error                                              │    │
│  │  ResolveAlert(id) → error                                                  │    │
│  │  GetStats(orgID) → Stats                                                   │    │
│  └────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                   │
│  ┌── DLP Service ────────────────────────────────────────────────────────────┐    │
│  │  CreatePolicy(DLPPolicy) → DLPPolicy                                        │    │
│  │  ListFindings(orgID) → []DLPFinding                                         │    │
│  └────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                   │
│  ┌── Compliance Service ──────────────────────────────────────────────────────┐   │
│  │  GenerateReport(orgID, framework) → ComplianceReport                        │    │
│  │  ListReports(orgID) → []ComplianceReport                                    │    │
│  │  GetReportDownloadURL(reportID) → (url, error)                              │    │
│  └────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                   │
│  ┌── Alerting Service ───────────────────────────────────────────────────────┐    │
│  │  ListAlerts(orgID, filter) → []Alert                                        │    │
│  │  Acknowledge(id) → error                                                   │    │
│  │  Resolve(id) → error                                                       │    │
│  └────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                   │
│  ┌── Connector Registry ──────────────────────────────────────────────────────┐   │
│  │  CreateConnector(req) → Connector                                           │    │
│  │  FindByPrefix(prefix) → Connector                                          │    │
│  │  UpdateStatus(id, status) → error                                          │    │
│  └────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                   │
└──────────────────────────────────────────────────────────────────────────────────┘
```

---

## STEP 4: Architecture — Establish Relationships

```
╔══════════════════════════════════════════════════════════════════════════════╗
║                   CLASS RELATIONSHIP DIAGRAM                                 ║
║                                                                              ║
║  LEGEND:                                                                     ║
║    ───▷  : Inheritance/Implementation (IS-A)                                 ║
║    ──◆   : Composition (HAS-A, owns lifetime)                                ║
║    ──◇   : Aggregation (HAS-A, weak reference)                               ║
║    ───→  : Association (USES-A)                                              ║
║    ╌╌▷  : Interface Implementation (dashed)                                 ║
║    ──┼── : Interface embedding (composition)                                 ║
╚══════════════════════════════════════════════════════════════════════════════╝


             ╔══════════════════════════════╗
             ║         DETECTOR             ║  ◀══ INTERFACE (Strategy)
             ║  ─────────────────────       ║
             ║  +Run(ctx) error             ║
             ╚══════════════════════════════╝
                        △
           ┌────────────┼────────────┬──────────────┐
           │             │            │              │
     ╌╌▷   │       ╌╌▷  │      ╌╌▷   │        ╌╌▷  │
  ┌─────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐
  │BruteForce│  │Impossible│  │OffHours  │  │PrivilegeEsc  │
  │Detector  │  │Travel    │  │Detector  │  │alationDetect │
  └──────────┘  └──────────┘  └──────────┘  └──────────────┘
  ┌──────────────┐  ┌───────────┐
  │DataExfiltrat │  │AccountTake│
  │ionDetector   │  │overDetect │
  └──────────────┘  └───────────┘


  ┌───────────────────◆─────────────────────┐
  │                                         │
  ▼                                         │
┌──────────────────┐                       │
│     Alert        │                       │
│  ────────────────│                   ┌───┴──────────────┐
│  Score, Severity │                   │  AlertingAlert   │
│  Status, Metadata│                   │  ────────────────│
│  CreatedAt       │                   │  RiskScore, Type │
│  ResolvedAt      │                   │  RawEvent, Steps │
│  MTTR            │                   │  SagaSteps[]     │
└──────────────────┘                   └──────────────────┘
   ▲                                              │
   │                                              │ composes
   │  Detector ───◆ creates                       ▼
   │                                       ┌──────────────┐
   │                                       │   SagaStep   │
   │                                       │  ─────────── │
   │                                       │  Step,Status │
   │                                       │  Error,At    │
   │                                       │  Retries     │
   │                                       └──────────────┘


  ╔════════════════════════════════════════════════════════════════╗
  ║              LAYERED SERVICE ARCHITECTURE                      ║
  ╚════════════════════════════════════════════════════════════════╝

                    ┌───────────────────┐
                    │     Handler       │  (HTTP layer)
                    │  ──────────────── │
                    │  svc *Service     │───◇ aggregates
                    └─────────┬─────────┘
                              │
                    ┌─────────▼─────────┐
                    │     Service       │  (Business logic)
                    │  ──────────────── │
                    │  userRepo  UserRep│──◇ interface ref (aggregation)
                    │  tokenRep TokenRep│──◇ interface ref
                    │  sessionRepo     │──◇ interface ref
                    │  mfaRepo         │──◇ interface ref
                    │  rdb *redis.Client│──◇ concrete ref
                    │  dbBreaker       │──◇ circuit breaker
                    │  pool *WorkerPool│──◇ bcrypt pool
                    └─────────┬─────────┘
                              │
                    ┌─────────▼─────────┐
                    │    Repository     │  (Data access)
                    │  ──────────────── │
                    │  pool *pgxpool.Pool│
                    └─────────┬─────────┘
                              │
                    ┌─────────▼─────────┐
                    │   PostgreSQL      │
                    │   (RLS enforced)  │
                    └───────────────────┘


         ╔═══════════════════════════════════════════════╗
         ║     INTERFACE COMPOSITION via EMBEDDING       ║
         ╚═══════════════════════════════════════════════╝

  ┌──────────────────────────────────────────────────────┐
  │              Repository (IAM master interface)        │
  │  ──────────────────────────────────────────────────   │
  │  embeds UserRepository       ──┼──  (User CRUD)       │
  │  embeds SessionRepository    ──┼──  (Session mgmt)    │
  │  embeds TokenRepository      ──┼──  (Refresh tokens)  │
  │  embeds MFARepository        ──┼──  (TOTP, Backup)    │
  │  embeds ConnectorRepository  ──┼──  (OAuth2)          │
  │  embeds WebAuthnRepository   ──┼──  (Passkeys)        │
  │  embeds SAMLRepository       ──┼──  (SAML IdP)        │
  │  embeds OrgRepository        ──┼──  (Org creation)    │
  │  embeds OutboxRepository     ──┼──  (Transactional)   │
  └──────────────────────────────────────────────────────┘
                       △
                       │ implements
              ┌────────┴────────┐
              │   Repository    │
              │  (concrete)     │
              │  pool *pgxpool  │
              └─────────────────┘

  ┌──────────────────────────────────────────────────────┐
  │              PolicyRepository (Policy master)          │
  │  ──────────────────────────────────────────────────   │
  │  embeds PolicyStore        ──┼──  (Policy CRUD)       │
  │  embeds EvalLogStore       ──┼──  (Eval records)      │
  │  embeds AssignmentStore    ──┼──  (Policy-subject)    │
  │  +Pool() *pgxpool.Pool                                 │
  └──────────────────────────────────────────────────────┘


         ╔═══════════════════════════════════════════════╗
         ║     TRANSACTIONAL OUTBOX PATTERN               ║
         ╚═══════════════════════════════════════════════╝

   ┌────────────┐     same TX      ┌─────────────────┐
   │  Service   │ ────────────────►│  PostgreSQL      │
   │  (IAM/     │    write event    │  outbox_records  │
   │   Policy)  │                  └────────┬─────────┘
   └────────────┘                           │
                                            │ pg_notify
                                            ▼
                                     ┌──────────────┐
                                     │  Outbox Relay │  (polls + publishes)
                                     └──────┬───────┘
                                            │
                                            ▼
                                     ┌──────────────┐
                                     │   Kafka       │
                                     │   Topic       │
                                     └──────────────┘


        ╔═══════════════════════════════════════════════╗
        ║     DLP SCANNER — STRATEGY TABLE PATTERN      ║
        ╚═══════════════════════════════════════════════╝

                    ┌──────────────────────┐
                    │  ScanContent(text)    │
                    │  ──────────────────── │
                    │  results := merge(    │
                    │    ScanRegex(text),   │  ◀── composite strategy
                    │    ScanEntropy(text)  │
                    │  )                   │
                    └──────────────────────┘

                   ScanRegex iterates over:
                    ┌──────────────────────────────┐
                    │  Rules = []ScanRule{           │  ◀── Strategy Table
                    │    {Kind:"email", Re: regex}, │
                    │    {Kind:"ssn", Re: regex},   │
                    │    {Kind:"credit_card",       │
                    │     Re: regex, Validate: luhn}│
                    │    {Kind:"aws_key", ...},     │
                    │    {Kind:"jwt", ...},         │
                    │  }                            │
                    └──────────────────────────────┘
```

---

## STEP 5: Optimization — Design Patterns Applied

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                     DESIGN PATTERNS IN OPEN-GUARD                                 │
├──────────────────────────────────────────────────────────────────────────────────┤
│                                                                                   │
│  ┌── STRATEGY ─────────────────────────────────────────────────────────────┐     │
│  │  ┌─────────────────────┐   ┌─────────────────────────────────────────┐  │     │
│  │  │ <<interface>>       │   │ Concrete Implementations:                │  │     │
│  │  │ Detector            │   │  • BruteForceDetector                   │  │     │
│  │  │ +Run(ctx) error     │──▷│  • ImpossibleTravelDetector             │  │     │
│  │  └─────────────────────┘   │  • OffHoursDetector                     │  │     │
│  │                            │  • DataExfiltrationDetector             │  │     │
│  │  Also used by:            │  • AccountTakeoverDetector               │  │     │
│  │  • SIEM formatForSIEM()   │  • PrivilegeEscalationDetector           │  │     │
│  │  • DLP ScanRule table     └─────────────────────────────────────────┘  │     │
│  │  • Webhook BackoffFunc                                                  │     │
│  └────────────────────────────────────────────────────────────────────────┘     │
│                                                                                   │
│  ┌── DECORATOR ─────────────────────────────────────────────────────────────┐    │
│  │  ┌──────────────────────────────────────┐                                │    │
│  │  │ CircuitBreakerTransport              │                                │    │
│  │  │  ──────────────────────────────────── │  wraps http.RoundTripper      │    │
│  │  │  cb *gobreaker.CircuitBreaker        │  with circuit breaker logic    │    │
│  │  │  rt http.RoundTripper (inner)        │                                │    │
│  │  └──────────────────────────────────────┘                                │    │
│  └────────────────────────────────────────────────────────────────────────┘     │
│                                                                                   │
│  ┌── SAGA ─────────────────────────────────────────────────────────────────┐     │
│  │  ┌──────────────────────┐  ┌──────────────────────────┐                 │     │
│  │  │ IAM Saga (Provision) │  │ Alerting Saga (4-step)    │                 │     │
│  │  │  Consumer + Watcher  │  │  1. persist              │                 │     │
│  │  │  Handles:            │  │  2. notify               │                 │     │
│  │  │  • user.provisioned  │  │  3. siem (HMAC webhook)  │                 │     │
│  │  │  • provisioning.fail │  │  4. audit trail          │                 │     │
│  │  │  • org.offboard      │  └──────────────────────────┘                 │     │
│  │  └──────────────────────┘                                               │     │
│  └────────────────────────────────────────────────────────────────────────┘     │
│                                                                                   │
│  ┌── CQRS ──────────────────────────────────────────────────────────────────┐    │
│  │  ┌────────────────────────┐  ┌─────────────────────────┐                 │    │
│  │  │ AuditWriteRepository   │  │ AuditReadRepository     │                 │    │
│  │  │  (MongoDB write-optim) │  │  (MongoDB read-optim)   │                 │    │
│  │  └────────────────────────┘  └─────────────────────────┘                 │    │
│  └────────────────────────────────────────────────────────────────────────┘     │
│                                                                                   │
│  ┌── FACTORY ──────────────────────────────────────────────────────────────┐    │
│  │  NewService(), NewHandler(), NewRepository(), NewConsumer()              │    │
│  │  across ALL services — idiomatic Go constructors                        │    │
│  └────────────────────────────────────────────────────────────────────────┘     │
│                                                                                   │
│  ┌── CIRCUIT BREAKER ───────────────────────────────────────────────────────┐   │
│  │  gobreaker in: IAM Service (redis), Policy Service (db), Control Plane   │    │
│  │  5-failure threshold, 30s open, then half-open                           │    │
│  └────────────────────────────────────────────────────────────────────────┘     │
│                                                                                   │
│  ┌── SINGLETON (via singleflight) ──────────────────────────────────────────┐    │
│  │  Policy Service uses singleflight.Group for cache-miss deduplication      │    │
│  └────────────────────────────────────────────────────────────────────────┘     │
│                                                                                   │
│  ┌── TRANSACTIONAL OUTBOX ──────────────────────────────────────────────────┐    │
│  │  Service writes event in same PG TX as business logic                    │    │
│  │  → OutboxRelay polls → publishes to Kafka →                              │    │
│  │    exactly-once delivery                                                   │    │
│  └────────────────────────────────────────────────────────────────────────┘     │
│                                                                                   │
│  ┌── DEAD-LETTER QUEUE ────────────────────────────────────────────────────┐    │
│  │  ▸ outbox.dlq   (outbox publish failures)                                │    │
│  │  ▸ webhook.dlq  (webhook delivery, 5 retries)                            │    │
│  │  ▸ dlp.dlq      (DLP scan failures, 5 consecutive)                      │    │
│  └────────────────────────────────────────────────────────────────────────┘     │
│                                                                                   │
└──────────────────────────────────────────────────────────────────────────────────┘
```

---

## STEP 6: Implementation — Code & Concurrency

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                     KEY IMPLEMENTATION PATTERNS                                   │
├──────────────────────────────────────────────────────────────────────────────────┤
│                                                                                   │
│  ┌── Dependency Injection ───────────────────────────────────────────────────┐    │
│  │                                                                           │    │
│  │  // interfaces.go — contracts                                             │    │
│  │  type UserRepository interface {                                          │    │
│  │      CreateUser(ctx, *User) (*User, error)                                │    │
│  │      GetUserByEmail(ctx, orgID, email) (*User, error)                     │    │
│  │      LockAccount(ctx, userID) error                                      │    │
│  │  }                                                                        │    │
│  │                                                                           │    │
│  │  // service.go — injected dependency                                      │    │
│  │  type Service struct {                                                    │    │
│  │      userRepo UserRepository     // interface field (aggregation)         │    │
│  │      rdb      *redis.Client      // concrete field                        │    │
│  │  }                                                                        │    │
│  │                                                                           │    │
│  │  func NewService(repo UserRepository, rdb *redis.Client) *Service {       │    │
│  │      return &Service{userRepo: repo, rdb: rdb}                            │    │
│  │  }                                                                        │    │
│  └──────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                   │
│  ┌── Thread Safety (Concurrency Patterns) ──────────────────────────────────┐    │
│  │                                                                           │    │
│  │  // 1. errgroup for goroutine ownership                                   │    │
│  │  g, ctx := errgroup.WithContext(parentCtx)                                │    │
│  │  g.Go(func() error { return detector.Run(ctx) })                         │    │
│  │  g.Go(func() error { return other.Run(ctx) })                            │    │
│  │  if err := g.Wait(); err != nil { log.Error("detector failed", err) }    │    │
│  │                                                                           │    │
│  │  // 2. Bulkhead pattern (bounded concurrency)                             │    │
│  │  type Bulkhead struct {                                                   │    │
│  │      sem chan struct{}  // buffered channel as semaphore                 │    │
│  │  }                                                                        │    │
│  │  func (b *Bulkhead) Execute(ctx, fn func() error) error {                │    │
│  │      select {                                                             │    │
│  │      case b.sem <- struct{}{}:                                            │    │
│  │          defer func() { <-b.sem }()                                       │    │
│  │          return fn()                                                      │    │
│  │      case <-ctx.Done():                                                   │    │
│  │          return ctx.Err()   // timeout/full → fail fast                  │    │
│  │      }                                                                    │    │
│  │  }                                                                        │    │
│  │                                                                           │    │
│  │  // 3. Worker pool (bcrypt)                                               │    │
│  │  type AuthWorkerPool struct {                                             │    │
│  │      jobs  chan work                                                      │    │
│  │      wg    sync.WaitGroup                                                 │    │
│  │  }                                                                        │    │
│  │                                                                           │    │
│  │  // 4. Singleflight for cache stampede protection                        │    │
│  │  sfGroup singleflight.Group                                               │    │
│  │  result, err, _ = sfGroup.Do(cacheKey, func() (interface{}, error) {     │    │
│  │      return repo.GetMatchingPolicies(...)  // one caller hits DB         │    │
│  │  })                                                                       │    │
│  │                                                                           │    │
│  │  // 5. RLS context propagation                                           │    │
│  │  ctx = rls.WithOrgID(parentCtx, orgID)                                   │    │
│  │  conn, _ := pool.Acquire(ctx)                                            │    │
│  │  rls.SetSessionVar(ctx, conn, orgID)  // sets app.current_org_id        │    │
│  │                                                                           │    │
│  │  // 6. Retry with exponential backoff                                    │    │
│  │  var backoff = func(attempt int) time.Duration {                         │    │
│  │      return time.Duration(math.Pow(2, float64(attempt))) * time.Second   │    │
│  │  }                                                                        │    │
│  └──────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                   │
│  ┌── Error Handling (Wrap at Boundaries) ───────────────────────────────────┐    │
│  │                                                                           │    │
│  │  // Handler layer (HTTP): log + return error response                    │    │
│  │  func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {       │    │
│  │      token, err := h.svc.Login(r.Context(), req)                         │    │
│  │      if err != nil {                                                     │    │
│  │          h.logger.Error("login failed", telemetry.SafeAttr("error",err)) │    │
│  │          http.Error(w, "invalid credentials", 401)                       │    │
│  │          return                                                          │    │
│  │      }                                                                    │    │
│  │      json.NewEncoder(w).Encode(token)                                     │    │
│  │  }                                                                        │    │
│  │                                                                           │    │
│  │  // Service layer: wrap with context                                     │    │
│  │  func (s *Service) Login(ctx, req) (*TokenResponse, error) {             │    │
│  │      user, err := s.userRepo.GetUserByEmail(ctx, req.OrgID, req.Email)   │    │
│  │      if err != nil { return nil, fmt.Errorf("get user: %w", err) }      │    │
│  │      // ... validate password...                                          │    │
│  │  }                                                                        │    │
│  └──────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                   │
└──────────────────────────────────────────────────────────────────────────────────┘
```

---

## Full Relationship Summary

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│  ENTITY              │  RELATIONSHIP                    │  CARDINALITY           │
├─────────────────────────────────────────────────────────────────────────────────┤
│  Organization  ──◆── User         │  Composition (org owns users) │  1 : N       │
│  User          ───→ RefreshToken  │  Association                 │  1 : N       │
│  User          ───→ Session       │  Association (one active)    │  1 : 1*      │
│  User          ───→ MFAConfig     │  Association                 │  1 : 1       │
│  Policy        ──◇── Assignment   │  Aggregation (weak ref)     │  1 : N       │
│  Policy        ───→ EvalLog       │  Association                 │  1 : N       │
│  Organization  ───→ Policy        │  Association (tenant scope)  │  1 : N       │
│  DLPPolicy     ──◇── DLPFinding   │  Aggregation                 │  1 : N       │
│  Alert (Threat)───→ Detector      │  Association                 │  N : 1       │
│  AlertingAlert ◆── SagaStep[]     │  Composition (embedded)      │  1 : N       │
│  Detector      ╌╌▷ <<interface>>  │  Implementation (strategy)   │  N : 1       │
│  Repository    ──┼── (9 sub-if)   │  Interface Composition       │  composed    │
│  PolicyRepo    ──┼── (3 sub-if)   │  Interface Composition       │  composed    │
│  Service       ──◇── Repo iface   │  Aggregation (DI)           │  1 : N       │
│  Handler       ──◇── Service      │  Aggregation (DI)           │  1 : 1       │
│  Service       ──◇── *redis.Client│  Aggregation                 │  1 : 1       │
│  Service       ──◇── CircuitBreak │  Aggregation                 │  1 : N       │
│  AuditWriteRep ╌╌▷ AuditReadRep  │  CQRS siblings (same source) │  N : 1       │
│  EventEnvelope ───→ Kafka Topic   │  Association                 │  1 : 1       │
└─────────────────────────────────────────────────────────────────────────────────┘
```
