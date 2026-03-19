#!/usr/bin/env bash
# seed.sh — Seed the database with sample data for development.
set -euo pipefail

: "${POSTGRES_HOST:=localhost}"
: "${POSTGRES_PORT:=5432}"
: "${POSTGRES_USER:=openguard}"
: "${POSTGRES_PASSWORD:=change-me}"
: "${POSTGRES_DB:=openguard}"

export PGPASSWORD="$POSTGRES_PASSWORD"

echo "==> Seeding database (placeholder — no seed data defined yet)"
echo "==> Done."
