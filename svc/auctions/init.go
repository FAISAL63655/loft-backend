package auctions

import (
	"context"
	"fmt"

	"encore.app/pkg/storagegcs"
	"encore.dev/storage/sqldb"
)

// Database connection
var db = sqldb.Named("coredb")

// Encore secrets for GCS configuration
var secrets struct {
	GCSProjectID       string //encore:secret
	GCSBucketName      string //encore:secret
	GCSCredentialsJSON string //encore:secret
}

// Initialize the auction service
func init() {
	// Initialize storage client using Encore secrets
	storageConfig := storagegcs.Config{
		ProjectID:      secrets.GCSProjectID,
		BucketName:     secrets.GCSBucketName,
		CredentialsKey: secrets.GCSCredentialsJSON,
		IsPublic:       false,
	}

	storageClient, err := storagegcs.NewClient(context.Background(), storageConfig)
	if err != nil {
		// In local/dev environments it's common to not have GCS credentials configured.
		// Continue without storage so read-only endpoints work.
		fmt.Printf("[auctions] storage init failed, continuing without GCS (thumbnails disabled): %v\n", err)
		storageClient = nil
	}

	// Create and initialize the service
	service := NewService(db, storageClient)

	// The service is automatically set for API endpoints in NewService
	_ = service
}
