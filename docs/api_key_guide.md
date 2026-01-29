# API Key Issuance and Usage Guide

Guide for external applications to obtain and use API Keys for the User Management API.

## 1. How to Get an API Key

### Via Web Admin Page

1. **Admin Login**
   ```
   http://localhost:8082/admin/login
   ```

2. **Navigate to API Keys Page**
   ```
   http://localhost:8082/admin/apikeys
   ```

3. **Click "Create New API Key" button**
   - Name: Identifier for the API key (e.g., "mobile-app", "backend-service")
   - Description: Purpose description (optional)

4. **Copy the API Key shown after creation**
   ```
   ⚠️ Important: The API Key is shown only once at creation!
   Save it securely immediately.
   ```

### Programmatic Creation (Go)

```go
import (
    "context"
    "github.com/lescuer97/nostr-oicd/storage"
)

// If you have a Storage instance
ctx := context.Background()
apiKeyRecord, rawKey, err := storage.CreateAPIKey(ctx, "my-app", "admin-user")
if err != nil {
    log.Fatal(err)
}

// Pass rawKey to the external application
fmt.Println("API Key:", rawKey)
// e.g.: ak_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6
```

---

## 2. How to Use API Keys

### Add Authorization Header to HTTP Requests

All API requests must include the `Authorization` header:

```
Authorization: Bearer {YOUR_API_KEY}
```

### Example: cURL

```bash
# List users
curl -X GET http://localhost:8082/api/admin/users \
  -H "Authorization: Bearer ak_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6"

# Create user
curl -X POST http://localhost:8082/api/admin/users \
  -H "Authorization: Bearer ak_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6" \
  -H "Content-Type: application/json" \
  -d '{
    "pubkey": "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d",
    "preferred_language": "en",
    "is_admin": false,
    "active": true
  }'
```

### Example: JavaScript (fetch)

```javascript
const API_KEY = 'ak_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6';
const BASE_URL = 'http://localhost:8082/api/admin';

// List users
async function getUsers() {
  const response = await fetch(`${BASE_URL}/users`, {
    headers: {
      'Authorization': `Bearer ${API_KEY}`
    }
  });

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }

  return await response.json();
}

// Create user
async function createUser(userData) {
  const response = await fetch(`${BASE_URL}/users`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${API_KEY}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(userData)
  });

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }

  return await response.json();
}

// Usage
getUsers()
  .then(users => console.log('Users:', users))
  .catch(err => console.error('Error:', err));
```

### Example: Python (requests)

```python
import requests

API_KEY = 'ak_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6'
BASE_URL = 'http://localhost:8082/api/admin'

headers = {
    'Authorization': f'Bearer {API_KEY}',
    'Content-Type': 'application/json'
}

# List users
response = requests.get(f'{BASE_URL}/users', headers=headers)
users = response.json()
print('Users:', users)

# Create user
new_user = {
    'pubkey': '3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d',
    'preferred_language': 'en',
    'is_admin': False,
    'active': True
}

response = requests.post(f'{BASE_URL}/users', headers=headers, json=new_user)
created_user = response.json()
print('Created user:', created_user)
```

### Example: Go

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
)

const (
    APIKey  = "ak_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6"
    BaseURL = "http://localhost:8082/api/admin"
)

type User struct {
    Pubkey            string `json:"pubkey"`
    PreferredLanguage string `json:"preferred_language"`
    IsAdmin           bool   `json:"is_admin"`
    Active            bool   `json:"active"`
}

func main() {
    // List users
    users, err := getUsers()
    if err != nil {
        panic(err)
    }
    fmt.Printf("Users: %+v\n", users)

    // Create user
    newUser := User{
        Pubkey:            "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d",
        PreferredLanguage: "en",
        IsAdmin:           false,
        Active:            true,
    }

    created, err := createUser(newUser)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Created user: %+v\n", created)
}

func getUsers() ([]User, error) {
    req, err := http.NewRequest("GET", BaseURL+"/users", nil)
    if err != nil {
        return nil, err
    }

    req.Header.Set("Authorization", "Bearer "+APIKey)

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var users []User
    if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
        return nil, err
    }

    return users, nil
}

func createUser(user User) (*User, error) {
    body, err := json.Marshal(user)
    if err != nil {
        return nil, err
    }

    req, err := http.NewRequest("POST", BaseURL+"/users", bytes.NewReader(body))
    if err != nil {
        return nil, err
    }

    req.Header.Set("Authorization", "Bearer "+APIKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var created User
    if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
        return nil, err
    }

    return &created, nil
}
```

---

## 3. Available API Endpoints

### User Management

```bash
# List users
GET /api/admin/users

# Get specific user
GET /api/admin/users/{pubkey}

# Create user
POST /api/admin/users
Body: {
  "pubkey": "hex_public_key",
  "preferred_language": "en",
  "is_admin": false,
  "active": true
}

