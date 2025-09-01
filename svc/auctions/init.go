package auctions

import (
	"encore.dev/storage/sqldb"
)

// Database connection
var db = sqldb.Named("coredb")

// Initialize the auction service
func init() {
	// Create and initialize the service
	service := NewService(db)

	// The service is automatically set for API endpoints in NewService
	_ = service
}
