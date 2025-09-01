// Package config provides system configuration management with hot-reload capabilities
package config

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"encore.dev/storage/sqldb"
)

// SystemSettings holds all system configuration
type SystemSettings struct {
	// WebSocket settings
	WSEnabled           bool `json:"ws_enabled"`
	WSMaxConnections    int  `json:"ws_max_connections"`
	WSHeartbeatInterval int  `json:"ws_heartbeat_interval"`
	// PRD additions
	WSMaxConnectionsPerHost int `json:"ws_max_connections_per_host"`
	WSMessagesPerMinute     int `json:"ws_msgs_per_minute"`

	// Payment settings
	PaymentsEnabled  bool   `json:"payments_enabled"`
	PaymentsProvider string `json:"payments_provider"`
	PaymentsTestMode bool   `json:"payments_test_mode"`
	PaymentsCurrency string `json:"payments_currency"`

	// CORS settings
	CORSAllowedOrigins []string `json:"cors_allowed_origins"`
	CORSAllowedMethods []string `json:"cors_allowed_methods"`
	CORSAllowedHeaders []string `json:"cors_allowed_headers"`
	CORSMaxAge         int      `json:"cors_max_age"`

	// Media settings
	MediaMaxFileSize       int64    `json:"media_max_file_size"`
	MediaAllowedTypes      []string `json:"media_allowed_types"`
	MediaStorageProvider   string   `json:"media_storage_provider"`
	MediaWatermarkEnabled  bool     `json:"media.watermark.enabled"`
	MediaWatermarkPosition string   `json:"media.watermark.position"`
	MediaWatermarkOpacity  float64  `json:"media.watermark.opacity"`

	// App settings
	AppName                string `json:"app_name"`
	AppVersion             string `json:"app_version"`
	AppMaintenanceMode     bool   `json:"app_maintenance_mode"`
	AppRegistrationEnabled bool   `json:"app_registration_enabled"`

	// Notification settings
	NotificationsEmailEnabled bool `json:"notifications_email_enabled"`
	NotificationsSMSEnabled   bool `json:"notifications_sms_enabled"`
	NotificationsPushEnabled  bool `json:"notifications_push_enabled"`

	// Security settings
	SecuritySessionTimeout   int `json:"security_session_timeout"`
	SecurityMaxLoginAttempts int `json:"security_max_login_attempts"`
	SecurityLockoutDuration  int `json:"security_lockout_duration"`

	// Auction settings
	AuctionsDefaultDuration    int     `json:"auctions_default_duration"`
	AuctionsMinBidIncrement    float64 `json:"auctions_min_bid_increment"`
	AuctionsAutoExtendEnabled  bool    `json:"auctions_auto_extend_enabled"`
	AuctionsAutoExtendDuration int     `json:"auctions_auto_extend_duration"`
	AuctionsAntiSnipingMinutes int     `json:"auctions_anti_sniping_minutes"`

	// VAT and shipping settings
	VATEnabled                  bool    `json:"vat_enabled"`
	VATRate                     float64 `json:"vat_rate"`
	ShippingFreeThreshold       float64 `json:"shipping_free_threshold"`
	AuctionsMaxExtensions       int     `json:"auctions_max_extensions"`
	PaymentsSessionTTL          int     `json:"payments_session_ttl_minutes"`
	PaymentsIdempotencyTTLHours int     `json:"payments_idempotency_ttl_hours"`
	NotificationsEmailRetention int     `json:"notifications_email_retention_days"`

	// Rate limits (PRD)
	BidsRateLimitPerMinute   int `json:"bids_rate_limit_per_minute"`
	PaymentsRateLimitPer5Min int `json:"payments_rate_limit_per_5min"`

	// Stock / Cart settings
	StockCheckoutHoldMinutes   int `json:"stock_checkout_hold_minutes"`
	StockSuppliesHoldMinutes   int `json:"stock_supplies_hold_minutes"`
	StockMaxActiveHoldsPerUser int `json:"stock_max_active_holds_per_user"`

	// Metadata
	LastUpdated time.Time `json:"last_updated"`
}

// ChangeListener is called when settings change
type ChangeListener func(settings *SystemSettings)

// ConfigManager manages system configuration with hot-reload
type ConfigManager struct {
	db           *sqldb.Database
	settings     *SystemSettings
	mutex        sync.RWMutex
	listeners    []ChangeListener
	stopReload   chan struct{}
	reloadTicker *time.Ticker
	cache        map[string]interface{}
	cacheTTL     time.Duration
	lastReload   time.Time
}

