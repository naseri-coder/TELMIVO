package appleservices

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/naseri-coder/TELMIVO/internal/domains/entitlements"
	"github.com/naseri-coder/TELMIVO/internal/platform/database"
	plugin "github.com/naseri-coder/TELMIVO/internal/plugins/appleservices"
)

const identityWindow = 10 * time.Minute

type identityVerification struct {
	Operation     Operation
	User          string
	ClientID      string
	StateHash     string
	NonceHash     string
	CodeHash      string
	ExchangeState string
	AttemptID     string
	ExpiresAt     time.Time
}

func identityDigest(v string) string {
	h := sha256.Sum256([]byte(v))
	return hex.EncodeToString(h[:])
}

func identityMAC(key []byte, value string) []byte {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write([]byte(value))
	return m.Sum(nil)
}

func identityNonce(key []byte, operation string) string {
	return base64.RawURLEncoding.EncodeToString(identityMAC(key, "nonce\x00"+operation))
}

func signIdentityState(key []byte, p identityStatePayload) (string, error) {
	if len(key) < 32 || p.Workspace == "" || p.User == "" || p.Operation == "" || p.Expires <= 0 {
		return "", database.ErrInvalid
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	sig := base64.RawURLEncoding.EncodeToString(identityMAC(key, "state\x00"+body))
	return body + "." + sig, nil
}

func parseIdentityState(key []byte, raw string, now time.Time) (identityStatePayload, error) {
	var out identityStatePayload
	if len(key) < 32 || len(raw) < 20 || len(raw) > 4096 {
		return out, database.ErrInvalid
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return out, database.ErrInvalid
	}
	got, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(got, identityMAC(key, "state\x00"+parts[0])) {
		return out, database.ErrInvalid
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(body, &out) != nil || out.Workspace == "" || out.User == "" || out.Operation == "" {
		return identityStatePayload{}, database.ErrInvalid
	}
	expiry := time.Unix(out.Expires, 0)
	if !expiry.After(now) || expiry.After(now.Add(15*time.Minute)) {
		return identityStatePayload{}, database.ErrInvalid
	}
	return out, nil
}

func (s *Service) identityReady() bool {
	return s != nil && s.DB != nil && s.IdentityProvider != nil && len(s.IdentityStateKey) >= 32 && len(s.IdentityProvider.ClientID()) > 0 && len(s.IdentityProvider.ClientID()) <= 200
}

func (s *Service) identityVerificationTx(ctx context.Context, tx pgx.Tx, workspace, operation string, lock bool) (identityVerification, error) {
	var out identityVerification
	query := `SELECT o.id::text,o.kind,o.state,o.subject_ref,o.revision,o.expires_at,o.created_at,o.updated_at,
        v.user_id::text,v.client_id,v.state_hash,v.nonce_hash,COALESCE(v.code_hash,''),v.exchange_state,COALESCE(v.attempt_id::text,''),v.expires_at
        FROM apple_identity_verifications v
        JOIN apple_service_operations o ON o.id=v.operation_id AND o.workspace_id=v.workspace_id
        WHERE v.workspace_id=$1 AND v.operation_id::text=$2`
	if lock {
		query += " FOR UPDATE OF o,v"
	}
	err := tx.QueryRow(ctx, query, workspace, operation).Scan(
		&out.Operation.ID, &out.Operation.Kind, &out.Operation.State, &out.Operation.SubjectRef, &out.Operation.Revision, &out.Operation.ExpiresAt, &out.Operation.Created, &out.Operation.Updated,
		&out.User, &out.ClientID, &out.StateHash, &out.NonceHash, &out.CodeHash, &out.ExchangeState, &out.AttemptID, &out.ExpiresAt,
	)
	return out, err
}

func identityStartFingerprint(subject, client string) string {
	return identityDigest(string(IdentityVerification) + "\x00" + subject + "\x00" + client)
}

func (s *Service) StartIdentity(ctx context.Context, actor, workspace, correlation, key string) (IdentityStart, error) {
	var out IdentityStart
	key = strings.TrimSpace(key)
	if !s.identityReady() || !validCorrelation(correlation) || !validKey(key) {
		return out, ErrIdentityUnavailable
	}
	clientID := s.IdentityProvider.ClientID()
	subject := "telmivo-user:" + actor
	fp := identityStartFingerprint(subject, clientID)
	now := s.now()
	err := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if err := entitlements.Require(ctx, tx, actor, workspace, plugin.Feature, plugin.VerifyIdentity, 0); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,7500270))", workspace+":"+key); err != nil {
			return err
		}
		replay, saved, err := s.operationByKey(ctx, tx, workspace, key)
		if err == nil {
			if saved != fp || replay.Kind != IdentityVerification || replay.SubjectRef != subject {
				return database.ErrConflict
			}
			v, err := s.identityVerificationTx(ctx, tx, workspace, replay.ID, false)
			if err != nil || v.User != actor || v.ClientID != clientID {
				return database.ErrConflict
			}
			state, err := signIdentityState(s.IdentityStateKey, identityStatePayload{Workspace: workspace, User: actor, Operation: replay.ID, Expires: v.ExpiresAt.Unix()})
			if err != nil {
				return err
			}
			nonce := identityNonce(s.IdentityStateKey, replay.ID)
			url, err := s.IdentityProvider.AuthorizationURL(state, nonce)
			if err != nil {
				return ErrIdentityUnavailable
			}
			out = IdentityStart{Operation: replay, AuthorizationURL: url, ExpiresAt: v.ExpiresAt}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		profile, created, err := s.ensureProfileTx(ctx, tx, actor, workspace)
		if err != nil {
			return err
		}
		if created {
			if err = database.Audit(ctx, tx, actor, workspace, "apple.profile.created", "apple_service_profile", workspace, correlation, map[string]any{}, map[string]any{"state": profile.State, "revision": profile.Revision}); err != nil {
				return err
			}
		}
		if profile.State != "FOUNDATION_READY" {
			return database.ErrConflict
		}
		if err = entitlements.Reserve(ctx, tx, actor, workspace, plugin.Feature, plugin.VerifyIdentity, 1); err != nil {
			return err
		}
		expiry := now.Add(identityWindow)
		var id string
		err = tx.QueryRow(ctx, `INSERT INTO apple_service_operations(workspace_id,kind,state,subject_ref,operation_key,fingerprint,actor_id,revision,expires_at,created_at,updated_at)
            VALUES($1,'IDENTITY_VERIFICATION','APPLE_ACTION_REQUIRED',$2,$3,$4,$5,3,$6,$7,$7) RETURNING id::text`,
			workspace, subject, key, fp, actor, expiry, now).Scan(&id)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO apple_service_operation_events(workspace_id,operation_id,sequence,actor_id,from_state,to_state,reason,correlation_id,created_at) VALUES
            ($1,$2,1,$3,'','REQUESTED','identity verification requested',$4,$5),
            ($1,$2,2,$3,'REQUESTED','PRECHECK','identity precheck passed',$4,$5),
            ($1,$2,3,$3,'PRECHECK','APPLE_ACTION_REQUIRED','Sign in with Apple required',$4,$5)`, workspace, id, actor, correlation, now); err != nil {
			return err
		}
		state, err := signIdentityState(s.IdentityStateKey, identityStatePayload{Workspace: workspace, User: actor, Operation: id, Expires: expiry.Unix()})
		if err != nil {
			return err
		}
		nonce := identityNonce(s.IdentityStateKey, id)
		url, err := s.IdentityProvider.AuthorizationURL(state, nonce)
		if err != nil {
			return ErrIdentityUnavailable
		}
		if _, err = tx.Exec(ctx, `INSERT INTO apple_identity_verifications(operation_id,workspace_id,user_id,client_id,state_hash,nonce_hash,expires_at,created_at,updated_at)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8)`, id, workspace, actor, clientID, identityDigest(state), identityDigest(nonce), expiry, now); err != nil {
			return err
		}
		op, err := s.operation(ctx, tx, workspace, id, false)
		if err != nil {
			return err
		}
		if err = database.Audit(ctx, tx, actor, workspace, "apple.identity.started", "apple_identity_verification", id, correlation, map[string]any{}, map[string]any{"client_id": clientID, "state": op.State, "revision": op.Revision}); err != nil {
			return err
		}
		out = IdentityStart{Operation: op, AuthorizationURL: url, ExpiresAt: expiry}
		return nil
	})
	return out, err
}

func (s *Service) IdentityStatus(ctx context.Context, actor, workspace string) (IdentityLink, error) {
	var out IdentityLink
	if !s.identityReady() {
		return out, ErrIdentityUnavailable
	}
	clientID := s.IdentityProvider.ClientID()
	err := s.DB.Tenant(ctx, actor, workspace, false, func(tx pgx.Tx) error {
		if err := entitlements.Require(ctx, tx, actor, workspace, plugin.Feature, plugin.VerifyIdentity, 0); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT client_id,email_verified,private_email,last_verified_at FROM apple_identity_links WHERE client_id=$1 AND user_id=$2`, clientID, actor).Scan(&out.ClientID, &out.EmailVerified, &out.PrivateEmail, &out.LastVerifiedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			out = IdentityLink{Linked: false}
			return nil
		}
		if err != nil {
			return err
		}
		out.Linked = true
		return nil
	})
	return out, err
}

