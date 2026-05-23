# Ticket Workflow

## Purpose

DDL/DML changes must go through a ticket workflow: submit → AI review → DBA approval → manual execution.

## Requirements

### Requirement: Platform SHALL create tickets with automatic AI review

#### Scenario: User submits a new ticket
- **WHEN** user submits a ticket with datasource, database, SQL, db_type, and change_reason
- **THEN** ticket is created in SUBMITTED status, AI review is triggered automatically

### Requirement: Platform SHALL enforce ticket state machine transitions

#### Scenario: Ticket transitions through workflow states
- **WHEN** ticket is created
- **THEN** status becomes SUBMITTED
- **WHEN** AI review completes
- **THEN** status becomes AI_REVIEWED
- **WHEN** AI approves
- **THEN** status becomes PENDING_APPROVAL (waiting for DBA)
- **WHEN** DBA approves
- **THEN** status becomes APPROVED
- **WHEN** executor clicks run
- **THEN** status becomes EXECUTING then DONE
- **WHEN** DBA rejects
- **THEN** status becomes REJECTED then DONE
- **WHEN** cancelled (by submitter or DBA before execution)
- **THEN** status becomes CANCELLED then DONE

### Requirement: Platform SHALL allow cancellation before execution

#### Scenario: Ticket is in cancellable status
- **WHEN** ticket is in SUBMITTED, AI_REVIEWED, or PENDING_APPROVAL status
- **THEN** submitter or DBA can cancel with a reason

### Requirement: Platform SHALL require manual confirmation for ticket execution

#### Scenario: Ticket is approved and ready for execution
- **WHEN** ticket is APPROVED
- **THEN** only submitter or dba/admin can click "execute"
- **THEN** execution happens only after manual confirmation

### Requirement: Platform SHALL differentiate simple and standard tickets

#### Scenario: AI review indicates low-impact change
- **WHEN** AI review indicates low-impact change (e.g., add index)
- **THEN** simple ticket (DBA one-click approve)

#### Scenario: AI review indicates high-impact change
- **WHEN** AI review indicates high-impact change
- **THEN** standard ticket (DBA must review details)

### Requirement: Platform SHALL support ticket list with filtering

#### Scenario: User views ticket list
- **WHEN** user views tickets
- **THEN** they can filter by status, data source, submitter, risk level, keyword
- **THEN** "my tickets" and "pending approval" quick filters available
