# API Key 기반 사용자 관리 시스템 구현 계획

## 목표
- API Key 발급 및 관리 시스템 구축
- Bearer 인증을 통한 RESTful API 제공
- 사용자 CRUD 기능 (Nostr 32-byte hex 공개키 형식 지원)

## 아키텍처 개요

```
/admin/apikeys          → API Key 관리 UI (HTMX + JWT 인증)
/api/admin/users        → 사용자 관리 REST API (API Key 인증)
```

### 계층 구조
1. **Database Layer**: api_keys 테이블 (migration 002)
2. **Storage Layer**: API Key CRUD, 사용자 관리 함수
3. **Web Layer**: API 미들웨어, REST 핸들러, HTMX UI
4. **Routing**: /api/admin/* 라우트 마운트

## 구현 단계

### Phase 1: 데이터베이스 및 Storage Layer

#### 1.1 Migration 파일 생성
**파일**: `storage/database/migrations/002_api_keys.sql`

```sql
-- +goose Up
CREATE TABLE api_keys (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL,
    last_used_at DATETIME,
    created_by TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT 1
);

CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX idx_api_keys_created_at ON api_keys(created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_api_keys_created_at;
DROP INDEX IF EXISTS idx_api_keys_key_hash;
DROP TABLE IF EXISTS api_keys;
```

**API Key 형식**:
- 원본 키: `ak_` + 48자 hex (24 바이트 random)
- 저장: SHA-256 해시만 DB에 저장
- 접두어: 처음 10자 (예: `ak_9f3b2c1`)

#### 1.2 API Key 모델 및 유틸리티
**파일**: `storage/apikeys.go` (새로 생성)

핵심 함수:
- `GenerateAPIKey() (string, error)` - 24바이트 랜덤 생성 → `ak_` + hex
- `HashAPIKey(key string) string` - SHA-256 해싱
- `ExtractKeyPrefix(key string) string` - 처음 10자 추출
- `APIKey struct` - ID, Label, KeyPrefix, KeyHash, CreatedAt, LastUsedAt, CreatedBy, IsActive
- `ScanRow()` 메서드 구현

#### 1.3 Storage DB 메서드
**파일**: `storage/db.go` (기존 파일에 추가)

함수 추가:
- `CreateAPIKey(tx, *APIKey) error`
- `SearchAPIKeyByHash(tx, keyHash string) (*APIKey, error)`
- `UpdateAPIKeyLastUsed(tx, keyHash string) error`
- `GetAllAPIKeys(tx) ([]APIKey, error)`
- `DeleteAPIKey(tx, id string) error`

#### 1.4 Storage 인터페이스 메서드
**파일**: `storage/storage.go` (기존 파일에 추가)

함수 추가:
- `CreateAPIKey(ctx, label, createdBy string) (*APIKey, string, error)` - 키 생성, 원본 키 반환
- `ValidateAPIKey(ctx, rawKey string) (*APIKey, error)` - 인증 + last_used_at 업데이트
- `GetAllAPIKeys(ctx) ([]APIKey, error)` - 목록 조회
- `DeleteAPIKey(ctx, id string) error` - 삭제
- `DeleteUser(ctx, id string) error` - 사용자 삭제 (아직 없음, 추가 필요)

**참고**: 기존 `storage/db.go:579`에 `DeleteUser` DB 메서드는 있지만, `storage.go`에 트랜잭션 래퍼가 없음

---

### Phase 2: API 인증 및 REST 엔드포인트

#### 2.1 API Key 인증 미들웨어
**파일**: `web/api_middleware.go` (새로 생성)

```go
func (s *Server) APIKeyMiddleware(next http.Handler) http.Handler
```

동작:
1. Authorization 헤더에서 Bearer 토큰 추출
2. `Storage.ValidateAPIKey()`로 검증
3. 성공 시 context에 APIKey 저장
4. 실패 시 JSON 에러 응답 (401 Unauthorized)

**기존 AuthMiddleware와의 차이점**:
- JWT ID Token vs API Key
- Redirect vs JSON 에러
- 웹 UI vs API 접근

#### 2.2 REST API 핸들러
**파일**: `web/api_admin.go` (새로 생성)

**라우트 설정**:
```go
func NewAPIAdminHandler(server *Server) chi.Router {
    router := chi.NewRouter()
    router.Use(server.APIKeyMiddleware)

    router.Get("/users", handleListUsers(server))
    router.Post("/users", handleCreateUser(server))
    router.Get("/users/{pubkey}", handleGetUser(server))
    router.Put("/users/{pubkey}", handleUpdateUser(server))
    router.Delete("/users/{pubkey}", handleDeleteUser(server))

    return router
}
```

**핵심 로직: 공개키 형식 변환**

Nostr 공개키는 **32-byte hex** (64자) 형식입니다.
현재 시스템은 33-byte 압축 공개키를 사용하므로 변환 필요:

```go
// API 입력 (32-byte hex) → Storage (33-byte compressed)
func convertHexToPubKey(hexPubkey string) (*btcec.PublicKey, error) {
    // 1. 길이 검증 (64자)
    // 2. hex.DecodeString() → 32 bytes
    // 3. schnorr.ParsePubKey() → btcec.PublicKey
}

// Storage (33-byte compressed) → API 응답 (32-byte hex)
func convertPubKeyToHex(pubkey *btcec.PublicKey) string {
    compressed := pubkey.SerializeCompressed()  // [prefix][32-byte-x]
    return hex.EncodeToString(compressed[1:])   // prefix 제외
}
```

**참고**: `storage/storage.go:700-705`에 동일한 로직이 있음 (PreferredUsername 클레임)

**DTOs**:
- `UserRequest`: pubkey (32-byte hex), preferred_language, is_admin, active
- `UserResponse`: id, pubkey (32-byte hex), preferred_language, is_admin, active
- `ErrorResponse`: error, message

**핸들러 함수**:
- `handleListUsers` - `GetAllUsers()` + JSON 응답
- `handleCreateUser` - hex→pubkey 변환, 중복 체크, `AddUser()`
- `handleGetUser` - `CheckUserNpub()` + JSON 응답
- `handleUpdateUser` - 기존 사용자 조회 + `EditUser()`
- `handleDeleteUser` - `CheckUserNpub()` + `DeleteUser()`

**에러 응답**:
- 400: 잘못된 요청 (형식 오류, 누락된 필드)
- 401: 인증 실패
- 404: 사용자 없음
- 409: 중복 (이미 존재)
- 500: 서버 오류

#### 2.3 라우팅 통합
**파일**: `web/server.go` (기존 파일 수정)

`SetupServer()` 함수에 추가:
```go
// 기존: adminRouter := NewAdminHandler(server)
// 기존: router.Mount("/admin", adminRouter)

apiAdminRouter := NewAPIAdminHandler(server)
router.Mount("/api/admin", apiAdminRouter)
```

---

### Phase 3: 관리자 UI (API Key 관리)

#### 3.1 템플릿 파일
**파일**: `web/templates/admin_apikeys.templ` (새로 생성)

**컴포넌트**:
1. `AdminAPIKeysPage()` - 메인 페이지
   - DashboardHeader 포함
   - "Create New API Key" 버튼
   - `hx-get="/admin/apikeys/list"` 목록 로드

2. `APIKeyList(keys []APIKeyListItem)` - 목록 테이블
   - Label, Key Prefix, Created, Last Used, Actions
   - Delete 버튼: `hx-delete="/admin/apikeys/{id}"`

3. `APIKeyCreateForm()` - 생성 폼 모달
   - Label 입력
   - `hx-post="/admin/apikeys"`

4. `APIKeyCreatedModal(response)` - 성공 모달
   - **중요**: 원본 키 1회만 표시
   - 복사 버튼
   - 노란색 경고: "This is the only time you'll see the full API key"

**데이터 구조**:
```go
type APIKeyListItem struct {
    ID         string
    Label      string
    KeyPrefix  string
    CreatedAt  time.Time
    LastUsedAt *time.Time
    CreatedBy  string
    IsActive   bool
}

type APIKeyCreatedResponse struct {
    RawKey  string  // 원본 키 (1회만)
    KeyInfo APIKeyListItem
}
```

#### 3.2 HTMX 핸들러
**파일**: `web/admin.go` (기존 파일에 추가)

`NewAdminHandler()` 라우트 추가:
```go
router.Group(func(r chi.Router) {
    r.Use(s.AuthMiddleware)  // JWT 인증

    // 기존 라우트들...

    r.Get("/apikeys", s.apiKeysPage)
    r.Get("/apikeys/list", s.apiKeysList)
    r.Get("/apikeys/create", s.apiKeysCreateForm)
    r.Post("/apikeys", s.apiKeysCreate)
    r.Delete("/apikeys/{id}", s.apiKeysDelete)
})
```

**핸들러 함수**:
- `apiKeysPage` - `AdminAPIKeysPage()` 렌더링
- `apiKeysList` - `GetAllAPIKeys()` + `APIKeyList()` 렌더링
- `apiKeysCreateForm` - `APIKeyCreateForm()` 렌더링
- `apiKeysCreate` - `CreateAPIKey()` + `APIKeyCreatedModal()` 렌더링 (원본 키 포함)
- `apiKeysDelete` - `DeleteAPIKey()` + 204 응답

**주의사항**:
- `apiKeysCreate`에서 current user ID 가져오기:
  ```go
  user := r.Context().Value(userContextKey).(*oidc.IDTokenClaims)
  createdBy := user.Subject
  ```

#### 3.3 대시보드 통합
**파일**: `web/templates/dashboard_header.templ` (기존 파일 수정)

네비게이션 링크 추가:
```html
<a href="/admin/apikeys">API Keys</a>
```

---

## 핵심 파일 목록

### 새로 생성
- `storage/database/migrations/002_api_keys.sql`
- `storage/apikeys.go`
- `web/api_middleware.go`
- `web/api_admin.go`
- `web/templates/admin_apikeys.templ`

### 수정 필요
- `storage/db.go` - API Key DB 메서드 추가
- `storage/storage.go` - API Key 인터페이스 메서드 + DeleteUser 추가
- `web/server.go` - `/api/admin` 라우트 마운트
- `web/admin.go` - API Key 관리 핸들러 추가
- `web/templates/dashboard_header.templ` - 네비게이션 링크 추가

---

## 보안 고려사항

1. **API Key 생성**
   - `crypto/rand.Read()` 사용 (24 bytes = 192 bits 엔트로피)
   - SHA-256 해싱 (Go `crypto/sha256`)

2. **저장**
   - 해시만 DB 저장
   - 원본 키는 생성 시 1회만 노출 후 폐기

3. **검증**
   - 해시 비교 (Go의 `==`는 상수 시간 아님, 추후 `subtle.ConstantTimeCompare` 고려)
   - `last_used_at` 업데이트 실패해도 인증 성공 처리

4. **프로덕션 권장사항**
   - HTTPS 필수
   - Rate limiting (향후 추가)
   - API Key 만료 기능 (향후 추가)
   - 감사 로그 (향후 추가)

---

## 공개키 형식 처리 (중요)

### Nostr 표준
- **공개키**: 32-bytes lowercase hex-encoded (64자)
- **예**: `3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d`

### 현재 시스템
- **DB 저장**: 33-byte 압축 공개키 (BLOB)
  - 첫 바이트: 02 또는 03 (Y 좌표 짝수/홀수)
  - 나머지 32 바이트: X 좌표
- **OIDC 클레임**: 32-byte hex (첫 바이트 제외)

### API 변환 로직
```
API 입력 (32-byte hex)
  → hex.DecodeString() → 32 bytes
  → schnorr.ParsePubKey() → btcec.PublicKey (33-byte compressed)
  → DB 저장

DB 조회
  → btcec.PublicKey (33-byte compressed)
  → SerializeCompressed()[1:] → 32 bytes
  → hex.EncodeToString() → 64자 hex
  → API 응답
```

---

## 테스트 전략

### 단위 테스트
- API Key 생성/해싱/검증
- 공개키 변환 함수
- Storage 메서드

### 통합 테스트
- REST API 엔드포인트 (각 CRUD)
- 인증 실패 시나리오
- 중복 사용자 생성

### E2E 테스트
1. 관리자 로그인 → API Key 생성 → 원본 키 복사
2. API Key로 사용자 목록 조회
3. 사용자 생성 (32-byte hex) → 저장 → 조회 (32-byte hex 반환 확인)
4. 사용자 수정 → 삭제
5. 잘못된 API Key로 401 응답 확인

---

## 구현 순서 (권장)

1. **DB 마이그레이션** (002_api_keys.sql)
2. **API Key 모델** (apikeys.go)
3. **Storage DB 메서드** (db.go)
4. **Storage 인터페이스** (storage.go - CreateAPIKey, ValidateAPIKey, DeleteUser)
5. **API 미들웨어** (api_middleware.go)
6. **REST API 핸들러** (api_admin.go - 공개키 변환 포함)
7. **라우팅 통합** (server.go)
8. **관리자 UI 템플릿** (admin_apikeys.templ)
9. **HTMX 핸들러** (admin.go)
10. **대시보드 링크 추가** (dashboard_header.templ)
11. **테스트 및 검증**

---

## API 사용 예제

### 사용자 생성
```bash
curl -X POST http://localhost:8082/api/admin/users \
  -H "Authorization: Bearer ak_9f3b2c1d8e7a6f5b4c3d2e1a0b9c8d7e6f5a4b3c2d1e0f9a8b7" \
  -H "Content-Type: application/json" \
  -d '{
    "pubkey": "82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2",
    "preferred_language": "ko",
    "is_admin": false,
    "active": true
  }'
```

### 사용자 목록 조회
```bash
curl -X GET http://localhost:8082/api/admin/users \
  -H "Authorization: Bearer ak_9f3b2c1d8e7a6f5b4c3d2e1a0b9c8d7e6f5a4b3c2d1e0f9a8b7"
```

### 사용자 삭제
```bash
curl -X DELETE http://localhost:8082/api/admin/users/82341f882b6eabcd2ba7f1ef90aad961cf074af15b9ef44a09f9d2a8fbfbe6a2 \
  -H "Authorization: Bearer ak_9f3b2c1d8e7a6f5b4c3d2e1a0b9c8d7e6f5a4b3c2d1e0f9a8b7"
```

---

## 검증 방법

1. **마이그레이션 확인**
   ```bash
   sqlite3 database.db ".schema api_keys"
   ```

2. **API Key 생성**
   - 관리자 로그인 → http://localhost:8082/admin/apikeys
   - "Create New API Key" 클릭
   - 원본 키 복사 (예: `ak_abc123...`)

3. **API 테스트**
   - curl로 /api/admin/users 요청
   - 32-byte hex 공개키로 사용자 생성
   - 응답에서 동일한 32-byte hex 반환 확인

4. **DB 검증**
   ```bash
   sqlite3 database.db "SELECT * FROM api_keys;"
   sqlite3 database.db "SELECT hex(npub) FROM users;"
   ```
   - api_keys: key_hash (64자), key_prefix (10자) 확인
   - users.npub: 33바이트 (66자 hex) 확인

---

## 완료 기준

- [ ] API Key 생성 UI 동작
- [ ] API Key로 REST API 인증 성공
- [ ] 사용자 CRUD 모두 동작 (32-byte hex 형식)
- [ ] 잘못된 API Key는 401 응답
- [ ] DB에 해시만 저장됨 (원본 키 없음)
- [ ] last_used_at 업데이트됨
- [ ] 공개키 변환 정확 (32-byte ↔ 33-byte)