type identityExchange struct {
	Payload      identityStatePayload
	Verification identityVerification
	CodeHash     string
	Nonce        string
	Attempt      string
	Replay       bool
}

func (s *Service) beginIdentityExchange(ctx context.Context, correlation, state, code string) (identityExchange, error) {
	var out identityExchange
	payload, err := parseIdentityState(s.IdentityStateKey, state, s.now())
	if err != nil {
		return out, err
	}
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 8192 {
		return out, database.ErrInvalid
	}
	out.Payload = payload
	out.CodeHash = identityDigest(code)
	out.Nonce = identityNonce(s.IdentityStateKey, payload.Operation)
	err = s.DB.Tenant(ctx, payload.User, payload.Workspace, false, func(tx pgx.Tx) error {
		if err := entitlements.Require(ctx, tx, payload.User, payload.Workspace, plugin.Feature, plugin.VerifyIdentity, 0); err != nil {
			return err
		}
		v, err := s.identityVerificationTx(ctx, tx, payload.Workspace, payload.Operation, true)
		if err != nil {
			return err
		}
		out.Verification = v
		if v.User != payload.User || v.ClientID != s.IdentityProvider.ClientID() || v.StateHash != identityDigest(state) || v.NonceHash != identityDigest(out.Nonce) {
			return database.ErrForbidden
		}
		if !v.ExpiresAt.After(s.now()) {
			return database.ErrConflict
		}
		if v.ExchangeState == "VERIFIED" && v.CodeHash == out.CodeHash {
			out.Replay = true
			return nil
		}
		if v.ExchangeState != "INITIATED" || v.Operation.State != AppleActionRequired {
			return database.ErrConflict
		}
		var attempt string
		if err = tx.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&attempt); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE apple_identity_verifications SET exchange_state='EXCHANGING',attempt_id=$3,code_hash=$4,updated_at=$5 WHERE workspace_id=$1 AND operation_id=$2`, payload.Workspace, payload.Operation, attempt, out.CodeHash, s.now()); err != nil {
			return err
		}
		nextRevision := v.Operation.Revision + 1
		tag, err := tx.Exec(ctx, `UPDATE apple_service_operations SET state='VERIFICATION_PENDING',revision=revision+1,updated_at=$3 WHERE workspace_id=$1 AND id=$2 AND revision=$4`, payload.Workspace, payload.Operation, s.now(), v.Operation.Revision)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		if _, err = tx.Exec(ctx, `INSERT INTO apple_service_operation_events(workspace_id,operation_id,sequence,actor_id,from_state,to_state,reason,correlation_id,created_at) VALUES($1,$2,$3,$4,'APPLE_ACTION_REQUIRED','VERIFICATION_PENDING','authorization code received',$5,$6)`, payload.Workspace, payload.Operation, nextRevision, payload.User, correlation, s.now()); err != nil {
			return err
		}
		if err = database.Audit(ctx, tx, payload.User, payload.Workspace, "apple.identity.exchange_started", "apple_identity_verification", payload.Operation, correlation, map[string]any{"state": "APPLE_ACTION_REQUIRED"}, map[string]any{"state": "VERIFICATION_PENDING", "attempt_id": attempt}); err != nil {
			return err
		}
		out.Attempt = attempt
		return nil
	})
	return out, err
}

func (s *Service) finishIdentityFailure(ctx context.Context, x identityExchange, correlation, state string) error {
	return s.DB.Tenant(ctx, x.Payload.User, x.Payload.Workspace, false, func(tx pgx.Tx) error {
		v, err := s.identityVerificationTx(ctx, tx, x.Payload.Workspace, x.Payload.Operation, true)
		if err != nil {
			return err
		}
		if v.ExchangeState != "EXCHANGING" || v.AttemptID != x.Attempt {
			return database.ErrConflict
		}
		now := s.now()
		if state == "UNKNOWN" {
			_, err = tx.Exec(ctx, `UPDATE apple_identity_verifications SET exchange_state='UNKNOWN',completed_at=$3,updated_at=$3 WHERE workspace_id=$1 AND operation_id=$2`, x.Payload.Workspace, x.Payload.Operation, now)
			if err != nil {
				return err
			}
			return database.Audit(ctx, tx, x.Payload.User, x.Payload.Workspace, "apple.identity.exchange_unknown", "apple_identity_verification", x.Payload.Operation, correlation, map[string]any{"exchange_state": "EXCHANGING"}, map[string]any{"exchange_state": "UNKNOWN", "restart_required": true})
		}
		_, err = tx.Exec(ctx, `UPDATE apple_identity_verifications SET exchange_state='REJECTED',completed_at=$3,updated_at=$3 WHERE workspace_id=$1 AND operation_id=$2`, x.Payload.Workspace, x.Payload.Operation, now)
		if err != nil {
			return err
		}
		nextRevision := v.Operation.Revision + 1
		tag, err := tx.Exec(ctx, `UPDATE apple_service_operations SET state='FAILED',revision=revision+1,updated_at=$3 WHERE workspace_id=$1 AND id=$2 AND revision=$4`, x.Payload.Workspace, x.Payload.Operation, now, v.Operation.Revision)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		if _, err = tx.Exec(ctx, `INSERT INTO apple_service_operation_events(workspace_id,operation_id,sequence,actor_id,from_state,to_state,reason,correlation_id,created_at) VALUES($1,$2,$3,$4,'VERIFICATION_PENDING','FAILED','Apple identity rejected',$5,$6)`, x.Payload.Workspace, x.Payload.Operation, nextRevision, x.Payload.User, correlation, now); err != nil {
			return err
		}
		return database.Audit(ctx, tx, x.Payload.User, x.Payload.Workspace, "apple.identity.rejected", "apple_identity_verification", x.Payload.Operation, correlation, map[string]any{"state": "VERIFICATION_PENDING"}, map[string]any{"state": "FAILED"})
	})
}

func (s *Service) finishIdentityVerified(ctx context.Context, x identityExchange, correlation string, identity VerifiedAppleIdentity) (IdentityLink, error) {
	var out IdentityLink
	subject := strings.TrimSpace(identity.Subject)
	if subject == "" || len(subject) > 512 || identityDigest(identity.Nonce) != x.Verification.NonceHash {
		return out, ErrIdentityRejected
	}
	subHash := identityDigest(subject)
	emailHash := ""
	if identity.EmailVerified && strings.TrimSpace(identity.Email) != "" {
		emailHash = identityDigest(strings.ToLower(strings.TrimSpace(identity.Email)))
	}
	err := s.DB.Tenant(ctx, x.Payload.User, x.Payload.Workspace, false, func(tx pgx.Tx) error {
		v, err := s.identityVerificationTx(ctx, tx, x.Payload.Workspace, x.Payload.Operation, true)
		if err != nil {
			return err
		}
		if v.ExchangeState != "EXCHANGING" || v.AttemptID != x.Attempt {
			return database.ErrConflict
		}
		var existing string
		err = tx.QueryRow(ctx, `SELECT subject_hash FROM apple_identity_links WHERE client_id=$1 AND user_id=$2 FOR UPDATE`, v.ClientID, x.Payload.User).Scan(&existing)
		if err == nil && existing != subHash {
			return database.ErrConflict
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		now := s.now()
		if errors.Is(err, pgx.ErrNoRows) {
			_, err = tx.Exec(ctx, `INSERT INTO apple_identity_links(client_id,subject_hash,user_id,email_hash,email_verified,private_email,created_at,last_verified_at) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$7)`, v.ClientID, subHash, x.Payload.User, emailHash, identity.EmailVerified, identity.PrivateEmail, now)
			if err != nil {
				var pe *pgconn.PgError
				if errors.As(err, &pe) && pe.Code == "23505" {
					return database.ErrConflict
				}
				return err
			}
		} else {
			_, err = tx.Exec(ctx, `UPDATE apple_identity_links SET email_hash=NULLIF($3,''),email_verified=$4,private_email=$5,last_verified_at=$6 WHERE client_id=$1 AND user_id=$2`, v.ClientID, x.Payload.User, emailHash, identity.EmailVerified, identity.PrivateEmail, now)
			if err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE apple_identity_verifications SET exchange_state='VERIFIED',completed_at=$3,updated_at=$3 WHERE workspace_id=$1 AND operation_id=$2`, x.Payload.Workspace, x.Payload.Operation, now); err != nil {
			return err
		}
		nextRevision := v.Operation.Revision + 1
		tag, err := tx.Exec(ctx, `UPDATE apple_service_operations SET state='READY',revision=revision+1,updated_at=$3 WHERE workspace_id=$1 AND id=$2 AND revision=$4`, x.Payload.Workspace, x.Payload.Operation, now, v.Operation.Revision)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		if _, err = tx.Exec(ctx, `INSERT INTO apple_service_operation_events(workspace_id,operation_id,sequence,actor_id,from_state,to_state,reason,correlation_id,created_at) VALUES($1,$2,$3,$4,'VERIFICATION_PENDING','READY','Apple identity verified',$5,$6)`, x.Payload.Workspace, x.Payload.Operation, nextRevision, x.Payload.User, correlation, now); err != nil {
			return err
		}
		if err = database.Audit(ctx, tx, x.Payload.User, x.Payload.Workspace, "apple.identity.verified", "apple_identity_link", x.Payload.User, correlation, map[string]any{}, map[string]any{"client_id": v.ClientID, "email_verified": identity.EmailVerified, "private_email": identity.PrivateEmail}); err != nil {
			return err
		}
		out = IdentityLink{Linked: true, ClientID: v.ClientID, EmailVerified: identity.EmailVerified, PrivateEmail: identity.PrivateEmail, LastVerifiedAt: now}
		return nil
	})
	if err != nil {
		return IdentityLink{}, errors.Join(ErrIdentityUnknown, err)
	}
	return out, nil
}

