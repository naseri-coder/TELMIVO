module telmivo.local/lic-a9-phase31-durable-witness

go 1.23

require (
    telmivo.local/lic-a9-phase28-effect-proof v0.0.0
    telmivo.local/lic-a9-phase29-outcome-ledger v0.0.0
    telmivo.local/lic-a9-phase30-owner-failover v0.0.0
)

replace telmivo.local/lic-a9-phase28-effect-proof => ../phase28_effect_gate_proof
replace telmivo.local/lic-a9-phase29-outcome-ledger => ../phase29_outcome_ledger
replace telmivo.local/lic-a9-phase30-owner-failover => ../phase30_owner_failover
