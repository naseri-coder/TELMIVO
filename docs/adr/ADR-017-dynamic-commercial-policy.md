# ADR-017 — Shared dynamic commercial policy and official entry points

Status: accepted for R2A foundation.

Official bots and Mini App are independent discovery transports sharing the unique Telegram/platform identity. Trial enrollment is a separate, versioned grant snapshot; a trial is never a plan. Group claims require verified administration authority and unique external group ownership.

Use PostgreSQL immutable typed policy versions, subject enrollments and durable usage/reservations. Reuse plan/feature/plugin/RBAC catalogs. Runtime, reservations and admin Preview share one resolver. Commercial combination strategies are explicit; read-only infrastructure caps apply last and cannot be disabled by policy administration.

Calendar/rolling usage, retained capacity, item size and concurrency remain separate measurements. Versioned offer/price references preserve historical values. New policies and existing catalog controls record revision audit evidence. Redis remains optional/non-authoritative and no policy cache is added.

Consequences: product adapters must supply trusted resource measurements and honor reservation expiry; complete media/payment/group products are outside R2A. Advanced configuration uses a lightweight Persian admin form and typed JSON bundle editor. Scope does not include rewriting active enrollment history, purchasing add-ons, or production integrations.
