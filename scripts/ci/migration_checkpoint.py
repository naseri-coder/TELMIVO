"""Exact version-scoped source migration chains, never an >= count check."""
CHECKPOINTS = {
    'R5C': (27, '0027_apple_identity_verification.sql'),
    'R4G': (23, '0023_ai_assistant_2.sql'),
    'R5B_REMEDIATED': (26, '0026_ai_ingestion_receipts.sql'),
    'R5B': (25, '0025_apple_consumer_assistance.sql'),
}

def validate_chain(names: list[str], checkpoint: str) -> None:
    if checkpoint not in CHECKPOINTS:
        raise ValueError(f'unknown migration checkpoint {checkpoint}')
    count, final = CHECKPOINTS[checkpoint]
    if len(names) != count:
        raise ValueError(f'expected {count} migrations for {checkpoint}, found {len(names)}')
    for index, name in enumerate(names, start=1):
        if not name.startswith(f'{index:04d}_') or not name.endswith('.sql'):
            raise ValueError(f'migration gap/duplicate/order mismatch: {name}')
    if names[-1] != final:
        raise ValueError(f'unexpected final migration: {names[-1]}')
