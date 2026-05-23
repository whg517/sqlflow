# User Authentication

## Purpose

Username + password + JWT authentication. Initial admin account configured via startup parameters.

## Requirements

### Requirement: Platform SHALL create initial admin account on first startup

#### Scenario: Server starts for the first time
- **WHEN** server starts for the first time
- **THEN** admin account is created from `ADMIN_USERNAME` and `ADMIN_PASSWORD` env vars (or CLI flags)

#### Scenario: Server starts with existing admin account
- **WHEN** admin already exists on subsequent starts
- **THEN** skip creation

### Requirement: Platform SHALL authenticate users with username and password

#### Scenario: User submits login credentials
- **WHEN** user submits username and password
- **THEN** password is verified against bcrypt hash
- **THEN** JWT (HS256, 24h expiry) is issued and returned

### Requirement: Platform SHALL validate JWT tokens for authenticated endpoints

#### Scenario: Authenticated endpoint receives request with valid Bearer token
- **WHEN** authenticated endpoint receives request with valid Bearer token
- **THEN** request proceeds with user context (id, username, role)

### Requirement: Platform SHALL enforce password policy

#### Scenario: User sets or changes password
- **WHEN** user sets or changes password
- **THEN** password must be 8-128 characters, containing both letters and numbers

### Requirement: Platform SHALL support authenticated password change

#### Scenario: Authenticated user changes their password
- **WHEN** authenticated user changes their own password
- **THEN** old password is verified, new password meets policy, stored as bcrypt hash

### Requirement: Platform SHALL support admin user CRUD operations

#### Scenario: Admin creates a user
- **WHEN** admin creates a user with username, password, and role
- **THEN** user is stored with bcrypt-hashed password

#### Scenario: Admin updates a user
- **WHEN** admin updates user
- **THEN** username/role can be changed

#### Scenario: Admin deletes a user
- **WHEN** admin deletes user
- **THEN** soft delete (status change)

#### Scenario: Admin resets user password
- **WHEN** admin resets password
- **THEN** new password must meet policy
