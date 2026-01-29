#!/bin/bash

# Smoke Test Script for Frontend
# Tests basic API endpoints are responding

set -e

echo "╔════════════════════════════════════════════╗"
echo "║     Frontend Smoke Test                    ║"
echo "╚════════════════════════════════════════════╝"
echo ""

# Configuration
BASE_URL="${BASE_URL:-http://localhost:8082}"
API_KEY="${API_KEY:-}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counter
PASSED=0
FAILED=0

# Function to test endpoint
test_endpoint() {
    local method=$1
    local path=$2
    local expected_status=$3
    local description=$4
    local auth_header=$5

    echo -n "Testing: $description ... "

    if [ -n "$auth_header" ]; then
        status=$(curl -s -o /dev/null -w "%{http_code}" -X "$method" \
            -H "Authorization: Bearer $auth_header" \
            "${BASE_URL}${path}")
    else
        status=$(curl -s -o /dev/null -w "%{http_code}" -X "$method" \
            "${BASE_URL}${path}")
    fi

    if [ "$status" -eq "$expected_status" ]; then
        echo -e "${GREEN}✓ PASS${NC} (HTTP $status)"
        ((PASSED++))
    else
        echo -e "${RED}✗ FAIL${NC} (Expected $expected_status, got $status)"
        ((FAILED++))
    fi
}

echo "1. Testing Static Assets"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
test_endpoint "GET" "/static/js/modal.js" 200 "Modal JavaScript"
test_endpoint "GET" "/static/css/loading.css" 200 "Loading CSS"
echo ""

echo "2. Testing Admin Routes"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
test_endpoint "GET" "/admin/login" 200 "Admin Login Page"
test_endpoint "GET" "/admin/" 200 "Admin Dashboard"
echo ""

echo "3. Testing API Endpoints (without auth)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
test_endpoint "GET" "/api/admin/users" 401 "List Users (should require auth)"
test_endpoint "POST" "/api/admin/users" 401 "Create User (should require auth)"
echo ""

if [ -n "$API_KEY" ]; then
    echo "4. Testing API Endpoints (with auth)"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    test_endpoint "GET" "/api/admin/users" 200 "List Users (authenticated)" "$API_KEY"
    echo ""
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Summary:"
echo -e "  ${GREEN}Passed: $PASSED${NC}"
echo -e "  ${RED}Failed: $FAILED${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ $FAILED -gt 0 ]; then
    echo -e "${RED}Some tests failed!${NC}"
    exit 1
else
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
fi
