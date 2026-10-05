# R4E — Automation 2.0

Base checkpoint: stacked R4D Commerce Checkout & Payments source checkpoint.

R4E extends the existing bounded Automation domain. It does not create a second workflow engine,
job system, scripting runtime or arbitrary integration surface.

## Scope

Automation 2.0 adds four scheduling modes on top of the existing durable workflow/execution model:

- event workflows with bounded delay up to 30 days;
- standard five-field Cron recurrence with an explicit IANA timezone;
- SLA timers for ticket events;
- durable schedule materialization by the existing worker.

Cron uses a persisted `automation_schedule_state` row per workflow. The worker materializes one
idempotent execution for a due occurrence and advances the next occurrence in the same database
transaction. Missed intervals do not produce an unbounded catch-up storm: after one due occurrence
is materialized, the next occurrence is calculated from current authoritative time.

## Bounded scheduled context

A Cron workflow may carry only explicit TELMIVO context: a ticket reference, contact identifier or
Telegram user ID. Those values are resolved inside the same workspace/bot scope before execution.
No URL, user-provided code, shell command, arbitrary SQL or arbitrary webhook is accepted.

Event and SLA workflows obtain their context from the authoritative business event.

## SLA safety

SLA execution is durable before its deadline. Immediately before action execution the engine reads
the current ticket state again. A ticket already `RESOLVED` or `CLOSED` cancels the timer with
`SLA_RESOLVED`; no late side effect is performed.

## External-effect safety

R4E keeps the existing action reservation rules. Interrupted or uncertain external effects are not
blindly replayed. Cron occurrence identity is deterministic per workflow and due timestamp.
Existing loop-depth, entitlement, permission, quota and action-count limits remain in force.

## Persistence

Migration 0022 adds `automation_schedule_state` with FORCE RLS, composite tenant ownership,
immutable scope, monotonic revision, next/last occurrence state and a due-time index.

The existing `automation_executions.available_at` remains the authoritative delay/deadline field;
R4E does not introduce a parallel delayed-job table.

## Validation boundary

R4E validation covers Cron parsing, recurrence advancement, delayed manual execution, SLA
cancellation, tenant-scoped browser configuration and fresh backup/restore of schedule state.

Validation uses synthetic Telegram transport and disposable PostgreSQL/Redis/NATS resources.
R4E does not authorize production deployment.
