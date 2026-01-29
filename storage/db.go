package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/go-playground/validator/v10"
	"github.com/zitadel/oidc/v3/pkg/op"
)

var validate *validator.Validate

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())
}

type storageDB struct {
	db *sql.DB
}

// BeginTx starts a new database transaction
func (s *storageDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if s.db == nil {
		panic("db cannot be nil")
	}
	return s.db.BeginTx(ctx, opts)
}

// AddAuthRequest inserts a new auth request into the database
func (s *storageDB) AddAuthRequest(tx *sql.Tx, authReq *AuthRequest) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	// Marshal slice fields to JSON
	promptJSON, err := json.Marshal(authReq.Prompt)
	if err != nil {
		return fmt.Errorf("json.Marshal(authReq.Prompt): %w", err)
	}
	uiLocalesJSON, err := json.Marshal(authReq.UiLocales)
	if err != nil {
		return fmt.Errorf("json.Marshal(authReq.UiLocales): %w", err)
	}
	scopesJSON, err := json.Marshal(authReq.Scopes)
	if err != nil {
		return fmt.Errorf("json.Marshal(authReq.Scopes): %w", err)
	}

	// Handle MaxAuthAge (pointer to duration) - store as nanoseconds
	var maxAuthAgeNanos sql.NullInt64
	if authReq.MaxAuthAge != nil {
		maxAuthAgeNanos.Int64 = authReq.MaxAuthAge.Nanoseconds()
		maxAuthAgeNanos.Valid = true
	}

	// Handle CodeChallenge (pointer)
	var challenge, method sql.NullString
	if authReq.CodeChallenge != nil {
		challenge.String = authReq.CodeChallenge.Challenge
		challenge.Valid = true
		method.String = authReq.CodeChallenge.Method
		method.Valid = true
	}

	query := `
		INSERT INTO auth_requests (
			id, creation_date, application_id, callback_uri, transfer_state, prompt,
			ui_locales, login_hint, max_auth_age, user_id, scopes, response_type,
			response_mode, nonce, code_challenge, code_challenge_method, code, done
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(AddAuthRequest): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		authReq.ID, authReq.CreationDate, authReq.ApplicationID, authReq.CallbackURI, authReq.TransferState, string(promptJSON),
		string(uiLocalesJSON), authReq.LoginHint, maxAuthAgeNanos, authReq.UserID, string(scopesJSON), string(authReq.ResponseType),
		string(authReq.ResponseMode), authReq.Nonce, challenge, method, authReq.Code, authReq.Done(),
	)
	if err != nil {
		return fmt.Errorf("stmt.Exec(AddAuthRequest): %w", err)
	}
	return nil
}

// SearchAuthRequestByID retrieves an auth request by ID
func (s *storageDB) SearchAuthRequestByID(tx *sql.Tx, id string) (*AuthRequest, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `
		SELECT id, creation_date, application_id, callback_uri, transfer_state, prompt,
			ui_locales, login_hint, max_auth_age, user_id, scopes, response_type,
			response_mode, nonce, code_challenge, code_challenge_method, code, done
		FROM auth_requests WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("tx.Prepare(SearchAuthRequestByID): %w", err)
	}
	defer stmt.Close()

	var authReq AuthRequest
	var maxAuthAgeNanos sql.NullInt64
	var challenge, method, code sql.NullString
	var promptJSON, uiLocalesJSON, scopesJSON []byte
	var done bool

	err = stmt.QueryRow(id).Scan(
		&authReq.ID, &authReq.CreationDate, &authReq.ApplicationID, &authReq.CallbackURI, &authReq.TransferState, &promptJSON,
		&uiLocalesJSON, &authReq.LoginHint, &maxAuthAgeNanos, &authReq.UserID, &scopesJSON,
		(*string)(&authReq.ResponseType), (*string)(&authReq.ResponseMode), &authReq.Nonce, &challenge, &method, &code, &done,
	)
	if err != nil {
		return nil, fmt.Errorf("stmt.QueryRow.Scan(SearchAuthRequestByID): %w", err)
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal(promptJSON, &authReq.Prompt); err != nil {
		return nil, fmt.Errorf("json.Unmarshal(promptJSON): %w", err)
	}
	if err := json.Unmarshal(uiLocalesJSON, &authReq.UiLocales); err != nil {
		return nil, fmt.Errorf("json.Unmarshal(uiLocalesJSON): %w", err)
	}
	if err := json.Unmarshal(scopesJSON, &authReq.Scopes); err != nil {
		return nil, fmt.Errorf("json.Unmarshal(scopesJSON): %w", err)
	}

	// Handle MaxAuthAge - convert from nanoseconds to duration
	if maxAuthAgeNanos.Valid {
		dur := time.Duration(maxAuthAgeNanos.Int64)
		authReq.MaxAuthAge = &dur
	}

	// Handle CodeChallenge
	if challenge.Valid || method.Valid {
		authReq.CodeChallenge = &OIDCCodeChallenge{
			Challenge: challenge.String,
			Method:    method.String,
		}
	}
	authReq.done = done

	return &authReq, nil
}

