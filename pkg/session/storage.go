// Package session provides secure session management with HttpOnly cookies
package session

import (
	"context"
	"sync"
	"time"
)

// Storage defines the interface for session storage backends
type Storage interface {
	// GetSession retrieves a session by ID
	GetSession(ctx context.Context, sessionID string) (*SessionData, error)

	// SetSession stores a session
	SetSession(ctx context.Context, sessionID string, data *SessionData) error

	// DeleteSession removes a session
	DeleteSession(ctx context.Context, sessionID string) error

	// DeleteUserSessions removes all sessions for a user
	DeleteUserSessions(ctx context.Context, userID int64) (int, error)

	// CleanupExpired removes expired sessions
	CleanupExpired(ctx context.Context) (int, error)

	// GetStats returns storage statistics
	GetStats(ctx context.Context) (map[string]interface{}, error)

	// Close closes the storage connection
	Close() error
}

// MemoryStorage implements in-memory storage for sessions
type MemoryStorage struct {
	sessions map[string]*SessionData
	mutex    sync.RWMutex
}

// NewMemoryStorage creates a new in-memory storage backend
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		sessions: make(map[string]*SessionData),
	}
}

// GetSession retrieves a session by ID
func (ms *MemoryStorage) GetSession(ctx context.Context, sessionID string) (*SessionData, error) {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	session, exists := ms.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	// Check if session has expired
	if time.Now().UTC().After(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	// Return a copy to prevent external modification
	sessionCopy := *session
	if session.Metadata != nil {
		sessionCopy.Metadata = make(map[string]interface{})
		for k, v := range session.Metadata {
			sessionCopy.Metadata[k] = v
		}
	}

	return &sessionCopy, nil
}

// SetSession stores a session
func (ms *MemoryStorage) SetSession(ctx context.Context, sessionID string, data *SessionData) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	// Store a copy to prevent external modification
	sessionCopy := *data
	if data.Metadata != nil {
		sessionCopy.Metadata = make(map[string]interface{})
		for k, v := range data.Metadata {
			sessionCopy.Metadata[k] = v
		}
	}

	ms.sessions[sessionID] = &sessionCopy
	return nil
}

// DeleteSession removes a session
func (ms *MemoryStorage) DeleteSession(ctx context.Context, sessionID string) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	if _, exists := ms.sessions[sessionID]; !exists {
		return ErrSessionNotFound
	}

	delete(ms.sessions, sessionID)
	return nil
}

// DeleteUserSessions removes all sessions for a user
func (ms *MemoryStorage) DeleteUserSessions(ctx context.Context, userID int64) (int, error) {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	deleted := 0
	for sessionID, session := range ms.sessions {
		if session.UserID == userID {
			delete(ms.sessions, sessionID)
			deleted++
		}
	}

	return deleted, nil
}

// CleanupExpired removes expired sessions
func (ms *MemoryStorage) CleanupExpired(ctx context.Context) (int, error) {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	now := time.Now().UTC()
	cleaned := 0

	for sessionID, session := range ms.sessions {
		if now.After(session.ExpiresAt) {
			delete(ms.sessions, sessionID)
			cleaned++
		}
	}

	return cleaned, nil
}

// GetStats returns storage statistics
func (ms *MemoryStorage) GetStats(ctx context.Context) (map[string]interface{}, error) {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	now := time.Now().UTC()
	activeCount := 0
	expiredCount := 0

	for _, session := range ms.sessions {
		if now.After(session.ExpiresAt) {
			expiredCount++
		} else {
			activeCount++
		}
	}

	stats := map[string]interface{}{
		"type":           "memory",
		"total_sessions": len(ms.sessions),
		"active_count":   activeCount,
		"expired_count":  expiredCount,
		"timestamp":      now,
	}

	return stats, nil
}

// Close closes the storage connection (no-op for memory storage)
func (ms *MemoryStorage) Close() error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	// Clear all sessions
	ms.sessions = make(map[string]*SessionData)
	return nil
}
