//go:build integration

package installationlicense

import "testing"

func TestLICA9Phase51NRequiredExplicitTestIdentity(t *testing.T) {
	for _, key := range []string{"TELMIVO_ENV", "TELMIVO_LICENSE_ENVIRONMENT", "SYNTHETIC_MODE"} {
		t.Run("missing_"+key, func(t *testing.T) {
			m := phase51mGoodTargets()
			delete(m, key)
			if phase51mCheck(m) == nil {
				t.Fatal("missing explicit TEST key admitted")
			}
		})
		t.Run("empty_"+key, func(t *testing.T) {
			m := phase51mGoodTargets()
			m[key] = ""
			if phase51mCheck(m) == nil {
				t.Fatal("empty explicit TEST key admitted")
			}
		})
	}
}