// AddUserIDToAuthRequest updates the user_id for a specific auth request
func (s *storageDB) AddUserIDToAuthRequest(tx *sql.Tx, id string, userID string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `UPDATE auth_requests SET user_id = ? WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(AddUserIDToAuthRequest): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(userID, id)
	if err != nil {
		return fmt.Errorf("stmt.Exec(AddUserIDToAuthRequest): %w", err)
	}
	return nil
}

// SetAuthRequestDone marks an auth request as done
func (s *storageDB) SetAuthRequestDone(tx *sql.Tx, id string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `UPDATE auth_requests SET done = ? WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(SetAuthRequestDone): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(true, id)
	if err != nil {
		return fmt.Errorf("stmt.Exec(SetAuthRequestDone): %w", err)
	}
	return nil
}

// SearchAuthRequestByCode retrieves an auth request by code_challenge
func (s *storageDB) SearchAuthRequestByCode(tx *sql.Tx, authCodeParam string) (*AuthRequest, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `
		SELECT id, creation_date, application_id, callback_uri, transfer_state, prompt,
			ui_locales, login_hint, max_auth_age, user_id, scopes, response_type,
			response_mode, nonce, code_challenge, code_challenge_method, code, done
		FROM auth_requests WHERE code = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("tx.Prepare(SearchAuthRequestByCode): %w", err)
	}
	defer stmt.Close()

	var authReq AuthRequest
	var maxAuthAgeNanos sql.NullInt64
	var challenge, method, code sql.NullString
	var promptJSON, uiLocalesJSON, scopesJSON []byte
	var done bool

	err = stmt.QueryRow(authCodeParam).Scan(
		&authReq.ID, &authReq.CreationDate, &authReq.ApplicationID, &authReq.CallbackURI, &authReq.TransferState, &promptJSON,
		&uiLocalesJSON, &authReq.LoginHint, &maxAuthAgeNanos, &authReq.UserID, &scopesJSON,
		(*string)(&authReq.ResponseType), (*string)(&authReq.ResponseMode), &authReq.Nonce, &challenge, &method, &code, &done,
	)
	if err != nil {
		return nil, fmt.Errorf("stmt.QueryRow.Scan(SearchAuthRequestByCode): %w", err)
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal(promptJSON, &authReq.Prompt); err != nil {
		return nil, fmt.Errorf("json.Unmarshal(promptJSON): %w", err)
	}
	if err := json.Unmarshal(uiLocalesJSON, &authReq.UiLocales); err != nil {
		return nil, fmt.Errorf("json.Unmarshal(uiLocalesJSON): %w", err)
	}
	if err := json.Unmarshal(scopesJSON, &authReq.Scopes); err != nil {
		return nil, fmt.Errorf("json.Unmarshal(scopesJSON): %w", err)
	}

	// Handle MaxAuthAge - convert from nanoseconds to duration
	if maxAuthAgeNanos.Valid {
		dur := time.Duration(maxAuthAgeNanos.Int64)
		authReq.MaxAuthAge = &dur
	}

	// Handle CodeChallenge
	if challenge.Valid || method.Valid {
		authReq.CodeChallenge = &OIDCCodeChallenge{
			Challenge: challenge.String,
			Method:    method.String,
		}
	}
	authReq.done = done

	return &authReq, nil
}

// DeleteAuthRequest removes an auth request by ID
func (s *storageDB) DeleteAuthRequest(tx *sql.Tx, id string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `DELETE FROM auth_requests WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(DeleteAuthRequest): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(id)
	if err != nil {
		return fmt.Errorf("stmt.Exec(DeleteAuthRequest): %w", err)
	}
	return nil
}

