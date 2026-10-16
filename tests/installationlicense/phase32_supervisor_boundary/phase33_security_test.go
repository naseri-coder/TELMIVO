//go:build linux

package supervisor

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// Mock types exercise the Product Manager.Effect method signature but DO NOT
// instantiate Product's real Go1.27 Manager or Authority-signed lease.
type mockEffect struct {
	allow  bool
	calls  *atomic.Int32
	inside *atomic.Bool
}

func (m mockEffect) Effect(ctx context.Context, fn func() error) error {
	if m.calls != nil {
		m.calls.Add(1)
	}
	if !m.allow {
		return ErrUnsafe
	}
	if m.inside != nil {
		m.inside.Store(true)
		defer m.inside.Store(false)
	}
	return fn()
}

type mockPolicy struct {
	allow  bool
	calls  *atomic.Int32
	inside *atomic.Bool
}

func (m mockPolicy) Authorize(ctx context.Context, c Claim) error {
	if m.calls != nil {
		m.calls.Add(1)
	}
	if !m.allow || c.WorkspaceID != "tenantA" || c.ActorID != "actorA" || c.ResourceID != "recordA" || c.Action != ActionApply {
		return ErrUnsafe
	}
	if m.inside != nil && !m.inside.Load() {
		return ErrUnsafe
	}
	return nil
}
func TestP33SessionBindingRejectsSignedOtherSession(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	w := grant(f, ActionApply, 1, "recordA")
	w.Claim.Session = "clonedSession"
	w = SignedTestWire(w.Claim, f.private)
	expectReply(t, f, w, "DENIED_NOT_READY_OR_STALE_SESSION")
}
func TestP33OwnerBindingRejectsSignedOtherOwner(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	w := grant(f, ActionApply, 1, "recordA")
	w.Claim.Owner = "staleOwner"
	w = SignedTestWire(w.Claim, f.private)
	expectReply(t, f, w, "DENIED_NOT_READY_OR_STALE_SESSION")
}
func TestP33DomainTenantIsolation(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	w := grant(f, ActionApply, 1, "recordA")
	w.Claim.WorkspaceID = "tenantB"
	w = SignedTestWire(w.Claim, f.private)
	expectReply(t, f, w, "DENIED_FINAL_GATE_POLICY_OR_REPLAY")
}
func TestP33DomainActorIsolation(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	w := grant(f, ActionApply, 1, "recordA")
	w.Claim.ActorID = "intruder"
	w = SignedTestWire(w.Claim, f.private)
	expectReply(t, f, w, "DENIED_FINAL_GATE_POLICY_OR_REPLAY")
}
func TestP33NilGateFailsClosed(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	f.srv.mu.Lock()
	f.srv.effect = nil
	f.srv.mu.Unlock()
	expectReply(t, f, grant(f, ActionApply, 1, "recordA"), "DENIED_FINAL_EFFECT_NOT_WIRED")
}
func TestP33NilPolicyFailsClosed(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	f.srv.mu.Lock()
	f.srv.policy = nil
	f.srv.mu.Unlock()
	expectReply(t, f, grant(f, ActionApply, 1, "recordA"), "DENIED_FINAL_EFFECT_NOT_WIRED")
}
func TestP33RestrictedMockLicenseStopsEffect(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	calls := new(atomic.Int32)
	policyCalls := new(atomic.Int32)
	f.srv.mu.Lock()
	f.srv.effect = mockEffect{allow: false, calls: calls}
	f.srv.policy = mockPolicy{allow: true, calls: policyCalls}
	f.srv.mu.Unlock()
	expectReply(t, f, grant(f, ActionApply, 1, "recordA"), "DENIED_FINAL_GATE_POLICY_OR_REPLAY")
	if calls.Load() != 1 || policyCalls.Load() != 0 {
		t.Fatal("policy/handler executed outside license gate")
	}
}
func TestP33PolicyRunsInsideFinalEffectCallback(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	inside := new(atomic.Bool)
	calls := new(atomic.Int32)
	pCalls := new(atomic.Int32)
	f.srv.mu.Lock()
	f.srv.effect = mockEffect{allow: true, inside: inside, calls: calls}
	f.srv.policy = mockPolicy{allow: true, inside: inside, calls: pCalls}
	f.srv.mu.Unlock()
	expectReply(t, f, grant(f, ActionApply, 1, "recordA"), "APPLIED_SYNTHETIC_TEST_ONLY")
	if calls.Load() != 1 || pCalls.Load() != 1 || inside.Load() {
		t.Fatal("gate ordering incorrect")
	}
}
func TestP33MalformedDomainFieldsDenied(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	for _, mutate := range []func(*Claim){
		func(c *Claim) { c.WorkspaceID = "" },
		func(c *Claim) { c.ActorID = "" },
		func(c *Claim) { c.ResourceID = "" },
		func(c *Claim) { c.WorkspaceID = "../../escape" },
	} {
		w := grant(f, ActionApply, 1, "recordA")
		mutate(&w.Claim)
		w = SignedTestWire(w.Claim, f.private)
		expectReply(t, f, w, "DENIED_SIGNATURE_OR_REQUEST")
	}
}
func TestP33UnsignedDomainMutationRejected(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	w := grant(f, ActionApply, 1, "recordA")
	w.Claim.WorkspaceID = "tenantB"
	// Do NOT re-sign.
	expectReply(t, f, w, "DENIED_SIGNATURE_OR_REQUEST")
}
func TestP33ConstructorRefusesNilDependencies(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	f := setup(t)
	f.srv.Close()
	if srv, e := OpenSecured(f.socketDir, f.witnessDir, "testInstallationA", ownerUID, ownerUID, f.issuer, nil, mockPolicy{allow: true}); e == nil {
		srv.Close()
		t.Fatal("nil Effect gate allowed")
	}
	if srv, e := OpenSecured(f.socketDir, f.witnessDir, "testInstallationA", ownerUID, ownerUID, f.issuer, mockEffect{allow: true}, nil); e == nil {
		srv.Close()
		t.Fatal("nil policy allowed")
	}
}
func TestP33IssuerKeyMutationDenied(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	w := grant(f, ActionApply, 1, "recordA")
	w = SignedTestWire(w.Claim, other)
	expectReply(t, f, w, "DENIED_SIGNATURE_OR_REQUEST")
}
func TestP33BackCompatUnsafeOpenFailsClosed(t *testing.T) {
	f := setup(t)
	f.srv.Close()
	raw, e := Open(f.socketDir, f.witnessDir, "testInstallationA", ownerUID, ownerUID, f.issuer)
	if e != nil {
		t.Fatal(e)
	}
	f.srv = raw
	go raw.Serve()
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	expectReply(t, f, grant(f, ActionApply, 1, "recordA"), "DENIED_FINAL_EFFECT_NOT_WIRED")
}
func TestP33DeniedByPolicyNoJournalMutation(t *testing.T) {
	f := setup(t)
	expectReply(t, f, grant(f, ActionFence, 1, ""), "FENCED_TEST_ONLY")
	f.srv.mu.Lock()
	f.srv.policy = mockPolicy{allow: false}
	f.srv.mu.Unlock()
	expectReply(t, f, grant(f, ActionApply, 1, "recordA"), "DENIED_FINAL_GATE_POLICY_OR_REPLAY")
	w := grant(f, ActionApply, 1, "recordA")
	f.srv.mu.Lock()
	f.srv.policy = mockPolicy{allow: true}
	f.srv.mu.Unlock()
	expectReply(t, f, w, "APPLIED_SYNTHETIC_TEST_ONLY")
}
func TestP33RejectUnboundedOtherOperation(t *testing.T) {
	f := setup(t)
	w := grant(f, ActionApply, 1, "id1")
	w.Claim.Action = "EXECUTE_SHELL"
	w = SignedTestWire(w.Claim, f.private)
	expectReply(t, f, w, "DENIED_SIGNATURE_OR_REQUEST")
	if strings.Contains(w.Claim.Action, "SYNTHETIC") {
		t.Fatal("test invalid")
	}
}