// settingRow represents a row from system_settings table
type settingRow struct {
	Key   string         `json:"key"`
	Value sql.NullString `json:"value"`
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(db *sqldb.Database, reloadInterval time.Duration) *ConfigManager {
	manager := &ConfigManager{
		db:         db,
		settings:   &SystemSettings{},
		listeners:  make([]ChangeListener, 0),
		stopReload: make(chan struct{}),
		cache:      make(map[string]interface{}),
		cacheTTL:   5 * time.Minute, // Cache settings for 5 minutes
	}

	// Load initial settings
	if err := manager.LoadSettings(); err != nil {
		log.Printf("Failed to load initial settings: %v", err)
		manager.setDefaults()
	}

	// Start hot-reload if interval > 0
	if reloadInterval > 0 {
		manager.startHotReload(reloadInterval)
	}

	return manager
}

// LoadSettings loads settings from database
func (cm *ConfigManager) LoadSettings() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := cm.db.Query(ctx, `
		SELECT key, value 
		FROM system_settings 
		WHERE value IS NOT NULL 
		ORDER BY key
	`)
	if err != nil {
		return fmt.Errorf("failed to query system_settings: %w", err)
	}
	defer rows.Close()

	settingsMap := make(map[string]string)

	for rows.Next() {
		var row settingRow
		if err := rows.Scan(&row.Key, &row.Value); err != nil {
			log.Printf("Failed to scan setting row: %v", err)
			continue
		}

		if row.Value.Valid {
			settingsMap[row.Key] = row.Value.String
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating settings: %w", err)
	}

	return cm.populateSettings(settingsMap)
}

// populateSettings populates SystemSettings from key-value map
func (cm *ConfigManager) populateSettings(settingsMap map[string]string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	settings := &SystemSettings{}

	// WebSocket settings
	settings.WSEnabled = parseBool(settingsMap["ws.enabled"], true)
	settings.WSMaxConnections = parseInt(settingsMap["ws.max_connections"], 1000)
	settings.WSHeartbeatInterval = parseInt(settingsMap["ws.heartbeat_interval"], 30)
	settings.WSMaxConnectionsPerHost = parseInt(settingsMap["ws.max_connections_per_host"], 120)
	settings.WSMessagesPerMinute = parseInt(settingsMap["ws.msgs_per_minute"], 30)

	// Payment settings
	settings.PaymentsEnabled = parseBool(settingsMap["payments.enabled"], true)
	settings.PaymentsProvider = parseString(settingsMap["payments.provider"], "moyasar")
	settings.PaymentsTestMode = parseBool(settingsMap["payments.test_mode"], true)
	settings.PaymentsCurrency = parseString(settingsMap["payments.currency"], "SAR")

	// CORS settings
	settings.CORSAllowedOrigins = parseStringSlice(settingsMap["cors.allowed_origins"], []string{"*"})
	settings.CORSAllowedMethods = parseStringSlice(settingsMap["cors.allowed_methods"], []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
	settings.CORSAllowedHeaders = parseStringSlice(settingsMap["cors.allowed_headers"], []string{"Content-Type", "Authorization", "X-Requested-With"})
	settings.CORSMaxAge = parseInt(settingsMap["cors.max_age"], 86400)

	// Media settings
	settings.MediaMaxFileSize = parseInt64(settingsMap["media.max_file_size"], 10485760)
	settings.MediaAllowedTypes = parseStringSlice(settingsMap["media.allowed_types"], []string{"image/jpeg", "image/png", "image/webp", "video/mp4"})
	settings.MediaStorageProvider = parseString(settingsMap["media.storage_provider"], "local")
	settings.MediaWatermarkEnabled = parseBool(settingsMap["media.watermark.enabled"], true)
	settings.MediaWatermarkPosition = parseString(settingsMap["media.watermark.position"], "center")
	// PRD: opacity as 0..100 int
	_opacityPct := parseInt(settingsMap["media.watermark.opacity"], 30)
	if _opacityPct < 0 {
		_opacityPct = 0
	}
	if _opacityPct > 100 {
		_opacityPct = 100
	}
	settings.MediaWatermarkOpacity = float64(_opacityPct) / 100.0

	// App settings
	settings.AppName = parseString(settingsMap["app.name"], "لوفت الدغيري")
	settings.AppVersion = parseString(settingsMap["app.version"], "1.0.0")
	settings.AppMaintenanceMode = parseBool(settingsMap["app.maintenance_mode"], false)
	settings.AppRegistrationEnabled = parseBool(settingsMap["app.registration_enabled"], true)

	// Notification settings
	settings.NotificationsEmailEnabled = parseBool(settingsMap["notifications.email_enabled"], true)
	settings.NotificationsSMSEnabled = parseBool(settingsMap["notifications.sms_enabled"], false)
	settings.NotificationsPushEnabled = parseBool(settingsMap["notifications.push_enabled"], false)

	// Security settings
	settings.SecuritySessionTimeout = parseInt(settingsMap["security.session_timeout"], 3600)
	settings.SecurityMaxLoginAttempts = parseInt(settingsMap["security.max_login_attempts"], 5)
	settings.SecurityLockoutDuration = parseInt(settingsMap["security.lockout_duration"], 900)

	// Auction settings
	settings.AuctionsDefaultDuration = parseInt(settingsMap["auctions.default_duration"], 7)
	settings.AuctionsMinBidIncrement = parseFloat(settingsMap["auctions.min_bid_increment"], 10.00)
	settings.AuctionsAutoExtendEnabled = parseBool(settingsMap["auctions.auto_extend_enabled"], true)
	settings.AuctionsAutoExtendDuration = parseInt(settingsMap["auctions.auto_extend_duration"], 300)
	settings.AuctionsAntiSnipingMinutes = parseInt(settingsMap["auctions.anti_sniping_minutes"], 10)

	// VAT and shipping
	settings.VATEnabled = parseBool(settingsMap["vat.enabled"], true)
	settings.VATRate = parseFloat(settingsMap["vat.rate"], 0.15)
	settings.ShippingFreeThreshold = parseFloat(settingsMap["shipping.free_shipping_threshold"], 300.00)
	settings.AuctionsMaxExtensions = parseInt(settingsMap["auctions.max_extensions"], 0)
	settings.PaymentsSessionTTL = parseInt(settingsMap["payments.session_ttl_minutes"], 30)
	settings.PaymentsIdempotencyTTLHours = parseInt(settingsMap["payments.idempotency_ttl_hours"], 24)
	settings.NotificationsEmailRetention = parseInt(settingsMap["notifications.email.retention_days"], 7)
	settings.BidsRateLimitPerMinute = parseInt(settingsMap["bids.rate_limit_per_minute"], 60)
	settings.PaymentsRateLimitPer5Min = parseInt(settingsMap["payments.rate_limit_per_5min"], 5)

	// Stock / Cart settings
	settings.StockCheckoutHoldMinutes = parseInt(settingsMap["stock.checkout_hold_minutes"], 10)
	settings.StockSuppliesHoldMinutes = parseInt(settingsMap["stock.supplies_hold_minutes"], 15)
	settings.StockMaxActiveHoldsPerUser = parseInt(settingsMap["stock.max_active_holds_per_user"], 5)

	settings.LastUpdated = time.Now().UTC()

	// Update settings atomically
	oldSettings := cm.settings
	cm.settings = settings

	// Clear cache on settings change
	cm.cache = make(map[string]interface{})
	cm.lastReload = time.Now().UTC()

	// Notify listeners if settings actually changed
	if oldSettings != nil {
		go cm.notifyListeners(settings)
	}

	return nil
}

// GetSettings returns current system settings (thread-safe)
func (cm *ConfigManager) GetSettings() *SystemSettings {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	// Return a copy to prevent external modifications
	settingsCopy := *cm.settings
	return &settingsCopy
}

// GetCachedValue gets a cached value with TTL
func (cm *ConfigManager) GetCachedValue(key string) (interface{}, bool) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	if time.Since(cm.lastReload) > cm.cacheTTL {
		return nil, false
	}

	value, exists := cm.cache[key]
	return value, exists
}

// SetCachedValue sets a cached value
func (cm *ConfigManager) SetCachedValue(key string, value interface{}) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	cm.cache[key] = value
}

