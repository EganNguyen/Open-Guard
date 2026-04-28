#!/bin/bash
# OpenGuard Experience Ingestion Script
# Usage: ./scripts/remember.sh "JOB-ID" "Category" "Insight" "Trap1, Trap2" "file1, file2"

if [ "$#" -lt 3 ]; then
  echo "Usage: $0 <JOB-ID> <Category> <Insight> [Traps] [Files]"
  exit 1
fi

JOB_ID=$1
CATEGORY=$2
INSIGHT=$3
TRAPS=$4
FILES=$5
LEDGER="docs/index/memory/EXPERIENCE_LEDGER.json"

# Create a simple JSON entry
NEW_ENTRY=$(cat <<EOF
    {
      "id": "LTM-$(date +%s)",
      "ref_job": "$JOB_ID",
      "category": "$CATEGORY",
      "insight": "$INSIGHT",
      "traps": [$(echo "$TRAPS" | sed 's/, /","/g' | sed 's/^/"/' | sed 's/$/"/')],
      "impacted_files": [$(echo "$FILES" | sed 's/, /","/g' | sed 's/^/"/' | sed 's/$/"/')],
      "status": "Logged"
    }
EOF
)

# Append to JSON (simple sed hack for this prototype)
# We remove the last ] and } and append the new entry
sed -i '' '$d' "$LEDGER"
sed -i '' '$d' "$LEDGER"
echo "    ," >> "$LEDGER"
echo "$NEW_ENTRY" >> "$LEDGER"
echo "  ]" >> "$LEDGER"
echo "}" >> "$LEDGER"

echo "✅ Knowledge ingested into Experience Ledger."
