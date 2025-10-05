// Package middleware - Factory functions for creating middleware with dynamic configuration
package middleware

import (
	"log"
	"net/http"
)

// CORSSettingsGetter defines interface to get CORS settings (avoids import cycle)
type CORSSettingsGetter interface {
	GetCORSSettings() CORSSettingsData
}

// CORSSettingsData holds CORS configuration data
type CORSSettingsData struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	MaxAge         int
}

// DynamicCORSMiddleware creates CORS middleware that refreshes settings per request
func DynamicCORSMiddleware(settingsGetter CORSSettingsGetter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get fresh CORS settings for each request (ensures hot-reload works)
			corsData := settingsGetter.GetCORSSettings()

			// Create dynamic CORS config
			corsConfig := CORSConfig{
				AllowedOrigins:     corsData.AllowedOrigins,
				AllowedMethods:     corsData.AllowedMethods,
				AllowedHeaders:     corsData.AllowedHeaders,
				ExposedHeaders:     []string{"Link", "X-Total-Count", "X-Request-ID"}, // Static exposed headers
				AllowCredentials:   true,                                              // ✅ لازم يكون true عشان الكوكيز يمر
				MaxAge:             corsData.MaxAge,
				OptionsPassthrough: false,
				UseSystemSettings:  false, // We're handling it manually here
			}

			// Apply CORS middleware with fresh settings
			corsMiddleware := CORSMiddleware(corsConfig)
			corsMiddleware(next).ServeHTTP(w, r)
		})
	}
}

// ConfigAdapter adapts config package to CORSSettingsGetter interface
type ConfigAdapter struct {
	GetSettingsFunc func() CORSSettingsData
}

// GetCORSSettings implements CORSSettingsGetter interface
func (ca *ConfigAdapter) GetCORSSettings() CORSSettingsData {
	return ca.GetSettingsFunc()
}

// CreateDynamicCORSMiddleware creates a CORS middleware that reads from system_settings
func CreateDynamicCORSMiddleware(getCORSSettings func() CORSSettingsData) func(http.Handler) http.Handler {
	adapter := &ConfigAdapter{
		GetSettingsFunc: getCORSSettings,
	}

	middleware := DynamicCORSMiddleware(adapter)

	log.Printf("Dynamic CORS middleware created - will refresh settings on each request")

	return middleware
}
