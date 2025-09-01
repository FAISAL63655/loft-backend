// Package storagegcs - Database integration for media settings
package storagegcs

import (
	"database/sql"
	"fmt"
	"strings"
)

// DatabaseClient interface for database operations
type DatabaseClient interface {
	QueryRow(query string, args ...interface{}) *sql.Row
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// LoadMediaSettingsFromDatabase loads media settings from system_settings table
func LoadMediaSettingsFromDatabase(db DatabaseClient) (*MediaSettings, error) {
	if db == nil {
		return getDefaultMediaSettings(), nil
	}

	settings := &MediaSettings{}
	
	// Query system settings related to media
	query := `
		SELECT 
			media_max_file_size,
			media_allowed_types,
			media_storage_provider,
			media_watermark_enabled,
			media_watermark_position,
			media_watermark_opacity
		FROM system_settings 
		LIMIT 1`
	
	var allowedTypesStr string
	var storageProvider string
	
	err := db.QueryRow(query).Scan(
		&settings.MaxFileSize,
		&allowedTypesStr,
		&storageProvider,
		&settings.WatermarkEnabled,
		&settings.WatermarkPosition,
		&settings.WatermarkOpacity,
	)
	
	if err != nil {
		if err == sql.ErrNoRows {
			// Return defaults if no settings found
			return getDefaultMediaSettings(), nil
		}
		return nil, fmt.Errorf("failed to query media settings: %w", err)
	}
	
	// Parse allowed types from comma-separated string
	if allowedTypesStr != "" {
		settings.AllowedTypes = strings.Split(allowedTypesStr, ",")
		// Trim whitespace from each type
		for i, t := range settings.AllowedTypes {
			settings.AllowedTypes[i] = strings.TrimSpace(t)
		}
	} else {
		settings.AllowedTypes = getDefaultMediaSettings().AllowedTypes
	}
	
	// Set defaults for fields not in database
	settings.ThumbnailsEnabled = true // Default enabled
	settings.ThumbnailSizes = []int{200, 400} // Default sizes
	
	// Validate and set reasonable defaults
	if settings.MaxFileSize <= 0 {
		settings.MaxFileSize = 10485760 // 10MB default
	}
	
	if settings.WatermarkOpacity < 0 || settings.WatermarkOpacity > 1 {
		settings.WatermarkOpacity = 0.7 // Default 70%
	}
	
	if settings.WatermarkPosition == "" {
		settings.WatermarkPosition = "bottom-right"
	}
	
	return settings, nil
}

// LoadMediaSettingsWithThumbnails loads settings with thumbnail configuration
func LoadMediaSettingsWithThumbnails(db DatabaseClient, enableThumbnails bool, sizes []int) (*MediaSettings, error) {
	settings, err := LoadMediaSettingsFromDatabase(db)
	if err != nil {
		return nil, err
	}
	
	// Override thumbnail settings
	settings.ThumbnailsEnabled = enableThumbnails
	if len(sizes) > 0 {
		settings.ThumbnailSizes = sizes
	}
	
	return settings, nil
}

// UpdateMediaSettings updates media settings in database
func UpdateMediaSettings(db DatabaseClient, settings *MediaSettings) error {
	if db == nil {
		return fmt.Errorf("database client is required")
	}
	
	// Convert allowed types to comma-separated string
	allowedTypesStr := strings.Join(settings.AllowedTypes, ",")
	
	query := `
		UPDATE system_settings SET
			media_max_file_size = $1,
			media_allowed_types = $2,
			media_watermark_enabled = $3,
			media_watermark_position = $4,
			media_watermark_opacity = $5,
			updated_at = NOW()
		WHERE id = 1`
	
	_, err := db.Query(query,
		settings.MaxFileSize,
		allowedTypesStr,
		settings.WatermarkEnabled,
		settings.WatermarkPosition,
		settings.WatermarkOpacity,
	)
	
	if err != nil {
		return fmt.Errorf("failed to update media settings: %w", err)
	}
	
	return nil
}

// getDefaultMediaSettings returns sensible defaults
func getDefaultMediaSettings() *MediaSettings {
	return &MediaSettings{
		WatermarkEnabled:  true,
		WatermarkPosition: "bottom-right",
		WatermarkOpacity:  0.7,
		ThumbnailsEnabled: true,
		ThumbnailSizes:    []int{200, 400},
		MaxFileSize:       10485760, // 10MB
		AllowedTypes:      []string{"image/jpeg", "image/png", "image/webp", "video/mp4", "application/pdf"},
	}
}

// ValidateMediaSettings validates media settings values
func ValidateMediaSettings(settings *MediaSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	
	// Validate max file size
	if settings.MaxFileSize <= 0 {
		return fmt.Errorf("max file size must be positive")
	}
	
	if settings.MaxFileSize > 104857600 { // 100MB
		return fmt.Errorf("max file size too large (max: 100MB)")
	}
	
	// Validate watermark opacity
	if settings.WatermarkOpacity < 0 || settings.WatermarkOpacity > 1 {
		return fmt.Errorf("watermark opacity must be between 0 and 1")
	}
	
	// Validate watermark position
	validPositions := []string{"center", "top-left", "top-right", "bottom-left", "bottom-right"}
	valid := false
	for _, pos := range validPositions {
		if settings.WatermarkPosition == pos {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid watermark position: %s", settings.WatermarkPosition)
	}
	
	// Validate allowed types
	if len(settings.AllowedTypes) == 0 {
		return fmt.Errorf("at least one allowed type must be specified")
	}
	
	// Validate thumbnail sizes
	for _, size := range settings.ThumbnailSizes {
		if size <= 0 || size > 2000 {
			return fmt.Errorf("invalid thumbnail size: %d (must be 1-2000)", size)
		}
	}
	
	return nil
}
