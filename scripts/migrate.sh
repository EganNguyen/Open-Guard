#!/usr/bin/env bash
# migrate.sh — Run all *.up.sql migrations in services/*/migrations/ in numeric order.
set -euo pipefail

: "${POSTGRES_HOST:=localhost}"
: "${POSTGRES_PORT:=5432}"
: "${POSTGRES_USER:=openguard}"
: "${POSTGRES_PASSWORD:=change-me}"
: "${POSTGRES_DB:=openguard}"

export PGPASSWORD="$POSTGRES_PASSWORD"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

echo "==> Running migrations against ${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}"

# Find all *.up.sql files, sort numerically by filename
find "$ROOT_DIR/services" -path "*/migrations/*.up.sql" | sort -t/ -k"$(echo "$ROOT_DIR/services" | tr '/' '\n' | wc -l | tr -d ' ')" | while read -r migration; do
    filename="$(basename "$migration")"
    service_dir="$(basename "$(dirname "$(dirname "$migration")")")"
    echo "  -> [$service_dir] $filename"
    psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f "$migration" -v ON_ERROR_STOP=1
done

echo "==> Migrations complete."
