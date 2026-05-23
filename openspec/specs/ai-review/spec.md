# AI Review

## Purpose

AI-powered SQL review that provides risk assessment, optimization suggestions, and impact analysis before query execution.

## Requirements

### Requirement: Platform SHALL classify SQL risk levels via AI review

#### Scenario: AI reviews a SQL statement
- **WHEN** AI reviews a SQL statement
- **THEN** it classifies risk as low/medium/high based on: operation type, table sensitivity, estimated impact scope, WHERE clause presence, query complexity, and user desensitization permission

### Requirement: Platform SHALL stream AI review results via SSE

#### Scenario: AI review is triggered
- **WHEN** AI review is triggered
- **THEN** review content is streamed via Server-Sent Events (text/event-stream)
- **THEN** final result includes: risk_level, risk_score, decision, summary, suggestions, impact_analysis, rollback_sql, warnings

### Requirement: Platform SHALL make risk-based execution decisions

#### Scenario: Low risk query auto-executes
- **WHEN** risk is low
- **THEN** decision is "execute" (auto-run)

#### Scenario: Medium risk query requires confirmation
- **WHEN** risk is medium
- **THEN** decision is "confirm" (user confirmation required)

#### Scenario: High risk query requires ticket
- **WHEN** risk is high
- **THEN** decision is "ticket" (must submit ticket)

### Requirement: Platform SHALL handle SELECT queries with desensitization context

#### Scenario: User queries sensitive table with bypass permission
- **WHEN** user queries a sensitive table WITH `desensitize:bypass` permission
- **THEN** classified as high risk (can view raw data)

#### Scenario: User queries sensitive table without bypass permission
- **WHEN** user queries a sensitive table WITHOUT bypass
- **THEN** classified as medium risk (results are masked)

### Requirement: Platform SHALL require ticket for all DDL and DML operations

#### Scenario: DDL or DML SQL is submitted
- **WHEN** SQL contains DDL (CREATE/ALTER/DROP) or DML (UPDATE/DELETE)
- **THEN** decision is always "ticket" regardless of risk level
- **THEN** impact analysis includes: estimated affected rows, lock potential, rollback SQL

### Requirement: Platform SHALL fall back to static rule-based review when AI is unavailable

#### Scenario: AI service is unavailable or times out
- **WHEN** AI service is unavailable or times out
- **THEN** fall back to static rule-based review (pattern matching, keyword detection)
- **THEN** indicate review_source as "static" or "fallback" in result

### Requirement: Platform SHALL persist AI review results with expiry

#### Scenario: AI review completes
- **WHEN** AI review completes
- **THEN** result is stored with 30-second expiry for ticket creation
