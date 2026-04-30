//go:build !test_native_sql

package itest

// bolt12DevMigrations is false because this build does not apply the
// development migrations that hold the BOLT 12 tables.
const bolt12DevMigrations = false
