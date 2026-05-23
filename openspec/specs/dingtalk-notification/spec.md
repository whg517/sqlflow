# DingTalk Notification

## Purpose

DingTalk webhook notifications for ticket status changes and high-risk operation alerts.

## Requirements

### Requirement: Platform SHALL send DingTalk notifications on ticket status changes

#### Scenario: Ticket status changes
- **WHEN** ticket status changes (submitted, approved, rejected, executed)
- **THEN** DingTalk webhook sends notification with: operator, SQL summary, risk level, ticket link

### Requirement: Platform SHALL send real-time alerts for high-risk operations

#### Scenario: Medium or high risk operation occurs
- **WHEN** a medium or high risk operation occurs
- **THEN** real-time alert is sent to DBA DingTalk group

### Requirement: Platform SHALL support DingTalk webhook configuration

#### Scenario: Admin configures webhook URL and secret
- **WHEN** admin configures DingTalk webhook URL and secret in settings
- **THEN** notifications are enabled; test notification can be sent

### Requirement: Platform SHALL degrade gracefully when DingTalk notification fails

#### Scenario: DingTalk notification fails
- **WHEN** DingTalk notification fails (network error, invalid webhook)
- **THEN** the operation itself is not affected; error is logged
