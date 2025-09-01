// Package session provides secure session management with HttpOnly cookies
package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Session management errors
var (
	ErrSessionNotFound    = errors.New("session not found")
	ErrSessionExpired     = errors.New("session has expired")
	ErrInvalidSessionID   = errors.New("invalid session ID")
	ErrSessionAlreadyUsed = errors.New("session has already been used")
)

// SessionData represents the data stored in a session
type SessionData struct {
	UserID       int64                  `json:"user_id"`
	Role         string                 `json:"role"`
	Email        string                 `json:"email"`
	AccessToken  string                 `json:"access_token"`
	RefreshToken string                 `json:"refresh_token"`
	CreatedAt    time.Time              `json:"created_at"`
	ExpiresAt    time.Time              `json:"expires_at"`
	LastUsedAt   time.Time              `json:"last_used_at"`
	IPAddress    string                 `json:"ip_address,omitempty"`
	UserAgent    string                 `json:"user_agent,omitempty"`
	IPChanges    int                    `json:"ip_changes,omitempty"`     // Number of IP changes
	LastIPChange time.Time              `json:"last_ip_change,omitempty"` // Time of last IP change
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// SessionConfig defines the configuration for session management
type SessionConfig struct {
	CookieName      string        `json:"cookie_name"`
	Domain          string        `json:"domain,omitempty"`
	Path            string        `json:"path"`
	MaxAge          time.Duration `json:"max_age"`
	Secure          bool          `json:"secure"`
	HttpOnly        bool          `json:"http_only"`
	SameSite        http.SameSite `json:"same_site"`
	CleanupInterval time.Duration `json:"cleanup_interval"`
	StrictIPBinding bool          `json:"strict_ip_binding"` // Require IP to match
	StrictUABinding bool          `json:"strict_ua_binding"` // Require UserAgent to match
	AllowIPChange   bool          `json:"allow_ip_change"`   // Allow IP changes (for mobile users)
	MaxIPChanges    int           `json:"max_ip_changes"`    // Max IP changes allowed per session
}

// SessionManager handles session operations
type SessionManager struct {
	config         SessionConfig
	storage        Storage
	ticker         *time.Ticker
	stopCh         chan struct{}
	cleanupEnabled bool
}

// NewSessionManager creates a new session manager with the given configuration using memory storage
func NewSessionManager(config SessionConfig) *SessionManager {
	return NewSessionManagerWithStorage(config, NewMemoryStorage())
}

// NewSessionManagerWithStorage creates a new session manager with custom storage backend
func NewSessionManagerWithStorage(config SessionConfig, storage Storage) *SessionManager {
	sm := &SessionManager{
		config:         config,
		storage:        storage,
		stopCh:         make(chan struct{}),
		cleanupEnabled: false,
	}

	// Start cleanup goroutine if cleanup interval is set
	if config.CleanupInterval > 0 {
		sm.enableAutoCleanup(config.CleanupInterval)
	}

	return sm
}

// enableAutoCleanup starts automatic cleanup of expired sessions
func (sm *SessionManager) enableAutoCleanup(interval time.Duration) {
	if sm.cleanupEnabled {
		return
	}

	sm.cleanupEnabled = true
	sm.ticker = time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-sm.ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				_, _ = sm.storage.CleanupExpired(ctx)
				cancel()
			case <-sm.stopCh:
				return
			}
		}
	}()
}

// DisableAutoCleanup stops automatic cleanup
func (sm *SessionManager) DisableAutoCleanup() {
	if !sm.cleanupEnabled {
		return
	}

	sm.cleanupEnabled = false
	if sm.ticker != nil {
		sm.ticker.Stop()
	}
	close(sm.stopCh)
	sm.stopCh = make(chan struct{})
}

// Close closes the session manager and cleans up resources
func (sm *SessionManager) Close() error {
	// Stop auto cleanup if enabled
	sm.DisableAutoCleanup()

	// Close storage
	if sm.storage != nil {
		return sm.storage.Close()
	}

	return nil
}