// AddClient inserts a new client into the database
func (s *storageDB) AddClient(tx *sql.Tx, client *Client) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	// Marshal slice fields to JSON
	redirectURIsJSON, err := json.Marshal(client.redirectURIs)
	if err != nil {
		return fmt.Errorf("json.Marshal(redirectURIs): %w", err)
	}
	responseTypesJSON, err := json.Marshal(client.responseTypes)
	if err != nil {
		return fmt.Errorf("json.Marshal(responseTypes): %w", err)
	}
	grantTypesJSON, err := json.Marshal(client.grantTypes)
	if err != nil {
		return fmt.Errorf("json.Marshal(grantTypes): %w", err)
	}
	postLogoutRedirectURIsJSON, err := json.Marshal(client.postLogoutRedirectURIGlobs)
	if err != nil {
		return fmt.Errorf("json.Marshal(postLogoutRedirectURIGlobs): %w", err)
	}
	redirectURIsGlobsJSON, err := json.Marshal(client.redirectURIGlobs)
	if err != nil {
		return fmt.Errorf("json.Marshal(redirectURIGlobs): %w", err)
	}

	query := `
		INSERT INTO clients (
			id, secret, redirect_uris, application_type, auth_method, response_types,
			grant_types, access_token_type, dev_mode, id_token_userinfo_claims_assertion,
			clock_skew, post_logout_redirect_uri_globs, redirect_uri_globs
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(AddClient): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		client.id, client.secret, string(redirectURIsJSON), int(client.applicationType),
		string(client.authMethod), string(responseTypesJSON), string(grantTypesJSON),
		int(client.accessTokenType), client.devMode, client.idTokenUserinfoClaimsAssertion,
		client.clockSkew.Nanoseconds(), string(postLogoutRedirectURIsJSON), string(redirectURIsGlobsJSON),
	)
	return err
}

// SearchClientByID retrieves a client by ID
func (s *storageDB) SearchClientByID(tx *sql.Tx, id string) (*Client, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `
		SELECT id, secret, redirect_uris, application_type, auth_method, response_types,
			   grant_types, access_token_type, dev_mode, id_token_userinfo_claims_assertion,
			   clock_skew, post_logout_redirect_uri_globs, redirect_uri_globs
		FROM clients WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("tx.Prepare(SearchClientByID): %w", err)
	}
	defer stmt.Close()

	var client Client
	row := stmt.QueryRow(id)

	err = client.ScanRow(row)
	if err != nil {
		return nil, fmt.Errorf("client.ScanRow(SearchClientByID): %w", err)
	}

	return &client, nil
}

// EditClient updates an existing client by ID with all fields
func (s *storageDB) EditClient(tx *sql.Tx, client *Client) error {
	if tx == nil {
		panic("tx cannot be nil")
	}
	if client == nil {
		panic("client cannot be nil")
	}

	// Marshal slice fields to JSON
	redirectURIsJSON, err := json.Marshal(client.redirectURIs)
	if err != nil {
		return fmt.Errorf("json.Marshal(redirectURIs): %w", err)
	}
	responseTypesJSON, err := json.Marshal(client.responseTypes)
	if err != nil {
		return fmt.Errorf("json.Marshal(responseTypes): %w", err)
	}
	grantTypesJSON, err := json.Marshal(client.grantTypes)
	if err != nil {
		return fmt.Errorf("json.Marshal(grantTypes): %w", err)
	}
	postLogoutRedirectURIsJSON, err := json.Marshal(client.postLogoutRedirectURIGlobs)
	if err != nil {
		return fmt.Errorf("json.Marshal(postLogoutRedirectURIGlobs): %w", err)
	}
	redirectURIsGlobsJSON, err := json.Marshal(client.redirectURIGlobs)
	if err != nil {
		return fmt.Errorf("json.Marshal(redirectURIGlobs): %w", err)
	}

	query := `
		UPDATE clients SET
			redirect_uris = ?, application_type = ?, auth_method = ?,
			response_types = ?, grant_types = ?, access_token_type = ?, dev_mode = ?,
			id_token_userinfo_claims_assertion = ?, clock_skew = ?,
			post_logout_redirect_uri_globs = ?, redirect_uri_globs = ?
		WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(EditClient): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		string(redirectURIsJSON), int(client.applicationType),
		string(client.authMethod), string(responseTypesJSON), string(grantTypesJSON),
		int(client.accessTokenType), client.devMode, client.idTokenUserinfoClaimsAssertion,
		client.clockSkew.Nanoseconds(), string(postLogoutRedirectURIsJSON), string(redirectURIsGlobsJSON),
		client.id,
	)
	if err != nil {
		return fmt.Errorf("stmt.Exec(EditClient): %w", err)
	}
	return nil
}

// DeleteClient removes a client by ID
func (s *storageDB) DeleteClient(tx *sql.Tx, id string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `DELETE FROM clients WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(DeleteClient): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(id)
	if err != nil {
		return fmt.Errorf("stmt.Exec(AddClient): %w", err)
	}
	return nil
}

