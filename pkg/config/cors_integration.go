// Package config - CORS integration to avoid import cycles
package config

import (
	"log"
)

// CORSSettings represents CORS configuration from system settings
type CORSSettings struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	MaxAge         int
}

// GetCORSSettings returns current CORS settings from system_settings
func GetCORSSettings() *CORSSettings {
	settings := GetSettings()
	if settings == nil {
		// Fallback values
		return &CORSSettings{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
			MaxAge:         86400,
		}
	}

	return &CORSSettings{
		AllowedOrigins: settings.CORSAllowedOrigins,
		AllowedMethods: settings.CORSAllowedMethods,
		AllowedHeaders: settings.CORSAllowedHeaders,
		MaxAge:         settings.CORSMaxAge,
	}
}

// SetupDynamicCORS initializes CORS with hot-reload capability
func SetupDynamicCORS() {
	manager := GetGlobalManager()

	// Add listener for CORS settings changes
	manager.AddChangeListener(func(newSettings *SystemSettings) {
		log.Printf("CORS settings updated:")
		log.Printf("  Allowed Origins: %v", newSettings.CORSAllowedOrigins)
		log.Printf("  Allowed Methods: %v", newSettings.CORSAllowedMethods)
		log.Printf("  Max Age: %d seconds", newSettings.CORSMaxAge)

		// Note: CORS changes will be picked up on next request
		// due to GetDynamicSettings() being called per request
	})

	log.Printf("Dynamic CORS initialized with hot-reload capability")
}
