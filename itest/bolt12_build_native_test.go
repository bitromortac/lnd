//go:build test_native_sql

package itest

// bolt12DevMigrations is true because this build applies the development
// migrations that hold the BOLT 12 tables.
const bolt12DevMigrations = true
