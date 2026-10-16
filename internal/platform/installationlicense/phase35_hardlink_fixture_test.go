package installationlicense

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

// phase35RequireHardlinkFixture tests the actual hardlink-denial property when
// the filesystem supports constructing the attack. Android/Termux may deny
// *all* same-UID hardlinks at the kernel or SELinux boundary. In that case
// the attack cannot be constructed and the case must be SKIPPED, never PASS.
// Linux and all other platforms retain their original strict failure policy.
func phase35RequireHardlinkFixture(t *testing.T, source, alias string) {
	t.Helper()
	if err := os.Link(source, alias); err != nil {
		if runtime.GOOS != "android" || (!errors.Is(err, syscall.EPERM) && !errors.Is(err, syscall.EACCES)) {
			t.Fatalf("hardlink attack fixture could not be created: %v", err)
		}
		// Independently probe ordinary same-UID links across two new temp
		// directories on the SAME Go test filesystem. If this succeeds, do
		// NOT waive failure to link the protected licensing file.
		probeSrc := filepath.Join(t.TempDir(), "ordinary-source")
		probeDst := filepath.Join(t.TempDir(), "ordinary-alias")
		if probeErr := os.WriteFile(probeSrc, []byte("non-secret-hardlink-probe"), 0600); probeErr != nil {
			t.Fatalf("cannot create independent hardlink capability probe: %v", probeErr)
		}
		probeErr := os.Link(probeSrc, probeDst)
		if probeErr == nil {
			t.Fatalf("ordinary Android hardlinks succeed, but licensing-file hardlink failed: %v", err)
		}
		if !errors.Is(probeErr, syscall.EPERM) && !errors.Is(probeErr, syscall.EACCES) {
			t.Fatalf("Android capability probe returned an unrelated error: %v (fixture: %v)", probeErr, err)
		}
		t.Skipf("LIC_A9_ANDROID_HARDLINK_FIXTURE_UNAVAILABLE: kernel denied both ordinary and protected hardlinks; Linux hardlink security test remains required (fixture=%v; probe=%v)", err, probeErr)
	}
}
