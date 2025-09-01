package session

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGenerateSessionID(t *testing.T) {
	id1, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID failed: %v", err)
	}

	if len(id1) != 64 { // 32 bytes = 64 hex characters
		t.Errorf("Session ID length should be 64, got %d", len(id1))
	}

	// Generate another ID to ensure uniqueness
	id2, err := GenerateSessionID()
	if err != nil {
		t.Fatalf("GenerateSessionID failed: %v", err)
	}

	if id1 == id2 {
		t.Error("Session IDs should be unique")
	}
}

func TestNewSessionManager(t *testing.T) {
	config := DefaultSessionConfig
	sm := NewSessionManager(config)

	if sm == nil {
		t.Fatal("NewSessionManager returned nil")
	}

	if sm.config.CookieName != config.CookieName {
		t.Errorf("Cookie name mismatch: got %s, want %s", sm.config.CookieName, config.CookieName)
	}

	if sm.storage == nil {
		t.Error("Storage not initialized")
	}
}

func TestCreateSession(t *testing.T) {
	config := DefaultSessionConfig
	config.CleanupInterval = 0 // Disable cleanup for testing
	sm := NewSessionManager(config)

	userID := int64(123)
	role := "verified"
	email := "test@example.com"
	refreshToken := "refresh_token_123"
	ipAddress := "192.168.1.1"
	userAgent := "Test Agent"

	sessionID, sessionData, err := sm.CreateSession(userID, role, email, "access_token_123", refreshToken, ipAddress, userAgent)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if sessionID == "" {
		t.Error("Session ID should not be empty")
	}

	if sessionData == nil {
		t.Fatal("Session data should not be nil")
	}

	if sessionData.UserID != userID {
		t.Errorf("User ID mismatch: got %d, want %d", sessionData.UserID, userID)
	}

	if sessionData.Role != role {
		t.Errorf("Role mismatch: got %s, want %s", sessionData.Role, role)
	}

	if sessionData.Email != email {
		t.Errorf("Email mismatch: got %s, want %s", sessionData.Email, email)
	}

	if sessionData.RefreshToken != refreshToken {
		t.Errorf("Refresh token mismatch: got %s, want %s", sessionData.RefreshToken, refreshToken)
	}

	if sessionData.IPAddress != ipAddress {
		t.Errorf("IP address mismatch: got %s, want %s", sessionData.IPAddress, ipAddress)
	}

	if sessionData.UserAgent != userAgent {
		t.Errorf("User agent mismatch: got %s, want %s", sessionData.UserAgent, userAgent)
	}

	if sessionData.ExpiresAt.Before(time.Now()) {
		t.Error("Session should not be expired immediately after creation")
	}
}

func TestGetSession(t *testing.T) {
	config := DefaultSessionConfig
	config.CleanupInterval = 0
	sm := NewSessionManager(config)

	userID := int64(123)
	role := "verified"
	email := "test@example.com"
	refreshToken := "refresh_token_123"

	// Create session
	sessionID, _, err := sm.CreateSession(userID, role, email, "access_token", refreshToken, "", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Get session
	sessionData, err := sm.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if sessionData.UserID != userID {
		t.Errorf("User ID mismatch: got %d, want %d", sessionData.UserID, userID)
	}

	// Test non-existent session
	_, err = sm.GetSession("non_existent_id")
	if err != ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound, got %v", err)
	}

	// Test empty session ID
	_, err = sm.GetSession("")
	if err != ErrInvalidSessionID {
		t.Errorf("Expected ErrInvalidSessionID, got %v", err)
	}
}

