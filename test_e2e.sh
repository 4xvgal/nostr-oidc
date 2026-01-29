#!/bin/bash

# E2E Test Script for API Key & User Management API
# This script tests the complete API flow with a real server instance

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
SERVER_PORT=${SERVER_PORT:-8082}
BASE_URL="http://localhost:${SERVER_PORT}"
API_BASE="${BASE_URL}/api/admin"
TEST_DB="test_database.db"
API_KEY=""

# Test data
TEST_USER_PUBKEY="3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"
TEST_USER_PUBKEY_2="82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2"

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"
    if [ ! -z "$SERVER_PID" ]; then
        echo "Killing server (PID: $SERVER_PID)..."
        kill $SERVER_PID 2>/dev/null || true
        wait $SERVER_PID 2>/dev/null || true
    fi
    if [ -f "$TEST_DB" ]; then
        echo "Removing test database..."
        rm -f "$TEST_DB"
    fi
}

trap cleanup EXIT

# Helper functions
print_test() {
    echo -e "${YELLOW}[TEST]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

print_error() {
    echo -e "${RED}[✗]${NC} $1"
    exit 1
}

# Wait for server to be ready
wait_for_server() {
    echo "Waiting for server to be ready..."
    for i in {1..30}; do
        if curl -s "${BASE_URL}/.well-known/openid-configuration" > /dev/null 2>&1; then
            print_success "Server is ready!"
            return 0
        fi
        echo -n "."
        sleep 1
    done
    print_error "Server failed to start within 30 seconds"
}

# Create API Key directly in database (since UI is not implemented)
create_api_key() {
    print_test "Creating API Key in database..."

    # Generate API key using Go code
    API_KEY=$(go run -tags=test << 'EOF'
package main

import (
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "time"
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
)

func main() {
    // Generate API key
    randomBytes := make([]byte, 24)
    rand.Read(randomBytes)
    apiKey := "ak_" + hex.EncodeToString(randomBytes)

    // Hash for storage
    hash := sha256.Sum256([]byte(apiKey))
    keyHash := hex.EncodeToString(hash[:])
    keyPrefix := apiKey[:10]

    // Open database
    db, err := sql.Open("sqlite3", "test_database.db")
    if err != nil {
        panic(err)
    }
    defer db.Close()

    // Insert API key
    _, err = db.Exec(`
        INSERT INTO api_keys (id, label, key_prefix, key_hash, created_at, created_by, is_active)
        VALUES (?, ?, ?, ?, ?, ?, ?)`,
        "test-api-key-id",
        "E2E Test Key",
        keyPrefix,
        keyHash,
        time.Now(),
        "e2e-test",
        true,
    )
    if err != nil {
        panic(err)
    }

    // Print the raw key (only time it's shown)
    fmt.Print(apiKey)
}
EOF
    )

    if [ -z "$API_KEY" ]; then
        print_error "Failed to create API key"
    fi

    print_success "API Key created: ${API_KEY:0:20}..."
}

# Simplified API Key creation using sqlite3 command
create_api_key_simple() {
    print_test "Creating API Key in database..."

    # Generate a simple API key
    API_KEY="ak_$(openssl rand -hex 24)"
    KEY_HASH=$(echo -n "$API_KEY" | openssl dgst -sha256 | cut -d' ' -f2)
    KEY_PREFIX="${API_KEY:0:10}"
    TIMESTAMP=$(date -u +"%Y-%m-%d %H:%M:%S")

    # Insert into database using sqlite3
    sqlite3 "$TEST_DB" << EOF
INSERT INTO api_keys (id, label, key_prefix, key_hash, created_at, created_by, is_active)
VALUES ('test-api-key-id', 'E2E Test Key', '$KEY_PREFIX', '$KEY_HASH', '$TIMESTAMP', 'e2e-test', 1);
EOF

    print_success "API Key created: ${API_KEY:0:20}..."
}

# API Test Functions
test_unauthorized_access() {
    print_test "Testing unauthorized access (no API key)..."

    response=$(curl -s -w "\n%{http_code}" "${API_BASE}/users")
    http_code=$(echo "$response" | tail -n1)

    if [ "$http_code" = "401" ]; then
        print_success "Correctly rejected unauthorized request"
    else
        print_error "Expected 401, got $http_code"
    fi
}

test_invalid_api_key() {
    print_test "Testing with invalid API key..."

    response=$(curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer invalid_key_12345" \
        "${API_BASE}/users")
    http_code=$(echo "$response" | tail -n1)

    if [ "$http_code" = "401" ]; then
        print_success "Correctly rejected invalid API key"
    else
        print_error "Expected 401, got $http_code"
    fi
}

test_list_users_empty() {
    print_test "Testing list users (should be empty)..."

    response=$(curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer $API_KEY" \
        "${API_BASE}/users")
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    if [ "$http_code" = "200" ]; then
        # Check if response is empty array
        if echo "$body" | grep -q "\[\]"; then
            print_success "User list is empty as expected"
        else
            print_error "Expected empty array, got: $body"
        fi
    else
        print_error "Expected 200, got $http_code"
    fi
}

test_create_user() {
    print_test "Creating a new user..."

    response=$(curl -s -w "\n%{http_code}" \
        -X POST \
        -H "Authorization: Bearer $API_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"pubkey\": \"$TEST_USER_PUBKEY\",
            \"preferred_language\": \"en\",
            \"is_admin\": false,
            \"active\": true
        }" \
        "${API_BASE}/users")
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    if [ "$http_code" = "201" ]; then
        # Verify response contains the pubkey
        if echo "$body" | grep -q "$TEST_USER_PUBKEY"; then
            print_success "User created successfully"
            echo "Response: $body"
        else
            print_error "Response doesn't contain expected pubkey: $body"
        fi
    else
        print_error "Expected 201, got $http_code. Body: $body"
    fi
}

test_create_duplicate_user() {
    print_test "Testing duplicate user creation (should fail)..."

    response=$(curl -s -w "\n%{http_code}" \
        -X POST \
        -H "Authorization: Bearer $API_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"pubkey\": \"$TEST_USER_PUBKEY\",
            \"preferred_language\": \"en\",
            \"is_admin\": false,
            \"active\": true
        }" \
        "${API_BASE}/users")
    http_code=$(echo "$response" | tail -n1)

    if [ "$http_code" = "409" ]; then
        print_success "Correctly rejected duplicate user"
    else
        print_error "Expected 409 (Conflict), got $http_code"
    fi
}

test_create_invalid_user() {
    print_test "Testing user creation with invalid pubkey..."

    response=$(curl -s -w "\n%{http_code}" \
        -X POST \
        -H "Authorization: Bearer $API_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"pubkey\": \"invalid_pubkey\",
            \"preferred_language\": \"en\",
            \"is_admin\": false,
            \"active\": true
        }" \
        "${API_BASE}/users")
    http_code=$(echo "$response" | tail -n1)

    if [ "$http_code" = "400" ]; then
        print_success "Correctly rejected invalid pubkey"
    else
        print_error "Expected 400, got $http_code"
    fi
}

test_get_user() {
    print_test "Getting user by pubkey..."

    response=$(curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer $API_KEY" \
        "${API_BASE}/users/${TEST_USER_PUBKEY}")
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    if [ "$http_code" = "200" ]; then
        if echo "$body" | grep -q "$TEST_USER_PUBKEY"; then
            print_success "User retrieved successfully"
            echo "Response: $body"
        else
            print_error "Response doesn't match expected pubkey: $body"
        fi
    else
        print_error "Expected 200, got $http_code. Body: $body"
    fi
}

test_get_nonexistent_user() {
    print_test "Testing get nonexistent user..."

    response=$(curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer $API_KEY" \
        "${API_BASE}/users/${TEST_USER_PUBKEY_2}")
    http_code=$(echo "$response" | tail -n1)

    if [ "$http_code" = "404" ]; then
        print_success "Correctly returned 404 for nonexistent user"
    else
        print_error "Expected 404, got $http_code"
    fi
}

test_list_users() {
    print_test "Testing list users (should have 1 user)..."

    response=$(curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer $API_KEY" \
        "${API_BASE}/users")
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    if [ "$http_code" = "200" ]; then
        if echo "$body" | grep -q "$TEST_USER_PUBKEY"; then
            print_success "User list contains created user"
            echo "Response: $body"
        else
            print_error "User not found in list: $body"
        fi
    else
        print_error "Expected 200, got $http_code"
    fi
}

test_update_user() {
    print_test "Updating user..."

    response=$(curl -s -w "\n%{http_code}" \
        -X PUT \
        -H "Authorization: Bearer $API_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"preferred_language\": \"ko\",
            \"is_admin\": false,
            \"active\": false
        }" \
        "${API_BASE}/users/${TEST_USER_PUBKEY}")
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    if [ "$http_code" = "200" ]; then
        # Check if active is now false and language is ko
        if echo "$body" | grep -q "\"active\":false" && echo "$body" | grep -q "\"preferred_language\":\"ko\""; then
            print_success "User updated successfully"
            echo "Response: $body"
        else
            print_error "User not updated correctly: $body"
        fi
    else
        print_error "Expected 200, got $http_code. Body: $body"
    fi
}

test_delete_user() {
    print_test "Deleting user..."

    response=$(curl -s -w "\n%{http_code}" \
        -X DELETE \
        -H "Authorization: Bearer $API_KEY" \
        "${API_BASE}/users/${TEST_USER_PUBKEY}")
    http_code=$(echo "$response" | tail -n1)

    if [ "$http_code" = "204" ]; then
        print_success "User deleted successfully"
    else
        print_error "Expected 204, got $http_code"
    fi
}

test_verify_deletion() {
    print_test "Verifying user deletion..."

    response=$(curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer $API_KEY" \
        "${API_BASE}/users/${TEST_USER_PUBKEY}")
    http_code=$(echo "$response" | tail -n1)

    if [ "$http_code" = "404" ]; then
        print_success "User successfully deleted (404 returned)"
    else
        print_error "Expected 404 after deletion, got $http_code"
    fi
}

# Main test execution
main() {
    echo ""
    echo "=========================================="
    echo "  API E2E Test Suite"
    echo "=========================================="
    echo ""

    # Check dependencies
    print_test "Checking dependencies..."
    command -v curl >/dev/null 2>&1 || print_error "curl is required"
    command -v sqlite3 >/dev/null 2>&1 || print_error "sqlite3 is required"
    command -v openssl >/dev/null 2>&1 || print_error "openssl is required"
    print_success "All dependencies found"

    # Cleanup any existing test database
    rm -f "$TEST_DB"

    # Run migrations
    print_test "Running database migrations..."
    if ! command -v goose >/dev/null 2>&1; then
        print_error "goose is required for migrations. Install: go install github.com/pressly/goose/v3/cmd/goose@latest"
    fi
    goose -dir storage/database/migrations sqlite3 "$TEST_DB" up
    print_success "Migrations completed"

    # Start server in background
    print_test "Starting server..."
    DATABASE_PATH="$TEST_DB" go run main.go > server.log 2>&1 &
    SERVER_PID=$!
    echo "Server PID: $SERVER_PID"

    # Wait for server
    wait_for_server

    # Create API Key
    create_api_key_simple

    echo ""
    echo "=========================================="
    echo "  Running Tests"
    echo "=========================================="
    echo ""

    # Run tests in sequence
    test_unauthorized_access
    test_invalid_api_key
    test_list_users_empty
    test_create_user
    test_create_duplicate_user
    test_create_invalid_user
    test_get_user
    test_get_nonexistent_user
    test_list_users
    test_update_user
    test_delete_user
    test_verify_deletion

    echo ""
    echo "=========================================="
    echo -e "  ${GREEN}All Tests Passed!${NC}"
    echo "=========================================="
    echo ""

    # Show server logs if there were errors
    if grep -i error server.log > /dev/null 2>&1; then
        echo "Server log (errors found):"
        grep -i error server.log | tail -20
    fi
}

# Run main function
main
