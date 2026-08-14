package db

// raw_sql.go — Raw SQL Preservation Registry
//
// This file documents all raw SQL that will remain after the ent migration.
// Each entry explains WHY raw SQL is preserved instead of using ent Client.
//
// Phase 2 service migration must reference this file when deciding whether
// a database call can be migrated to ent or must remain as raw SQL.

// ========================================================================
// RAW SQL PRESERVED — Permanent Exclusions
// ========================================================================

// 1. CasbinRule (casbin_rule table)
//    Reason: Casbin adapter is a mature standard implementation. Rewriting
//            it to use ent is high risk, low value. The generic v0-v5 column
//            structure conflicts with ent's strong typing design.
//    Owner:  internal/service/permission.go (Casbin adapter)
//    Action: Preserve existing adapter. Inject ent's underlying *sql.DB
//            connection pool so Casbin and ent share the same pool.
//    Note:   CasbinRule does NOT have an ent schema definition.

// ========================================================================
// RAW SQL PRESERVED — Complex Query Fallbacks (Phase 2 evaluation)
// ========================================================================

// The following scenarios MAY use raw SQL fallback via ent.Client.QueryContext()
// if ent's query builder cannot express them efficiently:
//
// 2. Audit report aggregations
//    Reason: Complex GROUP BY + HAVING + multi-table JOIN aggregations
//    Owner:  internal/service/audit_report.go
//    Fallback: ent.Client.QueryContext() → raw SQL
//
// 3. Dashboard statistics
//    Reason: Complex aggregations + time window calculations
//    Owner:  internal/service/dashboard.go
//    Fallback: ent.Client.QueryContext() → raw SQL
//
// Note: ADR-0009 removed the SQLite-specific items that used to live here.
// Full-text search now uses PostgreSQL tsvector + pg_trgm, and backups are
// produced by pg_dump in internal/ops/backup.go — neither is raw SQL anymore.

// ========================================================================
// RAW SQL USAGE RULES
// ========================================================================
//
// - All raw SQL must go through ent's underlying *sql.DB (via DB.DB field)
//   to maintain connection pool consistency.
// - Each raw SQL usage must have a comment: // RAW_SQL: <reason>
// - Raw SQL must have unit test coverage.
// - New raw SQL additions require updating this file.
