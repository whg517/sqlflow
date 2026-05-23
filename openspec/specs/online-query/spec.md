# Online SQL Query

## Purpose

Developers write and execute SQL queries against registered data sources with syntax validation, result display, and pagination.

## Requirements

### Requirement: Platform SHALL execute MySQL SELECT queries with pagination

#### Scenario: Developer submits a valid MySQL SELECT query
- **WHEN** developer submits a valid SELECT query against a MySQL data source
- **THEN** results are displayed in a table with pagination (default 1000 rows), execution time is shown

### Requirement: Platform SHALL enforce query timeout

#### Scenario: Query exceeds configured timeout
- **WHEN** a query exceeds the configured timeout (default 30s)
- **THEN** the query is automatically cancelled and an error message is returned

### Requirement: Platform SHALL validate SQL syntax before execution

#### Scenario: Developer enters SQL in the editor
- **WHEN** developer enters SQL in the editor
- **THEN** syntax errors are highlighted before execution using pingcap/parser AST

### Requirement: Platform SHALL execute MongoDB find queries with pagination

#### Scenario: Developer submits a MongoDB find query
- **WHEN** developer submits a MongoDB find query with filter, projection, sort, and limit
- **THEN** results are displayed in JSON table format with pagination

### Requirement: Platform SHALL restrict MongoDB aggregation to read-only stages

#### Scenario: Developer submits a MongoDB aggregation pipeline
- **WHEN** developer submits an aggregation pipeline
- **THEN** only read-only stages are allowed ($match, $group, $project, $sort, $limit, $skip, $count, $unwind, $addFields)
- **THEN** write stages ($set, $unset, $rename, $out, $merge) are rejected

### Requirement: Platform SHALL require ticket for MongoDB update operations

#### Scenario: Developer attempts a MongoDB update operation
- **WHEN** developer attempts a MongoDB update operation
- **THEN** the operation is not executed directly; user is guided to submit a ticket

### Requirement: Platform SHALL persist query history with filtering

#### Scenario: A query is executed
- **WHEN** a query is executed
- **THEN** the query, results metadata, timestamp, and user info are saved to query history

#### Scenario: User views query history
- **WHEN** user views history
- **THEN** they can filter by date, data source, and keyword

### Requirement: Platform SHALL support data export with row limits

#### Scenario: User exports query results
- **WHEN** user exports query results
- **THEN** results are exported as CSV or JSON, max 10000 rows
- **THEN** the export action is recorded in audit logs

### Requirement: Platform SHALL trigger AI review before query execution

#### Scenario: User clicks execute on a query
- **WHEN** user clicks execute on a query
- **THEN** AI review is triggered (SSE streaming), risk level and decision are returned

#### Scenario: AI review decision is execute (low risk)
- **WHEN** decision is "execute" (low risk)
- **THEN** query runs automatically

#### Scenario: AI review decision is confirm (medium risk)
- **WHEN** decision is "confirm" (medium risk)
- **THEN** user must confirm before execution

#### Scenario: AI review decision is ticket (high risk)
- **WHEN** decision is "ticket" (high risk)
- **THEN** user is prompted to submit a ticket

### Requirement: Platform SHALL apply data masking on query results

#### Scenario: Query results contain fields with masking rules
- **WHEN** query results contain fields with masking rules
- **THEN** data is masked according to configured rules unless user has `desensitize:bypass` permission
