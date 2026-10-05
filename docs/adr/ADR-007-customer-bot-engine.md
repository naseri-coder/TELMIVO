# ADR-007: Customer Bot Engine

Status: superseded by R4A runtime convergence.

The original R1 decision introduced a standalone customer-bot process shell as a future
boundary. R4A retires that shell as an executable runtime. Customer Telegram updates now
have one canonical ingress: the API route
`/api/v1/customer-bot-updates/{account}/{bot}`.

The Bot Factory remains the source of encrypted bot credentials and plugin composition.
Connecting a bot verifies the Telegram credential and provisions the webhook; pausing it
deletes the webhook and changes the persisted bot status to `DISABLED`. The public ingress
requires both the derived Telegram secret header and persisted `RUNNING` status before an
update body is dispatched.

The legacy `cmd/customer-bot-runtime` command is retained only as a fail-closed compatibility
stub so old operational references cannot accidentally start a second customer runtime.

See [R4A runtime convergence](../R4A-customer-bot-runtime.md).
