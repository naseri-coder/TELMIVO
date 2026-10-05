# ADR-004: Redis temporary state

Status: accepted foundation decision.

Redis supplies ephemeral cache and fixed-window rate limiting. Failure denies rate-limited requests. It never owns financial or subscription state.

Evidence and remaining validation: [R1 report](../operations/r1-validation.md).
