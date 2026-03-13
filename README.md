# Multi-Tenant Notification Platform

A production-style backend platform for sending notifications across channels with strong reliability, tenant isolation, and observability.

## Overview

This project provides a reusable notification service that allows tenants to:

- manage notification templates
- submit notification requests
- deliver messages asynchronously across channels
- enforce quotas and rate limits
- track delivery state
- inspect and replay dead-lettered jobs
- audit configuration changes

The initial MVP focuses on **email** and **webhook** delivery.

## Goals

- Provide a clean, tenant-scoped API for notification submission
- Support asynchronous processing for reliable delivery
- Prevent duplicate sends with idempotency keys
- Handle transient failures with bounded retries and exponential backoff
- Route terminal failures to a dead-letter queue
- Expose strong operational visibility through logs, metrics, and traces

## Non-Goals (MVP)

- Rich UI dashboard
- Full end-user identity management
- SMS/push provider integrations
- Complex scheduling and campaign management
- Exactly-once delivery guarantees

## Core Requirements

### Functional
- Create and manage tenants
- Create and manage templates
- Submit notifications
- Deliver through email and webhook channels
- Track notification and delivery attempt status
- Replay dead-lettered notifications

### Non-Functional
- Low-latency submission path
- At-least-once asynchronous processing
- Tenant isolation
- Idempotent request handling
- Observable request and worker flows
- Recoverable failure handling

## Architecture

### Components

#### API Service
Handles:
- API key authentication
- tenant-scoped access control
- template CRUD
- notification submission
- notification status reads
- operational endpoints

#### Dispatcher
Transforms accepted notification requests into channel-specific delivery jobs and places them onto the queue.

#### Channel Workers
Separate workers process channel jobs:
- email worker
- webhook worker

Workers:
- render templates
- attempt delivery
- apply retry policy
- emit logs, metrics, and traces
- update delivery status

#### Rate Limit / Quota Layer
Enforces:
- tenant daily quota
- per-tenant submission rate

#### Dead-Letter Processor
Captures jobs that exceed retry policy and supports replay.

## High-Level Flow

1. Client submits `POST /v1/notifications`
2. API authenticates tenant and validates request
3. API persists the notification intent and idempotency key
4. Dispatcher creates channel-specific jobs
5. Workers process deliveries asynchronously
6. Delivery attempt results are recorded
7. Terminal failures are moved to the dead-letter queue
8. Operators can inspect and replay failed jobs

## Status Model

### Notification Status
- `accepted`
- `processing`
- `partially_delivered`
- `delivered`
- `failed`
- `dead_lettered`

### Delivery Attempt Status
- `pending`
- `in_progress`
- `sent`
- `delivered`
- `failed`
- `retry_scheduled`
- `dead_lettered`

## API Summary

### Tenants
- `POST /v1/tenants`
- `GET /v1/tenants/{tenantId}`

### Templates
- `POST /v1/templates`
- `GET /v1/templates/{templateId}`
- `PUT /v1/templates/{templateId}`

### Notifications
- `POST /v1/notifications`
- `GET /v1/notifications/{notificationId}`
- `POST /v1/notifications/{notificationId}/replay`

### Operations
- `GET /v1/tenants/{tenantId}/usage`
- `GET /v1/dead-letters`
- `GET /v1/health`
- `GET /v1/readiness`

## Example Notification Request

```json
{
  "tenant_id": "acme",
  "template_id": "password-reset",
  "channels": ["email", "webhook"],
  "recipient": {
    "email": "user@example.com"
  },
  "variables": {
    "first_name": "Sam",
    "reset_url": "https://example.com/reset/abc"
  },
  "idempotency_key": "c8eb3b4c-0a4b-4f0d-a7d5-4a4d3baf4e98"
}