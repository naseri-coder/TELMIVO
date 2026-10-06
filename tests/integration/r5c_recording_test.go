//go:build integration

package integration

import (
	"context"
	"errors"
	apple "github.com/naseri-coder/TELMIVO/internal/domains/appleservices"
	"net/url"
	"strings"
	"testing"
)

type recordingFailureProvider struct {
	nonce     string
	resultErr error
	calls     int
}

func (p *recordingFailureProvider) ClientID() string { return "com.example.recording-test" }
func (p *recordingFailureProvider) AuthorizationURL(state, nonce string) (string, error) {
	p.nonce = nonce
	return "https://example.test/authorize?state=" + url.QueryEscape(state), nil
}
func (p *recordingFailureProvider) Exchange(context.Context, string) (apple.VerifiedAppleIdentity, error) {
	p.calls++
	return apple.VerifiedAppleIdentity{Subject: "synthetic-subject", Nonce: p.nonce}, p.resultErr
}

func TestR5CRecordingFailureRemainsFenced(t *testing.T) {
	for _, outcome := range []string{"rejected", "unknown", "verified"} {
		t.Run(outcome, func(t *testing.T) {
			u := fixture(t, false)
			r5aWorkspaceGrant(t, u.UserID, u.WorkspaceID, 3)
			provider := &recordingFailureProvider{}
			if outcome == "rejected" {
				provider.resultErr = apple.ErrIdentityRejected
			}
			if outcome == "unknown" {
				provider.resultErr = apple.ErrIdentityUnknown
			}
			svc := &apple.Service{DB: db, IdentityProvider: provider, IdentityStateKey: []byte(strings.Repeat("s", 32))}
			start, err := svc.StartIdentity(ctx, u.UserID, u.WorkspaceID, "r5c-start", "recording-failure")
			must(t, err)
			auth, err := url.Parse(start.AuthorizationURL)
			must(t, err)
			callback := apple.IdentityCallback{State: auth.Query().Get("state"), Code: "synthetic-one-time-code"}
			// Fault only this operation's final durable update, after the exchange reservation.
			_, err = owner.Pool.Exec(ctx, `CREATE FUNCTION r5c_reject_recording() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
    IF NEW.operation_id::text='`+start.Operation.ID+`' AND NEW.exchange_state IN ('REJECTED','UNKNOWN','VERIFIED') THEN
     RAISE EXCEPTION 'synthetic identity recording failure' USING ERRCODE='P0001';
    END IF; RETURN NEW; END $$;
    CREATE TRIGGER r5c_reject_recording BEFORE UPDATE ON apple_identity_verifications FOR EACH ROW EXECUTE FUNCTION r5c_reject_recording()`)
			must(t, err)
			t.Cleanup(func() {
				_, err := owner.Pool.Exec(ctx, `DROP TRIGGER r5c_reject_recording ON apple_identity_verifications; DROP FUNCTION r5c_reject_recording()`)
				if err != nil {
					t.Error(err)
				}
			})
			link, err := svc.CompleteIdentity(ctx, "r5c-complete", callback)
			if err == nil || !errors.Is(err, apple.ErrIdentityUnknown) || link.Linked {
				t.Fatalf("storage failure reported complete: %+v %v", link, err)
			}
			pgCode(t, err, "P0001")
			var exchangeState, operationState string
			var revision int64
			must(t, owner.Pool.QueryRow(ctx, `SELECT v.exchange_state,o.state,o.revision FROM apple_identity_verifications v JOIN apple_service_operations o ON o.id=v.operation_id WHERE v.operation_id=$1`, start.Operation.ID).Scan(&exchangeState, &operationState, &revision))
			if exchangeState != "EXCHANGING" || operationState != "VERIFICATION_PENDING" || revision != start.Operation.Revision+1 {
				t.Fatal("atomic reservation lost", exchangeState, operationState, revision)
			}
			var links, finalEvents int
			must(t, owner.Pool.QueryRow(ctx, `SELECT count(*) FROM apple_identity_links WHERE user_id=$1`, u.UserID).Scan(&links))
			must(t, owner.Pool.QueryRow(ctx, `SELECT count(*) FROM apple_service_operation_events WHERE operation_id=$1 AND to_state IN ('READY','FAILED')`, start.Operation.ID).Scan(&finalEvents))
			if links != 0 || finalEvents != 0 {
				t.Fatal("failed final transaction left successful state", links, finalEvents)
			}
			if _, err = svc.CompleteIdentity(ctx, "r5c-repeat", callback); err == nil || provider.calls != 1 {
				t.Fatal("uncertain exchange was replayed", err, provider.calls)
			}
		})
	}
}
