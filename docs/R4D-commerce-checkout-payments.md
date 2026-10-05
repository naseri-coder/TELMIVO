# R4D — Commerce Checkout & Payments

Base checkpoint: frozen R4C Uploader & Storage Platform.

R4D extends the existing Commerce plugin. It does not create a second order system and does
not move platform billing into customer commerce. Existing orders retain immutable product,
quantity and atomic-money snapshots; checkout references an existing PLACED order.

## Rails and scope

Two checkout rails are modeled:

- `TELEGRAM_STARS`: only for orders priced in integer XTR where every immutable order line
  is marked digital. TELMIVO creates a Telegram invoice link, validates pre-checkout payload,
  payer, currency and amount, and accepts a payment only from a verified
  `successful_payment` update.
- `EXTERNAL`: a provider-neutral interface. Core stores the provider key and durable payment
  state, but no external vendor is hard-coded or enabled by R4D. A concrete provider adapter
  must implement create, refund and reconciliation contracts separately.

The internal TELMIVO service-credit wallet remains separate from both rails.

## External-effect safety

A payment intent is durable before any invoice/provider creation call. A refund record and
`REFUND_PENDING` payment state are durable before any refund call.

If the process or transport can no longer prove whether an external effect happened, the
intent is sealed as `UNKNOWN` or `REFUND_UNKNOWN`. Replaying the same idempotency key does
not repeat the external effect. Resolution is by verified provider event or explicit
reconciliation.

Deterministic provider rejection becomes `FAILED`; verified payment becomes `PAID`.
Successful payment automatically moves the linked PLACED order to CONFIRMED and appends the
existing immutable order history.

## Telegram Stars verification

The customer-bot webhook accepts `pre_checkout_query` and `successful_payment` updates.
The trusted Core checks:

- payload is the opaque TELMIVO payment identifier;
- rail/provider are Telegram Stars / Telegram;
- currency is XTR;
- atomic amount exactly matches the immutable order total;
- payer Telegram identity matches the CRM identity captured for the order.

Only then is the payment marked PAID. Pre-checkout mismatches fail closed without mutating
payment state; accepted pre-checkout and verified/rejected successful-payment evidence are
recorded in the payment event ledger.

Telegram Stars refunds are full refunds because the Bot API refund primitive is full-payment.
Refund outcomes are persisted as acknowledged, rejected or unknown.

## Persistence and tenant isolation

Migration 0021 adds `commerce_payment_intents`, append-only
`commerce_payment_events` and `commerce_refunds`. All are FORCE RLS tenant scoped.
Payment/order/contact identity and atomic amount are immutable after creation. A partial
unique index prevents more than one active/unknown payment lifecycle for an order.

Payment events are replay-safe by event key plus digest. Provider payment identifiers are
unique per bot/provider when known.

## Mini App and API

The Persian RTL Commerce view can:

- mark products as digital;
- start Telegram Stars checkout for eligible digital XTR orders;
- request provider-neutral external checkout for other orders;
- open returned checkout URLs;
- inspect payment state;
- request a full refund after verified payment;
- explicitly reconcile ambiguous external outcomes.

The API never accepts a client-supplied price for checkout or refund.

## Validation boundary

R4D CI covers Go format/vet/race tests/build, migration 0021, payment/refund integration,
native Persian browser acceptance, security baseline, image/Compose checks and fresh
backup/restore recovery. Tests use synthetic transports and do not create real Telegram
charges or contact a real payment provider.

R4D does not authorize production deployment, settlement, merchant onboarding, payout,
chargeback handling, taxes, FX conversion or a concrete external provider.