func (s *Service) cancelIdentityCallback(ctx context.Context, correlation string, p identityStatePayload, rawState string) (IdentityLink, error) {
	var out IdentityLink
	err := s.DB.Tenant(ctx, p.User, p.Workspace, false, func(tx pgx.Tx) error {
		v, err := s.identityVerificationTx(ctx, tx, p.Workspace, p.Operation, true)
		if err != nil {
			return err
		}
		if v.User != p.User || v.ClientID != s.IdentityProvider.ClientID() || v.StateHash != identityDigest(rawState) {
			return database.ErrForbidden
		}
		if v.ExchangeState != "INITIATED" || v.Operation.State != AppleActionRequired {
			return database.ErrConflict
		}
		now := s.now()
		if _, err = tx.Exec(ctx, `UPDATE apple_identity_verifications SET exchange_state='REJECTED',attempt_id=gen_random_uuid(),code_hash=$3,completed_at=$4,updated_at=$4 WHERE workspace_id=$1 AND operation_id=$2`, p.Workspace, p.Operation, identityDigest("apple-callback-cancelled"), now); err != nil {
			return err
		}
		nextRevision := v.Operation.Revision + 1
		tag, err := tx.Exec(ctx, `UPDATE apple_service_operations SET state='CANCELLED',revision=revision+1,updated_at=$3 WHERE workspace_id=$1 AND id=$2 AND revision=$4`, p.Workspace, p.Operation, now, v.Operation.Revision)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return database.ErrConflict
		}
		if _, err = tx.Exec(ctx, `INSERT INTO apple_service_operation_events(workspace_id,operation_id,sequence,actor_id,from_state,to_state,reason,correlation_id,created_at) VALUES($1,$2,$3,$4,'APPLE_ACTION_REQUIRED','CANCELLED','Apple authorization cancelled',$5,$6)`, p.Workspace, p.Operation, nextRevision, p.User, correlation, now); err != nil {
			return err
		}
		return database.Audit(ctx, tx, p.User, p.Workspace, "apple.identity.cancelled", "apple_identity_verification", p.Operation, correlation, map[string]any{"state": "APPLE_ACTION_REQUIRED"}, map[string]any{"state": "CANCELLED"})
	})
	return out, err
}

