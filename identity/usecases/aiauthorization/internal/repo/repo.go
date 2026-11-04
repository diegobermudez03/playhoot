// Package repo persists AI applications' access: registered clients,
// authorization requests with their decision and code, approved connections
// and their rotating refresh tokens. Only secrets' hashes are kept; each
// decision, use and revocation is written once.
package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is AI authorization's persistence.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo on db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// GetDB returns the database, for transactions.
func (r *Repo) GetDB() *gorm.DB {
	return r.db
}

// Client is a registered AI application.
type Client struct {
	ID           int64
	ClientID     string
	ClientName   string
	ClientURI    *string
	RedirectURIs []string
	GrantTypes   []string
	CreatedAt    time.Time
}

type clientRow struct {
	ID           int64
	ClientID     string
	ClientName   string
	ClientURI    *string
	RedirectURIs []byte `gorm:"column:redirect_uris"`
	GrantTypes   []byte `gorm:"column:grant_types"`
	CreatedAt    time.Time
}

func (clientRow) TableName() string { return "identity_ai_clients" }

// CreateClient stores a registered application.
func (r *Repo) CreateClient(ctx context.Context, client Client) error {
	redirects, err := json.Marshal(client.RedirectURIs)
	if err != nil {
		return fmt.Errorf("encoding redirect URIs: %s", err)
	}
	grants, err := json.Marshal(client.GrantTypes)
	if err != nil {
		return fmt.Errorf("encoding grant types: %s", err)
	}
	row := clientRow{ClientID: client.ClientID, ClientName: client.ClientName, ClientURI: client.ClientURI, RedirectURIs: redirects, GrantTypes: grants, CreatedAt: client.CreatedAt}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating AI client: %s", err)
	}
	return nil
}

