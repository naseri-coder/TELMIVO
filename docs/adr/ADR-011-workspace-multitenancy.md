# ADR-011: Workspace multi-tenancy

Status: accepted foundation decision.

Membership authorization precedes tenant context, with transaction-local scope, FORCE RLS and composite foreign keys. Migration role is separate. Platform global plan usage uses a permission-checked boolean definer function.

Evidence and remaining validation: [R1 report](../operations/r1-validation.md).
