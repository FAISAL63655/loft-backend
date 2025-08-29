// Package coredb provides database connection and utilities for the Loft Dughairi platform.
package coredb

import (
	"encore.dev/storage/sqldb"
)

// DB is the core database instance for the Loft Dughairi platform.
// It uses PostgreSQL as the underlying database engine.
var DB = sqldb.NewDatabase("coredb", sqldb.DatabaseConfig{
	Migrations: "./migrations",
})
