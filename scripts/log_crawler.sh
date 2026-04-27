#!/bin/bash

# OpenGuard Log Crawler & Fixer Pipeline
# This script crawls Loki for ERROR logs and categorizes them.

LOKI_URL="http://localhost:3100/loki/api/v1/query_range"
QUERY='{job=~"openguard-.*"} | json | level="ERROR"'

echo "--- Crawling Loki for ERROR logs (last 5m) ---"

# Fetch logs from the last 5 minutes
RESULTS=$(curl -G -s "$LOKI_URL" \
  --data-urlencode "query=$QUERY" \
  --data-urlencode "start=$(date -v-5M +%s)" \
  | jq -r '.data.result[].values[][1]')

if [ -z "$RESULTS" ]; then
    echo "No errors found in the last 5 minutes. System healthy."
    exit 0
fi

echo "$RESULTS" | jq -c '.' | while read -r log; do
    MSG=$(echo "$log" | jq -r '.msg')
    COMP=$(echo "$log" | jq -r '.component // "unknown"')
    
    echo "DETECTED: [$COMP] $MSG"
    
    # Categorization & Action Suggestions
    if [[ "$MSG" == *"HTTP 200"* ]]; then
        echo "  -> ACTION: Fix telemetry middleware (Status 200 should not be ERROR)"
    elif [[ "$MSG" == *"migrations failed"* ]]; then
        echo "  -> ACTION: Fix Dockerfile migration path for $COMP"
    elif [[ "$MSG" == *"Unauthorized"* ]]; then
        echo "  -> ACTION: Check frontend token management / logout logic"
    fi
done
