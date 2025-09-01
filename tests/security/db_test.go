package security

import "encore.dev/storage/sqldb"

// testDB is a package-level handle to the core database for security tests.
var testDB = sqldb.Named("coredb")