func TestGetSessionExpired(t *testing.T) {
	config := DefaultSessionConfig
	config.MaxAge = time.Millisecond * 50 // Very short expiry for testing
	config.CleanupInterval = 0
	sm := NewSessionManager(config)

	userID := int64(123)
	sessionID, _, err := sm.CreateSession(userID, "verified", "test@example.com", "access_token", "refresh_token", "", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Wait for expiry
	time.Sleep(time.Millisecond * 100)

	// Should return expired error
	_, err = sm.GetSession(sessionID)
	if err != ErrSessionExpired {
		t.Errorf("Expected ErrSessionExpired, got %v", err)
	}
}

func TestUpdateSession(t *testing.T) {
	config := DefaultSessionConfig
	config.CleanupInterval = 0
	sm := NewSessionManager(config)

	userID := int64(123)
	sessionID, _, err := sm.CreateSession(userID, "verified", "test@example.com", "access_token", "refresh_token", "", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Update session
	updates := map[string]interface{}{
		"refresh_token": "new_token",
		"role":          "admin",
		"email":         "new@example.com",
		"metadata": map[string]interface{}{
			"last_login": time.Now().Unix(),
		},
	}

	err = sm.UpdateSession(sessionID, updates)
	if err != nil {
		t.Fatalf("UpdateSession failed: %v", err)
	}

	// Verify updates
	sessionData, err := sm.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if sessionData.RefreshToken != "new_token" {
		t.Errorf("Refresh token not updated: got %s, want new_token", sessionData.RefreshToken)
	}

	if sessionData.Role != "admin" {
		t.Errorf("Role not updated: got %s, want admin", sessionData.Role)
	}

	if sessionData.Email != "new@example.com" {
		t.Errorf("Email not updated: got %s, want new@example.com", sessionData.Email)
	}

	if sessionData.Metadata["last_login"] == nil {
		t.Error("Metadata not updated")
	}
}

func TestDeleteSession(t *testing.T) {
	config := DefaultSessionConfig
	config.CleanupInterval = 0
	sm := NewSessionManager(config)

	userID := int64(123)
	sessionID, _, err := sm.CreateSession(userID, "verified", "test@example.com", "access_token", "refresh_token", "", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Delete session
	deleted := sm.DeleteSession(sessionID)
	if !deleted {
		t.Error("DeleteSession should return true for existing session")
	}

	// Verify deletion
	_, err = sm.GetSession(sessionID)
	if err != ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound after deletion, got %v", err)
	}

	// Delete non-existent session
	deleted = sm.DeleteSession("non_existent")
	if deleted {
		t.Error("DeleteSession should return false for non-existent session")
	}
}

func TestDeleteUserSessions(t *testing.T) {
	config := DefaultSessionConfig
	config.CleanupInterval = 0
	sm := NewSessionManager(config)

	userID := int64(123)

	// Create multiple sessions for the same user
	_, _, err := sm.CreateSession(userID, "verified", "test@example.com", "access1", "token1", "", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, _, err = sm.CreateSession(userID, "verified", "test@example.com", "access2", "token2", "", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Create session for different user
	_, _, err = sm.CreateSession(456, "verified", "other@example.com", "access3", "token3", "", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Delete sessions for user 123
	deleted := sm.DeleteUserSessions(userID)
	if deleted != 2 {
		t.Errorf("Expected 2 sessions deleted, got %d", deleted)
	}

	// Verify user 123 has no sessions
	userSessions := sm.GetUserSessions(userID)
	if len(userSessions) != 0 {
		t.Errorf("User should have no sessions after deletion, got %d", len(userSessions))
	}

	// Verify other user still has session
	otherSessions := sm.GetUserSessions(456)
	if len(otherSessions) != 1 {
		t.Errorf("Other user should still have 1 session, got %d", len(otherSessions))
	}
}

func TestExtendSession(t *testing.T) {
	config := DefaultSessionConfig
	config.CleanupInterval = 0
	sm := NewSessionManager(config)

	userID := int64(123)
	sessionID, originalData, err := sm.CreateSession(userID, "verified", "test@example.com", "access_token", "refresh_token", "", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	originalExpiry := originalData.ExpiresAt

	// Extend session
	extension := time.Hour
	err = sm.ExtendSession(sessionID, extension)
	if err != nil {
		t.Fatalf("ExtendSession failed: %v", err)
	}

	// Verify extension
	sessionData, err := sm.GetSession(sessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	expectedExpiry := originalExpiry.Add(extension)
	if !sessionData.ExpiresAt.Equal(expectedExpiry) {
		t.Errorf("Session not extended properly: got %v, want %v", sessionData.ExpiresAt, expectedExpiry)
	}
}

func TestSetSessionCookie(t *testing.T) {
	config := DefaultSessionConfig
	config.CleanupInterval = 0
	sm := NewSessionManager(config)

	w := httptest.NewRecorder()
	sessionID := "test_session_id"

	sm.SetSessionCookie(w, sessionID)

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Expected 1 cookie, got %d", len(cookies))
	}

	cookie := cookies[0]
	if cookie.Name != config.CookieName {
		t.Errorf("Cookie name mismatch: got %s, want %s", cookie.Name, config.CookieName)
	}

	if cookie.Value != sessionID {
		t.Errorf("Cookie value mismatch: got %s, want %s", cookie.Value, sessionID)
	}

	if cookie.HttpOnly != config.HttpOnly {
		t.Errorf("HttpOnly mismatch: got %v, want %v", cookie.HttpOnly, config.HttpOnly)
	}

	if cookie.Secure != config.Secure {
		t.Errorf("Secure mismatch: got %v, want %v", cookie.Secure, config.Secure)
	}

	if cookie.SameSite != config.SameSite {
		t.Errorf("SameSite mismatch: got %v, want %v", cookie.SameSite, config.SameSite)
	}
}

func TestClearSessionCookie(t *testing.T) {
	config := DefaultSessionConfig
	config.CleanupInterval = 0
	sm := NewSessionManager(config)

	w := httptest.NewRecorder()
	sm.ClearSessionCookie(w)

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Expected 1 cookie, got %d", len(cookies))
	}

	cookie := cookies[0]
	if cookie.Name != config.CookieName {
		t.Errorf("Cookie name mismatch: got %s, want %s", cookie.Name, config.CookieName)
	}

	if cookie.Value != "" {
		t.Errorf("Cookie value should be empty, got %s", cookie.Value)
	}

	if cookie.MaxAge != -1 {
		t.Errorf("Cookie MaxAge should be -1, got %d", cookie.MaxAge)
	}
}

func TestGetSessionFromRequest(t *testing.T) {
	config := DefaultSessionConfig
	config.CleanupInterval = 0
	sm := NewSessionManager(config)

	sessionID := "test_session_id"

	// Create request with session cookie
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  config.CookieName,
		Value: sessionID,
	})

	extractedID, err := sm.GetSessionFromRequest(req)
	if err != nil {
		t.Fatalf("GetSessionFromRequest failed: %v", err)
	}

	if extractedID != sessionID {
		t.Errorf("Session ID mismatch: got %s, want %s", extractedID, sessionID)
	}

	// Test request without cookie
	reqNoCookie := httptest.NewRequest("GET", "/", nil)
	_, err = sm.GetSessionFromRequest(reqNoCookie)
	if err != ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound, got %v", err)
	}
}

func TestGetUserSessions(t *testing.T) {
	config := DefaultSessionConfig
	config.CleanupInterval = 0
	sm := NewSessionManager(config)

	userID := int64(123)

	// Create multiple sessions for the user
	_, _, err := sm.CreateSession(userID, "verified", "test@example.com", "access1", "token1", "", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, _, err = sm.CreateSession(userID, "verified", "test@example.com", "access2", "token2", "", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Get user sessions
	sessions := sm.GetUserSessions(userID)
	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(sessions))
	}

	// Verify all sessions belong to the user
	for _, session := range sessions {
		if session.UserID != userID {
			t.Errorf("Session belongs to wrong user: got %d, want %d", session.UserID, userID)
		}
	}
}

func TestCleanupExpiredSessions(t *testing.T) {
	config := DefaultSessionConfig
	config.MaxAge = time.Millisecond * 50 // Very short expiry
	config.CleanupInterval = 0
	sm := NewSessionManager(config)

	userID := int64(123)

	// Create sessions
	_, _, err := sm.CreateSession(userID, "verified", "test1@example.com", "access1", "token1", "", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, _, err = sm.CreateSession(userID, "verified", "test2@example.com", "access2", "token2", "", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Wait for expiry
	time.Sleep(time.Millisecond * 100)

	// Cleanup should remove expired sessions
	cleaned := sm.CleanupExpiredSessions()
	if cleaned != 2 {
		t.Errorf("Expected 2 sessions cleaned, got %d", cleaned)
	}
}

func TestGetStats(t *testing.T) {
	config := DefaultSessionConfig
	config.CleanupInterval = 0
	sm := NewSessionManager(config)

	userID1 := int64(123)
	userID2 := int64(456)

	// Create sessions
	_, _, err := sm.CreateSession(userID1, "verified", "test1@example.com", "access1", "token1", "", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, _, err = sm.CreateSession(userID2, "verified", "test2@example.com", "access2", "token2", "", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	stats := sm.GetStats()

	if stats["total_sessions"].(int) != 2 {
		t.Errorf("Total sessions should be 2, got %v", stats["total_sessions"])
	}

	if stats["active_count"].(int) != 2 {
		t.Errorf("Active sessions should be 2, got %v", stats["active_count"])
	}

	if stats["cookie_name"].(string) != config.CookieName {
		t.Errorf("Cookie name mismatch in stats: got %v, want %s", stats["cookie_name"], config.CookieName)
	}
}

func TestPredefinedConfigs(t *testing.T) {
	configs := []SessionConfig{
		DefaultSessionConfig,
		DevelopmentSessionConfig,
		ProductionSessionConfig,
	}

	for i, config := range configs {
		if config.CookieName == "" {
			t.Errorf("Config %d: Cookie name should not be empty", i)
		}
		if config.Path == "" {
			t.Errorf("Config %d: Path should not be empty", i)
		}
		if config.MaxAge <= 0 {
			t.Errorf("Config %d: MaxAge should be positive, got %v", i, config.MaxAge)
		}
		if !config.HttpOnly {
			t.Errorf("Config %d: HttpOnly should be true for security", i)
		}
	}

	// Production config should be secure
	if !ProductionSessionConfig.Secure {
		t.Error("Production config should have Secure=true")
	}

	// Development config can be insecure for local development
	if DevelopmentSessionConfig.Secure {
		t.Error("Development config should have Secure=false for local development")
	}
}

func BenchmarkCreateSession(b *testing.B) {
	config := DefaultSessionConfig
	config.CleanupInterval = 0
	sm := NewSessionManager(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		userID := int64(i)
		_, _, err := sm.CreateSession(userID, "verified", "test@example.com", "access_token", "refresh_token", "", "")
		if err != nil {
			b.Fatalf("CreateSession failed: %v", err)
		}
	}
}

func BenchmarkGetSession(b *testing.B) {
	config := DefaultSessionConfig
	config.CleanupInterval = 0
	sm := NewSessionManager(config)

	// Create sessions for benchmarking
	sessionIDs := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		userID := int64(i)
		sessionID, _, err := sm.CreateSession(userID, "verified", "test@example.com", "access_token", "refresh_token", "", "")
		if err != nil {
			b.Fatalf("CreateSession failed: %v", err)
		}
		sessionIDs[i] = sessionID
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := sm.GetSession(sessionIDs[i])
		if err != nil {
			b.Fatalf("GetSession failed: %v", err)
		}
	}
}
