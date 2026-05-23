# Data Masking

## Purpose

Field-level data masking for query results and exports, controlled by Casbin RBAC permissions.

## Requirements

### Requirement: Platform SHALL provide built-in masking types

#### Scenario: Admin configures masking rules
- **WHEN** admin configures masking rules
- **THEN** available types include: phone (前3后4), ID card, name (姓*), email, bank card, address, full mask, custom regex

### Requirement: Platform SHALL support table-level sensitivity marking

#### Scenario: Admin marks a table as sensitive
- **WHEN** admin marks a table as sensitive
- **THEN** all queries against that table are subject to masking rules

### Requirement: Platform SHALL support field-level masking rules

#### Scenario: Admin configures a masking rule for a specific field
- **WHEN** admin configures a masking rule for a specific field
- **THEN** query results for that field are masked according to the rule type

### Requirement: Platform SHALL apply masking by default on display

#### Scenario: Query results are displayed or exported
- **WHEN** query results are displayed or exported
- **THEN** masking is applied by default to all fields with configured rules

### Requirement: Platform SHALL enforce bypass permission control for masking

#### Scenario: User has desensitize bypass permission
- **WHEN** user has Casbin `desensitize:bypass` permission
- **THEN** user can view original unmasked data
- **THEN** all bypass operations are recorded in audit logs

### Requirement: Platform SHALL apply masking on data exports

#### Scenario: User exports data without bypass permission
- **WHEN** user exports data
- **THEN** masked data is exported by default

#### Scenario: User exports data with bypass permission
- **WHEN** user has bypass permission and actively checks "export raw data"
- **THEN** raw data is exported