// recordIdentityOutcome preserves storage failures and never claims a durable rejection
// when the final transaction failed. The exchange reservation remains fenced.
func recordIdentityOutcome(record func() error, outcome error) error {
	if err := record(); err != nil {
		return errors.Join(ErrIdentityUnknown, err)
	}
	return outcome
}

func (s *Service) CompleteIdentity(ctx context.Context, correlation string, in IdentityCallback) (IdentityLink, error) {
	var out IdentityLink
	if !s.identityReady() || !validCorrelation(correlation) {
		return out, ErrIdentityUnavailable
	}
	payload, err := parseIdentityState(s.IdentityStateKey, strings.TrimSpace(in.State), s.now())
	if err != nil {
		return out, err
	}
	if strings.TrimSpace(in.Error) != "" {
		return s.cancelIdentityCallback(ctx, correlation, payload, strings.TrimSpace(in.State))
	}
	x, err := s.beginIdentityExchange(ctx, correlation, strings.TrimSpace(in.State), in.Code)
	if err != nil {
		return out, err
	}
	if x.Replay {
		return s.IdentityStatus(ctx, x.Payload.User, x.Payload.Workspace)
	}
	identity, exchangeErr := s.IdentityProvider.Exchange(ctx, strings.TrimSpace(in.Code))
	if exchangeErr != nil {
		if errors.Is(exchangeErr, ErrIdentityRejected) {
			return out, recordIdentityOutcome(func() error { return s.finishIdentityFailure(ctx, x, correlation, "REJECTED") }, ErrIdentityRejected)
		}
		return out, recordIdentityOutcome(func() error { return s.finishIdentityFailure(ctx, x, correlation, "UNKNOWN") }, ErrIdentityUnknown)
	}
	if identityDigest(identity.Nonce) != x.Verification.NonceHash {
		return out, recordIdentityOutcome(func() error { return s.finishIdentityFailure(ctx, x, correlation, "REJECTED") }, ErrIdentityRejected)
	}
	return s.finishIdentityVerified(ctx, x, correlation, identity)
}