// FindClient returns the application registered as clientID, or nil.
func (r *Repo) FindClient(ctx context.Context, tx *gorm.DB, clientID string) (*Client, error) {
	var rows []clientRow
	if err := tx.WithContext(ctx).Raw(`SELECT id, client_id, client_name, client_uri, redirect_uris, grant_types, created_at FROM identity_ai_clients WHERE client_id = ?`, clientID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("finding AI client: %s", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return decodeClient(rows[0])
}

func decodeClient(row clientRow) (*Client, error) {
	client := &Client{ID: row.ID, ClientID: row.ClientID, ClientName: row.ClientName, ClientURI: row.ClientURI, CreatedAt: row.CreatedAt}
	if err := json.Unmarshal(row.RedirectURIs, &client.RedirectURIs); err != nil {
		return nil, fmt.Errorf("decoding redirect URIs: %s", err)
	}
	if err := json.Unmarshal(row.GrantTypes, &client.GrantTypes); err != nil {
		return nil, fmt.Errorf("decoding grant types: %s", err)
	}
	return client, nil
}

// Request is an authorization request with its client and decision.
type Request struct {
	ID            int64
	RequestID     string
	ClientRowID   int64
	ClientID      string
	ClientName    string
	ClientURI     *string
	RedirectURI   string
	State         *string
	CodeChallenge string
	Scope         string
	Resource      string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	DecidedAt     *time.Time
	DecidedUserID *int64
	Approved      *bool
	CodeExpiresAt *time.Time
	CodeUsedAt    *time.Time
	SessionRowID  *int64
}

type requestRow struct {
	ID            int64
	RequestID     string
	ClientRowID   int64
	RedirectURI   string
	State         *string
	CodeChallenge string
	Scope         string
	Resource      string
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

func (requestRow) TableName() string { return "identity_ai_authorization_requests" }

// CreateRequest stores a pending authorization request.
func (r *Repo) CreateRequest(ctx context.Context, request Request) error {
	row := requestRow{
		RequestID: request.RequestID, ClientRowID: request.ClientRowID, RedirectURI: request.RedirectURI, State: request.State,
		CodeChallenge: request.CodeChallenge, Scope: request.Scope, Resource: request.Resource, CreatedAt: request.CreatedAt, ExpiresAt: request.ExpiresAt,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating AI authorization request: %s", err)
	}
	return nil
}

const selectRequest = `
	SELECT q.id, q.request_id, q.client_row_id, c.client_id, c.client_name, c.client_uri,
		q.redirect_uri, q.state, q.code_challenge, q.scope, q.resource, q.created_at, q.expires_at,
		q.decided_at, q.decided_user_id, q.approved, q.code_expires_at, q.code_used_at, q.session_row_id
	FROM identity_ai_authorization_requests q
	JOIN identity_ai_clients c ON c.id = q.client_row_id
`

func (r *Repo) findRequest(ctx context.Context, tx *gorm.DB, where string, value string, lock bool) (*Request, error) {
	query := selectRequest + where
	if lock {
		query += ` FOR UPDATE OF q`
	}
	var found []Request
	if err := tx.WithContext(ctx).Raw(query, value).Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("finding AI authorization request: %s", err)
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}

// FindRequest returns the request requestID, locked until tx ends when lock
// is set, or nil.
func (r *Repo) FindRequest(ctx context.Context, tx *gorm.DB, requestID string, lock bool) (*Request, error) {
	return r.findRequest(ctx, tx, `WHERE q.request_id = ?`, requestID, lock)
}

// FindRequestByCode returns the request whose code hashes to codeSHA256,
// locked until tx ends, or nil.
func (r *Repo) FindRequestByCode(ctx context.Context, tx *gorm.DB, codeSHA256 string) (*Request, error) {
	return r.findRequest(ctx, tx, `WHERE q.code_sha256 = ?`, codeSHA256, true)
}

// DecideRequest records a still pending request's decision, with the code an
// approval issues; false when it was already decided.
func (r *Repo) DecideRequest(ctx context.Context, tx *gorm.DB, id int64, decidedAt time.Time, userID int64, approved bool, codeSHA256 *string, codeExpiresAt *time.Time) (bool, error) {
	result := tx.WithContext(ctx).Exec(`
		UPDATE identity_ai_authorization_requests
		SET decided_at = ?, decided_user_id = ?, approved = ?, code_sha256 = ?, code_expires_at = ?
		WHERE id = ? AND decided_at IS NULL`,
		decidedAt, userID, approved, codeSHA256, codeExpiresAt, id,
	)
	if result.Error != nil {
		return false, fmt.Errorf("deciding AI authorization request: %s", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// UseCode records that an approved request's code was exchanged for the
// connection sessionRowID; false when it already was.
func (r *Repo) UseCode(ctx context.Context, tx *gorm.DB, id int64, usedAt time.Time, sessionRowID int64) (bool, error) {
	result := tx.WithContext(ctx).Exec(`
		UPDATE identity_ai_authorization_requests SET code_used_at = ?, session_row_id = ?
		WHERE id = ? AND code_used_at IS NULL`,
		usedAt, sessionRowID, id,
	)
	if result.Error != nil {
		return false, fmt.Errorf("using AI authorization code: %s", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// FindUserID returns the id of the User userUUID, or false.
func (r *Repo) FindUserID(ctx context.Context, tx *gorm.DB, userUUID string) (int64, bool, error) {
	var ids []int64
	if err := tx.WithContext(ctx).Raw(`SELECT id FROM identity_users WHERE user_uuid = ?`, userUUID).Scan(&ids).Error; err != nil {
		return 0, false, fmt.Errorf("finding user: %s", err)
	}
	if len(ids) == 0 {
		return 0, false, nil
	}
	return ids[0], true, nil
}

// Session is an approved AI connection.
type Session struct {
	ID           int64
	ConnectionID string
	UserID       int64
	UserUUID     string
	ClientRowID  int64
	ClientID     string
	ClientName   string
	Scope        string
	Resource     string
	CreatedAt    time.Time
	RevokedAt    *time.Time
}

type sessionRow struct {
	ID           int64
	ConnectionID string
	UserID       int64
	ClientRowID  int64
	Scope        string
	Resource     string
	CreatedAt    time.Time
}

func (sessionRow) TableName() string { return "identity_ai_sessions" }

// CreateSession stores an approved connection and returns its row id.
func (r *Repo) CreateSession(ctx context.Context, tx *gorm.DB, session Session) (int64, error) {
	row := sessionRow{ConnectionID: session.ConnectionID, UserID: session.UserID, ClientRowID: session.ClientRowID, Scope: session.Scope, Resource: session.Resource, CreatedAt: session.CreatedAt}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("creating AI session: %s", err)
	}
	return row.ID, nil
}

const selectSession = `
	SELECT s.id, s.connection_id, s.user_id, u.user_uuid, s.client_row_id, c.client_id, c.client_name,
		s.scope, s.resource, s.created_at, s.revoked_at
	FROM identity_ai_sessions s
	JOIN identity_users u ON u.id = s.user_id
	JOIN identity_ai_clients c ON c.id = s.client_row_id
`

// FindSession returns the connection connectionID, or nil.
func (r *Repo) FindSession(ctx context.Context, tx *gorm.DB, connectionID string) (*Session, error) {
	var found []Session
	if err := tx.WithContext(ctx).Raw(selectSession+`WHERE s.connection_id = ?`, connectionID).Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("finding AI session: %s", err)
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}

// RevokeSession revokes a connection not yet revoked; false when it was.
func (r *Repo) RevokeSession(ctx context.Context, tx *gorm.DB, id int64, revokedAt time.Time, reason string) (bool, error) {
	result := tx.WithContext(ctx).Exec(`UPDATE identity_ai_sessions SET revoked_at = ?, revoke_reason = ? WHERE id = ? AND revoked_at IS NULL`, revokedAt, reason, id)
	if result.Error != nil {
		return false, fmt.Errorf("revoking AI session: %s", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// Connection is a live connection as its User lists it.
type Connection struct {
	ConnectionID string
	ClientName   string
	CreatedAt    time.Time
	LastUsedAt   time.Time
}

// ListLiveSessions returns userUUID's connections not revoked, the most
// recent first, with when each was last granted tokens.
func (r *Repo) ListLiveSessions(ctx context.Context, userUUID string) ([]Connection, error) {
	var found []Connection
	if err := r.db.WithContext(ctx).Raw(`
		SELECT s.connection_id, c.client_name, s.created_at,
			COALESCE((SELECT MAX(t.created_at) FROM identity_ai_refresh_tokens t WHERE t.session_row_id = s.id), s.created_at) AS last_used_at
		FROM identity_ai_sessions s
		JOIN identity_users u ON u.id = s.user_id
		JOIN identity_ai_clients c ON c.id = s.client_row_id
		WHERE u.user_uuid = ? AND s.revoked_at IS NULL
		ORDER BY s.created_at DESC, s.id DESC
	`, userUUID).Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("listing AI sessions: %s", err)
	}
	return found, nil
}

// RefreshToken is a stored refresh token with its connection.
type RefreshToken struct {
	ID           int64
	CreatedAt    time.Time
	ExpiresAt    time.Time
	UsedAt       *time.Time
	SessionID    int64
	ConnectionID string
	UserUUID     string
	ClientID     string
	Scope        string
	Resource     string
	RevokedAt    *time.Time
}

type refreshRow struct {
	ID           int64
	SessionRowID int64
	TokenSHA256  string `gorm:"column:token_sha256"`
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

func (refreshRow) TableName() string { return "identity_ai_refresh_tokens" }

// CreateRefreshToken stores a refresh token's hash for a connection.
func (r *Repo) CreateRefreshToken(ctx context.Context, tx *gorm.DB, sessionRowID int64, tokenSHA256 string, createdAt time.Time, expiresAt time.Time) error {
	row := refreshRow{SessionRowID: sessionRowID, TokenSHA256: tokenSHA256, CreatedAt: createdAt, ExpiresAt: expiresAt}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating AI refresh token: %s", err)
	}
	return nil
}

// FindRefreshToken returns the refresh token hashing to tokenSHA256 with
// its connection, locked until tx ends, or nil.
func (r *Repo) FindRefreshToken(ctx context.Context, tx *gorm.DB, tokenSHA256 string) (*RefreshToken, error) {
	var found []RefreshToken
	if err := tx.WithContext(ctx).Raw(`
		SELECT t.id, t.created_at, t.expires_at, t.used_at,
			s.id AS session_id, s.connection_id, u.user_uuid, c.client_id, s.scope, s.resource, s.revoked_at
		FROM identity_ai_refresh_tokens t
		JOIN identity_ai_sessions s ON s.id = t.session_row_id
		JOIN identity_users u ON u.id = s.user_id
		JOIN identity_ai_clients c ON c.id = s.client_row_id
		WHERE t.token_sha256 = ?
		FOR UPDATE OF t, s
	`, tokenSHA256).Scan(&found).Error; err != nil {
		return nil, fmt.Errorf("finding AI refresh token: %s", err)
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}

// UseRefreshToken records that a refresh token was exchanged; false when it
// already was.
func (r *Repo) UseRefreshToken(ctx context.Context, tx *gorm.DB, id int64, usedAt time.Time) (bool, error) {
	result := tx.WithContext(ctx).Exec(`UPDATE identity_ai_refresh_tokens SET used_at = ? WHERE id = ? AND used_at IS NULL`, usedAt, id)
	if result.Error != nil {
		return false, fmt.Errorf("using AI refresh token: %s", result.Error)
	}
	return result.RowsAffected > 0, nil
}
