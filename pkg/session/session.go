// Package session provides secure session management with HttpOnly cookies
package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"sync"
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
	RefreshToken string                 `json:"refresh_token"`
	CreatedAt    time.Time              `json:"created_at"`
	ExpiresAt    time.Time              `json:"expires_at"`
	LastUsedAt   time.Time              `json:"last_used_at"`
	IPAddress    string                 `json:"ip_address,omitempty"`
	UserAgent    string                 `json:"user_agent,omitempty"`
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
}

// SessionManager handles session operations
type SessionManager struct {
	config   SessionConfig
	sessions map[string]*SessionData
	mutex    sync.RWMutex
}

// NewSessionManager creates a new session manager with the given configuration
func NewSessionManager(config SessionConfig) *SessionManager {
	sm := &SessionManager{
		config:   config,
		sessions: make(map[string]*SessionData),
	}

	// Start cleanup goroutine if cleanup interval is set
	if config.CleanupInterval > 0 {
		go sm.startCleanupRoutine()
	}

	return sm
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
func (sm *SessionManager) CreateSession(userID int64, role, email, refreshToken, ipAddress, userAgent string) (string, *SessionData, error) {
	sessionID, err := GenerateSessionID()
	if err != nil {
		return "", nil, err
	}

	now := time.Now()
	sessionData := &SessionData{
		UserID:       userID,
		Role:         role,
		Email:        email,
		RefreshToken: refreshToken,
		CreatedAt:    now,
		ExpiresAt:    now.Add(sm.config.MaxAge),
		LastUsedAt:   now,
		IPAddress:    ipAddress,
		UserAgent:    userAgent,
		Metadata:     make(map[string]interface{}),
	}

	sm.mutex.Lock()
	sm.sessions[sessionID] = sessionData
	sm.mutex.Unlock()

	return sessionID, sessionData, nil
}

// GetSession retrieves a session by ID
func (sm *SessionManager) GetSession(sessionID string) (*SessionData, error) {
	if sessionID == "" {
		return nil, ErrInvalidSessionID
	}

	sm.mutex.RLock()
	sessionData, exists := sm.sessions[sessionID]
	sm.mutex.RUnlock()

	if !exists {
		return nil, ErrSessionNotFound
	}

	// Check if session has expired
	if time.Now().After(sessionData.ExpiresAt) {
		sm.DeleteSession(sessionID)
		return nil, ErrSessionExpired
	}

	// Update last used time
	sm.mutex.Lock()
	sessionData.LastUsedAt = time.Now()
	sm.mutex.Unlock()

	// Return a copy to prevent external modification
	sessionCopy := *sessionData
	if sessionData.Metadata != nil {
		sessionCopy.Metadata = make(map[string]interface{})
		for k, v := range sessionData.Metadata {
			sessionCopy.Metadata[k] = v
		}
	}

	return &sessionCopy, nil
}

// UpdateSession updates session data
func (sm *SessionManager) UpdateSession(sessionID string, updates map[string]interface{}) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sessionData, exists := sm.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	// Check if session has expired
	if time.Now().After(sessionData.ExpiresAt) {
		delete(sm.sessions, sessionID)
		return ErrSessionExpired
	}

	// Update allowed fields
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

	sessionData.LastUsedAt = time.Now()
	return nil
}

// DeleteSession removes a session
func (sm *SessionManager) DeleteSession(sessionID string) bool {
	if sessionID == "" {
		return false
	}

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if _, exists := sm.sessions[sessionID]; exists {
		delete(sm.sessions, sessionID)
		return true
	}
	return false
}

// DeleteUserSessions removes all sessions for a specific user
func (sm *SessionManager) DeleteUserSessions(userID int64) int {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	deleted := 0
	for sessionID, sessionData := range sm.sessions {
		if sessionData.UserID == userID {
			delete(sm.sessions, sessionID)
			deleted++
		}
	}
	return deleted
}

// ExtendSession extends the expiration time of a session
func (sm *SessionManager) ExtendSession(sessionID string, duration time.Duration) error {
	if sessionID == "" {
		return ErrInvalidSessionID
	}

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sessionData, exists := sm.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	// Check if session has expired
	if time.Now().After(sessionData.ExpiresAt) {
		delete(sm.sessions, sessionID)
		return ErrSessionExpired
	}

	sessionData.ExpiresAt = sessionData.ExpiresAt.Add(duration)
	sessionData.LastUsedAt = time.Now()
	return nil
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
func (sm *SessionManager) GetUserSessions(userID int64) []*SessionData {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	var userSessions []*SessionData
	now := time.Now()

	for _, sessionData := range sm.sessions {
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

// CleanupExpiredSessions removes expired sessions
func (sm *SessionManager) CleanupExpiredSessions() int {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	now := time.Now()
	cleaned := 0

	for sessionID, sessionData := range sm.sessions {
		if now.After(sessionData.ExpiresAt) {
			delete(sm.sessions, sessionID)
			cleaned++
		}
	}

	return cleaned
}

// GetStats returns statistics about the session manager
func (sm *SessionManager) GetStats() map[string]interface{} {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	now := time.Now()
	activeSessions := 0
	expiredSessions := 0
	userCounts := make(map[int64]int)

	for _, sessionData := range sm.sessions {
		if now.Before(sessionData.ExpiresAt) {
			activeSessions++
			userCounts[sessionData.UserID]++
		} else {
			expiredSessions++
		}
	}

	return map[string]interface{}{
		"total_sessions":   len(sm.sessions),
		"active_sessions":  activeSessions,
		"expired_sessions": expiredSessions,
		"unique_users":     len(userCounts),
		"cookie_name":      sm.config.CookieName,
		"max_age":          sm.config.MaxAge.String(),
		"secure":           sm.config.Secure,
		"http_only":        sm.config.HttpOnly,
		"same_site":        int(sm.config.SameSite),
	}
}

// startCleanupRoutine starts a background routine to clean up expired sessions
func (sm *SessionManager) startCleanupRoutine() {
	ticker := time.NewTicker(sm.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		sm.CleanupExpiredSessions()
	}
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
	}
)
