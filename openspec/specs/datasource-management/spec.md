# Data Source Management

## Purpose

DBA registers target database instances (MySQL / MongoDB) on the platform for use by developers.

## Requirements

### Requirement: Platform SHALL register data sources with encrypted credentials

#### Scenario: DBA registers a new data source
- **WHEN** DBA creates a data source with name, host, port, username, password, database, type (mysql/mongodb), and max connections
- **THEN** the platform stores it with encrypted password and status "active"

### Requirement: Platform SHALL support connection health check

#### Scenario: DBA tests a data source connection
- **WHEN** DBA clicks "test connection" on a data source
- **THEN** the platform attempts to connect and returns success/failure with latency

### Requirement: Platform SHALL list only active data sources with masked credentials

#### Scenario: Authenticated user lists data sources
- **WHEN** any authenticated user lists data sources
- **THEN** only data sources with status "active" are returned, sensitive fields (password) are masked

### Requirement: Platform SHALL support data source configuration editing

#### Scenario: DBA updates a data source
- **WHEN** DBA updates a data source's configuration
- **THEN** the changes are saved and existing connection pools are refreshed

### Requirement: Platform SHALL disable data sources gracefully

#### Scenario: DBA disables a data source
- **WHEN** DBA disables a data source
- **THEN** the data source status becomes "disabled", new queries cannot use it, existing connections are drained

### Requirement: Platform SHALL fetch database tables from data sources

#### Scenario: User requests table list for a data source
- **WHEN** a user requests table list for a data source
- **THEN** the platform returns available tables/databases from the target instance