// AddUser inserts a new user into the database
func (s *storageDB) AddUser(tx *sql.Tx, user *User) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	// Serialize public key to bytes
	var npubBytes []byte
	if user.Npub != nil {
		npubBytes = user.Npub.SerializeCompressed()
	}

	query := `INSERT INTO users (id, npub, preferred_language, is_admin, active) VALUES (?, ?, ?, ?, ?)`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(AddUser): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(user.ID, npubBytes, user.PreferredLanguage.String(), user.IsAdmin, user.Active)
	return err
}

// SearchUserByID retrieves a user by ID
func (s *storageDB) SearchUserByID(tx *sql.Tx, id string) (*User, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `SELECT id, npub, preferred_language, is_admin, active FROM users WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("tx.Prepare(SearchUserByID): %w", err)
	}
	defer stmt.Close()

	var user User
	row := stmt.QueryRow(id)

	err = user.ScanRow(row)
	if err != nil {
		return nil, fmt.Errorf("user.ScanRow(SearchUserByID): %w", err)
	}

	return &user, nil
}

// SearchUserByNpub retrieves a user by their public key
func (s *storageDB) SearchUserByNpub(tx *sql.Tx, npub *btcec.PublicKey) (*User, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}
	if npub == nil {
		panic("npub cannot be nil")
	}

	// Serialize public key to bytes for database lookup
	npubBytes := npub.SerializeCompressed()

	query := `SELECT id, npub, preferred_language, is_admin, active FROM users WHERE npub = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("tx.Prepare(SearchUserByNpub): %w", err)
	}
	defer stmt.Close()

	var user User
	row := stmt.QueryRow(npubBytes)

	err = user.ScanRow(row)
	if err != nil {
		return nil, fmt.Errorf("user.ScanRow(SearchUserByNpub): %w", err)
	}

	return &user, nil
}
func (s *storageDB) SearchUserById(tx *sql.Tx, id string) (*User, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `SELECT id, npub, preferred_language, is_admin, active FROM users WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("tx.Prepare(SearchUserByNpub): %w", err)
	}
	defer stmt.Close()

	var user User
	row := stmt.QueryRow(id)

	err = user.ScanRow(row)
	if err != nil {
		return nil, fmt.Errorf("user.ScanRow(SearchUserByNpub): %w", err)
	}

	return &user, nil
}

// EditUser updates an existing user by ID with all fields
func (s *storageDB) EditUser(tx *sql.Tx, user *User) error {
	if tx == nil {
		panic("tx cannot be nil")
	}
	if user == nil {
		panic("user cannot be nil")
	}

	// Serialize public key to bytes
	var npubBytes []byte
	if user.Npub != nil {
		npubBytes = user.Npub.SerializeCompressed()
	}

	query := `
		UPDATE users SET
			npub = ?, preferred_language = ?, is_admin = ?, active = ?
		WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(EditUser): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		npubBytes, user.PreferredLanguage.String(), user.IsAdmin, user.Active,
		user.ID,
	)
	if err != nil {
		return fmt.Errorf("stmt.Exec(EditUser): %w", err)
	}
	return nil
}

// DeleteUser removes a user by ID
func (s *storageDB) DeleteUser(tx *sql.Tx, id string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `DELETE FROM users WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(DeleteUser): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(id)
	if err != nil {
		return fmt.Errorf("stmt.Exec(DeleteClient): %w", err)
	}
	return nil
}

// AddToken inserts a new token into the database
func (s *storageDB) AddToken(tx *sql.Tx, token *Token) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	// Marshal slice fields to JSON
	audienceJSON, err := json.Marshal(token.Audience)
	if err != nil {
		return fmt.Errorf("json.Marshal(token.Audience): %w", err)
	}
	scopesJSON, err := json.Marshal(token.Scopes)
	if err != nil {
		return fmt.Errorf("json.Marshal(token.Scopes): %w", err)
	}

	fmt.Printf("\n token: %+v \n", *token)

	query := `
		INSERT INTO tokens (id, application_id, subject, refresh_token_id, audience, expiration, scopes)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(AddToken): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		token.ID, token.ApplicationID, token.Subject, token.RefreshTokenID,
		string(audienceJSON), token.Expiration, string(scopesJSON),
	)
	return err
}

