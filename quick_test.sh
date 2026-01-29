#!/bin/bash

# Quick API Test Script
# Assumes server is already running on localhost:8082

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

BASE_URL="${BASE_URL:-http://localhost:8082}"
API_BASE="${BASE_URL}/api/admin"
DB_PATH="${DATABASE_PATH:-database.db}"

print_header() {
    echo ""
    echo -e "${YELLOW}=========================================="
    echo "  $1"
    echo -e "==========================================${NC}"
}

print_test() {
    echo -e "${YELLOW}▶${NC} $1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

# Check if server is running
check_server() {
    if ! curl -s "${BASE_URL}/.well-known/openid-configuration" > /dev/null 2>&1; then
        print_error "Server is not running on ${BASE_URL}"
        echo "Start the server first: go run main.go"
        exit 1
    fi
    print_success "Server is running"
}

# Create a test API key
create_test_api_key() {
    if [ ! -f "$DB_PATH" ]; then
        print_error "Database not found at: $DB_PATH"
        exit 1
    fi

    print_test "Creating test API key..."

    API_KEY="ak_$(openssl rand -hex 24)"
    KEY_HASH=$(echo -n "$API_KEY" | openssl dgst -sha256 | cut -d' ' -f2)
    KEY_PREFIX="${API_KEY:0:10}"
    TIMESTAMP=$(date -u +"%Y-%m-%d %H:%M:%S")

    sqlite3 "$DB_PATH" << EOF
DELETE FROM api_keys WHERE label = 'Quick Test Key';
INSERT INTO api_keys (id, label, key_prefix, key_hash, created_at, created_by, is_active)
VALUES ('quick-test-key', 'Quick Test Key', '$KEY_PREFIX', '$KEY_HASH', '$TIMESTAMP', 'quick-test', 1);
EOF

    print_success "API Key: $API_KEY"
    echo ""
}

# Test endpoints
test_list_users() {
    print_test "GET /api/admin/users"
    response=$(curl -s -H "Authorization: Bearer $API_KEY" "${API_BASE}/users")
    echo "$response" | jq '.' 2>/dev/null || echo "$response"
    echo ""
}

test_create_user() {
    local pubkey="$1"
    print_test "POST /api/admin/users (pubkey: ${pubkey:0:16}...)"

    response=$(curl -s -w "\n%{http_code}" \
        -X POST \
        -H "Authorization: Bearer $API_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"pubkey\": \"$pubkey\",
            \"preferred_language\": \"en\",
            \"is_admin\": false,
            \"active\": true
        }" \
        "${API_BASE}/users")

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    echo "Status: $http_code"
    echo "$body" | jq '.' 2>/dev/null || echo "$body"
    echo ""
}

test_get_user() {
    local pubkey="$1"
    print_test "GET /api/admin/users/$pubkey"

    response=$(curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer $API_KEY" \
        "${API_BASE}/users/${pubkey}")

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    echo "Status: $http_code"
    echo "$body" | jq '.' 2>/dev/null || echo "$body"
    echo ""
}

test_update_user() {
    local pubkey="$1"
    print_test "PUT /api/admin/users/$pubkey"

    response=$(curl -s -w "\n%{http_code}" \
        -X PUT \
        -H "Authorization: Bearer $API_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"preferred_language\": \"ko\",
            \"is_admin\": false,
            \"active\": false
        }" \
        "${API_BASE}/users/${pubkey}")

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    echo "Status: $http_code"
    echo "$body" | jq '.' 2>/dev/null || echo "$body"
    echo ""
}

test_delete_user() {
    local pubkey="$1"
    print_test "DELETE /api/admin/users/$pubkey"

    http_code=$(curl -s -w "%{http_code}" -o /dev/null \
        -X DELETE \
        -H "Authorization: Bearer $API_KEY" \
        "${API_BASE}/users/${pubkey}")

    echo "Status: $http_code"
    echo ""
}

# Interactive mode
interactive_mode() {
    while true; do
        echo ""
        echo "Quick Test Menu:"
        echo "1) List all users"
        echo "2) Create user"
        echo "3) Get user"
        echo "4) Update user"
        echo "5) Delete user"
        echo "6) Create new API key"
        echo "7) Run full test sequence"
        echo "q) Quit"
        echo ""
        read -p "Choose option: " choice

        case $choice in
            1)
                test_list_users
                ;;
            2)
                read -p "Enter pubkey (32-byte hex): " pubkey
                test_create_user "$pubkey"
                ;;
            3)
                read -p "Enter pubkey: " pubkey
                test_get_user "$pubkey"
                ;;
            4)
                read -p "Enter pubkey: " pubkey
                test_update_user "$pubkey"
                ;;
            5)
                read -p "Enter pubkey: " pubkey
                test_delete_user "$pubkey"
                ;;
            6)
                create_test_api_key
                ;;
            7)
                run_full_test
                ;;
            q|Q)
                echo "Bye!"
                exit 0
                ;;
            *)
                echo "Invalid option"
                ;;
        esac
    done
}

# Full test sequence
run_full_test() {
    local test_pubkey="3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"

    print_header "Full Test Sequence"

    test_list_users
    test_create_user "$test_pubkey"
    test_get_user "$test_pubkey"
    test_list_users
    test_update_user "$test_pubkey"
    test_get_user "$test_pubkey"
    test_delete_user "$test_pubkey"
    test_list_users

    print_success "Test sequence completed!"
}

# Main
main() {
    print_header "Quick API Test"

    check_server

    # Check for existing API key
    if [ -f "$DB_PATH" ]; then
        existing_key=$(sqlite3 "$DB_PATH" "SELECT key_prefix FROM api_keys WHERE label = 'Quick Test Key' LIMIT 1" 2>/dev/null || echo "")
        if [ ! -z "$existing_key" ]; then
            print_success "Using existing Quick Test Key (prefix: $existing_key)"
            echo ""
            echo -e "${YELLOW}Note: The full API key was created previously and is not stored.${NC}"
            echo -e "${YELLOW}Creating a new API key...${NC}"
            echo ""
        fi
    fi

    create_test_api_key

    # Check if script was called with arguments
    if [ "$1" = "interactive" ] || [ "$1" = "-i" ]; then
        interactive_mode
    elif [ "$1" = "full" ] || [ "$1" = "-f" ]; then
        run_full_test
    else
        echo "Usage:"
        echo "  $0              - Create API key and show usage"
        echo "  $0 interactive  - Interactive mode"
        echo "  $0 full         - Run full test sequence"
        echo ""
        echo "Environment variables:"
        echo "  BASE_URL       - Base URL (default: http://localhost:8082)"
        echo "  DATABASE_PATH  - Database path (default: database.db)"
        echo ""
        echo "Manual test commands:"
        echo ""
        echo "  # List users"
        echo "  curl -H \"Authorization: Bearer $API_KEY\" ${API_BASE}/users | jq"
        echo ""
        echo "  # Create user"
        echo "  curl -X POST -H \"Authorization: Bearer $API_KEY\" \\"
        echo "    -H \"Content-Type: application/json\" \\"
        echo "    -d '{\"pubkey\":\"3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d\",\"preferred_language\":\"en\",\"is_admin\":false,\"active\":true}' \\"
        echo "    ${API_BASE}/users | jq"
        echo ""
    fi
}

main "$@"
