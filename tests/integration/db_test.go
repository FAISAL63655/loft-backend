package integration

import (
	"encore.dev/storage/sqldb"
)

// testDB is a package-level handle to the core database for tests.
// Per Encore rules, sqldb.Named must be called at package level.
var testDB = sqldb.Named("coredb")