// SearchTokenByID retrieves a token by ID
func (s *storageDB) SearchTokenByID(tx *sql.Tx, id string) (*Token, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `SELECT id, application_id, subject, refresh_token_id, audience, expiration, scopes FROM tokens WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("tx.Prepare(SearchTokenByID): %w", err)
	}
	defer stmt.Close()

	var token Token
	row := stmt.QueryRow(id)

	err = token.ScanRow(row)
	if err != nil {
		return nil, fmt.Errorf("token.ScanRow(SearchTokenByID): %w", err)
	}

	return &token, nil
}

// DeleteToken removes a token by ID
func (s *storageDB) DeleteToken(tx *sql.Tx, id string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `DELETE FROM tokens WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(DeleteToken): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(id)
	if err != nil {
		return fmt.Errorf("stmt.Exec(AddUser): %w", err)
	}
	return nil
}

// AddRefreshToken inserts a new refresh token into the database
func (s *storageDB) AddRefreshToken(tx *sql.Tx, refreshToken *RefreshToken) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	// Marshal slice fields to JSON
	amrJSON, err := json.Marshal(refreshToken.AMR)
	if err != nil {
		return fmt.Errorf("json.Marshal(refreshToken.AMR): %w", err)
	}
	audienceJSON, err := json.Marshal(refreshToken.Audience)
	if err != nil {
		return fmt.Errorf("json.Marshal(refreshToken.Audience): %w", err)
	}
	scopesJSON, err := json.Marshal(refreshToken.Scopes)
	if err != nil {
		return fmt.Errorf("json.Marshal(refreshToken.Scopes): %w", err)
	}

	query := `
		INSERT INTO refresh_tokens (
			id, token, auth_time, amr, audience, user_id, application_id, expiration, scopes, access_token
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(AddRefreshToken): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		refreshToken.ID, refreshToken.Token, refreshToken.AuthTime, string(amrJSON),
		string(audienceJSON), refreshToken.UserID, refreshToken.ApplicationID, refreshToken.Expiration,
		string(scopesJSON), refreshToken.AccessToken,
	)
	return err
}

// SearchRefreshTokenByID retrieves a refresh token by ID
func (s *storageDB) SearchRefreshTokenByID(tx *sql.Tx, id string) (*RefreshToken, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `
		SELECT id, token, auth_time, amr, audience, user_id, application_id, expiration, scopes, access_token
		FROM refresh_tokens WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("tx.Prepare(SearchRefreshTokenByID): %w", err)
	}
	defer stmt.Close()

	var refreshToken RefreshToken
	row := stmt.QueryRow(id)

	err = refreshToken.ScanRow(row)
	if err != nil {
		return nil, fmt.Errorf("refreshToken.ScanRow(SearchRefreshTokenByID): %w", err)
	}

	return &refreshToken, nil
}

// DeleteRefreshToken removes a refresh token by ID
func (s *storageDB) DeleteRefreshToken(tx *sql.Tx, id string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `DELETE FROM refresh_tokens WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(id)
	if err != nil {
		return fmt.Errorf("stmt.Exec(DeleteUser): %w", err)
	}
	return nil
}

// SaveAuthCode updates an auth request with the authorization code
func (s *storageDB) SaveAuthCode(tx *sql.Tx, id, code string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `UPDATE auth_requests SET code = ? WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(code, id)
	if err != nil {
		return fmt.Errorf("stmt.Exec(AddToken): %w", err)
	}
	return nil
}

// DeleteTokensByUserAndClient removes all tokens for a specific user and client
func (s *storageDB) DeleteTokensByUserAndClient(tx *sql.Tx, userID, clientID string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `DELETE FROM tokens WHERE subject = ? AND application_id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(DeleteToken): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(userID, clientID)
	if err != nil {
		return fmt.Errorf("stmt.Exec(AddRefreshToken): %w", err)
	}
	return nil
}

// DeleteRefreshTokensByUserAndClient removes all refresh tokens for a specific user and client
func (s *storageDB) DeleteRefreshTokensByUserAndClient(tx *sql.Tx, userID, clientID string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `DELETE FROM refresh_tokens WHERE user_id = ? AND application_id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(userID, clientID)
	return err
}

// HealthCheck performs a simple database health check
func (s *storageDB) HealthCheck(tx *sql.Tx) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	_, err := tx.Exec("SELECT 1")
	return err
}

