// Package ent provides the generated ent client for SQLFlow.
// Run `go generate ./internal/db/ent` to regenerate.
package ent

// sql/modifier is enabled because the reporting queries need aggregates the
// typed API cannot express — COUNT(DISTINCT x), and ORDER BY on a grouped
// count. Without it those queries would have to stay raw SQL, which is exactly
// what ADR-0010 sets out to eliminate. The modifier is the sanctioned escape
// hatch: still ent, still dialect-aware, just below the typed layer.
//
//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/modifier ./schema
