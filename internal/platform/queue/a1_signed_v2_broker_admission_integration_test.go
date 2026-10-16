//go:build integration

package queue

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/naseri-coder/TELMIVO/internal/platform/installationlicense"
	"github.com/nats-io/nats.go"
)

type a1SignedV2BrokerProbe struct {
	nats.JetStreamContext
	streamCreates int
	consumerReads int
	stop          error
}

func (p *a1SignedV2BrokerProbe) AddStream(*nats.StreamConfig, ...nats.JSOpt) (*nats.StreamInfo, error) {
	p.streamCreates++
	return &nats.StreamInfo{}, nil
}
func (p *a1SignedV2BrokerProbe) ConsumerInfo(string, string, ...nats.JSOpt) (*nats.ConsumerInfo, error) {
	p.consumerReads++
	return nil, p.stop
}

func TestLICA9A1SignedV2NATSAdminAdmissionChild(t *testing.T) {
	if os.Getenv("LIC_A9_A1_SIGNED_CHILD") != "YES" {
		return
	}
	t.Setenv("TELMIVO_ENV", "test")
	t.Setenv("TELMIVO_LICENSE_ENVIRONMENT", "TEST")
	t.Setenv("SYNTHETIC_MODE", "true")
	t.Setenv("TELMIVO_LIC_A9_NATIVE_FIXTURE", "YES")
	t.Setenv("TEST_DATABASE_URL", "postgres://telmivo_runtime:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable")
	t.Setenv("TEST_MIGRATION_DATABASE_URL", "postgres://telmivo_migration:synthetic@127.0.0.1:5432/telmivo_test?sslmode=disable")
	m, err := installationlicense.NewTestOnlySignedV2RuntimeForNativeIntegration(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	installationlicense.InstallRuntime(m)
	ctx := installationlicense.WithManager(installationlicense.RequireContext(context.Background()), m)
	sentinel := errors.New("A1_FAKE_BROKER_STOP_NO_TRANSPORT")
	probe := &a1SignedV2BrokerProbe{stop: sentinel}
	c := Client{JS: probe}
	if err := c.Ensure(ctx); err != nil || probe.streamCreates != 1 {
		t.Fatalf("signed Manager did not authorize exactly one stream creation: %v %+v", err, probe)
	}
	if err := c.Consume(ctx, nil, "telmivo.events.test", "a1-test", nil); !errors.Is(err, sentinel) || probe.consumerReads != 1 {
		t.Fatalf("signed Manager did not authorize exactly one ConsumerInfo: %v %+v", err, probe)
	}
	installationlicense.InstallRuntime(nil)
	if err := c.Ensure(ctx); !errors.Is(err, installationlicense.ErrRestricted) || probe.streamCreates != 1 {
		t.Fatalf("revoked Manager allowed AddStream: %v %+v", err, probe)
	}
	if err := c.ConsumeExternal(ctx, nil, "telmivo.events.test", "a1-test", nil); !errors.Is(err, installationlicense.ErrRestricted) || probe.consumerReads != 1 {
		t.Fatalf("revoked Manager allowed ConsumerInfo: %v %+v", err, probe)
	}
}

func TestLICA9A1SignedV2NATSAdminAdmission(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestLICA9A1SignedV2NATSAdminAdmissionChild$", "-test.v")
	cmd.Env = append(os.Environ(), "LIC_A9_A1_SIGNED_CHILD=YES")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "--- PASS: TestLICA9A1SignedV2NATSAdminAdmissionChild") {
		t.Fatalf("signed V2 broker admission contract failed: %v\n%s", err, out)
	}
}
