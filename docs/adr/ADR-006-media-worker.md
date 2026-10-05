# ADR-006: Media Worker

Status: accepted foundation decision.

Synthetic download orchestration runs in media-worker, outside HTTP handlers. FFmpeg has a bounded draft runner; actual media ingestion/delivery remains deferred.

Evidence and remaining validation: [R1 report](../operations/r1-validation.md).
