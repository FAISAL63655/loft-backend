// Package config - Example usage of dynamic configuration system
package config

import (
	"context"
	"log"
	"time"

	"encore.app/pkg/middleware"
	"encore.dev/storage/sqldb"
)

// ExampleUsage demonstrates how to use the dynamic configuration system
func ExampleUsage() {
	// Note: In a real service, you would get db from service definition
	// Example: var db = sqldb.Named("coredb")
	var db *sqldb.Database // This would be passed from service

	// Initialize the global config manager with hot-reload every 5 minutes
	manager := Initialize(db, 5*time.Minute)

	// Example 1: Get current settings
	settings := manager.GetSettings()
	log.Printf("Current app name: %s", settings.AppName)
	log.Printf("VAT enabled: %v, rate: %.2f", settings.VATEnabled, settings.VATRate)
	log.Printf("CORS allowed origins: %v", settings.CORSAllowedOrigins)

	// Example 2: Listen for settings changes
	manager.AddChangeListener(func(newSettings *SystemSettings) {
		log.Printf("Settings changed! New app name: %s", newSettings.AppName)
		log.Printf("CORS origins updated: %v", newSettings.CORSAllowedOrigins)
	})

	// Example 3: Update a setting programmatically
	ctx := context.Background()
	if err := manager.UpdateSetting(ctx, "app.name", "لوفت الدغيري - محدث"); err != nil {
		log.Printf("Failed to update setting: %v", err)
	}

	// Example 4: Use with CORS middleware (dynamic)
	// Create CORS provider to avoid import cycle
	corsProvider := func() *middleware.CORSConfig {
		settings := GetSettings()
		if settings == nil {
			return nil
		}
		return &middleware.CORSConfig{
			AllowedOrigins: settings.CORSAllowedOrigins,
			AllowedMethods: settings.CORSAllowedMethods,
			AllowedHeaders: settings.CORSAllowedHeaders,
			MaxAge:         settings.CORSMaxAge,
		}
	}

	corsConfig := middleware.NewDynamicCORSConfig(corsProvider)
	log.Printf("Dynamic CORS config created with %d allowed origins",
		len(corsConfig.AllowedOrigins))

	// Example 5: Create HTTP server with dynamic CORS
	log.Printf("Server would start on :8080 with dynamic CORS")
	log.Printf("Middleware chain: CORS -> Security -> Logging -> Recovery")

	// Uncomment to actually start server:
	// mux := http.NewServeMux()
	// mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
	//     w.WriteHeader(http.StatusOK)
	//     w.Write([]byte("OK"))
	// })
	//
	// corsMiddleware := middleware.CORSMiddleware(corsConfig)
	// handler := corsMiddleware(
	//     middleware.SecurityHeadersMiddleware(middleware.DefaultSecurityConfig)(
	//         middleware.LoggingMiddleware()(
	//             middleware.RecoveryMiddleware()(mux),
	//         ),
	//     ),
	// )
	// server := &http.Server{
	//     Addr:    ":8080",
	//     Handler: handler,
	// }
	// server.ListenAndServe()
}

// ExampleConfigValues shows how to access common configuration values
func ExampleConfigValues() {
	settings := GetSettings()

	// Payment configuration
	if settings.PaymentsEnabled {
		log.Printf("Payments enabled with provider: %s", settings.PaymentsProvider)
		if settings.PaymentsTestMode {
			log.Printf("Running in test mode")
		}
	}

	// VAT calculation example
	netAmount := 100.00
	if settings.VATEnabled {
		vatAmount := netAmount * settings.VATRate
		totalAmount := netAmount + vatAmount
		log.Printf("Net: %.2f %s, VAT (%.0f%%): %.2f %s, Total: %.2f %s",
			netAmount, settings.PaymentsCurrency,
			settings.VATRate*100, vatAmount, settings.PaymentsCurrency,
			totalAmount, settings.PaymentsCurrency)
	}

	// Shipping threshold check
	cartTotal := 250.00
	if cartTotal >= settings.ShippingFreeThreshold {
		log.Printf("Free shipping applied (cart: %.2f >= threshold: %.2f)",
			cartTotal, settings.ShippingFreeThreshold)
	}

	// Media upload limits
	log.Printf("Max file size: %d bytes", settings.MediaMaxFileSize)
	log.Printf("Allowed file types: %v", settings.MediaAllowedTypes)

	// Security settings
	log.Printf("Session timeout: %d seconds", settings.SecuritySessionTimeout)
	log.Printf("Max login attempts: %d", settings.SecurityMaxLoginAttempts)
}

// ExampleCachedOperations shows how to use caching for expensive operations
func ExampleCachedOperations() {
	manager := GetGlobalManager()

	// Check cache first
	if value, exists := manager.GetCachedValue("expensive_calculation"); exists {
		log.Printf("Using cached value: %v", value)
		return
	}

	// Perform expensive operation
	result := "expensive_result"
	time.Sleep(100 * time.Millisecond) // Simulate work

	// Cache the result
	manager.SetCachedValue("expensive_calculation", result)
	log.Printf("Calculated and cached: %v", result)
}
