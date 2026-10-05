# ADR-014: Double-entry Ledger

Status: accepted foundation decision.

Balances derive from signed integer atomic entries. Posting is asset-balanced, wallet overdrafts are denied, transaction entry sets seal at commit, and adjustments/audit are append-only. Synthetic IRR/USD service credits only.

Evidence and remaining validation: [R1 report](../operations/r1-validation.md).
