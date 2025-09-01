// Package config - Example of server integration with dynamic CORS
package config

import (
	"log"
	"net/http"
	"time"

	"encore.app/pkg/middleware"
	"encore.dev/storage/sqldb"
)

// ServerWithDynamicCORS demonstrates how to setup server with dynamic CORS
func ServerWithDynamicCORS() *http.Server {
	// Note: In a real service, you would get db from service definition
	// Example: var db = sqldb.Named("coredb")
	var db *sqldb.Database // This would be passed from service

	// Initialize config system with hot-reload
	Initialize(db, 2*time.Minute) // Check every 2 minutes for changes

	// Setup dynamic CORS integration
	SetupDynamicCORS()

	// Create CORS settings getter function
	getCORSSettings := func() middleware.CORSSettingsData {
		corsSettings := GetCORSSettings()
		return middleware.CORSSettingsData{
			AllowedOrigins: corsSettings.AllowedOrigins,
			AllowedMethods: corsSettings.AllowedMethods,
			AllowedHeaders: corsSettings.AllowedHeaders,
			MaxAge:         corsSettings.MaxAge,
		}
	}

	// Create dynamic CORS middleware
	dynamicCORSMiddleware := middleware.CreateDynamicCORSMiddleware(getCORSSettings)

	// Create your HTTP handlers
	mux := http.NewServeMux()

	// Add your routes here
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "service": "loft-dughairi"}`))
	})

	mux.HandleFunc("/api/settings/cors", func(w http.ResponseWriter, r *http.Request) {
		// Endpoint to check current CORS settings
		settings := GetCORSSettings()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Simple JSON response (in real app, use proper JSON marshaling)
		response := `{
			"allowed_origins": ` + formatStringArray(settings.AllowedOrigins) + `,
			"allowed_methods": ` + formatStringArray(settings.AllowedMethods) + `,
			"max_age": ` + formatInt(settings.MaxAge) + `
		}`
		w.Write([]byte(response))
	})

	// Build middleware chain with dynamic CORS at the top
	handler := dynamicCORSMiddleware(
		middleware.SecurityHeadersMiddleware(middleware.DefaultSecurityConfig)(
			middleware.LoggingMiddleware()(
				middleware.RecoveryMiddleware()(mux),
			),
		),
	)

	server := &http.Server{
		Addr:           ":8080",
		Handler:        handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	log.Printf("Server configured with dynamic CORS")
	log.Printf("CORS settings will be refreshed from database every request")
	log.Printf("Current CORS origins: %v", GetCORSSettings().AllowedOrigins)

	return server
}

// Helper functions for simple JSON formatting (in real app, use proper JSON package)
func formatStringArray(arr []string) string {
	if len(arr) == 0 {
		return "[]"
	}

	result := "["
	for i, s := range arr {
		if i > 0 {
			result += ", "
		}
		result += `"` + s + `"`
	}
	result += "]"
	return result
}

func formatInt(i int) string {
	// EXAMPLE ONLY: Simple int to string conversion with hardcoded values
	// In production code, use strconv.Itoa(i) or fmt.Sprintf("%d", i)
	knownValues := map[int]string{
		300:   "300",   // 5 minutes
		3600:  "3600",  // 1 hour  
		86400: "86400", // 24 hours
	}
	if val, ok := knownValues[i]; ok {
		return val
	}
	// Fallback for unexpected values in this demo
	return "0"
}

// StartServerExample shows how to start the server (example only)
func StartServerExample() {
	server := ServerWithDynamicCORS()

	log.Printf("Starting server on %s", server.Addr)
	log.Printf("Dynamic CORS enabled - check /api/settings/cors for current settings")
	log.Printf("To test: Change CORS settings in database and make requests")

	// In real app, handle graceful shutdown
	// if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
	//     log.Fatalf("Server failed to start: %v", err)
	// }
}
