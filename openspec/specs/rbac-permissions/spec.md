# RBAC Permissions (Casbin)

## Purpose

Casbin RBAC with domains for fine-grained access control across data sources.

## Requirements

### Requirement: Platform SHALL initialize with three built-in roles

#### Scenario: System initializes for the first time
- **WHEN** system initializes
- **THEN** three roles exist: admin (full access + user management), dba (full data access + approval), developer (read-only on authorized data sources)

### Requirement: Platform SHALL enforce domain-based permission isolation

#### Scenario: Permissions are evaluated for a request
- **WHEN** permissions are evaluated
- **THEN** domain (data source) is used as isolation boundary
- **THEN** user can only access tables they have explicit permission for in each data source

### Requirement: Platform SHALL support fine-grained permission configuration

#### Scenario: Admin configures permissions
- **WHEN** admin configures permissions
- **THEN** permissions are defined as: role → domain(data source) → object(table) → action(select/update/delete/ddl/export/desensitize:bypass)
- **THEN** wildcard `*` is supported for object and action

### Requirement: Platform SHALL protect sensitive tables by default

#### Scenario: A table is marked as sensitive
- **WHEN** a table is marked as sensitive
- **THEN** no role has access by default; admin must explicitly grant

### Requirement: Platform SHALL support desensitize bypass permission

#### Scenario: User has desensitize bypass permission
- **WHEN** user has `desensitize:bypass` action permission
- **THEN** user can view unmasked data regardless of role
- **THEN** this permission is decoupled from role — any user can be granted it

### Requirement: Platform SHALL support runtime policy management with audit logging

#### Scenario: Admin adds or removes Casbin policies
- **WHEN** admin adds/removes Casbin policies
- **THEN** changes are reflected immediately (no restart required)
- **THEN** all policy changes are recorded in audit logs
