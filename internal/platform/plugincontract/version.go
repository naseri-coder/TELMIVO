package plugincontract

import (
	"strconv"
	"strings"
)

// VersionRange is [minimum, maximum_exclusive). Only stable x.y.z versions are supported.
type VersionRange struct {
	Minimum          string `json:"minimum"`
	MaximumExclusive string `json:"maximum_exclusive,omitempty"`
}

func parseVersion(raw string) ([3]uint64, error) {
	var v [3]uint64
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return v, ErrVersion
	}
	for i, p := range parts {
		if p == "" || (len(p) > 1 && p[0] == '0') {
			return v, ErrVersion
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return v, ErrVersion
			}
		}
		n, err := strconv.ParseUint(p, 10, 32)
		if err != nil {
			return v, ErrVersion
		}
		v[i] = n
	}
	return v, nil
}

func compare(a, b [3]uint64) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func ValidVersion(raw string) bool { _, err := parseVersion(raw); return err == nil }

func (r VersionRange) Validate() error {
	lo, err := parseVersion(r.Minimum)
	if err != nil {
		return err
	}
	if r.MaximumExclusive != "" {
		hi, err := parseVersion(r.MaximumExclusive)
		if err != nil || compare(lo, hi) >= 0 {
			return ErrVersion
		}
	}
	return nil
}

func (r VersionRange) Allows(raw string) bool {
	if r.Validate() != nil {
		return false
	}
	v, err := parseVersion(raw)
	if err != nil {
		return false
	}
	lo, _ := parseVersion(r.Minimum)
	if compare(v, lo) < 0 {
		return false
	}
	if r.MaximumExclusive != "" {
		hi, _ := parseVersion(r.MaximumExclusive)
		return compare(v, hi) < 0
	}
	return true
}
