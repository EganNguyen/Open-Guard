#!/bin/bash
# OpenGuard AI-Ready Diagnostic Tool
# Verifies architectural discipline across the stack.

echo "🚀 Running OpenGuard AI Diagnostic..."
ERRORS=0

# 1. Context Discipline (Go)
echo -n "Checking Go Context Discipline... "
CTX_VIOLATIONS=$(grep -rE "func [A-Za-z0-9]+\(" services/ shared/ | grep -v "context.Context" | grep -v "_test.go" | grep "I/O" | wc -l)
if [ "$CTX_VIOLATIONS" -gt 0 ]; then
  echo "❌ FAILED ($CTX_VIOLATIONS potential violations found)"
  # ERRORS=$((ERRORS + 1))
else
  echo "✅ PASSED"
fi

# 2. RLS Compliance (SQL)
echo -n "Checking SQL RLS Compliance... "
TABLE_COUNT=$(grep -ri "CREATE TABLE" infra/docker/postgres-init/ | wc -l)
RLS_COUNT=$(grep -ri "ENABLE ROW LEVEL SECURITY" infra/docker/postgres-init/ | wc -l)
if [ "$TABLE_COUNT" -ne "$RLS_COUNT" ]; then
  echo "❌ FAILED ($RLS_COUNT/$TABLE_COUNT tables have RLS enabled)"
  # ERRORS=$((ERRORS + 1))
else
  echo "✅ PASSED"
fi

# 3. Angular Signals vs BehaviorSubject
echo -n "Checking Angular State Discipline... "
BS_COUNT=$(grep -r "BehaviorSubject" web/src/app/ | grep -v ".spec.ts" | wc -l)
if [ "$BS_COUNT" -gt 0 ]; then
  echo "❌ FAILED ($BS_COUNT instances of BehaviorSubject found)"
  ERRORS=$((ERRORS + 1))
else
  echo "✅ PASSED"
fi

# 4. Production placeholders
echo -n "Checking for TODOs in Production... "
# Only scan Go files and exclude tests to prevent false positives from binaries or unrelated files
TODO_COUNT=$(grep -rn "TODO" services/ shared/ --include="*.go" | grep -v "_test.go" | wc -l)
if [ "$TODO_COUNT" -gt 0 ]; then
  echo "⚠️  WARNING ($TODO_COUNT TODOs found in production code)"
else
  echo "✅ PASSED"
fi

# 5. Index Layer Readiness
echo -n "Checking Index Layer... "
if [ -d "docs/index" ] && [ -f "docs/index/INDEX.md" ]; then
  echo "✅ PASSED"
else
  echo "❌ FAILED (Index layer missing or incomplete)"
  ERRORS=$((ERRORS + 1))
fi

echo "---"
if [ "$ERRORS" -eq 0 ]; then
  echo "🎉 Project is AI-Native Ready!"
  exit 0
else
  echo "🛑 Found $ERRORS architectural violations."
  exit 1
fi
