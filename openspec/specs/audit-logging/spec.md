# Audit Logging

## Purpose

Full operation logging for queries, changes, exports, and permission modifications.

## Requirements

### Requirement: Platform SHALL audit all SQL query executions

#### Scenario: A SQL query is executed
- **WHEN** a SQL query is executed
- **THEN** audit log records: user, timestamp, data source, database, SQL content, result row count, execution time, review result

### Requirement: Platform SHALL audit all data exports

#### Scenario: Data is exported
- **WHEN** data is exported
- **THEN** audit log records: user, timestamp, data source, SQL, export format, row count

### Requirement: Platform SHALL audit all permission changes

#### Scenario: Admin modifies Casbin policies
- **WHEN** admin modifies Casbin policies
- **THEN** audit log records: operator, timestamp, policy change details

### Requirement: Platform SHALL support filtered audit log queries with immutability

#### Scenario: Admin or DBA views audit logs
- **WHEN** admin/dba views audit logs
- **THEN** logs can be filtered by user, time range, data source, operation type
- **THEN** logs cannot be deleted via API

### Requirement: Platform SHALL audit desensitize bypass actions

#### Scenario: User uses desensitize bypass
- **WHEN** user uses desensitize:bypass
- **THEN** the bypass action is recorded with full context
