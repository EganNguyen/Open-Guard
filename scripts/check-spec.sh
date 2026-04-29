#!/bin/bash

# check-spec.sh
# Verifies that mandatory architectural patterns from ai-spec exist in the codebase.

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

FAILED=0

check_pattern() {
    local name=$1
    local pattern=$2
    local include=$3
    
    echo -n "Checking $name... "
    if grep -rE "$pattern" $include > /dev/null; then
        echo -e "${GREEN}PASS${NC}"
    else
        echo -e "${RED}FAIL${NC}"
        echo "  Expected pattern: $pattern"
        FAILED=1
    fi
}

echo "Running AI-Spec Alignment Check..."
echo "=================================="

# 1. Outbox Patterns
check_pattern "Outbox Trigger (pg_notify)" "notify_outbox" "services"
check_pattern "Outbox Relay (pg_notify LISTEN)" "notifyChannel = \"outbox_new\"" "shared/kafka/outbox"

# 2. RLS Patterns
check_pattern "RLS AfterRelease Hook" "AfterRelease" "services"
check_pattern "RLS set_config(app.org_id)" "set_config\('app.org_id'" "shared/rls"

# 3. Security Patterns
check_pattern "Auth Bcrypt Worker Pool" "bcryptCompareJob|AuthWorkerPool" "services/iam"
check_pattern "Safe HTTP Client" "NewSafeHTTPClient" "shared/middleware"

# 4. Frontend Patterns
check_pattern "Angular Signals" "signal\(" "web/src/app"
check_pattern "Angular Standalone" "standalone: true" "web/src/app"

echo "=================================="
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}All architectural patterns are aligned with ai-spec.${NC}"
    exit 0
else
    echo -e "${RED}Alignment check failed. Please update code or ai-spec.${NC}"
    exit 1
fi
