// Package ratelimit provides rate limiting functionality for authentication endpoints
package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Storage defines the interface for rate limit storage backends
type Storage interface {
	// GetRecord retrieves an attempt record for the given key
	GetRecord(ctx context.Context, key string) (*AttemptRecord, error)

	// SetRecord stores an attempt record for the given key
	SetRecord(ctx context.Context, key string, record *AttemptRecord) error

	// DeleteRecord removes an attempt record for the given key
	DeleteRecord(ctx context.Context, key string) error

	// CleanupExpired removes all expired records
	CleanupExpired(ctx context.Context, window time.Duration) error

	// GetStats returns storage statistics
	GetStats(ctx context.Context) (map[string]interface{}, error)

	// Close closes the storage connection
	Close() error
}

// MemoryStorage implements in-memory storage for rate limiting
type MemoryStorage struct {
	attempts map[string]*AttemptRecord
	mutex    sync.RWMutex
}

// NewMemoryStorage creates a new in-memory storage backend
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		attempts: make(map[string]*AttemptRecord),
	}
}

// GetRecord retrieves an attempt record for the given key
func (ms *MemoryStorage) GetRecord(ctx context.Context, key string) (*AttemptRecord, error) {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	record, exists := ms.attempts[key]
	if !exists {
		return nil, nil
	}

	// Return a copy to prevent external modification
	recordCopy := *record
	if record.BlockedAt != nil {
		blockedAtCopy := *record.BlockedAt
		recordCopy.BlockedAt = &blockedAtCopy
	}

	return &recordCopy, nil
}

// SetRecord stores an attempt record for the given key
func (ms *MemoryStorage) SetRecord(ctx context.Context, key string, record *AttemptRecord) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	// Store a copy to prevent external modification
	recordCopy := *record
	if record.BlockedAt != nil {
		blockedAtCopy := *record.BlockedAt
		recordCopy.BlockedAt = &blockedAtCopy
	}

	ms.attempts[key] = &recordCopy
	return nil
}

// DeleteRecord removes an attempt record for the given key
func (ms *MemoryStorage) DeleteRecord(ctx context.Context, key string) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	delete(ms.attempts, key)
	return nil
}

// CleanupExpired removes all expired records
func (ms *MemoryStorage) CleanupExpired(ctx context.Context, window time.Duration) error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	now := time.Now().UTC()
	for key, record := range ms.attempts {
		// Remove records that are older than the window and not blocked
		if record.BlockedAt == nil && now.Sub(record.FirstSeen) > window {
			delete(ms.attempts, key)
		}
		// Remove blocked records that have expired their block time
		if record.BlockedAt != nil && now.Sub(*record.BlockedAt) > window {
			delete(ms.attempts, key)
		}
	}

	return nil
}

// GetStats returns storage statistics
func (ms *MemoryStorage) GetStats(ctx context.Context) (map[string]interface{}, error) {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	stats := map[string]interface{}{
		"type":          "memory",
		"total_records": len(ms.attempts),
		"blocked_count": 0,
		"active_count":  0,
	}

	now := time.Now().UTC()
	blockedCount := 0
	activeCount := 0

	for _, record := range ms.attempts {
		if record.BlockedAt != nil {
			blockedCount++
		} else {
			activeCount++
		}
	}

	stats["blocked_count"] = blockedCount
	stats["active_count"] = activeCount
	stats["timestamp"] = now

	return stats, nil
}

// Close closes the storage connection (no-op for memory storage)
func (ms *MemoryStorage) Close() error {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	// Clear all records
	ms.attempts = make(map[string]*AttemptRecord)
	return nil
}
