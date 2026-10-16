package queue

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
)

// An unconfigured JetStream transport must never be touched if the final
// process-wide runtime gate denies the publish. This test needs no NATS server.
// Fresh child processes isolate the one-way runtime installation lifecycle.
func TestLICA9Phase51EPublishDeniedChild(t *testing.T) {
	if os.Getenv("LIC_A9_PHASE51E_PUBLISH_CHILD") != "YES" {
		return
	}
	var client Client // JS intentionally nil: any PublishMsg call would panic.
	ctx := context.Background()
	event := Envelope{
		ID:          "bd082333-5953-440a-a4d2-a333b0999831",
		WorkspaceID: "d0923dfc-e30c-4a76-89b3-be6796f44f6a",
		Type:        "test.only",
	}
	for _, scenario := range []string{"preinstall", "terminal-restricted"} {
		if scenario == "terminal-restricted" {
			installationlicense.InstallRuntime(nil)
		}
		err := client.Publish(ctx, "telmivo.events.phase51e", event)
		if !errors.Is(err, installationlicense.ErrRestricted) {
			t.Fatalf("%s: Publish escaped process license gate: %v", scenario, err)
		}
	}
}

func TestLICA9Phase51EJetStreamFinalPublishRuntimeGate(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestLICA9Phase51EPublishDeniedChild$", "-test.v")
	cmd.Env = append(os.Environ(), "LIC_A9_PHASE51E_PUBLISH_CHILD=YES")
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "--- PASS: TestLICA9Phase51EPublishDeniedChild") {
		t.Fatalf("final signed Runtime gate: %v\n%s", err, output)
	}
}
