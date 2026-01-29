# API Quick Reference Guide

## 1-Minute Quick Start

### Get API Key
```
http://localhost:8082/admin/apikeys → "Create API Key" button
⚠️ The generated key is shown only once - copy it immediately!
```

### Basic Usage
```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
     http://localhost:8082/api/admin/users
```

---

## API Endpoints Summary

| Action | Method | URL | Body |
|--------|--------|-----|------|
| List Users | GET | `/api/admin/users` | - |
| Get User | GET | `/api/admin/users/{pubkey}` | - |
| Create | POST | `/api/admin/users` | JSON |
| Update | PUT | `/api/admin/users/{pubkey}` | JSON |
| Delete | DELETE | `/api/admin/users/{pubkey}` | - |

---

## Request Examples

### Create (POST)
```bash
curl -X POST http://localhost:8082/api/admin/users \
  -H "Authorization: Bearer ak_..." \
  -H "Content-Type: application/json" \
  -d '{
    "pubkey": "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d",
    "preferred_language": "en",
    "is_admin": false,
    "active": true
  }'
```

### Update (PUT)
```bash
curl -X PUT http://localhost:8082/api/admin/users/3bf0c63... \
  -H "Authorization: Bearer ak_..." \
  -H "Content-Type: application/json" \
  -d '{
    "preferred_language": "es",
    "is_admin": true,
    "active": true
  }'
```

### Delete (DELETE)
```bash
curl -X DELETE http://localhost:8082/api/admin/users/3bf0c63... \
  -H "Authorization: Bearer ak_..."
```

---

## Code Snippets

### JavaScript
```javascript
const API_KEY = process.env.NOSTR_API_KEY;

fetch('http://localhost:8082/api/admin/users', {
  headers: { 'Authorization': `Bearer ${API_KEY}` }
})
.then(r => r.json())
.then(users => console.log(users));
```

### Python
```python
import requests

headers = {'Authorization': f'Bearer {API_KEY}'}
r = requests.get('http://localhost:8082/api/admin/users', headers=headers)
users = r.json()
```

### Go
```go
req.Header.Set("Authorization", "Bearer "+apiKey)
resp, err := http.DefaultClient.Do(req)
```

---

## Error Codes

| Code | Meaning | Solution |
|------|---------|----------|
| 401 | Missing/Invalid API Key | Check Authorization header |
| 400 | Bad Request | Check JSON format |
| 404 | User Not Found | Check pubkey |
| 409 | Already Exists | Use different pubkey |
| 500 | Server Error | Check logs |

---

## Security Checklist

- [ ] Store API Key in environment variables
- [ ] Never hardcode in source code
- [ ] Add .env to .gitignore
- [ ] Use HTTPS (production)
- [ ] Never expose to client-side

---

## Testing

```bash
# Connection test
curl http://localhost:8082/api/admin/users \
  -H "Authorization: Bearer YOUR_API_KEY"

# Run smoke test
./scripts/smoke_test.sh

# E2E test
go test ./web -run TestAPIE2E_RealDatabase -v
```

---

## Full Guide
See [API Key Full Guide](./api_key_guide.md)
