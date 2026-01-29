# E2E Test Guide

## 개요
이 E2E 테스트 스크립트는 실제 서버를 실행하고 API Key 기반 사용자 관리 API를 완전히 테스트합니다.

## 사전 요구사항

### 1. 시스템 의존성
```bash
# macOS
brew install libsecret pkg-config sqlite openssl

# Ubuntu/Debian
sudo apt-get install libsecret-1-dev pkg-config sqlite3 openssl curl

# Fedora/RHEL
sudo dnf install libsecret-devel pkg-config sqlite openssl curl
```

### 2. Go 도구
```bash
# goose 설치 (DB 마이그레이션)
go install github.com/pressly/goose/v3/cmd/goose@latest

# sqlite 드라이버 확인
go get github.com/mattn/go-sqlite3
```

## 실행 방법

### 기본 실행
```bash
./test_e2e.sh
```

### 커스텀 포트로 실행
```bash
SERVER_PORT=9090 ./test_e2e.sh
```

## 테스트 시나리오

스크립트는 다음 시나리오를 순차적으로 테스트합니다:

### 1. 인증 테스트
- ✅ Authorization 헤더 없이 요청 (401 예상)
- ✅ 잘못된 API Key로 요청 (401 예상)

### 2. 사용자 목록 조회
- ✅ 빈 사용자 목록 조회 (200, 빈 배열)

### 3. 사용자 생성
- ✅ 유효한 공개키로 사용자 생성 (201)
- ✅ 중복 사용자 생성 시도 (409)
- ✅ 잘못된 공개키로 생성 시도 (400)

### 4. 사용자 조회
- ✅ 존재하는 사용자 조회 (200)
- ✅ 존재하지 않는 사용자 조회 (404)

### 5. 사용자 수정
- ✅ 사용자 정보 업데이트 (200)
- ✅ 언어 및 활성 상태 변경 확인

### 6. 사용자 삭제
- ✅ 사용자 삭제 (204)
- ✅ 삭제된 사용자 조회 시도 (404)

## 출력 예시

```
==========================================
  API E2E Test Suite
==========================================

[✓] All dependencies found
[TEST] Running database migrations...
[✓] Migrations completed
[TEST] Starting server...
Server PID: 12345
Waiting for server to be ready...
[✓] Server is ready!
[TEST] Creating API Key in database...
[✓] API Key created: ak_1234567890abcdef...

==========================================
  Running Tests
==========================================

[TEST] Testing unauthorized access (no API key)...
[✓] Correctly rejected unauthorized request
[TEST] Testing with invalid API key...
[✓] Correctly rejected invalid API key
[TEST] Testing list users (should be empty)...
[✓] User list is empty as expected
[TEST] Creating a new user...
[✓] User created successfully
Response: {"id":"...","pubkey":"3bf0c63f...","preferred_language":"en","is_admin":false,"active":true}
[TEST] Testing duplicate user creation (should fail)...
[✓] Correctly rejected duplicate user
...

==========================================
  All Tests Passed!
==========================================
```

## 수동 API 테스트

테스트 스크립트 실행 후 생성된 API Key로 수동 테스트도 가능합니다:

### 1. API Key 확인
```bash
sqlite3 test_database.db "SELECT key_prefix FROM api_keys;"
```

### 2. 사용자 생성
```bash
API_KEY="your_api_key_here"

curl -X POST http://localhost:8082/api/admin/users \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "pubkey": "3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d",
    "preferred_language": "ko",
    "is_admin": false,
    "active": true
  }'
```

### 3. 사용자 목록 조회
```bash
curl -X GET http://localhost:8082/api/admin/users \
  -H "Authorization: Bearer $API_KEY"
```

### 4. 특정 사용자 조회
```bash
curl -X GET http://localhost:8082/api/admin/users/3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d \
  -H "Authorization: Bearer $API_KEY"
```

### 5. 사용자 수정
```bash
curl -X PUT http://localhost:8082/api/admin/users/3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "preferred_language": "en",
    "is_admin": false,
    "active": false
  }'
```

### 6. 사용자 삭제
```bash
curl -X DELETE http://localhost:8082/api/admin/users/3bf0c63fcb93463407af97a5e5ee64fa883d107ef9e558472c4eb9aaaefa459d \
  -H "Authorization: Bearer $API_KEY"
```

## 트러블슈팅

### 서버가 시작되지 않음
```bash
# 로그 확인
cat server.log

# 포트가 이미 사용 중인지 확인
lsof -i :8082

# 다른 포트로 시도
SERVER_PORT=9090 ./test_e2e.sh
```

### 마이그레이션 실패
```bash
# goose가 설치되었는지 확인
which goose

# 수동 마이그레이션
goose -dir storage/database/migrations sqlite3 test_database.db up
```

### API Key 생성 실패
```bash
# sqlite3가 설치되었는지 확인
which sqlite3

# openssl이 설치되었는지 확인
which openssl
```

## 정리

테스트 후 생성된 파일 정리:
```bash
rm -f test_database.db server.log
```

## CI/CD 통합

GitHub Actions 예시:
```yaml
name: E2E Tests

on: [push, pull_request]

jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Install dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y libsecret-1-dev pkg-config sqlite3
          go install github.com/pressly/goose/v3/cmd/goose@latest

      - name: Run E2E tests
        run: ./test_e2e.sh
```

## 커스터마이징

스크립트 내부의 변수를 수정하여 커스터마이징 가능:
```bash
# test_e2e.sh 파일 내부
SERVER_PORT=8082              # 서버 포트
TEST_DB="test_database.db"    # 테스트 DB 파일명
TEST_USER_PUBKEY="..."        # 테스트용 공개키
```

## 참고

- 실제 프로덕션 환경에서는 API Key를 UI를 통해 생성해야 합니다
- 이 스크립트는 테스트용으로 직접 DB에 API Key를 삽입합니다
- 모든 테스트는 독립적으로 실행되며 서로 영향을 주지 않습니다
