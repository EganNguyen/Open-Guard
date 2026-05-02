# IAM Service TDD Test Suite Plan

## Status
- **Phase**: In Progress
- **User Decision**: All phases, systematic approach

## Overview
Add TDD test coverage for IAM service across 4 phases, following Red-Green-Refactor cycle.

## Infrastructure
- **Pattern**: Follow existing `services/iam/pkg/service/*_test.go` conventions
- **Redis**: `miniredis` for in-memory Redis
- **Repository Mocks**: Interface-based mocks (per existing pattern)
- **Test Packages**: `service`, `saga`

## Phase 1: SAML Tests
- **File**: `services/iam/pkg/service/saml_*.go` → `saml_test.go`
- **Test Cases**:
  - [ ] `UpsertSAMLProvider`: empty orgID → `ErrInvalidInput`
  - [ ] `UpsertSAMLProvider`: missing required fields → `ErrInvalidInput`
  - [ ] `UpsertSAMLProvider`: success upsert
  - [ ] `ProvisionOrGetSAMLUser`: existing user by externalID
  - [ ] `ProvisionOrGetSAMLUser`: link by email fallback
  - [ ] `ProvisionOrGetSAMLUser`: JIT provisioning new user
  - [ ] `ProvisionOrGetSAMLUser`: empty email → error

## Phase 2: Saga Watcher Tests
- **File**: `services/iam/pkg/saga/watcher.go` → `watcher_test.go`
- **Test Cases**:
  - [ ] `checkExpired`: publishes compensation for timeout
  - [ ] `checkExpired`: handles empty deadline set
  - [ ] `checkExpired`: handles publish failure

## Phase 3: User Management Tests
- **File**: `services/iam/pkg/service/users.go` → `users_test.go`
- **Test Cases**:
  - [ ] `RegisterUser`: happy path with password
  - [ ] `RegisterUser`: weak password → `ErrWeakPassword`
  - [ ] `RegisterUser`: creates outbox event
  - [ ] `RegisterUser`: creates saga deadline
  - [ ] `PatchUser`: set active=true (suspend→active)
  - [ ] `PatchUser`: set active=false (suspend user)
  - [ ] `PatchUser`: update displayName
  - [ ] `PatchUser`: creates outbox event
  - [ ] `ReprovisionUser`: happy path
  - [ ] `ReprovisionUser`: user not found
  - [ ] `OffboardOrg`: revokes all user sessions
  - [ ] `OffboardOrg`: creates outbox event
  - [ ] `ListUsers`: with email filter
  - [ ] `ListUsers`: system org access
  - [ ] `ListUsersPaginated`: returns correct total

## Phase 4: WebAuthn/MFA Tests
- **File**: `services/iam/pkg/service/mfa.go` → `mfa_webauthn_test.go`
- **Test Cases**:
  - [ ] `BeginWebAuthnRegistration`: webauthn not configured
  - [ ] `BeginWebAuthnRegistration`: user not found
  - [ ] `FinishWebAuthnRegistration`: session expired
  - [ ] `FinishWebAuthnRegistration`: credential saved
  - [ ] `BeginWebAuthnLogin`: user not found
  - [ ] `FinishWebAuthnLogin`: updates sign count
  - [ ] `VerifyBackupCode`: valid code
  - [ ] `VerifyBackupCode`: invalid code
  - [ ] `VerifyBackupCodeAndLogin`: happy path

## TDD Cycle
```
RED → write failing test → GREEN → minimal code → REFACTOR → repeat
```