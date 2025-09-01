package e2e

import (
	"encore.dev/storage/sqldb"
)

// testDB is a package-level handle to the core database for E2E tests.
var testDB = sqldb.Named("coredb")