// AddDeviceAuthorization inserts a new device authorization into the database
func (s *storageDB) AddDeviceAuthorization(tx *sql.Tx, entry *deviceAuthorizationEntry) error {
	if tx == nil {
		panic("tx cannot be nil")
	}
	if entry == nil || entry.state == nil {
		panic("entry and entry.state cannot be nil")
	}

	// Marshal the entire state to JSON
	stateJSON, err := json.Marshal(entry.state)
	if err != nil {
		return fmt.Errorf("json.Marshal(entry.state): %w", err)
	}

	query := `
		INSERT INTO device_authorizations (
			device_code, user_code, state
		) VALUES (?, ?, ?)`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(AddDeviceAuthorization): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(entry.deviceCode, entry.userCode, string(stateJSON))
	if err != nil {
		return fmt.Errorf("stmt.Exec(AddDeviceAuthorization): %w", err)
	}
	return nil
}

// SearchDeviceAuthorizationByDeviceCode retrieves a device authorization by device code
func (s *storageDB) SearchDeviceAuthorizationByDeviceCode(tx *sql.Tx, deviceCode string) (*deviceAuthorizationEntry, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `
		SELECT device_code, user_code, state
		FROM device_authorizations WHERE device_code = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("tx.Prepare(SearchDeviceAuthorizationByDeviceCode): %w", err)
	}
	defer stmt.Close()

	var entry deviceAuthorizationEntry
	row := stmt.QueryRow(deviceCode)

	err = entry.ScanRow(row)
	if err != nil {
		return nil, fmt.Errorf("entry.ScanRow(SearchDeviceAuthorizationByDeviceCode): %w", err)
	}

	return &entry, nil
}

// SearchDeviceAuthorizationByUserCode retrieves a device authorization by user code
func (s *storageDB) SearchDeviceAuthorizationByUserCode(tx *sql.Tx, userCode string) (*deviceAuthorizationEntry, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `
		SELECT device_code, user_code, state
		FROM device_authorizations WHERE user_code = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("tx.Prepare(SearchDeviceAuthorizationByUserCode): %w", err)
	}
	defer stmt.Close()

	var entry deviceAuthorizationEntry
	row := stmt.QueryRow(userCode)

	err = entry.ScanRow(row)
	if err != nil {
		return nil, fmt.Errorf("entry.ScanRow(SearchDeviceAuthorizationByUserCode): %w", err)
	}

	return &entry, nil
}