# Update user
PUT /api/admin/users/{pubkey}
Body: {
  "preferred_language": "es",
  "is_admin": true,
  "active": true
}

# Delete user
DELETE /api/admin/users/{pubkey}
```

### Response Format

#### Success Response
```json
{
  "id": "uuid",
  "pubkey": "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d",
  "preferred_language": "en",
  "is_admin": false,
  "active": true,
  "created_at": "2026-01-29T12:00:00Z",
  "updated_at": "2026-01-29T12:00:00Z"
}
```

#### Error Response
```json
{
  "error": "error_code",
  "message": "Human readable error message"
}
```

**Error Codes:**
- `unauthorized` (401) - API Key missing or invalid
- `invalid_pubkey` (400) - Invalid public key format
- `user_exists` (409) - User already exists
- `user_not_found` (404) - User not found
- `internal_error` (500) - Internal server error

---

## 4. Security Considerations

### DO (Recommended)

1. **Store API Key in environment variables**
   ```bash
   export NOSTR_API_KEY="ak_..."
   ```

2. **Use HTTPS** (production environment)
   ```
   https://api.example.com/api/admin/users
   ```

3. **Rotate API Keys regularly**
   - Periodically issue new API Keys and delete old ones

4. **Principle of least privilege**
   - Issue separate API Keys per application
   - Grant only necessary permissions

5. **Monitor logs**
   - Monitor API Key usage
   - Detect abnormal patterns

### DON'T (Prohibited)

1. **Never hardcode in source**
   ```javascript
   // ❌ BAD
   const API_KEY = 'ak_a1b2c3d4...';  // Directly in source code

   // ✅ GOOD
   const API_KEY = process.env.NOSTR_API_KEY;
   ```

2. **Never commit to Git**
   ```bash
   # Add to .gitignore
   .env
   config/api_keys.json
   ```

3. **Never use on client-side**
   ```html
   <!-- ❌ BAD - Direct API call from browser -->
   <script>
     const apiKey = 'ak_...';  // Exposed to client!
   </script>
   ```

4. **Never upload to public repositories**
   - Never include API Keys in GitHub, GitLab, etc.

---

## 5. Troubleshooting

### 401 Unauthorized

```bash
# Cause: API Key missing or invalid
# Solution: Check Authorization header

# Correct format
curl -H "Authorization: Bearer ak_..."

# Incorrect formats
curl -H "Authorization: ak_..."           # ❌ Missing Bearer
curl -H "Authorization: Bearer Bearer ak_..."  # ❌ Duplicate Bearer
```

### 400 Bad Request

```bash
# Cause: Invalid request data format
# Solution: Check JSON format and required fields

# Correct request
{
  "pubkey": "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d",
  "preferred_language": "en"
}

# Invalid pubkey (too short)
{
  "pubkey": "3bf0c63"  # ❌ Requires 64-character hex
}
```

### Connection Test

```bash
# 1. Request without API Key (expect 401 error)
curl -X GET http://localhost:8082/api/admin/users

# 2. Request with API Key (should work)
curl -X GET http://localhost:8082/api/admin/users \
  -H "Authorization: Bearer YOUR_API_KEY"
```

---

## 6. Quick Start Example

### Set Environment Variables

```bash
# Create .env file
cat > .env << EOF
NOSTR_API_KEY=ak_your_api_key_here
NOSTR_BASE_URL=http://localhost:8082/api/admin
EOF

# Load environment variables
export $(cat .env | xargs)
```

### Integration Example Script

```bash
#!/bin/bash
# test_api.sh

API_KEY="${NOSTR_API_KEY}"
BASE_URL="${NOSTR_BASE_URL:-http://localhost:8082/api/admin}"

# 1. List users
echo "=== List Users ==="
curl -s -X GET "${BASE_URL}/users" \
  -H "Authorization: Bearer ${API_KEY}" | jq

# 2. Create user
echo -e "\n=== Create User ==="
curl -s -X POST "${BASE_URL}/users" \
  -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "pubkey": "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d",
    "preferred_language": "en",
    "is_admin": false,
    "active": true
  }' | jq

# 3. Get specific user
PUBKEY="3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d"
echo -e "\n=== Get User ==="
curl -s -X GET "${BASE_URL}/users/${PUBKEY}" \
  -H "Authorization: Bearer ${API_KEY}" | jq
```

---

## Additional Resources

- **API Details**: [docs/api_integration.md](./api_integration.md)
- **Full Error Codes**: [docs/error_codes.md](./error_codes.md)
- **E2E Test Examples**: [web/api_e2e_test.go](../web/api_e2e_test.go)

## Support

For issues or questions:
- GitHub Issues: https://github.com/lescuer97/nostr-oicd/issues
- Run test script: `scripts/smoke_test.sh`
