#!/usr/bin/env python3
"""Non-operational retention capacity estimator; no database/cloud connections."""
import argparse
import json
import math


def estimate(main_mib, license_mib, interval_minutes, nominal_free_gb=1024):
    if interval_minutes not in (5, 10, 15):
        raise ValueError("interval must be one of 5,10,15 minutes")
    for value in (main_mib, license_mib, nominal_free_gb):
        if not math.isfinite(value) or value < 0:
            raise ValueError("sizes must be finite and >=0")
    hot = 1440 // interval_minutes + 1
    main_points = hot + 7 + 4 + 3 + 1
    license_points = 2 + 7 + 4 + 3 + 1
    total_mib = main_points * main_mib + license_points * license_mib
    return {
        "model": "NONPRODUCTION_RETENTION_ESTIMATE_V1",
        "main_interval_minutes": interval_minutes,
        "main_daily_generation_mib": main_mib * (1440 // interval_minutes),
        "main_retained_points_upper_nominal": main_points,
        "license_interval_minutes": 1440,
        "license_daily_generation_mib": license_mib,
        "license_retained_points_upper_nominal": license_points,
        "retained_mib_nominal": round(total_mib, 3),
        "retained_gib_nominal": round(total_mib / 1024, 4),
        "hypothetical_advertised_gb_decimal": nominal_free_gb,
        "fraction_of_hypothetical_quota_percent": round(100 * total_mib * 1048576 / 1e9 / nominal_free_gb, 3) if nominal_free_gb else None,
        "warnings": [
            "Size inputs must be measured compressed, encrypted backup bytes; no measurement performed.",
            "Does not include incomplete uploads, provider versions, evidence, locked objects, independent journal or quota changes.",
            "No deletion may be authorized from this model; TeraBox safety and free quota unqualified.",
        ],
    }


def main():
    parser = argparse.ArgumentParser(description="Offline capacity estimator only")
    parser.add_argument("--main-backup-mib", type=float, required=True)
    parser.add_argument("--license-backup-mib", type=float, default=0.0)
    parser.add_argument("--interval-minutes", type=int, default=10, choices=[5, 10, 15])
    parser.add_argument("--nominal-free-gb", type=float, default=1024)
    args = parser.parse_args()
    try:
        output = estimate(args.main_backup_mib, args.license_backup_mib,
                          args.interval_minutes, args.nominal_free_gb)
    except ValueError as ex:
        parser.error(str(ex))
    print(json.dumps(output, indent=2, sort_keys=True, ensure_ascii=False))

if __name__ == "__main__":
    main()
