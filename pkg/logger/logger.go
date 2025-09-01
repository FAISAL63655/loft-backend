// Package logger provides structured logging for Loft Dughairi backend
package logger

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"
)

// LogLevel represents the logging level
type LogLevel string

const (
	DEBUG LogLevel = "DEBUG"
	INFO  LogLevel = "INFO"
	WARN  LogLevel = "WARN"
	ERROR LogLevel = "ERROR"
	FATAL LogLevel = "FATAL"
)

// Fields represents structured logging fields
type Fields map[string]interface{}

// Logger provides structured logging capabilities
type Logger struct {
	level    LogLevel
	service  string
	instance string
}

// logEntry represents a single log entry
type logEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Service   string                 `json:"service,omitempty"`
	Instance  string                 `json:"instance,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	UserID    string                 `json:"user_id,omitempty"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	File      string                 `json:"file,omitempty"`
	Line      int                    `json:"line,omitempty"`
}

// globalLogger is the default logger instance
var globalLogger *Logger

func init() {
	globalLogger = NewLogger("loft-dughairi", "")
}

// NewLogger creates a new structured logger
func NewLogger(service, instance string) *Logger {
	return &Logger{
		level:    INFO,
		service:  service,
		instance: instance,
	}
}

// SetLevel sets the minimum logging level
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

// GetLevel returns current logging level
func (l *Logger) GetLevel() LogLevel {
	return l.level
}

// shouldLog checks if message should be logged based on level
func (l *Logger) shouldLog(level LogLevel) bool {
	levels := map[LogLevel]int{
		DEBUG: 0,
		INFO:  1,
		WARN:  2,
		ERROR: 3,
		FATAL: 4,
	}

	return levels[level] >= levels[l.level]
}

// getCallerInfo gets file and line info of the caller
func getCallerInfo(skip int) (string, int) {
	_, file, line, ok := runtime.Caller(skip + 2) // +2 to skip this function and the logging function
	if !ok {
		return "unknown", 0
	}

	// Show only filename, not full path
	parts := strings.Split(file, "/")
	if len(parts) > 0 {
		file = parts[len(parts)-1]
	}

	return file, line
}

// log performs the actual logging
func (l *Logger) log(ctx context.Context, level LogLevel, message string, fields Fields) {
	if !l.shouldLog(level) {
		return
	}

	file, line := getCallerInfo(2)

	entry := logEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     string(level),
		Service:   l.service,
		Instance:  l.instance,
		Message:   message,
		Fields:    map[string]interface{}(fields),
		File:      file,
		Line:      line,
	}

	// Extract context values if available
	if ctx != nil {
		if requestID := getRequestID(ctx); requestID != "" {
			entry.RequestID = requestID
		}
		if userID := getUserID(ctx); userID != "" {
			entry.UserID = userID
		}
	}

	// Format and output log entry
	output := formatLogEntry(entry)
	log.Print(output)

	// Fatal level should exit the program
	if level == FATAL {
		os.Exit(1)
	}
}

// formatLogEntry formats the log entry for output
func formatLogEntry(entry logEntry) string {
	// Simple structured format (in production, use JSON)
	parts := []string{
		fmt.Sprintf("[%s]", entry.Timestamp),
		fmt.Sprintf("%-5s", entry.Level),
	}

	if entry.Service != "" {
		parts = append(parts, fmt.Sprintf("service=%s", entry.Service))
	}

	if entry.RequestID != "" {
		parts = append(parts, fmt.Sprintf("req_id=%s", entry.RequestID))
	}

	if entry.UserID != "" {
		parts = append(parts, fmt.Sprintf("user_id=%s", entry.UserID))
	}

	parts = append(parts, fmt.Sprintf("file=%s:%d", entry.File, entry.Line))
	parts = append(parts, entry.Message)

	// Add fields if present
	if len(entry.Fields) > 0 {
		var fieldPairs []string
		for k, v := range entry.Fields {
			fieldPairs = append(fieldPairs, fmt.Sprintf("%s=%v", k, v))
		}
		parts = append(parts, fmt.Sprintf("fields=(%s)", strings.Join(fieldPairs, ", ")))
	}

	return strings.Join(parts, " ")
}

// Context key types for avoiding collisions
type contextKey string

const (
	requestIDKey contextKey = "request_id"
	userIDKey    contextKey = "user_id"
)

// getRequestID extracts request ID from context
func getRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// getUserID extracts user ID from context
func getUserID(ctx context.Context) string {
	if id, ok := ctx.Value(userIDKey).(string); ok {
		return id
	}
	return ""
}

// WithRequestID adds request ID to context
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// WithUserID adds user ID to context
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// Logging methods for the default logger
func Debug(ctx context.Context, message string, fields ...Fields) {
	globalLogger.Debug(ctx, message, fields...)
}

func Info(ctx context.Context, message string, fields ...Fields) {
	globalLogger.Info(ctx, message, fields...)
}

func Warn(ctx context.Context, message string, fields ...Fields) {
	globalLogger.Warn(ctx, message, fields...)
}

func Error(ctx context.Context, message string, fields ...Fields) {
	globalLogger.Error(ctx, message, fields...)
}

func Fatal(ctx context.Context, message string, fields ...Fields) {
	globalLogger.Fatal(ctx, message, fields...)
}

// Logging methods for Logger instance
func (l *Logger) Debug(ctx context.Context, message string, fields ...Fields) {
	mergedFields := mergeFields(fields...)
	l.log(ctx, DEBUG, message, mergedFields)
}

func (l *Logger) Info(ctx context.Context, message string, fields ...Fields) {
	mergedFields := mergeFields(fields...)
	l.log(ctx, INFO, message, mergedFields)
}

func (l *Logger) Warn(ctx context.Context, message string, fields ...Fields) {
	mergedFields := mergeFields(fields...)
	l.log(ctx, WARN, message, mergedFields)
}

func (l *Logger) Error(ctx context.Context, message string, fields ...Fields) {
	mergedFields := mergeFields(fields...)
	l.log(ctx, ERROR, message, mergedFields)
}

func (l *Logger) Fatal(ctx context.Context, message string, fields ...Fields) {
	mergedFields := mergeFields(fields...)
	l.log(ctx, FATAL, message, mergedFields)
}

// mergeFields combines multiple field maps
func mergeFields(fieldMaps ...Fields) Fields {
	result := make(Fields)
	for _, fields := range fieldMaps {
		for k, v := range fields {
			result[k] = v
		}
	}
	return result
}

// Convenience functions for common logging patterns

// LogError logs an error with optional fields
func LogError(ctx context.Context, err error, message string, fields ...Fields) {
	if err == nil {
		return
	}

	errorFields := Fields{"error": err.Error()}
	allFields := append([]Fields{errorFields}, fields...)
	Error(ctx, message, allFields...)
}

// LogPanic logs a panic with stack trace
func LogPanic(ctx context.Context, recovered interface{}, fields ...Fields) {
	panicFields := Fields{
		"panic": recovered,
		"stack": string(getStackTrace()),
	}
	allFields := append([]Fields{panicFields}, fields...)
	Error(ctx, "Panic recovered", allFields...)
}

// getStackTrace returns current stack trace
func getStackTrace() []byte {
	buf := make([]byte, 1024*8) // 8KB buffer should be enough
	n := runtime.Stack(buf, false)
	return buf[:n]
}

// SetGlobalLevel sets the global logger level
func SetGlobalLevel(level LogLevel) {
	globalLogger.SetLevel(level)
}

// GetGlobalLogger returns the global logger instance
func GetGlobalLogger() *Logger {
	return globalLogger
}