// UpdateSetting updates a single setting in the database
func (cm *ConfigManager) UpdateSetting(ctx context.Context, key, value string) error {
	_, err := cm.db.Exec(ctx, `
		UPDATE system_settings 
		SET value = $1, updated_at = (CURRENT_TIMESTAMP AT TIME ZONE 'UTC') 
		WHERE key = $2
	`, value, key)

	if err != nil {
		return fmt.Errorf("failed to update setting %s: %w", key, err)
	}

	// Reload settings to reflect changes
	go func() {
		time.Sleep(100 * time.Millisecond) // Small delay to ensure DB commit
		if err := cm.LoadSettings(); err != nil {
			log.Printf("Failed to reload settings after update: %v", err)
		}
	}()

	return nil
}

// AddChangeListener adds a listener for settings changes
func (cm *ConfigManager) AddChangeListener(listener ChangeListener) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	cm.listeners = append(cm.listeners, listener)
}

// startHotReload starts the hot-reload mechanism
func (cm *ConfigManager) startHotReload(interval time.Duration) {
	cm.reloadTicker = time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-cm.reloadTicker.C:
				if err := cm.LoadSettings(); err != nil {
					log.Printf("Hot-reload failed: %v", err)
				}
			case <-cm.stopReload:
				return
			}
		}
	}()
}