// GenerateSessionID generates a cryptographically secure session ID
func GenerateSessionID() (string, error) {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// CreateSession creates a new session for the user
func (sm *SessionManager) CreateSession(userID int64, role, email, accessToken, refreshToken, ipAddress, userAgent string) (string, *SessionData, error) {
	sessionID, err := GenerateSessionID()
	if err != nil {
		return "", nil, err
	}

	now := time.Now().UTC()
	sessionData := &SessionData{
		UserID:       userID,
		Role:         role,
		Email:        email,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		CreatedAt:    now,
		ExpiresAt:    now.Add(sm.config.MaxAge),
		LastUsedAt:   now,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		Metadata:     make(map[string]interface{}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = sm.storage.SetSession(ctx, sessionID, sessionData)
	if err != nil {
		return "", nil, err
	}

	return sessionID, sessionData, nil
}

// GetSession retrieves a session by ID
func (sm *SessionManager) GetSession(sessionID string) (*SessionData, error) {
	if sessionID == "" {
		return nil, ErrInvalidSessionID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sessionData, err := sm.storage.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Update last used time
	sessionData.LastUsedAt = time.Now().UTC()
	err = sm.storage.SetSession(ctx, sessionID, sessionData)
	if err != nil {
		// Log error but don't fail the request
		// In production, you might want to handle this differently
	}

	// Storage already returns a copy
	return sessionData, nil
}

// GetSessionWithValidation retrieves a session by ID and validates IP/UserAgent if configured
func (sm *SessionManager) GetSessionWithValidation(sessionID, clientIP, userAgent string) (*SessionData, error) {
	sessionData, err := sm.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// Validate IP binding if enabled
	if sm.config.StrictIPBinding && sessionData.IPAddress != "" && sessionData.IPAddress != clientIP {
		if !sm.config.AllowIPChange {
			return nil, errors.New("session IP mismatch - authentication required")
		}

		// Check if IP changes are within allowed limit
		if sessionData.IPChanges >= sm.config.MaxIPChanges {
			return nil, errors.New("too many IP changes - authentication required")
		}

		// Update IP and increment change counter
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		sessionData.IPAddress = clientIP
		sessionData.IPChanges++
		sessionData.LastIPChange = time.Now().UTC()

		err = sm.storage.SetSession(ctx, sessionID, sessionData)
		if err != nil {
			return nil, err
		}
	}

	// Validate UserAgent binding if enabled
	if sm.config.StrictUABinding && sessionData.UserAgent != "" && sessionData.UserAgent != userAgent {
		return nil, errors.New("session user agent mismatch - authentication required")
	}

	return sessionData, nil
}

// UpdateSession updates session data
func (sm *SessionManager) UpdateSession(sessionID string, updates map[string]interface{}) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sessionData, err := sm.storage.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	// Update allowed fields
	if accessToken, ok := updates["access_token"].(string); ok {
		sessionData.AccessToken = accessToken
	}
	if refreshToken, ok := updates["refresh_token"].(string); ok {
		sessionData.RefreshToken = refreshToken
	}
	if role, ok := updates["role"].(string); ok {
		sessionData.Role = role
	}
	if email, ok := updates["email"].(string); ok {
		sessionData.Email = email
	}

	// Update metadata
	if metadata, ok := updates["metadata"].(map[string]interface{}); ok {
		if sessionData.Metadata == nil {
			sessionData.Metadata = make(map[string]interface{})
		}
		for k, v := range metadata {
			sessionData.Metadata[k] = v
		}
	}

	// Update last used time
	sessionData.LastUsedAt = time.Now().UTC()

	return sm.storage.SetSession(ctx, sessionID, sessionData)
}

// DeleteSession removes a session
func (sm *SessionManager) DeleteSession(sessionID string) bool {
	if sessionID == "" {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := sm.storage.DeleteSession(ctx, sessionID)
	// Return true only if session was actually deleted (not if it didn't exist)
	return err == nil
}

// DeleteUserSessions removes all sessions for a specific user
func (sm *SessionManager) DeleteUserSessions(userID int64) int {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deleted, err := sm.storage.DeleteUserSessions(ctx, userID)
	if err != nil {
		return 0
	}
	return deleted
}

// ExtendSession extends the expiration time of a session
func (sm *SessionManager) ExtendSession(sessionID string, duration time.Duration) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sessionData, err := sm.storage.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	sessionData.ExpiresAt = sessionData.ExpiresAt.Add(duration)
	sessionData.LastUsedAt = time.Now().UTC()

	return sm.storage.SetSession(ctx, sessionID, sessionData)
}

// SetSessionCookie sets the session cookie in the HTTP response
func (sm *SessionManager) SetSessionCookie(w http.ResponseWriter, sessionID string) {
	cookie := &http.Cookie{
		Name:     sm.config.CookieName,
		Value:    sessionID,
		Domain:   sm.config.Domain,
		Path:     sm.config.Path,
		MaxAge:   int(sm.config.MaxAge.Seconds()),
		Secure:   sm.config.Secure,
		HttpOnly: sm.config.HttpOnly,
		SameSite: sm.config.SameSite,
	}
	http.SetCookie(w, cookie)
}

// ClearSessionCookie clears the session cookie from the HTTP response
func (sm *SessionManager) ClearSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     sm.config.CookieName,
		Value:    "",
		Domain:   sm.config.Domain,
		Path:     sm.config.Path,
		MaxAge:   -1,
		Secure:   sm.config.Secure,
		HttpOnly: sm.config.HttpOnly,
		SameSite: sm.config.SameSite,
	}
	http.SetCookie(w, cookie)
}

// GetSessionFromRequest extracts the session ID from the HTTP request
func (sm *SessionManager) GetSessionFromRequest(r *http.Request) (string, error) {
	cookie, err := r.Cookie(sm.config.CookieName)
	if err != nil {
		if err == http.ErrNoCookie {
			return "", ErrSessionNotFound
		}
		return "", fmt.Errorf("failed to get session cookie: %w", err)
	}
	return cookie.Value, nil
}

// GetUserSessions returns all active sessions for a user
// Note: This is a simplified implementation for memory storage
// For distributed storage, you would need to implement a more efficient query mechanism
func (sm *SessionManager) GetUserSessions(userID int64) []*SessionData {
	// For memory storage, we can access the underlying storage
	if memStorage, ok := sm.storage.(*MemoryStorage); ok {
		memStorage.mutex.RLock()
		defer memStorage.mutex.RUnlock()

		var userSessions []*SessionData
		now := time.Now().UTC()

		for _, sessionData := range memStorage.sessions {
			if sessionData.UserID == userID && now.Before(sessionData.ExpiresAt) {
				// Return a copy to prevent external modification
				sessionCopy := *sessionData
				if sessionData.Metadata != nil {
					sessionCopy.Metadata = make(map[string]interface{})
					for k, v := range sessionData.Metadata {
						sessionCopy.Metadata[k] = v
					}
				}
				userSessions = append(userSessions, &sessionCopy)
			}
		}

		return userSessions
	}

	// For other storage types, this would need to be implemented differently
	// For now, return empty slice
	return []*SessionData{}
}

// CleanupExpiredSessions removes expired sessions
func (sm *SessionManager) CleanupExpiredSessions() int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cleaned, err := sm.storage.CleanupExpired(ctx)
	if err != nil {
		return 0
	}

	return cleaned
}

