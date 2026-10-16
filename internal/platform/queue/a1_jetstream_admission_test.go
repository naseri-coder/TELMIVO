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

// No real broker is needed: Client.JS is nil, so an incorrectly admitted
// AddStream/ConsumerInfo would panic. Child processes isolate the one-way
// InstallRuntime state machine from the rest of the test suite.
func TestLICA9A1BrokerAdminFenceChild(t *testing.T) {
	if os.Getenv("LIC_A9_A1_BROKER_CHILD") != "YES" {
		return
	}
	var c Client
	for _, phase := range []string{"never_installed", "terminal_restricted"} {
		if phase == "terminal_restricted" {
			installationlicense.InstallRuntime(nil)
		}
		if err := c.Ensure(context.Background()); !errors.Is(err, installationlicense.ErrRestricted) {
			t.Fatalf("%s: AddStream was not denied: %v", phase, err)
		}
		if err := c.Consume(context.Background(), nil, "telmivo.events.test", "test-a1", nil); !errors.Is(err, installationlicense.ErrRestricted) {
			t.Fatalf("%s: ConsumerInfo/AddConsumer was not denied: %v", phase, err)
		}
		if err := c.ConsumeExternal(context.Background(), nil, "telmivo.events.test", "test-a1", nil); !errors.Is(err, installationlicense.ErrRestricted) {
			t.Fatalf("%s: external Consumer setup was not denied: %v", phase, err)
		}
	}
}

func TestLICA9A1JetStreamAdministrativeFailClosed(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestLICA9A1BrokerAdminFenceChild$", "-test.v")
	cmd.Env = append(os.Environ(), "LIC_A9_A1_BROKER_CHILD=YES")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "--- PASS: TestLICA9A1BrokerAdminFenceChild") {
		t.Fatalf("unlicensed JetStream administrative calls escaped fence: %v\n%s", err, out)
	}
}