// StopHotReload stops the hot-reload mechanism
func (cm *ConfigManager) StopHotReload() {
	if cm.reloadTicker != nil {
		cm.reloadTicker.Stop()
	}
	close(cm.stopReload)
}

// notifyListeners notifies all change listeners
func (cm *ConfigManager) notifyListeners(settings *SystemSettings) {
	cm.mutex.RLock()
	listeners := make([]ChangeListener, len(cm.listeners))
	copy(listeners, cm.listeners)
	cm.mutex.RUnlock()

	for _, listener := range listeners {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Config change listener panicked: %v", r)
				}
			}()
			listener(settings)
		}()
	}
}

// setDefaults sets default values when database is unavailable
func (cm *ConfigManager) setDefaults() {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	cm.settings = &SystemSettings{
		WSEnabled:                   true,
		WSMaxConnections:            1000,
		WSHeartbeatInterval:         30,
		PaymentsEnabled:             true,
		PaymentsProvider:            "moyasar",
		PaymentsTestMode:            true,
		PaymentsCurrency:            "SAR",
		CORSAllowedOrigins:          []string{"*"},
		CORSAllowedMethods:          []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		CORSAllowedHeaders:          []string{"Content-Type", "Authorization", "X-Requested-With"},
		CORSMaxAge:                  86400,
		AppName:                     "لوفت الدغيري",
		AppVersion:                  "1.0.0",
		AppMaintenanceMode:          false,
		AppRegistrationEnabled:      true,
		VATEnabled:                  true,
		VATRate:                     0.15,
		ShippingFreeThreshold:       300.00,
		PaymentsSessionTTL:          30,
		PaymentsIdempotencyTTLHours: 24,
		LastUpdated:                 time.Now().UTC(),
	}
}

// Helper parsing functions
func parseBool(value string, defaultValue bool) bool {
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func parseInt(value string, defaultValue int) int {
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func parseInt64(value string, defaultValue int64) int64 {
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func parseFloat(value string, defaultValue float64) float64 {
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func parseString(value string, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func parseStringSlice(value string, defaultValue []string) []string {
	if value == "" {
		return defaultValue
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" { // Skip empty strings
			result = append(result, trimmed)
		}
	}

	// Return default if no valid parts found
	if len(result) == 0 {
		return defaultValue
	}

	return result
}

// Global config manager instance
var (
	globalManager *ConfigManager
	initOnce      sync.Once
)

// Initialize initializes the global config manager
func Initialize(db *sqldb.Database, reloadInterval time.Duration) *ConfigManager {
	initOnce.Do(func() {
		globalManager = NewConfigManager(db, reloadInterval)
	})
	return globalManager
}

// GetGlobalManager returns the global config manager
func GetGlobalManager() *ConfigManager {
	// Note: GetGlobalManager should only be used after Initialize is called
	// Otherwise it will return nil
	return globalManager
}

// GetSettings is a shortcut for GetGlobalManager().GetSettings()
func GetSettings() *SystemSettings {
	return GetGlobalManager().GetSettings()
}

// CreateCORSProvider creates a settings provider for CORS middleware (avoids import cycle)
func CreateCORSProvider() func() interface{} {
	return func() interface{} {
		settings := GetSettings()
		if settings == nil {
			return nil
		}

		// Return CORS configuration compatible with middleware
		return map[string]interface{}{
			"allowed_origins": settings.CORSAllowedOrigins,
			"allowed_methods": settings.CORSAllowedMethods,
			"allowed_headers": settings.CORSAllowedHeaders,
			"max_age":         settings.CORSMaxAge,
		}
	}
}