// GetStats returns statistics about the session manager
func (sm *SessionManager) GetStats() map[string]interface{} {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	storageStats, err := sm.storage.GetStats(ctx)
	if err != nil {
		// Return basic config stats on error
		return map[string]interface{}{
			"cookie_name":     sm.config.CookieName,
			"max_age":         sm.config.MaxAge.String(),
			"secure":          sm.config.Secure,
			"http_only":       sm.config.HttpOnly,
			"same_site":       int(sm.config.SameSite),
			"cleanup_enabled": sm.cleanupEnabled,
			"error":           err.Error(),
		}
	}

	// Add configuration to storage stats
	storageStats["cookie_name"] = sm.config.CookieName
	storageStats["max_age"] = sm.config.MaxAge.String()
	storageStats["secure"] = sm.config.Secure
	storageStats["http_only"] = sm.config.HttpOnly
	storageStats["same_site"] = int(sm.config.SameSite)
	storageStats["cleanup_enabled"] = sm.cleanupEnabled

	return storageStats
}

// Predefined session configurations
var (
	// DefaultSessionConfig provides a secure default configuration
	DefaultSessionConfig = SessionConfig{
		CookieName:      "session_id",
		Path:            "/",
		MaxAge:          24 * time.Hour, // 24 hours
		Secure:          true,           // Should be true in production with HTTPS
		HttpOnly:        true,
		SameSite:        http.SameSiteLaxMode,
		CleanupInterval: time.Hour, // Cleanup every hour
		StrictIPBinding: false,     // Disabled by default for better UX
		StrictUABinding: false,     // Disabled by default for better UX
		AllowIPChange:   true,      // Allow IP changes for mobile users
		MaxIPChanges:    5,         // Allow up to 5 IP changes per session
	}

	// DevelopmentSessionConfig provides a configuration suitable for development
	DevelopmentSessionConfig = SessionConfig{
		CookieName:      "dev_session_id",
		Path:            "/",
		MaxAge:          8 * time.Hour, // 8 hours
		Secure:          false,         // Can be false in development (HTTP)
		HttpOnly:        true,
		SameSite:        http.SameSiteLaxMode,
		CleanupInterval: 30 * time.Minute, // Cleanup every 30 minutes
		StrictIPBinding: false,            // Disabled for development
		StrictUABinding: false,            // Disabled for development
		AllowIPChange:   true,             // Allow IP changes
		MaxIPChanges:    10,               // More lenient for development
	}

	// ProductionSessionConfig provides a secure configuration for production
	ProductionSessionConfig = SessionConfig{
		CookieName:      "loft_session",
		Path:            "/",
		MaxAge:          60 * 24 * time.Hour, // 60 days (same as refresh token)
		Secure:          true,                // Must be true in production
		HttpOnly:        true,
		SameSite:        http.SameSiteLaxMode,
		CleanupInterval: 2 * time.Hour, // Cleanup every 2 hours
		StrictIPBinding: true,          // Enable for production security
		StrictUABinding: false,         // Keep disabled for better UX
		AllowIPChange:   true,          // Allow but limit IP changes
		MaxIPChanges:    3,             // Stricter limit for production
	}
)
