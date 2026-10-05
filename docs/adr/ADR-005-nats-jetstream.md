# ADR-005: NATS JetStream

Status: accepted foundation decision.

Transactional PostgreSQL outbox publishes durable envelopes. A bound explicit-ack consumer commits inbox plus handler effects before acknowledgement. Retry and DLQ are persisted.

Evidence and remaining validation: [R1 report](../operations/r1-validation.md).