// UpdateDeviceAuthorizationSubject updates the subject field and marks the authorization as done
func (s *storageDB) UpdateDeviceAuthorizationSubject(tx *sql.Tx, userCode, subject string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	// First, read the current state
	var stateJSON []byte
	query := `SELECT state FROM device_authorizations WHERE user_code = ?`
	err := tx.QueryRow(query, userCode).Scan(&stateJSON)
	if err != nil {
		return fmt.Errorf("failed to read current state: %w", err)
	}

	// Unmarshal the state
	var state op.DeviceAuthorizationState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return fmt.Errorf("json.Unmarshal(stateJSON): %w", err)
	}

	// Update the state
	state.Subject = subject
	state.Done = true

	// Marshal back to JSON
	updatedStateJSON, err := json.Marshal(&state)
	if err != nil {
		return fmt.Errorf("json.Marshal(state): %w", err)
	}

	// Update in database
	updateQuery := `UPDATE device_authorizations SET state = ? WHERE user_code = ?`
	stmt, err := tx.Prepare(updateQuery)
	if err != nil {
		return fmt.Errorf("tx.Prepare(UpdateDeviceAuthorizationSubject): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(string(updatedStateJSON), userCode)
	if err != nil {
		return fmt.Errorf("stmt.Exec(UpdateDeviceAuthorizationSubject): %w", err)
	}
	return nil
}

// UpdateDeviceAuthorizationDenied marks the device authorization as denied
func (s *storageDB) UpdateDeviceAuthorizationDenied(tx *sql.Tx, userCode string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	// First, read the current state
	var stateJSON []byte
	query := `SELECT state FROM device_authorizations WHERE user_code = ?`
	err := tx.QueryRow(query).Scan(&stateJSON)
	if err != nil {
		return fmt.Errorf("failed to read current state: %w", err)
	}

	// Unmarshal the state
	var state op.DeviceAuthorizationState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return fmt.Errorf("json.Unmarshal(stateJSON): %w", err)
	}

	// Update the state
	state.Denied = true

	// Marshal back to JSON
	updatedStateJSON, err := json.Marshal(&state)
	if err != nil {
		return fmt.Errorf("json.Marshal(state): %w", err)
	}

	// Update in database
	updateQuery := `UPDATE device_authorizations SET state = ? WHERE user_code = ?`
	stmt, err := tx.Prepare(updateQuery)
	if err != nil {
		return fmt.Errorf("tx.Prepare(UpdateDeviceAuthorizationDenied): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(string(updatedStateJSON), userCode)
	if err != nil {
		return fmt.Errorf("stmt.Exec(UpdateDeviceAuthorizationDenied): %w", err)
	}
	return nil
}

// DeleteDeviceAuthorization removes a device authorization by device code
func (s *storageDB) DeleteDeviceAuthorization(tx *sql.Tx, deviceCode string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `DELETE FROM device_authorizations WHERE device_code = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(DeleteDeviceAuthorization): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(deviceCode)
	if err != nil {
		return fmt.Errorf("stmt.Exec(DeleteDeviceAuthorization): %w", err)
	}
	return nil
}

// SearchAllClients retrieves all clients from the database
func (s *storageDB) SearchAllClients(tx *sql.Tx) ([]Client, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `
		SELECT id, secret, redirect_uris, application_type, auth_method, response_types,
			   grant_types, access_token_type, dev_mode, id_token_userinfo_claims_assertion,
			   clock_skew, post_logout_redirect_uri_globs, redirect_uri_globs
		FROM clients`

	rows, err := tx.Query(query)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []Client{}, nil
		}
		return nil, fmt.Errorf("tx.Query(SearchAllClients): %w", err)
	}
	defer rows.Close()

	var clients []Client
	for rows.Next() {
		var client Client
		err := client.ScanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("client.ScanRow(SearchAllClients): %w", err)
		}
		clients = append(clients, client)
	}

	// Check for any error that occurred during iteration
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err(SearchAllClients): %w", err)
	}

	return clients, nil
}

func (s *storageDB) SearchAllNonAdminClients(tx *sql.Tx) ([]Client, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `
		SELECT id, secret, redirect_uris, application_type, auth_method, response_types,
			   grant_types, access_token_type, dev_mode, id_token_userinfo_claims_assertion,
			   clock_skew, post_logout_redirect_uri_globs, redirect_uri_globs
		FROM clients WHERE id != ?`

	rows, err := tx.Query(query, OICD_ADMIN_DASHBOARD_CLIENT_ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []Client{}, nil
		}
		return nil, fmt.Errorf("tx.Query(SearchAllClients): %w", err)
	}
	defer rows.Close()

	var clients []Client
	for rows.Next() {
		var client Client
		err := client.ScanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("client.ScanRow(SearchAllClients): %w", err)
		}
		clients = append(clients, client)
	}

	// Check for any error that occurred during iteration
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err(SearchAllClients): %w", err)
	}

	return clients, nil
}

// SearchAllUsers retrieves all users from the database
func (s *storageDB) SearchAllUsers(tx *sql.Tx, admin bool) ([]User, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `SELECT id, npub, preferred_language, is_admin, active FROM users WHERE is_admin == ?`

	rows, err := tx.Query(query, admin)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []User{}, nil
		}
		return nil, fmt.Errorf("tx.Query(SearchAllUsers): %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		err := user.ScanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("user.ScanRow(SearchAllUsers): %w", err)
		}
		users = append(users, user)
	}

	// Check for any error that occurred during iteration
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err(SearchAllUsers): %w", err)
	}

	return users, nil
}

// SaveConfig inserts or updates the application configuration
func (s *storageDB) SaveConfig(tx *sql.Tx, config *Configuration) error {
	if tx == nil {
		panic("tx cannot be nil")
	}
	if config == nil {
		panic("config cannot be nil")
	}

	// Validate the configuration struct
	if err := validate.Struct(config); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	query := `
		INSERT INTO configuration (id, max_clients, max_users, last_updated, registration_type, nsec, encryption_key)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			max_clients = excluded.max_clients,
			max_users = excluded.max_users,
			last_updated = excluded.last_updated,
			registration_type = excluded.registration_type,
			nsec = excluded.nsec,
			encryption_key = excluded.encryption_key`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(SaveConfig): %w", err)
	}
	defer stmt.Close()

	// Handle nsec as optional []byte - pass nil if nsec is empty
	var nsecValue interface{}
	if len(config.Nsec) > 0 {
		nsecValue = config.Nsec
	} else {
		nsecValue = nil
	}

	_, err = stmt.Exec(config.MaxClients, config.MaxUsers, config.LastUpdated, config.RegistrationType, nsecValue, config.EncryptionKey)
	if err != nil {
		return fmt.Errorf("stmt.Exec(SaveConfig): %w", err)
	}
	return nil
}

// GetConfig retrieves the application configuration, including the optional nsec field as a blob
func (s *storageDB) GetConfig(tx *sql.Tx) (*Configuration, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `SELECT max_clients, max_users, last_updated, registration_type, nsec, encryption_key FROM configuration WHERE id = 1`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("tx.Prepare(GetConfig): %w", err)
	}
	defer stmt.Close()

	var config Configuration
	row := stmt.QueryRow()

	err = config.ScanRow(row)
	if err != nil {
		return nil, fmt.Errorf("config.ScanRow(GetConfig): %w", err)
	}

	return &config, nil
}

// CreateAPIKey inserts a new API key into the database
func (s *storageDB) CreateAPIKey(tx *sql.Tx, apiKey *APIKey) error {
	if tx == nil {
		panic("tx cannot be nil")
	}
	if apiKey == nil {
		panic("apiKey cannot be nil")
	}

	query := `
		INSERT INTO api_keys (id, label, key_prefix, key_hash, created_at, last_used_at, created_by, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(CreateAPIKey): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(
		apiKey.ID,
		apiKey.Label,
		apiKey.KeyPrefix,
		apiKey.KeyHash,
		apiKey.CreatedAt,
		apiKey.LastUsedAt,
		apiKey.CreatedBy,
		apiKey.IsActive,
	)
	if err != nil {
		return fmt.Errorf("stmt.Exec(CreateAPIKey): %w", err)
	}

	return nil
}

// SearchAPIKeyByHash retrieves an API key by its hash
func (s *storageDB) SearchAPIKeyByHash(tx *sql.Tx, keyHash string) (*APIKey, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `
		SELECT id, label, key_prefix, key_hash, created_at, last_used_at, created_by, is_active
		FROM api_keys
		WHERE key_hash = ? AND is_active = 1`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return nil, fmt.Errorf("tx.Prepare(SearchAPIKeyByHash): %w", err)
	}
	defer stmt.Close()

	var apiKey APIKey
	row := stmt.QueryRow(keyHash)

	err = apiKey.ScanRow(row)
	if err != nil {
		return nil, fmt.Errorf("apiKey.ScanRow(SearchAPIKeyByHash): %w", err)
	}

	return &apiKey, nil
}

// UpdateAPIKeyLastUsed updates the last_used_at timestamp for an API key
func (s *storageDB) UpdateAPIKeyLastUsed(tx *sql.Tx, keyHash string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `UPDATE api_keys SET last_used_at = ? WHERE key_hash = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(UpdateAPIKeyLastUsed): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(time.Now(), keyHash)
	if err != nil {
		return fmt.Errorf("stmt.Exec(UpdateAPIKeyLastUsed): %w", err)
	}

	return nil
}

// GetAllAPIKeys retrieves all API keys from the database
func (s *storageDB) GetAllAPIKeys(tx *sql.Tx) ([]APIKey, error) {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `
		SELECT id, label, key_prefix, key_hash, created_at, last_used_at, created_by, is_active
		FROM api_keys
		ORDER BY created_at DESC`

	rows, err := tx.Query(query)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []APIKey{}, nil
		}
		return nil, fmt.Errorf("tx.Query(GetAllAPIKeys): %w", err)
	}
	defer rows.Close()

	var apiKeys []APIKey
	for rows.Next() {
		var apiKey APIKey
		err := apiKey.ScanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("apiKey.ScanRow(GetAllAPIKeys): %w", err)
		}
		apiKeys = append(apiKeys, apiKey)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err(GetAllAPIKeys): %w", err)
	}

	return apiKeys, nil
}

// DeleteAPIKey removes an API key by ID
func (s *storageDB) DeleteAPIKey(tx *sql.Tx, id string) error {
	if tx == nil {
		panic("tx cannot be nil")
	}

	query := `DELETE FROM api_keys WHERE id = ?`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return fmt.Errorf("tx.Prepare(DeleteAPIKey): %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(id)
	if err != nil {
		return fmt.Errorf("stmt.Exec(DeleteAPIKey): %w", err)
	}

	return nil
}
