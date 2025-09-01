// Package audit provides helper functions to write audit logs to the database
package audit

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"strconv"

	"encore.app/pkg/httpx"
	"encore.dev/beta/auth"
	"encore.dev/storage/sqldb"
)

var secrets struct {
	AuditEncryptionKey string //encore:secret
}

// encryptSensitiveData encrypts sensitive data using AES-GCM
func encryptSensitiveData(plaintext string) (string, error) {
	if secrets.AuditEncryptionKey == "" {
		// In test/dev mode without encryption key, hash instead
		hash := sha256.Sum256([]byte(plaintext))
		return "hashed:" + base64.URLEncoding.EncodeToString(hash[:8]), nil
	}
	
	// Create cipher
	key := sha256.Sum256([]byte(secrets.AuditEncryptionKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	
	// Generate nonce
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	
	// Encrypt
	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

// Entry represents an audit log entry to be written to audit_logs table
// Fields with pointer types are optional; nil values will be stored as NULL.
type Entry struct {
	ActorUserID   *int64      // user id performing the action
	Action        string      // action name, e.g. "system_settings.update"
	EntityType    string      // e.g. "system_settings"
	EntityID      string      // e.g. setting key
	Reason        *string     // optional reason
	Meta          interface{} // arbitrary JSON-serializable metadata
	IPAddress     *string     // optional client IP
	UserAgent     *string     // optional user agent
	CorrelationID *string     // optional correlation/request id
}

// Log writes an audit log entry. It JSON-encodes Meta and stores it as JSONB.
// Returns the inserted audit log id.
func Log(ctx context.Context, db *sqldb.Database, e Entry) (int64, error) {
	// Prepare JSONB meta
	var metaJSON []byte
	if e.Meta == nil {
		metaJSON = []byte("{}")
	} else if b, ok := e.Meta.([]byte); ok {
		metaJSON = b
	} else {
		b, err := json.Marshal(e.Meta)
		if err != nil {
			// fallback to empty object to avoid failing the write
			metaJSON = []byte("{}")
		} else {
			metaJSON = b
		}
	}

	// Convert optional fields to interfaces to allow NULL
	var (
		actor  interface{}
		reason interface{}
		corr   interface{}
		ip     interface{}
		ua     interface{}
	)
	if e.ActorUserID != nil {
		actor = *e.ActorUserID
	}
	if e.Reason != nil {
		reason = *e.Reason
	}
	if e.CorrelationID != nil {
		corr = *e.CorrelationID
	}

	// Auto-fill IP/User-Agent from context if not provided and encrypt sensitive data
	if e.IPAddress != nil && *e.IPAddress != "" {
		if encrypted, err := encryptSensitiveData(*e.IPAddress); err == nil {
			ip = encrypted
		} else {
			ip = "encrypt_failed"
		}
	} else if cip := httpx.GetClientIPFromContext(ctx); cip != "" && cip != "unknown" {
		if encrypted, err := encryptSensitiveData(cip); err == nil {
			ip = encrypted
		} else {
			ip = "encrypt_failed"
		}
	}
	if e.UserAgent != nil && *e.UserAgent != "" {
		if encrypted, err := encryptSensitiveData(*e.UserAgent); err == nil {
			ua = encrypted
		} else {
			ua = "encrypt_failed"
		}
	} else if u := httpx.GetUserAgentFromContext(ctx); u != "" && u != "Unknown-Client/1.0" {
		if encrypted, err := encryptSensitiveData(u); err == nil {
			ua = encrypted
		} else {
			ua = "encrypt_failed"
		}
	}

	// Insert directly into audit_logs to allow future inclusion of ip_address/user_agent if needed
	var id int64
	err := db.Stdlib().QueryRowContext(ctx, `
		INSERT INTO audit_logs (actor_user_id, action, entity_type, entity_id, reason, meta, ip_address, user_agent, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9)
		RETURNING id
	`, actor, e.Action, e.EntityType, e.EntityID, reason, string(metaJSON), ip, ua, corr).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// Option configures an audit Entry before logging
type Option func(*Entry)

// WithActor sets the actor user id
func WithActor(userID int64) Option { return func(e *Entry) { e.ActorUserID = &userID } }

// WithReason sets the audit reason
func WithReason(reason string) Option { return func(e *Entry) { e.Reason = &reason } }

// WithCorrelation sets the correlation/request id
func WithCorrelation(corr string) Option { return func(e *Entry) { e.CorrelationID = &corr } }

// InferActorFromAuth tries to read the current authenticated user and set ActorUserID
func InferActorFromAuth() Option {
	return func(e *Entry) {
		if uidStr, ok := auth.UserID(); ok {
			if v, err := strconv.ParseInt(string(uidStr), 10, 64); err == nil {
				e.ActorUserID = &v
			}
		}
	}
}

// LogAction is a convenience around Log to simplify common auditing calls
func LogAction(ctx context.Context, db *sqldb.Database, action, entityType, entityID string, meta interface{}, opts ...Option) (int64, error) {
	entry := Entry{
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Meta:       meta,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&entry)
		}
	}
	return Log(ctx, db, entry)
}
