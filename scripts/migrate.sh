#!/usr/bin/env bash
# migrate.sh — Run all *.up.sql migrations in services/*/migrations/ in numeric order.
set -euo pipefail
# Load .env if it exists in the root directory
if [ -f "$(dirname "$0")/../.env" ]; then
    set -a
    source "$(dirname "$0")/../.env"
    set +a
fi

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
    
    # Run psql inside the docker container to avoid local dependency
    docker exec -i openguard-postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 < "$migration"
done

echo "==> Migrations complete."
